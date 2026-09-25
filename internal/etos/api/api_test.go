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

package api

import (
	"context"
	"testing"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testNamespace = "default"

func providerSecret(name string, data map[string]string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Data:       map[string][]byte{},
	}
	for key, value := range data {
		secret.Data[key] = []byte(value)
	}
	return secret
}

func newTestDeployment(cli client.Client) *ETOSApiDeployment {
	spec := etosv1alpha1.ETOSAPI{
		IUTProviderSecret:            "iut-providers",
		LogAreaProviderSecret:        "log-area-providers",
		ExecutionSpaceProviderSecret: "execution-space-providers",
	}
	return NewETOSApiDeployment(spec, scheme.Scheme, cli, "", "", "", config.Config{})
}

// TestProvidersChecksumChangesWithSecretData tests that the checksum changes when the data
// of a provider secret changes, and is stable when it does not.
func TestProvidersChecksumChangesWithSecretData(t *testing.T) {
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithObjects(
		providerSecret("iut-providers", map[string]string{"iut.json": `{"iut": {}}`}),
		providerSecret("log-area-providers", map[string]string{"default.json": `{"log": {"password": "old"}}`}),
		providerSecret("execution-space-providers", map[string]string{"default.json": `{"execution_space": {}}`}),
	).Build()
	deployment := newTestDeployment(cli)

	first, err := deployment.providersChecksum(ctx, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first == "" {
		t.Fatal("expected a non-empty checksum")
	}
	second, err := deployment.providersChecksum(ctx, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Fatalf("expected checksum to be stable, got %q and %q", first, second)
	}

	secret := &corev1.Secret{}
	if err := cli.Get(ctx, types.NamespacedName{Name: "log-area-providers", Namespace: testNamespace}, secret); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	secret.Data["default.json"] = []byte(`{"log": {"password": "new"}}`)
	if err := cli.Update(ctx, secret); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	third, err := deployment.providersChecksum(ctx, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if third == first {
		t.Fatal("expected checksum to change when provider secret data changes")
	}
}

// TestProvidersChecksumMissingSecret tests that a missing provider secret does not fail the
// checksum calculation and that the checksum changes once the secret is created.
func TestProvidersChecksumMissingSecret(t *testing.T) {
	ctx := context.Background()
	cli := fake.NewClientBuilder().Build()
	deployment := newTestDeployment(cli)

	before, err := deployment.providersChecksum(ctx, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := cli.Create(ctx, providerSecret("log-area-providers", map[string]string{"default.json": "{}"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after, err := deployment.providersChecksum(ctx, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if before == after {
		t.Fatal("expected checksum to change when a provider secret is created")
	}
}

// TestProvidersChecksumNoProviders tests that no checksum annotation is set on the pod template
// when no provider secrets are configured.
func TestProvidersChecksumNoProviders(t *testing.T) {
	ctx := context.Background()
	cli := fake.NewClientBuilder().Build()
	deployment := NewETOSApiDeployment(etosv1alpha1.ETOSAPI{}, scheme.Scheme, cli, "", "", "", config.Config{})

	checksum, err := deployment.providersChecksum(ctx, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checksum != "" {
		t.Fatalf("expected empty checksum, got %q", checksum)
	}
	target := deployment.deployment(types.NamespacedName{Name: "api", Namespace: testNamespace}, "cfg", checksum, "cluster")
	if _, ok := target.Spec.Template.Annotations[ProvidersChecksumAnnotation]; ok {
		t.Fatal("expected no checksum annotation when no provider secrets are configured")
	}
}

// TestDeploymentHasProvidersChecksumAnnotation tests that the checksum is set as an annotation
// on the pod template, which is what triggers a rollout of the ETOS API.
func TestDeploymentHasProvidersChecksumAnnotation(t *testing.T) {
	deployment := newTestDeployment(fake.NewClientBuilder().Build())
	target := deployment.deployment(types.NamespacedName{Name: "api", Namespace: testNamespace}, "cfg", "abc123", "cluster")
	if got := target.Spec.Template.Annotations[ProvidersChecksumAnnotation]; got != "abc123" {
		t.Fatalf("expected annotation %q, got %q", "abc123", got)
	}
	if _, ok := target.Annotations[ProvidersChecksumAnnotation]; ok {
		t.Fatal("expected checksum annotation only on the pod template, not on the deployment")
	}
}
