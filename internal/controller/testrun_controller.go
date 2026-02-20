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
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/controller/jobs"
	"github.com/eiffel-community/etos/internal/controller/status"
)

const testRunKind = "TestRun"

// TestRunReconciler reconciles a TestRun object
type TestRunReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Clock
}

/*
We'll mock out the clock to make it easier to jump around in time while testing,
the "real" clock just calls `time.Now`.
*/
type realClock struct{}

func (_ realClock) Now() time.Time { return time.Now() }

// Clock knows how to get the current time.
// It can be used to fake out timing for testing.
type Clock interface {
	Now() time.Time
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments,verbs=get;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments/status,verbs=get
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentrequests,verbs=get;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentsrequests/status,verbs=get
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers,verbs=get;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers/status,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *TestRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger = logger.WithValues("namespace", req.Namespace, "name", req.Name)

	testrun := &etosv1alpha1.TestRun{}
	if err := r.Get(ctx, req.NamespacedName, testrun); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Testrun not found, exiting")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Error getting testrun")
		return ctrl.Result{}, err
	}

	if testrun.Status.CompletionTime != nil {
		testrunCondition := meta.FindStatusCondition(testrun.Status.Conditions, status.StatusActive)
		var retention *metav1.Duration
		if testrunCondition.Reason == status.ReasonCompleted {
			retention = testrun.Spec.Retention.Success
		} else {
			retention = testrun.Spec.Retention.Failure
		}
		if retention == nil {
			logger.Info("No retention set, ignoring")
			return ctrl.Result{}, nil
		}
		if testrun.Status.CompletionTime.Add(retention.Duration).Before(time.Now()) {
			logger.Info(fmt.Sprintf("Testrun TTL(%s) reached, delete", retention.Duration))
			if err := r.Delete(ctx, testrun, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "Failed deletion. Ignoring any errors and won't retry")
				}
			}
		} else {
			next := testrun.Status.CompletionTime.Add(retention.Duration).Sub(r.Now())
			logger.Info(fmt.Sprintf("Testrun queued for deletion in %s", next))
			return ctrl.Result{RequeueAfter: next}, nil
		}
		return ctrl.Result{}, nil
	}
	clusterNamespacedName := types.NamespacedName{
		Name:      testrun.Spec.Cluster,
		Namespace: req.NamespacedName.Namespace,
	}
	cluster := &etosv1alpha1.Cluster{}
	if err := r.Get(ctx, clusterNamespacedName, cluster); err != nil {
		logger.Info("Failed to get cluster resource!")
		return ctrl.Result{}, nil
	}

	if err := r.reconcile(ctx, cluster, testrun); err != nil {
		if apierrors.IsConflict(err) {
			logger.Error(err, "Conflict when updating testrun")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Reconciliation failed for testrun", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *TestRunReconciler) reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster, testrun *etosv1alpha1.TestRun) error {
	// Check providers availability
	if err := checkProviders(ctx, r, testrun.Namespace, testrun.Spec.Providers); err != nil {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{
			Type:    status.StatusActive,
			Status:  metav1.ConditionFalse,
			Reason:  status.ReasonFailed,
			Message: err.Error(),
		}) {
			return errors.Join(err, r.Status().Update(ctx, testrun))
		}
		return err
	}

	// Create environment request
	if updated, err := r.reconcileEnvironmentRequest(ctx, cluster, testrun); updated || err != nil {
		return err
	}

	// Check environment status
	if updated, err := r.checkEnvironment(ctx, testrun); updated || err != nil {
		return err
	}

	// Reconcile suite runners
	jobManager := jobs.NewJob(r.Client, TestRunOwnerKey, testrun.GetName(), testrun.GetNamespace())
	jobStatus, err := jobManager.Status(ctx)
	if err != nil {
		return err
	}
	if updated, err := r.reconcileSuiteRunner(ctx, testrun, jobManager, jobStatus); updated || err != nil {
		return err
	}
	if updated, err := r.reconcileActiveStatus(ctx, testrun, jobManager); updated || err != nil {
		return err
	}

	return nil
}

