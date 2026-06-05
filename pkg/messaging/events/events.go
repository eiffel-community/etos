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
	"fmt"
)

type Event interface {
	EventType() EventType
	EventData() any
	EventMeta() string
}

type BareEvent struct {
	ID    int       `json:"id,omitempty"`
	Event EventType `json:"event"`
	Data  any       `json:"data"`
}

type ValidationError struct {
	Message string
}

// Error returns the error message of the ValidationError.
func (e ValidationError) Error() string {
	return e.Message
}

// Parse takes a BareEvent and attempts to parse it into a specific Event type based on the EventType.
func Parse(event BareEvent) (Event, error) {
	switch event.Event {
	case MessageType:
		return parseEvent[Message](event)
	case ArtifactType:
		return parseEvent[Artifact](event)
	case ReportType:
		return parseEvent[Report](event)
	case ShutdownType:
		return parseEvent[Shutdown](event)
	case StatusType:
		return parseEvent[Status](event)
	case PingType:
		return parseEvent[Ping](event)
	default:
		return nil, &ValidationError{Message: fmt.Sprintf("unsupported event type: %s", event.Event)}
	}
}

// parseEvent is a generic function that takes a BareEvent and attempts to unmarshal it into the specified Event type T.
func parseEvent[T Event](event BareEvent) (T, error) {
	var result T
	jsonData, err := json.Marshal(event)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		return result, err
	}
	return result, nil
}
