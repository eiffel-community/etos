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
	"net/url"
	"sync"
	"time"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/go-logr/logr"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sethvargo/go-retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Publisher interface {
	Publish([]byte, string) error
	AddLogger(logr.Logger)
	Close() error
}

// NewPublisher creates a new Publisher instance.
func NewPublisher(ctx context.Context, config v1alpha1.RabbitMQ, client client.Client, namespace string) (Publisher, error) {
	return newRabbitMQPublisher(ctx, config, client, namespace)
}

type rabbitMQPublisher struct {
	url            string
	exchange       string
	logger         logr.Logger
	context        context.Context
	conn           *amqp.Connection
	channel        *amqp.Channel
	chanClosures   chan *amqp.Error
	connClosures   chan *amqp.Error
	confirmations  chan amqp.Confirmation
	hasOutstanding bool       // Is there an in-flight message that hasn't been (n)acked?
	connMu         sync.Mutex // Prevent overlapping connection setup/teardown
	publishMu      sync.Mutex // Prevent overlapping publishing
}

// newRabbitMQPublisher creates a new rabbitMQPublisher instance. It does not attempt to connect
// to RabbitMQ or set up a channel, but it does set up the connection and channel closure
// notification channels so that the publisher can react to any connection issues that might
// have caused them to be closed. The connection and channel will be lazily established upon
// the first call to Publish.
func newRabbitMQPublisher(ctx context.Context, config v1alpha1.RabbitMQ, client client.Client, namespace string) (*rabbitMQPublisher, error) {
	password, err := config.Password.Get(ctx, client, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get RabbitMQ password: %w", err)
	}
	_type := "amqp"
	if config.SSL == "true" {
		_type = "amqps"
	}
	// Implement RabbitMQ connection and channel setup here
	url := fmt.Sprintf("%s://%s:%s@%s:%s/%s",
		_type,
		config.Username,
		password,
		config.Host,
		config.Port,
		config.Vhost,
	)
	p := &rabbitMQPublisher{
		url:          url,
		context:      ctx,
		logger:       logr.Discard(), // Messages discarded until AddLogger is called.
		exchange:     config.Exchange,
		chanClosures: make(chan *amqp.Error),
		connClosures: make(chan *amqp.Error),
	}
	// Create these channels, but close them immediately to signal that no
	// connection or channel has been established.
	close(p.chanClosures)
	close(p.connClosures)
	return p, nil
}

// Close closes any current connection and any channel open within it.
// This will interrupt any ongoing publishing, but only temporarily as
// it'll retry. To permanently interrupt ongoing publishing and force
// a return to the caller, cancel the context passed to Publish.
func (p *rabbitMQPublisher) Close() error {
	p.connMu.Lock()
	if p.conn != nil {
		// Closing the connection also closes p.channel and notification channels.
		if err := p.conn.Close(); err != nil {
			return err
		}
	}
	p.connMu.Unlock()
	return nil
}

// AddLogger adds a logger to the publisher.
func (p *rabbitMQPublisher) AddLogger(logger logr.Logger) {
	logger = logger.WithValues("disableUserLog", true)
	p.logger = logger
}

// Publish attempts to publish a single message. It'll block until a connection
// is available, sends the message, and then waits for the message to be
// acknowledged by the broker. All kinds of errors except context expirations
// are retried indefinitely with a backoff.
//
// Connections are created lazily upon the first call to this method and will
// be kept alive. Any error will cause the connection to be torn down and
// reestablished upon the next attempt.
// func (p *rabbitMQPublisher) Publish(ctx context.Context, logger *logrus.Entry, topic string, message amqp.Publishing) error {
func (p *rabbitMQPublisher) Publish(b []byte, topic string) error {
	backoff := retry.WithCappedDuration(1*time.Minute, retry.NewExponential(1*time.Second))
	message := amqp.Publishing{Body: b}
	return retry.Do(p.context, backoff, func(ctx context.Context) error {
		if err := p.tryPublish(ctx, topic, message); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.logger.Error(err, "Could not publish message, will retry")
			return retry.RetryableError(err)
		}
		return nil
	})
}

