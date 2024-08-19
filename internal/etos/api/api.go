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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ApiServicePort int32 = 80
	apiPort        int32 = 8080
)

type ETOSApiDeployment struct {
	etosv1alpha1.ETOSApi
	client.Client
	Scheme           *runtime.Scheme
	rabbitmqSecret   string
	messagebusSecret string
	configmap        string
}

// NewETOSApiDeployment will create a new ETOS API reconciler.
func NewETOSApiDeployment(spec etosv1alpha1.ETOSApi, scheme *runtime.Scheme, client client.Client, rabbitmqSecret string, messagebusSecret string, configmap string) (*ETOSApiDeployment, error) {
	return &ETOSApiDeployment{spec, client, scheme, rabbitmqSecret, messagebusSecret, configmap}, nil
}

// Reconcile will reconcile the ETOS API to its expected state.
func (r *ETOSApiDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	var err error
	name := fmt.Sprintf("%s-etos-api", cluster.Name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}

	_, err = r.reconcileDeployment(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileSecret(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileRole(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileServiceAccount(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileRolebinding(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileService(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	return nil
}

// reconcileDeployment will reconcile the ETOS API deployment to its expected state.
func (r *ETOSApiDeployment) reconcileDeployment(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*appsv1.Deployment, error) {
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

// reconcileSecret will reconcile the ETOS API service account secret to its expected state.
func (r *ETOSApiDeployment) reconcileSecret(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
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
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileRole will reconcile the ETOS API service account role to its expected state.
func (r *ETOSApiDeployment) reconcileRole(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*rbacv1.Role, error) {
	labelName := name.Name
	name.Name = fmt.Sprintf("%s:sa:esr-handler", name.Name)

	target := r.role(name, labelName)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	role := &rbacv1.Role{}
	if err := r.Get(ctx, name, role); err != nil {
		if !apierrors.IsNotFound(err) {
			return role, err
		}
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(role))
}

// reconcileServiceAccount will reconcile the ETOS API service account to its expected state.
func (r *ETOSApiDeployment) reconcileServiceAccount(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
	target := r.serviceaccount(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	serviceaccount := &corev1.ServiceAccount{}
	if err := r.Get(ctx, name, serviceaccount); err != nil {
		if !apierrors.IsNotFound(err) {
			return serviceaccount, err
		}
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(serviceaccount))
}

// reconcileRolebinding will reconcile the ETOS API service account role binding to its expected state.
func (r *ETOSApiDeployment) reconcileRolebinding(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*rbacv1.RoleBinding, error) {
	target := r.rolebinding(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	rolebinding := &rbacv1.RoleBinding{}
	if err := r.Get(ctx, name, rolebinding); err != nil {
		if !apierrors.IsNotFound(err) {
			return rolebinding, err
		}
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(rolebinding))
}

// reconcileService will reconcile the ETOS API service to its expected state.
func (r *ETOSApiDeployment) reconcileService(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	target := r.service(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// secret creates a secret resource definition for the ETOS API.
func (r *ETOSApiDeployment) secret(name types.NamespacedName) *corev1.Secret {
	meta := r.meta(name)
	meta.Annotations["kubernetes.io/service-account.name"] = name.Name
	name.Name = fmt.Sprintf("%s-token", name.Name)
	return &corev1.Secret{
		ObjectMeta: meta,
		Type:       corev1.SecretTypeServiceAccountToken,
	}
}

// role creates a role resource definition for the ETOS API.
func (r *ETOSApiDeployment) role(name types.NamespacedName, labelName string) *rbacv1.Role {
	meta := r.meta(types.NamespacedName{Name: labelName, Namespace: name.Namespace})
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
					"get", "delete", "list", "watch",
				},
			},
			{
				APIGroups: []string{"etos.eiffel-community.github.io"},
				Resources: []string{
					"testruns",
				},
				Verbs: []string{
					"create", "get", "delete", "list", "watch", "deletecollection",
				},
			},
			{
				APIGroups: []string{"etos.eiffel-community.github.io"},
				Resources: []string{
					"environments",
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

// serviceaccount creates a service account resource definition for the ETOS API.
func (r *ETOSApiDeployment) serviceaccount(name types.NamespacedName) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: r.meta(name),
	}
}

// rolebinding creates a rolebinding resource definition for the ETOS API.
func (r *ETOSApiDeployment) rolebinding(name types.NamespacedName) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: r.meta(name),
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     fmt.Sprintf("%s:sa:esr-handler", name.Name),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: name.Name,
			},
		},
	}
}

// deployment creates a deployment resource definition for the ETOS API.
func (r *ETOSApiDeployment) deployment(name types.NamespacedName) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: r.meta(name),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      name.Name,
					"app.kubernetes.io/part-of":   "etos",
					"app.kubernetes.io/component": "api",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: r.meta(name),
				Spec: corev1.PodSpec{
					ServiceAccountName: name.Name,
					Containers:         []corev1.Container{r.container(name)},
				},
			},
		},
	}
}

// service creates a service resource definition for the ETOS API.
func (r *ETOSApiDeployment) service(name types.NamespacedName) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name),
		Spec: corev1.ServiceSpec{
			Ports: r.ports(),
			Selector: map[string]string{
				"app.kubernetes.io/name":      name.Name,
				"app.kubernetes.io/part-of":   "etos",
				"app.kubernetes.io/component": "api",
			},
		},
	}
}

// container creates the container resource for the ETOS API deployment.
func (r *ETOSApiDeployment) container(name types.NamespacedName) corev1.Container {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/selftest/ping",
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
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: apiPort,
				Protocol:      "TCP",
			},
		},
		LivenessProbe:  probe,
		ReadinessProbe: probe,
		EnvFrom:        r.environment(),
	}
}

// environment creates the environment resource for the ETOS API deployment.
func (r *ETOSApiDeployment) environment() []corev1.EnvFromSource {
	return []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.rabbitmqSecret,
				},
			},
		},
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.messagebusSecret,
				},
			},
		},
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.configmap,
				},
			},
		},
	}
}

// meta creates the common meta resource for the ETOS API deployment.
func (r *ETOSApiDeployment) meta(name types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name":      name.Name,
			"app.kubernetes.io/part-of":   "etos",
			"app.kubernetes.io/component": "api",
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// ports creates the port resource for the ETOS API service.
func (r *ETOSApiDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: ApiServicePort, Name: "http", Protocol: "TCP", TargetPort: intstr.FromString("http")},
	}
}
