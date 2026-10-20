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
	defer listener.Close()

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
