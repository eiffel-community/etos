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
"""Command registry and command bases."""
from typing import List, Any
import inspect

from docopt import docopt, DocoptExit

from etosctl.options import GLOBAL_OPTIONS
from etosctl.models import CommandMeta


class ICommandRegistry(type):
    """Command registry for etosctl commands."""

    commands: List = []

    def __init__(cls, name: str, bases: tuple[type, ...], dict: dict[str, Any]) -> None:
        """Add command to registry."""
        super().__init__(cls)
        if name not in ("Command", "MainCommand"):
            ICommandRegistry.commands.append(cls)

    @classmethod
    def reset(cls):
        """Reset the command registry."""
        cls.commands = []


class BaseCommand:
    """A new command line command for etosctl."""

    meta: CommandMeta = CommandMeta(
        name="mycommand",
        description="this is my command",
        version="v1alpha1",
    )
    subcommand: "Command" = None

    def __init__(self, parent: "Command" = None) -> None:
        """Clean up docstring and add global options."""
        self.__doc__ = inspect.cleandoc(self.__doc__) + GLOBAL_OPTIONS
        self.parent = parent

    def _command_tree(self, args: dict) -> list:
        """Get the complete command tree for a sub command."""
        if self.parent is None:
            return []
        return self.parent._command_tree(args) + [self.meta.name]

    def _parse_subcommand_args(self, args: dict) -> dict:
        """Parse command line arguments for sub commands."""
        subcommand = self.meta.subcommands.get(args.get("<command>"))
        if subcommand:
            self.subcommand = subcommand(self)
            sub_args = self.subcommand.parse_args(args["<args>"])
            return self._merge_args(args, sub_args)
        return args

    def _merge_args(self, args: dict, sub_args: dict) -> dict:
        """Merge command line arguments and sub command line arguments."""
        # Merge the two arg dictionaries, preferring the dictionary with a value.
        # If both have a value then prefer the sub command argument.
        for key, value in sub_args.items():
            if key in args.keys() and value:
                args[key] = value
            elif key not in args.keys():
                args[key] = value
        return args

    def _options_first(self, argv: list[str]) -> bool:
        """Options first parameter for docopt."""
        if self.meta.subcommands:
            return any(cmd in argv for cmd in self.meta.subcommands.keys())
        return False

    def parse_args(self, argv: list[str]) -> dict:
        """Parse command line arguments."""
        argv = self._command_tree(argv) + argv

        args = docopt(
            self.__doc__,
            argv=argv,
            version=self.meta.version if self.meta else "Unknown",
            options_first=self._options_first(argv),
        )
        if args.get("<command>") and self.meta.subcommands:
            return self._parse_subcommand_args(args)
        return args

    def run(self, args: dict):
        """Run command. Shall be overriden if there are no sub commands to run."""
        if self.subcommand is not None:
            return self.subcommand.run(args)
        raise DocoptExit("command not found")


class Command(BaseCommand, metaclass=ICommandRegistry):
    """Command line command for etosctl.

    Gets registered automatically into the command registry and loaded
    into etosctl as long as it has been imported. Etosctl will import it
    automatically if it's in an etos_* package.
    """


class SubCommand(BaseCommand):
    """A sub command for etosctl.

    Shall be used in conjuction with a Command in order for it to work properly.
    Loading a sub command can be done like this

    >>> class MyCommand(Command):
            meta: CommandMeta = CommandMeta(
                name="mycommand",
                description="A command",
                version="v1alpha1",
                subcommands={"mysubcommand": MySuBCommand}
            )

    A sub command does not get automatically loaded into etosctl unless it is part
    of the CommandMeta dataclass.
    """
