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
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestRegisterPrometheusExportsZeroBaselinesAndRetainsEvents verifies zero baselines and cumulative repeated scrapes.
func TestRegisterPrometheusExportsZeroBaselinesAndRetainsEvents(t *testing.T) {
	transitions, registry := newTransitionRegistry(t)
	families := gatherTransitions(t, registry)
	if len(families) != 4 {
		t.Fatalf("metric family count = %d, want 4", len(families))
	}
	for _, test := range []struct {
		counter, histogram string
		outcomes           []string
	}{
		{testRunFinishedName, testRunDurationName, []string{"success", "failure", "aborted", "timed_out", "inconclusive", "unknown"}},
		{environmentRequestFinishedName, environmentRequestDurationName, []string{"success", "failure"}},
	} {
		counter := findPrometheusFamily(t, families, test.counter)
		histogram := findPrometheusFamily(t, families, test.histogram)
		if len(counter.Metric) != len(test.outcomes) || len(histogram.Metric) != len(test.outcomes) {
			t.Fatalf("series counts = %d/%d, want %d each", len(counter.Metric), len(histogram.Metric), len(test.outcomes))
		}
		for _, outcome := range test.outcomes {
			count := prometheusMetricForOutcome(t, counter, outcome)
			duration := prometheusMetricForOutcome(t, histogram, outcome)
			if count.GetCounter().GetValue() != 0 {
				t.Errorf("%s baseline = %v, want 0", test.counter, count.GetCounter())
			}
			assertDuration(t, duration.GetHistogram(), 0, 0)
			assertDurationBuckets(t, duration.GetHistogram(), 0, 0)
		}
	}

	start := time.Date(2026, time.April, 5, 6, 7, 8, 0, time.UTC)
	for event := 1; event <= 2; event++ {
		transitions.RecordTestRunFinished(UnassignedCluster, OutcomeSuccess, start, start.Add(125*time.Millisecond))
		transitions.RecordEnvironmentRequestFinished(UnassignedCluster, OutcomeFailure, start, start.Add(125*time.Millisecond))
		for scrape := 1; scrape <= 2; scrape++ {
			families = gatherTransitions(t, registry)
			for _, test := range []struct {
				counter, histogram, outcome string
			}{
				{testRunFinishedName, testRunDurationName, OutcomeSuccess},
				{environmentRequestFinishedName, environmentRequestDurationName, OutcomeFailure},
			} {
				counter := prometheusMetricForOutcome(t, findPrometheusFamily(t, families, test.counter), test.outcome)
				if got := counter.GetCounter().GetValue(); got != float64(event) {
					t.Errorf("%s count on scrape %d = %f, want %d", test.counter, scrape, got, event)
				}
				histogram := prometheusMetricForOutcome(t, findPrometheusFamily(t, families, test.histogram), test.outcome)
				assertDuration(t, histogram.GetHistogram(), uint64(event), float64(event)*0.125)
				assertDurationBuckets(t, histogram.GetHistogram(), uint64(event), 0.125)
			}
			for _, outcome := range testRunOutcomes {
				if outcome != OutcomeSuccess {
					histogram := prometheusMetricForOutcome(t, findPrometheusFamily(t, families, testRunDurationName), outcome)
					assertDuration(t, histogram.GetHistogram(), 0, 0)
				}
			}
		}
	}
}

// TestRecordFinishedBoundsOutcomes verifies valid outcomes and invalid fallbacks never introduce additional labels.
func TestRecordFinishedBoundsOutcomes(t *testing.T) {
	tests := []struct {
		input, testRun, environmentRequest string
	}{
		{OutcomeSuccess, OutcomeSuccess, OutcomeSuccess},
		{OutcomeFailure, OutcomeFailure, OutcomeFailure},
		{OutcomeAborted, OutcomeAborted, OutcomeFailure},
		{OutcomeTimedOut, OutcomeTimedOut, OutcomeFailure},
		{OutcomeInconclusive, OutcomeInconclusive, OutcomeFailure},
		{OutcomeUnknown, OutcomeUnknown, OutcomeFailure},
		{"unbounded-resource-name", OutcomeUnknown, OutcomeFailure},
		{"", OutcomeUnknown, OutcomeFailure},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			transitions, registry := newTransitionRegistry(t)
			start := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
			transitions.RecordTestRunFinished(UnassignedCluster, test.input, start, start.Add(2*time.Second))
			transitions.RecordEnvironmentRequestFinished(UnassignedCluster, test.input, start, start.Add(2*time.Second))
			families := gatherTransitions(t, registry)
			for _, expected := range []struct {
				counter, histogram, outcome string
				series                      int
			}{
				{testRunFinishedName, testRunDurationName, test.testRun, 6},
				{environmentRequestFinishedName, environmentRequestDurationName, test.environmentRequest, 2},
			} {
				counter := findPrometheusFamily(t, families, expected.counter)
				histogram := findPrometheusFamily(t, families, expected.histogram)
				if len(counter.Metric) != expected.series || len(histogram.Metric) != expected.series {
					t.Fatalf("unbounded series count: %d/%d, want %d", len(counter.Metric), len(histogram.Metric), expected.series)
				}
				if got := prometheusMetricForOutcome(t, counter, expected.outcome).GetCounter().GetValue(); got != 1 {
					t.Errorf("%s count = %f, want 1", expected.counter, got)
				}
				assertDuration(t, prometheusMetricForOutcome(t, histogram, expected.outcome).GetHistogram(), 1, 2)
			}
		})
	}
}

