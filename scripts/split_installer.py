# Copyright Axis Communications AB.
#
# For a full list of individual contributors, please see the commit history.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
import sys
from typing import Iterator, Any
from pathlib import Path
from yaml import load_all, SafeLoader, dump_all, dump, SafeDumper


def split(installer: Iterator[Any]) -> tuple[list[dict], list[dict], dict]:
    cluster = []
    namespaced = []
    namespace = None
    for resource in installer:
        if resource.get("kind") == "Namespace":
            namespace = resource
            continue
        metadata = resource.get("metadata", {})
        if metadata.get("namespace") is None:
            cluster.append(resource)
        else:
            namespaced.append(resource)
    return cluster, namespaced, namespace


def run(installer: str):
    path = Path(installer)
    assert path.exists(), f"{installer} does not exist"
    directory = path.parent
    with path.open() as installer_file:
        installer_generator = load_all(installer_file, Loader=SafeLoader)
        cluster, namespaced, namespace = split(installer_generator)
        print("=== CLUSTER ===")
        for resource in cluster:
            print(f"[{resource.get('kind')}]: {resource.get('metadata', {}).get('name')}")
        with directory.joinpath("cluster.yaml").open("w") as cluster_file:
            dump_all(cluster, cluster_file, Dumper=SafeDumper)
        print("=== NAMESPACED ===")
        for resource in namespaced:
            print(f"[{resource.get('kind')}]: {resource.get('metadata', {}).get('namespace')}/{resource.get('metadata', {}).get('name')}")
        with directory.joinpath("namespaced.yaml").open("w") as namespaced_file:
            dump_all(namespaced, namespaced_file, Dumper=SafeDumper)
        print("=== NAMESPACE ===")
        print(f"[{namespace.get('kind')}]: {namespace.get('metadata', {}).get('name')}")
        with directory.joinpath("namespace.yaml").open("w") as namespace_file:
            dump(namespace, namespace_file, Dumper=SafeDumper)
    print()
    print("Successfully split the installer")
    print("Files are located here:")
    print(f"  - {directory.joinpath('namespaced.yaml')}")
    print(f"  - {directory.joinpath('namespace.yaml')}")
    print(f"  - {directory.joinpath('cluster.yaml')}")


if __name__ == "__main__":
    run(sys.argv[1])
