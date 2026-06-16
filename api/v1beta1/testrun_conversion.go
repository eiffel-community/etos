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
		name := src.Name
		if name == "" {
			name = src.GenerateName
		}
		testSuite.Name = fmt.Sprintf("%s-suite-%d", name, i)
		testSuite.Dataset = &apiextensionsv1.JSON{Raw: []byte("{}")}
		dst.Spec.Suites[i] = testSuite
	}

	if src.Spec.SuiteRunner != nil {
		dst.Spec.SuiteRunner = &etosv1alpha1.SuiteRunner{Image: &etosv1alpha1.Image{}}
		if err := src.Spec.SuiteRunner.convertTo(dst.Spec.SuiteRunner.Image); err != nil {
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
	if dst.Spec.Name == "" {
		dst.Spec.Name = src.GenerateName
	}
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
