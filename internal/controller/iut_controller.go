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

// IutReconciler reconciles a Iut object
type IutReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=iuts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=iuts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=iuts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *IutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	iut := &etosv1alpha2.Iut{}
	err := r.Get(ctx, req.NamespacedName, iut)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ownership handoff: If Environment owns this IUT, we relinquish control
	if ownedByEnvironment(iut.OwnerReferences) {
		if controllerutil.ContainsFinalizer(iut, providerFinalizer) {
			// Clean up our finalizer since the environment controller now owns the IUT.
			controllerutil.RemoveFinalizer(iut, providerFinalizer)
			if err := r.Update(ctx, iut); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
		}

		if iut.ObjectMeta.DeletionTimestamp.IsZero() {
			// The IUT is not being deleted, in use by environment
			if meta.SetStatusCondition(&iut.Status.Conditions,
				metav1.Condition{
					Status:  metav1.ConditionTrue,
					Type:    status.StatusActive,
					Reason:  status.ReasonActive,
					Message: "In use",
				}) {
				if err := r.Status().Update(ctx, iut); err != nil {
					if apierrors.IsConflict(err) {
						return ctrl.Result{Requeue: true}, nil
					}
					return ctrl.Result{}, err
				}
			}
		} else {
			// The IUT is being deleted, update status to reflect this.
			// Since the IUT controller at this point no longer has ownership of the IUT we will not
			// initiate and handle any release or cleanup, this is now handled by the environment
			// controller.
			if meta.SetStatusCondition(&iut.Status.Conditions,
				metav1.Condition{
					Status:  metav1.ConditionFalse,
					Type:    status.StatusActive,
					Reason:  status.ReasonPending,
					Message: "Releasing IUT",
				}) {
				if err := r.Status().Update(ctx, iut); err != nil {
					if apierrors.IsConflict(err) {
						return ctrl.Result{Requeue: true}, nil
					}
					return ctrl.Result{}, err
				}
			}
		}
		logger.Info("Iut is being managed by Environment", "iut", iut.Name)
		return ctrl.Result{}, nil
	}
	// If the IUT is considered 'Completed', it has been released. Check that the object is
	// being deleted and contains the finalizer and remove the finalizer.
	if iut.Status.CompletionTime != nil {
		if !iut.ObjectMeta.DeletionTimestamp.IsZero() {
			if controllerutil.ContainsFinalizer(iut, providerFinalizer) {
				controllerutil.RemoveFinalizer(iut, providerFinalizer)
				if err := r.Update(ctx, iut); err != nil {
					if apierrors.IsConflict(err) {
						return ctrl.Result{Requeue: true}, nil
					}
					return ctrl.Result{}, err
				}
			}
		}
		return ctrl.Result{}, nil
	}
	if err := r.reconcile(ctx, iut); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// reconcile an IUT resource to its desired state.
func (r *IutReconciler) reconcile(ctx context.Context, iut *etosv1alpha2.Iut) error {
	logger := logf.FromContext(ctx)

	// Set initial statuses if not set.
	if active := meta.FindStatusCondition(iut.Status.Conditions, status.StatusActive); active == nil {
		meta.SetStatusCondition(&iut.Status.Conditions,
			metav1.Condition{
				Status:  metav1.ConditionFalse,
				Type:    status.StatusActive,
				Reason:  status.ReasonPending,
				Message: "Waiting for environment",
			})
		return r.Status().Update(ctx, iut)
	} else if active.Reason == status.ReasonFailed {
		logger.Info("IUT failed, reconciliation canceled")
		return nil
	}
	if iut.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(iut, providerFinalizer) {
			controllerutil.AddFinalizer(iut, providerFinalizer)
			logger.Info("Iut is being managed by Iut controller", "iut", iut.Name)
			return r.Update(ctx, iut)
		}
	}

	if !iut.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileIutReleaser(ctx, iut)
	}
	return nil
}

