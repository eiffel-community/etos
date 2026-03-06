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

// +kubebuilder:validation:Required
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MongoDB describes the deployment of a MongoDB.
type MongoDB struct {
	// Deploy a local MongoDB for a cluster.
	// +kubebuilder:default=true
	// +optional
	Deploy bool `json:"deploy,omitempty"`
	// URI to an existing MongoDB instance. Required if Deploy is false.
	// +kubebuilder:default={"value": "mongodb://root:password@mongodb:27017/admin"}
	// +optional
	URI *Var `json:"uri,omitempty"`
}

// EventRepository describes the deployment of an event repository for ETOS.
type EventRepository struct {
	// Deploy a local event repository for a cluster.
	// +kubebuilder:default=true
	// +optional
	Deploy bool `json:"deploy,omitempty"`

	// API describes the image to use for the event repository API.
	// Defaults are used if not set.
	// +optional
	API Image `json:"api,omitempty"`
	// Storage describes the image to use for the event repository storage.
	// Defaults are used if not set.
	// +optional
	Storage Image `json:"storage"`

	// EiffelQueueName is the name of the queue to use for the event repository. Defaults to "etos".
	// +kubebuilder:default="etos"
	// +optional
	EiffelQueueName string `json:"eiffelQueueName,omitempty"`
	// EiffelQueueParams are the parameters to use for the event repository queue. Defaults to empty.
	// +optional
	EiffelQueueParams string `json:"eiffelQueueParams,omitempty"`

	// Database describes the database to use for the event repository. Defaults to a local MongoDB
	// if not set.
	// +kubebuilder:default={}
	// +optional
	Database MongoDB `json:"mongo,omitempty"`
	// Host specifies the event repository API URL. Required if deploy is false.
	// +optional
	Host string `json:"host,omitempty"`
	// Ingress describes the ingress to use for the event repository API.
	// +kubebuilder:default={}
	// +optional
	Ingress Ingress `json:"ingress,omitempty"`
}

// OpenTelemetry describes a deployment of an opentelemetry collector for ETOS to use.
type OpenTelemetry struct {
	// Enable opentelemetry support, adding the environment variables to services.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Sets the OTEL_EXPORTER_OTLP_ENDPOINT environment variable
	// +kubebuilder:default="http://localhost:4317"
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Sets the OTEL_EXPORTER_OTLP_INSECURE environment variable
	// +kubebuilder:default="true"
	// +optional
	Insecure string `json:"insecure,omitempty"`
}

// MessageBus describes the deployment of messagesbuses for ETOS.
type MessageBus struct {
	// EiffelMessageBus describes the message bus to use for Eiffel events. Defaults to a local
	// RabbitMQ if not set.
	// +kubebuilder:default={}
	// +optional
	EiffelMessageBus RabbitMQ `json:"eiffel,omitempty"`
	// ETOSMessageBus describes the message bus to use for ETOS internal communication. Defaults to
	// a local RabbitMQ if not set.
	// +kubebuilder:default={}
	// +optional
	ETOSMessageBus RabbitMQ `json:"logs,omitempty"`
}

// Etcd describes the deployment of an ETCD database. Ignored if Deploy is set to false.
type Etcd struct {
	// Host specifies the ETCD server hostname.
	// +kubebuilder:default="etcd-client"
	// +optional
	Host string `json:"host,omitempty"`
	// Port specifies the ETCD port number.
	// +kubebuilder:default="2379"
	// +optional
	Port string `json:"port,omitempty"`
	// Resources describes compute resource requirements per etcd pod which are three in a cluster.
	// +kubebuilder:default={}
	// +optional
	Resources EtcdResources `json:"resources,omitempty"`
}

// EtcdResources describes compute resource requirements for Etcd.
type EtcdResources struct {
	// Limits describes the maximum amount of compute resources allowed.
	// +kubebuilder:default={}
	// +optional
	Limits EtcdResourceList `json:"limits,omitempty"`

	// Requests describes the minimum amount of compute resources required.
	// +kubebuilder:default={}
	// +optional
	Requests EtcdResourceList `json:"requests,omitempty"`
}

// EtcdResourceList describes CPU and memory resources.
type EtcdResourceList struct {
	// CPU resource.
	// +kubebuilder:default="300m"
	// +optional
	CPU string `json:"cpu,omitempty"`

	// Memory resource.
	// +kubebuilder:default="768Mi"
	// +optional
	Memory string `json:"memory,omitempty"`
}

// Database describes the deployment of a database for ETOS.
type Database struct {
	// Deploy a local ETCD for a cluster.
	// +kubebuilder:default=true
	// +optional
	Deploy bool `json:"deploy,omitempty"`
	// Etcd describes the ETCD database to use for ETOS. Defaults to a local ETCD if Deploy is true.
	// +kubebuilder:default={}
	// +optional
	Etcd Etcd `json:"etcd,omitempty"`
}

