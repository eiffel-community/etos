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
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VarSource describes a value from either a secretmap or configmap.
type VarSource struct {
	// ConfigMapKeyRef describes a value from a configmap. Cannot be set if SecretKeyRef is set.
	// +optional
	ConfigMapKeyRef *corev1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
	// SecretKeyRef describes a value from a secret. Cannot be set if ConfigMapKeyRef is set.
	// +optional
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// Var describes either a string value or a value from a VarSource.
type Var struct {
	// Value describes a string value. Cannot be set if ValueFrom is set.
	// +optional
	Value string `json:"value,omitempty"`
	// ValueFrom describes a value from a VarSource. Cannot be set if Value is set.
	// +optional
	ValueFrom VarSource `json:"valueFrom,omitempty"`
}

// getFromSecretKeySelector returns the value of a key in a secret.
func (v *Var) getFromSecretKeySelector(ctx context.Context, client client.Client, secretKeySelector *corev1.SecretKeySelector, namespace string) ([]byte, error) {
	name := types.NamespacedName{Name: secretKeySelector.Name, Namespace: namespace}
	obj := &corev1.Secret{}
	err := client.Get(ctx, name, obj)
	if err != nil {
		return nil, err
	}
	d, ok := obj.Data[secretKeySelector.Key]
	if !ok {
		return nil, fmt.Errorf("%s does not exist in secret %s/%s", secretKeySelector.Key, secretKeySelector.Name, namespace)
	}
	return d, nil
}

// getFromConfigMapKeySelector returns the value of a key in a configmap.
func (v *Var) getFromConfigMapKeySelector(ctx context.Context, client client.Client, configMapKeySelector *corev1.ConfigMapKeySelector, namespace string) ([]byte, error) {
	name := types.NamespacedName{Name: configMapKeySelector.Name, Namespace: namespace}
	obj := &corev1.ConfigMap{}
	err := client.Get(ctx, name, obj)
	if err != nil {
		return nil, err
	}
	d, ok := obj.Data[configMapKeySelector.Key]
	if !ok {
		return nil, fmt.Errorf("%s does not exist in configmap %s/%s", configMapKeySelector.Key, configMapKeySelector.Name, namespace)
	}
	return []byte(d), nil
}

// Get the value from a Var struct. Either through the Value key, secret or configmap.
func (v *Var) Get(ctx context.Context, client client.Client, namespace string) ([]byte, error) {
	if v.Value != "" {
		return []byte(v.Value), nil
	}
	if v.ValueFrom.SecretKeyRef != nil {
		return v.getFromSecretKeySelector(ctx, client, v.ValueFrom.SecretKeyRef, namespace)
	}
	if v.ValueFrom.ConfigMapKeyRef != nil {
		return v.getFromConfigMapKeySelector(ctx, client, v.ValueFrom.ConfigMapKeyRef, namespace)
	}
	return nil, errors.New("found no source for key")
}

// Image configuration.
type Image struct {
	// Image describes the docker image to run for a service. ETOS applies defaults if empty.
	// +optional
	Image string `json:"image,omitempty"`

	// ImagePullPolicy describes the pull policy to use for the image. ETOS applies PullIfNotPresent
	// if empty.
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// Ingress configuration.
type Ingress struct {
	// Enabled describes whether to create an ingress for the service. Defaults to false.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// IngressClass describes the ingress class to use for the ingress.
	// +optional
	IngressClass string `json:"ingressClass,omitempty"`
	// Host describes the host to use for the ingress. Generated if Enabled is true.
	// +optional
	Host string `json:"host,omitempty"`
	// Annotations describes extra annotations to add to the ingress.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// RabbitMQ configuration.
type RabbitMQ struct {
	// Deploy describes whether to deploy a RabbitMQ instance for the service. Defaults to true.
	// +kubebuilder:default=true
	// +optional
	Deploy bool `json:"deploy,omitempty"`
	// Host describes the host to use for RabbitMQ. Only used if Deploy is false.
	// +kubebuilder:default="rabbitmq"
	// +optional
	Host string `json:"host,omitempty"`
	// Exchange describes the exchange to use for RabbitMQ. Only set if Deploy is false.
	// +kubebuilder:default="amq.topic"
	// +optional
	Exchange string `json:"exchange,omitempty"`
	// Password describes the password to use for RabbitMQ. Only set if Deploy is false.
	// +kubebuilder:default={"value": "guest"}
	// +optional
	Password *Var `json:"password,omitempty"`
	// Username describes the username to use for RabbitMQ. Only set if Deploy is false.
	// +kubebuilder:default="guest"
	// +optional
	Username string `json:"username,omitempty"`
	// Port describes the port to use for RabbitMQ. Only set if Deploy is false.
	// +kubebuilder:default="5672"
	// +optional
	Port string `json:"port,omitempty"`
	// SSL describes whether to use SSL for RabbitMQ. Only set if Deploy is false.
	// +kubebuilder:default="false"
	// +optional
	SSL string `json:"ssl,omitempty"`
	// Vhost describes the vhost to use for RabbitMQ. Only set if Deploy is false.
	// +kubebuilder:default=/
	// +optional
	Vhost string `json:"vhost,omitempty"`

	// StreamPort describes the port to use for RabbitMQ streaming. Only set if Deploy is false.
	// +kubebuilder:default="5552"
	// +optional
	StreamPort string `json:"streamPort,omitempty"`
	// StreamName describes the stream name to use for RabbitMQ streaming. Only set if Deploy is false.
	// +kubebuilder:default="etos-messagebus-stream"
	// +optional
	StreamName string `json:"streamName,omitempty"`
}
