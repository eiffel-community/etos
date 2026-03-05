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
package provider

import (
	"encoding/json"
	"fmt"

	"github.com/eiffel-community/etos/internal/messaging"
	"github.com/go-logr/logr"
)

type userLogs struct {
	logger    logr.Logger
	publisher messaging.Publisher
}

// newUserLogWriter creates a new userLogs instance.
func newUserLogWriter(publisher messaging.Publisher) *userLogs {
	return &userLogs{publisher: publisher, logger: logr.Discard()}
}

type entry struct {
	Message        string `json:"message"`
	Level          string `json:"levelname"`
	Identifier     string `json:"identifier"`
	DisableUserLog bool   `json:"disableUserLog"`
}

type event struct {
	Event string         `json:"event"`
	Data  map[string]any `json:"data"`
}

// setLogger sets the logger for the userLogs instance.
func (u *userLogs) setLogger(logger logr.Logger) {
	logger = logger.WithValues("disableUserLog", true)
	u.logger = logger
}

// Write implements the io.Writer interface for userLogs. It unmarshals the log entry and
// publishes it to the ETOS messagebus if it contains an identifier and is not disabled for
// user logs.
func (u *userLogs) Write(p []byte) (n int, err error) {
	e := entry{}
	if err := json.Unmarshal(p, &e); err != nil {
		return 0, fmt.Errorf("failed to unmarshal log entry: %w", err)
	}
	if !u.shouldLog(e) {
		return len(p), nil
	}
	message, err := u.formatLog(p)
	if err != nil {
		return 0, fmt.Errorf("failed to format log entry: %w", err)
	}
	routingKey := fmt.Sprintf("%s.log.%s", e.Identifier, e.Level)
	if err := u.publisher.Publish(message, routingKey); err != nil {
		return 0, fmt.Errorf("failed to publish log entry: %w", err)
	}
	return len(p), nil
}

// formatLog formats the log entry by unmarshaling it into a map and then marshaling it back
// into JSON with an "event" field.
func (u *userLogs) formatLog(p []byte) ([]byte, error) {
	data := map[string]any{}
	if err := json.Unmarshal(p, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal log entry into map: %w", err)
	}
	event := event{Event: "message", Data: data}
	return json.Marshal(event)
}

// shouldLog determines whether a log entry should be published to the ETOS messagebus.
// It checks if the entry has an identifier and is not disabled for user logs.
func (u *userLogs) shouldLog(e entry) bool {
	return !e.DisableUserLog && e.Identifier != ""
}
