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
	// managerImage is the manager image to be built and loaded for testing.
	managerImage               = "example.com/etos:v0.0.1"
	iutImage                   = "example.com/iutprovider"
	iutVersion                 = "v0.0.1"
	executionSpaceImage        = "example.com/executionspaceprovider"
	executionSpaceVersion      = "v0.0.1"
	logAreaImage               = "example.com/logareaprovider"
	logAreaVersion             = "v0.0.1"
	environmentProviderImage   = "example.com/environmentprovider"
	environmentProviderVersion = "v0.0.1"
	// shouldCleanupCertManager tracks whether CertManager was installed by this suite.
	shouldCleanupCertManager = false
	// shouldCleanupPrometheus tracks whether Prometheus was installed by this suite.
	shouldCleanupPrometheus = false
)

// TestE2E runs the e2e test suite to validate the solution in an isolated environment.
// The default setup requires Kind and CertManager.
//
// To skip CertManager installation, set: CERT_MANAGER_INSTALL_SKIP=true
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting etos e2e test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("patching the environment provider image in the default configuration")
	err := utils.UpdateServiceDefaultVersion("environment_provider", environmentProviderImage, environmentProviderVersion)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to update the provider image in the default configuration")
	By("patching the iut provider image in the default configuration")
	err = utils.UpdateServiceDefaultVersion("iut_provider", iutImage, iutVersion)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to update the provider image in the default configuration")
	By("patching the log area provider image in the default configuration")
	err = utils.UpdateServiceDefaultVersion("log_area_provider", logAreaImage, logAreaVersion)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to update the provider image in the default configuration")
	By("patching the execution space provider image in the default configuration")
	err = utils.UpdateServiceDefaultVersion("execution_space_provider", executionSpaceImage, executionSpaceVersion)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to update the provider image in the default configuration")

	By("building the manager image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", managerImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager image")

	By("loading the manager image on Kind")
	err = utils.LoadImageToKindClusterWithName(managerImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager image into Kind")

	By("building the iut provider")
	cmd = exec.Command("make", "iutprovider-docker", fmt.Sprintf("IMG=%s:%s", iutImage, iutVersion))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the IUT provider")

	By("loading the IUT provider image on kind")
	err = utils.LoadImageToKindClusterWithName(fmt.Sprintf("%s:%s", iutImage, iutVersion))
	ExpectWithOffset(1, err).NotTo(HaveOccurred(),
		"Failed to load the IUT provider image",
	)

	By("building the log area provider")
	cmd = exec.Command("make", "logareaprovider-docker", fmt.Sprintf("IMG=%s:%s", logAreaImage, logAreaVersion))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the log area provider")

	By("loading the log area provider image on kind")
	err = utils.LoadImageToKindClusterWithName(fmt.Sprintf("%s:%s", logAreaImage, logAreaVersion))
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the log area provider image")

	By("building the execution space provider")
	cmd = exec.Command(
		"make", "executionspaceprovider-docker", fmt.Sprintf("IMG=%s:%s", executionSpaceImage, executionSpaceVersion),
	)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the execution space provider")

	By("loading the execution space provider image on kind")
	err = utils.LoadImageToKindClusterWithName(fmt.Sprintf("%s:%s", executionSpaceImage, executionSpaceVersion))
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the execution space provider image")

	By("building the environment provider")
	cmd = exec.Command("make", "environmentprovider-docker", fmt.Sprintf(
		"IMG=%s:%s", environmentProviderImage, environmentProviderVersion,
	))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the environment provider")

	By("loading the environment provider image on kind")
	err = utils.LoadImageToKindClusterWithName(
		fmt.Sprintf("%s:%s", environmentProviderImage, environmentProviderVersion),
	)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the environment provider image")

	setupCertManager()
	setupPrometheusOperator()
})

var _ = AfterSuite(func() {
	teardownCertManager()
	teardownPrometheusOperator()
})

// setupCertManager installs CertManager if needed for webhook tests.
// Skips installation if CERT_MANAGER_INSTALL_SKIP=true or if already present.
func setupCertManager() {
	if os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true" {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager installation (CERT_MANAGER_INSTALL_SKIP=true)\n")
		return
	}

	By("checking if CertManager is already installed")
	if utils.IsCertManagerCRDsInstalled() {
		_, _ = fmt.Fprintf(GinkgoWriter, "CertManager is already installed. Skipping installation.\n")
		return
	}

	// Mark for cleanup before installation to handle interruptions and partial installs.
	shouldCleanupCertManager = true

	By("installing CertManager")
	Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
}

// setupPrometheusOperator installs PrometheusOperator if needed for monitoring tests.
// Skips installation if already present.
func setupPrometheusOperator() {
	By("checking if PrometheusOperator is already installed")
	if utils.IsPrometheusCRDsInstalled() {
		_, _ = fmt.Fprintf(GinkgoWriter, "PrometheusOperator is already installed. Skipping installation.\n")
		return
	}

	// Mark for cleanup before installation to handle interruptions and partial installs.
	shouldCleanupPrometheus = true

	By("installing PrometheusOperator")
	Expect(utils.InstallPrometheusOperator()).To(Succeed(), "Failed to install PrometheusOperator")
}

// teardownCertManager uninstalls CertManager if it was installed by setupCertManager.
// This ensures we only remove what we installed.
func teardownCertManager() {
	if !shouldCleanupCertManager {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager cleanup (not installed by this suite)\n")
		return
	}

	By("uninstalling CertManager")
	utils.UninstallCertManager()
}

// teardownPrometheusOperator uninstalls PrometheusOperator if it was installed by setupPrometheusOperator.
func teardownPrometheusOperator() {
	if !shouldCleanupPrometheus {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping PrometheusOperator cleanup (not installed by this suite)\n")
		return
	}
	By("uninstalling PrometheusOperator")
	utils.UninstallPrometheusOperator()
}
