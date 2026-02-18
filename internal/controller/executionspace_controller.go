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

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/internal/controller/jobs"
	"github.com/eiffel-community/etos/internal/controller/status"
	"github.com/eiffel-community/etos/internal/release"
)

// ExecutionSpaceReconciler reconciles a ExecutionSpace object
type ExecutionSpaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=executionspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=executionspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=executionspaces/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *ExecutionSpaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	executionSpace := &etosv1alpha2.ExecutionSpace{}
	err := r.Get(ctx, req.NamespacedName, executionSpace)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ownership handoff: If Environment owns this ExecutionSpace, we relinquish control
	if ownedByEnvironment(executionSpace.OwnerReferences) {
		if controllerutil.ContainsFinalizer(executionSpace, providerFinalizer) {
			// Clean up our finalizer since the environment controller now owns the ExecutionSpace
			controllerutil.RemoveFinalizer(executionSpace, providerFinalizer)
			if err := r.Update(ctx, executionSpace); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
		}
		if executionSpace.ObjectMeta.DeletionTimestamp.IsZero() {
			// The ExecutionSpace is not being deleted, in use by environment
			if meta.SetStatusCondition(&executionSpace.Status.Conditions,
				metav1.Condition{
					Status:  metav1.ConditionTrue,
					Type:    status.StatusActive,
					Reason:  status.ReasonActive,
					Message: "In use",
				}) {
				if err := r.Status().Update(ctx, executionSpace); err != nil {
					if apierrors.IsConflict(err) {
						return ctrl.Result{Requeue: true}, nil
					}
					return ctrl.Result{}, err
				}
			}
		} else {
			// The ExecutionSpace is being deleted, update status to reflect this
			// At this point the environment controller has ownership, so no release
			// job is being created here.
			if meta.SetStatusCondition(&executionSpace.Status.Conditions,
				metav1.Condition{
					Status:  metav1.ConditionFalse,
					Type:    status.StatusActive,
					Reason:  status.ReasonPending,
					Message: "Releasing ExecutionSpace",
				}) {
				if err := r.Status().Update(ctx, executionSpace); err != nil {
					if apierrors.IsConflict(err) {
						return ctrl.Result{Requeue: true}, nil
					}
					return ctrl.Result{}, err
				}
			}
		}
		logger.Info("ExecutionSpace is being managed by Environment", "executionSpace", executionSpace.Name)
		return ctrl.Result{}, nil
	}
	// If the ExecutionSpace is considered 'Completed', it has been released. Check that the object is
	// being deleted and contains the finalizer and remove the finalizer.
	if executionSpace.Status.CompletionTime != nil {
		if !executionSpace.ObjectMeta.DeletionTimestamp.IsZero() {
			if controllerutil.ContainsFinalizer(executionSpace, providerFinalizer) {
				controllerutil.RemoveFinalizer(executionSpace, providerFinalizer)
				if err := r.Update(ctx, executionSpace); err != nil {
					if apierrors.IsConflict(err) {
						return ctrl.Result{Requeue: true}, nil
					}
					return ctrl.Result{}, err
				}
			}
		}
		return ctrl.Result{}, nil
	}
	if err := r.reconcile(ctx, executionSpace); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// reconcile an ExecutionSpace resource to its desired state.
func (r *ExecutionSpaceReconciler) reconcile(ctx context.Context, executionSpace *etosv1alpha2.ExecutionSpace) error {
	logger := logf.FromContext(ctx)

	// Set initial statuses if not set.
	if active := meta.FindStatusCondition(executionSpace.Status.Conditions, status.StatusActive); active == nil {
		meta.SetStatusCondition(&executionSpace.Status.Conditions,
			metav1.Condition{
				Status:  metav1.ConditionFalse,
				Type:    status.StatusActive,
				Reason:  status.ReasonPending,
				Message: "Waiting for environment",
			})
		return r.Status().Update(ctx, executionSpace)
	} else if active.Reason == status.ReasonFailed {
		logger.Info("Execution space failed, reconciliation canceled")
		return nil
	}
	if executionSpace.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(executionSpace, providerFinalizer) {
			controllerutil.AddFinalizer(executionSpace, providerFinalizer)
			logger.Info("ExecutionSpace is being managed by ExecutionSpace controller", "executionSpace", executionSpace.Name)
			return r.Update(ctx, executionSpace)
		}
	}

	if !executionSpace.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileExecutionSpaceReleaser(ctx, executionSpace)
	}
	return nil
}

