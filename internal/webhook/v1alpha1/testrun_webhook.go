/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var testrunlog = logf.Log.WithName("testrun-resource")

// SetupTestRunWebhookWithManager registers the webhook for TestRun in the manager.
func SetupTestRunWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &etosv1alpha1.TestRun{}).
		WithValidator(&TestRunCustomValidator{}).
		WithDefaulter(&TestRunCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha1-testrun,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=testruns,verbs=create;update,versions=v1alpha1,name=mtestrun-v1alpha1.kb.io,admissionReviewVersions=v1

// TestRunCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind TestRun when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type TestRunCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind TestRun.
func (d *TestRunCustomDefaulter) Default(_ context.Context, obj *etosv1alpha1.TestRun) error {
	testrunlog.Info("Defaulting for TestRun", "name", obj.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-etos-eiffel-community-github-io-v1alpha1-testrun,mutating=false,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=testruns,verbs=create;update,versions=v1alpha1,name=vtestrun-v1alpha1.kb.io,admissionReviewVersions=v1

// TestRunCustomValidator struct is responsible for validating the TestRun resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type TestRunCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type TestRun.
func (v *TestRunCustomValidator) ValidateCreate(_ context.Context, obj *etosv1alpha1.TestRun) (admission.Warnings, error) {
	testrunlog.Info("Validation for TestRun upon creation", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type TestRun.
func (v *TestRunCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *etosv1alpha1.TestRun) (admission.Warnings, error) {
	testrunlog.Info("Validation for TestRun upon update", "name", newObj.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type TestRun.
func (v *TestRunCustomValidator) ValidateDelete(_ context.Context, obj *etosv1alpha1.TestRun) (admission.Warnings, error) {
	testrunlog.Info("Validation for TestRun upon deletion", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
