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
	"strings"
	"time"

	"github.com/eiffel-community/etos/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const Inconclusive = "Inconclusive"
const Failed = "Failed"

// VerifyETOSTestruns runs tests to verify ETOS testrun functionality.
func VerifyETOSTestruns() {
	Context("ETOS Testruns", func() {
		AfterAll(func() {
			By("cleaning up the artifact injector pod")
			cmd := exec.Command("kubectl", "delete", "pod", "artifact-injector", "--namespace", clusterNamespace)
			_, _ = utils.Run(cmd)
			By("cleaning up the ETOS testruns")
			cmd = exec.Command("kubectl", "delete", "testrun", "testrun-sample", "-n", clusterNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "testrun", "testrun-sample-multi-suite", "-n", clusterNamespace)
			_, _ = utils.Run(cmd)

			By("removing finalizers from EnvironmentRequests and Environments")
			cmd = exec.Command("kubectl", "get", "environmentrequests", "-o", "custom-columns=:metadata.name")
			output, _ := utils.Run(cmd)
			for name := range strings.SplitSeq(output, "\n") {
				if name == "" {
					continue
				}
				cmd := exec.Command(
					"kubectl", "patch", "environmentrequest", name, "--patch",
					"{\"metadata\": {\"finalizers\": []}}",
				)
				_, _ = utils.Run(cmd)
			}
			cmd = exec.Command("kubectl", "get", "environments", "-o", "custom-columns=:metadata.name")
			output, _ = utils.Run(cmd)
			for name := range strings.SplitSeq(output, "\n") {
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
		})

		AfterEach(func() {
			specReport := CurrentSpecReport()
			if specReport.Failed() {
				By("Fetching environment provider pods")
				cmd := exec.Command("kubectl", "describe", "pods", "-n", clusterNamespace,
					"-l", "app.kubernetes.io/name=environment-provider")
				podOutput, err := utils.Run(cmd)
				if err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Testrun description:\n %s", podOutput)
				} else {
					_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get testrun description: %s", err)
				}

				By("Fetching suite runner pods")
				cmd = exec.Command("kubectl", "describe", "pods", "-n", clusterNamespace,
					"-l", "app.kubernetes.io/name=suite-runner")
				podOutput, err = utils.Run(cmd)
				if err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Testrun description:\n %s", podOutput)
				} else {
					_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get testrun description: %s", err)
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
		})

		It("should be able to execute a v1alpha testrun", func() {
			By("creating a testrun")
			cmd := exec.Command("kubectl", "create", "-n", clusterNamespace, "-f", testRunSample)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create a testrun")

			By("checking the status field of the testrun")
			verifyTestRun := func(g Gomega) error {
				cmd := exec.Command("kubectl", "get",
					"testrun", "testrun-sample", "-o", "jsonpath={.status.verdict}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				switch output {
				case Failed:
					return StopTrying("TestRun failed")
				case Inconclusive:
					return StopTrying("TestRun became inconclusive")
				}
				g.Expect(output).To(Equal("Passed"), "TestRun did not become inactive")
				return nil
			}
			Eventually(verifyTestRun, "5m").Should(Succeed())
		})

		It("should be able to execute a v1alpha multi-suite testrun", func() {
			By("creating a testrun")
			cmd := exec.Command("kubectl", "create", "-n", clusterNamespace, "-f", multiSuiteTestRunSample)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create a multi-suite testrun")

			By("waiting for finished")
			verifyTestRun := func(g Gomega) error {
				cmd := exec.Command("kubectl", "get",
					"testrun", "testrun-sample-multi-suite", "-o", "jsonpath={.status.verdict}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				switch output {
				case Failed:
					return StopTrying("TestRun failed")
				case Inconclusive:
					return StopTrying("TestRun became inconclusive")
				}
				g.Expect(output).To(Equal("Passed"), "TestRun did not become inactive")
				return nil
			}
			Eventually(verifyTestRun, "5m").Should(Succeed())
		})

		It("should be able to execute a v1alpha multi-testrunner testrun", func() {
			By("creating a testrun")
			cmd := exec.Command("kubectl", "create", "-n", clusterNamespace, "-f", multiTestrunnerTestRunSample)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create a multi-testrunner testrun")

			By("waiting for finished")
			verifyTestRun := func(g Gomega) error {
				cmd := exec.Command("kubectl", "get",
					"testrun", "testrun-sample-multi-testrunner", "-o", "jsonpath={.status.verdict}",
					"-n", clusterNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				switch output {
				case Failed:
					return StopTrying("TestRun failed")
				case Inconclusive:
					return StopTrying("TestRun became inconclusive")
				}
				g.Expect(output).To(Equal("Passed"), "TestRun did not become inactive")
				return nil
			}
			Eventually(verifyTestRun, "5m").Should(Succeed())
		})
	})
}
