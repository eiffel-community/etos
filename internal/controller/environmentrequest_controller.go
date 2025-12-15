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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/internal/controller/jobs"
	"github.com/eiffel-community/etos/internal/controller/status"
)

const environmentRequestKind = "EnvironmentRequest"

// EnvironmentRequestReconciler reconciles a EnvironmentRequest object
type EnvironmentRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=iuts,verbs=get;list;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=executionspaces,verbs=get;list;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=logarea,verbs=get;list;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers,verbs=get;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers/status,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
// +kubebuilder:rbac:groups=*,resources=pods,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *EnvironmentRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Get environment if exists.
	environmentrequest := &etosv1alpha1.EnvironmentRequest{}
	err := r.Get(ctx, req.NamespacedName, environmentrequest)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("environmentrequest not found. ignoring object")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get environmentrequest")
		return ctrl.Result{}, err
	}
	if environmentrequest.DeletionTimestamp.IsZero() {
		// Add a finalizer if there is none, the finalizer will stop the Kubernetes garbage collector
		// from removing the EnvironmentRequest until the finalizer is removed. Removal of the finalizer
		// is done below, if DeletionTimestamp is set and all Environments related to this EnvironmentRequest
		// are removed.
		if !controllerutil.ContainsFinalizer(environmentrequest, releaseFinalizer) {
			controllerutil.AddFinalizer(environmentrequest, releaseFinalizer)
			if err := r.Update(ctx, environmentrequest); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	} else if controllerutil.ContainsFinalizer(environmentrequest, releaseFinalizer) {
		// Environmentrequest is under deletion. Wait for environments to get deleted before finalizing.
		return r.reconcileDeletion(ctx, environmentrequest)
	}
	if environmentrequest.Status.CompletionTime != nil {
		return ctrl.Result{}, nil
	}
	if err := r.reconcile(ctx, environmentrequest); err != nil {
		if apierrors.IsConflict(err) {
			logger.Error(err, "Reconciliation conflict, requeuing")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Reconciliation failed")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *EnvironmentRequestReconciler) reconcile(ctx context.Context, environmentrequest *etosv1alpha1.EnvironmentRequest) error {
	logger := logf.FromContext(ctx)
	// Set initial statuses if not set.
	if ready := meta.FindStatusCondition(environmentrequest.Status.Conditions, status.StatusReady); ready == nil {
		meta.SetStatusCondition(&environmentrequest.Status.Conditions,
			metav1.Condition{
				Type:    status.StatusReady,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonPending,
				Message: "Reconciliation started",
			})
		return r.Status().Update(ctx, environmentrequest)
	} else if ready.Reason == status.ReasonFailed {
		logger.Info("Environment request has failed, reconciliation canceled")
		return nil
	}

	// Check providers availability
	providers := etosv1alpha1.Providers{
		IUT:            environmentrequest.Spec.Providers.IUT.ID,
		ExecutionSpace: environmentrequest.Spec.Providers.ExecutionSpace.ID,
		LogArea:        environmentrequest.Spec.Providers.LogArea.ID,
	}
	if err := checkProviders(ctx, r, environmentrequest.Namespace, providers); err != nil {
		meta.SetStatusCondition(&environmentrequest.Status.Conditions,
			metav1.Condition{
				Type:    status.StatusReady,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonFailed,
				Message: fmt.Sprintf("Provider check failed: %s", err.Error()),
			})
		return r.Status().Update(ctx, environmentrequest)
	}

	if err := r.reconcileEnvironmentProvider(ctx, environmentrequest); err != nil {
		return err
	}

	return nil
}

// reconcileEnvironmentProvider will check the status of environment providers, create new ones if necessary.
func (r *EnvironmentRequestReconciler) reconcileEnvironmentProvider(ctx context.Context, environmentrequest *etosv1alpha1.EnvironmentRequest) error {
	logger := logf.FromContext(ctx)
	conditions := &environmentrequest.Status.Conditions
	jobManager := jobs.NewJob(r.Client, EnvironmentRequestOwnerKey, environmentrequest.GetName(), environmentrequest.GetNamespace())
	jobStatus, err := jobManager.Status(ctx)
	if err != nil {
		logger.Error(err, "error getting job status")
		return err
	}
	switch jobStatus {
	case jobs.StatusFailed:
		result := jobManager.Result(ctx, "environment-provider", "iut-provider", "execution-space-provider", "log-area-provider")
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusReady,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonFailed,
				Message: result.Description,
			}) {
			environmentRequestCondition := meta.FindStatusCondition(environmentrequest.Status.Conditions, status.StatusReady)
			environmentrequest.Status.CompletionTime = &environmentRequestCondition.LastTransitionTime
			return r.Status().Update(ctx, environmentrequest)
		}
	case jobs.StatusSuccessful:
		result := jobManager.Result(ctx, "environment-provider", "iut-provider", "execution-space-provider", "log-area-provider")
		var condition metav1.Condition
		if result.Conclusion == jobs.ConclusionFailed {
			condition = metav1.Condition{
				Type:    status.StatusReady,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonFailed,
				Message: result.Description,
			}
		} else {
			condition = metav1.Condition{
				Type:    status.StatusReady,
				Status:  metav1.ConditionTrue,
				Reason:  status.ReasonCompleted,
				Message: result.Description,
			}
		}
		if meta.SetStatusCondition(conditions, condition) {
			environmentRequestCondition := meta.FindStatusCondition(environmentrequest.Status.Conditions, status.StatusReady)
			environmentrequest.Status.CompletionTime = &environmentRequestCondition.LastTransitionTime
			return errors.Join(r.Status().Update(ctx, environmentrequest), jobManager.Delete(ctx), r.cleanupUnused(ctx, environmentrequest))
		}
	case jobs.StatusActive:
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusReady,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonActive,
				Message: "Environment provider is running",
			}) {
			return r.Status().Update(ctx, environmentrequest)
		}
	default:
		if !environmentrequest.GetDeletionTimestamp().IsZero() {
			return nil
		}
		if err := jobManager.Create(ctx, environmentrequest, r.environmentProviderJob); err != nil {
			logger.Error(err, "Failed to create environment provider job")
			// When we create a job the job gets a unique name. If there's an error for that unique name the error
			// message in Condition.Message is also unique meaning we will update the StatusCondition every time,
			// causing a nasty reconciliation loop (when the environment request gets updated a new reconciliation starts).
			// We mitigate this by checking that StatusReason is not already Failed.
			if !isStatusReason(*conditions, status.StatusReady, status.ReasonFailed) && meta.SetStatusCondition(conditions,
				metav1.Condition{
					Type:    status.StatusReady,
					Status:  metav1.ConditionFalse,
					Reason:  status.ReasonFailed,
					Message: err.Error(),
				}) {
				return r.Status().Update(ctx, environmentrequest)
			}
			return err
		}
	}
	return nil
}

