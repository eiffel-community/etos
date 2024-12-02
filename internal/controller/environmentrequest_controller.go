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
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// EnvironmentRequestReconciler reconciles a EnvironmentRequest object
type EnvironmentRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environmentrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers,verbs=get
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *EnvironmentRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

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
	if environmentrequest.Status.CompletionTime != nil {
		return ctrl.Result{}, nil
	}
	if err := r.reconcile(ctx, environmentrequest); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *EnvironmentRequestReconciler) reconcile(ctx context.Context, environmentrequest *etosv1alpha1.EnvironmentRequest) error {
	logger := log.FromContext(ctx)

	// Set initial statuses if not set.
	if meta.FindStatusCondition(environmentrequest.Status.Conditions, StatusReady) == nil {
		logger.Info("Set ready status")
		meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Status: metav1.ConditionFalse, Type: StatusReady, Message: "Reconciliation started", Reason: "Pending"})
		return r.Status().Update(ctx, environmentrequest)
	}

	// Get active, finished and failed environment providers.
	// TODO: Handle unique names for environment jobs.
	environmentProviders, err := jobStatus(ctx, r, environmentrequest.Namespace, environmentrequest.Name, EnvironmentRequestOwnerKey)
	if err != nil {
		return err
	}

	environmentrequest.Status.EnvironmentProviders = nil
	for _, activeProvider := range environmentProviders.activeJobs {
		jobRef, err := ref.GetReference(r.Scheme, activeProvider)
		if err != nil {
			logger.Error(err, "failed to make reference to active environment provider", "provider", activeProvider)
			continue
		}
		environmentrequest.Status.EnvironmentProviders = append(environmentrequest.Status.EnvironmentProviders, *jobRef)
	}
	if err := r.Status().Update(ctx, environmentrequest); err != nil {
		return err
	}
	logger.V(1).Info("environment provider count", "active", len(environmentProviders.activeJobs), "successful", len(environmentProviders.successfulJobs), "failed", len(environmentProviders.failedJobs))

	// Check providers availability
	providers := etosv1alpha1.Providers{
		IUT:            environmentrequest.Spec.Providers.IUT.ID,
		ExecutionSpace: environmentrequest.Spec.Providers.ExecutionSpace.ID,
		LogArea:        environmentrequest.Spec.Providers.LogArea.ID,
	}
	if err := checkProviders(ctx, r, environmentrequest.Namespace, providers); err != nil {
		meta.SetStatusCondition(&environmentrequest.Status.Conditions,
			metav1.Condition{
				Status:  metav1.ConditionFalse,
				Type:    StatusReady,
				Message: fmt.Sprintf("Provider check failed: %s", err.Error()),
				Reason:  "Failed",
			})
		logger.Error(err, "Error occurred while checking provider status")
		return r.Status().Update(ctx, environmentrequest)
	}

	// Reconcile environment provider
	if err := r.reconcileEnvironmentProvider(ctx, environmentProviders, environmentrequest); err != nil {
		meta.SetStatusCondition(&environmentrequest.Status.Conditions,
			metav1.Condition{
				Status:  metav1.ConditionFalse,
				Type:    StatusReady,
				Message: fmt.Sprintf("Environment provider reconciliation failed: %s", err.Error()),
				Reason:  "Failed",
			})
		logger.Error(err, "Error occurred while reconciling environment provider")
		return r.Status().Update(ctx, environmentrequest)
	}

	if environmentProviders.failed() {
		if environmentrequest.Status.CompletionTime == nil {
			environmentCondition := meta.FindStatusCondition(environmentrequest.Status.Conditions, StatusReady)
			environmentrequest.Status.CompletionTime = &environmentCondition.LastTransitionTime
			return r.Status().Update(ctx, environmentrequest)
		}
	}
	if environmentProviders.successful() {
		if environmentrequest.Status.CompletionTime == nil {
			environmentCondition := meta.FindStatusCondition(environmentrequest.Status.Conditions, StatusReady)
			environmentrequest.Status.CompletionTime = &environmentCondition.LastTransitionTime
			return r.Status().Update(ctx, environmentrequest)
		}
	}

	return nil
}

