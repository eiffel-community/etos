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
	"math"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	conditionActive = "Active"

	stateActive    = "active"
	stateReady     = "ready"
	stateCompleted = "completed"
	stateDeleting  = "deleting"
	stateFailed    = "failed"

	phaseAccepted             = "accepted"
	phaseEnvironmentProvision = "environment_provisioning"
	phaseEnvironmentReady     = "environment_ready"
	phaseSuiteRunnerStarting  = "suite_runner_starting"
	phaseRunning              = "running"
	phaseCompleting           = "completing"
	phaseCompleted            = "completed"
	phaseFailed               = "failed"
	phaseTimedOut             = "timed_out"
	phaseAborted              = "aborted"
	phaseCleaningUp           = "cleaning_up"
	phaseCleanupFailed        = "cleanup_failed"

	verdictPassed       = "passed"
	verdictFailed       = "failed"
	verdictInconclusive = "inconclusive"
	verdictNone         = "none"
	verdictUnknown      = "unknown"

	conclusionSuccessful   = "successful"
	conclusionFailed       = "failed"
	conclusionTimedOut     = "timed_out"
	conclusionAborted      = "aborted"
	conclusionInconclusive = "inconclusive"
	conclusionRunning      = "running"
	conclusionUnknown      = "unknown"
)

var testRunPhases = []string{
	phaseAccepted,
	phaseEnvironmentProvision,
	phaseEnvironmentReady,
	phaseSuiteRunnerStarting,
	phaseRunning,
	phaseCompleting,
	phaseCompleted,
	phaseFailed,
	phaseTimedOut,
	phaseAborted,
	phaseCleaningUp,
	phaseCleanupFailed,
}

var testRunVerdicts = []string{
	verdictPassed,
	verdictFailed,
	verdictInconclusive,
	verdictNone,
	verdictUnknown,
}

var testRunOperationalConclusions = []string{
	conclusionSuccessful,
	conclusionFailed,
	conclusionTimedOut,
	conclusionAborted,
	conclusionInconclusive,
	conclusionRunning,
	conclusionUnknown,
}

var inventoryWindows = []struct {
	label string
	d     time.Duration
}{
	{label: "1h", d: time.Hour},
	{label: "24h", d: 24 * time.Hour},
}

var (
	inventoryCounts              = prometheus.NewDesc("etos_inventory_resources", "Current unique ETOS resources reconstructed from the manager cache.", []string{"resource_type", "state", "scope", clusterAttributeName}, nil)
	snapshotReady                = prometheus.NewDesc("etos_inventory_snapshot_ready", "Whether the most recent complete ETOS inventory snapshot is fresh.", []string{"scope"}, nil)
	snapshotAge                  = prometheus.NewDesc("etos_inventory_snapshot_age_seconds", "Age of the most recent complete ETOS inventory snapshot.", []string{"scope"}, nil)
	snapshotLastSuccess          = prometheus.NewDesc("etos_inventory_snapshot_last_success_timestamp_seconds", "Unix timestamp of the most recent successful complete ETOS inventory collection.", []string{"scope"}, nil)
	collectionFailures           = prometheus.NewDesc("etos_inventory_collection_failures_total", "ETOS inventory snapshot collection failures.", []string{"scope"}, nil)
	testRunPhase                 = prometheus.NewDesc("etos_inventory_testrun_phase", "Current TestRun lifecycle phase reconstructed from persisted manager state.", []string{"phase", "scope", clusterAttributeName}, nil)
	testRunVerdict               = prometheus.NewDesc("etos_inventory_testrun_verdict", "Current TestRun verdict reconstructed from persisted manager state.", []string{"verdict", "scope", clusterAttributeName}, nil)
	testRunOperationalConclusion = prometheus.NewDesc("etos_inventory_testrun_operational_conclusion", "Current TestRun operational conclusion reconstructed from persisted manager state.", []string{"operational_conclusion", "scope", clusterAttributeName}, nil)
	testRunCreated               = prometheus.NewDesc("etos_inventory_testrun_created", "TestRuns created in the window, derived from retained TestRun resources; counts are a lower bound when window_complete is 0.", []string{"window", "scope", clusterAttributeName}, nil)
	testRunOldestActiveAge       = prometheus.NewDesc("etos_inventory_testrun_oldest_active_age_seconds", "Age of the oldest active TestRun, derived from retained TestRun resources.", []string{"scope", clusterAttributeName}, nil)
	testRunOverdue               = prometheus.NewDesc("etos_inventory_testrun_overdue", "Active TestRuns past their configured deadline, derived from retained TestRun resources.", []string{"scope", clusterAttributeName}, nil)
	testRunWindowComplete        = prometheus.NewDesc("etos_inventory_testrun_window_complete", "Whether configured retention covers the retained TestRun creation window; 0 means created counts are lower bounds. This is not controller transition metric completeness.", []string{"window", "scope", clusterAttributeName}, nil)
)

