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
"""Local ETOS pack."""
from local.commands.command import Command
from local.commands.kubectl import Kubectl, Resource
from local.commands.utilities import StdoutEquals, WaitUntil

from .base import BasePack


class Etos(BasePack):
    """Etos pack to create an ETOS cluster.

    Create the cluster spec, deploy GoER (for providers) and inject an
    artifact into the system which can be used to verify the deployment.
    """

    cluster_sample = "config/samples/etos_v1alpha1_cluster.yaml"
    goer = "testdata/goer.yaml"

    def name(self) -> str:
        """Name of the pack."""
        return "ETOS"

    def create(self) -> list[Command]:
        """Commands for creating a fully functioning (without providers) ETOS cluster."""
        kubectl = Kubectl()
        return [
            kubectl.create(
                Resource(
                    filename=self.cluster_sample,
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
            kubectl.wait(
                Resource(
                    type="cluster",
                    names=self.local_store["cluster_name"],
                    namespace=self.local_store["cluster_namespace"],
                ),
                wait_for="jsonpath={.status.conditions[?(@.type=='Ready')].status}=True",
            ),
            *self.__wait_for_deployments(kubectl),
            kubectl.create(
                Resource(
                    filename=self.goer, namespace=self.local_store["cluster_namespace"]
                )
            ),
            WaitUntil(
                StdoutEquals(
                    kubectl.get(
                        Resource(
                            names="goer",
                            type="deploy",
                            namespace=self.local_store["cluster_namespace"],
                        ),
                        output="jsonpath='{.status.readyReplicas}'",
                    ),
                    value="1",
                )
            ),
            *self.__inject_artifact(kubectl),
            kubectl.create(
                Resource(
                    type="secret",
                    # This is a hack, putting generic here because it has to come after 'type' and before 'name'
                    # and cannot be added to the extra args at the end.
                    names=["generic", "etos-encryption-key"],
                    namespace=self.local_store["cluster_namespace"],
                ),
                "--from-literal=ETOS_ENCRYPTION_KEY=ZmgcW2Qz43KNJfIuF0vYCoPneViMVyObH4GR8R9JE4g=",
            ),
        ]

    def delete(self) -> list[Command]:
        """Commands for deleting the ETOS cluster."""
        kubectl = Kubectl()
        return [
            kubectl.delete(
                Resource(
                    type="secret",
                    names="etos-encryption-key",
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
            kubectl.delete(
                Resource(
                    type="pod",
                    names="artifact-injector",
                    namespace=self.local_store["cluster_namespace"],
                ),
            ),
            kubectl.delete(
                Resource(
                    filename=self.goer, namespace=self.local_store["cluster_namespace"]
                )
            ),
            kubectl.delete(
                Resource(
                    filename=self.cluster_sample,
                    namespace=self.local_store["cluster_namespace"],
                )
            ),
        ]

    def __wait_for_deployments(self, kubectl: Kubectl) -> list[Command]:
        """Commands for waiting until the ETOS deployments reach the expected number of ready replicas."""
        commands: list[Command] = []
        cluster_name = self.local_store["cluster_name"].get()
        for name, type, count in (
            ("etos-sse", "deploy", "1"),
            ("etos-logarea", "deploy", "1"),
            ("etos-suite-starter", "deploy", "1"),
            ("messagebus", "statefulset", "1"),
            ("rabbitmq", "statefulset", "1"),
            ("graphql", "deploy", "1"),
            ("etos-api", "deploy", "1"),
            ("etcd", "statefulset", "3"),
        ):
            commands.append(
                WaitUntil(
                    StdoutEquals(
                        kubectl.get(
                            Resource(
                                type=type,
                                names=f"{cluster_name}-{name}",
                                namespace=self.local_store["cluster_namespace"],
                            ),
                            output="jsonpath='{.status.readyReplicas}'",
                        ),
                        value=count,
                    )
                )
            )
        return commands

    def __inject_artifact(self, kubectl: Kubectl) -> list[Command]:
        """Command for injecting an artifact into the system."""
        # TODO: Injecting multiple artifacts that reference the different ETOS repositories and versions
        artifact_id = self.local_store["artifact_id"].get()
        artifact_identity = self.local_store["artifact_identity"].get()
        cluster_name = self.local_store["cluster_name"].get()
        command = f"""
{{
	"spec": {{
		"containers": [{{
			"name": "test",
			"image": "ghcr.io/eiffel-community/eiffel-graphql-storage:latest",
			"envFrom": [{{"secretRef": {{"name": "{cluster_name}-graphql"}}}}],
			"command": ["python", "-c"],
			"args": ["from eiffel_graphql_api.graphql.db.database import insert_to_db;from eiffellib.events import EiffelArtifactCreatedEvent;event = EiffelArtifactCreatedEvent(); event.meta.event_id = '{artifact_id}';event.data.data['identity'] = '{artifact_identity}';insert_to_db(event);"],
			"securityContext": {{
				"allowPrivilegeEscalation": false,
				"capabilities": {{
					"drop": ["ALL"]
				}},
				"runAsNonRoot": true,
				"runAsUser": 1000,
				"seccompProfile": {{
					"type": "RuntimeDefault"
				}}
			}}
		}}]
	}}
}}
        """
        return [
            kubectl.run(
                "artifact-injector",
                self.local_store["cluster_namespace"],
                "ghcr.io/eiffel-community/eiffel-graphql-storage:latest",
                command,
            ),
            kubectl.wait(
                Resource(
                    type="pod",
                    names="artifact-injector",
                    namespace=self.local_store["cluster_namespace"],
                ),
                wait_for="jsonpath={.status.conditions[?(@.type=='Ready')].reason}=PodCompleted",
            ),
            kubectl.delete(
                Resource(
                    type="pod",
                    names="artifact-injector",
                    namespace=self.local_store["cluster_namespace"],
                ),
            ),
        ]
