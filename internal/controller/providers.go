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
	"context"
	"fmt"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/controller/status"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// checkProviders checks if all providers for this environment are available.
func checkProviders(ctx context.Context, c client.Reader, namespace string, providers etosv1alpha1.Providers) error {
	err := checkProvider(ctx, c, providers.IUT, namespace, &etosv1alpha1.Provider{})
	if err != nil {
		return err
	}
	err = checkProvider(ctx, c, providers.ExecutionSpace, namespace, &etosv1alpha1.Provider{})
	if err != nil {
		return err
	}
	err = checkProvider(ctx, c, providers.LogArea, namespace, &etosv1alpha1.Provider{})
	if err != nil {
		return err
	}
	return nil
}

// checkProvider checks if the provider condition 'Available' is set to True.
func checkProvider(ctx context.Context, c client.Reader, name string, namespace string, provider *etosv1alpha1.Provider) error {
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, provider)
	if err != nil {
		return err
	}
	if meta.IsStatusConditionPresentAndEqual(provider.Status.Conditions, status.StatusAvailable, metav1.ConditionTrue) {
		return nil
	}
	return fmt.Errorf("Provider '%s' does not have a status field", name)
}

// getProvider fetches a Provider resource by name from Kubernetes.
func getProvider(ctx context.Context, c client.Reader, name, namespace string) (*etosv1alpha1.Provider, error) {
	var provider etosv1alpha1.Provider
	return &provider, c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &provider)
}

// image returns an image that can be used to execute provider.
func image(provider *etosv1alpha1.Provider) string {
	var image string
	if provider.Spec.Image != "" {
		return provider.Spec.Image
	}
	return image
}