// reconcileActiveStatus will set the active status properly based on active suite runners.
func (r *TestRunReconciler) reconcileActiveStatus(ctx context.Context, testrun *etosv1alpha1.TestRun, jobManager jobs.Job) (bool, error) {
	logger := logf.FromContext(ctx)

	conditions := testrun.Status.Conditions
	if meta.FindStatusCondition(conditions, status.StatusActive) == nil {
		meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{
			Type:    status.StatusActive,
			Status:  metav1.ConditionFalse,
			Reason:  status.ReasonPending,
			Message: "Reconciliation started",
		})
		testrun.Status.Verdict = string(jobs.StatusNone)
		logger.Info("Setting initial status on testrun")
		return true, r.Status().Update(ctx, testrun)
	}

	condition := metav1.ConditionUnknown
	message := "Unknown status"
	reason := status.ReasonPending

	if isStatusReason(conditions, status.StatusEnvironment, status.ReasonFailed) {
		environment := meta.FindStatusCondition(conditions, status.StatusEnvironment)
		if environment != nil {
			condition = metav1.ConditionFalse
			message = environment.Message
			reason = status.ReasonFailed
		}
	}
	if isStatusReason(conditions, status.StatusSuiteRunner, status.ReasonFailed) {
		suiteRunner := meta.FindStatusCondition(conditions, status.StatusSuiteRunner)
		if suiteRunner != nil {
			condition = metav1.ConditionFalse
			message = suiteRunner.Message
			reason = status.ReasonFailed
		}
	}
	if isStatusReason(conditions, status.StatusSuiteRunner, status.ReasonCompleted) {
		condition = metav1.ConditionFalse
		reason = status.ReasonCompleted
		message = "Suite runners finished successfully"
	}
	if isStatusReason(conditions, status.StatusSuiteRunner, status.ReasonActive) {
		condition = metav1.ConditionTrue
		reason = status.ReasonActive
		message = "Waiting for suite runners to finish"
	}

	if condition != metav1.ConditionUnknown {
		logger.Info("Setting Active status on testrun", "message", message, "reason", reason, "condition", condition)
		if meta.SetStatusCondition(&testrun.Status.Conditions,
			metav1.Condition{
				Type:    status.StatusActive,
				Status:  condition,
				Reason:  reason,
				Message: message,
			}) {
			if condition == metav1.ConditionFalse {
				now := metav1.Now()
				testrun.Status.CompletionTime = &now
				return true, errors.Join(r.Status().Update(ctx, testrun), jobManager.Delete(ctx), r.deleteEnvironmentRequests(ctx, testrun))
			}
			return true, r.Status().Update(ctx, testrun)
		}
	}
	return false, nil
}

