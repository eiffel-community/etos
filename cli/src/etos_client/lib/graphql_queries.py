# Copyright 2020-2022 Axis Communications AB.
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
  testExecutionRecipeCollectionCreated(search: "{'meta.id': '%s'}") {
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
  activityTriggered(search: "{'links.type': 'CAUSE', 'links.target': '%s'}") {
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

ACTIVITY_CANCELED = """
{
  activityCanceled(search: "{'links.type': 'ACTIVITY_EXECUTION', 'links.target': '%s'}") {
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
  testSuiteStarted(search:"{'links.type': 'CAUSE', 'links.target': '%s'}") {
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


MAIN_TEST_SUITES_STARTED = """
{
  testSuiteStarted(search:"{'links.type': 'CONTEXT', 'links.target': '%s', 'data.categories': {'$ne': 'Sub suite'}}") {
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


TEST_SUITE_FINISHED = """
{
  testSuiteFinished(search: "{'links.type': 'TEST_SUITE_EXECUTION', 'links.target': '%s'}" last: 1) {
    edges {
      node {
        data {
          testSuitePersistentLogs {
            name
            uri
          }
          testSuiteOutcome {
            verdict
          }
        }
      }
    }
  }
}
"""


ANNOUNCEMENTS = """
{
  announcementPublished(search: "%s") {
    edges {
      node {
        data {
          heading
          body
        }
      }
    }
  }
}
"""


ENVIRONMENTS = """
{
  environmentDefined(search: "%s") {
    edges {
      node {
        data {
          name
          uri
        }
      }
    }
  }
}
"""


ARTIFACTS = """
{
  artifactCreated(search: "{'links.type': 'CONTEXT', 'links.target': '%s'}") {
    edges {
      node {
        data {
          fileInformation {
            name
          }
        }
        links {
          ... on Cause {
            links {
              ... on TestSuiteStarted {
                data {
                  name
                }
              }
            }
          }
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
