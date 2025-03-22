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
from uuid import UUID
from typing import Iterator, Union
from pathlib import Path

from etos_client.sse.v1.protocol import Message, Report, Artifact
from etos_client.sse.v1.client import SSEClient
from etos_client.shared.events import Event
from etos_client.shared.downloader import Downloader, Downloadable
from ..schema.response import ResponseSchema
from ..events.collector import Collector
from ..events.events import Events


class TestRun:
    """Track an ETOS test run and log relevant information."""

    logger = logging.getLogger(__name__)
    remote_logger = logging.getLogger("ETOS")

    # The log interval is not a guarantee as it depends on several
    # factors, such as the SSE log listener which has its own Ping
    # interval.
    log_interval = 120

    def __init__(
        self, collector: Collector, downloader: Downloader, report_dir: Path, artifact_dir: Path
    ) -> None:
        """Initialize."""
        assert downloader.started, "Downloader must be started before it can be used in TestRun"

        self.__collector = collector
        self.__downloader = downloader
        self.__report_dir = report_dir
        self.__artifact_dir = artifact_dir

    def setup_logging(self, verbosity: int) -> None:
        """Set up logging for ETOS remote logs."""
        if verbosity == 1:
            loglevel = logging.INFO
        elif verbosity == 2:
            loglevel = logging.DEBUG
        else:
            loglevel = logging.WARNING
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

    def track(self, sse_client: SSEClient, response: ResponseSchema, end: float) -> Events:
        """Track, and wait for, an ETOS test run."""

        self.__log_debug_information(response)
        last_log = time.time()
        timer = None
        for event in self.__log_until_eof(sse_client, str(response.tercc), end):
            if isinstance(event, (Report, Artifact)):
                self.download(event)
            if last_log + self.log_interval >= time.time():
                events = self.__collector.collect_activity(response.tercc)
                self.__status(events)
                self.__announce(events)
                last_log = time.time()
                if events.activity.finished and timer is None:
                    timer = time.time() + 300  # 5 minutes
            if timer and time.time() >= timer:
                self.logger.warning("ETOS finished, but did not shut down the log server.")
                self.logger.warning(
                    "Forcing a shut down to avoid hanging. Some data may be incorrect, "
                    "such as the number of tests executed or the number of downloaded logs"
                )
                break
        self.__wait(response.tercc, end)
        events = self.__collector.collect(response.tercc)
        self.__announce(events)
        return events

    def download_report(self, report: Report):
        """Download a report to the report directory."""
        reports = self.__report_dir.relative_to(Path.cwd()).joinpath(
            report.file.get("directory", "")
        )
        self.__downloader.queue_download(
            Downloadable(
                url=report.file.get("url"),
                name=report.file.get("name"),
                checksums=report.file.get("checksums", {}),
                path=reports,
            )
        )

    def download_artifact(self, artifact: Artifact):
        """Download an artifact to the artifact directory."""
        artifacts = self.__artifact_dir.relative_to(Path.cwd()).joinpath(
            artifact.file.get("directory", "")
        )
        self.__downloader.queue_download(
            Downloadable(
                url=artifact.file.get("url"),
                name=artifact.file.get("name"),
                checksums=artifact.file.get("checksums", {}),
                path=artifacts,
            )
        )

    def download(self, file: Union[Report, Artifact]):
        """Download a file from either a Report or Artifact event."""
        if isinstance(file, Report):
            self.download_report(file)
        elif isinstance(file, Artifact):
            self.download_artifact(file)

    def __wait(self, tercc: UUID, timeout: float) -> None:
        """Wait for ETOS to finish its activity."""
        events = self.__collector.collect_activity(tercc)
        while not events.activity.finished:
            self.logger.info("Waiting for ETOS to finish")
            self.__status(events)
            time.sleep(5)
            if time.time() >= timeout:
                raise TimeoutError("ETOS did not complete in 24 hours. Exiting")
            events = self.__collector.collect_activity(tercc)

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

    def __log_until_eof(
        self, sse_client: SSEClient, stream_id: str, endtime: float
    ) -> Iterator[Event]:
        """Log from the ETOS log API until finished."""
        for event in sse_client.event_stream(stream_id):
            if time.time() >= endtime:
                raise TimeoutError("Timed out!")
            if isinstance(event, Message):
                self.__log(event)
                continue
            yield event
