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
	"encoding/json"
	"errors"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ref "k8s.io/client-go/tools/reference"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

var (
	OwnerKey               = ".metadata.controller"
	APIGVStr               = etosv1alpha1.GroupVersion.String()
	iutProvider            = ".spec.providers.iut"
	executionSpaceProvider = ".spec.providers.executionSpace"
	logAreaProvider        = ".spec.providers.logarea"
)

// TestRunReconciler reconciles a TestRun object
type TestRunReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Clock  clock.WithTicker
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments,verbs=get;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments/status,verbs=get
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers,verbs=get;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers/status,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile

func (r *TestRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	testrun := &etosv1alpha1.TestRun{}
	if err := r.Get(ctx, req.NamespacedName, testrun); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Testrun not found, exiting", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Testrun not found", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}
	if testrun.Status.CompletionTime != nil {
		return ctrl.Result{}, nil
	}

	if err := r.reconcile(ctx, testrun); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return r.update(ctx, testrun, metav1.ConditionFalse, err.Error())
	}

	return ctrl.Result{}, nil
}

type jobs struct {
	activeJobs     []*batchv1.Job
	successfulJobs []*batchv1.Job
	failedJobs     []*batchv1.Job
}

func (j jobs) active() bool {
	return len(j.successfulJobs) == 0 && len(j.failedJobs) == 0 && len(j.activeJobs) > 0
}

func (j jobs) failed() bool {
	return len(j.failedJobs) > 0 && len(j.activeJobs) == 0
}

func (j jobs) successful() bool {
	return len(j.successfulJobs) > 0 && len(j.activeJobs) == 0
}

func (j jobs) empty() bool {
	return len(j.successfulJobs) == 0 && len(j.failedJobs) == 0 && len(j.activeJobs) == 0
}

func (r *TestRunReconciler) reconcile(ctx context.Context, testrun *etosv1alpha1.TestRun) error {
	logger := log.FromContext(ctx)

	// Set initial statuses if not set.
	if meta.FindStatusCondition(testrun.Status.Conditions, StatusActive) == nil {
		meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Status: metav1.ConditionFalse, Type: StatusActive, Message: "Reconciliation started", Reason: "Pending"})
		return r.Status().Update(ctx, testrun)
	}
	if meta.FindStatusCondition(testrun.Status.Conditions, StatusSuiteRunner) == nil {
		meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Status: metav1.ConditionFalse, Type: StatusSuiteRunner, Message: "Reconciliation started", Reason: "Pending"})
		return r.Status().Update(ctx, testrun)
	}
	if meta.FindStatusCondition(testrun.Status.Conditions, StatusEnvironment) == nil {
		meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Status: metav1.ConditionFalse, Type: StatusEnvironment, Message: "Reconciliation started", Reason: "Pending"})
		return r.Status().Update(ctx, testrun)
	}

	// Get active, finished and failed sutie runners.
	suiteRunners, err := r.suiteRunnerStatus(ctx, testrun)
	if err != nil {
		return err
	}

	// Add active suite runners to testrun status
	testrun.Status.SuiteRunners = nil
	for _, activeSuiteRunner := range suiteRunners.activeJobs {
		jobRef, err := ref.GetReference(r.Scheme, activeSuiteRunner)
		if err != nil {
			logger.Error(err, "failed to make reference to active suite runner", "suiterunner", activeSuiteRunner)
			continue
		}
		testrun.Status.SuiteRunners = append(testrun.Status.SuiteRunners, *jobRef)
	}
	if err := r.Status().Update(ctx, testrun); err != nil {
		return err
	}
	logger.V(1).Info("suite runner count", "active", len(suiteRunners.activeJobs), "successful", len(suiteRunners.successfulJobs), "failed", len(suiteRunners.failedJobs))

	// Check providers availability
	if err := r.checkProviders(ctx, testrun); err != nil {
		return err
	}

	// Check environment
	if err := r.checkEnvironment(ctx, testrun); err != nil {
		return err
	}

	// Reconcile suite runners
	if err := r.reconcileSuiteRunner(ctx, suiteRunners, testrun); err != nil {
		return err
	}

	// Set testrun status
	if suiteRunners.active() {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionTrue, Reason: "Active", Message: "Waiting for suite runners to finish"}) {
			return r.Status().Update(ctx, testrun)
		}
	}
	if suiteRunners.failed() {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionFalse, Reason: "Failed", Message: "Suite runners failed to finish"}) {
			testrunCondition := meta.FindStatusCondition(testrun.Status.Conditions, StatusActive)
			testrun.Status.CompletionTime = &testrunCondition.LastTransitionTime
			return r.Status().Update(ctx, testrun)
		}
	}
	if suiteRunners.successful() {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionFalse, Reason: "Successful", Message: "Suite runners finished successfully"}) {
			testrunCondition := meta.FindStatusCondition(testrun.Status.Conditions, StatusActive)
			testrun.Status.CompletionTime = &testrunCondition.LastTransitionTime
			return r.Status().Update(ctx, testrun)
		}
	}

	return nil
}

