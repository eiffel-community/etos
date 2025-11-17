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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/eiffel-community/etos/test/utils"
)

// namespace where the project is deployed in
const namespace = "etos-system"
const clusterNamespace = "etos-test"
const clusterName = "cluster-sample"

const clusterSample = "config/samples/etos_v1alpha1_cluster.yaml"
const testRunSample = "config/samples/etos_v1alpha1_testrun.yaml"
const multiSuiteTestRunSample = "config/samples/etos_v1alpha1_testrun_multi_suite.yaml"
const iutProviderSample = "config/samples/etos_v1alpha1_iut_provider.yaml"
const executionSpaceProviderSample = "config/samples/etos_v1alpha1_execution_space_provider.yaml"
const logAreaProviderSample = "config/samples/etos_v1alpha1_log_area_provider.yaml"

const executionSpaceProviderKustomization = "testdata/executionspace"
const iutProviderKustomization = "testdata/iut"

const goer = "testdata/goer.yaml"

const artifactID = "268dd4db-93da-4232-a544-bf4c0fb26dac"
const artifactIdentity = "pkg:testrun/etos/eiffel_community"

// serviceAccountName created for the project
const serviceAccountName = "etos-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "etos-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "etos-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("creating cluster namespace")
		cmd = exec.Command("kubectl", "create", "ns", clusterNamespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the execution space provider")
		cmd = exec.Command("kubectl", "create", "-k", executionSpaceProviderKustomization, "-n", clusterNamespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install execution space provider")

		By("deploying the IUT provider")
		cmd = exec.Command("kubectl", "create", "-k", iutProviderKustomization, "-n", clusterNamespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install IUT provider")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the etos testrun")
		cmd := exec.Command("kubectl", "delete", "--wait", "-n", clusterNamespace, "testrun/testrun-sample")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "--wait", "-n", clusterNamespace, "testrun/testrun-sample-multi-suite")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "--wait", "-n", clusterNamespace, "environmentrequests")
		_, _ = utils.Run(cmd)

		cmd = exec.Command("kubectl", "get", "environmentrequests", "-o", "custom-columns=:metadata.name")
		output, _ := utils.Run(cmd)
		for _, name := range strings.Split(output, "\n") {
			if name == "" {
				continue
			}
			cmd := exec.Command("kubectl", "patch", "environmentrequest", name, "--patch", "{\"metadata\": {\"finalizers\": []}}")
			_, _ = utils.Run(cmd)
		}
		cmd = exec.Command("kubectl", "get", "environments", "-o", "custom-columns=:metadata.name")
		output, _ = utils.Run(cmd)
		for _, name := range strings.Split(output, "\n") {
			if name == "" {
				continue
			}
			cmd := exec.Command("kubectl", "patch", "environment", name, "--patch", "{\"metadata\": {\"finalizers\": []}}")
			_, _ = utils.Run(cmd)
		}

		// This wait is necessary to make sure we clean up all resources before deleting the CRs that are
		// being used. If we don't delete them the tests won't pass since we'll get stuck waiting for the
		// namespace being deleted.
		// TODO: We need a better way of waiting here.
		time.Sleep(10 * time.Second)

		By("cleaning up the etos cluster")
		cmd = exec.Command("kubectl", "delete", "-n", clusterNamespace, "-f", clusterSample)
		_, _ = utils.Run(cmd)

		By("cleaning up the goer service")
		cmd = exec.Command("kubectl", "delete", "-n", clusterNamespace, "-f", goer)
		_, _ = utils.Run(cmd)

		By("cleaning up the etos IUT provider")
		cmd = exec.Command("kubectl", "delete", "-n", clusterNamespace, "-f", iutProviderSample)
		_, _ = utils.Run(cmd)

		By("cleaning up the etos log area provider")
		cmd = exec.Command("kubectl", "delete", "-n", clusterNamespace, "-f", logAreaProviderSample)
		_, _ = utils.Run(cmd)

		By("cleaning up the etos execution space provider")
		cmd = exec.Command("kubectl", "delete", "-n", clusterNamespace, "-f", executionSpaceProviderSample)
		_, _ = utils.Run(cmd)

		By("cleaning up the curl pod for metrics")
		cmd = exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the IUT provider")
		cmd = exec.Command("kubectl", "delete", "-k", iutProviderKustomization, "-n", clusterNamespace)
		_, _ = utils.Run(cmd)

		By("undeploying the execution space provider")
		cmd = exec.Command("kubectl", "delete", "-k", executionSpaceProviderKustomization, "-n", clusterNamespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing cluster namespace")
		cmd = exec.Command("kubectl", "delete", "ns", clusterNamespace)
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)

		By("removing metrics clusterrolebinding")
		cmd = exec.Command("kubectl", "delete", "clusterrolebinding", metricsRoleBindingName)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
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
				controllerPodName = podNames[0]
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

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=etos-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccount": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		It("should provisioned cert-manager", func() {
			By("validating that cert-manager has the certificate Secret")
			verifyCertManager := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secrets", "webhook-server-cert", "-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyCertManager).Should(Succeed())
		})

		It("should have CA injection for mutating webhooks", func() {
			By("checking CA injection for mutating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"mutatingwebhookconfigurations.admissionregistration.k8s.io",
					"etos-mutating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				mwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(mwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should have CA injection for validating webhooks", func() {
			By("checking CA injection for validating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"validatingwebhookconfigurations.admissionregistration.k8s.io",
					"etos-validating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				vwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should have CA injection for mutating webhooks", func() {
			By("checking CA injection for mutating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"mutatingwebhookconfigurations.admissionregistration.k8s.io",
					"etos-mutating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				mwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(mwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should have CA injection for Provider conversion webhook", func() {
			By("checking CA injection for Provider conversion webhook")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"customresourcedefinitions.apiextensions.k8s.io",
					"providers.etos.eiffel-community.github.io",
					"-o", "go-template={{ .spec.conversion.webhook.clientConfig.caBundle }}")
				vwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should have CA injection for TestRun conversion webhook", func() {
			By("checking CA injection for TestRun conversion webhook")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"customresourcedefinitions.apiextensions.k8s.io",
					"testruns.etos.eiffel-community.github.io",
					"-o", "go-template={{ .spec.conversion.webhook.clientConfig.caBundle }}")
				vwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should have CA injection for EnvironmentRequest conversion webhook", func() {
			By("checking CA injection for EnvironmentRequest conversion webhook")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"customresourcedefinitions.apiextensions.k8s.io",
					"environmentrequests.etos.eiffel-community.github.io",
					"-o", "go-template={{ .spec.conversion.webhook.clientConfig.caBundle }}")
				vwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		It("should be possible to deploy an ETOS execution space provider", func() {
			By("deploying the sample Execution space provider")
			cmd := exec.Command("kubectl", "create",
				"-f", executionSpaceProviderSample,
				"-n", clusterNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			By("checking the status field")
			verifyExecutionSpace := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"provider", "execution-space-provider-sample", "-o",
					"jsonpath={.status.conditions[?(@.type=='Available')].status}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Incorrect Provider status")
			}
			Eventually(verifyExecutionSpace).Should(Succeed())
		})
		It("should be possible to deploy an ETOS IUT provider", func() {
			By("deploying the sample IUT provider")
			cmd := exec.Command("kubectl", "create",
				"-f", iutProviderSample,
				"-n", clusterNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			By("checking the status field")
			verifyIut := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"provider", "iut-provider-sample", "-o", "jsonpath={.status.conditions[?(@.type=='Available')].status}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Incorrect Provider status")
			}
			Eventually(verifyIut).Should(Succeed())
		})

		It("should be possible to deploy an ETOS log area provider", func() {
			By("deploying the sample Log area provider")
			cmd := exec.Command("kubectl", "create",
				"-f", logAreaProviderSample,
				"-n", clusterNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			verifyLogArea := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"provider", "log-area-provider-sample", "-o", "jsonpath={.status.conditions[?(@.type=='Available')].status}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Incorrect Provider status")
			}
			Eventually(verifyLogArea).Should(Succeed())
		})

		It("should be possible to deploy an ETOS cluster", func() {
			By("deploying the sample Cluster")
			cmd := exec.Command("kubectl", "create",
				"-f", clusterSample,
				"-n", clusterNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should set cluster ready status true", func() {
			By("checking Status for Cluster")
			verifyClusterReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"cluster", clusterName, "-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Incorrect ETOS cluster status")
			}
			Eventually(verifyClusterReady).Should(Succeed())
		})
		It("should deploy ETOS SSE", func() {
			By("checking deployment status for ETOS SSE")
			etosSSEName := fmt.Sprintf("%s-etos-sse", clusterName)
			verifySSEReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"deploy", etosSSEName, "-o", "jsonpath={.status.readyReplicas}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Incorrect ETOS SSE deployment status")

			}
			Eventually(verifySSEReady).Should(Succeed())
		})
		It("should deploy ETOS LogArea", func() {
			By("checking deployment status for ETOS LogArea")
			etosLogAreaName := fmt.Sprintf("%s-etos-logarea", clusterName)
			verifyLogAreaReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"deploy", etosLogAreaName, "-o", "jsonpath={.status.readyReplicas}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Incorrect ETOS LogArea deployment status")
			}
			Eventually(verifyLogAreaReady).Should(Succeed())
		})
		It("should deploy ETOS SuiteStarter", func() {
			By("checking deployment status for ETOS SuiteStarter")
			etosSuiteStarterName := fmt.Sprintf("%s-etos-suite-starter", clusterName)
			verifySuiteStarterReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"deploy", etosSuiteStarterName, "-o", "jsonpath={.status.readyReplicas}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Incorrect ETOS SuiteStarter deployment status")

			}
			Eventually(verifySuiteStarterReady).Should(Succeed())
		})
		It("should deploy ETOS Messagebus", func() {
			By("checking statefulset status for ETOS Messagebus")
			etosMessageBusName := fmt.Sprintf("%s-messagebus", clusterName)
			verifyMessageBusReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"statefulset", etosMessageBusName, "-o", "jsonpath={.status.readyReplicas}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Incorrect ETOS MessageBus statefulset status")
			}
			Eventually(verifyMessageBusReady).Should(Succeed())
		})
		It("should deploy ETOS RabbitMQ", func() {
			By("checking statefulset status for ETOS RabbitMQ")
			etosRabbitMQName := fmt.Sprintf("%s-rabbitmq", clusterName)
			verifyRabbitMQReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"statefulset", etosRabbitMQName, "-o", "jsonpath={.status.readyReplicas}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Incorrect ETOS RabbitMQ statefulset status")
			}
			Eventually(verifyRabbitMQReady).Should(Succeed())
		})
		It("should deploy ETOS EventRepository", func() {
			By("checking deployment status for ETOS EventRepository")
			etosEventRepositoryName := fmt.Sprintf("%s-graphql", clusterName)
			verifyEventRepositoryReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"deploy", etosEventRepositoryName, "-o", "jsonpath={.status.readyReplicas}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Incorrect ETOS GraphQL statefulset status")
			}
			Eventually(verifyEventRepositoryReady).Should(Succeed())
		})
		It("should deploy ETOS API", func() {
			By("checking deployment status for ETOS API")
			etosAPIName := fmt.Sprintf("%s-etos-api", clusterName)
			verifyAPIReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"deploy", etosAPIName, "-o", "jsonpath={.status.readyReplicas}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Incorrect ETOS API deployment status")
			}
			Eventually(verifyAPIReady).Should(Succeed())
		})
		It("should deploy ETOS database", func() {
			By("checking statefulset status for database")
			etosDatabaseName := fmt.Sprintf("%s-etcd", clusterName)
			verifyDatabaseReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"statefulset", etosDatabaseName, "-o", "jsonpath={.status.readyReplicas}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("3"), "Incorrect ETOS Database statefulset status")
			}
			Eventually(verifyDatabaseReady).Should(Succeed())
		})

		It("should deploy Goer for execution space provider", func() {
			By("applying the yaml file")
			cmd := exec.Command("kubectl", "create", "-n", clusterNamespace, "-f", goer)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create a goer deployment")
			verifyGoerReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"deploy", "goer", "-o", "jsonpath={.status.readyReplicas}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Incorrect Eiffel Goer deployment status")
			}
			Eventually(verifyGoerReady).Should(Succeed())
		})
		cmd := "from eiffel_graphql_api.graphql.db.database import insert_to_db;" +
			"from eiffellib.events import EiffelArtifactCreatedEvent;" +
			fmt.Sprintf("event = EiffelArtifactCreatedEvent(); event.meta.event_id = '%s';", artifactID) +
			fmt.Sprintf("event.data.identity = '%s';insert_to_db(event);", artifactIdentity)
		It("should prepare test environment", func() {
			By("injecting a fake artifact to test")
			cmd := exec.Command("kubectl", "run", "artifact-injector", "--restart=Never",
				"--namespace", clusterNamespace,
				"--image=ghcr.io/eiffel-community/eiffel-graphql-storage:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "test",
							"image": "ghcr.io/eiffel-community/eiffel-graphql-storage:latest",
					    "envFrom": [{"secretRef": {"name": "cluster-sample-graphql"}}],
							"command": ["python", "-c"],
							"args": ["%s"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}]
					}
				}`, cmd))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to inject artifact to eventrepository")

			// TODO: Remove when fixed: https://github.com/eiffel-community/etos/issues/408
			By("creating a generic encryption key")
			cmd = exec.Command("kubectl", "create", "secret", "-n", clusterNamespace, "generic",
				"etos-encryption-key", "--from-literal", "ETOS_ENCRYPTION_KEY=ZmgcW2Qz43KNJfIuF0vYCoPneViMVyObH4GR8R9JE4g=")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create an encryption key secret")
			// Getting an EOF error every now and then from the environment-provider.
			// I think it is because ETCD reports that it is up, but it is not ready
			// to accept connections. A wait for ETCD to respond is a better fix.
			time.Sleep(30 * time.Second)
		})

		It("should be able to execute a v1alpha testrun", func() {
			// TODO: This testrun could create a v0 testrun and wait for it to complete, as a way to test
			// both ETOS versions.
			By("creating a testrun")
			cmd := exec.Command("kubectl", "create", "-n", clusterNamespace, "-f", testRunSample)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create a testrun")

			By("checking the status field of the testrun")
			verifyTestRun := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"testrun", "testrun-sample", "-o", "jsonpath={.status.verdict}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Passed"), "TestRun did not become inactive")
			}
			Eventually(verifyTestRun, "5m").Should(Succeed())
		})

		It("should be able to execute a v1alpha multi-suite testrun", func() {
			By("creating a testrun")
			cmd := exec.Command("kubectl", "create", "-n", clusterNamespace, "-f", multiSuiteTestRunSample)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create a multi-suite testrun")

			By("waiting for finished")
			verifyTestRun := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"testrun", "testrun-sample-multi-suite", "-o", "jsonpath={.status.verdict}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Passed"), "TestRun did not become inactive")
			}
			Eventually(verifyTestRun, "5m").Should(Succeed())
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
