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

package controller

import (
	"testing"

	. "github.com/onsi/gomega"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/controller/jobs"
)

// TestSetInconclusiveVerdictIfUnset verifies that only empty and None verdicts change.
func TestSetInconclusiveVerdictIfUnset(t *testing.T) {
	tests := []struct {
		name           string
		initialVerdict string
		wantChanged    bool
		wantVerdict    string
	}{
		{name: "empty verdict", initialVerdict: "", wantChanged: true, wantVerdict: string(jobs.VerdictInconclusive)},
		{name: "none verdict", initialVerdict: string(jobs.VerdictNone), wantChanged: true, wantVerdict: string(jobs.VerdictInconclusive)},
		{name: "passed verdict", initialVerdict: string(jobs.VerdictPassed), wantChanged: false, wantVerdict: string(jobs.VerdictPassed)},
		{name: "failed verdict", initialVerdict: string(jobs.VerdictFailed), wantChanged: false, wantVerdict: string(jobs.VerdictFailed)},
		{name: "inconclusive verdict", initialVerdict: string(jobs.VerdictInconclusive), wantChanged: false, wantVerdict: string(jobs.VerdictInconclusive)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			testrun := &etosv1alpha1.TestRun{}
			testrun.Status.Verdict = tt.initialVerdict

			g.Expect(setInconclusiveVerdictIfUnset(testrun)).To(Equal(tt.wantChanged))
			g.Expect(testrun.Status.Verdict).To(Equal(tt.wantVerdict))
		})
	}
}
