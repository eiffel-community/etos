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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/go-logr/logr"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/ha"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/message"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ConfirmationTimeout = 2 * time.Second
)

type PublisherClosedError struct{}

func (e *PublisherClosedError) Error() string {
	return "Publisher is closed"
}

// RabbitMQStreamPublisher is a structure implementing the Publisher interface. Used to publish events
// to a RabbitMQ stream.
type rabbitMQStreamPublisher struct {
	logger              logr.Logger
	streamName          string
	environment         *stream.Environment
	options             *stream.ProducerOptions
	producer            *ha.ReliableProducer
	unConfirmedMessages chan message.StreamMessage
	unConfirmed         *sync.WaitGroup
	done                chan struct{}
	shutdown            bool
}

// NewRabbitMQStreamPublisher creates a new RabbitMQ stream publisher. It connects to the
// RabbitMQ stream and checks if the stream exists. If it does, it starts the publisher.
func NewRabbitMQStreamPublisher(
	ctx context.Context,
	name string,
	config v1alpha1.RabbitMQ,
	cli client.Client,
	namespace string,
) (EventPublisher, error) {
	password, err := config.Password.Get(ctx, cli, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get RabbitMQ password: %w", err)
	}
	scheme := "rabbitmq-stream"
	if config.SSL == "true" {
		scheme = "rabbitmq-stream+tls"
	}
	if config.Vhost == "/" {
		// Avoid having double slashes for the vhost in the following url.
		config.Vhost = ""
	}
	address := fmt.Sprintf("%s://%s:%s@%s:%s/%s",
		scheme,
		config.Username,
		password,
		config.Host,
		config.StreamPort,
		config.Vhost,
	)
	environmentOptions := stream.NewEnvironmentOptions().SetMaxProducersPerClient(1).SetUri(address)
	if config.SSL == "true" {
		environmentOptions = environmentOptions.SetTLSConfig(&tls.Config{ServerName: config.Host})
	}
	env, err := stream.NewEnvironment(environmentOptions)
	if err != nil {
		return nil, err
	}

	exists, err := env.StreamExists(config.StreamName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("no stream exists, cannot stream events")
	}
	options := stream.NewProducerOptions().
		SetClientProvidedName(name).
		SetConfirmationTimeOut(ConfirmationTimeout)
	publisher := &rabbitMQStreamPublisher{
		logger:      logr.Discard(),
		streamName:  config.StreamName,
		environment: env,
		options:     options,
		unConfirmed: &sync.WaitGroup{},
	}
	return publisher, publisher.Start()
}

// Start will start the RabbitMQ stream publisher, non blocking.
func (s *rabbitMQStreamPublisher) Start() error {
	s.unConfirmedMessages = make(chan message.StreamMessage)
	s.options.SetFilter(stream.NewProducerFilter(func(message message.StreamMessage) string {
		p := message.GetApplicationProperties()
		return fmt.Sprintf("%s.%s.%s", p["identifier"], p["type"], p["meta"])
	}))
	producer, err := ha.NewReliableProducer(
		s.environment,
		s.streamName,
		s.options,
		func(messageStatus []*stream.ConfirmationStatus) {
			go func() {
				for _, msgStatus := range messageStatus {
					if msgStatus.IsConfirmed() {
						s.logger.V(1).Info("Message confirmed")
						s.unConfirmed.Done()
					} else {
						s.logger.Error(msgStatus.GetError(), "Unconfirmed message")
						s.unConfirmedMessages <- msgStatus.GetMessage()
					}
				}
			}()
		})
	if err != nil {
		return err
	}
	s.producer = producer
	s.done = make(chan struct{})
	go s.publish(s.done)
	return nil
}

func (s *rabbitMQStreamPublisher) AddLogger(logger logr.Logger) {
	logger = logger.WithValues("disableUserLog", true)
	s.logger = logger
}

// Publish an event to the RabbitMQ stream.
func (s *rabbitMQStreamPublisher) Publish(identifier string, e events.Event) error {
	if s.shutdown || s.producer == nil {
		err := &PublisherClosedError{}
		s.logger.Error(err, "Publisher is closed")
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	s.unConfirmed.Add(1)
	msg := amqp.NewMessage(b)
	msg.ApplicationProperties = map[string]any{
		"identifier": identifier,
		"type":       strings.ToLower(e.EventType()),
		"meta":       e.EventMeta(),
	}
	s.unConfirmedMessages <- msg
	s.logger.V(1).Info("Event published to unconfirmed channel")
	return nil
}

// publish a message from unconfirmed messages to RabbitMQ.
func (s *rabbitMQStreamPublisher) publish(done chan struct{}) {
	for {
		select {
		case msg := <-s.unConfirmedMessages:
			s.logger.V(1).Info("Publishing message to RabbitMQ stream")
			if err := s.producer.Send(msg); err != nil {
				s.logger.Error(err, "Failed to send message")
				s.unConfirmed.Done()
				continue
			}
			s.logger.V(1).Info("Published")
		case <-done:
			return
		}
	}
}

// Close the RabbitMQ stream publisher.
func (s *rabbitMQStreamPublisher) Close() error {
	if s.producer != nil {
		s.logger.Info("Stopping publisher")
		s.shutdown = true
		s.logger.Info("Wait for unconfirmed messages")
		s.unConfirmed.Wait()
		s.done <- struct{}{}
		s.logger.Info("Done, closing down")
		if err := s.producer.Close(); err != nil {
			s.logger.Error(err, "Failed to close rabbitmq publisher")
		}
		close(s.unConfirmedMessages)
	}
	return nil
}