type resourceSpec struct {
	name         string
	resourceType string
	group        string
	version      string
	kind         string
}

var resources = []resourceSpec{
	{name: "testrun", resourceType: "testrun", group: "etos.eiffel-community.github.io", version: "v1beta1", kind: "TestRun"},
	{name: "environmentrequest", resourceType: "environmentrequest", group: "etos.eiffel-community.github.io", version: "v1alpha1", kind: "EnvironmentRequest"},
	{name: "environment", resourceType: "environment", group: "etos.eiffel-community.github.io", version: "v1beta1", kind: "Environment"},
	{name: "executionspace", resourceType: "executionspace", group: "etos.eiffel-community.github.io", version: "v1alpha2", kind: "ExecutionSpace"},
	{name: "iut", resourceType: "iut", group: "etos.eiffel-community.github.io", version: "v1alpha2", kind: "Iut"},
	{name: "logarea", resourceType: "logarea", group: "etos.eiffel-community.github.io", version: "v1alpha2", kind: "LogArea"},
}

type inventorySnapshot struct {
	metrics     []prometheus.Metric
	lastSuccess time.Time
}

// Collector periodically publishes immutable, absolute inventory snapshots.
// Configure fields (including a concurrency-safe Now function) before registration or Start.
// Reader must honor context cancellation; collection does not spawn goroutines to abandon reads.
type Collector struct {
	Reader   client.Reader
	Interval time.Duration
	// Scope labels domain snapshots only; snapshot health always has scope="manager".
	Scope   string
	MaxAge  time.Duration
	Now     func() time.Time
	Timeout time.Duration

	mu             sync.RWMutex
	snapshot       *inventorySnapshot
	leaderCtx      context.Context
	attemptCtx     context.Context
	attemptStarted time.Time
	failed         bool
	failures       float64
	unassigned     map[string]int
}

// Register registers this instance; repeating registration of the same instance is harmless.
// A different collector already owning these metric names remains an error.
func (c *Collector) Register(registry prometheus.Registerer) error {
	err := registry.Register(c)
	var already prometheus.AlreadyRegisteredError
	if errors.As(err, &already) && already.ExistingCollector == c {
		return nil
	}
	return err
}

// interval returns the configured collection interval or its default.
func (c *Collector) interval() time.Duration {
	if c.Interval <= 0 {
		return 15 * time.Second
	}
	return c.Interval
}

// scope returns the configured inventory scope or the cluster default.
func (c *Collector) scope() string {
	if c.Scope == "" {
		return "cluster"
	}
	return c.Scope
}

// maxAge returns the freshness limit, defaulting to two collection intervals.
func (c *Collector) maxAge() time.Duration {
	if c.MaxAge <= 0 {
		return 2 * c.interval()
	}
	return c.MaxAge
}

// now reads the configured clock or the system clock.
func (c *Collector) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Describe exports the stable metric schema even while this instance is a follower.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{
		inventoryCounts, snapshotReady, snapshotAge, snapshotLastSuccess, collectionFailures,
		testRunPhase, testRunVerdict, testRunOperationalConclusion, testRunCreated,
		testRunOldestActiveAge, testRunOverdue, testRunWindowComplete,
	} {
		ch <- desc
	}
}

