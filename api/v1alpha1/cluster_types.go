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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MongoDB struct {
	// +kubebuilder:default=false
	// +optional
	Deploy bool `json:"deploy"`
	// Ignored if deploy is true
	// +kubebuilder:default={"value": "mongodb://root:password@mongodb:27017/admin"}
	// +optional
	URI Var `json:"uri"`
}

type EventRepository struct {
	// Deploy a local event repository for a cluster.
	// +kubebuilder:default=false
	// +optional
	Deploy bool `json:"deploy"`

	// We do not build the GraphQL API automatically nor publish it remotely.
	// This will need to be provided to work.
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/eiffel-graphql-api:latest"}
	// +optional
	API Image `json:"api"`

	// We do not build the GraphQL API automatically nor publish it remotely.
	// This will need to be provided to work.
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/eiffel-graphql-storage:latest"}
	// +optional
	Storage Image `json:"storage"`

	// +kubebuilder:default={}
	// +optional
	Database MongoDB `json:"mongo"`
	// +kubebuilder:default="eventrepository"
	// +optional
	Host string `json:"host"`
	// +kubebuilder:default={}
	// +optional
	Ingress Ingress `json:"ingress"`
}

type MessageBus struct {
	// +kubebuilder:default={"queueName": "etos"}
	// +optional
	EiffelMessageBus RabbitMQ `json:"eiffel"`
	// +kubebuilder:default={"queueName": "etos-*-temp"}
	// +optional
	ETOSMessageBus RabbitMQ `json:"logs"`
}

type Etcd struct {
	// Parameter is ignored if Deploy is set to true.
	// +kubebuilder:default="etcd-client"
	// +optional
	Host string `json:"host"`
	// Parameter is ignored if Deploy is set to true.
	// +kubebuilder:default="2379"
	// +optional
	Port string `json:"port"`
}

type Database struct {
	// +kubebuilder:default=true
	// +optional
	Deploy bool `json:"deploy"`
	// +kubebuilder:default={}
	// +optional
	Etcd Etcd `json:"etcd"`
}

type ETOSAPI struct {
	Image `json:",inline"`
}

type ETOSSSE struct {
	Image `json:",inline"`
}

type ETOSLogArea struct {
	Image `json:",inline"`
}

type ETOSSuiteRunner struct {
	Image       `json:",inline"`
	LogListener Image `json:"logListener"`
}

type ETOSTestRunner struct {
	Version string `json:"version"`
}

type ETOSEnvironmentProvider struct {
	Image `json:",inline"`
}

type ETOSConfig struct {
	// +kubebuilder:default="true"
	// +optional
	Dev string `json:"dev"`
	// +kubebuilder:default="60"
	// +optional
	EventDataTimeout string `json:"eventDataTimeout"`
	// +kubebuilder:default="10"
	// +optional
	TestSuiteTimeout string `json:"testSuiteTimeout"`
	// +kubebuilder:default="3600"
	// +optional
	EnvironmentTimeout string `json:"environmentTimeout"`
	// +kubebuilder:default="ETOS"
	// +optional
	Source string `json:"source"`

	// +optional
	TestRunRetention Retention `json:"testrunRetention,omitempty"`

	// +kubebuilder:default={"value": ""}
	EncryptionKey Var `json:"encryptionKey"`

	ETOSApiURL             string `json:"etosApiURL,omitempty"`
	ETOSEventRepositoryURL string `json:"etosEventRepositoryURL,omitempty"`

	// +kubebuilder:default="etos"
	// +optional
	RoutingKeyTag string `json:"routingKeyTag"`

	Timezone string `json:"timezone,omitempty"`
}

type ETOS struct {
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/etos-api:latest"}
	// +optional
	API ETOSAPI `json:"api"`
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/etos-sse:latest"}
	// +optional
	SSE ETOSSSE `json:"sse"`
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/etos-log-area:latest"}
	// +optional
	LogArea ETOSLogArea `json:"logArea"`
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/etos-suite-runner:latest", "logListener": {"image": "registry.nordix.org/eiffel/etos-log-listener:latest"}}
	// +optional
	SuiteRunner ETOSSuiteRunner `json:"suiteRunner"`
	// +kubebuilder:default={"version": "latest"}
	// +optional
	TestRunner ETOSTestRunner `json:"testRunner"`
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/etos-environment-provider:latest"}
	// +optional
	EnvironmentProvider ETOSEnvironmentProvider `json:"environmentProvider"`
	Ingress             Ingress                 `json:"ingress,omitempty"`
	// +kubebuilder:default={"encryptionKey": {"value": ""}}
	// +optional
	Config ETOSConfig `json:"config"`
}

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	// +kubebuilder:default={}
	ETOS ETOS `json:"etos"`
	// +kubebuilder:default={}
	Database Database `json:"database"`
	// +kubebuilder:default={}
	MessageBus MessageBus `json:"messageBus"`
	// +kubebuilder:default={}
	EventRepository EventRepository `json:"eventRepository"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Cluster is the Schema for the clusters API
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message"
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
