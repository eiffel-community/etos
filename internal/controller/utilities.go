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
package controller

import (
	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	APIGroupVersionString   = etosv1alpha1.GroupVersion.String()
	APIv2GroupVersionString = etosv1alpha2.GroupVersion.String()
)

const (
	TestRunOwnerKey            = ".metadata.controller.suiterunner"
	EnvironmentRequestOwnerKey = ".metadata.controller.environmentrequest"
	EnvironmentOwnerKey        = ".metadata.controller.environment"
	LogAreaOwnerKey            = ".metadata.controller.log-area-provider"
	ExecutionSpaceOwnerKey     = ".metadata.controller.execution-space-provider"
	IutOwnerKey                = ".metadata.controller.iut-provider"
)

const (
	iutProvider            = ".spec.providers.iut"
	executionSpaceProvider = ".spec.providers.executionSpace"
	logAreaProvider        = ".spec.providers.logarea"
)

const (
	releaseFinalizer  = "etos.eiffel-community.github.io/release"
	providerFinalizer = "etos.eiffel-community.github.io/managed-by-provider"
)

// hasOwner checks if a resource kind exists in ownerReferences.
func hasOwner(ownerReferences []metav1.OwnerReference, kind string) bool {
	for _, ownerReference := range ownerReferences {
		if ownerReference.Kind == kind {
			return true
		}
	}
	return false
}

// isStatusReason return true when the conditionType is present and reason is set to reason
func isStatusReason(conditions []metav1.Condition, conditionType, reason string) bool {
	if condition := meta.FindStatusCondition(conditions, conditionType); condition == nil {
		return false
	} else if condition.Reason == reason {
		return true
	}
	return false
}
