// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eiffel-community/etos/pkg/opentelemetry"
	"github.com/go-logr/logr"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/internal/controller/jobs"
	"github.com/eiffel-community/etos/internal/controller/status"
	healthmetrics "github.com/eiffel-community/etos/internal/metrics"
	"github.com/eiffel-community/etos/pkg/messaging/events"
)

var transitionTestTime = time.Date(2026, time.June, 7, 8, 9, 10, 0, time.UTC)

// TestSuiteRunnerJobSetsAPIVersion verifies controller-created suite runners receive API provenance.
func TestSuiteRunnerJobSetsAPIVersion(t *testing.T) {
	scheme := newTransitionTestScheme(t)
	cluster := &etosv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name: "cluster", Namespace: "default",
	}}
	testRun := &etosv1alpha1.TestRun{
		ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: "default"},
		Spec: etosv1alpha1.TestRunSpec{
			Cluster:     cluster.Name,
			SuiteRunner: &etosv1alpha1.SuiteRunner{Image: &etosv1alpha1.Image{Image: "suite-runner"}},
		},
	}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	reconciler := TestRunReconciler{Client: kubeClient, Scheme: scheme}

	job, err := reconciler.suiteRunnerJob(context.Background(), testRun)
	if err != nil {
		t.Fatalf("suiteRunnerJob() error = %v", err)
	}
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "ETOS_API_VERSION" {
			if env.Value != "v1beta1" {
				t.Fatalf("ETOS_API_VERSION = %q, want %q", env.Value, "v1beta1")
			}
			return
		}
	}
	t.Fatal("suite runner job is missing ETOS_API_VERSION")
}

// TestClusterTransitionsPersistOnce verifies terminal counters route to real Clusters without replay on resync.
func TestClusterTransitionsPersistOnce(t *testing.T) {
	transitions, registry := newControllerTransitionRegistry(t)
	for _, location := range []struct{ namespace, cluster string }{
		{"team", "blue"}, {"team", "green"}, {"other-team", "blue"},
	} {
		identity := location.namespace + "/" + location.cluster
		transitions.InitializeCluster(identity)
		cluster := &etosv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{
			Name: location.cluster, Namespace: location.namespace,
		}}
		run := terminalTestRun(status.ReasonCompleted)
		run.Name = location.cluster + "-run"
		run.Namespace = location.namespace
		run.Spec.Cluster = location.cluster
		kubeClient := fake.NewClientBuilder().WithScheme(newTransitionTestScheme(t)).
			WithStatusSubresource(&etosv1alpha1.TestRun{}).WithObjects(run, cluster).Build()
		reconciler := &TestRunReconciler{
			Client: kubeClient, Clock: transitionTestClock{now: transitionTestTime}, Transitions: transitions,
		}
		if updated, err := reconciler.reconcileActiveStatus(context.Background(), run); err != nil || !updated {
			t.Fatalf("initial terminal transition = %t, %v", updated, err)
		}
		persisted := &etosv1alpha1.TestRun{}
		if err := kubeClient.Get(context.Background(), client.ObjectKeyFromObject(run), persisted); err != nil {
			t.Fatal(err)
		}
		if updated, err := reconciler.reconcileActiveStatus(context.Background(), persisted); err != nil || updated {
			t.Fatalf("repeated terminal transition = %t, %v", updated, err)
		}
		request, job := terminalEnvironmentRequest(transitionTestTime.Add(-time.Minute),
			batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
			timePtr(transitionTestTime))
		request.Namespace, job.Namespace = location.namespace, location.namespace
		request.Labels = map[string]string{"etos.eiffel-community.github.io/cluster": location.cluster}
		requestReconciler, requestClient := newEnvironmentRequestMetricReconciler(t, transitions, request, job)
		requestCluster := cluster.DeepCopy()
		requestCluster.ResourceVersion = ""
		if err := requestClient.Create(context.Background(), requestCluster); err != nil {
			t.Fatal(err)
		}
		if err := requestReconciler.updateTerminalStatus(context.Background(), request, jobs.StatusSuccessful); err != nil {
			t.Fatal(err)
		}
		if _, err := requestReconciler.Reconcile(context.Background(), ctrlRequest(request)); err != nil {
			t.Fatal(err)
		}
		families := collectTransitionMetrics(t, registry)
		for _, name := range []string{"etos_testrun_finished_total", "etos_environment_request_finished_total"} {
			found := false
			for _, family := range families {
				if family.Name != name {
					continue
				}
				for _, metric := range family.Data.(metricdata.Sum[int64]).DataPoints {
					cluster, _ := metric.Attributes.Value("etos_cluster")
					outcome, _ := metric.Attributes.Value("outcome")
					if cluster.AsString() == identity && outcome.AsString() == healthmetrics.OutcomeSuccess {
						found = true
						if metric.Value != 1 {
							t.Errorf("%s %s = %v, want exactly one persisted transition", name, identity, metric.Value)
						}
					}
				}
			}
			if !found {
				t.Errorf("%s missing real Cluster identity %s", name, identity)
			}
		}
	}
}

