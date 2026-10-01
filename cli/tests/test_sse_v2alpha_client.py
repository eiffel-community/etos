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
"""Tests for how the SSE v2alpha client splits a received byte stream into events.

The SSE format is line-based, but the client receives the stream from the network
in blocks of bytes. A block can end anywhere, including in the middle of a line,
and the client must still produce complete events.
"""

# pylint: disable=protected-access

import pytest

from etos_client.sse.v2alpha.client import SSEClient

EVENT_1 = b'id: 1\nevent: message\ndata: {"message": "first"}\n\n'
EVENT_2 = b'id: 2\nevent: message\ndata: {"message": "second"}\n\n'
STREAM = EVENT_1 + EVENT_2

# Each event as the client should yield it: its lines, without the empty line that ends it.
EXPECTED_EVENTS = [EVENT_1[:-1].decode(), EVENT_2[:-1].decode()]


@pytest.fixture(name="client")
def fixture_client():
    """Create an SSE v2alpha client. No connection is made."""
    client = SSEClient("http://localhost", [])
    yield client
    client.close()


def read_events(client, blocks: list[bytes]) -> list[str]:
    """Feed blocks of bytes to the client, as if received from the network, and return the events."""
    return list(client._SSEClient__read(blocks))


def test_stream_in_one_block(client):
    """Test that a stream received in one block is split into its events."""
    assert read_events(client, [STREAM]) == EXPECTED_EVENTS


def test_block_ends_just_before_line_break(client):
    """Test that an event is intact when a block ends between a line and its line break.

    The second block then starts with '\\n', which must not be mistaken for the empty
    line that ends an event.
    """
    first_block = b"id: 1\nevent: message"
    second_block = STREAM[len(first_block) :]
    assert second_block.startswith(b"\ndata:")

    assert read_events(client, [first_block, second_block]) == EXPECTED_EVENTS


@pytest.mark.parametrize("position", range(1, len(STREAM)))
def test_block_ends_at_any_position(client, position):
    """Test that the events are intact no matter where the stream is split into two blocks."""
    blocks = [STREAM[:position], STREAM[position:]]
    assert read_events(client, blocks) == EXPECTED_EVENTS


def test_every_byte_in_its_own_block(client):
    """Test that the events are intact when each byte is received in a separate block."""
    blocks = [bytes([byte]) for byte in STREAM]
    assert read_events(client, blocks) == EXPECTED_EVENTS
