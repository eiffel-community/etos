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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ref "k8s.io/client-go/tools/reference"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
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
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments,verbs=get;watch;create;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments/status,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *TestRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.V(2).Info("Get testrun", "namespace", req.Namespace, "name", req.Name)
	testrun := &etosv1alpha1.TestRun{}
	if err := r.Get(ctx, req.NamespacedName, testrun); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Testrun not found, exiting", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Testrun not found", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	requeue, err := r.reconcile(ctx, testrun, req)
	if err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "error reconciling testrun")
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: requeue}, nil
}

// reconcile a testrun.
func (r *TestRunReconciler) reconcile(ctx context.Context, testrun *etosv1alpha1.TestRun, req ctrl.Request) (bool, error) {
	logger := log.FromContext(ctx)
	patch := client.MergeFrom(testrun.DeepCopy())

	// Check if testrun has completion time. Delete suite runner (if any) and exit.
	if testrun.Status.CompletionTime != nil {
		if err := r.cleanup(ctx, req.NamespacedName); err != nil {
			return true, err
		}
		return false, nil
	}

	// Check if testrun has finished. Update completion time and exit.
	testrunCondition := meta.FindStatusCondition(testrun.Status.Conditions, StatusActive)
	if testrunCondition != nil && testrunCondition.Status == metav1.ConditionFalse {
		if testrun.Status.CompletionTime == nil {
			logger.Info("Setting completion time")
			testrun.Status.CompletionTime = &testrunCondition.LastTransitionTime
			return false, r.Status().Patch(ctx, testrun, patch)
		}
		return false, nil
	}

	// Check if suite runner has finished. Set testrun active to false.
	suiteRunnerCondition := meta.FindStatusCondition(testrun.Status.Conditions, StatusSuiteRunner)
	if suiteRunnerCondition != nil && suiteRunnerCondition.Status == metav1.ConditionFalse {
		message := "Testrun finished execution successfully"
		if suiteRunnerCondition.Reason == "Failed" {
			message = "Testrun failed"
		}
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionFalse, Reason: "Done", Message: message}) {
			logger.Info("Setting active false")
			return false, r.Status().Patch(ctx, testrun, patch)
		}
		return false, nil
	}

	// Set start time on new testruns.
	if testrun.Status.StartTime == nil {
		logger.Info("Setting start time")
		now := metav1.NewTime(r.Clock.Now())
		testrun.Status.StartTime = &now
		return false, r.Status().Patch(ctx, testrun, patch)
	}

	// Set active to unknown on new testruns.
	if meta.FindStatusCondition(testrun.Status.Conditions, StatusActive) == nil {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"}) {
			logger.Info("Setting active unknown")
			return false, r.Status().Patch(ctx, testrun, patch)
		}
	}

	// Set active true if suite runner has started.
	if meta.IsStatusConditionPresentAndEqual(testrun.Status.Conditions, StatusSuiteRunner, metav1.ConditionTrue) {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionTrue, Reason: "Executing", Message: "Executing testrun"}) {
			logger.Info("Setting active true")
			return false, r.Status().Patch(ctx, testrun, patch)
		}
	}

	if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusEnvironment, Status: metav1.ConditionTrue, Reason: "OK", Message: "Environments exists"}) {
		logger.Info("Setting environment true")
		return false, r.Status().Patch(ctx, testrun, patch)
	}

	// Set Active condition to True whilst waiting for an environment.
	environmentStatus := meta.FindStatusCondition(testrun.Status.Conditions, StatusEnvironment)
	if environmentStatus == nil || environmentStatus.Status == metav1.ConditionFalse {
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionTrue, Reason: "Waiting", Message: "Waiting for environment providers to be available"}) {
			return true, r.Status().Patch(ctx, testrun, patch)
		}
		return true, nil
	}

	// Get or create a new suite runner.
	suiteRunner, err := r.getOrCreateSuiteRunner(ctx, testrun, req.NamespacedName)
	if err != nil {
		logger.Info("Setting suiterunner false")
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionFalse, Reason: "Failed", Message: fmt.Sprintf("Failed to create suite runner: %s", err.Error())}) {
			return false, r.Status().Patch(ctx, testrun, patch)
		}
		return false, nil
	}

	testrun.Status.SuiteRunners = nil
	jobRef, err := ref.GetReference(r.Scheme, suiteRunner)
	if err != nil {
		logger.Error(err, "could not get reference to an active suite runner", "suiteRunner", suiteRunner)
	} else {
		testrun.Status.SuiteRunners = append(testrun.Status.SuiteRunners, *jobRef)
	}

	// Check the status of the suite runner job.
	_, finishedType := r.isSuiteRunnerFinished(suiteRunner)
	switch finishedType {
	case "":
		// Active
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionTrue, Reason: "Running", Message: "Suite runner is running"}) {
			logger.Info("Setting suiterunner (active) true")
			return false, r.Status().Patch(ctx, testrun, patch)
		}
	case batchv1.JobFailed:
		// Failed
		message, err := r.terminationLog(ctx, suiteRunner)
		if err != nil {
			message = err.Error()
		}
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionFalse, Reason: "Failed", Message: message}) {
			logger.Info("Setting suiterunner (failed) false")
			return false, r.Status().Patch(ctx, testrun, patch)
		}
	case batchv1.JobComplete:
		// Success
		if meta.SetStatusCondition(&testrun.Status.Conditions, metav1.Condition{Type: StatusSuiteRunner, Status: metav1.ConditionFalse, Reason: "Done", Message: "Suite runner finished"}) {
			logger.Info("Setting suiterunner (success) false")
			return false, r.Status().Patch(ctx, testrun, patch)
		}
	}
	return false, nil
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
			return status.State.Terminated.Message, nil
		}
	}
	return "", errors.New("found no container status for suite runner pod")
}

