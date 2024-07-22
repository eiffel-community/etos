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
package extras

import (
	"context"
	"fmt"
	"net/url"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var graphqlPort int32 = 5000

type EventRepositoryDeployment struct {
	*etosv1alpha1.EventRepository
	client.Client
	Scheme         *runtime.Scheme
	mongoUri       url.URL
	rabbitmqSecret string
	mongodbSecret  string
}

// NewEventRepositoryDeployment will create a new event repository reconciler.
func NewEventRepositoryDeployment(spec *etosv1alpha1.EventRepository, scheme *runtime.Scheme, client client.Client, rabbitmqSecret string, mongodbSecret string) (*EventRepositoryDeployment, error) {
	mongodbURL, err := url.Parse(spec.Database.URI)
	if err != nil {
		return nil, err
	}
	return &EventRepositoryDeployment{spec, client, scheme, *mongodbURL, rabbitmqSecret, mongodbSecret}, nil
}

// Reconcile will reconcile the event repository to its expected state.
func (r *EventRepositoryDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	name := fmt.Sprintf("%s-graphql", cluster.Name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}

	_, err := r.reconcileDeployment(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileService(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileIngress(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	if r.Ingress.Enabled {
		host := namespacedName.Name
		if r.Ingress.Host != "" {
			host = r.Ingress.Host
		}
		r.Host = fmt.Sprintf("http://%s/graphql", host)
	}
	return nil
}

// reconcileDeployment will reconcile the event repository deployment to its expected state.
func (r *EventRepositoryDeployment) reconcileDeployment(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*appsv1.Deployment, error) {
	target := r.deployment(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, name, deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			return deployment, err
		}
		if r.Deploy {
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		return nil, r.Delete(ctx, deployment)
	}
	if equality.Semantic.DeepDerivative(target.Spec, deployment.Spec) {
		return deployment, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(deployment))
}

// reconcileService will reconcile the event repository service to its expected state.
func (r *EventRepositoryDeployment) reconcileService(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	target := r.service(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		if r.Deploy {
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		return nil, r.Delete(ctx, service)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// reconcileIngress will reconcile the event repository ingress to its expected state.
func (r *EventRepositoryDeployment) reconcileIngress(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*networkingv1.Ingress, error) {
	target := r.ingress(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	ingress := &networkingv1.Ingress{}
	if err := r.Get(ctx, name, ingress); err != nil {
		if !apierrors.IsNotFound(err) {
			return ingress, err
		}
		if r.Ingress.Enabled {
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Ingress.Enabled {
		return nil, r.Delete(ctx, ingress)
	}

	if equality.Semantic.DeepDerivative(target.Spec, ingress.Spec) {
		return ingress, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(ingress))
}

// deployment will create a deployment resource definition for the event repository.
func (r *EventRepositoryDeployment) deployment(name types.NamespacedName) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: r.meta(name),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: r.meta(name),
				Spec: corev1.PodSpec{
					Containers: r.containers(name),
				},
			},
		},
	}
}

// service will create a service resource definition for the event repository.
func (r *EventRepositoryDeployment) service(name types.NamespacedName) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name),
		Spec: corev1.ServiceSpec{
			Ports:    r.ports(),
			Selector: map[string]string{"app.kubernetes.io/name": name.Name},
		},
	}
}

// ingress will create a ingress resource definition for the event repository.
func (r *EventRepositoryDeployment) ingress(name types.NamespacedName) *networkingv1.Ingress {
	ingress := &networkingv1.Ingress{
		ObjectMeta: r.meta(name),
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{r.ingressRule(name)},
		},
	}
	if r.Ingress.IngressClass != "" {
		ingress.Spec.IngressClassName = &r.Ingress.IngressClass
	}
	return ingress
}

// meta will create a common meta resource object for the event repository.
func (r *EventRepositoryDeployment) meta(name types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name": name.Name,
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// containers will create a container resource definition for the event repository deployment.
func (r *EventRepositoryDeployment) containers(name types.NamespacedName) []corev1.Container {
	return []corev1.Container{
		{
			Name:  fmt.Sprintf("%s-api", name.Name),
			Image: r.APIImage,
			Ports: []corev1.ContainerPort{
				{
					Name:          "amqp",
					ContainerPort: graphqlPort,
					Protocol:      "TCP",
				},
			},
			EnvFrom: r.environment(),
		}, {
			Name:  fmt.Sprintf("%s-storage", name.Name),
			Image: r.StorageImage,
			Command: []string{
				"python3",
				"-m",
				"eiffel_graphql_api.storage",
			},
			EnvFrom: r.environment(),
		},
	}
}

// environment will create an enivonrmnet resource definition for the event repository deployment.
func (r *EventRepositoryDeployment) environment() []corev1.EnvFromSource {
	return []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.mongodbSecret,
				},
			},
		},
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.rabbitmqSecret,
				},
			},
		},
	}
}

// ingressRule will create an ingress rule resource definition for the event repository ingress.
func (r *EventRepositoryDeployment) ingressRule(name types.NamespacedName) networkingv1.IngressRule {
	prefix := networkingv1.PathTypePrefix
	ingressRule := networkingv1.IngressRule{
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					{
						Path:     "/graphql",
						PathType: &prefix,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: name.Name,
								Port: networkingv1.ServiceBackendPort{
									Number: graphqlPort,
								},
							},
						},
					},
				},
			},
		},
	}
	if r.Ingress.Host != "" {
		ingressRule.Host = r.Ingress.Host
	}
	return ingressRule
}

// ports will create a service port resource definition for the event repository service.
func (r *EventRepositoryDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: graphqlPort, Name: "amqp", Protocol: "TCP"},
	}
}
