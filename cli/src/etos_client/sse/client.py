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
from json import JSONDecodeError
from typing import Callable, Iterable, Optional

from urllib3.exceptions import HTTPError, MaxRetryError
from urllib3.poolmanager import PoolManager
from urllib3.util import Retry

from ..etos import ETOS
from .protocol import Event, Shutdown, parse

CHUNK_SIZE = 500
RETRIES = Retry(
    total=None,
    read=0,
    connect=10,
    status=10,
    backoff_factor=1,
    status_forcelist=Retry.RETRY_AFTER_STATUS_CODES,  # 413, 429, 503
)


class Desynced(Exception):
    """The event stream has desynced."""


class ServerShutdown(Exception):
    """Server wants the client to shut down."""


class NoResponse(HTTPError):
    """No response from the SSE server."""


class HTTPStatusError(HTTPError):
    """A bad status request from the SSE server."""


class HTTPWrongContentType(HTTPError):
    """Wrong content type from the SSE server."""


class SSEClient:
    """A client for reading an event stream from ETOS."""

    logger = logging.getLogger(__name__)
    __connected = False
    __shutdown = False

    def __init__(self, etos: ETOS) -> None:
        """Set up a connection pool and retry strategy."""
        self.last_event_id = None
        self.__pool = PoolManager(retries=RETRIES)
        self.__sse_server = f"{etos.cluster}/sse/v1"
        self.__id = etos.response.tercc
        self.__release: Optional[Callable] = None

    def __connect(self, retry_not_found=False) -> Iterable[bytes]:
        """Connect to an event-stream server, retrying if necessary.

        Sets the attribute `__release` which must be closed before exiting.
        """
        headers = {
            "Cache-Control": "no-cache",
            "Accept": "text/event-stream",
        }
        if self.last_event_id:
            headers["Last-Event-ID"] = self.last_event_id
        try:
            retries = RETRIES
            if retry_not_found:
                retries = retries.new(status_forcelist={413, 429, 503, 404})
            response = self.__pool.request(
                "GET",
                f"{self.__sse_server}/events/{self.__id}",
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
            raise HTTPStatusError(f"Error code {response.status} when connecting to SSE server")
        if response.status == 204:
            raise NoResponse("No response from the SSE server")

        content_type = response.headers.get("Content-Type", None)
        if content_type is None:
            raise HTTPWrongContentType("Server did not respond with a content type")
        if not content_type.startswith("text/event-stream"):
            raise HTTPWrongContentType(f"Bad content type from server: {content_type!r}")
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

    def __line_buffered(self, chunks: Iterable[bytes]) -> list[bytes]:
        """Read chunks from a byte feed and split each chunk on line-break."""
        for chunk in chunks:
            if len(chunk) == 0:
                continue
            yield from chunk.splitlines(keepends=True)

    def __read(self, chunks: Iterable[bytes]) -> str:
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
            if isinstance(event, Shutdown):
                raise ServerShutdown

            if event.id is None:
                yield event
                continue
            if event.id == self.last_event_id:
                continue  # Ignore if already received
            if self.__out_of_sync(event):
                raise Desynced
            self.last_event_id = event.id
            yield event
            time.sleep(0.001)

    def __parse_event(self, feed: str) -> Event:
        """Parse an SSE event string into an ETOS event."""
        event = {}
        for line in feed.splitlines():
            _type, value = line.split(":", 1)
            event.setdefault(_type, "")
            event[_type] += value.strip()
        return parse(event)

    def __out_of_sync(self, event: Event) -> bool:
        """Check if the eventstream is out of sync."""
        if not self.last_event_id:
            return False
        if event.id - self.last_event_id == 1:
            return False
        return True

    def event_stream(self) -> Iterable[Event]:
        """Follow the ETOS SSE event stream."""
        stream = None
        while not self.__shutdown:
            while self.__connected is False:
                self.logger.info("Connecting to SSE server")
                stream = self.__connect(retry_not_found=stream is None)
                time.sleep(1)
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
