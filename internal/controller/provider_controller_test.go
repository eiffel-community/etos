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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/controller/status"
)

var _ = Describe("Provider Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		provider := &etosv1alpha1.Provider{}
		SetDefaultEventuallyTimeout(2 * time.Minute)
		SetDefaultEventuallyPollingInterval(time.Second)

		BeforeEach(func() {
			By("creating the custom resource for the Kind Provider")
			err := k8sClient.Get(ctx, typeNamespacedName, provider)
			if err != nil && errors.IsNotFound(err) {
				resource := &etosv1alpha1.Provider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: etosv1alpha1.ProviderSpec{
						Type: "iut",
						// Because webhooks don't run during these tests we need to initialize the JSONTas key
						// to avoid health-check errors from the provider controller.
						JSONTas: &etosv1alpha1.JSONTas{
							Iut:            nil,
							ExecutionSpace: nil,
							LogArea:        nil,
						},
						JSONTasSource: &etosv1alpha1.VarSource{
							ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "cm"},
								Key:                  "test",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &etosv1alpha1.Provider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Provider")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Checking if the custom resource was successfully created")
			Eventually(func(g Gomega) {
				found := &etosv1alpha1.Provider{}
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, found)).To(Succeed())
			}).Should(Succeed())

			By("Checking that status conditions are initialized")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, provider)).To(Succeed())
				g.Expect(provider.Status.Conditions).NotTo(BeEmpty())
			}).Should(Succeed())

			By("Checking that the status condition 'Available' is set to True with reason 'Active'")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, provider)).To(Succeed())
				active := meta.FindStatusCondition(provider.Status.Conditions, status.StatusAvailable)
				g.Expect(active).NotTo(BeNil())
				g.Expect(active).NotTo(HaveValue(Equal(metav1.Condition{}))) // Not empty
				g.Expect(active.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(active.Reason).To(Equal(status.ReasonActive))
			}).Should(Succeed())
		})
	})
})
