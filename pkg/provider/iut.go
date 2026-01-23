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
package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RunIutProvider is the base runner for an IUT provider. Checks input parameters and calls either
// Release or Provision on a Provider.
//
// This function panics on errors, propagating errors back to the controller that executed it.
func RunIutProvider(provider Provider) {
	params := ParseParameters()
	params.providerType = "Iut"
	params.amountFunc = func(_ context.Context, environmentRequest *v1alpha1.EnvironmentRequest) (int, error) {
		return min(environmentRequest.Spec.MaximumAmount, environmentRequest.Spec.MinimumAmount), nil
	}

	ctx := context.TODO()
	if err := writeTerminationLog(ctx, runProvider, provider, params); err != nil {
		panic(err)
	}
}

// GetIUT gets an IUT resource by name from Kubernetes.
func GetIUT(ctx context.Context, name, namespace string) (*v1alpha2.Iut, error) {
	cli, err := KubernetesClient()
	if err != nil {
		return nil, err
	}
	var iut v1alpha2.Iut
	if err := cli.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &iut); err != nil {
		return nil, err
	}
	return &iut, nil
}

// GetIUTCount gets the number of IUTs for an environmentRequest.
func GetIUTCount(ctx context.Context, environmentRequest *v1alpha1.EnvironmentRequest) (int, error) {
	iutList, err := GetIUTs(ctx, environmentRequest.Spec.ID, environmentRequest.Namespace)
	if err != nil {
		return -1, err
	}
	return len(iutList.Items), nil
}

// GetIUTs fetches all IUTs for an environmentrequest from Kubernetes.
func GetIUTs(ctx context.Context, environmentRequestID, namespace string) (v1alpha2.IutList, error) {
	var iuts v1alpha2.IutList
	cli, err := KubernetesClient()
	if err != nil {
		return iuts, err
	}
	err = cli.List(
		ctx,
		&iuts,
		client.InNamespace(namespace),
		client.MatchingLabels{"etos.eiffel-community.github.io/environment-request-id": environmentRequestID},
	)
	return iuts, err
}

// CreateIUT creates a new IUT resource in Kubernetes.
//
// The spec.ID, spec.Identity, spec.EnvironmentRequest, and spec.ProviderID fields are
// automatically populated by this function. They will be overwritten if set.
func CreateIUT(
	ctx context.Context,
	environmentrequest *v1alpha1.EnvironmentRequest,
	namespace string,
	spec v1alpha2.IutSpec,
) (*v1alpha2.Iut, error) {
	logger, _ := logr.FromContext(ctx)
	var iut v1alpha2.Iut

	logger.Info("Getting Kubernetes client")
	cli, err := KubernetesClient()
	if err != nil {
		return nil, err
	}
	labels := map[string]string{
		"app.kubernetes.io/name":    "iut-provider",
		"app.kubernetes.io/part-of": "etos",
	}

	spec.ID = uuid.NewString()
	spec.ProviderID = environmentrequest.Spec.Providers.IUT.ID
	spec.Identity = environmentrequest.Spec.Identity
	spec.EnvironmentRequest = environmentrequest.Name

	isController := false
	blockOwnerDeletion := true
	iut = v1alpha2.Iut{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			GenerateName: fmt.Sprintf("%s-iut-", strings.ToLower(environmentrequest.Spec.Name)),
			Namespace:    namespace,
			OwnerReferences: []metav1.OwnerReference{{
				Kind:               "EnvironmentRequest",
				Name:               environmentrequest.GetName(),
				UID:                environmentrequest.GetUID(),
				APIVersion:         v1alpha1.GroupVersion.String(),
				Controller:         &isController,
				BlockOwnerDeletion: &blockOwnerDeletion,
			}},
		},
		Spec: spec,
	}
	return &iut, cli.Create(ctx, &iut)
}

// DeleteIUT deletes an IUT resource from Kubernetes.
func DeleteIUT(ctx context.Context, iut *v1alpha2.Iut) error {
	cli, err := KubernetesClient()
	if err != nil {
		return err
	}
	return cli.Delete(ctx, iut)
}
