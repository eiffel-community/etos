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
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/pkg/logging"
	"github.com/eiffel-community/etos/pkg/provider"
	"github.com/fernet/fernet-go"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type genericExecutionSpaceProvider struct{}

// main creates a new ExecutionSpace resource based on data in an EnvironmentRequest.
func main() {
	provider.RunExecutionSpaceProvider(&genericExecutionSpaceProvider{})
}

type dataset struct {
	Dev       bool   `json:"dev,omitempty"`
	ETRRepo   string `json:"ETR_REPO,omitempty"`
	ETRBranch string `json:"ETR_BRANCH,omitempty"`
}

// Provision provisions a new ExecutionSpace.
func (p *genericExecutionSpaceProvider) Provision(
	ctx context.Context, cfg provider.ProvisionConfig,
) error {
	ctx, span := cfg.Tracer.Start(ctx, "Provision", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	if cfg.MinimumAmount <= 0 {
		err := errors.New("minimum amount of ExecutionSpaces requested is less than or equal to 0")
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid minimum amount of ExecutionSpaces requested")
		return err
	}
	if err := p.createExecutionSpaces(ctx, cfg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create ExecutionSpaces")
		return err
	}
	span.SetStatus(codes.Ok, "successfully provisioned ExecutionSpaces")
	return nil
}

// createEnvironment creates a map of environment variables for the ETOS test runner based on the EnvironmentRequest.
func (p *genericExecutionSpaceProvider) createEnvironment(
	ctx context.Context,
	cfg provider.ProvisionConfig,
) (map[string]string, error) {
	var environment map[string]string
	logger := logging.FromContextOrDiscard(ctx)
	cli, err := provider.KubernetesClient()
	if err != nil {
		return environment, err
	}
	key, err := cfg.EnvironmentRequest.Spec.Config.EncryptionKey.Get(ctx, cli, cfg.EnvironmentRequest.Namespace)
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
	logger.Info("Provisioning a new ExecutionSpace for EnvironmentRequest",
		"EnvironmentRequest", cfg.EnvironmentRequest.Name,
		"Namespace", cfg.EnvironmentRequest.Namespace,
		"Amount", cfg.MinimumAmount,
	)
	etosMessagebusPassword, err := getAndEncrypt(ctx,
		cli, cfg.EnvironmentRequest.Spec.Config.EtosMessageBus.Password,
		cfg.EnvironmentRequest.Namespace, encryptionKey,
	)
	if err != nil {
		return environment, errors.Join(errors.New("failed to get and encrypt ETOS MessageBus password"), err)
	}
	eiffelMessagebusPassword, err := getAndEncrypt(ctx,
		cli, cfg.EnvironmentRequest.Spec.Config.EiffelMessageBus.Password,
		cfg.EnvironmentRequest.Namespace, encryptionKey,
	)
	if err != nil {
		return environment, errors.Join(errors.New("failed to get and encrypt Eiffel MessageBus password"), err)
	}

	environment = map[string]string{
		"SOURCE_HOST":                 hostname,
		"ETOS_API":                    cfg.EnvironmentRequest.Spec.Config.EtosApi,
		"ETR_VERSION":                 cfg.EnvironmentRequest.Spec.Providers.ExecutionSpace.TestRunner,
		"ETOS_GRAPHQL_SERVER":         cfg.EnvironmentRequest.Spec.Config.GraphQlServer,
		"ETOS_RABBITMQ_EXCHANGE":      cfg.EnvironmentRequest.Spec.Config.EtosMessageBus.Exchange,
		"ETOS_RABBITMQ_HOST":          cfg.EnvironmentRequest.Spec.Config.EtosMessageBus.Host,
		"ETOS_RABBITMQ_PASSWORD":      string(etosMessagebusPassword),
		"ETOS_RABBITMQ_PORT":          cfg.EnvironmentRequest.Spec.Config.EtosMessageBus.Port,
		"ETOS_RABBITMQ_USERNAME":      cfg.EnvironmentRequest.Spec.Config.EtosMessageBus.Username,
		"ETOS_RABBITMQ_VHOST":         cfg.EnvironmentRequest.Spec.Config.EtosMessageBus.Vhost,
		"ETOS_RABBITMQ_SSL":           cfg.EnvironmentRequest.Spec.Config.EtosMessageBus.SSL,
		"RABBITMQ_EXCHANGE":           cfg.EnvironmentRequest.Spec.Config.EiffelMessageBus.Exchange,
		"RABBITMQ_HOST":               cfg.EnvironmentRequest.Spec.Config.EiffelMessageBus.Host,
		"RABBITMQ_PASSWORD":           string(eiffelMessagebusPassword),
		"RABBITMQ_PORT":               cfg.EnvironmentRequest.Spec.Config.EiffelMessageBus.Port,
		"RABBITMQ_USERNAME":           cfg.EnvironmentRequest.Spec.Config.EiffelMessageBus.Username,
		"RABBITMQ_VHOST":              cfg.EnvironmentRequest.Spec.Config.EiffelMessageBus.Vhost,
		"RABBITMQ_SSL":                cfg.EnvironmentRequest.Spec.Config.EiffelMessageBus.SSL,
		"OTEL_EXPORTER_OTLP_ENDPOINT": os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		"OTEL_EXPORTER_OTLP_INSECURE": os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"),
	}
	return environment, nil
}

// createExecutionSpaces creates the specified number of ExecutionSpaces for an EnvironmentRequest.
func (p *genericExecutionSpaceProvider) createExecutionSpaces(
	ctx context.Context, cfg provider.ProvisionConfig,
) error {
	logger := logging.FromContextOrDiscard(ctx)
	ctx, span := cfg.Tracer.Start(ctx, "CreateExecutionSpaces", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	environment, err := p.createEnvironment(ctx, cfg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create environment variables for ExecutionSpace")
		return err
	}

	ds := dataset{}
	if err := json.Unmarshal(cfg.EnvironmentRequest.Spec.Dataset.Raw, &ds); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal dataset")
		return err
	}
	if ds.Dev {
		environment["DEV"] = "true"
	}
	if ds.ETRBranch != "" {
		environment["ETR_BRANCH"] = ds.ETRBranch
	}
	if ds.ETRRepo != "" {
		environment["ETR_REPOSITORY"] = ds.ETRRepo
	}
	// Add traceparent, tracestate and baggage to the environment variables so that they can be
	// propagated to the test runner.
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(environment))

	for range cfg.MinimumAmount {
		id := uuid.NewString()
		testrunner := cfg.EnvironmentRequest.Spec.Providers.ExecutionSpace.TestRunnerImage
		logger.Info("Creating a generic ExecutionSpace",
			"id", id, "image", testrunner, "identifier", cfg.EnvironmentRequest.Spec.Identifier,
		)
		environment["ENVIRONMENT_ID"] = id
		environment["ENVIRONMENT_URL"] = fmt.Sprintf("%s/v1alpha/testrun/%s", cfg.EnvironmentRequest.Spec.Config.EtosApi, id)
		executionSpace, err := provider.CreateExecutionSpace(ctx, cfg.EnvironmentRequest, cfg.Namespace, "",
			v1alpha2.ExecutionSpaceSpec{
				ID:         id,
				TestRunner: testrunner,
				Instructions: v1alpha2.Instructions{
					Identifier:  cfg.EnvironmentRequest.Spec.Identifier,
					Image:       testrunner,
					Parameters:  map[string]string{},
					Environment: environment,
				},
			})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to create ExecutionSpace")
			return err
		}
		logger.Info(fmt.Sprintf("ExecutionSpace created with name '%s', launching ETOS test runner",
			executionSpace.Name))
		if err := p.start(ctx, cfg.EnvironmentRequest, executionSpace); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to start ETOS test runner")
			return err
		}
		logger.Info("Test runner has launched and is waiting for tests")
	}
	return nil
}

