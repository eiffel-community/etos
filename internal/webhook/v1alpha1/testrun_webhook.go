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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var testrunlog = logf.Log.WithName("testrun-resource")

// SetupTestRunWebhookWithManager registers the webhook for TestRun in the manager.
func SetupTestRunWebhookWithManager(mgr ctrl.Manager) error {
	if cli == nil {
		cli = mgr.GetClient()
	}
	return ctrl.NewWebhookManagedBy(mgr).For(&etosv1alpha1.TestRun{}).
		WithValidator(&TestRunCustomValidator{}).
		WithDefaulter(&TestRunCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha1-testrun,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=testruns,verbs=create;update,versions=v1alpha1,name=mtestrun-v1alpha1.kb.io,admissionReviewVersions=v1

// TestRunCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind TestRun when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type TestRunCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &TestRunCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind TestRun.
func (d *TestRunCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	testrun, ok := obj.(*etosv1alpha1.TestRun)

	if !ok {
		return fmt.Errorf("expected a TestRun object but got %T", obj)
	}
	testrunlog.Info("Defaulting for TestRun", "name", testrun.GetName())
	testrunlog.Info("TestRun spec", "spec", testrun.Spec)

	if testrun.Spec.ID == "" {
		testrunlog.Info("TestRun ID does not exist, generating a new one")
		testrun.Spec.ID = string(uuid.NewUUID())
	}

	testrunlog.Info("Checking for a cluster, either in spec or in namespace")
	clusters := &etosv1alpha1.ClusterList{}
	var cluster *etosv1alpha1.Cluster
	if testrun.Spec.Cluster == "" {
		testrunlog.Info("TestRun cluster is empty, checking if a cluster is deployed in namespace")
		if err := cli.List(ctx, clusters, client.InNamespace(testrun.Namespace)); err != nil {
			testrunlog.Error(err, "name", testrun.Name, "namespace", testrun.Namespace, "Failed to get clusters in namespace")
		}
		testrunlog.Info("Number of clusters found in namespace", "count", len(clusters.Items))
		if len(clusters.Items) == 1 {
			cluster = &clusters.Items[0]
			testrun.Spec.Cluster = cluster.Name
		}
	} else {
		testrunlog.Info("Getting cluster from specification")
		clusterNamespacedName := types.NamespacedName{
			Name:      testrun.Spec.Cluster,
			Namespace: testrun.Namespace,
		}
		testrunlog.Info("Getting cluster from specification", "name", clusterNamespacedName.Name, "namespace", clusterNamespacedName.Namespace)
		clu := &etosv1alpha1.Cluster{}
		if err := cli.Get(ctx, clusterNamespacedName, clu); err != nil {
			testrunlog.Error(err, "name", testrun.Name, "namespace", testrun.Namespace, "cluster", clusterNamespacedName.Name,
				"Failed to get cluster in namespace")
		}
		cluster = clu
	}

	if testrun.Spec.SuiteRunner == nil && cluster != nil {
		testrunlog.Info("Loading suite runner image from cluster", "suiteRunner", cluster.Spec.ETOS.SuiteRunner.Image)
		testrun.Spec.SuiteRunner = &etosv1alpha1.SuiteRunner{Image: &cluster.Spec.ETOS.SuiteRunner.Image}
	}
	if testrun.Spec.LogListener == nil && cluster != nil {
		testrunlog.Info("Loading log listener image from cluster", "logListener", cluster.Spec.ETOS.SuiteRunner.LogListener.Image)
		testrun.Spec.LogListener = &etosv1alpha1.LogListener{Image: &cluster.Spec.ETOS.SuiteRunner.LogListener.Image}
	}
	if testrun.Spec.EnvironmentProvider == nil && cluster != nil {
		testrunlog.Info("Loading environment provider image from cluster", "environmentProvider", cluster.Spec.ETOS.EnvironmentProvider.Image)
		testrun.Spec.EnvironmentProvider = &etosv1alpha1.EnvironmentProvider{Image: &cluster.Spec.ETOS.EnvironmentProvider.Image}
	}
	if testrun.Spec.TestRunner == nil && cluster != nil {
		testrunlog.Info("Loading test runner version from cluster", "testRunner", cluster.Spec.ETOS.TestRunner.Version)
		testrun.Spec.TestRunner = &etosv1alpha1.TestRunner{Version: cluster.Spec.ETOS.TestRunner.Version}
	}

	addLabel := true
	if testrun.ObjectMeta.Labels != nil {
		for key := range testrun.ObjectMeta.GetLabels() {
			if key == "etos.eiffel-community.github.io/id" {
				addLabel = false
			}
		}
	} else {
		testrun.ObjectMeta.Labels = map[string]string{}
	}
	if addLabel {
		testrunlog.Info("Adding ETOS test run ID label", "id", testrun.Spec.ID)
		testrun.ObjectMeta.Labels["etos.eiffel-community.github.io/id"] = testrun.Spec.ID
	}
	testrunlog.Info("Defaulting webhook has finished")

	return nil
}

// +kubebuilder:webhook:path=/validate-etos-eiffel-community-github-io-v1alpha1-testrun,mutating=false,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=testruns,verbs=create;update,versions=v1alpha1,name=vtestrun-v1alpha1.kb.io,admissionReviewVersions=v1

// TestRunCustomValidator struct is responsible for validating the TestRun resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type TestRunCustomValidator struct{}

var _ webhook.CustomValidator = &TestRunCustomValidator{}

// validate that the required parameters are set. This validation is done here instead of directly in the struct since
// we do mutate the input in the Default function.
func (d *TestRunCustomValidator) validate(testrun *etosv1alpha1.TestRun) error {
	var allErrs field.ErrorList
	if testrun.Spec.Cluster == "" {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("cluster"),
			testrun.Spec.Cluster,
			"Cluster is missing, either no cluster exists in namespace or too many to choose from",
		))
	}

	if testrun.Spec.SuiteRunner == nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("suiteRunner"),
			testrun.Spec.SuiteRunner,
			"SuiteRunner image information is missing, maybe because cluster is missing?",
		))
	}

	if testrun.Spec.LogListener == nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("logListener"),
			testrun.Spec.LogListener,
			"LogListener image information is missing, maybe because cluster is missing?",
		))
	}

	if testrun.Spec.EnvironmentProvider == nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("environmentProvider"),
			testrun.Spec.EnvironmentProvider,
			"EnvironmentProvider image information is missing, maybe because cluster is missing?",
		))
	}

	if testrun.Spec.TestRunner == nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("testRunner"),
			testrun.Spec.TestRunner,
			"TestRunner version information is missing, maybe because cluster is missing?",
		))
	}

	groupVersionKind := testrun.GroupVersionKind()
	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: groupVersionKind.Group, Kind: groupVersionKind.Kind},
			testrun.Name, allErrs,
		)
	}
	return nil
}

// ValidateCreate validates the creation of a TestRun.
func (d *TestRunCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	testrun, ok := obj.(*etosv1alpha1.TestRun)

	if !ok {
		return nil, fmt.Errorf("expected a TestRun object but got %T", obj)
	}
	testrunlog.Info("Validation for TestRun upon creation", "name", testrun.GetName())
	return nil, d.validate(testrun)
}

// ValidateUpdate validates the updates of a TestRun.
func (d *TestRunCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	testrun, ok := newObj.(*etosv1alpha1.TestRun)

	if !ok {
		return nil, fmt.Errorf("expected a TestRun object but got %T", newObj)
	}
	testrunlog.Info("Validation for TestRun upon update", "name", testrun.GetName())
	return nil, d.validate(testrun)
}

// ValidateDelete validates the deletion of a TestRun.
func (d *TestRunCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	testrun, ok := obj.(*etosv1alpha1.TestRun)

	if !ok {
		return nil, fmt.Errorf("expected a TestRun object but got %T", obj)
	}
	testrunlog.Info("Validation for TestRun upon deletion", "name", testrun.GetName())
	return nil, nil
}
