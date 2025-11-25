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

type genericIutProvider struct{}

// main creates a new Iut resource based on data in an EnvironmentRequest.
func main() {
	provider.RunIutProvider(&genericIutProvider{})
}

// Provision provisions a new IUT.
func (p *genericIutProvider) Provision(ctx context.Context, logger logr.Logger, cfg provider.ProvisionConfig) error {
	environmentRequest := cfg.EnvironmentRequest
	if cfg.MinimumAmount <= 0 {
		return errors.New("minimum amount of IUTs requested is less than or equal to 0")
	}
	logger.Info(
		"Provisioning a new IUT for EnvironmentRequest",
		"EnvironmentRequest", environmentRequest.Name,
		"Namespace", environmentRequest.Namespace,
		"Amount", cfg.MinimumAmount,
	)
	for range cfg.MinimumAmount {
		logger.Info("Creating a generic IUT")
		if _, err := provider.CreateIUT(ctx, environmentRequest, cfg.Namespace, v1alpha2.IutSpec{}); err != nil {
			return err
		}
		logger.Info("IUT created")
	}
	return nil
}

// Release releases an IUT.
func (p *genericIutProvider) Release(ctx context.Context, logger logr.Logger, cfg provider.ReleaseConfig) error {
	logger.Info("Releasing IUT", "Name", cfg.Name, "Namespace", cfg.Namespace)
	iut, err := provider.GetIUT(ctx, cfg.Name, cfg.Namespace)
	if err != nil {
		return err
	}
	logger.Info("IUT", "name", iut.Name)
	if cfg.NoDelete {
		return nil
	} else {
		return provider.DeleteIUT(ctx, iut)
	}
}
