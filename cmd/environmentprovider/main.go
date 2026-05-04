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
	"fmt"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/pkg/logging"
	"github.com/eiffel-community/etos/pkg/provider"
	"github.com/eiffel-community/etos/pkg/splitter"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type environmentProvider struct{}

// main creates a new Environment resource based on data in an EnvironmentRequest.
func main() {
	provider.RunEnvironmentProvider(&environmentProvider{})
}

// Provision provisions a new Environment.
func (p *environmentProvider) Provision(ctx context.Context, cfg provider.ProvisionConfig) error {
	ctx, span := cfg.Tracer.Start(ctx, "Provision", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	logger := logging.FromContextOrDiscard(ctx)
	environmentRequest := cfg.EnvironmentRequest

	logger.Info("Starting environment provisioning",
		"EnvironmentRequest", cfg.EnvironmentRequest.Name,
		"Namespace", cfg.EnvironmentRequest.Namespace,
	)

	iuts, err := provider.GetIUTs(ctx, environmentRequest.Spec.ID, cfg.Namespace)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get IUTs")
		return err
	}
	logAreas, err := provider.GetLogAreas(ctx, environmentRequest.Spec.ID, cfg.Namespace)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get LogAreas")
		return err
	}
	executionSpaces, err := provider.GetExecutionSpaces(ctx, environmentRequest.Spec.ID, cfg.Namespace)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get ExecutionSpaces")
		return err
	}

	minRequired := min(environmentRequest.Spec.MaximumAmount, environmentRequest.Spec.MinimumAmount)
	maxPossible := min(len(iuts.Items), len(logAreas.Items), len(executionSpaces.Items))
	if maxPossible < minRequired {
		err := fmt.Errorf(
			`not enough resources to create environments, expected at least %d environments, got at
			most %d environments with %d log areas and %d execution spaces`,
			minRequired, maxPossible, len(logAreas.Items), len(executionSpaces.Items),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "not enough resources to create environments")
		return err
	}

	// TODO: Choose strategy
	split := splitter.NewRoundRobinSplitter().SetSize(maxPossible)
	for _, test := range environmentRequest.Spec.Splitter.Tests {
		split.AddTest(test)
	}
	if err := p.createEnvironments(
		ctx,
		cfg,
		maxPossible,
		split.Split(),
		environmentRequest,
		iuts,
		logAreas,
		executionSpaces,
	); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create environments")
		return err
	}
	span.SetStatus(codes.Ok, "successfully provisioned environment(s)")
	return nil
}

