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

package v1alpha2

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IutSpec defines the desired state of Iut
type IutSpec struct {
	// ID is the ID for the IUT. The ID is a UUID, any version, and regex matches that.
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	// +required
	ID string `json:"id"`

	// Identity is the PackageURL definition of the IUT.
	// +required
	Identity string `json:"identity"`

	// EnvironmentRequest is the name of the environmentrequest which requested this IUT.
	// +required
	EnvironmentRequest string `json:"environmentRequest"`

	// ProviderID is the name of the Provider used to create this Iut.
	// +required
	ProviderID string `json:"provider_id"`

	// ProviderData is specific data provided by the IUT providers
	// +optional
	ProviderData *apiextensionsv1.JSON `json:"provider_data,omitempty"`
}

// IutStatus defines the observed state of Iut.
type IutStatus struct {
	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Iut resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Iut is the Schema for the iuts API
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].reason"
// +kubebuilder:printcolumn:name="Description",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].message"
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=.spec.provider_id
// +kubebuilder:printcolumn:name="Environment",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Environment\")].name"
// +kubebuilder:printcolumn:name="TestRun",type="string",JSONPath=.metadata.labels.etos\.eiffel-community\.github\.io/id
type Iut struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Iut
	// +required
	Spec IutSpec `json:"spec"`

	// status defines the observed state of Iut
	// +optional
	Status IutStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// IutList contains a list of Iut
type IutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Iut `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Iut{}, &IutList{})
}