// reconcileExecutionSpaceReleaser gets the status of a release job, creating a new release job if necessary.
func (r *ExecutionSpaceReconciler) reconcileExecutionSpaceReleaser(ctx context.Context, executionSpace *etosv1alpha2.ExecutionSpace) error {
	conditions := &executionSpace.Status.Conditions
	jobManager := jobs.NewJob(r.Client, ExecutionSpaceOwnerKey, executionSpace.GetName(), executionSpace.GetNamespace())
	jobStatus, err := jobManager.Status(ctx)
	if err != nil {
		return err
	}
	switch jobStatus {
	case jobs.StatusFailed:
		result := jobManager.Result(ctx, release.ExecutionSpaceReleaserName)
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusActive,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonFailed,
				Message: result.Description,
			}) {
			return r.Status().Update(ctx, executionSpace)
		}
	case jobs.StatusSuccessful:
		result := jobManager.Result(ctx, release.ExecutionSpaceReleaserName)
		var condition metav1.Condition
		if result.Conclusion == jobs.ConclusionFailed {
			condition = metav1.Condition{
				Type:    status.StatusActive,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonFailed,
				Message: result.Description,
			}
		} else {
			condition = metav1.Condition{
				Type:    status.StatusActive,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonCompleted,
				Message: result.Description,
			}
		}
		now := metav1.Now()
		executionSpace.Status.CompletionTime = &now
		if meta.SetStatusCondition(conditions, condition) {
			return errors.Join(r.Status().Update(ctx, executionSpace), jobManager.Delete(ctx))
		}
	case jobs.StatusActive:
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusActive,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonPending,
				Message: "Releasing ExecutionSpace",
			}) {
			return r.Status().Update(ctx, executionSpace)
		}
	default:
		// Since this is a release job, we don't want to release if we are not deleting.
		if executionSpace.GetDeletionTimestamp().IsZero() {
			return nil
		}
		if err := jobManager.Create(ctx, executionSpace, r.releaseJob); err != nil {
			// When we create a job the job gets a unique name. If there's an error for that unique name the error
			// message in Condition.Message is also unique meaning we will update the StatusCondition every time,
			// causing a nasty reconciliation loop (when the execution space gets updated a new reconciliation starts).
			// We mitigate this by checking that StatusReason is not already Failed.
			if !isStatusReason(*conditions, status.StatusActive, status.ReasonFailed) && meta.SetStatusCondition(conditions,
				metav1.Condition{
					Type:    status.StatusActive,
					Status:  metav1.ConditionFalse,
					Reason:  status.ReasonFailed,
					Message: err.Error(),
				}) {
				return r.Status().Update(ctx, executionSpace)
			}
			return err
		}
		if meta.SetStatusCondition(conditions, metav1.Condition{
			Status:  metav1.ConditionFalse,
			Type:    status.StatusActive,
			Reason:  status.ReasonPending,
			Message: "Releasing ExecutionSpace",
		}) {
			return r.Status().Update(ctx, executionSpace)
		}
	}
	return nil
}

// releaseJob is the job definition for an execution space releaser.
func (r ExecutionSpaceReconciler) releaseJob(ctx context.Context, obj client.Object) (*batchv1.Job, error) {
	executionSpace, ok := obj.(*etosv1alpha2.ExecutionSpace)
	if !ok {
		return nil, errors.New("object received from job manager is not an ExecutionSpace")
	}

	provider, err := getProvider(ctx, r, executionSpace.Spec.ProviderID, executionSpace.GetNamespace())
	if err != nil {
		return nil, err
	}
	environmentrequest := &etosv1alpha1.EnvironmentRequest{}
	if err := r.Get(ctx, types.NamespacedName{Name: executionSpace.Spec.EnvironmentRequest, Namespace: executionSpace.Namespace}, environmentrequest); err != nil {
		return nil, err
	}

	jobSpec := release.ExecutionSpaceReleaser(executionSpace, environmentrequest, imageFromProvider(provider), true)
	return jobSpec, ctrl.SetControllerReference(executionSpace, jobSpec, r.Scheme)
}

// registerOwnerIndexForJob will set an index of the jobs that an execution space controller owns.
func (r *ExecutionSpaceReconciler) registerOwnerIndexForJob(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, ExecutionSpaceOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIv2GroupVersionString || owner.Kind != "ExecutionSpace" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExecutionSpaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register indexes for faster lookups
	if err := r.registerOwnerIndexForJob(mgr); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha2.ExecutionSpace{}).
		Named("executionspace").
		Owns(&batchv1.Job{}). // Release job
		Complete(r)
}