// reconcileSuiteRunner will check the status of suite runners, create new ones if necessary.
func (r *TestRunReconciler) reconcileSuiteRunner(ctx context.Context, testrun *etosv1alpha1.TestRun, jobManager jobs.Job, jobStatus jobs.Status) (bool, error) {
	logger := logf.FromContext(ctx)
	if meta.FindStatusCondition(testrun.Status.Conditions, status.StatusSuiteRunner) == nil {
		meta.SetStatusCondition(&testrun.Status.Conditions,
			metav1.Condition{
				Type:    status.StatusSuiteRunner,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonPending,
				Message: "Reconciliation started",
			})
		return true, r.Status().Update(ctx, testrun)
	}
	if isStatusReason(testrun.Status.Conditions, status.StatusSuiteRunner, status.ReasonFailed) {
		logger.Info("SuiteRunner has failed, reconciliation canceled")
		return false, errors.Join(jobManager.Delete(ctx), r.deleteEnvironmentRequests(ctx, testrun))
	}
	if !isStatusReason(testrun.Status.Conditions, status.StatusEnvironment, status.ReasonCompleted) {
		logger.Info("Environment is not ready yet")
		return false, nil
	}

	conditions := &testrun.Status.Conditions
	switch jobStatus {
	case jobs.StatusFailed:
		result := jobManager.Result(ctx, testrun.Name)
		if result.Verdict == "" {
			result.Verdict = jobs.VerdictNone
		}
		testrun.Status.Verdict = string(result.Verdict)
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusSuiteRunner,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonFailed,
				Message: result.Description,
			}) {
			return true, r.Status().Update(ctx, testrun)
		}
	case jobs.StatusSuccessful:
		result := jobManager.Result(ctx, testrun.Name)
		if result.Verdict == "" {
			result.Verdict = jobs.VerdictNone
		}
		testrun.Status.Verdict = string(result.Verdict)

		var condition metav1.Condition
		if result.Conclusion == jobs.ConclusionFailed {
			condition = metav1.Condition{
				Type:    status.StatusSuiteRunner,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonFailed,
				Message: result.Description,
			}
		} else {
			condition = metav1.Condition{
				Type:    status.StatusSuiteRunner,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonCompleted,
				Message: result.Description,
			}
		}
		if meta.SetStatusCondition(conditions, condition) {
			return true, r.Status().Update(ctx, testrun)
		}
	case jobs.StatusActive:
		testrun.Status.Verdict = string(jobs.VerdictNone)
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusSuiteRunner,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonActive,
				Message: "Job is running",
			}) {
			return true, r.Status().Update(ctx, testrun)
		}
	default:
		if !testrun.GetDeletionTimestamp().IsZero() {
			return false, nil
		}
		if err := jobManager.Create(ctx, testrun, r.suiteRunnerJob); err != nil {
			// When we create a job the job gets a unique name. If there's an error for that unique name the error
			// message in Condition.Message is also unique meaning we will update the StatusCondition every time,
			// causing a nasty reconciliation loop (when the testrun gets updated a new reconciliation starts).
			// We mitigate this by checking that StatusReason is not already Failed.
			if !isStatusReason(*conditions, status.StatusSuiteRunner, status.ReasonFailed) && meta.SetStatusCondition(conditions,
				metav1.Condition{
					Type:    status.StatusSuiteRunner,
					Status:  metav1.ConditionFalse,
					Reason:  status.ReasonFailed,
					Message: err.Error(),
				}) {
				return true, r.Status().Update(ctx, testrun)
			}
			return false, err
		}
	}
	return false, nil
}

// reconcileEnvironmentRequest will check the status of environment requests, create new ones if necessary.
func (r *TestRunReconciler) reconcileEnvironmentRequest(ctx context.Context, cluster *etosv1alpha1.Cluster, testrun *etosv1alpha1.TestRun) (bool, error) {
	logger := logf.FromContext(ctx)
	if meta.FindStatusCondition(testrun.Status.Conditions, status.StatusEnvironment) == nil {
		meta.SetStatusCondition(&testrun.Status.Conditions,
			metav1.Condition{
				Type:    status.StatusEnvironment,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonPending,
				Message: "Reconciliation started",
			})
		return true, r.Status().Update(ctx, testrun)
	}
	if isStatusReason(testrun.Status.Conditions, status.StatusEnvironment, status.ReasonFailed) {
		logger.Info("Environment provisioning failed, reconciliation canceled")
		return false, r.deleteEnvironmentRequests(ctx, testrun)
	}

	var environmentRequestList etosv1alpha1.EnvironmentRequestList
	if err := r.List(ctx, &environmentRequestList, client.InNamespace(testrun.Namespace), client.MatchingFields{TestRunOwnerKey: testrun.Name}); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, err
		}
	}
	testrun.Status.EnvironmentRequests = nil
	for _, request := range environmentRequestList.Items {
		reqRef, err := ref.GetReference(r.Scheme, &request)
		if err != nil {
			logger.Error(err, "failed to make reference to active environment request", "environmentrequest", request)
			continue
		}
		testrun.Status.EnvironmentRequests = append(testrun.Status.EnvironmentRequests, *reqRef)
	}

	for _, suite := range testrun.Spec.Suites {
		found := false
		for _, request := range environmentRequestList.Items {
			if request.Spec.Name == suite.Name {
				found = true
			}
		}
		if !found {
			request := r.environmentRequest(ctx, suite.Name, cluster, testrun, suite.Tests, suite.Dataset)
			if err := ctrl.SetControllerReference(testrun, request, r.Scheme); err != nil {
				return true, err
			}
			logger.Info("Creating a new environment request", "request", request.Name)
			if err := r.Create(ctx, request); err != nil {
				return true, err
			}
		}
	}

	allCompleted := len(environmentRequestList.Items) == len(testrun.Spec.Suites)
	for _, environmentRequest := range environmentRequestList.Items {
		condition := meta.FindStatusCondition(environmentRequest.Status.Conditions, status.StatusReady)
		if condition != nil && condition.Status == metav1.ConditionFalse && condition.Reason == status.ReasonFailed {
			if meta.SetStatusCondition(&testrun.Status.Conditions,
				metav1.Condition{
					Type:    status.StatusEnvironment,
					Status:  metav1.ConditionFalse,
					Reason:  status.ReasonFailed,
					Message: condition.Message,
				}) {
				return true, r.Status().Update(ctx, testrun)
			}
			if environmentRequest.ObjectMeta.DeletionTimestamp.IsZero() {
				if err := r.Delete(ctx, &environmentRequest); err != nil {
					logger.Error(err, "failed to delete environment", "environmentRequest", environmentRequest)
				}
			}
			return false, nil
		} else if condition == nil || condition.Status != metav1.ConditionTrue {
			allCompleted = false
		}
	}
	if allCompleted {
		if meta.SetStatusCondition(&testrun.Status.Conditions,
			metav1.Condition{
				Type:    status.StatusEnvironment,
				Status:  metav1.ConditionTrue,
				Reason:  status.ReasonCompleted,
				Message: "All environments provisioned",
			}) {
			return true, r.Status().Update(ctx, testrun)
		}
	}
	return false, nil
}

