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
"""ETOS Client module."""
import time
import logging
import traceback
from json import JSONDecodeError
from uuid import UUID
from packageurl import PackageURL
from urllib3.exceptions import MaxRetryError, NewConnectionError
from requests.exceptions import HTTPError


_LOGGER = logging.getLogger(__name__)


class ETOSClient:
    """Client for starting test suites in ETOS."""

    event_repository = None
    test_suite_id = None
    artifact_id = None
    artifact_identity = None

    def __init__(self, etos, cluster):
        """Initialize ETOS client.

        :param etos: ETOS Library instance.
        :type etos: :obj:`etos_lib.etos.ETOS`
        :param cluster: ETOS cluster to start tests in.
        :type cluster: str
        """
        self.etos = etos
        self.cluster = cluster
        self.test_execution = {}

    @staticmethod
    def is_uuid(string):
        """Test if string is a valid UUID v4.

        :param string: A string to test.
        :type string: str
        :return: Is string a valid UUID.
        :rtype: bool
        """
        try:
            UUID(string, version=4)
            return True
        except ValueError:
            return False

    @staticmethod
    def is_packageurl(string):
        """Test if string is a valid PackageURL.

        :param string: A string to test.
        :type string: str
        :return: Is string a valid PackageURL.
        :rtype: bool
        """
        try:
            PackageURL.from_string(string)
            return True
        except ValueError:
            return False

    @property
    def data(self):
        """ETOS request data."""
        dataset = self.etos.config.get("dataset")
        if len(dataset) == 1:
            dataset = dataset[0]
        data = {
            "dataset": dataset,
            "iut_provider": self.etos.config.get("iut_provider"),
            "execution_space_provider": self.etos.config.get(
                "execution_space_provider"
            ),
            "log_area_provider": self.etos.config.get("log_area_provider"),
            "test_suite_url": self.etos.config.get("test_suite"),
        }
        artifact_identifier = self.etos.config.get("artifact_identifier")
        if self.is_uuid(artifact_identifier):
            data["artifact_id"] = artifact_identifier
        elif self.is_packageurl(artifact_identifier):
            data["artifact_identity"] = artifact_identifier
        else:
            raise ValueError(
                f"Identity {artifact_identifier!r} is not a valid PackageURL or UUID."
            )
        return data

    def start(self, spinner):
        """Start ETOS test execution.

        :param spinner: Spinner text item.
        :type spinner: :obj:`Spinner`
        :return: Whether or not suite triggered correctly.
        :rtype: bool
        """
        spinner.info(str(self.data))

        end_time = time.time() + 30
        while time.time() < end_time:
            try:
                response = self.etos.http.request(
                    "POST", f"{self.cluster}/etos", json=self.data
                )
                self.test_execution = response
                self.test_suite_id = response.get("tercc")
                self.artifact_id = response.get("artifact_id")
                self.artifact_identity = response.get("artifact_identity")
                self.event_repository = response.get("event_repository")
                break
            except HTTPError as http_error:
                response = http_error.response
                if 400 <= response.status_code < 500:
                    try:
                        response_json = response.json()
                    except JSONDecodeError:
                        spinner.info(f"Raw response from ETOS: {response.text!r}")
                        response_json = {"detail": "Unknown client error from ETOS"}
                    spinner.fail(response_json.get("detail"))
                    return False
                traceback.print_exc()
                time.sleep(2)
            except (
                ConnectionError,
                NewConnectionError,
                MaxRetryError,
                TimeoutError,
            ):
                traceback.print_exc()
                time.sleep(2)
        else:
            spinner.fail("Failed to trigger ETOS.")
            return False
        spinner.succeed("ETOS triggered.")
        return True
