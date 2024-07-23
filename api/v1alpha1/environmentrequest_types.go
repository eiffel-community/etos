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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnvironmentRequestSpec defines the desired state of EnvironmentRequest
type EnvironmentRequestSpec struct {
	TestRun                string `json:"testrun"`
	IUTProvider            string `json:"iut"`
	LogAreaProvider        string `json:"logArea"`
	ExecutionSpaceProvider string `json:"executionSpace"`
	*Image                 `json:",inline"`
}

// EnvironmentRequestStatus defines the observed state of EnvironmentRequest
type EnvironmentRequestStatus struct {
	ActiveProviders []corev1.ObjectReference `json:"provider,omitempty"`
	Conditions      []metav1.Condition       `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
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
