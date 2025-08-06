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
from typing import Union
from pathlib import Path

#from etos_lib.messaging.events import Message, Report, Artifact, Shutdown  # import disabled due to: https://github.com/eiffel-community/etos/issues/417
from etos_client.sse.v2alpha.client import SSEClient
from etos_client.shared.downloader import Downloader, Downloadable
from ..schema.response import ResponseSchema

# dummy classes: remove when the etos_lib.messaging module is available: https://github.com/eiffel-community/etos/issues/417
class Artifact:
    pass

class Message:
    pass

class Report:
    pass

class Shutdown:
    pass

class TestRun:
    """Track an ETOS test run and log relevant information."""

    logger = logging.getLogger(__name__)
    remote_logger = logging.getLogger("ETOS")

    # The log interval is not a guarantee as it depends on several
    # factors, such as the SSE log listener which has its own Ping
    # interval.
    log_interval = 120

    def __init__(self, downloader: Downloader, report_dir: Path, artifact_dir: Path) -> None:
        """Initialize."""
        assert downloader.started, "Downloader must be started before it can be used in TestRun"

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

    def track(
        self, sse_client: SSEClient, apikey: str, response: ResponseSchema, end_time: float
    ) -> Shutdown:
        """Track, and wait for, an ETOS test run."""
        self.__log_debug_information(response)
        try:
            shutdown = self.stream(sse_client, str(response.tercc), apikey, end_time)
        finally:
            sse_client.close()
        return shutdown

    def stream(
        self, sse_client: SSEClient, stream_id: str, apikey: str, end_time: float
    ) -> Shutdown:
        """Stream SSE."""
        for event in sse_client.event_stream(stream_id, apikey):
            if time.time() >= end_time:
                raise TimeoutError("Timed out!")
            if isinstance(event, Shutdown):
                return event
            elif isinstance(event, Message):
                self.__log(event)
            elif isinstance(event, (Report, Artifact)):
                self.download(event)
        return Shutdown.model_validate({
            "conclusion": "FAILED",
            "verdict": "INCONCLUSIVE",
            "description": "Event stream died",
        })

    def download_report(self, report: Report):
        """Download a report to the report directory."""
        reports = self.__report_dir.relative_to(Path.cwd()).joinpath(
            report.data.directory or "",
        )
        self.__downloader.queue_download(
            Downloadable(
                url=report.data.url,
                name=report.data.name,
                checksums=report.data.checksums,
                path=reports,
            )
        )

    def download_artifact(self, artifact: Artifact):
        """Download an artifact to the artifact directory."""
        artifacts = self.__artifact_dir.relative_to(Path.cwd()).joinpath(
            artifact.data.directory or ""
        )
        self.__downloader.queue_download(
            Downloadable(
                url=artifact.data.url,
                name=artifact.data.name,
                checksums=artifact.data.checksums,
                path=artifacts,
            )
        )

    def download(self, file: Union[Report, Artifact]):
        """Download a file from either a Report or Artifact event."""
        if isinstance(file, Report):
            self.download_report(file)
        elif isinstance(file, Artifact):
            self.download_artifact(file)

    def __log(self, message: Message) -> None:
        """Log a message from the ETOS log API."""
        logger = getattr(self.remote_logger, message.data.level)
        logger(
            message,
            extra={
                "rname": message.data.name,
                "rtime": message.data.datestring.strftime("%Y-%m-%d %H:%M:%S"),
            },
        )
