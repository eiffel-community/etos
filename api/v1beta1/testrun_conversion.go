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
	"log"
	"maps"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// ConvertTo converts this TestRun (v1beta1) to the Hub version (v1alpha1).
func (src *TestRun) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*etosv1alpha1.TestRun)
	log.Printf("ConvertTo: Converting TestRun from Spoke version v1beta1 to Hub version v1alpha1;"+
		"source: %s/%s, target: %s/%s", src.Namespace, src.Name, dst.Namespace, dst.Name)

	dst.Spec.ID = src.Spec.ID
	dst.Spec.Artifact = src.Spec.Artifact
	dst.Spec.Identity = src.Spec.Identity
	dst.Spec.Cluster = src.Spec.Cluster
	dst.Spec.SuiteSource = src.Spec.SuiteSource
	dst.Spec.Timeout = src.Spec.Timeout
	dst.Spec.Deadline = src.Spec.Deadline
	dst.Spec.Providers = etosv1alpha1.Providers{
		IUT:            src.Spec.Providers.IUT,
		LogArea:        src.Spec.Providers.LogArea,
		ExecutionSpace: src.Spec.Providers.ExecutionSpace,
	}
	dst.Spec.Retention = etosv1alpha1.Retention{
		Success: src.Spec.Retention.Success,
		Failure: src.Spec.Retention.Failure,
	}

	dst.Spec.Suites = make([]etosv1alpha1.Suite, len(src.Spec.Suites))
	for i, suite := range src.Spec.Suites {

		testSuite := etosv1alpha1.Suite{}
		suite.convertTo(&testSuite)
		testSuite.Name = fmt.Sprintf("%s-suite-%d", src.Name, i)
		testSuite.Dataset = &apiextensionsv1.JSON{Raw: []byte("{}")}
		dst.Spec.Suites[i] = testSuite
	}

	if src.Spec.SuiteRunner != nil {
		dst.Spec.SuiteRunner = &etosv1alpha1.SuiteRunner{Image: &etosv1alpha1.Image{}}
		if err := src.Spec.SuiteRunner.convertTo(dst.Spec.SuiteRunner.Image); err != nil {
			return err
		}
	}
	if src.Spec.LogListener != nil {
		dst.Spec.LogListener = &etosv1alpha1.LogListener{Image: &etosv1alpha1.Image{}}
		if err := src.Spec.LogListener.convertTo(dst.Spec.LogListener.Image); err != nil {
			return err
		}
	}
	if src.Spec.EnvironmentProvider != nil {
		dst.Spec.EnvironmentProvider = &etosv1alpha1.EnvironmentProvider{Image: &etosv1alpha1.Image{}}
		if err := src.Spec.EnvironmentProvider.convertTo(dst.Spec.EnvironmentProvider.Image); err != nil {
			return err
		}
	}
	if src.Spec.TestRunner != nil {
		dst.Spec.TestRunner = &etosv1alpha1.TestRunner{}
		if err := src.Spec.TestRunner.convertTo(dst.Spec.TestRunner); err != nil {
			return err
		}
	}

	// Copy ObjectMeta to preserve name, namespace, labels, etc.
	dst.ObjectMeta = src.ObjectMeta
	dst.Status = etosv1alpha1.TestRunStatus{
		Conditions:          src.Status.Conditions,
		EnvironmentRequests: src.Status.EnvironmentRequests,
		CompletionTime:      src.Status.CompletionTime,
		Verdict:             src.Status.Verdict,
	}

	return nil
}