// deleteEnvironmentRequests will delete all environment requests that are a part of a testrun.
func (r *TestRunReconciler) deleteEnvironmentRequests(ctx context.Context, testrun *etosv1alpha1.TestRun) error {
	var environmentRequestList etosv1alpha1.EnvironmentRequestList
	if err := r.List(ctx, &environmentRequestList, client.InNamespace(testrun.Namespace), client.MatchingFields{TestRunOwnerKey: testrun.Name}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	for _, environmentRequest := range environmentRequestList.Items {
		if environmentRequest.ObjectMeta.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, &environmentRequest, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				if !apierrors.IsNotFound(err) {
					return err
				}
			}
		}
	}
	return nil
}

// checkEnvironment will check the status of the environment used for test.
func (r *TestRunReconciler) checkEnvironment(ctx context.Context, testrun *etosv1alpha1.TestRun) (bool, error) {
	logger := logf.FromContext(ctx)
	var environments etosv1alpha1.EnvironmentList
	if err := r.List(
		ctx,
		&environments,
		client.InNamespace(testrun.Namespace),
		client.MatchingLabels{"etos.eiffel-community.github.io/id": testrun.Spec.ID},
	); err != nil {
		logger.Error(err, "Error listing environments for testrun", "testrun", testrun)
		return false, err
	}
	if len(environments.Items) >= len(testrun.Spec.Suites) {
		if meta.SetStatusCondition(&testrun.Status.Conditions,
			metav1.Condition{
				Type:    status.StatusEnvironment,
				Status:  metav1.ConditionTrue,
				Reason:  status.ReasonCompleted,
				Message: "Environment ready",
			}) {
			return true, r.Status().Update(ctx, testrun)
		}
	}
	return false, nil
}

