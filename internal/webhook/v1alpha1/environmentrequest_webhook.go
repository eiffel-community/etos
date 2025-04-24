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

	"k8s.io/apimachinery/pkg/util/uuid"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var environmentrequestlog = logf.Log.WithName("environmentrequest-resource")

// SetupEnvironmentRequestWebhookWithManager registers the webhook for EnvironmentRequest in the manager.
func SetupEnvironmentRequestWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&etosv1alpha1.EnvironmentRequest{}).
		WithDefaulter(&EnvironmentRequestCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha1-environmentrequest,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=environmentrequests,verbs=create;update,versions=v1alpha1,name=menvironmentrequest-v1alpha1.kb.io,admissionReviewVersions=v1

// EnvironmentRequestCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind EnvironmentRequest when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type EnvironmentRequestCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &EnvironmentRequestCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind EnvironmentRequest.
func (d *EnvironmentRequestCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	environmentrequest, ok := obj.(*etosv1alpha1.EnvironmentRequest)

	if !ok {
		return fmt.Errorf("expected an EnvironmentRequest object but got %T", obj)
	}
	environmentrequestlog.Info("Defaulting for EnvironmentRequest", "name", environmentrequest.GetName())

	if environmentrequest.Spec.ID == "" {
		environmentrequest.Spec.ID = string(uuid.NewUUID())
	}

	return nil
}
