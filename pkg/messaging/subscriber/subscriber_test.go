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
package subscriber

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	events "github.com/eiffel-community/etos/pkg/messaging/events"
)

const (
	testStatus   = `{"name":"test-runner","instance":"etr","status":"running"}`
	testShutdown = `{"conclusion":"Successful","verdict":"Passed","description":"done"}`
)

// fakeSSEServer serves one response body per connection and records the Last-Event-ID
// header of each connection. Connections after the last response get an empty stream.
type fakeSSEServer struct {
	mu           sync.Mutex
	responses    []string
	lastEventIDs []string
}

// ServeHTTP writes the next response as an event stream.
func (f *fakeSSEServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	f.lastEventIDs = append(f.lastEventIDs, request.Header.Get("Last-Event-ID"))
	response := ""
	if len(f.responses) > 0 {
		response = f.responses[0]
		f.responses = f.responses[1:]
	}
	f.mu.Unlock()
	writer.Header().Set("Content-Type", "text/event-stream")
	_, _ = writer.Write([]byte(response))
}

// headers returns the Last-Event-ID headers received so far.
func (f *fakeSSEServer) headers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.lastEventIDs...)
}

// sseEvent formats an SSE event. The id line is omitted when id is 0.
func sseEvent(id int, event, data string) string {
	if id == 0 {
		return fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	}
	return fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", id, event, data)
}

// collect runs the subscriber against the fake server until it stops, a shutdown is received or
// the test times out, and returns the event types it yielded and the errors.
func collect(t *testing.T, fake *fakeSSEServer) ([]events.EventType, []error) {
	t.Helper()
	previousDelay, previousMaxDelay := reconnectDelay, maxReconnectDelay
	reconnectDelay, maxReconnectDelay = time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { reconnectDelay, maxReconnectDelay = previousDelay, previousMaxDelay })

	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var types []events.EventType
	var errs []error
	for event, err := range NewSSESubscriber(server.URL).Events(ctx, "id") {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		types = append(types, event.EventType())
		if event.EventType() == events.ShutdownType {
			break
		}
	}
	if ctx.Err() != nil {
		t.Fatalf("subscriber did not stop before the test timeout")
	}
	return types, errs
}

// equal reports whether two slices are equal.
func equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestEventsAcceptsSparseIDs verifies that event IDs only need to be strictly increasing.
func TestEventsAcceptsSparseIDs(t *testing.T) {
	fake := &fakeSSEServer{responses: []string{
		sseEvent(3, "status", testStatus) + sseEvent(4711, "status", testStatus) + sseEvent(4800, "shutdown", testShutdown),
	}}
	types, errs := collect(t, fake)
	want := []events.EventType{events.StatusType, events.StatusType, events.ShutdownType}
	if !equal(types, want) || len(errs) != 0 {
		t.Fatalf("got types %v and errors %v, want %v", types, errs, want)
	}
}

// TestEventsReconnectsWithLastEventID verifies that the subscriber reconnects with the
// Last-Event-ID header when the stream closes and drops events that it has already received.
func TestEventsReconnectsWithLastEventID(t *testing.T) {
	fake := &fakeSSEServer{responses: []string{
		sseEvent(1, "status", testStatus) + sseEvent(9, "status", testStatus),
		sseEvent(9, "status", testStatus) + sseEvent(5, "status", testStatus) + sseEvent(12, "shutdown", testShutdown),
	}}
	types, errs := collect(t, fake)
	want := []events.EventType{events.StatusType, events.StatusType, events.ShutdownType}
	if !equal(types, want) || len(errs) != 0 {
		t.Fatalf("got types %v and errors %v, want %v", types, errs, want)
	}
	if headers := fake.headers(); !equal(headers, []string{"", "9"}) {
		t.Fatalf("got Last-Event-ID headers %q, want [\"\" \"9\"]", headers)
	}
}

// TestEventsPingUpdatesProgress verifies that ping IDs update the Last-Event-ID used when
// reconnecting, that lower ping IDs are ignored and that pings are not yielded.
func TestEventsPingUpdatesProgress(t *testing.T) {
	fake := &fakeSSEServer{responses: []string{
		sseEvent(1, "status", testStatus) + sseEvent(50, "ping", "") + sseEvent(0, "ping", "") + sseEvent(20, "ping", ""),
		sseEvent(60, "shutdown", testShutdown),
	}}
	types, errs := collect(t, fake)
	want := []events.EventType{events.StatusType, events.ShutdownType}
	if !equal(types, want) || len(errs) != 0 {
		t.Fatalf("got types %v and errors %v, want %v", types, errs, want)
	}
	if headers := fake.headers(); !equal(headers, []string{"", "50"}) {
		t.Fatalf("got Last-Event-ID headers %q, want [\"\" \"50\"]", headers)
	}
}

