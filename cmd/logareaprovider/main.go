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
	"errors"

	"github.com/eiffel-community/etos/api/v1alpha2"
	"github.com/eiffel-community/etos/pkg/provider"
	"github.com/go-logr/logr"
)

type genericLogAreaProvider struct{}

// main creates a new LogArea resource based on data in an EnvironmentRequest.
func main() {
	provider.RunLogAreaProvider(&genericLogAreaProvider{})
}

// Provision provisions a new LogArea.
func (p *genericLogAreaProvider) Provision(
	ctx context.Context, logger logr.Logger, cfg provider.ProvisionConfig,
) error {
	environmentRequest := cfg.EnvironmentRequest
	if cfg.MinimumAmount <= 0 {
		return errors.New("minimum amount of LogAreas requested is less than or equal to 0")
	}
	logger.Info("Provisioning a new LogArea for EnvironmentRequest",
		"EnvironmentRequest", environmentRequest.Name,
		"Namespace", environmentRequest.Namespace,
		"Amount", cfg.MinimumAmount,
	)
	for range cfg.MinimumAmount {
		logger.Info("Creating a generic LogArea")
		if _, err := provider.CreateLogArea(ctx, environmentRequest, cfg.Namespace, v1alpha2.LogAreaSpec{
			// TODO: These parameters are obviously fake. They work for local testing, but not in a real-world scenario
			LiveLogs: "http://fake",
			Logs:     map[string]string{},
			Upload: v1alpha2.Upload{
				AsJSON: false,
				Method: "GET",
				URL:    "http://cluster-sample-etos-api/api/v1alpha/ping",
			},
		}); err != nil {
			return err
		}
		logger.Info("LogArea created")
	}
	return nil
}

// Release releases a LogArea.
func (p *genericLogAreaProvider) Release(ctx context.Context, logger logr.Logger, cfg provider.ReleaseConfig) error {
	logger.Info("Releasing LogArea", "Name", cfg.Name, "Namespace", cfg.Namespace)
	logArea, err := provider.GetLogArea(ctx, cfg.Name, cfg.Namespace)
	if err != nil {
		return err
	}
	logger.Info("LogArea", "name", logArea.Name)
	if cfg.NoDelete {
		return nil
	} else {
		return provider.DeleteLogArea(ctx, logArea)
	}
}
