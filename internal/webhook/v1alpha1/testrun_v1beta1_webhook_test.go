// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package v1alpha1

import (
	"context"
	"reflect"
	"testing"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1beta1 "github.com/eiffel-community/etos/api/v1beta1"
	"github.com/eiffel-community/etos/internal/config"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// TestBetaTestRunCustomDefaulterMatchesAlpha verifies that beta defaulting uses the alpha implementation.
func TestBetaTestRunCustomDefaulterMatchesAlpha(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := etosv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := etosv1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	cluster := &etosv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "test",
		},
		Spec: etosv1alpha1.ClusterSpec{
			ETOS: etosv1alpha1.ETOS{
				SuiteRunner: etosv1alpha1.ETOSSuiteRunner{
					Image: etosv1alpha1.Image{
						Image:           "suite-runner:test",
						ImagePullPolicy: corev1.PullIfNotPresent,
					},
					LogListener: etosv1alpha1.ETOSLogListener{
						Image: etosv1alpha1.Image{
							Image:           "log-listener:test",
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
				},
				EnvironmentProvider: etosv1alpha1.ETOSEnvironmentProvider{
					Image: etosv1alpha1.Image{
						Image:           "environment-provider:test",
						ImagePullPolicy: corev1.PullIfNotPresent,
					},
				},
				TestRunner: etosv1alpha1.ETOSTestRunner{
					Version: "test-version",
				},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()

	previousClient := cli
	cli = fakeClient
	t.Cleanup(func() {
		cli = previousClient
	})

	cfg := config.New()
	ctx := context.Background()
	input := &etosv1beta1.TestRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run",
			Namespace: "test",
		},
		Spec: etosv1beta1.TestRunSpec{
			ID:      "test-id",
			Cluster: "cluster",
		},
	}

	expected := input.DeepCopy()
	alphaExpected := &etosv1alpha1.TestRun{}
	if err := expected.ConvertTo(alphaExpected); err != nil {
		t.Fatal(err)
	}
	if err := (&TestRunCustomDefaulter{cfg: cfg}).Default(ctx, alphaExpected); err != nil {
		t.Fatal(err)
	}
	if err := expected.ConvertFrom(alphaExpected); err != nil {
		t.Fatal(err)
	}

	got := input.DeepCopy()
	if err := (&BetaTestRunCustomDefaulter{cfg: cfg}).Default(ctx, got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("beta defaulting did not match alpha defaulting:\n got: %#v\nwant: %#v", got, expected)
	}
}

// TestBetaTestRunCustomValidatorMatchesAlpha verifies that beta validation uses the alpha implementation.
func TestBetaTestRunCustomValidatorMatchesAlpha(t *testing.T) {
	ctx := context.Background()
	betaTestRun := &etosv1beta1.TestRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run",
			Namespace: "test",
		},
	}
	alphaTestRun := &etosv1alpha1.TestRun{}
	if err := betaTestRun.ConvertTo(alphaTestRun); err != nil {
		t.Fatal(err)
	}

	expectedWarnings, expectedErr := (&TestRunCustomValidator{}).ValidateCreate(ctx, alphaTestRun)
	gotWarnings, gotErr := (&BetaTestRunCustomValidator{}).ValidateCreate(ctx, betaTestRun)
	if !reflect.DeepEqual(gotWarnings, expectedWarnings) {
		t.Fatalf("warnings mismatch: got %v, want %v", gotWarnings, expectedWarnings)
	}
	if (gotErr == nil) != (expectedErr == nil) {
		t.Fatalf("error mismatch: got %v, want %v", gotErr, expectedErr)
	}
	if gotErr == nil {
		t.Fatal("expected invalid beta TestRun to be rejected")
	}
	if !apierrors.IsInvalid(gotErr) || !apierrors.IsInvalid(expectedErr) {
		t.Fatalf("expected alpha and beta validation to return invalid errors, got beta: %v, alpha: %v", gotErr, expectedErr)
	}
}

var _ admission.Validator[*etosv1beta1.TestRun] = (*BetaTestRunCustomValidator)(nil)
