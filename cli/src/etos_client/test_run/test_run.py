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
"""ETOS client testrun module."""
import os
import time
import traceback

from json import JSONDecodeError
from urllib3.exceptions import MaxRetryError, NewConnectionError
import requests
from requests.exceptions import HTTPError
from etos_lib import ETOS as ETOSLibrary

from etos_client.lib.test_result_handler import ETOSTestResultHandler
from etos_client.etos.schema import ResponseSchema


class State:  # pylint:disable=too-few-public-methods
    """ETOS Testrun states."""

    NOT_STARTED = 0
    STARTED = 1
    SUCCESS = 2
    FAILURE = 3
    CANCELED = 4


class TestRun:
    """An ETOS test run representation and handler."""

    state = State.NOT_STARTED

    def __init__(self, cluster: str, spinner: "etos_client.Printer"):
        """Initialize the test run handler."""
        self.etos_library = ETOSLibrary(
            "ETOS Client", os.getenv("HOSTNAME"), "ETOS Client"
        )
        self.cluster = cluster
        self.__test_result_handler = None
        self.__results = None
        self.logger = spinner

    def run(self, request_data: "etos_client.etos.schema.RequestSchema") -> int:
        """Run ETOS and wait for it to finish."""
        if not self.check_connection():
            self.state = State.CANCELED
            self.__results = "Unable to connect to ETOS. Please check your connection."
            return self.state
        self.logger.succeed("Connection successful.")
        self.logger.succeed("Ready to launch ETOS.")

        self.logger.start("Triggering ETOS.")
        if not self.__start(request_data):
            self.state = State.CANCELED
            self.__results = "Failed to start ETOS"
            return self.state
        self.logger.start("Waiting for ETOS.")
        return self.__wait()

    def check_connection(self):
        """Check connection to ETOS."""
        try:
            response = requests.get(f"{self.cluster}/selftest/ping", timeout=5)
            response.raise_for_status()
            return True
        except Exception:  # pylint:disable=broad-exception-caught
            return False

    def __wait(self) -> int:
        """Wait for test run to finish.

        Test result handling shall be moved from this method.
        """
        self.__test_result_handler = ETOSTestResultHandler(self.etos_library)
        (
            success,
            results,
            canceled,
        ) = self.__test_result_handler.wait_for_test_suite_finished(self.logger)
        if not success:
            if canceled:
                self.state = State.CANCELED
                self.__results = canceled
            else:
                self.state = State.FAILURE
                self.__results = results
        else:
            self.state = State.SUCCESS
            self.__results = results
        return self.state

    def __start(self, request_data: "etos_client.etos.schema.RequestSchema") -> bool:
        """Start an ETOS testrun.

        Initializing an ETOSClient here feels weird. Should be passed into
        this test run handler instead.
        """
        response = self.__retry_trigger_etos(request_data)
        if not response:
            self.state = State.FAILURE
            return False
        self.state = State.STARTED
        self.logger.info(f"Suite ID: {response.tercc}")
        self.logger.info(f"Artifact ID: {response.artifact_id}")
        self.logger.info(f"Purl: {response.artifact_identity}")

        # TODO: Let's not access etos-library here
        self.etos_library.config.set("suite_id", str(response.tercc))
        os.environ["ETOS_GRAPHQL_SERVER"] = response.event_repository
        self.logger.info(f"Event repository: {response.event_repository!r}")
        return True

    def __retry_trigger_etos(
        self, request_data: "etos_client.etos.schema.RequestSchema"
    ) -> ResponseSchema:
        """Trigger ETOS, retrying on non-client errors until successful."""
        end_time = time.time() + 30
        response = None
        while time.time() < end_time:
            try:
                response = self.etos_library.http.request(
                    "POST", f"{self.cluster}/etos", json=request_data.dict()
                )
                break
            except HTTPError as http_error:
                response = http_error.response
                if 400 <= response.status_code < 500:
                    try:
                        response_json = response.json()
                    except JSONDecodeError:
                        self.logger.info(f"Raw response from ETOS: {response.text!r}")
                        response_json = {"detail": "Unknown client error from ETOS"}
                    # TODO:!
                    self.logger.fail(response_json.get("detail"))
                    return None
                traceback.print_exc()
                time.sleep(2)
            except (
                ConnectionError,
                NewConnectionError,
                MaxRetryError,
                TimeoutError,
            ):
                traceback.print_exc()
                time.sleep(2)
        else:
            self.logger.fail("Failed to trigger ETOS.")
            return None
        if response is not None:
            self.logger.succeed("ETOS triggered.")
            return ResponseSchema.from_response(response)
        self.logger.fail("Failed to trigger ETOS.")
        return None

    def events(self):
        """Events that were sent in this testrun.

        Events shall not be handled in this handler.
        """
        return self.__test_result_handler.events

    def result(self):
        """Test run result.

        Results to be handled in another way later.
        """
        return self.__results
