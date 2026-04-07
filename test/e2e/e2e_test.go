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
	"fmt"
	"os/exec"
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
		By("cleaning up the etos cluster")
		cmd := exec.Command("kubectl", "delete", "-n", clusterNamespace, "-f", clusterSample)
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
			By("Fetching controller manager pod name")
			controllerPodName, err := ControllerPodName()
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller pod name:\n %s", controllerPodName)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller pod name: %s", err)
			}

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

			By("Fetching testrun description")
			cmd = exec.Command("kubectl", "describe", "testruns", "-n", clusterNamespace)
			testrunOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Testrun description:\n %s", testrunOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get testrun description: %s", err)
			}

			By("Fetching testrun, environment and environmentrequests")
			cmd = exec.Command("kubectl", "get", "testruns,environments,environmentrequests", "-n", clusterNamespace)
			listOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Testruns, environments and environmentrequests:\n %s", listOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get testruns, environments and environmentrequests: %s", err)
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	DeployVerifyController()
	DeployVerifyCluster()
	VerifyETOSTestruns()
	VerifyClusterDefaults()
})
