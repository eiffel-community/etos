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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Test_projectInventoryObject_dropsPayloadWithoutAliasing verifies payload removal and independent projected data.
func Test_projectInventoryObject_dropsPayloadWithoutAliasing(t *testing.T) {
	run := inventoryProjectionFixture(t)
	original := run.DeepCopy()
	projected := projectInventoryForTest(t, &run)
	if !reflect.DeepEqual(&run, original) {
		t.Fatal("projection mutated the source")
	}
	data, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "execution-payload") {
		t.Fatal("projection retained an unrelated spec, status, or metadata payload")
	}
	larger := run.DeepCopy()
	inventoryTestValue[map[string]any](t, larger.Object["spec"])["suites"] = []any{strings.Repeat("execution-payload", 100000)}
	if !reflect.DeepEqual(projected, projectInventoryForTest(t, larger)) {
		t.Fatal("retained inventory depends on suite size")
	}
	const changed = "changed"
	inventoryTestValue[map[string]any](t, projected.Object["spec"])["cluster"] = changed
	projected.SetLabels(map[string]string{clusterLabel: changed})
	status := inventoryTestValue[map[string]any](t, projected.Object["status"])
	conditions := inventoryTestValue[[]any](t, status["conditions"])
	inventoryTestValue[map[string]any](t, conditions[0])["reason"] = changed
	metadata := inventoryTestValue[map[string]any](t, projected.Object["metadata"])
	owners := inventoryTestValue[[]any](t, metadata["ownerReferences"])
	inventoryTestValue[map[string]any](t, owners[0])["uid"] = changed
	if !reflect.DeepEqual(&run, original) {
		t.Fatal("projected data aliases the source")
	}
}

// Test_projectInventoryObject_preservesMetrics compares full and projected lifecycle and attribution evidence.
func Test_projectInventoryObject_preservesMetrics(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	for _, scenario := range []string{"active", "completed", "deleting", "conflict", "stale-owner", "malformed-retention", "duplicate-conditions"} {
		t.Run(scenario, func(t *testing.T) {
			cluster := attributionObject(clusterKind, "team", "blue", "cluster-uid")
			cluster.Object["spec"] = map[string]any{
				"etos": map[string]any{"config": map[string]any{
					"testrunRetention": map[string]any{"success": "2h", "failure": "48h"},
					"other":            "execution-payload",
				}},
			}
			run := inventoryProjectionFixture(t)
			run.SetOwnerReferences(nil)
			spec := inventoryTestValue[map[string]any](t, run.Object["spec"])
			status := inventoryTestValue[map[string]any](t, run.Object["status"])
			metadata := inventoryTestValue[map[string]any](t, run.Object["metadata"])
			spec["deadline"] = now.Add(-time.Minute).Unix()
			switch scenario {
			case "completed":
				status["completionTime"] = now.Format(time.RFC3339)
			case "deleting":
				metadata["deletionTimestamp"] = now.Format(time.RFC3339)
			case "conflict":
				run.SetLabels(map[string]string{clusterLabel: "another"})
			case "malformed-retention":
				spec["retention"] = int64(123)
			case "duplicate-conditions":
				status["conditions"] = []any{
					map[string]any{"type": "Active", "status": "Unknown", "reason": "Failed"},
					map[string]any{"type": "Active", "status": "False", "reason": "Completed"},
					map[string]any{"type": "Active", "status": "True", "reason": "Active", "message": "Suite runner failed"},
				}
			}
			items := make([]unstructured.Unstructured, 0, len(resources)+1)
			items = append(items, cluster, run)
			for _, spec := range resources[1:] {
				child := attributionObject(spec.kind, "team", spec.name, spec.name+"-uid")
				attributionOwner(&child, &run)
				if scenario == "stale-owner" {
					childMetadata := inventoryTestValue[map[string]any](t, child.Object["metadata"])
					owners := inventoryTestValue[[]any](t, childMetadata["ownerReferences"])
					inventoryTestValue[map[string]any](t, owners[0])["uid"] = "previous"
				}
				child.Object["status"] = map[string]any{
					"conditions": []any{map[string]any{
						"type": "Ready", "status": "False", "reason": "Failed", "message": "execution-payload",
					}},
				}
				child.Object["spec"] = map[string]any{"payload": "execution-payload"}
				items = append(items, child)
			}
			projected := make([]unstructured.Unstructured, len(items))
			for i := range items {
				projected[i] = *projectInventoryForTest(t, &items[i])
			}
			collect := func(objects []unstructured.Unstructured) map[string]float64 {
				c := &Collector{Reader: &partitionReader{items: objects}, Now: func() time.Time { return now }, leaderCtx: t.Context()}
				c.collect(t.Context())
				values := make(map[string]float64)
				for _, point := range c.observations() {
					values[fmt.Sprintf("%s/%v", point.instrument.name, point.attributes.ToSlice())] = point.value
				}
				return values
			}
			if full, slim := collect(items), collect(projected); !reflect.DeepEqual(full, slim) {
				t.Fatal("projection changed inventory metrics")
			}
		})
	}
}

