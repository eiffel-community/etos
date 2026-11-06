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
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/eiffel-community/etos/api/v1alpha1"
)

// TestAMQPURI verifies that RabbitMQ configuration is converted to an AMQP URI that parses back
// to the configured connection parameters.
func TestAMQPURI(t *testing.T) {
	testcases := []struct {
		name     string
		config   v1alpha1.RabbitMQ
		expected amqp.URI
	}{
		{
			name:   "default vhost",
			config: v1alpha1.RabbitMQ{Host: "rabbitmq", Port: "15672", Username: "guest", Vhost: "/"},
			expected: amqp.URI{
				Scheme: "amqp", Host: "rabbitmq", Port: 15672, Username: "guest", Password: "secret", Vhost: "/",
			},
		},
		{
			name: "ssl uses amqps",
			config: v1alpha1.RabbitMQ{
				Host: "rabbitmq", Port: "5671", Username: "guest", Vhost: "eiffel", SSL: "true",
			},
			expected: amqp.URI{
				Scheme: "amqps", Host: "rabbitmq", Port: 5671, Username: "guest", Password: "secret", Vhost: "eiffel",
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			uri, err := amqpURI(tc.config, "secret")
			if err != nil {
				t.Fatalf("Failed to build AMQP URI: %v", err)
			}
			parsed, err := amqp.ParseURI(uri)
			if err != nil {
				t.Fatalf("Failed to parse AMQP URI: %v", err)
			}
			if parsed.Scheme != tc.expected.Scheme || parsed.Host != tc.expected.Host ||
				parsed.Port != tc.expected.Port || parsed.Username != tc.expected.Username ||
				parsed.Password != tc.expected.Password || parsed.Vhost != tc.expected.Vhost {
				t.Errorf("Expected %+v, got %+v", tc.expected, parsed)
			}
		})
	}
}

// TestAMQPURIInvalidPort verifies that a non-numeric port is rejected.
func TestAMQPURIInvalidPort(t *testing.T) {
	if _, err := amqpURI(v1alpha1.RabbitMQ{Host: "rabbitmq", Port: "amqp"}, "secret"); err == nil {
		t.Error("Expected an error for an invalid port")
	}
}
