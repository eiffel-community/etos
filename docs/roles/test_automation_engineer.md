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
# Test Automation Engineer

The test automation engineers role is to create the recipes that defines how to test and to create the tests that will be run by the recipes.

## Responsibilities of a test automation engineer

- Creating the tests
- Creating recipe instructions for the tests
- Making sure the tests work

## Creating tests

We are not going to go into detail on how to create tests, but the test automation engineer is responsible for creating the tests that will be run by ETOS. This includes both the test code itself and the recipe instructions that tells ETOS how to run the tests.

Other than that you should follow best practices for testing, such as making sure your tests are deterministic, that they are testing one thing at a time and that they are easy to understand and maintain.

## How to create recipe instructions


Recipes are created in JSON and tell ETOS how to execute the tests.
The recipe instructions need to be shared with the test engineers somehow, either by sharing the JSON file directly or by creating a tool that generates the JSON file from a more user-friendly format. We (the ETOS maintainers) do not provide any tools for this, but we are open to suggestions and contributions in this area.

A full recipe collection example can be found [here](../getting_started/index.md#creating-a-recipe-collection), but as a test automation engineer you are only responsible for a part of that recipe collection.

The recipe instructions that you need to create are the ones that tells ETOS how to run the tests, but also some metadata about the test. An example of such instructions can look like this:

```json
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
        "Hello, World!": ""
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
```

Table of the keys in the recipe instructions:

| Key | Description |
| --- | --- |
| id | A unique identifier for the recipe. This should be a UUID. Shall be updated when the recipe changes. |
| testCase | Metadata about the test case. |
| testCase.tracker | The name of the test case tracker. This should be the name of the tracker where the test case is stored. |
| testCase.url | The URL to the tracker. Together with testCase.id this should be a link to where the test case can be found. |
| testCase.id | The name or id of the test case. This should be a unique identifier for the test case which shall be searchable in the tracker |
| testCase.version | The version of the test case. This is optional and can be used to specify which version of the test case to run, if the test cases are managed in git (for example) then this is preferably the sha-1 (commit id) |
| constraints | A list of constraints that tells ETOS how to run the test. |
| constraints.ENVIRONMENT | Environment variables that should be set when running the test. |
| constraints.COMMAND | The command to run the test. |
| constraints.PARAMETERS | The parameters to pass to the command when running the test. |
| constraints.TEST_RUNNER | The test runner to use when running the test. This should be a valid Docker image that can be pulled from a container registry. |
| constraints.CHECKOUT | This is used to checkout the code that should be tested. |
| constraints.EXECUTE | This is used to execute commands before the test is run. This can be used to prepare the environment for the test. |
