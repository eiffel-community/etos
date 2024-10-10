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
)

type ETOSSuiteStarterDeployment struct {
	etosv1alpha1.ETOSSuiteStarter
	client.Client
	Scheme           *runtime.Scheme
	rabbitmqSecret   string
	messagebusSecret string
	etosConfigmap    *corev1.ConfigMap
	encryptionSecret *corev1.Secret
}

// NewETOSSuiteStarterDeployment will create a new ETOS SuiteStarter reconciler.
func NewETOSSuiteStarterDeployment(spec etosv1alpha1.ETOSSuiteStarter, scheme *runtime.Scheme, client client.Client, rabbitmqSecret, messagebusSecret string, configmap *corev1.ConfigMap, encryption *corev1.Secret) *ETOSSuiteStarterDeployment {
	return &ETOSSuiteStarterDeployment{spec, client, scheme, rabbitmqSecret, messagebusSecret, configmap, encryption}
}

// Reconcile will reconcile the ETOS suite starter to its expected state.
func (r *ETOSSuiteStarterDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	var err error
	name := fmt.Sprintf("%s-etos-suite-starter", cluster.Name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}
	// There is an assumption in the suite starter that the service account is named 'etos-sa'.
	_, err = r.reconcileSuiteRunnerServiceAccount(ctx, types.NamespacedName{Name: "etos-sa", Namespace: cluster.Namespace}, cluster)
	if err != nil {
		return err
	}
	secret, err := r.reconcileSuiteRunnerSecret(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	configmap, err := r.reconcileConfigmap(ctx, secret.ObjectMeta.Name, namespacedName, cluster)
	if err != nil {
		return err
	}

	template, err := r.reconcileTemplate(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	var suiteRunnerTemplateName string
	if r.SuiteRunnerTemplateConfigmapName == "" {
		suiteRunnerTemplateName = template.ObjectMeta.Name
	} else {
		suiteRunnerTemplateName = r.SuiteRunnerTemplateConfigmapName
	}
	_, err = r.reconcileDeployment(ctx, configmap.ObjectMeta.Name, suiteRunnerTemplateName, namespacedName, cluster)
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
	return err
}

// reconcileSuiteRunnerServiceAccount will reconcile the ETOS SuiteStarter service account to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileSuiteRunnerServiceAccount(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
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

// reconcileSuiteRunnerSecret will reconcile the ETOS suite runner secret to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileSuiteRunnerSecret(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	name.Name = fmt.Sprintf("%s-suite-runner", name.Name)
	target, err := r.mergedSecret(ctx, name)
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
		if err := r.Create(ctx, target); err != nil {
			return target, err
		}
		return target, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileConfigmap will reconcile the ETOS suite starter config to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileConfigmap(ctx context.Context, secretName string, name types.NamespacedName, cluster *etosv1alpha1.Cluster) (*corev1.ConfigMap, error) {
	target := r.configmap(name, secretName, cluster)
	if err := ctrl.SetControllerReference(cluster, target, r.Scheme); err != nil {
		return target, err
	}

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
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(configmap))
}

// reconcileTemplate will reconcile the ETOS SuiteRunner template to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileTemplate(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.ConfigMap, error) {
	name.Name = fmt.Sprintf("%s-template", name.Name)
	target := r.suiteRunnerTemplate(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
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
	if equality.Semantic.DeepDerivative(target.Data, configmap.Data) {
		return configmap, nil
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(configmap))
}

// reconcileDeployment will reconcile the ETOS SuiteStarter deployment to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileDeployment(ctx context.Context, configmap string, suiteRunnerTemplate string, name types.NamespacedName, owner metav1.Object) (*appsv1.Deployment, error) {
	target := r.deployment(name, configmap, suiteRunnerTemplate)
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

// reconcileSecret will reconcile the ETOS SuiteStarter service account secret to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileSecret(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
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

// reconcileRole will reconcile the ETOS SuiteStarter service account role to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileRole(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*rbacv1.Role, error) {
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

// reconcileServiceAccount will reconcile the ETOS SuiteStarter service account to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileServiceAccount(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.ServiceAccount, error) {
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

// reconcileRolebinding will reconcile the ETOS SuiteStarter service account role binding to its expected state.
func (r *ETOSSuiteStarterDeployment) reconcileRolebinding(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*rbacv1.RoleBinding, error) {
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

// configmap creates a configmap resource definition for the ETOS suite runner.
func (r *ETOSSuiteStarterDeployment) configmap(name types.NamespacedName, secretName string, cluster *etosv1alpha1.Cluster) *corev1.ConfigMap {
	data := map[string]string{
		"SUITE_RUNNER":                  cluster.Spec.ETOS.SuiteRunner.Image.Image,
		"LOG_LISTENER":                  cluster.Spec.ETOS.SuiteRunner.LogListener.Image.Image,
		"ETOS_CONFIGMAP":                r.etosConfigmap.ObjectMeta.Name,
		"ETOS_RABBITMQ_SECRET":          secretName,
		"ETOS_ESR_TTL":                  r.Config.TTL,
		"ETOS_TERMINATION_GRACE_PERIOD": r.Config.GracePeriod,
	}
	if r.Config.ObservabilityConfigmapName != "" {
		data["ETOS_OBSERVABILITY_CONFIGMAP"] = r.Config.ObservabilityConfigmapName
	}
	if r.Config.SidecarImage != "" {
		data["ETOS_SIDECAR_IMAGE"] = r.Config.SidecarImage
	}
	if r.Config.OTELCollectorHost != "" {
		data["OTEL_COLLECTOR_HOST"] = r.Config.OTELCollectorHost
	}
	maps.Copy(data, r.etosConfigmap.Data)
	return &corev1.ConfigMap{
		ObjectMeta: r.meta(name),
		Data:       data,
	}
}

// secret creates a secret resource definition for the ETOS SuiteStarter.
func (r *ETOSSuiteStarterDeployment) secret(name types.NamespacedName) *corev1.Secret {
	meta := r.meta(name)
	meta.Annotations["kubernetes.io/service-account.name"] = name.Name
	name.Name = fmt.Sprintf("%s-token", name.Name)
	return &corev1.Secret{
		ObjectMeta: meta,
		Type:       corev1.SecretTypeServiceAccountToken,
	}
}

// mergedSecret creates a secret that is the merged values of Eiffel, ETOS and encryption key secrets.
// This is for use in the suite runner which only has a single secret as input.
func (r *ETOSSuiteStarterDeployment) mergedSecret(ctx context.Context, name types.NamespacedName) (*corev1.Secret, error) {
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
	maps.Copy(data, r.encryptionSecret.Data)
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
func (r *ETOSSuiteStarterDeployment) deployment(name types.NamespacedName, configmap string, suiteRunnerTemplate string) *appsv1.Deployment {
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
					Containers:         []corev1.Container{r.container(name, configmap)},
					Volumes: []corev1.Volume{{
						Name: "suite-runner-template",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: suiteRunnerTemplate,
								},
							},
						},
					}},
				},
			},
		},
	}
}