// Collect loads one immutable snapshot without listing resources, and evaluates freshness at scrape time.
// Followers and canceled leadership contexts export no inventory series, including readiness.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	snapshot, leaderCtx, attemptCtx := c.snapshot, c.leaderCtx, c.attemptCtx
	attemptStarted, failed, failures := c.attemptStarted, c.failed, c.failures
	c.mu.RUnlock()
	if leaderCtx == nil || leaderCtx.Err() != nil {
		return
	}
	now, scope := c.now(), "manager"
	ready := false
	if snapshot != nil {
		age := now.Sub(snapshot.lastSuccess)
		ready = !failed && age <= c.maxAge()
		if attemptCtx != nil && (attemptCtx.Err() != nil || now.Sub(attemptStarted) > c.maxAge()) {
			ready = false
		}
		ch <- prometheus.MustNewConstMetric(snapshotAge, prometheus.GaugeValue, math.Max(0, age.Seconds()), scope)
		ch <- prometheus.MustNewConstMetric(snapshotLastSuccess, prometheus.GaugeValue, float64(snapshot.lastSuccess.UnixNano())/1e9, scope)
		for _, metric := range snapshot.metrics {
			ch <- metric
		}
	}
	ch <- prometheus.MustNewConstMetric(snapshotReady, prometheus.GaugeValue, boolToFloat(ready), scope)
	ch <- prometheus.MustNewConstMetric(collectionFailures, prometheus.CounterValue, failures, scope)
}

// Start collects after manager leadership is granted, or immediately when election is disabled.
// Registration is separate so setup errors are returned before starting the manager.
func (c *Collector) Start(ctx context.Context) error {
	if c.Reader == nil {
		return errors.New("inventory reader is required")
	}
	c.mu.Lock()
	if c.leaderCtx != nil {
		c.mu.Unlock()
		return errors.New("inventory collector is already running")
	}
	c.leaderCtx = ctx
	c.snapshot, c.failed = nil, false
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.leaderCtx = nil
		c.mu.Unlock()
	}()
	ticker := time.NewTicker(c.interval())
	defer ticker.Stop()
	c.collect(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

// NeedLeaderElection selects the manager's leader-election runnable group.
// When manager election is disabled, this group starts without acquiring a lease.
func (c *Collector) NeedLeaderElection() bool {
	return true
}

// collect builds a complete inventory and lifecycle snapshot before atomically publishing it.
func (c *Collector) collect(ctx context.Context) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	now := c.now()
	c.mu.Lock()
	c.attemptCtx, c.attemptStarted = ctx, now
	c.mu.Unlock()
	success := false
	defer func() {
		c.mu.Lock()
		c.attemptCtx = nil
		c.failed = !success
		if !success {
			c.failures++
		}
		c.mu.Unlock()
	}()
	objects := make(map[string]*unstructured.Unstructured)
	clusters := &unstructured.UnstructuredList{}
	clusters.SetGroupVersionKind(schema.GroupVersionKind{Group: etosGroup, Version: "v1alpha1", Kind: "ClusterList"})
	if err := c.Reader.List(ctx, clusters); err != nil || ctx.Err() != nil {
		return
	}
	partitions := map[string]*clusterInventory{UnassignedCluster: newClusterInventory()}
	for _, cluster := range clusters.Items {
		objects[clusterObjectKey("Cluster", &cluster)] = cluster.DeepCopy()
		identity := cluster.GetNamespace() + "/" + cluster.GetName()
		partitions[identity] = newClusterInventory()
		for _, window := range inventoryWindows {
			partitions[identity].complete[window.label] = !hasRetentionShorterThan(cluster, window.d, "spec", "etos", "config", "testrunRetention")
		}
	}
	// Unknown ownership cannot establish retention guarantees from an arbitrary Cluster.
	for _, window := range inventoryWindows {
		partitions[UnassignedCluster].complete[window.label] = false
	}
	for _, spec := range resources {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   spec.group,
			Version: spec.version,
			Kind:    spec.kind + "List",
		})
		if err := c.Reader.List(ctx, list); err != nil || ctx.Err() != nil {
			return
		}

		for _, item := range list.Items {
			item.SetKind(spec.kind)
			objects[clusterObjectKey(spec.kind, &item)] = item.DeepCopy()
		}
	}
	lookup := func(kind, namespace, name string) (*unstructured.Unstructured, bool) {
		item, ok := objects[kind+"/"+namespace+"/"+name]
		return item, ok
	}
	unassigned := make(map[string]int)
	for _, item := range objects {
		if item.GetKind() == "Cluster" {
			continue
		}
		identity := clusterIdentity(item, lookup, make(map[string]bool))
		partition := partitions[identity]
		if partition == nil {
			partition = partitions[UnassignedCluster]
			identity = UnassignedCluster
		}
		if identity == UnassignedCluster {
			unassigned[item.GetKind()]++
		}
		partition.states[item.GetKind()][classify(*item)]++
		if item.GetKind() == "TestRun" {
			partition.phases[classifyTestRunPhase(*item)]++
			partition.verdicts[classifyTestRunVerdict(*item)]++
			partition.conclusions[classifyTestRunOperationalConclusion(*item)]++
			collectTestRunLifecycle(*item, now, partition.created, partition.complete, &partition.oldestAge, &partition.overdue)
		}
	}
	if ctx.Err() != nil {
		return
	}
	scope := c.scope()
	var metrics []prometheus.Metric
	add := func(desc *prometheus.Desc, value float64, labels ...string) {
		metrics = append(metrics, prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labels...))
	}
	for identity, partition := range partitions {
		for _, spec := range resources {
			for _, state := range []string{stateActive, stateReady, stateCompleted, stateDeleting, stateFailed} {
				add(inventoryCounts, partition.states[spec.kind][state], spec.resourceType, state, scope, identity)
			}
		}
		for _, phase := range testRunPhases {
			add(testRunPhase, partition.phases[phase], phase, scope, identity)
		}
		for _, verdict := range testRunVerdicts {
			add(testRunVerdict, partition.verdicts[verdict], verdict, scope, identity)
		}
		for _, conclusion := range testRunOperationalConclusions {
			add(testRunOperationalConclusion, partition.conclusions[conclusion], conclusion, scope, identity)
		}
		for _, window := range inventoryWindows {
			add(testRunCreated, partition.created[window.label], window.label, scope, identity)
			add(testRunWindowComplete, boolToFloat(partition.complete[window.label]), window.label, scope, identity)
		}
		add(testRunOldestActiveAge, partition.oldestAge, scope, identity)
		add(testRunOverdue, partition.overdue, scope, identity)
	}
	c.mu.Lock()
	if ctx.Err() == nil {
		c.snapshot = &inventorySnapshot{metrics: metrics, lastSuccess: c.now()}
		c.failed = false
		success = true
		if !reflect.DeepEqual(c.unassigned, unassigned) && len(unassigned) > 0 {
			log.FromContext(ctx).Info("Could not attribute inventory resources to ETOS Cluster", "resourceCounts", unassigned)
		}
		c.unassigned = unassigned
	}
	c.mu.Unlock()
}

