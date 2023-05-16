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
"""ETOS Client log handler module."""
import os
import logging
import json
import shutil
from requests.exceptions import HTTPError
from etos_client.event_repository import request_artifacts


class ETOSLogHandler:
    """ETOS client log handler. Download all logs sent via EiffelTestSuiteFinishedEvent."""

    logger = logging.getLogger(__name__)

    def __init__(self, etos: "etos_lib.ETOS", workspace: str, events: list):
        """Initialize log handler.

        :param etos: ETOS Library instance.
        :param workspace: Directory where ETOS client shall store logs.
        :param events: All events collected from the test execution.
        """
        self.etos = etos
        self.workspace = workspace
        self.events = events

    @staticmethod
    def _logs(test_suite_finished):
        """Iterate over all persistentLogs in test_suite_finished event.

        :param test_suite_finished: JSON data from test_suite_finished event.
        :type test_suite_finished: str
        :return: Log name and log url.
        :rtype: tuple
        """
        for log in test_suite_finished.get("data", {}).get(
            "testSuitePersistentLogs", []
        ):
            yield log.get("name"), log.get("uri")

    @property
    def all_logs(self):
        """Iterate over all logs for the executed test suite."""
        for finished in self.events.get("testSuiteFinished", []) + self.events.get(
            "mainSuiteFinished", []
        ):
            for log in self._logs(finished):
                yield log

    @property
    def all_artifacts(self):
        """Iterate over all artifacts for the executed test suite."""
        for artifact_created in request_artifacts(
            self.etos, str(self.events.get("activityId"))
        ):
            for _, location in self.etos.utils.search(artifact_created, "uri"):
                suite_name = ""
                for link in artifact_created.get("links", []):
                    for _, name in self.etos.utils.search(
                        link.get("links", {}), "name"
                    ):
                        suite_name = name  # There should be exactly one!
                for _, name in self.etos.utils.search(
                    artifact_created.get("data", {}), "name"
                ):
                    yield f"{suite_name}_{name}", f"{location}/{name}"

    def _iut_data(self, environment):
        """Get IUT data from Environment URI.

        :param environment: Environment event to get URI from.
        :type environment: dict
        :return: IUT JSON data.
        :rtype: dict
        """
        if environment.get("uri"):
            iut_data = self.etos.http.wait_for_request(environment.get("uri"))
            return iut_data.json()
        return None

    @property
    def iuts(self):
        """All IUT Data environment events."""
        for environment in self.events.get("environmentDefined", []):
            if environment.get("data", {}).get("name", "").startswith("IUT Data"):
                yield self._iut_data(environment.get("data"))

    def _download(self, name, uri, directory):
        """Download a file and and write to disk.

        :param name: Name of resulting file.
        :type name: str
        :param uri: URI from where the file can be downloaded.
        :type uri: str
        :param directory: Into which directory to write the downloaded file.
        :type directory: str
        """
        index = 0
        download_name = name
        while os.path.exists(os.path.join(directory, download_name)):
            index += 1
            download_name = f"{index}_{name}"
        self.logger.info("Downloading %s", download_name)
        generator = self.etos.http.wait_for_request(uri, as_json=False, stream=True)
        try:
            for response in generator:
                with open(os.path.join(directory, download_name), "wb+") as report:
                    for chunk in response:
                        report.write(chunk)
                break
            return True
        except (ConnectionError, HTTPError) as error:
            self.logger.warning("Failed in downloading %r.", download_name)
            self.logger.warning(str(error))
            return False

    def download_logs(self, report_dir: str, artifact_dir: str):
        """Download all logs to report and artifact directories."""
        nbr_of_logs_downloaded = 0
        incomplete = False

        report_dir = os.path.join(self.workspace, report_dir)
        if not os.path.exists(report_dir):
            os.makedirs(report_dir)
        for name, uri in self.all_logs:
            result = self._download(name, uri, report_dir)
            if result:
                nbr_of_logs_downloaded += 1
            else:
                incomplete = True

        artifact_dir = os.path.join(self.workspace, artifact_dir)
        if not os.path.exists(artifact_dir):
            os.makedirs(artifact_dir)
        for name, uri in self.all_artifacts:
            result = self._download(name, uri, artifact_dir)
            if result:
                nbr_of_logs_downloaded += 1
            else:
                incomplete = True

        for index, iut in enumerate(self.iuts):
            if iut is None:
                break
            self.logger.info("Downloading IUT Data")
            try:
                filename = f"IUT_{index}.json"
                with open(
                    os.path.join(artifact_dir, filename), "w+", encoding="utf-8"
                ) as report:
                    json.dump(iut, report)
            except Exception as error:  # pylint:disable=broad-except
                self.logger.warning("Failed in downloading %r.", filename)
                self.logger.warning(str(error))
                incomplete = True
            nbr_of_logs_downloaded += 1

        shutil.make_archive(os.path.join(artifact_dir, "reports"), "zip", report_dir)
        self.logger.info("Downloaded %d logs", nbr_of_logs_downloaded)
        self.logger.info("Reports: %s", report_dir)
        self.logger.info("Artifacs: %s", artifact_dir)
        if incomplete:
            self.logger.error("Logs failed downloading.")
            return False
        self.logger.info("Logs downloaded.")
        return True
