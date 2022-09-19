# Copyright 2020-2022 Axis Communications AB.
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
"""ETOS Client test result handler."""
import time
import logging
from .graphql import (
    request_activity,
    request_activity_canceled,
    request_main_test_suites_started,
    request_test_suite_finished,
    request_test_suite_started,
    request_suite,
    request_announcements,
    request_environment,
)

_LOGGER = logging.getLogger(__name__)


# https://github.com/eiffel-community/etos/issues/127 pylint: disable=too-many-instance-attributes
class ETOSTestResultHandler:
    """Handle ETOS test results."""

    __activity_id = None
    has_started = False
    has_finished = False

    def __init__(self, etos):
        """Initialize ETOS Client test result handler.

        :param etos: ETOS Library instance.
        :type etos: :obj:`etos_lib.etos.ETOS`
        """
        self.etos = etos
        self.suite_id = self.etos.config.get("suite_id")

        self.test_suite_started = {}
        self.test_suites_finished = []
        self.main_suite_started = {}
        self.main_suites_finished = []

        self.events = {}
        self.announcements = []

    @property
    def activity_id(self):
        """Get the activity ID from event repository."""
        if self.__activity_id is None:
            activity = request_activity(self.etos, self.suite_id)
            if activity is not None:
                self.__activity_id = activity["meta"]["id"]
        return self.__activity_id

    @property
    def main_test_suites_started(self):
        """Get main test suites started from event repository."""
        if self.activity_id is not None:
            yield from request_main_test_suites_started(self.etos, self.activity_id)

    @property
    def spinner_text(self):
        """Generate the spinner text based on test results.

        :return: String formatted for the Halo spinner.
        :rtype: str
        """
        message_template = (
            "{announcement}\t"
            "Started : {started_length}\t"
            "Finished: {finished_length}\t"
        )
        try:
            announcement = self.announcements[-1]["data"]["body"]
        except (KeyError, IndexError, TypeError):
            announcement = ""

        params = {
            "started_length": len(self.test_suite_started.keys()),
            "finished_length": len(self.events.get("testSuiteFinished", [])),
            "announcement": announcement,
        }
        return message_template.format(**params)

    def get_environment_events(self):
        """Get the environment events set for this execution."""
        ids = list(self.test_suite_started.keys()) + list(
            self.main_suite_started.keys()
        )
        ids.append(self.activity_id)
        self.events["environmentDefined"] = list(request_environment(self.etos, ids))

    def test_result(self):
        """Build test results based on events retrieved.

        :return: Result and message.
        :rtype: tuple
        """
        nbr_of_fail = 0
        if not self.has_started:
            return False, "Test suite did not start."
        self.get_environment_events()

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

    def latest_announcement(self, spinner):
        """Find latest announcement and print it.

        :param spinner: Spinner text item.
        :type spinner: :obj:`Spinner`
        """
        for announcement in request_announcements(
            self.etos, [self.etos.config.get("suite_id"), self.activity_id]
        ):
            if announcement not in self.announcements:
                self.announcements.append(announcement)
                data = self.announcements[-1].get("data")
                spinner.info(f"{data.get('heading')}: {data.get('body')}")
                spinner.start("Waiting for ETOS.")

    def finished(self, test_suite_started_id):
        """Return the test suite finished event for a test suite started.

        :param test_suite_started_id: Test suite to check finished for.
        :type test_suite_started_id: dict
        """
        return request_test_suite_finished(self.etos, test_suite_started_id)

    def collect_sub_suites(self, main_test_suite_id):
        """Collect sub suites for a main test suite started.

        :param main_test_suite_id: The main test suite to look for sub suites for.
        :type main_test_suite_id: str
        """
        for sub_suite_started in request_test_suite_started(
            self.etos, main_test_suite_id
        ):
            time.sleep(1)
            event_id = sub_suite_started["meta"]["id"]
            self.test_suite_started.setdefault(event_id, {"finished": False})
            if self.test_suite_started[event_id].get("finished"):
                continue
            finished = self.finished(event_id)
            if finished is not None:
                self.test_suite_started[event_id]["finished"] = finished is not None
                yield finished

    def get_events(self):
        """Get events for this ETOS execution.

        :return: List of test suite finished.
        :rtype: list
        """
        if self.activity_id is None:
            return {}
        finished = True
        for main_suite in self.main_test_suites_started:
            main_suite_id = main_suite["meta"]["id"]

            self.has_started = True
            main_suite_finished = self.finished(main_suite_id)
            if main_suite_finished in self.main_suites_finished:
                continue
            finished = False

            for sub_suite_finished in self.collect_sub_suites(main_suite_id):
                self.test_suites_finished.append(sub_suite_finished)

            if main_suite_finished:
                self.main_suites_finished.append(main_suite_finished)

        self.has_finished = finished
        return self.test_suites_finished, self.main_suites_finished

    def print_suite(self, spinner):
        """Print test suite batchesUri.

        :param spinner: Spinner text item.
        :type spinner: :obj:`Spinner`
        """
        spinner.text = "Waiting for test suite url."
        timeout = time.time() + 60
        while time.time() < timeout:
            tercc = request_suite(self.etos, self.etos.config.get("suite_id"))
            if tercc:
                spinner.info(f"Test suite: {tercc['data']['batchesUri']}")
                spinner.start()
                return
            time.sleep(1)
        raise TimeoutError("Test suite not available in 10s.")

    def wait_for_test_suite_finished(self, spinner):
        """Query graphql server until the number of started is equal to number of finished.

        :param spinner: Spinner text item.
        :type spinner: :obj:`Spinner`
        :return: Whether it was a successful execution or not, the test results
                 and if the execution was canceled.
        :rtype: tuple
        """
        self.print_suite(spinner)
        timeout = time.time() + 3600 * 24
        while time.time() < timeout:
            self.latest_announcement(spinner)
            time.sleep(10)
            test_suites, main_suites = self.get_events()
            self.events = {
                "activityId": self.activity_id,
                "testSuiteFinished": test_suites,
                "mainSuiteFinished": main_suites,
            }
            canceled = request_activity_canceled(self.etos, self.activity_id)
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
            spinner.text = self.spinner_text
            if self.has_finished:
                return *self.test_result(), canceled
        return False, "Test suites did not finish", canceled
