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
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/eiffel-community/etos/pkg/opentelemetry"
	"github.com/go-logr/logr"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/message"
	"go.opentelemetry.io/otel/baggage"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type closeTestProducer struct{ closed bool }

// Send accepts messages without issuing confirmations.
func (*closeTestProducer) Send(message.StreamMessage) error { return nil }

// Close records that transport shutdown was attempted.
func (p *closeTestProducer) Close() error { p.closed = true; return nil }

// TestCloseBoundsMissingConfirmations verifies that cancellation bounds draining and preserves the close result.
func TestCloseBoundsMissingConfirmations(t *testing.T) {
	producer := &closeTestProducer{}
	publisher := &rabbitMQStreamPublisher{
		producer: producer, logger: logr.Discard(), pending: 1,
		drained: make(chan struct{}, 1), done: make(chan struct{}), workerDone: make(chan struct{}),
	}
	go func() { <-publisher.done; close(publisher.workerDone) }()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := publisher.CloseContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("missing drain error: %v", err)
	}
	if !producer.closed {
		t.Fatal("transport close not attempted")
	}
	if err := publisher.CloseContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("repeated close must return the original result")
	}
}

type callbackTestProducer struct {
	send func(message.StreamMessage) error
}

// Send delegates to the configured callback to simulate transport outcomes.
func (p callbackTestProducer) Send(msg message.StreamMessage) error { return p.send(msg) }

// Close succeeds without closing any transport resources.
func (callbackTestProducer) Close() error { return nil }

// newCallbackPublisher creates a publisher with manually collected metrics and registers shutdown cleanup.
func newCallbackPublisher(t *testing.T) (*rabbitMQStreamPublisher, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	options := append(opentelemetry.MetricProviderOptions(), sdkmetric.WithReader(reader))
	provider := sdkmetric.NewMeterProvider(options...)
	recorder, err := newPublishRecorder(provider.Meter("etos/messaging"))
	if err != nil {
		t.Fatal(err)
	}
	publisher := &rabbitMQStreamPublisher{
		logger: logr.Discard(), recorder: recorder, ctx: t.Context(),
		clusterIdentity: "team/blue", unConfirmedMessages: make(chan message.StreamMessage),
		drained: make(chan struct{}, 1), done: make(chan struct{}), workerDone: make(chan struct{}),
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := publisher.CloseContext(ctx); err != nil {
			t.Error(err)
		}
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return publisher, reader
}

// publisherOutcomes collects publication counters and sums their values by outcome.
func publisherOutcomes(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	outcomes := make(map[string]int64)
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if sum, ok := metric.Data.(metricdata.Sum[int64]); ok {
				for _, point := range sum.DataPoints {
					outcome, _ := point.Attributes.Value("outcome")
					outcomes[outcome.AsString()] += point.Value
				}
			}
		}
	}
	return outcomes
}

// TestUnconfirmedResendAndSuccessfulDrain verifies retry telemetry and draining after successful confirmation.
func TestUnconfirmedResendAndSuccessfulDrain(t *testing.T) {
	publisher, reader := newCallbackPublisher(t)
	var sends atomic.Int32
	publisher.producer = callbackTestProducer{send: func(msg message.StreamMessage) error {
		confirmed := sends.Add(1) > 1
		publisher.confirmation(msg, confirmed, nil)
		return nil
	}}
	go publisher.publish(publisher.done)
	if err := publisher.Publish("id", events.NewMessage(events.Log{Message: "test"})); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := publisher.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}
	got := publisherOutcomes(t, reader)
	for outcome, count := range map[string]int64{"accepted": 1, "queued": 2, "unconfirmed": 1, "confirmed": 1} {
		if got[outcome] != count {
			t.Errorf("%s = %d want %d", outcome, got[outcome], count)
		}
	}
	if publisher.pending != 0 {
		t.Fatal("successful drain left pending messages")
	}
}

// TestSendErrorAndConfirmationCannotDoubleComplete verifies that competing terminal callbacks cannot underflow pending counts.
func TestSendErrorAndConfirmationCannotDoubleComplete(t *testing.T) {
	publisher, reader := newCallbackPublisher(t)
	publisher.producer = callbackTestProducer{send: func(msg message.StreamMessage) error {
		// Simulate a terminal send error racing with a duplicate confirmation.
		publisher.confirmation(msg, true, nil)
		publisher.confirmation(msg, true, nil)
		return errors.New("terminal send failure")
	}}
	go publisher.publish(publisher.done)
	if err := publisher.Publish("id", events.NewMessage(events.Log{Message: "test"})); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := publisher.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}
	if publisher.pending != 0 {
		t.Fatalf("drain count underflowed: %d", publisher.pending)
	}
	if publisherOutcomes(t, reader)["send_error"] != 1 {
		t.Fatal("send error stage was not recorded")
	}
}

