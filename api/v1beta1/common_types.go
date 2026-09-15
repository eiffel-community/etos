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

// Providers defines the providers to use for test execution. These names must correspond to existing
// Provider kinds in the namespace where a testrun is created.
type Providers struct {
	// IUT defines the provider to use for item under test provisioning.
	// +optional
	IUT string `json:"iut,omitempty"`

	// LogArea defines the provider to use for log area provisioning.
	// +optional
	LogArea string `json:"logArea"`

	// ExecutionSpace defines the provider to use for execution space provisioning.
	// +optional
	ExecutionSpace string `json:"executionSpace"`
}
