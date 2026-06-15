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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestRunSpec defines the desired state of TestRun
type TestRunSpec struct {
	// TestSuite defines the test suite to run.
	TestSuite `json:",inline"`

	// ID defines the unique identifier for this test run as UUID. This field will be generated if not set.
	// +optional
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	ID string `json:"id,omitempty"`

	// Artifact defines the unique identifier for the artifact under test as UUID. This field is required.
	// +required
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	Artifact string `json:"artifact"`

	// Identity defines the identity of the artifact as a packageurl.
	// +required
	// +kubebuilder:validation:Pattern="^pkg:[a-z]+/.+$"
	// +kubebuilder:validation:MinLength=1
	Identity string `json:"identity"`

	// Cluster defines the cluster to run the test suite on.
	// This field is optional and can be used to specify a particular cluster for test execution.
	// +optional
	Cluster string `json:"cluster,omitempty"`

	// Providers defines the providers to use for test execution. These names must correspond to existing
	// Provider kinds in the namespace where a testrun is created.
	// If not set the controller will attempt to find the default providers in the namespace.
	// If any of the default providers are not found, the controller will fail the testrun.
	// +optional
	Providers Providers `json:"providers"`

	// SuiteRunner defines the image to use for the suite runner.
	// If not specified, a default image will be used.
	// +optional
	SuiteRunner *Image `json:"suiteRunner,omitempty"`

	// EnvironmentProvider defines the image to use for the environment provider.
	// If not specified, a default image will be used.
	EnvironmentProvider *Image `json:"environmentProvider,omitempty"`

	// TestRunner defines the test runner version to use for executing tests.
	// If not specified, a default test runner version will be used.
	// +optional
	TestRunner *TestRunner `json:"testRunner,omitempty"`

	// SuiteSource defines the URL from which the test suite definition can be fetched.
	// It is used to set batchesUri in the TERCC event.
	// +optional
	SuiteSource string `json:"suiteSource,omitempty"`

	// Timeout defines the maximum duration, in seconds, the testrun is allowed to take.
	// If not set, defaults to 86400 (24 hours). If both Timeout and Deadline are set,
	// Deadline takes precedence.
	// +optional
	// +kubebuilder:default=86400
	Timeout int64 `json:"timeout,omitempty"`

	// Deadline defines the timestamp, in unix epoch seconds, by which the testrun
	// must have completed. If the deadline is exceeded, the controller will fail
	// the testrun. If not set, it is computed from Timeout at the start of the testrun.
	// If both Timeout and Deadline are set, Deadline takes precedence.
	// +optional
	Deadline int64 `json:"deadline,omitempty"`

	// Retention defines the failure and success retentions for keeping testrun resources after completion.
	// If not set, the testrun resource will not be deleted after completion.
	// +optional
	Retention Retention `json:"retention,omitempty"`
}

// Retention defines the failure and success retentions for keeping testrun resources after completion.
// If not set, the testrun resource will not be deleted after completion.
type Retention struct {

	// Failure defines the duration to retain testrun resources after a failed execution.
	// +optional
	Failure *metav1.Duration `json:"failure,omitempty"`

	// Success defines the duration to retain testrun resources after a successful execution.
	// +optional
	Success *metav1.Duration `json:"success,omitempty"`
}

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

// TestRunner defines the test runner to use for executing tests. This field is optional and can be used to specify a
// particular test runner for test execution. If not specified, a default test runner will be used.
type TestRunner struct {
	Version string `json:"version"`
}

// Image defines the docker image to run for a service. This field is optional and can be used to specify a particular
// image for a service. If not specified, a default image will be used.
type Image struct {
	// Image defines the docker image to run for a service. ETOS applies defaults if empty.
	// +optional
	Image string `json:"image,omitempty"`

	// ImagePullPolicy defines the pull policy to use for the image. ETOS applies PullIfNotPresent
	// if empty.
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// TestRunStatus defines the observed state of TestRun.
type TestRunStatus struct {
	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the TestRun resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	EnvironmentRequests []corev1.ObjectReference `json:"environmentRequests,omitempty"`

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
// +kubebuilder:selectablefield:JSONPath=".spec.cluster"
// +kubebuilder:selectablefield:JSONPath=".spec.artifact"
// +kubebuilder:selectablefield:JSONPath=".spec.id"
type TestRun struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of TestRun
	// +required
	Spec TestRunSpec `json:"spec"`

	// status defines the observed state of TestRun
	// +optional
	Status TestRunStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// TestRunList contains a list of TestRun
type TestRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []TestRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestRun{}, &TestRunList{})
}
