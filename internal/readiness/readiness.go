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
	"errors"

	"github.com/eiffel-community/etos/internal/controller/status"
	appsv1 "k8s.io/api/apps/v1"
)

// IsNotReadyError returns true if the error is (or wraps) a NotReadyError.
func IsNotReadyError(err error) bool {
	var notReady *status.NotReadyError
	return errors.As(err, &notReady)
}

// DeploymentReady checks whether a Deployment has its desired number of ready replicas.
// Returns nil if ready, or a NotReadyError if not yet ready.
func DeploymentReady(dep *appsv1.Deployment) error {
	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	if dep.Status.ReadyReplicas < desired {
		return &status.NotReadyError{
			Name:            dep.Name,
			ReadyReplicas:   dep.Status.ReadyReplicas,
			DesiredReplicas: desired,
		}
	}
	return nil
}

// StatefulSetReady checks whether a StatefulSet has its desired number of ready replicas.
// Returns nil if ready, or a NotReadyError if not yet ready.
func StatefulSetReady(ss *appsv1.StatefulSet) error {
	desired := int32(1)
	if ss.Spec.Replicas != nil {
		desired = *ss.Spec.Replicas
	}
	if ss.Status.ReadyReplicas < desired {
		return &status.NotReadyError{
			Name:            ss.Name,
			ReadyReplicas:   ss.Status.ReadyReplicas,
			DesiredReplicas: desired,
		}
	}
	return nil
}
