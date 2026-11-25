// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"testing"

	v1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	v1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/fernet/fernet-go"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestEnvironmentVariablesMetricsTemporality verifies execution spaces pass the preference to test runners.
func TestEnvironmentVariablesMetricsTemporality(t *testing.T) {
	previousClient := cli
	SetKubernetesClient(fake.NewClientBuilder().Build())
	t.Cleanup(func() { SetKubernetesClient(previousClient) })
	key := &fernet.Key{}
	if err := key.Generate(); err != nil {
		t.Fatal(err)
	}
	request := &v1alpha1.EnvironmentRequest{}
	request.Spec.Config.EncryptionKey = v1alpha1.Var{Value: key.Encode()}
	password := &v1alpha1.Var{Value: "test-password"}
	request.Spec.Config.EtosMessageBus.Password = password
	request.Spec.Config.EiffelMessageBus.Password = password
	for _, preference := range []string{"", "DELTA", "CUMULATIVE"} {
		t.Run("preference="+preference, func(t *testing.T) {
			const variable = "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE"
			t.Setenv(variable, preference)
			environment, err := environmentVariables(t.Context(), request, v1alpha2.ExecutionSpaceSpec{})
			if err != nil {
				t.Fatal(err)
			}
			got, exists := environment[variable]
			if got != preference || exists != (preference != "") {
				t.Fatalf("temporality = %q (present %v), want %q", got, exists, preference)
			}
		})
	}
}
