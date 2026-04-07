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
# Getting started with ETOS

This guide will focus on starting testruns in ETOS using the ETOS client. The ETOS client is the recommended way to start testruns in ETOS, as it provides a simple and easy-to-use interface.

If you are interested in installing ETOS, please see the [installation guide](../installation/index.md) instead.  
If you want to start testruns using the ETOS API, please see the [API documentation](api.md).  
If you want to start testruns directly in Kubernetes, please see the [TestRun documentation](testrun.md).

## Prerequisites

- An ETOS instance up and running. If you don't have an ETOS instance, please see the [installation guide](../installation/index.md).
- The ETOS client installed. If you don't have the ETOS client installed, please see the [ETOS client documentation](https://pypi.org/project/etos-client/).

ETOS requires an Eiffel artifact to test. All Eiffel events should be stored in an event repository. ETOS uses the [Eiffel GraphQL API](https://github.com/eiffel-community/eiffel-graphql-api) to query the event repository for the artifacts to test.
If you don't have an event repository (or don't know how to use it), please contact your [System Engineer](../roles/system_engineer.md) for more information.

## Creating a recipe collection

A recipe collection is a collection of recipes that defines what to run in a testrun. A recipe collection is defined in a JSON file, which contains a list of recipes and their parameters.

```json
[
  {
    "name": "my-recipe-collection",
    "priority": 1,
    "recipes": [
      {
        "id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
        "testCase": {
          "id": "name_of_testcase",
          "tracker": "Name of the test case tracker"
          "url": "https://tracker.com/testcase/name_of_testcase"
        },
        "constraints": [
          {
            "key": "ENVIRONMENT",
            "value": {"MY_ENVIRONMENT_VARIABLE": "hello"}
          },
          {
            "key": "COMMAND",
            "value": "echo"
          },
          {
            "key": "PARAMETERS",
            "value": {
              "'Hello, World!'": ""
            }
          },
          {
              "key": "TEST_RUNNER",
              "value": "ghcr.io/eiffel-community/etos-base-test-runner:ubuntu-noble"
          },
          {
              "key": "CHECKOUT",
              "value": [
                "git clone https://github.com/eiffel-community/etos"
              ]
          }
        ]
      }
    ]
  }
]
```

This file must be uploaded to a location that is accessible by the ETOS instance by HTTP or HTTPS.

## Starting a testrun

To start a testrun, you can use the ETOS client. The ETOS client provides a simple interface to start testruns in ETOS.

```bash
etosctl testrun start -s https://path.to/your/recipe_collection.json --identity artifact-id --dataset='{"key": "value"}' https://etos-api-instance
```
(Due to a bug in ETOS, the `--dataset` flag is currently required, and it cannot be an empty JSON object)

This command will start a testrun in ETOS using the recipe collection defined in the JSON file. The `--identity` flag is used to specify the identity of the artifact to test, and the `--dataset` flag is used to specify any additional configuration options that should be passed to the ETOS environment providers.

## Checking the results

After the testrun has finished you will get a quick summary of the results in the terminal. However if tests have failed, you will need to check the reports from your tests in the `reports` directory of the testrun.

Congratulations! You have successfully started a testrun in ETOS and checked the results. For more information on how to use ETOS, please see the [documentation](../index.md).
