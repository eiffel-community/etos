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
# TestRun custom resource

A TestRun is a custom resource in Kubernetes that defines a testrun. A TestRun is created by the ETOS V1Alpha API or directly in Kubernetes, and it is executed by the ETOS controller.

Creating a TestRun directly in Kubernetes is useful for testing and development purposes, as it allows you to bypass the API and directly test the execution of a testrun.

It is also useful for deploying tests directly during the deployment of a service. Maybe even as pre-requisite for a deployment, where the deployment will only proceed if the TestRun is successful.

## TestRun specification

This specification is taken from the `config/samples/etos_v1alpha1_testrun.yaml` file in the ETOS repository and used in the e2e tests of the ETOS controller.

```yaml
apiVersion: etos.eiffel-community.github.io/v1alpha1
kind: TestRun
metadata:
  generateName: my-testrun-
spec:
  identity: pkg:docker/my-image@sha256:1234567890abcdef
  artifact: 12345678-1234-1234-1234-123456789012
  cluster: cluster-sample  # The cluster to run the testrun on. This should be the name of the Cluster resource in Kubernetes.
  providers:
    iut: iut-provider-sample  # config/samples/etos_v1alpha1_iut_provider.yaml
    executionSpace: execution-space-provider-sample # config/samples/etos_v1alpha1_execution_space_provider.yaml
    logArea: log-area-provider-sample # config/samples/etos_v1alpha1_log_area_provider.yaml
  suites:
  - name: name-of-suite
    dataset:
      test: Hello  # A key-value map of additional parameters to pass to the ETOS
    tests:
    - environment: {}
      execution:
        checkout:
        - git clone https://github.com/eiffel-community/etos
        command: exit 0
        environment: {}
        execute:
        - sleep 30
        parameters: {}
        testRunner: ghcr.io/eiffel-community/etos-base-test-runner:ubuntu-noble
      id: 12345678-1234-1234-1234-123456789012
      testCase:
        id: etos-sample-test
        version: main
```

## Apply it in Kubernetes

```bash
kubectl create -f testrun.yaml -n etos # or the namespace that the ETOS cluster is deployed to
```

## Check result

```bash
kubectl get testrun -n etos
```
