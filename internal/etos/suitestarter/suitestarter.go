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

package suitestarter

import (
	"context"
	"fmt"
	"maps"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
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
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ETOSSuiteStarterDeployment struct {
	etosv1alpha1.ETOSSuiteStarter
	client.Client
	Scheme           *runtime.Scheme
	rabbitmqSecret   string
	messagebusSecret string
	etosConfig       *corev1.Secret
	encryptionSecret *corev1.Secret
}

// NewETOSSuiteStarterDeployment will create a new ETOS SuiteStarter reconciler.
func NewETOSSuiteStarterDeployment(spec etosv1alpha1.ETOSSuiteStarter, scheme *runtime.Scheme, client client.Client, rabbitmqSecret, messagebusSecret string, config *corev1.Secret, encryption *corev1.Secret) *ETOSSuiteStarterDeployment {
	return &ETOSSuiteStarterDeployment{spec, client, scheme, rabbitmqSecret, messagebusSecret, config, encryption}
}

// Reconcile will reconcile the ETOS suite starter to its expected state.
func (r *ETOSSuiteStarterDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	var err error
	name := fmt.Sprintf("%s-etos-suite-starter", cluster.Name)
	logger := log.FromContext(ctx, "Reconciler", "ETOSSuiteStarter", "BaseName", name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}
	_, err = r.reconcileSuiteRunnerServiceAccount(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the Suite runner service account")
		return err
	}
	// This secret is in use when running the TestRun controller. When the suite starter is removed, this MUST still be created.
	secret, err := r.reconcileSuiteRunnerSecret(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the Suite runner secret")
		return err
	}
	cfg, err := r.reconcileConfig(ctx, logger, secret.ObjectMeta.Name, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the Suite starter config")
		return err
	}
	template, err := r.reconcileTemplate(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the Suite runner template")
		return err
	}
	var suiteRunnerTemplateName string
	if r.SuiteRunnerTemplateSecretName == "" {
		suiteRunnerTemplateName = template.ObjectMeta.Name
	} else {
		suiteRunnerTemplateName = r.SuiteRunnerTemplateSecretName
	}
	logger.Info("Suite runner template", "suiteRunnerTemplateName", suiteRunnerTemplateName)
	_, err = r.reconcileDeployment(ctx, logger, cfg.ObjectMeta.Name, suiteRunnerTemplateName, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the deployment for the ETOS Suite Starter")
		return err
	}
	_, err = r.reconcileSecret(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the secret for the ETOS Suite Starter")
		return err
	}
	_, err = r.reconcileRole(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the role for the ETOS Suite Starter")
		return err
	}
	_, err = r.reconcileServiceAccount(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the service account for the ETOS Suite Starter")
		return err
	}
	_, err = r.reconcileRolebinding(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the role binding for the ETOS Suite Starter")
		return err
	}
	return err
}

// reconcileSuiteRunnerServiceAccount will reconcile the ETOS SuiteStarter service account to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileSuiteRunnerServiceAccount(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
	target := r.serviceaccount(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	serviceaccount := &corev1.ServiceAccount{}
	if err := r.Get(ctx, name, serviceaccount); err != nil {
		if !apierrors.IsNotFound(err) {
			return serviceaccount, err
		}
		logger.Info("Creating a new service account for the suite runner")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(serviceaccount))
}

// reconcileSuiteRunnerSecret will reconcile the ETOS suite runner secret to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileSuiteRunnerSecret(ctx context.Context, logger logr.Logger, name types.NamespacedName, cluster *etosv1alpha1.Cluster) (*corev1.Secret, error) {
	name.Name = fmt.Sprintf("%s-etos-suite-runner-cfg", cluster.ObjectMeta.Name)
	target, err := r.mergedSecret(ctx, name, cluster)
	if err != nil {
		return nil, err
	}
	if err := ctrl.SetControllerReference(cluster, target, r.Scheme); err != nil {
		return target, err
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, name, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return secret, err
		}
		logger.Info("Creating a new secret for the suite runner")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileConfigmap will reconcile the ETOS suite starter config to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileConfig(ctx context.Context, logger logr.Logger, secretName string, name types.NamespacedName, cluster *etosv1alpha1.Cluster) (*corev1.Secret, error) {
	name = types.NamespacedName{Name: fmt.Sprintf("%s-cfg", name.Name), Namespace: name.Namespace}
	target, err := r.config(ctx, name, secretName, cluster)
	if err != nil {
		return nil, err
	}
	if err := ctrl.SetControllerReference(cluster, target, r.Scheme); err != nil {
		return target, err
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, name, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return secret, err
		}
		logger.Info("Creating a new config for the suite starter")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileTemplate will reconcile the ETOS SuiteRunner template to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileTemplate(ctx context.Context, logger logr.Logger, saName types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	name := types.NamespacedName{Name: fmt.Sprintf("%s-template", saName.Name), Namespace: saName.Namespace}
	target := r.suiteRunnerTemplate(name, saName)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	secret := &corev1.Secret{}
	if err := r.Get(ctx, name, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return secret, err
		}
		logger.Info("Creating a new suite runner template for the suite starter")
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

// reconcileDeployment will reconcile the ETOS SuiteStarter deployment to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileDeployment(ctx context.Context, logger logr.Logger, secretName string, suiteRunnerTemplate string, name types.NamespacedName, owner metav1.Object) (*appsv1.Deployment, error) {
	target := r.deployment(name, secretName, suiteRunnerTemplate)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, name, deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			return deployment, err
		}
		logger.Info("Creating a new deployment for the suite starter")
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

// reconcileSecret will reconcile the ETOS SuiteStarter service account secret to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileSecret(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	tokenName := types.NamespacedName{Name: fmt.Sprintf("%s-token", name.Name), Namespace: name.Namespace}
	target := r.secret(tokenName, name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	secret := &corev1.Secret{}
	if err := r.Get(ctx, tokenName, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return secret, err
		}
		logger.Info("Creating a new secret for the suite starter service account")
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

// reconcileRole will reconcile the ETOS SuiteStarter service account role to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileRole(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*rbacv1.Role, error) {
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
		logger.Info("Creating a new role for the suite starter")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(role))
}

// reconcileServiceAccount will reconcile the ETOS SuiteStarter service account to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileServiceAccount(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
	target := r.serviceaccount(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	serviceaccount := &corev1.ServiceAccount{}
	if err := r.Get(ctx, name, serviceaccount); err != nil {
		if !apierrors.IsNotFound(err) {
			return serviceaccount, err
		}
		logger.Info("Creating a new service account for the suite starter")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(serviceaccount))
}

// reconcileRolebinding will reconcile the ETOS SuiteStarter service account role binding to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileRolebinding(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*rbacv1.RoleBinding, error) {
	target := r.rolebinding(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	rolebinding := &rbacv1.RoleBinding{}
	if err := r.Get(ctx, name, rolebinding); err != nil {
		if !apierrors.IsNotFound(err) {
			return rolebinding, err
		}
		logger.Info("Creating a new role binding for the suite starter")
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(rolebinding))
}

// config creates a secret resource definition for the ETOS suite runner.
func (r *ETOSSuiteStarterDeployment) config(ctx context.Context, name types.NamespacedName, secretName string, cluster *etosv1alpha1.Cluster) (*corev1.Secret, error) {
	routingKey := fmt.Sprintf("eiffel.*.EiffelTestExecutionRecipeCollectionCreatedEvent.%s.*", cluster.Spec.ETOS.Config.RoutingKeyTag)
	data := map[string][]byte{
		"SUITE_RUNNER":                  []byte(cluster.Spec.ETOS.SuiteRunner.Image.Image),
		"LOG_LISTENER":                  []byte(cluster.Spec.ETOS.SuiteRunner.LogListener.Image.Image),
		"ETOS_CONFIGMAP":                []byte(r.etosConfig.ObjectMeta.Name),
		"ETOS_RABBITMQ_SECRET":          []byte(secretName),
		"ETOS_ESR_TTL":                  []byte(r.Config.TTL),
		"ETOS_TERMINATION_GRACE_PERIOD": []byte(r.Config.GracePeriod),
		"RABBITMQ_ROUTING_KEY":          []byte(routingKey),
		"RABBITMQ_QUEUE":                []byte(r.EiffelQueueName),
	}
	if r.Config.ObservabilityConfigmapName != "" {
		data["ETOS_OBSERVABILITY_CONFIGMAP"] = []byte(r.Config.ObservabilityConfigmapName)
	}
	if r.Config.SidecarImage != "" {
		data["ETOS_SIDECAR_IMAGE"] = []byte(r.Config.SidecarImage)
	}
	if r.Config.OTELCollectorHost != "" {
		data["OTEL_COLLECTOR_HOST"] = []byte(r.Config.OTELCollectorHost)
	}
	if r.EiffelQueueParams != "" {
		data["RABBITMQ_QUEUE_PARAMS"] = []byte(r.EiffelQueueParams)
	}
	eiffel := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.rabbitmqSecret, Namespace: name.Namespace}, eiffel); err != nil {
		return nil, err
	}
	etos := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.messagebusSecret, Namespace: name.Namespace}, etos); err != nil {
		return nil, err
	}
	maps.Copy(data, eiffel.Data)
	maps.Copy(data, etos.Data)
	maps.Copy(data, r.etosConfig.Data)
	return &corev1.Secret{
		ObjectMeta: r.meta(name),
		Data:       data,
	}, nil
}

