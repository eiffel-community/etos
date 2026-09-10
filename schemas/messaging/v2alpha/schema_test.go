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
package v2alpha

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidateValidExamples verifies that every valid example fixture is accepted.
func TestValidateValidExamples(t *testing.T) {
	files, err := filepath.Glob("examples/valid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no valid example fixtures found")
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(data); err != nil {
				t.Errorf("valid example was rejected: %v", err)
			}
		})
	}
}

// TestValidateInvalidExamples verifies that every invalid example fixture is rejected.
func TestValidateInvalidExamples(t *testing.T) {
	files, err := filepath.Glob("examples/invalid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no invalid example fixtures found")
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(data); err == nil {
				t.Error("invalid example was accepted")
			}
		})
	}
}
