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
package subscriber

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eiffel-community/etos/pkg/logging"
	events "github.com/eiffel-community/etos/pkg/messaging/events"
	"github.com/sethvargo/go-retry"
	"go.jetify.com/sse"
)

type Filter struct {
	EventType events.EventType
	Meta      string
}

// String returns a string representation of the filter in the format "EventType.Meta".
func (f Filter) String() string {
	if f.Meta == "" {
		return strings.ToLower(fmt.Sprintf("%s.*", f.EventType))
	}
	return strings.ToLower(fmt.Sprintf("%s.%s", f.EventType, f.Meta))
}

type SSESubscriber struct {
	baseUrl string
}

// NewSSESubscriber creates a new SSEClient instance with the specified ID and optional filters.
func NewSSESubscriber(host string) *SSESubscriber {
	return &SSESubscriber{
		baseUrl: fmt.Sprintf("%s/v2alpha/events", host),
	}
}

// Events iterates over the events from the SSE stream and yields them as Event objects.
// If an error occurs while reading from the stream or decoding an event, it yields the error and stops the iteration.
func (c *SSESubscriber) Events(ctx context.Context, id string, filter ...Filter) iter.Seq2[events.Event, error] {
	logger := logging.FromContextOrDiscard(ctx)
	return func(yield func(events.Event, error) bool) {
		stream, err := c.stream(ctx, id, filter...)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() {
			if closeErr := stream.Close(); closeErr != nil {
				logger.Error(closeErr, "Error closing stream")
			}
		}()

		decoder := sse.NewDecoder(stream)
		for {
			event, err := c.decode(decoder)
			if err != nil {
				logger.Error(err, "Error decoding event")
				if err == io.EOF {
					return
				}
				yield(nil, err)
				return
			}
			if !yield(event, nil) {
				return
			}
		}
	}
}

// stream establishes a connection to the SSE endpoint and returns the response body as an io.ReadCloser.
func (c *SSESubscriber) stream(ctx context.Context, id string, filter ...Filter) (io.ReadCloser, error) {
	logger := logging.FromContextOrDiscard(ctx)
	url := fmt.Sprintf("%s/%s%s", c.baseUrl, id, filtersToQuery(filter))
	logger.Info(fmt.Sprintf("Connecting to SSE stream, %s", url))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	var response *http.Response
	err = retry.Constant(ctx, 5*time.Second, func(ctx context.Context) error {
		response, err = client.Do(request)
		if err != nil {
			return err
		}
		switch response.StatusCode {
		case http.StatusOK:
			logger.Info("Successfully connected to SSE stream")
			return nil
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			if closeErr := response.Body.Close(); closeErr != nil {
				logger.Error(closeErr, "Error closing response body")
			}
			return retry.RetryableError(fmt.Errorf("received status code %d, retrying", response.StatusCode))
		default:
			if closeErr := response.Body.Close(); closeErr != nil {
				logger.Error(closeErr, "Error closing response body")
			}
			return fmt.Errorf("unexpected status code: %d", response.StatusCode)
		}
	})
	return response.Body, err
}

// decode reads the next event from the SSE decoder and attempts to parse it into an Event object.
func (c *SSESubscriber) decode(decoder *sse.Decoder) (events.Event, error) {
	var sseEvent sse.Event
	var err error
	if err = decoder.Decode(&sseEvent); err != nil {
		return nil, err
	}
	id := -1
	if sseEvent.ID != "" {
		id, err = strconv.Atoi(sseEvent.ID)
		if err != nil {
			return nil, err
		}
	}
	return events.Parse(events.BareEvent{
		ID:    id,
		Event: events.EventType(strings.ToLower(sseEvent.Event)),
		Data:  sseEvent.Data,
	})
}

// filtersToQuery converts a slice of Filter objects into a query string format suitable for the SSE endpoint.
func filtersToQuery(filters []Filter) string {
	if len(filters) == 0 {
		return ""
	}
	var query strings.Builder
	query.WriteString("?")
	for i, filter := range filters {
		fmt.Fprintf(&query, "filter=%s", filter.String())
		if i < len(filters)-1 {
			query.WriteString("&")
		}
	}
	return query.String()
}
