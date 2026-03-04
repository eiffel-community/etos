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
	"time"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var rabbitmqPort int32 = 5672

type RabbitMQDeployment struct {
	etosv1alpha1.RabbitMQ
	client.Client
	Scheme          *runtime.Scheme
	SecretName      string
	restartRequired bool
}

// NewRabbitMQDeployment will create a new RabbitMQ reconciler.
func NewRabbitMQDeployment(spec etosv1alpha1.RabbitMQ, scheme *runtime.Scheme, client client.Client) *RabbitMQDeployment {
	return &RabbitMQDeployment{spec, client, scheme, "", false}
}

// Reconcile will reconcile RabbitMQ to its expected state.
func (r *RabbitMQDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	name := fmt.Sprintf("%s-rabbitmq", cluster.Name)
	logger := log.FromContext(ctx, "Reconciler", "RabbitMQ", "BaseName", name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}
	if r.Deploy {
		logger.Info("Patching host & port when deploying RabbitMQ", "host", name, "port", rabbitmqPort)
		r.Host = name
		r.Port = fmt.Sprintf("%d", rabbitmqPort)
	}

	secret, err := r.reconcileSecret(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the RabbitMQ secret")
		return err
	}
	r.SecretName = secret.Name

	_, err = r.reconcileStatefulset(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the RabbitMQ statefulset")
		return err
	}
	_, err = r.reconcileService(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the RabbitMQ service")
		return err
	}

	return nil
}

// reconcileSecret will reconcile the RabbitMQ secret to its expected state.
func (r *RabbitMQDeployment) reconcileSecret(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	target, err := r.secret(ctx, name, owner.GetName())
	if err != nil {
		return target, err
	}
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	secret := &corev1.Secret{}
	if err := r.Get(ctx, name, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "failed to get rabbitmq secret")
			return secret, err
		}
		r.restartRequired = true
		logger.Info("Secret not found. Creating")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	if equality.Semantic.DeepDerivative(target.Data, secret.Data) {
		return secret, nil
	}
	r.restartRequired = true
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileStatefulset will reconcile the RabbitMQ statefulset to its expected state.
func (r *RabbitMQDeployment) reconcileStatefulset(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*appsv1.StatefulSet, error) {
	target := r.statefulset(name, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	rabbitmq := &appsv1.StatefulSet{}
	if err := r.Get(ctx, name, rabbitmq); err != nil {
		if !apierrors.IsNotFound(err) {
			return rabbitmq, err
		}
		if r.Deploy {
			logger.Info("Creating a new RabbitMQ statefulset")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		logger.Info("Removing the statefulset for RabbitMQ")
		return nil, r.Delete(ctx, rabbitmq)
	} else if r.restartRequired {
		logger.Info("Configuration(s) have changed, restarting statefulset")
		if target.Spec.Template.Annotations == nil {
			target.Spec.Template.Annotations = make(map[string]string)
		}
		target.Spec.Template.Annotations["etos.eiffel-community.github.io/restartedAt"] = time.Now().Format(time.RFC3339)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(rabbitmq))
}

// reconcileService will reconcile the RabbitMQ service to its expected state.
func (r *RabbitMQDeployment) reconcileService(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	target := r.service(name, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		if r.Deploy {
			logger.Info("Creating a new RabbitMQ kubernetes service")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		logger.Info("Removing the kubernetes service for RabbitMQ")
		return nil, r.Delete(ctx, service)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// secret will create a secret resource definition for RabbitMQ.
func (r *RabbitMQDeployment) secret(ctx context.Context, name types.NamespacedName, clusterName string) (*corev1.Secret, error) {
	data, err := r.secretData(ctx, name)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: r.meta(name, clusterName),
		Data:       data,
	}, nil
}

// statefulset will create a statefulset resource definition for RabbitMQ.
func (r *RabbitMQDeployment) statefulset(name types.NamespacedName, clusterName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: r.meta(name, clusterName),
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name.Name},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{r.volumeClaim(name)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: r.meta(name, clusterName),
				Spec: corev1.PodSpec{
					Volumes:    []corev1.Volume{r.volume(name)},
					Containers: []corev1.Container{r.container(name)},
				},
			},
		},
	}
}

// service will create a service resource definition for RabbitMQ.
func (r *RabbitMQDeployment) service(name types.NamespacedName, clusterName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name, clusterName),
		Spec: corev1.ServiceSpec{
			Ports:    r.ports(),
			Selector: map[string]string{"app.kubernetes.io/name": name.Name},
		},
	}
}

// secretData will create a map of secrets for the RabbitMQ secret.
func (r *RabbitMQDeployment) secretData(ctx context.Context, name types.NamespacedName) (map[string][]byte, error) {
	data := map[string][]byte{
		"RABBITMQ_HOST":     []byte(r.Host),
		"RABBITMQ_EXCHANGE": []byte(r.Exchange),
		"RABBITMQ_PORT":     []byte(r.Port),
		"RABBITMQ_SSL":      []byte(r.SSL),
		"RABBITMQ_VHOST":    []byte(r.Vhost),
	}
	if r.Password != nil {
		password, err := r.Password.Get(ctx, r.Client, name.Namespace)
		if err != nil {
			return nil, err
		}
		data["RABBITMQ_PASSWORD"] = password
	}
	if r.Username != "" {
		data["RABBITMQ_USERNAME"] = []byte(r.Username)
	}
	return data, nil
}

// meta will create a common meta object for RabbitMQ.
func (r *RabbitMQDeployment) meta(name types.NamespacedName, clusterName string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name":                  name.Name,
			"etos.eiffel-community.github.io/cluster": clusterName,
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// volumeClaim will create a volume claim resource definition for the RabbitMQ statefulset.
func (r *RabbitMQDeployment) volumeClaim(name types.NamespacedName) corev1.PersistentVolumeClaim {
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-data", name.Name),
			Namespace: name.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{"storage": resource.MustParse("1Gi")},
			},
		},
	}
}

// volume will create a volume resource definition for the RabbitMQ statefulset.
func (r *RabbitMQDeployment) volume(name types.NamespacedName) corev1.Volume {
	return corev1.Volume{
		Name: fmt.Sprintf("%s-data", name.Name),
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: fmt.Sprintf("%s-data", name.Name),
			},
		},
	}
}

// container will create a container resource definition for the RabbitMQ statefulset.
func (r *RabbitMQDeployment) container(name types.NamespacedName) corev1.Container {
	return corev1.Container{
		Name:  name.Name,
		Image: "rabbitmq:latest",
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      fmt.Sprintf("%s-data", name.Name),
				MountPath: "/var/lib/rabbitmq/data",
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "amqp",
				ContainerPort: rabbitmqPort,
				Protocol:      "TCP",
			},
		},
	}
}

// ports will create a service port resource definition for the RabbitMQ service.
func (r *RabbitMQDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: rabbitmqPort, Name: "amqp", Protocol: "TCP"},
	}
}