// TestConcurrentPublishAndClose verifies that concurrent shutdown accounts for accepted and rejected publications.
func TestConcurrentPublishAndClose(t *testing.T) {
	publisher, reader := newCallbackPublisher(t)
	publisher.producer = callbackTestProducer{send: func(msg message.StreamMessage) error {
		publisher.confirmation(msg, true, nil)
		return nil
	}}
	go publisher.publish(publisher.done)
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			err := publisher.Publish("id", events.NewMessage(events.Log{Message: "concurrent"}))
			var closed *PublisherClosedError
			if err != nil && !errors.As(err, &closed) {
				t.Error(err)
			}
		})
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := publisher.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	got := publisherOutcomes(t, reader)
	if got["accepted"]+got["closed"] != 100 || got["accepted"] != got["confirmed"] {
		t.Fatalf("publication lifecycle lost stages: %+v", got)
	}
}

// TestSerializationFailureCompletesDrain verifies that invalid events finish drain accounting without reaching transport.
func TestSerializationFailureCompletesDrain(t *testing.T) {
	publisher, reader := newCallbackPublisher(t)
	publisher.producer = callbackTestProducer{send: func(message.StreamMessage) error {
		t.Error("invalid event reached transport")
		return nil
	}}
	go publisher.publish(publisher.done)
	event := events.NewMessage(events.Log{Extra: map[string]any{"invalid": make(chan int)}})
	if err := publisher.Publish("id", event); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if publisher.pending != 0 || publisherOutcomes(t, reader)["serialization_error"] != 1 {
		t.Fatal("serialization failure did not finish its lifecycle")
	}
}

// TestPublishMetricsSeparateClusters verifies cluster attribution, bounded outcomes and histogram boundaries.
func TestPublishMetricsSeparateClusters(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	options := append(opentelemetry.MetricProviderOptions(), sdkmetric.WithReader(reader))
	provider := sdkmetric.NewMeterProvider(options...)
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	recorder, err := newPublishRecorder(provider.Meter("etos/messaging"))
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{"first/shared", "second/shared"} {
		member, err := baggage.NewMemberRaw("etos_cluster", identity)
		if err != nil {
			t.Fatal(err)
		}
		value, err := baggage.New(member)
		if err != nil {
			t.Fatal(err)
		}
		publisher := rabbitMQStreamPublisher{ctx: t.Context(), recorder: recorder,
			clusterIdentity: publisherCluster(baggage.ContextWithBaggage(context.Background(), value))}
		for _, outcome := range []string{
			publishOutcomeAccepted, publishOutcomeClosed, publishOutcomeSerializationError, publishOutcomeQueued,
			publishOutcomeSendError, publishOutcomeConfirmed, publishOutcomeUnconfirmed, "invalid",
		} {
			publisher.recordPublishTelemetry(outcome, time.Now())
		}
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.ScopeMetrics) != 1 || len(data.ScopeMetrics[0].Metrics) != 2 {
		t.Fatalf("unexpected native instrument count: %+v", data)
	}
	for _, instrument := range data.ScopeMetrics[0].Metrics {
		switch points := instrument.Data.(type) {
		case metricdata.Sum[int64]:
			if len(points.DataPoints) != 14 || !points.IsMonotonic || points.Temporality != metricdata.CumulativeTemporality {
				t.Fatalf("invalid counter %+v", points)
			}
			for _, point := range points.DataPoints {
				if point.Value != 1 || point.Attributes.Len() != 3 {
					t.Fatalf("mixed cluster or outcome: %+v", point)
				}
			}
		case metricdata.Histogram[float64]:
			if len(points.DataPoints) != 14 || points.Temporality != metricdata.CumulativeTemporality {
				t.Fatal("missing lifecycle durations")
			}
			for _, point := range points.DataPoints {
				bounds := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
				if point.Count != 1 || !reflect.DeepEqual(point.Bounds, bounds) || len(point.BucketCounts) != len(bounds)+1 {
					t.Fatalf("duration contract changed: %+v", point)
				}
			}
		default:
			t.Fatalf("unexpected aggregation: %T", points)
		}
	}
	if got := publisherCluster(context.Background()); got != unassignedCluster {
		t.Fatalf("missing identity = %q", got)
	}
}

// TestIsPublishOutcome checks that publish outcomes exclude arbitrary label values.
func TestIsPublishOutcome(t *testing.T) {
	tests := []struct {
		name    string
		outcome string
		want    bool
	}{
		{name: "accepted", outcome: publishOutcomeAccepted, want: true},
		{name: "closed", outcome: publishOutcomeClosed, want: true},
		{name: "serialization error", outcome: publishOutcomeSerializationError, want: true},
		{name: "queued", outcome: publishOutcomeQueued, want: true},
		{name: "send error", outcome: publishOutcomeSendError, want: true},
		{name: "confirmed", outcome: publishOutcomeConfirmed, want: true},
		{name: "unconfirmed", outcome: publishOutcomeUnconfirmed, want: true},
		{name: "unbounded value", outcome: "rabbitmq-error-text", want: false},
		{name: "empty", outcome: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isPublishOutcome(test.outcome); got != test.want {
				t.Fatalf("isPublishOutcome(%q) = %t, want %t", test.outcome, got, test.want)
			}
		})
	}
}
