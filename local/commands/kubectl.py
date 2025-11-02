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
"""Kubectl wrapper."""
import logging

from local.utilities.store import Value

from .command import Command
from .shell import Shell


class Resource:
    """Kubernetes resource definition.

    A resource can either be a `filename` for `kubectl -f`,
    a `kustomize` for `kubectl -k` or a `type`/`name(s)`.
    """

    filename: str | Value | None = None
    kustomize: str | Value | None = None
    type: str | Value | None = None
    names: list[str | Value] | None = None
    namespace: str | Value | None = None
    selector: list[str | Value] | None = None

    def __init__(
        self,
        filename: str | Value | None = None,
        kustomize: str | Value | None = None,
        type: str | Value | None = None,
        names: list[str | Value] | str | Value | None = None,
        namespace: str | Value | None = None,
        selector: list[str | Value] | str | Value | None = None,
    ):
        self.filename = filename
        self.kustomize = kustomize
        self.type = type

        if names is not None and not isinstance(names, list):
            names = [names]
        self.names = names
        if selector is not None and not isinstance(selector, list):
            selector = [selector]
        self.selector = selector
        self.namespace = namespace
        self.__verify()

    def __verify(self):
        """Verify that not too much or too little information is stored in Resource."""
        if any(
            [
                (
                    self.filename is not None
                    and (
                        self.kustomize is not None
                        or self.type is not None
                        or self.names is not None
                        or self.selector is not None
                    )
                ),
                (
                    self.kustomize is not None
                    and (
                        self.filename is not None
                        or self.type is not None
                        or self.names is not None
                        or self.selector is not None
                    )
                ),
                (
                    self.type is not None
                    and self.names is not None
                    and (
                        self.filename is not None
                        or self.kustomize is not None
                        or self.selector is not None
                    )
                ),
                (
                    self.type is not None
                    and self.selector is not None
                    and (
                        self.filename is not None
                        or self.kustomize is not None
                        or self.names is not None
                    )
                ),
            ]
        ):
            raise ValueError(
                "Only one of filename, kustomize, type/name pair and type/selector pair are allowed"
            )
        if (
            self.filename is None
            and self.kustomize is None
            and self.type is None
            and self.names is None
            and self.selector is None
        ):
            raise ValueError(
                "One of filename, kustomize and type/name pair is required"
            )

        if self.type is None and (self.names or self.selector):
            raise ValueError(
                "Type and names or selector come in pairs, must supply both"
            )
        if (self.names is None and self.selector is None) and self.type is not None:
            raise ValueError(
                "Type and names or selector come in pairs, must supply both"
            )


class Kubectl:
    """Wrapper for the kubectl shell command."""

    logger = logging.getLogger(__name__)

    def create(self, resource: Resource, *args: str) -> Command:
        """Command for running `kubectl create`.

        Resource.selector is not supported.
        """
        command: list[str | Value] = ["kubectl", "create"]
        if resource.namespace is not None:
            command.extend(["--namespace", resource.namespace])
        if resource.filename is not None:
            command.extend(["--filename", resource.filename])
        if resource.kustomize is not None:
            command.extend(["--kustomize", resource.kustomize])
        if resource.type is not None and resource.names is not None:
            command.extend([resource.type, *resource.names])
        command.extend(args)
        return Shell(command)

    def delete(
        self,
        resource: Resource,
        ignore_not_found: bool = True,
    ) -> Command:
        """Command for running `kubectl delete`.

        Resource.selector is not supported.
        """
        command: list[str | Value] = ["kubectl", "delete"]
        if resource.namespace is not None:
            command.extend(["--namespace", resource.namespace])
        if resource.filename is not None:
            command.extend(["--filename", resource.filename])
        if resource.kustomize is not None:
            command.extend(["--kustomize", resource.kustomize])
        if resource.type is not None and resource.names is not None:
            command.extend([resource.type, *resource.names])
        if ignore_not_found:
            command.append(f"--ignore-not-found={str(ignore_not_found).lower()}")
        return Shell(command)

    def run(
        self,
        name: str,
        namespace: str | Value,
        image: str,
        overrides: str,
        restart_policy: str = "Never",
    ) -> Command:
        """Command for running `kubectl run`."""
        command = [
            "kubectl",
            "run",
            name,
            "--namespace",
            namespace,
            f"--restart={restart_policy}",
            f"--image={image}",
            "--overrides",
            overrides,
        ]
        return Shell(command)

    def label(self, resource: Resource, label: str, overwrite: bool = True) -> Command:
        """Command for running `kubectl label`.

        Resource.selector is not supported.
        """
        command: list[str | Value] = ["kubectl", "label"]
        if resource.namespace is not None:
            command.extend(["--namespace", resource.namespace])
        if resource.filename is not None:
            command.extend(["--filename", resource.filename])
        if resource.kustomize is not None:
            command.extend(["--kustomize", resource.kustomize])
        if resource.type is not None and resource.names is not None:
            command.extend([resource.type, *resource.names])
        if overwrite:
            command.append("--overwrite")
        command.append(label)
        return Shell(command)

    def wait(
        self,
        resource: Resource,
        wait_for: str,
        timeout: int = Command.default_timeout,
    ) -> Command:
        """Command for running `kubectl wait`.

        Only Resource.type/Resource.names or Resource.selector pairs are supported.
        """
        if resource.type is None or (
            resource.names is None and resource.selector is None
        ):
            raise ValueError("Wait only supports the type & names resource style")
        command: list[str | Value] = [
            "kubectl",
            "wait",
            "--for",
            wait_for,
            "--timeout",
            f"{timeout}s",
        ]
        if resource.type is not None and resource.names is not None:
            command.extend([resource.type, *resource.names])
        if resource.type is not None and resource.selector is not None:
            command.extend([resource.type, "--selector", *resource.selector])
        if resource.namespace is not None:
            command.extend(["--namespace", resource.namespace])
        return Shell(command, timeout)

    def get(
        self,
        resource: Resource,
        output: str | None = None,
    ) -> Command:
        """Command for running `kubectl get`.

        Only Resource.type/Resource.names or Resource.selector pairs are supported.
        """
        if resource.type is None or (
            resource.names is None and resource.selector is None
        ):
            raise ValueError("Get only supports the type & names resource style")
        command: list[str | Value] = ["kubectl", "get"]
        if resource.namespace is not None:
            command.extend(["--namespace", resource.namespace])
        if resource.type is not None and resource.names is not None:
            command.extend([resource.type, *resource.names])
        if resource.type is not None and resource.selector is not None:
            command.extend([resource.type, "--selector", *resource.selector])
        if output is not None:
            command.extend(["--output", output])
        return Shell(command)
