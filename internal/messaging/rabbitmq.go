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
	"math"
	"strings"
	"sync"
	"time"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/ha"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/message"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
	"go.opentelemetry.io/otel/baggage"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	unassignedCluster   = "unassigned"
	ConfirmationTimeout = 2 * time.Second
	publishOperation    = "publish"

	publishOutcomeAccepted           = "accepted"
	publishOutcomeClosed             = "closed"
	publishOutcomeSerializationError = "serialization_error"
	publishOutcomeQueued             = "queued"
	publishOutcomeSendError          = "send_error"
	publishOutcomeConfirmed          = "confirmed"
	publishOutcomeUnconfirmed        = "unconfirmed"

	// connectInitialInterval is the wait before the first reconnect attempt when
	// establishing the initial RabbitMQ stream connection.
	connectInitialInterval = 1 * time.Second
	// connectMaxInterval caps the wait between reconnect attempts.
	connectMaxInterval = 30 * time.Second
	// connectBackoffFactor is the factor the wait is multiplied by after each attempt.
	connectBackoffFactor = 2.0
	// connectBackoffJitter adds randomness to the wait so multiple clients don't
	// reconnect at the same time.
	connectBackoffJitter = 0.1
	// connectTimeout bounds the total time spent trying to establish the initial
	// connection before giving up and reporting the failure.
	connectTimeout = 2 * time.Minute
)

var (
	publishAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "etos_message_publish_attempts_total",
			Help: "ETOS RabbitMQ message publish lifecycle outcomes.",
		},
		[]string{"operation", "outcome", "etos_cluster"},
	)
	publishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "etos_message_publish_duration_seconds",
			Help: "ETOS RabbitMQ message publish lifecycle duration.",
		},
		[]string{"operation", "outcome", "etos_cluster"},
	)
	registerPublishMetrics sync.Once
)

type PublisherClosedError struct{}

// Error describes an attempt to publish through a closed publisher.
func (e *PublisherClosedError) Error() string {
	return "Publisher is closed"
}

// RabbitMQStreamPublisher is a structure implementing the Publisher interface. Used to publish events
// to a RabbitMQ stream.
type rabbitMQStreamPublisher struct {
	clusterIdentity     string
	logger              logr.Logger
	streamName          string
	environmentOptions  *stream.EnvironmentOptions
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
// The initial connection is retried with an increasing wait between attempts to recover
// from temporary RabbitMQ connection problems during startup.
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
	options := stream.NewProducerOptions().
		SetClientProvidedName(name).
		SetConfirmationTimeOut(ConfirmationTimeout)
	publisher := &rabbitMQStreamPublisher{
		clusterIdentity:    publisherCluster(ctx),
		logger:             logr.Discard(),
		streamName:         config.StreamName,
		environmentOptions: environmentOptions,
		options:            options,
		unConfirmed:        &sync.WaitGroup{},
	}
	if err := publisher.connect(ctx); err != nil {
		return nil, err
	}
	return publisher, nil
}

