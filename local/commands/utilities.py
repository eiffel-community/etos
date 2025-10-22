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
"""Utility commands."""
import logging
import time

from local.utilities.result import Result
from local.utilities.store import Store

from .command import Command


class WaitUntil(Command):
    """Wait until a command has result.code 0.

    Default timeout is 2 minutes and default interval is 1 second.
    """

    logger = logging.getLogger(__name__)
    # Force the type to be Command when running WaitUntil.
    command: Command

    def __init__(self, command: Command, *args, interval: int = 1, **kwargs):
        super().__init__(command, *args, **kwargs)
        self.command = command
        assert isinstance(self.command, Command)
        self.interval = interval

    def execute(self) -> Result:
        """Execute a command until it succeeds or timeout is reached.

        If command times out return with a failed Result.
        """
        end = time.time() + self.default_timeout
        result = Result(code=128, stderr="No command was executed at all")
        self.logger.info(
            "Waiting for command %r, timeout=%ds (interval=%ds)",
            self.command,
            self.default_timeout,
            self.interval,
        )
        while time.time() < end:
            result = self.command.execute()
            if result.code == 0:
                return result
            self.logger.debug("Command failed, trying again in %ds", self.interval)
            self.logger.debug("stdout: %r", result.stdout)
            self.logger.debug("stderr: %r", result.stderr)
            time.sleep(self.interval)
        self.logger.error("Timed out waiting for command to succeed")
        return result


class HasLines(Command):
    """Check number of lines of a commands stdout."""

    logger = logging.getLogger(__name__)
    # Force the type to be Command when running HasLines.
    command: Command

    def __init__(self, command: Command, *args, length: int, **kwargs):
        super().__init__(command, *args, **kwargs)
        self.command = command
        assert isinstance(self.command, Command)
        self.length = length

    def execute(self) -> Result:
        """Execute a stored command, split stdout on newlines and check the length.

        If the stdout does not match the number of lines, then exit with a failed Result.
        """
        result = self.command.execute()
        if result.code != 0:
            return result
        if result.stdout is None:
            return Result(code=128, stderr="There's no text in stdout")
        lines = len(result.stdout.splitlines())
        if lines != self.length:
            return Result(
                code=128,
                stdout=result.stdout,
                stderr=f"Length {lines} does not match {self.length}",
            )
        return result


class StdoutEquals(Command):
    """Test if stdout of a command matches a value."""

    # Force the type to be Command when running StdoutEquals.
    command: Command

    def __init__(self, command: Command, *args, value: str, **kwargs):
        super().__init__(command, *args, **kwargs)
        self.command = command
        assert isinstance(self.command, Command)
        self.value = value

    def execute(self) -> Result:
        """Execute a command and match its output against a stored value.

        If stdout does not match the value, then exit with a failed Result.
        """
        result = self.command.execute()
        if result.code != 0:
            return result
        if result.stdout is None:
            return Result(code=128, stderr="There's no text in stdout")
        if result.stdout != self.value:
            return Result(
                code=128,
                stdout=result.stdout,
                stderr=f"Text in stdout does not match value {self.value!r}",
            )
        return result


class StdoutLength(Command):
    """Check if the stdout of a command matches length."""

    # Force the type to be Command when running StdoutLength.
    command: Command

    def __init__(self, command: Command, *args, length: int, **kwargs):
        super().__init__(command, *args, **kwargs)
        self.command = command
        assert isinstance(self.command, Command)
        self.length = length

    def execute(self) -> Result:
        """Execute a command and check the stdout length.

        If stdout length does not match the expected length, then exit with a failed Result.
        """
        result = self.command.execute()
        if result.code != 0:
            return result
        if result.stdout is None:
            return Result(code=128, stderr="There's no text in stdout")
        length = len(result.stdout)
        if length < self.length:
            return Result(
                code=128,
                stdout=result.stdout,
                stderr=f"Length of stdout does not match length {self.length}",
            )
        return result


class StoreStdout(Command):
    """Store the stdout of a command."""

    # Force the type to be Command when running StoreStdout.
    command: Command

    def __init__(self, command: Command, *args, key: str, store: Store, **kwargs):
        super().__init__(command, *args, **kwargs)
        self.command = command
        assert isinstance(self.command, Command)
        self.key = key
        self.store = store

    def execute(self) -> Result:
        """Execute command and store its stdout."""
        result = self.command.execute()
        self.store[self.key] = result.stdout
        return result
