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
	"strings"
	"testing"
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
			input:    "My-IUT",
			expected: "my-iut",
		},
		{
			name:     "input with invalid characters",
			input:    "my_@#%&*()Iut!",
			expected: "my-iut",
		},
		{
			name:     "input with leading and trailing hyphens",
			input:    "-my-iut-",
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
			name:     "input exceeding 253 characters",
			input:    strings.Repeat("a", 255),
			expected: strings.Repeat("a", 253),
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
			result := toRFC1123(tt.input)
			if result != tt.expected {
				t.Errorf("toRFC1123(%q) = %q; expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
