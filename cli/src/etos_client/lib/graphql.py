# Copyright 2020-2023 Axis Communications AB.
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
"""GraphQL query handler."""
from .graphql_queries import (
    ARTIFACTS,
    ACTIVITY_TRIGGERED,
    ACTIVITY_CANCELED,
    MAIN_TEST_SUITES_STARTED,
    TEST_SUITE_STARTED,
    TEST_SUITE_FINISHED,
    TEST_SUITE,
    ANNOUNCEMENTS,
    ENVIRONMENTS,
)


class Search(dict):
    """A search dictionary for eiffel graphql API.

    This makes it easier to create search strings that are more
    coherent. Just create them as you would do a dictionary.
    When this object is then converted to a string in the graphql
    query it will convert the dictionary into a query string that
    works with the eiffel graphql API.
    """

    def __str__(self):
        """Convert dictionary to an eiffel graphql API compatible string."""
        strings = []
        for key, value in self.items():
            if key == "search":
                strings.append(f'search: "{self["search"]}"')
            else:
                strings.append(f'{key}: {value}')
        return ", ".join(strings)


def request(etos, query, search):
    """Request graphql in a generator.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param query: Query to send to graphql.
    :type query: str
    :param search: Search string for graphql query.
    :type search: :obj:`Search`
    :return: Generator
    :rtype: generator
    """
    wait_generator = etos.utils.wait(etos.graphql.execute, query=query % search, timeout=60)
    yield from wait_generator


def search_for(etos, query, search, node):
    """Request graphql and search for node in response.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param query: Query to send to graphql.
    :type query: str
    :param search: Search string for graphql query.
    :type search: :obj:`Search`
    :param node: Node to search for.
    :type node: str
    :return: Generator
    :rtype: generator
    """
    for response in request(etos, query, search):
        if response:
            for _, event in etos.graphql.search_for_nodes(
                response, node
            ):
                yield event
            return None  # StopIteration
    return None  # StopIteration


def request_all(etos, query, search, node):
    """Request all events for a given query.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param query: Query to send to graphql.
    :type query: str
    :param search: Search string for graphql query.
    :type search: :obj:`Search`
    :param node: Node to search for.
    :type node: str
    :return: Generator
    :rtype: generator
    """
    # TODO:
    search["last"] = 1
    while True:
        last_event = None
        for event in search_for(etos, query, search, node):
            last_event = event
            yield event
        if last_event is None:
            return None  # StopIteration
        # TODO: This assumes that the query has meta.time
        search["search"]["meta.time"] = {"$lt": last_event["meta"]["time"]}


def get_one(etos, query, search, node):
    """Request graphql and get a single node from response.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param query: Query to send to graphql.
    :type query: str
    :param search: Search string for graphql query.
    :type search: :obj:`Search`
    :param node: Node to search for.
    :type node: str
    :return: Event
    :rtype: :obj:`eiffellib.lib.events.EiffelBaseEvent`
    """
    for response in request(etos, query, search):
        if response:
            try:
                _, event = next(
                    etos.graphql.search_for_nodes(response, node)
                )
            except StopIteration:
                return None
            return event
    return None


def request_suite(etos, suite_id):
    """Request a tercc from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param suite_id: ID of execution recipe.
    :type suite_id: str
    :return: Response from graphql or None
    :rtype: dict or None
    """
    return get_one(etos, TEST_SUITE, Search(
        search={"meta.id": suite_id}
    ), "testExecutionRecipeCollectionCreated")


def request_activity(etos, suite_id):
    """Request an activity event from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param suite_id: ID of execution recipe triggering this activity.
    :type suite_id: str
    :return: Response from graphql or None
    :rtype: dict or None
    """
    return get_one(etos, ACTIVITY_TRIGGERED, Search(
        search={"links.type": "CAUSE", "links.target": suite_id}
    ), "activityTriggered")


def request_activity_canceled(etos, activity_id):
    """Request an activity event from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param activity_id: ID of the activity beeing canceled.
    :type activity_id: str
    :return: Response from graphql or None
    :rtype: dict or None
    """
    return get_one(etos, ACTIVITY_CANCELED, Search(
        search={"links.type": "ACTIVITY_EXECUTION", "links.target": activity_id}
    ), "activityCanceled")


def request_test_suite_started(etos, activity_id):
    """Request test suite started from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param activity_id: ID of activity in which the test suites started
    :type activity_id: str
    :return: Iterator of test suite started graphql responses.
    :rtype: iterator
    """
    yield from search_for(etos, TEST_SUITE_STARTED, Search(
        search={"links.type": "CAUSE", "links.target": activity_id}
    ), "testSuiteStarted")


def request_main_test_suites_started(etos, activity_id):
    """Request test suite started from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param activity_id: ID of activity in which the test suites started
    :type activity_id: str
    :return: Iterator of test suite started graphql responses.
    :rtype: iterator
    """
    yield from search_for(etos, MAIN_TEST_SUITES_STARTED, Search(
        search={
            "links.type": "CONTEXT",
            "links.target": activity_id,
            "data.categories": {"$ne": "Sub suite"}
        }
    ), "testSuiteStarted")


def request_test_suite_finished(etos, test_suite_id):
    """Request test suite finished from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param test_suite_id: Test suite started ID of which finished to search for.
    :type test_suite_id: list
    :return: Test suite finished graphql response.
    :rtype: dict
    """
    return get_one(etos, TEST_SUITE_FINISHED, Search(
        search={"links.type": "TEST_SUITE_EXECUTION", "links.target": test_suite_id},
        last=1
    ), "testSuiteFinished")


def request_announcements(etos, ids):
    """Request announcements from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param ids: list of IDs of which announcements to search for.
    :type ids: list
    :return: Iterator of announcement published graphql responses.
    :rtype: iterator
    """
    yield from search_for(etos, ANNOUNCEMENTS, Search(
        search={"$or": [{"links.target": _id} for _id in ids]}
    ), "announcementPublished")


def request_environment(etos, ids):
    """Request environments from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param ids: list of IDs of which environments to search for.
    :type ids: list
    :return: Iterator of environnent defined graphql responses.
    :rtype: iterator
    """
    yield from request_all(etos, ENVIRONMENTS, Search(
        search={"$or": [{"links.target": _id} for _id in ids]}
    ), "environmentDefined")


def request_artifacts(etos, context):
    """Request artifacts from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param context: ID of the activity used in CONTEXT.
    :type context: str
    """
    yield from search_for(etos, ARTIFACTS, Search(
        search={"links.type": "CONTEXT", "links.target": context}
    ), "artifactCreated")
