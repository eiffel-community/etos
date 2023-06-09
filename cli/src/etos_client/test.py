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

Usage: etosctl testrun [-v|-vv] [-h] [--version] [--help] (start|attach) [<args>...]

Commands:
    start         Start a new ETOS testrun

Options:
    -h,--help     Show this screen
    -v,-vv        Increase loglevel.
    --version     Print version and exit
"""
import logging
from docopt import docopt, DocoptExit
from etos_client import start

LOGGER = logging.getLogger(__name__)


def main(argv: list[str], version: str):
    """Manage testruns in etosctl."""
    # Options first is set to allow sub-commands to print their help.
    # However if options first is set and we try to do 'etosctl testrun --help'
    # the text help would be extremely lackluster.
    options_first = any(cmd in argv for cmd in ("start",))
    args = docopt(__doc__, argv=argv, version=version, options_first=options_first)

    cmd = None
    if args["start"]:
        start.main(["testrun", "start"] + args["<args>"], version)
    else:
        raise DocoptExit()
