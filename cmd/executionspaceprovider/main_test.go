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
	"testing"
	"time"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/pkg/provider"
	batchv1 "k8s.io/api/batch/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestDatasetEnvironment verifies nil, valid, and malformed dataset handling.
func TestDatasetEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		dataset *apiextensionsv1.JSON
		want    map[string]string
		wantErr bool
	}{
		{
			name: "nil dataset",
			want: map[string]string{},
		},
		{
			name: "populated dataset",
			dataset: &apiextensionsv1.JSON{
				Raw: []byte(`{"dev":true,"ETR_REPO":"https://example.test/etr.git","ETR_BRANCH":"main"}`),
			},
			want: map[string]string{
				"DEV":            "true",
				"ETR_REPOSITORY": "https://example.test/etr.git",
				"ETR_BRANCH":     "main",
			},
		},
		{
			name: "malformed dataset",
			dataset: &apiextensionsv1.JSON{
				Raw: []byte(`{`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := datasetEnvironment(tt.dataset)
			if (err != nil) != tt.wantErr {
				t.Fatalf("datasetEnvironment() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("datasetEnvironment() = %#v, want %#v", got, tt.want)
			}
			for key, want := range tt.want {
				if got[key] != want {
					t.Errorf("datasetEnvironment()[%q] = %q, want %q", key, got[key], want)
				}
			}
		})
	}
}

// TestWaitForTestRunnersStartsWaitersConcurrently verifies that all readiness waits start before
// any result is collected.
func TestWaitForTestRunnersStartsWaitersConcurrently(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan error, 2)
	results <- nil
	results <- nil
	done := make(chan error, 1)

	waiter := func() error {
		started <- struct{}{}
		<-release
		return <-results
	}
	go func() {
		done <- waitForTestRunners([]func() error{waiter, waiter})
	}()

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("did not start all test runner waits concurrently")
		}
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("waitForTestRunners() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waitForTestRunners() did not return")
	}
}

// TestStartMakesTestRunnerJobOwnedByExecutionSpace verifies that deleting an ExecutionSpace
// garbage-collects its Test Runner Job.
func TestStartMakesTestRunnerJobOwnedByExecutionSpace(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(provider.Scheme).Build()
	provider.SetKubernetesClient(client)
	t.Cleanup(func() { provider.SetKubernetesClient(nil) })

	executionSpace := &v1alpha2.ExecutionSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "execution-space",
			Namespace: "test-namespace",
			UID:       types.UID("execution-space-uid"),
		},
		Spec: v1alpha2.ExecutionSpaceSpec{
			ID: "etr-instance",
			Instructions: v1alpha2.Instructions{
				Image:       "example.test/etr:1.0.0",
				Environment: map[string]string{},
				Parameters:  map[string]string{},
			},
		},
	}
	environmentRequest := &v1alpha1.EnvironmentRequest{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace"},
		Spec: v1alpha1.EnvironmentRequestSpec{
			Providers: v1alpha1.EnvironmentProviders{
				ExecutionSpace: v1alpha1.ExecutionSpaceProvider{ID: "execution-space-provider"},
			},
		},
	}

	providerRunner := &genericExecutionSpaceProvider{}
	if err := providerRunner.start(context.Background(), environmentRequest, executionSpace); err != nil {
		t.Fatalf("start() error = %v", err)
	}

	job := &batchv1.Job{}
	jobName := types.NamespacedName{Name: "etr-etr-instance", Namespace: "test-namespace"}
	if err := client.Get(context.Background(), jobName, job); err != nil {
		t.Fatalf("getting Test Runner Job: %v", err)
	}
	if len(job.OwnerReferences) != 1 {
		t.Fatalf("Test Runner Job owner references = %#v, want exactly one ExecutionSpace owner", job.OwnerReferences)
	}
	owner := job.OwnerReferences[0]
	if owner.UID != executionSpace.UID || owner.Kind != "ExecutionSpace" {
		t.Fatalf("Test Runner Job owner = %#v, want owner reference for ExecutionSpace %q", owner, executionSpace.UID)
	}
}
