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
"""Etosctl command handler."""
import sys
import logging
import inspect

from typing import Optional

from .engine import CustomizationEngine
from .command import Command
from .models import CommandMeta


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
    logging.getLogger("gql").setLevel(logging.WARNING)


class Main(Command):
    """
    etosctl.

    Usage: etosctl [-v|-vv] [options] <command> [<args>...]

    Commands:
    %s

    Options:
        -h,--help        Show this screen.
        --version        Print version and exit.
    """

    meta: CommandMeta = CommandMeta(
        name="etosctl",
        description="etosctl",
        version="v1alpha1",
        subcommands={},
    )

    def __init__(
        self,
        parent: Optional[Command] = None,
        commands: Optional[dict[str, Command]] = None,
        argv: list[str] = None,
    ) -> None:
        """Load commands into etosctl as sub commands."""
        super().__init__(parent)
        if commands:
            self.load_commands(commands)
        subcommands = "\n".join(
            f"    {cmd.meta.name:<15}  {cmd.meta.description}"
            for cmd in self.meta.subcommands.values()
        )
        self.__doc__ = self.__doc__ % subcommands
        self.argv = argv or []

    def load_commands(self, commands: dict) -> None:
        """Load commands into etosctl as sub commands."""
        for name, command in commands.items():
            if type(self) == command:
                continue
            self.meta.subcommands[name] = command

    def start(self) -> None:
        """Parse input arguments and run etosctl."""
        args = self.parse_args(self.argv)
        setup_logging(args["-v"])
        self.run(args)


def main(argv: list[str]) -> None:
    """Entry point allowing external calls."""
    engine = CustomizationEngine()
    engine.start()
    main = Main(commands=engine.commands, argv=argv)
    main.start()


def run() -> None:
    """Entry point for console_scripts."""
    main(sys.argv[1:])


if __name__ == "__main__":
    run()
