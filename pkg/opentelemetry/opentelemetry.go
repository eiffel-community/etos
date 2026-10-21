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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/go-logr/logr"
	goruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// ETOSTracer configures OpenTelemetry metrics, tracing and logging with optional Go runtime instrumentation.
type ETOSTracer struct {
	enabled        bool
	Name           string
	namespace      string
	tracerProvider *trace.TracerProvider
	LoggerProvider *log.LoggerProvider
	MeterProvider  *sdkmetric.MeterProvider
	shutdownOnce   sync.Once
	shutdownErr    error
	exporters      exporterFactories
}

type exporterFactories struct {
	traces  func(context.Context) (trace.SpanExporter, error)
	logs    func(context.Context) (log.Exporter, error)
	metrics func(context.Context) (sdkmetric.Exporter, error)
}

// buildRevision is injected by the linker when building container images.
var buildRevision string

// New creates a new ETOSTracer with the given name and namespace.
func New(serviceName, serviceNamespace string) *ETOSTracer {
	_, otelEnabled := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	return &ETOSTracer{
		Name:      serviceName,
		namespace: serviceNamespace,
		enabled:   otelEnabled,
		exporters: exporterFactories{
			traces: func(ctx context.Context) (trace.SpanExporter, error) { return otlptracegrpc.New(ctx) },
			logs:   func(ctx context.Context) (log.Exporter, error) { return otlploggrpc.New(ctx) },
			metrics: func(ctx context.Context) (sdkmetric.Exporter, error) {
				return otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithTemporalitySelector(sdkmetric.DefaultTemporalitySelector))
			},
		},
	}
}

// Start initializes all telemetry providers, including optional external metric producers.
func (t *ETOSTracer) Start(ctx context.Context, producers ...sdkmetric.Producer) error {
	return t.start(ctx, false, producers...)
}

// StartWithRuntimeMetrics initializes OpenTelemetry and reports Go runtime metrics.
func (t *ETOSTracer) StartWithRuntimeMetrics(ctx context.Context) error {
	return t.start(ctx, true)
}

// start initializes OpenTelemetry providers and optionally enables Go runtime metrics.
func (t *ETOSTracer) start(ctx context.Context, withRuntimeMetrics bool, producers ...sdkmetric.Producer) (err error) {
	logger := logr.FromContextOrDiscard(ctx)
	// Propagation is product behavior, including when network export is disabled.
	if !t.enabled {
		installPropagator()
		otel.SetMeterProvider(noop.NewMeterProvider())
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		logger.Info("OpenTelemetry is not enabled, skipping exporter initialization")
		return nil
	}
	interval, timeout, err := validateConfiguration()
	if err != nil {
		return err
	}
	res, err := t.newOtelResource(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, t.Shutdown(context.Background()))
		}
	}()
	exporter, err := t.exporters.traces(ctx)
	if err != nil {
		return fmt.Errorf("create trace exporter: %w", err)
	}
	t.tracerProvider = trace.NewTracerProvider(trace.WithBatcher(exporter), trace.WithResource(res))
	if err = t.logger(ctx, res); err != nil {
		return err
	}
	metricExporter, err := t.exporters.metrics(ctx)
	if err != nil {
		return fmt.Errorf("create metric exporter: %w", err)
	}
	readerOptions := []sdkmetric.PeriodicReaderOption{
		sdkmetric.WithInterval(interval), sdkmetric.WithTimeout(timeout),
	}
	for _, producer := range producers {
		readerOptions = append(readerOptions, sdkmetric.WithProducer(producer))
	}
	options := MetricProviderOptions()
	options = append(options, sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, readerOptions...)))
	t.MeterProvider = sdkmetric.NewMeterProvider(options...)
	if withRuntimeMetrics {
		if err = goruntime.Start(goruntime.WithMeterProvider(t.MeterProvider)); err != nil {
			return fmt.Errorf("start Go runtime metrics: %w", err)
		}
	}
	otel.SetTracerProvider(t.tracerProvider)
	otel.SetMeterProvider(t.MeterProvider)
	installPropagator()
	return nil
}

// installPropagator installs global W3C trace-context and baggage propagation.
func installPropagator() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
}

// logger initializes the OpenTelemetry logger provider and sets it as the global logger provider.
func (t *ETOSTracer) logger(ctx context.Context, res *resource.Resource) error {
	exporter, err := t.exporters.logs(ctx)
	if err != nil {
		return errors.Join(err, errors.New("failed to create OpenTelemetry gRPC log exporter"))
	}
	logProvider := log.NewLoggerProvider(
		log.WithResource(res),
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
	carrier := carrierFromEnvironmentRequest(environmentRequest)
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(carrier))
}

// carrierFromEnvironmentRequest extracts the traceparent and baggage from the EnvironmentRequest
// annotations and returns them as a map.
func carrierFromEnvironmentRequest(environmentRequest *v1alpha1.EnvironmentRequest) map[string]string {
	return map[string]string{
		"traceparent": environmentRequest.Annotations["etos.eiffel-community.github.io/traceparent"],
		"tracestate":  environmentRequest.Annotations["etos.eiffel-community.github.io/tracestate"],
		"baggage":     environmentRequest.Annotations["etos.eiffel-community.github.io/baggage"],
	}
}

