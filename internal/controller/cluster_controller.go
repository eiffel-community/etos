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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/config"
	"github.com/eiffel-community/etos/internal/controller/status"
	"github.com/eiffel-community/etos/internal/etos"
	"github.com/eiffel-community/etos/internal/extras"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config config.Config
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=environments,verbs=get;list;watch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=testruns,verbs=get;list;watch;create;delete;deletecollection
// +kubebuilder:rbac:groups=*,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=*,resources=jobs,verbs=get;list;watch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger = logger.WithValues("namespace", req.Namespace, "name", req.Name)

	// TODO: Logstash

	cluster := &etosv1alpha1.Cluster{}
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("cluster not found. ignoring object")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get cluster")
		return ctrl.Result{}, err
	}

	eiffelbus := extras.NewRabbitMQDeployment(cluster.Spec.MessageBus.EiffelMessageBus, r.Scheme, r.Client)
	if err := eiffelbus.Reconcile(ctx, cluster); err != nil {
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Error reconciling the Eiffel event bus")
		return r.handleReconcileError(ctx, cluster, err)
	}

	etosbus := extras.NewMessageBusDeployment(cluster.Spec.MessageBus.ETOSMessageBus, r.Scheme, r.Client)
	if err := etosbus.Reconcile(ctx, cluster); err != nil {
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Error reconciling the ETOS message bus")
		return r.handleReconcileError(ctx, cluster, err)
	}

	mongodb := extras.NewMongoDBDeployment(cluster.Spec.EventRepository.Database, r.Scheme, r.Client)
	if err := mongodb.Reconcile(ctx, cluster); err != nil {
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Error reconciling the Eiffel event bus database")
		return r.handleReconcileError(ctx, cluster, err)
	}

	eventrepository := extras.NewEventRepositoryDeployment(&cluster.Spec.EventRepository, r.Scheme, r.Client, mongodb, eiffelbus.SecretName, r.Config)
	if err := eventrepository.Reconcile(ctx, cluster); err != nil {
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Error reconciling the Eiffel event repository")
		return r.handleReconcileError(ctx, cluster, err)
	}

	etcd := etos.NewETCDDeployment(&cluster.Spec.Database, r.Scheme, r.Client)
	if err := etcd.Reconcile(ctx, cluster); err != nil {
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Error reconciling the ETOS database")
		return r.handleReconcileError(ctx, cluster, err)
	}

	etosDeployment := etos.NewETOSDeployment(cluster.Spec.ETOS, r.Scheme, r.Client, eiffelbus.SecretName, etosbus.SecretName, r.Config)
	if err := etosDeployment.Reconcile(ctx, cluster); err != nil {
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Error reconciling ETOS")
		return r.handleReconcileError(ctx, cluster, err)
	}

	return r.update(ctx, cluster, metav1.ConditionTrue, status.ReasonCompleted, "Cluster is up and running")
}

// handleReconcileError inspects the error from a sub-reconciler.
// If it is a NotReadyError (pods not yet ready), set Ready=False with Reason=Pending.
// Otherwise, set Ready=False with Reason=Failed.
func (r *ClusterReconciler) handleReconcileError(ctx context.Context, cluster *etosv1alpha1.Cluster, err error) (ctrl.Result, error) {
	var notReady *status.NotReadyError
	if errors.As(err, &notReady) {
		return r.update(ctx, cluster, metav1.ConditionFalse, status.ReasonPending, notReady.Error())
	}
	return r.update(ctx, cluster, metav1.ConditionFalse, status.ReasonFailed, err.Error())
}

// update will set the Ready status condition and update the status of the ETOS cluster.
// If the update fails due to conflict the reconciliation will requeue after one second.
func (r *ClusterReconciler) update(ctx context.Context, cluster *etosv1alpha1.Cluster, clusterStatus metav1.ConditionStatus, reason string, message string) (ctrl.Result, error) {
	if meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{Type: status.StatusReady, Status: clusterStatus, Reason: reason, Message: message}) {
		if err := r.Status().Update(ctx, cluster); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{RequeueAfter: time.Second}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha1.Cluster{}).
		Named("cluster").
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}