// TestPersistedTerminalTestRunRecordsOnceAndSurvivesDeletion verifies terminal observations are retained once.
func TestPersistedTerminalTestRunRecordsOnceAndSurvivesDeletion(t *testing.T) {
	tests := []struct {
		name    string
		reason  string
		message string
		verdict string
		result  string
	}{
		{name: "successful", reason: status.ReasonCompleted, verdict: string(jobs.VerdictFailed), result: healthmetrics.OutcomeSuccess},
		{name: "failed", reason: status.ReasonFailed, verdict: string(jobs.VerdictPassed), result: healthmetrics.OutcomeFailure},
		{name: "incidental abort", reason: status.ReasonFailed, message: "Suite runner aborted", verdict: string(jobs.VerdictInconclusive), result: healthmetrics.OutcomeFailure},
		{name: "incidental timeout", reason: status.ReasonFailed, message: "Suite runner timed out", result: healthmetrics.OutcomeFailure},
		{name: "failed tests", reason: status.ReasonCompleted, message: "Tests failed", verdict: string(jobs.VerdictFailed), result: healthmetrics.OutcomeSuccess},
		{name: "incidental inconclusive", reason: status.ReasonCompleted, message: "Suite runner inconclusive", result: healthmetrics.OutcomeSuccess},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transitions, registry := newControllerTransitionRegistry(t)

			scheme := newTransitionTestScheme(t)
			testrun := terminalTestRun(test.reason)
			if test.message != "" {
				testrun.Status.Conditions[0].Message = test.message
			}
			testrun.Status.Verdict = test.verdict
			kubeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&etosv1alpha1.TestRun{}).
				WithObjects(testrun).
				Build()
			reconciler := &TestRunReconciler{
				Client:      kubeClient,
				Clock:       transitionTestClock{now: transitionTestTime},
				Transitions: transitions,
			}

			updated, err := reconciler.reconcileActiveStatus(context.Background(), testrun)
			if err != nil || !updated {
				t.Fatalf("reconcileActiveStatus() = %t, %v; want true, nil", updated, err)
			}
			reloaded := &etosv1alpha1.TestRun{}
			if err := kubeClient.Get(context.Background(), client.ObjectKeyFromObject(testrun), reloaded); err != nil {
				t.Fatalf("Get(TestRun) after status update error = %v", err)
			}
			updated, err = reconciler.reconcileActiveStatus(context.Background(), reloaded)
			wantVerdictUpdate := test.reason == status.ReasonFailed && test.verdict == ""
			if err != nil || updated != wantVerdictUpdate {
				t.Fatalf("repeat reconcileActiveStatus() = %t, %v; want %t, nil", updated, err, wantVerdictUpdate)
			}
			if wantVerdictUpdate {
				if err := kubeClient.Get(context.Background(), client.ObjectKeyFromObject(testrun), reloaded); err != nil {
					t.Fatal(err)
				}
				if reloaded.Status.Verdict != string(jobs.VerdictInconclusive) {
					t.Fatalf("terminal verdict = %q, want inconclusive", reloaded.Status.Verdict)
				}
				if updated, err := reconciler.reconcileActiveStatus(context.Background(), reloaded); err != nil || updated {
					t.Fatalf("reconcile after verdict persistence = %t, %v; want false, nil", updated, err)
				}
			}
			if got := transitionCounterValue(t, registry, "etos_testrun_finished_total", test.result); got != 1 {
				t.Errorf("finished count = %d, want 1 after repeat reconcile", got)
			}
			if got := transitionHistogramCount(t, registry, "etos_testrun_duration_seconds", test.result); got != 1 {
				t.Errorf("duration count = %d, want 1 after persisted terminal status", got)
			}

			if err := kubeClient.Delete(context.Background(), reloaded); err != nil {
				t.Fatalf("Delete(TestRun) error = %v", err)
			}
			if got := transitionCounterValue(t, registry, "etos_testrun_finished_total", test.result); got != 1 {
				t.Errorf("finished count after TestRun deletion = %d, want 1", got)
			}
		})
	}
}

