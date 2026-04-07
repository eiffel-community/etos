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
package opentelemetry

import (
	"context"
	"errors"
	"os"
	"runtime/debug"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.9.0"
)

type ETOSTracer struct {
	enabled        bool
	collectorHost  string
	Name           string
	providerType   string
	tracerProvider *trace.TracerProvider
	LoggerProvider *log.LoggerProvider
}

// New creates a new ETOSTracer with the given name and provider type.
// It checks the environment variables to determine if OpenTelemetry is enabled and what the
// collector host is.
func New(name, providerType string) *ETOSTracer {
	collectorHost, otelEnabled := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	return &ETOSTracer{
		Name:          name,
		providerType:  providerType,
		enabled:       otelEnabled,
		collectorHost: collectorHost,
	}
}

// Start initializes the OpenTelemetry tracer provider and sets it as the global tracer provider.
func (t *ETOSTracer) Start(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)
	if !t.enabled {
		logger.Info("OpenTelemetry is not enabled, skipping tracer initialization")
		return nil
	}
	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(t.opts(t.collectorHost)...))
	if err != nil {
		return errors.Join(err, errors.New("failed to create OpenTelemetry exporter"))
	}

	traceExporter := trace.WithBatcher(exporter)
	resource, err := t.newOtelResource(ctx)
	if err != nil {
		return err
	}

	tracerProvider := trace.NewTracerProvider(
		traceExporter,
		trace.WithResource(resource),
	)
	t.tracerProvider = tracerProvider

	// Set the global tracer provider to the one we just created.
	// This will allow us to use the global tracer provider in our code and have it
	// use the one we just created.
	otel.SetTracerProvider(tracerProvider)
	// Set the global propagator to the TraceContext propagator. This will allow us to
	// propagate trace context across process boundaries using the W3C TraceContext format.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return t.logger(ctx, resource)
}

// logger initializes the OpenTelemetry logger provider and sets it as the global logger provider.
func (t *ETOSTracer) logger(ctx context.Context, resource *resource.Resource) error {
	exporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return errors.Join(err, errors.New("failed to create OpenTelemetry gRPC log exporter"))
	}
	logProvider := log.NewLoggerProvider(
		log.WithResource(resource),
		log.WithProcessor(
			log.NewBatchProcessor(exporter),
		),
	)
	t.LoggerProvider = logProvider
	return nil
}

// ContextFromEnvironmentRequest extracts the trace context from the EnvironmentRequest and
// returns a new context with the trace context.
func (t *ETOSTracer) ContextFromEnvironmentRequest(
	ctx context.Context,
	environmentRequest *v1alpha1.EnvironmentRequest,
) context.Context {
	if !t.enabled {
		return ctx
	}
	carrier := carrierFromEnvironmentRequest(environmentRequest)
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(carrier))
}

// carrierFromEnvironmentRequest extracts the traceparent and baggage from the EnvironmentRequest
// annotations and returns them as a map.
func carrierFromEnvironmentRequest(environmentRequest *v1alpha1.EnvironmentRequest) map[string]string {
	return map[string]string{
		"traceparent": environmentRequest.Annotations["etos.eiffel-community.github.io/traceparent"],
		"baggage":     environmentRequest.Annotations["etos.eiffel-community.github.io/baggage"],
	}
}

// Shutdown shuts down the tracer provider, flushing any remaining spans to the collector.
func (t *ETOSTracer) Shutdown(ctx context.Context) error {
	if !t.enabled {
		return nil
	}
	return errors.Join(
		t.tracerProvider.Shutdown(ctx),
		t.LoggerProvider.Shutdown(ctx),
	)
}

// opts returns the options for the OpenTelemetry gRPC exporter based on the environment variables.
func (t *ETOSTracer) opts(collector string) []otlptracegrpc.Option {
	var opts []otlptracegrpc.Option
	opts = append(opts, otlptracegrpc.WithEndpointURL(collector))

	insecure, isSet := os.LookupEnv("OTEL_EXPORTER_OTLP_INSECURE")
	if isSet && insecure == "true" {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	return opts
}

// newOtelResource returns an OpenTelemetry resource with appropriate metadata for the provider.
func (t *ETOSTracer) newOtelResource(ctx context.Context) (*resource.Resource, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to get hostname for OpenTelemetry resource"))
	}
	return resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(t.Name),
			semconv.ServiceNamespaceKey.String(t.providerType),
			semconv.ServiceInstanceIDKey.String(hostname),
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
