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
"""ETOS test result handler."""
import logging
from typing import Optional

from etos_client.events.events import Events, SubSuite, TestSuite

# pylint:disable=too-few-public-methods


class TestResults:
    """Get test results from an ETOS testrun."""

    logger = logging.getLogger(__name__)

    def __has_failed(self, test_suites: list[TestSuite]) -> bool:
        """Check if any test suite has failed in a list of test suites."""
        for test_suite in test_suites:
            outcome = test_suite.finished["data"]["testSuiteOutcome"]
            if outcome["verdict"] != "PASSED":
                return True
        return False

    def __count_sub_suite_failures(self, test_suites: list[SubSuite]) -> int:
        """Count the number of sub suite failures in a list of sub suites."""
        failures = 0
        for sub_suite in test_suites:
            outcome = sub_suite.finished["data"]["testSuiteOutcome"]
            if outcome["verdict"] != "PASSED":
                failures += 1
        return failures

    def __fail_messages(self, test_suites: list[TestSuite]) -> list[str]:
        """Build a fail message from main suites errors."""
        messages = []
        for test_suite in test_suites:
            outcome = test_suite.finished["data"]["testSuiteOutcome"]
            if outcome["conclusion"] != "SUCCESSFUL":
                messages.append(f'{test_suite.started["data"]["name"]}: {outcome["description"]}')
        return messages

    def __test_result(self, test_suites: list[TestSuite]) -> tuple[bool, str]:
        """Build test results based on events retrieved."""
        if not self.__has_failed(test_suites):
            return True, "Test suite finished successfully."

        failures = 0
        sub_suites = 0
        for test_suite in test_suites:
            failures += self.__count_sub_suite_failures(test_suite.sub_suites)
            sub_suites += len(test_suite.sub_suites)
        messages = self.__fail_messages(test_suites)
        if len(messages) == 1:
            return False, messages[0]
        if messages:
            for message in messages[:-1]:
                self.logger.error(message)
            return False, messages[-1]
        if sub_suites == 0:
            return False, "ETOS failed to start any test suites"
        return False, f"{failures}/{sub_suites} test suites failed."

    def get_results(self, events: Events) -> tuple[Optional[bool], Optional[str]]:
        """Get results from an ETOS testrun."""
        if not events.tercc:
            return None, None
        if not events.activity.triggered:
            return None, None
        if events.activity.canceled:
            return None, None
        if not events.main_suites:
            return None, None
        for test_suite in events.main_suites:
            if not test_suite.finished:
                return None, None
        return self.__test_result(events.main_suites)