// cleanupUnused deletes IUTs, LogAreas and ExecutionSpaces without Environments i.e. danglig resources.
func (r EnvironmentRequestReconciler) cleanupUnused(ctx context.Context, environmentrequest *etosv1alpha1.EnvironmentRequest) error {
	logger := logf.FromContext(ctx)
	var iuts etosv1alpha2.IutList
	if err := r.List(ctx, &iuts, client.InNamespace(environmentrequest.Namespace), client.MatchingLabels{"etos.eiffel-community.github.io/environment-request-id": environmentrequest.Spec.ID}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list IUTs for environmentrequest %s", environmentrequest.Name), "id", environmentrequest.Spec.ID)
		return err
	}
	for _, iut := range iuts.Items {
		if !ownedByEnvironment(iut.ObjectMeta.OwnerReferences) {
			if err := r.Delete(ctx, &iut); err != nil {
				return err
			}
		}
	}
	var executionSpaces etosv1alpha2.ExecutionSpaceList
	if err := r.List(ctx, &executionSpaces, client.InNamespace(environmentrequest.Namespace), client.MatchingLabels{"etos.eiffel-community.github.io/environment-request-id": environmentrequest.Spec.ID}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list execution spaces for environmentrequest %s", environmentrequest.Name), "id", environmentrequest.Spec.ID)
		return err
	}
	for _, executionSpace := range executionSpaces.Items {
		if !ownedByEnvironment(executionSpace.ObjectMeta.OwnerReferences) {
			if err := r.Delete(ctx, &executionSpace); err != nil {
				return err
			}
		}
	}
	var logAreas etosv1alpha2.LogAreaList
	if err := r.List(ctx, &logAreas, client.InNamespace(environmentrequest.Namespace), client.MatchingLabels{"etos.eiffel-community.github.io/environment-request-id": environmentrequest.Spec.ID}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list log areas for environmentrequest %s", environmentrequest.Name), "id", environmentrequest.Spec.ID)
		return err
	}
	for _, logArea := range logAreas.Items {
		if !ownedByEnvironment(logArea.ObjectMeta.OwnerReferences) {
			if err := r.Delete(ctx, &logArea); err != nil {
				return err
			}
		}
	}
	return nil
}

