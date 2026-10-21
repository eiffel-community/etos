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
const multiTestrunnerTestRunSample = "config/samples/etos_v1alpha1_testrun_multi_testrunner.yaml"
const v1beta1TestRunSample = "config/samples/etos_v1beta1_testrun.yaml"
const v1beta1MultiSuiteTestRunSample = "config/samples/etos_v1beta1_testrun_multi_suite.yaml"
const v1beta1MultiTestrunnerTestRunSample = "config/samples/etos_v1beta1_testrun_multi_testrunner.yaml"

const artifactID = "268dd4db-93da-4232-a544-bf4c0fb26dac"
const artifactIdentity = "pkg:testrun/etos/eiffel_community"

// collectorFixture is the isolated OTLP Collector used to verify exported telemetry.
const collectorFixture = "test/e2e/otel-collector.yaml"

// collectorDeployment is the name of the e2e Collector deployment.
const collectorDeployment = "e2e-otel-collector"

// collectorEndpoint is the in-cluster OTLP gRPC endpoint of the e2e Collector.
const collectorEndpoint = "http://e2e-otel-collector.etos-system.svc.cluster.local:4317"

var _ = Describe("Manager", Ordered, func() {

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	// When E2E_REUSE=true, idempotent commands are used so that existing resources
	// are reused rather than requiring a fresh setup every time.
	BeforeAll(func() {
		if reuseCluster {
			_, _ = fmt.Fprintf(GinkgoWriter, "E2E_REUSE=true: reusing existing cluster and controller if present\n")
		}

		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		if _, err := utils.Run(cmd); err != nil {
			// Namespace may already exist when reusing.
			cmd = exec.Command("kubectl", "get", "ns", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Manager namespace does not exist and could not be created")
		}

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("creating cluster namespace")
		cmd = exec.Command("kubectl", "create", "ns", clusterNamespace)
		if _, err = utils.Run(cmd); err != nil {
			// Namespace may already exist when reusing.
			cmd = exec.Command("kubectl", "get", "ns", clusterNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Cluster namespace does not exist and could not be created")
		}

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the isolated OTLP Collector fixture")
		cmd = exec.Command("kubectl", "apply", "-f", collectorFixture)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the OTLP Collector fixture")

		By("waiting for the OTLP Collector to be available")
		cmd = exec.Command("kubectl", "rollout", "status",
			"deployment/"+collectorDeployment, "-n", namespace, "--timeout=180s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "OTLP Collector did not become available")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("pointing the controller-manager at the OTLP Collector fixture")
		cmd = exec.Command("kubectl", "set", "env", "deployment/etos-controller-manager", "-n", namespace,
			"OTEL_EXPORTER_OTLP_ENDPOINT="+collectorEndpoint,
			"OTEL_EXPORTER_OTLP_INSECURE=true",
			"OTEL_METRIC_EXPORT_INTERVAL=5000",
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to configure OTLP export on the controller-manager")

		By("waiting for the OTLP-enabled controller-manager rollout to complete")
		cmd = exec.Command("kubectl", "rollout", "status",
			"deployment/etos-controller-manager", "-n", namespace, "--timeout=120s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Controller-manager rollout did not complete")

		if reuseCluster {
			By("annotating the cluster to trigger reconciliation")
			cmd = exec.Command("kubectl", "annotate", "cluster", clusterName,
				"-n", clusterNamespace,
				fmt.Sprintf("etos.eiffel-community.github.io/reconcile-trigger=%d", time.Now().Unix()),
				"--overwrite")
			// Ignore errors here — the cluster may not exist yet on first run.
			_, _ = utils.Run(cmd)
		}
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace. When E2E_REUSE=true, the controller and cluster infrastructure
	// are preserved so they can be reused in the next run.
	AfterAll(func() {
		if reuseCluster {
			_, _ = fmt.Fprintf(GinkgoWriter, "E2E_REUSE=true: preserving cluster, controller, CRDs and namespaces\n")

			return
		}

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
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events in %s:\n%s", namespace, eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events from %s: %s", namespace, err)
			}

			By("Fetching OTLP Collector fixture logs")
			cmd = exec.Command("kubectl", "logs", "deployment/"+collectorDeployment, "-n", namespace, "--tail=200")
			collectorOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Collector logs:\n %s", collectorOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Collector logs: %s", err)
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
	DeployVerifyController()
	DeployVerifyCluster()
	VerifyETOSTestruns()
	It("should receive completed provider spans and structured logs", func() {
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "logs", "deployment/"+collectorDeployment, "-n", namespace)
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("ResourceSpans #"))
			g.Expect(output).To(ContainSubstring("RunProvider"))
			g.Expect(output).To(ContainSubstring("etos.provider.completed"))
			g.Expect(output).To(ContainSubstring("duration_seconds"))
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
	})
	VerifyClusterDefaults()
})
