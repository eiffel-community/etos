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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/internal/controller/status"
)

var _ = Describe("Environment Controller", func() {
	Context("When reconciling a resource", func() {

		const resourceName = "test-environment"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		environment := &etosv1alpha1.Environment{}
		SetDefaultEventuallyTimeout(2 * time.Minute)
		SetDefaultEventuallyPollingInterval(time.Second)

		BeforeEach(func() {
			By("creating the EnvironmentRequest resource that the Environment depends on")
			environmentRequest, err := createEnvironmentRequest(ctx, "test-environment-request")
			Expect(err).NotTo(HaveOccurred())
			By("ensuring that the iut, executor and log area providers exist")
			iutProvider, err := createProvider(ctx, environmentRequest.Spec.Providers.IUT.ID, "iut")
			Expect(err).NotTo(HaveOccurred())
			executorProvider, err := createProvider(ctx, environmentRequest.Spec.Providers.ExecutionSpace.ID, "execution-space")
			Expect(err).NotTo(HaveOccurred())
			logAreaProvider, err := createProvider(ctx, environmentRequest.Spec.Providers.LogArea.ID, "log-area")
			Expect(err).NotTo(HaveOccurred())
			By("ensuring that the iut, executor and log areas exist")
			iut, err := createIut(ctx, iutProvider, environmentRequest, "iut")
			Expect(err).NotTo(HaveOccurred())
			executor, err := createExecutor(ctx, executorProvider, environmentRequest, "execution-space")
			Expect(err).NotTo(HaveOccurred())
			logArea, err := createLogArea(ctx, logAreaProvider, environmentRequest, "log-area")
			Expect(err).NotTo(HaveOccurred())
			By("creating the custom resource for the Kind Environment")
			err = k8sClient.Get(ctx, typeNamespacedName, environment)
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
						Providers: &etosv1alpha1.Providers{
							ExecutionSpace: executor.Name,
							LogArea:        logArea.Name,
							IUT:            iut.Name,
						},
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
				Expect(controllerutil.SetControllerReference(environmentRequest, resource, k8sClient.Scheme())).To(Succeed())
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &etosv1alpha1.Environment{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Environment")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			By("Cleaning up the iut, executor and log area providers")
			Expect(k8sClient.Delete(ctx, &etosv1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "iut",
					Namespace: "default",
				},
			})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &etosv1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "execution-space",
					Namespace: "default",
				},
			})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &etosv1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "log-area",
					Namespace: "default",
				},
			})).To(Succeed())
			By("Cleaning up the iut, executor and log areas")
			Expect(k8sClient.Delete(ctx, &etosv1alpha2.Iut{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "iut",
					Namespace: "default",
				},
			})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &etosv1alpha2.ExecutionSpace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "execution-space",
					Namespace: "default",
				},
			})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &etosv1alpha2.LogArea{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "log-area",
					Namespace: "default",
				},
			})).To(Succeed())
		})

		It("should successfully reconcile the custom resource for Environment", func() {
			By("Checking if the custom resource was successfully created")
			Eventually(func(g Gomega) {
				found := &etosv1alpha1.Environment{}
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, found)).To(Succeed())
			}).Should(Succeed())

			By("Checking that status conditions are initialized")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, environment)).To(Succeed())
				g.Expect(environment.Status.Conditions).NotTo(BeEmpty())
			}).Should(Succeed())

			By("Checking that the status condition 'Active' is set to True with reason 'Completed'")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, environment)).To(Succeed())
				active := meta.FindStatusCondition(environment.Status.Conditions, status.StatusActive)
				g.Expect(active).NotTo(BeNil())
				g.Expect(active).NotTo(HaveValue(Equal(metav1.Condition{}))) // Not empty
				g.Expect(active.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(active.Reason).To(Equal(status.ReasonCompleted))
			}).Should(Succeed())

			By("Checking that the finalizer is added to the resource")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, environment)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(environment, releaseFinalizer)).To(BeTrue())
			}).Should(Succeed())

			By("Setting the deadline to trigger the timeout logic in the controller")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, environment)).To(Succeed())
				environment.Spec.Deadline = time.Now().Unix()
				g.Expect(k8sClient.Update(ctx, environment)).To(Succeed())
			}).Should(Succeed())

			By("Checking that the status condition 'Active' is set to False with reason 'TimedOut'")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, environment)).To(Succeed())
				active := meta.FindStatusCondition(environment.Status.Conditions, status.StatusActive)
				g.Expect(active).NotTo(BeNil())
				g.Expect(active).NotTo(HaveValue(Equal(metav1.Condition{}))) // Not empty
				g.Expect(active.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(active.Reason).To(Equal(status.ReasonTimedOut))
			}).Should(Succeed())
		})
	})
})

// createProvider is a helper function to create a Provider resource with the specified name and type.
func createProvider(ctx context.Context, name, providerType string) (client.Object, error) {
	provider := &etosv1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: etosv1alpha1.ProviderSpec{
			Type:  providerType,
			Image: "example.com/provider-image:latest",
		},
	}
	return provider, k8sClient.Create(ctx, provider)
}

