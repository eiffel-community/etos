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

package e2e

import (
	"encoding/base64"
	"fmt"
	"os/exec"

	"github.com/eiffel-community/etos/internal/config"
	"github.com/eiffel-community/etos/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const defaultPullPolicy = "IfNotPresent"
const testPullPolicy = "Always"
const testImage = "test-image"

// VerifyClusterDefaults verifies that the cluster defaults are applied
// when optional image and version values are not set in the cluster spec,
// and that custom values are applied when set.
func VerifyClusterDefaults() {
	defaults := config.New()
	Context("Cluster defaults", func() {
		AfterAll(func() {
			By("setting back the controller image to the original")
			cmd := exec.Command("kubectl", "set", "image", "deploy/etos-controller-manager",
				fmt.Sprintf("manager=%s", projectImage), "-n", namespace)
			_, err := utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to set back the controller image")
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName := podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})
		services :=
			[]struct {
				name           string
				deploymentName string
				containerName  string
				path           string
				serviceDefault config.Service
				secretKey      string
				verifySecret   bool
			}{
				{
					name:           "event repository API",
					deploymentName: fmt.Sprintf("deploy/%s-graphql", clusterName),
					containerName:  fmt.Sprintf("%s-graphql-api", clusterName),
					path:           "/spec/eventRepository/api",
					serviceDefault: defaults.EventRepositoryAPI,
				},
				{
					name:           "event repository storage",
					deploymentName: fmt.Sprintf("deploy/%s-graphql", clusterName),
					containerName:  fmt.Sprintf("%s-graphql-storage", clusterName),
					path:           "/spec/eventRepository/storage",
					serviceDefault: defaults.EventRepositoryStorage,
				},
				{
					name:           "ETOS API",
					deploymentName: fmt.Sprintf("deploy/%s-etos-api", clusterName),
					containerName:  fmt.Sprintf("%s-etos-api", clusterName),
					path:           "/spec/etos/api",
					serviceDefault: defaults.API,
				},
				{
					name:           "ETOS SSE",
					deploymentName: fmt.Sprintf("deploy/%s-etos-sse", clusterName),
					containerName:  fmt.Sprintf("%s-etos-sse", clusterName),
					path:           "/spec/etos/sse",
					serviceDefault: defaults.SSE,
				},
				{
					name:           "ETOS LogArea",
					deploymentName: fmt.Sprintf("deploy/%s-etos-logarea", clusterName),
					containerName:  fmt.Sprintf("%s-etos-logarea", clusterName),
					path:           "/spec/etos/logArea",
					serviceDefault: defaults.LogArea,
				},
				{
					name:           "ETOS SuiteStarter",
					deploymentName: fmt.Sprintf("deploy/%s-etos-suite-starter", clusterName),
					containerName:  fmt.Sprintf("%s-etos-suite-starter", clusterName),
					path:           "/spec/etos/suiteStarter",
					serviceDefault: defaults.SuiteStarter,
				},
				{
					name:           "ETOS SuiteRunner",
					path:           "/spec/etos/suiteRunner",
					secretKey:      "SUITE_RUNNER_IMAGE",
					serviceDefault: defaults.SuiteRunner,
					verifySecret:   true,
				},
				{
					name:           "ETOS LogListener",
					path:           "/spec/etos/suiteRunner/logListener",
					secretKey:      "LOG_LISTENER_IMAGE",
					serviceDefault: defaults.LogListener,
					verifySecret:   true,
				},
				{
					name:           "ETOS EnvironmentProvider",
					path:           "/spec/etos/environmentProvider",
					secretKey:      "ENVIRONMENT_PROVIDER_IMAGE",
					serviceDefault: defaults.EnvironmentProvider,
					verifySecret:   true,
				},
				// ETR is handled separately below since it is not an image
			}
		It("shall deploy optional image values if they are added to the cluster spec", func() {
			for _, service := range services {
				By(fmt.Sprintf("patching the cluster with optional image value for the %s", service.name))
				patchVerify(service.path)
				By(fmt.Sprintf("checking if the image values are added to the %s", service.name))
				if service.verifySecret {
					waitVerifySecretValue(service.secretKey, testImage, testPullPolicy)
				} else {
					waitVerifyImageValues(
						service.deploymentName, service.containerName, testImage, testPullPolicy,
					)
				}
			}
			By("patching the cluster with optional version value for the ETOS TestRunner")
			cmd := exec.Command("kubectl", "patch", "cluster", clusterName, "-n", clusterNamespace,
				"--type=json", "-p", `[{"op":"replace","path":"/spec/etos/testRunner","value":{"version":"1.2.3"}}]`,
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to patch the cluster with image values")
			By("checking if the version value is added to the ETOS TestRunner")
			verifySecretValue := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", fmt.Sprintf("secret/%s-cfg", clusterName), "-n",
					clusterNamespace, "-o", fmt.Sprintf("jsonpath={.data.ETR_VERSION}"))
				secretOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secretOutput).To(Equal(base64.StdEncoding.EncodeToString([]byte("1.2.3"))), "Incorrect secret value in the deployment")
			}
			Eventually(verifySecretValue).Should(Succeed())
		})
		It("shall deploy defaults if optional image values are removed from cluster spec", func() {
			for _, service := range services {
				By(fmt.Sprintf("removing the optional image values from the cluster spec for the %s",
					service.name),
				)
				deleteVerify(service.path)
				By(fmt.Sprintf("checking if the default image values are set to the %s", service.name))
				if service.verifySecret {
					waitVerifySecretValue(service.secretKey,
						fmt.Sprintf("%s:%s",
							service.serviceDefault.Image, service.serviceDefault.Version,
						),
						defaultPullPolicy,
					)
				} else {
					waitVerifyImageValues(service.deploymentName, service.containerName,
						fmt.Sprintf("%s:%s",
							service.serviceDefault.Image, service.serviceDefault.Version,
						),
						defaultPullPolicy,
					)
				}
			}
			By("removing the optional version value from the cluster spec for the ETOS TestRunner")
			cmd := exec.Command("kubectl", "patch", "cluster", clusterName, "-n", clusterNamespace,
				"--type=json", "-p", `[{"op":"remove","path":"/spec/etos/testRunner/version"}]`,
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to patch the cluster with version value")
			By("checking if the default version value is added to the ETOS TestRunner")
			verifySecretValue := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", fmt.Sprintf("secret/%s-cfg", clusterName), "-n",
					clusterNamespace, "-o", fmt.Sprintf("jsonpath={.data.ETR_VERSION}"))
				secretOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secretOutput).To(Equal(base64.StdEncoding.EncodeToString([]byte(defaults.TestRunner.Version))), "Incorrect secret value in the deployment")
			}
			Eventually(verifySecretValue).Should(Succeed())
		})
		It("shall update images controller is updated with new defaults", func() {
			image := "example.com/etos-defaults:v1.0.0"
			By("building the manager(Operator) image")
			cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", image),
				"EXTRA_DOCKER_ARGS=--build-arg=DEFAULTS_PATH=testdata/defaults")
			_, err := utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")
			By("loading the manager(Operator) image on Kind")
			err = utils.LoadImageToKindClusterWithName(image)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")
			cmd = exec.Command("kubectl", "set", "image", "deploy/etos-controller-manager",
				fmt.Sprintf("manager=%s", image), "-n", namespace)
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to update the controller-manager image")

			waitVerifyControllerReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"deploy/etos-controller-manager", "-o", "jsonpath={.status.readyReplicas}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Incorrect controller-manager deployment status")
			}
			Eventually(waitVerifyControllerReady).Should(Succeed())

			By("checking if the new default image values are added to the deployments")

			for _, service := range services {
				if service.verifySecret {
					waitVerifySecretValue(service.secretKey,
						fmt.Sprintf("%s:1.2.3", testImage),
						defaultPullPolicy,
					)
				} else {
					waitVerifyImageValues(service.deploymentName, service.containerName,
						fmt.Sprintf("%s:1.2.3", testImage),
						defaultPullPolicy,
					)
				}
			}
		})
	})
}