// reconcileSuiteRunner will check the status of suite runners, create new ones if necessary.
func (r *TestRunReconciler) reconcileSuiteRunner(ctx context.Context, suiteRunners *jobs, testrun *etosv1alpha1.TestRun) error {
	logger := log.FromContext(ctx)
	// Suite runners failed, setting status.
	if suiteRunners.failed() {
		suiteRunner := suiteRunners.failedJobs[0] // TODO
		message, err := r.terminationLog(ctx, suiteRunner)
		if err != nil {
			message = err.Error()
		}
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionFalse, Reason: "Failed", Message: message}) {
			logger.Info("Setting suiterunner (failed) false")
			return r.Status().Update(ctx, testrun)
		}
	}
	// Suite runners successful, setting status.
	if suiteRunners.successful() {
		suiteRunner := suiteRunners.successfulJobs[0] // TODO
		message, err := r.terminationLog(ctx, suiteRunner)
		if err != nil {
			message = err.Error()
		}
		if message != "" {
			if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionFalse, Reason: "Failed", Message: message}) {
				logger.Info("Setting suiterunner (failed) false")
				return r.Status().Update(ctx, testrun)
			}
		}
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionFalse, Reason: "Done", Message: "Suite runner finished"}) {
			logger.Info("Setting suiterunner (success) false")
			for _, suiteRunner := range suiteRunners.successfulJobs {
				if err := r.Delete(ctx, suiteRunner, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
					if !apierrors.IsNotFound(err) {
						return err
					}
				}
			}
			return r.Status().Update(ctx, testrun)
		}
	}
	// Suite runners active, setting status
	if suiteRunners.active() {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionTrue, Reason: "Running", Message: "Suite runner is running"}) {
			logger.Info("Setting suiterunner (active) true")
			return r.Status().Update(ctx, testrun)
		}
	}
	// No suite runners, create suite runner
	if suiteRunners.empty() {
		tercc, err := json.Marshal(testrun.Spec.Suites)
		if err != nil {
			return err
		}
		suiteRunner := r.suiteRunnerJob(tercc, testrun)
		if err := ctrl.SetControllerReference(testrun, suiteRunner, r.Scheme); err != nil {
			return err
		}
		if err := r.Create(ctx, suiteRunner); err != nil {
			return err
		}
	}
	return nil
}

// suiteRunnerStatus creates a job struct with suite runner jobs that are active, failed or successful.
func (r *TestRunReconciler) suiteRunnerStatus(ctx context.Context, testrun *etosv1alpha1.TestRun) (*jobs, error) {
	var activeSuiteRunners []*batchv1.Job
	var successfulSuiteRunners []*batchv1.Job
	var failedSuiteRunners []*batchv1.Job
	var suiteRunners batchv1.JobList
	if err := r.List(ctx, &suiteRunners, client.InNamespace(testrun.Namespace), client.MatchingFields{OwnerKey: testrun.Name}); err != nil {
		return &jobs{}, err
	}

	for i, suiteRunner := range suiteRunners.Items {
		_, finishedType := r.isSuiteRunnerFinished(suiteRunner)
		switch finishedType {
		case "": // Ongoing
			activeSuiteRunners = append(activeSuiteRunners, &suiteRunners.Items[i])
		case batchv1.JobFailed:
			failedSuiteRunners = append(failedSuiteRunners, &suiteRunners.Items[i])
		case batchv1.JobComplete:
			successfulSuiteRunners = append(successfulSuiteRunners, &suiteRunners.Items[i])
		}
	}
	return &jobs{activeJobs: activeSuiteRunners, failedJobs: failedSuiteRunners, successfulJobs: successfulSuiteRunners}, nil
}

