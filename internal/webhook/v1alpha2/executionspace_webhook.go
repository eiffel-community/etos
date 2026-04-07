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
var executionspacelog = logf.Log.WithName("executionspace-resource")

// SetupExecutionSpaceWebhookWithManager registers the webhook for ExecutionSpace in the manager.
func SetupExecutionSpaceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&etosv1alpha2.ExecutionSpace{}).
		WithDefaulter(&ExecutionSpaceCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha2-executionspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=executionspaces,verbs=create;update,versions=v1alpha2,name=mexecutionspace-v1alpha2.kb.io,admissionReviewVersions=v1

// ExecutionSpaceCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind ExecutionSpace when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ExecutionSpaceCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &ExecutionSpaceCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind ExecutionSpace.
func (d *ExecutionSpaceCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	executionspace, ok := obj.(*etosv1alpha2.ExecutionSpace)

	if !ok {
		return fmt.Errorf("expected an ExecutionSpace object but got %T", obj)
	}
	executionspacelog.Info("Defaulting for ExecutionSpace", "name", executionspace.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}