// ConvertFrom converts the Hub version (v1alpha1) to this TestRun (v1beta1).
func (dst *TestRun) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*etosv1alpha1.TestRun)
	log.Printf("ConvertFrom: Converting TestRun from Hub version v1alpha1 to Spoke version v1beta1;"+
		"source: %s/%s, target: %s/%s", src.Namespace, src.Name, dst.Namespace, dst.Name)

	dst.Spec.ID = src.Spec.ID
	dst.Spec.Name = src.Name
	dst.Spec.SchemaVersion = "v1beta1"
	dst.Spec.Artifact = src.Spec.Artifact
	dst.Spec.Identity = src.Spec.Identity
	dst.Spec.Cluster = src.Spec.Cluster
	dst.Spec.SuiteSource = src.Spec.SuiteSource
	dst.Spec.Timeout = src.Spec.Timeout
	dst.Spec.Deadline = src.Spec.Deadline
	dst.Spec.Providers = Providers{
		IUT:            src.Spec.Providers.IUT,
		LogArea:        src.Spec.Providers.LogArea,
		ExecutionSpace: src.Spec.Providers.ExecutionSpace,
	}
	dst.Spec.Retention = Retention{
		Success: src.Spec.Retention.Success,
		Failure: src.Spec.Retention.Failure,
	}

	dst.Spec.Suites = make([]Suite, len(src.Spec.Suites))
	for i, suite := range src.Spec.Suites {

		testSuite := Suite{}
		testSuite.convertFrom(&suite)
		dst.Spec.Suites[i] = testSuite
	}

	dst.Spec.SuiteRunner = &Image{}
	if err := dst.Spec.SuiteRunner.convertFrom(src.Spec.SuiteRunner.Image); err != nil {
		return err
	}
	dst.Spec.LogListener = &Image{}
	if err := dst.Spec.LogListener.convertFrom(src.Spec.LogListener.Image); err != nil {
		return err
	}
	dst.Spec.EnvironmentProvider = &Image{}
	if err := dst.Spec.EnvironmentProvider.convertFrom(src.Spec.EnvironmentProvider.Image); err != nil {
		return err
	}
	dst.Spec.TestRunner = &TestRunner{}
	if err := dst.Spec.TestRunner.convertFrom(src.Spec.TestRunner); err != nil {
		return err
	}

	// Copy ObjectMeta to preserve name, namespace, labels, etc.
	dst.ObjectMeta = src.ObjectMeta
	dst.Status = TestRunStatus{
		Conditions:          src.Status.Conditions,
		EnvironmentRequests: src.Status.EnvironmentRequests,
		CompletionTime:      src.Status.CompletionTime,
		Verdict:             src.Status.Verdict,
	}

	return nil
}

// convertFrom converts the Suite from the v1alpha1 Suite to the v1beta1 Suite.
func (dst *Suite) convertFrom(src *etosv1alpha1.Suite) {
	dst.Priority = src.Priority
	dst.TestExecutions = make([]TestExecution, len(src.Tests))
	dst.Dataset = &apiextensionsv1.JSON{Raw: src.Dataset.Raw}
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
	dst.Dataset = &apiextensionsv1.JSON{Raw: dst.Dataset.Raw}
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

// convertFrom converts the Image from the v1alpha1 Image to the v1beta1 Image.
func (dst *Image) convertFrom(src *etosv1alpha1.Image) error {
	if src == nil {
		log.Printf("convertFrom: source Image is nil, skipping conversion to v1beta1")
		return nil
	}
	dst.Image = src.Image
	dst.ImagePullPolicy = src.ImagePullPolicy
	return nil
}

// convertTo converts the Image from the v1beta1 Image to the v1alpha1 Image.
func (src *Image) convertTo(dst *etosv1alpha1.Image) error {
	if src == nil {
		log.Printf("convertTo: source Image is nil, skipping conversion to v1alpha1")
		return nil
	}
	dst.Image = src.Image
	dst.ImagePullPolicy = src.ImagePullPolicy
	return nil
}

// convertFrom converts the TestRunner from the v1alpha1 TestRunner to the v1beta1 TestRunner.
func (dst *TestRunner) convertFrom(src *etosv1alpha1.TestRunner) error {
	if src == nil {
		log.Printf("convertFrom: source TestRunner is nil, skipping conversion to v1beta1")
		return nil
	}
	dst.Version = src.Version
	return nil
}

// convertTo converts the TestRunner from the v1beta1 TestRunner to the v1alpha1 TestRunner.
func (src *TestRunner) convertTo(dst *etosv1alpha1.TestRunner) error {
	if src == nil {
		log.Printf("convertTo: source TestRunner is nil, skipping conversion to v1alpha1")
		return nil
	}
	dst.Version = src.Version
	return nil
}
