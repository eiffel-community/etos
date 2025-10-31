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
from json import loads, JSONDecodeError
from typing import Callable, Iterable, Optional

from urllib3.exceptions import HTTPError, MaxRetryError
from urllib3.poolmanager import PoolManager
from urllib3.util import Retry

#from etos_lib.messaging.events import Shutdown, Event, parse # import disabled due to: https://github.com/eiffel-community/etos/issues/417
# dummy classes: remove when the etos_lib.messaging module is available
class Event:
    pass

class Shutdown:
    pass

def parse(event):
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


class LogRetry(Retry):
    """A Retry class that logs progress during initial connection attempts."""

    def __init__(self, *args, **kwargs):
        self.logger = logging.getLogger(__name__)
        self._attempt_count = 0
        super().__init__(*args, **kwargs)

    def increment(self, method=None, url=None, response=None, error=None, _pool=None, _stacktrace=None):
        """Override increment to add logging for initial connection attempts."""
        self._attempt_count += 1
        new_retry = super().increment(method, url, response, error, _pool, _stacktrace)

        # Copy counter to new retry instance
        if hasattr(new_retry, '_attempt_count'):
            new_retry._attempt_count = self._attempt_count

        # Log progress for 404 responses (test run not ready) at specific intervals
        is_404_response = response is not None and response.status == 404
        is_logging_interval = self._attempt_count in [4, 8, 12, 16, 20]

        if is_404_response and is_logging_interval:
            self.logger.info("Still waiting for test run to start...")

        return new_retry


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
        self.__pool = PoolManager(retries=RETRIES)
        self.__release: Optional[Callable] = None

    @classmethod
    def version(cls) -> str:
        """SSE protocol version."""
        return "v2alpha"

    def __connect(self, stream_id: str, apikey: str, is_initial_connection=False) -> Iterable[bytes]:
        """Handle connection for reconnections."""
        if is_initial_connection:
            # Use LogRetry with extended retries for initial connection
            retries = LogRetry(
                total=None,
                read=0,
                connect=20,  # More attempts for initial connection
                status=20,
                backoff_factor=2,  # Exponential backoff
                backoff_max=120,  # Cap at 2 minutes
                status_forcelist={413, 429, 503, 404},  # Include 404 for initial connection
            )
            self.logger.info("Connecting to SSE server")
            self.logger.info("Waiting for test run to start...")
        else:
            # Standard retries for reconnections
            retries = RETRIES

        return self.__do_connect(stream_id, apikey, retries)

    def __do_connect(self, stream_id: str, apikey: str, retries: Retry) -> Iterable[bytes]:
        """Connect to an event-stream server with the given retry policy.

        Sets the attribute `__release` which must be closed before exiting.
        """
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
                stream = self.__connect(stream_id, apikey, is_initial_connection=not bool(stream))
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
