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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
)

var _ = Describe("Environment Webhook", func() {
	var (
		obj    *etosv1alpha1.Environment
		oldObj *etosv1alpha1.Environment
	)

	BeforeEach(func() {
		obj = &etosv1alpha1.Environment{}
		oldObj = &etosv1alpha1.Environment{}
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
	})

	Context("When creating Environment under Conversion Webhook", func() {
	})

})
