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

	"github.com/eiffel-community/etos/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// DeployVerifyCluster verifies that the ETOS cluster can be deployed and that all components are running correctly.
func DeployVerifyCluster() {
	Context("Cluster", func() {
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
	})
}
