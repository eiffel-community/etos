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
from etos_client.lib.test_result_handler import ETOSTestResultHandler
from etos_client.client import ETOSClient


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

    def __init__(self, etos: "etos_lib.etos.ETOS", spinner: "etos_client.Printer"):
        """Initialize the test run handler."""
        self.__test_result_handler = ETOSTestResultHandler(etos)
        self.__results = None
        self.etos = etos
        self.logger = spinner

    def run(self, cluster: str) -> int:
        """Run ETOS and wait for it to finish."""
        self.logger.start("Triggering ETOS.")
        if not self.__start(cluster):
            self.state = State.CANCELED
            self.__results = "Failed to start ETOS"
            return self.state
        self.logger.start("Waiting for ETOS.")
        return self.__wait()

    def __wait(self) -> int:
        """Wait for test run to finish.

        Test result handling shall be moved from this method.
        """
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

    def __start(self, cluster: str) -> bool:
        """Start an ETOS testrun.

        Initializing an ETOSClient here feels weird. Should be passed into
        this test run handler instead.
        """
        client = ETOSClient(self.etos, cluster)
        success = client.start(self.logger)
        if not success:
            self.state = State.FAILURE
            return False
        self.state = State.STARTED
        self.logger.info(f"Suite ID: {client.test_suite_id}")
        self.logger.info(f"Artifact ID: {client.artifact_id}")
        self.logger.info(f"Purl: {client.artifact_identity}")

        self.etos.config.set("suite_id", client.test_suite_id)
        os.environ["ETOS_GRAPHQL_SERVER"] = client.event_repository
        self.logger.info(f"Event repository: {self.etos.debug.graphql_server!r}")
        return True

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
