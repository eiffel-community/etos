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
"""Get ETOS sse logs."""
import traceback
import time
import json
import logging
from datetime import datetime
from zoneinfo import ZoneInfo
from typing import Iterator, Union

import requests
from sseclient import SSEClient

from etos_client.etos import ETOS

# pylint: disable=too-few-public-methods


class Ping:
    """ETOS log API ping."""


class Message:
    """An ETOS log message."""

    def __init__(self, message: str) -> None:
        """Initialize fields that are necessary to log an ETOS message."""
        self.message = json.loads(message)
        self.level = self.message.get("levelname", "INFO").lower()
        self.name = self.message.get("name")

        # ETOS library will always format the timestamps as ISO8601 UTC
        # https://github.com/eiffel-community/etos-library/blob/main/src/etos_lib/logging/formatter.py
        dtime = datetime.strptime(
            self.message.get("@timestamp"), "%Y-%m-%dT%H:%M:%S.%fZ"
        ).replace(tzinfo=ZoneInfo("UTC"))
        dtime = dtime.astimezone()

        self.datestring = datetime.strftime(dtime, "%Y-%m-%d %H:%M:%S")

    def __str__(self) -> str:
        """Message part of an ETOS message."""
        return self.message.get("message")


class Logs:
    """Connect to ETOS log API and print all logs from ETOS."""

    logger = logging.getLogger(__name__)
    __events = None
    __current_id = 0

    def connect_to_log_server(self, etos: ETOS, endtime: int) -> None:
        """Connect to the ETOS log server."""
        while time.time() < endtime:
            try:
                self.__events = SSEClient(
                    f"{etos.cluster}/logs/{str(etos.response.tercc)}"
                )
                break
            except requests.exceptions.HTTPError as http_error:
                if http_error.response.status_code != 404:
                    traceback.print_exc()
                time.sleep(2)
        else:
            raise TimeoutError("Timed out while connecting to log server")

    def logs(self, endtime: int) -> Iterator[Union[Message, Ping]]:
        """Connect to log server and collect logs from it."""
        try:
            for event in self.__events:
                if event.data:
                    event_id = int(event.id)
                    if event_id <= self.__current_id:
                        # When the SSE client reconnects all messages will be returned
                        # again. Tracking the IDs so that we don't print them unnecessarily.
                        self.logger.debug(
                            "Ignoring message with id %d because it has already been logged",
                            event_id,
                        )
                        continue
                    self.__current_id = event_id
                    yield Message(event.data)
                else:
                    # Pings are expected, by default, to come at a 15s interval.
                    yield Ping()
                if time.time() >= endtime:
                    raise TimeoutError("Timed out!")
        except requests.exceptions.ChunkedEncodingError:
            traceback.print_exc()
        except requests.exceptions.HTTPError as http_error:
            if http_error.response.status_code == 404:
                return
            raise
