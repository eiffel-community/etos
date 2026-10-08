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

package etos

import (
	"context"
	"net/url"
	"strings"
	"testing"

	etosv1alpha1 "github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestReconcileLogAreaProviderURLs verifies creation and reconciliation advertise
// the live logs and upload routes served by ETOS.
func TestReconcileLogAreaProviderURLs(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := etosv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &etosv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-sample", Namespace: "etos-test", UID: "cluster-uid"},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	deployment := NewETOSDeployment(etosv1alpha1.ETOS{}, scheme, cli, "", "", config.Config{
		LogAreaProvider: config.Service{Image: "example.com/logareaprovider", Version: "v0.0.1"},
	})
	name := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	providerName := types.NamespacedName{Name: cluster.Name + "-log-area-provider", Namespace: cluster.Namespace}
	wantLiveLogs := "http://cluster-sample-etos-sse/sse/v2alpha/events/$testrunid"
	wantUploadURL := "http://cluster-sample-etos-logarea/logarea/upload?path={context}/{folder}/{name}"

	for _, stage := range []string{"create", "update"} {
		t.Run(stage, func(t *testing.T) {
			if stage == "update" {
				existing := &etosv1alpha1.Provider{}
				if err := cli.Get(ctx, providerName, existing); err != nil {
					t.Fatal(err)
				}
				existing.Spec.LogAreaProviderConfig.Upload.URL = "http://cluster-sample-etos-logarea/logarea/v1alpha/upload"
				if err := cli.Update(ctx, existing); err != nil {
					t.Fatal(err)
				}
			}
			target, err := deployment.reconcileLogAreaProvider(ctx, name, cluster)
			if err != nil {
				t.Fatal(err)
			}
			if got := target.Spec.LogAreaProviderConfig.LiveLogs; got != wantLiveLogs {
				t.Errorf("reconciled live logs URL = %q, want %q", got, wantLiveLogs)
			}
			if got := target.Spec.LogAreaProviderConfig.Upload.URL; got != wantUploadURL {
				t.Errorf("reconciled upload URL = %q, want %q", got, wantUploadURL)
			}
			provider := &etosv1alpha1.Provider{}
			if err := cli.Get(ctx, providerName, provider); err != nil {
				t.Fatal(err)
			}
			if got := provider.Spec.LogAreaProviderConfig.LiveLogs; got != wantLiveLogs {
				t.Errorf("live logs URL = %q, want %q", got, wantLiveLogs)
			}
			if got := provider.Spec.LogAreaProviderConfig.Upload.URL; got != wantUploadURL {
				t.Errorf("upload URL = %q, want %q", got, wantUploadURL)
			}
			if got := provider.Spec.LogAreaProviderConfig.Upload.Method; got != "POST" {
				t.Errorf("upload method = %q, want POST", got)
			}
			formatted := strings.NewReplacer(
				"{context}", "run-1",
				"{folder}", "suite-1/subsuite-1",
				"{name}", "report.json",
			).Replace(provider.Spec.LogAreaProviderConfig.Upload.URL)
			uploadURL, err := url.Parse(formatted)
			if err != nil {
				t.Fatal(err)
			}
			if got := uploadURL.Path; got != "/logarea/upload" {
				t.Errorf("upload path = %q, want /logarea/upload", got)
			}
			if got := uploadURL.Query().Get("path"); got != "run-1/suite-1/subsuite-1/report.json" {
				t.Errorf("file path = %q, want run-1/suite-1/subsuite-1/report.json", got)
			}
		})
	}
}
