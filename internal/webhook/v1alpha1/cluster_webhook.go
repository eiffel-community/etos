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

package v1alpha1

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// log is for logging in this package.
var clusterlog = logf.Log.WithName("cluster-resource")

// SetupClusterWebhookWithManager registers the webhook for Cluster in the manager.
func SetupClusterWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &etosv1alpha1.Cluster{}).
		WithDefaulter(&ClusterCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1alpha1-cluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=clusters,verbs=create;update,versions=v1alpha1,name=mcluster-v1alpha1.kb.io,admissionReviewVersions=v1

// ClusterCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Cluster when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ClusterCustomDefaulter struct {
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Cluster.
func (d *ClusterCustomDefaulter) Default(_ context.Context, obj *etosv1alpha1.Cluster) error {
	clusterlog.Info("Defaulting for Cluster", "name", obj.GetName())
	clusterlog.Info("Cluster spec", "spec", obj.Spec)

	if obj.Spec.MessageBus.ETOSMessageBus == nil {
		clusterlog.Info("ETOSMessageBus is not set, setting it to default values")
		obj.Spec.MessageBus.ETOSMessageBus = &etosv1alpha1.RabbitMQ{
			Host: fmt.Sprintf("%s-messagebus.%s.svc.cluster.local", obj.GetName(), obj.GetNamespace()),
		}
	}
	if obj.Spec.MessageBus.EiffelMessageBus == nil {
		clusterlog.Info("EiffelMessageBus is not set, setting it to default values")
		obj.Spec.MessageBus.EiffelMessageBus = &etosv1alpha1.RabbitMQ{
			Host: fmt.Sprintf("%s-rabbitmq.%s.svc.cluster.local", obj.GetName(), obj.GetNamespace()),
		}
	}
	if obj.Spec.Database == nil {
		obj.Spec.Database = &etosv1alpha1.Database{Deploy: true}
	}
	if obj.Spec.Database.Etcd.Host == "" {
		clusterlog.Info("Etcd host is not set, setting it to default values")
		obj.Spec.Database.Etcd.Host = fmt.Sprintf("%s-etcd-client.%s.svc.cluster.local", obj.GetName(), obj.GetNamespace())
	}
	if obj.Spec.Database.Etcd.Port == "" {
		clusterlog.Info("Etcd port is not set, setting it to default values")
		obj.Spec.Database.Etcd.Port = "2379"
	}
	if obj.Spec.EventRepository == nil {
		clusterlog.Info("Event Repository host is not set, setting it to default values")
		obj.Spec.EventRepository = &etosv1alpha1.EventRepository{
			Deploy:          true,
			EiffelQueueName: obj.GetName(),
			Host:            fmt.Sprintf("http://%s-graphql.%s.svc.cluster.local:5000/graphql", obj.GetName(), obj.GetNamespace()),
			Database: etosv1alpha1.MongoDB{
				Deploy: true,
			},
		}
	}

	scheme := "https"
	if !obj.Spec.ETOS.Ingress.SSL {
		scheme = "http"
	}
	if obj.Spec.ETOS.Ingress.Enabled {
		obj.Spec.ETOS.Config.ETOSApiURL = fmt.Sprintf("%s://%s/api", scheme, obj.Spec.ETOS.Ingress.Host)
	} else if obj.Spec.ETOS.Config.ETOSApiURL == "" {
		obj.Spec.ETOS.Config.ETOSApiURL = fmt.Sprintf(
			"http://%s-etos-api.%s.svc.cluster.local/api", obj.GetName(), obj.GetNamespace(),
		)
	}

	return nil
}
