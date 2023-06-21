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
"""ETOS API handler."""
import time
import logging
from typing import Union
from json import JSONDecodeError

from urllib3.exceptions import MaxRetryError, NewConnectionError
import requests
from requests.exceptions import HTTPError

from .schema import RequestSchema, ResponseSchema


class ETOS:  # pylint:disable=too-few-public-methods
    """Handle communication with ETOS."""

    logger = logging.getLogger(__name__)
    reason = ""

    def __init__(self, cluster: str) -> None:
        """Initialize ETOS."""
        self.cluster = cluster

    def start(self, request_data: RequestSchema) -> Union[ResponseSchema, None]:
        """Start ETOS."""
        self.logger.info("Check connection to ETOS.")
        if not self.__check_connection():
            self.reason = "Unable to connect to ETOS. Please check your connection."
            return None
        self.logger.info("Connection successful.")
        self.logger.info("Triggering ETOS.")
        response = self.__start(request_data)
        if not response:
            self.reason = "Failed to start ETOS"
            return None
        return response

    def __check_connection(self) -> bool:
        """Check connection to ETOS."""
        try:
            response = requests.get(f"{self.cluster}/selftest/ping", timeout=5)
            response.raise_for_status()
            return True
        except Exception:  # pylint:disable=broad-exception-caught
            return False

    def __start(self, request_data: RequestSchema) -> Union[ResponseSchema, None]:
        """Start an ETOS testrun."""
        response = self.__retry_trigger_etos(request_data)
        if not response:
            return None

        return response

    def __retry_trigger_etos(
        self, request_data: RequestSchema
    ) -> Union[ResponseSchema, None]:
        """Trigger ETOS, retrying on non-client errors until successful."""
        end_time = time.time() + 30
        while time.time() < end_time:
            response = requests.post(
                f"{self.cluster}/etos", json=request_data.dict(), timeout=10
            )
            if self.__should_retry(response):
                time.sleep(2)
                continue
            if not response.ok:
                return None
            return ResponseSchema.from_response(response.json())
        self.logger.critical("Failed to trigger ETOS.")
        return None

    def __should_retry(self, response: requests.Response) -> bool:
        """Check response to see whether it is worth retrying or not."""
        try:
            response.raise_for_status()
        except HTTPError as http_error:
            if 400 <= http_error.response.status_code < 500:
                try:
                    response_json = response.json()
                except JSONDecodeError:
                    self.logger.info("Raw response from ETOS: %r", response.text)
                    response_json = {"detail": "Unknown client error from ETOS"}
                self.logger.critical(response_json.get("detail"))
                return False
            return True
        except (
            ConnectionError,
            NewConnectionError,
            MaxRetryError,
            TimeoutError,
        ):
            self.logger.exception("Network connectivity errors when triggering ETOS.")
            return True
        return False
