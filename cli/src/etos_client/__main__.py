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
"""Main executable module."""
import argparse
import sys
import os
import logging
import warnings
import shutil
from pathlib import Path

from etos_lib import ETOS as ETOSLibrary
from etos_client import __version__
from etos_client.events.collector import Collector
from etos_client.etos.schema import RequestSchema
from etos_client.etos import ETOS
from etos_client.test_results import TestResults
from etos_client.downloader import Downloader
from etos_client.event_repository import graphql
from etos_client.test_run import TestRun

LOGGER = logging.getLogger(__name__)
MINUTE = 60
HOUR = MINUTE * 60


def environ_or_required(key: str) -> dict:
    """Get key from environment or make it a required input.

    If a key is available as an environment variable, use it as default.
    Else require the argument to be set.

    Note:
        It is still possible to override the value with input even if the
        environment variable is set.
    """
    if os.getenv(key):
        return {"default": os.getenv(key)}
    return {"required": True}


def parse_args(args: list[str]) -> argparse.Namespace:
    """Parse command line parameters."""
    parser = argparse.ArgumentParser(
        description="Client for executing test automation suites in ETOS"
    )
    parser.add_argument(
        "cluster",
        help="Cluster is the URL to the ETOS API.",
    )
    parser.add_argument(
        "-i",
        "--identity",
        help="Artifact created identity purl or ID to execute test suite on.",
        **environ_or_required("IDENTITY"),
    )
    parser.add_argument(
        "-s",
        "--test-suite",
        help="Test suite execute. Either URL or name.",
        **environ_or_required("TEST_SUITE"),
    )

    parser.add_argument(
        "--no-tty", help="This parameter is no longer in use.", action="store_true"
    )
    parser.add_argument(
        "-w",
        "--workspace",
        default=os.getenv("WORKSPACE", os.getcwd()),
        help="Which workspace to do all the work in.",
    )
    parser.add_argument(
        "-a",
        "--artifact-dir",
        default="artifacts",
        help="Where test artifacts should be stored. Relative to workspace.",
    )
    parser.add_argument(
        "-r",
        "--report-dir",
        default="reports",
        help="Where test reports should be stored. Relative to workspace.",
    )
    parser.add_argument(
        "-d",
        "--download-reports",
        default=None,
        help="This parameter is no longer in use.",
    )

    parser.add_argument(
        "--iut-provider", default="default", help="Which IUT provider to use."
    )
    parser.add_argument(
        "--execution-space-provider",
        default="default",
        help="Which execution space provider to use.",
    )
    parser.add_argument(
        "--log-area-provider", default="default", help="Which log area provider to use."
    )
    parser.add_argument(
        "--dataset",
        action="append",
        help=(
            "Additional dataset information to the environment provider. "
            "Check with your provider which information can be supplied."
        ),
    )

    parser.add_argument(
        "--version",
        action="version",
        version=f"etos_client {__version__}",
    )
    parser.add_argument(
        "-v",
        "--verbose",
        dest="loglevel",
        help="set loglevel to INFO",
        action="store_const",
        default=logging.WARNING,
        const=logging.INFO,
    )
    parser.add_argument(
        "-vv",
        "--very-verbose",
        dest="loglevel",
        help="set loglevel to DEBUG",
        action="store_const",
        default=logging.WARNING,
        const=logging.DEBUG,
    )
    return parser.parse_args(args)


def setup_logging(loglevel: int) -> None:
    """Set up basic logging."""
    logformat = "[%(asctime)s] %(levelname)s:%(message)s"
    logging.basicConfig(
        level=loglevel, stream=sys.stdout, format=logformat, datefmt="%Y-%m-%d %H:%M:%S"
    )


def main(args: list[str]) -> None:
    """Entry point allowing external calls."""
    args = parse_args(args)
    if args.download_reports:
        warnings.warn(
            "The '-d/--download-reports' parameter is deprecated", DeprecationWarning
        )
    if args.no_tty:
        warnings.warn("The '--no-tty' parameter is deprecated", DeprecationWarning)

    setup_logging(args.loglevel)

    artifact_dir = Path(args.workspace).joinpath(args.artifact_dir)
    artifact_dir.mkdir(exist_ok=True)
    report_dir = Path(args.workspace).joinpath(args.report_dir)
    report_dir.mkdir(exist_ok=True)

    LOGGER.info("Running in cluster: %r", args.cluster)
    etos_library = ETOSLibrary("ETOS Client", os.getenv("HOSTNAME"), "ETOS Client")

    etos = ETOS(args.cluster)
    response = etos.start(RequestSchema.from_args(args))
    if not response:
        sys.exit(etos.reason)
    os.environ["ETOS_GRAPHQL_SERVER"] = response.event_repository

    collector = Collector(etos_library, graphql)
    log_downloader = Downloader(report_dir, artifact_dir)
    clear_queue = True
    log_downloader.start()
    try:
        test_run = TestRun(collector, log_downloader)
        test_run.setup_logging(args.loglevel)
        events = test_run.track(etos, 24 * HOUR)
    except SystemExit:
        clear_queue = False
        raise
    finally:
        log_downloader.stop(clear_queue)
        log_downloader.join()

    LOGGER.info("Archiving reports.")
    shutil.make_archive(
        artifact_dir.joinpath("reports").relative_to(Path.cwd()), "zip", report_dir
    )
    LOGGER.info("Reports: %s", report_dir)
    LOGGER.info("Artifacs: %s", artifact_dir)

    if log_downloader.failed:
        sys.exit("ETOS logs did not download successfully.")

    result, message = TestResults().get_results(events)
    if result:
        LOGGER.info(message)
    else:
        LOGGER.error(message)


def run() -> None:
    """Entry point for console_scripts."""
    main(sys.argv[1:])


if __name__ == "__main__":
    run()