// environmentRequest is the definition for an environment request.
func (r TestRunReconciler) environmentRequest(ctx context.Context, name string, cluster *etosv1alpha1.Cluster, testrun *etosv1alpha1.TestRun, tests []etosv1alpha1.Test, dataset *apiextensionsv1.JSON) *etosv1alpha1.EnvironmentRequest {
	logger := logf.FromContext(ctx)
	eventRepository := cluster.Spec.EventRepository.Host
	if cluster.Spec.ETOS.Config.ETOSEventRepositoryURL != "" {
		logger.Info("Cluster configured with an event repository URL, using that instead")
		eventRepository = cluster.Spec.ETOS.Config.ETOSEventRepositoryURL
	}
	if eventRepository == "" {
		// TODO: We must fix a global config to store these default values.
		eventRepository = fmt.Sprintf("http://%s-graphql:%d/graphql", cluster.Name, 5000)
	}
	logger.Info("Event repository configured", "url", eventRepository)

	eiffelMessageBus := cluster.Spec.MessageBus.EiffelMessageBus
	if cluster.Spec.MessageBus.EiffelMessageBus.Deploy {
		logger.Info("Cluster deployed with an Eiffel messagebus, using that instead")
		eiffelMessageBus.Host = fmt.Sprintf("%s-%s", cluster.Name, "rabbitmq")
	}
	logger.Info("Eiffel messagebus configured", "url", eiffelMessageBus)

	etosMessageBus := cluster.Spec.MessageBus.ETOSMessageBus
	if cluster.Spec.MessageBus.ETOSMessageBus.Deploy {
		logger.Info("Cluster deployed with an ETOS messagebus, using that instead")
		etosMessageBus.Host = fmt.Sprintf("%s-%s", cluster.Name, "messagebus")
	}
	logger.Info("ETOS messagebus configured", "url", etosMessageBus)

	databaseHost := cluster.Spec.Database.Etcd.Host
	if databaseHost == "" {
		databaseHost = "etcd-client"
	}
	if cluster.Spec.Database.Deploy {
		logger.Info("Cluster deployed with an ETOS database, using that instead")
		databaseHost = fmt.Sprintf("%s-etcd-client", cluster.Name)
	}
	databasePort := cluster.Spec.Database.Etcd.Port
	if databasePort == "" {
		logger.Info("Database port not configured, using default")
		databasePort = "2379"
	}
	logger.Info("ETOS database configured", "host", databaseHost, "port", databasePort)

	annotations := make(map[string]string)
	traceparent, ok := testrun.Annotations["etos.eiffel-community.github.io/traceparent"]
	if ok {
		annotations["etos.eiffel-community.github.io/traceparent"] = traceparent
	}

	return &etosv1alpha1.EnvironmentRequest{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"etos.eiffel-community.github.io/id":      testrun.Spec.ID,
				"etos.eiffel-community.github.io/cluster": testrun.Spec.Cluster,
				"app.kubernetes.io/name":                  "suite-runner",
				"app.kubernetes.io/part-of":               "etos",
			},
			Annotations:  annotations,
			GenerateName: fmt.Sprintf("%s-", testrun.Name),
			Namespace:    testrun.Namespace,
		},
		Spec: etosv1alpha1.EnvironmentRequestSpec{
			ID:            string(uuid.NewUUID()),
			Name:          name,
			Identifier:    testrun.Spec.ID,
			Artifact:      testrun.Spec.Artifact,
			Identity:      testrun.Spec.Identity,
			MinimumAmount: 1,
			MaximumAmount: len(tests),
			Dataset:       dataset,
			Providers: etosv1alpha1.EnvironmentProviders{
				IUT: etosv1alpha1.IutProvider{
					ID: testrun.Spec.Providers.IUT,
				},
				ExecutionSpace: etosv1alpha1.ExecutionSpaceProvider{
					ID:         testrun.Spec.Providers.ExecutionSpace,
					TestRunner: testrun.Spec.TestRunner.Version,
				},
				LogArea: etosv1alpha1.LogAreaProvider{
					ID: testrun.Spec.Providers.LogArea,
				},
			},
			Splitter: etosv1alpha1.Splitter{
				Tests: tests,
			},
			Image:              testrun.Spec.EnvironmentProvider.Image,
			ServiceAccountName: fmt.Sprintf("%s-provider", testrun.Spec.Cluster),
			Config: etosv1alpha1.EnvironmentProviderJobConfig{
				EiffelMessageBus:                    eiffelMessageBus,
				EtosMessageBus:                      etosMessageBus,
				EtosApi:                             cluster.Spec.ETOS.Config.ETOSApiURL,
				EncryptionKey:                       cluster.Spec.ETOS.Config.EncryptionKey,
				RoutingKeyTag:                       cluster.Spec.ETOS.Config.RoutingKeyTag,
				GraphQlServer:                       eventRepository,
				EtcdHost:                            databaseHost,
				EtcdPort:                            databasePort,
				WaitForTimeout:                      cluster.Spec.ETOS.Config.EnvironmentTimeout,
				EnvironmentProviderEventDataTimeout: cluster.Spec.ETOS.Config.EventDataTimeout,
				EnvironmentProviderServiceAccount:   fmt.Sprintf("%s-provider", cluster.Name),
				EnvironmentProviderTestSuiteTimeout: cluster.Spec.ETOS.Config.TestSuiteTimeout,
				TestRunnerVersion:                   testrun.Spec.TestRunner.Version,
			},
		},
	}
}

