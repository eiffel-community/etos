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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogAreaSpec defines the desired state of LogArea
type LogAreaSpec struct {
	// ID is the ID for the LogArea. The ID is a UUID, any version, and regex matches that.
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	// +required
	ID string `json:"id"`

	// EnvironmentRequest is the ID of the environmentrequest which requested this log area.
	// +required
	EnvironmentRequest string `json:"environmentRequest"`

	// ProviderID is the name of the Provider used to create this LogArea.
	// +required
	ProviderID string `json:"provider_id"`

	// LiveLogs is a URI to where live logs of an execution can be found.
	// +kubebuilder:validation:Type="string"
	// +kubebuilder:validation:Format="uri"
	// +required
	LiveLogs string `json:"livelogs"`

	// Logs provides a map of special instructions for the ETR to apply on the files it creates.
	// Example instructions include
	// - prepend: prepend a string to log file name.
	// - join_character: a character to join 'prepend' and log file name.
	// +required
	Logs map[string]string `json:"logs"`

	// Upload defines the log upload instructions for the ETR.
	// +required
	Upload Upload `json:"upload"`
}

type Upload struct {
	// AsJSON tells a client that the payload should be posted as JSON.
	AsJSON bool `json:"as_json"`

	// Auth defines authorization instructions for a request.
	// +optional
	Auth *Auth `json:"auth,omitempty"`

	// URL defines the HTTP(s) URL to send payload to.
	// +kubebuilder:validation:Type="string"
	// +kubebuilder:validation:Format="uri"
	URL string `json:"url"`

	// Method defines the HTTP method for an upload.
	// +kubebuilder:validation:Enum=GET;POST;PUT
	Method string `json:"method"`

	// Headers define optional headers to send with the payload
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// Params define optional parameters to send with the payload
	// +optional
	Params map[string]string `json:"params,omitempty"`
}

// Auth defines authorization instructions for a request.
type Auth struct {
	// Username defines the username to use when authenticating
	Username string `json:"username,omitempty"`
	// Password defines an encrypted password to use when authenticating
	Password Decrypt `json:"password,omitempty"`
	// AuthType defines the type of authentication to do.
	// +kubebuilder:validation:Enum=basic;digest
	AuthType string `json:"type,omitempty"`
}

// Decrypt defines decryption instructions for clients.
type Decrypt struct {
	// Decrypt defines a JsonTas style json for decrypting a value
	Decrypt DecryptValue `json:"$decrypt"`
}

// DecryptValue defines a value to decrypt.
type DecryptValue struct {
	// Value defines an encrypted string to decrypt
	Value string `json:"value"`
}

// LogAreaStatus defines the observed state of LogArea.
type LogAreaStatus struct {
	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the LogArea resource.
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

	// CompletionTime defines the time that a LogArea was completed.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// LogArea is the Schema for the logarea API
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].reason"
// +kubebuilder:printcolumn:name="Description",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].message"
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=.spec.provider_id
// +kubebuilder:printcolumn:name="Environment",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Environment\")].name"
// +kubebuilder:printcolumn:name="TestRun",type="string",JSONPath=.metadata.labels.etos\.eiffel-community\.github\.io/id
type LogArea struct {
	// TypeMeta describes an individual object in an API response or request
	// with strings representing the type of the object and its API schema version.
	// Structures that are versioned or persisted should inline TypeMeta.
	metav1.TypeMeta `json:",inline"`

	// ObjectMeta is metadata that all persisted resources must have, which includes all objects
	// users must create.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of LogArea
	// +required
	Spec LogAreaSpec `json:"spec"`

	// status defines the observed state of LogArea
	// +optional
	Status LogAreaStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// LogAreaList contains a list of LogArea
type LogAreaList struct {
	// TypeMeta describes an individual object in an API response or request
	// with strings representing the type of the object and its API schema version.
	// Structures that are versioned or persisted should inline TypeMeta.
	metav1.TypeMeta `json:",inline"`
	// ListMeta describes metadata that synthetic resources must have, including lists and
	// various status objects. A resource may have only one of {ObjectMeta, ListMeta}.
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items defines a slice of LogAreas when listing.
	Items []LogArea `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LogArea{}, &LogAreaList{})
}
