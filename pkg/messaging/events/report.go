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

type File struct {
	URL       string            `json:"url"`
	Name      string            `json:"name"`
	Directory string            `json:"directory,omitempty"`
	Checksums map[string]string `json:"checksums,omitempty"`
}

type Report struct {
	ID    int       `json:"id,omitempty"`
	Event EventType `json:"event"`
	Meta  string    `json:"meta"`
	Data  File      `json:"data"`
}

// NewReport creates a new Report event with the given File.
func NewReport(file File) Report {
	return Report{
		Event: ReportType,
		Data:  file,
		Meta:  "*",
	}
}

// EventType returns the Event field of the event.
func (e Report) EventType() EventType {
	return e.Event
}

// EventData returns the Data field of the event.
func (e Report) EventData() any {
	return e.Data
}

// EventMeta returns the Meta field of the event.
func (e Report) EventMeta() string {
	return e.Meta
}
