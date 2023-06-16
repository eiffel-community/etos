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

Usage: etosctl [-v|-vv] [options] <command> [<args>...]

Commands:
%s
Options:
    -h,--help        Show this screen.
    --version        Print version and exit.
"""
import sys
import logging
import importlib
import pkgutil
from typing import Optional

from docopt import docopt, DocoptExit

from etos_client.test import TestRun
from etosctl.options import GLOBAL_OPTIONS
from etosctl.plugin import PLUGINS
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


def parse_args(argv: list[str], commands: dict, version: Optional[str]) -> dict:
    """Parse arguments for etosctl."""
    commands_docs = ""
    for name, command in commands.items():
        commands_docs += f"    {name:<16} {command.description}\n"

    options_first = any(cmd in argv for cmd in commands.keys())
    args = docopt(
        __doc__ % commands_docs + GLOBAL_OPTIONS,
        argv=argv,
        version=version,
        options_first=options_first,
    )
    command = commands.get(args["<command>"])
    if command is None:
        sys.stderr.write(f"Command {args['<command>']!r} not found\n")
        raise DocoptExit()
    args["<args>"] = [args["<command>"]] + args["<args>"]
    sub_args = command.parse_args(args["<args>"], version)

    # Merge the two arg dictionaries, preferring the dictionary with a value.
    # If both have a value then prefer the sub command argument.
    for key, value in sub_args.items():
        if key in args.keys() and value:
            args[key] = value
        elif key not in args.keys():
            args[key] = value
    return args


def load_plugins() -> dict:
    """Load etosctl plugins."""
    # Make sure we import all plugins, the plugins themselves should
    # register in order to be loaded into ETOS cli.
    _ = {
        name: importlib.import_module(name)
        for _, name, _ in pkgutil.iter_modules()
        if name.startswith("etos_") and name.endswith("_plugin")
    }
    return PLUGINS


def main(argv: list[str]) -> None:
    """Entry point allowing external calls."""
    commands = {"testrun": TestRun("testrun")} | load_plugins()
    args = parse_args(argv, commands, __version__)

    setup_logging(args["-v"])

    command = commands.get(args["<command>"])
    # None verification of command has already been done in parse_args.
    command.main(args)


def run() -> None:
    """Entry point for console_scripts."""
    main(sys.argv[1:])


if __name__ == "__main__":
    run()