// clusterInventory holds one absolute snapshot partition, including empty bounded series.
type clusterInventory struct {
	states                                 map[string]map[string]float64
	phases, verdicts, conclusions, created map[string]float64
	complete                               map[string]bool
	oldestAge, overdue                     float64
}

// newClusterInventory creates an independent partition with known window defaults.
func newClusterInventory() *clusterInventory {
	partition := &clusterInventory{
		states: make(map[string]map[string]float64),
		phases: make(map[string]float64), verdicts: make(map[string]float64),
		conclusions: make(map[string]float64), created: make(map[string]float64),
		complete: make(map[string]bool),
	}
	for _, spec := range resources {
		partition.states[spec.kind] = make(map[string]float64)
	}
	for _, window := range inventoryWindows {
		partition.complete[window.label] = true
	}
	return partition
}

// collectTestRunLifecycle derives retained creation compatibility and current active gauges.
// Completion history is emitted by controllers, not reconstructed from retained objects.
func collectTestRunLifecycle(item unstructured.Unstructured, now time.Time, created map[string]float64, complete map[string]bool, oldestActiveAge, overdueCount *float64) {
	creationTime := item.GetCreationTimestamp().Time
	completionValue, hasCompletionTime, _ := unstructured.NestedString(item.Object, "status", "completionTime")
	hasCompletionTime = hasCompletionTime && completionValue != ""
	deletionTime := item.GetDeletionTimestamp()
	active := !hasCompletionTime && deletionTime == nil
	if active {
		if !creationTime.IsZero() {
			age := now.Sub(creationTime).Seconds()
			if age > *oldestActiveAge {
				*oldestActiveAge = age
			}
		}
		deadline, found, _ := unstructured.NestedInt64(item.Object, "spec", "deadline")
		if found && deadline > 0 && now.Unix() > deadline {
			*overdueCount = *overdueCount + 1
		}
	}

	for _, window := range inventoryWindows {
		if !creationTime.IsZero() && timestampInWindow(creationTime, now, window.d) {
			created[window.label]++
		}
		if hasRetentionShorterThan(item, window.d, "spec", "retention") {
			complete[window.label] = false
		}
	}
}

