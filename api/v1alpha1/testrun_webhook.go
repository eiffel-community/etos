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
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var testrunlog = logf.Log.WithName("testrun-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *TestRun) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if cli == nil {
		cli = mgr.GetClient()
	}
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha1-testrun,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=testruns,verbs=create;update,versions=v1alpha1,name=mtestrun.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &TestRun{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *TestRun) Default() {
	testrunlog.Info("default", "name", r.Name, "namespace", r.Namespace)

	if r.Spec.ID == "" {
		r.Spec.ID = string(uuid.NewUUID())
	}
	clusters := &ClusterList{}
	if r.Spec.Cluster == "" {
		if err := cli.List(context.TODO(), clusters, client.InNamespace(r.Namespace)); err != nil {
			testrunlog.Error(err, "Failed to get clusters in namespace")
		}
	}
	var cluster *Cluster
	if len(clusters.Items) == 1 {
		cluster = &clusters.Items[0]
		r.Spec.Cluster = cluster.Name
	}
	if r.Spec.SuiteRunner == nil && cluster != nil {
		r.Spec.SuiteRunner = &SuiteRunner{&cluster.Spec.ETOS.SuiteRunner.Image}
	}
	if r.Spec.LogListener == nil && cluster != nil {
		r.Spec.LogListener = &LogListener{&cluster.Spec.ETOS.SuiteRunner.LogListener}
	}
	if r.Spec.EnvironmentProvider == nil && cluster != nil {
		r.Spec.EnvironmentProvider = &EnvironmentProvider{&cluster.Spec.ETOS.EnvironmentProvider.Image}
	}
	if r.Spec.TestRunner == nil && cluster != nil {
		r.Spec.TestRunner = &TestRunner{Version: cluster.Spec.ETOS.TestRunner.Version}
	}

	addLabel := true
	if r.ObjectMeta.Labels != nil {
		for key := range r.ObjectMeta.GetLabels() {
			if key == "etos.eiffel-community.github.io/id" {
				addLabel = false
			}
		}
	} else {
		r.ObjectMeta.Labels = map[string]string{}
	}
	if addLabel {
		r.ObjectMeta.Labels["etos.eiffel-community.github.io/id"] = r.Spec.ID
	}
}

// +kubebuilder:webhook:path=/validate-etos-eiffel-community-github-io-v1alpha1-testrun,mutating=false,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=testruns,verbs=create;update,versions=v1alpha1,name=mtestrun.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &TestRun{}

// validate that the required parameters are set. This validation is done here instead of directly in the struct since
// we do mutate the input in the Default function.
func (r *TestRun) validate() error {
	var allErrs field.ErrorList
	if r.Spec.Cluster == "" {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("cluster"),
			r.Spec.Cluster,
			"Cluster is missing, either no cluster exists in namespace or too many to choose from",
		))
	}

	if r.Spec.SuiteRunner == nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("suiteRunner"),
			r.Spec.SuiteRunner,
			"SuiteRunner image information is missing, maybe because cluster is missing?",
		))
	}

	if r.Spec.LogListener == nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("logListener"),
			r.Spec.LogListener,
			"LogListener image information is missing, maybe because cluster is missing?",
		))
	}

	if r.Spec.EnvironmentProvider == nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("environmentProvider"),
			r.Spec.EnvironmentProvider,
			"EnvironmentProvider image information is missing, maybe because cluster is missing?",
		))
	}

	if r.Spec.TestRunner == nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("testRunner"),
			r.Spec.TestRunner,
			"TestRunner version information is missing, maybe because cluster is missing?",
		))
	}

	groupVersionKind := r.GroupVersionKind()
	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: groupVersionKind.Group, Kind: groupVersionKind.Kind},
			r.Name, allErrs,
		)
	}
	return nil
}

// ValidateCreate validates the creation of a TestRun.
func (r *TestRun) ValidateCreate() (admission.Warnings, error) {
	testrunlog.Info("validate create", "name", r.Name)
	return nil, r.validate()
}

// ValidateUpdate validates the updates of a TestRun.
func (r *TestRun) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	testrunlog.Info("validate update", "name", r.Name)
	return nil, r.validate()
}

// ValidateDelete validates the deletion of a TestRun.
func (r *TestRun) ValidateDelete() (admission.Warnings, error) {
	testrunlog.Info("validate delete", "name", r.Name)
	return nil, nil
}
