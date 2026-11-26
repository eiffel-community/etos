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

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	healthmetrics "github.com/eiffel-community/etos/internal/metrics"
	"github.com/eiffel-community/etos/pkg/opentelemetry"
	"github.com/prometheus/client_golang/prometheus"
	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// TestManagerStartupFailureRunsCleanup verifies that manager startup failure still runs exporter cleanup.
func TestManagerStartupFailureRunsCleanup(t *testing.T) {
	if os.Getenv("ETOS_TEST_STARTUP_FAILURE") == "1" {
		err := run()
		if err == nil {
			t.Fatal("missing kubeconfig unexpectedly succeeded")
		}
		fmt.Fprintln(os.Stderr, "Expected startup failure:", err)
		return
	}
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestManagerStartupFailureRunsCleanup$")
	command.Env = append(os.Environ(),
		"ETOS_TEST_STARTUP_FAILURE=1", "KUBECONFIG=/missing/etos-test-kubeconfig",
		"KUBERNETES_SERVICE_HOST=", "KUBERNETES_SERVICE_PORT=",
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:1", "OTEL_EXPORTER_OTLP_INSECURE=true",
		"OTEL_EXPORTER_OTLP_TIMEOUT=50", "OTEL_METRIC_EXPORT_TIMEOUT=50",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("startup fixture failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Expected startup failure:") ||
		!strings.Contains(string(output), "Failed to shutdown OpenTelemetry") {
		t.Fatalf("startup failure bypassed final exporter cleanup: %s", output)
	}
}

// TestFrameworkBridgeNativeSeparationAndTypes verifies framework metric parity and aggregation types
// while ensuring custom ETOS metrics bypass the Prometheus bridge.
func TestFrameworkBridgeNativeSeparationAndTypes(t *testing.T) {
	previous := otel.GetErrorHandler()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { t.Errorf("bridge dropped metric: %v", err) }))
	t.Cleanup(func() { otel.SetErrorHandler(previous) })
	// Match controller-runtime's native histogram mode and test a Go-style summary.
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "framework_test_native_seconds", Help: "Representative framework native histogram",
		NativeHistogramBucketFactor: 1.1,
	})
	summary := prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "framework_test_summary", Help: "Representative framework summary",
	})
	for _, collector := range []prometheus.Collector{histogram, summary} {
		if err := ctrlmetrics.Registry.Register(collector); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ctrlmetrics.Registry.Unregister(collector) })
	}
	histogram.Observe(0.25)
	summary.Observe(0.5)
	families, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	reader := sdkmetric.NewManualReader(sdkmetric.WithProducer(
		prombridge.NewMetricProducer(prombridge.WithGatherer(ctrlmetrics.Registry))))
	options := append(opentelemetry.MetricProviderOptions(), sdkmetric.WithReader(reader))
	provider := sdkmetric.NewMeterProvider(options...)
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if _, err := healthmetrics.NewTransitions(provider.Meter("etos/controller")); err != nil {
		t.Fatal(err)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	bridged := make(map[string]metricdata.Metrics)
	native := 0
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch scope.Scope.Name {
			case "go.opentelemetry.io/contrib/bridges/prometheus":
				if strings.HasPrefix(metric.Name, "etos_") {
					t.Fatalf("custom metric in bridge scope: %s", metric.Name)
				}
				bridged[metric.Name] = metric
			case "etos/controller":
				native++
			}
		}
	}
	if native != 2 || len(bridged) != len(families) {
		t.Fatalf("family parity native=%d bridged=%d gathered=%d", native, len(bridged), len(families))
	}
	for _, family := range families {
		if _, ok := bridged[family.GetName()]; !ok {
			t.Errorf("framework family omitted: %s", family.GetName())
		}
	}
	exponential, ok := bridged["framework_test_native_seconds"].Data.(metricdata.ExponentialHistogram[float64])
	if !ok || exponential.DataPoints[0].Count != 1 || exponential.DataPoints[0].Sum != 0.25 {
		t.Fatal("native framework histogram conversion changed")
	}
	converted, ok := bridged["framework_test_summary"].Data.(metricdata.Summary)
	if !ok || converted.DataPoints[0].Count != 1 || converted.DataPoints[0].Sum != 0.5 {
		t.Fatal("framework summary conversion changed")
	}
}