// container creates the container resource for the ETOS SuiteStarter deployment.
func (r *ETOSSuiteStarterDeployment) container(name types.NamespacedName, configmap string) corev1.Container {
	env := []corev1.EnvVar{
		{Name: "RABBITMQ_QUEUE", Value: r.EiffelQueueName},
	}
	if r.EiffelQueueParams != "" {
		env = append(env, corev1.EnvVar{Name: "RABBITMQ_QUEUE_PARAMS", Value: r.EiffelQueueParams})
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
		EnvFrom: r.environment(configmap),
		Env:     env,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "suite-runner-template",
				MountPath: "/app/suite_runner_template.yaml",
				SubPath:   "suite_runner_template.yaml",
			},
		},
	}
}

// suiteRunnerTemplate creates a configmap resource for the ETOS SuiteStarter.
func (r *ETOSSuiteStarterDeployment) suiteRunnerTemplate(name types.NamespacedName) *corev1.ConfigMap {
	data := map[string]string{
		"suite_runner_template.yaml": `
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
              - configMapRef:
                  name: {etos_configmap}
              - secretRef:
                  name: {etos_rabbitmq_secret}
              env:
              - name: TERCC
                value: '{EiffelTestExecutionRecipeCollectionCreatedEvent}'
            serviceAccountName: etos-sa
            containers:
            - name: {job_name}
              image: {docker_image}
              imagePullPolicy: Always
              command: ['/kubexit/kubexit']
              args: ['python', '-u', '-m', 'etos_suite_runner']
              envFrom:
              - configMapRef:
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
              - configMapRef:
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
    `,
	}
	return &corev1.ConfigMap{
		ObjectMeta: r.meta(name),
		Data:       data,
	}
}

// environment creates the environment resource for the ETOS SuiteStarter deployment.
func (r *ETOSSuiteStarterDeployment) environment(configmap string) []corev1.EnvFromSource {
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
					Name: configmap,
				},
			},
		},
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
