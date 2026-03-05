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
	"os"
	"runtime/debug"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// initOpentelemetryTracer initializes an OpenTelemetry tracer and sets it as the global tracer provider.
func initOpentelemetryTracer(ctx context.Context, name string) error {
	logger := logr.FromContextOrDiscard(ctx)
	collector, otelEnabled := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if !otelEnabled {
		logger.Info("OpenTelemetry is not enabled, skipping tracer initialization")
		return nil
	}

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpointURL(collector),
	))
	if err != nil {
		return errors.Join(err, errors.New("failed to create OpenTelemetry exporter"))
	}
	traceExporter := trace.WithBatcher(exporter)

	resource, err := newOtelResource(ctx, name)
	if err != nil {
		return err
	}
	tracerProvider := trace.NewTracerProvider(
		traceExporter,
		trace.WithResource(resource),
	)

	// Set the global tracer provider to the one we just created.
	// This will allow us to use the global tracer provider in our code and have it
	// use the one we just created.
	otel.SetTracerProvider(tracerProvider)
	// Set the global propagator to the TraceContext propagator. This will allow us to
	// propagate trace context across process boundaries using the W3C TraceContext format.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return nil
}

// initOpentelemetryLogger initializes an OpenTelemetry logger and returns an otelzap.Core that can
// be used to create a logger with the OpenTelemetry core.
func initOpentelemetryLogger(ctx context.Context, name string) (*otelzap.Core, error) {
	_, otelEnabled := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if !otelEnabled {
		return nil, nil
	}
	exporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create OpenTelemetry gRPC log exporter"))
	}
	resource, err := newOtelResource(ctx, name)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create OpenTelemetry resource for logger"))
	}
	logProvider := log.NewLoggerProvider(
		log.WithResource(resource),
		log.WithProcessor(
			log.NewBatchProcessor(exporter),
		),
	)
	return otelzap.NewCore(
		name,
		otelzap.WithLoggerProvider(logProvider),
	), nil
}

// newOtelResource returns an OpenTelemetry resource with appropriate metadata for the provider.
func newOtelResource(ctx context.Context, name string) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(name),
			semconv.ServiceNamespaceKey.String(os.Getenv("OTEL_SERVICE_NAMESPACE")),
			semconv.ServiceVersionKey.String(vcsRevision()),
		),
		resource.WithTelemetrySDK(),
		resource.WithProcess(),
		resource.WithOS(),
	)
}

// vcsRevision returns the vcs revision from the build.
func vcsRevision() string {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	for _, val := range buildInfo.Settings {
		if val.Key == "vcs.revision" {
			return val.Value
		}
	}
	return "(unknown)"
}
