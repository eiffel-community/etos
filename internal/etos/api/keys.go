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

package api

import (
	"context"
	"fmt"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/config"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	keyPort        int32 = 8080
	KeyServicePort int32 = 80
)

type ETOSKeysDeployment struct {
	etosv1alpha1.ETOSKeyService
	client.Client
	Scheme *runtime.Scheme
	config config.Config
}

// NewETOSKeysDeployment will create a new ETOS key service reconciler.
func NewETOSKeysDeployment(spec etosv1alpha1.ETOSKeyService, scheme *runtime.Scheme, client client.Client, config config.Config) *ETOSKeysDeployment {
	return &ETOSKeysDeployment{spec, client, scheme, config}
}

// Reconcile will reconcile the ETOS key service to its expected state.
func (r *ETOSKeysDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	var err error
	name := fmt.Sprintf("%s-etos-key-service", cluster.Name)
	logger := log.FromContext(ctx, "Reconciler", "ETOSKeyService", "BaseName", name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}

	_, err = r.reconcileDeployment(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the deployment for the ETOS Key Service")
		return err
	}

	_, err = r.reconcileServiceAccount(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the service account for the ETOS Key Service")
		return err
	}
	_, err = r.reconcileService(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the service for the ETOS Key Service")
		return err
	}
	return nil
}

// reconcileDeployment will reconcile the ETOS key service deployment to its expected state.
func (r *ETOSKeysDeployment) reconcileDeployment(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*appsv1.Deployment, error) {
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
		logger.Info("Creating a new deployment for the key service")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	if equality.Semantic.DeepDerivative(target.Spec, deployment.Spec) {
		return deployment, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(deployment))
}

// reconcileServiceAccount will reconcile the ETOS key service account to its expected state.
func (r *ETOSKeysDeployment) reconcileServiceAccount(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
	target := r.serviceaccount(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	serviceaccount := &corev1.ServiceAccount{}
	if err := r.Get(ctx, name, serviceaccount); err != nil {
		if !apierrors.IsNotFound(err) {
			return serviceaccount, err
		}
		logger.Info("Creating a new service account for the key server")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(serviceaccount))
}

// reconcileService will reconcile the ETOS key service to its expected state.
func (r *ETOSKeysDeployment) reconcileService(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	target := r.service(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		logger.Info("Creating a new kubernetes service for the key server")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// serviceaccount creates a serviceaccount resource definition for the ETOS key service.
func (r *ETOSKeysDeployment) serviceaccount(name types.NamespacedName) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: r.meta(name),
	}
}

// deployment creates a deployment resource definition for the ETOS key service.
func (r *ETOSKeysDeployment) deployment(name types.NamespacedName) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: r.meta(name),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      name.Name,
					"app.kubernetes.io/part-of":   "etos",
					"app.kubernetes.io/component": "key-service",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: r.meta(name),
				Spec: corev1.PodSpec{
					Volumes:            r.volumes(),
					ServiceAccountName: name.Name,
					Containers:         []corev1.Container{r.container(name)},
				},
			},
		},
	}
}

// service creates a service resource definition for the ETOS key service.
func (r *ETOSKeysDeployment) service(name types.NamespacedName) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name),
		Spec: corev1.ServiceSpec{
			Ports: r.ports(),
			Selector: map[string]string{
				"app.kubernetes.io/name":      name.Name,
				"app.kubernetes.io/part-of":   "etos",
				"app.kubernetes.io/component": "key-service",
			},
		},
	}
}

// volumes return volumes to mount into the ETOS key service deployment.
func (r *ETOSKeysDeployment) volumes() []corev1.Volume {
	volumes := []corev1.Volume{}
	if privateKey := r.privateKey(); privateKey != nil {
		volumes = append(volumes, corev1.Volume{Name: "private-key", VolumeSource: *privateKey})
	}
	if publicKey := r.publicKey(); publicKey != nil {
		volumes = append(volumes, corev1.Volume{Name: "public-key", VolumeSource: *publicKey})
	}
	return volumes
}

