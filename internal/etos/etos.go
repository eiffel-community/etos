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

package etos

import (
	"context"
	"fmt"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosapi "github.com/eiffel-community/etos/internal/etos/api"
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

type ETOSDeployment struct {
	etosv1alpha1.ETOS
	client.Client
	Scheme           *runtime.Scheme
	rabbitmqSecret   string
	messagebusSecret string
}

// NewETOSDeployment will create a new ETOSDeployment reconciler.
func NewETOSDeployment(spec etosv1alpha1.ETOS, scheme *runtime.Scheme, client client.Client, rabbitmqSecret string, messagebusSecret string) (*ETOSDeployment, error) {
	return &ETOSDeployment{spec, client, scheme, rabbitmqSecret, messagebusSecret}, nil
}

// Reconcile will reconcile ETOS to its expected state.
func (r *ETOSDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	namespacedName := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	if _, err := r.reconcileIngress(ctx, namespacedName, cluster); err != nil {
		return err
	}
	configmap, err := r.reconcileConfigmap(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}

	api, err := etosapi.NewETOSApiDeployment(r.API, r.Scheme, r.Client, r.rabbitmqSecret, r.messagebusSecret, configmap.Name)
	if err != nil {
		return err
	}
	if err := api.Reconcile(ctx, cluster); err != nil {
		return err
	}

	sse, err := etosapi.NewETOSSSEDeployment(r.SSE, r.Scheme, r.Client)
	if err != nil {
		return err
	}
	if err := sse.Reconcile(ctx, cluster); err != nil {
		return err
	}

	logarea, err := etosapi.NewETOSLogAreaDeployment(r.LogArea, r.Scheme, r.Client)
	if err != nil {
		return err
	}
	if err := logarea.Reconcile(ctx, cluster); err != nil {
		return err
	}

	return nil
}

// reconcileIngress will reconcile the ETOS ingress to its expected state.
func (r *ETOSDeployment) reconcileIngress(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*networkingv1.Ingress, error) {
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
	return ingress, r.Patch(ctx, target, client.StrategicMergeFrom(ingress))
}

// reconcileConfigmap will reconcile the ETOS configmap to its expected state.
func (r *ETOSDeployment) reconcileConfigmap(ctx context.Context, name types.NamespacedName, cluster *etosv1alpha1.Cluster) (*corev1.ConfigMap, error) {
	target := r.configmap(name, cluster)
	if err := ctrl.SetControllerReference(cluster, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	configmap := &corev1.ConfigMap{}
	if err := r.Get(ctx, name, configmap); err != nil {
		if !apierrors.IsNotFound(err) {
			return configmap, err
		}
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return configmap, r.Patch(ctx, target, client.StrategicMergeFrom(configmap))
}

// ingress creates an ingress resource definition for ETOS.
func (r *ETOSDeployment) ingress(name types.NamespacedName) *networkingv1.Ingress {
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

// configmap creates a configmap definition for ETOS.
func (r *ETOSDeployment) configmap(name types.NamespacedName, cluster *etosv1alpha1.Cluster) *corev1.ConfigMap {
	etosHost := name.Name
	if r.Ingress.Host != "" {
		etosHost = r.Ingress.Host
	}
	etosApi := fmt.Sprintf("http://%s/api", etosHost)
	if r.Config.ETOSApiURL != "" {
		etosApi = r.Config.ETOSApiURL
	}
	eventRepository := cluster.Spec.EventRepository.Host
	if r.Config.ETOSEventRepositoryURL != "" {
		eventRepository = r.Config.ETOSEventRepositoryURL
	}

	data := map[string]string{
		"ETOS_GRAPHQL_SERVER": eventRepository,
		"ETOS_NAMESPACE":      cluster.Namespace,
		"SOURCE_HOST":         r.Config.Source,
		"ETOS_API":            etosApi,

		"ETOS_ETCD_HOST": cluster.Spec.Database.Etcd.Host,
		"ETOS_ETCD_PORT": cluster.Spec.Database.Etcd.Port,

		// Eiffel
		"EIFFEL_RABBITMQ_SECRET": r.rabbitmqSecret,
		// Logs
		"ETOS_RABBITMQ_SECRET": r.messagebusSecret,

		"ETOS_CONFIGMAP": name.Name,
		"DEV":            r.Config.Dev,

		// TODO: A few of these seem redundant
		"ESR_WAIT_FOR_ENVIRONMENT_TIMEOUT":        r.Config.EnvironmentTimeout,
		"ETOS_WAIT_FOR_IUT_TIMEOUT":               r.Config.EnvironmentTimeout,
		"ENVIRONMENT_PROVIDER_EVENT_DATA_TIMEOUT": r.Config.EventDataTimeout,
		"ENVIRONMENT_PROVIDER_TEST_SUITE_TIMEOUT": r.Config.TestSuiteTimeout,
	}
	if r.Config.Timezone != "" {
		data["TZ"] = r.Config.Timezone
	}
	return &corev1.ConfigMap{
		ObjectMeta: r.meta(name),
		Data:       data,
	}
}

// meta creates a common meta object for kubernetes resources.
func (r *ETOSDeployment) meta(name types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name": name.Name,
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// ingressRule creates the ingress rules for ETOS.
func (r *ETOSDeployment) ingressRule(name types.NamespacedName) networkingv1.IngressRule {
	// TODO: Hard-coded names.
	prefix := networkingv1.PathTypePrefix
	ingressRule := networkingv1.IngressRule{
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					{
						Path:     "/api",
						PathType: &prefix,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: fmt.Sprintf("%s-etos-api", name.Name),
								Port: networkingv1.ServiceBackendPort{
									Number: etosapi.ApiServicePort,
								},
							},
						},
					},
					{
						Path:     "/sse",
						PathType: &prefix,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: fmt.Sprintf("%s-etos-sse", name.Name),
								Port: networkingv1.ServiceBackendPort{
									Number: etosapi.SSEServicePort,
								},
							},
						},
					},
					{
						Path:     "/logarea",
						PathType: &prefix,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: fmt.Sprintf("%s-etos-logarea", name.Name),
								Port: networkingv1.ServiceBackendPort{
									Number: etosapi.LogAreaServicePort,
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
