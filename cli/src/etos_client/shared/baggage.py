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
"""Baggage management for ETOS."""

import os

MAX_SIZE_BYTES = 8192  # https://www.w3.org/TR/baggage/#limits
MAX_LIST_MEMBERS = 64  # https://www.w3.org/TR/baggage/#limits


class Baggage:
    """Handle baggage management for ETOS."""

    def __init__(self, baggage_str: str):
        """Initialize the Baggage class."""
        self.__baggage = self.__parse_baggage(baggage_str)

    @classmethod
    def from_env(cls) -> "Baggage":
        """Create a Baggage instance from the ETOS_BAGGAGE environment variable.

        Returns:
            A Baggage instance.
        """
        baggage_str = ""
        if "BAGGAGE" in os.environ:
            baggage_str = os.environ["BAGGAGE"]
        return cls(baggage_str)

    def get(self, key: str) -> str | list | None:
        """Get a value from the baggage.

        Returns:
            The value for the given key, or None if the key does not exist.
        """
        return self.__baggage.get(key)

    def add(self, key: str, value: str | list[str]) -> None:
        """Add a key-value pair to the baggage."""
        if len(self.__baggage.keys()) == MAX_LIST_MEMBERS:
            raise ValueError("Baggage exceeds maximum number of members (64).")
        self.__baggage[key] = value

    @property
    def string(self) -> str:
        """Get the baggage as a string.

        Returns:
            The baggage as a string.
        """
        items = []
        if len(self.__baggage.keys()) > MAX_LIST_MEMBERS:
            raise ValueError("Baggage exceeds maximum number of members (64).")

        for key, value in self.__baggage.items():
            if isinstance(value, list):
                value = ";".join(value)
            items.append(f"{key}={value}")
        baggage_str = ",".join(items)

        if len(baggage_str.encode("utf-8")) > 8192:
            raise ValueError("Baggage string exceeds maximum length of 8192 characters.")
        return baggage_str

    def __parse_baggage(self, baggage_str: str) -> dict:
        """Parse baggage string into a dictionary.

        Returns:
            A dictionary representation of the baggage.
        """
        baggage = {}
        items = baggage_str.split(",")
        for item in items:
            if "=" not in item:
                continue
            key, value = item.split("=", 1)
            if len(value.split(";")) > 1:
                value = [v.strip() for v in value.split(";")]
            else:
                value = value.strip()
            baggage[key.strip()] = value
        return baggage
