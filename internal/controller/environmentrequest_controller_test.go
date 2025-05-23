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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

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
