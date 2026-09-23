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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"go.jetify.com/sse"
)

// TestToRFC1123 tests the toRFC1123 function with various input cases to ensure it correctly converts
// strings to RFC 1123 compliant format.
func TestToRFC1123(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid input",
			input:    "my-iut",
			expected: "my-iut",
		},
		{
			name:     "input with uppercase letters",
			input:    "My----IUT",
			expected: "my-iut",
		},
		{
			name:     "input with invalid characters",
			input:    "my_@#%&*()Iut!",
			expected: "my-iut",
		},
		{
			name:     "input with leading and trailing hyphens",
			input:    "-my-iut--",
			expected: "my-iut",
		},
		{
			name:     "input with multiple spaces",
			input:    "my   iut",
			expected: "my-iut",
		},
		{
			name:     "input with invalid character at end",
			input:    "my-iut!",
			expected: "my-iut",
		},
		{
			name:     "input with invalid character at start",
			input:    "@my-iut",
			expected: "my-iut",
		},
		{
			name:     "input with mixed case",
			input:    "MyIUTName",
			expected: "myiutname",
		},
		{
			name:     "input exceeding 63 characters",
			input:    strings.Repeat("a", 65),
			expected: strings.Repeat("a", 63),
		},
		{
			name:     "input single word",
			input:    "iut",
			expected: "iut",
		},
		{
			name:     "input with only numbers",
			input:    "123",
			expected: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toRFC1123(tt.input, 63)
			if result != tt.expected {
				t.Errorf("toRFC1123(%q) = %q; expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestWaitForTestRunnerReturnsErrorsForMissingOrFailedStatus verifies that readiness cannot succeed
// when the SSE stream ends before a status or when the matching Test Runner reports failure.
func TestWaitForTestRunnerReturnsErrorsForMissingOrFailedStatus(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantError string
	}{
		{
			name:      "missing status",
			wantError: "event stream closed",
		},
		{
			name:      "failure status",
			payload:   `{"instance":"etr-instance","status":"error","message":"startup failed"}`,
			wantError: "test runner reported an error status: startup failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				if tt.payload == "" {
					return
				}
				if err := sse.NewEncoder(writer).EncodeEvent(&sse.Event{
					Event: "status",
					Data:  sse.Raw(tt.payload),
				}); err != nil {
					t.Errorf("encoding SSE status event: %v", err)
				}
				writer.(http.Flusher).Flush()
			}))
			defer server.Close()

			executionSpace := &ExecutionSpace{ExecutionSpace: &v1alpha2.ExecutionSpace{
				Spec: v1alpha2.ExecutionSpaceSpec{
					Instructions: v1alpha2.Instructions{Environment: map[string]string{"ENVIRONMENT_ID": "etr-instance"}},
				},
			}}
			environmentRequest := &v1alpha1.EnvironmentRequest{
				Spec: v1alpha1.EnvironmentRequestSpec{
					Identifier: "test-identifier",
					Config:     v1alpha1.EnvironmentProviderJobConfig{EtosSse: server.URL},
				},
			}

			err := executionSpace.WaitForTestRunner(context.Background(), environmentRequest)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("WaitForTestRunner() error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}
}