// ETOSAPI describes the deployment of the ETOS API.
type ETOSAPI struct {
	// Image describes the image to use for the ETOS API. Defaults are used if not set.
	Image `json:",inline"`
	// The provider secrets are necessary in order deploy and run ETOS without using the
	// kubernetes controller.
	// They can be removed from here when the suite starter is no longer in use.
	// +optional
	IUTProviderSecret string `json:"iutProviderSecret,omitempty"`
	// +optional
	ExecutionSpaceProviderSecret string `json:"executionSpaceProviderSecret,omitempty"`
	// +optional
	LogAreaProviderSecret string `json:"logAreaProviderSecret,omitempty"`

	// Replicas describes the number of replicas to use for the ETOS API. Defaults to 1 if not set.
	// +optional
	Replicas *int32 `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`
}

// ETOSSuiteStarterConfig describes the configuration required for a suite starter.
// This is separate from the ETOSConfig as we want to remove this in the future when the suite
// starter is no longer in use.
type ETOSSuiteStarterConfig struct {
	// TTL describes the time to live for a suite runner job that the suite starter starts.
	// Defaults to 3600 seconds if not set.
	// +kubebuilder:default="3600"
	// +optional
	TTL string `json:"ttl,omitempty"`
	// GracePeriod describes the grace period for a suite runner job that the suite starter starts.
	// Defaults to 300 seconds if not set.
	// +kubebuilder:default="300"
	// +optional
	GracePeriod string `json:"gracePeriod,omitempty"`
	// SidecarImage describes the image to use for the suite runner sidecar.
	// Defaults to empty, which means no sidecar will be used.
	// +optional
	SidecarImage string `json:"sidecarImage,omitempty"`
}

// ETOSSuiteStarter describes the deployment of an ETOS suite starter.
type ETOSSuiteStarter struct {
	// Image describes the image to use for the Suite Starter. Defaults are used if not set.
	Image `json:",inline"`
	// EiffelQueueName is the name of the RabbitMQ queue to use for the suite starter to listen for
	// incoming Eiffel events. Defaults to "etos-suite-starter".
	// +kubebuilder:default="etos-suite-starter"
	// +optional
	EiffelQueueName string `json:"eiffelQueueName,omitempty"`
	// EiffelQueueParams are the parameters to use for the suite starter queue. Defaults to empty.
	// +optional
	EiffelQueueParams string `json:"eiffelQueueParams,omitempty"`
	// Provide a custom suite runner template.
	// +optional
	SuiteRunnerTemplateSecretName string `json:"suiteRunnerTemplateSecretName,omitempty"`
	// Config describes the configuration for the suite runner job started by the suite starter.
	// Defaults are used if not set.
	// +kubebuilder:default={}
	// +optional
	Config ETOSSuiteStarterConfig `json:"config,omitempty"`
	// Replicas describes the number of replicas to use for the Suite Starter.
	// Defaults to 1 if not set.
	// +optional
	Replicas *int32 `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`
}

// ETOSSSE describes th deployment of an ETOS Server Sent Events API.
type ETOSSSE struct {
	// Image describes the image to use for the ETOS SSE API. Defaults are used if not set.
	Image `json:",inline"`
}

// ETOSLogArea describes th deployment of an ETOS log area API.
type ETOSLogArea struct {
	// Image describes the image to use for the ETOS log area API. Defaults are used if not set.
	Image `json:",inline"`
}

// ETOSLogListener describes the deployment of an ETOS log listener.
type ETOSLogListener struct {
	// Image describes the image to use for the ETOS log listener. Defaults are used if not set.
	Image `json:",inline"`
	// ETOSQueueName is the name of the RabbitMQ queue to use for the log listener to listen for
	// incoming log messages. Defaults to "etos-*-temp", where * is a unique string to avoid
	// conflicts between multiple log listeners.
	// +kubebuilder:default="etos-*-temp"
	// +optional
	ETOSQueueName string `json:"etosQueueName,omitempty"`
	// ETOSQueueParams are the parameters to use for the log listener queue. Defaults to empty.
	// +optional
	ETOSQueueParams string `json:"etosQueueParams,omitempty"`
}

// ETOSSuiteRunner describes the deployment of an ETOS suite runner.
type ETOSSuiteRunner struct {
	// Image describes the image to use for the ETOS suite runner. Defaults are used if not set.
	Image `json:",inline"`
	// LogListener describes the log listener to use for the suite runner.
	// +kubebuilder:default={}
	// +optional
	LogListener ETOSLogListener `json:"logListener,omitempty"`
}

// ETOSTestRunner describes the deployment of an ETOS test runner.
type ETOSTestRunner struct {
	// Version describes the version of the ETOS test runner to use. Defaults are used if not set.
	// +optional
	Version string `json:"version,omitempty"`
}

// ETOSEnvironmentProvider describes the deployment of an ETOS environment provider.
type ETOSEnvironmentProvider struct {
	// Image describes the image to use for the environment provider. Defaults are used if not set.
	Image `json:",inline"`
}