// reconcileEnvironmentProvider will check the status of environment providers, create new ones if necessary.
func (r *EnvironmentRequestReconciler) reconcileEnvironmentProvider(ctx context.Context, providers *jobs, environmentrequest *etosv1alpha1.EnvironmentRequest) error {
	// Environment provider failed, setting status.
	if providers.failed() {
		environmentProvider := providers.failedJobs[0] // TODO: We should support multiple providers in the future
		result, err := terminationLog(ctx, r, environmentProvider, environmentrequest.Name)
		if err != nil {
			result.Description = err.Error()
		}
		if result.Description == "" {
			result.Description = "Failed to provision an environment - Unknown error"
		}
		if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: StatusReady, Status: metav1.ConditionFalse, Reason: "Failed", Message: result.Description}) {
			return r.Status().Update(ctx, environmentrequest)
		}
	}
	// Environment provider successful, setting status.
	if providers.successful() {
		environmentProvider := providers.successfulJobs[0] // TODO: We should support multiple providers in the future
		result, err := terminationLog(ctx, r, environmentProvider, environmentrequest.Name)
		if err != nil {
			result.Description = err.Error()
		}
		if result.Conclusion == ConclusionFailed {
			if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: StatusReady, Status: metav1.ConditionFalse, Reason: "Failed", Message: result.Description}) {
				return r.Status().Update(ctx, environmentrequest)
			}
		}
		if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: StatusReady, Status: metav1.ConditionTrue, Reason: "Done", Message: result.Description}) {
			for _, environmentProvider := range providers.successfulJobs {
				if err := r.Delete(ctx, environmentProvider, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
					if !apierrors.IsNotFound(err) {
						return err
					}
				}
			}
			return r.Status().Update(ctx, environmentrequest)
		}
	}
	// Suite runners active, setting status
	if providers.active() {
		if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: StatusReady, Status: metav1.ConditionFalse, Reason: "Running", Message: "Environment provider is running"}) {
			return r.Status().Update(ctx, environmentrequest)
		}
	}
	// No environment providers, create environment provider
	if providers.empty() {
		environmentProvider := r.environmentProviderJob(environmentrequest)
		if err := ctrl.SetControllerReference(environmentrequest, environmentProvider, r.Scheme); err != nil {
			return err
		}
		if err := r.Create(ctx, environmentProvider); err != nil {
			return err
		}
	}
	return nil
}

// environmentProviderJob is the job definition for an etos environment provider.
func (r EnvironmentRequestReconciler) environmentProviderJob(environmentrequest *etosv1alpha1.EnvironmentRequest) *batchv1.Job {
	ttl := int32(300)
	grace := int64(30)
	backoff := int32(0)
	// TODO: Cluster might not be a part of the environment request.
	cluster := environmentrequest.Labels["etos.eiffel-community.github.io/cluster"]
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"etos.eiffel-community.github.io/id":      environmentrequest.Spec.Identifier, // TODO: omitempty
				"etos.eiffel-community.github.io/cluster": cluster,
				"app.kubernetes.io/name":                  "environment-provider",
				"app.kubernetes.io/part-of":               "etos",
			},
			Annotations:  make(map[string]string),
			GenerateName: "environment-provider-", // unique names to allow multiple environment provider jobs
			Namespace:    environmentrequest.Namespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			BackoffLimit:            &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: environmentrequest.Name,
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &grace,
					ServiceAccountName:            fmt.Sprintf("%s-provider", cluster),
					RestartPolicy:                 "Never",
					Containers: []corev1.Container{
						{
							Name:            environmentrequest.Name,
							Image:           environmentrequest.Spec.Image.Image,
							ImagePullPolicy: environmentrequest.Spec.ImagePullPolicy,
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
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-environment-provider-cfg", cluster),
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "REQUEST",
									Value: environmentrequest.Name,
								},
							},
						},
					},
				},
			},
		},
	}
}

// registerOwnerIndexForJob will set an index of the jobs that an environment request owns.
func (r *EnvironmentRequestReconciler) registerOwnerIndexForJob(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, EnvironmentRequestOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGroupVersionString || owner.Kind != "EnvironmentRequest" {
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
		Owns(&batchv1.Job{}).
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
