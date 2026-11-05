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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/eiffel-community/eiffelevents-sdk-go"
	"github.com/eiffel-community/eiffelevents-sdk-go/routingkey"
	amqp "github.com/rabbitmq/amqp091-go"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/eiffel-community/etos/api/v1alpha1"
)

// EiffelConfirmationTimeout is the maximum time to wait for RabbitMQ to confirm a published Eiffel event.
const EiffelConfirmationTimeout = 10 * time.Second

// EiffelPublisher publishes Eiffel events to a RabbitMQ exchange.
type EiffelPublisher interface {
	// Publish sends an Eiffel event using a routing key that contains the routing key tag.
	Publish(ctx context.Context, routingKeyTag string, event eiffelevents.MetaTeller) error
	Close() error
}

// rabbitMQEiffelPublisher publishes Eiffel events to a RabbitMQ exchange and waits for broker confirmation.
type rabbitMQEiffelPublisher struct {
	lock     sync.Mutex
	uri      string
	config   amqp.Config
	exchange string
	conn     *amqp.Connection
	channel  *amqp.Channel
	closed   bool
}

// NewRabbitMQEiffelPublisher creates a publisher connected to the RabbitMQ described by config.
func NewRabbitMQEiffelPublisher(
	ctx context.Context,
	name string,
	config v1alpha1.RabbitMQ,
	cli client.Client,
	namespace string,
) (EiffelPublisher, error) {
	password, err := config.Password.Get(ctx, cli, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get RabbitMQ password: %w", err)
	}
	uri, err := amqpURI(config, string(password))
	if err != nil {
		return nil, err
	}
	amqpConfig := amqp.Config{Properties: amqp.NewConnectionProperties()}
	amqpConfig.Properties.SetClientConnectionName(name)
	if config.SSL == "true" {
		amqpConfig.TLSClientConfig = &tls.Config{ServerName: config.Host}
	}
	publisher := &rabbitMQEiffelPublisher{
		uri:      uri,
		config:   amqpConfig,
		exchange: config.Exchange,
	}
	if err := publisher.ensureChannel(); err != nil {
		return nil, err
	}
	return publisher, nil
}

// amqpURI builds an AMQP URI from the RabbitMQ configuration.
func amqpURI(config v1alpha1.RabbitMQ, password string) (string, error) {
	port, err := strconv.Atoi(config.Port)
	if err != nil {
		return "", fmt.Errorf("invalid RabbitMQ port %q: %w", config.Port, err)
	}
	scheme := "amqp"
	if config.SSL == "true" {
		scheme = "amqps"
	}
	uri := amqp.URI{
		Scheme:   scheme,
		Host:     config.Host,
		Port:     port,
		Username: config.Username,
		Password: password,
		Vhost:    config.Vhost,
	}
	return uri.String(), nil
}

// ensureChannel opens a connection and a confirm-mode channel if they are not already open.
// The caller must hold the lock, except during construction.
func (p *rabbitMQEiffelPublisher) ensureChannel() error {
	if p.conn == nil || p.conn.IsClosed() {
		conn, err := amqp.DialConfig(p.uri, p.config)
		if err != nil {
			return fmt.Errorf("failed to connect to Eiffel RabbitMQ: %w", err)
		}
		p.conn = conn
		p.channel = nil
	}
	if p.channel == nil || p.channel.IsClosed() {
		channel, err := p.conn.Channel()
		if err != nil {
			return fmt.Errorf("failed to open Eiffel RabbitMQ channel: %w", err)
		}
		if err := channel.Confirm(false); err != nil {
			_ = channel.Close()
			return fmt.Errorf("failed to enable publisher confirms on Eiffel RabbitMQ channel: %w", err)
		}
		p.channel = channel
	}
	return nil
}

// Publish sends an Eiffel event and waits until RabbitMQ has confirmed it.
func (p *rabbitMQEiffelPublisher) Publish(
	ctx context.Context, routingKeyTag string, event eiffelevents.MetaTeller,
) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal Eiffel event %s: %w", event.ID(), err)
	}
	ctx, cancel := context.WithTimeout(ctx, EiffelConfirmationTimeout)
	defer cancel()

	p.lock.Lock()
	defer p.lock.Unlock()
	if p.closed {
		return &PublisherClosedError{}
	}
	if err := p.ensureChannel(); err != nil {
		return err
	}
	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		p.exchange,
		routingkey.Sepia(event, "", routingKeyTag),
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.ID(),
			Timestamp:    time.UnixMilli(event.Time()),
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish Eiffel event %s: %w", event.ID(), err)
	}
	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to confirm Eiffel event %s: %w", event.ID(), err)
	}
	if !acked {
		return fmt.Errorf("eiffel event %s was rejected by RabbitMQ", event.ID())
	}
	return nil
}

// Close closes the RabbitMQ channel and connection. Publishing after Close returns an error.
func (p *rabbitMQEiffelPublisher) Close() error {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.closed = true
	if p.conn == nil || p.conn.IsClosed() {
		return nil
	}
	return p.conn.Close()
}
