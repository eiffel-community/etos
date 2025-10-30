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

import logging
import time
from enum import Enum
from json import loads, JSONDecodeError
from typing import Callable, Iterable, Optional

from urllib3.exceptions import HTTPError, MaxRetryError
from urllib3.poolmanager import PoolManager
from urllib3.util import Retry

#from etos_lib.messaging.events import Shutdown, Event, parse # import disabled due to: https://github.com/eiffel-community/etos/issues/417
# dummy class: remove when the etos_lib.messaging module is available
class Event:
    pass


CHUNK_SIZE = 500
RETRIES = Retry(
    total=None,
    read=0,
    connect=10,
    status=10,
    backoff_factor=1,
    status_forcelist=Retry.RETRY_AFTER_STATUS_CODES,  # 413, 429, 503
)


class ConnectionState(Enum):
    """Connection state for SSE client."""
    INITIAL = "initial"        # Waiting for test run to start
    CONNECTED = "connected"    # Successfully connected, streaming events
    RECONNECTING = "reconnecting"  # Reconnecting after temporary failure


class Desynced(Exception):
    """The event stream has desynced."""


class TokenExpired(Exception):
    """The API key token has expired."""


class ServerShutdown(Exception):
    """Server wants the client to shut down."""


class NoResponse(HTTPError):
    """Empty response from the SSE server."""


class HTTPStatusError(HTTPError):
    """A bad status request from the SSE server."""


class HTTPWrongContentType(HTTPError):
    """Wrong content type from the SSE server."""


