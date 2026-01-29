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

package v1alpha1

import (
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProviderSpec defines the desired state of Provider
type ProviderSpec struct {
	// +kubebuilder:validation:Enum=execution-space;iut;log-area
	Type string `json:"type"`

	// Image describes the docker image to run when providing a resource.
	Image string `json:"image"`

	// Env and EnvFrom describe environment variables to be passed to the provider container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// IutProviderConfig describe the configuration for an IUT provider.
	// +optional
	IutProviderConfig *IutProviderConfig `json:"iutProviderConfig,omitempty"`

	// LogAreaProviderConfig describe the configuration for an log area provider.
	// +optional
	LogAreaProviderConfig *LogAreaProviderConfig `json:"logAreaProviderConfig,omitempty"`

	// ExecutionSpaceProviderConfig describe the configuration for an execution space provider.
	// +optional
	ExecutionSpaceProviderConfig *ExecutionSpaceProviderConfig `json:"executionSpaceProviderConfig,omitempty"`
}

// IutProviderConfig describe the configuration for an IUT provider.
type IutProviderConfig struct {
	// The configuration of a provider is very implementation-specific and we cannot give
	// a perfectly generic configuration for all cases. The following field allows any
	// data-structure to be added to this configuration and it is expected that providers
	// can handle the data they require themselves.
	apiextensionsv1.JSON `json:",inline"`
}

// LogAreaProviderConfig describe the configuration for an log area provider.
type LogAreaProviderConfig struct {
	// The configuration of a provider is very implementation-specific and we cannot give
	// a perfectly generic configuration for all cases. The following field allows any
	// data-structure to be added to this configuration and it is expected that providers
	// can handle the data they require themselves.
	apiextensionsv1.JSON `json:",inline"`

	// LiveLogs is a URI to where live logs of an execution can be found.
	// +kubebuilder:validation:Format="uri"
	LiveLogs string `json:"livelogs"`

	// Upload defines the log upload instructions for the ETR.
	Upload etosv1alpha2.Upload `json:"upload"`
}

// ExecutionSpaceProviderConfig describe the configuration for an execution space provider.
type ExecutionSpaceProviderConfig struct {
	// The configuration of a provider is very implementation-specific and we cannot give
	// a perfectly generic configuration for all cases. The following field allows any
	// data-structure to be added to this configuration and it is expected that providers
	// can handle the data they require themselves.
	apiextensionsv1.JSON `json:",inline"`

	// Dev describes whether or not this provider should run the ETR in dev mode.
	// While using dev mode the ETR can be installed from github using ETRBranch and ETRRepository.
	// +kubebuilder:default="false"
	// +optional
	Dev string `json:"dev"`

	// ETRBranch describes a git branch to use when running the ETR in dev mode.
	// Can be used in conjunction with ETRRepository to test a fork, otherwise the
	// ETRRepository defaults to github.com/eiffel-community/etos.
	// +optional
	ETRBranch string `json:"ETR_BRANCH,omitempty"`

	// ETRRepository describes the git repository to fetch an ETR from when running in
	// dev mode. Defaults to github.com/eiffel-community/etos
	// +optional
	ETRRepository string `json:"ETR_REPOSITORY,omitempty"`
}

// ProviderStatus defines the observed state of Provider
type ProviderStatus struct {
	Conditions          []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	LastHealthCheckTime *metav1.Time       `json:"lastHealthCheckTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:conversion:hub
// +kubebuilder:subresource:status

// Provider is the Schema for the providers API
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=".status.conditions[?(@.type==\"Available\")].status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type==\"Available\")].message"
type Provider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderSpec   `json:"spec,omitempty"`
	Status ProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderList contains a list of Provider
type ProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Provider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Provider{}, &ProviderList{})
}
