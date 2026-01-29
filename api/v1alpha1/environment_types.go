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

// EnvironmentSpec defines the desired state of Environment
type EnvironmentSpec struct {
	Name string `json:"name"`

	// Snake casing as to be compatible with ETR.
	// Regexes match any UUID version.

	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	SuiteID string `json:"suite_id"`
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	SubSuiteID string `json:"sub_suite_id"`
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	MainSuiteID string `json:"test_suite_started_id"`
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	Artifact string `json:"artifact"`
	// +kubebuilder:validation:Pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	Context string `json:"context"`

	// Providers describes which providers were used to create this environment
	Providers *Providers `json:"providers"`

	// Deadline is the end time, in unix epoch, before which the environment shall have
	// been released. If not set or set to 0, there is no deadline.
	// +optional
	Deadline int64 `json:"deadline"`

	Priority   int                   `json:"priority"`
	Tests      []Test                `json:"recipes"`
	TestRunner string                `json:"test_runner"`
	Iut        *apiextensionsv1.JSON `json:"iut"`
	Executor   *apiextensionsv1.JSON `json:"executor"`
	LogArea    *apiextensionsv1.JSON `json:"log_area"`
}

// EnvironmentStatus defines the observed state of Environment
type EnvironmentStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	EnvironmentReleasers []corev1.ObjectReference `json:"environmentReleasers,omitempty"`

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
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentSpec   `json:"spec,omitempty"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EnvironmentList contains a list of Environment
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Environment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
}