// reconcileIutReleaser gets the status of a release job, creating a new release job if necessary.
func (r *IutReconciler) reconcileIutReleaser(ctx context.Context, iut *etosv1alpha2.Iut) error {
	conditions := &iut.Status.Conditions
	jobManager := jobs.NewJob(r.Client, IutOwnerKey, iut.GetName(), iut.GetNamespace())
	jobStatus, err := jobManager.Status(ctx)
	if err != nil {
		return err
	}
	switch jobStatus {
	case jobs.StatusFailed:
		result := jobManager.Result(ctx, release.IutReleaserName)
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusActive,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonFailed,
				Message: result.Description,
			}) {
			return r.Status().Update(ctx, iut)
		}
	case jobs.StatusSuccessful:
		result := jobManager.Result(ctx, release.IutReleaserName)
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
		iutCondition := meta.FindStatusCondition(*conditions, status.StatusActive)
		iut.Status.CompletionTime = &iutCondition.LastTransitionTime
		if meta.SetStatusCondition(conditions, condition) {
			return errors.Join(r.Status().Update(ctx, iut), jobManager.Delete(ctx))
		}
	case jobs.StatusActive:
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusActive,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonPending,
				Message: "Releasing IUT",
			}) {
			return r.Status().Update(ctx, iut)
		}
	default:
		// Since this is a release job, we don't want to release if we are not deleting.
		if iut.GetDeletionTimestamp().IsZero() {
			return nil
		}
		if err := jobManager.Create(ctx, iut, r.releaseJob); err != nil {
			// When we create a job the job gets a unique name. If there's an error for that unique name the error
			// message in Condition.Message is also unique meaning we will update the StatusCondition every time,
			// causing a nasty reconciliation loop (when the iut gets updated a new reconciliation starts).
			// We mitigate this by checking that StatusReason is not already Failed.
			if !isStatusReason(*conditions, status.StatusActive, status.ReasonFailed) && meta.SetStatusCondition(conditions,
				metav1.Condition{
					Type:    status.StatusActive,
					Status:  metav1.ConditionFalse,
					Reason:  status.ReasonFailed,
					Message: err.Error(),
				}) {
				return r.Status().Update(ctx, iut)
			}
			return err
		}
		if meta.SetStatusCondition(conditions, metav1.Condition{
			Status:  metav1.ConditionFalse,
			Type:    status.StatusActive,
			Reason:  status.ReasonPending,
			Message: "Releasing IUT",
		}) {
			return r.Status().Update(ctx, iut)
		}
	}
	return nil
}

// releaseJob is the job definition for an IUT releaser.
func (r IutReconciler) releaseJob(ctx context.Context, obj client.Object) (*batchv1.Job, error) {
	iut, ok := obj.(*etosv1alpha2.Iut)
	if !ok {
		return nil, errors.New("object received from job manager is not an Iut")
	}

	provider, err := getProvider(ctx, r, iut.Spec.ProviderID, iut.GetNamespace())
	if err != nil {
		return nil, err
	}
	environmentrequest := &etosv1alpha1.EnvironmentRequest{}
	if err := r.Get(ctx, types.NamespacedName{Name: iut.Spec.EnvironmentRequest, Namespace: iut.Namespace}, environmentrequest); err != nil {
		return nil, err
	}

	jobSpec := release.IutReleaser(iut, environmentrequest, imageFromProvider(provider), true)
	return jobSpec, ctrl.SetControllerReference(iut, jobSpec, r.Scheme)
}

// registerOwnerIndexForJob will set an index of the jobs that an iut controller owns.
func (r *IutReconciler) registerOwnerIndexForJob(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, IutOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIv2GroupVersionString || owner.Kind != "Iut" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register indexes for faster lookups
	if err := r.registerOwnerIndexForJob(mgr); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha2.Iut{}).
		Named("iut").
		Owns(&batchv1.Job{}). // Release job
		Complete(r)
}
