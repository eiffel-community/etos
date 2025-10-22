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
"""CertManager pack."""
from local.commands.command import MINUTE, Command
from local.commands.kubectl import Kubectl, Resource

from .base import BasePack


class CertManager(BasePack):
    """Pack for deploying cert manager."""

    version = "v1.16.3"
    url = f"https://github.com/cert-manager/cert-manager/releases/download/{version}/cert-manager.yaml"
    cert_manager_controller_lease = "cert-manager-controller"
    cert_manager_ca_injector_lease = "cert-manager-cainjector-leader-election"

    def name(self) -> str:
        """Name of pack."""
        return "CertManager"

    def create(self) -> list[Command]:
        """Commands for deploying cert-manager."""
        kubectl = Kubectl()
        return [
            kubectl.create(Resource(filename=self.url)),
            kubectl.wait(
                Resource(
                    type="deployment.apps",
                    names="cert-manager-webhook",
                    namespace="cert-manager",
                ),
                wait_for="condition=Available",
                timeout=5 * MINUTE,
            ),
        ]

    def delete(self) -> list[Command]:
        """Commands for deleting cert-manager and its leases."""
        kubectl = Kubectl()
        return [
            kubectl.delete(Resource(filename=self.url)),
            kubectl.delete(
                Resource(
                    type="leases",
                    names=[
                        self.cert_manager_controller_lease,
                        self.cert_manager_ca_injector_lease,
                    ],
                    namespace="kube-system",
                )
            ),
        ]
