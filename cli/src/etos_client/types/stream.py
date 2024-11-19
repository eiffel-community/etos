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
"""Stream type description."""
import abc
import time

from typing import Iterable


class Event:
    """An event from ETOS."""

    def __init__(self, event: dict) -> None:
        """Initialize the object from a dict."""
        self.data = event.get("data", "")
        self.id = event.get("id")
        if self.id is not None:
            self.id = int(self.id)
        self.event = event.get("event")
        self.received = time.time()

    def __str__(self) -> str:
        """Return the string representation of an event."""
        return f"{self.event}({self.id}): {self.data}"

    def __eq__(self, other: "Event") -> bool:  # type:ignore
        """Check if the event is the same by testing the IDs."""
        if self.id is None or other.id is None:
            return super().__eq__(other)
        return self.id == other.id


class Stream(metaclass=abc.ABCMeta):
    """Base class for ETOS event streaming protocol."""

    def __init__(self, url: str, stream_id: str):
        """Set up URL and stream ID."""
        self.url = url
        self.id = stream_id

    @classmethod
    @abc.abstractmethod
    def version(cls) -> str:
        """SSE stream version."""
        return "Unknown"

    @abc.abstractmethod
    def event_stream(self) -> Iterable[Event]:
        """Connect to and follow the ETOS SSE event stream."""

    @abc.abstractmethod
    def close(self) -> None:
        """Close down the SSE client."""

    @abc.abstractmethod
    def reset(self) -> None:
        """Reset the client. Must be done before reconnecting."""
