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
"""Announcer module."""
import logging
from etos_client.events.events import Events


class Announcer:  # pylint:disable=too-few-public-methods
    """Announce the state of ETOS."""

    logger = logging.getLogger(__name__)

    def __failed(self, test_cases_finished: list[dict]) -> int:
        """Failed test case count."""
        failed = 0
        for test_case in test_cases_finished:
            if test_case["data"]["testCaseOutcome"]["verdict"] != "PASSED":
                failed += 1
        return failed

    def __successful(self, test_cases_finished: list[dict]) -> int:
        """Successful test case count."""
        successful = 0
        for test_case in test_cases_finished:
            if test_case["data"]["testCaseOutcome"]["verdict"] == "PASSED":
                successful += 1
        return successful

    def __build_announcement(self, events: Events) -> str:
        """Build an announcement based on executed test cases and results."""
        nbr_of_executed = len(events.test_cases)
        if nbr_of_executed == 0:
            return "Still waiting for the first test cases to execute"

        message = f"Number of test cases executed: {nbr_of_executed}"

        detailed = []

        finished = [test_case.finished for test_case in events.test_cases if test_case.finished]

        nbr_of_successful = self.__successful(finished)
        if nbr_of_successful > 0:
            detailed.append(f"passed={nbr_of_successful}")
        nbr_of_failed = self.__failed(finished)
        if nbr_of_failed > 0:
            detailed.append(f"failed={nbr_of_failed}")
        nbr_of_canceled = len(
            [test_case.canceled for test_case in events.test_cases if test_case.canceled]
        )
        if nbr_of_canceled > 0:
            detailed.append(f"skipped={nbr_of_canceled}")

        if detailed:
            message += f" ({', '.join(detailed)})"
        return message

    def announce(self, events: Events, logs: set) -> None:
        """Announce the ETOS state."""
        if not events.tercc:
            self.logger.info("Waiting for ETOS to start.")
            return
        if not events.activity.triggered:
            self.logger.info("Waiting for ETOS to start.")
            return
        if not events.main_suites:
            self.logger.info("Waiting for main suites to start.")
            return
        sub_suites_started = False
        for main_suite in events.main_suites:
            if main_suite.sub_suites:
                sub_suites_started = True
        if not sub_suites_started:
            self.logger.info("Waiting for sub suites to start.")
            return

        if logs:
            self.logger.info("Downloaded a total of %d logs from test runners", len(logs))
