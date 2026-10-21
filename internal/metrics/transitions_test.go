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

package metrics

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/eiffel-community/etos/pkg/opentelemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type failingInstrumentMeter struct {
	metric.Meter
	name string
	err  error
}

// Int64Counter injects an error for the configured instrument and delegates other counter creation.
func (m failingInstrumentMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	if name == m.name {
		return nil, m.err
	}
	return m.Meter.Int64Counter(name, opts...)
}

// Float64Histogram injects an error for the configured instrument and delegates other histogram creation.
func (m failingInstrumentMeter) Float64Histogram(
	name string, opts ...metric.Float64HistogramOption,
) (metric.Float64Histogram, error) {
	if name == m.name {
		return nil, m.err
	}
	return m.Meter.Float64Histogram(name, opts...)
}

// TestTransitionConstructionErrorsAreReturned verifies that each instrument's initialization error reaches the caller.
func TestTransitionConstructionErrorsAreReturned(t *testing.T) {
	for _, name := range []string{
		testRunFinishedName, testRunDurationName, environmentRequestFinishedName, environmentRequestDurationName,
	} {
		t.Run(name, func(t *testing.T) {
			injected := errors.New("instrument construction failure")
			meter := failingInstrumentMeter{noop.NewMeterProvider().Meter("test"), name, injected}
			if _, err := NewTransitions(meter); !errors.Is(err, injected) {
				t.Fatalf("constructor error lost: %v", err)
			}
		})
	}
}

// newTestMeter creates a manually collected meter with ETOS histogram settings and automatic cleanup.
func newTestMeter(t *testing.T) (metric.Meter, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	opts := append(opentelemetry.MetricProviderOptions(), sdkmetric.WithReader(reader))
	provider := sdkmetric.NewMeterProvider(opts...)
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return provider.Meter("etos/controller"), reader
}

// newTransitionRegistry creates transition instruments and a reader for inspecting their measurements.
func newTransitionRegistry(t *testing.T) (*Transitions, *sdkmetric.ManualReader) {
	t.Helper()
	meter, reader := newTestMeter(t)
	transitions, err := NewTransitions(meter)
	if err != nil {
		t.Fatal(err)
	}
	return transitions, reader
}

// gatherTransitions collects measurements indexed by instrument name, failing the test on collection errors.
func gatherTransitions(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	result := make(map[string]metricdata.Metrics)
	for _, scope := range data.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			result[instrument.Name] = instrument
		}
	}
	return result
}

// counterPoint validates a cumulative counter and returns the matching cluster/outcome value.
// It fails the test if the expected point is absent.
func counterPoint(t *testing.T, data metricdata.Metrics, cluster, outcome string) int64 {
	t.Helper()
	sum, ok := data.Data.(metricdata.Sum[int64])
	if !ok || !sum.IsMonotonic || sum.Temporality != metricdata.CumulativeTemporality || data.Unit != "1" || data.Description == "" {
		t.Fatalf("invalid native counter: %+v", data)
	}
	for _, point := range sum.DataPoints {
		if point.Attributes.Len() != 2 {
			t.Fatal("unbounded counter attributes", point.Attributes)
		}
		want := attribute.NewSet(attribute.String(clusterAttributeName, cluster), attribute.String(outcomeAttributeName, outcome))
		if point.Attributes.Equals(&want) {
			return point.Value
		}
	}
	t.Fatalf("missing counter baseline %s/%s", cluster, outcome)
	return 0
}

// histogramPoint validates a cumulative duration histogram and returns the matching cluster/outcome point,
// or nil when that observation is absent.
func histogramPoint(t *testing.T, data metricdata.Metrics, cluster, outcome string) *metricdata.HistogramDataPoint[float64] {
	t.Helper()
	if data.Data == nil {
		return nil
	}
	histogram, ok := data.Data.(metricdata.Histogram[float64])
	if !ok || histogram.Temporality != metricdata.CumulativeTemporality || data.Unit != "s" || data.Description == "" {
		t.Fatalf("invalid native histogram: %+v", data)
	}
	for _, point := range histogram.DataPoints {
		want := attribute.NewSet(attribute.String(clusterAttributeName, cluster), attribute.String(outcomeAttributeName, outcome))
		if point.Attributes.Equals(&want) {
			return &point
		}
	}
	return nil
}

// assertDuration checks histogram totals and bucket boundaries, requiring absence when no observations are expected.
func assertDuration(t *testing.T, point *metricdata.HistogramDataPoint[float64], count uint64, sum float64) {
	t.Helper()
	if count == 0 {
		if point != nil {
			t.Fatal("empty histogram must be absent, not a fake sample")
		}
		return
	}
	if point == nil || point.Count != count || point.Sum != sum {
		t.Fatalf("histogram = %+v, want count %d sum %v", point, count, sum)
	}
	if !reflect.DeepEqual(point.Bounds, durationBucketBounds) || len(point.BucketCounts) != len(point.Bounds)+1 {
		t.Fatalf("histogram boundaries changed: %+v", point)
	}
	var bucketCount uint64
	for _, count := range point.BucketCounts {
		bucketCount += count
	}
	if bucketCount != point.Count {
		t.Fatal("per-bucket counts do not sum to total")
	}
}

