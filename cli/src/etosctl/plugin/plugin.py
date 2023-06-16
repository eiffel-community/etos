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
"""Etosctl plugin.

ETOS will load all plugins that register themselves to the Plugins list.

>>> from etosctl.plugin import register
>>> register("myplugin", TestPlugin)
"""
from typing import Optional

PLUGINS = {}


class Plugin:
    """Base plugin class."""

    description = "Short description of plugin command"

    def __init__(self, name: str) -> None:
        """Initialize plugin."""
        self.name = name

    def __str__(self) -> str:
        """Representation of plugin."""
        return self.name

    def parse_args(self, argv: list[str], version: Optional[str]) -> dict:
        """Parse arguments for this etosctl plugin."""
        raise NotImplementedError(f"Plugin {self.name!r} has not been properly loaded")

    def main(self, args: dict) -> None:
        """Run command from plugin."""
        raise NotImplementedError(f"Plugin {self.name!r} has not been properly loaded")


def register(name: str, cls: Plugin):
    """Register a plugin into etosctl."""
    PLUGINS[name] = cls(name)
