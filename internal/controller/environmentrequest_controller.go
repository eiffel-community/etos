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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

var checkInterval = 30 * time.Second

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

	// Check if providers are available.
	if err := r.checkProviders(ctx, environmentrequest); err != nil {
		if meta.SetStatusCondition(&environmentrequest.Status.Conditions, metav1.Condition{Type: StatusReady, Status: metav1.ConditionFalse, Reason: "Ready", Message: err.Error()}) {
			if err := r.Status().Update(ctx, environmentrequest); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				logger.Error(err, "error updating status")
				return ctrl.Result{}, err
			}
		}
		logger.Error(err, "providers are not ready")
		return ctrl.Result{RequeueAfter: checkInterval}, nil
	}

	return ctrl.Result{}, nil
}

// checkProviders checks if all providers for this environment are available.
func (r EnvironmentRequestReconciler) checkProviders(ctx context.Context, request *etosv1alpha1.EnvironmentRequest) error {
	err := r.checkProvider(ctx, request.Spec.IUTProvider, request.Namespace, &etosv1alpha1.Provider{})
	if err != nil {
		return err
	}
	err = r.checkProvider(ctx, request.Spec.ExecutionSpaceProvider, request.Namespace, &etosv1alpha1.Provider{})
	if err != nil {
		return err
	}
	err = r.checkProvider(ctx, request.Spec.LogAreaProvider, request.Namespace, &etosv1alpha1.Provider{})
	if err != nil {
		return err
	}
	return nil
}

// checkProvider checks if the provider condition 'Available' is set to True.
func (r EnvironmentRequestReconciler) checkProvider(ctx context.Context, name string, namespace string, provider *etosv1alpha1.Provider) error {
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, provider)
	if err != nil {
		return err
	}
	if meta.IsStatusConditionPresentAndEqual(provider.Status.Conditions, StatusAvailable, metav1.ConditionTrue) {
		return nil
	}
	return fmt.Errorf("Provider '%s' does not have a status field", name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *EnvironmentRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha1.EnvironmentRequest{}).
		Owns(&etosv1alpha1.Provider{}).
		Complete(r)
}
