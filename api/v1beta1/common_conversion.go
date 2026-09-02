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
	"fmt"
	"maps"
	"strings"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// convertFrom converts the Suite from the v1alpha1 Suite to the v1beta1 Suite.
func (dst *Suite) convertFrom(src *etosv1alpha1.Suite) {
	dst.Priority = src.Priority
	dst.TestExecutions = make([]TestExecution, len(src.Tests))
	dst.Dataset = src.Dataset
	for i, test := range src.Tests {
		testExecution := TestExecution{}
		testExecution.convertFrom(&test)
		dst.TestExecutions[i] = testExecution
	}
}

// convertTo converts the Suite from the v1beta1 Suite to the v1alpha1 Suite.
func (src *Suite) convertTo(dst *etosv1alpha1.Suite) {
	dst.Priority = src.Priority
	dst.Tests = make([]etosv1alpha1.Test, len(src.TestExecutions))
	dst.Dataset = src.Dataset
	for i, testExecution := range src.TestExecutions {
		test := etosv1alpha1.Test{}
		testExecution.convertTo(&test)
		dst.Tests[i] = test
	}
}

// convertFrom converts the TestExecution from the v1alpha1 Test to the v1beta1 TestExecution.
func (dst *TestExecution) convertFrom(src *etosv1alpha1.Test) {
	dst.ID = src.ID
	testCase := TestCase{}
	testCase.convertFrom(&src.TestCase)
	dst.TestCase = testCase

	execution := Execution{}
	execution.convertFrom(&src.Execution)
	dst.Execution = execution

	testEnvironment := TestEnvironment{}
	testEnvironment.convertFrom(&src.Execution)
	dst.Environment = testEnvironment
}

// convertTo converts the TestExecution from the v1beta1 TestExecution to the v1alpha1 Test.
func (src *TestExecution) convertTo(dst *etosv1alpha1.Test) {
	dst.ID = src.ID
	testCase := etosv1alpha1.TestCase{}
	src.TestCase.convertTo(&testCase)
	dst.TestCase = testCase

	execution := etosv1alpha1.Execution{}
	src.Execution.convertTo(&execution)
	src.Environment.convertTo(&execution)
	dst.Execution = execution
}

// convertFrom converts the TestCase from the v1alpha1 Test to the v1beta1 TestCase.
func (dst *TestCase) convertFrom(src *etosv1alpha1.TestCase) {
	dst.ID = src.ID
	dst.Version = src.Version
	dst.Repository = src.Tracker
	dst.URI = src.URI
}

// convertTo converts the TestCase from the v1beta1 TestCase to the v1alpha1 TestCase.
func (src *TestCase) convertTo(dst *etosv1alpha1.TestCase) {
	dst.ID = src.ID
	dst.Version = src.Version
	dst.Tracker = src.Repository
	dst.URI = src.URI
}

// convertFrom converts the Execution from the v1alpha1 Test to the v1beta1 Execution.
func (dst *Execution) convertFrom(src *etosv1alpha1.Execution) {
	var command strings.Builder
	command.WriteString(src.Command)
	for key, param := range src.Parameters {
		if param == "" {
			command.WriteString(key)
		} else {
			fmt.Fprintf(&command, "%s=%s", key, param)
		}
		command.WriteString(" ")
	}

	dst.Checkout = src.Checkout
	dst.Command = command.String()
	dst.PreExecution = src.Execute
}

// convertTo converts the Execution from the v1beta1 Execution to the v1alpha1 Execution.
func (src *Execution) convertTo(dst *etosv1alpha1.Execution) {
	if src.Checkout == nil {
		dst.Checkout = []string{}
	} else {
		dst.Checkout = src.Checkout
	}
	dst.Execute = src.PreExecution

	// Note that parameters and command may change during the conversion, since v1beta merges parameters into
	// its command field, while v1alpha1 has them as separate fields. For simplicity, we will not attempt to parse
	// the command to extract parameters, and instead will just set parameters to an empty map.
	dst.Parameters = make(map[string]string)
	dst.Command = src.Command
}

// convertFrom converts the TestEnvironment from the v1alpha1 Test to the v1beta1 TestEnvironment.
func (dst *TestEnvironment) convertFrom(src *etosv1alpha1.Execution) {
	dst.EnvironmentVariables = src.Environment
	dst.TestRunner = src.TestRunner
}

// convertTo converts the TestEnvironment from the v1beta1 TestEnvironment to the v1alpha1 Environment.
func (src *TestEnvironment) convertTo(dst *etosv1alpha1.Execution) {
	if dst.Environment == nil {
		dst.Environment = make(map[string]string)
	}
	maps.Copy(dst.Environment, src.EnvironmentVariables)
	dst.TestRunner = src.TestRunner
}
