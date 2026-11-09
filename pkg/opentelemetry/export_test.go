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

package opentelemetry_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eiffel-community/etos/pkg/logging"
	"github.com/eiffel-community/etos/pkg/opentelemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	logservice "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricservice "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	traceservice "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type receiver struct {
	mu      sync.Mutex
	metrics []*metricpb.ResourceMetrics
	logs    []*logpb.ResourceLogs
	traces  []*tracepb.ResourceSpans
}

type metricsReceiver struct {
	metricservice.UnimplementedMetricsServiceServer
	receiver *receiver
}

// Export captures OTLP resource metrics under the receiver mutex.
func (s metricsReceiver) Export(
	_ context.Context, req *metricservice.ExportMetricsServiceRequest,
) (*metricservice.ExportMetricsServiceResponse, error) {
	s.receiver.mu.Lock()
	defer s.receiver.mu.Unlock()
	s.receiver.metrics = append(s.receiver.metrics, req.ResourceMetrics...)
	return &metricservice.ExportMetricsServiceResponse{}, nil
}

type logsReceiver struct {
	logservice.UnimplementedLogsServiceServer
	receiver *receiver
}

// Export captures OTLP resource logs under the receiver mutex.
func (s logsReceiver) Export(
	_ context.Context, req *logservice.ExportLogsServiceRequest,
) (*logservice.ExportLogsServiceResponse, error) {
	s.receiver.mu.Lock()
	defer s.receiver.mu.Unlock()
	s.receiver.logs = append(s.receiver.logs, req.ResourceLogs...)
	return &logservice.ExportLogsServiceResponse{}, nil
}

type tracesReceiver struct {
	traceservice.UnimplementedTraceServiceServer
	receiver *receiver
}

// Export captures OTLP resource spans under the receiver mutex.
func (s tracesReceiver) Export(
	_ context.Context, req *traceservice.ExportTraceServiceRequest,
) (*traceservice.ExportTraceServiceResponse, error) {
	s.receiver.mu.Lock()
	defer s.receiver.mu.Unlock()
	s.receiver.traces = append(s.receiver.traces, req.ResourceSpans...)
	return &traceservice.ExportTraceServiceResponse{}, nil
}

// startReceiver starts a loopback OTLP server for all three signals and registers shutdown cleanup.
func startReceiver(t *testing.T, options ...grpc.ServerOption) (*receiver, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(options...)
	r := &receiver{}
	metricservice.RegisterMetricsServiceServer(server, metricsReceiver{receiver: r})
	logservice.RegisterLogsServiceServer(server, logsReceiver{receiver: r})
	traceservice.RegisterTraceServiceServer(server, tracesReceiver{receiver: r})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil {
			t.Error(err)
		}
	}()
	t.Cleanup(func() { server.Stop(); <-done })
	return r, "http://" + listener.Addr().String()
}

