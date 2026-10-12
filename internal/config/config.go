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
package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/eiffel-community/etos"
	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VersionLabel is the recommended kubernetes label for the version of an application.
const VersionLabel = "app.kubernetes.io/version"

var labelValue = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$`)

// VersionFromImage returns the tag of a container image reference.
// An empty string is returned if the image has no tag or if the tag is not a valid label value.
// A digest is ignored, so "repo:1.0@sha256:abc" results in "1.0".
func VersionFromImage(image string) string {
	image, _, _ = strings.Cut(image, "@")
	// A colon before the last slash is a registry port, not a tag.
	idx := strings.LastIndex(image, ":")
	if idx < 0 || idx < strings.LastIndex(image, "/") {
		return ""
	}
	tag := image[idx+1:]
	if len(tag) > 63 || !labelValue.MatchString(tag) {
		return ""
	}
	return tag
}

// SetVersionLabel sets the app.kubernetes.io/version label on meta from the tag of image.
// The label is left untouched if no version can be determined.
func SetVersionLabel(meta *metav1.ObjectMeta, image string) {
	version := VersionFromImage(image)
	if version == "" {
		return
	}
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	meta.Labels[VersionLabel] = version
}

// Service holds the configuration for a single ETOS service.
type Service struct {
	Image   string `yaml:"image,omitempty"`
	Version string `yaml:"version"`
}

// Config holds the configuration for all ETOS services.
type Config struct {
	API                    Service
	SSE                    Service
	LogArea                Service
	SuiteStarter           Service
	SuiteRunner            Service
	LogListener            Service
	TestRunner             Service
	EnvironmentProvider    Service
	IutProvider            Service
	LogAreaProvider        Service
	ExecutionSpaceProvider Service
	EventRepositoryAPI     Service
	EventRepositoryStorage Service
}

// New creates a new Config with default values.
func New() Config {
	return Config{
		API:                    loadDefaults("defaults/api.yaml"),
		SSE:                    loadDefaults("defaults/sse.yaml"),
		LogArea:                loadDefaults("defaults/log_area.yaml"),
		SuiteStarter:           loadDefaults("defaults/suite_starter.yaml"),
		SuiteRunner:            loadDefaults("defaults/suite_runner.yaml"),
		LogListener:            loadDefaults("defaults/log_listener.yaml"),
		TestRunner:             loadDefaults("defaults/test_runner.yaml"),
		EnvironmentProvider:    loadDefaults("defaults/environment_provider.yaml"),
		IutProvider:            loadDefaults("defaults/iut_provider.yaml"),
		LogAreaProvider:        loadDefaults("defaults/log_area_provider.yaml"),
		ExecutionSpaceProvider: loadDefaults("defaults/execution_space_provider.yaml"),
		EventRepositoryAPI:     loadDefaults("defaults/event_repository_api.yaml"),
		EventRepositoryStorage: loadDefaults("defaults/event_repository_storage.yaml"),
	}
}

// ImageOrDefault returns a Service with the image and version from the given Image if it is not nil.
// Otherwise, it returns the given default Service.
func ImageOrDefault(service Service, image etosv1alpha1.Image) string {
	if image.Image != "" {
		return image.Image
	}
	return fmt.Sprintf("%s:%s", service.Image, service.Version)
}

// VersionOrDefault returns the given version if it is not empty. Otherwise, it returns the version from the given Service.
func VersionOrDefault(service Service, version string) string {
	if version != "" {
		return version
	}
	return service.Version
}

// PullPolicyOrDefault returns the given pull policy if it is not empty. Otherwise, it returns PullIfNotPresent.
func PullPolicyOrDefault(service Service, image etosv1alpha1.Image) corev1.PullPolicy {
	if image.ImagePullPolicy != "" {
		return image.ImagePullPolicy
	}
	return corev1.PullIfNotPresent
}

// loadDefaults loads the default configuration for a service from a YAML file.
// Panics if the file cannot be read or unmarshalled.
func loadDefaults(file string) Service {
	data, err := etos.Defaults.ReadFile(file)
	if err != nil {
		panic(err)
	}
	var service Service
	if err = yaml.Unmarshal(data, &service); err != nil {
		panic(err)
	}
	return service
}
