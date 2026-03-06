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

// RunLogAreaProvider is the base runner for an LogArea provider.
// Checks input parameters and calls either Release or Provision on a Provider.
//
// This function panics on errors, propagating errors back to the controller that executed it.
func RunLogAreaProvider(provider Provider) {
	params := ParseParameters()
	params.providerType = "LogArea"
	params.amountFunc = GetIUTCount

	ctx := context.TODO()
	logger := params.logger.WithValues(
		"providerType", params.providerType,
		"environmentRequest", params.environmentRequestName,
		"namespace", params.namespace,
		"providerName", params.providerName,
	)
	ctx = logr.NewContext(ctx, logger)
	if err := writeTerminationLog(ctx, runProvider, provider, params); err != nil {
		panic(err)
	}
}

// GetLogArea gets an LogArea resource by name from Kubernetes.
func GetLogArea(ctx context.Context, name, namespace string) (*v1alpha2.LogArea, error) {
	cli, err := KubernetesClient()
	if err != nil {
		return nil, err
	}
	var logArea v1alpha2.LogArea
	if err := cli.Get(
		ctx,
		types.NamespacedName{Name: name, Namespace: namespace},
		&logArea,
	); err != nil {
		return nil, err
	}
	return &logArea, nil
}

// GetLogAreas fetches all LogAreas for an environmentrequest from Kubernetes.
func GetLogAreas(
	ctx context.Context,
	environmentRequestID, namespace string,
) (v1alpha2.LogAreaList, error) {
	var logAreas v1alpha2.LogAreaList
	cli, err := KubernetesClient()
	if err != nil {
		return logAreas, err
	}
	err = cli.List(
		ctx,
		&logAreas,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"etos.eiffel-community.github.io/environment-request-id": environmentRequestID,
		},
	)
	return logAreas, err
}

// CreateLogArea creates a new LogArea resource in Kubernetes.
//
// The spec.ID, spec.EnvironmentRequest, and spec.ProviderID fields are automatically populated
// by this function. They will be overwritten if set.
// If a name is not provided, a name will be generated based on the EnvironmentRequest name.
// If a name is provided it is the caller's responsibility to ensure name uniqueness, it will
// not be guaranteed by this function.
func CreateLogArea(
	ctx context.Context,
	environmentrequest *v1alpha1.EnvironmentRequest,
	namespace, name string,
	spec v1alpha2.LogAreaSpec,
) (*v1alpha2.LogArea, error) {
	logger := logr.FromContextOrDiscard(ctx)
	var logArea v1alpha2.LogArea

	logger.Info("Getting Kubernetes client")
	cli, err := KubernetesClient()
	if err != nil {
		return nil, err
	}
	labels := map[string]string{
		"app.kubernetes.io/name":    "log-area-provider",
		"app.kubernetes.io/part-of": "etos",
	}

	spec.ID = uuid.NewString()
	spec.ProviderID = environmentrequest.Spec.Providers.LogArea.ID
	spec.EnvironmentRequest = environmentrequest.Name

	var generateName string
	if name == "" {
		generateName = fmt.Sprintf("%s-log-area-", strings.ToLower(environmentrequest.Spec.Name))
	}

	isController := false
	blockOwnerDeletion := true
	logArea = v1alpha2.LogArea{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Name:         name,
			GenerateName: generateName,
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

	return &logArea, cli.Create(ctx, &logArea)
}

// DeleteLogArea deletes an LogArea resource from Kubernetes.
func DeleteLogArea(ctx context.Context, logArea *v1alpha2.LogArea) error {
	cli, err := KubernetesClient()
	if err != nil {
		return err
	}
	return cli.Delete(ctx, logArea)
}
