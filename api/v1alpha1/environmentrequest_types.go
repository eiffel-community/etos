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
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IutProvider struct {
	ID string `json:"id"`
}

type LogAreaProvider struct {
	ID string `json:"id"`
}

type ExecutionSpaceProvider struct {
	ID         string `json:"id"`
	TestRunner string `json:"testRunner"`
}

type EnvironmentProviders struct {
	IUT            IutProvider            `json:"iut,omitempty"`
	ExecutionSpace ExecutionSpaceProvider `json:"executionSpace,omitempty"`
	LogArea        LogAreaProvider        `json:"logArea,omitempty"`
}

type Splitter struct {
	Tests []Test `json:"tests"`
}

// EnvironmentRequestSpec defines the desired state of EnvironmentRequest
type EnvironmentRequestSpec struct {
	// ID is the ID for the environments generated. Will be generated if nil
	// +kubebuilder:validation:Pattern="[a-f0-9]{8}-?[a-f0-9]{4}-?4[a-f0-9]{3}-?[89ab][a-f0-9]{3}-?[a-f0-9]{12}"
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`

	*Image        `json:",inline"`
	Identifier    string `json:"identifier,omitempty"`
	Artifact      string `json:"artifact,omitempty"`
	Identity      string `json:"identity,omitempty"`
	MinimumAmount int    `json:"minimumAmount"`
	MaximumAmount int    `json:"maximumAmount"`
	// TODO: Dataset per provider?
	Dataset *apiextensionsv1.JSON `json:"dataset,omitempty"`

	Providers EnvironmentProviders `json:"providers"`
	Splitter  Splitter             `json:"splitter"`
}

// EnvironmentRequestStatus defines the observed state of EnvironmentRequest
type EnvironmentRequestStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	EnvironmentProviders []corev1.ObjectReference `json:"environmentProviders,omitempty"`

	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EnvironmentRequest is the Schema for the environmentrequests API
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].reason"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message"
type EnvironmentRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentRequestSpec   `json:"spec,omitempty"`
	Status EnvironmentRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EnvironmentRequestList contains a list of EnvironmentRequest
type EnvironmentRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnvironmentRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EnvironmentRequest{}, &EnvironmentRequestList{})
}