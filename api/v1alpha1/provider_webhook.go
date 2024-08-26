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
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var (
	providerlog = logf.Log.WithName("provider-resource")
	cli         client.Client
)

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *Provider) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if cli == nil {
		cli = mgr.GetClient()
	}
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
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
func (r *Provider) Get(ctx context.Context, client client.Client, namespace string) ([]byte, error) {
	if r.Spec.JSONTasSource.SecretKeyRef != nil {
		return getFromSecretKeySelector(ctx, client, r.Spec.JSONTasSource.SecretKeyRef, namespace)
	}
	if r.Spec.JSONTasSource.ConfigMapKeyRef != nil {
		return getFromConfigMapKeySelector(ctx, client, r.Spec.JSONTasSource.ConfigMapKeyRef, namespace)
	}
	return nil, errors.New("found no source for key")
}

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha1-provider,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=providers,verbs=create;update,versions=v1alpha1,name=mprovider.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Provider{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Provider) Default() {
	providerlog.Info("default", "name", r.Name)

	if r.Spec.JSONTasSource == nil {
		return
	}

	jsontasBytes, err := r.Get(context.TODO(), cli, r.Namespace)
	if err != nil {
		providerlog.Error(err, "failed to get jsontas from provider")
		return
	}
	providerlog.Info("Unmarshalling jsontasbytes", "jsontas", string(jsontasBytes))
	jsontas := &JSONTas{}
	if err := json.Unmarshal(jsontasBytes, jsontas); err != nil {
		providerlog.Error(err, "failed to unmarshal jsontas from provider")
		return
	}
	providerlog.Info("Done", "jsontas", jsontas)
	r.Spec.Healthcheck = nil   // Not sure about this one
	r.Spec.Host = ""           // Not sure about this one
	r.Spec.JSONTasSource = nil // Not sure about this one
	r.Spec.JSONTas = jsontas
}

// validate the spec of a Provider object.
func (r *Provider) validate() error {
	var allErrs field.ErrorList
	groupVersionKind := r.GroupVersionKind()
	if r.Spec.JSONTas != nil && r.Spec.JSONTasSource != nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("jsonTas"),
			r.Spec.JSONTas,
			"only one of jsonTas and jsonTasSource is allowed"))
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("jsonTasSource"),
			r.Spec.JSONTasSource,
			"only one of jsonTas and jsonTasSource is allowed"))
	}
	if r.Spec.JSONTas == nil && r.Spec.JSONTasSource == nil {
		if r.Spec.Host == "" {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("host"),
				r.Spec.Host,
				"host must be set when JSONTas is not"))
		}
		if r.Spec.Healthcheck == nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("healthCheck"),
				r.Spec.Healthcheck,
				"healthCheck must be set when JSONTas is not"))
		}
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: groupVersionKind.Group, Kind: groupVersionKind.Kind},
			r.Name, allErrs,
		)
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-etos-eiffel-community-github-io-v1alpha1-provider,mutating=false,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=providers,verbs=create;update,versions=v1alpha1,name=mprovider.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Provider{}

// ValidateCreate validates the creation of a Provider.
func (r *Provider) ValidateCreate() (admission.Warnings, error) {
	providerlog.Info("validate create", "name", r.Name)
	return nil, r.validate()
}

// ValidateUpdate validates the updates of a Provider.
func (r *Provider) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	providerlog.Info("validate update", "name", r.Name)
	return nil, r.validate()
}

// ValidateDelete validates the deletion of a Provider.
func (r *Provider) ValidateDelete() (admission.Warnings, error) {
	providerlog.Info("validate delete", "name", r.Name)
	return nil, nil
}
