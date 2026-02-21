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
	logAreaPort        int32 = 8080
	LogAreaServicePort int32 = 80
)

type ETOSLogAreaDeployment struct {
	etosv1alpha1.ETOSLogArea
	client.Client
	Scheme *runtime.Scheme
	cfg    config.Config
}

// NewETOSLogAreaDeployment will create a new ETOS logarea reconciler.
func NewETOSLogAreaDeployment(spec etosv1alpha1.ETOSLogArea, scheme *runtime.Scheme, client client.Client, cfg config.Config) *ETOSLogAreaDeployment {
	return &ETOSLogAreaDeployment{spec, client, scheme, cfg}
}

// Reconcile will reconcile the ETOS logarea to its expected state.
func (r *ETOSLogAreaDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	var err error
	name := fmt.Sprintf("%s-etos-logarea", cluster.Name)
	logger := log.FromContext(ctx, "Reconciler", "ETOSLogArea", "BaseName", name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}

	_, err = r.reconcileDeployment(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the deployment for the ETOS LogArea")
		return err
	}
	_, err = r.reconcileServiceAccount(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the service account for the ETOS LogArea")
		return err
	}
	_, err = r.reconcileService(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the service for the ETOS LogArea")
		return err
	}
	return nil
}

// reconcileDeployment will reconcile the ETOS logarea deployment to its expected state.
func (r *ETOSLogAreaDeployment) reconcileDeployment(ctx context.Context, logger logr.Logger, name types.NamespacedName, cluster *etosv1alpha1.Cluster) (*appsv1.Deployment, error) {
	target := r.deployment(name, cluster)
	if err := ctrl.SetControllerReference(cluster, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, name, deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			return deployment, err
		}
		logger.Info("Creating a new deployment for the ETOS LogArea")
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

// reconcileServiceAccount will reconcile the ETOS logarea service account to its expected state.
func (r *ETOSLogAreaDeployment) reconcileServiceAccount(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
	target := r.serviceaccount(name, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	serviceaccount := &corev1.ServiceAccount{}
	if err := r.Get(ctx, name, serviceaccount); err != nil {
		if !apierrors.IsNotFound(err) {
			return serviceaccount, err
		}
		logger.Info("Creating a new service account for the ETOS LogArea")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(serviceaccount))
}

// reconcileService will reconcile the ETOS logarea service to its expected state.
func (r *ETOSLogAreaDeployment) reconcileService(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	target := r.service(name, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		logger.Info("Creating a new kubernetes service for the ETOS LogArea")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// serviceaccount creates a service account resource definition for the ETOS logarea.
func (r *ETOSLogAreaDeployment) serviceaccount(name types.NamespacedName, clusterName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: r.meta(name, clusterName),
	}
}

// deployment creates a deployment resource definition for the ETOS logarea.
func (r *ETOSLogAreaDeployment) deployment(name types.NamespacedName, cluster *etosv1alpha1.Cluster) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: r.meta(name, cluster.GetName()),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      name.Name,
					"app.kubernetes.io/part-of":   "etos",
					"app.kubernetes.io/component": "logarea",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: r.meta(name, cluster.GetName()),
				Spec: corev1.PodSpec{
					ServiceAccountName: name.Name,
					Containers:         []corev1.Container{r.container(name, cluster)},
				},
			},
		},
	}
}

// service creates a service resource definition for the ETOS logarea.
func (r *ETOSLogAreaDeployment) service(name types.NamespacedName, clusterName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name, clusterName),
		Spec: corev1.ServiceSpec{
			Ports: r.ports(),
			Selector: map[string]string{
				"app.kubernetes.io/name":      name.Name,
				"app.kubernetes.io/part-of":   "etos",
				"app.kubernetes.io/component": "logarea",
			},
		},
	}
}

// container creates the container resource for the ETOS logarea deployment.
func (r *ETOSLogAreaDeployment) container(name types.NamespacedName, cluster *etosv1alpha1.Cluster) corev1.Container {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/logarea/v1alpha/selftest/ping",
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
		Image:           config.ImageOrDefault(r.cfg.LogArea, r.Image),
		ImagePullPolicy: config.PullPolicyOrDefault(r.cfg.LogArea, r.Image),
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("200m"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: logAreaPort,
				Protocol:      "TCP",
			},
		},
		LivenessProbe:  probe,
		ReadinessProbe: probe,
		Env:            r.environment(cluster),
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-encryption-key", cluster.Name),
					},
				},
			},
		},
	}
}

// environment creates the environment resource for the ETOS logarea deployment.
func (r *ETOSLogAreaDeployment) environment(cluster *etosv1alpha1.Cluster) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "SERVICE_HOST",
			Value: "0.0.0.0",
		},
		{
			Name:  "ETOS_ETCD_HOST",
			Value: cluster.Spec.Database.Etcd.Host,
		},
		{
			Name:  "ETOS_ETCD_PORT",
			Value: cluster.Spec.Database.Etcd.Port,
		},
	}
}

// meta creates the common meta resource for the ETOS logarea deployment.
func (r *ETOSLogAreaDeployment) meta(name types.NamespacedName, clusterName string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name":                  name.Name,
			"app.kubernetes.io/part-of":               "etos",
			"app.kubernetes.io/component":             "logarea",
			"etos.eiffel-community.github.io/cluster": clusterName,
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// ports creates the port resource for the ETOS logarea service.
func (r *ETOSLogAreaDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: LogAreaServicePort, Name: "http", Protocol: "TCP", TargetPort: intstr.FromString("http")},
	}
}
