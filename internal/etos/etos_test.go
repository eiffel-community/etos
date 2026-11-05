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

package etos

import (
	"testing"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/config"
	"k8s.io/apimachinery/pkg/types"
)

// TestConfigMetricsTemporality verifies shared service configuration preserves optional metric preferences.
func TestConfigMetricsTemporality(t *testing.T) {
	deployment := &ETOSDeployment{cfg: config.New()}
	for _, step := range []struct {
		name       string
		enabled    bool
		preference string
	}{
		{name: "disabled", preference: "DELTA"},
		{name: "unset", enabled: true},
		{name: "delta", enabled: true, preference: "DELTA"},
		{name: "cumulative", enabled: true, preference: "CUMULATIVE"},
	} {
		t.Run(step.name, func(t *testing.T) {
			cluster := &etosv1alpha1.Cluster{}
			cluster.Spec.EventRepository = &etosv1alpha1.EventRepository{}
			cluster.Spec.Database = &etosv1alpha1.Database{}
			cluster.Spec.OpenTelemetry = etosv1alpha1.OpenTelemetry{
				Enabled: step.enabled, MetricsTemporalityPreference: step.preference,
			}
			secret := deployment.config(types.NamespacedName{Name: "etos", Namespace: "team"}, cluster)
			got, exists := secret.Data["OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE"]
			want := step.enabled && step.preference != ""
			if exists != want || (want && string(got) != step.preference) {
				t.Fatalf("temporality = %q (present %v), want %q (present %v)", got, exists, step.preference, want)
			}
		})
	}
}
