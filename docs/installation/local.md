<!---
   Copyright Axis Communications AB
   For a full list of individual contributors, please see the commit history.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
--->
# Installation Guide

Great for development and testing purposes. This will run ETOS in a single-node Kubernetes cluster on your local machine using [kind](https://kind.sigs.k8s.io/).

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/)

## Installation

#### Clone the repository:

```bash
git clone https://github.com/eiffel-community/etos
cd etos
```

#### Create a kind cluster:

```bash
kind create cluster
```

#### Install ETOS:

```bash
make deploy-local
```

This will install a local instance of ETOS on your kind cluster and will have executed a single testrun to verify that the installation was successful.
For more information on how to run tests locally, please see the [getting started guide](../getting_started/index.md).