// TestOperationalOutcomeUnknownDoesNotRequireNewStatusReasons verifies missing evidence stays unknown.
func TestOperationalOutcomeUnknownDoesNotRequireNewStatusReasons(t *testing.T) {
	conditions := []metav1.Condition{{
		Type:    status.StatusSuiteRunner,
		Status:  metav1.ConditionFalse,
		Reason:  status.ReasonPending,
		Message: "No operational conclusion available",
	}}
	if got := classifyTestRunOperationalOutcome(conditions); got != healthmetrics.OutcomeUnknown {
		t.Fatalf("operational outcome = %q, want unknown", got)
	}
}

// TestCollapsedSuiteRunnerConclusionUsesPersistedReason verifies free text cannot recover a collapsed conclusion.
func TestCollapsedSuiteRunnerConclusionUsesPersistedReason(t *testing.T) {
	transitions, registry := newControllerTransitionRegistry(t)

	ctx := context.Background()
	scheme := newTransitionTestScheme(t)
	testrun := terminalTestRun(status.ReasonActive)
	testrun.Status.Conditions[0].Status = metav1.ConditionTrue
	testrun.Status.Conditions[1].Reason = status.ReasonActive
	testrun.Status.Conditions = append(testrun.Status.Conditions, metav1.Condition{
		Type:               status.StatusEnvironment,
		Status:             metav1.ConditionTrue,
		Reason:             status.ReasonCompleted,
		LastTransitionTime: metav1.NewTime(transitionTestTime.Add(-time.Minute)),
	})
	controller := true
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "suite-runner-job",
			Namespace: testrun.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: APIGroupVersionString,
				Kind:       testRunKind,
				Name:       testrun.Name,
				UID:        "test-run-uid",
				Controller: &controller,
			}},
		},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
			Type:               batchv1.JobFailed,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(transitionTestTime),
		}}},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "suite-runner-pod",
			Namespace: testrun.Namespace,
			Labels:    map[string]string{"job-name": job.Name},
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name: testrun.Name,
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					Message: `{"conclusion":"Aborted","verdict":"Inconclusive","description":"Suite runner aborted"}`,
				},
			},
		}}},
	}
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&etosv1alpha1.TestRun{}).
		WithIndex(&batchv1.Job{}, TestRunOwnerKey, func(object client.Object) []string {
			owner := metav1.GetControllerOf(object)
			if owner == nil {
				return nil
			}
			return []string{owner.Name}
		}).
		WithObjects(testrun, job, pod).
		Build()
	reconciler := &TestRunReconciler{
		Client:      kubeClient,
		Clock:       transitionTestClock{now: transitionTestTime},
		Transitions: transitions,
	}
	jobManager := jobs.NewJob(kubeClient, TestRunOwnerKey, testrun.Name, testrun.Namespace)
	jobStatus, err := jobManager.Status(ctx)
	if err != nil || jobStatus != jobs.StatusFailed {
		t.Fatalf("Job.Status() = %q, %v; want Failed, nil", jobStatus, err)
	}
	updated, err := reconciler.reconcileSuiteRunner(
		ctx,
		testrun,
		jobManager,
		jobStatus,
		transitionTestEventPublisher{},
	)
	if err != nil || !updated {
		t.Fatalf("reconcileSuiteRunner() = %t, %v; want true, nil", updated, err)
	}

	reloaded := &etosv1alpha1.TestRun{}
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(testrun), reloaded); err != nil {
		t.Fatalf("Get(TestRun) after suite runner status update error = %v", err)
	}
	suiteRunnerCondition := metav1.Condition{}
	for _, condition := range reloaded.Status.Conditions {
		if condition.Type == status.StatusSuiteRunner {
			suiteRunnerCondition = condition
			break
		}
	}
	if suiteRunnerCondition.Reason != status.ReasonFailed {
		t.Fatalf("SuiteRunner reason = %q, want unchanged %q", suiteRunnerCondition.Reason, status.ReasonFailed)
	}
	updated, err = reconciler.reconcileActiveStatus(ctx, reloaded)
	if err != nil || !updated {
		t.Fatalf("reconcileActiveStatus() = %t, %v; want true, nil", updated, err)
	}
	if got := transitionCounterValue(t, registry, "etos_testrun_finished_total", healthmetrics.OutcomeAborted); got != 0 {
		t.Errorf("aborted finished count = %d, want 0 without authoritative persisted evidence", got)
	}
	if got := transitionCounterValue(t, registry, "etos_testrun_finished_total", healthmetrics.OutcomeFailure); got != 1 {
		t.Errorf("failure finished count for persisted Failed reason = %d, want 1", got)
	}
	if got := transitionHistogramCount(t, registry, "etos_testrun_duration_seconds", healthmetrics.OutcomeFailure); got != 1 {
		t.Errorf("failure duration count = %d, want 1", got)
	}
}

