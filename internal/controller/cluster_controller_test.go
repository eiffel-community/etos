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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

	Context("checkReadiness", func() {
		const clusterName = "readiness-test"
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
			// Re-fetch to get UID populated.
			Expect(k8sClient.Get(ctx, typeNamespacedName, cluster)).To(Succeed())
		})

		AfterEach(func() {
			resource := &etosv1alpha1.Cluster{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
			// Clean up any deployments created during the test.
			depList := &appsv1.DeploymentList{}
			if err := k8sClient.List(ctx, depList); err == nil {
				for i := range depList.Items {
					_ = k8sClient.Delete(ctx, &depList.Items[i])
				}
			}
		})

		It("should return true when there are no owned deployments or statefulsets", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			ready, message, err := reconciler.checkReadiness(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())
			Expect(message).To(BeEmpty())
		})

		It("should return false when an owned deployment has no ready replicas", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			replicas := int32(1)
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName + "-etos-api",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "etos.eiffel-community.github.io/v1alpha1",
							Kind:       "Cluster",
							Name:       cluster.Name,
							UID:        cluster.UID,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "etos-api"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "etos-api"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "etos-api", Image: "busybox"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())

			ready, message, err := reconciler.checkReadiness(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
			Expect(message).To(ContainSubstring("Deployment"))
			Expect(message).To(ContainSubstring("0/1"))
		})

		It("should not consider deployments not owned by this cluster", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			replicas := int32(1)
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unrelated-deployment",
					Namespace: "default",
					// No owner references pointing to our cluster.
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "unrelated"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "unrelated"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "unrelated", Image: "busybox"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())

			ready, message, err := reconciler.checkReadiness(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())
			Expect(message).To(BeEmpty())
		})
	})

	Context("updateStatus", func() {
		const clusterName = "status-test"
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

		It("should set Reconciling=True and Ready=False when pods are not ready", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.updateStatus(ctx, cluster,
				metav1.ConditionTrue, status.ReasonReconciling,
				metav1.ConditionFalse, status.ReasonPodsNotReady,
				"Deployment test-api: 0/1 replicas ready",
			)
			Expect(err).NotTo(HaveOccurred())

			updated := &etosv1alpha1.Cluster{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

			reconcilingCond := meta.FindStatusCondition(updated.Status.Conditions, status.StatusReconciling)
			Expect(reconcilingCond).NotTo(BeNil())
			Expect(reconcilingCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(reconcilingCond.Reason).To(Equal(status.ReasonReconciling))

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, status.StatusReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(status.ReasonPodsNotReady))
		})

		It("should set Reconciling=False and Ready=True when cluster is fully ready", func() {
			reconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.updateStatus(ctx, cluster,
				metav1.ConditionFalse, status.ReasonCompleted,
				metav1.ConditionTrue, status.ReasonCompleted,
				"Cluster is up and running",
			)
			Expect(err).NotTo(HaveOccurred())

			updated := &etosv1alpha1.Cluster{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

			reconcilingCond := meta.FindStatusCondition(updated.Status.Conditions, status.StatusReconciling)
			Expect(reconcilingCond).NotTo(BeNil())
			Expect(reconcilingCond.Status).To(Equal(metav1.ConditionFalse))

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, status.StatusReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Message).To(Equal("Cluster is up and running"))
		})
	})
})