// privateKey returns a private key volume source from either ConfigMap or Secret.
func (r *ETOSKeysDeployment) privateKey() *corev1.VolumeSource {
	if r.PrivateKey == nil {
		return nil
	}
	if r.PrivateKey.SecretKeyRef != nil {
		return &corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: r.PrivateKey.SecretKeyRef.Name,
			},
		}
	}
	if r.PrivateKey.ConfigMapKeyRef != nil {
		return &corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.PrivateKey.ConfigMapKeyRef.Name,
				},
			},
		}
	}
	return nil
}

// publicKey returns a private key volume source from either ConfigMap or Secret.
func (r *ETOSKeysDeployment) publicKey() *corev1.VolumeSource {
	if r.PublicKey == nil {
		return nil
	}
	if r.PublicKey.SecretKeyRef != nil {
		return &corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: r.PublicKey.SecretKeyRef.Name,
			},
		}
	}
	if r.PublicKey.ConfigMapKeyRef != nil {
		return &corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.PublicKey.ConfigMapKeyRef.Name,
				},
			},
		}
	}
	return nil
}

// container creates a container resource definition for the ETOS key service deployment.
func (r *ETOSKeysDeployment) container(name types.NamespacedName) corev1.Container {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/keys/v1alpha/selftest/ping",
				Port:   intstr.FromString("http"),
				Scheme: "HTTP",
			},
		},
		TimeoutSeconds:   1,
		PeriodSeconds:    10,
		SuccessThreshold: 1,
		FailureThreshold: 3,
	}
	return corev1.Container{
		Name:            name.Name,
		Image:           r.Image.Image,
		ImagePullPolicy: r.ImagePullPolicy,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("256Mi"),
				corev1.ResourceCPU:    resource.MustParse("200m"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: keyPort,
				Protocol:      "TCP",
			},
		},
		LivenessProbe:  probe,
		ReadinessProbe: probe,
		VolumeMounts:   r.volumeMounts(),
		Env:            r.environment(),
	}
}

// volumeMounts return a volume mount specification if there are any signing key volumes.
func (r *ETOSKeysDeployment) volumeMounts() []corev1.VolumeMount {
	if r.PublicKey != nil {
		return []corev1.VolumeMount{{
			Name:      "public-key",
			MountPath: "/keys",
			ReadOnly:  true,
		}}
	}
	if r.PrivateKey != nil {
		return []corev1.VolumeMount{{
			Name:      "private-key",
			MountPath: "/keys",
			ReadOnly:  true,
		}}
	}
	return nil
}

// environment creates an environment resource definition for the ETOS key service deployment.
func (r *ETOSKeysDeployment) environment() []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "SERVICE_HOST",
			Value: "0.0.0.0",
		},
		{
			Name:  "STRIP_PREFIX",
			Value: "/keys",
		},
	}
	if r.PublicKey != nil {
		var key string
		if r.PublicKey.SecretKeyRef != nil {
			key = r.PublicKey.SecretKeyRef.Key
		} else if r.PublicKey.ConfigMapKeyRef != nil {
			key = r.PublicKey.ConfigMapKeyRef.Key
		}
		envs = append(envs, corev1.EnvVar{
			Name:  "PUBLIC_KEY_PATH",
			Value: fmt.Sprintf("/keys/%s", key),
		})
	}
	if r.PrivateKey != nil {
		var key string
		if r.PrivateKey.SecretKeyRef != nil {
			key = r.PrivateKey.SecretKeyRef.Key
		} else if r.PrivateKey.ConfigMapKeyRef != nil {
			key = r.PrivateKey.ConfigMapKeyRef.Key
		}
		envs = append(envs, corev1.EnvVar{
			Name:  "PRIVATE_KEY_PATH",
			Value: fmt.Sprintf("/keys/%s", key),
		})
	}
	return envs
}

// meta creates a common meta resource definition for the ETOS key service.
func (r *ETOSKeysDeployment) meta(name types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name":      name.Name,
			"app.kubernetes.io/part-of":   "etos",
			"app.kubernetes.io/component": "key-service",
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// ports creates a service port resource definition for the ETOS key service.
func (r *ETOSKeysDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: KeyServicePort, Name: "http", Protocol: "TCP", TargetPort: intstr.FromString("http")},
	}
}