// TestOperationalOutcomeUsesAuthoritativeReason verifies free-text descriptions never override lifecycle reasons.
func TestOperationalOutcomeUsesAuthoritativeReason(t *testing.T) {
	tests := []struct {
		reason  string
		message string
		want    string
	}{
		{status.ReasonCompleted, "Tests failed", healthmetrics.OutcomeSuccess},
		{status.ReasonFailed, "Failed after an incidental abort and deadline timeout", healthmetrics.OutcomeFailure},
		{status.ReasonTimedOut, "Tests passed", healthmetrics.OutcomeTimedOut},
		{status.ReasonPending, "Completed successfully", healthmetrics.OutcomeUnknown},
	}
	for _, test := range tests {
		t.Run(test.reason, func(t *testing.T) {
			condition := metav1.Condition{
				Type:    status.StatusSuiteRunner,
				Status:  metav1.ConditionFalse,
				Reason:  test.reason,
				Message: test.message,
			}
			if got := classifyTestRunOperationalOutcome([]metav1.Condition{condition}); got != test.want {
				t.Fatalf("operational outcome = %q, want %q", got, test.want)
			}
		})
	}
}

// TestTestRunStatusConflictDoesNotRecordMetrics verifies an unsuccessful status write produces no terminal sample.
func TestTestRunStatusConflictDoesNotRecordMetrics(t *testing.T) {
	transitions, registry := newControllerTransitionRegistry(t)

	scheme := newTransitionTestScheme(t)
	testrun := terminalTestRun(status.ReasonCompleted)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&etosv1alpha1.TestRun{}).
		WithObjects(testrun).
		Build()
	reconciler := &TestRunReconciler{
		Client:      statusConflictClient{Client: kubeClient},
		Clock:       transitionTestClock{now: transitionTestTime},
		Transitions: transitions,
	}

	updated, err := reconciler.reconcileActiveStatus(context.Background(), testrun)
	if !updated || !apierrors.IsConflict(err) {
		t.Fatalf("reconcileActiveStatus() = %t, %v; want true and conflict", updated, err)
	}
	if got := transitionCounterValue(t, registry, "etos_testrun_finished_total", healthmetrics.OutcomeSuccess); got != 0 {
		t.Errorf("finished count after conflict = %d, want 0", got)
	}
	if got := transitionHistogramCount(t, registry, "etos_testrun_duration_seconds", healthmetrics.OutcomeSuccess); got != 0 {
		t.Errorf("duration count after conflict = %d, want 0", got)
	}
}