// Shutdown flushes and shuts down all telemetry providers with an independent budget for each signal.
func (t *ETOSTracer) Shutdown(ctx context.Context) error {
	t.shutdownOnce.Do(func() {
		var shutdowns []func(context.Context) error
		if t.MeterProvider != nil {
			shutdowns = append(shutdowns, t.MeterProvider.Shutdown)
		}
		if t.tracerProvider != nil {
			shutdowns = append(shutdowns, t.tracerProvider.Shutdown)
		}
		if t.LoggerProvider != nil {
			shutdowns = append(shutdowns, t.LoggerProvider.Shutdown)
		}
		// Each signal gets its own opportunity; canceled work never cancels cleanup.
		budget := 15 * time.Second
		if deadline, ok := ctx.Deadline(); ok && ctx.Err() == nil {
			budget = min(budget, time.Until(deadline))
		}
		for _, shutdown := range shutdowns {
			signalCtx, cancel := context.WithTimeout(context.Background(), budget/time.Duration(len(shutdowns)))
			t.shutdownErr = errors.Join(t.shutdownErr, shutdown(signalCtx))
			cancel()
		}
	})
	return t.shutdownErr
}

// MetricProviderOptions fixes custom histogram aggregation and cardinality.
// 100,000 attribute sets per instrument supports 1,000 clusters (30 resource
// state combinations each) with ample headroom. Overflow is incomplete telemetry.
func MetricProviderOptions() []sdkmetric.Option {
	transitionBounds := []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1800, 3600, 21600, 86400}
	messagingBounds := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	opts := make([]sdkmetric.Option, 0, 4)
	opts = append(opts, sdkmetric.WithCardinalityLimit(100000))
	for name, bounds := range map[string][]float64{
		"etos_testrun_duration_seconds":             transitionBounds,
		"etos_environment_request_duration_seconds": transitionBounds,
		"etos_message_publish_duration_seconds":     messagingBounds,
	} {
		opts = append(opts, sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Name: name},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: bounds}},
		)))
	}
	return opts
}

// validateConfiguration checks OTLP endpoints, protocols, TLS and temporality settings,
// returning the metric export interval and timeout.
func validateConfiguration() (time.Duration, time.Duration, error) {
	for _, signal := range []string{"", "_METRICS", "_TRACES", "_LOGS"} {
		key := "OTEL_EXPORTER_OTLP" + signal
		if endpoint, set := os.LookupEnv(key + "_ENDPOINT"); set {
			u, err := url.Parse(endpoint)
			if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
				u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return 0, 0, fmt.Errorf("%s_ENDPOINT must be an absolute http(s) URL without credentials, query, or fragment", key)
			}
		}
		if protocol, set := os.LookupEnv(key + "_PROTOCOL"); set && protocol != "grpc" {
			return 0, 0, fmt.Errorf("%s_PROTOCOL must be grpc", key)
		}
		if err := validateTLS(key); err != nil {
			return 0, 0, err
		}
	}
	value, set := os.LookupEnv("OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE")
	if set && value != "cumulative" && value != "CUMULATIVE" {
		return 0, 0, errors.New("metric temporality must be cumulative")
	}
	interval, err := durationEnvironment("OTEL_METRIC_EXPORT_INTERVAL", 15*time.Second)
	if err != nil {
		return 0, 0, err
	}
	timeout, err := durationEnvironment("OTEL_METRIC_EXPORT_TIMEOUT", 5*time.Second)
	return interval, timeout, err
}

// validateTLS prevents exporters' environment parsing warnings from silently
// downgrading an invalid operator-supplied certificate configuration.
func validateTLS(prefix string) error {
	if path, set := os.LookupEnv(prefix + "_CERTIFICATE"); set {
		pem, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s_CERTIFICATE: %w", prefix, err)
		}
		if !x509.NewCertPool().AppendCertsFromPEM(pem) {
			return fmt.Errorf("%s_CERTIFICATE contains no valid CA certificates", prefix)
		}
	}
	cert, hasCert := os.LookupEnv(prefix + "_CLIENT_CERTIFICATE")
	key, hasKey := os.LookupEnv(prefix + "_CLIENT_KEY")
	if hasCert != hasKey {
		return fmt.Errorf("%s client certificate and key must be configured together", prefix)
	}
	if hasCert {
		if _, err := tls.LoadX509KeyPair(cert, key); err != nil {
			return fmt.Errorf("load %s client credentials: %w", prefix, err)
		}
	}
	return nil
}

// durationEnvironment reads a positive millisecond duration of at most one hour,
// using the fallback only when the environment variable is unset.
func durationEnvironment(key string, fallback time.Duration) (time.Duration, error) {
	value, set := os.LookupEnv(key)
	if !set {
		return fallback, nil
	}
	ms, err := strconv.ParseInt(value, 10, 64)
	if err != nil || ms <= 0 || ms > int64(time.Hour/time.Millisecond) {
		return 0, fmt.Errorf("%s must be positive milliseconds no greater than one hour", key)
	}
	return time.Duration(ms) * time.Millisecond, nil
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
			semconv.ServiceNamespaceKey.String(t.namespace),
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
	if buildRevision != "" {
		return buildRevision
	}
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
