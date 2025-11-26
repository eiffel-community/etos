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

package v1alpha2

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
)

// nolint:unused
// logarealog is for logging in this package.
var logarealog = logf.Log.WithName("logarea-resource")

// SetupLogAreaWebhookWithManager registers the webhook for LogArea in the manager.
func SetupLogAreaWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&etosv1alpha2.LogArea{}).
		WithDefaulter(&LogAreaCustomDefaulter{mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha2-logarea,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=logarea,verbs=create;update,versions=v1alpha2,name=mlogarea-v1alpha2.kb.io,admissionReviewVersions=v1

// LogAreaCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind LogArea when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type LogAreaCustomDefaulter struct {
	client.Reader
}

var _ webhook.CustomDefaulter = &LogAreaCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind LogArea.
func (d *LogAreaCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	logarea, ok := obj.(*etosv1alpha2.LogArea)

	if !ok {
		return fmt.Errorf("expected an LogArea object but got %T", obj)
	}
	logarealog.Info("Defaulting for LogArea", "name", logarea.GetName())

	environmentrequest := &v1alpha1.EnvironmentRequest{}
	namespacedName := types.NamespacedName{Name: logarea.Spec.EnvironmentRequest, Namespace: logarea.Namespace}
	if err := d.Get(ctx, namespacedName, environmentrequest); err != nil {
		logarealog.Error(err, "Failed to get environmentrequest in namespace",
			"name", logarea.Name,
			"namespace", logarea.Namespace,
			"environmentRequest", namespacedName.Name,
		)
		return err
	}

	if logarea.Labels == nil {
		logarea.Labels = make(map[string]string)
	}
	logarea.Labels["etos.eiffel-community.github.io/environment-request"] = environmentrequest.Spec.Name
	logarea.Labels["etos.eiffel-community.github.io/environment-request-id"] = environmentrequest.Spec.ID
	logarea.Labels["etos.eiffel-community.github.io/provider"] = logarea.Spec.ProviderID
	logarea.Labels["app.kubernetes.io/part-of"] = etos
	if logarea.Labels["app.kubernetes.io/name"] == "" {
		logarea.Labels["app.kubernetes.io/name"] = "logarea-provider"
	}
	if cluster := environmentrequest.Labels["etos.eiffel-community.github.io/cluster"]; cluster != "" {
		logarea.Labels["etos.eiffel-community.github.io/cluster"] = cluster
	}
	if environmentrequest.Spec.Identifier != "" {
		logarea.Labels["etos.eiffel-community.github.io/id"] = environmentrequest.Spec.Identifier
	}

	return nil
}
