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
	"strings"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	etcdClientPort int32 = 2379
	etcdServerPort int32 = 2380
	etcdMetricPort int32 = 8080
)

type ETCDDeployment struct {
	*etosv1alpha1.Database
	client.Client
	Scheme *runtime.Scheme
}

// NewETCDDeployment will create a new ETCD reconciler.
func NewETCDDeployment(spec *etosv1alpha1.Database, scheme *runtime.Scheme, client client.Client) *ETCDDeployment {
	return &ETCDDeployment{spec, client, scheme}
}

// Reconcile will reconcile ETCD to its expected state.
func (r *ETCDDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	logger := log.FromContext(ctx)
	name := fmt.Sprintf("%s-etcd", cluster.Name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}
	if r.Deploy {
		logger.Info("Patching host when deploying etcd", "host", name)
		r.Etcd.Host = name
	}

	_, err := r.reconcileStatefulset(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileService(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}
	_, err = r.reconcileClientService(ctx, namespacedName, cluster)
	if err != nil {
		return err
	}

	return nil
}

// reconcileStatefulset will reconcile the ETCD statefulset to its expected state.
func (r *ETCDDeployment) reconcileStatefulset(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*appsv1.StatefulSet, error) {
	target := r.statefulset(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	etcd := &appsv1.StatefulSet{}
	if err := r.Get(ctx, name, etcd); err != nil {
		if !apierrors.IsNotFound(err) {
			return etcd, err
		}
		if r.Deploy {
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		return nil, r.Delete(ctx, etcd)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(etcd))
}

// reconcileService will reconcile the ETCD service to its expected state.
func (r *ETCDDeployment) reconcileService(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	target := r.headlessService(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		if r.Deploy {
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		return nil, r.Delete(ctx, service)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// reconcileClientService will reconcile the ETCD client service to its expected state.
func (r *ETCDDeployment) reconcileClientService(ctx context.Context, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	labelName := name.Name
	name.Name = fmt.Sprintf("%s-client", name.Name)
	target := r.service(name, labelName)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		if r.Deploy {
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		return nil, r.Delete(ctx, service)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// statefulset creates a statefulset resource definition for ETCD.
func (r *ETCDDeployment) statefulset(name types.NamespacedName) *appsv1.StatefulSet {

	return &appsv1.StatefulSet{
		ObjectMeta: r.meta(name),
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":    name.Name,
					"app.kubernetes.io/part-of": "etos",
				},
			},
			ServiceName: name.Name,
			Replicas:    r.Etcd.Replicas,
			// For initialization, the etcd pods must be available to each other before
			// they are "ready" for traffic. The "Parallel" policy makes this possible.
			PodManagementPolicy: appsv1.ParallelPodManagement,
			// Ensure availability of the etcd cluster.
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{r.volumeClaim(name)},
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

// headlessService creates a headless service resource definition for ETCD.
func (r *ETCDDeployment) headlessService(name types.NamespacedName) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name),
		Spec: corev1.ServiceSpec{
			Ports:                    r.ports(),
			ClusterIP:                "None",
			PublishNotReadyAddresses: true,
			Selector: map[string]string{
				"app.kubernetes.io/name":    name.Name,
				"app.kubernetes.io/part-of": "etos",
			},
		},
	}
}

// service creates a service resource definition for ETCD.
func (r *ETCDDeployment) service(name types.NamespacedName, labelName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name),
		Spec: corev1.ServiceSpec{
			Ports: r.clientPorts(),
			Selector: map[string]string{
				"app.kubernetes.io/name":    labelName,
				"app.kubernetes.io/part-of": "etos",
			},
		},
	}
}

// meta creates a common meta resource definition for ETCD.
func (r *ETCDDeployment) meta(name types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name":    name.Name,
			"app.kubernetes.io/part-of": "etos",
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// volumeClaim creates a volume claim resource definition for the ETCD statefulset.
func (r *ETCDDeployment) volumeClaim(name types.NamespacedName) corev1.PersistentVolumeClaim {
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

// volume creates a volume resource definition for the ETCD statefulset.
func (r *ETCDDeployment) volume(name types.NamespacedName) corev1.Volume {
	return corev1.Volume{
		Name: fmt.Sprintf("%s-data", name.Name),
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: fmt.Sprintf("%s-data", name.Name),
			},
		},
	}
}

// container creates a container resource definition for the ETCD statefulset.
func (r *ETCDDeployment) container(name types.NamespacedName) corev1.Container {
	// Build the initial cluster configuration dynamically based on replicas
	var initialClusterMembers []string
	for i := int32(0); i < *r.Etcd.Replicas; i++ {
		member := fmt.Sprintf("%s-%d=$(URI_SCHEME)://%s-%d.$(SERVICE_NAME):%d", name.Name, i, name.Name, i, etcdServerPort)
		initialClusterMembers = append(initialClusterMembers, member)
	}
	initialCluster := fmt.Sprintf("--initial-cluster=%s", fmt.Sprintf("%s", strings.Join(initialClusterMembers, ",")))

	// Build args slice with dynamic initial cluster
	args := []string{
		"--name=$(HOSTNAME)",
		"--data-dir=/data",
		"--wal-dir=/data/wal",
		fmt.Sprintf("--listen-peer-urls=$(URI_SCHEME)://0.0.0.0:%d", etcdServerPort),
		fmt.Sprintf("--listen-client-urls=$(URI_SCHEME)://0.0.0.0:%d", etcdClientPort),
		fmt.Sprintf("--advertise-client-urls=$(URI_SCHEME)://$(HOSTNAME).$(SERVICE_NAME):%d", etcdClientPort),
		"--initial-cluster-state=new",
		fmt.Sprintf("--initial-cluster-token=%s-$(K8S_NAMESPACE)", name.Name),
		initialCluster,
		fmt.Sprintf("--initial-advertise-peer-urls=$(URI_SCHEME)://$(HOSTNAME).$(SERVICE_NAME):%d", etcdServerPort),
		fmt.Sprintf("--listen-metrics-urls=http://0.0.0.0:%d", etcdMetricPort),
		"--auto-compaction-mode=revision",
		"--auto-compaction-retention=1",
	}

	return corev1.Container{
		Name:  "etcd",
		Image: r.Etcd.Image,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse(r.Etcd.LimitsMemory),
				corev1.ResourceCPU:    resource.MustParse(r.Etcd.LimitsCPU),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse(r.Etcd.RequestsMemory),
				corev1.ResourceCPU:    resource.MustParse(r.Etcd.RequestsCPU),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      fmt.Sprintf("%s-data", name.Name),
				MountPath: "/data",
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "etcd-client",
				ContainerPort: etcdClientPort,
			},
			{
				Name:          "etcd-server",
				ContainerPort: etcdServerPort,
			},
			{
				Name:          "etcd-metrics",
				ContainerPort: etcdMetricPort,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name: "K8S_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name: "HOSTNAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name:  "SERVICE_NAME",
				Value: name.Name,
			},
			{
				Name:  "ETCDCTL_ENDPOINTS",
				Value: fmt.Sprintf("$(HOSTNAME).$(SERVICE_NAME):%d", etcdClientPort),
			},
			{
				Name:  "URI_SCHEME",
				Value: "http",
			},
		},
		Command: []string{
			"/usr/local/bin/etcd",
		},
		Args: args,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/livez",
					Port: intstr.FromInt(int(etcdMetricPort)),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromInt(int(etcdMetricPort)),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       5,
			TimeoutSeconds:      5,
			SuccessThreshold:    1,
			FailureThreshold:    30,
		},
	}
}

// ports creates a service port resource definition for the ETCD service.
func (r *ETCDDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: etcdClientPort, Name: "etcd-client"},
		{Port: etcdServerPort, Name: "etcd-server"},
		{Port: etcdMetricPort, Name: "etcd-metrics"},
	}
}

// clientPorts creates a service port resource definition for the ETCD headless service.
func (r *ETCDDeployment) clientPorts() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: etcdClientPort, Name: "etcd-client"},
	}
}
