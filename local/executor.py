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
"""Command executor."""
import logging

from local.commands.command import Command
from local.utilities.result import Result


class Fail(Exception):
    """Exception to raise when a command fails.

    Exception contains a `Result` with information about the command execution,
    such as stdout, stderr and code.
    """

    result: Result

    def __init__(self, result: Result, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.result = result


class Executor:
    """Executor to execute a list of commands in order."""

    logger = logging.getLogger(__name__)

    def __init__(self, commands: list[Command]):
        self.commands = commands

    def execute(self, ignore_errors=False):
        """Execute all commands."""
        self.logger.info("Executing all commands")
        failed = False
        for command in self.commands:
            self.logger.info("Executing command: %r", command)
            try:
                self.handle_result(command.execute())
            except Fail:
                failed = True
                if ignore_errors:
                    self.logger.warning("Error executing command: %r", command)
                    continue
                raise
        if failed:
            self.logger.warning("Did not successfully execute all commands")
            return
        self.logger.info("Successfully executed all commands")

    def handle_result(self, result: Result):
        """Handle the result of a command, by logging and raising Fail."""
        if result.code != 0:
            self.logger.info("Result for pack: code: %d", result.code)
            if result.stdout:
                self.logger.info(result.stdout)
            if result.stderr:
                self.logger.error(result.stderr)
            raise Fail(result)
        self.logger.debug("Result for pack: code: %d", result.code)
        if result.stdout:
            self.logger.debug(result.stdout)
        if result.stderr:
            self.logger.warning(result.stderr)