// secret creates a secret resource definition for the ETOS SuiteStarter.
func (r *ETOSSuiteStarterDeployment) secret(name, serviceAccountName types.NamespacedName) *corev1.Secret {
	meta := r.meta(name)
	meta.Annotations["kubernetes.io/service-account.name"] = serviceAccountName.Name
	return &corev1.Secret{
		ObjectMeta: meta,
		Type:       corev1.SecretTypeServiceAccountToken,
	}
}

// mergedSecret creates a secret that is the merged values of Eiffel, ETOS and encryption key secrets.
// This is for use in the suite runner which only has a single secret as input.
func (r *ETOSSuiteStarterDeployment) mergedSecret(ctx context.Context, name types.NamespacedName, cluster *etosv1alpha1.Cluster) (*corev1.Secret, error) {
	eiffel := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.rabbitmqSecret, Namespace: name.Namespace}, eiffel); err != nil {
		return nil, err
	}
	etos := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.messagebusSecret, Namespace: name.Namespace}, etos); err != nil {
		return nil, err
	}
	data := map[string][]byte{}
	maps.Copy(data, eiffel.Data)
	maps.Copy(data, etos.Data)
	maps.Copy(data, r.etosConfig.Data)
	maps.Copy(data, r.encryptionSecret.Data)
	// Used by the LogListener and the CreateQueue initContainer.
	data["ETOS_RABBITMQ_QUEUE_NAME"] = []byte(cluster.Spec.ETOS.SuiteRunner.LogListener.ETOSQueueName)
	if cluster.Spec.ETOS.SuiteRunner.LogListener.ETOSQueueParams != "" {
		data["ETOS_RABBITMQ_QUEUE_PARAMS"] = []byte(cluster.Spec.ETOS.SuiteRunner.LogListener.ETOSQueueParams)
	}
	return &corev1.Secret{
		ObjectMeta: r.meta(name),
		Data:       data,
	}, nil
}

