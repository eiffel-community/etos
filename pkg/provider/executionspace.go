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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RunExecutionSpaceProvider is the base runner for an ExecutionSpace provider.
// Checks input parameters and calls either Release or Provision on a Provider.
//
// This function panics on errors, propagating errors back to the controller that executed it.
func RunExecutionSpaceProvider(provider Provider) {
	params := ParseParameters()
	params.providerType = "ExecutionSpace"
	params.amountFunc = GetIUTCount

	ctx := context.TODO()
	if err := writeTerminationLog(ctx, runProvider, provider, params); err != nil {
		panic(err)
	}
}

// GetExecutionSpace gets an ExecutionSpace resource by name from Kubernetes.
func GetExecutionSpace(ctx context.Context, name, namespace string) (*v1alpha2.ExecutionSpace, error) {
	cli, err := KubernetesClient()
	if err != nil {
		return nil, err
	}
	var executionSpace v1alpha2.ExecutionSpace
	if err := cli.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &executionSpace); err != nil {
		return nil, err
	}
	return &executionSpace, nil
}

// GetExecutionSpaces fetches all ExecutionSpaces for an environmentrequest from Kubernetes.
func GetExecutionSpaces(
	ctx context.Context,
	environmentRequestID,
	namespace string,
) (v1alpha2.ExecutionSpaceList, error) {
	var executionSpaces v1alpha2.ExecutionSpaceList
	cli, err := KubernetesClient()
	if err != nil {
		return executionSpaces, err
	}
	err = cli.List(
		ctx,
		&executionSpaces,
		client.InNamespace(namespace),
		client.MatchingLabels{"etos.eiffel-community.github.io/environment-request-id": environmentRequestID},
	)
	return executionSpaces, err
}

// CreateExecutionSpace creates a new ExecutionSpace resource in Kubernetes.
//
// The spec.ProviderID and spec.EnvironmentRequest fields are automatically populated by this
// function. It will be overwritten if set.
func CreateExecutionSpace(
	ctx context.Context,
	environmentrequest *v1alpha1.EnvironmentRequest,
	namespace string,
	spec v1alpha2.ExecutionSpaceSpec,
) (*v1alpha2.ExecutionSpace, error) {
	logger, _ := logr.FromContext(ctx)
	var executionSpace v1alpha2.ExecutionSpace

	logger.Info("Getting Kubernetes client")
	cli, err := KubernetesClient()
	if err != nil {
		return nil, err
	}
	labels := map[string]string{
		"app.kubernetes.io/name":    "execution-space-provider",
		"app.kubernetes.io/part-of": "etos",
	}

	spec.ProviderID = environmentrequest.Spec.Providers.ExecutionSpace.ID
	spec.EnvironmentRequest = environmentrequest.Name

	isController := false
	blockOwnerDeletion := true
	executionSpace = v1alpha2.ExecutionSpace{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			GenerateName: fmt.Sprintf("%s-execution-space-", strings.ToLower(environmentrequest.Spec.Name)),
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

	return &executionSpace, cli.Create(ctx, &executionSpace)
}

// DeleteExecutionSpace deletes an ExecutionSpace resource from Kubernetes.
func DeleteExecutionSpace(ctx context.Context, executionSpace *v1alpha2.ExecutionSpace) error {
	cli, err := KubernetesClient()
	if err != nil {
		return err
	}
	return cli.Delete(ctx, executionSpace)
}