// TestEnvironmentRequestTerminalMetricsUseJobTime verifies Job completion and failed-condition timestamps are used.
func TestEnvironmentRequestTerminalMetricsUseJobTime(t *testing.T) {
	tests := []struct {
		name       string
		jobStatus  jobs.Status
		condition  batchv1.JobCondition
		outcome    string
		duration   time.Duration
		completion *metav1.Time
	}{
		{
			name:      "successful",
			jobStatus: jobs.StatusSuccessful,
			condition: batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
			outcome:   healthmetrics.OutcomeSuccess,
			duration:  2 * time.Second,
		},
		{
			name:      "failed",
			jobStatus: jobs.StatusFailed,
			condition: batchv1.JobCondition{
				Type:               batchv1.JobFailed,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(transitionTestTime.Add(-time.Minute)),
			},
			outcome:  healthmetrics.OutcomeFailure,
			duration: 3 * time.Second,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transitions, registry := newControllerTransitionRegistry(t)

			createdAt := transitionTestTime.Add(-time.Hour)
			terminalAt := createdAt.Add(test.duration)
			test.condition.LastTransitionTime = metav1.NewTime(terminalAt)
			completionTime := test.completion
			if test.jobStatus == jobs.StatusSuccessful {
				completionTime = timePtr(terminalAt)
			}
			environmentrequest, job := terminalEnvironmentRequest(createdAt, test.condition, completionTime)
			reconciler, kubeClient := newEnvironmentRequestMetricReconciler(t, transitions, environmentrequest, job)

			if err := reconciler.updateTerminalStatus(context.Background(), environmentrequest, test.jobStatus); err != nil {
				t.Fatalf("updateTerminalStatus() error = %v", err)
			}
			if got := transitionCounterValue(t, registry, "etos_environment_request_finished_total", test.outcome); got != 1 {
				t.Errorf("finished count = %d, want 1", got)
			}
			if got := environmentRequestDurationSum(t, registry, test.outcome); got != test.duration.Seconds() {
				t.Errorf("duration sum = %f, want %f", got, test.duration.Seconds())
			}

			// A restart/resync of an already-terminal resource follows the terminal cleanup branch, not the emit path.
			if _, err := reconciler.Reconcile(context.Background(), ctrlRequest(environmentrequest)); err != nil {
				t.Fatalf("terminal Reconcile() error = %v", err)
			}
			if got := transitionCounterValue(t, registry, "etos_environment_request_finished_total", test.outcome); got != 1 {
				t.Errorf("finished count after terminal reconcile = %d, want 1", got)
			}
			if err := kubeClient.Delete(context.Background(), environmentrequest); err != nil {
				t.Fatalf("Delete(EnvironmentRequest) error = %v", err)
			}
			if got := transitionCounterValue(t, registry, "etos_environment_request_finished_total", test.outcome); got != 1 {
				t.Errorf("finished count after deletion = %d, want 1", got)
			}
		})
	}
}