// reconcileDeletion checks for active environments and deletes them, causing them to clean up, and then, when all environments
// are deleted, this function will remove the finalizer on the environmentrequest and the environmentrequest will be removed.
func (r EnvironmentRequestReconciler) reconcileDeletion(ctx context.Context, environmentrequest *etosv1alpha1.EnvironmentRequest) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	statusReady := meta.FindStatusCondition(environmentrequest.Status.Conditions, status.StatusReady)
	if statusReady == nil {
		statusReady = &metav1.Condition{Type: status.StatusReady, Status: metav1.ConditionFalse, Reason: "Unknown"}
	}
	if !isStatusReason(environmentrequest.Status.Conditions, status.StatusReady, status.ReasonFailed) && meta.SetStatusCondition(&environmentrequest.Status.Conditions,
		metav1.Condition{
			Type:    statusReady.Type,
			Status:  statusReady.Status,
			Reason:  statusReady.Reason,
			Message: "Releasing environment",
		}) {
		if err := r.Status().Update(ctx, environmentrequest); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	var allErr error
	environments, err := r.deleteEnvironments(ctx, *environmentrequest)
	allErr = errors.Join(allErr, err)
	iuts, err := r.deleteIuts(ctx, *environmentrequest)
	allErr = errors.Join(allErr, err)
	logAreas, err := r.deleteLogAreas(ctx, *environmentrequest)
	allErr = errors.Join(allErr, err)
	executionSpaces, err := r.deleteExecutionSpaces(ctx, *environmentrequest)
	allErr = errors.Join(allErr, err)

	if allErr != nil {
		return ctrl.Result{Requeue: true}, nil
	}
	if (environments + iuts + logAreas + executionSpaces) != 0 {
		logger.Info("Waiting for Environments to get deleted", "environments", environments, "iuts", iuts, "logareas", logAreas, "executionspaces", executionSpaces)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if controllerutil.RemoveFinalizer(environmentrequest, releaseFinalizer) {
		if err := r.Update(ctx, environmentrequest); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// deleteEnvironments deletes a list of environments.
func (r EnvironmentRequestReconciler) deleteEnvironments(ctx context.Context, environmentrequest etosv1alpha1.EnvironmentRequest) (int, error) {
	logger := logf.FromContext(ctx)
	var environments etosv1alpha1.EnvironmentList
	if err := r.List(ctx, &environments, client.InNamespace(environmentrequest.Namespace), client.MatchingLabels{"etos.eiffel-community.github.io/id": environmentrequest.Spec.Identifier}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list environments for environmentrequest %s", environmentrequest.Name))
		return -1, err
	}
	var err error
	var allErr error
	for _, environment := range environments.Items {
		if environment.ObjectMeta.DeletionTimestamp.IsZero() {
			if err = r.Delete(ctx, &environment); err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "failed to delete environment", "environment", environment)
					allErr = errors.Join(allErr, err)
				}
			}
		}
	}
	return len(environments.Items), allErr
}

