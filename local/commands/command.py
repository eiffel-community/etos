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
"""Base command."""
from typing import Union

from local.utilities.result import Result
from local.utilities.store import Value, get_values

MINUTE = 60


class Command:
    """Base command for all commands to inherit."""

    default_timeout = 2 * MINUTE

    def __init__(
        self,
        command: Union[list[str | Value], "Command"],
        timeout: int = default_timeout,
    ):
        self.command = command
        self.default_timeout = timeout

    def execute(self) -> Result:
        """Execute command and return Result. Override this."""
        raise NotImplementedError

    def __repr__(self) -> str:
        if isinstance(self.command, list):
            return " ".join(get_values(*self.command))
        return repr(self.command)