// TestEnvironmentRequestStatusConflictDoesNotRecordMetrics verifies failed persistence does not increment terminal counters.
func TestEnvironmentRequestStatusConflictDoesNotRecordMetrics(t *testing.T) {
	transitions, registry := newControllerTransitionRegistry(t)

	createdAt := transitionTestTime.Add(-time.Minute)
	terminalAt := createdAt.Add(5 * time.Second)
	environmentrequest, job := terminalEnvironmentRequest(
		createdAt,
		batchv1.JobCondition{
			Type:               batchv1.JobComplete,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(terminalAt),
		},
		timePtr(terminalAt),
	)
	reconciler, kubeClient := newEnvironmentRequestMetricReconciler(t, transitions, environmentrequest, job)
	reconciler.Client = statusConflictClient{Client: kubeClient}

	err := reconciler.updateTerminalStatus(context.Background(), environmentrequest, jobs.StatusSuccessful)
	if !apierrors.IsConflict(err) {
		t.Fatalf("updateTerminalStatus() error = %v, want conflict", err)
	}
	if got := transitionCounterValue(t, registry, "etos_environment_request_finished_total", healthmetrics.OutcomeSuccess); got != 0 {
		t.Errorf("finished count after conflict = %d, want 0", got)
	}
}

// TestEnvironmentRequestInvalidTimestampsOmitDuration verifies a real Job result without valid endpoints has no duration.
func TestEnvironmentRequestInvalidTimestampsOmitDuration(t *testing.T) {
	transitions, registry := newControllerTransitionRegistry(t)

	environmentrequest, job := terminalEnvironmentRequest(
		transitionTestTime.Add(-time.Minute),
		batchv1.JobCondition{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
		nil,
	)
	reconciler, _ := newEnvironmentRequestMetricReconciler(t, transitions, environmentrequest, job)
	if err := reconciler.updateTerminalStatus(context.Background(), environmentrequest, jobs.StatusFailed); err != nil {
		t.Fatalf("updateTerminalStatus() error = %v", err)
	}
	if got := transitionCounterValue(t, registry, "etos_environment_request_finished_total", healthmetrics.OutcomeFailure); got != 1 {
		t.Errorf("finished count = %d, want 1", got)
	}
	if got := transitionHistogramCount(t, registry, "etos_environment_request_duration_seconds", healthmetrics.OutcomeFailure); got != 0 {
		t.Errorf("duration count = %d, want 0 without a real JobFailed transition timestamp", got)
	}
}

// newTransitionTestScheme creates the API scheme required by fake transition tests.
func newTransitionTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		etosv1alpha1.AddToScheme,
		etosv1alpha2.AddToScheme,
		corev1.AddToScheme,
		batchv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatalf("AddToScheme() error = %v", err)
		}
	}
	return scheme
}

// terminalTestRun creates a TestRun waiting for the specified terminal suite-runner result.
func terminalTestRun(reason string) *etosv1alpha1.TestRun {
	now := metav1.NewTime(transitionTestTime.Add(-time.Hour))
	return &etosv1alpha1.TestRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-run",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Status: etosv1alpha1.TestRunStatus{
			Conditions: []metav1.Condition{
				{
					Type:               status.StatusSuiteRunner,
					Status:             metav1.ConditionFalse,
					Reason:             reason,
					LastTransitionTime: now,
				},
				{
					Type:               status.StatusActive,
					Status:             metav1.ConditionFalse,
					Reason:             status.ReasonPending,
					LastTransitionTime: now,
				},
			},
		},
	}
}

