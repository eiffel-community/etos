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
from typing import Optional
from .events import (
    Events,
    Activity,
    TestSuite,
    SubSuite,
    Environment,
    Announcement,
    Artifact,
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
        collected = {
            suite.started["meta"]["id"]: suite for suite in self.__events.main_suites
        }
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

    def __sub_test_suites(self, main_suite: TestSuite) -> list[SubSuite]:
        """Collect sub suites for a single main suite."""
        sub_suites = []
        collected = {
            suite.started["meta"]["id"]: suite for suite in main_suite.sub_suites
        }
        for started in self.event_repository.request_sub_test_suite_started(
            self.etos_library, main_suite.started["meta"]["id"]
        ):
            if started["meta"]["id"] not in collected:
                collected[started["meta"]["id"]] = SubSuite(started=started)
        for test_suite_id, test_suite in collected.items():
            if test_suite.finished is None:
                test_suite.finished = self.event_repository.request_test_suite_finished(
                    self.etos_library, test_suite_id
                )
            sub_suites.append(test_suite)
        return sub_suites

    def __environments(
        self, activity_id: UUID, test_suites: list[TestSuite]
    ) -> list[Environment]:
        """Collect environment defined events from an ETOS test run."""
        ids = [str(activity_id)]
        for test_suite in test_suites:
            ids.append(test_suite.started["meta"]["id"])
            for sub_suite in test_suite.sub_suites:
                ids.append(sub_suite.started["meta"]["id"])
        environments = []
        for environment in self.event_repository.request_environment(
            self.etos_library, ids
        ):
            environments.append(
                Environment(
                    name=environment["data"]["name"], uri=environment["data"]["uri"]
                )
            )
        return environments

    def __activity(self, tercc_id: UUID) -> Activity:
        """Collect activity events from an ETOS test run."""
        activity = Activity()
        triggered = self.__events.activity.triggered
        if triggered is None:
            triggered = self.event_repository.request_activity(
                self.etos_library, str(tercc_id)
            )
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

    def __announcements(
        self, tercc_id: UUID, activity_id: Optional[UUID]
    ) -> list[Announcement]:
        """Collect announcements for an ETOS test run."""
        ids = [str(tercc_id)]
        if activity_id is not None:
            ids.append(str(activity_id))
        announcements = []
        for announcement in self.event_repository.request_announcements(
            self.etos_library, ids
        ):
            announcements.append(Announcement.parse_obj(announcement["data"]))
        return announcements

    def __artifacts(self, activity_id: UUID) -> list[Artifact]:
        """Collect artifacts from ETOS."""
        artifacts = []
        for artifact_created in self.event_repository.request_artifacts(
            self.etos_library, str(activity_id)
        ):
            file_names = [
                _file["name"] for _file in artifact_created["data"]["fileInformation"]
            ]
            suite_name = ""
            for link in artifact_created.get("links", []):
                try:
                    suite_name = link["links"]["data"]["name"]
                except KeyError:
                    pass
            for _, location in self.etos_library.utils.search(artifact_created, "uri"):
                artifacts.append(
                    Artifact(files=file_names, suite_name=suite_name, location=location)
                )
        return artifacts

    def collect(self, tercc_id: UUID) -> Events:
        """Collect events from ETOS."""
        activity_id = None
        self.__events.tercc = self.__tercc(tercc_id)
        if self.__events.tercc is None:
            return self.__events
        self.__events.announcements = self.__announcements(tercc_id, activity_id)

        self.__events.activity = self.__activity(tercc_id)
        if self.__events.activity.triggered is None:
            return self.__events
        activity_id = UUID(self.__events.activity.triggered["meta"]["id"], version=4)
        self.__events.main_suites = self.__main_test_suites(activity_id)
        for main_suite in self.__events.main_suites:
            main_suite.sub_suites = self.__sub_test_suites(main_suite)
        self.__events.environments = self.__environments(
            activity_id, self.__events.main_suites
        )
        self.__events.artifacts = self.__artifacts(activity_id)
        return self.__events
