# -*- coding: utf-8 -*-
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
"""Main executable module."""

import argparse
import sys
import os
import logging
import time
import json

import requests
from halo import Halo
from etos_lib.etos import ETOS

from etos_client import __version__
from etos_client.client import ETOSClient
from etos_client.lib import ETOSTestResultHandler, ETOSLogHandler

_logger = logging.getLogger(__name__)


def environ_or_required(key):
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


def parse_args(args):
    """Parse command line parameters.

    Args:
      args ([str]): command line parameters as list of strings

    Returns:
      :obj:`argparse.Namespace`: command line parameters namespace
    """
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
        "--no-tty", help="Disable features requiring a tty.", action="store_true"
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
        help="Should we download reports. Can be 'yes', 'y', 'no', 'n'.",
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


def setup_logging(loglevel):
    """Set up basic logging.

    Args:
      loglevel (int): minimum loglevel for emitting messages
    """
    logformat = "[%(asctime)s] %(levelname)s:%(message)s"
    logging.basicConfig(
        level=loglevel, stream=sys.stdout, format=logformat, datefmt="%Y-%m-%d %H:%M:%S"
    )


def check_etos_connectivity(url):
    """Check the connection to ETOS.

    :param url: URL to test connection to.
    :type url: str
    """
    try:
        response = requests.get(url, timeout=5)
        response.raise_for_status()
    except Exception as exception:  # pylint:disable=broad-except
        raise Exception(
            "Unable to connect to ETOS. Please check your network connection."
        ) from exception


class Printer:
    """Generic printing class matching the interface of halo."""

    __text = None
    timeout = 60
    timer = None

    def __init__(self, text, **_):
        """Initialize the printer class.

        :param text: Initial text.
        :type text: str
        """
        self.timer = time.time() + self.timeout
        self.text = text

    def __enter__(self):
        """Enter printer context."""
        return self

    @property
    def text(self):
        """Printer text."""
        return self.__text

    @text.setter
    def text(self, text):
        """Set text for this object.

        Don't reprint unless the text is new or a timeout has been reached.
        This is to disable unecessary text spam into the console log.

        :param text: Text to print.
        :type text: str
        """
        timeout_reached = time.time() > self.timer
        if text != self.__text or timeout_reached:
            self.timer = time.time() + self.timeout
            self.info(text)
        self.__text = text

    @staticmethod
    def info(text):
        """Print text to stdout.

        :param text: Text to print.
        :type text: str
        """
        _logger.info(text)

    @staticmethod
    def fail(text):
        """Print text to stdout.

        :param text: Text to print.
        :type text: str
        """
        _logger.error(text)

    @staticmethod
    def warn(text):
        """Print text to stdout.

        :param text: Text to print.
        :type text: str
        """
        _logger.warning(text)

    succeed = info

    def start(self, text=None):
        """Set starting text."""
        if text is not None:
            self.text = text

    def __exit__(self, *_, **__):
        """Exit printer context."""


def generate_spinner(no_tty):
    """If no tty, return a generic 'printer' class. Else return halo.

    :param no_tty: Whether the executor has an interactive tty or not.
    :type no_tty: bool
    :return: Spinner text item.
    :rtype: :obj:`Spinner`
    """
    if no_tty:
        return Printer
    return Halo


def main(args):  # pylint:disable=too-many-statements
    """Entry point allowing external calls.

    Args:
      args ([str]): command line parameter list
    """
    args = parse_args(args)
    etos = ETOS("ETOS Client", os.getenv("HOSTNAME"), "ETOS Client")

    setup_logging(args.loglevel)
    info = generate_spinner(args.no_tty)

    for key, value in args._get_kwargs():  # pylint:disable=protected-access
        etos.config.set(key, value)

    etos.config.set("artifact_identifier", args.identity)
    etos.config.set("dataset", [json.loads(dataset) for dataset in args.dataset] or {})

    with info(text="Checking connectivity to ETOS", spinner="dots") as spinner:
        spinner.info(f"Running in cluster: {args.cluster!r}")
        spinner.info("Configuration:")
        spinner.info(f"{etos.config.config}")
        try:
            check_etos_connectivity(f"{args.cluster}/selftest/ping")
        except Exception as exception:  # pylint:disable=broad-except
            spinner.fail(str(exception))
            sys.exit(1)
        spinner.start()
        spinner.succeed("Connection successful.")
        spinner.succeed("Ready to launch ETOS.")

        # Start execution
        etos_client = ETOSClient(etos, args.cluster)
        spinner.start("Triggering ETOS.")
        success = etos_client.start(spinner)
        if not success:
            # Unix  : 0 == Success, 1 == Fail
            # Python: 1 == True   , 0 == False
            sys.exit(not success)

        spinner.info(f"Suite ID: {etos_client.test_suite_id}")
        spinner.info(f"Artifact ID: {etos_client.artifact_id}")
        spinner.info(f"Purl: {etos_client.artifact_identity}")

        etos.config.set("suite_id", etos_client.test_suite_id)
        os.environ["ETOS_GRAPHQL_SERVER"] = etos_client.event_repository
        spinner.info(f"Event repository: {etos.debug.graphql_server!r}")

        # Wait for test results
        test_result_handler = ETOSTestResultHandler(etos)
        spinner.start("Waiting for ETOS.")
        success, results, canceled = test_result_handler.wait_for_test_suite_finished(
            spinner
        )
        if not success:
            spinner.fail(results)
            if canceled:
                sys.exit(canceled)
        else:
            spinner.succeed(results)

        # Download reports
        if args.download_reports is None:
            answer = input(
                "Do you want to download all logs for this test execution? (y/n): "
            )
            while answer.lower() not in ("y", "yes", "no", "n"):
                print("Please answer 'yes' or 'no'")
                answer = input()
        else:
            answer = args.download_reports
        if answer.lower() in ("y", "yes"):
            log_handler = ETOSLogHandler(etos, test_result_handler.events)
            spinner.start("Downloading test logs.")
            logs_downloaded_successfully = log_handler.download_logs(spinner)
            if not logs_downloaded_successfully:
                sys.exit("ETOS logs did not download successfully.")


def run():
    """Entry point for console_scripts."""
    main(sys.argv[1:])


if __name__ == "__main__":
    run()