// Test_projectInventoryObject_rejectsUnexpectedObjects checks that unsupported transform inputs return errors.
func Test_projectInventoryObject_rejectsUnexpectedObjects(t *testing.T) {
	for _, object := range []any{nil, &unstructured.Unstructured{Object: map[string]any{"kind": "Secret"}}} {
		if _, err := projectInventoryObject(object); err == nil {
			t.Fatalf("transform accepted %T", object)
		}
	}
}

// inventoryProjectionFixture creates lifecycle evidence alongside payloads that inventory must discard.
func inventoryProjectionFixture(t testing.TB) unstructured.Unstructured {
	t.Helper()
	run := attributionObject(testRunKind, "team", "run", "run-uid")
	metadata := inventoryTestValue[map[string]any](t, run.Object["metadata"])
	metadata["creationTimestamp"] = "2026-11-13T08:30:00Z"
	run.SetLabels(map[string]string{clusterLabel: "blue", "unrelated": "execution-payload"})
	run.SetAnnotations(map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "execution-payload"})
	metadata["managedFields"] = []any{map[string]any{"manager": "execution-payload"}}
	owner := attributionObject(clusterKind, "team", "blue", "cluster-uid")
	attributionOwner(&run, &owner)
	run.Object["spec"] = map[string]any{
		"cluster": "blue", "deadline": int64(1794560340),
		"retention": map[string]any{"success": "2h", "failure": "48h"},
		"suites":    []any{map[string]any{"tests": []any{"execution-payload"}, "dataset": "execution-payload"}},
		"providers": map[string]any{"other": "execution-payload"},
	}
	run.Object["status"] = map[string]any{
		"verdict": "Passed", "environmentRequests": []any{"execution-payload"},
		"conditions": []any{map[string]any{
			"type": "Active", "status": "False", "reason": "Completed", "message": "Suite runner failed",
			"unrelated": "execution-payload",
		}},
	}
	return run
}

// projectInventoryForTest requires a successful transform and checks its result type.
func projectInventoryForTest(t *testing.T, item *unstructured.Unstructured) *unstructured.Unstructured {
	t.Helper()
	projected, err := projectInventoryObject(item)
	if err != nil {
		t.Fatal(err)
	}
	return inventoryTestValue[*unstructured.Unstructured](t, projected)
}

// inventoryTestValue reports unexpected JSON fixture types at the calling test or benchmark.
func inventoryTestValue[T any](t testing.TB, value any) T {
	t.Helper()
	typed, ok := value.(T)
	if !ok {
		t.Fatalf("expected %T, got %T", typed, value)
	}
	return typed
}

// Benchmark_projectInventoryObject_payloadSize measures whether irrelevant suite size affects projection allocations.
func Benchmark_projectInventoryObject_payloadSize(b *testing.B) {
	for _, size := range []int{1, 100000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			run := inventoryProjectionFixture(b)
			inventoryTestValue[map[string]any](b, run.Object["spec"])["suites"] = []any{strings.Repeat("execution-payload", size)}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := projectInventoryObject(&run); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
