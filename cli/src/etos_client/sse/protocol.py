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
"""Get ETOS sse events."""
import inspect
import json
import sys
import time
from datetime import datetime
from zoneinfo import ZoneInfo

# pylint: disable=too-few-public-methods


class Event:
    """An event from ETOS."""

    def __init__(self, event: dict) -> None:
        """Initialize the object from a dict."""
        self.data = event.get("data", "")
        self.id = int(event.get("id")) if event.get("id") else None
        self.event = event.get("event", "message")
        self.received = time.time()

    def __str__(self) -> str:
        """Return the string representation of an event."""
        return f"{self.event}({self.id}): {self.data}"

    def __eq__(self, other: "Event") -> bool:
        """Check if the event is the same by testing the IDs."""
        if self.id is None or other.id is None:
            return super().__eq__(other)
        return self.id == other.id


class ServerEvent(Event):
    """Events to be handled by the client."""


class UserEvent(Event):
    """Events to be handled by the user."""


class Ping(ServerEvent):
    """An SSE ping event."""


class Shutdown(ServerEvent):
    """A shutdown event from the SSE server."""


class Error(ServerEvent):
    """An error from the SSE server."""


class Unknown(UserEvent):
    """An unknown SSE event."""


class Message(UserEvent):
    """An ETOS user log event."""

    def __init__(self, event: dict) -> None:
        """Initialize a message by loading an expected json string."""
        super().__init__(event)

        try:
            self.message = json.loads(self.data)
        except json.JSONDecodeError:
            print(self.data)
            raise
        self.level = self.message.get("levelname", "INFO").lower()
        self.name = self.message.get("name")

        # ETOS library will always format the timestamps as ISO8601 UTC
        # https://github.com/eiffel-community/etos-library/blob/main/src/etos_lib/logging/formatter.py
        dtime = datetime.strptime(self.message.get("@timestamp"), "%Y-%m-%dT%H:%M:%S.%fZ").replace(
            tzinfo=ZoneInfo("UTC")
        )
        dtime = dtime.astimezone()

        self.datestring = datetime.strftime(dtime, "%Y-%m-%d %H:%M:%S")

    def __str__(self):
        """Return the string representation of a user log."""
        return self.message.get("message")


def parse(event: dict) -> Event:
    """Parse an event dict and return a corresponding Event class."""
    for name, obj in inspect.getmembers(sys.modules[__name__]):
        if event.get("event", "").lower() == name.lower():
            return obj(event)
    return Unknown(event)
