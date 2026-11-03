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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/go-logr/logr"
	"github.com/gohugoio/hashstructure"
	"go.opentelemetry.io/otel/baggage"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventPublisher interface {
	Publish(string, events.Event) error
	AddLogger(logr.Logger)
	Close() error
}

type PublisherPool struct {
	ctx        context.Context
	publishers map[string]*Publisher
	lock       sync.Mutex
}

// namespacedParameters is a struct that combines the RabbitMQ parameters with the
// namespace to create a unique identifier for the connection.
type namespacedParameters struct {
	v1alpha1.RabbitMQ
	Namespace string
	Cluster   string
}

// NewPublisherPool creates a new PublisherPool instance.
func NewPublisherPool(ctx context.Context) *PublisherPool {
	return &PublisherPool{ctx, make(map[string]*Publisher), sync.Mutex{}}
}

// GetPublisher retrieves a publisher from the pool based on the provided parameters.
// If a publisher with the same parameters does not exist, a new one is created and added to the pool.
func (p *PublisherPool) GetPublisher(
	namespace string, cli client.Client, parameters v1alpha1.RabbitMQ, clusterName ...string,
) (*Publisher, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	// Hash the connection parameters and namespace to create a unique name for the connection
	identity := unassignedCluster
	if len(clusterName) == 1 && namespace != "" && clusterName[0] != "" &&
		!strings.Contains(namespace, "/") && !strings.Contains(clusterName[0], "/") {
		identity = namespace + "/" + clusterName[0]
	}
	namespacedParams := namespacedParameters{parameters, namespace, identity}
	hash, err := hashstructure.Hash(namespacedParams, nil)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%x", hash)
	publisher, exists := p.publishers[name]
	if !exists {
		if err := p.addPublisher(name, namespace, cli, parameters, identity); err != nil {
			return nil, err
		}
		publisher = p.publishers[name]
	}
	return publisher, nil
}

// addPublisher adds a new connection to the pool based on the provided parameters.
func (p *PublisherPool) addPublisher(name, namespace string, cli client.Client, parameters v1alpha1.RabbitMQ, identity string) error {
	if _, exists := p.publishers[name]; exists {
		return nil
	}
	member, err := baggage.NewMemberRaw("etos_cluster", identity)
	if err != nil {
		return err
	}
	ownership, err := baggage.FromContext(p.ctx).SetMember(member)
	if err != nil {
		return err
	}
	ctx := baggage.ContextWithBaggage(p.ctx, ownership)
	publisher, err := publisherImpl(ctx, "controller", parameters, cli, namespace)
	if err != nil {
		return err
	}
	p.publishers[name] = &Publisher{publisher}
	return nil
}

// Close closes all connections in the pool and returns any errors encountered.
func (p *PublisherPool) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	p.lock.Lock()
	defer p.lock.Unlock()
	var err error
	for _, connection := range p.publishers {
		if closeErr := connection.CloseContext(ctx); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}
	return err
}

type Publisher struct {
	publisher EventPublisher
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

// CloseContext shares a total drain budget across native publishers in the pool.
// External publisher implementations retain the existing Close contract.
func (p Publisher) CloseContext(ctx context.Context) error {
	if publisher, ok := p.publisher.(interface{ CloseContext(context.Context) error }); ok {
		return publisher.CloseContext(ctx)
	}
	return p.publisher.Close()
}

var publisherImpl = NewRabbitMQStreamPublisher

// SetPublisherImpl allows overriding the default implementation of the publisher, which is useful for testing.
func SetPublisherImpl(impl func(
	ctx context.Context,
	identifier string,
	parameters v1alpha1.RabbitMQ,
	cli client.Client,
	namespace string,
) (EventPublisher, error)) {
	publisherImpl = impl
}