// terminalEnvironmentRequest creates an EnvironmentRequest and owned provider Job with actual terminal time data.
func terminalEnvironmentRequest(
	createdAt time.Time,
	condition batchv1.JobCondition,
	completionTime *metav1.Time,
) (*etosv1alpha1.EnvironmentRequest, *batchv1.Job) {
	trueValue := true
	request := &etosv1alpha1.EnvironmentRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "environment-request",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(createdAt),
		},
		Spec: etosv1alpha1.EnvironmentRequestSpec{
			Providers: etosv1alpha1.EnvironmentProviders{
				IUT: etosv1alpha1.IutProvider{ID: "configured-iut"},
				ExecutionSpace: etosv1alpha1.ExecutionSpaceProvider{
					ID: "configured-execution-space",
				},
				LogArea: etosv1alpha1.LogAreaProvider{ID: "configured-log-area"},
			},
		},
		Status: etosv1alpha1.EnvironmentRequestStatus{
			Conditions: []metav1.Condition{
				{
					Type:               status.StatusReady,
					Status:             metav1.ConditionFalse,
					Reason:             status.ReasonFailed,
					LastTransitionTime: metav1.NewTime(condition.LastTransitionTime.Time),
				},
			},
			CompletionTime: &metav1.Time{Time: condition.LastTransitionTime.Time},
		},
	}
	if condition.Type == batchv1.JobComplete {
		request.Status.Conditions[0].Status = metav1.ConditionTrue
		request.Status.Conditions[0].Reason = status.ReasonCompleted
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "environment-provider-job",
			Namespace: request.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: APIGroupVersionString,
					Kind:       environmentRequestKind,
					Name:       request.Name,
					UID:        "uid",
					Controller: &trueValue,
				},
			},
		},
		Status: batchv1.JobStatus{
			Conditions:     []batchv1.JobCondition{condition},
			CompletionTime: completionTime,
		},
	}
	return request, job
}

// newEnvironmentRequestMetricReconciler configures a fake indexed Job client and status-subresource client.
func newEnvironmentRequestMetricReconciler(
	t *testing.T,
	transitions *healthmetrics.Transitions,
	environmentrequest *etosv1alpha1.EnvironmentRequest,
	job *batchv1.Job,
) (*EnvironmentRequestReconciler, client.Client) {
	t.Helper()
	scheme := newTransitionTestScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&etosv1alpha1.EnvironmentRequest{}).
		WithIndex(&batchv1.Job{}, EnvironmentRequestOwnerKey, func(object client.Object) []string {
			owner := metav1.GetControllerOf(object)
			if owner == nil {
				return nil
			}
			return []string{owner.Name}
		}).
		WithObjects(environmentrequest, job).
		Build()
	return &EnvironmentRequestReconciler{
		Client:      kubeClient,
		Scheme:      scheme,
		Transitions: transitions,
	}, kubeClient
}

// newControllerTransitionRegistry registers transition collectors for deterministic controller tests.
func newControllerTransitionRegistry(t *testing.T) (*healthmetrics.Transitions, *sdkmetric.ManualReader) {
	t.Helper()
	registry := sdkmetric.NewManualReader()
	options := append(opentelemetry.MetricProviderOptions(), sdkmetric.WithReader(registry))
	provider := sdkmetric.NewMeterProvider(options...)
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	transitions, err := healthmetrics.NewTransitions(provider.Meter("etos/controller"))
	if err != nil {
		t.Fatal(err)
	}
	return transitions, registry
}

// collectTransitionMetrics collects and flattens all metric scopes, failing the test on collection errors.
func collectTransitionMetrics(t *testing.T, reader *sdkmetric.ManualReader) []metricdata.Metrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	metrics := make([]metricdata.Metrics, 0, 4)
	for _, scope := range data.ScopeMetrics {
		metrics = append(metrics, scope.Metrics...)
	}
	return metrics
}

// transitionCounterValue returns one transition counter outcome from a registry snapshot.
func transitionCounterValue(t *testing.T, registry *sdkmetric.ManualReader, name, outcome string) int64 {
	t.Helper()
	for _, instrument := range collectTransitionMetrics(t, registry) {
		if instrument.Name != name {
			continue
		}
		for _, point := range instrument.Data.(metricdata.Sum[int64]).DataPoints {
			cluster, _ := point.Attributes.Value("etos_cluster")
			result, _ := point.Attributes.Value("outcome")
			if cluster.AsString() == healthmetrics.UnassignedCluster && result.AsString() == outcome {
				return point.Value
			}
		}
	}
	t.Fatalf("missing counter baseline %s/%s", name, outcome)
	return 0
}

