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

package main

import "testing"

// TestResolveLiveLogsURL resolves the TestRun identifier placeholder and preserves literal URLs.
func TestResolveLiveLogsURL(t *testing.T) {
	tests := []struct {
		name       string
		liveLogs   string
		identifier string
		want       string
		wantErr    bool
	}{
		{
			name:       "resolves TestRun ID placeholder",
			liveLogs:   "http://cluster-etos-sse/sse/v2alpha/events/$testrunid",
			identifier: "a4c10222-1937-4f71-9fd0-9a74da503c91",
			want:       "http://cluster-etos-sse/sse/v2alpha/events/a4c10222-1937-4f71-9fd0-9a74da503c91",
		},
		{
			name:     "preserves URL without placeholder",
			liveLogs: "http://cluster-etos-sse/sse/v2alpha/events/static-id",
			want:     "http://cluster-etos-sse/sse/v2alpha/events/static-id",
		},
		{
			name:     "rejects missing TestRun ID for placeholder",
			liveLogs: "http://cluster-etos-sse/sse/v2alpha/events/$testrunid",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLiveLogsURL(tt.liveLogs, tt.identifier)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveLiveLogsURL() error = %v, wantErr %t", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("resolveLiveLogsURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
