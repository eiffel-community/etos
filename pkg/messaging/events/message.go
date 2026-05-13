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

import "encoding/json"

type Log struct {
	Name       string         `json:"name"`
	Level      string         `json:"levelname"`
	Message    string         `json:"message"`
	Identifier string         `json:"identifier"`
	Timestamp  string         `json:"@timestamp"`
	Extra      map[string]any `json:"-"`

	// Extra field for the logging package to indicate that the log should not be logged to the user. This is used to
	// prevent logs that are only meant for internal use from being logged to the user.
	// Defaults to false, meaning that logs will be logged to the user by default.
	DisableUserLog bool `json:"disableUserLog,omitempty"`
}

type Message struct {
	ID         int    `json:"id,omitempty"`
	Identifier string `json:"identifier"`
	Event      string `json:"event"`
	Meta       string `json:"meta"`
	Data       Log    `json:"data"`
}

// NewMessage creates a new Message instance with the given Log.
func NewMessage(log Log) Message {
	return Message{
		Event: "message",
		Data:  log,
		Meta:  log.Level,
	}
}

// EventType returns the Event field of the event.
func (e Message) EventType() string {
	return e.Event
}

// EventData returns the Data field of the event.
func (e Message) EventData() any {
	return e.Data
}

// EventMeta returns the Meta field of the event.
func (e Message) EventMeta() string {
	return e.Meta
}

// MarshalJSON implements the json.Marshaler interface for the Log struct. It marshals the Log struct to JSON
// and includes any extra fields from the Extra map without overwriting existing fields.
func (l Log) MarshalJSON() ([]byte, error) {
	// Avoid recursion
	type Log_ Log
	b, _ := json.Marshal(Log_(l))

	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)

	for k, v := range l.Extra {
		// Don't overwrite existing fields with extra fields
		if _, ok := m[k]; ok {
			continue
		}
		b, _ := json.Marshal(v)
		m[k] = b
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements the json.Unmarshaler interface for the Log struct. It unmarshals JSON data into the Log
// struct and extracts any extra fields into the Extra map without overwriting existing fields.
func (l *Log) UnmarshalJSON(data []byte) error {
	type Log_ Log

	log := &struct {
		*Log_
	}{
		Log_: (*Log_)(l),
	}

	// Unmarshal the known fields into the Log struct
	if err := json.Unmarshal(data, &log); err != nil {
		return err
	}

	// Unmarshal the JSON into a map to extract the extra fields
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	// Remove known fields from the map to get the extra fields
	delete(m, "name")
	delete(m, "levelname")
	delete(m, "message")
	delete(m, "identifier")
	delete(m, "@timestamp")
	delete(m, "disableUserLog")

	l.Extra = make(map[string]any)
	for k, v := range m {
		var value any
		if err := json.Unmarshal(v, &value); err != nil {
			return err
		}
		l.Extra[k] = value
	}

	return nil
}
