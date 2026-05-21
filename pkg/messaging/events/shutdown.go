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

type Result struct {
	Conclusion  Conclusion `json:"conclusion"`
	Verdict     Verdict    `json:"verdict"`
	Description string     `json:"description,omitempty"`
}

type Shutdown struct {
	ID    int    `json:"id,omitempty"`
	Event string `json:"event"`
	Meta  string `json:"meta"`
	Data  Result `json:"data"`
}

// NewShutdown creates a new Shutdown event with the given Result.
func NewShutdown(result Result) Shutdown {
	return Shutdown{
		Event: "shutdown",
		Data:  result,
		Meta:  "*",
	}
}

// EventType returns the Event field of the event.
func (e Shutdown) EventType() string {
	return e.Event
}

// EventData returns the Data field of the event.
func (e Shutdown) EventData() any {
	return e.Data
}

// EventMeta returns the Meta field of the event.
func (e Shutdown) EventMeta() string {
	return e.Meta
}