// start up a Kubernetes Job for the ETOS test runner.
func (p *genericExecutionSpaceProvider) start(
	ctx context.Context, environmentrequest *v1alpha1.EnvironmentRequest, executionSpace *v1alpha2.ExecutionSpace,
) error {
	cli, err := provider.KubernetesClient()
	if err != nil {
		return err
	}
	envs := []corev1.EnvVar{}
	for key, value := range executionSpace.Spec.Instructions.Environment {
		envs = append(envs, corev1.EnvVar{Name: key, Value: value})
	}
	if environmentrequest.Spec.Config.EncryptionKey.Value != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  "ETOS_ENCRYPTION_KEY",
			Value: environmentrequest.Spec.Config.EncryptionKey.Value,
		})
	} else if environmentrequest.Spec.Config.EncryptionKey.ValueFrom.SecretKeyRef != nil {
		envs = append(envs, corev1.EnvVar{
			Name: "ETOS_ENCRYPTION_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: environmentrequest.Spec.Config.EncryptionKey.ValueFrom.SecretKeyRef,
			},
		})
	} else if environmentrequest.Spec.Config.EncryptionKey.ValueFrom.ConfigMapKeyRef != nil {
		envs = append(envs, corev1.EnvVar{
			Name: "ETOS_ENCRYPTION_KEY",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: environmentrequest.Spec.Config.EncryptionKey.ValueFrom.ConfigMapKeyRef,
			},
		})
	}
	args := []string{}
	for key, value := range executionSpace.Spec.Instructions.Parameters {
		args = append(args, fmt.Sprintf("%s=%s", key, value))
	}
	var backoffLimit int32 = 0
	var parallel int32 = 1
	var completions int32 = 1

	labels := map[string]string{
		"etos.eiffel-community.github.io/provider":               environmentrequest.Spec.Providers.ExecutionSpace.ID,
		"etos.eiffel-community.github.io/environment-request":    environmentrequest.Spec.Name,
		"etos.eiffel-community.github.io/environment-request-id": environmentrequest.Spec.ID,
		"app.kubernetes.io/name":                                 "etr",
		"app.kubernetes.io/part-of":                              "etos",
	}
	if cluster := environmentrequest.Labels["etos.eiffel-community.github.io/cluster"]; cluster != "" {
		labels["etos.eiffel-community.github.io/cluster"] = cluster
	}
	if environmentrequest.Spec.Identifier != "" {
		labels["etos.eiffel-community.github.io/id"] = environmentrequest.Spec.Identifier
	}

	job := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("etr-%s", executionSpace.Spec.ID),
			Namespace: environmentrequest.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Completions:  &completions,
			Parallelism:  &parallel,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "etos-test-runner",
							Image: executionSpace.Spec.Instructions.Image,
							Args:  args,
							Env:   envs,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("400m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("200m"),
								},
							},
						},
					},
				},
			},
		},
	}
	if err := controllerutil.SetOwnerReference(executionSpace, &job, provider.Scheme); err != nil {
		return err
	}
	return cli.Create(ctx, &job)
}

// Release releases an ExecutionSpace.
func (p *genericExecutionSpaceProvider) Release(
	ctx context.Context, cfg provider.ReleaseConfig,
) error {
	ctx, span := cfg.Tracer.Start(ctx, "Release", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	logger := logging.FromContextOrDiscard(ctx)

	logger.Info(fmt.Sprintf("Releasing ExecutionSpace with name %s", cfg.Name), "Namespace", cfg.Namespace)
	executionSpace, err := provider.GetExecutionSpace(ctx, cfg.Name, cfg.Namespace)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get ExecutionSpace")
		return err
	}
	if cfg.NoDelete {
		span.SetStatus(codes.Ok, "no-delete flag set, not deleting ExecutionSpace")
		return nil
	}
	if err := provider.DeleteExecutionSpace(ctx, executionSpace); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete ExecutionSpace")
		return err
	}
	logger.Info(fmt.Sprintf("ExecutionSpace '%s' released", cfg.Name))
	span.SetStatus(codes.Ok, "successfully released ExecutionSpace")
	return nil
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
