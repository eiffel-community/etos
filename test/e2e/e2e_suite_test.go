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
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/eiffel-community/etos/test/utils"
)

var (
	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if CertManager is already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false
	isPrometheusAlreadyInstalled  = false

	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/etos:v0.0.1"

	iutImage                 = "example.com/iutprovider:v0.0.1"
	executionSpaceImage      = "example.com/executionspaceprovider:v0.0.1"
	logAreaImage             = "example.com/logareaprovider:v0.0.1"
	environmentProviderImage = "example.com/environmentprovider:v0.0.1"
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purposed to be used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting etos integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the manager(Operator) image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	By("building the iut provider")
	cmd = exec.Command("make", "iutprovider-docker", fmt.Sprintf("IMG=%s", iutImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the IUT provider")

	By("loading the IUT provider image on kind")
	err = utils.LoadImageToKindClusterWithName(iutImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(),
		"Failed to load the IUT provider image",
	)

	By("building the log area provider")
	cmd = exec.Command("make", "logareaprovider-docker", fmt.Sprintf("IMG=%s", logAreaImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the log area provider")

	By("loading the log area provider image on kind")
	err = utils.LoadImageToKindClusterWithName(logAreaImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the log area provider image")

	By("building the execution space provider")
	cmd = exec.Command("make", "executionspaceprovider-docker", fmt.Sprintf("IMG=%s", executionSpaceImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the execution space provider")

	By("loading the execution space provider image on kind")
	err = utils.LoadImageToKindClusterWithName(executionSpaceImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the execution space provider image")

	By("building the environment provider")
	cmd = exec.Command("make", "environmentprovider-docker", fmt.Sprintf("IMG=%s", environmentProviderImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the environment provider")

	By("loading the environment provider image on kind")
	err = utils.LoadImageToKindClusterWithName(environmentProviderImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the environment provider image")

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for its presence before execution.
	// Setup CertManager before the suite if not skipped and if not already installed
	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = utils.IsCertManagerCRDsInstalled()
		isPrometheusAlreadyInstalled = utils.IsPrometheusCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
			Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
		}
	}
	if !isPrometheusAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Installing PrometheusOperator...\n")
		Expect(utils.InstallPrometheusOperator()).To(Succeed(), "Failed to install PrometheusOperator")
	} else {
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: PrometheusOperator is already installed. Skipping installation...\n")
	}
})

var _ = AfterSuite(func() {
	// Teardown CertManager after the suite if not skipped and if it was not already installed
	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}
	if !isPrometheusAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling PrometheusOperator...\n")
		utils.UninstallPrometheusOperator()
	}
})
