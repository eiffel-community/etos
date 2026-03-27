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
	"github.com/eiffel-community/etos/pkg/logging"
	"github.com/eiffel-community/etos/pkg/opentelemetry"
	"github.com/eiffel-community/etos/pkg/opentelemetry/semconv"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	tracer                 trace.Tracer
	opts                   zap.Options
}

type ProvisionConfig struct {
	MinimumAmount      int
	MaximumAmount      int
	Namespace          string
	Tracer             trace.Tracer
	EnvironmentRequest *v1alpha1.EnvironmentRequest
}

type ReleaseConfig struct {
	Name               string
	Namespace          string
	Tracer             trace.Tracer
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
	params.tracer = otel.Tracer(params.providerName)

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

	otel := opentelemetry.New(params.providerName, params.providerType)
	if err := otel.Start(ctx); err != nil {
		return fmt.Errorf("failed to start OpenTelemetry tracer: %w", err)
	}
	// Will get assigned properly later.
	logger := logr.Discard()
	defer func() {
		if shutdownErr := otel.Shutdown(ctx); shutdownErr != nil {
			logger.Error(shutdownErr, "failed to shutdown OpenTelemetry tracer")
		}
	}()

	environmentRequest, err := EnvironmentRequest(ctx, params.environmentRequestName, params.namespace)
	if err != nil {
		return fmt.Errorf("failed to get EnvironmentRequest: %w", err)
	}
	ctx = otel.ContextFromEnvironmentRequest(ctx, environmentRequest)

	ctx, span := params.tracer.Start(
		ctx, "run-provider", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	span.SetAttributes(
		semconv.ETOSProviderEnvironmentRequest(params.environmentRequestName),
		semconv.ETOSProviderNamespace(params.namespace),
		semconv.ETOSProviderName(params.providerName),
	)

	client, err := KubernetesClient()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create Kubernetes client")
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	publisher, err := messaging.NewPublisher(ctx,
		environmentRequest.Spec.Config.EtosMessageBus,
		client,
		params.namespace,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create message bus publisher")
		return fmt.Errorf("failed to create message bus publisher: %w", err)
	}
	defer func() {
		if closeErr := publisher.Close(); closeErr != nil {
			fmt.Printf("failed to close message bus publisher: %v\n", closeErr)
		}
	}()

	ctx = logging.New(params.opts).
		WithOtel(otel).
		WithUserLog(publisher).
		WithConsole().
		Start(ctx)
	logger = logging.FromContextOrDiscard(ctx).WithValues(
		"providerType", params.providerType,
		"environmentRequest", params.environmentRequestName,
		"namespace", params.namespace,
		"providerName", params.providerName,
		"identifier", environmentRequest.Spec.Identifier,
	).WithName(params.providerName)
	publisher.AddLogger(logger)
	ctx = logr.NewContext(ctx, logger)

	if err := writeTerminationLog(ctx, runProvider, provider, params, environmentRequest); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Provider execution failed")
		return err
	}
	span.SetStatus(codes.Ok, "Provider execution succeeded")
	return nil
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
func runProvider(ctx context.Context, provider Provider, params Parameters, environmentRequest *v1alpha1.EnvironmentRequest) error {
	if params.releaseEnvironment {
		return runReleaser(ctx, provider, params, environmentRequest)
	}
	ctx, span := params.tracer.Start(
		ctx, "provision", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	logger := logging.FromContextOrDiscard(ctx)

	minimumAmount, err := params.amountFunc(ctx, environmentRequest)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to calculate minimum amount")
		return err
	}
	span.SetAttributes(
		semconv.ETOSProviderMinimumAmount(minimumAmount),
		semconv.ETOSProviderMaximumAmount(environmentRequest.Spec.MaximumAmount),
	)
	if err := provider.Provision(ctx, logger, ProvisionConfig{
		EnvironmentRequest: environmentRequest,
		Namespace:          params.namespace,
		MaximumAmount:      environmentRequest.Spec.MaximumAmount,
		MinimumAmount:      minimumAmount,
		Tracer:             params.tracer,
	}); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to provision resource")
		return err
	}
	span.SetStatus(codes.Ok, "Resource provisioned successfully")
	return nil
}

// runReleaser runs the provision.Release function
func runReleaser(ctx context.Context, provider Provider, params Parameters, environmentRequest *v1alpha1.EnvironmentRequest) error {
	ctx, span := params.tracer.Start(
		ctx, "release", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	logger := logging.FromContextOrDiscard(ctx)

	if params.name == "" {
		err := errors.New("Must set -name")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to release resource: name not provided")
		return err
	}
	span.SetAttributes(
		semconv.ETOSProviderName(params.name),
	)
	if err := provider.Release(ctx, logger, ReleaseConfig{
		EnvironmentRequest: environmentRequest,
		Name:               params.name,
		Namespace:          params.namespace,
		NoDelete:           params.noDelete,
		Tracer:             params.tracer,
	}); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to release resource")
		return err
	}
	span.SetStatus(codes.Ok, "Resource released successfully")
	return nil
}

// writeTerminationLog will run a function and will write the result into a termination log.
func writeTerminationLog(
	ctx context.Context,
	run func(context.Context, Provider, Parameters, *v1alpha1.EnvironmentRequest) error,
	provider Provider,
	params Parameters,
	environmentRequest *v1alpha1.EnvironmentRequest,
) error {
	err := run(ctx, provider, params, environmentRequest)

	ctx, span := params.tracer.Start(
		ctx, "writeResult", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	logger := logging.FromContextOrDiscard(ctx)
	if err != nil {
		if writeErr := WriteResult(logger,
			jobs.Result{
				Conclusion:  jobs.ConclusionFailed,
				Description: err.Error(),
				Verdict:     jobs.VerdictNone,
			}); writeErr != nil {
			logger.Error(writeErr, "failed to write error result to termination-log")
			span.RecordError(writeErr)
			span.SetStatus(codes.Error, "Failed to write error result to termination-log")
		}
		return err
	}
	var successMessage string
	if params.releaseEnvironment {
		successMessage = fmt.Sprintf("Successfully released %s", params.providerType)
	} else {
		successMessage = fmt.Sprintf("Successfully provisioned %s", params.providerType)
	}
	span.SetAttributes(
		semconv.ETOSProviderConclusion(string(jobs.ConclusionSuccessful)),
		semconv.ETOSProviderVerdict(string(jobs.VerdictNone)),
		semconv.ETOSProviderDescription(successMessage),
	)
	if err := WriteResult(logger,
		jobs.Result{
			Conclusion:  jobs.ConclusionSuccessful,
			Description: successMessage,
			Verdict:     jobs.VerdictNone,
		}); err != nil {
		logger.Error(err, "failed to write error result to termination-log")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to write success result to termination-log")
		return err
	}
	span.SetStatus(codes.Ok, "Result written to termination-log successfully")
	return nil
}
