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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/pkg/logging"
	"go.jetify.com/sse"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace/noop"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// TestProvisioningCompletionSeparatesClusters verifies distinct completion-log attribution
// for identically named clusters.
func TestProvisioningCompletionSeparatesClusters(t *testing.T) {
	for _, identity := range []string{"first/shared", "second/shared"} {
		t.Run(identity, func(t *testing.T) {
			request := &v1alpha1.EnvironmentRequest{ObjectMeta: metav1.ObjectMeta{
				Namespace: strings.Split(identity, "/")[0],
				Labels:    map[string]string{"etos.eiffel-community.github.io/cluster": "shared"},
			}}
			var output bytes.Buffer
			ctx := logging.New(zap.Options{DestWriter: &output}).WithConsole().Start(context.Background())
			member, err := baggage.NewMemberRaw("etos_cluster", providerCluster(request))
			if err != nil {
				t.Fatal(err)
			}

			value, err := baggage.New(member)
			if err != nil {
				t.Fatal(err)
			}
			ctx = baggage.ContextWithBaggage(ctx, value)
			now := time.Now()
			recordProvisioningCompletion(ctx, "Iut", request, now, now.Add(time.Second), nil)
			var entry map[string]any
			if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry["etos_cluster"] != identity {
				t.Fatalf("unexpected completion ownership: %v", entry)
			}
		})
	}
}

// TestProviderClusterUsesRequestMetadata verifies validated request-based attribution
// rather than stale environment values.
func TestProviderClusterUsesRequestMetadata(t *testing.T) {
	t.Setenv("ETOS_CLUSTER", "other/stale")
	for _, test := range []struct {
		name, cluster, namespace, want string
	}{
		{"first cluster", "shared", "first", "first/shared"},
		{"second cluster", "shared", "second", "second/shared"},
		{"dotted CR name", "shared.cluster", "first", "first/shared.cluster"},
		{"missing cluster", "", "first", unassignedCluster},
		{"missing namespace", "shared", "", unassignedCluster},
		{"qualified label", "second/shared", "first", unassignedCluster},
		{"invalid namespace", "shared", "first.invalid", unassignedCluster},
		{"invalid CR name", "Shared", "first", unassignedCluster},
		{"whitespace", "shared ", "first", unassignedCluster},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := &v1alpha1.EnvironmentRequest{ObjectMeta: metav1.ObjectMeta{
				Namespace: test.namespace,
				Labels:    map[string]string{"etos.eiffel-community.github.io/cluster": test.cluster},
			}}
			if test.cluster == "" {
				request.Labels = nil
			}
			if got := providerCluster(request); got != test.want {
				t.Fatalf("identity = %q, want %q", got, test.want)
			}
		})
	}
}

type completionTestProvider struct {
	err error
}

// Provision returns the configured result without creating resources.
func (p completionTestProvider) Provision(context.Context, ProvisionConfig) error {
	return p.err
}

// Release returns without creating a provisioning observation.
func (p completionTestProvider) Release(context.Context, ReleaseConfig) error {
	return nil
}

// TestRunProviderEmitsCompletionOnce verifies integration for success, failure and release paths.
func TestRunProviderEmitsCompletionOnce(t *testing.T) {
	for _, test := range []struct {
		name    string
		err     error
		release bool
	}{
		{name: "success"},
		{name: "failure", err: errors.New("provider failed")},
		{name: "release", release: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			ctx := logging.New(zap.Options{DestWriter: &output}).WithConsole().Start(context.Background())
			request := &v1alpha1.EnvironmentRequest{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Second))},
				Spec:       v1alpha1.EnvironmentRequestSpec{Deadline: time.Now().Add(time.Minute).Unix()},
			}
			params := Parameters{
				providerType: "Environment", name: "test", releaseEnvironment: test.release,
				tracer: noop.NewTracerProvider().Tracer("test"),
				amountFunc: func(context.Context, *v1alpha1.EnvironmentRequest) (int, error) {
					return 1, nil
				},
			}
			err := runProvider(ctx, completionTestProvider{err: test.err}, params, request)
			if !errors.Is(err, test.err) {
				t.Fatalf("result = %v, want %v", err, test.err)
			}
			want := 1
			if test.release {
				want = 0
			}
			if got := strings.Count(output.String(), `"event":"etos.provider.completed"`); got != want {
				t.Fatalf("completion count = %d, want %d: %s", got, want, output.String())
			}
		})
	}
}

// TestProvisioningCompletionRecordsProviderAndRequestTiming verifies one structured completion per attempt.
func TestProvisioningCompletionRecordsProviderAndRequestTiming(t *testing.T) {
	created := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	request := &v1alpha1.EnvironmentRequest{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(created)}}
	for _, test := range []struct {
		name         string
		providerType string
		err          error
		wantReady    bool
	}{
		{name: "environment success", providerType: "Environment", wantReady: true},
		{name: "environment failure", providerType: "Environment", err: errors.New("provision failed")},
		{name: "iut success", providerType: "Iut"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			ctx := logging.New(zap.Options{DestWriter: &output}).WithConsole().Start(context.Background())
			recordProvisioningCompletion(
				ctx, test.providerType, request, created.Add(10*time.Second), created.Add(15*time.Second), test.err,
			)
			var entry map[string]any
			if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry["event"] != "etos.provider.completed" ||
				entry["duration_seconds"] != float64(5) || entry["disableUserLog"] != true {
				t.Fatalf("unexpected completion: %v", entry)
			}
			ready, found := entry["request_to_ready_seconds"]
			if found != test.wantReady || (found && ready != float64(15)) {
				t.Fatalf("unexpected request-to-ready timing: %v", entry)
			}
			wantOutcome := "success"
			if test.err != nil {
				wantOutcome = "failure"
			}
			if entry["outcome"] != wantOutcome {
				t.Fatalf("outcome = %v, want %s", entry["outcome"], wantOutcome)
			}
		})
	}
}

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
