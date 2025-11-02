# Copyright Axis Communications AB.
#
# For a full list of individual contributors, please see the commit history.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# 	http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Local ETOS deployment."""
import argparse
import logging
import os
import sys
from contextlib import contextmanager
from pathlib import Path
from typing import Type

from .commands.command import Command
from .executor import Executor, Fail
from .packs import PACKS
from .packs.base import BasePack
from .utilities.store import Store

LOGGER = logging.getLogger(__name__)
MAX_LOG_LEVEL = logging.WARNING
REPO_PATH = Path(__file__).parent.parent


def setup_logging(loglevel: int):
    """Setup logging for the client."""
    logformat = "%(asctime)s %(levelname)s: %(message)s"
    logging.basicConfig(stream=sys.stdout, level=loglevel, format=logformat)


def parse_args() -> argparse.Namespace:
    """Parse input arguments."""
    parser = argparse.ArgumentParser(description="ETOS local bootstrap")
    parser.add_argument(
        "direction",
        type=str,
        choices=["up", "down"],
        help="Which direction to deploy ETOS, up or down",
    )
    parser.add_argument(
        "-r",
        "--rollback",
        action="store_true",
        help="Whether or not to rollback changes on failure (only affects direction=up)",
    )
    parser.add_argument(
        "-i",
        "--ignore-errors",
        action="store_true",
        help="Whether or not to continue after an error. Only affects direction=down and during rollback",
    )
    parser.add_argument(
        "-v", "--verbose", action="count", default=0, help="Enable more verbose output"
    )
    return parser.parse_args()


def loglevel(level: int) -> int:
    """Convert loglevel count input to a loglevel in logging."""
    return MAX_LOG_LEVEL - (level * 10)


def get_packs() -> list[Type[BasePack]]:
    """Get all packs that we want to deploy."""
    LOGGER.debug("Fetching all packs")
    return PACKS


def deploy_commands(packs: list[Type[BasePack]], store: Store) -> list[Command]:
    """Extract deploy commands from a list of uninitialized packs."""
    LOGGER.info("Deploying ETOS local")
    commands = []
    for uninitialized_pack in packs:
        pack = uninitialized_pack(store)
        LOGGER.info("Loading create commands from pack %r", pack.name())
        commands.extend(pack.create())
    return commands


def undeploy_commands(packs: list[Type[BasePack]], store: Store) -> list[Command]:
    """Extract undeploy commands from a list of uninitialized packs."""
    LOGGER.info("Undeploying ETOS local")
    commands = []
    for uninitialized_pack in reversed(packs):
        pack = uninitialized_pack(store)
        LOGGER.info("Loading delete commands from pack %r", pack.name())
        commands.extend(pack.delete())
    return commands


def deploy(commands: list[Command], rollback_commands: list[Command]):
    """Deploy by running a list of commands."""
    LOGGER.info("Deploying ETOS on kind")
    try:
        Executor(commands).execute()
    except Fail:
        LOGGER.critical("Failed to deploy ETOS!")
        if rollback_commands:
            LOGGER.info("Rolling back changes made")
            Executor(rollback_commands).execute(ignore_errors=True)
        raise
    LOGGER.info("ETOS is up and running and ready for use, happy testing")


def undeploy(commands: list[Command], ignore_errors: bool):
    """Undeploy by running a list of commands."""
    LOGGER.info("Undeploying ETOS from kind")
    try:
        Executor(commands).execute(ignore_errors)
    except Fail:
        LOGGER.critical("Failed to undeploy ETOS!")
        raise
    LOGGER.info("ETOS has now been completely undeployed")


@contextmanager
def chdir(x):
    """Change directory as a contextmanager."""
    d = os.getcwd()
    os.chdir(x)
    try:
        yield
    finally:
        os.chdir(d)


def run(args: argparse.Namespace):
    """Run the local deployment program."""

    setup_logging(loglevel(args.verbose))
    store = Store()
    store["namespace"] = "etos-system"
    store["cluster_namespace"] = "etos-test"
    store["cluster_name"] = "cluster-sample"
    store["project_image"] = "example.com/etos:v0.0.1"
    store["artifact_id"] = "268dd4db-93da-4232-a544-bf4c0fb26dac"
    store["artifact_identity"] = "pkg:testrun/etos/eiffel_community"

    try:
        with chdir(REPO_PATH):
            if args.direction.lower() == "up":
                rollback = []
                if args.rollback:
                    rollback = undeploy_commands(get_packs(), store)
                deploy(deploy_commands(get_packs(), store), rollback)
            else:
                undeploy(undeploy_commands(get_packs(), store), args.ignore_errors)
    except Fail as failure:
        LOGGER.critical("stderr: %s", failure.result.stderr)
        raise SystemExit(1) from failure


def main():
    """Main entrypoint to program."""
    run(parse_args())


if __name__ == "__main__":
    main()
