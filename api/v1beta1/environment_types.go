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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnvironmentSpec defines the desired state of Environment
type EnvironmentSpec struct {
	// Suite defines the test suite to run in the environment.
	// +required
	Suite `json:",inline"`

	// Name defines the name of the environment or sub suite.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`

	// ID defines the unique identifier for this environment or sub suite as UUID.
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	// +required
	ID string `json:"id"`

	// TestrunID defines the unique identifier for the testrun as UUID.
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	// +required
	TestrunID string `json:"testrunID"`

	// MainSuiteID defines the unique identifier for the main suite as UUID.
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	// +required
	MainSuiteID string `json:"mainSuiteID"`

	// Artifact defines the unique identifier for the artifact under test as UUID.
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	// +required
	Artifact string `json:"artifact"`

	// Context defines the unique identifier for the context as UUID.
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	// +required
	Context string `json:"context"`

	// TestRunner defines the image to use for the test runner.
	// +kubebuilder:validation:MinLength=1
	// +required
	TestRunner string `json:"testRunner"`

	// Providers defines which providers were used to create this environment
	// +required
	Providers Providers `json:"providers,omitempty"`

	// Iut defines the item under test. The content and structure of the IUT is provider-defined and can be used to
	// pass arbitrary data to the test execution environment. The IUT is represented as a raw JSON object, allowing
	// for flexible and extensible data structures as needed by different providers and test cases.
	// +required
	Iut *apiextensionsv1.JSON `json:"iut"`

	// Executor defines the executor configuration. The content and structure of the executor configuration is
	// provider-defined and can be used to pass arbitrary data to the test execution environment. The executor
	// configuration is represented as a raw JSON object, allowing for flexible and extensible data structures as
	// needed by different providers and test cases.
	// +required
	Executor *apiextensionsv1.JSON `json:"executor"`

	// LogArea defines the log area configuration. The content and structure of the log area configuration is
	// provider-defined and can be used to pass arbitrary data to the test execution environment. The log area
	// configuration is represented as a raw JSON object, allowing for flexible and extensible data structures as
	// needed by different providers and test cases.
	// +required
	LogArea *apiextensionsv1.JSON `json:"logArea"`

	// Deadline defines the end time, in unix epoch, before which the environment shall have
	// been released. If not set or set to 0, there is no deadline.
	// +optional
	Deadline int64 `json:"deadline"`
}

// EnvironmentStatus defines the observed state of Environment.
type EnvironmentStatus struct {
	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Environment resource.
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

	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Environment is the Schema for the environments API
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].reason"
// +kubebuilder:printcolumn:name="Description",type="string",JSONPath=".status.conditions[?(@.type==\"Active\")].message"
// +kubebuilder:printcolumn:name="Environment Request",type="string",JSONPath=.metadata.labels.etos\.eiffel-community\.github\.io/environment-request
// +kubebuilder:printcolumn:name="TestRun",type="string",JSONPath=.metadata.labels.etos\.eiffel-community\.github\.io/id
type Environment struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Environment
	// +required
	Spec EnvironmentSpec `json:"spec"`

	// status defines the observed state of Environment
	// +optional
	Status EnvironmentStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// EnvironmentList contains a list of Environment
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Environment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
}
