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
	"sync/atomic"
	"time"

	"github.com/eiffel-community/etos/api/v1alpha1"
	"github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/go-logr/logr"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/ha"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/message"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/metric"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

type publishRecorder struct {
	attempts metric.Int64Counter
	duration metric.Float64Histogram
}

type streamProducer interface {
	Send(message.StreamMessage) error
	Close() error
}

// pendingMessage makes confirmation/send-error races idempotent for drain accounting.
// It embeds the unchanged AMQP payload; the stream client returns this same message
// in confirmations, so no transport-visible field or metric dimension is added.
type pendingMessage struct {
	message.StreamMessage
	finished atomic.Bool
}

// newPublishRecorder creates RabbitMQ publish lifecycle counter and duration instruments using the supplied meter.
// It returns an error if either instrument cannot be created.
func newPublishRecorder(meter metric.Meter) (*publishRecorder, error) {
	attempts, err := meter.Int64Counter("etos_message_publish_attempts_total", metric.WithUnit("1"),
		metric.WithDescription("ETOS RabbitMQ message publish lifecycle outcomes."))
	if err != nil {
		return nil, err
	}
	duration, err := meter.Float64Histogram("etos_message_publish_duration_seconds", metric.WithUnit("s"),
		metric.WithDescription("ETOS RabbitMQ message publish lifecycle duration."))
	if err != nil {
		return nil, err
	}
	return &publishRecorder{attempts, duration}, nil
}

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
	producer            streamProducer
	unConfirmedMessages chan message.StreamMessage
	mu                  sync.Mutex
	pending             int
	drained             chan struct{}
	workerDone          chan struct{}
	callbacks           sync.WaitGroup
	publishes           sync.WaitGroup
	closeOnce           sync.Once
	closeErr            error
	recorder            *publishRecorder
	ctx                 context.Context
	done                chan struct{}
	shutdown            bool
	stopped             bool
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
	recorder, err := newPublishRecorder(otel.Meter("etos/messaging"))
	if err != nil {
		return nil, fmt.Errorf("create publisher instruments: %w", err)
	}
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
		recorder:           recorder,
		ctx:                ctx,
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
			s.log().Error(establishErr, "Failed to connect to RabbitMQ stream, retrying")
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
	s.unConfirmedMessages = make(chan message.StreamMessage)
	s.done = make(chan struct{})
	s.workerDone = make(chan struct{})
	s.drained = make(chan struct{}, 1)
	s.options.SetFilter(stream.NewProducerFilter(func(message message.StreamMessage) string {
		p := message.GetApplicationProperties()
		return fmt.Sprintf("%s.%s.%s", p["identifier"], p["type"], p["meta"])
	}))
	producer, err := ha.NewReliableProducer(
		s.environment,
		s.streamName,
		s.options,
		func(messageStatus []*stream.ConfirmationStatus) {
			for _, status := range messageStatus {
				s.confirmation(status.GetMessage(), status.IsConfirmed(), status.GetError())
			}
		})
	if err != nil {
		return err
	}
	s.producer = producer
	go s.publish(s.done)
	return nil
}

// confirmation asynchronously records a confirmation outcome and completes or requeues the message.
// It ignores callbacks after the publisher has stopped.
func (s *rabbitMQStreamPublisher) confirmation(msg message.StreamMessage, confirmed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.callbacks.Go(func() {
		if confirmed {
			s.recordPublishTelemetry(publishOutcomeConfirmed, time.Now())
			s.complete(msg)
			return
		}
		s.recordPublishTelemetry(publishOutcomeUnconfirmed, time.Now())
		s.log().Error(err, "Unconfirmed message")
		select {
		case s.unConfirmedMessages <- msg:
		case <-s.done:
		}
	})
}

// AddLogger configures the publisher logger without emitting user-facing logs.
func (s *rabbitMQStreamPublisher) AddLogger(logger logr.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	logger = logger.WithValues("disableUserLog", true)
	s.logger = logger
}

// log returns the publisher logger under the mutex to synchronize with AddLogger.
func (s *rabbitMQStreamPublisher) log() logr.Logger {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logger
}

