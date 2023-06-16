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
"""ETOS client testrun.

Usage: etosctl testrun [-v|-vv] [-h] [--version] [--help] start [<args>...]

Commands:
    start         Start a new ETOS testrun

Options:
    -h,--help     Show this screen
    --version     Print version and exit
"""
import logging
from typing import Optional

from docopt import docopt, DocoptExit

from etosctl.options import GLOBAL_OPTIONS
from etosctl.plugin import Plugin
from etos_client import start

LOGGER = logging.getLogger(__name__)


class TestRun(Plugin):
    """ETOS client testrun."""

    description = "Operate on ETOS testruns"

    def parse_args(self, argv: list[str], version: Optional[str]) -> dict:
        """Parse arguments for etosctl testrun."""
        options_first = any(cmd in argv for cmd in ("start",))
        args = docopt(
            __doc__ + GLOBAL_OPTIONS,
            argv=argv,
            version=version,
            options_first=options_first,
        )

        if args["start"]:
            args["<args>"] = ["testrun", "start"] + args["<args>"]
            return start.parse_args(args["<args>"], version)
        raise DocoptExit()

    def main(self, args: dict) -> None:
        """Manage testruns in etosctl."""
        if args["start"]:
            start.main(args)
        else:
            raise DocoptExit()
