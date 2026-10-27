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
	"testing"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/config"
	"github.com/eiffel-community/etos/internal/readiness"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestSSEConfigCarriesActualClusterIdentity verifies telemetry config distinguishes same-namespace deployments.
func TestSSEConfigCarriesActualClusterIdentity(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, namespace := range []string{"team", "other-team"} {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "bus", Namespace: namespace},
			Data: map[string][]byte{
				"ETOS_RABBITMQ_HOST": []byte("rabbitmq"), "ETOS_RABBITMQ_STREAM_PORT": []byte("5552"),
				"ETOS_RABBITMQ_VHOST": []byte("/"),
			},
		}
		deployment := &ETOSSSEDeployment{
			Client:           fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
			messagebusSecret: "bus",
		}
		for _, cluster := range []string{"blue", "green"} {
			target, err := deployment.config(context.Background(), types.NamespacedName{
				Namespace: namespace, Name: cluster + "-etos-sse-cfg",
			}, &etosv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: cluster, Namespace: namespace}})
			if err != nil {
				t.Fatal(err)
			}
			if string(target.Data["ETOS_CLUSTER"]) != cluster || string(target.Data["ETOS_NAMESPACE"]) != namespace {
				t.Fatalf("SSE config identity = %q/%q, want %s/%s", target.Data["ETOS_NAMESPACE"], target.Data["ETOS_CLUSTER"], namespace, cluster)
			}
		}
	}
}

// TestSSEOpenTelemetryReconciliation verifies OTLP settings follow the Cluster spec and restart SSE when changed.
func TestSSEOpenTelemetryReconciliation(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, rbacv1.AddToScheme, etosv1alpha1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	cluster := &etosv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "blue", Namespace: "team", UID: "cluster-uid"},
	}
	bus := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "bus", Namespace: cluster.Namespace},
		Data: map[string][]byte{
			"ETOS_RABBITMQ_HOST":        []byte("rabbitmq"),
			"ETOS_RABBITMQ_STREAM_PORT": []byte("5552"),
			"ETOS_RABBITMQ_VHOST":       []byte("/"),
			"ETOS_RABBITMQ_STREAM_NAME": []byte("events"),
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bus).Build()
	name := types.NamespacedName{Name: "blue-etos-sse", Namespace: cluster.Namespace}
	for _, step := range []struct {
		name      string
		enabled   bool
		endpoint  string
		insecure  string
		unchanged bool
	}{
		{name: "initially disabled", endpoint: "http://collector:4317", insecure: "true"},
		{name: "enabled", enabled: true, endpoint: "http://collector:4317", insecure: "true"},
		{name: "updated", enabled: true, endpoint: "https://other-collector:4317", insecure: "false"},
		{name: "disabled", endpoint: "https://other-collector:4317", insecure: "false"},
		{name: "still disabled", unchanged: true},
	} {
		t.Run(step.name, func(t *testing.T) {
			cluster.Spec.OpenTelemetry = etosv1alpha1.OpenTelemetry{
				Enabled: step.enabled, Endpoint: step.endpoint, Insecure: step.insecure,
			}
			reconciler := NewETOSSSEDeployment(cluster.Spec.ETOS.SSE, scheme, cli, bus.Name, config.New())
			if err := reconciler.Reconcile(t.Context(), cluster); err != nil && !readiness.IsNotReadyError(err) {
				t.Fatal(err)
			}
			if reconciler.restartRequired == step.unchanged {
				t.Fatalf("restartRequired = %v, want %v", reconciler.restartRequired, !step.unchanged)
			}
			secret := &corev1.Secret{}
			if err := cli.Get(t.Context(), types.NamespacedName{
				Name: name.Name + "-cfg", Namespace: name.Namespace,
			}, secret); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": step.endpoint,
				"OTEL_EXPORTER_OTLP_INSECURE": step.insecure,
			} {
				got, exists := secret.Data[key]
				if exists != step.enabled || (step.enabled && string(got) != want) {
					t.Fatalf("%s = %q (present %v), want %q (present %v)", key, got, exists, want, step.enabled)
				}
			}
			if string(secret.Data["ETOS_RABBITMQ_URI"]) != "rabbitmq-stream://rabbitmq:5552/" ||
				string(secret.Data["ETOS_CLUSTER"]) != cluster.Name ||
				string(secret.Data["ETOS_NAMESPACE"]) != cluster.Namespace {
				t.Fatal("SSE message bus or cluster identity configuration changed")
			}
			deployment := &appsv1.Deployment{}
			if err := cli.Get(t.Context(), name, deployment); err != nil {
				t.Fatal(err)
			}
			container := deployment.Spec.Template.Spec.Containers[0]
			if len(container.EnvFrom) != 1 || container.EnvFrom[0].SecretRef == nil ||
				container.EnvFrom[0].SecretRef.Name != secret.Name {
				t.Fatal("SSE container does not consume its reconciled configuration")
			}
			if step.name != "initially disabled" &&
				deployment.Spec.Template.Annotations["etos.eiffel-community.github.io/restartedAt"] == "" {
				t.Fatal("SSE deployment was not marked for restart after configuration changed")
			}
			reconciler = NewETOSSSEDeployment(cluster.Spec.ETOS.SSE, scheme, cli, bus.Name, config.New())
			if err := reconciler.Reconcile(t.Context(), cluster); err != nil && !readiness.IsNotReadyError(err) {
				t.Fatal(err)
			}
			if reconciler.restartRequired {
				t.Fatal("unchanged SSE configuration requested another restart")
			}
		})
	}
}
