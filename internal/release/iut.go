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
package release

import (
	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

const IutReleaserName = "iut-releaser"

// IutReleaser returns an IUT releaser job specification.
func IutReleaser(iut *v1alpha2.Iut, environmentrequest *v1alpha1.EnvironmentRequest, provider *v1alpha1.Provider, skipDeletingIut bool) *batchv1.Job {
	return ReleaseJob(
		iut.Name,
		IutReleaserName,
		iut.Namespace,
		environmentrequest,
		IutReleaserContainer(iut, provider, skipDeletingIut),
	)
}

// IutReleaserContainer returns an IUT releaser container specification.
func IutReleaserContainer(iut *v1alpha2.Iut, provider *v1alpha1.Provider, skipDeletingIut bool) corev1.Container {
	return ReleaseContainer(
		iut.Name,
		IutReleaserName,
		iut.Namespace,
		provider,
		skipDeletingIut,
	)
}
