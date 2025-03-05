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

var rabbitmqStreamPort int32 = 5552

type MessageBusDeployment struct {
	etosv1alpha1.RabbitMQ
	client.Client
	Scheme          *runtime.Scheme
	SecretName      string
	restartRequired bool
}

// NewMessageBusDeployment will create a new messagebus reconciler.
func NewMessageBusDeployment(spec etosv1alpha1.RabbitMQ, scheme *runtime.Scheme, client client.Client) *MessageBusDeployment {
	return &MessageBusDeployment{spec, client, scheme, "", false}
}

// Reconcile will reconcile the messagebus to its expected state.
func (r *MessageBusDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	name := fmt.Sprintf("%s-messagebus", cluster.Name)
	logger := log.FromContext(ctx, "Reconciler", "MessageBus", "BaseName", name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}
	if r.Deploy {
		logger.Info("Patching host & port when deploying RabbitMQ", "host", name, "port", rabbitmqPort, "streamPort", rabbitmqStreamPort)
		r.Host = name
		r.Port = fmt.Sprintf("%d", rabbitmqPort)
		r.StreamPort = fmt.Sprintf("%d", rabbitmqStreamPort)
	}

	secret, err := r.reconcileSecret(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the MessageBus secret")
		return err
	}
	r.SecretName = secret.Name

	// A headless service is required for when we enable rabbitmq-streams since each node in the RabbitMQ cluster
	// advertises itself and expects clients to connect to that specific node. A headless service allows clients
	// to connect directly to the advertised host.
	_, err = r.reconcileHeadlessService(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the MessageBus service")
		return err
	}

	_, err = r.reconcileStatefulset(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the MessageBus statefulset")
		return err
	}
	_, err = r.reconcileService(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the MessageBus service")
		return err
	}
	return nil
}

// reconcileSecret will reconcile the messagebus secret to its expected state.
func (r *MessageBusDeployment) reconcileSecret(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	target, err := r.secret(ctx, name)
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

// reconcileStatefulset will reconcile the messagebus statefulset to its expected state.
func (r *MessageBusDeployment) reconcileStatefulset(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*appsv1.StatefulSet, error) {
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
			logger.Info("Creating a new MessageBus statefulset")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		logger.Info("Removing the statefulset for MessageBus")
		return nil, r.Delete(ctx, rabbitmq)
	} else if r.restartRequired {
		logger.Info("Configuration(s) have changed, restarting statefulset")
		rabbitmq.Spec.Template.Annotations["etos.eiffel-community.github.io/restartedAt"] = time.Now().Format(time.RFC3339)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(rabbitmq))
}

// reconcileHeadlessService will reconcile the messagebus headless service to its expected state.
func (r *MessageBusDeployment) reconcileHeadlessService(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	selectorName := name.Name
	name = types.NamespacedName{Name: fmt.Sprintf("%s-headless", name.Name), Namespace: name.Namespace}
	target := r.service(name, selectorName, true)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		if r.Deploy {
			logger.Info("Creating a new MessageBus kubernetes headless service")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		logger.Info("Removing the kubernetes headless service for MessageBus")
		return nil, r.Delete(ctx, service)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// reconcileService will reconcile the messagebus service to its expected state.
func (r *MessageBusDeployment) reconcileService(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	target := r.service(name, name.Name, false)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		if r.Deploy {
			logger.Info("Creating a new MessageBus kubernetes service")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		logger.Info("Removing the kubernetes service for MessageBus")
		return nil, r.Delete(ctx, service)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// secret will create a secret resource definition for the messagebus.
func (r *MessageBusDeployment) secret(ctx context.Context, name types.NamespacedName) (*corev1.Secret, error) {
	data, err := r.secretData(ctx, name.Namespace)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: r.meta(name),
		Data:       data,
	}, nil
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
			ServiceName:          fmt.Sprintf("%s-headless", name.Name),
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
func (r *MessageBusDeployment) service(name types.NamespacedName, selectorName string, headless bool) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: r.meta(name),
		Spec: corev1.ServiceSpec{
			Ports:    r.ports(),
			Selector: map[string]string{"app.kubernetes.io/name": selectorName},
		},
	}
	if headless {
		svc.Spec.ClusterIP = "None"
	}
	return svc
}

// secretData will create a map of secrets for the messagebus secret.
func (r *MessageBusDeployment) secretData(ctx context.Context, namespace string) (map[string][]byte, error) {
	data := map[string][]byte{
		"ETOS_RABBITMQ_HOST":        []byte(r.Host),
		"ETOS_RABBITMQ_EXCHANGE":    []byte(r.Exchange),
		"ETOS_RABBITMQ_PORT":        []byte(r.Port),
		"ETOS_RABBITMQ_STREAM_PORT": []byte(r.StreamPort),
		"ETOS_RABBITMQ_SSL":         []byte(r.SSL),
		"ETOS_RABBITMQ_VHOST":       []byte(r.Vhost),
	}

	if r.Password != nil {
		password, err := r.Password.Get(ctx, r.Client, namespace)
		if err != nil {
			return nil, err
		}
		data["ETOS_RABBITMQ_PASSWORD"] = password
	}
	if r.Username != "" {
		data["ETOS_RABBITMQ_USERNAME"] = []byte(r.Username)
	}
	return data, nil
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
		Image: "rabbitmq:3.13.7",
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      fmt.Sprintf("%s-data", name.Name),
				MountPath: "/var/lib/rabbitmq/data",
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "SERVICE_NAME",
				Value: fmt.Sprintf("%s-headless", name.Name),
			},
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name:  "RABBITMQ_SERVER_ADDITIONAL_ERL_ARGS",
				Value: "-rabbitmq_stream advertised_host \"$(POD_NAME).$(SERVICE_NAME)\"",
			},
		},
		Lifecycle: &corev1.Lifecycle{
			PostStart: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						"rabbitmq-plugins --offline enable rabbitmq_stream_management",
					},
				},
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "amqp",
				ContainerPort: rabbitmqPort,
				Protocol:      "TCP",
			},
			{
				Name:          "rabbitmq-stream",
				ContainerPort: rabbitmqStreamPort,
				Protocol:      "TCP",
			},
		},
	}
}

// ports will create a ports resource definition for the messagebus service.
func (r *MessageBusDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: rabbitmqPort, Name: "amqp", Protocol: "TCP"},
		{Port: rabbitmqStreamPort, Name: "rabbitmq-stream", Protocol: "TCP"},
	}
}