// createEnvironments creates the Environment resources in Kubernetes.
func (p *environmentProvider) createEnvironments(
	ctx context.Context,
	cfg provider.ProvisionConfig,
	maxPossible int,
	tests [][]v1alpha1.Test,
	environmentRequest *v1alpha1.EnvironmentRequest,
	iuts v1alpha2.IutList,
	logAreas v1alpha2.LogAreaList,
	executionSpaces v1alpha2.ExecutionSpaceList,
) error {
	ctx, span := cfg.Tracer.Start(ctx, "CreateEnvironments", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	logger := logging.FromContextOrDiscard(ctx)

	for i := range maxPossible {
		logArea := logAreas.Items[i]
		executionSpace := executionSpaces.Items[i]
		iut := iuts.Items[i]
		if err := CreateEnvironment(
			ctx, i, tests[i], environmentRequest, cfg.Namespace, iut, executionSpace, logArea,
		); err != nil {
			// We have to fail environment provisioning if any environment fails to be created, since
			// we split the tests across all environments and if one environment fails to be created,
			// we can't guarantee that all tests will be executed.
			logger.Error(err, "failed to create environment", "index", i)
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to create environment")
			return err
		}
	}
	logger.Info("Successfully created environment(s)")
	span.SetStatus(codes.Ok, "successfully created environment(s)")
	return nil
}

// Release releases the Environment. Currently, this does nothing.
func (p *environmentProvider) Release(ctx context.Context, cfg provider.ReleaseConfig) error {
	_, span := cfg.Tracer.Start(ctx, "Release", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	// Currently does nothing
	span.SetStatus(codes.Ok, "Successfully released environment(s)")
	return nil
}

// fakeIut is used because we don't have a proper specification of an IUT.
//
// fakeIut allows us to Marshal v1alpha2.IutSpec.ProviderData into v1alpha2.IutSpec with a MarshalJSON hack.
type fakeIut struct {
	v1alpha2.IutSpec
}

// MarshalJSON merges v1alpha2.IutSpec.ProviderData into the v1alpha2.IutSpec before marshalling.
func (f fakeIut) MarshalJSON() ([]byte, error) {
	type tmp fakeIut
	g := tmp(f)
	if g.ProviderData == nil {
		return json.Marshal(g)
	}
	first, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	second := g.ProviderData.Raw
	data := make(map[string]any)
	if err = json.Unmarshal(first, &data); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(second, &data); err != nil {
		return nil, err
	}
	delete(data, "ProviderData")
	return json.Marshal(data)
}

// CreateEnvironment creates a new Environment resource in Kubernetes.
func CreateEnvironment(
	ctx context.Context,
	index int,
	tests []v1alpha1.Test,
	environmentrequest *v1alpha1.EnvironmentRequest,
	namespace string,
	iut v1alpha2.Iut,
	executionSpace v1alpha2.ExecutionSpace,
	logArea v1alpha2.LogArea,
) error {
	cli, err := provider.KubernetesClient()
	if err != nil {
		return err
	}
	labels := map[string]string{
		"etos.eiffel-community.github.io/environment-request":    environmentrequest.Name,
		"etos.eiffel-community.github.io/environment-request-id": environmentrequest.Spec.ID,
		"etos.eiffel-community.github.io/sub-suite-id":           executionSpace.Spec.ID,
		"app.kubernetes.io/name":                                 "environment-provider",
		"app.kubernetes.io/part-of":                              "etos",
	}
	if cluster := environmentrequest.Labels["etos.eiffel-community.github.io/cluster"]; cluster != "" {
		labels["etos.eiffel-community.github.io/cluster"] = cluster
	}
	if environmentrequest.Spec.Identifier != "" {
		labels["etos.eiffel-community.github.io/id"] = environmentrequest.Spec.Identifier
	}

	deadline := min(executionSpace.Spec.Deadline, logArea.Spec.Deadline, iut.Spec.Deadline)

	executorJson, err := json.Marshal(executionSpace.Spec)
	if err != nil {
		return err
	}
	logAreaJson, err := json.Marshal(logArea.Spec)
	if err != nil {
		return err
	}
	iutJson, err := json.Marshal(fakeIut{iut.Spec})
	if err != nil {
		return err
	}

	isController := false
	blockOwnerDeletion := true
	// TODO: No Eiffel events are being sent.
	return cli.Create(ctx, &v1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    labels,
			Name:      executionSpace.Spec.ID,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{{
				Kind:               "EnvironmentRequest",
				Name:               environmentrequest.GetName(),
				UID:                environmentrequest.GetUID(),
				APIVersion:         v1alpha1.GroupVersion.String(),
				Controller:         &isController,
				BlockOwnerDeletion: &blockOwnerDeletion,
			}},
		},
		Spec: v1alpha1.EnvironmentSpec{
			Name:     fmt.Sprintf("%s_SubSuite_%d", environmentrequest.Spec.Name, index),
			Priority: 1,
			Providers: &v1alpha1.Providers{
				IUT:            iut.Name,
				LogArea:        logArea.Name,
				ExecutionSpace: executionSpace.Name,
			},
			Deadline:    deadline,
			SuiteID:     environmentrequest.Spec.Identifier,
			MainSuiteID: environmentrequest.Spec.ID,
			Artifact:    environmentrequest.Spec.Artifact,
			SubSuiteID:  executionSpace.Spec.ID,
			TestRunner:  executionSpace.Spec.TestRunner,
			Context:     environmentrequest.Spec.ID,
			Executor:    &apiextensionsv1.JSON{Raw: executorJson},
			LogArea:     &apiextensionsv1.JSON{Raw: logAreaJson},
			Iut:         &apiextensionsv1.JSON{Raw: iutJson},
			Tests:       tests,
		},
	})
}
