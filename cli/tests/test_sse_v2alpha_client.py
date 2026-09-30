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
"""Tests for the SSE v2alpha client."""

import json
from typing import Iterable, Optional

import pytest
from etos_lib.messaging.events import Message, Shutdown

from etos_client.sse.v2alpha import client as client_module
from etos_client.sse.v2alpha.client import SSEClient

LOG = {"message": "hello", "name": "etos", "@timestamp": "2026-09-30T10:00:00Z", "level": "info"}
RESULT = {"conclusion": "Successful", "verdict": "Passed", "description": "done"}


def message(event_id: int) -> str:
    """Return a message SSE event with an ID."""
    return f"id: {event_id}\nevent: message\ndata: {json.dumps(LOG)}\n\n"


def shutdown(event_id: int) -> str:
    """Return a shutdown SSE event with an ID."""
    return f"id: {event_id}\nevent: shutdown\ndata: {json.dumps(RESULT)}\n\n"


def ping(event_id: Optional[int] = None) -> str:
    """Return a ping SSE event, optionally with an ID."""
    if event_id is None:
        return "event: ping\ndata: \n\n"
    return f"id: {event_id}\nevent: ping\ndata: \n\n"


def error(retry: bool, reason: str) -> str:
    """Return an error SSE event."""
    return f"event: error\ndata: {json.dumps({'retry': retry, 'reason': reason})}\n\n"


class FakeServer:  # pylint: disable=too-few-public-methods
    """Replace the connection of an SSEClient with a series of fake responses."""

    def __init__(self, client: SSEClient, responses: list[str]) -> None:
        """Install the fake connection on the client."""
        self.client = client
        self.responses = list(responses)
        self.last_event_ids: list[Optional[int]] = []
        client._SSEClient__connect = self._connect  # pylint: disable=protected-access

    def _connect(self, _: str, is_initial_connection: bool = False) -> Iterable[bytes]:
        """Record the Last-Event-ID of the connection and return the next response."""
        del is_initial_connection
        self.client._SSEClient__connected = True  # pylint: disable=protected-access
        if not self.responses:
            self.client._SSEClient__shutdown = True  # pylint: disable=protected-access
            return iter([])
        self.last_event_ids.append(self.client.last_event_id)
        return iter([self.responses.pop(0).encode("utf-8")])


@pytest.fixture(name="client")
def fixture_client(monkeypatch: pytest.MonkeyPatch) -> SSEClient:
    """Return an SSEClient that does not sleep between reconnects."""
    monkeypatch.setattr(client_module.time, "sleep", lambda _: None)
    return SSEClient("http://localhost", [])


def test_sparse_ids_are_accepted(client: SSEClient):
    """Test that event IDs only need to be strictly increasing, not contiguous."""
    server = FakeServer(client, [message(3) + message(10) + message(4711) + shutdown(4800)])
    events = list(client.event_stream("id"))
    assert [event.id for event in events] == [3, 10, 4711, 4800]
    assert isinstance(events[-1], Shutdown)
    assert server.last_event_ids == [None]


def test_already_received_events_are_dropped(client: SSEClient):
    """Test that events with an ID not greater than the last received ID are dropped."""
    FakeServer(client, [message(5) + message(5) + message(2) + message(6) + shutdown(7)])
    events = list(client.event_stream("id"))
    assert [event.id for event in events] == [5, 6, 7]


def test_reconnect_sends_last_event_id(client: SSEClient):
    """Test that the client reconnects with the ID of the last event when the stream ends."""
    server = FakeServer(client, [message(1) + message(9), message(9) + message(12) + shutdown(20)])
    events = list(client.event_stream("id"))
    assert [event.id for event in events] == [1, 9, 12, 20]
    assert server.last_event_ids == [None, 9]


def test_ping_updates_last_event_id(client: SSEClient):
    """Test that a ping ID updates the last event ID, and that pings are not yielded."""
    server = FakeServer(client, [message(1) + ping(50) + ping() + ping(20), shutdown(60)])
    events = list(client.event_stream("id"))
    assert [event.id for event in events] == [1, 60]
    assert all(isinstance(event, (Message, Shutdown)) for event in events)
    assert server.last_event_ids == [None, 50]


def test_retryable_error_reconnects(client: SSEClient):
    """Test that a retryable error from the server makes the client reconnect."""
    server = FakeServer(
        client, [message(1) + error(True, "closed") + message(2), message(2) + shutdown(3)]
    )
    events = list(client.event_stream("id"))
    assert [event.id for event in events] == [1, 2, 3]
    assert server.last_event_ids == [None, 1]


def test_non_retryable_error_stops(client: SSEClient, caplog: pytest.LogCaptureFixture):
    """Test that a non-retryable error stops the client without reconnecting."""
    server = FakeServer(client, [message(1) + error(False, "events expired"), shutdown(2)])
    events = list(client.event_stream("id"))
    assert [event.id for event in events] == [1]
    assert server.last_event_ids == [None]
    assert "events expired" in caplog.text


def test_error_without_retry_hint_reconnects(client: SSEClient):
    """Test that an error without a retry hint is treated as retryable."""
    server = FakeServer(client, ["event: error\ndata: oops\n\n", shutdown(1)])
    events = list(client.event_stream("id"))
    assert [event.id for event in events] == [1]
    assert server.last_event_ids == [None, None]


EVENT_1 = b'id: 1\nevent: message\ndata: {"message": "first"}\n\n'
EVENT_2 = b'id: 2\nevent: message\ndata: {"message": "second"}\n\n'
STREAM = EVENT_1 + EVENT_2

# Each event as the client should yield it: its lines, without the empty line that ends it.
EXPECTED_EVENTS = [EVENT_1[:-1].decode(), EVENT_2[:-1].decode()]


def read_events(client: SSEClient, blocks: list[bytes]) -> list[str]:
    """Feed blocks of bytes to the client and return the events."""
    return list(client._SSEClient__read(blocks))


def test_stream_in_one_block(client: SSEClient):
    """Test that a stream received in one block is split into its events."""
    assert read_events(client, [STREAM]) == EXPECTED_EVENTS


def test_block_ends_just_before_line_break(client: SSEClient):
    """Test that an event is intact when a block ends between a line and its line break."""
    first_block = b"id: 1\nevent: message"
    second_block = STREAM[len(first_block) :]
    assert second_block.startswith(b"\ndata:")
    assert read_events(client, [first_block, second_block]) == EXPECTED_EVENTS


@pytest.mark.parametrize("position", range(1, len(STREAM)))
def test_block_ends_at_any_position(client: SSEClient, position: int):
    """Test that events are intact no matter where the stream is split."""
    blocks = [STREAM[:position], STREAM[position:]]
    assert read_events(client, blocks) == EXPECTED_EVENTS


def test_every_byte_in_its_own_block(client: SSEClient):
    """Test that events are intact when each byte is received in a separate block."""
    blocks = [bytes([byte]) for byte in STREAM]
    assert read_events(client, blocks) == EXPECTED_EVENTS
