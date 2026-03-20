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
	"net/url"
	"time"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/config"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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
	ssePort        int32 = 8080
	SSEServicePort int32 = 80
)

type ETOSSSEDeployment struct {
	etosv1alpha1.ETOSSSE
	client.Client
	Scheme           *runtime.Scheme
	messagebusSecret string
	restartRequired  bool
	cfg              config.Config
}

// NewETOSSSEDeployment will create a new ETOS SSE reconciler.
func NewETOSSSEDeployment(spec etosv1alpha1.ETOSSSE, scheme *runtime.Scheme, client client.Client, messagebusSecret string, cfg config.Config) *ETOSSSEDeployment {
	return &ETOSSSEDeployment{spec, client, scheme, messagebusSecret, false, cfg}
}

// Reconcile will reconcile the ETOS SSE service to its expected state.
func (r *ETOSSSEDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	var err error
	name := fmt.Sprintf("%s-etos-sse", cluster.Name)
	logger := log.FromContext(ctx, "Reconciler", "ETOSSSE", "BaseName", name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}

	cfg, err := r.reconcileConfig(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the config for the ETOS API")
		return err
	}
	_, err = r.reconcileDeployment(ctx, logger, namespacedName, cfg.ObjectMeta.Name, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the deployment for the ETOS SSE")
		return err
	}

	_, err = r.reconcileRole(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the role for the ETOS SSE")
		return err
	}
	_, err = r.reconcileServiceAccount(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the service account for the ETOS SSE")
		return err
	}
	_, err = r.reconcileRolebinding(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the role binding for the ETOS SSE")
		return err
	}
	_, err = r.reconcileService(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the service for the ETOS SSE")
		return err
	}
	return nil
}

// reconcileConfig will reconcile the secret to use as configuration for ETOS SSE.
func (r *ETOSSSEDeployment) reconcileConfig(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	name = types.NamespacedName{Name: fmt.Sprintf("%s-cfg", name.Name), Namespace: name.Namespace}
	target, err := r.config(ctx, name, owner.GetName())
	if err != nil {
		return nil, err
	}
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, name, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return secret, err
		}
		r.restartRequired = true
		logger.Info("Creating a new config for the ETOS API")
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

// reconcileDeployment will reconcile the ETOS SSE deployment to its expected state.
func (r *ETOSSSEDeployment) reconcileDeployment(ctx context.Context, logger logr.Logger, name types.NamespacedName, secretName string, owner metav1.Object) (*appsv1.Deployment, error) {
	target := r.deployment(name, secretName, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, name, deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			return deployment, err
		}
		logger.Info("Creating a new deployment for the SSE server")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
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

// reconcileRole will reconcile the ETOS SSE service account role to its expected state.
func (r *ETOSSSEDeployment) reconcileRole(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*rbacv1.Role, error) {
	labelName := name.Name
	name.Name = fmt.Sprintf("%s:sa:esr-reader", name.Name)

	target := r.role(name, labelName, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	role := &rbacv1.Role{}
	if err := r.Get(ctx, name, role); err != nil {
		if !apierrors.IsNotFound(err) {
			return role, err
		}
		logger.Info("Creating a new role for the SSE server")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(role))
}

// reconcileServiceAccount will reconcile the ETOS SSE service account to its expected state.
func (r *ETOSSSEDeployment) reconcileServiceAccount(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
	target := r.serviceaccount(name, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	serviceaccount := &corev1.ServiceAccount{}
	if err := r.Get(ctx, name, serviceaccount); err != nil {
		if !apierrors.IsNotFound(err) {
			return serviceaccount, err
		}
		logger.Info("Creating a new service account for the SSE server")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(serviceaccount))
}

// reconcileRolebinding will reconcile the ETOS SSE service account rolebinding to its expected state.
func (r *ETOSSSEDeployment) reconcileRolebinding(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*rbacv1.RoleBinding, error) {
	target := r.rolebinding(name, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	rolebinding := &rbacv1.RoleBinding{}
	if err := r.Get(ctx, name, rolebinding); err != nil {
		if !apierrors.IsNotFound(err) {
			return rolebinding, err
		}
		logger.Info("Creating a new role binding for the SSE server")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(rolebinding))
}

// reconcileService will reconcile the ETOS SSE service to its expected state.
func (r *ETOSSSEDeployment) reconcileService(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	target := r.service(name, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		logger.Info("Creating a new kubernetes service for the SSE server")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// config creates a new Secret to be used as configuration for the ETO SSE service.
func (r *ETOSSSEDeployment) config(ctx context.Context, name types.NamespacedName, clusterName string) (*corev1.Secret, error) {
	etos := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.messagebusSecret, Namespace: name.Namespace}, etos); err != nil {
		return nil, err
	}
	rabbitmqURL := url.URL{}
	if d, ok := etos.Data["ETOS_RABBITMQ_SSL"]; ok {
		if string(d) == "true" {
			rabbitmqURL.Scheme = "amqps"
		} else {
			rabbitmqURL.Scheme = "amqp"
		}
	} else {
		rabbitmqURL.Scheme = "amqp"
	}
	if d, ok := etos.Data["ETOS_RABBITMQ_USERNAME"]; ok {
		rabbitmqURL.User = url.User(string(d))
	}
	if d, ok := etos.Data["ETOS_RABBITMQ_PASSWORD"]; ok {
		if rabbitmqURL.User != nil {
			rabbitmqURL.User = url.UserPassword(rabbitmqURL.User.Username(), string(d))
		} else {
			rabbitmqURL.User = url.UserPassword("", string(d))
		}
	}
	if d, ok := etos.Data["ETOS_RABBITMQ_HOST"]; ok {
		rabbitmqURL.Host = string(d)
	} else {
		return nil, fmt.Errorf("ETOS_RABBITMQ_HOST is required in the message bus secret")
	}
	if d, ok := etos.Data["ETOS_RABBITMQ_STREAM_PORT"]; ok {
		rabbitmqURL.Host = fmt.Sprintf("%s:%s", rabbitmqURL.Host, string(d))
	} else {
		return nil, fmt.Errorf("ETOS_RABBITMQ_STREAM_PORT is required in the message bus secret")
	}
	if d, ok := etos.Data["ETOS_RABBITMQ_VHOST"]; ok {
		rabbitmqURL.Path = string(d)
	} else {
		return nil, fmt.Errorf("ETOS_RABBITMQ_VHOST is required in the message bus secret")
	}

	data := make(map[string][]byte)
	data["ETOS_RABBITMQ_URI"] = []byte(rabbitmqURL.String())
	data["ETOS_RABBITMQ_STREAM_NAME"] = etos.Data["ETOS_RABBITMQ_STREAM_NAME"]

	return &corev1.Secret{
		ObjectMeta: r.meta(name, clusterName),
		Data:       data,
	}, nil
}

// role creates a role resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) role(name types.NamespacedName, labelName, clusterName string) *rbacv1.Role {
	meta := r.meta(types.NamespacedName{Name: labelName, Namespace: name.Namespace}, clusterName)
	meta.Name = name.Name
	meta.Annotations["rbac.authorization.kubernetes.io/autoupdate"] = "true"
	return &rbacv1.Role{
		ObjectMeta: meta,
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{
					"batch",
				},
				Resources: []string{
					"jobs",
				},
				Verbs: []string{
					"get", "list", "watch",
				},
			},
			{
				APIGroups: []string{""},
				Resources: []string{
					"pods",
				},
				Verbs: []string{
					"get", "list", "watch",
				},
			},
		},
	}
}

// serviceaccount creates a serviceaccount resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) serviceaccount(name types.NamespacedName, clusterName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: r.meta(name, clusterName),
	}
}

