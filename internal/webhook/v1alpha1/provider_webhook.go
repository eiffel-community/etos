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
var providerlog = logf.Log.WithName("provider-resource")

// SetupProviderWebhookWithManager registers the webhook for Provider in the manager.
func SetupProviderWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &etosv1alpha1.Provider{}).
		WithValidator(&ProviderCustomValidator{}).
		WithDefaulter(&ProviderCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha1-provider,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=providers,verbs=create;update,versions=v1alpha1,name=mprovider-v1alpha1.kb.io,admissionReviewVersions=v1

// ProviderCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Provider when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ProviderCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Provider.
func (d *ProviderCustomDefaulter) Default(_ context.Context, obj *etosv1alpha1.Provider) error {
	providerlog.Info("Defaulting for Provider", "name", obj.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-etos-eiffel-community-github-io-v1alpha1-provider,mutating=false,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=providers,verbs=create;update,versions=v1alpha1,name=vprovider-v1alpha1.kb.io,admissionReviewVersions=v1

// ProviderCustomValidator struct is responsible for validating the Provider resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ProviderCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Provider.
func (v *ProviderCustomValidator) ValidateCreate(_ context.Context, obj *etosv1alpha1.Provider) (admission.Warnings, error) {
	providerlog.Info("Validation for Provider upon creation", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Provider.
func (v *ProviderCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *etosv1alpha1.Provider) (admission.Warnings, error) {
	providerlog.Info("Validation for Provider upon update", "name", newObj.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Provider.
func (v *ProviderCustomValidator) ValidateDelete(_ context.Context, obj *etosv1alpha1.Provider) (admission.Warnings, error) {
	providerlog.Info("Validation for Provider upon deletion", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
