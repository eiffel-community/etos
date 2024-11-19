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
"""ETOS v1alpha."""
import os
import logging
import shutil
from pathlib import Path
from json import JSONDecodeError
from typing import Optional, Type

from requests.exceptions import HTTPError
from etos_lib.lib.http import Http
from urllib3.util import Retry
from etos_lib import ETOS as ETOSLibrary

from etos_client.types.stream import Stream
from etos_client.types.result import Result, Verdict, Conclusion
from etos_client.shared.downloader import Downloader
from etos_client.shared.utilities import directories
from etos_client.sse.v1.client import SSEClient

from etos_client.etos.v0.events.collector import Collector
from etos_client.etos.v0.test_results import TestResults
from etos_client.etos.v0.event_repository import graphql
from etos_client.etos.v0.test_run import TestRun
from etos_client.etos.v0.schema.response import ResponseSchema
from etos_client.etos.v0.schema.request import RequestSchema


# Max total time for a ping request including delays with backoff factor 0.5 will be:
# 0.5 + 1.5 + 3.5 + 7.5 + 15.5 = 28.5 (seconds)
HTTP_RETRY_PARAMETERS = Retry(
    total=5,  # limit the total number of retries only (regardless of error type)
    connect=None,
    read=None,
    status_forcelist=[500, 502, 503, 504],  # Common temporary error status codes
    backoff_factor=0.5,
    raise_on_redirect=True,  # Raise an exception if too many redirects
)


class Etos:
    """Handle communication with ETOS v1alpha.

    TODO: At the moment this version mostly re-uses the v0 ETOS client because
    SSE v2 is not yet finished and we have no SSE protocol ready to support all
    cases where we use Eiffel today.
    """

    version = "v1alpha"
    logger = logging.getLogger(__name__)
    start_response = ResponseSchema
    start_request = RequestSchema
    supported_sse_versions = ("v1",)  # TODO: Add support for "v2alpha"

    def __init__(self, args: dict, streamer: Type[Stream] = SSEClient):
        """Set up streamer and cluster variables."""
        assert streamer.version() in self.supported_sse_versions, (
            f"Version {streamer.version()} is not supported by this version of ETOS client, "
            "supported versions: {self.supported_sse_versions}"
        )
        self.args = args
        self.cluster = args.get("<cluster>")
        assert self.cluster is not None
        self.streamer = streamer
        self.logger.info("Running ETOS version %s", self.version)

    def run(self) -> Result:
        """Run ETOS v1alpha."""
        error = self.__check()
        if error is not None:
            return Result(verdict=Verdict.INCONCLUSIVE, conclusion=Conclusion.FAILED, reason=error)
        response, error = self.__start()
        if error is not None:
            return Result(verdict=Verdict.INCONCLUSIVE, conclusion=Conclusion.FAILED, reason=error)
        assert response is not None
        (success, msg), error = self.__wait(response)
        if error is not None:
            return Result(verdict=Verdict.INCONCLUSIVE, conclusion=Conclusion.FAILED, reason=error)
        if success is None or msg is None:
            return Result(
                verdict=Verdict.INCONCLUSIVE,
                conclusion=Conclusion.FAILED,
                reason="No test result received from ETOS testrun",
            )
        return Result(
            verdict=Verdict.PASSED if success else Verdict.FAILED,
            conclusion=Conclusion.SUCCESSFUL,
            reason=msg,
        )

    def __start(self) -> tuple[Optional[ResponseSchema], Optional[str]]:
        """Trigger ETOS, retrying on non-client errors until successful or timeout."""
        request = self.start_request.from_args(self.args)
        url = f"{self.cluster}/api/{self.version}/testrun"
        self.logger.info("Triggering ETOS using %r", url)

        response_json = {}
        http = Http(retry=HTTP_RETRY_PARAMETERS, timeout=10)
        response = http.post(url, json=request.model_dump())
        try:
            response.raise_for_status()
            response_json = response.json()
        except HTTPError:
            self.logger.error("Failed to start ETOS.")
            try:
                response_json = response.json()
            except JSONDecodeError:
                self.logger.info("Raw response from ETOS: %r", response.text)
                response_json = {}
            return None, response_json.get(
                "detail", "Unknown error from ETOS, please contact ETOS support"
            )
        return self.start_response.from_response(response_json), None

    def __wait(
        self, response: ResponseSchema
    ) -> tuple[tuple[Optional[bool], Optional[str]], Optional[str]]:
        """Wait for ETOS to finish."""
        etos_library = ETOSLibrary("ETOS Client", os.getenv("HOSTNAME"), "ETOS Client")
        os.environ["ETOS_GRAPHQL_SERVER"] = response.event_repository

        report_dir, artifact_dir = directories(self.args)

        collector = Collector(etos_library, graphql)
        log_downloader = Downloader()
        clear_queue = True
        log_downloader.start()
        try:
            test_run = TestRun(collector, log_downloader, report_dir, artifact_dir)
            test_run.setup_logging(self.args["-v"])
            events = test_run.track(
                self.streamer(self.args["<cluster>"], str(response.tercc)),
                response,
                24 * 60 * 60,  # 24 hours
            )
        except SystemExit as exit:
            clear_queue = False
            return (False, None), str(exit)
        finally:
            log_downloader.stop(clear_queue)
            log_downloader.join()

        self.logger.info(
            "Downloaded a total of %d logs from test runners", len(log_downloader.downloads)
        )
        self.logger.info("Archiving reports.")
        shutil.make_archive(
            str(artifact_dir.joinpath("reports").relative_to(Path.cwd())), "zip", report_dir
        )
        self.logger.info("Reports: %s", report_dir)
        self.logger.info("Artifacts: %s", artifact_dir)

        if log_downloader.failed:
            return (False, None), "ETOS logs did not download successfully"
        return TestResults().get_results(events), None

    def __check(self) -> Optional[str]:
        """Check connection to ETOS."""
        url = f"{self.cluster}/api/selftest/ping"
        self.logger.info("Checking connection to ETOS at %r.", url)
        http = Http(retry=HTTP_RETRY_PARAMETERS, timeout=5)
        response = http.get(url)
        try:
            response.raise_for_status()
        except HTTPError:
            return "Connection failed, please check your connection or contact ETOS support"
        self.logger.info("Connection successful.")
        return None
