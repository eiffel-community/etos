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

import apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

// TestSuite defines one or more test suites with their test executions, environments, and dependencies.
type TestSuite struct {
	// Name is the name of the suite definition.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// SchemaVersion is the version of the test run schema.
	// This field is used to determine which version of the test run schema to use when creating a test run.
	// +optional
	// +kubebuilder:default="v1beta1"
	// +kubebuilder:validation:Enum=v1beta1
	// +kubebuilder:validation:MinLength=1
	SchemaVersion string `json:"schemaVersion"`

	// Suites is the list of test suites to run.
	// +required
	// +kubebuilder:validation:MinItems=1
	Suites []Suite `json:"suites"`
}

// Suite defines a single test suite contining prioritized test executions.
type Suite struct {
	// Priority is the execution priority when multiple suites are defined.
	// Lower values indicate higher priority. Suites with the same priority may be executed in any order.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Priority int `json:"priority"`

	// TestExecutions is the list of test executions to run in this suite.
	// +required
	// +kubebuilder:validation:MinItems=1
	TestExecutions []TestExecution `json:"testExecutions"`
}

// TestExecution defines a single test execution combining a test case, its execution instructions,
// and environment requirements.
type TestExecution struct {
	// ID is the unique identifier for this test execution as UUID.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	ID string `json:"id"`

	// Dependencies is an optional list of test execution IDs that must complete before this test can run.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	Dependencies []string `json:"dependencies,omitempty"`

	// TestCase defines metadata about a test case. The repository and version can be used to check out the test
	// source code.
	// +required
	TestCase TestCase `json:"testCase"`

	// Execution defines instructions for how to execute a test case.
	// +required
	Execution Execution `json:"execution"`

	// Environment defines configuration for a test execution, including the test runner image, environment variables,
	// and additional provider-defined resources.
	Environment TestEnvironment `json:"environment"`
}

// TestCase defines metadata about a test case. The repository and version can be used to check out the test
// source code.
type TestCase struct {
	// ID defines the fully qualified test case identifier, e.g
	// 'tests/integration/test_api.py::TestAPI::test_create_resource'
	// +required
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// Version defines the version of the test case, typically a Git commit SHA or branch name.
	// +optional
	Version string `json:"version,omitempty"`

	// Repository defines the repository URL where the test case source code can be found, e.g. a Git repository URL.
	// +optional
	Repository string `json:"repository,omitempty"`

	// URI defines an optional URI to the test case documentation or external reference for a test case.
	// +optional
	URI string `json:"uri,omitempty"`
}

// Execution defines instructions for how to execute a test case.
type Execution struct {
	// Command defines the shell command to execute the test, e.g. a pytest invocation.
	// +required
	Command string `json:"command"`

	// Checkout defines shell commands to check out the source code. Executed in order before the test command.
	// +optional
	Checkout []string `json:"checkout"`

	// PreExecution defines shell commands to run after checkout but before the main test command
	// (e.g. setup steps, dependency installation).
	// +optional
	PreExecution []string `json:"preExecution,omitempty"`
}

// TestEnvironment defines configuration for a test execution, including the test runner image, environment variables,
// and additional provider-defined resources.
type TestEnvironment struct {

	// TestRunner defines the container image to use as the test runner, e.g.
	// 'ghcr.io/eiffel-community/etos-base-test-runner:latest'.
	// +required
	TestRunner string `json:"testRunner"`

	// EnvironmentVariables defines keyt-value pairs of environment variables to set in the test runner.
	// +optional
	EnvironmentVariables map[string]string `json:"environmentVariables"`

	// AdditionalResources defines provider-defined resources required by the test beyond the primary IUT (Item Under
	// Test), e.g. a sidecar containers or additional hardware. Each items fields are open-ended and interpreted by
	// the provider responsible for that resource; not every field needs to be defined in this schema.
	// +optional
	AdditionalResources []AdditionalResource `json:"additionalResources,omitempty"`
}

// AdditionalResource defines a single provider-defined resource required by a test, such as a sidecar container or
// additional hardware. Aside from the 'type' discriminator, fields are open-ended and validated by the provider
// that handles the resource, not by this schema.
type AdditionalResource struct {
	// Type defines a provider-defined resource type used to select the provider that handles this resource, e.g.
	// 'device' or 'container'. The set of valid values is defined by the available providers, not by this schema.
	// +required
	Type string `json:"type"`

	*apiextensionsv1.JSON `json:",inline"`
}