// awaitConfirmation waits for and returns the confirmation (positive or
// negative) for a single message. An error is returned if the context
// expires or the connection or channel closes. Must only be called while
// the p.publishMu mutex is held.
func (p *rabbitMQPublisher) awaitConfirmation(ctx context.Context) (amqp.Confirmation, error) {
	select {
	case err, ok := <-p.connClosures:
		if ok {
			return amqp.Confirmation{}, fmt.Errorf("connection closed: %w", err)
		}
		return amqp.Confirmation{}, errors.New("connection not established")
	case err, ok := <-p.chanClosures:
		if ok {
			return amqp.Confirmation{}, fmt.Errorf("channel closed: %w", err)
		}
		return amqp.Confirmation{}, errors.New("channel not established")
	case c := <-p.confirmations:
		p.hasOutstanding = false
		return c, nil
	case <-ctx.Done():
		return amqp.Confirmation{}, context.Cause(ctx)
	}
}

func (p *rabbitMQPublisher) ensureConnection() error {
	p.connMu.Lock()
	defer p.connMu.Unlock()
	select {
	case connErr, ok := <-p.connClosures:
		if ok {
			p.logger.Info(fmt.Sprintf("Re-establishing failed connection: %s", connErr.Error()))
		}
		amqpURL, parseErr := url.Parse(p.url)
		if parseErr != nil {
			return fmt.Errorf("invalid AMQP URL: %w", parseErr)
		}
		var err error
		p.logger.Info(fmt.Sprintf("Opening AMQP connection to %s", amqpURL.Redacted()))
		if p.conn, err = amqp.Dial(amqpURL.String()); err != nil {
			return fmt.Errorf("error making AMQP connection: %w", err)
		}
		p.connClosures = p.conn.NotifyClose(make(chan *amqp.Error, 1))
	default:
		// Non-blocking default case if connection exists and is healthy.
	}
	select {
	case chanErr, ok := <-p.chanClosures:
		if ok {
			p.logger.Info(fmt.Sprintf("Re-establishing closed channel: %s", chanErr.Error()))
		}
		var err error
		if p.channel, err = p.conn.Channel(); err != nil {
			return fmt.Errorf("error creating channel: %w", err)
		}
		if err := p.channel.Confirm(false); err != nil {
			// Force closure of possibly healthy connection to force a full
			// re-negotiation in the next ensureConnection call.
			p.conn.Close()
			return fmt.Errorf("error enabling publisher confirms: %w", err)
		}
		p.hasOutstanding = false
		// Might be overkill to set up notifications for both connection and channel
		// closures, but this might save us from getting into weird half-open states.
		p.chanClosures = p.channel.NotifyClose(make(chan *amqp.Error, 1))
		p.confirmations = p.channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	default:
		// Non-blocking default case if channel exists and is healthy.
	}
	return nil
}

// closeOnTimeout closes the connection if the context contains an error (timeout)
func (p *rabbitMQPublisher) CloseOnTimeout(ctx context.Context) {
	if ctx.Err() != nil {
		p.logger.Info("Forcibly closing RabbitMQ connection due to timeout")
		p.Close()
	}
}

func (p *rabbitMQPublisher) tryPublish(ctx context.Context, topic string, message amqp.Publishing) error {
	if err := p.ensureConnection(); err != nil {
		return err
	}
	// Only one goroutine should be publishing at the same time so we won't
	// have to correlate delivery tags to figure out which message was acked.
	p.publishMu.Lock()
	defer p.publishMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	defer p.CloseOnTimeout(ctx) // will always be called, but connection will be closed if ctx contains error
	// If a previous tryPublish call's context expires while it's waiting for
	// its confirmation there could be a confirmation already queued up in
	// the next tryPublish call. That wouldn't have to be a problem in itself
	// (although we'd have to track the delivery tag so we won't claim delivery
	// victory over an old confirmation), but if many publishers bail out
	// (e.g. because of a retry loop that reuses the same expired context)
	// we'll fill up the confirmation channel and eventually block the whole
	// AMQP channel and deadlock everything. We mitigate this by only allowing
	// one in-flight outbound message and draining the confirmation channel
	// prior to each publish operation.
	if p.hasOutstanding {
		c, err := p.awaitConfirmation(ctx)
		if err != nil {
			return err
		}
		if !c.Ack {
			p.logger.Info("A previous message was nacked by the broker")
		}
	}
	if err := p.channel.Publish(p.exchange, topic, false, false, message); err != nil {
		return fmt.Errorf("error publishing message: %w", err)
	}
	p.hasOutstanding = true
	c, err := p.awaitConfirmation(ctx)
	if err != nil {
		return err
	}
	if !c.Ack {
		return fmt.Errorf("message nacked")
	}
	return nil
}