func (r *TestRunReconciler) getOrCreateSuiteRunner(ctx context.Context, testrun *etosv1alpha1.TestRun, name types.NamespacedName) (*batchv1.Job, error) {
	suiteRunner := &batchv1.Job{}
	if err := r.Get(ctx, name, suiteRunner); err != nil {
		if apierrors.IsNotFound(err) {
			tercc, err := json.Marshal(testrun.Spec.Suites)
			if err != nil {
				return suiteRunner, err
			}
			suiteRunner = r.suiteRunnerJob(tercc, testrun)
			if err := ctrl.SetControllerReference(testrun, suiteRunner, r.Scheme); err != nil {
				return suiteRunner, err
			}
			if err := r.Create(ctx, suiteRunner); err != nil {
				return suiteRunner, err
			}
		} else {
			return suiteRunner, err
		}
	}
	return suiteRunner, nil
}

// isSuiteRunnerFinished checks if a suite runner has status Complete or Failed.
func (r TestRunReconciler) isSuiteRunnerFinished(suiteRunner *batchv1.Job) (bool, batchv1.JobConditionType) {
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
			Labels:      make(map[string]string),
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
					TerminationGracePeriodSeconds: &grace,
					ServiceAccountName:            fmt.Sprintf("%s-provider", testrun.Spec.Cluster),
					RestartPolicy:                 "Never",
					Containers: []corev1.Container{
						{
							Name:            testrun.Name,
							Image:           testrun.Spec.SuiteRunner.Image.Image,
							ImagePullPolicy: testrun.Spec.SuiteRunner.ImagePullPolicy,
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
									Name:  "IDENTIFIER",
									Value: testrun.Spec.ID,
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

// deleteSuiteRunner tries to delete a suite runner job if it exists.
func (r *TestRunReconciler) deleteSuiteRunner(ctx context.Context, name types.NamespacedName) error {
	suiteRunner := &batchv1.Job{}
	if err := r.Get(ctx, name, suiteRunner); err != nil {
		return err
	}
	if err := r.Delete(ctx, suiteRunner, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		return err
	}
	return nil
}

// cleanup cleans up finished suite runners.
func (r *TestRunReconciler) cleanup(ctx context.Context, name types.NamespacedName) error {
	if err := r.deleteSuiteRunner(ctx, name); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha1.TestRun{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
