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
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// ProviderReconciler reconciles a Provider object
type ProviderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etos.eiffel-community.github.io,resources=providers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *ProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	provider := &etosv1alpha1.Provider{}
	err := r.Get(ctx, req.NamespacedName, provider)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("provider not found. ignoring object")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get provider")
		return ctrl.Result{}, err
	}

	interval := time.Duration(provider.Spec.Healthcheck.IntervalSeconds) * time.Second
	lastHealthCheckTime := metav1.NewTime(time.Now())

	if provider.Status.LastHealthCheckTime == nil {
		// run healthcheck on first reconciliation and update status after that
		provider.Status.LastHealthCheckTime = &lastHealthCheckTime
	} else {
		next := provider.Status.LastHealthCheckTime.Time.Add(interval)
		if time.Until(next) > 0 {
			// postpone healthcheck if it is too early to do it now
			return ctrl.Result{RequeueAfter: time.Until(next)}, nil
		}
	}

	// We don't check the availability of JSONTas as it is not yet running as a service we can check.
	if provider.Spec.JSONTas == nil {
		logger.V(2).Info("Healthcheck", "endpoint", fmt.Sprintf("%s/%s", provider.Spec.Host, provider.Spec.Healthcheck.Endpoint))
		resp, err := http.Get(fmt.Sprintf("%s/%s", provider.Spec.Host, provider.Spec.Healthcheck.Endpoint))
		if err != nil {
			meta.SetStatusCondition(&provider.Status.Conditions, metav1.Condition{Type: StatusAvailable, Status: metav1.ConditionFalse, Reason: "Error", Message: "Could not communicate with host"})
			if err = r.Status().Update(ctx, provider); err != nil {
				logger.Error(err, "failed to update provider status")
				return ctrl.Result{}, err
			}
			logger.Info("Provider did not respond", "provider", req.NamespacedName)
			return ctrl.Result{RequeueAfter: interval}, nil
		}
		if resp.StatusCode != 204 {
			meta.SetStatusCondition(&provider.Status.Conditions, metav1.Condition{Type: StatusAvailable, Status: metav1.ConditionFalse, Reason: "Error", Message: fmt.Sprintf("Wrong status code (%d) from health check endpoint", resp.StatusCode)})
			if err = r.Status().Update(ctx, provider); err != nil {
				logger.Error(err, "failed to update provider status")
				return ctrl.Result{}, err
			}
			logger.Info("Provider responded with a bad status code", "provider", req.NamespacedName, "status", resp.StatusCode)
			return ctrl.Result{RequeueAfter: interval}, nil
		}
	}
	meta.SetStatusCondition(&provider.Status.Conditions, metav1.Condition{Type: StatusAvailable, Status: metav1.ConditionTrue, Reason: "OK", Message: "Provider is up and running"})
	if err = r.Status().Update(ctx, provider); err != nil {
		logger.Error(err, "failed to update provider status")
		return ctrl.Result{}, err
	}
	logger.V(2).Info("Provider is available", "provider", req.NamespacedName)
	return ctrl.Result{RequeueAfter: interval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha1.Provider{}).
		Complete(r)
}
