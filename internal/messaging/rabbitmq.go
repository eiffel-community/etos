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
	"github.com/go-logr/logr"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/ha"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/message"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Publisher interface {
	Publish([]byte, string) error
	AddLogger(logr.Logger)
	Close() error
}

const (
	IgnoreUnfiltered    = false
	MaxLengthBytes      = "2gb"
	MaxAge              = 10 * time.Second
	ConfirmationTimeout = 2 * time.Second
)

// NewPublisher creates a new Publisher instance.
func NewPublisher(ctx context.Context, config v1alpha1.RabbitMQ, cli client.Client, namespace string) (Publisher, error) {
	return newRabbitMQStreamPublisher(ctx, "provider", config, cli, namespace)
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

type Filter struct {
	Identifier string
	Type       string
	Meta       string
}

// FromString creates a Filter struct from a string in the format "identifier.type.meta".
// If the string does not have three parts, the missing parts will be set to an empty string.
func (f Filter) FromString(s string) Filter {
	parts := make([]string, 3)
	copy(parts, strings.SplitN(s, ".", 3))
	return Filter{
		Identifier: parts[0],
		Type:       parts[1],
		Meta:       parts[2],
	}
}

// newRabbitMQStreamPublisher creates a new RabbitMQ stream publisher. It connects to the
// RabbitMQ stream and checks if the stream exists. If it does, it starts the publisher.
func newRabbitMQStreamPublisher(
	ctx context.Context,
	name string,
	config v1alpha1.RabbitMQ,
	cli client.Client,
	namespace string,
) (*rabbitMQStreamPublisher, error) {
	password, err := config.Password.Get(ctx, cli, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get RabbitMQ password: %w", err)
	}
	scheme := "amqp"
	if config.SSL == "true" {
		scheme = "amqps"
	}
	address := fmt.Sprintf("%s://%s:%s@%s:%s%s",
		scheme,
		config.Username,
		password,
		config.Host,
		config.StreamPort,
		config.Vhost,
	)
	environmentOptions := stream.NewEnvironmentOptions().SetMaxProducersPerClient(1).SetUri(address)
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
		SetProducerName(name).
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
						s.logger.Info("Message confirmed")
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
func (s *rabbitMQStreamPublisher) Publish(b []byte, filterString string) error {
	filter := Filter{}.FromString(filterString)
	if s.shutdown || s.producer == nil {
		err := errors.New("Publisher closed")
		s.logger.Error(err, "Publisher is closed")
		return err
	}
	s.unConfirmed.Add(1)
	msg := amqp.NewMessage(b)
	if filter.Meta == "" {
		filter.Meta = "*"
	}
	msg.ApplicationProperties = map[string]any{
		"identifier": filter.Identifier,
		"type":       filter.Type,
		"meta":       filter.Meta,
	}
	s.unConfirmedMessages <- msg
	s.logger.Info("Message published to unconfirmed channel")
	return nil
}

// publish a message from unconfirmed messages to RabbitMQ.
func (s *rabbitMQStreamPublisher) publish(done chan struct{}) {
	for {
		select {
		case msg := <-s.unConfirmedMessages:
			s.logger.Info("Publishing message to RabbitMQ stream")
			if err := s.producer.Send(msg); err != nil {
				s.logger.Error(err, "Failed to send message")
				s.unConfirmed.Done()
				continue
			}
			s.logger.Info("Published")
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
