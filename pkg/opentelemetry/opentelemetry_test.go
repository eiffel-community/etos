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
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
)

// TestNewProviderResourceIdentity verifies provider resource identity attributes.
func TestNewProviderResourceIdentity(t *testing.T) {
	tests := []struct {
		name            string
		serviceName     string
		configuredName  string
		providerName    string
		clusterName     string
		wantServiceName string
		wantClusterName string
	}{
		{
			name:            "configured stable service name",
			serviceName:     "resource-name",
			configuredName:  "etos-iut-provider",
			providerName:    "custom-provider",
			clusterName:     "team-cluster",
			wantServiceName: "etos-iut-provider",
			wantClusterName: "team-cluster",
		},
		{
			name:            "fallback service name without cluster",
			serviceName:     "custom-provider",
			providerName:    "custom-provider",
			wantServiceName: "custom-provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_SERVICE_NAME", tt.configuredName)
			tracer := NewProvider(tt.serviceName, tt.providerName, tt.clusterName)
			res, err := tracer.newOtelResource(context.Background())
			if err != nil {
				t.Fatalf("newOtelResource() error = %v", err)
			}

			attributes := make(map[string]string)
			for _, attribute := range res.Set().ToSlice() {
				attributes[string(attribute.Key)] = attribute.Value.AsString()
			}
			for key, want := range map[string]string{
				"service.name":       tt.wantServiceName,
				"service.namespace":  "etos",
				"service.version":    vcsRevision(),
				"etos.provider.name": tt.providerName,
			} {
				if got := attributes[key]; got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if tt.wantClusterName != "" {
				if got := attributes["etos.cluster.name"]; got != tt.wantClusterName {
					t.Errorf("etos.cluster.name = %q, want %q", got, tt.wantClusterName)
				}
			} else if _, ok := attributes["etos.cluster.name"]; ok {
				t.Errorf("etos.cluster.name unexpectedly set to %q", attributes["etos.cluster.name"])
			}
		})
	}
}

// metricsCollector records whether resource metrics were exported.
type metricsCollector struct {
	metricsv1.UnimplementedMetricsServiceServer
	exported atomic.Bool
}

// Export records whether the collector received resource metrics.
func (c *metricsCollector) Export(
	_ context.Context,
	request *metricsv1.ExportMetricsServiceRequest,
) (*metricsv1.ExportMetricsServiceResponse, error) {
	if len(request.ResourceMetrics) > 0 {
		c.exported.Store(true)
	}
	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

// TestRuntimeMetricsAreFlushedOnShutdown verifies shutdown flushes runtime metrics.
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
	tracer := New("provider-test", "etos")
	if err := tracer.StartWithRuntimeMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tracer.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if !collector.exported.Load() {
		t.Fatal("runtime metrics were not exported during shutdown")
	}
}
