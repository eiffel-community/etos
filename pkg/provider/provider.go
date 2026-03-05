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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/internal/controller/jobs"
	"github.com/eiffel-community/etos/internal/messaging"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	cli            client.Client
	Scheme         = runtime.NewScheme()
	terminationLog = "/dev/termination-log"
)

type AmountFunc func(context.Context, *v1alpha1.EnvironmentRequest) (int, error)

type Parameters struct {
	providerType           string
	amountFunc             AmountFunc
	environmentRequestName string
	namespace              string
	name                   string
	providerName           string
	releaseEnvironment     bool
	noDelete               bool
	opts                   zap.Options
}

type ProvisionConfig struct {
	MinimumAmount      int
	MaximumAmount      int
	Namespace          string
	EnvironmentRequest *v1alpha1.EnvironmentRequest
}

type ReleaseConfig struct {
	Name               string
	Namespace          string
	NoDelete           bool
	EnvironmentRequest *v1alpha1.EnvironmentRequest
}

// Provider is an interface for providers to implement for the Run* functions.
type Provider interface {
	Provision(ctx context.Context, logger logr.Logger, cfg ProvisionConfig) error
	Release(ctx context.Context, logger logr.Logger, cfg ReleaseConfig) error
}

// init sets up the ETOS controller schemes as well as the default schemes from client-go.
func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(Scheme))

	utilruntime.Must(v1alpha1.AddToScheme(Scheme))
	utilruntime.Must(v1alpha2.AddToScheme(Scheme))
}

// ParseParameters parses the input parameters for a provider.
func ParseParameters(providerType string, amountFunc AmountFunc) Parameters {
	opts := zap.Options{}
	params := Parameters{}
	flag.BoolVar(&params.releaseEnvironment, "release", false, "Release instead of creating")
	flag.BoolVar(&params.noDelete, "nodelete", false, "Don't delete the resource")
	flag.StringVar(&params.environmentRequestName, "environment-request", "", "The environment request to provision for.")
	flag.StringVar(&params.name, "name", "", "The name of the resource to release.")
	flag.StringVar(&params.providerName, "provider", "", "The provider used.")
	flag.StringVar(&params.namespace, "namespace", "", "The namespace of the environment request.")
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	params.opts = opts
	params.providerType = providerType
	params.amountFunc = amountFunc

	if params.environmentRequestName == "" {
		panic("Must set -environment-request")
	}
	if params.namespace == "" {
		panic("Must set -namespace")
	}

	return params
}

// run is the main function for running a provider. It will fetch the EnvironmentRequest,
// create a message bus publisher, and a logger, and then call the runProvider function.
func run(provider Provider, params Parameters) error {
	ctx := context.TODO()

	environmentRequest, err := EnvironmentRequest(ctx, params.environmentRequestName, params.namespace)
	if err != nil {
		return fmt.Errorf("failed to get EnvironmentRequest: %w", err)
	}
	client, err := KubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	publisher, err := messaging.NewPublisher(ctx,
		environmentRequest.Spec.Config.EtosMessageBus,
		client,
		params.namespace,
	)
	if err != nil {
		return fmt.Errorf("failed to create message bus publisher: %w", err)
	}
	defer func() {
		if closeErr := publisher.Close(); closeErr != nil {
			fmt.Printf("failed to close message bus publisher: %v\n", closeErr)
		}
	}()
	logger, err := newLogger(ctx, params, publisher)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	logger = logger.WithValues(
		"providerType", params.providerType,
		"environmentRequest", params.environmentRequestName,
		"namespace", params.namespace,
		"providerName", params.providerName,
		"identifier", environmentRequest.Spec.Identifier,
	).WithName(params.providerName)

	publisher.AddLogger(logger)
	ctx = logr.NewContext(ctx, logger)
	logger = logr.FromContextOrDiscard(ctx)
	return writeTerminationLog(ctx, runProvider, logger, provider, params, environmentRequest)
}

// WriteResult writes a job result JSON structure to the termination-log if running in a Kubernetes pod.
func WriteResult(logger logr.Logger, result jobs.Result) error {
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		logger.Info("Provider is not running in a Kubernetes pod, won't write termination-log")
		return nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return os.WriteFile(terminationLog, b, os.ModePerm)
}

