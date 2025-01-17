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
"""GraphQL queries."""

TEST_SUITE = """
{
  testExecutionRecipeCollectionCreated(%s) {
    edges {
      node {
        data {
          batchesUri
        }
      }
    }
  }
}
"""


ACTIVITY_TRIGGERED = """
{
  activityTriggered(%s) {
    edges {
      node {
        meta {
          id
        }
      }
    }
  }
}
"""


ACTIVITY_FINISHED = """
{
  activityFinished(%s) {
    edges {
      node {
        data {
          activityOutcome {
            description
            conclusion
          }
        }
      }
    }
  }
}
"""


ACTIVITY_CANCELED = """
{
  activityCanceled(%s) {
    edges {
      node {
        data {
          reason
        }
      }
    }
  }
}
"""


TEST_SUITE_STARTED = """
{
  testSuiteStarted(%s) {
    edges {
      node {
        data {
          name
        }
        meta {
          id
        }
      }
    }
  }
}
"""


MAIN_TEST_SUITES_STARTED = """
{
  testSuiteStarted(%s) {
    edges {
      node {
        data {
          name
        }
        meta {
          id
        }
      }
    }
  }
}
"""


TEST_SUITE_FINISHED = """
{
  testSuiteFinished(%s) {
    edges {
      node {
        data {
          testSuitePersistentLogs {
            name
            uri
          }
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
"""


TEST_CASE_FINISHED = """
{
  testCaseFinished(%s) {
    edges {
      node {
        data {
          testCaseOutcome {
            conclusion
            verdict
          }
        }
        links {
          ... on TestCaseExecution {
            testCaseTriggered {
              data {
                testCase{
                  id
                }
              }
            }
          }
        }
        meta {
          time
        }
      }
    }
  }
}
"""


TEST_CASE_CANCELED = """
{
  testCaseCanceled(%s) {
    edges {
      node {
        links {
          ... on TestCaseExecution {
            testCaseTriggered {
              data {
                testCase{
                  id
                }
              }
            }
          }
        }
        meta {
          time
        }
      }
    }
  }
}
"""


ENVIRONMENTS = """
{
  environmentDefined(%s) {
    edges {
      node {
        data {
          name
          uri
        }
        meta {
          time
        }
      }
    }
  }
}
"""


ARTIFACTS = """
{
  artifactCreated(%s) {
    edges {
      node {
        data {
          fileInformation {
            name
          }
        }
        meta {
          time
        }
        reverse {
          edges {
            node {
              ... on ArtifactPublished {
                data {
                  locations {
                    uri
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
"""
