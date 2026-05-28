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
	"errors"
	"fmt"
	"maps"
	"os"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/fernet/fernet-go"
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
	if err := run(provider, ParseParameters("ExecutionSpace", GetIUTCount)); err != nil {
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
// If a name is not provided, a name will be generated based on the EnvironmentRequest name.
// If a name is provided it is the caller's responsibility to ensure name uniqueness, it will
// not be guaranteed by this function.
func CreateExecutionSpace(
	ctx context.Context,
	environmentrequest *v1alpha1.EnvironmentRequest,
	namespace, name string,
	spec v1alpha2.ExecutionSpaceSpec,
) (*v1alpha2.ExecutionSpace, error) {
	logger := logr.FromContextOrDiscard(ctx)
	var executionSpace v1alpha2.ExecutionSpace

	logger.Info("Getting Kubernetes client")
	cli, err := KubernetesClient()
	if err != nil {
		return nil, err
	}
	environmentVariables, err := environmentVariables(ctx, environmentrequest)
	if err != nil {
		return nil, err
	}

	// Copies environment variables from the spec into the environmentVariables.
	// This means that spec.Instructions.Environment will overwrite any environment
	// variables with the same name as those generated from the EnvironmentRequest config.
	maps.Copy(environmentVariables, spec.Instructions.Environment)
	spec.Instructions.Environment = environmentVariables

	labels := map[string]string{
		"app.kubernetes.io/name":    "execution-space-provider",
		"app.kubernetes.io/part-of": "etos",
	}

	spec.ProviderID = environmentrequest.Spec.Providers.ExecutionSpace.ID
	spec.EnvironmentRequest = environmentrequest.Name

	var generateName string
	if name == "" {
		// 63 is the maximum length for generateName, and we need to reserve one character for the
		// hyphen added by generateName, so we use 62 here.
		generateName = fmt.Sprintf("%s-", toRFC1123(environmentrequest.Spec.Name, 62))
	}

	isController := false
	blockOwnerDeletion := true
	executionSpace = v1alpha2.ExecutionSpace{
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

// environmentVariables gets the environment variables for the Environment from the EnvironmentRequest
// and encrypts sensitive information using the provided encryption key.
func environmentVariables(
	ctx context.Context,
	environmentrequest *v1alpha1.EnvironmentRequest,
) (map[string]string, error) {
	var environment map[string]string
	cli, err := KubernetesClient()
	if err != nil {
		return environment, err
	}
	key, err := environmentrequest.Spec.Config.EncryptionKey.Get(ctx, cli, environmentrequest.Namespace)
	if err != nil {
		return environment, errors.Join(errors.New("failed to get encryption key"), err)
	}
	encryptionKey, err := fernet.DecodeKey(string(key))
	if err != nil {
		return environment, errors.Join(errors.New("failed to decode encryption key"), err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return environment, errors.Join(errors.New("failed to get hostname"), err)
	}
	etosMessagebusPassword, err := getAndEncrypt(ctx,
		cli, environmentrequest.Spec.Config.EtosMessageBus.Password,
		environmentrequest.Namespace, encryptionKey,
	)
	if err != nil {
		return environment, errors.Join(errors.New("failed to get and encrypt ETOS MessageBus password"), err)
	}
	eiffelMessagebusPassword, err := getAndEncrypt(ctx,
		cli, environmentrequest.Spec.Config.EiffelMessageBus.Password,
		environmentrequest.Namespace, encryptionKey,
	)
	if err != nil {
		return environment, errors.Join(errors.New("failed to get and encrypt Eiffel MessageBus password"), err)
	}

	environment = map[string]string{
		"SOURCE_HOST":                 hostname,
		"SUITE_ID":                    environmentrequest.Spec.Identifier,
		"ETOS_API":                    environmentrequest.Spec.Config.EtosApi,
		"ETR_VERSION":                 environmentrequest.Spec.Providers.ExecutionSpace.TestRunner,
		"ETOS_GRAPHQL_SERVER":         environmentrequest.Spec.Config.GraphQlServer,
		"ETOS_RABBITMQ_EXCHANGE":      environmentrequest.Spec.Config.EtosMessageBus.Exchange,
		"ETOS_RABBITMQ_HOST":          environmentrequest.Spec.Config.EtosMessageBus.Host,
		"ETOS_RABBITMQ_PASSWORD":      string(etosMessagebusPassword),
		"ETOS_RABBITMQ_PORT":          environmentrequest.Spec.Config.EtosMessageBus.Port,
		"ETOS_RABBITMQ_USERNAME":      environmentrequest.Spec.Config.EtosMessageBus.Username,
		"ETOS_RABBITMQ_VHOST":         environmentrequest.Spec.Config.EtosMessageBus.Vhost,
		"ETOS_RABBITMQ_SSL":           environmentrequest.Spec.Config.EtosMessageBus.SSL,
		"RABBITMQ_EXCHANGE":           environmentrequest.Spec.Config.EiffelMessageBus.Exchange,
		"RABBITMQ_HOST":               environmentrequest.Spec.Config.EiffelMessageBus.Host,
		"RABBITMQ_PASSWORD":           string(eiffelMessagebusPassword),
		"RABBITMQ_PORT":               environmentrequest.Spec.Config.EiffelMessageBus.Port,
		"RABBITMQ_USERNAME":           environmentrequest.Spec.Config.EiffelMessageBus.Username,
		"RABBITMQ_VHOST":              environmentrequest.Spec.Config.EiffelMessageBus.Vhost,
		"RABBITMQ_SSL":                environmentrequest.Spec.Config.EiffelMessageBus.SSL,
		"OTEL_EXPORTER_OTLP_ENDPOINT": os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		"OTEL_EXPORTER_OTLP_INSECURE": os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"),
	}
	return environment, nil
}

// encrypt encrypts a string using the provided Fernet key.
func encrypt(s []byte, key *fernet.Key) ([]byte, error) {
	return fernet.EncryptAndSign(s, key)
}

// getAndEncrypt gets a value from a Var struct and encrypts it using the provided Fernet key.
func getAndEncrypt(
	ctx context.Context, cli client.Client, s *v1alpha1.Var, namespace string, key *fernet.Key,
) ([]byte, error) {
	if s == nil {
		return nil, errors.New("no value provided")
	}
	value, err := s.Get(ctx, cli, namespace)
	if err != nil {
		return nil, err
	}
	return encrypt(value, key)
}
