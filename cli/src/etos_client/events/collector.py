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
"""ETOS client event collector."""
from uuid import UUID
from .events import (
    Events,
    Activity,
    TestSuite,
)


class Collector:  # pylint:disable=too-few-public-methods
    """Collect events from an event repository."""

    def __init__(
        self,
        etos_library: "etos_lib.ETOS",
        event_repository: "etos_client.event_repository.graphql",
    ) -> None:
        """Initialize with event repository."""
        self.event_repository = event_repository
        self.etos_library = etos_library
        self.__events = Events()

    def __tercc(self, tercc_id: UUID) -> dict:
        """Get TERCC event to make sure it has been sent."""
        if self.__events.tercc is None:
            return self.event_repository.request_suite(self.etos_library, str(tercc_id))
        return self.__events.tercc

    def __main_test_suites(self, activity_id: UUID) -> list[TestSuite]:
        """Collect main test suite events for an ETOS activity."""
        test_suites = []
        collected = {suite.started["meta"]["id"]: suite for suite in self.__events.main_suites}
        for started in self.event_repository.request_main_test_suites_started(
            self.etos_library, str(activity_id)
        ):
            if started["meta"]["id"] not in collected.keys():
                collected[started["meta"]["id"]] = TestSuite(started=started)
        for test_suite_id, test_suite in collected.items():
            if test_suite.finished is None:
                test_suite.finished = self.event_repository.request_test_suite_finished(
                    self.etos_library, test_suite_id
                )
            test_suites.append(test_suite)
        return test_suites

    def __activity(self, tercc_id: UUID) -> Activity:
        """Collect activity events from an ETOS test run."""
        activity = Activity()
        triggered = self.__events.activity.triggered
        if triggered is None:
            triggered = self.event_repository.request_activity(self.etos_library, str(tercc_id))
        activity.triggered = triggered
        if activity.triggered is None:
            return activity

        canceled = self.__events.activity.canceled
        if canceled is None:
            canceled = self.event_repository.request_activity_canceled(
                self.etos_library, triggered["meta"]["id"]
            )
        activity.canceled = canceled

        finished = self.__events.activity.finished
        if finished is None:
            finished = self.event_repository.request_activity_finished(
                self.etos_library, triggered["meta"]["id"]
            )
        activity.finished = finished
        return activity

    def collect_activity(self, tercc_id: UUID) -> Events:
        """Collect activity events from ETOS."""
        self.__events.tercc = self.__tercc(tercc_id)
        if self.__events.tercc is None:
            return self.__events

        self.__events.activity = self.__activity(tercc_id)
        if self.__events.activity.triggered is None:
            return self.__events
        return self.__events

    def collect(self, tercc_id: UUID) -> Events:
        """Collect events from ETOS."""
        if self.__events.activity.triggered is None:
            events = self.collect_activity(tercc_id)
            if events.activity.triggered is None:
                return self.__events
        activity_id = UUID(self.__events.activity.triggered["meta"]["id"], version=4)
        self.__events.main_suites = self.__main_test_suites(activity_id)
        return self.__events
