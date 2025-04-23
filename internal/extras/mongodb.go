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
package extras

import (
	"context"
	"fmt"
	"net/url"
	"time"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

var mongodbPort int32 = 27017

type MongoDBDeployment struct {
	etosv1alpha1.MongoDB
	client.Client
	Scheme          *runtime.Scheme
	URL             url.URL
	SecretName      string
	restartRequired bool
}

// NewMongoDBDeployment will create a new MongoDB reconciler.
func NewMongoDBDeployment(spec etosv1alpha1.MongoDB, scheme *runtime.Scheme, client client.Client) *MongoDBDeployment {
	return &MongoDBDeployment{spec, client, scheme, url.URL{}, "", false}
}

// Reconcile will reconcile MongoDB to its expected state.
func (r *MongoDBDeployment) Reconcile(ctx context.Context, cluster *etosv1alpha1.Cluster) error {
	name := fmt.Sprintf("%s-mongodb", cluster.Name)
	logger := log.FromContext(ctx, "Reconciler", "MongoDB", "BaseName", name)
	namespacedName := types.NamespacedName{Name: name, Namespace: cluster.Namespace}

	if (url.URL{}) == r.URL {
		uri, err := r.URI.Get(ctx, r.Client, cluster.Namespace)
		if err != nil {
			return err
		}
		mongodbURL, err := url.Parse(string(uri))
		if err != nil {
			return err
		}
		r.URL = *mongodbURL
	}

	if r.Deploy {
		logger.Info("Patching host & port when deploying mongodb", "host", name, "port", mongodbPort)
		r.URL.Host = fmt.Sprintf("%s:%d", name, mongodbPort)
		r.URI.Value = r.URL.String()
	} else {
		logger.Info("Not deploying MongoDB")
	}
	secret, err := r.reconcileSecret(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the MongoDB secret")
		return err
	}
	r.SecretName = secret.Name

	_, err = r.reconcileStatefulset(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the MongoDB statefulset")
		return err
	}
	_, err = r.reconcileService(ctx, logger, namespacedName, cluster)
	if err != nil {
		logger.Error(err, "Failed to reconcile the MongoDB service")
		return err
	}

	return nil
}

// reconcileSecret will reconcile the MongoDB secret to its expected state.
func (r *MongoDBDeployment) reconcileSecret(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Secret, error) {
	target := r.secret(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}
	scheme.Scheme.Default(target)

	secret := &corev1.Secret{}
	if err := r.Get(ctx, name, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "failed to get mongodb secret")
			return secret, err
		}
		if r.Deploy {
			r.restartRequired = true
			logger.Info("Secret not found. Creating")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return target, nil
	} else if !r.Deploy {
		logger.Info("Removing the MongoDB secret")
		return nil, r.Delete(ctx, secret)
	}
	if equality.Semantic.DeepDerivative(target.Data, secret.Data) {
		return secret, nil
	}
	r.restartRequired = true
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(secret))
}

// reconcileStatefulset will reconcile the MongoDB statefulset to its expected state.
func (r *MongoDBDeployment) reconcileStatefulset(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*appsv1.StatefulSet, error) {
	target := r.statefulset(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	mongodb := &appsv1.StatefulSet{}
	if err := r.Get(ctx, name, mongodb); err != nil {
		if !apierrors.IsNotFound(err) {
			return mongodb, err
		}
		if r.Deploy {
			logger.Info("Deploying a new MongoDB statefulset")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return mongodb, nil
	} else if !r.Deploy {
		logger.Info("Removing the MongoDB statefulset")
		return nil, r.Delete(ctx, mongodb)
	} else if r.restartRequired {
		logger.Info("Configuration(s) have changed, restarting statefulset")
		if mongodb.Spec.Template.Annotations == nil {
			mongodb.Spec.Template.Annotations = make(map[string]string)
		}
		mongodb.Spec.Template.Annotations["etos.eiffel-community.github.io/restartedAt"] = time.Now().Format(time.RFC3339)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(mongodb))
}

// reconcileService will reconcile the MongoDB service to its expected state.
func (r *MongoDBDeployment) reconcileService(ctx context.Context, logger logr.Logger, name types.NamespacedName, owner metav1.Object) (*corev1.Service, error) {
	target := r.service(name)
	if err := ctrl.SetControllerReference(owner, target, r.Scheme); err != nil {
		return target, err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return service, err
		}
		if r.Deploy {
			logger.Info("Creating a new MongoDB kubernetes service")
			if err := r.Create(ctx, target); err != nil {
				return target, err
			}
		}
		return service, nil
	} else if !r.Deploy {
		logger.Info("Removing the kubernetes service for MongoDB")
		return nil, r.Delete(ctx, service)
	}
	return target, r.Patch(ctx, target, client.StrategicMergeFrom(service))
}

// secret will create a secret resource definition for MongoDB.
func (r *MongoDBDeployment) secret(name types.NamespacedName) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: r.meta(name),
		Data:       r.secretData(),
	}
}

// statefulset will create a statefulset resource definition for MongoDB.
func (r *MongoDBDeployment) statefulset(name types.NamespacedName) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: r.meta(name),
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name.Name},
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

// service will create a service resource definition for MongoDB.
func (r *MongoDBDeployment) service(name types.NamespacedName) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: r.meta(name),
		Spec: corev1.ServiceSpec{
			Ports:    r.ports(),
			Selector: map[string]string{"app.kubernetes.io/name": name.Name},
		},
	}
}

// secretData will create a map of secret data for the MongoDB secret.
func (r *MongoDBDeployment) secretData() map[string][]byte {
	return map[string][]byte{
		"MONGODB_CONNSTRING": []byte(r.URL.String()),
		"MONGODB_DATABASE":   []byte(r.URL.Path[1:]), // Path always start with '/'
	}
}

// meta will create a common meta object for MongoDB.
func (r *MongoDBDeployment) meta(name types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels: map[string]string{
			"app.kubernetes.io/name": name.Name,
		},
		Annotations: make(map[string]string),
		Name:        name.Name,
		Namespace:   name.Namespace,
	}
}

// volumeClaim will create a volume claim resource definition for the MongoDB statefulset.
func (r *MongoDBDeployment) volumeClaim(name types.NamespacedName) corev1.PersistentVolumeClaim {
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

// volume will create a volume resource definition for the MongoDB statefulset.
func (r *MongoDBDeployment) volume(name types.NamespacedName) corev1.Volume {
	return corev1.Volume{
		Name: fmt.Sprintf("%s-data", name.Name),
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: fmt.Sprintf("%s-data", name.Name),
			},
		},
	}
}

// container will create a container resource definition for the MongoDB statefulset.
func (r *MongoDBDeployment) container(name types.NamespacedName) corev1.Container {
	password, _ := r.URL.User.Password()
	return corev1.Container{
		Name:  name.Name,
		Image: "mongo:latest",
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      fmt.Sprintf("%s-data", name.Name),
				MountPath: "/data/db",
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "mongo",
				ContainerPort: mongodbPort,
				Protocol:      "TCP",
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "MONGO_INITDB_DATABASE",
				Value: r.URL.Path[1:], // Path always start with '/'
			},
			{
				Name:  "MONGO_INITDB_ROOT_USERNAME",
				Value: r.URL.User.Username(),
			},
			{
				Name:  "MONGO_INITDB_ROOT_PASSWORD",
				Value: password,
			},
		},
	}
}

// ports will create a service port resource definition for the MongoDB service.
func (r *MongoDBDeployment) ports() []corev1.ServicePort {
	return []corev1.ServicePort{
		{Port: mongodbPort, Name: "mongo", Protocol: "TCP"},
	}
}
