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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// NewInventoryCache constructs a separate, payload-trimmed cache for Collector.
// Add it to the manager before starting it; collector reads wait for informer sync
// within their collection timeout, independently of reconciliation cache startup.
func NewInventoryCache(ctx context.Context, mgr manager.Manager) (cache.Cache, error) {
	inventory, err := cache.New(mgr.GetConfig(), cache.Options{
		Scheme:                      mgr.GetScheme(),
		Mapper:                      mgr.GetRESTMapper(),
		HTTPClient:                  mgr.GetHTTPClient(),
		DefaultTransform:            projectInventoryObject,
		ReaderFailOnMissingInformer: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create inventory cache: %w", err)
	}
	kinds := make([]schema.GroupVersionKind, 1, len(resources)+1)
	kinds[0] = schema.GroupVersionKind{Group: etosGroup, Version: "v1alpha1", Kind: clusterKind}
	for _, spec := range resources {
		kinds = append(kinds, schema.GroupVersionKind{Group: spec.group, Version: spec.version, Kind: spec.kind})
	}
	for _, gvk := range kinds {
		item := &unstructured.Unstructured{}
		item.SetGroupVersionKind(gvk)
		if _, err := inventory.GetInformer(ctx, item, cache.BlockUntilSynced(false)); err != nil {
			return nil, fmt.Errorf("register inventory informer for %s: %w", gvk, err)
		}
	}
	return inventory, nil
}

// inventoryFields selects nested fields; a nil selection retains the complete leaf value.
type inventoryFields map[string]inventoryFields

var inventoryMetadata = inventoryFields{
	"name": nil, "namespace": nil, "uid": nil, "resourceVersion": nil,
	"creationTimestamp": nil, "deletionTimestamp": nil, "ownerReferences": nil,
	"labels": {clusterLabel: nil},
}

var inventoryConditionFields = inventoryFields{"type": nil, "status": nil, "reason": nil}

var inventoryTestRunConditionFields = inventoryFields{"type": nil, "status": nil, "reason": nil, "message": nil}

var inventoryRetentionFields = inventoryFields{"success": nil, "failure": nil}

// projectInventoryObject drops execution payloads before initial and updated objects enter the inventory store.
func projectInventoryObject(object any) (any, error) {
	item, ok := object.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("inventory transform expected an unstructured object, got %T", object)
	}
	fields := inventoryFields{"apiVersion": nil, "kind": nil, "metadata": inventoryMetadata}
	switch item.GetKind() {
	case clusterKind:
		fields["spec"] = inventoryFields{
			"etos": {"config": {"testrunRetention": inventoryRetentionFields}},
		}
	case testRunKind:
		fields["spec"] = inventoryFields{
			"cluster": nil, "deadline": nil, "retention": inventoryRetentionFields,
		}
		fields["status"] = inventoryFields{
			"completionTime": nil, "verdict": nil, "conditions": inventoryTestRunConditionFields,
		}
	case environmentRequestKind, environmentKind, executionSpaceKind, iutKind, logAreaKind:
		fields["status"] = inventoryFields{
			"completionTime": nil, "conditions": inventoryConditionFields,
		}
	default:
		return nil, fmt.Errorf("unsupported inventory kind %q", item.GetKind())
	}
	return &unstructured.Unstructured{Object: projectInventoryFields(item.Object, fields)}, nil
}

// projectInventoryFields copies only selected keys without aliasing the source object.
func projectInventoryFields(object map[string]any, fields inventoryFields) map[string]any {
	projected := make(map[string]any, len(fields))
	for key, selection := range fields {
		if value, found := object[key]; found {
			projected[key] = projectInventoryValue(value, selection)
		}
	}
	return projected
}

// projectInventoryValue applies a selection to objects and arrays while preserving malformed scalar values.
func projectInventoryValue(value any, fields inventoryFields) any {
	if fields == nil {
		return runtime.DeepCopyJSONValue(value)
	}
	switch value := value.(type) {
	case map[string]any:
		return projectInventoryFields(value, fields)
	case []any:
		projected := make([]any, len(value))
		for i, item := range value {
			projected[i] = projectInventoryValue(item, fields)
		}
		return projected
	default:
		// Preserve malformed selected values so classification still fails closed.
		return runtime.DeepCopyJSONValue(value)
	}
}
