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
	"maps"
	"net/url"
	"time"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/go-logr/logr"
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
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var graphqlPort int32 = 5000

type EventRepositoryDeployment struct {
	*etosv1alpha1.EventRepository
	client.Client
	Scheme          *runtime.Scheme
	mongoUri        url.URL
	rabbitmqSecret  string
	mongodbSecret   string
	restartRequired bool
}

// NewEventRepositoryDeployment will create a new event repository reconciler.
func NewEventRepositoryDeployment(spec *etosv1alpha1.EventRepository, scheme *runtime.Scheme, client client.Client, mongodb *MongoDBDeployment, rabbitmqSecret string) *EventRepositoryDeployment {
	return &EventRepositoryDeployment{spec, client, scheme, mongodb.URL, rabbitmqSecret, mongodb.SecretName, false}
}

// Reconcile will reconcile the event repository to its expected state.
func (r *EventRepositoryDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	name := fmt.Sprintf("%s-graphql", cluster.Name)
	logger := log.FromContext(ctx, "Reconciler", "EventRepository", "BaseName", name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}

	cfg, err := r.reconcileConfig(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the EventRepository configuration")
		return err
	}
	var configName string
	if cfg != nil {
		configName = cfg.ObjectMeta.Name
	} else {
		configName = namespacedName.Name
	}

	_, err = r.reconcileDeployment(ctx, logger, namespacedName, configName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the EventRepository deployment")
		return err
	}
	_, err = r.reconcileService(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the EventRepository service")
		return err
	}
	_, err = r.reconcileIngress(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the EventRepository ingress")
		return err
	}
	if r.Ingress.Enabled {
		host := namespacedName.Name
		if r.Ingress.Host != "" {
			host = r.Ingress.Host
		}
		r.Host = fmt.Sprintf("http://%s/graphql", host)
		logger.Info("Host for the EventRepository", "host", r.Host)
	} else if r.Host == "" {
		r.Host = fmt.Sprintf("http://%s:%d/graphql", namespacedName.Name, graphqlPort)
		logger.Info("Host for the EventRepository", "host", r.Host)
	}
	return nil
}

// reconcileConfig will reconcile the secret to use as configuration for the event repository.
func (r *EventRepositoryDeployment) reconcileConfig(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	var err error
	var target *corev1.Secret
	if r.Deploy {
		target, err = r.config(ctx, name)
		if err != nil {
			return nil, err
		}
		if err = ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
			return target, err
		}
	}

	secret := &corev1.Secret{}
	if err = r.Get(ctx, name, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return secret, err
		}
		if r.Deploy {
			r.restartRequired = true
			logger.Info("Creating the configuration for an EventRepository")
			if err = r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		logger.Info("Removing the configuration for EventRepository")
		return nil, r.Delete(ctx, secret)
	}
	if equality.Semantic.DeepDerivative(target.Data, secret.Data) {
		return secret, nil
	}
	r.restartRequired = true
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileDeployment will reconcile the event repository deployment to its expected state.
func (r *EventRepositoryDeployment) reconcileDeployment(ctx context.Context, logger logr.Logger, name types.NamespacedName, secretName string, owner metav1.Object) (*appsv1.Deployment, error) {
	target := r.deployment(name, secretName)
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
			logger.Info("Creating a new EventRepository deployment")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		logger.Info("Removing the deployment for EventRepository")
		return nil, r.Delete(ctx, deployment)
	} else if r.restartRequired {
		logger.Info("Configuration(s) have changed, restarting deployment")
		if target.Spec.Template.Annotations == nil {
			target.Spec.Template.Annotations = make(map[string]string)
		}
		target.Spec.Template.Annotations["etos.eiffel-community.github.io/restartedAt"] = time.Now().Format(time.RFC3339)
	}
	if !r.restartRequired && equality.Semantic.DeepDerivative(target.Spec, deployment.Spec) {
		return deployment, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(deployment))
}

// reconcileService will reconcile the event repository service to its expected state.
func (r *EventRepositoryDeployment) reconcileService(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
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
			logger.Info("Creating a new EventRepository kubernetes service")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		logger.Info("Removing the kubernetes service for EventRepository")
		return nil, r.Delete(ctx, service)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// reconcileIngress will reconcile the event repository ingress to its expected state.
func (r *EventRepositoryDeployment) reconcileIngress(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*networkingv1.Ingress, error) {
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
			logger.Info("Ingress enabled, creating a new ingress for the EventRepository")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Ingress.Enabled {
		logger.Info("Ingress disabled, removing ingress for the EventRepository")
		return nil, r.Delete(ctx, ingress)
	}

	if equality.Semantic.DeepDerivative(target.Spec, ingress.Spec) {
		return ingress, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(ingress))
}

// config creates a new Secret to be used as configuration for the event repository.
func (r *EventRepositoryDeployment) config(ctx context.Context, name types.NamespacedName) (*corev1.Secret, error) {
	eiffel := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.rabbitmqSecret, Namespace: name.Namespace}, eiffel); err != nil {
		return nil, err
	}
	etos := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.mongodbSecret, Namespace: name.Namespace}, etos); err != nil {
		return nil, err
	}
	data := map[string][]byte{}
	maps.Copy(data, eiffel.Data)
	maps.Copy(data, etos.Data)
	data["RABBITMQ_QUEUE"] = []byte(r.EventRepository.EiffelQueueName)
	data["RABBITMQ_QUEUE_PARAMS"] = []byte(r.EventRepository.EiffelQueueParams)
	return &corev1.Secret{
		ObjectMeta: r.meta(name),
		Data:       data,
	}, nil
}

// deployment will create a deployment resource definition for the event repository.
func (r *EventRepositoryDeployment) deployment(name types.NamespacedName, secretName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: r.meta(name),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: r.meta(name),
				Spec: corev1.PodSpec{
					Containers: r.containers(name, secretName),
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
func (r *EventRepositoryDeployment) containers(name types.NamespacedName, secretName string) []corev1.Container {
	return []corev1.Container{
		{
			Name:            fmt.Sprintf("%s-api", name.Name),
			Image:           r.API.Image,
			ImagePullPolicy: r.API.ImagePullPolicy,
			Ports: []corev1.ContainerPort{
				{
					Name:          "amqp",
					ContainerPort: graphqlPort,
					Protocol:      "TCP",
				},
			},
			EnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secretName,
						},
					},
				},
			},
		}, {
			Name:            fmt.Sprintf("%s-storage", name.Name),
			Image:           r.Storage.Image,
			ImagePullPolicy: r.Storage.ImagePullPolicy,
			Command: []string{
				"python3",
				"-m",
				"eiffel_graphql_api.storage",
			},
			EnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secretName,
						},
					},
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
