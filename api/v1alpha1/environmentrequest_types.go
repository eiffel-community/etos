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
	// TestRunnerImage describes the container image to run in an execution space
	TestRunnerImage string `json:"testrunnerImage"`
}

type EnvironmentProviders struct {
	IUT            IutProvider            `json:"iut,omitempty"`
	ExecutionSpace ExecutionSpaceProvider `json:"executionSpace,omitempty"`
	LogArea        LogAreaProvider        `json:"logArea,omitempty"`
}

type Splitter struct {
	Tests []Test `json:"tests"`
}

// EnvironmentProviderJobConfig defines parameters required by environment provider job
type EnvironmentProviderJobConfig struct {
	EiffelMessageBus  RabbitMQ `json:"eiffelMessageBus"`
	EtosMessageBus    RabbitMQ `json:"etosMessageBus"`
	EtosApi           string   `json:"etosApi"`
	EncryptionKey     Var      `json:"encryptionKeySecretRef"`
	EtcdHost          string   `json:"etcdHost"`
	EtcdPort          string   `json:"etcdPort"`
	GraphQlServer     string   `json:"graphQlServer"`
	RoutingKeyTag     string   `json:"routingKeyTag"`
	TestRunnerVersion string   `json:"testRunnerVersion"`

	EnvironmentProviderEventDataTimeout string `json:"environmentProviderEventDataTimeout"`
	EnvironmentProviderServiceAccount   string `json:"environmentProviderServiceAccount"`
	EnvironmentProviderTestSuiteTimeout string `json:"environmentProviderTestSuiteTimeout"`
}

// EnvironmentRequestSpec defines the desired state of EnvironmentRequest
type EnvironmentRequestSpec struct {
	// ID is the ID for the environments generated. Will be generated if nil. The ID is a UUID, any version, and regex matches that.
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`

	*Image        `json:",inline"`
	Identifier    string `json:"identifier,omitempty"`
	Artifact      string `json:"artifact,omitempty"`
	Identity      string `json:"identity,omitempty"`
	MinimumAmount int    `json:"minimumAmount"`
	MaximumAmount int    `json:"maximumAmount"`

	// Deadline is the end time, in unix epoch, before which the environment request shall have
	// finished before it is cancelled
	// If deadline is not set, then deadline is set to Now + Timeout(see below)
	// +optional
	Deadline int64 `json:"deadline"`

	// Timeout is the time, in seconds, the environment request is allowed to take.
	// If both timeout and deadline is set, then deadline takes precedence.
	// +optional
	// +kubebuilder:default=60
	Timeout int64 `json:"timeout"`

	// TODO: Dataset per provider?
	Dataset *apiextensionsv1.JSON `json:"dataset,omitempty"`

	Providers          EnvironmentProviders         `json:"providers"`
	Splitter           Splitter                     `json:"splitter"`
	ServiceAccountName string                       `json:"serviceaccountname,omitempty"`
	Config             EnvironmentProviderJobConfig `json:"Config,omitempty"`
}

// EnvironmentRequestStatus defines the observed state of EnvironmentRequest
type EnvironmentRequestStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	EnvironmentProviders []corev1.ObjectReference `json:"environmentProviders,omitempty"`

	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:conversion:hub
// +kubebuilder:subresource:status

// EnvironmentRequest is the Schema for the environmentrequests API
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].reason"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message"
// +kubebuilder:printcolumn:name="TestRun",type="string",JSONPath=.metadata.labels.etos\.eiffel-community\.github\.io/id
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
