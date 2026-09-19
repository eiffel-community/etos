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
	"net"
	"testing"
	"time"
)

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