// update will set the status condition and update the status of the ETOS testrun.
// if the update fails due to conflict the reconciliation will requeue.
func (r *TestRunReconciler) update(ctx context.Context, testrun *etosv1alpha1.TestRun, status metav1.ConditionStatus, message string) (ctrl.Result, error) {
	// TODO: Verify this function.
	if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: status, Reason: "Active", Message: message}) {
		if err := r.Status().Update(ctx, testrun); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// checkProviders checks if all providers for this environment are available.
// TODO: this function should not be necessary when environmentrequests are supported
func (r *TestRunReconciler) checkProviders(ctx context.Context, testrun *etosv1alpha1.TestRun) error {
	err := r.checkProvider(ctx, testrun.Spec.Providers.IUT, testrun.Namespace, &etosv1alpha1.Provider{})
	if err != nil {
		return err
	}
	err = r.checkProvider(ctx, testrun.Spec.Providers.ExecutionSpace, testrun.Namespace, &etosv1alpha1.Provider{})
	if err != nil {
		return err
	}
	err = r.checkProvider(ctx, testrun.Spec.Providers.LogArea, testrun.Namespace, &etosv1alpha1.Provider{})
	if err != nil {
		return err
	}
	return nil
}

// checkEnvironment will check the status of the environment used for test.
func (r *TestRunReconciler) checkEnvironment(ctx context.Context, testrun *etosv1alpha1.TestRun) error {
	logger := log.FromContext(ctx)
	logger.Info("Checking environment")
	var environments etosv1alpha1.EnvironmentList
	if err := r.List(ctx, &environments, client.InNamespace(testrun.Namespace), client.MatchingFields{OwnerKey: testrun.Name}); err != nil {
		return err
	}
	logger.Info("Listed environments", "environments", len(environments.Items))
	// TODO: this only checks one environment, not all of them if there are many
	if len(environments.Items) > 0 {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusEnvironment, Status: metav1.ConditionTrue, Reason: "Ready", Message: "Environment ready"}) {
			logger.Info("Set condition")
			return r.Status().Update(ctx, testrun)
		}
	}
	return nil
}

// checkProvider checks if the provider condition 'Available' is set to True.
func (r *TestRunReconciler) checkProvider(ctx context.Context, name string, namespace string, provider *etosv1alpha1.Provider) error {
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, provider)
	if err != nil {
		return err
	}
	if meta.IsStatusConditionPresentAndEqual(provider.Status.Conditions, StatusAvailable, metav1.ConditionTrue) {
		return nil
	}
	return fmt.Errorf("Provider '%s' does not have a status field", name)
}

// terminationLog reads the termination-log part of the ESR pod and returns it.
func (r *TestRunReconciler) terminationLog(ctx context.Context, suiteRunner *batchv1.Job) (string, error) {
	logger := log.FromContext(ctx)
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(suiteRunner.Namespace), client.MatchingLabels{"job-name": suiteRunner.Name}); err != nil {
		logger.Error(err, "could not list suite runner pods")
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", errors.New("no pods found for suite runner job")
	}
	if len(pods.Items) > 1 {
		logger.Info("found more than 1 pod active. Will only check termination-log for the first one", "pod", pods.Items[0])
	}
	pod := pods.Items[0]

	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == suiteRunner.Name {
			if status.State.Terminated == nil {
				return "", errors.New("could not read termination log from suite runner pod")
			}
			return status.State.Terminated.Message, nil
		}
	}
	return "", errors.New("found no container status for suite runner pod")
}

// isSuiteRunnerFinished checks if a suite runner has status Complete or Failed.
func (r TestRunReconciler) isSuiteRunnerFinished(suiteRunner batchv1.Job) (bool, batchv1.JobConditionType) {
	if IsJobStatusConditionPresentAndEqual(suiteRunner.Status.Conditions, batchv1.JobComplete, corev1.ConditionTrue) {
		return true, batchv1.JobComplete
	}
	if IsJobStatusConditionPresentAndEqual(suiteRunner.Status.Conditions, batchv1.JobFailed, corev1.ConditionTrue) {
		return true, batchv1.JobFailed
	}
	return false, ""
}

