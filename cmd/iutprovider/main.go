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
	"github.com/eiffel-community/etos/pkg/provider"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type genericIutProvider struct{}

// main creates a new Iut resource based on data in an EnvironmentRequest.
func main() {
	provider.RunIutProvider(&genericIutProvider{})
}

// Provision provisions a new IUT.
func (p *genericIutProvider) Provision(ctx context.Context, logger logr.Logger, cfg provider.ProvisionConfig) error {
	ctx, span := cfg.Tracer.Start(
		ctx, "provisionIut", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	// Adds the trace_id, span_id and baggage to the logger, making it searchable if opentelemetry
	// is enabled.
	logger = logging.FromContextOrDiscard(ctx)

	environmentRequest := cfg.EnvironmentRequest
	if cfg.MinimumAmount <= 0 {
		err := errors.New("minimum amount of IUTs requested is less than or equal to 0")
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid minimum amount of IUTs requested")
		return err
	}
	logger.Info("Provisioning a new IUT for EnvironmentRequest",
		"EnvironmentRequest", environmentRequest.Name,
		"Namespace", environmentRequest.Namespace,
		"Amount", cfg.MinimumAmount,
	)

	ctx, span = cfg.Tracer.Start(
		ctx, "createIuts", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	logger = logging.FromContextOrDiscard(ctx)
	for range cfg.MinimumAmount {
		logger.Info("Creating a generic IUT")
		iut, err := provider.CreateIUT(ctx, environmentRequest, cfg.Namespace, "", v1alpha2.IutSpec{})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to create IUT")
			return err
		}
		logger.Info(fmt.Sprintf("IUT created with name '%s'", iut.Name))
	}
	span.SetStatus(codes.Ok, "IUTs provisioned successfully")
	return nil
}

// Release releases an IUT.
func (p *genericIutProvider) Release(ctx context.Context, logger logr.Logger, cfg provider.ReleaseConfig) error {
	ctx, span := cfg.Tracer.Start(
		ctx, "releaseIut", trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	// Adds the trace_id, span_id and baggage to the logger, making it searchable if opentelemetry
	// is enabled.
	logger = logging.FromContextOrDiscard(ctx)

	logger.Info(fmt.Sprintf("Releasing IUT '%s'", cfg.Name), "Namespace", cfg.Namespace)
	iut, err := provider.GetIUT(ctx, cfg.Name, cfg.Namespace)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("failed to get IUT '%s'", cfg.Name))
		return err
	}
	if cfg.NoDelete {
		span.SetStatus(codes.Ok, "no-delete flag is set, skipping deletion of IUT")
		return nil
	}
	if err := provider.DeleteIUT(ctx, iut); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("failed to delete IUT '%s'", cfg.Name))
		return err
	}
	logger.Info(fmt.Sprintf("IUT '%s' released", cfg.Name))
	span.SetStatus(codes.Ok, "IUT released successfully")
	return nil
}