// deleteIuts deletes a list of iuts.
func (r EnvironmentRequestReconciler) deleteIuts(ctx context.Context, environmentrequest etosv1alpha1.EnvironmentRequest) (int, error) {
	logger := logf.FromContext(ctx)
	var iuts etosv1alpha2.IutList
	if err := r.List(ctx, &iuts, client.InNamespace(environmentrequest.Namespace), client.MatchingLabels{"etos.eiffel-community.github.io/id": environmentrequest.Spec.Identifier}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list iuts for environmentrequest %s", environmentrequest.Name))
		return -1, err
	}
	var err error
	var allErr error
	for _, iut := range iuts.Items {
		if iut.ObjectMeta.DeletionTimestamp.IsZero() {
			if err = r.Delete(ctx, &iut); err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "failed to delete iut", "iut", iut)
					allErr = errors.Join(allErr, err)
				}
			}
		}
	}
	return len(iuts.Items), allErr
}

// deleteLogAreas deletes a list of log areas.
func (r EnvironmentRequestReconciler) deleteLogAreas(ctx context.Context, environmentrequest etosv1alpha1.EnvironmentRequest) (int, error) {
	logger := logf.FromContext(ctx)
	var logAreas etosv1alpha2.LogAreaList
	if err := r.List(ctx, &logAreas, client.InNamespace(environmentrequest.Namespace), client.MatchingLabels{"etos.eiffel-community.github.io/id": environmentrequest.Spec.Identifier}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list iuts for environmentrequest %s", environmentrequest.Name))
		return -1, err
	}
	var err error
	var allErr error
	for _, logArea := range logAreas.Items {
		if logArea.ObjectMeta.DeletionTimestamp.IsZero() {
			if err = r.Delete(ctx, &logArea); err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "failed to delete logArea", "logArea", logArea)
					allErr = errors.Join(allErr, err)
				}
			}
		}
	}
	return len(logAreas.Items), allErr
}

// deleteExecutionSpaces deletes a list of execution spaces.
func (r EnvironmentRequestReconciler) deleteExecutionSpaces(ctx context.Context, environmentrequest etosv1alpha1.EnvironmentRequest) (int, error) {
	logger := logf.FromContext(ctx)
	var executionSpaces etosv1alpha2.ExecutionSpaceList
	if err := r.List(ctx, &executionSpaces, client.InNamespace(environmentrequest.Namespace), client.MatchingLabels{"etos.eiffel-community.github.io/id": environmentrequest.Spec.Identifier}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list iuts for environmentrequest %s", environmentrequest.Name))
		return -1, err
	}
	var err error
	var allErr error
	for _, executionSpace := range executionSpaces.Items {
		if executionSpace.ObjectMeta.DeletionTimestamp.IsZero() {
			if err = r.Delete(ctx, &executionSpace); err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "failed to delete executionSpace", "executionSpace", executionSpace)
					allErr = errors.Join(allErr, err)
				}
			}
		}
	}
	return len(executionSpaces.Items), allErr
}

