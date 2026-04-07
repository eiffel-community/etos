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
"""Controller pack."""

from local.commands.command import Command
from local.commands.kubectl import Kubectl, Resource
from local.commands.make import Make
from local.commands.shell import Shell
from local.commands.utilities import (HasLines, StdoutEquals, StdoutLength,
                                      StoreStdout, WaitUntil)

from .base import BasePack


class Controller(BasePack):
    """Pack for deploying the ETOS controller."""

    def name(self) -> str:
        """Name of pack."""
        return "Controller"

    def create(self) -> list[Command]:
        """Commands for generating a new ETOS controller deployment"""
        kubectl = Kubectl()
        make = Make()
        return [
            kubectl.create(
                Resource(type="namespace", names=self.local_store["namespace"])
            ),
            kubectl.label(
                Resource(type="namespace", names=self.local_store["namespace"]),
                "pod-security.kubernetes.io/enforce=restricted",
            ),
            kubectl.create(
                Resource(type="namespace", names=self.local_store["cluster_namespace"])
            ),
            make.install(),
            make.docker_build(self.local_store["project_image"]),
            Shell(["kind", "load", "docker-image", self.local_store["project_image"]]),
            make.deploy(self.local_store["project_image"]),
            *self.__wait_for_control_plane(kubectl),
            *self.__wait_for_webhook_certificates(kubectl),
        ]

    def delete(self) -> list[Command]:
        """Commands for deleting an ETOS controller deployment"""
        kubectl = Kubectl()
        make = Make()
        return [
            make.undeploy(),
            make.uninstall(),
            kubectl.delete(
                Resource(type="namespace", names=self.local_store["cluster_namespace"])
            ),
            kubectl.delete(
                Resource(type="namespace", names=self.local_store["namespace"])
            ),
        ]

    def __wait_for_control_plane(self, kubectl: Kubectl) -> list[Command]:
        """Commands for waiting for the ETOS control plane."""
        return [
            StoreStdout(
                WaitUntil(
                    HasLines(
                        kubectl.get(
                            Resource(
                                type="pods",
                                selector="control-plane=controller-manager",
                                namespace=self.local_store["namespace"],
                            ),
                            output='go-template="{{ range .items }} {{- if not .metadata.deletionTimestamp }} {{- .metadata.name }} {{ "\\n" }} {{- end -}} {{- end -}}"',
                        ),
                        length=1,
                    ),
                ),
                key="control_plane_pod_name",
                store=self.local_store,
            ),
            WaitUntil(
                StdoutEquals(
                    kubectl.get(
                        Resource(
                            type="pods",
                            names=self.local_store["control_plane_pod_name"],
                            namespace=self.local_store["namespace"],
                        ),
                        output="jsonpath={.status.phase}",
                    ),
                    value="Running",
                ),
            ),
            kubectl.wait(
                Resource(
                    type="pods",
                    names=self.local_store["control_plane_pod_name"],
                    namespace=self.local_store["namespace"],
                ),
                wait_for="condition=ready",
            ),
        ]

    def __wait_for_webhook_certificates(self, kubectl: Kubectl) -> list[Command]:
        """Commands for waiting for the ETOS controller webhooks."""
        commands: list[Command] = []
        for type, name in (
            (
                "validatingwebhookconfigurations.admissionregistration.k8s.io",
                "etos-validating-webhook-configuration",
            ),
            (
                "mutatingwebhookconfigurations.admissionregistration.k8s.io",
                "etos-mutating-webhook-configuration",
            ),
        ):
            commands.append(
                WaitUntil(
                    StdoutLength(
                        kubectl.get(
                            Resource(
                                type=type,
                                names=name,
                            ),
                            output="go-template='{{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}'",
                        ),
                        length=10,
                    ),
                )
            )
        for type, name in (
            (
                "customresourcedefinitions.apiextensions.k8s.io",
                "providers.etos.eiffel-community.github.io",
            ),
            (
                "customresourcedefinitions.apiextensions.k8s.io",
                "testruns.etos.eiffel-community.github.io",
            ),
            (
                "customresourcedefinitions.apiextensions.k8s.io",
                "environmentrequests.etos.eiffel-community.github.io",
            ),
        ):
            commands.append(
                WaitUntil(
                    StdoutLength(
                        kubectl.get(
                            Resource(
                                type=type,
                                names=name,
                            ),
                            output="go-template='{{ .spec.conversion.webhook.clientConfig.caBundle }}'",
                        ),
                        length=10,
                    ),
                )
            )
        return commands
