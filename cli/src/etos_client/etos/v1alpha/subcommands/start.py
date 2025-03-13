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
"""Command line for starting ETOSv1alpha testruns."""

import sys
import os
import logging
import warnings

from etos_client.types.result import Conclusion, Verdict
from etos_client.etos.v1alpha.etos import Etos
from etos_client.sse.v2alpha.client import SSEClient as SSEV2AlphaClient
from etos_client.sse.v1.client import SSEClient as SSEV1Client

from etosctl.command import SubCommand
from etosctl.models import CommandMeta


class Start(SubCommand):
    """
    Client for executing test automation suites in ETOS.

    Usage: etosctl testrun v1alpha start [-v|-vv] [options] [--dataset=DATASET]... -i IDENTITY -s TEST_SUITE <cluster>

    Options:
        -h, --help                                                Show this help message and exit
        -i IDENTITY, --identity IDENTITY                          Artifact created identity purl or ID to execute test suite on.
        -s TEST_SUITE, --test-suite TEST_SUITE                    Test suite execute. Either URL or name.
        -w WORKSPACE, --workspace WORKSPACE                       Which workspace to do all the work in.
        -a ARTIFACT_DIR, --artifact-dir ARTIFACT_DIR              Where test artifacts should be stored. Relative to workspace.
        -r REPORT_DIR, --report-dir REPORT_DIR                    Where test reports should be stored. Relative to workspace.
        --iut-provider IUT_PROVIDER                               Which IUT provider to use.
        --execution-space-provider EXECUTION_SPACE_PROVIDER       Which execution space provider to use.
        --log-area-provider LOG_AREA_PROVIDER                     Which log area provider to use.
        --dataset DATASET                                         Additional dataset information to the environment provider.
                                                                  Check with your provider which information can be supplied.
        --ssev2alpha                                              Use the v2alpha version of sse.
        --version                                                 Show program's version number and exit
    """

    logger = logging.getLogger(__name__)
    meta: CommandMeta = CommandMeta(
        name="start", description="Start an ETOSv1alpha testrun.", version="v1alpha"
    )

    def parse_args(self, argv: list[str]) -> dict:
        """Parse arguments for etosctl testrun start."""
        if ("-i" not in argv and "--identity" not in argv) and os.getenv("IDENTITY") is not None:
            argv.extend(["--identity", os.getenv("IDENTITY", "")])
        if ("-s" not in argv and "--test-suite" not in argv) and os.getenv(
            "TEST_SUITE"
        ) is not None:
            argv.extend(["--test-suite", os.getenv("TEST_SUITE", "")])
        return super().parse_args(argv)

    def run(self, args: dict) -> None:
        """Start an ETOS testrun."""
        warnings.warn("This is an alpha version of ETOS! Don't expect it to work properly")
        self.logger.info("Running in cluster: %r", args["<cluster>"])

        filter = [
            "message.info",
            "message.warning",
            "message.error",
            "message.critical",
            "report.*",
            "artifact.*",
            "shutdown.*",
        ]
        if args["--ssev2alpha"]:
            etos = Etos(args, SSEV2AlphaClient(args["<cluster>"], filter))
        else:
            etos = Etos(args, SSEV1Client(args["<cluster>"]))
        result = etos.run()
        if result.conclusion == Conclusion.FAILED:
            sys.exit(result.reason)
        if result.verdict == Verdict.FAILED:
            self.logger.error(result.reason)
        else:
            self.logger.info(result.reason)