// KubernetesClient creates a new Kubernetes client or reuses an already created.
func KubernetesClient() (client.Client, error) {
	var err error
	if cli == nil {
		cli, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: Scheme})
		if err != nil {
			return nil, err
		}
	}
	return cli, err
}

// EnvironmentRequest gets an environment request from Kubernetes by name and namespace.
func EnvironmentRequest(
	ctx context.Context,
	environmentRequestName, namespace string,
) (*v1alpha1.EnvironmentRequest, error) {
	cli, err := KubernetesClient()
	if err != nil {
		return nil, err
	}
	var request v1alpha1.EnvironmentRequest
	if err := cli.Get(
		ctx,
		types.NamespacedName{Name: environmentRequestName, Namespace: namespace},
		&request,
	); err != nil {
		return nil, err
	}
	return &request, nil
}

// GetProvider gets a provider from Kubernetes by name and namespace.
func GetProvider(ctx context.Context, providerName, namespace string) (*v1alpha1.Provider, error) {
	cli, err := KubernetesClient()
	if err != nil {
		return nil, err
	}
	var provider v1alpha1.Provider
	if err := cli.Get(
		ctx,
		types.NamespacedName{Name: providerName, Namespace: namespace},
		&provider,
	); err != nil {
		return nil, err
	}
	return &provider, nil
}

// runProvider runs a provider.
//
// If the releaseEnvironment parameter is set then it will run Release
// If the releaseEnvironment parameter is not set then it will run Provision
func runProvider(ctx context.Context, logger logr.Logger, provider Provider, params Parameters, environmentRequest *v1alpha1.EnvironmentRequest) error {
	if params.releaseEnvironment {
		return runReleaser(ctx, logger, provider, params, environmentRequest)
	}
	minimumAmount, err := params.amountFunc(ctx, environmentRequest)
	if err != nil {
		return err
	}
	return provider.Provision(ctx, logger, ProvisionConfig{
		EnvironmentRequest: environmentRequest,
		Namespace:          params.namespace,
		MaximumAmount:      environmentRequest.Spec.MaximumAmount,
		MinimumAmount:      minimumAmount,
	})
}

// runReleaser runs the provision.Release function
func runReleaser(ctx context.Context, logger logr.Logger, provider Provider, params Parameters, environmentRequest *v1alpha1.EnvironmentRequest) error {
	if params.name == "" {
		return errors.New("Must set -name")
	}
	return provider.Release(ctx, logger, ReleaseConfig{
		EnvironmentRequest: environmentRequest,
		Name:               params.name,
		Namespace:          params.namespace,
		NoDelete:           params.noDelete,
	})
}

// writeTerminationLog will run a function and will write the result into a termination log.
func writeTerminationLog(
	ctx context.Context,
	run func(context.Context, logr.Logger, Provider, Parameters, *v1alpha1.EnvironmentRequest) error,
	logger logr.Logger,
	provider Provider,
	params Parameters,
	environmentRequest *v1alpha1.EnvironmentRequest,
) error {
	err := run(ctx, logger, provider, params, environmentRequest)

	if err != nil {
		if writeErr := WriteResult(logger,
			jobs.Result{
				Conclusion:  jobs.ConclusionFailed,
				Description: err.Error(),
				Verdict:     jobs.VerdictNone,
			}); writeErr != nil {
			logger.Error(writeErr, "failed to write error result to termination-log")
		}
		return err
	}
	var successMessage string
	if params.releaseEnvironment {
		successMessage = fmt.Sprintf("Successfully released %s", params.providerType)
	} else {
		successMessage = fmt.Sprintf("Successfully provisioned %s", params.providerType)
	}
	if err := WriteResult(logger,
		jobs.Result{
			Conclusion:  jobs.ConclusionSuccessful,
			Description: successMessage,
			Verdict:     jobs.VerdictNone,
		}); err != nil {
		logger.Error(err, "failed to write error result to termination-log")
		return err
	}
	return nil
}
