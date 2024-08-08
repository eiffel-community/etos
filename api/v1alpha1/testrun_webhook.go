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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var testrunlog = logf.Log.WithName("testrun-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *TestRun) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha1-testrun,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=testruns,verbs=create;update,versions=v1alpha1,name=mtestrun.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &TestRun{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *TestRun) Default() {
	testrunlog.Info("default", "name", r.Name)

	if r.Spec.ID == "" {
		r.Spec.ID = string(uuid.NewUUID())
	}
}