// Publish an event to the RabbitMQ stream.
func (s *rabbitMQStreamPublisher) Publish(identifier string, e events.Event) error {
	start := time.Now()
	tracked := &pendingMessage{}
	s.mu.Lock()
	if s.shutdown || s.producer == nil {
		logger := s.logger
		s.mu.Unlock()
		err := &PublisherClosedError{}
		logger.Error(err, "Publisher is closed")
		s.recordPublishTelemetry(publishOutcomeClosed, start)
		return err
	}
	s.pending++
	s.publishes.Add(1)
	defer s.publishes.Done()
	if s.pending == 1 {
		select {
		case <-s.drained:
		default:
		}
	}
	s.mu.Unlock()
	b, err := json.Marshal(e)
	if err != nil {
		s.complete(tracked)
		s.recordPublishTelemetry(publishOutcomeSerializationError, start)
		return err
	}
	msg := amqp.NewMessage(b)
	msg.ApplicationProperties = map[string]any{
		"identifier": identifier,
		"type":       e.EventType().ToLower(),
		"meta":       e.EventMeta(),
	}
	tracked.StreamMessage = msg
	select {
	case s.unConfirmedMessages <- tracked:
	case <-s.done:
		s.complete(tracked)
		s.recordPublishTelemetry(publishOutcomeClosed, start)
		return &PublisherClosedError{}
	}
	s.recordPublishTelemetry(publishOutcomeAccepted, start)
	return nil
}

// publish a message from unconfirmed messages to RabbitMQ.
func (s *rabbitMQStreamPublisher) publish(done chan struct{}) {
	defer close(s.workerDone)
	for {
		select {
		case msg := <-s.unConfirmedMessages:
			if tracked, ok := msg.(*pendingMessage); ok && tracked.finished.Load() {
				continue
			}
			queuedAt := time.Now()
			s.recordPublishTelemetry(publishOutcomeQueued, queuedAt)
			sendStarted := time.Now()
			if err := s.producer.Send(msg); err != nil {
				s.recordPublishTelemetry(publishOutcomeSendError, sendStarted)
				s.log().Error(err, "Failed to send message")
				s.complete(msg)
				continue
			}
		case <-done:
			return
		}
	}
}

// record records the original callback-local timing, not queue or acknowledgement latency.
func (r *publishRecorder) record(ctx context.Context, outcome string, started time.Time, cluster string) {
	if !isPublishOutcome(outcome) {
		return
	}
	identity := unassignedCluster
	if cluster != "" {
		identity = cluster
	}
	attrs := metric.WithAttributes(attribute.String("operation", publishOperation),
		attribute.String("outcome", outcome), attribute.String("etos_cluster", identity))
	r.attempts.Add(ctx, 1, attrs)
	r.duration.Record(ctx, time.Since(started).Seconds(), attrs)
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

// recordPublishTelemetry records an outcome and elapsed duration with the publisher's cluster identity
// when a recorder is configured.
func (s *rabbitMQStreamPublisher) recordPublishTelemetry(outcome string, started time.Time) {
	if s.recorder != nil {
		s.recorder.record(s.ctx, outcome, started, s.clusterIdentity)
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.CloseContext(ctx)
}

// CloseContext bounds confirmation draining without changing the public publisher interface.
// The pinned HA client's Send/Close calls are not cancellable; transport stalls
// still require an upstream fix before a hard process shutdown bound is possible.
func (s *rabbitMQStreamPublisher) CloseContext(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.shutdown = true
		pending := s.pending
		s.mu.Unlock()
		if s.producer == nil {
			return
		}
		s.log().Info("Stopping publisher")
		s.log().Info("Wait for unconfirmed messages")
		if pending > 0 {
			select {
			case <-s.drained:
			case <-ctx.Done():
				s.closeErr = ctx.Err()
			}
		}
		s.mu.Lock()
		s.stopped = true
		close(s.done)
		s.mu.Unlock()
		s.closeErr = errors.Join(s.closeErr, s.producer.Close())
		<-s.workerDone
		s.callbacks.Wait()
		s.publishes.Wait()
		s.log().Info("Done, closing down")
		if s.environment != nil {
			s.closeErr = errors.Join(s.closeErr, s.environment.Close())
		}
	})
	return s.closeErr
}

// complete decrements the pending count once per tracked message and signals when all messages are drained.
// It ignores untracked messages and repeated completion attempts.
func (s *rabbitMQStreamPublisher) complete(msg message.StreamMessage) {
	tracked, ok := msg.(*pendingMessage)
	if !ok || !tracked.finished.CompareAndSwap(false, true) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending--
	if s.pending == 0 {
		select {
		case s.drained <- struct{}{}:
		default:
		}
	}
}
