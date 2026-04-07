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
"""Local ETOS BasePack."""
from local.commands.command import Command
from local.utilities.store import Store


class BasePack:
    """Base pack implementation. Does nothing."""

    def __init__(self, store: Store):
        self.local_store = store

    def name(self) -> str:
        """Name of pack."""
        return "Base"

    def create(self) -> list[Command]:
        """Create nothing. Override this."""
        return []

    def delete(self) -> list[Command]:
        """Delete nothing. Override this."""
        return []
