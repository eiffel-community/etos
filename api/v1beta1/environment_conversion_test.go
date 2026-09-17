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

package v1beta1

import (
	"testing"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// TestEnvironmentConvertFromNilProviders verifies conversion handles missing optional Providers.
func TestEnvironmentConvertFromNilProviders(t *testing.T) {
	dst := &Environment{
		Spec: EnvironmentSpec{
			Providers: Providers{IUT: "stale-provider"},
		},
	}

	if err := dst.ConvertFrom(&etosv1alpha1.Environment{}); err != nil {
		t.Fatalf("ConvertFrom() error = %v", err)
	}

	if dst.Spec.Providers != (Providers{}) {
		t.Errorf("Providers = %#v, want zero value", dst.Spec.Providers)
	}
}
