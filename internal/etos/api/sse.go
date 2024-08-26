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
	ssePort        int32 = 8080
	SSEServicePort int32 = 80
)

type ETOSSSEDeployment struct {
	etosv1alpha1.ETOSSSE
	client.Client
	Scheme *runtime.Scheme
}

// NewETOSSSEDeployment will create a new ETOS SSE reconciler.
func NewETOSSSEDeployment(spec etosv1alpha1.ETOSSSE, scheme *runtime.Scheme, client client.Client) *ETOSSSEDeployment {
	return &ETOSSSEDeployment{spec, client, scheme}
}

// Reconcile will reconcile the ETOS SSE service to its expected state.
func (r *ETOSSSEDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	var err error
	name := fmt.Sprintf("%s-etos-sse", cluster.Name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}

	_, err = r.reconcileDeployment(ctx, namespacedName, cluster)
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

// reconcileDeployment will reconcile the ETOS SSE deployment to its expected state.
func (r *ETOSSSEDeployment) reconcileDeployment(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*appsv1.Deployment, error) {
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

// reconcileRole will reconcile the ETOS SSE service account role to its expected state.
func (r *ETOSSSEDeployment) reconcileRole(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*rbacv1.Role, error) {
	labelName := name.Name
	name.Name = fmt.Sprintf("%s:sa:esr-reader", name.Name)

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

// reconcileServiceAccount will reconcile the ETOS SSE service account to its expected state.
func (r *ETOSSSEDeployment) reconcileServiceAccount(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
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

// reconcileRolebinding will reconcile the ETOS SSE service account rolebinding to its expected state.
func (r *ETOSSSEDeployment) reconcileRolebinding(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*rbacv1.RoleBinding, error) {
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

// reconcileService will reconcile the ETOS SSE service to its expected state.
func (r *ETOSSSEDeployment) reconcileService(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
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

// role creates a role resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) role(name types.NamespacedName, labelName string) *rbacv1.Role {
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
func (r *ETOSSSEDeployment) serviceaccount(name types.NamespacedName) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: r.meta(name),
	}
}

// rolebinding creates a rolebinding resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) rolebinding(name types.NamespacedName) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: r.meta(name),
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
func (r *ETOSSSEDeployment) deployment(name types.NamespacedName) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: r.meta(name),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      name.Name,
					"app.kubernetes.io/part-of":   "etos",
					"app.kubernetes.io/component": "sse",
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

// service creates a service resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) service(name types.NamespacedName) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name),
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
func (r *ETOSSSEDeployment) container(name types.NamespacedName) corev1.Container {
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
		Image:           r.Image.Image,
		ImagePullPolicy: r.ImagePullPolicy,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: ssePort,
				Protocol:      "TCP",
			},
		},
		LivenessProbe:  probe,
		ReadinessProbe: probe,
		Env:            r.environment(),
	}
}

// environment creates an environment resource definition for the ETOS SSE deployment.
func (r *ETOSSSEDeployment) environment() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "SERVICE_HOST",
			Value: "0.0.0.0",
		},
		{
			Name:  "STRIP_PREFIX",
			Value: "/sse",
		},
	}
}

// meta creates a common meta resource definition for the ETOS SSE service.
func (r *ETOSSSEDeployment) meta(name types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name":      name.Name,
			"app.kubernetes.io/part-of":   "etos",
			"app.kubernetes.io/component": "sse",
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