// TestNativeCounterBaselinesAndRealHistograms verifies zero counter baselines and cumulative real observations without replay.
func TestNativeCounterBaselinesAndRealHistograms(t *testing.T) {
	transitions, reader := newTransitionRegistry(t)
	data := gatherTransitions(t, reader)
	if len(data) != 2 {
		t.Fatalf("only counters should exist before an event, got %d", len(data))
	}
	for _, outcome := range testRunOutcomes {
		if counterPoint(t, data[testRunFinishedName], UnassignedCluster, outcome) != 0 {
			t.Fatal("nonzero baseline")
		}
	}
	for _, outcome := range environmentRequestOutcomes {
		if counterPoint(t, data[environmentRequestFinishedName], UnassignedCluster, outcome) != 0 {
			t.Fatal("nonzero baseline")
		}
	}
	start := time.Now()
	for event := 1; event <= 2; event++ {
		transitions.RecordTestRunFinished(t.Context(), UnassignedCluster, OutcomeSuccess, start, start.Add(125*time.Millisecond))
		transitions.RecordEnvironmentRequestFinished(t.Context(), UnassignedCluster, OutcomeFailure, start, start.Add(125*time.Millisecond))
		for range 2 {
			data = gatherTransitions(t, reader)
			for _, pair := range []struct{ counter, histogram, outcome string }{
				{testRunFinishedName, testRunDurationName, OutcomeSuccess},
				{environmentRequestFinishedName, environmentRequestDurationName, OutcomeFailure},
			} {
				if counterPoint(t, data[pair.counter], UnassignedCluster, pair.outcome) != int64(event) {
					t.Fatal("counter replayed or dropped an event")
				}
				point := histogramPoint(t, data[pair.histogram], UnassignedCluster, pair.outcome)
				assertDuration(t, point, uint64(event), float64(event)*0.125)
				for index, count := range point.BucketCounts {
					want := uint64(0)
					if index == 3 {
						want = uint64(event)
					}
					if count != want {
						t.Fatalf("bucket %d = %d want %d", index, count, want)
					}
				}
			}
		}
	}
}

// TestRecordFinishedBoundsOutcomes verifies that completion metrics map arbitrary outcomes into the bounded vocabulary.
func TestRecordFinishedBoundsOutcomes(t *testing.T) {
	for _, input := range append(append([]string{}, testRunOutcomes...), "", "unbounded-resource-name") {
		t.Run(input, func(t *testing.T) {
			transitions, reader := newTransitionRegistry(t)
			start := time.Now()
			transitions.RecordTestRunFinished(t.Context(), UnassignedCluster, input, start, start.Add(time.Second))
			transitions.RecordEnvironmentRequestFinished(t.Context(), UnassignedCluster, input, start, start.Add(time.Second))
			data := gatherTransitions(t, reader)
			if counterPoint(t, data[testRunFinishedName], UnassignedCluster, boundedOutcome(input)) != 1 ||
				counterPoint(t, data[environmentRequestFinishedName], UnassignedCluster, boundedEnvironmentRequestOutcome(input)) != 1 {
				t.Fatal("outcome mapping changed")
			}
			if len(data[testRunDurationName].Data.(metricdata.Histogram[float64]).DataPoints) != 1 {
				t.Fatal("histogram created unobserved outcomes")
			}
		})
	}
}

// TestRecordFinishedGuardsTimestamps verifies that invalid timestamps omit durations without losing completion counts.
func TestRecordFinishedGuardsTimestamps(t *testing.T) {
	start := time.Now()
	for _, test := range []struct {
		name       string
		start, end time.Time
		count      uint64
	}{
		{"missing start", time.Time{}, start, 0}, {"missing end", start, time.Time{}, 0},
		{"both missing", time.Time{}, time.Time{}, 0}, {"reversed", start, start.Add(-time.Second), 0},
		{"equal", start, start, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			transitions, reader := newTransitionRegistry(t)
			transitions.RecordTestRunFinished(t.Context(), UnassignedCluster, OutcomeSuccess, test.start, test.end)
			transitions.RecordEnvironmentRequestFinished(t.Context(), UnassignedCluster, OutcomeFailure, test.start, test.end)
			for range 2 {
				data := gatherTransitions(t, reader)
				if counterPoint(t, data[testRunFinishedName], UnassignedCluster, OutcomeSuccess) != 1 ||
					counterPoint(t, data[environmentRequestFinishedName], UnassignedCluster, OutcomeFailure) != 1 {
					t.Fatal("invalid time must still increment counter")
				}
				assertDuration(t, histogramPoint(t, data[testRunDurationName], UnassignedCluster, OutcomeSuccess), test.count, 0)
				assertDuration(t, histogramPoint(t, data[environmentRequestDurationName], UnassignedCluster, OutcomeFailure), test.count, 0)
			}
		})
	}
}

// TestNativeTransitionScaleAndConcurrency verifies that concurrent recording preserves 1000 clusters without overflow.
func TestNativeTransitionScaleAndConcurrency(t *testing.T) {
	transitions, reader := newTransitionRegistry(t)
	var wg sync.WaitGroup
	for cluster := range 1000 {
		wg.Go(func() {
			identity := fmt.Sprintf("namespace/cluster-%d", cluster)
			transitions.InitializeCluster(identity)
			start := time.Now()
			transitions.RecordTestRunFinished(t.Context(), identity, OutcomeSuccess, start, start.Add(time.Second))
		})
	}
	gatherTransitions(t, reader)
	wg.Wait()
	data := gatherTransitions(t, reader)
	sum := data[testRunFinishedName].Data.(metricdata.Sum[int64])
	if len(sum.DataPoints) != 6006 {
		t.Fatalf("cluster series collapsed: %d", len(sum.DataPoints))
	}
	for _, point := range sum.DataPoints {
		if _, ok := point.Attributes.Value("otel.metric.overflow"); ok {
			t.Fatal("overflow makes cluster telemetry incomplete")
		}
	}
}