// IsStatusConditionPresentAndEqual returns true when conditionType is present and equal to status.
func IsJobStatusConditionPresentAndEqual(conditions []batchv1.JobCondition, conditionType batchv1.JobConditionType, status corev1.ConditionStatus) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == status
		}
	}
	return false
}

// suiteRunnerJob is the job definition for an etos suite runner.
func (r TestRunReconciler) suiteRunnerJob(tercc []byte, testrun *etosv1alpha1.TestRun) *batchv1.Job {
	ttl := int32(300)
	grace := int64(30)
	backoff := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"id": testrun.Spec.ID,
			},
			Annotations: make(map[string]string),
			Name:        testrun.Name,
			Namespace:   testrun.Namespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			BackoffLimit:            &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: testrun.Name,
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
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: testrun.Spec.Cluster,
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-messagebus", testrun.Spec.Cluster),
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
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: testrun.Spec.Cluster,
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: testrun.Spec.Cluster,
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-rabbitmq", testrun.Spec.Cluster),
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-messagebus", testrun.Spec.Cluster),
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "TERCC",
									Value: string(tercc),
								},
								{
									Name:  "ARTIFACT",
									Value: testrun.Spec.Artifact,
								},
								{
									Name:  "IDENTITY",
									Value: testrun.Spec.Identity,
								},
								{
									Name:  "TESTRUN",
									Value: testrun.Name,
								},
								{
									Name:  "SUITE_SOURCE",
									Value: testrun.Spec.SuiteSource,
								},
								{
									Name:  "IDENTIFIER",
									Value: testrun.Spec.ID,
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
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: testrun.Spec.Cluster,
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: testrun.Spec.Cluster,
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-messagebus", testrun.Spec.Cluster),
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
	job.Labels["app"] = "suite-runner"

	return job
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register indexes for faster lookups
	if err := r.registerOwnerIndexForJob(mgr); err != nil {
		return err
	}
	if err := r.registerOwnerIndexForEnvironment(mgr); err != nil {
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
		Owns(&batchv1.Job{}).
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
		Watches(
			&etosv1alpha1.Environment{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findTestrunsForEnvironment),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

// registerOwnerIndexForJob will set an index of the suite runner jobs that a testrun owns.
func (r *TestRunReconciler) registerOwnerIndexForJob(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, OwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGVStr || owner.Kind != "TestRun" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// registerOwnerIndexForJob will set an index of the environments that a testrun owns.
func (r *TestRunReconciler) registerOwnerIndexForEnvironment(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.Environment{}, OwnerKey, func(rawObj client.Object) []string {
		environment := rawObj.(*etosv1alpha1.Environment)

		// TODO: Since we are setting controller to false when creating Environment in the environment provider
		// we need to find the TestRun owner in another way. This way is much more insecure, but since we are going
		// to create environments from the controllers later we wuill be able to fix this to be more in line with
		// how registerOwnerIndexForJob does it.
		refs := environment.GetOwnerReferences()
		for i := range refs {
			if refs[i].APIVersion == APIGVStr && refs[i].Kind == "TestRun" {
				return []string{refs[i].Name}
			}
		}
		return nil
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

// findTestrunsForEnvironment will return reconciliation requests for Environment objects with a specific testrun as
// owner.
func (r *TestRunReconciler) findTestrunsForEnvironment(ctx context.Context, environment client.Object) []reconcile.Request {
	// TODO: Since we are setting controller to false when creating Environment in the environment provider
	// we need to find the TestRun owner in another way. This way is much more insecure, but since we are going
	// to create environments from the controllers later we wuill be able to fix this to be more in line with
	// how registerOwnerIndexForJob does it.
	refs := environment.GetOwnerReferences()
	for i := range refs {
		if refs[i].APIVersion == APIGVStr && refs[i].Kind == "TestRun" {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      refs[i].Name,
						Namespace: environment.GetNamespace(),
					},
				},
			}
		}
	}
	return nil
}