class SSEClient:
    """A client for reading an event stream from ETOS."""

    logger = logging.getLogger(__name__)
    __connected = False
    __shutdown = False

    def __init__(self, url: str, filter: list) -> None:
        """Set up a connection pool and retry strategy."""
        self.url = url
        self.filter = filter
        self.last_event_id = None
        self.connection_state = ConnectionState.INITIAL
        self.__pool = PoolManager(retries=RETRIES)
        self.__release: Optional[Callable] = None

    @classmethod
    def version(cls) -> str:
        """SSE protocol version."""
        return "v2alpha"

    def __connect(self, stream_id: str, apikey: str, retry_not_found=False) -> Iterable[bytes]:
        """Connect to an event-stream server with state-aware retry logic.

        Sets the attribute `__release` which must be closed before exiting.
        """
        retries = RETRIES
        if retry_not_found:
            retries = retries.new(status_forcelist={413, 429, 503, 404})

        # Use extended retries and user feedback only during initial connection
        if self.connection_state == ConnectionState.INITIAL:
            return self.__initial_connect(stream_id, apikey, retries)
        else:
            # Standard connection for reconnections
            return self.__standard_connect(stream_id, apikey, retries)

    def __initial_connect(self, stream_id: str, apikey: str, retries: Retry) -> Iterable[bytes]:
        """Handle initial connection with user feedback and exponential backoff retries."""
        # Use exponential backoff: 20 attempts with backoff_factor=2 gives ~30 minutes total
        # Wait times: 1s, 2s, 4s, 8s, 16s, 32s, 64s, 120s, 120s, ... (capped at 120s)
        extended_retries = retries.new(connect=20, status=20, backoff_factor=2, backoff_max=120)
        filter_param = "&".join(f"filter={value}" for value in self.filter)
        url = f"{self.url}/sse/{self.version()}/events/{stream_id}?{filter_param}"

        self.logger.info("Connecting to SSE server at %s", url)
        self.logger.info("Waiting for test run to start...")

        headers = {
            "Cache-Control": "no-cache",
            "Accept": "text/event-stream",
            "Authorization": f"Bearer {apikey}",
        }
        if self.last_event_id is not None:
            headers["Last-Event-ID"] = str(self.last_event_id)

        max_attempts = extended_retries.connect or extended_retries.status or 20
        total_elapsed_time = 0

        for attempt in range(max_attempts):
            try:
                # Single attempt per loop iteration
                response = self.__pool.request(
                    "GET",
                    url,
                    headers=headers,
                    preload_content=False,
                    retries=Retry(total=0, connect=0, status=0),
                )
                self.__release = response.release_conn

                if response.status == 200:
                    content_type = response.headers.get("Content-Type", None)
                    if content_type is None:
                        raise HTTPWrongContentType("Server did not respond with a content type")
                    if not content_type.startswith("text/event-stream"):
                        raise HTTPWrongContentType(f"Bad content type from server: {content_type!r}, expected text/event-stream")

                    self.logger.info("Test run started successfully, beginning event stream")
                    # Switch to connected state after successful connection
                    self.connection_state = ConnectionState.CONNECTED
                    self.__connected = True
                    return response.stream(CHUNK_SIZE, True)

                elif response.status == 404:
                    # Log progress for 404 errors (test run not ready yet)
                    if attempt in [0, 2, 5, 8, 11, 14, 17]:  # Log at key intervals
                        self.logger.info("Still waiting for test run to start...")
                elif response.status == 401:
                    raise TokenExpired("API Key has expired")
                elif response.status >= 400:
                    if attempt % 3 == 0:  # Log errors more frequently for non-404 errors
                        self.logger.info(f"Connection failed with status {response.status} (attempt {attempt+1}), retrying...")
                elif response.status == 204:
                    if attempt % 3 == 0:
                        self.logger.info(f"Empty response from server (attempt {attempt+1}), retrying...")

                response.release_conn()

            except Exception as e:
                if attempt % 3 == 0:  # Log connection errors
                    self.logger.info(f"Connection attempt {attempt+1} failed: {str(e)}, retrying...")

            if attempt < max_attempts - 1:
                # Calculate exponential backoff with max cap
                backoff_time = min(
                    extended_retries.backoff_factor * (2 ** attempt),
                    extended_retries.backoff_max or 120
                )
                total_elapsed_time += backoff_time
                time.sleep(backoff_time)

        # If we get here, all attempts failed
        raise MaxRetryError(
            pool=None,
            url=url,
            reason="Max retries exceeded waiting for test run to start"
        )

    def __standard_connect(self, stream_id: str, apikey: str, retries: Retry) -> Iterable[bytes]:
        """Handle standard connection for reconnections."""
        headers = {
            "Cache-Control": "no-cache",
            "Accept": "text/event-stream",
            "Authorization": f"Bearer {apikey}",
        }
        if self.last_event_id is not None:
            headers["Last-Event-ID"] = str(self.last_event_id)

        try:
            filter_param = "&".join(f"filter={value}" for value in self.filter)
            url = f"{self.url}/sse/{self.version()}/events/{stream_id}?{filter_param}"
            response = self.__pool.request(
                "GET",
                url,
                headers=headers,
                preload_content=False,
                retries=retries,
            )
            self.__release = response.release_conn
        except MaxRetryError as exception:
            # `MaxRetryError.reason` is the underlying exception
            if exception.reason is not None:
                raise exception.reason
            raise

        if response.status >= 400:
            if response.status == 401:
                raise TokenExpired("API Key has expired")
            raise HTTPStatusError(f"Error code {response.status} when connecting to SSE server")
        if response.status == 204:
            raise NoResponse("Empty response from the SSE server")

        content_type = response.headers.get("Content-Type", None)
        if content_type is None:
            raise HTTPWrongContentType("Server did not respond with a content type")
        if not content_type.startswith("text/event-stream"):
            raise HTTPWrongContentType(
                f"Bad content type from server: {content_type!r}, expected text/event-stream"
            )

        self.__connected = True
        return response.stream(CHUNK_SIZE, True)

    def __del__(self) -> None:
        """Make sure we release the connection on object deletion."""
        self.close()

    def close(self) -> None:
        """Close down the SSE client."""
        self.__shutdown = True
        self.reset()

    def reset(self) -> None:
        """Reset the client. Must be done before reconnecting."""
        self.__pool.clear()
        if self.__release is not None:
            self.__release()
        self.__connected = False
        # If we were connected, switch to reconnecting state; otherwise stay in initial state
        if self.connection_state == ConnectionState.CONNECTED:
            self.connection_state = ConnectionState.RECONNECTING

    def __line_buffered(self, chunks: Iterable[bytes]) -> Iterable[bytes]:
        """Read chunks from a byte feed and split each chunk on line-break."""
        for chunk in chunks:
            if len(chunk) == 0:
                continue
            yield from chunk.splitlines(keepends=True)

    def __read(self, chunks: Iterable[bytes]) -> Iterable[str]:
        """Read chunks from a byte feed and split them into SSE event blobs."""
        feed = b""
        for line in self.__line_buffered(chunks):
            if line == b"\n":
                yield feed.decode("utf-8")
                feed = b""
            else:
                feed += line

    def __stream(self, stream: Iterable[bytes]) -> Iterable[Event]:
        """Parse a feed of bytes into a feed of events."""
        for event_str in self.__read(stream):
            event = self.__parse_event(event_str)
            if event.id is None:
                yield event
                continue
            # if event.id or 0 == self.last_event_id or 0:
            if self.__already_received(event):
                continue  # Ignore if already received
            if self.__out_of_sync(event):
                raise Desynced
            self.last_event_id = event.id
            yield event
            if isinstance(event, Shutdown):
                raise ServerShutdown
            time.sleep(0.001)

    def __parse_event(self, feed: str) -> Event:
        """Parse an SSE event string into an ETOS event."""
        event = {}
        for line in feed.splitlines():
            _type, value = line.split(":", 1)
            event.setdefault(_type, "")
            event[_type] += value.strip()
        try:
            event["data"] = loads(event["data"])
        except JSONDecodeError:
            pass
        return parse(event)

    def __already_received(self, event: Event) -> bool:
        """Check if an even has already been received."""
        if self.last_event_id is None:
            return False
        if event.id is None:
            return False
        return event.id <= self.last_event_id

    def __out_of_sync(self, event: Event) -> bool:
        """Check if the eventstream is out of sync."""
        if self.last_event_id is None:
            return False
        if event.id is None:
            return False
        if event.id - self.last_event_id == 1:
            return False
        return True

    def event_stream(self, stream_id: str, apikey: str) -> Iterable[Event]:
        """Follow the ETOS SSE event stream."""
        stream = []
        while not self.__shutdown:
            while self.__connected is False:
                stream = self.__connect(stream_id, apikey, retry_not_found=not bool(stream))
                time.sleep(1)
                if not stream:
                    self.logger.warning("Failed connecting to stream. Reconnecting")
                    self.reset()
                    continue
            try:
                yield from self.__stream(stream)
            except (Desynced, JSONDecodeError):
                self.logger.warning("Desynced event stream. Reconnecting")
                self.reset()
            except ServerShutdown:
                self.logger.info("SSE server has requested a shut down")
                self.close()  # close sets __shutdown to True, exiting the while loop.
            except HTTPError:
                self.logger.debug("HTTP error from the SSE server, reconnecting", exc_info=True)
                self.reset()
        self.logger.info("Shutting down SSE client")