// timestampInWindow reports whether timestamp is in the half-open interval (now-window, now].
func timestampInWindow(timestamp, now time.Time, window time.Duration) bool {
	return timestamp.After(now.Add(-window)) && !timestamp.After(now)
}

// hasRetentionShorterThan reports whether either retention below fields is unparseable or shorter than the window.
func hasRetentionShorterThan(item unstructured.Unstructured, window time.Duration, fields ...string) bool {
	for _, retention := range []string{"success", "failure"} {
		value, found, err := unstructured.NestedString(item.Object, append(append([]string{}, fields...), retention)...)
		if err != nil {
			return true
		}
		if !found {
			continue
		}
		duration, err := time.ParseDuration(value)
		if err != nil || duration < window {
			return true
		}
	}
	return false
}

// boolToFloat converts a boolean metric state to its Prometheus gauge representation.
func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

// classify determines the current resource state from persisted metadata and status.
func classify(item unstructured.Unstructured) string {
	if item.GetDeletionTimestamp() != nil {
		return stateDeleting
	}
	if completionTime, found, _ := unstructured.NestedString(item.Object, "status", "completionTime"); found && completionTime != "" {
		return stateCompleted
	}
	conditions, found, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	if found {
		for _, raw := range conditions {
			condition, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			status, _, _ := unstructured.NestedString(condition, "status")
			if status != "True" {
				continue
			}
			conditionType, _, _ := unstructured.NestedString(condition, "type")
			if conditionType == "Ready" || conditionType == conditionActive {
				return stateReady
			}
		}
		for _, raw := range conditions {
			condition, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			status, _, _ := unstructured.NestedString(condition, "status")
			reason, _, _ := unstructured.NestedString(condition, "reason")
			if status == "False" && strings.Contains(strings.ToLower(reason), "fail") {
				return stateFailed
			}
		}
	}
	return stateActive
}

// classifyTestRunPhase determines a TestRun phase from persisted lifecycle evidence.
func classifyTestRunPhase(item unstructured.Unstructured) string {
	if item.GetDeletionTimestamp() != nil {
		if hasConditionReason(item, "Cleanup", "Failed") {
			return phaseCleanupFailed
		}
		return phaseCleaningUp
	}

	activeReason := conditionReason(item, conditionActive)
	switch activeReason {
	case "DeadlineExceeded", "TimedOut":
		return phaseTimedOut
	case "Aborted":
		return phaseAborted
	case "Failed":
		return phaseFailed
	}
	if hasAnyConditionReason(item, "Failed") {
		return phaseFailed
	}

	if completionTime, found, _ := unstructured.NestedString(item.Object, "status", "completionTime"); found && completionTime != "" {
		return phaseCompleted
	}

	switch {
	case hasConditionReason(item, "SuiteRunner", "Starting"):
		return phaseSuiteRunnerStarting
	case hasConditionReason(item, "SuiteRunner", conditionActive) || activeReason == conditionActive:
		return phaseRunning
	case hasConditionReason(item, "Environment", "Completed"):
		return phaseEnvironmentReady
	case hasConditionReason(item, "Environment", "Pending") ||
		hasConditionReason(item, "Environment", "Starting"):
		return phaseEnvironmentProvision
	case activeReason == "Completed":
		return phaseCompleting
	default:
		return phaseAccepted
	}
}

// classifyTestRunVerdict normalizes a persisted TestRun verdict to a bounded value.
func classifyTestRunVerdict(item unstructured.Unstructured) string {
	verdict, found, _ := unstructured.NestedString(item.Object, "status", "verdict")
	if !found || strings.TrimSpace(verdict) == "" {
		return verdictNone
	}
	switch strings.ToLower(strings.TrimSpace(verdict)) {
	case "passed", "pass":
		return verdictPassed
	case "failed", "fail":
		return verdictFailed
	case "inconclusive":
		return verdictInconclusive
	case "none":
		return verdictNone
	default:
		return verdictUnknown
	}
}