// connect establishes the initial RabbitMQ stream connection, retrying with an increasing
// wait between attempts to recover from temporary connection problems. It returns the last
// error encountered if the connection cannot be established before connectTimeout elapses.
func (s *rabbitMQStreamPublisher) connect(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	backoff := wait.Backoff{
		Duration: connectInitialInterval,
		Factor:   connectBackoffFactor,
		Jitter:   connectBackoffJitter,
		Cap:      connectMaxInterval,
		Steps:    math.MaxInt32,
	}
	var lastErr error
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(context.Context) (bool, error) {
		if establishErr := s.establish(); establishErr != nil {
			lastErr = establishErr
			s.logger.Error(establishErr, "Failed to connect to RabbitMQ stream, retrying")
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		if lastErr != nil {
			return fmt.Errorf("failed to establish RabbitMQ stream connection: %w", lastErr)
		}
		return fmt.Errorf("failed to establish RabbitMQ stream connection: %w", err)
	}
	return nil
}

// establish creates a RabbitMQ stream environment, verifies that the stream exists and
// starts the publisher. If any step fails, the environment it created is closed so that a
// subsequent reconnect attempt starts from a clean state.
func (s *rabbitMQStreamPublisher) establish() error {
	env, err := stream.NewEnvironment(s.environmentOptions)
	if err != nil {
		return err
	}
	exists, err := env.StreamExists(s.streamName)
	if err != nil {
		_ = env.Close()
		return err
	}
	if !exists {
		_ = env.Close()
		return errors.New("no stream exists, cannot stream events")
	}
	s.environment = env
	if err := s.Start(); err != nil {
		_ = env.Close()
		s.environment = nil
		return err
	}
	return nil
}

// Start will start the RabbitMQ stream publisher, non blocking.
func (s *rabbitMQStreamPublisher) Start() error {
	registerRabbitMQPublishMetrics()
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
						s.recordPublishTelemetry(publishOutcomeConfirmed, time.Now())
						s.unConfirmed.Done()
					} else {
						s.recordPublishTelemetry(publishOutcomeUnconfirmed, time.Now())
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

// AddLogger configures the publisher logger without emitting user-facing logs.
func (s *rabbitMQStreamPublisher) AddLogger(logger logr.Logger) {
	logger = logger.WithValues("disableUserLog", true)
	s.logger = logger
}

// Publish an event to the RabbitMQ stream.
func (s *rabbitMQStreamPublisher) Publish(identifier string, e events.Event) error {
	registerRabbitMQPublishMetrics()
	start := time.Now()
	if s.shutdown || s.producer == nil {
		err := &PublisherClosedError{}
		s.logger.Error(err, "Publisher is closed")
		s.recordPublishTelemetry(publishOutcomeClosed, start)
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		s.recordPublishTelemetry(publishOutcomeSerializationError, start)
		return err
	}
	s.unConfirmed.Add(1)
	msg := amqp.NewMessage(b)
	msg.ApplicationProperties = map[string]any{
		"identifier": identifier,
		"type":       e.EventType().ToLower(),
		"meta":       e.EventMeta(),
	}
	s.unConfirmedMessages <- msg
	s.recordPublishTelemetry(publishOutcomeAccepted, start)
	return nil
}

// publish a message from unconfirmed messages to RabbitMQ.
func (s *rabbitMQStreamPublisher) publish(done chan struct{}) {
	for {
		select {
		case msg := <-s.unConfirmedMessages:
			queuedAt := time.Now()
			s.recordPublishTelemetry(publishOutcomeQueued, queuedAt)
			sendStarted := time.Now()
			if err := s.producer.Send(msg); err != nil {
				s.recordPublishTelemetry(publishOutcomeSendError, sendStarted)
				s.logger.Error(err, "Failed to send message")
				s.unConfirmed.Done()
				continue
			}
		case <-done:
			return
		}
	}
}

// registerRabbitMQPublishMetrics registers publisher instruments once per process.
func registerRabbitMQPublishMetrics() {
	registerPublishMetrics.Do(func() {
		ctrlmetrics.Registry.MustRegister(publishAttempts)
		ctrlmetrics.Registry.MustRegister(publishDuration)
	})
}

// recordPublishTelemetry records duration and count for a bounded publish outcome.
func recordPublishTelemetry(outcome string, started time.Time, cluster ...string) {
	if !isPublishOutcome(outcome) {
		return
	}
	identity := unassignedCluster
	if len(cluster) == 1 && cluster[0] != "" {
		identity = cluster[0]
	}
	publishAttempts.WithLabelValues(publishOperation, outcome, identity).Inc()
	publishDuration.WithLabelValues(publishOperation, outcome, identity).Observe(time.Since(started).Seconds())
}

// publisherCluster captures explicit propagated ownership, not a RabbitMQ/service name.
func publisherCluster(ctx context.Context) string {
	identity := baggage.FromContext(ctx).Member("etos_cluster").Value()
	parts := strings.Split(identity, "/")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return identity
	}
	return unassignedCluster
}

func (s *rabbitMQStreamPublisher) recordPublishTelemetry(outcome string, started time.Time) {
	recordPublishTelemetry(outcome, started, s.clusterIdentity)
}

// isPublishOutcome reports whether an outcome belongs to the bounded metric vocabulary.
func isPublishOutcome(outcome string) bool {
	switch outcome {
	case publishOutcomeAccepted,
		publishOutcomeClosed,
		publishOutcomeSerializationError,
		publishOutcomeQueued,
		publishOutcomeSendError,
		publishOutcomeConfirmed,
		publishOutcomeUnconfirmed:
		return true
	default:
		return false
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
