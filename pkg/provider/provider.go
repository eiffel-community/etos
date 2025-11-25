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
	"encoding/json"
	"flag"
	"os"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/internal/controller/jobs"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	cli            client.Client
	Scheme         = runtime.NewScheme()
	terminationLog = "/dev/termination-log"
)

type Parameters struct {
	environmentRequestName string
	namespace              string
	name                   string
	providerName           string
	releaseEnvironment     bool
	noDelete               bool
	logger                 logr.Logger
}

type ProvisionConfig struct {
	MinimumAmount      int
	MaximumAmount      int
	Namespace          string
	EnvironmentRequest *v1alpha1.EnvironmentRequest
}

type ReleaseConfig struct {
	Name      string
	Namespace string
	NoDelete  bool
}

// Provider is an interface for providers to implement for the Run* functions.
type Provider interface {
	Provision(ctx context.Context, logger logr.Logger, cfg ProvisionConfig) error
	Release(ctx context.Context, logger logr.Logger, cfg ReleaseConfig) error
}

// init sets up the ETOS controller schemes as well as the default schemes from client-go.
func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(Scheme))

	utilruntime.Must(v1alpha1.AddToScheme(Scheme))
	utilruntime.Must(v1alpha2.AddToScheme(Scheme))
}

// ParseParameters parses the input parameters for a provider.
func ParseParameters() Parameters {
	opts := zap.Options{
		Development: true,
	}
	params := Parameters{}
	flag.BoolVar(&params.releaseEnvironment, "release", false, "Release instead of creating")
	flag.BoolVar(&params.noDelete, "nodelete", false, "Don't delete the resource")
	flag.StringVar(&params.environmentRequestName, "environment-request", "", "The environment request to provision for.")
	flag.StringVar(&params.name, "name", "", "The name of the resource to release.")
	flag.StringVar(&params.providerName, "provider", "", "The provider used to release.")
	flag.StringVar(&params.namespace, "namespace", "", "The namespace of the environment request.")
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	params.logger = zap.New(zap.UseFlagOptions(&opts))
	return params
}

// WriteResult writes a job result JSON structure to the termination-log if running in a Kubernetes pod.
func WriteResult(logger logr.Logger, result jobs.Result) error {
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		logger.Info("Provider is not running in a Kubernetes pod, won't write termination-log")
		return nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return os.WriteFile(terminationLog, b, os.ModePerm)
}

// KubernetesClient creates a new Kubernetes client or reuses an already created.
func KubernetesClient() (client.Client, error) {
	var err error
	if cli == nil {
		cli, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: Scheme})
		if err != nil {
			return nil, err
		}
	}
	return cli, err
}
