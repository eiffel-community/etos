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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

const releaseFinalizer = "etos.eiffel-community.github.io/release"

// EnvironmentReconciler reconciles a Environment object
type EnvironmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments/finalizers,verbs=update
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers,verbs=get
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *EnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	environment := &etosv1alpha1.Environment{}
	err := r.Get(ctx, req.NamespacedName, environment)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// If the environment is considered 'Completed', it has been released. Check that the object is
	// being deleted and contains the finalizer and remove the finalizer.
	if environment.Status.CompletionTime != nil {
		if !environment.ObjectMeta.DeletionTimestamp.IsZero() {
			if controllerutil.ContainsFinalizer(environment, releaseFinalizer) {
				controllerutil.RemoveFinalizer(environment, releaseFinalizer)
				if err := r.Update(ctx, environment); err != nil {
					if apierrors.IsConflict(err) {
						return ctrl.Result{Requeue: true}, nil
					}
					return ctrl.Result{}, err
				}
			}
		}
		return ctrl.Result{}, nil
	}

	if err := r.reconcile(ctx, environment); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *EnvironmentReconciler) reconcile(ctx context.Context, environment *etosv1alpha1.Environment) error {
	logger := log.FromContext(ctx)

	// Set initial statuses if not set.
	if meta.FindStatusCondition(environment.Status.Conditions, StatusActive) == nil {
		meta.SetStatusCondition(&environment.Status.Conditions, metav1.Condition{Status: metav1.ConditionTrue, Type: StatusActive, Message: "Actively being used", Reason: "Active"})
		return r.Status().Update(ctx, environment)
	}
	if environment.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(environment, releaseFinalizer) {
			controllerutil.AddFinalizer(environment, releaseFinalizer)
			return r.Update(ctx, environment)
		}
	}

	// Get active, finished and failed environment releasers.
	releasers, err := jobStatus(ctx, r, environment.Namespace, environment.Name, EnvironmentOwnerKey)
	if err != nil {
		return err
	}

	environment.Status.EnvironmentReleasers = nil
	for _, activeReleaser := range releasers.activeJobs {
		jobRef, err := ref.GetReference(r.Scheme, activeReleaser)
		if err != nil {
			logger.Error(err, "failed to make reference to active environment releaser", "releaser", activeReleaser)
			continue
		}
		environment.Status.EnvironmentReleasers = append(environment.Status.EnvironmentReleasers, *jobRef)
	}
	if err := r.Status().Update(ctx, environment); err != nil {
		return err
	}
	logger.V(1).Info("environment releaser count", "active", len(releasers.activeJobs), "successful", len(releasers.successfulJobs), "failed", len(releasers.failedJobs))

	// TODO: Provider information does not exist in a deterministic way in the Environment resource
	// so either we need to find the EnvironmentRequest or the Environment resource needs an update.
	// if err := checkProviders(ctx, r, environment.Namespace, environment.Spec.Providers); err != nil {
	// 	return err
	// }

	if err := r.reconcileReleaser(ctx, releasers, environment); err != nil {
		return err
	}

	// There is no explicit retry here as it is not necessarily needed. If releasers is not successful
	// then the Job will get deleted after a while. When that job is deleted, a reconcile is called for
	// and the Environment will try to get released again.
	if releasers.successful() {
		environmentCondition := meta.FindStatusCondition(environment.Status.Conditions, StatusActive)
		environment.Status.CompletionTime = &environmentCondition.LastTransitionTime
		return r.Status().Update(ctx, environment)
	}

	return nil
}

