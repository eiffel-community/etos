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

import (
	"encoding/json"
	"testing"
)

// TestLogMarshalEmitsLevel verifies that a Log is serialized with the canonical
// 'level' field name on the message bus (not the 'levelname' input alias).
func TestLogMarshalEmitsLevel(t *testing.T) {
	log := Log{Name: "etos", Level: "info", Message: "hello", Timestamp: "2026-09-03T10:00:00Z"}
	b, err := json.Marshal(log)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["level"]; !ok {
		t.Errorf("expected 'level' key in %s", b)
	}
	if _, ok := m["levelname"]; ok {
		t.Errorf("did not expect 'levelname' key in %s", b)
	}
}

// TestLogUnmarshalAcceptsLevelnameAlias verifies that a Log produced by the zap
// encoder (which emits 'levelname') still populates Level and does not leak the
// alias into Extra.
func TestLogUnmarshalAcceptsLevelnameAlias(t *testing.T) {
	data := []byte(`{"name":"etos","levelname":"info","message":"hello","@timestamp":"2026-09-03T10:00:00Z"}`)
	var log Log
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatal(err)
	}
	if log.Level != "info" {
		t.Errorf("expected Level 'info' from levelname alias, got %q", log.Level)
	}
	if _, ok := log.Extra["levelname"]; ok {
		t.Errorf("'levelname' should not be captured in Extra")
	}
}

// TestLogUnmarshalAcceptsLevel verifies that a Log serialized with 'level'
// round-trips back into Level.
func TestLogUnmarshalAcceptsLevel(t *testing.T) {
	data := []byte(`{"name":"etos","level":"info","message":"hello","@timestamp":"2026-09-03T10:00:00Z"}`)
	var log Log
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatal(err)
	}
	if log.Level != "info" {
		t.Errorf("expected Level 'info', got %q", log.Level)
	}
	if _, ok := log.Extra["level"]; ok {
		t.Errorf("'level' should not be captured in Extra")
	}
}