// suiteRunnerJob is the job definition for an etos suite runner.
func (r TestRunReconciler) suiteRunnerJob(ctx context.Context, obj client.Object) (*batchv1.Job, error) {
	grace := int64(30)
	backoff := int32(0)
	testrun, ok := obj.(*etosv1alpha1.TestRun)
	if !ok {
		return nil, errors.New("object received from job manager is not a TestRun")
	}
	traceparent, ok := testrun.Annotations["etos.eiffel-community.github.io/traceparent"]
	if !ok {
		traceparent = ""
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"etos.eiffel-community.github.io/id": testrun.Spec.ID,
				"app.kubernetes.io/name":             "suite-runner",
				"app.kubernetes.io/part-of":          "etos",
			},
			Annotations: make(map[string]string),
			Name:        testrun.Name,
			Namespace:   testrun.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: testrun.Name,
					Labels: map[string]string{
						"etos.eiffel-community.github.io/id": testrun.Spec.ID,
						"app.kubernetes.io/name":             "suite-runner",
						"app.kubernetes.io/part-of":          "etos",
					},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "kubexit",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "graveyard",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
							},
						},
					},
					TerminationGracePeriodSeconds: &grace,
					ServiceAccountName:            fmt.Sprintf("%s-provider", testrun.Spec.Cluster),
					RestartPolicy:                 "Never",
					InitContainers: []corev1.Container{
						{
							Name:    "kubexit",
							Image:   "karlkfi/kubexit:latest",
							Command: []string{"cp", "/bin/kubexit", "/kubexit/kubexit"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("64Mi"),
									corev1.ResourceCPU:    resource.MustParse("50m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("32Mi"),
									corev1.ResourceCPU:    resource.MustParse("25m"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kubexit",
									MountPath: "/kubexit",
								},
							},
						},
						{
							Name:            "create-queue",
							Image:           testrun.Spec.LogListener.Image.Image,
							ImagePullPolicy: testrun.Spec.LogListener.ImagePullPolicy,
							Command:         []string{"python", "-u", "-m", "create_queue"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("50m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("25m"),
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-etos-suite-runner-cfg", testrun.Spec.Cluster),
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "IDENTIFIER",
									Value: testrun.Spec.ID,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            testrun.Name,
							Image:           testrun.Spec.SuiteRunner.Image.Image,
							ImagePullPolicy: testrun.Spec.SuiteRunner.ImagePullPolicy,
							Command:         []string{"/kubexit/kubexit"},
							Args:            []string{"python", "-u", "-m", "etos_suite_runner"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("200m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-etos-suite-runner-cfg", testrun.Spec.Cluster),
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "ARTIFACT",
									Value: testrun.Spec.Artifact,
								},
								{
									Name:  "IDENTITY",
									Value: testrun.Spec.Identity,
								},
								{
									Name:  "ETOS_ROUTING_KEY_TAG",
									Value: fmt.Sprintf("%s-%s", testrun.Namespace, testrun.Spec.Cluster),
								},
								{
									Name:  "TESTRUN",
									Value: testrun.Name,
								},
								{
									Name:  "IDENTIFIER",
									Value: testrun.Spec.ID,
								},
								{
									Name:  "OTEL_CONTEXT",
									Value: traceparent,
								},
								{
									Name:  "KUBEXIT_NAME",
									Value: "esr",
								},
								{
									Name:  "KUBEXIT_GRAVEYARD",
									Value: "/graveyard",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "graveyard",
									MountPath: "/graveyard",
								},
								{
									Name:      "kubexit",
									MountPath: "/kubexit",
								},
							},
						},
						{
							Name:            "etos-log-listener",
							Image:           testrun.Spec.LogListener.Image.Image,
							ImagePullPolicy: testrun.Spec.LogListener.ImagePullPolicy,
							Command:         []string{"/kubexit/kubexit"},
							Args:            []string{"python", "-u", "-m", "log_listener"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("50m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("25m"),
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-etos-suite-runner-cfg", testrun.Spec.Cluster),
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "IDENTIFIER",
									Value: testrun.Spec.ID,
								},
								{
									Name:  "KUBEXIT_NAME",
									Value: "log_listener",
								},
								{
									Name:  "KUBEXIT_GRAVE_PERIOD",
									Value: "400s",
								},
								{
									Name:  "KUBEXIT_GRAVEYARD",
									Value: "/graveyard",
								},
								{
									Name:  "KUBEXIT_DEATH_DEPS",
									Value: "esr",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "graveyard",
									MountPath: "/graveyard",
								},
								{
									Name:      "kubexit",
									MountPath: "/kubexit",
								},
							},
						},
					},
				},
			},
		},
	}
	return job, ctrl.SetControllerReference(testrun, job, r.Scheme)
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up real clock, since we are not in a test
	if r.Clock == nil {
		r.Clock = realClock{}
	}

	// Register indexes for faster lookups
	if err := r.registerOwnerIndexForJob(mgr); err != nil {
		return err
	}
	if err := r.registerOwnerIndexForEnvironmentRequest(mgr); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.TestRun{}, iutProvider, func(rawObj client.Object) []string {
		testrun := rawObj.(*etosv1alpha1.TestRun)
		return []string{testrun.Spec.Providers.IUT}
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.TestRun{}, logAreaProvider, func(rawObj client.Object) []string {
		testrun := rawObj.(*etosv1alpha1.TestRun)
		return []string{testrun.Spec.Providers.LogArea}
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.TestRun{}, executionSpaceProvider, func(rawObj client.Object) []string {
		testrun := rawObj.(*etosv1alpha1.TestRun)
		return []string{testrun.Spec.Providers.ExecutionSpace}
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha1.TestRun{}).
		Named("testrun").
		Owns(&batchv1.Job{}).
		Owns(&etosv1alpha1.EnvironmentRequest{}).
		Watches(
			&etosv1alpha1.Provider{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findTestrunsForIUTProvider),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&etosv1alpha1.Provider{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findTestrunsForExecutionSpaceProvider),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&etosv1alpha1.Provider{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findTestrunsForLogAreaProvider),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

// registerOwnerIndexForJob will set an index of the suite runner jobs that a testrun owns.
func (r *TestRunReconciler) registerOwnerIndexForJob(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, TestRunOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGroupVersionString || owner.Kind != testRunKind {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// registerOwnerIndexForEnvironmentRequest will set an index of the environment requests that a testrun owns.
func (r *TestRunReconciler) registerOwnerIndexForEnvironmentRequest(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.EnvironmentRequest{}, TestRunOwnerKey, func(rawObj client.Object) []string {
		environmentRequest := rawObj.(*etosv1alpha1.EnvironmentRequest)
		owner := metav1.GetControllerOf(environmentRequest)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGroupVersionString || owner.Kind != testRunKind {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// findTestrunsForIUTProvider will return reconciliation requests for each Provider object that a testrun has stored
// in its spec as IUT. This will cause reconciliations whenever a Provider gets updated, created, deleted etc.
func (r *TestRunReconciler) findTestrunsForIUTProvider(ctx context.Context, provider client.Object) []reconcile.Request {
	return r.findTestrunsForProvider(ctx, iutProvider, provider)
}

// findTestrunsForIUTProvider will return reconciliation requests for each Provider object that a testrun has stored
// in its spec as execution space. This will cause reconciliations whenever a Provider gets updated, created, deleted etc.
func (r *TestRunReconciler) findTestrunsForExecutionSpaceProvider(ctx context.Context, provider client.Object) []reconcile.Request {
	return r.findTestrunsForProvider(ctx, executionSpaceProvider, provider)
}

// findTestrunsForIUTProvider will return reconciliation requests for each Provider object that a testrun has stored
// in its spec as log area. This will cause reconciliations whenever a Provider gets updated, created, deleted etc.
func (r *TestRunReconciler) findTestrunsForLogAreaProvider(ctx context.Context, provider client.Object) []reconcile.Request {
	return r.findTestrunsForProvider(ctx, logAreaProvider, provider)
}

// findTestrunsForProvider will find testruns for a providerName.
func (r *TestRunReconciler) findTestrunsForProvider(ctx context.Context, providerName string, provider client.Object) []reconcile.Request {
	testrunList := &etosv1alpha1.TestRunList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(providerName, provider.GetName()),
		Namespace:     provider.GetNamespace(),
	}
	err := r.List(ctx, testrunList, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(testrunList.Items))
	for i, item := range testrunList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}
