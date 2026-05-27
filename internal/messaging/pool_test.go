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
package messaging_test

import (
	"context"
	"testing"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/messaging"
	"github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestPoolReuseSameStruct tests that the PublisherPool reuses the same publisher when the
// same struct is provided.
func TestPoolReuseSameStruct(t *testing.T) {

	messaging.SetPublisherImpl(func(
		ctx context.Context,
		identifier string,
		parameters v1alpha1.RabbitMQ,
		cli client.Client,
		namespace string,
	) (messaging.EventPublisher, error) {
		return &mockPublisher{}, nil
	})
	pool := messaging.NewPublisherPool(context.Background())
	cli := fake.NewClientBuilder().Build()

	parameters := v1alpha1.RabbitMQ{
		Host: "localhost",
		Port: "5672",
		Password: &v1alpha1.Var{
			Value: "password",
		},
	}
	publisher, err := pool.GetPublisher("test", cli, parameters)
	if err != nil {
		t.Errorf("Failed to create publisher: %v", err)
	}

	second, err := pool.GetPublisher("test", cli, parameters)
	if err != nil {
		t.Errorf("Failed to create second publisher: %v", err)
	}
	if publisher != second {
		t.Errorf("Expected to get the same publisher, but got different ones")
	}
}

// TestPoolReuseSameParameters tests that the PublisherPool reuses the same publisher when the
// same parameters are provided, even if they are different structs.
func TestPoolReuseSameParameters(t *testing.T) {

	messaging.SetPublisherImpl(func(
		ctx context.Context,
		identifier string,
		parameters v1alpha1.RabbitMQ,
		cli client.Client,
		namespace string,
	) (messaging.EventPublisher, error) {
		return &mockPublisher{}, nil
	})
	pool := messaging.NewPublisherPool(context.Background())
	cli := fake.NewClientBuilder().Build()

	parameters := v1alpha1.RabbitMQ{
		Host: "localhost",
		Port: "5672",
		Password: &v1alpha1.Var{
			Value: "password",
		},
	}
	publisher, err := pool.GetPublisher("test", cli, parameters)
	if err != nil {
		t.Errorf("Failed to create publisher: %v", err)
	}

	parameters = v1alpha1.RabbitMQ{
		Host: "localhost",
		Port: "5672",
		Password: &v1alpha1.Var{
			Value: "password",
		},
	}
	second, err := pool.GetPublisher("test", cli, parameters)
	if err != nil {
		t.Errorf("Failed to create second publisher: %v", err)
	}
	if publisher != second {
		t.Errorf("Expected to get the same publisher, but got different ones")
	}
}

// TestPoolUseDifferentParameters tests that the PublisherPool creates a new publisher when different
// parameters are provided.
func TestPoolUseDifferentParameters(t *testing.T) {

	messaging.SetPublisherImpl(func(
		ctx context.Context,
		identifier string,
		parameters v1alpha1.RabbitMQ,
		cli client.Client,
		namespace string,
	) (messaging.EventPublisher, error) {
		return &mockPublisher{}, nil
	})
	pool := messaging.NewPublisherPool(context.Background())
	cli := fake.NewClientBuilder().Build()

	parameters := v1alpha1.RabbitMQ{
		Host: "localhost",
		Port: "5672",
		Password: &v1alpha1.Var{
			Value: "password",
		},
	}
	publisher, err := pool.GetPublisher("test", cli, parameters)
	if err != nil {
		t.Errorf("Failed to create publisher: %v", err)
	}

	parameters = v1alpha1.RabbitMQ{
		Host: "localhost",
		Port: "5672",
		Password: &v1alpha1.Var{
			Value: "differentpassword",
		},
	}
	second, err := pool.GetPublisher("test", cli, parameters)
	if err != nil {
		t.Errorf("Failed to create second publisher: %v", err)
	}
	if publisher == second {
		t.Errorf("Expected to get different publishers, but got the same one")
	}
}

// TestPoolUseDifferentNamespace tests that the PublisherPool creates a new publisher when different
// namespaces are provided, even if the parameters are the same.
// This test is the same as TestPoolUseDifferentParameters but since namespace is handled slightly
// differently, it is good to have a separate test for it.
func TestPoolUseDifferentNamespace(t *testing.T) {

	messaging.SetPublisherImpl(func(
		ctx context.Context,
		identifier string,
		parameters v1alpha1.RabbitMQ,
		cli client.Client,
		namespace string,
	) (messaging.EventPublisher, error) {
		return &mockPublisher{}, nil
	})
	pool := messaging.NewPublisherPool(context.Background())
	cli := fake.NewClientBuilder().Build()

	parameters := v1alpha1.RabbitMQ{
		Host: "localhost",
		Port: "5672",
		Password: &v1alpha1.Var{
			Value: "password",
		},
	}
	publisher, err := pool.GetPublisher("test", cli, parameters)
	if err != nil {
		t.Errorf("Failed to create publisher: %v", err)
	}

	second, err := pool.GetPublisher("differentnamespace", cli, parameters)
	if err != nil {
		t.Errorf("Failed to create second publisher: %v", err)
	}
	if publisher == second {
		t.Errorf("Expected to get different publishers, but got the same one")
	}
}

type mockPublisher struct{}

func (m *mockPublisher) Publish(string, events.Event) error {
	return nil
}

func (m *mockPublisher) AddLogger(logr.Logger) {}

func (m *mockPublisher) Close() error {
	return nil
}
