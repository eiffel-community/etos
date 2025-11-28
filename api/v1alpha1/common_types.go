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
	ConfigMapKeyRef *corev1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
	SecretKeyRef    *corev1.SecretKeySelector    `json:"secretKeyRef,omitempty"`
}

// Var describes either a string value or a value from a VarSource.
type Var struct {
	Value     string    `json:"value,omitempty"`
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
	// +optional
	Image string `json:"image"`

	// +kubebuilder:default="IfNotPresent"
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`
}

// Ingress configuration.
type Ingress struct {
	// +kubebuilder:default=false
	// +optional
	Enabled      bool   `json:"enabled"`
	IngressClass string `json:"ingressClass,omitempty"`
	// +kubebuilder:default=""
	Host        string            `json:"host,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// RabbitMQ configuration.
type RabbitMQ struct {
	// +kubebuilder:default=false
	// +optional
	Deploy bool `json:"deploy"`
	// +kubebuilder:default="rabbitmq"
	// +optional
	Host string `json:"host"`
	// +kubebuilder:default="amq.topic"
	// +optional
	Exchange string `json:"exchange"`
	// +kubebuilder:default={"value": "guest"}
	// +optional
	Password *Var `json:"password,omitempty"`
	// +kubebuilder:default="guest"
	// +optional
	Username string `json:"username,omitempty"`
	// +kubebuilder:default="5672"
	// +optional
	Port string `json:"port"`
	// +kubebuilder:default="false"
	// +optional
	SSL string `json:"ssl"`
	// +kubebuilder:default=/
	// +optional
	Vhost string `json:"vhost"`
}