// TestRecordFinishedGuardsTimestamps verifies invalid times retain counts and equal times record a genuine zero duration.
func TestRecordFinishedGuardsTimestamps(t *testing.T) {
	start := time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)
	tests := []struct {
		name          string
		start, end    time.Time
		durationCount uint64
	}{
		{"missing start", time.Time{}, start, 0},
		{"missing end", start, time.Time{}, 0},
		{"both missing", time.Time{}, time.Time{}, 0},
		{"reversed", start, start.Add(-time.Second), 0},
		{"equal", start, start, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transitions, registry := newTransitionRegistry(t)
			transitions.RecordTestRunFinished(UnassignedCluster, OutcomeSuccess, test.start, test.end)
			transitions.RecordEnvironmentRequestFinished(UnassignedCluster, OutcomeFailure, test.start, test.end)
			for scrape := 1; scrape <= 2; scrape++ {
				families := gatherTransitions(t, registry)
				for _, expected := range []struct {
					counter, histogram, outcome string
				}{
					{testRunFinishedName, testRunDurationName, OutcomeSuccess},
					{environmentRequestFinishedName, environmentRequestDurationName, OutcomeFailure},
				} {
					if got := prometheusMetricForOutcome(t, findPrometheusFamily(t, families, expected.counter), expected.outcome).GetCounter().GetValue(); got != 1 {
						t.Errorf("%s count = %f, want 1 despite invalid times", expected.counter, got)
					}
					histogram := prometheusMetricForOutcome(t, findPrometheusFamily(t, families, expected.histogram), expected.outcome)
					assertDuration(t, histogram.GetHistogram(), test.durationCount, 0)
					assertDurationBuckets(t, histogram.GetHistogram(), test.durationCount, 0)
				}
			}
		})
	}
}

// TestRegisterPrometheusRejectsNilAndDuplicates verifies registration errors preserve already registered collectors.
func TestRegisterPrometheusRejectsNilAndDuplicates(t *testing.T) {
	transitions, registry := newTransitionRegistry(t)
	if err := transitions.RegisterPrometheus(nil); err == nil {
		t.Fatal("RegisterPrometheus(nil) succeeded")
	}
	for _, duplicate := range []*Transitions{transitions, NewTransitions()} {
		err := duplicate.RegisterPrometheus(registry)
		var alreadyRegistered prometheus.AlreadyRegisteredError
		if !errors.As(err, &alreadyRegistered) {
			t.Fatalf("duplicate registration error = %v, want AlreadyRegisteredError", err)
		}
	}
	start := time.Now()
	transitions.RecordTestRunFinished(UnassignedCluster, OutcomeSuccess, start, start.Add(time.Second))
	families := gatherTransitions(t, registry)
	if len(families) != 4 {
		t.Fatalf("family count after duplicate registration = %d, want 4", len(families))
	}
	if got := prometheusMetricForOutcome(t, findPrometheusFamily(t, families, testRunFinishedName), OutcomeSuccess).GetCounter().GetValue(); got != 1 {
		t.Errorf("original counter after duplicate registration = %f, want 1", got)
	}
}

