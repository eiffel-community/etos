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
"""Local ETOS make wrapper."""
import logging

from local.utilities.store import Value

from .command import Command
from .shell import Shell


class Make:
    """Wrapper to the shell command 'make'."""

    logger = logging.getLogger(__name__)

    def install(self) -> Command:
        """Command for running 'make install'."""
        return Shell(["make", "install"])

    def uninstall(self, ignore_not_found: bool = True) -> Command:
        """Command for running 'make uninstall'."""
        return Shell(
            ["make", "uninstall", f"ignore-not-found={str(ignore_not_found).lower()}"]
        )

    def deploy(self, image: str | Value) -> Command:
        """Command for running 'make deploy'."""
        # TODO: This only works because the image value is set in main, no runtime variables work here
        return Shell(["make", "deploy", f"IMG={image}"])

    def undeploy(self, ignore_not_found: bool = True) -> Command:
        """Command for running 'make undeploy'."""
        return Shell(
            ["make", "undeploy", f"ignore-not-found={str(ignore_not_found).lower()}"]
        )
