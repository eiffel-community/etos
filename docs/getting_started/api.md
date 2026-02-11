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
# Starting tests via the API

While it is recommended to use the ETOS client to start testruns, it is also possible to start testruns via the API. This can be useful if you want to integrate ETOS with other tools or if you want to automate the process of starting testruns.

This should be seen as an advanced use case, as it requires a good understanding of the ETOS API and how to interact with it. If you are new to ETOS, it is recommended to start with the ETOS client and then move on to the API once you are familiar with the concepts and how ETOS works.

Please see the [getting started guide](index.md) for more information on how to start testruns using the ETOS client.

## Prerequisites

- A test run recipe collection. For more information on how to create a recipe collection, please see the [Getting started guide](index.md#creating-a-recipe-collection).
- An ETOS instance up and running. If you don't have an ETOS instance, please see the [installation guide](../installation/index.md).

## Starting a testrun ETOS

The V0 API documentation is available here `https://etos-api-instance/api/v0/docs/`.  
The V1Alpha API documentation is available here `https://etos-api-instance/api/v1alpha/docs/`.

The v1alpha API and the v0 API are the same in terms of parameters and response. Pick any of the two to start a testrun.

### Parameters

- artifact_id: The ID of the artifact to test. This should be the ID of the artifact in the event repository.
- test_suite_url: The URL of the recipe collection to use for the testrun. (see [getting started](index.md#creating-a-recipe-collection))
- dataset: A key-value map of additional parameters to pass to the ETOS environment providers.
- execution_space_provider: The execution space provider to use for this testrun.
- iut_provider: The IUT provider to use for this testrun.
- log_area_provider: The log area provider to use for this testrun.

### Example

V0 URL: `https://etos-api-instance/api/v0/etos` \
V1Alpha URL: `https://etos-api-instance/api/v1alpha/testrun`

```bash
curl -X POST "https://etos-api-instance/api/v0/etos" \
-H "Content-Type: application/json" \
-d '{
  "artifact_id": "12345678-1234-1234-1234-123456789012",
  "test_suite_url": "https://my-recipe-collection.json",
  "dataset": {"test": "value"},
  "execution_space_provider": "default",
  "iut_provider": "default",
  "log_area_provider": "default"
}'
```

### Response

The response will contain the ID of the created testrun, which event repository that was used and some artifact information.

```json
{
  "tercc": "12345678-1234-1234-1234-123456789012",
  "event_repository": "https://my-event-repository.com",
  "artifact_id": "12345678-1234-1234-1234-123456789012",
  "artifact_identity": "pkg:docker/my-image@sha256:1234567890abcdef",
}
```

Now that the testrun has been started, you have to follow the testrun by quering the event repository for Eiffel events that denote if the testrun is finished.

Here's a query that you can use to check if the testrun is finished:

```graphql
{
  testExecutionRecipeCollectionCreated(search: "{'meta.id': 'tercc-id'}") {
    edges {
      node {
        reverse {
          ... on activityTriggered {
            edges {
              node {
                reverse {
                  ... on testSuiteFinished {
                    edges {
                      node {
                        data {
                          testSuiteOutcome {
                            verdict
                            conclusion
                            description
                          }
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
```
