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

package readiness

import (
	"context"

	"github.com/eiffel-community/etos/internal/controller/status"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckDeployment verifies that a Deployment has its desired number of ready replicas.
// Returns nil if ready, a NotReadyError if not yet ready, or an error if the Get fails.
func CheckDeployment(ctx context.Context, c client.Client, name types.NamespacedName) error {
	dep := &appsv1.Deployment{}
	if err := c.Get(ctx, name, dep); err != nil {
		return err
	}
	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	if dep.Status.ReadyReplicas < desired {
		return &status.NotReadyError{
			Name:            name.Name,
			ReadyReplicas:   dep.Status.ReadyReplicas,
			DesiredReplicas: desired,
		}
	}
	return nil
}

// CheckStatefulSet verifies that a StatefulSet has its desired number of ready replicas.
// Returns nil if ready, a NotReadyError if not yet ready, or an error if the Get fails.
func CheckStatefulSet(ctx context.Context, c client.Client, name types.NamespacedName) error {
	ss := &appsv1.StatefulSet{}
	if err := c.Get(ctx, name, ss); err != nil {
		return err
	}
	desired := int32(1)
	if ss.Spec.Replicas != nil {
		desired = *ss.Spec.Replicas
	}
	if ss.Status.ReadyReplicas < desired {
		return &status.NotReadyError{
			Name:            name.Name,
			ReadyReplicas:   ss.Status.ReadyReplicas,
			DesiredReplicas: desired,
		}
	}
	return nil
}
