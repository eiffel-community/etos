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
	"fmt"
	"time"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// OutcomeSuccess identifies a successfully completed resource.
	OutcomeSuccess = "success"
	// OutcomeFailure identifies a resource that completed unsuccessfully.
	OutcomeFailure = "failure"
	// OutcomeAborted identifies a TestRun that was explicitly aborted.
	OutcomeAborted = "aborted"
	// OutcomeTimedOut identifies a TestRun that exceeded its deadline.
	OutcomeTimedOut = "timed_out"
	// OutcomeInconclusive identifies a TestRun with an inconclusive operational result.
	OutcomeInconclusive = "inconclusive"
	// OutcomeUnknown identifies a TestRun whose persisted conditions have no recognized conclusion.
	OutcomeUnknown = "unknown"

	testRunFinishedName            = "etos_testrun_finished_total"
	testRunDurationName            = "etos_testrun_duration_seconds"
	environmentRequestFinishedName = "etos_environment_request_finished_total"
	environmentRequestDurationName = "etos_environment_request_duration_seconds"
	outcomeAttributeName           = "outcome"
)

var (
	durationBucketBounds = []float64{
		0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1800, 3600, 21600, 86400,
	}
	testRunOutcomes            = []string{OutcomeSuccess, OutcomeFailure, OutcomeAborted, OutcomeTimedOut, OutcomeInconclusive, OutcomeUnknown}
	environmentRequestOutcomes = []string{OutcomeSuccess, OutcomeFailure}
)

// Transitions records successfully persisted terminal events through native OTel instruments.
type Transitions struct {
	testRunFinished            metric.Int64Counter
	testRunDuration            metric.Float64Histogram
	environmentRequestFinished metric.Int64Counter
	environmentRequestDuration metric.Float64Histogram
}

// ClusterTransform initializes baseline children synchronously on cache ingestion without modifying objects.
// Unlike asynchronous informer callbacks, initial list transforms complete before cache synchronization
// permits controller workers to run. Watch transforms also initialize newly discovered Clusters.
func (m *Transitions) ClusterTransform(object any) (any, error) {
	if cluster, ok := object.(*etosv1alpha1.Cluster); ok && cluster.Namespace != "" && cluster.Name != "" {
		m.InitializeCluster(cluster.Namespace + "/" + cluster.Name)
	}
	return object, nil
}

// NewTransitions creates terminal transition collectors with zero-valued bounded series.
func NewTransitions(meter metric.Meter) (*Transitions, error) {
	transitions := &Transitions{}
	var err error
	transitions.testRunFinished, err = meter.Int64Counter(testRunFinishedName, metric.WithUnit("1"),
		metric.WithDescription("Number of TestRuns whose terminal status was persisted"))
	if err != nil {
		return nil, fmt.Errorf("create TestRun counter: %w", err)
	}
	transitions.testRunDuration, err = meter.Float64Histogram(testRunDurationName, metric.WithUnit("s"),
		metric.WithDescription("Elapsed time from TestRun start to persisted terminal status"))
	if err != nil {
		return nil, fmt.Errorf("create TestRun histogram: %w", err)
	}
	transitions.environmentRequestFinished, err = meter.Int64Counter(environmentRequestFinishedName, metric.WithUnit("1"),
		metric.WithDescription("Number of EnvironmentRequests whose provider Job reached a persisted terminal status"))
	if err != nil {
		return nil, fmt.Errorf("create EnvironmentRequest counter: %w", err)
	}
	transitions.environmentRequestDuration, err = meter.Float64Histogram(environmentRequestDurationName, metric.WithUnit("s"),
		metric.WithDescription("Elapsed time from EnvironmentRequest creation to provider Job completion"))
	if err != nil {
		return nil, fmt.Errorf("create EnvironmentRequest histogram: %w", err)
	}
	transitions.InitializeCluster(UnassignedCluster)
	return transitions, nil
}

// InitializeCluster creates bounded zero children without observing fake durations.
// The Cluster cache informer calls this before controllers start and on Cluster discovery.
// Historical transition series remain after Cluster deletion for cumulative counter continuity.
func (m *Transitions) InitializeCluster(cluster string) {
	// Native histograms are absent until the first real observation.
	for _, outcome := range testRunOutcomes {
		m.testRunFinished.Add(context.Background(), 0, transitionAttributes(cluster, outcome))
	}
	for _, outcome := range environmentRequestOutcomes {
		m.environmentRequestFinished.Add(context.Background(), 0, transitionAttributes(cluster, outcome))
	}
}

// RecordTestRunFinished records one persisted terminal TestRun status and, when timestamps are valid, its duration.
func (m *Transitions) RecordTestRunFinished(ctx context.Context, cluster, outcome string, startedAt, completedAt time.Time) {
	result := boundedOutcome(outcome)
	m.testRunFinished.Add(ctx, 1, transitionAttributes(cluster, result))
	if seconds, ok := elapsedSeconds(startedAt, completedAt); ok {
		m.testRunDuration.Record(ctx, seconds, transitionAttributes(cluster, result))
	}
}

// RecordEnvironmentRequestFinished records a persisted provider Job result and its valid elapsed duration.
func (m *Transitions) RecordEnvironmentRequestFinished(
	ctx context.Context,
	cluster, outcome string,
	createdAt time.Time,
	completedAt time.Time,
) {
	result := boundedEnvironmentRequestOutcome(outcome)
	m.environmentRequestFinished.Add(ctx, 1, transitionAttributes(cluster, result))
	seconds, ok := elapsedSeconds(createdAt, completedAt)
	if !ok {
		return
	}
	m.environmentRequestDuration.Record(ctx, seconds, transitionAttributes(cluster, result))
}

// transitionAttributes builds the cluster and outcome attributes shared by transition measurements.
func transitionAttributes(cluster, outcome string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String(clusterAttributeName, cluster), attribute.String(outcomeAttributeName, outcome))
}

// boundedOutcome maps any unrecognized outcome to unknown so metric cardinality stays fixed.
func boundedOutcome(outcome string) string {
	switch outcome {
	case OutcomeSuccess, OutcomeFailure, OutcomeAborted, OutcomeTimedOut, OutcomeInconclusive, OutcomeUnknown:
		return outcome
	default:
		return OutcomeUnknown
	}
}

// boundedEnvironmentRequestOutcome limits provider provisioning results to success or failure.
func boundedEnvironmentRequestOutcome(outcome string) string {
	if outcome == OutcomeSuccess {
		return OutcomeSuccess
	}
	return OutcomeFailure
}

// elapsedSeconds returns a non-negative duration only when both timestamps are present and ordered.
func elapsedSeconds(startedAt, completedAt time.Time) (float64, bool) {
	if startedAt.IsZero() || completedAt.IsZero() || completedAt.Before(startedAt) {
		return 0, false
	}
	return completedAt.Sub(startedAt).Seconds(), true
}
