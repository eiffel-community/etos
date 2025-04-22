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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/controller/jobs"
	"github.com/eiffel-community/etos/internal/controller/status"
)

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
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *EnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger = logger.WithValues("namespace", req.Namespace, "name", req.Name)
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
					return ctrl.Result{}, client.IgnoreNotFound(err)
				}
			}
		}
		return ctrl.Result{}, nil
	}

	if err := r.reconcile(ctx, environment); err != nil {
		if apierrors.IsConflict(err) {
			logger.Error(err, "Environment reconciliation conflict, requeueing")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Environment reconciliation failed")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcile an environment resource to its desired state.
func (r *EnvironmentReconciler) reconcile(ctx context.Context, environment *etosv1alpha1.Environment) error {
	// Set initial statuses if not set.
	if active := meta.FindStatusCondition(environment.Status.Conditions, status.StatusActive); active == nil {
		meta.SetStatusCondition(&environment.Status.Conditions,
			metav1.Condition{
				Status:  metav1.ConditionFalse,
				Type:    status.StatusActive,
				Reason:  status.ReasonPending,
				Message: "Waiting environment to become ready",
			})
		return r.Status().Update(ctx, environment)
	}
	if environment.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(environment, releaseFinalizer) {
			controllerutil.AddFinalizer(environment, releaseFinalizer)
			return r.Update(ctx, environment)
		}
	}

	if isStatusReason(environment.Status.Conditions, status.StatusActive, status.ReasonPending) {
		return r.reconcileEnvironment(ctx, environment)
	}

	conditions := &environment.Status.Conditions
	jobManager := jobs.NewJob(r.Client, EnvironmentOwnerKey, environment.GetName(), environment.GetNamespace())

	jobStatus, err := jobManager.Status(ctx)
	if err != nil {
		return err
	}
	switch jobStatus {
	case jobs.StatusFailed:
		result := jobManager.Result(ctx, environment.Name)
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusActive,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonFailed,
				Message: result.Description,
			}) {
			return r.Status().Update(ctx, environment)
		}
	case jobs.StatusSuccessful:
		result := jobManager.Result(ctx, environment.Name)
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
		environmentCondition := meta.FindStatusCondition(environment.Status.Conditions, status.StatusActive)
		environment.Status.CompletionTime = &environmentCondition.LastTransitionTime
		if meta.SetStatusCondition(conditions, condition) {
			return errors.Join(r.Status().Update(ctx, environment), jobManager.Delete(ctx))
		}
	case jobs.StatusActive:
		if meta.SetStatusCondition(conditions,
			metav1.Condition{
				Type:    status.StatusActive,
				Status:  metav1.ConditionFalse,
				Reason:  status.ReasonActive,
				Message: "Release job is running",
			}) {
			return r.Status().Update(ctx, environment)
		}
	default:
		// Since this is a release job, we don't want to release if we are not deleting.
		if environment.GetDeletionTimestamp().IsZero() {
			return nil
		}
		if err := jobManager.Create(ctx, environment, r.releaseJob); err != nil {
			// When we create a job the job gets a unique name. If there's an error for that unique name the error
			// message in Condition.Message is also unique meaning we will update the StatusCondition every time,
			// causing a nasty reconciliation loop (when the environment gets updated a new reconciliation starts).
			// We mitigate this by checking that StatusReason is not already Failed.
			if !isStatusReason(*conditions, status.StatusActive, status.ReasonFailed) && meta.SetStatusCondition(conditions,
				metav1.Condition{
					Type:    status.StatusActive,
					Status:  metav1.ConditionFalse,
					Reason:  status.ReasonFailed,
					Message: err.Error(),
				}) {
				return r.Status().Update(ctx, environment)
			}
			return err
		}
	}
	return nil
}

