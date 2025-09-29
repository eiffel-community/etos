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
"""Command line for starting ETOSv0 testruns."""
import sys
import os
import logging
import json
from json import JSONDecodeError
from typing import Optional
import warnings

from etos_client.types.result import Conclusion, Verdict
from etos_client.etos.v0.etos import Etos
from etos_client.sse.v1.client import SSEClient

from etosctl.command import SubCommand
from etosctl.models import CommandMeta


class Start(SubCommand):
    """
    Client for executing test automation suites in ETOS.

    Usage: etosctl testrun v0 start [-v|-vv] [options] [--dataset=DATASET]... -i IDENTITY -s TEST_SUITE <cluster>

    Options:
        -h, --help                                                Show this help message and exit
        -i IDENTITY, --identity IDENTITY                          Artifact created identity purl or ID to execute test suite on.
        -s TEST_SUITE, --test-suite TEST_SUITE                    Test suite execute. Either URL or name.
        -p PARENT_ACTIVITY, --parent-activity PARENT_ACTIVITY     Activity for the TERCC to link to as the cause of the test execution.
        --no-tty                                                  This parameter is no longer in use.
        -w WORKSPACE, --workspace WORKSPACE                       Which workspace to do all the work in.
        -a ARTIFACT_DIR, --artifact-dir ARTIFACT_DIR              Where test artifacts should be stored. Relative to workspace.
        -r REPORT_DIR, --report-dir REPORT_DIR                    Where test reports should be stored. Relative to workspace.
        -d DOWNLOAD_REPORTS, --download-reports DOWNLOAD_REPORTS  This parameter is no longer in use.
        --iut-provider IUT_PROVIDER                               Which IUT provider to use.
        --execution-space-provider EXECUTION_SPACE_PROVIDER       Which execution space provider to use.
        --log-area-provider LOG_AREA_PROVIDER                     Which log area provider to use.
        --dataset DATASET                                         Additional dataset information to the environment provider.
                                                                  Check with your provider which information can be supplied.
        --version                                                 Show program's version number and exit
    """

    logger = logging.getLogger(__name__)
    meta: CommandMeta = CommandMeta(
        name="start", description="Start an ETOSv0 testrun.", version="v0"
    )

    def activity_triggered(self, argv: list[str]) -> Optional[str]:
        """Get an activity triggered event ID from environment or arguments."""
        if ("-p" not in argv and "--parent-activity" not in argv) and os.getenv(
            "EIFFEL_ACTIVITY_TRIGGERED"
        ) is not None:
            try:
                actt = json.loads(os.getenv("EIFFEL_ACTIVITY_TRIGGERED", ""))
                return actt["meta"]["id"]
            except (JSONDecodeError, IndexError):
                self.logger.warning("Could not load EIFFEL_ACTIVITY_TRIGGERED from environment due it not being formatted JSON")
        return None

    def parse_args(self, argv: list[str]) -> dict:
        """Parse arguments for etosctl testrun start."""
        if ("-i" not in argv and "--identity" not in argv) and os.getenv("IDENTITY") is not None:
            argv.extend(["--identity", os.getenv("IDENTITY", "")])
        if ("-s" not in argv and "--test-suite" not in argv) and os.getenv(
            "TEST_SUITE"
        ) is not None:
            argv.extend(["--test-suite", os.getenv("TEST_SUITE", "")])
        parent_activity_id = self.activity_triggered(argv)
        if parent_activity_id is not None:
            argv.extend(["--parent-activity", parent_activity_id])
        return super().parse_args(argv)

    def run(self, args: dict) -> None:
        """Start an ETOS testrun."""
        if args["--download-reports"]:
            warnings.warn(
                "The '-d/--download-reports' parameter is deprecated",
                DeprecationWarning,
            )
        if args["--no-tty"]:
            warnings.warn("The '--no-tty' parameter is deprecated", DeprecationWarning)

        # Remove trailing slash.
        if args["<cluster>"].endswith("/"):
            args["<cluster>"] = args["<cluster>"].rstrip("/")

        # Remove '/api' if it exists, as we want the ETOS Client to know about the endpoints.
        if args["<cluster>"].endswith("/api"):
            warnings.warn(
                "Cluster URL should no longer end with '/api'",
                DeprecationWarning,
            )
            args["<cluster>"] = args["<cluster>"].rsplit("/", 1)[0]

        self.logger.info("Running in cluster: %r", args["<cluster>"])
        etos = Etos(args, SSEClient(args["<cluster>"]))
        result = etos.run()
        if result.conclusion == Conclusion.FAILED:
            sys.exit(result.reason)
        if result.verdict == Verdict.FAILED:
            self.logger.error(result.reason)
        else:
            self.logger.info(result.reason)