// role creates a role resource definition for the ETOS SuiteStarter.
func (r *ETOSSuiteStarterDeployment) role(name types.NamespacedName, labelName string) *rbacv1.Role {
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
					"get", "create", "delete", "list", "watch",
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

// serviceaccount creates a service account resource definition for the ETOS SuiteStarter.
func (r *ETOSSuiteStarterDeployment) serviceaccount(name types.NamespacedName) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: r.meta(name),
	}
}

// rolebinding creates a rolebinding resource definition for the ETOS SuiteStarter.
func (r *ETOSSuiteStarterDeployment) rolebinding(name types.NamespacedName) *rbacv1.RoleBinding {
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

// deployment creates a deployment resource definition for the ETOS SuiteStarter.
func (r *ETOSSuiteStarterDeployment) deployment(name types.NamespacedName, secretName string, suiteRunnerTemplate string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: r.meta(name),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      name.Name,
					"app.kubernetes.io/part-of":   "etos",
					"app.kubernetes.io/component": "suitestarter",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: r.meta(name),
				Spec: corev1.PodSpec{
					ServiceAccountName: name.Name,
					Containers:         []corev1.Container{r.container(name, secretName)},
					Volumes: []corev1.Volume{{
						Name: "suite-runner-template",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: suiteRunnerTemplate,
							},
						},
					}},
				},
			},
		},
	}
}

