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
"""Local ETOS deployment verification."""
from local.commands.command import MINUTE, Command
from local.commands.kubectl import Kubectl, Resource
from local.commands.utilities import StdoutEquals, WaitUntil

from .base import BasePack


class Verify(BasePack):
    """Verify the deployment of ETOS by executing a TestRun."""

    test_run_sample = "config/samples/etos_v1alpha1_testrun.yaml"

    def name(self) -> str:
        """Name of the pack."""
        return "Verify"

    def create(self) -> list[Command]:
        """Commands for creating and waiting for an ETOS testrun."""
        kubectl = Kubectl()
        return [
            kubectl.create(
                Resource(
                    filename=self.test_run_sample,
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
            WaitUntil(
                StdoutEquals(
                    kubectl.get(
                        Resource(
                            type="testrun",
                            names="testrun-sample",
                            namespace=self.local_store["cluster_namespace"],
                        ),
                        output="jsonpath={.status.verdict}",
                    ),
                    value="Passed",
                ),
                timeout=5 * MINUTE,
            ),
            Kubectl().delete(
                Resource(
                    filename=self.test_run_sample,
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
        ]

    def delete(self) -> list[Command]:
        """Commands for deleting created testruns."""
        return [
            Kubectl().delete(
                Resource(
                    filename=self.test_run_sample,
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
        ]
