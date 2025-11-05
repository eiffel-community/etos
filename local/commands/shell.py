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
"""Local ETOS shell command wrapper."""
import logging
from subprocess import run

from local.utilities.result import Result
from local.utilities.store import get_values

from .command import Command


class Shell(Command):
    """Wrapper for subprocess.run."""

    logger = logging.getLogger(__name__)

    def execute(self) -> Result:
        """Execute command."""
        assert isinstance(self.command, list)
        return self.__run(get_values(*self.command))

    def __clean(self, output: str) -> str:
        """Clean up output from subprocess.run."""
        return "\n".join(
            [line.strip() for line in output.strip('"').strip("'").split("\n") if line]
        )

    def __run(self, command: list[str]) -> Result:
        """Run a shell command using subprocess.run, returning Result."""
        process = run(
            command,
            capture_output=True,
            timeout=self.default_timeout,
            universal_newlines=True,
        )
        return Result(
            process.returncode,
            stderr=self.__clean(process.stderr),
            stdout=self.__clean(process.stdout),
        )
