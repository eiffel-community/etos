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
	"encoding/json"
	"errors"
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

// maxEmptyReconnects is the number of consecutive connections that may close without delivering
// any event, ping or retryable server error before the subscriber gives up.
const maxEmptyReconnects = 5

// reconnectDelay is the initial delay before reconnecting to the SSE stream. It doubles for each
// consecutive reconnect without progress, up to maxReconnectDelay.
var (
	reconnectDelay    = 100 * time.Millisecond
	maxReconnectDelay = 5 * time.Second
)

// ErrStreamClosed is yielded when the SSE stream keeps closing without delivering anything.
var ErrStreamClosed = errors.New("SSE event stream closed without delivering any events")

// ServerError is an error event sent by the SSE server.
type ServerError struct {
	Retry  bool   `json:"retry"`
	Reason string `json:"reason"`
}

// Error returns the reason of the ServerError.
func (e *ServerError) Error() string {
	return fmt.Sprintf("SSE server error: %s", e.Reason)
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
//
// Event IDs are strictly increasing but not contiguous. The subscriber reconnects with the
// Last-Event-ID header when the stream closes or the server sends a retryable error, and drops
// events that it has already received. Pings are not yielded; their IDs only record progress.
// A retryable server error makes the subscriber reconnect until the context is done. A
// non-retryable server error, a decoding error or a failure to connect is yielded as an error and
// stops the iteration. ErrStreamClosed is yielded if the stream closes maxEmptyReconnects times in
// a row without delivering anything.
func (c *SSESubscriber) Events(ctx context.Context, id string, filter ...Filter) iter.Seq2[events.Event, error] {
	logger := logging.FromContextOrDiscard(ctx)
	return func(yield func(events.Event, error) bool) {
		var lastID int64
		emptyConnections := 0
		attempt := 0
		for {
			result := c.follow(ctx, id, &lastID, yield, filter...)
			switch result {
			case resultDone:
				return
			case resultProgress:
				emptyConnections = 0
				attempt = 0
			case resultRetry:
				emptyConnections = 0
				attempt++
			case resultEmpty:
				emptyConnections++
				attempt++
			}
			if emptyConnections >= maxEmptyReconnects {
				logger.Info("SSE stream closed without delivering any events, giving up")
				yield(nil, ErrStreamClosed)
				return
			}
			delay := min(reconnectDelay<<min(attempt, 16), maxReconnectDelay)
			logger.Info("Reconnecting to SSE stream", "last_event_id", lastID, "delay", delay)
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			case <-time.After(delay):
			}
		}
	}
}

// followResult describes how a connection to the SSE stream ended.
type followResult int

const (
	// resultDone means that the iteration is done.
	resultDone followResult = iota
	// resultProgress means that the stream closed after delivering at least one event or ping.
	resultProgress
	// resultRetry means that the server sent a retryable error before any event or ping.
	resultRetry
	// resultEmpty means that the stream closed without delivering anything.
	resultEmpty
)

// follow connects to the SSE stream and yields its events until the stream closes.
func (c *SSESubscriber) follow(
	ctx context.Context, id string, lastID *int64, yield func(events.Event, error) bool, filter ...Filter,
) followResult {
	logger := logging.FromContextOrDiscard(ctx)
	stream, err := c.stream(ctx, id, *lastID, filter...)
	if err != nil {
		yield(nil, err)
		return resultDone
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			logger.Error(closeErr, "Error closing stream")
		}
	}()

	progressed := false
	closedResult := func() followResult {
		if progressed {
			return resultProgress
		}
		return resultEmpty
	}
	decoder := sse.NewDecoder(stream)
	for {
		var sseEvent sse.Event
		if err := decoder.Decode(&sseEvent); err != nil {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return resultDone
			}
			if errors.Is(err, io.EOF) {
				logger.Info("SSE stream closed")
			} else {
				logger.Error(err, "Error reading SSE stream")
			}
			return closedResult()
		}
		eventID, event, err := c.decode(sseEvent)
		if err != nil {
			var serverError *ServerError
			if errors.As(err, &serverError) && serverError.Retry {
				logger.Info("SSE server sent a retryable error", "reason", serverError.Reason)
				if progressed {
					return resultProgress
				}
				return resultRetry
			}
			logger.Error(err, "Error decoding event")
			yield(nil, err)
			return resultDone
		}
		progressed = true
		if eventID > 0 && eventID <= *lastID {
			logger.V(1).Info("Dropping already received event", "id", eventID, "last_event_id", *lastID)
			continue
		}
		if eventID > 0 {
			*lastID = eventID
		}
		if _, ok := event.(events.Ping); ok {
			continue
		}
		if !yield(event, nil) {
			return resultDone
		}
	}
}

// stream establishes a connection to the SSE endpoint and returns the response body as an io.ReadCloser.
func (c *SSESubscriber) stream(ctx context.Context, id string, lastID int64, filter ...Filter) (io.ReadCloser, error) {
	logger := logging.FromContextOrDiscard(ctx)
	url := fmt.Sprintf("%s/%s%s", c.baseUrl, id, filtersToQuery(filter))
	logger.Info(fmt.Sprintf("Connecting to SSE stream, %s", url))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if lastID > 0 {
		request.Header.Set("Last-Event-ID", strconv.FormatInt(lastID, 10))
	}
	client := &http.Client{}
	var response *http.Response
	err = retry.Constant(ctx, 5*time.Second, func(ctx context.Context) error {
		response, err = client.Do(request)
		if err != nil {
			// A transport error (e.g. connection refused while the server is
			// still starting up) is typically transient, so retry it instead
			// of giving up immediately.
			return retry.RetryableError(err)
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
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

// decode parses an SSE event into its ID and an Event object. The ID is 0 if the event has none.
// Event IDs are stream offsets, which are int64 in the ETOS API.
// An error event from the server is returned as a *ServerError.
func (c *SSESubscriber) decode(sseEvent sse.Event) (int64, events.Event, error) {
	var id int64
	if sseEvent.ID != "" {
		var err error
		id, err = strconv.ParseInt(sseEvent.ID, 10, 64)
		if err != nil {
			return 0, nil, err
		}
	}
	eventType := events.EventType(strings.ToLower(sseEvent.Event))
	if eventType == "error" {
		return id, nil, parseServerError(sseEvent.Data)
	}
	bareID := int(id)
	if bareID == 0 {
		bareID = -1
	}
	event, err := events.Parse(events.BareEvent{
		ID:    bareID,
		Event: eventType,
		Data:  sseEvent.Data,
	})
	return id, event, err
}

// parseServerError parses the data of an error event. An error without a retry hint is retryable.
func parseServerError(data any) *ServerError {
	if raw, ok := data.(sse.Raw); ok {
		return &ServerError{Retry: true, Reason: string(raw)}
	}
	serverError := &ServerError{Retry: true}
	raw, err := json.Marshal(data)
	if err != nil || json.Unmarshal(raw, serverError) != nil {
		return &ServerError{Retry: true, Reason: fmt.Sprint(data)}
	}
	return serverError
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
