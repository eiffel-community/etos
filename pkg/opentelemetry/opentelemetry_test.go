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

package opentelemetry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// metricsCollector records whether resource metrics were exported.
type metricsCollector struct {
	metricsv1.UnimplementedMetricsServiceServer
	runtimeExported atomic.Bool
	customExported  atomic.Bool
}

// Export records whether the collector received both runtime and custom metrics.
func (c *metricsCollector) Export(
	_ context.Context,
	request *metricsv1.ExportMetricsServiceRequest,
) (*metricsv1.ExportMetricsServiceResponse, error) {
	for _, resource := range request.ResourceMetrics {
		for _, scope := range resource.ScopeMetrics {
			if scope.Scope.GetName() == "go.opentelemetry.io/contrib/instrumentation/runtime" && len(scope.Metrics) > 0 {
				c.runtimeExported.Store(true)
			}
			for _, instrument := range scope.Metrics {
				if instrument.Name == "test_provider_counter" && len(instrument.GetSum().GetDataPoints()) > 0 {
					c.customExported.Store(true)
				}
			}
		}
	}
	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

// TestRuntimeMetricsAreFlushedOnShutdown verifies shutdown flushes runtime and custom metrics from the shared provider.
func TestRuntimeMetricsAreFlushedOnShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	collector := &metricsCollector{}
	server := grpc.NewServer()
	metricsv1.RegisterMetricsServiceServer(server, collector)
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", fmt.Sprintf("http://%s", listener.Addr()))
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "3600000")
	tracer := New("provider-test", "etos")
	if err := tracer.StartWithRuntimeMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tracer.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	counter, err := otel.Meter("provider-test").Int64Counter("test_provider_counter")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(t.Context(), 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tracer.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if !collector.runtimeExported.Load() || !collector.customExported.Load() {
		t.Fatal("runtime and custom metrics were not both exported during shutdown")
	}
}

// TestDisabledTelemetryPreservesPropagation verifies that disabling exporters preserves trace context and baggage.
func TestDisabledTelemetryPreservesPropagation(t *testing.T) {
	old, present := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if err := os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if present {
			if err := os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", old); err != nil {
				t.Error(err)
			}
		}
	})
	lifecycle := New("disabled", "etos")
	if err := lifecycle.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	request := &v1alpha1.EnvironmentRequest{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		"etos.eiffel-community.github.io/traceparent": "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01",
		"etos.eiffel-community.github.io/tracestate":  "vendor=value",
		"etos.eiffel-community.github.io/baggage":     "etos_cluster=namespace/cluster,testrun_id=test,artifact_id=artifact",
	}}}
	ctx := lifecycle.ContextFromEnvironmentRequest(t.Context(), request)
	cluster := baggage.FromContext(ctx).Member("etos_cluster").Value()
	if !trace.SpanContextFromContext(ctx).IsValid() || cluster != "namespace/cluster" {
		t.Fatal("disabled export must not disable propagation")
	}
	if lifecycle.MeterProvider != nil || lifecycle.LoggerProvider != nil || lifecycle.tracerProvider != nil {
		t.Fatal("disabled telemetry allocated exporters")
	}
	if err := lifecycle.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// TestConfigurationRejectsInvalidValues verifies that invalid settings fail initialization
// without replacing global metrics.
func TestConfigurationRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{"OTEL_EXPORTER_OTLP_ENDPOINT", ""}, {"OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"},
		{"OTEL_EXPORTER_OTLP_ENDPOINT", "http://"}, {"OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf"},
		{"OTEL_EXPORTER_OTLP_LOGS_PROTOCOL", "http/json"},
		{"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "invalid"},
		{"OTEL_METRIC_EXPORT_INTERVAL", "0"}, {"OTEL_METRIC_EXPORT_TIMEOUT", "-1"},
		{"OTEL_METRIC_EXPORT_INTERVAL", "nan"},
		{"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE", "delta"},
	} {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4317")
			t.Setenv(test.key, test.value)
			lifecycle := New("invalid", "etos")
			previous := otel.GetMeterProvider()
			if err := lifecycle.Start(t.Context()); err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if previous != otel.GetMeterProvider() {
				t.Fatal("failed initialization installed global state")
			}
			if err := lifecycle.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestMetricIntervalPrecedence verifies that explicit metric interval and timeout settings override defaults.
func TestMetricIntervalPrecedence(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4317")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "12345")
	t.Setenv("OTEL_METRIC_EXPORT_TIMEOUT", "1234")
	interval, timeout, err := validateConfiguration()
	if err != nil || interval != 12345*time.Millisecond || timeout != 1234*time.Millisecond {
		t.Fatalf("validated overrides = %v %v %v", interval, timeout, err)
	}
}

// TestInvalidTLSFailsClosed verifies that invalid client credentials reject startup
// without installing partial providers.
func TestInvalidTLSFailsClosed(t *testing.T) {
	for _, signal := range []string{"TRACES", "LOGS", "METRICS"} {
		t.Run(signal, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://127.0.0.1:4317")
			t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_CLIENT_CERTIFICATE", "/missing/otel-certificate.pem")
			t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_CLIENT_KEY", "/missing/otel-key.pem")
			lifecycle := New("rollback", "etos")
			previousTrace, previousMeter := otel.GetTracerProvider(), otel.GetMeterProvider()
			if err := lifecycle.Start(t.Context()); err == nil {
				if shutdownErr := lifecycle.Shutdown(t.Context()); shutdownErr != nil {
					t.Error(shutdownErr)
				}

				t.Fatal("invalid client certificate must reject initialization")
			}
			if previousTrace != otel.GetTracerProvider() || previousMeter != otel.GetMeterProvider() {
				t.Fatal("partially initialized providers installed globally")
			}
			if err := lifecycle.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestPartialInitializationRollsBack verifies exporter cleanup and unchanged globals after each initialization failure.
func TestPartialInitializationRollsBack(t *testing.T) {
	for _, signal := range []string{"traces", "logs", "metrics"} {
		t.Run(signal, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4317")
			lifecycle := New("rollback", "etos")
			traceExporter, logExporter := &shutdownTraceExporter{}, &shutdownLogExporter{}
			lifecycle.exporters.traces = func(context.Context) (sdktrace.SpanExporter, error) { return traceExporter, nil }
			lifecycle.exporters.logs = func(context.Context) (sdklog.Exporter, error) { return logExporter, nil }
			injected := errors.New("injected constructor failure")
			switch signal {
			case "traces":
				lifecycle.exporters.traces = func(context.Context) (sdktrace.SpanExporter, error) { return nil, injected }
			case "logs":
				lifecycle.exporters.logs = func(context.Context) (sdklog.Exporter, error) { return nil, injected }
			case "metrics":
				lifecycle.exporters.metrics = func(context.Context) (sdkmetric.Exporter, error) { return nil, injected }
			}
			previousTrace, previousMeter := otel.GetTracerProvider(), otel.GetMeterProvider()
			if err := lifecycle.Start(t.Context()); !errors.Is(err, injected) {
				t.Fatalf("missing initialization error: %v", err)
			}
			if previousTrace != otel.GetTracerProvider() || previousMeter != otel.GetMeterProvider() {
				t.Fatal("partial initialization changed globals")
			}
			if err := lifecycle.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
			if signal != "traces" && traceExporter.shutdown.Load() != 1 {
				t.Fatal("partial initialization leaked trace exporter")
			}
			if signal == "metrics" && logExporter.shutdown.Load() != 1 {
				t.Fatal("partial initialization leaked log exporter")
			}
		})
	}
}
