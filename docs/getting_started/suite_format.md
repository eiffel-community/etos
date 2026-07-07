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
# Suite definition format

ETOS supports a standalone YAML format for defining test suites. A suite
definition describes one or more test suites, the test executions within each
suite, how each test is checked out and executed, and the environment it
requires.

> **Note:** The standalone YAML suite definition format is a v1 feature.

The format is described by a JSON Schema located at
[`schemas/suite/v1alpha1/suite.schema.json`](https://github.com/eiffel-community/etos/blob/main/schemas/suite/v1alpha1/suite.schema.json).
A complete, validating example is available at
[`schemas/suite/v1alpha1/example.yaml`](https://github.com/eiffel-community/etos/blob/main/schemas/suite/v1alpha1/example.yaml).
The example is validated against the schema in CI.

## Top-level fields

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | Name of the suite definition. |
| `schemaVersion` | string | yes | Version of the suite schema. Must be `v1alpha1`. |
| `suites` | array | yes | One or more test suites to execute. |

## Suite

Each entry in `suites` describes a prioritized group of test executions.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `priority` | integer | yes | Execution priority. Lower values indicate higher priority. |
| `testExecutions` | array | yes | One or more test executions in this suite. |

## Test execution

A test execution combines a test case with its execution instructions and
environment requirements.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | string | yes | Unique identifier for this test execution. |
| `testCase` | object | yes | Metadata about the test case (see below). |
| `execution` | object | yes | How to check out and run the test (see below). |
| `environment` | object | yes | Environment requirements (see below). |
| `dependencies` | array | no | IDs of test executions that must complete before this one runs. |

### testCase

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | string | yes | Fully qualified test case identifier. |
| `version` | string | no | Test case version, typically a Git commit SHA or branch. |
| `repository` | string (uri) | no | Source repository containing the test case. |
| `uri` | string (uri) | no | Documentation or external reference for the test case. |

### execution

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `command` | string | yes | The shell command that runs the test. |
| `checkout` | array | no | Shell commands to check out the test source code, run in order before the command. |
| `preExecution` | array | no | Shell commands to run after checkout but before the main command. |

### environment

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `testRunner` | string | yes | Container image to use as the test runner. |
| `environmentVariables` | object | no | Key-value environment variables set in the test runner. |
| `additionalResources` | array | no | Provider-defined resources required by the test (see below). |

#### additionalResources

`additionalResources` is a list of provider-defined resources that a test needs
in addition to the primary IUT (Item Under Test). Typical examples are sidecar
containers or additional devices. Each resource has a `type` field that selects
the provider responsible for it. The value of `type` (for example `device` or
`container`) and all remaining fields are defined by the available providers,
not by this schema; a resource is only usable if a provider exists that can
handle its type.

```yaml
additionalResources:
- type: container
  image: postgres:16
  environmentVariables:
    POSTGRES_DB: testdb
    POSTGRES_PASSWORD: test
- type: device
  model: example-board
  features:
  - GPIO
  - WiFi
```

## Validating a suite definition

The example is validated against the schema in CI using
[`check-jsonschema`](https://check-jsonschema.readthedocs.io/). You can run the
same validation locally:

```bash
pip install check-jsonschema
check-jsonschema \
  --schemafile schemas/suite/v1alpha1/suite.schema.json \
  schemas/suite/v1alpha1/example.yaml
```
