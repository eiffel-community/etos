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
"""Client for executing test automation suites in ETOS."""
import sys
import os
import logging
import warnings
import shutil
from pathlib import Path
from typing import Optional

from etos_lib import ETOS as ETOSLibrary
from etos_client.events.collector import Collector
from etos_client.etos.schema import RequestSchema
from etos_client.etos import ETOS
from etos_client.test_results import TestResults
from etos_client.downloader import Downloader
from etos_client.event_repository import graphql
from etos_client.test_run import TestRun
from etosctl.command import SubCommand
from etosctl.models import CommandMeta

LOGGER = logging.getLogger(__name__)
MINUTE = 60
HOUR = MINUTE * 60


def start(args: dict) -> ETOS:
    """Start ETOS."""
    etos = ETOS(args["<cluster>"])
    response = etos.start(RequestSchema.from_args(args))
    if not response:
        sys.exit(etos.reason)
    return etos


def directories(args: dict) -> tuple[Path, Path]:
    """Create directories for ETOS logs."""
    artifact_dir = Path(args["--workspace"]).joinpath(args["--artifact-dir"])
    artifact_dir.mkdir(exist_ok=True)
    report_dir = Path(args["--workspace"]).joinpath(args["--report-dir"])
    report_dir.mkdir(exist_ok=True)
    return report_dir, artifact_dir


def loglevel(verbosity: int) -> int:
    """Get loglevel for ETOS remote logs."""
    if verbosity == 1:
        return logging.INFO
    if verbosity == 2:
        return logging.DEBUG
    return logging.WARNING


def wait(
    etos: ETOS, verbosity: int, report_dir: Path, artifact_dir: Path
) -> tuple[Optional[bool], Optional[str]]:
    """Wait for an ETOS testrun to finish."""
    etos_library = ETOSLibrary("ETOS Client", os.getenv("HOSTNAME"), "ETOS Client")
    os.environ["ETOS_GRAPHQL_SERVER"] = etos.response.event_repository

    collector = Collector(etos_library, graphql)
    log_downloader = Downloader(report_dir, artifact_dir)
    clear_queue = True
    log_downloader.start()
    try:
        test_run = TestRun(collector, log_downloader)
        test_run.setup_logging(loglevel(verbosity))
        events = test_run.track(etos, 24 * HOUR)
    except SystemExit:
        clear_queue = False
        raise
    finally:
        log_downloader.stop(clear_queue)
        log_downloader.join()

    LOGGER.info("Downloaded a total of %d logs from test runners", len(log_downloader.downloads))
    LOGGER.info("Archiving reports.")
    shutil.make_archive(artifact_dir.joinpath("reports").relative_to(Path.cwd()), "zip", report_dir)
    LOGGER.info("Reports: %s", report_dir)
    LOGGER.info("Artifacts: %s", artifact_dir)

    if log_downloader.failed:
        sys.exit("ETOS logs did not download successfully.")
    return TestResults().get_results(events)


class Start(SubCommand):
    """
    Client for executing test automation suites in ETOS.

    Usage: etosctl testrun start [-v|-vv] [-h] -i IDENTITY -s TEST_SUITE [--no-tty] [-w WORKSPACE] [-a ARTIFACT_DIR] [-r REPORT_DIR] [-d DOWNLOAD_REPORTS] [--iut-provider IUT_PROVIDER] [--execution-space-provider EXECUTION_SPACE_PROVIDER] [--log-area-provider LOG_AREA_PROVIDER] [--dataset=DATASET]... [--version] <cluster>

    Options:
        -h, --help                                                Show this help message and exit
        -i IDENTITY, --identity IDENTITY                          Artifact created identity purl or ID to execute test suite on.
        -s TEST_SUITE, --test-suite TEST_SUITE                    Test suite execute. Either URL or name.
        --no-tty                                                  This parameter is no longer in use.
        -w WORKSPACE, --workspace WORKSPACE                       Which workspace to do all the work in.
        -a ARTIFACT_DIR, --artifact-dir ARTIFACT_DIR              Where test artifacts should be stored. Relative to workspace.
        -r REPORT_DIR, --report-dir REPORT_DIR                    Where test reports should be stored. Relative to workspace.
        -d DOWNLOAD_REPORTS, --download-reports DOWNLOAD_REPORTS  This parameter is no longer in use.
        --iut-provider IUT_PROVIDER                               Which IUT provider to use.
        --execution-space-provider EXECUTION_SPACE_PROVIDER       Which execution space provider to use.
        --log-area-provider LOG_AREA_PROVIDER                     Which log area provider to use.
        --dataset DATASET                                         Additional dataset information to the environment provider. Check with your provider which information can be supplied.
        --version                                                 Show program's version number and exit
    """

    meta: CommandMeta = CommandMeta(
        name="start", description="Start an ETOS testrun.", version="Something!"
    )

    def parse_args(self, argv: list[str]) -> dict:
        """Parse arguments for etosctl testrun start."""
        if ("-i" not in argv and "--identity" not in argv) and os.getenv("IDENTITY") is not None:
            argv.extend(["--identity", os.getenv("IDENTITY")])
        if ("-s" not in argv and "--test-suite" not in argv) and os.getenv(
            "TEST_SUITE"
        ) is not None:
            argv.extend(["--test-suite", os.getenv("TEST_SUITE")])
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
        args["--workspace"] = args["--workspace"] or os.getenv("WORKSPACE", os.getcwd())
        args["--artifact-dir"] = args["--artifact-dir"] or "artifacts"
        args["--report-dir"] = args["--report-dir"] or "reports"

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

        LOGGER.info("Running in cluster: %r", args["<cluster>"])
        report_dir, artifact_dir = directories(args)
        etos = start(args)
        result, message = wait(etos, args["-v"], report_dir, artifact_dir)

        if result:
            LOGGER.info(message)
        else:
            LOGGER.error(message)
