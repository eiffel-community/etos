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

package controller

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
)

// TestCleanupUnusedPreservesAssignedProviders verifies cleanup can run before ownership is recorded.
func TestCleanupUnusedPreservesAssignedProviders(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := etosv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := etosv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	request := &etosv1alpha1.EnvironmentRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "request", Namespace: "default"},
		Spec:       etosv1alpha1.EnvironmentRequestSpec{ID: "6f3a5a46-f2f6-48d8-a29d-174321583ae9"},
	}
	labels := map[string]string{"etos.eiffel-community.github.io/environment-request-id": request.Spec.ID}
	assigned := []client.Object{
		&etosv1alpha2.Iut{ObjectMeta: metav1.ObjectMeta{Name: "device", Namespace: request.Namespace, Labels: labels}},
		&etosv1alpha2.LogArea{ObjectMeta: metav1.ObjectMeta{Name: "logs", Namespace: request.Namespace, Labels: labels}},
		&etosv1alpha2.ExecutionSpace{ObjectMeta: metav1.ObjectMeta{Name: "runner", Namespace: request.Namespace, Labels: labels}},
	}
	unused := []client.Object{
		&etosv1alpha2.Iut{ObjectMeta: metav1.ObjectMeta{Name: "runner", Namespace: request.Namespace, Labels: labels}},
		&etosv1alpha2.LogArea{ObjectMeta: metav1.ObjectMeta{Name: "device", Namespace: request.Namespace, Labels: labels}},
		&etosv1alpha2.ExecutionSpace{ObjectMeta: metav1.ObjectMeta{Name: "logs", Namespace: request.Namespace, Labels: labels}},
	}
	environment := &etosv1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "environment", Namespace: request.Namespace, Labels: labels},
		Spec:       etosv1alpha1.EnvironmentSpec{Providers: &etosv1alpha1.Providers{IUT: "device", LogArea: "logs", ExecutionSpace: "runner"}},
	}
	cached := fake.NewClientBuilder().WithScheme(scheme).WithObjects(assigned...).WithObjects(unused...).Build()
	apiReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(environment).Build()
	reconciler := EnvironmentRequestReconciler{Client: cached, apiReader: apiReader}

	if err := reconciler.cleanupUnused(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	for _, obj := range assigned {
		if err := cached.Get(t.Context(), client.ObjectKeyFromObject(obj), obj); err != nil {
			t.Errorf("assigned %T %q was not retained: %v", obj, obj.GetName(), err)
		}
	}
	for _, obj := range unused {
		if err := cached.Get(t.Context(), client.ObjectKeyFromObject(obj), obj); !errors.IsNotFound(err) {
			t.Errorf("unused %T %q: expected NotFound, got %v", obj, obj.GetName(), err)
		}
	}
}

// TestProviderMetricsTemporality verifies provider jobs inherit only explicitly enabled metric preferences.
func TestProviderMetricsTemporality(t *testing.T) {
	reconciler := EnvironmentRequestReconciler{Client: fake.NewClientBuilder().Build()}
	request := &etosv1alpha1.EnvironmentRequest{}
	request.Spec.Config.EncryptionKey = etosv1alpha1.Var{Value: "test-key"}
	password := &etosv1alpha1.Var{Value: "test-password"}
	request.Spec.Config.EtosMessageBus.Password = password
	request.Spec.Config.EiffelMessageBus.Password = password
	for _, step := range []struct {
		name       string
		cluster    *etosv1alpha1.Cluster
		preference string
	}{
		{name: "no cluster"},
		{name: "disabled", cluster: &etosv1alpha1.Cluster{}, preference: "DELTA"},
		{name: "unset", cluster: &etosv1alpha1.Cluster{}},
		{name: "delta", cluster: &etosv1alpha1.Cluster{}, preference: "DELTA"},
		{name: "cumulative", cluster: &etosv1alpha1.Cluster{}, preference: "CUMULATIVE"},
	} {
		t.Run(step.name, func(t *testing.T) {
			enabled := step.cluster != nil && step.name != "disabled"
			if step.cluster != nil {
				step.cluster.Spec.OpenTelemetry = etosv1alpha1.OpenTelemetry{
					Enabled: enabled, MetricsTemporalityPreference: step.preference,
				}
			}
			environment, err := reconciler.envVarListFrom(t.Context(), request, step.cluster)
			if err != nil {
				t.Fatal(err)
			}
			var value string
			found := false
			for _, variable := range environment {
				if variable.Name == "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE" {
					value, found = variable.Value, true
				}
			}
			want := enabled && step.preference != ""
			if found != want || (want && value != step.preference) {
				t.Fatalf("temporality = %q (present %v), want %q (present %v)", value, found, step.preference, want)
			}
		})
	}
}

var _ = Describe("EnvironmentRequest Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		environmentrequest := &etosv1alpha1.EnvironmentRequest{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind EnvironmentRequest")
			err := k8sClient.Get(ctx, typeNamespacedName, environmentrequest)
			if err != nil && errors.IsNotFound(err) {
				resource := &etosv1alpha1.EnvironmentRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: etosv1alpha1.EnvironmentRequestSpec{
						Image: &etosv1alpha1.Image{
							Image: "ghcr.io/eiffel-community/etos-environment-provider:60bf50aa",
						},
						MinimumAmount: 1,
						MaximumAmount: 1,
						Providers: etosv1alpha1.EnvironmentProviders{
							IUT:     etosv1alpha1.IutProvider{ID: "default"},
							LogArea: etosv1alpha1.LogAreaProvider{ID: "default"},
							ExecutionSpace: etosv1alpha1.ExecutionSpaceProvider{
								ID:         "default",
								TestRunner: "ghcr.io/eiffel-community/etos-base-test-runner:bullseye",
							},
						},
						Splitter: etosv1alpha1.Splitter{
							Tests: []etosv1alpha1.Test{},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &etosv1alpha1.EnvironmentRequest{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance EnvironmentRequest")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &EnvironmentRequestReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
