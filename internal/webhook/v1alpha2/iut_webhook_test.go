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

package v1alpha2

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
)

var _ = Describe("Iut Webhook", func() {
	var (
		obj       *etosv1alpha2.Iut
		oldObj    *etosv1alpha2.Iut
		defaulter IutCustomDefaulter

		requestID           = "89224612-3851-45c9-95c0-72b719ae46ea"
		requestIdentifier   = "fbb4096d-6529-4c39-bac3-08a7e45bf69a"
		requestName         = "test-environment-request"
		requestLabelCluster = "cluster-sample"
		providerID          = "iut-provider-sample"
	)

	BeforeEach(func() {
		environmentRequest := etosv1alpha1.EnvironmentRequest{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{
					"etos.eiffel-community.github.io/cluster": requestLabelCluster,
				},
			},
			Spec: etosv1alpha1.EnvironmentRequestSpec{
				Identifier: requestIdentifier,
				Name:       requestName,
				ID:         requestID,
			},
		}
		obj = &etosv1alpha2.Iut{}
		oldObj = &etosv1alpha2.Iut{}
		defaulter = IutCustomDefaulter{FakeReader(&environmentRequest, nil)}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
	})

	Context("When creating Iut under Defaulting Webhook", func() {
		It("Should apply defaults when a required field is empty", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Labels = nil
			obj.Spec.ProviderID = providerID
			By("calling the Default method to apply defaults")
			Expect(defaulter.Default(ctx, obj)).ToNot(HaveOccurred())
			By("checking that the default values are set")
			Expect(obj.Labels).To(Equal(map[string]string{
				"etos.eiffel-community.github.io/environment-request":    requestName,
				"etos.eiffel-community.github.io/environment-request-id": requestID,
				"etos.eiffel-community.github.io/cluster":                requestLabelCluster,
				"etos.eiffel-community.github.io/provider":               providerID,
				"etos.eiffel-community.github.io/id":                     requestIdentifier,
				"app.kubernetes.io/part-of":                              "etos",
				"app.kubernetes.io/name":                                 "iut-provider",
			}))
		})
	})

})
