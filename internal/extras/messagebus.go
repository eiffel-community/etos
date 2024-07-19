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

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
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

type MessageBusDeployment struct {
	etosv1alpha1.RabbitMQ
	client.Client
	Scheme     *runtime.Scheme
	SecretName string
}

// NewMessageBusDeployment will create a new messagebus reconciler.
func NewMessageBusDeployment(spec etosv1alpha1.RabbitMQ, scheme *runtime.Scheme, client client.Client) (*MessageBusDeployment, error) {
	return &MessageBusDeployment{spec, client, scheme, ""}, nil
}

// Reconcile will reconcile the messagebus to its expected state.
func (r *MessageBusDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	logger := log.FromContext(ctx)
	name := fmt.Sprintf("%s-messagebus", cluster.Name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}
	if r.Deploy {
		logger.Info("Patching host & port when deploying RabbitMQ", "host", name, "port", rabbitmqPort)
		r.Host = name
		r.Port = fmt.Sprintf("%d", rabbitmqPort)
	}

	secret, err := r.reconcileSecret(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	r.SecretName = secret.Name

	_, err = r.reconcileStatefulset(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileService(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	return nil
}

// reconcileSecret will reconcile the messagebus secret to its expected state.
func (r *MessageBusDeployment) reconcileSecret(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	target := r.secret(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	secret := &corev1.Secret{}
	if err := r.Get(ctx, name, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return secret, err
		}
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	if equality.Semantic.DeepDerivative(target.Data, secret.Data) {
		return secret, nil
	}
	return secret, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileStatefulset will reconcile the messagebus statefulset to its expected state.
func (r *MessageBusDeployment) reconcileStatefulset(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*appsv1.StatefulSet, error) {
	target := r.statefulset(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	rabbitmq := &appsv1.StatefulSet{}
	if err := r.Get(ctx, name, rabbitmq); err != nil {
		if !apierrors.IsNotFound(err) {
			return rabbitmq, err
		}
		if r.Deploy {
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		return nil, r.Delete(ctx, rabbitmq)
	}
	return rabbitmq, r.Patch(ctx, target, client.StrategicMergeFrom(rabbitmq))
}

// reconcileService will reconcile the messagebus service to its expected state.
func (r *MessageBusDeployment) reconcileService(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
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
	return service, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// secret will create a secret resource definition for the messagebus.
func (r *MessageBusDeployment) secret(name types.NamespacedName) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: r.meta(name),
		Data:       r.secretData(),
	}
}

// statefulset will create a statefulset resource definition for the messagebus.
func (r *MessageBusDeployment) statefulset(name types.NamespacedName) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: r.meta(name),
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name.Name},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{r.volumeClaim(name)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: r.meta(name),
				Spec: corev1.PodSpec{
					Volumes:    []corev1.Volume{r.volume(name)},
					Containers: []corev1.Container{r.container(name)},
				},
			},
		},
	}
}

// service will create a service resource definition for the messagebus.
func (r *MessageBusDeployment) service(name types.NamespacedName) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name),
		Spec: corev1.ServiceSpec{
			Ports:    r.ports(),
			Selector: map[string]string{"app.kubernetes.io/name": name.Name},
		},
	}
}

// secretData will create a map of secrets for the messagebus secret.
func (r *MessageBusDeployment) secretData() map[string][]byte {
	data := map[string][]byte{
		"ETOS_RABBITMQ_HOST":     []byte(r.Host),
		"ETOS_RABBITMQ_EXCHANGE": []byte(r.Exchange),
		"ETOS_RABBITMQ_PORT":     []byte(r.Port),
		"ETOS_RABBITMQ_SSL":      []byte(r.SSL),
		"ETOS_RABBITMQ_VHOST":    []byte(r.Vhost),
	}
	// TODO: Deal with PasswordSecret
	if r.Password != "" {
		data["ETOS_RABBITMQ_PASSWORD"] = []byte(r.Password)
	}
	if r.Username != "" {
		data["ETOS_RABBITMQ_USERNAME"] = []byte(r.Username)
	}
	if r.QueueName != "" {
		data["ETOS_RABBITMQ_QUEUE_NAME"] = []byte(r.QueueName)
	}
	if r.QueueParams != "" {
		data["ETOS_RABBITMQ_QUEUE_PARAMS"] = []byte(r.QueueParams)
	}
	return data
}

// meta will create a common meta resource object for the messagebus.
func (r *MessageBusDeployment) meta(name types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name": name.Name,
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// volumeClaim will create a volume claim resource definition for the messagebus statefulset.
func (r *MessageBusDeployment) volumeClaim(name types.NamespacedName) corev1.PersistentVolumeClaim {
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

// volume will create a volume resource definition for the messagebus statefulset.
func (r *MessageBusDeployment) volume(name types.NamespacedName) corev1.Volume {
	return corev1.Volume{
		Name: fmt.Sprintf("%s-data", name.Name),
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: fmt.Sprintf("%s-data", name.Name),
			},
		},
	}
}

// container will create a container resource definition for the messagebus statefulset.
func (r *MessageBusDeployment) container(name types.NamespacedName) corev1.Container {
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

// ports will create a ports resource definition for the messagebus service.
func (r *MessageBusDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: rabbitmqPort, Name: "amqp", Protocol: "TCP"},
	}
}
