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
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/pkg/logging"
	"github.com/eiffel-community/etos/pkg/opentelemetry/semconv"
	"github.com/eiffel-community/etos/pkg/provider"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type genericLogAreaProvider struct{}

// main creates a new LogArea resource based on data in an EnvironmentRequest.
func main() {
	provider.RunLogAreaProvider(&genericLogAreaProvider{})
}

// Provision provisions a new LogArea.
func (p *genericLogAreaProvider) Provision(
	ctx context.Context, logger logr.Logger, cfg provider.ProvisionConfig,
) error {

	ctx, span := cfg.Tracer.Start(
		ctx, "provisionLogArea", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	// Adds the trace_id, span_id and baggage to the logger, making it searchable if opentelemetry
	// is enabled.
	logger = logging.FromContextOrDiscard(ctx)

	environmentRequest := cfg.EnvironmentRequest
	if cfg.MinimumAmount <= 0 {
		err := errors.New("minimum amount of LogAreas requested is less than or equal to 0")
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid minimum amount of LogAreas requested")
		return err
	}
	logger.Info("Provisioning a new LogArea for EnvironmentRequest",
		"EnvironmentRequest", environmentRequest.Name,
		"Namespace", environmentRequest.Namespace,
		"Amount", cfg.MinimumAmount,
	)
	logAreaProvider, err := provider.GetProvider(ctx, environmentRequest.Spec.Providers.LogArea.ID, cfg.Namespace)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get LogAreaProvider")
		return err
	}
	if logAreaProvider.Spec.LogAreaProviderConfig == nil {
		err := errors.New("LogAreaProviderConfig is nil, and shouldn't be.")
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid LogAreaProviderConfig")
		return err
	}

	ctx, span = cfg.Tracer.Start(
		ctx, "createLogAreas", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	logger = logging.FromContextOrDiscard(ctx)

	span.SetAttributes(
		semconv.ETOSLogAreaProviderLiveLogs(logAreaProvider.Spec.LogAreaProviderConfig.LiveLogs),
		semconv.ETOSLogAreaProviderLogAreaUploadURL(logAreaProvider.Spec.LogAreaProviderConfig.Upload.URL),
	)

	for range cfg.MinimumAmount {
		logger.Info("Creating a generic LogArea")
		logger.V(0).Info(fmt.Sprintf("Logs will be uploaded to %s", logAreaProvider.Spec.LogAreaProviderConfig.Upload.URL))
		logArea, err := provider.CreateLogArea(ctx, environmentRequest, cfg.Namespace, "", v1alpha2.LogAreaSpec{
			LiveLogs: logAreaProvider.Spec.LogAreaProviderConfig.LiveLogs,
			Logs:     map[string]string{},
			Upload:   logAreaProvider.Spec.LogAreaProviderConfig.Upload,
		})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to create LogArea")
			return err
		}
		logger.Info(fmt.Sprintf("LogArea created with name %s", logArea.Name))
	}
	span.SetStatus(codes.Ok, "LogArea(s) provisioned successfully")
	return nil
}

// Release releases a LogArea.
func (p *genericLogAreaProvider) Release(ctx context.Context, logger logr.Logger, cfg provider.ReleaseConfig) error {
	ctx, span := cfg.Tracer.Start(
		ctx, "releaseLogArea", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	// Adds the trace_id, span_id and baggage to the logger, making it searchable if opentelemetry
	// is enabled.
	logger = logging.FromContextOrDiscard(ctx)

	logger.Info(fmt.Sprintf("Releasing LogArea '%s'", cfg.Name), "Namespace", cfg.Namespace)
	logArea, err := provider.GetLogArea(ctx, cfg.Name, cfg.Namespace)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get LogArea")
		return err
	}
	if cfg.NoDelete {
		span.SetStatus(codes.Ok, "no-delete flag is set, skipping deletion of LogArea")
		return nil
	}
	if err := provider.DeleteLogArea(ctx, logArea); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete LogArea")
		return err
	}
	logger.Info(fmt.Sprintf("LogArea '%s' released", cfg.Name))
	span.SetStatus(codes.Ok, "LogArea released successfully")
	return nil
}