// patchVerify patches the cluster with test image and pull policy at the given path in the spec.
func patchVerify(path string) {
	cmd := exec.Command("kubectl", "patch", "cluster", clusterName, "-n", clusterNamespace,
		"--type=json", "-p",
		fmt.Sprintf(
			`[{"op":"replace","path":"%s","value":{"image":"%s","imagePullPolicy":"%s"}}]`,
			path, testImage, testPullPolicy,
		),
	)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to patch the cluster with image values")
}

// deleteVerify removes the image and pull policy from the given path in the cluster spec.
func deleteVerify(path string) {
	cmd := exec.Command("kubectl", "patch", "cluster", clusterName, "-n", clusterNamespace,
		"--type=json", "-p", fmt.Sprintf(`[{"op":"remove","path":"%s/image"}]`, path))
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to remove image from the cluster")
	cmd = exec.Command("kubectl", "patch", "cluster", clusterName, "-n", clusterNamespace,
		"--type=json", "-p", fmt.Sprintf(`[{"op":"remove","path":"%s/imagePullPolicy"}]`, path))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to remove image policy from the cluster")
}

// waitVerifyImageValues waits and verifies that the given image name and pull policy
// are set in the given deployment and container.
func waitVerifyImageValues(name, containerName, imageName, pullPolicy string) {
	By("checking if the image values are added to the service")
	verifyImageValues := func(g Gomega) {
		cmd := exec.Command("kubectl", "get", name, "-o",
			fmt.Sprintf("jsonpath={.spec.template.spec.containers[?(@.name=='%s')].image}", containerName),
			"-n", clusterNamespace)
		imageOutput, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(imageOutput).To(Equal(imageName), "Incorrect image value in the deployment")
		cmd = exec.Command("kubectl", "get", name, "-o",
			fmt.Sprintf("jsonpath={.spec.template.spec.containers[?(@.name=='%s')].imagePullPolicy}", containerName),
			"-n", clusterNamespace)
		imagePullPolicyOutput, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(imagePullPolicyOutput).To(Equal(pullPolicy), "Incorrect image pull policy value in the deployment")
	}
	Eventually(verifyImageValues).Should(Succeed())
}

// waitVerifySecretValue waits and verifies that the given image name and pull policy
// are set in the cluster secret.
func waitVerifySecretValue(secretKey, imageName, pullPolicy string) {
	By("checking if the secret value is added to the service")
	verifySecretValue := func(g Gomega) {
		cmd := exec.Command("kubectl", "get", fmt.Sprintf("secret/%s-cfg", clusterName), "-n",
			clusterNamespace, "-o", fmt.Sprintf("jsonpath={.data.%s}", secretKey))
		secretOutput, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(secretOutput).To(Equal(base64.StdEncoding.EncodeToString([]byte(imageName))), "Incorrect secret value in the deployment")
		cmd = exec.Command("kubectl", "get", fmt.Sprintf("secret/%s-cfg", clusterName), "-n",
			clusterNamespace, "-o", fmt.Sprintf("jsonpath={.data.%s_PULL_POLICY}", secretKey))
		secretOutput, err = utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(secretOutput).To(Equal(base64.StdEncoding.EncodeToString([]byte(pullPolicy))), "Incorrect secret value in the deployment")
	}
	Eventually(verifySecretValue).Should(Succeed())
}
