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
import logging
from json import JSONDecodeError
from typing import Union

import requests
from requests.exceptions import HTTPError
from urllib3.util import Retry

from etos_lib.lib.http import Http
from .schema import RequestSchema, ResponseSchema


# Max total time for a ping request including delays with backoff factor 0.5 will be:
# 0.5 + 1.5 + 3.5 + 7.5 + 15.5 = 28.5 (seconds)
HTTP_RETRY_PARAMETERS = Retry(
    total=5,  # limit the total number of retries only (regardless of error type)
    connect=None,
    read=None,
    status_forcelist=[500, 502, 503, 504],  # Common temporary error status codes
    backoff_factor=0.5,
    raise_on_redirect=True,  # Raise an exception if too many redirects
)


class ETOS:  # pylint:disable=too-few-public-methods
    """Handle communication with ETOS."""

    logger = logging.getLogger(__name__)
    reason = ""
    response: ResponseSchema = None

    def __init__(self, cluster: str) -> None:
        """Initialize ETOS."""
        self.cluster = cluster
        # ping HTTP client with 5 sec timeout for each attempt:
        self.__http_ping = Http(retry=HTTP_RETRY_PARAMETERS, timeout=5)
        # greater HTTP timeout for other requests:
        self.__http = Http(retry=HTTP_RETRY_PARAMETERS, timeout=10)

    def start(self, request_data: RequestSchema) -> Union[ResponseSchema, None]:
        """Start ETOS."""
        self.logger.info("Check connection to ETOS.")
        if not self.__check_connection():
            self.reason = "Unable to connect to ETOS. Please check your connection."
            return None
        self.logger.info("Connection successful.")
        self.logger.info("Triggering ETOS.")
        self.response = self.__start(request_data)
        if not self.response:
            self.reason = "Failed to start ETOS"
            return None
        return self.response

    def __check_connection(self) -> bool:
        """Check connection to ETOS."""
        # retry rules are set in the Http client
        response = self.__http_ping.get(f"{self.cluster}/api/selftest/ping")
        return self.__response_ok(response)

    def __start(self, request_data: RequestSchema) -> Union[ResponseSchema, None]:
        """Start an ETOS testrun."""
        response = self.__retry_trigger_etos(request_data)
        if not response:
            return None
        return response

    def __retry_trigger_etos(self, request_data: RequestSchema) -> Union[ResponseSchema, None]:
        """Trigger ETOS, retrying on non-client errors until successful."""
        # retry rules are set in the Http client
        response = self.__http.post(f"{self.cluster}/api/etos", json=request_data.dict())
        if self.__response_ok(response):
            return ResponseSchema.from_response(response.json())
        self.logger.critical("Failed to trigger ETOS.")
        return None

    def __response_ok(self, response: requests.Response) -> bool:
        """Check response and log the relevant information."""
        try:
            response.raise_for_status()
            return True
        except HTTPError as http_error:
            if 400 <= http_error.response.status_code < 500:
                try:
                    response_json = response.json()
                except JSONDecodeError:
                    self.logger.info("Raw response from ETOS: %r", response.text)
                    response_json = {"detail": "Unknown client error from ETOS"}
                self.logger.critical(response_json.get("detail"))
            else:
                self.logger.critical("Network connectivity error")
        return False
