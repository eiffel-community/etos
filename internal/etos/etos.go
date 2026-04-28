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
	"errors"
	"fmt"
	"maps"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/internal/config"
	etosapi "github.com/eiffel-community/etos/internal/etos/api"
	etossuitestarter "github.com/eiffel-community/etos/internal/etos/suitestarter"
	"github.com/eiffel-community/etos/internal/readiness"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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

type ETOSDeployment struct {
	etosv1alpha1.ETOS
	client.Client
	Scheme           *runtime.Scheme
	rabbitmqSecret   string
	messagebusSecret string
	cfg              config.Config
}

// NewETOSDeployment will create a new ETOSDeployment reconciler.
func NewETOSDeployment(spec etosv1alpha1.ETOS, sch *runtime.Scheme, cli client.Client, rabbitmqSecret string, messagebusSecret string, cfg config.Config) *ETOSDeployment {
	return &ETOSDeployment{spec, cli, sch, rabbitmqSecret, messagebusSecret, cfg}
}

// Reconcile will reconcile ETOS to its expected state.
func (r *ETOSDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	var err error

	logger := log.FromContext(ctx, "Reconciler", "ETOS", "BaseName", cluster.Name)
	namespacedName := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	if _, err := r.reconcileIngress(ctx, namespacedName, cluster); err != nil {
		logger.Error(err, "Ingress reconciliation failed")
		return err
	}

	_, err = r.reconcileRole(ctx, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Role reconciliation failed")
		return err
	}

	_, err = r.reconcileServiceAccount(ctx, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "ServiceAccount reconciliation failed")
		return err
	}

	_, err = r.reconcileRolebinding(ctx, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Rolebinding reconciliation failed")
		return err
	}

	cfg, err := r.reconcileConfig(ctx, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Config reconciliation failed")
		return err
	}

	encryption, err := r.reconcileSecret(ctx, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Secret reconciliation failed")
		return err
	}

	_, err = r.reconcileEnvironmentProviderConfig(ctx, namespacedName, encryption.Name, cfg.Name, cluster)
	if err != nil {
		logger.Error(err, "Environment provider config reconciliation failed")
		return err
	}

	_, err = r.reconcileIutProvider(ctx, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "IUT provider reconciliation failed")
		return err
	}

	_, err = r.reconcileLogAreaProvider(ctx, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Log area provider reconciliation failed")
		return err
	}

	_, err = r.reconcileExecutionSpaceProvider(ctx, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Execution space provider reconciliation failed")
		return err
	}
	var notReadyErr error

	api := etosapi.NewETOSApiDeployment(r.API, r.Scheme, r.Client, r.rabbitmqSecret, r.messagebusSecret, cfg.Name, r.cfg)
	if err := api.Reconcile(ctx, cluster); err != nil {
		if !readiness.IsNotReadyError(err) {
			logger.Error(err, "ETOS API reconciliation failed")
			return err
		}
		notReadyErr = errors.Join(notReadyErr, err)
	}

	sse := etosapi.NewETOSSSEDeployment(r.SSE, r.Scheme, r.Client, r.messagebusSecret, r.cfg)
	if err := sse.Reconcile(ctx, cluster); err != nil {
		if !readiness.IsNotReadyError(err) {
			logger.Error(err, "ETOS SSE reconciliation failed")
			return err
		}
		notReadyErr = errors.Join(notReadyErr, err)
	}

	logarea := etosapi.NewETOSLogAreaDeployment(r.LogArea, r.Scheme, r.Client, r.cfg)
	if err := logarea.Reconcile(ctx, cluster); err != nil {
		if !readiness.IsNotReadyError(err) {
			logger.Error(err, "ETOS LogArea reconciliation failed")
			return err
		}
		notReadyErr = errors.Join(notReadyErr, err)
	}

	suitestarter := etossuitestarter.NewETOSSuiteStarterDeployment(r.SuiteStarter, r.Scheme, r.Client, r.rabbitmqSecret, r.messagebusSecret, cfg, encryption, r.cfg)
	if err := suitestarter.Reconcile(ctx, cluster); err != nil {
		if !readiness.IsNotReadyError(err) {
			logger.Error(err, "ETOS SuiteStarter reconciliation failed")
			return err
		}
		notReadyErr = errors.Join(notReadyErr, err)
	}

	return notReadyErr
}

