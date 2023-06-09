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

Usage: etosctl [-v|-vv] [-h] [--version] [--help] testrun [<args>...]

Commands:
    testrun       Operate on ETOS testruns

Options:
    -h,--help     Show this screen.
    -v|-vv        Increase loglevel.
    --version     Print version and exit.
"""
import sys
import logging

from docopt import docopt, DocoptExit

import etos_client.test as test
from etosctl import __version__

LOGGER = logging.getLogger(__name__)


def setup_logging(loglevel: int) -> None:
    """Set up basic logging."""
    logformat = "[%(asctime)s] %(levelname)s:%(message)s"
    logging.basicConfig(
        level=loglevel, stream=sys.stdout, format=logformat, datefmt="%Y-%m-%d %H:%M:%S"
    )


def main(argv: list[str]) -> None:
    """Entry point allowing external calls."""
    options_first = any(cmd in argv for cmd in ("testrun",))
    args = docopt(__doc__, argv=argv, version=__version__, options_first=options_first)
    loglevel = logging.WARNING
    if args["-v"] == 1 or "-v" in args["<args>"]:
        loglevel = logging.INFO
    elif args["-v"] == 2 or "-vv" in args["<args>"]:
        loglevel = logging.DEBUG
    setup_logging(loglevel)

    if args["testrun"]:
        test.main(["testrun"] + args["<args>"], __version__)
    else:
        raise DocoptExit()


def run() -> None:
    """Entry point for console_scripts."""
    main(sys.argv[1:])


if __name__ == "__main__":
    run()
