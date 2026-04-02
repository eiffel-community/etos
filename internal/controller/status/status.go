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
package status

import "fmt"

const (
	StatusAvailable   = "Available"
	StatusReady       = "Ready"
	StatusFailed      = "Failed"
	StatusActive      = "Active"
	StatusEnvironment = "Environment"
	StatusSuiteRunner = "SuiteRunner"
)

const (
	ReasonPending   = "Pending"
	ReasonStarting  = "Starting"
	ReasonActive    = "Active"
	ReasonFailed    = "Failed"
	ReasonTimedOut  = "DeadlineExceeded"
	ReasonCompleted = "Completed"
)

// NotReadyError is returned by sub-reconcilers when their resources have been
// reconciled successfully but the underlying pods are not yet ready.
type NotReadyError struct {
	Name            string
	ReadyReplicas   int32
	DesiredReplicas int32
}

func (e *NotReadyError) Error() string {
	return fmt.Sprintf("%s: %d/%d replicas ready", e.Name, e.ReadyReplicas, e.DesiredReplicas)
}