// TestShortLivedProcessExportsAllSignalsAndCorrelation verifies final export,
// resource identity and log/trace correlation.
//
//nolint:gocyclo // One wire fixture verifies cross-signal identity and final-flush ordering.
func TestShortLivedProcessExportsAllSignalsAndCorrelation(t *testing.T) {
	for _, name := range []string{"etos-controller-manager", "environment-provider"} {
		t.Run(name, func(t *testing.T) {
			r, endpoint := startReceiver(t)
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", endpoint)
			t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
			t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "3600000")
			lifecycle := opentelemetry.New(name, "etos")
			if err := lifecycle.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := lifecycle.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			})
			var console bytes.Buffer
			ctx := logging.New(zap.Options{DestWriter: &console}).WithConsole().WithOtel(lifecycle).Start(t.Context())
			ctx, span := otel.Tracer("provider").Start(ctx, "RunProvider")
			spanContext := trace.SpanContextFromContext(ctx)
			meter := otel.Meter("etos/messaging")
			counter, err := meter.Int64Counter("etos_message_publish_attempts_total", metric.WithUnit("1"))
			if err != nil {
				t.Fatal(err)
			}
			histogram, err := meter.Float64Histogram("etos_message_publish_duration_seconds", metric.WithUnit("s"))
			if err != nil {
				t.Fatal(err)
			}
			attrs := metric.WithAttributes(attribute.String("etos_cluster", "team/blue"),
				attribute.String("outcome", "confirmed"), attribute.String("operation", "publish"))
			counter.Add(ctx, 1, attrs)
			histogram.Record(ctx, 0.125, attrs)
			logging.FromContextOrDiscard(ctx).Info("Provider provisioning completed",
				"event", "etos.provider.completed", "duration_seconds", 0.125, "disableUserLog", true)
			span.End()
			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			if err := lifecycle.Shutdown(canceled); err != nil {
				t.Fatal(err)
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			if len(r.metrics) == 0 || len(r.logs) == 0 || len(r.traces) == 0 {
				t.Fatalf("final export missing signals: metrics=%d logs=%d traces=%d", len(r.metrics), len(r.logs), len(r.traces))
			}
			for _, resource := range []*resourcepb.Resource{r.metrics[0].Resource, r.logs[0].Resource, r.traces[0].Resource} {
				if attr(resource.Attributes, "service.name").GetStringValue() != name ||
					attr(resource.Attributes, "service.instance.id").GetStringValue() == "" ||
					attr(resource.Attributes, "service.namespace").GetStringValue() != "etos" {
					t.Fatalf("resource identity missing: %+v", resource)
				}
			}
			observed := false
			for _, rm := range r.metrics {
				for _, scope := range rm.ScopeMetrics {
					for _, m := range scope.Metrics {
						if h := m.GetHistogram(); h != nil {
							p := h.DataPoints[0]
							observed = p.Count == 1 && p.GetSum() == 0.125 && len(p.ExplicitBounds) == 11 && len(p.BucketCounts) == 12
							if h.AggregationTemporality != metricpb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE {
								t.Fatal("histogram must be cumulative")
							}
						}
					}
				}
			}
			if !observed {
				t.Fatal("first cumulative histogram sample lost or rebucketed")
			}
			record := r.logs[0].ScopeLogs[0].LogRecords[0]
			traceID, spanID := spanContext.TraceID(), spanContext.SpanID()
			if !bytes.Equal(record.TraceId, traceID[:]) || !bytes.Equal(record.SpanId, spanID[:]) {
				t.Fatalf("native log correlation missing: %+v", record)
			}
			if record.Body.GetStringValue() != "Provider provisioning completed" ||
				attr(record.Attributes, "duration_seconds").GetDoubleValue() != 0.125 ||
				attr(record.Attributes, "trace_id").GetStringValue() != spanContext.TraceID().String() {
				t.Fatalf("structured completion record changed: %+v", record)
			}
			if !strings.Contains(console.String(), "Provider provisioning completed") ||
				strings.Contains(console.String(), "otel.context") {
				t.Fatalf("console routing changed: %s", console.String())
			}
			ended := r.traces[0].ScopeSpans[0].Spans[0]
			if ended.Name != "RunProvider" || ended.EndTimeUnixNano < ended.StartTimeUnixNano {
				t.Fatal("ended span was lost")
			}
		})
	}
}

// attr returns the first OTLP attribute matching the key, or nil when none exists.
func attr(attrs []*commonpb.KeyValue, key string) *commonpb.AnyValue {
	for _, value := range attrs {
		if value.Key == key {
			return value.Value
		}
	}
	return nil
}

// TestSignalEndpointOverride verifies that metrics use their endpoint override while traces use the common endpoint.
func TestSignalEndpointOverride(t *testing.T) {
	common, endpoint := startReceiver(t)
	specific, override := startReceiver(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", endpoint)
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", override)
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
	lifecycle := opentelemetry.New("override", "etos")
	if err := lifecycle.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	counter, err := otel.Meter("override").Int64Counter("test_counter")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(t.Context(), 1)
	_, span := otel.Tracer("override").Start(t.Context(), "override")
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := lifecycle.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if len(common.metrics) != 0 || len(specific.metrics) == 0 || len(common.traces) == 0 {
		t.Fatal("signal-specific endpoint did not override common endpoint")
	}
}

// TestTLSExporterTrustConfiguration verifies metric delivery using an explicitly trusted Collector certificate.
func TestTLSExporterTrustConfiguration(t *testing.T) {
	certificateServer := httptest.NewTLSServer(nil)
	certificates := certificateServer.TLS.Certificates
	certificate := certificateServer.Certificate()
	certificateServer.Close()
	receiver, endpoint := startReceiver(t, grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: certificates, MinVersion: tls.VersionTLS12,
	})))
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(caPath, ca, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", strings.Replace(endpoint, "http://", "https://", 1))
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", caPath)
	lifecycle := opentelemetry.New("tls-test", "etos")
	if err := lifecycle.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	counter, err := otel.Meter("tls-test").Int64Counter("tls_counter")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(t.Context(), 1)
	if err := lifecycle.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	if len(receiver.metrics) == 0 {
		t.Fatal("TLS receiver did not receive final metrics")
	}
}
