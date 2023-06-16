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
"""
etosctl.

Usage: etosctl [-v|-vv] [options] testrun [<args>...]

Commands:
    testrun       Operate on ETOS testruns

Options:
    -h,--help     Show this screen.
    --version     Print version and exit.
"""
import sys
import logging
from typing import Optional

from docopt import docopt, DocoptExit

import etos_client.test as test

from etosctl.options import GLOBAL_OPTIONS
from etosctl import __version__

LOGGER = logging.getLogger(__name__)


def setup_logging(verbosity: int) -> None:
    """Set up basic logging."""
    loglevel = logging.WARNING
    if verbosity == 1:
        loglevel = logging.INFO
    elif verbosity == 2:
        loglevel = logging.DEBUG

    logformat = "[%(asctime)s] %(levelname)s:%(message)s"
    logging.basicConfig(
        level=loglevel, stream=sys.stdout, format=logformat, datefmt="%Y-%m-%d %H:%M:%S"
    )


def parse_args(argv: list[str], version: Optional[str]) -> dict:
    """Parse arguments for etosctl."""
    options_first = any(cmd in argv for cmd in ("testrun", "config"))
    args = docopt(
        __doc__ + GLOBAL_OPTIONS,
        argv=argv,
        version=version,
        options_first=options_first,
    )

    if args["testrun"]:
        args["<args>"] = ["testrun"] + args["<args>"]
        sub_args = test.parse_args(args["<args>"], version)
    elif args["config"]:
        args["<args>"] = ["config"] + args["<args>"]
        sub_args = config.parse_args(args["<args>"], version)
    else:
        raise DocoptExit()

    # Merge the two arg dictionaries, preferring the dictionary with a value.
    # If both have a value then prefer the sub command argument.
    for key, value in sub_args.items():
        if key in args.keys() and value:
            args[key] = value
        elif key not in args.keys():
            args[key] = value
    return args


def main(argv: list[str]) -> None:
    """Entry point allowing external calls."""
    args = parse_args(argv, __version__)

    setup_logging(args["-v"])

    if args["testrun"]:
        test.main(args)
    else:
        raise DocoptExit()


def run() -> None:
    """Entry point for console_scripts."""
    main(sys.argv[1:])


if __name__ == "__main__":
    run()