// createIut is a helper function to create an Iut resource with the specified name and owner.
func createIut(ctx context.Context, owner client.Object, environmentRequest *etosv1alpha1.EnvironmentRequest, name string) (*etosv1alpha2.Iut, error) {
	iut := &etosv1alpha2.Iut{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				"etos.eiffel-community.github.io/environment-request-id": environmentRequest.Spec.ID,
				"etos.eiffel-community.github.io/environment-request":    environmentRequest.Spec.Name,
				"etos.eiffel-community.github.io/provider":               owner.GetName(),
			},
		},
		Spec: etosv1alpha2.IutSpec{
			ID:                 uuid.NewString(),
			ProviderID:         owner.GetName(),
			EnvironmentRequest: environmentRequest.Name,
			Identity:           "pkg:example/iut",
		},
	}
	if err := controllerutil.SetControllerReference(owner, iut, k8sClient.Scheme()); err != nil {
		return iut, err
	}
	return iut, k8sClient.Create(ctx, iut)
}

// createExecutor is a helper function to create an ExecutionSpace resource with the specified name and owner.
func createExecutor(ctx context.Context, owner client.Object, environmentRequest *etosv1alpha1.EnvironmentRequest, name string) (*etosv1alpha2.ExecutionSpace, error) {
	executor := &etosv1alpha2.ExecutionSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				"etos.eiffel-community.github.io/environment-request-id": environmentRequest.Spec.ID,
				"etos.eiffel-community.github.io/environment-request":    environmentRequest.Spec.Name,
				"etos.eiffel-community.github.io/provider":               owner.GetName(),
			},
		},
		Spec: etosv1alpha2.ExecutionSpaceSpec{
			ID:                 uuid.NewString(),
			ProviderID:         owner.GetName(),
			EnvironmentRequest: environmentRequest.Name,
			TestRunner:         "ghcr.io/eiffel-community/etos-base-test-runner:bullseye",
			Instructions: etosv1alpha2.Instructions{
				Identifier:  uuid.NewString(),
				Image:       "ghcr.io/eiffel-community/etos-base-test-runner:bullseye",
				Parameters:  map[string]string{},
				Environment: map[string]string{},
			},
		},
	}
	if err := controllerutil.SetControllerReference(owner, executor, k8sClient.Scheme()); err != nil {
		return executor, err
	}
	return executor, k8sClient.Create(ctx, executor)
}

// createLogArea is a helper function to create a LogArea resource with the specified name and owner.
func createLogArea(ctx context.Context, owner client.Object, environmentRequest *etosv1alpha1.EnvironmentRequest, name string) (*etosv1alpha2.LogArea, error) {
	logArea := &etosv1alpha2.LogArea{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				"etos.eiffel-community.github.io/environment-request-id": environmentRequest.Spec.ID,
				"etos.eiffel-community.github.io/environment-request":    environmentRequest.Spec.Name,
				"etos.eiffel-community.github.io/provider":               owner.GetName(),
			},
		},
		Spec: etosv1alpha2.LogAreaSpec{
			ID:                 uuid.NewString(),
			ProviderID:         owner.GetName(),
			EnvironmentRequest: environmentRequest.Name,
			LiveLogs:           "http://example.com/live-logs",
			Logs:               map[string]string{},
			Upload: etosv1alpha2.Upload{
				Method: "GET",
				URL:    "http://example.com/upload",
			},
		},
	}
	if err := controllerutil.SetControllerReference(owner, logArea, k8sClient.Scheme()); err != nil {
		return logArea, err
	}
	return logArea, k8sClient.Create(ctx, logArea)
}

// createEnvironmentRequest is a helper function to create an EnvironmentRequest resource with the specified name.
func createEnvironmentRequest(ctx context.Context, name string) (*etosv1alpha1.EnvironmentRequest, error) {
	environmentRequest := &etosv1alpha1.EnvironmentRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: etosv1alpha1.EnvironmentRequestSpec{
			Image: &etosv1alpha1.Image{
				Image: "ghcr.io/eiffel-community/etos-environment-provider:60bf50aa",
			},
			MinimumAmount: 1,
			MaximumAmount: 1,
			Providers: etosv1alpha1.EnvironmentProviders{
				IUT:     etosv1alpha1.IutProvider{ID: "iut"},
				LogArea: etosv1alpha1.LogAreaProvider{ID: "log-area"},
				ExecutionSpace: etosv1alpha1.ExecutionSpaceProvider{
					ID:         "execution-space",
					TestRunner: "ghcr.io/eiffel-community/etos-base-test-runner:bullseye",
				},
			},
			Splitter: etosv1alpha1.Splitter{
				Tests: []etosv1alpha1.Test{},
			},
		},
	}
	return environmentRequest, k8sClient.Create(ctx, environmentRequest)
}