// environmentRequestDurationSum returns the sum of the histogram matching an outcome.
func environmentRequestDurationSum(t *testing.T, registry *sdkmetric.ManualReader, outcome string) float64 {
	t.Helper()
	point := transitionHistogram(t, registry, "etos_environment_request_duration_seconds", outcome)
	if point == nil {
		t.Fatal("missing real duration observation")
	}
	return point.Sum
}

// transitionHistogramCount returns one histogram outcome's count from a registry snapshot.
func transitionHistogramCount(t *testing.T, registry *sdkmetric.ManualReader, name, outcome string) uint64 {
	t.Helper()
	point := transitionHistogram(t, registry, name, outcome)
	if point == nil {
		return 0 // Absence, not a fabricated exported histogram.
	}
	return point.Count
}

// transitionHistogram returns the histogram point matching the name and outcome for the unassigned cluster,
// or nil if no matching observation exists.
func transitionHistogram(t *testing.T, registry *sdkmetric.ManualReader, name, outcome string) *metricdata.HistogramDataPoint[float64] {
	t.Helper()
	for _, instrument := range collectTransitionMetrics(t, registry) {
		if instrument.Name != name {
			continue
		}
		for _, point := range instrument.Data.(metricdata.Histogram[float64]).DataPoints {
			cluster, _ := point.Attributes.Value("etos_cluster")
			result, _ := point.Attributes.Value("outcome")
			if cluster.AsString() == healthmetrics.UnassignedCluster && result.AsString() == outcome {
				return &point
			}
		}
	}
	return nil
}

// timePtr returns a pointer to a metav1 timestamp.
func timePtr(value time.Time) *metav1.Time {
	result := metav1.NewTime(value)
	return &result
}

// ctrlRequest returns the reconcile request identifying an EnvironmentRequest.
func ctrlRequest(environmentrequest *etosv1alpha1.EnvironmentRequest) ctrl.Request {
	return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(environmentrequest)}
}

// transitionTestClock provides a fixed timestamp to reconcile transition tests deterministically.
type transitionTestClock struct {
	now time.Time
}

// Now returns the fixed time configured for a transition test.
func (c transitionTestClock) Now() time.Time {
	return c.now
}

// statusConflictClient injects a conflict into status writes while preserving ordinary fake-client behavior.
type statusConflictClient struct {
	client.Client
}

// Status returns a writer that rejects status updates with a simulated resource-version conflict.
func (c statusConflictClient) Status() client.SubResourceWriter {
	return conflictStatusWriter{SubResourceWriter: c.Client.Status()}
}

// conflictStatusWriter rejects status updates and delegates other subresource operations.
type conflictStatusWriter struct {
	client.SubResourceWriter
}

// Update simulates a Kubernetes status conflict without changing the stored object.
func (w conflictStatusWriter) Update(
	_ context.Context,
	object client.Object,
	_ ...client.SubResourceUpdateOption,
) error {
	return apierrors.NewConflict(
		schema.GroupResource{Group: etosv1alpha1.GroupVersion.Group, Resource: "status"},
		object.GetName(),
		errors.New("simulated conflict"),
	)
}

// transitionTestEventPublisher accepts controller events without external side effects.
type transitionTestEventPublisher struct{}

// Publish accepts an event emitted by a transition test.
func (transitionTestEventPublisher) Publish(_ string, _ events.Event) error {
	return nil
}

// AddLogger satisfies the event publisher interface for transition tests.
func (transitionTestEventPublisher) AddLogger(_ logr.Logger) {}

// Close satisfies the event publisher interface for transition tests.
func (transitionTestEventPublisher) Close() error {
	return nil
}
