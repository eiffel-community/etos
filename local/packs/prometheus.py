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
"""Prometheus pack."""
from local.commands.command import Command
from local.commands.kubectl import Kubectl, Resource

from .base import BasePack


class Prometheus(BasePack):
    """Pack for deploying prometheus."""

    # Same version that the e2e tests use
    version = "v0.77.1"
    url = f"https://github.com/prometheus-operator/prometheus-operator/releases/download/{version}/bundle.yaml"

    def name(self) -> str:
        """Name of pack."""
        return "Prometheus"

    def create(self) -> list[Command]:
        """Commands for deploying prometheus."""
        return [Kubectl().create(Resource(filename=self.url))]

    def delete(self) -> list[Command]:
        """Commands for deleting prometheus."""
        return [Kubectl().delete(Resource(filename=self.url))]
