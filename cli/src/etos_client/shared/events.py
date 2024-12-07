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
"""Shared event types."""
import time


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