// reconcileReleaser will check the status of environment releasers, create new ones if necessary.
func (r *EnvironmentReconciler) reconcileReleaser(ctx context.Context, releasers *jobs, environment *etosv1alpha1.Environment) error {
	logger := log.FromContext(ctx)

	// Environment releaser failed, setting status.
	if releasers.failed() {
		releaser := releasers.failedJobs[0] // TODO
		result, err := terminationLog(ctx, r, releaser)
		if err != nil {
			result.Description = err.Error()
		}
		if result.Description == "" {
			result.Description = "Failed release an environment - Unknown error"
		}
		if meta.SetStatusCondition(&environment.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionFalse, Reason: "Failed", Message: result.Description}) {
			logger.Info("Setting environment (failed) false")
			return r.Status().Update(ctx, environment)
		}
	}
	// Environment releaser successful, setting status.
	if releasers.successful() {
		releaser := releasers.successfulJobs[0] // TODO
		result, err := terminationLog(ctx, r, releaser)
		if err != nil {
			result.Description = err.Error()
		}
		if result.Conclusion == ConclusionFailed {
			if meta.SetStatusCondition(&environment.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionFalse, Reason: "Failed", Message: result.Description}) {
				logger.Info("Setting environment (failed) false")
				return r.Status().Update(ctx, environment)
			}
		}
		if meta.SetStatusCondition(&environment.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionFalse, Reason: "Released", Message: result.Description}) {
			logger.Info("Setting environment (success) false")
			for _, environmentProvider := range releasers.successfulJobs {
				if err := r.Delete(ctx, environmentProvider, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
					if !apierrors.IsNotFound(err) {
						return err
					}
				}
			}
			return r.Status().Update(ctx, environment)
		}
	}
	// Suite runners active, setting status
	if releasers.active() {
		if meta.SetStatusCondition(&environment.Status.Conditions, metav1.Condition{Type: StatusActive, Status: metav1.ConditionFalse, Reason: "Releasing", Message: "Environment is being released"}) {
			logger.Info("Setting environment (active) true")
			return r.Status().Update(ctx, environment)
		}
	}
	// Environment is being released and no releaser is active, create an environment releaser
	if releasers.empty() && !environment.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(environment, releaseFinalizer) {
			logger.Info("Environment is being deleted, release it")
			environmentRequest, err := r.environmentRequest(ctx, environment)
			if err != nil {
				return err
			}
			releaser := r.releaseJob(environment, environmentRequest)
			fmt.Println(releaser)
			if err := ctrl.SetControllerReference(environment, releaser, r.Scheme); err != nil {
				return err
			}
			if err := r.Create(ctx, releaser); err != nil {
				return err
			}
		}
	}
	return nil
}

// environmentRequest that owns an environment
func (r *EnvironmentReconciler) environmentRequest(ctx context.Context, environment *etosv1alpha1.Environment) (*etosv1alpha1.EnvironmentRequest, error) {
	environmentRequestName := ""
	for _, owner := range environment.GetOwnerReferences() {
		if owner.Kind == "EnvironmentRequest" {
			environmentRequestName = owner.Name
		}
	}
	if environmentRequestName == "" {
		return nil, errors.New("failed to find EnvironmentRequest owner")
	}
	environmentRequest := &etosv1alpha1.EnvironmentRequest{}
	err := r.Get(ctx, types.NamespacedName{Name: environmentRequestName, Namespace: environment.Namespace}, environmentRequest)
	if err != nil {
		return nil, err
	}
	return environmentRequest, nil
}

// releaseJob is the job definition for an environment releaser.
func (r EnvironmentReconciler) releaseJob(environment *etosv1alpha1.Environment, environmentRequest *etosv1alpha1.EnvironmentRequest) *batchv1.Job {
	id := environment.Labels["etos.eiffel-community.github.io/id"]
	cluster := environment.Labels["etos.eiffel-community.github.io/cluster"]
	ttl := int32(300)
	grace := int64(30)
	backoff := int32(0)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"etos.eiffel-community.github.io/id":        id,
				"etos.eiffel-community.github.io/sub-suite": environment.Name,
				"etos.eiffel-community.github.io/cluster":   cluster,
				"app.kubernetes.io/name":                    "environment-releaser",
				"app.kubernetes.io/part-of":                 "etos",
			},
			Annotations: make(map[string]string),
			Name:        environment.Name,
			Namespace:   environment.Namespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			BackoffLimit:            &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: environment.Name,
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &grace,
					ServiceAccountName:            fmt.Sprintf("%s-provider", cluster),
					RestartPolicy:                 "Never",
					Containers: []corev1.Container{
						{
							Name:            environment.Name,
							Image:           environmentRequest.Spec.Image.Image,
							ImagePullPolicy: environmentRequest.Spec.ImagePullPolicy,
							Command:         []string{"python", "-u", "-m", "environment_provider.environment"},
							Args:            []string{environment.Name},
						},
					},
				},
			},
		},
	}
}

// registerOwnerIndexForJob will set an index of the jobs that an environment owns.
func (r *EnvironmentReconciler) registerOwnerIndexForJob(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, EnvironmentOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGVStr || owner.Kind != "Environment" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register indexes for faster lookups
	if err := r.registerOwnerIndexForJob(mgr); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha1.Environment{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
