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
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	healthmetrics "github.com/eiffel-community/etos/internal/metrics"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var _ = Describe("Inventory cache", func() {
	It("trims initial and updated objects without changing the reconciliation cache", func() {
		run := &etosv1alpha1.TestRun{
			ObjectMeta: metav1.ObjectMeta{Name: "inventory-cache-test", Namespace: "default"},
			Spec: etosv1alpha1.TestRunSpec{
				Artifact: "268dd4db-93da-4232-a544-bf4c0fb26dac", Identity: "pkg:testrun/etos/cache",
				Cluster: "cache-cluster", Suites: []etosv1alpha1.Suite{{
					Name: "payload", Tests: []etosv1alpha1.Test{},
					Dataset: &apiextensionsv1.JSON{Raw: []byte(`{"marker":"initial-payload"}`)},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, run)).To(Succeed())
		DeferCleanup(func() {
			if err := k8sClient.Delete(ctx, run); !apierrors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		})
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: k8sClient.Scheme(), Metrics: metricsserver.Options{BindAddress: "0"},
			HealthProbeBindAddress: "0", LeaderElection: false,
		})
		Expect(err).NotTo(HaveOccurred())
		inventory, err := healthmetrics.NewInventoryCache(ctx, mgr)
		Expect(err).NotTo(HaveOccurred())
		Expect(mgr.Add(inventory)).To(Succeed())
		managerCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- mgr.Start(managerCtx) }()
		DeferCleanup(func() {
			stop()
			Eventually(done, 10*time.Second).Should(Receive(Succeed()))
		})
		readCtx, cancelRead := context.WithTimeout(ctx, 15*time.Second)
		defer cancelRead()
		key := client.ObjectKeyFromObject(run)
		readInventory := func() *unstructured.Unstructured {
			item := &unstructured.Unstructured{}
			item.SetAPIVersion("etos.eiffel-community.github.io/v1alpha1")
			item.SetKind("TestRun")
			Expect(inventory.Get(readCtx, key, item)).To(Succeed())
			return item
		}
		Eventually(func() error {
			list := &unstructured.UnstructuredList{}
			list.SetAPIVersion("etos.eiffel-community.github.io/v1alpha1")
			list.SetKind("TestRunList")
			return inventory.List(readCtx, list)
		}, 10*time.Second).Should(Succeed())
		item := readInventory()
		_, hasSuites, err := unstructured.NestedFieldNoCopy(item.Object, "spec", "suites")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasSuites).To(BeFalse())
		cluster, _, err := unstructured.NestedString(item.Object, "spec", "cluster")
		Expect(err).NotTo(HaveOccurred())
		Expect(cluster).To(Equal(run.Spec.Cluster))

		full := &etosv1alpha1.TestRun{}
		Expect(mgr.GetClient().Get(readCtx, key, full)).To(Succeed())
		Expect(full.Spec.Suites).To(HaveLen(1))
		Expect(string(full.Spec.Suites[0].Dataset.Raw)).To(ContainSubstring("initial-payload"))
		Expect(k8sClient.Get(ctx, key, run)).To(Succeed())
		run.Spec.Suites[0].Dataset.Raw = []byte(`{"marker":"updated-payload"}`)
		run.Spec.Deadline = time.Now().Add(time.Hour).Unix()
		Expect(k8sClient.Update(ctx, run)).To(Succeed())
		Eventually(func() int64 {
			deadline, _, err := unstructured.NestedInt64(readInventory().Object, "spec", "deadline")
			Expect(err).NotTo(HaveOccurred())
			return deadline
		}, 10*time.Second).Should(Equal(run.Spec.Deadline))
		_, hasSuites, err = unstructured.NestedFieldNoCopy(readInventory().Object, "spec", "suites")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasSuites).To(BeFalse())
		Eventually(func() string {
			Expect(mgr.GetClient().Get(readCtx, key, full)).To(Succeed())
			return string(full.Spec.Suites[0].Dataset.Raw)
		}, 10*time.Second).Should(ContainSubstring("updated-payload"))

		run.Status.Verdict = "Passed"
		run.Status.CompletionTime = &metav1.Time{Time: time.Now()}
		Expect(k8sClient.Status().Update(ctx, run)).To(Succeed())
		Eventually(func() string {
			verdict, _, err := unstructured.NestedString(readInventory().Object, "status", "verdict")
			Expect(err).NotTo(HaveOccurred())
			return verdict
		}, 10*time.Second).Should(Equal("Passed"))
		unsupported := &unstructured.UnstructuredList{}
		unsupported.SetAPIVersion("v1")
		unsupported.SetKind("SecretList")
		Expect(inventory.List(readCtx, unsupported)).NotTo(Succeed())
		Expect(k8sClient.Delete(ctx, run)).To(Succeed())
		Eventually(func() bool {
			object := &unstructured.Unstructured{}
			object.SetAPIVersion("etos.eiffel-community.github.io/v1alpha1")
			object.SetKind("TestRun")
			return apierrors.IsNotFound(inventory.Get(readCtx, key, object))
		}, 10*time.Second).Should(BeTrue())
	})
})

// Test_reconcileEnvironmentRequest_doesNotLogSuitePayload verifies that reconciliation omits the suite payload from logs.
func Test_reconcileEnvironmentRequest_doesNotLogSuitePayload(t *testing.T) {
	const marker = "suite-payload-must-not-be-logged"
	scheme := newTransitionTestScheme(t)
	run := &etosv1alpha1.TestRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "default"},
		Spec: etosv1alpha1.TestRunSpec{Suites: []etosv1alpha1.Suite{{
			Name: "suite", Dataset: &apiextensionsv1.JSON{Raw: []byte(`{"marker":"` + marker + `"}`)},
		}}},
		Status: etosv1alpha1.TestRunStatus{Conditions: []metav1.Condition{{
			Type: "Environment", Status: metav1.ConditionFalse, Reason: "Pending",
		}}},
	}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&etosv1alpha1.EnvironmentRequest{}, TestRunOwnerKey, func(client.Object) []string { return nil }).
		Build()
	reconciler := &TestRunReconciler{Client: kubeClient, Scheme: scheme}
	var output bytes.Buffer
	logger := zap.New(zap.WriteTo(&output), zap.UseDevMode(true))
	if _, err := reconciler.reconcileEnvironmentRequest(logf.IntoContext(t.Context(), logger), &etosv1alpha1.Cluster{}, run, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), marker) {
		t.Fatal("environment-request reconciliation logged the suite payload")
	}
}
