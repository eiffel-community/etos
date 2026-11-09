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
	"sync/atomic"
	"testing"
	"time"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type stalledMetricExporter struct{ shutdown atomic.Int32 }

// Temporality uses the SDK's default selection for the supplied instrument kind.
func (*stalledMetricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return sdkmetric.DefaultTemporalitySelector(kind)
}

// Aggregation uses the SDK's default selection for the supplied instrument kind.
func (*stalledMetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

// Export simulates a stalled exporter by waiting for context cancellation.
func (*stalledMetricExporter) Export(ctx context.Context, _ *metricdata.ResourceMetrics) error {
	<-ctx.Done()
	return ctx.Err()
}

// Shutdown counts cleanup attempts without blocking.
func (e *stalledMetricExporter) Shutdown(context.Context) error { e.shutdown.Add(1); return nil }

// ForceFlush succeeds without exporting buffered data.
func (*stalledMetricExporter) ForceFlush(context.Context) error { return nil }

type shutdownTraceExporter struct{ shutdown atomic.Int32 }

// ExportSpans accepts spans without retaining or transmitting them.
func (*shutdownTraceExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error { return nil }

// Shutdown counts trace exporter cleanup attempts.
func (e *shutdownTraceExporter) Shutdown(context.Context) error {
	e.shutdown.Add(1)
	return nil
}

type shutdownLogExporter struct{ shutdown atomic.Int32 }

// Export accepts log records without retaining or transmitting them.
func (*shutdownLogExporter) Export(context.Context, []sdklog.Record) error { return nil }

// Shutdown counts log exporter cleanup attempts.
func (e *shutdownLogExporter) Shutdown(context.Context) error { e.shutdown.Add(1); return nil }

// ForceFlush succeeds without exporting buffered records.
func (*shutdownLogExporter) ForceFlush(context.Context) error { return nil }

// TestSlowMetricsCannotStarveTraceAndLogShutdown verifies independent, idempotent cleanup
// after a metric export timeout.
func TestSlowMetricsCannotStarveTraceAndLogShutdown(t *testing.T) {
	metrics, traces, logs := &stalledMetricExporter{}, &shutdownTraceExporter{}, &shutdownLogExporter{}
	lifecycle := New("shutdown", "etos")
	lifecycle.MeterProvider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(
		sdkmetric.NewPeriodicReader(metrics, sdkmetric.WithInterval(time.Hour))))
	lifecycle.tracerProvider = sdktrace.NewTracerProvider(sdktrace.WithSyncer(traces))
	lifecycle.LoggerProvider = sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(logs)))
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := lifecycle.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow metric exporter error lost: %v", err)
	}
	if traces.shutdown.Load() != 1 || logs.shutdown.Load() != 1 {
		t.Fatal("later signals were starved")
	}
	if err := lifecycle.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("repeated shutdown lost original error")
	}
	if metrics.shutdown.Load() != 1 || traces.shutdown.Load() != 1 || logs.shutdown.Load() != 1 {
		t.Fatal("shutdown called more than once")
	}
}
