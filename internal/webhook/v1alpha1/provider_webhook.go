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
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var (
	providerlog = logf.Log.WithName("provider-resource")
	cli         client.Client
)

// SetupProviderWebhookWithManager registers the webhook for Provider in the manager.
func SetupProviderWebhookWithManager(mgr ctrl.Manager) error {
	if cli == nil {
		cli = mgr.GetClient()
	}
	return ctrl.NewWebhookManagedBy(mgr).For(&etosv1alpha1.Provider{}).
		WithValidator(&ProviderCustomValidator{}).
		WithDefaulter(&ProviderCustomDefaulter{}).
		Complete()
}

// getFromSecretKeySelector returns the value of a key in a secret.
func getFromSecretKeySelector(ctx context.Context, client client.Client, secretKeySelector *corev1.SecretKeySelector, namespace string) ([]byte, error) {
	name := types.NamespacedName{Name: secretKeySelector.Name, Namespace: namespace}
	obj := &corev1.Secret{}

	providerlog.Info("Getting jsontas from a secret", "name", secretKeySelector.Name, "key", secretKeySelector.Key)

	// Retrying to make sure that the secret has been properly generated before a provider is applied.
	// There is a race where, for example, a provider and a custom secret resource (such as a SealedSecret)
	// are created at the same time and the secret does not get generated in time.
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		err := client.Get(ctx, name, obj)
		if err != nil {
			providerlog.Error(err, "retry")
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	d, ok := obj.Data[secretKeySelector.Key]
	if !ok {
		return nil, fmt.Errorf("%s does not exist in secret %s/%s", secretKeySelector.Key, secretKeySelector.Name, namespace)
	}
	return d, nil
}

// getFromConfigMapKeySelector returns the value of a key in a configmap.
func getFromConfigMapKeySelector(ctx context.Context, client client.Client, configMapKeySelector *corev1.ConfigMapKeySelector, namespace string) ([]byte, error) {
	name := types.NamespacedName{Name: configMapKeySelector.Name, Namespace: namespace}
	obj := &corev1.ConfigMap{}

	// Retrying to make sure that the configmap has been properly generated before a provider is applied.
	// There is a race where, for example, a provider and a custom configmap resource are created at the
	// same time and the configmap does not get generated in time.
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return client.Get(ctx, name, obj)
	})
	if err != nil {
		return nil, err
	}
	d, ok := obj.Data[configMapKeySelector.Key]
	if !ok {
		return nil, fmt.Errorf("%s does not exist in configmap %s/%s", configMapKeySelector.Key, configMapKeySelector.Name, namespace)
	}
	return []byte(d), nil
}

// Get the value from a secret or configmap ref.
func (r *ProviderCustomDefaulter) Get(ctx context.Context, provider *etosv1alpha1.Provider, client client.Client, namespace string) ([]byte, error) {
	if provider.Spec.JSONTasSource.SecretKeyRef != nil {
		return getFromSecretKeySelector(ctx, client, provider.Spec.JSONTasSource.SecretKeyRef, namespace)
	}
	if provider.Spec.JSONTasSource.ConfigMapKeyRef != nil {
		return getFromConfigMapKeySelector(ctx, client, provider.Spec.JSONTasSource.ConfigMapKeyRef, namespace)
	}
	return nil, errors.New("found no source for key")
}

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha1-provider,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=providers,verbs=create;update,versions=v1alpha1,name=mprovider-v1alpha1.kb.io,admissionReviewVersions=v1

// ProviderCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Provider when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ProviderCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &ProviderCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Provider.
func (d *ProviderCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	provider, ok := obj.(*etosv1alpha1.Provider)

	if !ok {
		return fmt.Errorf("expected an Provider object but got %T", obj)
	}
	providerlog.Info("Defaulting for Provider", "name", provider.GetName())

	if provider.Spec.JSONTasSource == nil {
		return nil
	}
	jsontasBytes, err := d.Get(ctx, provider, cli, provider.Namespace)
	if err != nil {
		providerlog.Error(err, "failed to get jsontas from provider")
		return nil
	}
	providerlog.Info("Unmarshalling jsontasbytes")
	jsontas := &etosv1alpha1.JSONTas{}
	if err := json.Unmarshal(jsontasBytes, jsontas); err != nil {
		providerlog.Error(err, "failed to unmarshal jsontas from provider")
		return nil
	}
	providerlog.Info("Done", "jsontas", jsontas)
	provider.Spec.Healthcheck = nil   // Not sure about this one
	provider.Spec.Host = ""           // Not sure about this one
	provider.Spec.JSONTasSource = nil // Not sure about this one
	provider.Spec.JSONTas = jsontas
	return nil
}

// validate the spec of a Provider object.
func (d *ProviderCustomValidator) validate(provider *etosv1alpha1.Provider) error {
	var allErrs field.ErrorList
	groupVersionKind := provider.GroupVersionKind()
	if provider.Spec.JSONTas != nil && provider.Spec.JSONTasSource != nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("jsonTas"),
			provider.Spec.JSONTas,
			"only one of jsonTas and jsonTasSource is allowed"))
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("jsonTasSource"),
			provider.Spec.JSONTasSource,
			"only one of jsonTas and jsonTasSource is allowed"))
	}
	if provider.Spec.JSONTas == nil && provider.Spec.JSONTasSource == nil {
		if provider.Spec.Host == "" {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("host"),
				provider.Spec.Host,
				"host must be set when JSONTas is not"))
		}
		if provider.Spec.Healthcheck == nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("healthCheck"),
				provider.Spec.Healthcheck,
				"healthCheck must be set when JSONTas is not"))
		}
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: groupVersionKind.Group, Kind: groupVersionKind.Kind},
			provider.Name, allErrs,
		)
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-etos-eiffel-community-github-io-v1alpha1-provider,mutating=false,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=providers,verbs=create;update,versions=v1alpha1,name=vprovider-v1alpha1.kb.io,admissionReviewVersions=v1

// ProviderCustomValidator struct is responsible for validating the Provider resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ProviderCustomValidator struct{}

var _ webhook.CustomValidator = &ProviderCustomValidator{}

// ValidateCreate validates the creation of a Provider.
func (d *ProviderCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	provider, ok := obj.(*etosv1alpha1.Provider)

	if !ok {
		return nil, fmt.Errorf("expected an Provider object but got %T", obj)
	}
	providerlog.Info("Validation for Provider upon creation", "name", provider.GetName())
	return nil, d.validate(provider)
}

// ValidateUpdate validates the updates of a Provider.
func (d *ProviderCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	provider, ok := newObj.(*etosv1alpha1.Provider)

	if !ok {
		return nil, fmt.Errorf("expected an Provider object but got %T", newObj)
	}
	providerlog.Info("Validation for Provider upon update", "name", provider.GetName())
	return nil, d.validate(provider)
}

// ValidateDelete validates the deletion of a Provider.
func (d *ProviderCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	provider, ok := obj.(*etosv1alpha1.Provider)

	if !ok {
		return nil, fmt.Errorf("expected an Provider object but got %T", obj)
	}
	providerlog.Info("Validation for Provider upon deletion", "name", provider.GetName())
	return nil, nil
}
