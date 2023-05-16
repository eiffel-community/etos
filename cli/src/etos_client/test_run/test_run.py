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
import logging

from etos_client.etos.schema import RequestSchema, ResponseSchema
from etos_client.etos import ETOS
from etos_client.test_results import TestResults


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
    data: ResponseSchema = None
    logger = logging.getLogger(__name__)

    def __init__(self):
        """Initialize the test run handler."""
        self.__results = None
        self.__events = None
        # TODO: This is silly
        self.test_results = TestResults()

    def run(self, etos: ETOS, request_data: RequestSchema) -> int:
        """Run ETOS and wait for it to finish."""
        response = etos.start(request_data)
        if not response:
            self.state = State.CANCELED
            self.__results = etos.reason
            return self.state

        self.data = response

        self.logger.info("Suite ID: %s", response.tercc)
        self.logger.info("Artifact ID: %s", response.artifact_id)
        self.logger.info("Purl: %s", response.artifact_identity)
        os.environ["ETOS_GRAPHQL_SERVER"] = response.event_repository
        self.logger.info("Event repository: %r", response.event_repository)

        self.logger.info("Waiting for ETOS.")
        return self.__wait()

    def __wait(self) -> int:
        """Wait for test run to finish.

        Test result handling shall be moved from this method.
        """
        success, results, canceled = self.test_results.get_results(self)
        if success:
            self.state = State.SUCCESS
            self.__results = results
            self.__events = self.test_results.events
        elif canceled:
            self.state = State.CANCELED
            self.__results = canceled
        else:
            self.state = State.FAILURE
            self.__results = results
        return self.state

    def events(self):
        """Events that were sent in this testrun.

        Events shall not be handled in this handler.
        """
        return self.__events

    def result(self):
        """Test run result.

        Results to be handled in another way later.
        """
        return self.__results
