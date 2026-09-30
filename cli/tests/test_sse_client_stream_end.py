# Copyright Axis Communications AB.
#
# For a full list of individual contributors, please see the commit history.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Tests for how the ETOS SSE v2Alpha client handle the end of an event stream."""

import json
import logging
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from etos_client.sse.v2alpha.client import SSEClient as SSEV2AlphaClient

# Long enough for a client to reconnect, short enough to fail a hanging test quickly.
CONSUMER_TIMEOUT = 15


def _event(event: str, data, event_id: int | None = None) -> bytes:
    """Format a single server-sent event."""
    lines = []
    if event_id is not None:
        lines.append(f"id: {event_id}")
    lines.append(f"event: {event}")
    lines.append(f"data: {data if isinstance(data, str) else json.dumps(data)}")
    return ("\n".join(lines) + "\n\n").encode("utf-8")


V2ALPHA_MESSAGE = {"message": "hello", "name": "etos", "@timestamp": "2026-08-31T10:00:00Z"}
V2ALPHA_SHUTDOWN = {"conclusion": "Successful", "verdict": "Passed", "description": "done"}


class _Response:  # pylint:disable=too-few-public-methods
    """A scripted response from the fake SSE server."""

    def __init__(self, *events: bytes, hold: bool = False) -> None:
        """Store the events to send and whether to keep the response open afterwards."""
        self.events = events
        self.hold = hold


class _FakeSSEServer:
    """A local HTTP server that answers each request with the next scripted response.

    Every response ends cleanly with a terminating chunk, unless it is held open, in which
    case it ends cleanly when the test releases it during teardown.
    """

    def __init__(self, responses: list[_Response]) -> None:
        """Start the server on a free local port."""
        self.responses = list(responses)
        self.requests: list[dict] = []
        self.release = threading.Event()
        self.__lock = threading.Lock()
        self.__server = ThreadingHTTPServer(("127.0.0.1", 0), self.__handler())
        self.__server.daemon_threads = True
        self.__thread = threading.Thread(target=self.__server.serve_forever, daemon=True)
        self.__thread.start()

    @property
    def url(self) -> str:
        """Return the base URL of the server."""
        host, port = self.__server.server_address[:2]
        return f"http://{host}:{port}"

    def close(self) -> None:
        """Release held responses and stop the server."""
        self.release.set()
        self.__server.shutdown()
        self.__server.server_close()

    def _next(self, headers) -> _Response | None:
        """Record a request and return the response scripted for it."""
        with self.__lock:
            self.requests.append({"last_event_id": headers.get("Last-Event-ID")})
            if len(self.requests) > len(self.responses):
                return None
            return self.responses[len(self.requests) - 1]

    def __handler(self) -> type[BaseHTTPRequestHandler]:
        """Create a request handler class bound to this server."""
        fake = self

        class Handler(BaseHTTPRequestHandler):
            """Serve scripted event streams using chunked transfer encoding."""

            protocol_version = "HTTP/1.1"

            def end_headers(self):
                """Close each connection after its response, as clients reset them anyway."""
                self.send_header("Connection", "close")
                self.close_connection = True  # pylint:disable=attribute-defined-outside-init
                super().end_headers()

            def do_GET(self):  # pylint:disable=invalid-name
                """Send the next scripted response."""
                response = fake._next(self.headers)  # pylint:disable=protected-access
                if response is None:
                    self.send_response(400)
                    self.send_header("Content-Length", "0")
                    self.end_headers()
                    return
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Transfer-Encoding", "chunked")
                self.end_headers()
                try:
                    for event in response.events:
                        self.__chunk(event)
                    if response.hold:
                        fake.release.wait()
                    self.__chunk(b"")
                except (BrokenPipeError, ConnectionResetError):
                    # The client has disconnected from a held response, which some tests expect.
                    pass

            def __chunk(self, data: bytes) -> None:
                """Write one chunk of a chunked response and flush it."""
                self.wfile.write(f"{len(data):X}\r\n".encode("ascii") + data + b"\r\n")
                self.wfile.flush()

            def log_message(self, format, *args):  # pylint:disable=redefined-builtin
                """Silence the default request logging."""

        return Handler


