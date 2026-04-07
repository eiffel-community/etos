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

package v1alpha2

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
)

// nolint:unused
// log is for logging in this package.
var iutlog = logf.Log.WithName("iut-resource")

// SetupIutWebhookWithManager registers the webhook for Iut in the manager.
func SetupIutWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&etosv1alpha2.Iut{}).
		WithDefaulter(&IutCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha2-iut,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=iuts,verbs=create;update,versions=v1alpha2,name=miut-v1alpha2.kb.io,admissionReviewVersions=v1

// IutCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Iut when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type IutCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &IutCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Iut.
func (d *IutCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	iut, ok := obj.(*etosv1alpha2.Iut)

	if !ok {
		return fmt.Errorf("expected an Iut object but got %T", obj)
	}
	iutlog.Info("Defaulting for Iut", "name", iut.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}
