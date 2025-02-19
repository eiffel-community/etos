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
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	// Status successful will be set only if all environment provider jobs are successful:
	if environmentProviders.successful() && !environmentProviders.failed() {
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
		description := ""
		for _, environmentProvider := range providers.failedJobs {
			result, err := terminationLog(ctx, r, environmentProvider, environmentrequest.Name)
			if err != nil {
				result.Description = err.Error()
			}
			if result.Description == "" {
				result.Description = "Failed to provision an environment - Unknown error"
			}
			description = fmt.Sprintf("%s; %s: %s", description, environmentProvider.Name, result.Description)
		}
		if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: StatusReady, Status: metav1.ConditionFalse, Reason: "Failed", Message: description}) {
			return r.Status().Update(ctx, environmentrequest)
		}
	}

	// Environment provider successful, setting status.
	if providers.successful() {
		statusUpdated := false
		for _, environmentProvider := range providers.successfulJobs {
			result, err := terminationLog(ctx, r, environmentProvider, environmentrequest.Name)
			if err != nil {
				result.Description = err.Error()
			}
			if result.Conclusion == ConclusionFailed {
				if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: StatusReady, Status: metav1.ConditionFalse, Reason: "Failed", Message: result.Description}) {
					statusUpdated = true
				}
			}
			if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: StatusReady, Status: metav1.ConditionTrue, Reason: "Done", Message: result.Description}) {
				statusUpdated = true
				for _, environmentProvider := range providers.successfulJobs {
					if err := r.Delete(ctx, environmentProvider, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
						if !apierrors.IsNotFound(err) {
							// If returning due to a delete error, the environmentrequest shall be updated here, otherwise later.
							if _err := r.Status().Update(ctx, environmentrequest); _err != nil {
								return _err
							}
							return err
						}
					}
				}
			}
		}
		if statusUpdated {
			return r.Status().Update(ctx, environmentrequest)
		}
	}

	// Providers active, setting status
	// StatusCondition will be the same (Reason: "Running", Ready: false) as long as any provider is active.
	if providers.active() {
		if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: StatusReady, Status: metav1.ConditionFalse, Reason: "Running", Message: "Environment provider is running"}) {
			return r.Status().Update(ctx, environmentrequest)
		}
	}
	// No environment providers, create environment provider
	if providers.empty() {
		environmentProvider, err := r.environmentProviderJob(ctx, environmentrequest)
		if err != nil {
			return err
		}
		if err := ctrl.SetControllerReference(environmentrequest, environmentProvider, r.Scheme); err != nil {
			return err
		}
		if err := r.Create(ctx, environmentProvider); err != nil {
			return err
		}
	}
	return nil
}

// envVarListFrom creates a list of EnvVar key-value pairs from an EnvironmentRequest instance
func (r EnvironmentRequestReconciler) envVarListFrom(ctx context.Context, environmentrequest *etosv1alpha1.EnvironmentRequest) ([]corev1.EnvVar, error) {
	etosEncryptionKey, err := environmentrequest.Spec.Config.EncryptionKey.Get(ctx, r.Client, environmentrequest.Namespace)
	if err != nil {
		return nil, err
	}
	etosRabbitMQPassword, err := environmentrequest.Spec.Config.EtosMessageBus.Password.Get(ctx, r.Client, environmentrequest.Namespace)
	if err != nil {
		return nil, err
	}
	eiffelRabbitMQPassword, err := environmentrequest.Spec.Config.EiffelMessageBus.Password.Get(ctx, r.Client, environmentrequest.Namespace)
	if err != nil {
		return nil, err
	}

	envList := []corev1.EnvVar{
		{
			Name:  "REQUEST",
			Value: environmentrequest.Name,
		},
		{
			Name:  "ETOS_API",
			Value: environmentrequest.Spec.Config.EtosApi,
		},
		{
			Name:  "ETOS_GRAPHQL_SERVER",
			Value: environmentrequest.Spec.Config.GraphQlServer,
		},
		{
			Name:  "ETOS_ENCRYPTION_KEY",
			Value: string(etosEncryptionKey),
		},
		{
			Name:  "ETOS_ETCD_HOST",
			Value: environmentrequest.Spec.Config.EtcdHost,
		},
		{
			Name:  "ETOS_ETCD_PORT",
			Value: environmentrequest.Spec.Config.EtcdPort,
		},
		{
			// Optional when environmentrequest is not issued by testrun, i. e. created separately.
			// When the environment request is issued by a testrun, this variable is propagated
			// further from environment provider to test runner.
			Name:  "ETR_VERSION",
			Value: environmentrequest.Spec.Config.TestRunnerVersion,
		},

		// Eiffel Message Bus variables
		{
			Name:  "RABBITMQ_HOST",
			Value: environmentrequest.Spec.Config.EiffelMessageBus.Host,
		},
		{
			Name:  "RABBITMQ_VHOST",
			Value: environmentrequest.Spec.Config.EiffelMessageBus.Vhost,
		},
		{
			Name:  "RABBITMQ_PORT",
			Value: environmentrequest.Spec.Config.EiffelMessageBus.Port,
		},
		{
			Name:  "RABBITMQ_SSL",
			Value: environmentrequest.Spec.Config.EiffelMessageBus.SSL,
		},
		{
			Name:  "RABBITMQ_EXCHANGE",
			Value: environmentrequest.Spec.Config.EiffelMessageBus.Exchange,
		},
		{
			Name:  "RABBITMQ_USERNAME",
			Value: environmentrequest.Spec.Config.EiffelMessageBus.Username,
		},
		{
			Name:  "RABBITMQ_PASSWORD",
			Value: string(eiffelRabbitMQPassword),
		},

		// ETOS Message Bus variables
		{
			Name:  "ETOS_RABBITMQ_HOST",
			Value: environmentrequest.Spec.Config.EtosMessageBus.Host,
		},
		{
			Name:  "ETOS_RABBITMQ_VHOST",
			Value: environmentrequest.Spec.Config.EtosMessageBus.Vhost,
		},
		{
			Name:  "ETOS_RABBITMQ_PORT",
			Value: environmentrequest.Spec.Config.EtosMessageBus.Port,
		},
		{
			Name:  "ETOS_RABBITMQ_SSL",
			Value: environmentrequest.Spec.Config.EtosMessageBus.SSL,
		},
		{
			Name:  "ETOS_RABBITMQ_EXCHANGE",
			Value: environmentrequest.Spec.Config.EtosMessageBus.Exchange,
		},
		{
			Name:  "ETOS_RABBITMQ_USERNAME",
			Value: environmentrequest.Spec.Config.EtosMessageBus.Username,
		},
		{
			Name:  "ETOS_RABBITMQ_PASSWORD",
			Value: string(etosRabbitMQPassword),
		},
	}
	return envList, nil
}

