# -*- coding: utf-8 -*-
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
"""ETOS debug testrun."""

import sys
import logging

from etos_lib.lib.http import Http
#from etos_lib.messaging.events import Artifact, Message, Report, Shutdown  # import disabled due to: https://github.com/eiffel-community/etos/issues/417
from urllib3.util import Retry
from requests.exceptions import HTTPError
from etosctl.command import Command
from etosctl.models import CommandMeta
from etos_client.sse.v2alpha.client import SSEClient

# dummy classes: remove when the etos_lib.messaging module is available: https://github.com/eiffel-community/etos/issues/417
class Artifact:
    pass

class Message:
    pass

class Report:
    pass

class Shutdown:
    pass

LOGGER = logging.getLogger(__name__)
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


class Debug(Command):
    """ETOS debug testrun.

    Usage: etosctl [-v|-vv] [options] debug <cluster> <testrun> [<args>...]

    Options:
        -h,--help     Show this screen
        --version     Print version and exit
    """

    meta = CommandMeta(
        name="debug",
        description="Debug ETOS testruns",
        version="v0",
    )
    logger = logging.getLogger(__name__)
    remote_logger = logging.getLogger("ETOS")
    __apikey = None

    @property
    def apikey(self) -> str:
        """Generate and return an API key."""
        if self.__apikey is None:
            http = Http(retry=HTTP_RETRY_PARAMETERS, timeout=10)
            url = f"{self.cluster}/keys/v1alpha/generate"
            response = http.post(
                url,
                json={"identity": "etos-debug", "scope": "get-sse"},
            )
            try:
                response.raise_for_status()
                response_json = response.json()
            except HTTPError:
                self.logger.exception("Failed to generate an API key for ETOS.")
                response_json = {}
            self.__apikey = response_json.get("token")
        return self.__apikey or ""

    def __log(self, message: Message) -> None:
        """Log a message from the ETOS log API."""
        logger = getattr(self.remote_logger, message.data.level)
        logger(
            message,
            extra={
                "rname": message.data.name,
                "rtime": message.data.datestring.strftime("%Y-%m-%d %H:%M:%S"),
            },
        )

    def setup_remote_logging(self) -> None:
        """Set up logging for ETOS remote logs."""
        rhandler = logging.StreamHandler(sys.stdout)
        formatter = logging.Formatter(
            fmt="[%(rtime)s] %(levelname)s:%(rname)s: %(message)s",
        )
        rhandler.setFormatter(formatter)
        rhandler.setLevel(logging.DEBUG)
        self.remote_logger.propagate = False
        self.remote_logger.addHandler(rhandler)

    def run(self, args: dict):
        """Run the debug tool."""
        self.setup_remote_logging()
        self.logger.setLevel(logging.DEBUG)
        self.remote_logger.setLevel(logging.DEBUG)

        self.cluster = args["<cluster>"]
        client = SSEClient(
            self.cluster,
            [
                "message.debug",
                "message.info",
                "message.warning",
                "message.error",
                "message.critical",
                "report.*",
                "artifact.*",
                "shutdown.*",
            ],
        )
        artifacts = []
        reports = []
        shutdown = None
        messages = []
        for event in client.event_stream(args["<testrun>"], self.apikey):
            if isinstance(event, Message):
                self.__log(event)
                messages.append(event)
            elif isinstance(event, Artifact):
                artifacts.append(event)
            elif isinstance(event, Report):
                reports.append(event)
            elif isinstance(event, Shutdown):
                shutdown = event
        self.logger.info("== Report ==")
        self.logger.info("Number of produced artifacts: %d", len(artifacts))
        self.logger.info("Number of produced reports  : %d", len(reports))
        self.logger.info("Number of produced message  : %d", len(messages))
        if isinstance(shutdown, Shutdown):
            self.logger.info("Verdict                     : %s", shutdown.data.verdict)
            self.logger.info("Conclusion                  : %s", shutdown.data.conclusion)
            self.logger.info("Description                 : %s", shutdown.data.description)
        else:
            self.logger.info("Description                 : No shutdown event found")
        self.logger.info("============")
