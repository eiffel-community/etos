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
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"go.opentelemetry.io/contrib/bridges/otelzap"
	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel"
	rawzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/utils/clock"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/go-logr/logr"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	etosv1alpha2 "github.com/eiffel-community/etos/api/v1alpha2"
	etosv1beta1 "github.com/eiffel-community/etos/api/v1beta1"
	"github.com/eiffel-community/etos/internal/config"
	"github.com/eiffel-community/etos/internal/controller"
	"github.com/eiffel-community/etos/internal/messaging"
	healthmetrics "github.com/eiffel-community/etos/internal/metrics"
	webhookv1alpha1 "github.com/eiffel-community/etos/internal/webhook/v1alpha1"
	webhookv1alpha2 "github.com/eiffel-community/etos/internal/webhook/v1alpha2"
	"github.com/eiffel-community/etos/pkg/logging"
	"github.com/eiffel-community/etos/pkg/opentelemetry"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// init registers the Kubernetes and ETOS API types used by the manager.
func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(etosv1alpha1.AddToScheme(scheme))
	utilruntime.Must(etosv1alpha2.AddToScheme(scheme))
	utilruntime.Must(etosv1beta1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// main configures and starts the ETOS controller manager.
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Manager stopped:", err)
		os.Exit(1)
	}
}

// run owns all post-initialization cleanup paths.
//
//nolint:gocyclo // Explicit controller/webhook registrations preserve scaffold insertion points.
func run() error {
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8181", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the webhook server")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	consoleLog := zap.New(zap.UseFlagOptions(&opts)).WithName("telemetry")
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		consoleLog.Error(err, "OpenTelemetry export failed")
	}))
	ctx := logr.NewContext(ctrl.SetupSignalHandler(), consoleLog)
	otelTracer := opentelemetry.New("etos-controller-manager", "etos")
	producer := prombridge.NewMetricProducer(prombridge.WithGatherer(ctrlmetrics.Registry))
	if err := otelTracer.Start(ctx, producer); err != nil {
		return fmt.Errorf("initialize telemetry: %w", err)
	}
	var inventoryCollector *healthmetrics.Collector
	defer func() {
		if err := otelTracer.Shutdown(context.Background()); err != nil {
			consoleLog.Error(err, "Failed to shutdown OpenTelemetry")
		}
		if inventoryCollector != nil {
			if err := inventoryCollector.Unregister(); err != nil {
				consoleLog.Error(err, "Failed to unregister inventory callback")
			}
		}
	}()
	if otelTracer.LoggerProvider != nil {
		opts.ZapOpts = append(opts.ZapOpts, rawzap.WrapCore(func(core zapcore.Core) zapcore.Core {
			otelCore := otelzap.NewCore(otelTracer.Name, otelzap.WithLoggerProvider(otelTracer.LoggerProvider))
			return zapcore.NewTee(logging.ConsoleCore(core), otelCore)
		}))
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	ctx = logr.NewContext(ctx, setupLog)
	meter := otel.Meter("etos/controller")

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	transitionMetrics, err := healthmetrics.NewTransitions(meter)
	if err != nil {
		return fmt.Errorf("initialize transition metrics: %w", err)
	}
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return err
	}
	drainTimeout := 15 * time.Second
	mgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{ByObject: map[client.Object]cache.ByObject{
			&etosv1alpha1.Cluster{}: {Transform: transitionMetrics.ClusterTransform},
		}},
		Metrics:                 metricsserver.Options{BindAddress: "0"},
		GracefulShutdownTimeout: &drainTimeout,
		WebhookServer:           webhookServer,
		HealthProbeBindAddress:  probeAddr,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "8ff393c5.eiffel-community.github.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		return err
	}

	cfg := config.New()
	setupLog.Info("ETOS configuration",
		"API Image", cfg.API.Image,
		"API Version", cfg.API.Version,
		"SSE Image", cfg.SSE.Image,
		"SSE Version", cfg.SSE.Version,
		"LogArea Image", cfg.LogArea.Image,
		"LogArea Version", cfg.LogArea.Version,
		"SuiteStarter Image", cfg.SuiteStarter.Image,
		"SuiteStarter Version", cfg.SuiteStarter.Version,
		"SuiteRunner Image", cfg.SuiteRunner.Image,
		"SuiteRunner Version", cfg.SuiteRunner.Version,
		"LogListener Image", cfg.LogListener.Image,
		"LogListener Version", cfg.LogListener.Version,
		"TestRunner Version", cfg.TestRunner.Version,
		"EnvironmentProvider Image", cfg.EnvironmentProvider.Image,
		"EnvironmentProvider Version", cfg.EnvironmentProvider.Version,
		"IutProvider Image", cfg.IutProvider.Image,
		"IutProvider Version", cfg.IutProvider.Version,
		"LogAreaProvider Image", cfg.LogAreaProvider.Image,
		"LogAreaProvider Version", cfg.LogAreaProvider.Version,
		"ExecutionSpaceProvider Image", cfg.ExecutionSpaceProvider.Image,
		"ExecutionSpaceProvider Version", cfg.ExecutionSpaceProvider.Version,
		"EventRepositoryAPI Image", cfg.EventRepositoryAPI.Image,
		"EventRepositoryAPI Version", cfg.EventRepositoryAPI.Version,
		"EventRepositoryStorage Image", cfg.EventRepositoryStorage.Image,
		"EventRepositoryStorage Version", cfg.EventRepositoryStorage.Version,
	)

	publisherPool := messaging.NewPublisherPool(ctx)
	defer func() {
		if err := publisherPool.Close(); err != nil {
			setupLog.Error(err, "Failed to close messaging publisher pool")
		}
	}()

	if err = (&controller.TestRunReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Clock:       &clock.RealClock{},
		Tracer:      otel.Tracer("testrun-controller"),
		Publishers:  publisherPool,
		Transitions: transitionMetrics,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "TestRun")
		return err
	}
	inventoryCollector = &healthmetrics.Collector{
		Reader: mgr.GetCache(),
		Scope:  "cluster",
	}
	if err := inventoryCollector.Register(meter); err != nil {
		setupLog.Error(err, "Failed to register inventory collector")
		return err
	}
	if err := mgr.Add(inventoryCollector); err != nil {
		setupLog.Error(err, "Failed to add inventory collector")
		return err
	}
	if err := (&controller.ProviderReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "Provider")
		return err
	}
	if err := (&controller.EnvironmentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "Environment")
		return err
	}
	if err := (&controller.ClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "Cluster")
		return err
	}
	if err := (&controller.EnvironmentRequestReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Transitions: transitionMetrics,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "EnvironmentRequest")
		return err
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha1.SetupTestRunWebhookWithManager(mgr, cfg); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "TestRun")
			return err
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha1.SetupEnvironmentRequestWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "EnvironmentRequest")
			return err
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha1.SetupProviderWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "Provider")
			return err
		}
	}
	if err := (&controller.IutReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Iut")
		return err
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha2.SetupIutWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "Iut")
			return err
		}
	}
	if err := (&controller.ExecutionSpaceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ExecutionSpace")
		return err
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha2.SetupExecutionSpaceWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "ExecutionSpace")
			return err
		}
	}
	if err := (&controller.LogAreaReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LogArea")
		return err
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha2.SetupLogAreaWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "LogArea")
			return err
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha1.SetupClusterWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "Cluster")
			return err
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha1.SetupEnvironmentWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "Environment")
			return err
		}
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		return err
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Failed to run manager")
		return err
	}
	return nil
}
