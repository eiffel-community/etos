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
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/baggage"
)

func TestPublishMetricsSeparateClusters(t *testing.T) {
	for _, identity := range []string{"first/shared", "second/shared"} {
		member, err := baggage.NewMemberRaw("etos_cluster", identity)
		if err != nil {
			t.Fatal(err)
		}
		value, err := baggage.New(member)
		if err != nil {
			t.Fatal(err)
		}
		publisher := rabbitMQStreamPublisher{clusterIdentity: publisherCluster(baggage.ContextWithBaggage(context.Background(), value))}
		counter := publishAttempts.WithLabelValues(publishOperation, publishOutcomeClosed, identity)
		before := &dto.Metric{}
		if err := counter.Write(before); err != nil {
			t.Fatal(err)
		}
		publisher.recordPublishTelemetry(publishOutcomeClosed, time.Now())
		after := &dto.Metric{}
		if err := counter.Write(after); err != nil {
			t.Fatal(err)
		}
		if got := after.GetCounter().GetValue(); got != before.GetCounter().GetValue()+1 {
			t.Fatalf("cluster %s counter = %v, want %v", identity, got, before.GetCounter().GetValue()+1)
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