// reconcileIngress will reconcile the ETOS ingress to its expected state.
func (r *ETOSDeployment) reconcileIngress(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*networkingv1.Ingress, error) {
	logger := log.FromContext(ctx)
	target := r.ingress(name, owner.GetName())
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
			logger.Info("ETOS ingress enabled, creating")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Ingress.Enabled {
		logger.Info("ETOS ingress disabled, removing")
		return nil, r.Delete(ctx, ingress)
	}
	if equality.Semantic.DeepDerivative(target.Spec, ingress.Spec) {
		return ingress, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(ingress))
}

// reconcileRole will reconcile the ETOS API service account role to its expected state.
func (r *ETOSDeployment) reconcileRole(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*rbacv1.Role, error) {
	logger := log.FromContext(ctx)
	name.Name = fmt.Sprintf("%s-provider", name.Name)

	labelName := name.Name
	name.Name = fmt.Sprintf("%s:sa:environment-provider", name.Name)

	target := r.role(name, labelName, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	role := &rbacv1.Role{}
	if err := r.Get(ctx, name, role); err != nil {
		if !apierrors.IsNotFound(err) {
			return role, err
		}
		logger.Info("Creating an ETOS environment provider role", "roleName", name.Name)
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(role))
}

// reconcileServiceAccount will reconcile the ETOS API service account to its expected state.
func (r *ETOSDeployment) reconcileServiceAccount(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
	logger := log.FromContext(ctx)
	name.Name = fmt.Sprintf("%s-provider", name.Name)

	target := r.serviceaccount(name, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	serviceaccount := &corev1.ServiceAccount{}
	if err := r.Get(ctx, name, serviceaccount); err != nil {
		if !apierrors.IsNotFound(err) {
			return serviceaccount, err
		}
		logger.Info("Creating an ETOS service account", "serviceAccountName", name.Name)
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(serviceaccount))
}

// reconcileRolebinding will reconcile the ETOS API service account role binding to its expected state.
func (r *ETOSDeployment) reconcileRolebinding(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*rbacv1.RoleBinding, error) {
	logger := log.FromContext(ctx)
	name.Name = fmt.Sprintf("%s-provider", name.Name)

	target := r.rolebinding(name, owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	rolebinding := &rbacv1.RoleBinding{}
	if err := r.Get(ctx, name, rolebinding); err != nil {
		if !apierrors.IsNotFound(err) {
			return rolebinding, err
		}
		logger.Info("Creating role binding for ETOS", "roleBindingName", name.Name)
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(rolebinding))
}

// reconcileConfig will reconcile the ETOS config to its expected state.
func (r *ETOSDeployment) reconcileConfig(ctx context.Context, name types.NamespacedName, cluster *etosv1alpha1.Cluster) (*corev1.Secret, error) {
	logger := log.FromContext(ctx)
	name = types.NamespacedName{Name: fmt.Sprintf("%s-cfg", name.Name), Namespace: name.Namespace}
	target := r.config(name, cluster)
	if err := ctrl.SetControllerReference(cluster, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	secret := &corev1.Secret{}
	if err := r.Get(ctx, name, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return secret, err
		}
		logger.Info("Creating the ETOS configmap")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileSecret will reconcile the secret to its expected state.
func (r *ETOSDeployment) reconcileSecret(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	logger := log.FromContext(ctx)
	name = types.NamespacedName{Name: fmt.Sprintf("%s-encryption-key", name.Name), Namespace: name.Namespace}
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
			return secret, err
		}
		logger.Info("Creating the ETOS encryption key secret")
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

// reconcileEnvironmentProviderConfig will reconcile the secret to use as configuration for the ETOS environment provider.
func (r *ETOSDeployment) reconcileEnvironmentProviderConfig(ctx context.Context, name types.NamespacedName, encryptionKeyName, configmapName string, owner metav1.Object) (*corev1.Secret, error) {
	logger := log.FromContext(ctx)
	name = types.NamespacedName{Name: fmt.Sprintf("%s-environment-provider-cfg", name.Name), Namespace: name.Namespace}
	target, err := r.environmentProviderConfig(ctx, name, encryptionKeyName, configmapName, owner.GetName())
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
		logger.Info("Creating the ETOS environment provider configmap")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileIutProvider will reconcile the IUT provider to its expected state.
func (r *ETOSDeployment) reconcileIutProvider(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*etosv1alpha1.Provider, error) {
	logger := log.FromContext(ctx)
	name = types.NamespacedName{Name: fmt.Sprintf("%s-iut-provider", name.Name), Namespace: name.Namespace}
	target := r.provider(name, "iut", config.ImageOrDefault(r.cfg.IutProvider, etosv1alpha1.Image{}), owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	provider := &etosv1alpha1.Provider{}
	if err := r.Get(ctx, name, provider); err != nil {
		if !apierrors.IsNotFound(err) {
			return provider, err
		}
		logger.Info("Creating the IUT provider")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	// Patching does not work if the resource version is not set, and it is not set
	// by default when creating custom resources.
	target.ObjectMeta = provider.ObjectMeta
	return target, r.Patch(ctx, target, client.MergeFrom(provider))
}

// reconcileLogAreaProvider will reconcile the LogArea provider to its expected state.
func (r *ETOSDeployment) reconcileLogAreaProvider(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*etosv1alpha1.Provider, error) {
	logger := log.FromContext(ctx)
	clusterName := name.Name
	name = types.NamespacedName{Name: fmt.Sprintf("%s-log-area-provider", name.Name), Namespace: name.Namespace}
	target := r.provider(name, "log-area", config.ImageOrDefault(r.cfg.LogAreaProvider, etosv1alpha1.Image{}), owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	target.Spec.LogAreaProviderConfig = &etosv1alpha1.LogAreaProviderConfig{
		LiveLogs: fmt.Sprintf("http://%s-etos-logarea/logarea/v1alpha/$context", clusterName),
		Upload: etosv1alpha2.Upload{
			AsJSON: false,
			Method: "POST",
			URL:    fmt.Sprintf("http://%s-etos-logarea/logarea/v1alpha/upload", clusterName),
		},
	}
	scheme.Scheme.Default(target)

	provider := &etosv1alpha1.Provider{}
	if err := r.Get(ctx, name, provider); err != nil {
		if !apierrors.IsNotFound(err) {
			return provider, err
		}
		logger.Info("Creating the LogArea provider")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	// Patching does not work if the resource version is not set, and it is not set
	// by default when creating custom resources.
	target.ObjectMeta = provider.ObjectMeta
	return target, r.Patch(ctx, target, client.MergeFrom(provider))
}

// reconcileExecutionSpaceProvider will reconcile the Execution Space provider to its expected state.
func (r *ETOSDeployment) reconcileExecutionSpaceProvider(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*etosv1alpha1.Provider, error) {
	logger := log.FromContext(ctx)
	name = types.NamespacedName{Name: fmt.Sprintf("%s-execution-space-provider", name.Name), Namespace: name.Namespace}
	target := r.provider(name, "execution-space", config.ImageOrDefault(r.cfg.ExecutionSpaceProvider, etosv1alpha1.Image{}), owner.GetName())
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	provider := &etosv1alpha1.Provider{}
	if err := r.Get(ctx, name, provider); err != nil {
		if !apierrors.IsNotFound(err) {
			return provider, err
		}
		logger.Info("Creating the Execution Space provider")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	// Patching does not work if the resource version is not set, and it is not set
	// by default when creating custom resources.
	target.ObjectMeta = provider.ObjectMeta
	return target, r.Patch(ctx, target, client.MergeFrom(provider))
}

// provider creates a provider definition for the ETOS API.
func (r *ETOSDeployment) provider(name types.NamespacedName, providerType, image, clusterName string) *etosv1alpha1.Provider {
	return &etosv1alpha1.Provider{
		TypeMeta: metav1.TypeMeta{
			APIVersion: etosv1alpha1.GroupVersion.String(),
			Kind:       "Provider",
		},
		ObjectMeta: r.meta(name, clusterName),
		Spec: etosv1alpha1.ProviderSpec{
			Type:  providerType,
			Image: image,
		},
	}
}

// config creates a new Secret to be used as configuration for the ETOS API.
func (r *ETOSDeployment) environmentProviderConfig(ctx context.Context, name types.NamespacedName, encryptionKeyName, configmapName, clusterName string) (*corev1.Secret, error) {
	eiffel := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.rabbitmqSecret, Namespace: name.Namespace}, eiffel); err != nil {
		return nil, err
	}
	etos := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.messagebusSecret, Namespace: name.Namespace}, etos); err != nil {
		return nil, err
	}
	encryption := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: encryptionKeyName, Namespace: name.Namespace}, encryption); err != nil {
		return nil, err
	}
	cfg := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: configmapName, Namespace: name.Namespace}, cfg); err != nil {
		return nil, err
	}
	data := map[string][]byte{}
	maps.Copy(data, eiffel.Data)
	maps.Copy(data, etos.Data)
	maps.Copy(data, encryption.Data)
	maps.Copy(data, cfg.Data)
	return &corev1.Secret{
		ObjectMeta: r.meta(name, clusterName),
		Data:       data,
	}, nil
}

// ingress creates an ingress resource definition for ETOS.
func (r *ETOSDeployment) ingress(name types.NamespacedName, clusterName string) *networkingv1.Ingress {
	meta := r.meta(name, clusterName)

	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}

	maps.Copy(meta.Annotations, r.Ingress.Annotations)

	ingress := &networkingv1.Ingress{
		ObjectMeta: meta,
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{r.ingressRule(name)},
		},
	}

	if r.Ingress.IngressClass != "" {
		ingress.Spec.IngressClassName = &r.Ingress.IngressClass
	}
	return ingress
}

// config creates a secret definition for ETOS.
func (r *ETOSDeployment) config(name types.NamespacedName, cluster *etosv1alpha1.Cluster) *corev1.Secret {
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

	data := map[string][]byte{
		"ETOS_GRAPHQL_SERVER":                    []byte(eventRepository),
		"ETOS_CLUSTER":                           []byte(cluster.Name),
		"ETOS_NAMESPACE":                         []byte(cluster.Namespace),
		"ENVIRONMENT_PROVIDER_SERVICE_ACCOUNT":   fmt.Appendf(nil, "%s-provider", cluster.Name),
		"SOURCE_HOST":                            []byte(r.Config.Source),
		"ETOS_API":                               []byte(etosApi),
		"SUITE_RUNNER_IMAGE":                     []byte(config.ImageOrDefault(r.cfg.SuiteRunner, cluster.Spec.ETOS.SuiteRunner.Image)),
		"SUITE_RUNNER_IMAGE_PULL_POLICY":         []byte(config.PullPolicyOrDefault(r.cfg.SuiteRunner, cluster.Spec.ETOS.SuiteRunner.Image)),
		"LOG_LISTENER_IMAGE":                     []byte(config.ImageOrDefault(r.cfg.LogListener, cluster.Spec.ETOS.SuiteRunner.LogListener.Image)),
		"LOG_LISTENER_IMAGE_PULL_POLICY":         []byte(config.PullPolicyOrDefault(r.cfg.LogListener, cluster.Spec.ETOS.SuiteRunner.LogListener.Image)),
		"ENVIRONMENT_PROVIDER_IMAGE":             []byte(config.ImageOrDefault(r.cfg.EnvironmentProvider, cluster.Spec.ETOS.EnvironmentProvider.Image)),
		"ENVIRONMENT_PROVIDER_IMAGE_PULL_POLICY": []byte(config.PullPolicyOrDefault(r.cfg.EnvironmentProvider, cluster.Spec.ETOS.EnvironmentProvider.Image)),
		"ETR_VERSION":                            []byte(config.VersionOrDefault(r.cfg.TestRunner, cluster.Spec.ETOS.TestRunner.Version)),
		"ETOS_ROUTING_KEY_TAG":                   []byte(cluster.Spec.ETOS.Config.RoutingKeyTag),

		"ETOS_ETCD_HOST": []byte(cluster.Spec.Database.Etcd.Host),
		"ETOS_ETCD_PORT": []byte(cluster.Spec.Database.Etcd.Port),

		"DEV": []byte(r.Config.Dev),

		// TODO: A few of these seem redundant
		"ESR_WAIT_FOR_ENVIRONMENT_TIMEOUT":        []byte(r.Config.EnvironmentTimeout),
		"ETOS_WAIT_FOR_IUT_TIMEOUT":               []byte(r.Config.EnvironmentTimeout),
		"ETOS_EVENT_DATA_TIMEOUT":                 []byte(r.Config.EventDataTimeout),
		"ENVIRONMENT_PROVIDER_EVENT_DATA_TIMEOUT": []byte(r.Config.EventDataTimeout),
		"ENVIRONMENT_PROVIDER_TEST_SUITE_TIMEOUT": []byte(r.Config.TestSuiteTimeout),
		"ETOS_TEST_SUITE_TIMEOUT":                 []byte(r.Config.TestSuiteTimeout),
	}
	if cluster.Spec.ETOS.Config.TestRunRetention.Failure != nil {
		data["TESTRUN_FAILURE_RETENTION"] = []byte(cluster.Spec.ETOS.Config.TestRunRetention.Failure.Duration.String())
	}
	if cluster.Spec.ETOS.Config.TestRunRetention.Success != nil {
		data["TESTRUN_SUCCESS_RETENTION"] = []byte(cluster.Spec.ETOS.Config.TestRunRetention.Success.Duration.String())
	}
	if cluster.Spec.OpenTelemetry.Enabled {
		data["OTEL_EXPORTER_OTLP_ENDPOINT"] = []byte(cluster.Spec.OpenTelemetry.Endpoint)
		data["OTEL_EXPORTER_OTLP_INSECURE"] = []byte(cluster.Spec.OpenTelemetry.Insecure)
	}
	if r.Config.Timezone != "" {
		data["TZ"] = []byte(r.Config.Timezone)
	}
	return &corev1.Secret{
		ObjectMeta: r.meta(name, cluster.GetName()),
		Data:       data,
	}
}

// secret creates a secret definition for ETOS.
func (r *ETOSDeployment) secret(ctx context.Context, name types.NamespacedName, clusterName string) (*corev1.Secret, error) {
	value, err := r.Config.EncryptionKey.Get(ctx, r.Client, name.Namespace)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: r.meta(name, clusterName),
		Data: map[string][]byte{
			"ETOS_ENCRYPTION_KEY": value,
		},
	}, nil
}