// rolebinding creates a rolebinding resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) rolebinding(name types.NamespacedName, clusterName string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: r.meta(name, clusterName),
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     fmt.Sprintf("%s:sa:esr-reader", name.Name),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: name.Name,
			},
		},
	}
}

// deployment creates a deployment resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) deployment(name types.NamespacedName, secretName, clusterName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: r.meta(name, clusterName),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      name.Name,
					"app.kubernetes.io/part-of":   "etos",
					"app.kubernetes.io/component": "sse",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: r.meta(name, clusterName),
				Spec: corev1.PodSpec{
					ServiceAccountName: name.Name,
					Containers:         []corev1.Container{r.container(name, secretName)},
				},
			},
		},
	}
}

// service creates a service resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) service(name types.NamespacedName, clusterName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name, clusterName),
		Spec: corev1.ServiceSpec{
			Ports: r.ports(),
			Selector: map[string]string{
				"app.kubernetes.io/name":      name.Name,
				"app.kubernetes.io/part-of":   "etos",
				"app.kubernetes.io/component": "sse",
			},
		},
	}
}

// container creates a container resource definition for the ETOS SSE deployment.
func (r *ETOSSSEDeployment) container(name types.NamespacedName, secretName string) corev1.Container {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/sse/v1alpha/selftest/ping",
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
		Image:           config.ImageOrDefault(r.cfg.SSE, r.Image),
		ImagePullPolicy: config.PullPolicyOrDefault(r.cfg.SSE, r.Image),
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
				ContainerPort: ssePort,
				Protocol:      "TCP",
			},
		},
		LivenessProbe:  probe,
		ReadinessProbe: probe,
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
				},
			},
		},
		Env: r.environment(),
	}
}

// environment creates an environment resource definition for the ETOS SSE deployment.
func (r *ETOSSSEDeployment) environment() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "SERVICE_HOST",
			Value: "0.0.0.0",
		},
	}
}

// meta creates a common meta resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) meta(name types.NamespacedName, clusterName string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name":                  name.Name,
			"app.kubernetes.io/part-of":               "etos",
			"app.kubernetes.io/component":             "sse",
			"etos.eiffel-community.github.io/cluster": clusterName,
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// ports creates a service port resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: SSEServicePort, Name: "http", Protocol: "TCP", TargetPort: intstr.FromString("http")},
	}
}
