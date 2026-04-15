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
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type genericIutProvider struct{}

// main creates a new Iut resource based on data in an EnvironmentRequest.
func main() {
	provider.RunIutProvider(&genericIutProvider{})
}

// Provision provisions a new IUT.
func (p *genericIutProvider) Provision(ctx context.Context, cfg provider.ProvisionConfig) error {
	ctx, span := cfg.Tracer.Start(ctx, "Provision", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	logger := logging.FromContextOrDiscard(ctx)
	if cfg.MinimumAmount <= 0 {
		err := errors.New("minimum amount of IUTs requested is less than or equal to 0")
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid minimum amount of IUTs requested")
		return err
	}
	logger.Info("Provisioning a new IUT for EnvironmentRequest",
		"EnvironmentRequest", cfg.EnvironmentRequest.Name,
		"Namespace", cfg.EnvironmentRequest.Namespace,
		"Amount", cfg.MinimumAmount,
	)
	if err := p.createIUTs(ctx, cfg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create IUTs")
		return err
	}
	span.SetStatus(codes.Ok, "successfully provisioned IUTs")
	return nil
}

// createIUTs creates the specified number of IUTs for an EnvironmentRequest.
func (p *genericIutProvider) createIUTs(ctx context.Context, cfg provider.ProvisionConfig) error {
	logger := logging.FromContextOrDiscard(ctx)
	ctx, span := cfg.Tracer.Start(ctx, "CreateIUTs", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	logger = logging.FromContextOrDiscard(ctx)

	for range cfg.MinimumAmount {
		logger.Info("Creating a generic IUT")
		iut, err := provider.CreateIUT(ctx, cfg.EnvironmentRequest, cfg.Namespace, "", v1alpha2.IutSpec{})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to create IUT")
			return err
		}
		logger.Info(fmt.Sprintf("IUT created with name '%s'", iut.Name))
	}
	return nil
}

// Release releases an IUT.
func (p *genericIutProvider) Release(ctx context.Context, cfg provider.ReleaseConfig) error {
	ctx, span := cfg.Tracer.Start(ctx, "Release", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	logger := logging.FromContextOrDiscard(ctx)

	logger.Info(fmt.Sprintf("Releasing IUT with name %s", cfg.Name), "Namespace", cfg.Namespace)
	iut, err := provider.GetIUT(ctx, cfg.Name, cfg.Namespace)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get IUT")
		return err
	}
	if cfg.NoDelete {
		span.SetStatus(codes.Ok, "no-delete flag set, skipping deletion of IUT")
		return nil
	}
	if err := provider.DeleteIUT(ctx, iut); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete IUT")
		return err
	}
	logger.Info(fmt.Sprintf("IUT '%s' released", cfg.Name))
	span.SetStatus(codes.Ok, "successfully released IUT")
	return nil
}
