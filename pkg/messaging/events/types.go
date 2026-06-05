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
package events

import "strings"

type EventType string

const (
	MessageType  EventType = "message"
	ArtifactType EventType = "artifact"
	ReportType   EventType = "report"
	ShutdownType EventType = "shutdown"
	StatusType   EventType = "status"
	PingType     EventType = "ping"
)

// String returns the string representation of the EventType.
func (e EventType) String() string {
	return string(e)
}

// ToLower returns the lowercase string representation of the EventType.
func (e EventType) ToLower() string {
	return strings.ToLower(string(e))
}

type (
	Conclusion string
	Verdict    string
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
