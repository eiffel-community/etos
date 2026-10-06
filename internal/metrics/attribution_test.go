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
	"testing"
	"time"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1beta1 "github.com/eiffel-community/etos/api/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// attributionObject creates an independently identifiable ETOS object using its served inventory version.
func attributionObject(kind, namespace, name, uid string) unstructured.Unstructured {
	version := "v1alpha1"
	switch kind {
	case testRunKind, environmentKind:
		version = "v1beta1"
	case iutKind, logAreaKind, executionSpaceKind:
		version = "v1alpha2"
	}
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": etosGroup + "/" + version, "kind": kind,
		"metadata": map[string]any{"namespace": namespace, "name": name, "uid": uid},
	}}
}

// attributionOwner assigns an exact API group, kind, name and UID to a fixture owner chain.
func attributionOwner(child, parent *unstructured.Unstructured) {
	controller := true
	child.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: parent.GetAPIVersion(), Kind: parent.GetKind(), Name: parent.GetName(),
		UID: parent.GetUID(), Controller: &controller,
	}})
}

// TestClusterIdentityValidatesRoutingAndOwnership covers every resource kind and conflicting or stale references.
func TestClusterIdentityValidatesRoutingAndOwnership(t *testing.T) {
	cluster := attributionObject("Cluster", "team", "first", "cluster-first")
	other := attributionObject("Cluster", "team", "second", "cluster-second")
	run := attributionObject(testRunKind, "team", "run", "run-uid")
	run.Object["spec"] = map[string]any{"cluster": "first"}
	request := attributionObject(environmentRequestKind, "team", "request", "request-uid")
	attributionOwner(&request, &run)
	environment := attributionObject(environmentKind, "team", "env", "env-uid")
	attributionOwner(&environment, &request)
	objects := map[string]*unstructured.Unstructured{}
	for _, object := range []unstructured.Unstructured{cluster, other, run, request, environment} {
		objects[clusterObjectKey(object.GetKind(), &object)] = object.DeepCopy()
	}
	lookup := func(kind, namespace, name string) (*unstructured.Unstructured, bool) {
		object, ok := objects[kind+"/"+namespace+"/"+name]
		return object, ok
	}
	for _, kind := range []string{testRunKind, environmentRequestKind, environmentKind, iutKind, logAreaKind, executionSpaceKind} {
		t.Run(kind, func(t *testing.T) {
			object := attributionObject(kind, "team", "provider", "provider-uid")
			switch kind {
			case testRunKind:
				object = run
			case environmentRequestKind:
				object = request
			case environmentKind:
				object = environment
			default:
				attributionOwner(&object, &environment)
			}
			if got := clusterIdentity(&object, lookup, map[string]bool{}); got != "team/first" {
				t.Fatalf("identity = %q, want team/first", got)
			}
			direct := object.DeepCopy()
			direct.SetOwnerReferences(nil)
			direct.SetLabels(map[string]string{clusterLabel: "first"})
			if got := clusterIdentity(direct, lookup, map[string]bool{}); got != "team/first" {
				t.Fatalf("explicit routing identity = %q, want team/first", got)
			}
			if kind == testRunKind {
				direct.SetLabels(map[string]string{clusterLabel: "second"})
				if got := clusterIdentity(direct, lookup, map[string]bool{}); got != UnassignedCluster {
					t.Fatalf("conflicting TestRun spec and label identity = %q, want unassigned", got)
				}
			}
		})
	}
	for _, scenario := range []string{"missing", "unknown cluster", "wrong namespace", "conflicting label", "stale UID", "wrong API group", "cycle", "deleted cluster"} {
		t.Run(scenario, func(t *testing.T) {
			object := request.DeepCopy()
			switch scenario {
			case "missing":
				object.SetOwnerReferences(nil)
			case "unknown cluster":
				object.SetOwnerReferences(nil)
				object.SetLabels(map[string]string{clusterLabel: "does-not-exist"})
			case "wrong namespace":
				object.SetNamespace("another-team")
			case "conflicting label":
				object.SetLabels(map[string]string{clusterLabel: "second"})
			case "stale UID":
				owners := object.GetOwnerReferences()
				owners[0].UID = "previous-run-uid"
				object.SetOwnerReferences(owners)
			case "wrong API group":
				owners := object.GetOwnerReferences()
				owners[0].APIVersion = "foreign.example/v1"
				object.SetOwnerReferences(owners)
			case "cycle":
				attributionOwner(object, object)
				objects[clusterObjectKey(object.GetKind(), object)] = object
				t.Cleanup(func() { objects[clusterObjectKey(request.GetKind(), &request)] = request.DeepCopy() })
			case "deleted cluster":
				delete(objects, clusterObjectKey(cluster.GetKind(), &cluster))
				t.Cleanup(func() { objects[clusterObjectKey(cluster.GetKind(), &cluster)] = cluster.DeepCopy() })
			}
			if got := clusterIdentity(object, lookup, map[string]bool{}); got != UnassignedCluster {
				t.Fatalf("identity = %q, want unassigned", got)
			}
		})
	}
}

