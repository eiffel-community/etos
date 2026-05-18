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
package messaging

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PublisherPool struct {
	ctx        context.Context
	publishers map[string]*Publisher
	lock       sync.Mutex
}

// NewPublisherPool creates a new PublisherPool instance.
func NewPublisherPool(ctx context.Context) *PublisherPool {
	return &PublisherPool{ctx, make(map[string]*Publisher), sync.Mutex{}}
}

// GetPublisher retrieves a publisher from the pool based on the provided parameters.
// If a publisher with the same parameters does not exist, a new one is created and added to the pool.
func (p *PublisherPool) GetPublisher(
	namespace string, cli client.Client, parameters v1alpha1.RabbitMQ,
) (*Publisher, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	// Hash the connection parameters and namespace to create a unique name for the connection
	var hashInput []byte
	hashInput = fmt.Appendf(hashInput, "%v", parameters)
	hashInput = fmt.Append(hashInput, namespace)
	name := fmt.Sprintf("%x", sha256.Sum256(hashInput))
	publisher, exists := p.publishers[name]
	if !exists {
		if err := p.addPublisher(name, namespace, cli, parameters); err != nil {
			return nil, err
		}
		publisher = p.publishers[name]
	}
	return publisher, nil
}

// addPublisher adds a new connection to the pool based on the provided parameters.
func (p *PublisherPool) addPublisher(name, namespace string, cli client.Client, parameters v1alpha1.RabbitMQ) error {
	if _, exists := p.publishers[name]; exists {
		return nil
	}
	publisher, err := NewRabbitMQStreamPublisher(p.ctx, "controller", parameters, cli, namespace)
	if err != nil {
		return err
	}
	p.publishers[name] = &Publisher{publisher}
	return nil
}

// Close closes all connections in the pool and returns any errors encountered.
func (p *PublisherPool) Close() error {
	p.lock.Lock()
	defer p.lock.Unlock()
	var err error
	for _, connection := range p.publishers {
		if closeErr := connection.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}
	return err
}

type Publisher struct {
	publisher *rabbitMQStreamPublisher
}

// Publish sends an event to the EventPublisher.
func (p Publisher) Publish(identifier string, event events.Event) error {
	return p.publisher.Publish(identifier, event)
}

// AddLogger adds a logger to the publisher's connection.
func (p Publisher) AddLogger(logger logr.Logger) {
	p.publisher.AddLogger(logger)
}

// Close closes the publisher's connection.
func (p Publisher) Close() error {
	return p.publisher.Close()
}
