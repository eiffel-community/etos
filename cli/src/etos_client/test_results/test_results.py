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
import os
import time
import logging
from uuid import UUID
from typing import Optional, Iterator

from etos_lib import ETOS as ETOSLibrary

# from etos_client.test_run import TestRun
from etos_client.event_repository import (
    wait_for,
    request_activity,
    request_activity_canceled,
    request_main_test_suites_started,
    request_test_suite_finished,
    request_test_suite_started,
    request_suite,
    request_announcements,
    request_environment,
)

HOUR = 3600
MINUTE = 60

# pylint:disable=too-few-public-methods


class TestResults:  # pylint:disable=too-many-instance-attributes
    """Get test results from an ETOS testrun."""

    logger = logging.getLogger(__name__)
    has_started = False
    has_finished = False

    def __init__(self):
        self.etos_library = ETOSLibrary(
            "ETOS Client", os.getenv("HOSTNAME"), "ETOS Client"
        )
        self.events = {"announcements": []}
        self.test_suite_started = {}
        self.test_suites_finished = []
        self.main_suite_started = {}
        self.main_suites_finished = []

    def __wait_for_test_suite(self, tercc_id: UUID) -> None:
        """Wait for test suite event to have been sent."""
        self.logger.info("Waiting for test suite url")
        tercc = wait_for(self.etos_library, request_suite, MINUTE, str(tercc_id))
        if not tercc:
            raise TimeoutError(f"Test suite not available in {MINUTE}s.")
        self.logger.info("Test suite: %s", tercc["data"]["batchesUri"])

    def __wait_for_activity(self, tercc_id: UUID) -> UUID:
        """Wait for the ETOS test activity to have been sent."""
        self.logger.info("Waiting for ETOS to start.")
        activity = wait_for(self.etos_library, request_activity, MINUTE, str(tercc_id))
        if not activity:
            raise TimeoutError(f"ETOS did not start within {MINUTE}s.")
        self.logger.info("Activity ID: %r", activity["meta"]["id"])
        return UUID(activity["meta"]["id"], version=4)

    def __is_canceled(self, activity_id: UUID) -> dict:
        """Check if testrun has been canceled."""
        return request_activity_canceled(self.etos_library, str(activity_id))

    def __is_finished(self, test_suite_started_id: UUID) -> dict:
        """Return the test suite finished event for a test suite started."""
        return request_test_suite_finished(
            self.etos_library, str(test_suite_started_id)
        )

    def __log_announcements(
        self, tercc_id: UUID, activity_id: Optional[UUID]
    ) -> list[str]:
        """Get latest new announcements."""
        ids = [str(tercc_id)]
        if activity_id is not None:
            ids.append(str(activity_id))
        announcements = self.events.get("announcements")
        for announcement in request_announcements(self.etos_library, ids):
            if announcement not in announcements:
                announcements.append(announcement)
                data = announcements[-1].get("data")
                self.logger.info("%s: %s", data.get("heading"), data.get("body"))

    def __main_test_suites(self, activity_id: UUID) -> Iterator[dict]:
        """Get main test suites started from event repository."""
        yield from request_main_test_suites_started(self.etos_library, str(activity_id))

    def __sub_test_suites(self, main_test_suite_id: UUID) -> Iterator[dict]:
        """Get sub test suites started from event repository."""
        for sub_suite_started in request_test_suite_started(
            self.etos_library, str(main_test_suite_id)
        ):
            time.sleep(1)
            event_id = sub_suite_started["meta"]["id"]
            self.test_suite_started.setdefault(event_id, {"finished": False})
            if self.test_suite_started[event_id].get("finished"):
                continue
            finished = self.__is_finished(UUID(event_id, version=4))
            if finished is not None:
                self.test_suite_started[event_id]["finished"] = True
                yield finished

    def __collect_events(self, activity_id: UUID):
        """Get events for an ETOS testrun."""
        finished = True
        for main_suite in self.__main_test_suites(activity_id):
            main_suite_id = UUID(main_suite["meta"]["id"], version=4)

            self.has_started = True
            main_suite_finished = self.__is_finished(main_suite_id)
            if main_suite_finished not in self.main_suites_finished:
                finished = False

            for sub_suite_finished in self.__sub_test_suites(main_suite_id):
                self.test_suites_finished.append(sub_suite_finished)

            if main_suite_finished:
                self.main_suites_finished.append(main_suite_finished)

        self.has_finished = finished
        return self.test_suites_finished, self.main_suites_finished

    def __text(self) -> str:
        """Generate text based on test results."""
        message_template = (
            "{announcement}\t"
            "Started : {started_length}\t"
            "Finished: {finished_length}\t"
        )
        try:
            announcement = self.events["announcements"][-1]["data"]["body"]
        except (KeyError, IndexError, TypeError):
            announcement = ""

        params = {
            "started_length": len(self.test_suite_started.keys()),
            "finished_length": len(self.events.get("testSuiteFinished", [])),
            "announcement": announcement,
        }
        return message_template.format(**params)

    def __environment_defined(self, activity_id: UUID) -> None:
        """Get environment events set for this execution."""
        ids = list(self.test_suite_started.keys()) + list(
            self.main_suite_started.keys()
        )
        ids.append(str(activity_id))
        self.events["environmentDefined"] = list(
            request_environment(self.etos_library, ids)
        )

    def __test_result(self, activity_id: UUID) -> tuple[bool, str]:
        """Build test results based on events retrieved."""
        nbr_of_fail = 0
        if not self.has_started:
            return False, "Test suite did not start."
        self.__environment_defined(activity_id)

        verdict = "PASSED"
        for main_suite_finished in self.events.get("mainSuiteFinished", [{}]):
            data = main_suite_finished.get("data", {})
            outcome = data.get("testSuiteOutcome", {})
            if outcome.get("verdict") != "PASSED":
                verdict = outcome.get("verdict")

        if verdict == "PASSED":
            result = True
        else:
            for test_suite_finished in self.events.get("testSuiteFinished"):
                data = test_suite_finished.get("data", {})
                outcome = data.get("testSuiteOutcome", {})
                verdict = outcome.get("verdict")
                if verdict != "PASSED":
                    nbr_of_fail += 1
        if nbr_of_fail > 0:
            nbr_of_suites = len(self.test_suite_started.keys())
            message = f"{nbr_of_fail}/{nbr_of_suites} test suites failed."
            result = False
        else:
            message = "Test suite finished successfully."
            result = True
        return result, message

    def get_results(self, testrun: "TestRun"):
        """Get results from an ETOS testrun.

        This method is a blocking method until test run has finished.
        """
        timeout = time.time() + 24 * HOUR
        self.__wait_for_test_suite(testrun.data.tercc)
        activity_id = self.__wait_for_activity(testrun.data.tercc)
        self.events["activityId"] = activity_id

        while time.time() < timeout:
            self.__log_announcements(testrun.data.tercc, activity_id)
            time.sleep(10)
            test_suites, main_suites = self.__collect_events(activity_id)
            self.events["testSuiteFinished"] = test_suites
            self.events["mainSuiteFinished"] = main_suites
            canceled = self.__is_canceled(activity_id)
            if canceled:
                return (
                    False,
                    (
                        "Test Suite was canceled, with the following reason: "
                        f"{canceled['data']['reason']}"
                    ),
                    canceled,
                )
            if not self.has_started:
                continue
            self.logger.info(self.__text())
            if self.has_finished:
                return *self.__test_result(activity_id), None
        return False, "Test suites did not finish", None
