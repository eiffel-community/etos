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
	"maps"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// UnassignedCluster is the bounded identity for missing or unverifiable ownership.
	UnassignedCluster      = "unassigned"
	clusterAttributeName   = "etos_cluster"
	clusterLabel           = "etos.eiffel-community.github.io/cluster"
	etosGroup              = "etos.eiffel-community.github.io"
	clusterKind            = "Cluster"
	testRunKind            = "TestRun"
	iutKind                = "Iut"
	logAreaKind            = "LogArea"
	environmentKind        = "Environment"
	environmentRequestKind = "EnvironmentRequest"
	executionSpaceKind     = "ExecutionSpace"
)

// objectLookup finds an ETOS object in the resource's namespace without crossing namespaces.
type objectLookup func(kind, namespace, name string) (*unstructured.Unstructured, bool)

// clusterIdentity resolves explicit routing and validated ETOS owner chains.
// Owner UIDs prevent a recreated object with the same name from stealing attribution.
// Conflicting explicit routing and ownership fail closed rather than choosing a cluster.
func clusterIdentity(item *unstructured.Unstructured, lookup objectLookup, visited map[string]bool) string {
	key := item.GetKind() + "/" + item.GetNamespace() + "/" + item.GetName()
	if visited[key] || item.GetNamespace() == "" {
		return UnassignedCluster
	}
	visited[key] = true
	name := item.GetLabels()[clusterLabel]
	if item.GetKind() == testRunKind {
		routed, _, _ := unstructured.NestedString(item.Object, "spec", "cluster")
		if routed != "" {
			if name != "" && name != routed {
				return UnassignedCluster
			}
			name = routed
		}
	}
	direct := UnassignedCluster
	if name != "" {
		if _, ok := lookup(clusterKind, item.GetNamespace(), name); ok {
			direct = item.GetNamespace() + "/" + name
		}
	}
	owned := UnassignedCluster
	hasOwner := false
	for _, owner := range item.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil || gv.Group != etosGroup || !attributionKind(owner.Kind) {
			continue
		}
		hasOwner = true
		parent, ok := lookup(owner.Kind, item.GetNamespace(), owner.Name)
		if !ok || owner.UID == "" || owner.UID != parent.GetUID() {
			return UnassignedCluster
		}
		var resolved string
		if owner.Kind == clusterKind {
			resolved = parent.GetNamespace() + "/" + parent.GetName()
		} else {
			branch := make(map[string]bool, len(visited))
			maps.Copy(branch, visited)
			resolved = clusterIdentity(parent, lookup, branch)
		}
		if resolved == UnassignedCluster || (owned != UnassignedCluster && owned != resolved) {
			return UnassignedCluster
		}
		owned = resolved
	}
	if hasOwner {
		if name != "" && direct != owned {
			return UnassignedCluster
		}
		return owned
	}
	return direct
}

// attributionKind limits ownership traversal to ETOS domain kinds, never provider configuration or Jobs.
func attributionKind(kind string) bool {
	switch kind {
	case clusterKind, testRunKind, environmentRequestKind, environmentKind, executionSpaceKind, iutKind, logAreaKind:
		return true
	default:
		return false
	}
}

// ResolveCluster resolves a resource using the supplied reader; controllers supply a cache-backed client.
// This runs on transitions or provider startup, never during Prometheus scrapes.
// Lookup failures cannot change lifecycle persistence and instead produce unassigned attribution.
func ResolveCluster(ctx context.Context, reader client.Reader, object client.Object, kind string) string {
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to resolve metric cluster attribution", "kind", kind)
		return UnassignedCluster
	}
	item := &unstructured.Unstructured{Object: data}
	item.SetKind(kind)
	lookup := func(kind, namespace, name string) (*unstructured.Unstructured, bool) {
		// Typed objects keep controller-runtime's delegating client on the cache path;
		// unstructured reads on that client bypass the cache by default.
		var object client.Object
		switch kind {
		case clusterKind:
			object = &etosv1alpha1.Cluster{}
		case testRunKind:
			object = &etosv1alpha1.TestRun{}
		case environmentRequestKind:
			object = &etosv1alpha1.EnvironmentRequest{}
		case environmentKind:
			object = &etosv1alpha1.Environment{}
		case executionSpaceKind:
			object = &etosv1alpha2.ExecutionSpace{}
		case iutKind:
			object = &etosv1alpha2.Iut{}
		case logAreaKind:
			object = &etosv1alpha2.LogArea{}
		default:
			return nil, false
		}
		if err := reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, object); err != nil {
			return nil, false
		}
		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if err != nil {
			return nil, false
		}
		parent := &unstructured.Unstructured{Object: data}
		parent.SetKind(kind)
		return parent, true
	}
	identity := clusterIdentity(item, lookup, make(map[string]bool))
	if identity == UnassignedCluster {
		log.FromContext(ctx).Info("Could not attribute ETOS resource to Cluster",
			"kind", kind, "resource", client.ObjectKeyFromObject(object))
	}
	return identity
}

// clusterObjectKey keys batch inventory objects by kind and namespaced name, not bare name or UID alone.
func clusterObjectKey(kind string, object metav1.Object) string {
	return kind + "/" + object.GetNamespace() + "/" + object.GetName()
}