// environmentProviderJob is the job definition for an etos environment provider.
func (r EnvironmentRequestReconciler) environmentProviderJob(ctx context.Context, obj client.Object) (*batchv1.Job, error) {
	grace := int64(30)
	backoff := int32(0)

	environmentrequest, ok := obj.(*etosv1alpha1.EnvironmentRequest)
	if !ok {
		return nil, errors.New("object received from job manager is not an EnvironmentRequest")
	}
	labels := map[string]string{
		"app.kubernetes.io/name":    "environment-provider",
		"app.kubernetes.io/part-of": "etos",
	}
	if environmentrequest.Spec.Identifier != "" {
		labels["etos.eiffel-community.github.io/id"] = environmentrequest.Spec.Identifier
	}

	var cluster *etosv1alpha1.Cluster
	if clusterName := environmentrequest.Labels["etos.eiffel-community.github.io/cluster"]; clusterName != "" {
		labels["etos.eiffel-community.github.io/cluster"] = clusterName
		clusterNamespacedName := types.NamespacedName{
			Name:      clusterName,
			Namespace: environmentrequest.Namespace,
		}
		cluster = &etosv1alpha1.Cluster{}
		if err := r.Get(ctx, clusterNamespacedName, cluster); err != nil {
			logger.Info("Failed to get cluster resource!")
			return nil, err
		}
	}

	envVarList, err := r.envVarListFrom(ctx, environmentrequest, cluster)
	if err != nil {
		logger.Error(err, "Failed to create environment variable list for environment provider")
		return nil, err
	}

	iutProvider, err := getProvider(ctx, r.Client, environmentrequest.Spec.Providers.IUT.ID, environmentrequest.Namespace)
	if err != nil {
		return nil, err
	}
	logAreaProvider, err := getProvider(ctx, r.Client, environmentrequest.Spec.Providers.LogArea.ID, environmentrequest.Namespace)
	if err != nil {
		return nil, err
	}
	executionSpaceProvider, err := getProvider(ctx, r.Client, environmentrequest.Spec.Providers.ExecutionSpace.ID, environmentrequest.Namespace)
	if err != nil {
		return nil, err
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: "environment-provider-", // unique names to allow multiple environment provider jobs
			Namespace:    environmentrequest.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   environmentrequest.Name,
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:            environmentrequest.Spec.ServiceAccountName,
					TerminationGracePeriodSeconds: &grace,
					RestartPolicy:                 "Never",
					InitContainers: []corev1.Container{
						{
							Name:  "iut-provider",
							Image: imageFromProvider(iutProvider),
							Args: []string{
								fmt.Sprintf("-namespace=%s", environmentrequest.Namespace),
								fmt.Sprintf("-environment-request=%s", environmentrequest.Name),
								fmt.Sprintf("-provider=%s", iutProvider.Name),
							},
						},
						{
							Name:  "log-area-provider",
							Image: imageFromProvider(logAreaProvider),
							Args: []string{
								fmt.Sprintf("-namespace=%s", environmentrequest.Namespace),
								fmt.Sprintf("-environment-request=%s", environmentrequest.Name),
								fmt.Sprintf("-provider=%s", logAreaProvider.Name),
							},
						},
						{
							Name:  "execution-space-provider",
							Image: imageFromProvider(executionSpaceProvider),
							Args: []string{
								fmt.Sprintf("-namespace=%s", environmentrequest.Namespace),
								fmt.Sprintf("-environment-request=%s", environmentrequest.Name),
								fmt.Sprintf("-provider=%s", executionSpaceProvider.Name),
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "environment-provider",
							Image:           environmentrequest.Spec.Image.Image,
							ImagePullPolicy: environmentrequest.Spec.Image.ImagePullPolicy,
							Args: []string{
								fmt.Sprintf("-namespace=%s", environmentrequest.Namespace),
								fmt.Sprintf("-environment-request=%s", environmentrequest.Name),
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("250m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
							},
						},
					},
				},
			},
		},
	}
	return job, ctrl.SetControllerReference(environmentrequest, job, r.Scheme)
}

