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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Providers to use for test execution. These names must correspond to existing
// Provider kinds in the namespace where a testrun is created.
type Providers struct {
	IUT            string `json:"iut"`
	LogArea        string `json:"logArea"`
	ExecutionSpace string `json:"executionSpace"`
}

// TestCase metadata.
type TestCase struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
	Tracker string `json:"tracker,omitempty"`
	URI     string `json:"uri,omitempty"`
}

// Execution describes how to execute a testCase.
type Execution struct {
	Checkout    []string          `json:"checkout"`
	Parameters  map[string]string `json:"parameters"`
	Environment map[string]string `json:"environment"`
	Command     string            `json:"command"`
	Execute     []string          `json:"execute,omitempty"`
	TestRunner  string            `json:"testRunner"`
}

// TestEnvironment to run tests within.
type TestEnvironment struct{}

type Test struct {
	ID          string          `json:"id"`
	TestCase    TestCase        `json:"testCase"`
	Execution   Execution       `json:"execution"`
	Environment TestEnvironment `json:"environment"`
}

// Suite to execute.
type Suite struct {
	// Name of the test suite.
	Name string `json:"name"`

	// Priority to execute the test suite.
	// +kubebuilder:default=1
	Priority int `json:"priority"`

	// Tests to execute as part of this suite.
	Tests []Test `json:"tests"`

	// Dataset for this suite.
	Dataset *apiextensionsv1.JSON `json:"dataset"`
}

type TestRunner struct {
	Version string `json:"version"`
}

type SuiteRunner struct {
	*Image `json:",inline"`
}

type LogListener struct {
	*Image `json:",inline"`
}

type EnvironmentProvider struct {
	*Image `json:",inline"`
}

// Retention describes the failure and success retentions for testruns.
type Retention struct {
	// +optional
	Failure *metav1.Duration `json:"failure,omitempty"`
	// +optional
	Success *metav1.Duration `json:"success,omitempty"`
}

// TestRunSpec defines the desired state of TestRun
type TestRunSpec struct {
	// Name of the ETOS cluster to execute the testrun in.
	Cluster string `json:"cluster,omitempty"`

	// ID is the test suite ID for this execution. Will be generated if nil
	// +kubebuilder:validation:Pattern="[a-f0-9]{8}-?[a-f0-9]{4}-?4[a-f0-9]{3}-?[89ab][a-f0-9]{3}-?[a-f0-9]{12}"
	ID string `json:"id,omitempty"`

	// Artifact is the ID of the software under test.
	// +kubebuilder:validation:Pattern="[a-f0-9]{8}-?[a-f0-9]{4}-?4[a-f0-9]{3}-?[89ab][a-f0-9]{3}-?[a-f0-9]{12}"
	Artifact string `json:"artifact"`

	// +optional
	Retention Retention `json:"retention,omitempty"`

	SuiteRunner         *SuiteRunner         `json:"suiteRunner,omitempty"`
	TestRunner          *TestRunner          `json:"testRunner,omitempty"`
	LogListener         *LogListener         `json:"logListener,omitempty"`
	EnvironmentProvider *EnvironmentProvider `json:"environmentProvider,omitempty"`
	Identity            string               `json:"identity"`
	Providers           Providers            `json:"providers"`
	Suites              []Suite              `json:"suites"`
}

// TestRunStatus defines the observed state of TestRun
type TestRunStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	SuiteRunners        []corev1.ObjectReference `json:"suiteRunners,omitempty"`
	EnvironmentRequests []corev1.ObjectReference `json:"environmentRequests,omitempty"`

	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	Verdict        string       `json:"verdict,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// TestRun is the Schema for the testruns API
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Environment",type="string",JSONPath=".status.conditions[?(@.type==\"Environment\")].reason"
// +kubebuilder:printcolumn:name="Suiterunner",type="string",JSONPath=".status.conditions[?(@.type==\"SuiteRunner\")].reason"
// +kubebuilder:printcolumn:name="Active",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].status"
// +kubebuilder:printcolumn:name="Verdict",type="string",JSONPath=".status.verdict"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].message"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=.metadata.labels.etos\.eiffel-community\.github\.io/id
type TestRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestRunSpec   `json:"spec,omitempty"`
	Status TestRunStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TestRunList contains a list of TestRun
type TestRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestRun{}, &TestRunList{})
}