// meta creates a common meta object for kubernetes resources.
func (r *ETOSDeployment) meta(name types.NamespacedName, clusterName string) metav1.ObjectMeta {
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

// role creates a role resource definition for the ETOS API.
func (r *ETOSDeployment) role(name types.NamespacedName, labelName, clusterName string) *rbacv1.Role {
	meta := r.meta(types.NamespacedName{Name: labelName, Namespace: name.Namespace}, clusterName)
	meta.Name = name.Name
	meta.Annotations["rbac.authorization.kubernetes.io/autoupdate"] = "true"
	return &rbacv1.Role{
		ObjectMeta: meta,
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{
					"etos.eiffel-community.github.io",
				},
				Resources: []string{
					"testruns",
					"providers",
					"environmentrequests",
				},
				Verbs: []string{
					"get", "list", "watch",
				},
			},
			{
				APIGroups: []string{"etos.eiffel-community.github.io"},
				Resources: []string{
					"environments", "iuts", "logarea", "executionspaces",
				},
				Verbs: []string{
					"create", "get", "list", "watch", "delete",
				},
			},
			{
				APIGroups: []string{"batch"},
				Resources: []string{
					"jobs",
				},
				Verbs: []string{
					"create", "get", "list", "watch", "delete",
				},
			},
			{
				APIGroups: []string{""},
				Resources: []string{
					"pods",
					"secrets",
				},
				Verbs: []string{
					"get", "list", "watch",
				},
			},
		},
	}
}

// serviceaccount creates a service account resource definition for the ETOS API.
func (r *ETOSDeployment) serviceaccount(name types.NamespacedName, clusterName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: r.meta(name, clusterName),
	}
}

// rolebinding creates a rolebinding resource definition for the ETOS API.
func (r *ETOSDeployment) rolebinding(name types.NamespacedName, clusterName string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: r.meta(name, clusterName),
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     fmt.Sprintf("%s:sa:environment-provider", name.Name),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: name.Name,
			},
		},
	}
}
