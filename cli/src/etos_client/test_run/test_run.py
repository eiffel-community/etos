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

from etos_client.announcer import Announcer
from etos_client.downloader import Downloader
from etos_client.etos import ETOS
from etos_client.etos.schema import ResponseSchema
from etos_client.events.collector import Collector
from etos_client.events.events import Events
from etos_client.sse.client import SSEClient
from etos_client.sse.protocol import Message, Ping

MINUTE = 60


class TestRun:
    """Track an ETOS test run and log relevant information."""

    logger = logging.getLogger(__name__)
    remote_logger = logging.getLogger("ETOS")
    __last_log: int
    # This log interval is not a guarantee as it depends on several
    # factors, such as the SSE log listener which has its own Ping
    # interval.
    log_interval = 30

    def __init__(self, collector: Collector, downloader: Downloader) -> None:
        """Initialize."""
        assert downloader.started, "Downloader must be started before it can be used in TestRun"

        self.__collector = collector
        self.__downloader = downloader
        self.__announcer = Announcer()

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
        self.__last_log = time.time()
        timer = None
        for _ in self.__log_until_eof(etos, end):
            events = self.__collect(etos)
            self.__status(events)
            self.__announce()
            self.__download(events)
            if events.activity.finished and timer is None:
                timer = time.time() + 300  # 5 minutes
            if timer and time.time() >= timer:
                self.logger.warning("ETOS finished, but did not shut down the log server.")
                self.logger.warning(
                    "Forcing a shut down to avoid hanging. Some data may be incorrect, "
                    "such as the number of tests executed or the number of downloaded logs"
                )
                break
        self.__wait(etos, end)
        events = self.__collect(etos)
        self.__download(events)
        self.__announce(force=True)
        return events

    def __wait(self, etos: ETOS, timeout: int) -> None:
        """Wait for ETOS to finish its activity."""
        events = self.__collect(etos)
        while not events.activity.finished:
            self.logger.info("Waiting for ETOS to finish")
            self.__status(events)
            time.sleep(5)
            if time.time() >= timeout:
                raise TimeoutError("ETOS did not complete in 24 hours. Exiting")
            events = self.__collect(etos)

    def __status(self, events: Events) -> None:
        """Check if ETOS test run has been canceled."""
        if events.activity.canceled:
            raise SystemExit(events.activity.canceled["data"]["reason"])

    def __announce(self, force: bool = False) -> None:
        """Announce current state of ETOS."""
        if force or self.__last_log + self.log_interval <= time.time():
            events = self.__collector.collect_test_case_events()
            self.__announcer.announce(events, self.__downloader.downloads)
            self.__last_log = time.time()

    def __download(self, events: Events) -> None:
        """Download logs and artifacts."""
        if events.main_suites:
            self.__downloader.download_logs(events.main_suites)
        self.__downloader.download_artifacts(events.artifacts)

    def __collect(self, etos: ETOS) -> Events:
        """Collect events from ETOS."""
        return self.__collector.collect(etos.response.tercc)

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
        self.__last_log = time.time()

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
