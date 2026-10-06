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
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestClassifyUsesPersistedResourceState verifies state classification from persisted fields.
func TestClassifyUsesPersistedResourceState(t *testing.T) {
	tests := []struct {
		name  string
		obj   unstructured.Unstructured
		state string
	}{
		{
			name: "deleting",
			obj: unstructured.Unstructured{Object: map[string]any{
				"metadata": map[string]any{"deletionTimestamp": "2026-01-01T00:00:00Z"},
			}},
			state: stateDeleting,
		},
		{
			name: "completed",
			obj: unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{"completionTime": "2026-01-01T00:00:00Z"},
			}},
			state: stateCompleted,
		},
		{
			name: "ready",
			obj: unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{"conditions": []any{
					map[string]any{"type": "Ready", "status": string(metav1.ConditionTrue)},
				}},
			}},
			state: stateReady,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classify(test.obj); got != test.state {
				t.Fatalf("classify() = %q, want %q", got, test.state)
			}
		})
	}
}

// TestClassifyTestRunPhaseUsesPersistedLifecycleState covers persisted lifecycle evidence.
func TestClassifyTestRunPhaseUsesPersistedLifecycleState(t *testing.T) {
	tests := []struct {
		name       string
		conditions []map[string]any
		status     map[string]any
		metadata   map[string]any
		want       string
	}{
		{name: "accepted", want: phaseAccepted},
		{name: "environment provisioning", conditions: []map[string]any{
			{"type": "Environment", "status": "False", "reason": "Pending"},
		}, want: phaseEnvironmentProvision},
		{name: "environment ready", conditions: []map[string]any{
			{"type": "Environment", "status": "True", "reason": "Completed"},
		}, want: phaseEnvironmentReady},
		{name: "suite runner starting", conditions: []map[string]any{
			{"type": "SuiteRunner", "status": "False", "reason": "Starting"},
		}, want: phaseSuiteRunnerStarting},
		{name: "running", conditions: []map[string]any{
			{"type": "SuiteRunner", "status": "True", "reason": "Active"},
		}, want: phaseRunning},
		{name: "completing", conditions: []map[string]any{
			{"type": "Active", "status": "False", "reason": "Completed"},
		}, want: phaseCompleting},
		{name: "completed", status: map[string]any{
			"completionTime": "2026-01-01T00:00:00Z",
		}, want: phaseCompleted},
		{name: "failed", conditions: []map[string]any{
			{"type": "Active", "status": "False", "reason": "Failed"},
		}, want: phaseFailed},
		{name: "timed out", conditions: []map[string]any{
			{"type": "Active", "status": "False", "reason": "DeadlineExceeded"},
		}, want: phaseTimedOut},
		{name: "aborted", conditions: []map[string]any{
			{"type": "Active", "status": "False", "reason": "Aborted"},
		}, want: phaseAborted},
		{name: "cleaning up", metadata: map[string]any{
			"deletionTimestamp": "2026-01-01T00:00:00Z",
		}, want: phaseCleaningUp},
		{name: "cleanup failed", metadata: map[string]any{
			"deletionTimestamp": "2026-01-01T00:00:00Z",
		}, conditions: []map[string]any{
			{"type": "Cleanup", "status": "False", "reason": "Failed"},
		}, want: phaseCleanupFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object := map[string]any{}
			if test.metadata != nil {
				object["metadata"] = test.metadata
			}
			if test.status != nil || test.conditions != nil {
				status := test.status
				if status == nil {
					status = map[string]any{}
				}
				if test.conditions != nil {
					rawConditions := make([]any, 0, len(test.conditions))
					for _, condition := range test.conditions {
						rawConditions = append(rawConditions, condition)
					}
					status["conditions"] = rawConditions
				}
				object["status"] = status
			}

			if got := classifyTestRunPhase(unstructured.Unstructured{Object: object}); got != test.want {
				t.Fatalf("classifyTestRunPhase() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestClassifyTestRunVerdictNormalizesPersistedValues checks bounded verdict normalization.
func TestClassifyTestRunVerdictNormalizesPersistedValues(t *testing.T) {
	tests := []struct {
		name    string
		verdict string
		want    string
	}{
		{name: "passed", verdict: "PASSED", want: verdictPassed},
		{name: "failed", verdict: "failed", want: verdictFailed},
		{name: "inconclusive", verdict: "INCONCLUSIVE", want: verdictInconclusive},
		{name: "none", verdict: "", want: verdictNone},
		{name: "unknown", verdict: "unexpected", want: verdictUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object := unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{"verdict": test.verdict},
			}}
			if got := classifyTestRunVerdict(object); got != test.want {
				t.Fatalf("classifyTestRunVerdict() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestClassifyTestRunOperationalConclusionUsesActiveAndSuiteRunnerEvidence checks condition-based conclusions.
func TestClassifyTestRunOperationalConclusionUsesActiveAndSuiteRunnerEvidence(t *testing.T) {
	tests := []struct {
		name       string
		conditions []map[string]any
		want       string
	}{
		{
			name: "successful from suite runner reason",
			conditions: []map[string]any{
				{"type": "SuiteRunner", "status": "True", "reason": "Completed"},
			},
			want: conclusionSuccessful,
		},
		{
			name: "failed from active message",
			conditions: []map[string]any{
				{"type": "Active", "status": "False", "reason": "Completed", "message": "Suite runner failed"},
			},
			want: conclusionFailed,
		},
		{
			name: "timed out",
			conditions: []map[string]any{
				{"type": "Active", "status": "False", "reason": "DeadlineExceeded"},
			},
			want: conclusionTimedOut,
		},
		{
			name: "aborted",
			conditions: []map[string]any{
				{"type": "SuiteRunner", "status": "False", "reason": "Aborted"},
			},
			want: conclusionAborted,
		},
		{
			name: "running",
			conditions: []map[string]any{
				{"type": "SuiteRunner", "status": "True", "reason": "Active"},
			},
			want: conclusionRunning,
		},
		{
			name: "unknown without relevant conditions",
			conditions: []map[string]any{
				{"type": "Environment", "status": "True", "reason": "Completed"},
			},
			want: conclusionUnknown,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rawConditions := make([]any, 0, len(test.conditions))
			for _, condition := range test.conditions {
				rawConditions = append(rawConditions, condition)
			}
			object := unstructured.Unstructured{Object: map[string]any{
				"status": map[string]any{"conditions": rawConditions},
			}}
			if got := classifyTestRunOperationalConclusion(object); got != test.want {
				t.Fatalf("classifyTestRunOperationalConclusion() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestTestRunLifecycleWindowsUseHalfOpenEventTimeBounds verifies retained creation compatibility.
func TestTestRunLifecycleWindowsUseHalfOpenEventTimeBounds(t *testing.T) {
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	items := []unstructured.Unstructured{
		newTestRun(now.Add(-time.Hour), nil, nil, nil),
		newTestRun(now.Add(-30*time.Minute), timePointer(now.Add(-30*time.Minute)), nil, nil),
		newTestRun(now, timePointer(now), nil, nil),
		newTestRun(now.Add(time.Second), timePointer(now.Add(time.Second)), nil, nil),
		newTestRun(now.Add(-2*time.Hour), timePointer(now.Add(-time.Hour)), nil, nil),
	}
	for _, index := range []int{1, 2, 4} {
		if err := unstructured.SetNestedSlice(items[index].Object, []any{
			map[string]any{"type": "Active", "status": "True", "reason": "Completed"},
		}, "status", "conditions"); err != nil {
			t.Fatalf("set successful condition: %v", err)
		}
	}
	collector := collectLifecycleForTest(t, "window-boundaries", now, items)

	if got := metricValue(t, collector, testRunCreated, "1h", "window-boundaries"); got != 2 {
		t.Fatalf("created 1h = %v, want 2 (strict lower edge; includes now)", got)
	}
	if got := metricValue(t, collector, testRunCreated, "24h", "window-boundaries"); got != 4 {
		t.Fatalf("created 24h = %v, want 4", got)
	}
}

// TestInventoryOmitsMigratedHistoricalMetrics verifies removed families cannot mislead consumers.
func TestInventoryOmitsMigratedHistoricalMetrics(t *testing.T) {
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	collector := collectLifecycleForTest(t, "migrated-history", now, []unstructured.Unstructured{
		newTestRun(now.Add(-75*time.Second), timePointer(now.Add(-15*time.Second)), nil, nil),
	})
	registry := prometheus.NewRegistry()
	if err := collector.Register(registry); err != nil {
		t.Fatalf("register inventory: %v", err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather inventory: %v", err)
	}
	for _, family := range families {
		switch family.GetName() {
		case "etos_inventory_testrun_finished", "etos_inventory_testrun_duration_seconds":
			t.Errorf("migrated historical family still exported: %s", family.GetName())
		}
	}
}

// TestTestRunOldestActiveAgeIgnoresCompletedAndDeleting verifies active-only age calculation.
func TestTestRunOldestActiveAgeIgnoresCompletedAndDeleting(t *testing.T) {
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	deletingAt := now.Add(-time.Minute)
	items := []unstructured.Unstructured{
		newTestRun(now.Add(-2*time.Hour), nil, nil, nil),
		newTestRun(now.Add(-4*time.Hour), timePointer(now.Add(-time.Hour)), nil, nil),
		newTestRun(now.Add(-6*time.Hour), nil, &deletingAt, nil),
	}
	collector := collectLifecycleForTest(t, "active-age", now, items)
	if got := metricValue(t, collector, testRunOldestActiveAge, "active-age"); got != 7200 {
		t.Fatalf("oldest active age = %v, want 7200", got)
	}
	collector = collectLifecycleForTest(t, "no-active-age", now, items[1:])
	if got := metricValue(t, collector, testRunOldestActiveAge, "no-active-age"); got != 0 {
		t.Fatalf("oldest active age without active TestRuns = %v, want 0", got)
	}
}

// TestTestRunOverdueCountsOnlyPastPositiveDeadlines verifies deadline boundary behavior.
func TestTestRunOverdueCountsOnlyPastPositiveDeadlines(t *testing.T) {
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	items := []unstructured.Unstructured{
		newTestRun(now.Add(-time.Minute), nil, nil, nil),
		newTestRun(now.Add(-time.Minute), nil, nil, map[string]any{"deadline": now.Add(time.Minute).Unix()}),
		newTestRun(now.Add(-time.Minute), nil, nil, map[string]any{"deadline": now.Unix() - 1}),
		newTestRun(now.Add(-time.Minute), nil, nil, map[string]any{"deadline": now.Unix()}),
	}
	collector := collectLifecycleForTest(t, "overdue", now, items)
	if got := metricValue(t, collector, testRunOverdue, "overdue"); got != 1 {
		t.Fatalf("overdue count = %v, want 1", got)
	}
}

// TestTestRunWindowCompletenessReflectsRetentionDurations checks retention coverage for each window.
func TestTestRunWindowCompletenessReflectsRetentionDurations(t *testing.T) {
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		retention  map[string]any
		wantOne    float64
		wantTwenty float64
	}{
		{name: "unset retention is kept forever", wantOne: 1, wantTwenty: 1},
		{name: "short retention", retention: map[string]any{"success": "30m"}, wantOne: 0, wantTwenty: 0},
		{name: "long retention", retention: map[string]any{"failure": "48h"}, wantOne: 1, wantTwenty: 1},
		{name: "retention equals short window", retention: map[string]any{"success": "1h"}, wantOne: 1, wantTwenty: 0},
		{name: "unparseable retention", retention: map[string]any{"failure": "later"}, wantOne: 0, wantTwenty: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var spec map[string]any
			if test.retention != nil {
				spec = map[string]any{"retention": test.retention}
			}
			collector := collectLifecycleForTest(t, "retention-"+test.name, now, []unstructured.Unstructured{
				newTestRun(now.Add(-time.Minute), nil, nil, spec),
			})
			reader := collector.Reader.(*inventoryTestReader)
			reader.clusters = []unstructured.Unstructured{attributionObject("Cluster", "test", "sample", "cluster-uid")}
			reader.items[0].SetLabels(map[string]string{clusterLabel: "sample"})
			collector.collect(context.Background())
			scope := "retention-" + test.name
			if got := exactMetricValue(t, collector, testRunWindowComplete, "1h", scope, "test/sample"); got != test.wantOne {
				t.Errorf("window complete 1h = %v, want %v", got, test.wantOne)
			}
			if got := exactMetricValue(t, collector, testRunWindowComplete, "24h", scope, "test/sample"); got != test.wantTwenty {
				t.Errorf("window complete 24h = %v, want %v", got, test.wantTwenty)
			}
		})
	}
}

// TestTestRunWindowCompletenessReflectsClusterRetention verifies the Cluster default retention marks windows
// incomplete even when no retained TestRun carries that retention, and that a failed Cluster list fails closed.
func TestTestRunWindowCompletenessReflectsClusterRetention(t *testing.T) {
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		retention  map[string]any
		failKind   string
		wantOne    float64
		wantTwenty float64
	}{
		{name: "no cluster retention", wantOne: 1, wantTwenty: 1},
		{name: "cluster retention between windows", retention: map[string]any{"success": "2h"}, wantOne: 1, wantTwenty: 0},
		{name: "short cluster retention", retention: map[string]any{"failure": "10m"}, wantOne: 0, wantTwenty: 0},
		{name: "cluster list failure", failKind: "ClusterList", wantOne: 0, wantTwenty: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := map[string]any{}
			if test.retention != nil {
				config["testrunRetention"] = test.retention
			}
			cluster := unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "etos.eiffel-community.github.io/v1alpha1",
				"kind":       "Cluster",
				"metadata":   map[string]any{"name": "cluster-sample", "namespace": "test"},
				"spec":       map[string]any{"etos": map[string]any{"config": config}},
			}}
			scope := "cluster-retention-" + test.name
			collector := &Collector{
				Reader:    &inventoryTestReader{clusters: []unstructured.Unstructured{cluster}, failKind: test.failKind, err: errors.New("list failed")},
				Scope:     scope,
				Now:       func() time.Time { return now },
				leaderCtx: context.Background(),
			}
			collector.collect(context.Background())
			if test.failKind != "" {
				if exactMetricValue(t, collector, snapshotReady, "manager") != 0 || metricCount(collector, testRunWindowComplete) != 0 {
					t.Fatal("failed Cluster inventory must publish no partial domain snapshot")
				}
				return
			}
			if got := exactMetricValue(t, collector, testRunWindowComplete, "1h", scope, "test/cluster-sample"); got != test.wantOne {
				t.Errorf("window complete 1h = %v, want %v", got, test.wantOne)
			}
			if got := exactMetricValue(t, collector, testRunWindowComplete, "24h", scope, "test/cluster-sample"); got != test.wantTwenty {
				t.Errorf("window complete 24h = %v, want %v", got, test.wantTwenty)
			}
			if got := metricValue(t, collector, snapshotReady, scope); got != 1 {
				t.Errorf("snapshot ready = %v, want 1", got)
			}
		})
	}
}

// TestTestRunLifecycleListFailurePreservesLastSnapshot verifies failed collection does not publish partial values.
func TestTestRunLifecycleListFailurePreservesLastSnapshot(t *testing.T) {
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	const scope = "lifecycle-list-failure"
	collector := collectLifecycleForTest(t, scope, now, []unstructured.Unstructured{
		newTestRun(now.Add(-time.Minute), nil, nil, nil),
	})
	if got := metricValue(t, collector, testRunCreated, "1h", scope); got != 1 {
		t.Fatalf("initial created count = %v, want 1", got)
	}

	reader := &inventoryTestReader{failKind: "EnvironmentRequestList", err: errors.New("list failed")}
	collector.Reader = reader
	collector.collect(context.Background())
	if got := metricValue(t, collector, testRunCreated, "1h", scope); got != 1 {
		t.Fatalf("created count after failed collection = %v, want preserved 1", got)
	}
	if got := metricValue(t, collector, snapshotReady, scope); got != 0 {
		t.Fatalf("snapshot ready after failed collection = %v, want 0", got)
	}
}

// inventoryTestReader supplies TestRun lists and can fail a selected resource list.
type inventoryTestReader struct {
	items    []unstructured.Unstructured
	clusters []unstructured.Unstructured
	failKind string
	err      error
	listHook func(context.Context, client.ObjectList) error
}

// Get is unsupported because inventory collection only reads complete lists.
func (r *inventoryTestReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("Get is unsupported")
}

// List returns configured TestRuns and Clusters or the configured list error.
func (r *inventoryTestReader) List(ctx context.Context, list client.ObjectList, _ ...client.ListOption) error {
	if r.listHook != nil {
		return r.listHook(ctx, list)
	}
	unstructuredList, ok := list.(*unstructured.UnstructuredList)
	if !ok {
		return errors.New("expected an unstructured list")
	}
	if unstructuredList.GroupVersionKind().Kind == r.failKind {
		return r.err
	}
	if unstructuredList.GroupVersionKind().Kind == "TestRunList" {
		unstructuredList.Items = append(unstructuredList.Items, r.items...)
	}
	if unstructuredList.GroupVersionKind().Kind == "ClusterList" {
		unstructuredList.Items = append(unstructuredList.Items, r.clusters...)
	}
	return nil
}

// collectLifecycleForTest runs one deterministic collection with an isolated scope.
func collectLifecycleForTest(t *testing.T, scope string, now time.Time, items []unstructured.Unstructured) *Collector {
	t.Helper()
	collector := &Collector{
		Reader:    &inventoryTestReader{items: items},
		Scope:     scope,
		Now:       func() time.Time { return now },
		leaderCtx: context.Background(),
	}
	collector.collect(context.Background())
	return collector
}

// newTestRun constructs a minimally populated unstructured TestRun for inventory metrics tests.
func newTestRun(created time.Time, completed, deleting *time.Time, spec map[string]any) unstructured.Unstructured {
	metadata := map[string]any{
		"creationTimestamp": metav1.NewTime(created).Format(time.RFC3339Nano),
		"namespace":         "test",
		"name":              fmt.Sprintf("testrun-%d", testRunSequence.Add(1)),
	}
	if deleting != nil {
		metadata["deletionTimestamp"] = metav1.NewTime(*deleting).Format(time.RFC3339Nano)
	}
	object := map[string]any{
		"apiVersion": "etos.eiffel-community.github.io/v1beta1",
		"kind":       "TestRun",
		"metadata":   metadata,
		"spec":       map[string]any{},
	}
	if spec != nil {
		object["spec"] = spec
	}
	if completed != nil {
		object["status"] = map[string]any{"completionTime": metav1.NewTime(*completed).Format(time.RFC3339Nano)}
	}
	return unstructured.Unstructured{Object: object}
}

// testRunSequence supplies distinct fixture names even when timestamps overlap.
var testRunSequence atomic.Uint64

// timePointer returns a pointer to a copy of the supplied time.
func timePointer(value time.Time) *time.Time {
	return &value
}

// metricValue reads a labeled const metric; absence is an error, never a synthetic zero.
func metricValue(t *testing.T, collector *Collector, desc *prometheus.Desc, labels ...string) float64 {
	t.Helper()
	switch desc {
	case snapshotReady, snapshotAge, snapshotLastSuccess, collectionFailures:
		return exactMetricValue(t, collector, desc, "manager")
	default:
		// Legacy single-partition tests exercise the explicit unassigned partition.
		labels = append(labels, UnassignedCluster)
	}
	return exactMetricValue(t, collector, desc, labels...)
}

// exactMetricValue reads a metric using its complete canonical dimensions without compatibility defaults.
func exactMetricValue(t *testing.T, collector *Collector, desc *prometheus.Desc, labels ...string) float64 {
	t.Helper()
	want := &dto.Metric{}
	if err := prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, 0, labels...).Write(want); err != nil {
		t.Fatal(err)
	}
	ch := make(chan prometheus.Metric, 1024)
	collector.Collect(ch)
	close(ch)
	for metric := range ch {
		if metric.Desc() != desc {
			continue
		}
		value := &dto.Metric{}
		if err := metric.Write(value); err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(value.Label, want.Label) {
			if value.Counter != nil {
				return value.Counter.GetValue()
			}
			return value.Gauge.GetValue()
		}
	}
	t.Fatalf("missing metric %s with labels %v", desc, labels)
	return 0
}

// metricCount returns the number of series currently exported by a collector.
func metricCount(collector prometheus.Collector, desc *prometheus.Desc) int {
	ch := make(chan prometheus.Metric, 1024)
	collector.Collect(ch)
	close(ch)
	count := 0
	for metric := range ch {
		if desc == nil || metric.Desc() == desc {
			count++
		}
	}
	return count
}

// TestCollectorInitialEmptyAndFreshness checks empty snapshots and freshness boundaries.
func TestCollectorInitialEmptyAndFreshness(t *testing.T) {
	base := time.Date(2026, 10, 8, 11, 0, 0, 0, time.UTC)
	now := base
	c := &Collector{Reader: &inventoryTestReader{}, Now: func() time.Time { return now }}
	if !c.NeedLeaderElection() || metricCount(c, nil) != 0 {
		t.Fatal("followers must require leadership and export no series")
	}
	c.leaderCtx = context.Background()
	if metricValue(t, c, snapshotReady, "cluster") != 0 {
		t.Fatal("initial leader must not be ready")
	}
	for _, desc := range []*prometheus.Desc{snapshotAge, snapshotLastSuccess, testRunCreated, inventoryCounts} {
		if metricCount(c, desc) != 0 {
			t.Fatalf("initial snapshot must omit %s", desc)
		}
	}
	c.collect(context.Background())
	if metricValue(t, c, snapshotReady, "cluster") != 1 || metricValue(t, c, testRunCreated, "1h", "cluster") != 0 {
		t.Fatal("successful empty snapshot must be ready with explicit zero lifecycle counts")
	}
	if metricCount(c, inventoryCounts) != len(resources)*5 {
		t.Fatal("empty partitions must export zero-valued bounded resource states")
	}
	for _, age := range []time.Duration{0, 29 * time.Second, 30 * time.Second, 30*time.Second + time.Nanosecond} {
		now = base.Add(age)
		want := boolToFloat(age <= 30*time.Second)
		if got := metricValue(t, c, snapshotReady, "cluster"); got != want {
			t.Fatalf("ready at age %s = %v, want %v", age, got, want)
		}
		if got := metricValue(t, c, snapshotAge, "cluster"); got != age.Seconds() {
			t.Fatalf("age = %v, want %v", got, age.Seconds())
		}
		if got := metricValue(t, c, snapshotLastSuccess, "cluster"); got != float64(base.Unix()) {
			t.Fatalf("last success changed to %v", got)
		}
	}
	now = base.Add(-time.Second)
	if metricValue(t, c, snapshotAge, "cluster") != 0 {
		t.Fatal("clock rollback must not export negative age")
	}
}

// TestCollectorFailurePreservesTimestampAndRecovers checks failed reads and recovery.
func TestCollectorFailurePreservesTimestampAndRecovers(t *testing.T) {
	base := time.Date(2026, 10, 8, 11, 0, 0, 0, time.UTC)
	now := base
	c := collectLifecycleForTest(t, "cluster", base, []unstructured.Unstructured{newTestRun(base, nil, nil, nil)})
	c.Now = func() time.Time { return now }
	before := c.snapshot
	now = base.Add(10 * time.Second)
	c.Reader = &inventoryTestReader{failKind: "EnvironmentRequestList", err: errors.New("list failed")}
	c.collect(context.Background())
	if c.snapshot != before || metricValue(t, c, snapshotLastSuccess, "cluster") != float64(base.Unix()) {
		t.Fatal("failure must preserve the immutable snapshot and last success")
	}
	if metricValue(t, c, snapshotReady, "cluster") != 0 || metricValue(t, c, snapshotAge, "cluster") != 10 ||
		metricValue(t, c, collectionFailures, "cluster") != 1 || metricValue(t, c, testRunCreated, "1h", "cluster") != 1 {
		t.Fatal("failure must be visible, not published as zero or ready")
	}
	c.Reader = &inventoryTestReader{}
	c.collect(context.Background())
	if metricValue(t, c, snapshotLastSuccess, "cluster") != float64(now.Unix()) ||
		metricValue(t, c, snapshotReady, "cluster") != 1 || metricValue(t, c, testRunCreated, "1h", "cluster") != 0 {
		t.Fatal("successful recovery must replace the snapshot and advance last success")
	}
}

type contextReader struct {
	inventoryTestReader
	entered chan context.Context
	release chan struct{}
}

// List blocks until cancellation or release to simulate slow and noncompliant readers.
func (r *contextReader) List(ctx context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	if r.entered != nil {
		select {
		case r.entered <- ctx:
		default:
		}
	}
	if r.release != nil {
		<-r.release
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

// TestCollectorTimeoutAndCancellation checks bounded reads and parent cancellation.
func TestCollectorTimeoutAndCancellation(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelParent), func(t *testing.T) {
			c := collectLifecycleForTest(t, "cluster", time.Now(), nil)
			reader := &contextReader{entered: make(chan context.Context, 1)}
			c.Reader, c.Timeout = reader, 25*time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() { c.collect(ctx); close(done) }()
			readCtx := <-reader.entered
			if _, ok := readCtx.Deadline(); !ok {
				t.Fatal("reader must receive a bounded context")
			}
			if cancelParent {
				cancel()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("compliant reader did not stop")
			}
			if metricValue(t, c, snapshotReady, "cluster") != 0 || metricValue(t, c, collectionFailures, "cluster") != 1 {
				t.Fatal("timeout/cancellation must mark collection failed")
			}
		})
	}
}

// TestCollectorUnfinishedReadExpiresWithoutOrphaningWorker checks freshness during blocked reads.
func TestCollectorUnfinishedReadExpiresWithoutOrphaningWorker(t *testing.T) {
	base := time.Now()
	var elapsed atomic.Int64
	c := collectLifecycleForTest(t, "cluster", base, nil)
	c.Now = func() time.Time { return base.Add(time.Duration(elapsed.Load())) }
	c.MaxAge, c.Timeout = time.Second, time.Hour
	reader := &contextReader{entered: make(chan context.Context, 1), release: make(chan struct{})}
	c.Reader = reader
	done := make(chan struct{})
	go func() { c.collect(context.Background()); close(done) }()
	var release sync.Once
	t.Cleanup(func() {
		release.Do(func() { close(reader.release) })
		<-done
	})
	readCtx := <-reader.entered
	if metricValue(t, c, snapshotReady, "cluster") != 1 {
		t.Fatal("in-flight collection must not immediately invalidate fresh prior success")
	}
	elapsed.Store(int64(time.Second + time.Nanosecond))
	if metricValue(t, c, snapshotReady, "cluster") != 0 {
		t.Fatal("unfinished collection must not remain ready beyond MaxAge")
	}
	c.mu.RLock()
	cancelAttempt := c.attemptCtx
	c.mu.RUnlock()
	if cancelAttempt != readCtx {
		t.Fatal("scrape must evaluate the worker's actual context")
	}
	release.Do(func() { close(reader.release) })
	<-done
}

// TestCollectorTimeoutDoesNotAbandonNoncompliantReader checks expired reads cannot publish success.
func TestCollectorTimeoutDoesNotAbandonNoncompliantReader(t *testing.T) {
	base := time.Date(2026, 10, 8, 11, 0, 0, 0, time.UTC)
	c := collectLifecycleForTest(t, "cluster", base, nil)
	before := c.snapshot
	reader := &contextReader{entered: make(chan context.Context, 1), release: make(chan struct{})}
	c.Reader, c.Timeout = reader, 25*time.Millisecond
	done := make(chan struct{})
	go func() { c.collect(context.Background()); close(done) }()
	var release sync.Once
	t.Cleanup(func() {
		release.Do(func() { close(reader.release) })
		<-done
	})
	readCtx := <-reader.entered
	<-readCtx.Done()
	if metricValue(t, c, snapshotReady, "cluster") != 0 {
		t.Fatal("deadline must make a still-fresh snapshot unready even when reader ignores cancellation")
	}
	select {
	case <-done:
		t.Fatal("collection must not abandon a noncompliant reader in an orphan goroutine")
	default:
	}
	release.Do(func() { close(reader.release) })
	<-done
	if c.snapshot != before || metricValue(t, c, collectionFailures, "cluster") != 1 {
		t.Fatal("expired read returning nil must fail without replacing last success")
	}
}

// TestCollectorLeaderLifecycleAndRegistration checks registration and leader-only publication.
func TestCollectorLeaderLifecycleAndRegistration(t *testing.T) {
	c := &Collector{Reader: &contextReader{entered: make(chan context.Context, 1)}, Interval: time.Hour}
	registry := prometheus.NewPedanticRegistry()
	if err := c.Register(registry); err != nil {
		t.Fatal(err)
	}
	if err := c.Register(registry); err != nil {
		t.Fatalf("same instance registration must be idempotent: %v", err)
	}
	if err := (&Collector{}).Register(registry); err == nil {
		t.Fatal("different instance registration conflict must return an error")
	}
	families, err := registry.Gather()
	if err != nil || len(families) != 0 {
		t.Fatalf("registered follower exported inventory: %v %v", families, err)
	}
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- c.Start(ctx) }()
		<-c.Reader.(*contextReader).entered
		if metricValue(t, c, snapshotReady, "cluster") != 0 {
			t.Fatal("new leader without snapshot must be unready")
		}
		if err := c.Start(ctx); err == nil {
			t.Fatal("duplicate worker start must fail")
		}
		cancel()
		if metricCount(c, nil) != 0 {
			t.Fatal("canceled leadership must immediately suppress inventory series")
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if metricCount(c, nil) != 0 {
			t.Fatal("stopped collector must export no series")
		}
	}
	if err := (&Collector{}).Start(context.Background()); err == nil {
		t.Fatal("missing reader must fail setup")
	}
}

// TestCollectorConcurrentScrapesAndIndependentPublication checks snapshot isolation under concurrency.
func TestCollectorConcurrentScrapesAndIndependentPublication(t *testing.T) {
	base := time.Date(2026, 10, 8, 11, 0, 0, 0, time.UTC)
	first := collectLifecycleForTest(t, "cluster", base, []unstructured.Unstructured{newTestRun(base, nil, nil, nil)})
	second := collectLifecycleForTest(t, "cluster", base, nil)
	registry := prometheus.NewPedanticRegistry()
	if err := first.Register(registry); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 100 {
			first.collect(context.Background())
			second.collect(context.Background())
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 100 {
				if _, err := registry.Gather(); err != nil {
					t.Errorf("gather immutable snapshot: %v", err)
				}
				if metricValue(t, first, testRunCreated, "1h", "cluster") != 1 ||
					metricValue(t, second, testRunCreated, "1h", "cluster") != 0 {
					t.Error("per-instance snapshots contaminated each other")
				}
			}
		})
	}
	wg.Wait()
}

// TestCollectorLastSuccessUsesCompletionTimeAndRejectsExpiredRead checks successful completion timestamps.
func TestCollectorLastSuccessUsesCompletionTimeAndRejectsExpiredRead(t *testing.T) {
	base := time.Date(2026, 10, 8, 11, 0, 0, 0, time.UTC)
	now := base
	c := &Collector{
		Reader: &inventoryTestReader{listHook: func(_ context.Context, list client.ObjectList) error {
			if list.GetObjectKind().GroupVersionKind().Kind == "ClusterList" {
				now = base.Add(4 * time.Second)
			}
			return nil
		}},
		Now: func() time.Time { return now }, leaderCtx: context.Background(),
	}
	c.collect(context.Background())
	if got := metricValue(t, c, snapshotLastSuccess, "cluster"); got != float64(base.Unix()+4) {
		t.Fatalf("last success must be completion time, got %v", got)
	}
	before := c.snapshot
	c.Reader = &inventoryTestReader{listHook: func(ctx context.Context, _ client.ObjectList) error {
		<-ctx.Done()
		return nil // A misbehaving reader returning nil after expiry cannot publish success.
	}}
	c.Timeout = time.Millisecond
	c.collect(context.Background())
	if c.snapshot != before || metricValue(t, c, snapshotReady, "cluster") != 0 {
		t.Fatal("an expired read returning nil must preserve prior snapshot and fail readiness")
	}
}

// TestCollectorScrapeUsesExactlyOnePublishedSnapshot checks atomic publication during a scrape.
func TestCollectorScrapeUsesExactlyOnePublishedSnapshot(t *testing.T) {
	base := time.Date(2026, 10, 8, 11, 0, 0, 0, time.UTC)
	c := collectLifecycleForTest(t, "cluster", base, []unstructured.Unstructured{newTestRun(base, nil, nil, nil)})
	ch := make(chan prometheus.Metric)
	done := make(chan struct{})
	go func() { c.Collect(ch); close(ch); close(done) }()
	t.Cleanup(func() {
		for range ch {
		}
		<-done
	})
	first := <-ch // Collect has loaded the old snapshot and started emitting.
	if first.Desc() != snapshotAge {
		t.Fatal("scrape did not emit snapshot age first")
	}
	c.Reader = &inventoryTestReader{}
	c.collect(context.Background())
	oldCreated := false
	for metric := range ch {
		value := &dto.Metric{}
		if err := metric.Write(value); err != nil {
			t.Fatal(err)
		}
		if metric.Desc() == testRunCreated && value.Gauge.GetValue() == 1 {
			oldCreated = true
		}
	}
	<-done
	if !oldCreated || metricValue(t, c, testRunCreated, "1h", "cluster") != 0 {
		t.Fatal("in-flight scrape must finish the old snapshot while next scrape observes new snapshot")
	}
}

// TestCollectorMetricSourceCompatibility checks stable metric names, types, and dimensions.
func TestCollectorMetricSourceCompatibility(t *testing.T) {
	base := time.Date(2026, 10, 8, 11, 0, 0, 0, time.UTC)
	c := collectLifecycleForTest(t, "cluster", base, []unstructured.Unstructured{
		newTestRun(base.Add(-time.Minute), &base, nil, nil),
	})
	registry := prometheus.NewPedanticRegistry()
	if err := c.Register(registry); err != nil {
		t.Fatal(err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	wantLabels := map[string][]string{
		"etos_inventory_resources":                               {"etos_cluster", "resource_type", "scope", "state"},
		"etos_inventory_snapshot_ready":                          {"scope"},
		"etos_inventory_snapshot_age_seconds":                    {"scope"},
		"etos_inventory_snapshot_last_success_timestamp_seconds": {"scope"},
		"etos_inventory_collection_failures_total":               {"scope"},
		"etos_inventory_testrun_phase":                           {"etos_cluster", "phase", "scope"},
		"etos_inventory_testrun_verdict":                         {"etos_cluster", "scope", "verdict"},
		"etos_inventory_testrun_operational_conclusion":          {"etos_cluster", "operational_conclusion", "scope"},
		"etos_inventory_testrun_created":                         {"etos_cluster", "scope", "window"},
		"etos_inventory_testrun_oldest_active_age_seconds":       {"etos_cluster", "scope"},
		"etos_inventory_testrun_overdue":                         {"etos_cluster", "scope"},
		"etos_inventory_testrun_window_complete":                 {"etos_cluster", "scope", "window"},
	}

	if len(families) != len(wantLabels) {
		t.Fatalf("metric families = %d, want %d", len(families), len(wantLabels))
	}
	for _, family := range families {
		wantType := dto.MetricType_GAUGE
		if family.GetName() == "etos_inventory_collection_failures_total" {
			wantType = dto.MetricType_COUNTER
		}
		if family.GetType() != wantType {
			t.Errorf("%s type = %v, want %v", family.GetName(), family.GetType(), wantType)
		}
		for _, metric := range family.Metric {
			labels := make([]string, 0, len(metric.Label))
			for _, label := range metric.Label {
				labels = append(labels, label.GetName())
			}
			if !reflect.DeepEqual(labels, wantLabels[family.GetName()]) {
				t.Errorf("%s labels = %v, want %v", family.GetName(), labels, wantLabels[family.GetName()])
			}
		}
	}
}
