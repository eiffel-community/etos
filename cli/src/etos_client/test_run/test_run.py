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
"""ETOS test run handler."""
import logging
import sys
import time
from typing import Iterator

from urllib3.util import Retry
from etos_lib.lib.http import Http
from requests.exceptions import HTTPError

from etos_client.downloader import Downloader
from etos_client.etos import ETOS
from etos_client.etos.schema import ResponseSchema
from etos_client.events.collector import Collector
from etos_client.events.events import Events
from etos_client.sse.client import SSEClient
from etos_client.sse.protocol import Message, Ping


HTTP_RETRY_PARAMETERS = Retry(
    total=None,
    read=0,
    connect=2,
    status=2,
    backoff_factor=1,
    other=0,
    status_forcelist=list(Retry.RETRY_AFTER_STATUS_CODES),
)


class TestRun:
    """Track an ETOS test run and log relevant information."""

    logger = logging.getLogger(__name__)
    remote_logger = logging.getLogger("ETOS")

    # The log interval is not a guarantee as it depends on several
    # factors, such as the SSE log listener which has its own Ping
    # interval.
    log_interval = 120

    def __init__(self, collector: Collector, downloader: Downloader) -> None:
        """Initialize."""
        assert downloader.started, "Downloader must be started before it can be used in TestRun"

        self.__http = Http(retry=HTTP_RETRY_PARAMETERS)
        self.__collector = collector
        self.__downloader = downloader

    def setup_logging(self, loglevel: int) -> None:
        """Set up logging for ETOS remote logs."""
        rhandler = logging.StreamHandler(sys.stdout)
        formatter = logging.Formatter(
            fmt="[%(rtime)s] %(levelname)s:%(rname)s: %(message)s",
            datefmt="%Y-%m-%d %H:%M:%S",
        )
        rhandler.setFormatter(formatter)
        rhandler.setLevel(loglevel)
        self.remote_logger.propagate = False
        self.remote_logger.addHandler(rhandler)

    def __log_debug_information(self, response: ResponseSchema) -> None:
        """Log debug information from the ETOS API response."""
        self.logger.info("Suite ID: %s", response.tercc)
        self.logger.info("Artifact ID: %s", response.artifact_id)
        self.logger.info("Purl: %s", response.artifact_identity)
        self.logger.info("Event repository: %r", response.event_repository)

    def track(self, etos: ETOS, timeout: int) -> Events:
        """Track, and wait for, an ETOS test run."""
        end = time.time() + timeout

        self.__log_debug_information(etos.response)
        last_log = time.time()
        timer = None
        for _ in self.__log_until_eof(etos, end):
            if last_log + self.log_interval >= time.time():
                events = self.__collector.collect_activity(etos.response.tercc)
                self.__status(events)
                self.__announce(events)
                last_log = time.time()
                if events.activity.finished and timer is None:
                    timer = time.time() + 300  # 5 minutes
            self.__download(etos)
            if timer and time.time() >= timer:
                self.logger.warning("ETOS finished, but did not shut down the log server.")
                self.logger.warning(
                    "Forcing a shut down to avoid hanging. Some data may be incorrect, "
                    "such as the number of tests executed or the number of downloaded logs"
                )
                break
        self.__wait(etos, end)
        self.__download(etos)
        events = self.__collector.collect(etos.response.tercc)
        self.__announce(events)
        return events

    def __wait(self, etos: ETOS, timeout: int) -> None:
        """Wait for ETOS to finish its activity."""
        events = self.__collector.collect_activity(etos.response.tercc)
        while not events.activity.finished:
            self.logger.info("Waiting for ETOS to finish")
            self.__status(events)
            time.sleep(5)
            if time.time() >= timeout:
                raise TimeoutError("ETOS did not complete in 24 hours. Exiting")
            events = self.__collector.collect_activity(etos.response.tercc)

    def __status(self, events: Events) -> None:
        """Check if ETOS test run has been canceled."""
        if events.activity.canceled:
            raise SystemExit(events.activity.canceled["data"]["reason"])

    def __announce(self, events: Events) -> None:
        """Announce current state of ETOS."""
        if not events.tercc:
            self.logger.info("Waiting for ETOS to start.")
            return
        if not events.activity.triggered:
            self.logger.info("Waiting for ETOS to start.")
            return
        if self.__downloader.downloads:
            self.logger.info(
                "Downloaded a total of %d logs from test runners", len(self.__downloader.downloads)
            )

    def __download(self, etos: ETOS) -> None:
        """Download logs and artifacts."""
        response = self.__http.get(f"{etos.cluster}/logarea/v1alpha/logarea/{etos.response.tercc}")
        try:
            response.raise_for_status()
        except HTTPError as error:
            self.logger.warning("Got an HTTP error: %r when listing logs from log area", error)
            return
        directories = response.json()
        self.__downloader.download_directories(directories)

    def __log(self, message: Message) -> None:
        """Log a message from the ETOS log API."""
        logger = getattr(self.remote_logger, message.level)
        logger(
            message,
            extra={
                "rname": message.name,
                "rtime": message.datestring,
            },
        )

    def __log_until_eof(self, etos: ETOS, endtime: int) -> Iterator[Ping]:
        """Log from the ETOS log API until finished."""
        client = SSEClient(etos)
        for log_message in client.event_stream():
            if time.time() >= endtime:
                raise TimeoutError("Timed out!")
            if isinstance(log_message, Message):
                self.__log(log_message)
                continue
            yield log_message
