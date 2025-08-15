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

// OpenTelemetry describes a deployment of an opentelemetry collector for ETOS to use.
type OpenTelemetry struct {
	// Enable opentelemetry support, adding the environment variables to services.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled"`

	// Sets the OTEL_EXPORTER_OTLP_ENDPOINT environment variable
	// +kubebuilder:default="http://localhost:4317"
	// +optional
	Endpoint string `json:"endpoint"`

	// Sets the OTEL_EXPORTER_OTLP_INSECURE environment variable
	// +kubebuilder:default="true"
	// +optional
	Insecure string `json:"insecure"`
}

// MessageBus describes the deployment of messagesbuses for ETOS.
type MessageBus struct {
	// +optional
	EiffelMessageBus RabbitMQ `json:"eiffel"`
	// +optional
	ETOSMessageBus RabbitMQ `json:"logs"`
}

// Etcd describes the deployment of an ETCD database. Ignored if Deploy is set to false.
type Etcd struct {
	// Host specifies the ETCD server hostname.
	// +kubebuilder:default="etcd-client"
	// +optional
	Host string `json:"host"`
	// Port specifies the ETCD port number.
	// +kubebuilder:default="2379"
	// +optional
	Port string `json:"port"`
	// Resources describes compute resource requirements per etcd pod which are three in a cluster.
	// +kubebuilder:default={"limits": {"cpu": "300m", "memory": "768Mi"}, "requests": {"cpu": "300m", "memory": "768Mi"}}
	// +optional
	Resources EtcdResources `json:"resources"`
}

// EtcdResources describes compute resource requirements for Etcd.
type EtcdResources struct {
	// Limits describes the maximum amount of compute resources allowed.
	// +kubebuilder:default={"cpu": "300m", "memory": "768Mi"}
	// +optional
	Limits EtcdResourceList `json:"limits"`

	// Requests describes the minimum amount of compute resources required.
	// +kubebuilder:default={"cpu": "300m", "memory": "768Mi"}
	// +optional
	Requests EtcdResourceList `json:"requests"`
}

// EtcdResourceList describes CPU and memory resources.
type EtcdResourceList struct {
	// CPU resource.
	// +kubebuilder:default="300m"
	// +optional
	CPU string `json:"cpu"`

	// Memory resource.
	// +kubebuilder:default="768Mi"
	// +optional
	Memory string `json:"memory"`
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
	// The provider secrets are necessary in order deploy and run ETOS without using the
	// kubernetes controller.
	// They can be removed from here when the suite starter is no longer in use.
	// +optional
	IUTProviderSecret string `json:"iutProviderSecret"`
	// +optional
	ExecutionSpaceProviderSecret string `json:"executionSpaceProviderSecret"`
	// +optional
	LogAreaProviderSecret string `json:"logAreaProviderSecret"`
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`
}

// ETOSSuiteStarterConfig describes the configuration required for a suite starter.
// This is separate from the ETOSConfig as we want to remove this in the future when the suite
// starter is no longer in use.
type ETOSSuiteStarterConfig struct {
	// +kubebuilder:default="3600"
	// +optional
	TTL string `json:"ttl,omitempty"`
	// +kubebuilder:default="300"
	// +optional
	GracePeriod string `json:"gracePeriod,omitempty"`
	// +kubebuilder:default=""
	// +optional
	SidecarImage string `json:"sidecarImage,omitempty"`
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
	SuiteRunnerTemplateSecretName string `json:"suiteRunnerTemplateSecretName,omitempty"`
	// +kubebuilder:default={"ttl": "3600", "gracePeriod": "300"}
	// +optional
	Config ETOSSuiteStarterConfig `json:"config"`
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`
}

// ETOSSSE describes th deployment of an ETOS Server Sent Events API.
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
	// +kubebuilder:default={"image": "ghcr.io/eiffel-community/etos-api:latest"}
	// +optional
	API ETOSAPI `json:"api"`
	// +kubebuilder:default={"image": "ghcr.io/eiffel-community/etos-sse:latest"}
	// +optional
	SSE ETOSSSE `json:"sse"`
	// +kubebuilder:default={"image": "ghcr.io/eiffel-community/etos-log-area:latest"}
	// +optional
	LogArea ETOSLogArea `json:"logArea"`
	// +kubebuilder:default={"image": "ghcr.io/eiffel-community/etos-suite-runner:latest", "logListener": {"image": "ghcr.io/eiffel-community/etos-log-listener:latest"}}
	// +optional
	SuiteRunner ETOSSuiteRunner `json:"suiteRunner"`
	// +kubebuilder:default={"version": "latest"}
	// +optional
	TestRunner ETOSTestRunner `json:"testRunner"`
	// +kubebuilder:default={"image": "ghcr.io/eiffel-community/etos-suite-starter:latest"}
	// +optional
	SuiteStarter ETOSSuiteStarter `json:"suiteStarter"`
	// +kubebuilder:default={"image": "ghcr.io/eiffel-community/etos-environment-provider:latest"}
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
	// +kubebuilder:default={}
	OpenTelemetry OpenTelemetry `json:"openTelemetry"`
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