// TestEventsRetryableErrorReconnects verifies that a retryable error event, and an error event
// without a retry hint, make the subscriber reconnect.
func TestEventsRetryableErrorReconnects(t *testing.T) {
	fake := &fakeSSEServer{responses: []string{
		sseEvent(1, "status", testStatus) +
			sseEvent(0, "error", `{"retry":true,"reason":"closed"}`) +
			sseEvent(2, "status", testStatus),
		sseEvent(0, "error", "oops"),
		sseEvent(3, "shutdown", testShutdown),
	}}
	types, errs := collect(t, fake)
	want := []events.EventType{events.StatusType, events.ShutdownType}
	if !equal(types, want) || len(errs) != 0 {
		t.Fatalf("got types %v and errors %v, want %v", types, errs, want)
	}
	if headers := fake.headers(); !equal(headers, []string{"", "1", "1"}) {
		t.Fatalf("got Last-Event-ID headers %q, want [\"\" \"1\" \"1\"]", headers)
	}
}

// TestEventsNonRetryableErrorStops verifies that a non-retryable error event is yielded as a
// *ServerError and stops the iteration without reconnecting.
func TestEventsNonRetryableErrorStops(t *testing.T) {
	fake := &fakeSSEServer{responses: []string{
		sseEvent(1, "status", testStatus) + sseEvent(0, "error", `{"retry":false,"reason":"events expired"}`),
		sseEvent(2, "shutdown", testShutdown),
	}}
	types, errs := collect(t, fake)
	if !equal(types, []events.EventType{events.StatusType}) || len(errs) != 1 {
		t.Fatalf("got types %v and errors %v, want one status and one error", types, errs)
	}
	var serverError *ServerError
	if !errors.As(errs[0], &serverError) || serverError.Retry || !strings.Contains(serverError.Reason, "expired") {
		t.Fatalf("got error %v, want a non-retryable ServerError", errs[0])
	}
	if headers := fake.headers(); len(headers) != 1 {
		t.Fatalf("got %d connections, want 1", len(headers))
	}
}

// TestEventsStopsAfterEmptyReconnects verifies that ErrStreamClosed is yielded when the stream
// repeatedly closes without delivering anything.
func TestEventsStopsAfterEmptyReconnects(t *testing.T) {
	fake := &fakeSSEServer{}
	types, errs := collect(t, fake)
	if len(types) != 0 || len(errs) != 1 || !errors.Is(errs[0], ErrStreamClosed) {
		t.Fatalf("got types %v and errors %v, want only ErrStreamClosed", types, errs)
	}
	if headers := fake.headers(); len(headers) != maxEmptyReconnects {
		t.Fatalf("got %d connections, want %d", len(headers), maxEmptyReconnects)
	}
}

// TestEventsDecodingErrorStops verifies that an event that cannot be decoded is yielded as an
// error and stops the iteration.
func TestEventsDecodingErrorStops(t *testing.T) {
	fake := &fakeSSEServer{responses: []string{sseEvent(1, "unknown", "{}"), sseEvent(2, "shutdown", testShutdown)}}
	types, errs := collect(t, fake)
	if len(types) != 0 || len(errs) != 1 {
		t.Fatalf("got types %v and errors %v, want one error", types, errs)
	}
}

// TestEventsTransportErrorDoesNotPanic verifies that a transport error (e.g. connection
// refused) while connecting to the SSE endpoint is returned as an error instead of causing
// a nil pointer dereference panic.
func TestEventsTransportErrorDoesNotPanic(t *testing.T) {
	// Bind to a port and immediately close it so connections to it are refused.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	subscriber := NewSSESubscriber("http://" + addr)

	// Bound the retry loop so the test does not wait for the full retry interval.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Events panicked: %v", r)
		}
	}()

	for _, err := range subscriber.Events(ctx, "some-id") {
		if err == nil {
			t.Fatalf("expected an error when the SSE endpoint is unreachable")
		}
		break
	}
}

// TestEventsKeepsRetryingRetryableErrors verifies that retryable server errors do not count
// towards the limit of connections without progress.
func TestEventsKeepsRetryingRetryableErrors(t *testing.T) {
	retryable := sseEvent(0, "error", `{"retry":true,"reason":"failed to consume the event stream"}`)
	responses := make([]string, 0, 3*maxEmptyReconnects+1)
	for range 3 * maxEmptyReconnects {
		responses = append(responses, retryable)
	}
	fake := &fakeSSEServer{responses: append(responses, sseEvent(1, "shutdown", testShutdown))}
	types, errs := collect(t, fake)
	if !equal(types, []events.EventType{events.ShutdownType}) || len(errs) != 0 {
		t.Fatalf("got types %v and errors %v, want one shutdown", types, errs)
	}
}

// TestEventsLargeIDs verifies that event IDs beyond the 32-bit range, as stream offsets may be,
// are accepted and sent back as Last-Event-ID.
func TestEventsLargeIDs(t *testing.T) {
	fake := &fakeSSEServer{responses: []string{
		sseEvent(0, "status", testStatus) + "id: 4294967296000\nevent: status\ndata: " + testStatus + "\n\n",
		"id: 4294967296001\nevent: shutdown\ndata: " + testShutdown + "\n\n",
	}}
	types, errs := collect(t, fake)
	want := []events.EventType{events.StatusType, events.StatusType, events.ShutdownType}
	if !equal(types, want) || len(errs) != 0 {
		t.Fatalf("got types %v and errors %v, want %v", types, errs, want)
	}
	if headers := fake.headers(); !equal(headers, []string{"", "4294967296000"}) {
		t.Fatalf("got Last-Event-ID headers %q, want [\"\" \"4294967296000\"]", headers)
	}
}