// reconcileEnvironment sets the active status on an environment.
func (r *EnvironmentReconciler) reconcileEnvironment(ctx context.Context, environment *etosv1alpha1.Environment) error {
	logger := logf.FromContext(ctx)
	// TODO: Provider information does not exist in a deterministic way in the Environment resource
	// so either we need to find the EnvironmentRequest or the Environment resource needs an update.
	// if err := checkProviders(ctx, r, environment.Namespace, environment.Spec.Providers); err != nil {
	// 	return err
	// }
	if meta.SetStatusCondition(&environment.Status.Conditions,
		metav1.Condition{
			Status:  metav1.ConditionTrue,
			Type:    status.StatusActive,
			Reason:  status.ReasonCompleted,
			Message: "Actively being used",
		}) {
		logger.Info("Environment is active and ready for use")
		return r.Status().Update(ctx, environment)
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
func (r EnvironmentReconciler) releaseJob(ctx context.Context, obj client.Object) (*batchv1.Job, error) {
	logger := logf.FromContext(ctx)
	ttl := int32(300)
	grace := int64(30)
	backoff := int32(0)

	environment, ok := obj.(*etosv1alpha1.Environment)
	if !ok {
		return nil, errors.New("object received from job manager is not an Environment")
	}
	environmentRequest, err := r.environmentRequest(ctx, environment)
	if err != nil {
		return nil, err
	}
	clusterName := environment.Labels["etos.eiffel-community.github.io/cluster"]
	var cluster *etosv1alpha1.Cluster
	if clusterName != "" {
		cluster = &etosv1alpha1.Cluster{}
		if err := r.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: environment.Namespace}, cluster); err != nil {
			logger.Info("Failed to get cluster resource!")
			return nil, err
		}
	}

	databaseHost := "etcd-client"
	if cluster != nil {
		if cluster.Spec.Database.Deploy {
			databaseHost = fmt.Sprintf("%s-etcd-client", cluster.Name)
		} else {
			databaseHost = cluster.Spec.Database.Etcd.Host
		}
	}
	traceparent, ok := environmentRequest.Annotations["etos.eiffel-community.github.io/traceparent"]
	if !ok {
		traceparent = ""
	}

	envList := []corev1.EnvVar{
		{
			Name:  "REQUEST",
			Value: environmentRequest.Name,
		},
		{
			Name:  "ENVIRONMENT",
			Value: environment.Name,
		},
		{
			Name:  "ETOS_ETCD_HOST",
			Value: databaseHost,
		},
		{
			Name:  "OTEL_CONTEXT",
			Value: traceparent,
		},
	}
	if cluster != nil && cluster.Spec.OpenTelemetry.Enabled {
		envList = append(envList, corev1.EnvVar{
			Name:  "OTEL_EXPORTER_OTLP_ENDPOINT",
			Value: cluster.Spec.OpenTelemetry.Endpoint,
		})
		envList = append(envList, corev1.EnvVar{
			Name:  "OTEL_EXPORTER_OTLP_INSECURE",
			Value: cluster.Spec.OpenTelemetry.Insecure,
		})
	}

	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"etos.eiffel-community.github.io/id":        environment.Labels["etos.eiffel-community.github.io/id"],
				"etos.eiffel-community.github.io/sub-suite": environment.Name,
				"etos.eiffel-community.github.io/cluster":   clusterName,
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
					Labels: map[string]string{
						"etos.eiffel-community.github.io/id":        environment.Labels["etos.eiffel-community.github.io/id"],
						"etos.eiffel-community.github.io/sub-suite": environment.Name,
						"etos.eiffel-community.github.io/cluster":   clusterName,
						"app.kubernetes.io/name":                    "environment-releaser",
						"app.kubernetes.io/part-of":                 "etos",
					},
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &grace,
					ServiceAccountName:            environmentRequest.Spec.ServiceAccountName,
					RestartPolicy:                 "Never",
					Containers: []corev1.Container{
						{
							Name:            environment.Name,
							Image:           environmentRequest.Spec.Image.Image,
							ImagePullPolicy: environmentRequest.Spec.ImagePullPolicy,
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
							Command: []string{"python", "-u", "-m", "environment_provider.environment"},
							Args:    []string{environment.Name},
							Env:     envList,
						},
					},
				},
			},
		},
	}
	return jobSpec, ctrl.SetControllerReference(environment, jobSpec, r.Scheme)
}

// registerOwnerIndexForJob will set an index of the jobs that an environment owns.
func (r *EnvironmentReconciler) registerOwnerIndexForJob(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, EnvironmentOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != APIGroupVersionString || owner.Kind != "Environment" {
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
		Named("environment").
		Owns(&batchv1.Job{}).
		Complete(r)
}
