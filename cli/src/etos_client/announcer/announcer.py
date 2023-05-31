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
from etos_client.events.events import Events, TestSuite, Announcement


class Announcer:  # pylint:disable=too-few-public-methods
    """Announce the state of ETOS."""

    logger = logging.getLogger(__name__)

    def __init__(self):
        """Init."""
        self.__already_logged = []

    def __log_announcements(self, announcements: list[Announcement]) -> None:
        """Get latest new announcements."""
        for announcement in announcements:
            if announcement not in self.__already_logged:
                self.__already_logged.append(announcement)
                self.logger.info("%s: %s", announcement.heading, announcement.body)

    def __etos_state_information(self, test_suites: list[TestSuite]) -> str:
        """Generate text based on ETOS state."""
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

    def announce(self, events: Events) -> None:
        """Announce the ETOS state."""
        if not events.tercc:
            return
        if not events.activity.triggered:
            return
        self.__log_announcements(events.announcements)
        if events.main_suites:
            self.logger.info(self.__etos_state_information(events.main_suites))
