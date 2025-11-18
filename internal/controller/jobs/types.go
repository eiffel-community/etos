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
package jobs

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type (
	Conclusion string
	Verdict    string
	Status     string
)

const (
	ConclusionSuccessful   Conclusion = "Successful"
	ConclusionFailed       Conclusion = "Failed"
	ConclusionAborted      Conclusion = "Aborted"
	ConclusionTimedOut     Conclusion = "TimedOut"
	ConclusionInconclusive Conclusion = "Inconclusive"
)

const (
	VerdictPassed       Verdict = "Passed"
	VerdictFailed       Verdict = "Failed"
	VerdictInconclusive Verdict = "Inconclusive"
	VerdictNone         Verdict = "None"
)

const (
	StatusFailed     Status = "Failed"
	StatusSuccessful Status = "Successful"
	StatusActive     Status = "Active"
	StatusNone       Status = ""
)

// Result describes the status and result of an ETOS job
type Result struct {
	Conclusion  Conclusion `json:"conclusion"`
	Verdict     Verdict    `json:"verdict,omitempty"`
	Description string     `json:"description,omitempty"`
}

type JobSpecFunc func(context.Context, client.Object) (*batchv1.Job, error)

type Job interface {
	Create(context.Context, client.Object, JobSpecFunc) error
	Delete(context.Context) error
	Result(context.Context, ...string) Result
	Status(context.Context) (Status, error)
}
