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
package config

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestVersionFromImage verifies that the tag is extracted from image references, ignoring registry ports and digests, and that missing or invalid tags give an empty version.
func TestVersionFromImage(t *testing.T) {
	tests := map[string]string{
		"ghcr.io/eiffel-community/etos-api:20b851eb": "20b851eb",
		"localhost:5000/etos-api:latest":             "latest",
		"localhost:5000/etos-api":                    "",
		"etos-api":                                   "",
		"repo/etos-api:1.0@sha256:abcdef":            "1.0",
		"repo/etos-api@sha256:abcdef":                "",
		"repo/etos-api:-invalid-":                    "",
	}
	for image, want := range tests {
		if got := VersionFromImage(image); got != want {
			t.Errorf("VersionFromImage(%q) = %q, want %q", image, got, want)
		}
	}
}

// TestSetVersionLabel verifies that the version label is only set when the image has a tag.
func TestSetVersionLabel(t *testing.T) {
	meta := metav1.ObjectMeta{}
	SetVersionLabel(&meta, "repo/app")
	if len(meta.Labels) != 0 {
		t.Errorf("expected no labels, got %v", meta.Labels)
	}
	SetVersionLabel(&meta, "repo/app:20b851eb")
	if meta.Labels[VersionLabel] != "20b851eb" {
		t.Errorf("unexpected labels %v", meta.Labels)
	}
}
