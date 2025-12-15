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
	"flag"
	"fmt"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/internal/controller/jobs"
	providerHelper "github.com/eiffel-community/etos/pkg/provider"
	"github.com/eiffel-community/etos/pkg/splitter"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type environmentProvider struct {
	environmentRequestName string
	namespace              string
	name                   string
	releaseEnvironment     bool
}

// main creates a new environment resource based on data in an EnvironmentRequest.
func main() {
	opts := zap.Options{
		Development: true,
	}
	provider := environmentProvider{}
	flag.BoolVar(&provider.releaseEnvironment, "release", false, "Release instead of creating")
	flag.StringVar(&provider.environmentRequestName,
		"environment-request", "", "The environment request to provision for.",
	)
	flag.StringVar(&provider.name, "name", "", "The name of the resource to release.")
	flag.StringVar(&provider.namespace, "namespace", "", "The namespace of the environment request.")
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	logger := zap.New(zap.UseFlagOptions(&opts))

	if provider.releaseEnvironment {
		if err := runReleaser(provider); err != nil {
			if writeErr := providerHelper.WriteResult(logger,
				jobs.Result{
					Conclusion:  jobs.ConclusionFailed,
					Description: err.Error(),
					Verdict:     jobs.VerdictNone,
				}); writeErr != nil {
				logger.Error(writeErr, "failed to write error result to termination-log")
			}
			panic(err)
		}
		if err := providerHelper.WriteResult(logger,
			jobs.Result{
				Conclusion:  jobs.ConclusionSuccessful,
				Description: "Successfully released Environment",
				Verdict:     jobs.VerdictNone,
			}); err != nil {
			logger.Error(err, "failed to write error result to termination-log")
			panic(err)
		}
	} else {
		if err := runProvider(provider); err != nil {
			if writeErr := providerHelper.WriteResult(logger,
				jobs.Result{
					Conclusion:  jobs.ConclusionFailed,
					Description: err.Error(),
					Verdict:     jobs.VerdictNone,
				}); writeErr != nil {
				logger.Error(writeErr, "failed to write error result to termination-log")
			}
			panic(err)
		}
		if err := providerHelper.WriteResult(logger,
			jobs.Result{
				Conclusion:  jobs.ConclusionSuccessful,
				Description: "Successfully provisioned Environments",
				Verdict:     jobs.VerdictNone,
			}); err != nil {
			logger.Error(err, "failed to write error result to termination-log")
			panic(err)
		}
	}
}

func runReleaser(_ environmentProvider) error {
	// Currently does nothing
	return nil
}

// runProvider is the base provider for the EnvironmentProvider.
func runProvider(provider environmentProvider) error {
	ctx := context.Background()
	if provider.environmentRequestName == "" {
		return errors.New("Must set -environment-request")
	}
	if provider.namespace == "" {
		return errors.New("Must set -namespace")
	}

	cli, err := providerHelper.KubernetesClient()
	if err != nil {
		return err
	}
	var request v1alpha1.EnvironmentRequest
	if err := cli.Get(
		ctx, types.NamespacedName{Name: provider.environmentRequestName, Namespace: provider.namespace}, &request,
	); err != nil {
		return err
	}
	iuts, err := providerHelper.GetIUTs(ctx, request.Spec.ID, provider.namespace)
	if err != nil {
		return err
	}
	logAreas, err := providerHelper.GetLogAreas(ctx, request.Spec.ID, provider.namespace)
	if err != nil {
		return err
	}
	executionSpaces, err := providerHelper.GetExecutionSpaces(ctx, request.Spec.ID, provider.namespace)
	if err != nil {
		return err
	}

	// TODO: Choose strategy
	splitter := splitter.NewRoundRobinSplitter().SetSize(len(iuts.Items))
	for _, test := range request.Spec.Splitter.Tests {
		splitter.AddTest(test)
	}
	tests := splitter.Split()

	created := 0
	for i, iut := range iuts.Items {
		logArea := logAreas.Items[i]
		executionSpace := executionSpaces.Items[i]
		if err := CreateEnvironment(
			ctx, i, tests[i], &request, provider.namespace, iut, executionSpace, logArea,
		); err != nil {
			return err
		}
		created++
	}
	minimumAmount := min(request.Spec.MaximumAmount, request.Spec.MinimumAmount)
	if created < minimumAmount {
		return fmt.Errorf("not enough environments created, expected %d created %d", request.Spec.MinimumAmount, created)
	}
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
	cli, err := providerHelper.KubernetesClient()
	if err != nil {
		return err
	}
	labels := map[string]string{
		"etos.eiffel-community.github.io/environment-request":    environmentrequest.Name,
		"etos.eiffel-community.github.io/environment-request-id": environmentrequest.Spec.ID,
		"etos.eiffel-community.github.io/suite-id":               environmentrequest.Spec.ID,
		"app.kubernetes.io/name":                                 "environment-provider",
		"app.kubernetes.io/part-of":                              "etos",
	}
	if cluster := environmentrequest.Labels["etos.eiffel-community.github.io/cluster"]; cluster != "" {
		labels["etos.eiffel-community.github.io/cluster"] = cluster
	}
	if environmentrequest.Spec.Identifier != "" {
		labels["etos.eiffel-community.github.io/id"] = environmentrequest.Spec.Identifier
	}

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
