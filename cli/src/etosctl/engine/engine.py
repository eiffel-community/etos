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
"""Customization engine."""
import pkgutil
import importlib
from etosctl.command import ICommandRegistry


class CustomizationEngine:
    """Engine for loading custom code into etosctl."""

    def __init__(self) -> None:
        """Initialize the command dictionary."""
        self.commands = {}

    def start(self) -> None:
        """Start up the customization engine."""
        self.__discover()

    def __discover(self) -> None:
        """Discover and load plugins and custom commands."""
        _ = {
            name: importlib.import_module(name)
            for _, name, _ in pkgutil.iter_modules()
            if name.startswith("etos_")
        }
        for command in ICommandRegistry.commands:
            assert (
                command.meta.name not in self.commands.keys()
            ), f"A command with name {command.meta.name!r} already exists"
            self.commands[command.meta.name] = command
        ICommandRegistry.reset()
