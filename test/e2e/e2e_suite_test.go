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
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/eiffel-community/etos/test/utils"
)

const environmentProviderImage = "example.com/environmentprovider"

var (
	runTag          string
	managerImage    string
	managerManifest string
	crdManifest     string
	// shouldCleanupCertManager tracks whether CertManager was installed by this suite.
	shouldCleanupCertManager = false
	// reuseCluster indicates if the cluster and controller should be reused across runs.
	// Set E2E_REUSE=true to enable this behavior.
	reuseCluster = os.Getenv("E2E_REUSE") == "true"
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
	providers := []struct{ target, defaults, image string }{
		{"iutprovider-docker", "iut_provider.yaml", "example.com/iutprovider"},
		{"logareaprovider-docker", "log_area_provider.yaml", "example.com/logareaprovider"},
		{"executionspaceprovider-docker", "execution_space_provider.yaml", "example.com/executionspaceprovider"},
		{"environmentprovider-docker", "environment_provider.yaml", environmentProviderImage},
	}

	By("staging the E2E configuration")
	root, err := utils.GetProjectDir()
	Expect(err).NotTo(HaveOccurred())
	stage, err := os.MkdirTemp(root, ".e2e-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, stage)
	runTag = strings.TrimPrefix(filepath.Base(stage), ".")
	managerImage = "example.com/etos:" + runTag
	defaultsDir := filepath.Join(stage, "defaults")
	Expect(os.CopyFS(defaultsDir, os.DirFS(filepath.Join(root, "defaults")))).To(Succeed())
	for _, provider := range providers {
		Expect(utils.UpdateServiceDefaultVersion(filepath.Join(defaultsDir, provider.defaults), provider.image, runTag)).
			To(Succeed())
	}
	prepareManifests(root, stage)

	By("building the manager image")
	cmd := exec.Command("make", "docker-build", "IMG="+managerImage,
		"EXTRA_DOCKER_ARGS=--build-arg=DEFAULTS_PATH="+filepath.Join(filepath.Base(stage), "defaults"))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager image")

	By("loading the manager image on Kind")
	err = utils.LoadImageToKindClusterWithName(managerImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager image into Kind")

	for _, provider := range providers {
		image := provider.image + ":" + runTag
		By("building " + image)
		_, err := utils.Run(exec.Command("make", provider.target, "IMG="+image))
		Expect(err).NotTo(HaveOccurred())
		By("loading " + image + " into Kind")
		Expect(utils.LoadImageToKindClusterWithName(image)).To(Succeed())
	}

	setupCertManager()
})

func prepareManifests(root, stage string) {
	kustomize := os.Getenv("KUSTOMIZE")
	if kustomize == "" {
		kustomize = filepath.Join(root, "bin", "kustomize")
	} else if !filepath.IsAbs(kustomize) && strings.ContainsRune(kustomize, filepath.Separator) {
		kustomize = filepath.Join(root, kustomize)
	}

	configDir := filepath.Join(stage, "config")
	Expect(os.CopyFS(configDir, os.DirFS(filepath.Join(root, "config")))).To(Succeed())
	cmd := exec.Command(kustomize, "edit", "set", "image", "controller="+managerImage)
	cmd.Dir = filepath.Join(configDir, "manager")
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	crdManifest = filepath.Join(stage, "crds.yaml")
	managerManifest = filepath.Join(stage, "manager.yaml")
	for _, render := range []struct{ config, output string }{
		{"crd", crdManifest},
		{"default", managerManifest},
	} {
		_, err := utils.Run(exec.Command(kustomize, "build", filepath.Join(configDir, render.config), "-o", render.output))
		Expect(err).NotTo(HaveOccurred())
	}
}

var _ = AfterSuite(func() {
	if reuseCluster {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping suite cleanup (E2E_REUSE=true)\n")
		return
	}
	teardownCertManager()
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
