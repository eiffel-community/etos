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
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

// TestProviderSetsStableTelemetryIdentity verifies providers use stable telemetry identity values.
func TestProviderSetsStableTelemetryIdentity(t *testing.T) {
	tests := []struct {
		providerType string
		serviceName  string
	}{
		{providerType: "iut", serviceName: "etos-iut-provider"},
		{providerType: "log-area", serviceName: "etos-log-area-provider"},
		{providerType: "execution-space", serviceName: "etos-execution-space-provider"},
	}

	for _, tt := range tests {
		t.Run(tt.providerType, func(t *testing.T) {
			provider := (&ETOSDeployment{}).provider(
				types.NamespacedName{Name: "provider-resource-name", Namespace: "etos"},
				tt.providerType,
				"provider-image",
				"cluster-resource-name",
			)

			env := make(map[string]string)
			for _, variable := range provider.Spec.Env {
				env[variable.Name] = variable.Value
			}
			if got := env["OTEL_SERVICE_NAME"]; got != tt.serviceName {
				t.Errorf("OTEL_SERVICE_NAME = %q, want %q", got, tt.serviceName)
			}
			if got := env["ETOS_CLUSTER_NAME"]; got != "cluster-resource-name" {
				t.Errorf("ETOS_CLUSTER_NAME = %q, want %q", got, "cluster-resource-name")
			}
			if len(provider.Spec.Env) != 2 || provider.Spec.Env[0].Name != "OTEL_SERVICE_NAME" ||
				provider.Spec.Env[1].Name != "ETOS_CLUSTER_NAME" {
				t.Errorf("provider environment = %#v, want only telemetry identity variables", provider.Spec.Env)
			}
		})
	}
}