// classifyTestRunOperationalConclusion derives the operational conclusion from status conditions.
func classifyTestRunOperationalConclusion(item unstructured.Unstructured) string {
	conditions := testRunConditions(item)
	for _, conditionType := range []string{conditionActive, "SuiteRunner"} {
		condition, ok := conditions[conditionType]
		if !ok {
			continue
		}
		value := normalizeOperationalConclusion(condition.reason, condition.message)
		if value == conclusionFailed || value == conclusionTimedOut || value == conclusionAborted {
			return value
		}
	}
	for _, conditionType := range []string{conditionActive, "SuiteRunner"} {
		condition, ok := conditions[conditionType]
		if !ok {
			continue
		}
		value := normalizeOperationalConclusion(condition.reason, condition.message)
		if value != conclusionUnknown {
			return value
		}
	}
	return conclusionUnknown
}

type testRunCondition struct {
	reason  string
	message string
}

// testRunConditions returns relevant, known Active and SuiteRunner conditions.
func testRunConditions(item unstructured.Unstructured) map[string]testRunCondition {
	result := make(map[string]testRunCondition)
	conditions, found, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	if !found {
		return result
	}
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		conditionType, _, _ := unstructured.NestedString(condition, "type")
		if conditionType != conditionActive && conditionType != "SuiteRunner" {
			continue
		}
		status, _, _ := unstructured.NestedString(condition, "status")
		if status == "" || status == string(metav1.ConditionUnknown) {
			continue
		}
		reason, _, _ := unstructured.NestedString(condition, "reason")
		message, _, _ := unstructured.NestedString(condition, "message")
		result[conditionType] = testRunCondition{reason: reason, message: message}
	}
	return result
}

// normalizeOperationalConclusion maps condition text to a bounded operational conclusion.
func normalizeOperationalConclusion(reason, message string) string {
	text := strings.ToLower(strings.TrimSpace(reason + " " + message))
	switch {
	case strings.Contains(text, "timedout"),
		strings.Contains(text, "timed out"),
		strings.Contains(text, "deadlineexceeded"),
		strings.Contains(text, "deadline exceeded"),
		strings.Contains(text, "timeout"):
		return conclusionTimedOut
	case strings.Contains(text, "aborted"),
		strings.Contains(text, "abort"):
		return conclusionAborted
	case strings.Contains(text, "failed"),
		strings.Contains(text, "failure"):
		return conclusionFailed
	case strings.Contains(text, "inconclusive"):
		return conclusionInconclusive
	case strings.Contains(text, "completed"),
		strings.Contains(text, "successful"),
		strings.Contains(text, "success"):
		return conclusionSuccessful
	case strings.Contains(text, "active"),
		strings.Contains(text, "running"),
		strings.Contains(text, "starting"),
		strings.Contains(text, "pending"):
		return conclusionRunning
	default:
		return conclusionUnknown
	}
}

// conditionReason returns the first non-unknown condition reason for the requested type.
func conditionReason(item unstructured.Unstructured, conditionType string) string {
	conditions, found, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	if !found {
		return ""
	}
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		currentType, _, _ := unstructured.NestedString(condition, "type")
		if currentType != conditionType {
			continue
		}
		status, _, _ := unstructured.NestedString(condition, "status")
		if status == "" || status == string(metav1.ConditionUnknown) {
			continue
		}
		reason, _, _ := unstructured.NestedString(condition, "reason")
		return reason
	}
	return ""
}

// hasConditionReason reports whether the requested condition has the specified reason.
func hasConditionReason(item unstructured.Unstructured, conditionType, reason string) bool {
	return conditionReason(item, conditionType) == reason
}

// hasAnyConditionReason reports whether any condition has the specified reason.
func hasAnyConditionReason(item unstructured.Unstructured, reason string) bool {
	conditions, found, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	if !found {
		return false
	}
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		currentReason, _, _ := unstructured.NestedString(condition, "reason")
		status, _, _ := unstructured.NestedString(condition, "status")
		if status != string(metav1.ConditionUnknown) && currentReason == reason {
			return true
		}
	}
	return false
}