// container creates the container resource for the ETOS SuiteStarter deployment.
func (r *ETOSSuiteStarterDeployment) container(name types.NamespacedName, secretName string) corev1.Container {
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
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "suite-runner-template",
				MountPath: "/app/suite_runner_template.yaml",
				SubPath:   "suite_runner_template.yaml",
			},
		},
	}
}

// suiteRunnerTemplate creates a secret resource for the ETOS SuiteStarter.
func (r *ETOSSuiteStarterDeployment) suiteRunnerTemplate(templateName types.NamespacedName, saName types.NamespacedName) *corev1.Secret {
	data := map[string][]byte{
		"suite_runner_template.yaml": []byte(fmt.Sprintf(`
      apiVersion: batch/v1
      kind: Job
      metadata:
        name: {job_name}
        labels:
          app: suite-runner
          id: {suite_id}
      spec:
        ttlSecondsAfterFinished: {ttl}
        template:
          metadata:
            name: {job_name}
          spec:
            terminationGracePeriodSeconds: {termination_grace_period}
            volumes:
            - name: kubexit
              emptyDir: {{}}
            - name: graveyard
              emptyDir:
                medium: Memory
            initContainers:
            - name: kubexit
              image: karlkfi/kubexit:latest
              command: ["cp", "/bin/kubexit", "/kubexit/kubexit"]
              volumeMounts:
              - mountPath: /kubexit
                name: kubexit
            - name: create-queue
              image: registry.nordix.org/eiffel/etos-log-listener:4969c9b2
              command: ["python", "-u", "-m", "create_queue"]
              envFrom:
              - secretRef:
                  name: {etos_configmap}
              - secretRef:
                  name: {etos_rabbitmq_secret}
              env:
              - name: TERCC
                value: '{EiffelTestExecutionRecipeCollectionCreatedEvent}'
            serviceAccountName: %s
            containers:
            - name: {job_name}
              image: {docker_image}
              imagePullPolicy: Always
              command: ['/kubexit/kubexit']
              args: ['python', '-u', '-m', 'etos_suite_runner']
              envFrom:
              - secretRef:
                  name: {etos_configmap}
              - secretRef:
                  name: {etos_rabbitmq_secret}
              - configMapRef:
                  name: {etos_observability_configmap}
              env:
              - name: TERCC
                value: '{EiffelTestExecutionRecipeCollectionCreatedEvent}'
              - name: KUBEXIT_NAME
                value: esr
              - name: KUBEXIT_GRAVEYARD
                value: /graveyard
              - name: OTEL_CONTEXT
                value: {otel_context}
              - name: OTEL_COLLECTOR_HOST
                value: {otel_collector_host}
              volumeMounts:
              - name: graveyard
                mountPath: /graveyard
              - name: kubexit
                mountPath: /kubexit
            - name: etos-log-listener
              image: {log_listener}
              imagePullPolicy: Always
              command: ['/kubexit/kubexit']
              args: ['python', '-u', '-m', 'log_listener']
              envFrom:
              - secretRef:
                  name: {etos_configmap}
              - secretRef:
                  name: {etos_rabbitmq_secret}
              env:
              - name: TERCC
                value: '{EiffelTestExecutionRecipeCollectionCreatedEvent}'
              - name: KUBEXIT_NAME
                value: log_listener
              - name: KUBEXIT_GRACE_PERIOD
                value: '400s' # needs to be greater than grace period of the container
              - name: KUBEXIT_GRAVEYARD
                value: /graveyard
              - name: KUBEXIT_DEATH_DEPS
                value: esr
              volumeMounts:
              - name: graveyard
                mountPath: /graveyard
              - name: kubexit
                mountPath: /kubexit
            restartPolicy: Never
        backoffLimit: 0
    `, saName.Name)),
	}
	return &corev1.Secret{
		ObjectMeta: r.meta(templateName),
		Data:       data,
	}
}

// meta creates the common meta resource for the ETOS SuiteStarter deployment.
func (r *ETOSSuiteStarterDeployment) meta(name types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name":      name.Name,
			"app.kubernetes.io/part-of":   "etos",
			"app.kubernetes.io/component": "suitestarter",
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}
