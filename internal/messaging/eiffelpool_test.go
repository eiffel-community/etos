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

	"github.com/eiffel-community/eiffelevents-sdk-go"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/messaging"
)

// TestEiffelPoolSharesPublisherPerRabbitMQAndNamespace verifies that clusters with the same
// RabbitMQ configuration in a namespace share a publisher, while other namespaces and
// RabbitMQ configurations get their own.
func TestEiffelPoolSharesPublisherPerRabbitMQAndNamespace(t *testing.T) {
	messaging.SetEiffelPublisherImpl(func(
		context.Context, string, v1alpha1.RabbitMQ, client.Client, string,
	) (messaging.EiffelPublisher, error) {
		return &mockEiffelPublisher{}, nil
	})
	pool := messaging.NewEiffelPublisherPool(context.Background())
	cli := fake.NewClientBuilder().Build()
	parameters := v1alpha1.RabbitMQ{Host: "rabbitmq-a", Port: "5672"}

	first, err := pool.GetPublisher("test", cli, parameters)
	if err != nil {
		t.Fatalf("Failed to create publisher: %v", err)
	}
	same, err := pool.GetPublisher("test", cli, v1alpha1.RabbitMQ{Host: "rabbitmq-a", Port: "5672"})
	if err != nil {
		t.Fatalf("Failed to get publisher: %v", err)
	}
	otherNamespace, err := pool.GetPublisher("other", cli, parameters)
	if err != nil {
		t.Fatalf("Failed to create publisher: %v", err)
	}
	otherRabbitMQ, err := pool.GetPublisher("test", cli, v1alpha1.RabbitMQ{Host: "rabbitmq-b", Port: "5672"})
	if err != nil {
		t.Fatalf("Failed to create publisher: %v", err)
	}

	if first != same {
		t.Error("Expected the same publisher for identical RabbitMQ parameters and namespace")
	}
	if first == otherNamespace {
		t.Error("Expected a different publisher for a different namespace")
	}
	if first == otherRabbitMQ {
		t.Error("Expected a different publisher for different RabbitMQ parameters")
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("Failed to close pool: %v", err)
	}
	for _, publisher := range []messaging.EiffelPublisher{first, otherNamespace, otherRabbitMQ} {
		if !publisher.(*mockEiffelPublisher).closed {
			t.Error("Expected pool to close all publishers")
		}
	}
}

type mockEiffelPublisher struct {
	closed bool
}

// Publish discards the event.
func (m *mockEiffelPublisher) Publish(context.Context, string, eiffelevents.MetaTeller) error {
	return nil
}

// Close marks the publisher as closed.
func (m *mockEiffelPublisher) Close() error {
	m.closed = true
	return nil
}
