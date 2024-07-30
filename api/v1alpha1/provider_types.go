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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JSONTasList is the List command in the JSONTas provider.
type JSONTasList struct {
	Possible  *apiextensionsv1.JSON `json:"possible"`
	Available *apiextensionsv1.JSON `json:"available"`
}

// Stage is the definition of a stage where to execute steps.
type Stage struct {
	Steps *apiextensionsv1.JSON `json:"steps"`
}

// JSONTasIUTPrepareaStages defines the preparation stages required for an IUT.
type JSONTasPrepareStages struct {
	// Underscore used in these due to backwards compatibility
	EnvironmentProvider Stage `json:"environment_provider"`
	SuiteRunner         Stage `json:"suite_runner"`
	TestRunner          Stage `json:"test_runner"`
}

// JSONTasIUTPrepare defines the preparation required for an IUT.
type JSONTasIUTPrepare struct {
	Stages JSONTasPrepareStages `json:"stages"`
}

// JSONTasIut is the IUT provider definition for the JSONTas provider.
type JSONTasIut struct {
	ID       string                `json:"id"`
	Checkin  *apiextensionsv1.JSON `json:"checkin,omitempty"`
	Checkout *apiextensionsv1.JSON `json:"checkout,omitempty"`
	List     JSONTasList           `json:"list"`
}

// JSONTasExecutionSpace is the execution space provider definition for the JSONTas provider
type JSONTasExecutionSpace struct {
	ID       string                `json:"id"`
	Checkin  *apiextensionsv1.JSON `json:"checkin,omitempty"`
	Checkout *apiextensionsv1.JSON `json:"checkout,omitempty"`
	List     JSONTasList           `json:"list"`
}

// JSONTasLogArea is the log area provider definition for the JSONTas provider
type JSONTasLogArea struct {
	ID       string                `json:"id"`
	Checkin  *apiextensionsv1.JSON `json:"checkin,omitempty"`
	Checkout *apiextensionsv1.JSON `json:"checkout,omitempty"`
	List     JSONTasList           `json:"list"`
}

// JSONTas defines the definitions that a JSONTas provider shall use.
type JSONTas struct {
	Image string `json:"image,omitempty"`
	// These are pointers so that they become nil in the Provider object in Kubernetes
	// and don't muddle up the yaml with empty data.
	Iut            *JSONTasIut            `json:"iut,omitempty"`
	ExecutionSpace *JSONTasExecutionSpace `json:"execution_space,omitempty"`
	LogArea        *JSONTasLogArea        `json:"log,omitempty"`
}

// Healthcheck defines the health check endpoint and interval for providers.
// The defaults of this should work most of the time.
type Healthcheck struct {
	// +kubebuilder:default=/v1alpha1/selftest/ping
	// +optional
	Endpoint string `json:"endpoint"`
	// +kubebuilder:default=30
	// +optional
	IntervalSeconds int `json:"intervalSeconds"`
}

// ProviderSpec defines the desired state of Provider
type ProviderSpec struct {
	// +kubebuilder:validation:Enum=execution-space;iut;log-area
	Type string `json:"type"`
	// +optional
	Host string `json:"host,omitempty"`

	// +kubebuilder:default={}
	// +optional
	Healthcheck *Healthcheck `json:"healthCheck,omitempty"`

	// These are pointers so that they become nil in the Provider object in Kubernetes
	// and don't muddle up the yaml with empty data.
	JSONTas       *JSONTas   `json:"jsontas,omitempty"`
	JSONTasSource *VarSource `json:"jsontasSource,omitempty"`
}

// ProviderStatus defines the observed state of Provider
type ProviderStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Provider is the Schema for the providers API
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=".status.conditions[?(@.type==\"Available\")].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"Available\")].reason"
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
