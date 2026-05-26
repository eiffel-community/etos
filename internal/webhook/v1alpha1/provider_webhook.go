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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

var providerlog = logf.Log.WithName("provider-resource")

// SetupProviderWebhookWithManager registers the webhook for Provider in the manager.
func SetupProviderWebhookWithManager(mgr ctrl.Manager) error {
	if cli == nil {
		cli = mgr.GetClient()
	}
	return ctrl.NewWebhookManagedBy(mgr, &etosv1alpha1.Provider{}).
		WithValidator(&ProviderCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-etos-eiffel-community-github-io-v1alpha1-provider,mutating=false,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=providers,verbs=create;update,versions=v1alpha1,name=vprovider-v1alpha1.kb.io,admissionReviewVersions=v1

// ProviderCustomValidator struct is responsible for validating the Provider resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ProviderCustomValidator struct{}

// ValidateCreate validates the creation of a Provider.
func (v *ProviderCustomValidator) ValidateCreate(ctx context.Context, provider *etosv1alpha1.Provider) (admission.Warnings, error) {
	providerlog.Info("Validation for Provider upon creation", "name", provider.GetName())
	return nil, v.validateDefaultLabel(ctx, provider)
}

// ValidateUpdate validates the update of a Provider.
func (v *ProviderCustomValidator) ValidateUpdate(ctx context.Context, _, provider *etosv1alpha1.Provider) (admission.Warnings, error) {
	providerlog.Info("Validation for Provider upon update", "name", provider.GetName())
	return nil, v.validateDefaultLabel(ctx, provider)
}

// ValidateDelete validates the deletion of a Provider.
func (v *ProviderCustomValidator) ValidateDelete(_ context.Context, _ *etosv1alpha1.Provider) (admission.Warnings, error) {
	return nil, nil
}

// validateDefaultLabel ensures that at most one provider per type is labeled as default
// within the same namespace.
func (v *ProviderCustomValidator) validateDefaultLabel(ctx context.Context, provider *etosv1alpha1.Provider) error {
	// Only validate if this provider is being marked as default.
	if provider.Labels == nil || provider.Labels["etos.eiffel-community.github.io/default"] != "true" {
		return nil
	}

	// List all default providers of the same type in the namespace.
	providers := &etosv1alpha1.ProviderList{}
	if err := cli.List(ctx, providers,
		client.InNamespace(provider.Namespace),
		client.MatchingLabels{"etos.eiffel-community.github.io/default": "true"},
		client.MatchingFields{".spec.type": provider.Spec.Type},
	); err != nil {
		return fmt.Errorf("failed to list providers: %w", err)
	}

	for _, p := range providers.Items {
		// Skip self (relevant for updates).
		if p.Name == provider.Name && p.Namespace == provider.Namespace {
			continue
		}
		var allErrs field.ErrorList
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("metadata").Child("labels").Key("etos.eiffel-community.github.io/default"),
			"true",
			fmt.Sprintf("a default provider of type %q already exists in this namespace: %q", provider.Spec.Type, p.Name),
		))
		groupVersionKind := provider.GroupVersionKind()
		return apierrors.NewInvalid(
			schema.GroupKind{Group: groupVersionKind.Group, Kind: groupVersionKind.Kind},
			provider.Name, allErrs,
		)
	}
	return nil
}