// reconcileDeletion checks for active environments and deletes them, causing them to clean up, and then, when all environments
// are deleted, this function will remove the finalizer on the environmentrequest and the environmentrequest will be removed.
func (r EnvironmentRequestReconciler) reconcileDeletion(ctx context.Context, environmentrequest *etosv1alpha1.EnvironmentRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	statusReady := meta.FindStatusCondition(environmentrequest.Status.Conditions, StatusReady)
	if statusReady == nil {
		statusReady = &metav1.Condition{Type: StatusReady, Status: metav1.ConditionFalse, Reason: "Unknown"}
	}
	logger.Info("Setting status message", "status", metav1.Condition{Type: statusReady.Type, Status: statusReady.Status, Reason: statusReady.Reason, Message: "Releasing environment"})
	if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: statusReady.Type, Status: statusReady.Status, Reason: statusReady.Reason, Message: "Releasing environment"}) {
		if err := r.Status().Update(ctx, environmentrequest); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	var environments etosv1alpha1.EnvironmentList
	if err := r.List(ctx, &environments, client.InNamespace(environmentrequest.Namespace), client.MatchingLabels{"etos.eiffel-community.github.io/id": environmentrequest.Spec.Identifier}); err != nil {
		logger.Error(err, fmt.Sprintf("could not list pods for job %s", environmentrequest.Name))
		return ctrl.Result{Requeue: true}, err
	}
	var allErr error
	var err error
	for _, environment := range environments.Items {
		// Environment is not deleted.
		if environment.ObjectMeta.DeletionTimestamp.IsZero() {
			if err = r.Delete(ctx, &environment); err != nil {
				logger.Error(err, "failed to delete environment", "environment", environment)
				allErr = errors.Join(allErr, err)
			}
		}
	}
	if allErr != nil {
		return ctrl.Result{Requeue: true}, nil
	}
	if len(environments.Items) != 0 {
		logger.Info("Waiting for Environments to get deleted", "stillAlive", len(environments.Items))
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

// environmentProviderJob is the job definition for an etos environment provider.
func (r EnvironmentRequestReconciler) environmentProviderJob(ctx context.Context, environmentrequest *etosv1alpha1.EnvironmentRequest) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)
	ttl := int32(300)
	grace := int64(30)
	backoff := int32(0)

	envVarList, err := r.envVarListFrom(ctx, environmentrequest)
	if err != nil {
		logger.Error(err, "Failed to create environment variable list for environment provider")
		return nil, err
	}

	labels := map[string]string{
		"etos.eiffel-community.github.io/id": environmentrequest.Spec.Identifier, // TODO: omitempty
		"app.kubernetes.io/name":             "environment-provider",
		"app.kubernetes.io/part-of":          "etos",
	}
	if cluster := environmentrequest.Labels["etos.eiffel-community.github.io/cluster"]; cluster != "" {
		labels["etos.eiffel-community.github.io/cluster"] = cluster
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
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
					ServiceAccountName:            environmentrequest.Spec.ServiceAccountName,
					TerminationGracePeriodSeconds: &grace,
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
							Env: envVarList,
						},
					},
				},
			},
		},
	}, nil
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
