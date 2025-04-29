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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

var _ = Describe("Environment Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		environment := &etosv1alpha1.Environment{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Environment")
			err := k8sClient.Get(ctx, typeNamespacedName, environment)
			if err != nil && errors.IsNotFound(err) {
				resource := &etosv1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: etosv1alpha1.EnvironmentSpec{
						Name:        "etos-controller-test",
						SuiteID:     "98f061a2-ee55-447c-aff4-78cafa6ae15d",
						SubSuiteID:  "7dbbed66-b71f-4ff7-a76c-fe9cd77409b4",
						MainSuiteID: "1ad41f38-3745-41cc-bd19-cafc0c673dae",
						Artifact:    "268dd4db-93da-4232-a544-bf4c0fb26dac",
						Context:     "012a79a7-3f43-41e4-82c2-d71b4899d82e",
						Priority:    1,
						Tests:       []etosv1alpha1.Test{},
						TestRunner:  "ghcr.io/eiffel-community/etos-base-test-runner:bullseye",
						Iut: &apiextensionsv1.JSON{
							Raw: []byte("{}"),
						},
						Executor: &apiextensionsv1.JSON{
							Raw: []byte("{}"),
						},
						LogArea: &apiextensionsv1.JSON{
							Raw: []byte("{}"),
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &etosv1alpha1.Environment{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Environment")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &EnvironmentReconciler{
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
