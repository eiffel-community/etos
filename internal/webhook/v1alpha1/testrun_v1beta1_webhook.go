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

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1beta1 "github.com/eiffel-community/etos/api/v1beta1"
	"github.com/eiffel-community/etos/internal/config"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupTestRunV1beta1WebhookWithManager registers v1beta1 admission handlers for TestRun.
func SetupTestRunV1beta1WebhookWithManager(mgr ctrl.Manager, cfg config.Config) error {
	if cli == nil {
		cli = mgr.GetClient()
	}

	return ctrl.NewWebhookManagedBy(mgr, &etosv1beta1.TestRun{}).
		WithValidator(&BetaTestRunCustomValidator{}).
		WithDefaulter(&BetaTestRunCustomDefaulter{cfg: cfg}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-etos-eiffel-community-github-io-v1beta1-testrun,mutating=true,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=testruns,verbs=create;update,versions=v1beta1,name=mtestrun-v1beta1.kb.io,admissionReviewVersions=v1

// BetaTestRunCustomDefaulter applies the v1alpha1 hub's defaults to v1beta1 TestRuns through
// conversion, keeping admission behavior consistent without duplicating defaulting logic.
type BetaTestRunCustomDefaulter struct {
	cfg config.Config
}

// Default applies the v1alpha1 TestRun defaults to a v1beta1 TestRun.
func (d *BetaTestRunCustomDefaulter) Default(ctx context.Context, testRun *etosv1beta1.TestRun) error {
	alphaTestRun := &etosv1alpha1.TestRun{}
	if err := testRun.ConvertTo(alphaTestRun); err != nil {
		return err
	}

	alphaDefaulter := &TestRunCustomDefaulter{cfg: d.cfg}
	if err := alphaDefaulter.Default(ctx, alphaTestRun); err != nil {
		return err
	}

	return testRun.ConvertFrom(alphaTestRun)
}

// +kubebuilder:webhook:path=/validate-etos-eiffel-community-github-io-v1beta1-testrun,mutating=false,failurePolicy=fail,sideEffects=None,groups=etos.eiffel-community.github.io,resources=testruns,verbs=create;update,versions=v1beta1,name=vtestrun-v1beta1.kb.io,admissionReviewVersions=v1

// BetaTestRunCustomValidator applies v1alpha1 TestRun validation to v1beta1 objects.
type BetaTestRunCustomValidator struct{}

// ValidateCreate applies the v1alpha1 TestRun create validation to a v1beta1 TestRun.
func (v *BetaTestRunCustomValidator) ValidateCreate(
	ctx context.Context,
	testRun *etosv1beta1.TestRun,
) (admission.Warnings, error) {
	alphaTestRun := &etosv1alpha1.TestRun{}
	if err := testRun.ConvertTo(alphaTestRun); err != nil {
		return nil, err
	}

	return (&TestRunCustomValidator{}).ValidateCreate(ctx, alphaTestRun)
}

// ValidateUpdate applies the v1alpha1 TestRun update validation to v1beta1 TestRuns.
func (v *BetaTestRunCustomValidator) ValidateUpdate(
	ctx context.Context,
	oldTestRun, newTestRun *etosv1beta1.TestRun,
) (admission.Warnings, error) {
	alphaOldTestRun := &etosv1alpha1.TestRun{}
	if err := oldTestRun.ConvertTo(alphaOldTestRun); err != nil {
		return nil, err
	}

	alphaNewTestRun := &etosv1alpha1.TestRun{}
	if err := newTestRun.ConvertTo(alphaNewTestRun); err != nil {
		return nil, err
	}

	return (&TestRunCustomValidator{}).ValidateUpdate(ctx, alphaOldTestRun, alphaNewTestRun)
}

// ValidateDelete applies the v1alpha1 TestRun delete validation to a v1beta1 TestRun.
func (v *BetaTestRunCustomValidator) ValidateDelete(
	ctx context.Context,
	testRun *etosv1beta1.TestRun,
) (admission.Warnings, error) {
	alphaTestRun := &etosv1alpha1.TestRun{}
	if err := testRun.ConvertTo(alphaTestRun); err != nil {
		return nil, err
	}

	return (&TestRunCustomValidator{}).ValidateDelete(ctx, alphaTestRun)
}

var _ admission.Defaulter[*etosv1beta1.TestRun] = (*BetaTestRunCustomDefaulter)(nil)

var _ admission.Validator[*etosv1beta1.TestRun] = (*BetaTestRunCustomValidator)(nil)