@pytest.fixture(name="sse_server")
def fixture_sse_server():
    """Provide a factory for fake SSE servers that are closed after the test."""
    servers = []

    def create(*responses: _Response) -> _FakeSSEServer:
        """Create and start a fake SSE server with scripted responses."""
        server = _FakeSSEServer(list(responses))
        servers.append(server)
        return server

    yield create
    for server in servers:
        server.close()


def _consume(client, stream_id: str = "run1") -> list:
    """Consume a client's event stream in a thread and return the received events.

    Fails the test if the stream does not end within CONSUMER_TIMEOUT, for example when the
    client spins on a finished response. The thread is a daemon so that a spinning client does
    not keep the test process alive.
    """
    received = []
    errors = []

    def run():
        """Collect events until the stream ends."""
        try:
            for event in client.event_stream(stream_id):
                received.append(event)
        except Exception as exception:  # pylint:disable=broad-exception-caught
            errors.append(exception)

    thread = threading.Thread(target=run, daemon=True)
    thread.start()
    thread.join(CONSUMER_TIMEOUT)
    assert not thread.is_alive(), "event stream did not end; the client is stuck"
    assert not errors, f"event stream raised {errors[0]!r}"
    return received


def test_v2alpha_reconnects_when_stream_ends_cleanly(sse_server):
    """The v2alpha client reconnects with Last-Event-ID when a stream ends without shutdown."""
    server = sse_server(
        _Response(_event("message", V2ALPHA_MESSAGE, 1)),
        _Response(_event("message", V2ALPHA_MESSAGE, 2), _event("shutdown", V2ALPHA_SHUTDOWN, 3)),
    )
    events = _consume(SSEV2AlphaClient(server.url, []))

    assert [event.id for event in events] == [1, 2, 3]
    assert events[-1].event == "shutdown"
    assert [request["last_event_id"] for request in server.requests] == [None, "1"]


def test_v2alpha_reconnects_on_retryable_error(sse_server, caplog):
    """The v2alpha client reconnects as soon as it gets an error event with retry true."""
    server = sse_server(
        _Response(
            _event("message", V2ALPHA_MESSAGE, 1),
            _event("error", {"retry": True, "reason": "server is shutting down"}),
            hold=True,
        ),
        _Response(_event("message", V2ALPHA_MESSAGE, 2), _event("shutdown", V2ALPHA_SHUTDOWN, 3)),
    )
    with caplog.at_level(logging.WARNING):
        events = _consume(SSEV2AlphaClient(server.url, []))

    # The first response is still held open, so the client reconnected on the error event.
    assert not server.release.is_set()
    assert [event.event for event in events] == ["message", "message", "shutdown"]
    assert [request["last_event_id"] for request in server.requests] == [None, "1"]
    assert "server is shutting down" in caplog.text


def test_v2alpha_stops_on_non_retryable_error(sse_server, caplog):
    """The v2alpha client stops and logs the reason when it gets an error with retry false."""
    server = sse_server(
        _Response(
            _event("message", V2ALPHA_MESSAGE, 1),
            _event("error", {"retry": False, "reason": "failed to start consuming stream"}),
            hold=True,
        ),
    )
    with caplog.at_level(logging.ERROR):
        events = _consume(SSEV2AlphaClient(server.url, []))

    assert not server.release.is_set()
    assert [event.event for event in events] == ["message"]
    assert len(server.requests) == 1
    assert "failed to start consuming stream" in caplog.text


def test_v2alpha_ignores_error_without_retry_data(sse_server):
    """An error event without the expected data does not change how the stream is handled."""
    server = sse_server(
        _Response(
            _event("message", V2ALPHA_MESSAGE, 1),
            _event("error", "not json"),
            _event("error", {"retry": False}),
            _event("shutdown", V2ALPHA_SHUTDOWN, 2),
        ),
    )
    events = _consume(SSEV2AlphaClient(server.url, []))

    assert [event.event for event in events] == ["message", "shutdown"]
    assert len(server.requests) == 1
