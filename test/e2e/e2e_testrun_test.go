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
			for _, name := range strings.Split(output, "\n") {
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
			for _, name := range strings.Split(output, "\n") {
				if name == "" {
					continue
				}
				cmd := exec.Command("kubectl", "patch", "environment", name, "--patch", "{\"metadata\": {\"finalizers\": []}}")
				_, _ = utils.Run(cmd)
			}

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

			By("undeploying the IUT provider")
			cmd = exec.Command("kubectl", "delete", "-k", iutProviderKustomization, "-n", clusterNamespace)
			_, _ = utils.Run(cmd)

			By("undeploying the execution space provider")
			cmd = exec.Command("kubectl", "delete", "-k", executionSpaceProviderKustomization, "-n", clusterNamespace)
			_, _ = utils.Run(cmd)
			// This wait is necessary to make sure we clean up all resources before deleting the CRs that are
			// being used. If we don't delete them the tests won't pass since we'll get stuck waiting for the
			// namespace being deleted.
			// TODO: We need a better way of waiting here.
			time.Sleep(10 * time.Second)
		})

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
}