// TestResolveClusterPreservesAlphaBetaRouting checks actual schemas/conversion and typed cache lookups.
func TestResolveClusterPreservesAlphaBetaRouting(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := etosv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &etosv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "blue", Namespace: "team"}}
	beta := &etosv1beta1.TestRun{
		ObjectMeta: metav1.ObjectMeta{Name: "overlap", Namespace: "team"},
		Spec:       etosv1beta1.TestRunSpec{Cluster: "blue"},
	}
	alpha := &etosv1alpha1.TestRun{}
	if err := beta.ConvertTo(alpha); err != nil {
		t.Fatal(err)
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	for _, object := range []client.Object{alpha, beta} {
		if got := ResolveCluster(context.Background(), reader, object, testRunKind); got != "team/blue" {
			t.Fatalf("resolved %T to %q", object, got)
		}
	}
	if err := reader.Delete(context.Background(), cluster); err != nil {
		t.Fatal(err)
	}
	if got := ResolveCluster(context.Background(), reader, alpha, testRunKind); got != UnassignedCluster {
		t.Fatalf("deleted cluster resolved to %q", got)
	}
}

// partitionReader provides complete cache lists and deliberately rejects per-object reads.
type partitionReader struct {
	items []unstructured.Unstructured
}

// Get rejects per-resource reads to ensure inventory uses only bounded batch lists.
func (r *partitionReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("inventory must not perform per-object reads")
}

// List returns only the requested resource kind, as a real manager cache does.
func (r *partitionReader) List(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
	target := list.(*unstructured.UnstructuredList)
	for _, item := range r.items {
		if item.GetKind()+"List" == target.GetKind() {
			target.Items = append(target.Items, *item.DeepCopy())
		}
	}
	return nil
}

// TestInventoryPartitionsTrueClusters verifies overlapping names, same-namespace routing, zero baselines and deletion.
func TestInventoryPartitionsTrueClusters(t *testing.T) {
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	reader := &partitionReader{}
	for _, location := range []struct{ namespace, cluster, run string }{
		{"team", "blue", "overlap"}, {"team", "green", "other"}, {"other-team", "blue", "overlap"},
	} {
		cluster := attributionObject("Cluster", location.namespace, location.cluster, location.namespace+"-"+location.cluster)
		if location.cluster == "green" {
			cluster.Object["spec"] = map[string]any{"etos": map[string]any{"config": map[string]any{
				"testrunRetention": map[string]any{"success": "30m"},
			}}}
		}
		run := attributionObject(testRunKind, location.namespace, location.run, location.namespace+"-"+location.run)
		run.SetCreationTimestamp(metav1.NewTime(now.Add(-time.Minute)))
		run.Object["spec"] = map[string]any{"cluster": location.cluster, "deadline": now.Add(-time.Second).Unix()}
		request := attributionObject(environmentRequestKind, location.namespace, location.run+"-request", location.namespace+"-"+location.run+"-request")
		attributionOwner(&request, &run)
		environment := attributionObject(environmentKind, location.namespace, location.run+"-env", location.namespace+"-"+location.run+"-env")
		attributionOwner(&environment, &request)
		reader.items = append(reader.items, cluster, run, request, environment, run) // Duplicate lists must not inflate any family.
		for _, kind := range []string{iutKind, logAreaKind, executionSpaceKind} {
			provider := attributionObject(kind, location.namespace, location.run+"-provider", location.namespace+"-"+location.run+"-"+kind)
			attributionOwner(&provider, &environment)
			reader.items = append(reader.items, provider)
		}
	}
	empty := attributionObject("Cluster", "team", "empty", "empty")
	orphan := attributionObject(iutKind, "team", "orphan", "orphan")
	reader.items = append(reader.items, empty, orphan)
	collector := &Collector{Reader: reader, Now: func() time.Time { return now }, leaderCtx: context.Background()}
	collector.collect(context.Background())
	for _, identity := range []string{"team/blue", "team/green", "other-team/blue"} {
		for _, spec := range resources {
			if got := exactMetricValue(t, collector, inventoryCounts, spec.resourceType, stateActive, "cluster", identity); got != 1 {
				t.Errorf("%s %s = %v, want 1", identity, spec.kind, got)
			}
		}
		if exactMetricValue(t, collector, testRunCreated, "1h", "cluster", identity) != 1 ||
			exactMetricValue(t, collector, testRunOldestActiveAge, "cluster", identity) != 60 ||
			exactMetricValue(t, collector, testRunOverdue, "cluster", identity) != 1 ||
			exactMetricValue(t, collector, testRunPhase, phaseAccepted, "cluster", identity) != 1 ||
			exactMetricValue(t, collector, testRunVerdict, verdictNone, "cluster", identity) != 1 ||
			exactMetricValue(t, collector, testRunOperationalConclusion, conclusionUnknown, "cluster", identity) != 1 {
			t.Errorf("%s lifecycle partition does not contain exactly one retained run", identity)
		}
		wantCoverage := 1.0
		if identity == "team/green" {
			wantCoverage = 0
		}
		if exactMetricValue(t, collector, testRunWindowComplete, "1h", "cluster", identity) != wantCoverage {
			t.Errorf("%s retention coverage was contaminated by another Cluster", identity)
		}
	}
	if exactMetricValue(t, collector, testRunCreated, "1h", "cluster", "team/empty") != 0 ||
		exactMetricValue(t, collector, inventoryCounts, "iut", stateActive, "cluster", UnassignedCluster) != 1 ||
		exactMetricValue(t, collector, testRunWindowComplete, "24h", "cluster", UnassignedCluster) != 0 ||
		exactMetricValue(t, collector, snapshotReady, "manager") != 1 {
		t.Fatal("empty, unassigned or manager-wide metrics have incorrect semantics")
	}
	retained := reader.items[:0]
	for _, item := range reader.items {
		if item.GetKind() != "Cluster" || item.GetNamespace() != "team" || item.GetName() != "blue" {
			retained = append(retained, item)
		}
	}
	reader.items = retained
	collector.collect(context.Background())
	if exactMetricValue(t, collector, testRunCreated, "1h", "cluster", UnassignedCluster) != 1 ||
		exactMetricValue(t, collector, testRunCreated, "1h", "cluster", "other-team/blue") != 1 {
		t.Fatal("deleted Cluster resources must become unassigned without affecting overlapping names")
	}
}

