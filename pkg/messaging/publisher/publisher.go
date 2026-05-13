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
package publisher

import (
	"context"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/internal/messaging"
	"github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventPublisher interface {
	Publish(string, events.Event) error
	AddLogger(logr.Logger)
	Close() error
}

type Publisher struct {
	connection EventPublisher
}

// NewPublisher creates a new Publisher instance.
func NewPublisher(
	ctx context.Context, config v1alpha1.RabbitMQ, cli client.Client, namespace string,
) (EventPublisher, error) {
	connection, err := messaging.NewRabbitMQStreamPublisher(ctx, "provider", config, cli, namespace)
	if err != nil {
		return nil, err
	}
	return Publisher{
		connection: connection,
	}, nil
}

// Publish sends an event to the EventPublisher.
func (p Publisher) Publish(identifier string, event events.Event) error {
	return p.connection.Publish(identifier, event)
}

// AddLogger adds a logger to the publisher's connection.
func (p Publisher) AddLogger(logger logr.Logger) {
	p.connection.AddLogger(logger)
}

// Close closes the publisher's connection.
func (p Publisher) Close() error {
	return p.connection.Close()
}
