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

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type logPublisher struct{ messages []events.Event }

// Publish captures an event for assertions on user-log routing.
func (p *logPublisher) Publish(_ string, e events.Event) error {
	p.messages = append(p.messages, e)
	return nil
}

// Close succeeds without releasing any external resources.
func (*logPublisher) Close() error { return nil }

// AddLogger ignores logger configuration for the in-memory publisher.
func (*logPublisher) AddLogger(logr.Logger) {}

// TestContextNeverEntersConsoleOrUserLog verifies context filtering, correlation fields and user-log suppression.
func TestContextNeverEntersConsoleOrUserLog(t *testing.T) {
	var console bytes.Buffer
	publisher := &logPublisher{}
	ctx := New(zap.Options{DestWriter: &console}).WithConsole().WithUserLog(publisher).Start(context.Background())
	traceID, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID})
	ctx = trace.ContextWithSpanContext(ctx, spanContext)
	logger := FromContextOrDiscard(ctx).WithValues("identifier", "user-log-id")
	logger.Info("Visible user message", "duration_seconds", 0.25)
	logger.Info("Provider provisioning completed", "event", "etos.provider.completed", "disableUserLog", true)
	if len(publisher.messages) != 1 {
		t.Fatalf("user log filtering changed: %d", len(publisher.messages))
	}
	decoder := json.NewDecoder(&console)
	for range 2 {
		var entry map[string]any
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}
		if _, exists := entry["otel.context"]; exists {
			t.Fatal("context leaked into console JSON")
		}
		if entry["trace_id"] != traceID.String() || entry["span_id"] != spanID.String() {
			t.Fatal("legacy correlation fields lost")
		}
	}
	encoded, err := json.Marshal(publisher.messages[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("otel.context")) {
		t.Fatal("context leaked into user log")
	}
}
