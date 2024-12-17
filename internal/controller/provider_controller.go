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
	"sync"
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

// ProviderReconciler reconciles a Provider object
type ProviderReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	healthCheckerMap   map[string]*ProviderHealthChecker
	healthCheckerMapMu sync.Mutex
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

	if r.healthCheckerMap == nil {
		r.healthCheckerMap = make(map[string]*ProviderHealthChecker)
	}

	providerList := &etosv1alpha1.ProviderList{}
	err := r.List(ctx, providerList)
	if err != nil {
		logger.Error(err, "failed to list providers")
		return ctrl.Result{}, err
	}

	r.healthCheckerMapMu.Lock()
	// Ensure there is a ProviderHealthChecker for each provider in the list
	for _, provider := range providerList.Items {
		if _, exists := r.healthCheckerMap[provider.Name]; !exists {
			// create a health checker instance if the provider doesn't have one yet
			hc := ProviderHealthChecker{
				name:       req.NamespacedName,
				provider:   &provider,
				reconciler: r,
				ticker:     time.NewTicker(time.Duration(provider.Spec.Healthcheck.IntervalSeconds) * time.Second),
			}
			r.healthCheckerMap[provider.Name] = &hc
			go hc.Start(ctx)
		} else {
			// relaunch health checker if it isn't running
			hc := r.healthCheckerMap[provider.Name]
			if !hc.running {
				go hc.Start(ctx)
			}
		}
	}

	// Ensure r.healthCheckerMap does not have any health checkers for non-existent providers
	for providerName, hc := range r.healthCheckerMap {
		providerFound := false
		for _, provider := range providerList.Items {
			if provider.Name == providerName {
				providerFound = true
				break
			}
			if !providerFound {
				if hc.running {
					hc.Stop()
				}
				delete(r.healthCheckerMap, providerName)
			}
		}
	}
	r.healthCheckerMapMu.Unlock()

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etosv1alpha1.Provider{}).
		Complete(r)
}

// ProviderHealthChecker is a background worker updating providers' status
type ProviderHealthChecker struct {
	name       types.NamespacedName
	provider   *etosv1alpha1.Provider
	reconciler *ProviderReconciler

	running  bool
	mu       sync.Mutex
	stopChan chan struct{}
	ticker   *time.Ticker
}

// Start launches a provider health checker instance
func (phc *ProviderHealthChecker) Start(ctx context.Context) {
	logger := log.FromContext(ctx)
	for {
		select {
		case <-phc.ticker.C:
			logger.V(2).Info("Performing provider health checks")
			if err := phc.Check(ctx); err != nil {
				logger.V(2).Info("Error occured during provider health check")
			}
		case <-phc.stopChan:
			logger.V(2).Info("Stopping provider health checker")
			phc.ticker.Stop()
			return
		case <-ctx.Done():
			phc.mu.Lock()
			phc.running = false
			phc.mu.Unlock()
			phc.ticker.Stop()
			return
		}
	}
}

// Stop terminates a provider health checker instance
func (phc *ProviderHealthChecker) Stop() {
	close(phc.stopChan)
}

// Check performs a health check of the provider
func (phc *ProviderHealthChecker) Check(ctx context.Context) error {
	logger := log.FromContext(ctx)

	provider := &etosv1alpha1.Provider{}
	err := phc.reconciler.Get(ctx, phc.name, provider)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("provider not found. ignoring object")
			return nil
		}
		logger.Error(err, "failed to get provider")
		return err
	}

	logger.V(2).Info("Checking availability of provider", "provider", phc.provider.Name)
	// We don't check the availability of JSONTas as it is not yet running as a service we can check.
	if provider.Spec.JSONTas == nil {
		resp, err := http.Get(fmt.Sprintf("%s/%s", phc.provider.Spec.Host, phc.provider.Spec.Healthcheck.Endpoint))
		if err != nil {
			meta.SetStatusCondition(&phc.provider.Status.Conditions, metav1.Condition{Type: StatusAvailable, Status: metav1.ConditionFalse, Reason: "Error", Message: "Could not communicate with host"})
			if err = phc.reconciler.Status().Update(ctx, provider); err != nil {
				logger.Error(err, "failed to update provider status")
			}
			logger.Info("Provider did not respond", "provider", provider.Name)
		}
		if resp.StatusCode != 204 {
			meta.SetStatusCondition(&provider.Status.Conditions, metav1.Condition{Type: StatusAvailable, Status: metav1.ConditionFalse, Reason: "Error", Message: fmt.Sprintf("Wrong status code (%d) from health check endpoint", resp.StatusCode)})
			if err = phc.reconciler.Status().Update(ctx, provider); err != nil {
				logger.Error(err, "failed to update provider status")
			}
			logger.Info("Provider responded with a bad status code", "provider", provider.Name, "status", resp.StatusCode)
		}
	}
	meta.SetStatusCondition(&provider.Status.Conditions, metav1.Condition{Type: StatusAvailable, Status: metav1.ConditionTrue, Reason: "OK", Message: "Provider is up and running"})
	if err = phc.reconciler.Status().Update(ctx, provider); err != nil {
		logger.Error(err, "failed to update provider status")
	}
	logger.V(2).Info("Provider is available", "provider", provider.Name)
	return nil
}
