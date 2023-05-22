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

from etos_client.events.events import Events, Announcement, TestSuite, SubSuite

# pylint:disable=too-few-public-methods


class TestResults:
    """Get test results from an ETOS testrun."""

    logger = logging.getLogger(__name__)

    def __init__(self):
        """Init."""
        self.__already_logged = []

    # TODO: This is not test results.
    def __log_announcements(self, announcements: list[Announcement]) -> None:
        """Get latest new announcements."""
        for announcement in announcements:
            if announcement not in self.__already_logged:
                self.__already_logged.append(announcement)
                self.logger.info("%s: %s", announcement.heading, announcement.body)

    # TODO: This is not test results.
    def __text(self, test_suites: list[TestSuite]) -> str:
        """Generate text based on test results."""
        message_template = (
            "{announcement}\t"
            "Started : {started_length}\t"
            "Finished: {finished_length}\t"
        )
        try:
            announcement = self.__already_logged[-1].body
        except (KeyError, IndexError, TypeError):
            announcement = ""

        started_length = 0
        finished_length = 0
        for test_suite in test_suites:
            for sub_suite in test_suite.sub_suites:
                started_length += 1
                if sub_suite.finished:
                    finished_length += 1

        params = {
            "started_length": started_length,
            "finished_length": finished_length,
            "announcement": announcement,
        }
        return message_template.format(**params)

    def __has_failed(self, test_suites: list[TestSuite]):
        """Check if any test suite has failed in a list of test suites."""
        for test_suite in test_suites:
            outcome = test_suite.finished["data"]["testSuiteOutcome"]
            if outcome["verdict"] != "PASSED":
                return True
        return False

    def __count_sub_suite_failures(self, test_suites: list[SubSuite]):
        """Count the number of sub suite failures in a list of sub suites."""
        failures = 0
        for sub_suite in test_suites:
            outcome = sub_suite.finished["data"]["testSuiteOutcome"]
            if outcome["verdict"] != "PASSED":
                failures += 1
        return failures

    def __test_result(self, test_suites: list[TestSuite]) -> tuple[bool, str]:
        """Build test results based on events retrieved."""
        if not self.__has_failed(test_suites):
            return True, "Test suite finished successfully."

        failures = 0
        sub_suites = 0
        for test_suite in test_suites:
            failures += self.__count_sub_suite_failures(test_suite.sub_suites)
            sub_suites += len(test_suite.sub_suites)
        return False, f"{failures}/{sub_suites} test suites failed."

    def get_results(self, events: Events) -> tuple[Optional[bool], Optional[str]]:
        """Get results from an ETOS testrun."""
        if not events.tercc:
            return None, None
        if not events.activity.triggered:
            return None, None

        self.__log_announcements(events.announcements)
        if events.activity.canceled:
            return None, None
        if events.main_suites:
            started = True
        if not started:
            return None, None
        self.logger.info(self.__text(events.main_suites))
        for test_suite in events.main_suites:
            if not test_suite.finished:
                return None, None
        self.logger.info("Done")
        return self.__test_result(events.main_suites)
