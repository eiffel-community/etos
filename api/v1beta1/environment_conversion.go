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
	"log"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

// ConvertTo converts this Environment (v1beta1) to the Hub version (v1alpha1).
func (src *Environment) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*etosv1alpha1.Environment)
	log.Printf("ConvertTo: Converting Environment from Spoke version v1beta1 to Hub version v1alpha1;"+
		"source: %s/%s, target: %s/%s", src.Namespace, src.Name, dst.Namespace, dst.Name)

	dst.Spec.Name = src.Spec.Name
	dst.Spec.SuiteID = src.Spec.TestrunID
	dst.Spec.SubSuiteID = src.Spec.ID
	dst.Spec.MainSuiteID = src.Spec.MainSuiteID
	dst.Spec.Artifact = src.Spec.Artifact
	dst.Spec.Context = src.Spec.Context
	dst.Spec.TestRunner = src.Spec.TestRunner
	dst.Spec.Deadline = src.Spec.Deadline
	dst.Spec.Priority = src.Spec.Priority
	dst.Spec.Iut = src.Spec.Iut
	dst.Spec.Executor = src.Spec.Executor
	dst.Spec.LogArea = src.Spec.LogArea
	dst.Spec.Providers = &etosv1alpha1.Providers{
		IUT:            src.Spec.Providers.IUT,
		LogArea:        src.Spec.Providers.LogArea,
		ExecutionSpace: src.Spec.Providers.ExecutionSpace,
	}

	dst.Spec.Tests = make([]etosv1alpha1.Test, len(src.Spec.TestExecutions))
	for i, testExecution := range src.Spec.TestExecutions {
		test := etosv1alpha1.Test{}
		testExecution.convertTo(&test)
		dst.Spec.Tests[i] = test
	}

	// Copy ObjectMeta to preserve name, namespace, labels, etc.
	dst.ObjectMeta = src.ObjectMeta
	dst.Status = etosv1alpha1.EnvironmentStatus{
		Conditions:     src.Status.Conditions,
		CompletionTime: src.Status.CompletionTime,
	}

	return nil
}

// ConvertFrom converts the Hub version (v1alpha1) to this Environment (v1beta1).
func (dst *Environment) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*etosv1alpha1.Environment)
	log.Printf("ConvertFrom: Converting Environment from Hub version v1alpha1 to Spoke version v1beta1;"+
		"source: %s/%s, target: %s/%s", src.Namespace, src.Name, dst.Namespace, dst.Name)

	dst.Spec.Name = src.Spec.Name
	dst.Spec.ID = src.Spec.SubSuiteID
	dst.Spec.MainSuiteID = src.Spec.MainSuiteID
	dst.Spec.TestrunID = src.Spec.SuiteID
	dst.Spec.Artifact = src.Spec.Artifact
	dst.Spec.Context = src.Spec.Context
	dst.Spec.TestRunner = src.Spec.TestRunner
	dst.Spec.Deadline = src.Spec.Deadline
	dst.Spec.Providers = Providers{
		IUT:            src.Spec.Providers.IUT,
		LogArea:        src.Spec.Providers.LogArea,
		ExecutionSpace: src.Spec.Providers.ExecutionSpace,
	}
	dst.Spec.Priority = src.Spec.Priority
	dst.Spec.Iut = src.Spec.Iut
	dst.Spec.Executor = src.Spec.Executor
	dst.Spec.LogArea = src.Spec.LogArea

	dst.Spec.TestExecutions = make([]TestExecution, len(src.Spec.Tests))
	for i, test := range src.Spec.Tests {
		testExecution := TestExecution{}
		testExecution.convertFrom(&test)
		dst.Spec.TestExecutions[i] = testExecution
	}

	// Copy ObjectMeta to preserve name, namespace, labels, etc.
	dst.ObjectMeta = src.ObjectMeta
	dst.Status = EnvironmentStatus{
		Conditions:     src.Status.Conditions,
		CompletionTime: src.Status.CompletionTime,
	}

	return nil
}
