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

	corev1 "k8s.io/api/core/v1"
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
			}, cluster)
			if err != nil {
				t.Fatal(err)
			}
			if string(target.Data["ETOS_CLUSTER"]) != cluster || string(target.Data["ETOS_NAMESPACE"]) != namespace {
				t.Fatalf("SSE config identity = %q/%q, want %s/%s", target.Data["ETOS_NAMESPACE"], target.Data["ETOS_CLUSTER"], namespace, cluster)
			}
		}
	}
}