// ETOSConfig describes a common configuration for ETOS.
type ETOSConfig struct {
	// Dev describes whether to deploy ETOS in development mode. Defaults to true if not set.
	// +kubebuilder:default="true"
	// +required
	Dev string `json:"dev,omitempty"`
	// EventDataTimeout describes the timeout for getting event data from the eiffel bus in seconds.
	// Defaults to 60 seconds if not set.
	// +kubebuilder:default="60"
	// +required
	EventDataTimeout string `json:"eventDataTimeout,omitempty"`
	// TestSuiteTimeout describes the timeout for downloading a test suite in seconds.
	// Defaults to 10 seconds if not set.
	// +kubebuilder:default="10"
	// +required
	TestSuiteTimeout string `json:"testSuiteTimeout,omitempty"`
	// EnvironmentTimeout describes the maximum time a suite runner waits for an environment from the
	// environment provider. Defaults to 3600 seconds if not set.
	// +kubebuilder:default="3600"
	// +required
	EnvironmentTimeout string `json:"environmentTimeout,omitempty"`
	// Source describes the source tag added to all Eiffel events sent by ETOS.
	// +kubebuilder:default="ETOS"
	// +required
	Source string `json:"source,omitempty"`

	// TestRunRetention describes the retention for testruns in ETOS. Defaults to no retention if
	// not set.
	// +optional
	TestRunRetention Retention `json:"testrunRetention"`

	// EncryptionKey describes the key to use for encrypting sensitive data in ETOS.
	// The EncryptionKey is a 32 byte long base64 encoded string.
	// It is recommended to put this string into a Kubernetes secret and reference it using a Var
	// with valueFrom.
	// +required
	EncryptionKey Var `json:"encryptionKey"`

	// ETOSApiURL describes the URL to the ETOS API for use by internal ETOS services.
	// Defaults are used if not set.
	// +kubebuilder:default=""
	// +required
	ETOSApiURL string `json:"etosApiURL,omitempty"`
	// ETOSEventRepositoryURL describes the URL to the event repository API for use by ETOS services.
	// Defaults are used if not set, but in order to use the ETOS client it should be set to an
	// externally available URL.
	// +optional
	ETOSEventRepositoryURL string `json:"etosEventRepositoryURL,omitempty"`

	// RoutingKeyTag describes the tag to use for routing keys for ETOS eiffel events.
	// +kubebuilder:default="etos"
	// +required
	RoutingKeyTag string `json:"routingKeyTag,omitempty"`

	// Timezone describes the timezone to use for ETOS. Defaults to empty, which means no timezone
	// is set. It is recommended to set this to a valid timezone, as it is used for all log
	// timestamps in ETOS.
	// +optional
	Timezone string `json:"timezone,omitempty"`
}

// ETOS describes the deployment of an ETOS cluster.
type ETOS struct {
	// API describes a configuration for the ETOS API to use for the cluster.
	// +optional
	API ETOSAPI `json:"api,omitempty"`
	// SSE describes a configuration for the ETOS Server Sent Events API to use for the cluster.
	// +optional
	SSE ETOSSSE `json:"sse,omitempty"`
	// LogArea describes a configuration for the ETOS log area API to use for the cluster.
	// +optional
	LogArea ETOSLogArea `json:"logArea,omitempty"`
	// SuiteRunner describes a configuration for the ETOS suite runner to use for the cluster.
	// +kubebuilder:default={}
	// +optional
	SuiteRunner ETOSSuiteRunner `json:"suiteRunner,omitempty"`
	// TestRunner describes a configuration for the ETOS test runner to use for the cluster.
	// +optional
	TestRunner ETOSTestRunner `json:"testRunner,omitempty"`
	// SuiteStarter describes a configuration for the ETOS suite starter to use for the cluster.
	// +kubebuilder:default={}
	// +optional
	SuiteStarter ETOSSuiteStarter `json:"suiteStarter,omitempty"`
	// EnvironmentProvider describes a configuration for the ETOS environment provider to use for
	// the cluster.
	// +optional
	EnvironmentProvider ETOSEnvironmentProvider `json:"environmentProvider,omitempty"`
	// Ingress describes the ingress to use for the ETOS API services.
	// +kubebuilder:default={}
	// +optional
	Ingress Ingress `json:"ingress,omitempty"`
	// Config describes the common configuration for ETOS.
	// +required
	Config ETOSConfig `json:"config,omitempty"`
}

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	// ETOS describes the deployment of ETOS for the cluster.
	// +required
	ETOS ETOS `json:"etos,omitempty"`
	// Database describes the deployment of a database for the cluster. Defaults to a local ETCD if
	// not set.
	// +kubebuilder:default={}
	// +optional
	Database Database `json:"database,omitempty"`
	// MessageBus describes the deployment of message buses for the cluster. Defaults to local
	// RabbitMQ instances if not set.
	// +kubebuilder:default={}
	// +optional
	MessageBus MessageBus `json:"messageBus"`
	// EventRepository describes the deployment of an event repository for the cluster. Defaults to a
	// local event repository with a local MongoDB if not set.
	// +kubebuilder:default={}
	// +optional
	EventRepository EventRepository `json:"eventRepository,omitempty"`
	// OpenTelemetry describes the configuration for an OpenTelemetry collector for the cluster.
	// +kubebuilder:default={}
	// +optional
	OpenTelemetry OpenTelemetry `json:"openTelemetry,omitempty"`
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

	Spec   ClusterSpec   `json:"spec"`
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