// TestRegisterPrometheusRollsBackPartialRegistration verifies every failure position removes only owned collectors.
func TestRegisterPrometheusRollsBackPartialRegistration(t *testing.T) {
	for _, name := range []string{
		testRunFinishedName, testRunDurationName, environmentRequestFinishedName, environmentRequestDurationName,
	} {
		t.Run(name, func(t *testing.T) {
			transitions := NewTransitions()
			registry := prometheus.NewRegistry()
			existing := NewTransitions()
			collectors := map[string]prometheus.Collector{
				testRunFinishedName:            existing.testRunFinished,
				testRunDurationName:            existing.testRunDuration,
				environmentRequestFinishedName: existing.environmentRequestFinished,
				environmentRequestDurationName: existing.environmentRequestDuration,
			}
			conflict := collectors[name]
			if err := registry.Register(conflict); err != nil {
				t.Fatalf("Register(existing) error = %v", err)
			}
			err := transitions.RegisterPrometheus(registry)
			var alreadyRegistered prometheus.AlreadyRegisteredError
			if !errors.As(err, &alreadyRegistered) {
				t.Fatalf("partial registration error = %v, want AlreadyRegisteredError", err)
			}
			families := gatherTransitions(t, registry)
			if len(families) != 1 || families[0].GetName() != name {
				t.Fatalf("families after rollback = %v, want only existing %s", families, name)
			}
			if !registry.Unregister(conflict) {
				t.Fatal("rollback removed the pre-existing collector")
			}
			if err := transitions.RegisterPrometheus(registry); err != nil {
				t.Fatalf("RegisterPrometheus() after removing conflict error = %v", err)
			}
			if got := len(gatherTransitions(t, registry)); got != 4 {
				t.Errorf("family count after retry = %d, want 4", got)
			}
		})
	}
}

// newTransitionRegistry creates native collectors registered in an isolated, pedantic registry.
func newTransitionRegistry(t *testing.T) (*Transitions, *prometheus.Registry) {
	t.Helper()
	transitions := NewTransitions()
	registry := prometheus.NewPedanticRegistry()
	if err := transitions.RegisterPrometheus(registry); err != nil {
		t.Fatalf("RegisterPrometheus() error = %v", err)
	}
	return transitions, registry
}

// gatherTransitions gathers a cumulative registry snapshot for assertions.
func gatherTransitions(t *testing.T, registry *prometheus.Registry) []*dto.MetricFamily {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	return families
}

// findPrometheusFamily returns a gathered metric family with the requested name.
func findPrometheusFamily(t *testing.T, families []*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	t.Fatalf("Prometheus metric family %q was not gathered", name)
	return nil
}

// prometheusMetricForOutcome returns an unassigned metric and verifies the cluster/outcome dimensions.
func prometheusMetricForOutcome(t *testing.T, family *dto.MetricFamily, outcome string) *dto.Metric {
	t.Helper()
	for _, metric := range family.Metric {
		if len(metric.Label) != 2 || metric.Label[0].GetName() != clusterAttributeName || metric.Label[1].GetName() != outcomeAttributeName {
			t.Fatalf("%s labels = %v, want etos_cluster and outcome", family.GetName(), metric.Label)
		}
		if metric.Label[0].GetValue() == UnassignedCluster && metric.Label[1].GetValue() == outcome {
			return metric
		}
	}
	t.Fatalf("Prometheus metric %q has no outcome %q", family.GetName(), outcome)
	return nil
}

// assertDuration verifies histogram count and sum without manufacturing an initial sample.
func assertDuration(t *testing.T, histogram *dto.Histogram, count uint64, sum float64) {
	t.Helper()
	if histogram == nil {
		t.Fatal("histogram is nil")
	}
	if histogram.GetSampleCount() != count || histogram.GetSampleSum() != sum {
		t.Errorf("duration count/sum = %d/%f, want %d/%f", histogram.GetSampleCount(), histogram.GetSampleSum(), count, sum)
	}
}

// assertDurationBuckets verifies the exact exported bounds and cumulative counts for identical observations.
func assertDurationBuckets(t *testing.T, histogram *dto.Histogram, count uint64, seconds float64) {
	t.Helper()
	bounds := []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1800, 3600, 21600, 86400}
	if len(histogram.Bucket) != len(bounds) {
		t.Fatalf("duration bucket count = %d, want %d", len(histogram.Bucket), len(bounds))
	}
	for index, bound := range bounds {
		bucket := histogram.Bucket[index]
		want := uint64(0)
		if seconds <= bound {
			want = count
		}
		if bucket.GetUpperBound() != bound || bucket.GetCumulativeCount() != want {
			t.Errorf("bucket %d bound/count = %f/%d, want %f/%d", index, bucket.GetUpperBound(), bucket.GetCumulativeCount(), bound, want)
		}
	}
}
