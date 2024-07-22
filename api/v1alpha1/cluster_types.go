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

type MongoDB struct {
	Deploy bool `json:"deploy"`
	// Ignored if deploy is true
	// +kubebuilder:default="mongodb://root:password@mongodb:27017/admin"
	URI       string `json:"uri"`
	URISecret string `json:"uriSecretRef,omitempty"`
}

// Image configuration.
type Image struct {
	Image string `json:"image"`

	// +kubebuilder:default="IfNotPresent"
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`
}

type EventRepository struct {
	// Deploy a local event repository for a cluster.
	Deploy bool `json:"deploy"`

	// We do not build the GraphQL API automatically nor publish it remotely.
	// This will need to be provided to work.
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/eiffel-graphql-api:latest", "imagePullPolicy": "IfNotPresent"}
	API Image `json:"api"`

	// We do not build the GraphQL API automatically nor publish it remotely.
	// This will need to be provided to work.
	// +kubebuilder:default={"image": "registry.nordix.org/eiffel/eiffel-graphql-storage:latest", "imagePullPolicy": "IfNotPresent"}
	Storage Image `json:"storage"`

	// +kubebuilder:default={"uri": "mongodb://root:password@mongodb:27017/admin", "deploy": false}
	Database MongoDB `json:"mongo"`
	// +kubebuilder:default="eventrepository"
	Host string `json:"host"`
	// +kubebuilder:default={"enabled": false}
	Ingress Ingress `json:"ingress"`
}

type RabbitMQ struct {
	// +kubebuilder:default=false
	Deploy bool `json:"deploy"`
	// +kubebuilder:default="rabbitmq"
	Host string `json:"host"`
	// +kubebuilder:default="amq.topic"
	Exchange       string `json:"exchange"`
	PasswordSecret string `json:"passwordSecret,omitempty"`
	// +kubebuilder:default="guest"
	Password string `json:"password,omitempty"`
	// +kubebuilder:default="guest"
	Username string `json:"username,omitempty"`
	// +kubebuilder:default="5672"
	Port string `json:"port"`
	// +kubebuilder:default="false"
	SSL string `json:"ssl"`
	// +kubebuilder:default=/
	Vhost       string `json:"vhost"`
	QueueName   string `json:"queueName,omitempty"`
	QueueParams string `json:"queueParams,omitempty"`
}

type MessageBus struct {
	// +kubebuilder:default={"host": "rabbitmq", "exchange": "amq.topic", "port": "5672", "ssl": "false", "vhost": "/", "queueName": "etos", "deploy": false}
	EiffelMessageBus RabbitMQ `json:"eiffel"`
	// +kubebuilder:default={"host": "rabbitmq", "exchange": "amq.topic", "port": "5672", "ssl": "false", "vhost": "/", "queueName": "etos-*-temp", "deploy": false}
	ETOSMessageBus RabbitMQ `json:"logs"`
}

type Etcd struct {
	// Parameter is ignored if Deploy is set to true.
	// +kubebuilder:default="etcd-client"
	Host string `json:"host"`
	// Parameter is ignored if Deploy is set to true.
	// +kubebuilder:default="2379"
	Port string `json:"port"`
}

type Database struct {
	// +kubebuilder:default=true
	Deploy bool `json:"deploy"`
	// +kubebuilder:default={"host": "etcd-client", "port": "2379"}
	Etcd Etcd `json:"etcd"`
}

type Ingress struct {
	// +kubebuilder:default=false
	Enabled      bool   `json:"enabled"`
	IngressClass string `json:"ingressClass,omitempty"`
	// +kubebuilder:default=""
	Host        string            `json:"host,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type ETOSApi struct {
	Image `json:",inline"`
}

type ETOSSSE struct {
	Image `json:",inline"`
}

type ETOSLogArea struct {
	Image `json:",inline"`
}

type ETOSSuiteRunner struct {
	Image `json:",inline"`
}

type ETOSConfig struct {
	// +kubebuilder:default="true"
	Dev string `json:"dev"`
	// +kubebuilder:default="60"
	EventDataTimeout string `json:"eventDataTimeout"`
	// +kubebuilder:default="10"
	TestSuiteTimeout string `json:"testSuiteTimeout"`
	// +kubebuilder:default="3600"
	EnvironmentTimeout string `json:"environmentTimeout"`
	// +kubebuilder:default="ETOS"
	Source string `json:"source"`

	ETOSApiURL             string `json:"etosApiURL,omitempty"`
	ETOSEventRepositoryURL string `json:"etosEventRepositoryURL,omitempty"`

	Timezone string `json:"timezone,omitempty"`
}

type ETOS struct {
	API         ETOSApi         `json:"api"`
	SSE         ETOSSSE         `json:"sse"`
	LogArea     ETOSLogArea     `json:"logArea"`
	SuiteRunner ETOSSuiteRunner `json:"suiteRunner"`
	Ingress     Ingress         `json:"ingress,omitempty"`
	// +kubebuilder:default={"dev": "true", "eventDataTimeout": "60", "testSuiteTimeout": "10", "environmentTimeout": "3600", "source": "ETOS"}
	Config ETOSConfig `json:"config"`
}

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	ETOS            ETOS            `json:"etos"`
	Database        Database        `json:"database"`
	MessageBus      MessageBus      `json:"messageBus"`
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
