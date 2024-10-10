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

// MongoDB describes the deployment of a MongoDB.
type MongoDB struct {
	// +kubebuilder:default=false
	// +optional
	Deploy bool `json:"deploy"`
	// Ignored if deploy is true
	// +kubebuilder:default={"value": "mongodb://root:password@mongodb:27017/admin"}
	// +optional
	URI Var `json:"uri"`
}

// EventRepository describes the deployment of an event repository for ETOS.
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
	// +kubebuilder:default="etos"
	// +optional
	EiffelQueueName string `json:"eiffelQueueName,omitempty"`
	// +kubebuilder:default=""
	// +optional
	EiffelQueueParams string `json:"eiffelQueueParams,omitempty"`

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

// MessageBus describes the deployment of messagesbuses for ETOS.
type MessageBus struct {
	// +optional
	EiffelMessageBus RabbitMQ `json:"eiffel"`
	// +optional
	ETOSMessageBus RabbitMQ `json:"logs"`
}

// Etcd describes the deployment of an ETCD database.
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

// Database describes the deployment of a database for ETOS.
type Database struct {
	// +kubebuilder:default=true
	// +optional
	Deploy bool `json:"deploy"`
	// +kubebuilder:default={}
	// +optional
	Etcd Etcd `json:"etcd"`
}

// ETOSAPI describes the deployment of the ETOS API.
type ETOSAPI struct {
	Image `json:",inline"`
	// The provider secrets are necessary in order to run ETOS the old way and not using the controller.
	// They can be removed from here when the suite starter is no longer in use.
	// +optional
	IUTProviderSecret string `json:"iutProviderSecret"`
	// +optional
	ExecutionSpaceProviderSecret string `json:"executionSpaceProviderSecret"`
	// +optional
	LogAreaProviderSecret string `json:"logAreaProviderSecret"`
}

// ETOSSuiteStarterConfig describes the configuration required for a suite starter.
// This is separate from the ETOSConfig as we want to remove this in the future when the suite
// starter is no longer in use.
type ETOSSuiteStarterConfig struct {
	// +kubebuilder:default="3600"
	// +optional
	TTL string `json:"ttl,omitempty"`
	// +kubebuilder:default=""
	// +optional
	ObservabilityConfigmapName string `json:"observabilityConfigmapName,omitempty"`
	// +kubebuilder:default="300"
	// +optional
	GracePeriod string `json:"gracePeriod,omitempty"`
	// +kubebuilder:default=""
	// +optional
	SidecarImage string `json:"sidecarImage,omitempty"`
	// +kubebuilder:default=""
	// +optional
	OTELCollectorHost string `json:"otelCollectorHost,omitempty"`
}

// ETOSSuiteStarter describes the deployment of an ETOS suite starter.
type ETOSSuiteStarter struct {
	Image `json:",inline"`
	// +kubebuilder:default="etos-suite-starter"
	// +optional
	EiffelQueueName string `json:"eiffelQueueName,omitempty"`
	// +kubebuilder:default=""
	// +optional
	EiffelQueueParams string `json:"eiffelQueueParams,omitempty"`
	// Provide a custom suite runner template.
	// +kubebuilder:default=""
	// +optional
	SuiteRunnerTemplateConfigmapName string `json:"suiteRunnerTemplateConfigmapName,omitempty"`
	// +kubebuilder:default={"ttl": "3600", "gracePeriod": "300"}
	// +optional
	Config ETOSSuiteStarterConfig `json:"config"`
}

// ETOSSSE describes th deployment of an ETOS SSE API.
type ETOSSSE struct {
	Image `json:",inline"`
}

// ETOSLogArea describes th deployment of an ETOS log area API.
type ETOSLogArea struct {
	Image `json:",inline"`
}

// ETOSLogListener describes the deployment of an ETOS log listener.
type ETOSLogListener struct {
	Image `json:",inline"`
	// +kubebuilder:default="etos-*-temp"
	// +optional
	ETOSQueueName string `json:"etosQueueName,omitempty"`
	// +kubebuilder:default=""
	// +optional
	ETOSQueueParams string `json:"etosQueueParams,omitempty"`
}

// ETOSSuiteRunner describes the deployment of an ETOS suite runner.
type ETOSSuiteRunner struct {
	Image       `json:",inline"`
	LogListener ETOSLogListener `json:"logListener"`
}

// ETOSTestRunner describes the deployment of an ETOS test runner.
type ETOSTestRunner struct {
	Version string `json:"version"`
}

// ETOSEnvironmentProvider describes the deployment of an ETOS environment provider.
type ETOSEnvironmentProvider struct {
	Image `json:",inline"`
}

// ETOSConfig describes a common configuration for ETOS.
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

// ETOS describes the deployment of an ETOS cluster.
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
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/etos-suite-starter:latest"}
	// +optional
	SuiteStarter ETOSSuiteStarter `json:"suiteStarter"`
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
