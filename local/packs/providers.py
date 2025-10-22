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
"""Local ETOS provider pack."""
from local.commands.command import Command
from local.commands.kubectl import Kubectl, Resource
from local.commands.utilities import StdoutEquals, WaitUntil

from .base import BasePack


class Providers(BasePack):
    """Providers pack to deploy provider services and provider resources."""

    iut_provider_sample = "config/samples/etos_v1alpha1_iut_provider.yaml"
    execution_space_provider_sample = (
        "config/samples/etos_v1alpha1_execution_space_provider.yaml"
    )
    log_area_provider_sample = "config/samples/etos_v1alpha1_log_area_provider.yaml"
    iut_provider_kustomization = "testdata/iut"
    execution_space_provider_kustomization = "testdata/executionspace"

    def name(self) -> str:
        """Name of pack."""
        return "Providers"

    def create(self) -> list[Command]:
        """Commands for creating provider services and provider resources."""
        kubectl = Kubectl()
        return [
            kubectl.create(
                Resource(
                    kustomize=self.execution_space_provider_kustomization,
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
            WaitUntil(
                StdoutEquals(
                    kubectl.get(
                        Resource(
                            type="deploy",
                            names="etos-executionspace",
                            namespace=self.local_store["cluster_namespace"],
                        ),
                        output="jsonpath='{.status.readyReplicas}'",
                    ),
                    value="1",
                )
            ),
            kubectl.create(
                Resource(
                    filename=self.execution_space_provider_sample,
                    namespace=self.local_store["cluster_namespace"],
                ),
            ),
            WaitUntil(
                StdoutEquals(
                    kubectl.get(
                        Resource(
                            type="provider",
                            names="execution-space-provider-sample",
                            namespace=self.local_store["cluster_namespace"],
                        ),
                        output="jsonpath={.status.conditions[?(@.type=='Available')].status}",
                    ),
                    value="True",
                ),
            ),
            kubectl.create(
                Resource(
                    kustomize=self.iut_provider_kustomization,
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
            WaitUntil(
                StdoutEquals(
                    kubectl.get(
                        Resource(
                            type="deploy",
                            names="etos-iut",
                            namespace=self.local_store["cluster_namespace"],
                        ),
                        output="jsonpath='{.status.readyReplicas}'",
                    ),
                    value="1",
                )
            ),
            kubectl.create(
                Resource(
                    filename=self.iut_provider_sample,
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
            WaitUntil(
                StdoutEquals(
                    kubectl.get(
                        Resource(
                            type="provider",
                            names="iut-provider-sample",
                            namespace=self.local_store["cluster_namespace"],
                        ),
                        output="jsonpath={.status.conditions[?(@.type=='Available')].status}",
                    ),
                    value="True",
                ),
            ),
            kubectl.create(
                Resource(
                    filename=self.log_area_provider_sample,
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
            WaitUntil(
                StdoutEquals(
                    kubectl.get(
                        Resource(
                            type="provider",
                            names="log-area-provider-sample",
                            namespace=self.local_store["cluster_namespace"],
                        ),
                        output="jsonpath={.status.conditions[?(@.type=='Available')].status}",
                    ),
                    value="True",
                ),
            ),
        ]

    def delete(self) -> list[Command]:
        """Commands for deleting provider services and resources."""
        kubectl = Kubectl()
        return [
            kubectl.delete(
                Resource(
                    filename=self.log_area_provider_sample,
                    namespace=self.local_store["cluster_namespace"],
                ),
            ),
            kubectl.delete(
                Resource(
                    filename=self.iut_provider_sample,
                    namespace=self.local_store["cluster_namespace"],
                ),
            ),
            kubectl.delete(
                Resource(
                    kustomize=self.iut_provider_kustomization,
                    namespace=self.local_store["cluster_namespace"],
                ),
            ),
            kubectl.delete(
                Resource(
                    filename=self.execution_space_provider_sample,
                    namespace=self.local_store["cluster_namespace"],
                ),
            ),
            kubectl.delete(
                Resource(
                    kustomize=self.execution_space_provider_kustomization,
                    namespace=self.local_store["cluster_namespace"],
                ),
            ),
        ]
