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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/controller/status"
)

var _ = Describe("Cluster Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		cluster := &etosv1alpha1.Cluster{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Cluster")
			err := k8sClient.Get(ctx, typeNamespacedName, cluster)
			if err != nil && errors.IsNotFound(err) {
				resource := &etosv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: etosv1alpha1.ClusterSpec{
						ETOS:            etosv1alpha1.ETOS{},
						Database:        etosv1alpha1.Database{},
						MessageBus:      etosv1alpha1.MessageBus{},
						EventRepository: etosv1alpha1.EventRepository{},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &etosv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Cluster")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ClusterReconciler{
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

	Context("update", func() {
		const clusterName = "update-test"
		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      clusterName,
			Namespace: "default",
		}

		var cluster *etosv1alpha1.Cluster

		BeforeEach(func() {
			cluster = &etosv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: "default",
				},
				Spec: etosv1alpha1.ClusterSpec{
					ETOS:            etosv1alpha1.ETOS{},
					Database:        etosv1alpha1.Database{},
					MessageBus:      etosv1alpha1.MessageBus{},
					EventRepository: etosv1alpha1.EventRepository{},
				},
			}
			err := k8sClient.Get(ctx, typeNamespacedName, &etosv1alpha1.Cluster{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			}
			Expect(k8sClient.Get(ctx, typeNamespacedName, cluster)).To(Succeed())
		})

		AfterEach(func() {
			resource := &etosv1alpha1.Cluster{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Ready=True with Completed reason when cluster is fully ready", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.update(ctx, cluster, metav1.ConditionTrue, status.ReasonCompleted, "Cluster is up and running")
			Expect(err).NotTo(HaveOccurred())

			updated := &etosv1alpha1.Cluster{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, status.StatusReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal(status.ReasonCompleted))
			Expect(readyCond.Message).To(Equal("Cluster is up and running"))
		})

		It("should set Ready=False with Pending reason", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.update(ctx, cluster, metav1.ConditionFalse, status.ReasonPending, "test-api: 0/1 replicas ready")
			Expect(err).NotTo(HaveOccurred())

			updated := &etosv1alpha1.Cluster{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, status.StatusReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(status.ReasonPending))
			Expect(readyCond.Message).To(ContainSubstring("0/1"))
		})

		It("should set Ready=False with Failed reason", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.update(ctx, cluster, metav1.ConditionFalse, status.ReasonFailed, "something went wrong")
			Expect(err).NotTo(HaveOccurred())

			updated := &etosv1alpha1.Cluster{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, status.StatusReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(status.ReasonFailed))
		})
	})

	Context("handleReconcileError", func() {
		const clusterName = "handle-error-test"
		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      clusterName,
			Namespace: "default",
		}

		var cluster *etosv1alpha1.Cluster

		BeforeEach(func() {
			cluster = &etosv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: "default",
				},
				Spec: etosv1alpha1.ClusterSpec{
					ETOS:            etosv1alpha1.ETOS{},
					Database:        etosv1alpha1.Database{},
					MessageBus:      etosv1alpha1.MessageBus{},
					EventRepository: etosv1alpha1.EventRepository{},
				},
			}
			err := k8sClient.Get(ctx, typeNamespacedName, &etosv1alpha1.Cluster{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			}
			Expect(k8sClient.Get(ctx, typeNamespacedName, cluster)).To(Succeed())
		})

		AfterEach(func() {
			resource := &etosv1alpha1.Cluster{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Ready=False with Pending reason for NotReadyError", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			notReadyErr := &status.NotReadyError{
				Name:            "test-etos-api",
				ReadyReplicas:   0,
				DesiredReplicas: 1,
			}
			_, err := reconciler.handleReconcileError(ctx, cluster, notReadyErr)
			Expect(err).NotTo(HaveOccurred())

			updated := &etosv1alpha1.Cluster{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, status.StatusReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(status.ReasonPending))
			Expect(readyCond.Message).To(ContainSubstring("0/1"))
		})

		It("should set Ready=False with Failed reason for other errors", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.handleReconcileError(ctx, cluster, fmt.Errorf("connection refused"))
			Expect(err).NotTo(HaveOccurred())

			updated := &etosv1alpha1.Cluster{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, status.StatusReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(status.ReasonFailed))
			Expect(readyCond.Message).To(Equal("connection refused"))
		})
	})
})