// registerOwnerIndexForJob will set an index of the jobs that an environment request owns.
func (r *EnvironmentRequestReconciler) registerOwnerIndexForJob(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, EnvironmentRequestOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGroupVersionString || owner.Kind != environmentRequestKind {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// registerOwnerIndexForEnvironment will set an index of the environments that an environment request owns.
func (r *EnvironmentRequestReconciler) registerOwnerIndexForEnvironment(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.Environment{}, EnvironmentRequestOwnerKey, func(rawObj client.Object) []string {
		environment := rawObj.(*etosv1alpha1.Environment)
		owner := metav1.GetControllerOf(environment)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGroupVersionString || owner.Kind != environmentRequestKind {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return nil
}

// findEnvironmentRequestsForIUTProvider will return reconciliation requests for each Provider object that an environment request has stored
// in its spec as IUT. This will cause reconciliations whenever a Provider gets updated, created, deleted etc.
func (r *EnvironmentRequestReconciler) findEnvironmentRequestsForIUTProvider(ctx context.Context, provider client.Object) []reconcile.Request {
	return r.findEnvironmentRequestsForObject(ctx, iutProvider, provider)
}

// findEnvironmentRequestsForIUTProvider will return reconciliation requests for each Provider object that an environment request has stored
// in its spec as execution space. This will cause reconciliations whenever a Provider gets updated, created, deleted etc.
func (r *EnvironmentRequestReconciler) findEnvironmentRequestsForExecutionSpaceProvider(ctx context.Context, provider client.Object) []reconcile.Request {
	return r.findEnvironmentRequestsForObject(ctx, executionSpaceProvider, provider)
}

// findEnvironmentRequestsForIUTProvider will return reconciliation requests for each Provider object that an environment request has stored
// in its spec as log area. This will cause reconciliations whenever a Provider gets updated, created, deleted etc.
func (r *EnvironmentRequestReconciler) findEnvironmentRequestsForLogAreaProvider(ctx context.Context, provider client.Object) []reconcile.Request {
	return r.findEnvironmentRequestsForObject(ctx, logAreaProvider, provider)
}

// findEnvironmentRequestsForTestrun will return reconciliation requests for each testrun object that an environment request has stored
// in its spec.
func (r *EnvironmentRequestReconciler) FindEnvironmentRequestsForTestrun(ctx context.Context, testrun client.Object) []reconcile.Request {
	return r.findEnvironmentRequestsForObject(ctx, ".spec.testrun", testrun)
}

// findEnvironmentRequestsForObject will find environment requests for a kubernetes object.
func (r *EnvironmentRequestReconciler) findEnvironmentRequestsForObject(ctx context.Context, name string, obj client.Object) []reconcile.Request {
	environmentRequestList := &etosv1alpha1.EnvironmentRequestList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(name, obj.GetName()),
		Namespace:     obj.GetNamespace(),
	}
	err := r.List(ctx, environmentRequestList, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(environmentRequestList.Items))
	for i, item := range environmentRequestList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *EnvironmentRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register indexes for faster lookups
	if err := r.registerOwnerIndexForJob(mgr); err != nil {
		return err
	}
	if err := r.registerOwnerIndexForEnvironment(mgr); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.EnvironmentRequest{}, iutProvider, func(rawObj client.Object) []string {
		environmentRequest := rawObj.(*etosv1alpha1.EnvironmentRequest)
		return []string{environmentRequest.Spec.Providers.IUT.ID}
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.EnvironmentRequest{}, logAreaProvider, func(rawObj client.Object) []string {
		environmentRequest := rawObj.(*etosv1alpha1.EnvironmentRequest)
		return []string{environmentRequest.Spec.Providers.LogArea.ID}
	}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &etosv1alpha1.EnvironmentRequest{}, executionSpaceProvider, func(rawObj client.Object) []string {
		environmentRequest := rawObj.(*etosv1alpha1.EnvironmentRequest)
		return []string{environmentRequest.Spec.Providers.ExecutionSpace.ID}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha1.EnvironmentRequest{}).
		Named("environmentrequest").
		Owns(&batchv1.Job{}).
		Owns(&etosv1alpha1.Environment{}).
		Watches(
			&etosv1alpha1.Provider{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findEnvironmentRequestsForIUTProvider),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&etosv1alpha1.Provider{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findEnvironmentRequestsForLogAreaProvider),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&etosv1alpha1.Provider{},
			handler.TypedEnqueueRequestsFromMapFunc(r.findEnvironmentRequestsForExecutionSpaceProvider),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