// TestClusterTransformInitializesRealZeroBaselines verifies cache ingestion before terminal samples.
func TestClusterTransformInitializesRealZeroBaselines(t *testing.T) {
	transitions, registry := newTransitionRegistry(t)
	for _, identity := range []types.NamespacedName{{Namespace: "team", Name: "blue"}, {Namespace: "team", Name: "green"}, {Namespace: "other-team", Name: "blue"}} {
		cluster := &etosv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: identity.Namespace, Name: identity.Name}}
		for range 2 {
			object, err := transitions.ClusterTransform(cluster)
			if err != nil || object != cluster {
				t.Fatal("baseline initialization must preserve the cache object")
			}
		}
	}
	families := gatherTransitions(t, registry)
	for _, test := range []struct {
		name     string
		outcomes int
	}{
		{testRunFinishedName, len(testRunOutcomes)},
		{environmentRequestFinishedName, len(environmentRequestOutcomes)},
	} {
		family := findPrometheusFamily(t, families, test.name)
		if len(family.Metric) != 4*test.outcomes {
			t.Fatalf("%s zero baseline series = %d, want %d", test.name, len(family.Metric), 4*test.outcomes)
		}
		for _, metric := range family.Metric {
			if metric.GetCounter().GetValue() != 0 {
				t.Fatalf("%s baseline has a fake terminal count", test.name)
			}
		}
	}
	for _, name := range []string{testRunDurationName, environmentRequestDurationName} {
		for _, metric := range findPrometheusFamily(t, families, name).Metric {
			assertDuration(t, metric.GetHistogram(), 0, 0)
		}
	}
	start := time.Now()
	transitions.RecordTestRunFinished("team/blue", OutcomeSuccess, start, start.Add(time.Second))
	transitions.RecordTestRunFinished("team/green", OutcomeSuccess, start.Add(time.Second), start)
	transitions.RecordEnvironmentRequestFinished("other-team/blue", OutcomeFailure, start, start.Add(2*time.Second))
	for _, test := range []struct {
		name, cluster string
		count         uint64
		sum           float64
	}{
		{testRunDurationName, "team/blue", 1, 1},
		{testRunDurationName, "team/green", 0, 0},
		{testRunDurationName, "other-team/blue", 0, 0},
		{environmentRequestDurationName, "other-team/blue", 1, 2},
	} {
		outcome := OutcomeSuccess
		if test.name == environmentRequestDurationName {
			outcome = OutcomeFailure
		}
		found := false
		for _, metric := range findPrometheusFamily(t, gatherTransitions(t, registry), test.name).Metric {
			if metric.Label[0].GetValue() == test.cluster && metric.Label[1].GetValue() == outcome {
				found = true
				assertDuration(t, metric.GetHistogram(), test.count, test.sum)
			}
		}
		if !found {
			t.Errorf("%s missing zero-count baseline for %s", test.name, test.cluster)
		}
	}
	// Use a pedantic registry to check the full dimension schema, not synthetic samples.
	if err := transitions.RegisterPrometheus(prometheus.NewPedanticRegistry()); err != nil {
		t.Fatal(err)
	}
}
