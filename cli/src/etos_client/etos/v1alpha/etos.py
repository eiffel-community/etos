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

import logging
import os
import shutil
import time
from json import JSONDecodeError
from pathlib import Path
from typing import Optional, Union

from etos_lib import ETOS as ETOSLibrary
from etos_lib.lib.http import Http
from requests.exceptions import HTTPError
from urllib3.util import Retry

from etos_client.etos.v0.event_repository import graphql
from etos_client.etos.v0.events.collector import Collector
from etos_client.etos.v0.test_results import TestResults
from etos_client.etos.v0.test_run import TestRun as V0TestRun
from etos_client.etos.v1alpha.schema.request import RequestSchema
from etos_client.etos.v1alpha.schema.response import ResponseSchema
from etos_client.etos.v1alpha.test_run import TestRun as V1AlphaTestRun
from etos_client.shared.downloader import Downloader
from etos_client.shared.utilities import directories
from etos_client.sse.v1.client import SSEClient as SSEV1Client
from etos_client.sse.v2alpha.client import SSEClient as SSEV2AlphaClient
from etos_client.sse.v2alpha.client import TokenExpired
from etos_client.types.result import Conclusion, Result, Verdict

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
    __apikey = None
    start_response = ResponseSchema
    start_request = RequestSchema

    def __init__(self, args: dict, sse_client: Union[SSEV1Client, SSEV2AlphaClient]):
        """Set up sse client and cluster variables."""
        self.args = args
        self.cluster = args.get("<cluster>")
        assert self.cluster is not None
        self.sse_client = sse_client
        self.logger.info("Running ETOS version %s", self.version)

    @property
    def apikey(self) -> str:
        """Generate and return an API key."""
        http = Http(retry=HTTP_RETRY_PARAMETERS, timeout=10)
        if self.__apikey is None:
            url = f"{self.cluster}/keys/v1alpha/generate"
            response = http.post(
                url,
                json={"identity": "etos-client", "scope": "post-testrun delete-testrun get-sse"},
            )
            try:
                response.raise_for_status()
                response_json = response.json()
            except HTTPError:
                self.logger.exception("Failed to generate an API key for ETOS.")
                response_json = {}
            self.__apikey = response_json.get("token")
        return self.__apikey or ""

    def run(self) -> Result:
        """Run ETOS v1alpha."""
        error = self.__check()
        if error is not None:
            return Result(verdict=Verdict.INCONCLUSIVE, conclusion=Conclusion.FAILED, reason=error)
        response, error = self.__start()
        if error is not None:
            return Result(verdict=Verdict.INCONCLUSIVE, conclusion=Conclusion.FAILED, reason=error)
        assert response is not None
        return self.__wait(response)

    def __start(self) -> tuple[Optional[ResponseSchema], Optional[str]]:
        """Trigger ETOS, retrying on non-client errors until successful or timeout."""
        request = self.start_request.from_args(self.args)
        url = f"{self.cluster}/api/{self.version}/testrun"
        self.logger.info("Triggering ETOS using %r", url)

        response_json = {}
        http = Http(retry=HTTP_RETRY_PARAMETERS, timeout=10)
        if isinstance(self.sse_client, SSEV2AlphaClient):
            response = http.post(
                url, json=request.model_dump(), headers={"Authorization": f"Bearer {self.apikey}"}
            )
        else:
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

    def __wait(self, response: ResponseSchema) -> Result:
        """Wait for ETOS to finish."""
        report_dir, artifact_dir = directories(self.args)

        log_downloader = Downloader()
        clear_queue = True
        log_downloader.start()

        end = time.time() + 24 * 60 * 60  # 24 hours

        if isinstance(self.sse_client, SSEV2AlphaClient):
            test_run = V1AlphaTestRun(log_downloader, report_dir, artifact_dir)
        else:
            etos_library = ETOSLibrary("ETOS Client", os.getenv("HOSTNAME"), "ETOS Client")
            os.environ["ETOS_GRAPHQL_SERVER"] = response.event_repository
            collector = Collector(etos_library, graphql)
            test_run = V0TestRun(collector, log_downloader, report_dir, artifact_dir)
        test_run.setup_logging(self.args["-v"])
        result = None
        try:
            while time.time() < end:
                try:
                    if isinstance(self.sse_client, SSEV2AlphaClient):
                        result = self.__track(test_run, response, end)
                    else:
                        result = self.__track_v0(test_run, response, end)
                    break
                except TokenExpired:
                    self.__apikey = None
                    continue
                except SystemExit as exit:
                    clear_queue = False
                    result = Result(
                        verdict=Verdict.INCONCLUSIVE, conclusion=Conclusion.FAILED, reason=str(exit)
                    )
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
            return Result(
                verdict=Verdict.INCONCLUSIVE,
                conclusion=Conclusion.FAILED,
                reason="ETOS logs did not download succesfully",
            )
        if result is not None:
            return result
        return Result(
            verdict=Verdict.INCONCLUSIVE,
            conclusion=Conclusion.INCONCLUSIVE,
            reason="Got no result from ETOS so could not determine test result.",
        )

    def __track(self, test_run: V1AlphaTestRun, response: ResponseSchema, end: float) -> Result:
        """Track a testrun."""
        shutdown = test_run.track(
            self.sse_client,
            self.apikey,
            response,
            end,
        )
        return Result(
            verdict=Verdict(shutdown.data.verdict.upper()),
            conclusion=Conclusion(shutdown.data.conclusion.upper()),
            reason=shutdown.data.description,
        )

    def __track_v0(self, test_run: V0TestRun, response: ResponseSchema, end: float) -> Result:
        """Track a testrun using the v0 testrun handler."""
        events = test_run.track(
            self.sse_client,
            response,
            end,
        )
        success, msg = TestResults().get_results(events)
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
