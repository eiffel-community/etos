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
	"sync"

	"github.com/gohugoio/hashstructure"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/eiffel-community/etos/api/v1alpha1"
)

// EiffelPublisherPool shares Eiffel publishers between clusters that use the same RabbitMQ
// configuration in the same namespace.
type EiffelPublisherPool struct {
	ctx        context.Context
	publishers map[string]EiffelPublisher
	lock       sync.Mutex
}

// NewEiffelPublisherPool creates an empty EiffelPublisherPool.
func NewEiffelPublisherPool(ctx context.Context) *EiffelPublisherPool {
	return &EiffelPublisherPool{ctx: ctx, publishers: make(map[string]EiffelPublisher)}
}

// GetPublisher returns the publisher for the RabbitMQ parameters and namespace, creating it if needed.
func (p *EiffelPublisherPool) GetPublisher(
	namespace string, cli client.Client, parameters v1alpha1.RabbitMQ,
) (EiffelPublisher, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	hash, err := hashstructure.Hash(namespacedParameters{parameters, namespace}, nil)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%x", hash)
	if publisher, exists := p.publishers[name]; exists {
		return publisher, nil
	}
	publisher, err := eiffelPublisherImpl(p.ctx, "controller", parameters, cli, namespace)
	if err != nil {
		return nil, err
	}
	p.publishers[name] = publisher
	return publisher, nil
}

// Close closes all publishers in the pool and returns any errors encountered.
func (p *EiffelPublisherPool) Close() error {
	p.lock.Lock()
	defer p.lock.Unlock()
	var err error
	for _, publisher := range p.publishers {
		err = errors.Join(err, publisher.Close())
	}
	return err
}

var eiffelPublisherImpl = NewRabbitMQEiffelPublisher

// SetEiffelPublisherImpl overrides the Eiffel publisher implementation, which is useful for testing.
func SetEiffelPublisherImpl(impl func(
	ctx context.Context,
	name string,
	parameters v1alpha1.RabbitMQ,
	cli client.Client,
	namespace string,
) (EiffelPublisher, error)) {
	eiffelPublisherImpl = impl
}
