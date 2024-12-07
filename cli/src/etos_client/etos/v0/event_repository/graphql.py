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
"""GraphQL query handler."""
from typing import Iterator, Optional
from etos_lib import ETOS
from .graphql_queries import (
    ARTIFACTS,
    ACTIVITY_TRIGGERED,
    ACTIVITY_FINISHED,
    ACTIVITY_CANCELED,
    MAIN_TEST_SUITES_STARTED,
    TEST_SUITE_STARTED,
    TEST_SUITE_FINISHED,
    TEST_SUITE,
    TEST_CASE_FINISHED,
    TEST_CASE_CANCELED,
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

    def __str__(self) -> str:
        """Convert dictionary to an eiffel graphql API compatible string."""
        strings = []
        for key, value in self.items():
            if key == "search":
                strings.append(f'search: "{self["search"]}"')
            else:
                strings.append(f"{key}: {value}")
        return ", ".join(strings)


def request(etos: ETOS, query: str, search: Search) -> Iterator[dict]:
    """Request graphql in a generator."""
    wait_generator = etos.utils.wait(etos.graphql.execute, query=query % search, timeout=60)
    yield from wait_generator


def search_for(etos: ETOS, query: str, search: Search, node: str) -> Iterator[dict]:
    """Request graphql and search for node in response."""
    for response in request(etos, query, search):
        if response:
            for _, event in etos.graphql.search_for_nodes(response, node):
                yield event
            return None  # StopIteration
    return None  # StopIteration


def request_all(etos: ETOS, query: str, search: Search, node: str) -> Iterator[dict]:
    """Request all events for a given query."""
    limit = 100
    search["last"] = limit
    while True:
        last_event = None
        count = 0
        for event in search_for(etos, query, search, node):
            if event.get("meta", {}).get("time") is None:
                raise AssertionError(
                    "Meta time must be added to graphql query in order to use this function"
                )
            last_event = event
            count += 1
            yield event
        if last_event is None:
            return None  # StopIteration
        if count < limit:
            return None  # StopIteration
        search["search"]["meta.time"] = {"$lt": last_event["meta"]["time"]}


def get_one(etos: ETOS, query: str, search: Search, node: str) -> Optional[dict]:
    """Request graphql and get a single node from response."""
    for response in request(etos, query, search):
        if response:
            try:
                _, event = next(etos.graphql.search_for_nodes(response, node))
            except StopIteration:
                return None
            return event
    return None


def request_suite(etos: ETOS, suite_id: str) -> Optional[dict]:
    """Request a tercc from graphql."""
    return get_one(
        etos,
        TEST_SUITE,
        Search(search={"meta.id": suite_id}),
        "testExecutionRecipeCollectionCreated",
    )


def request_activity(etos: ETOS, suite_id: str) -> Optional[dict]:
    """Request an activity event from graphql."""
    return get_one(
        etos,
        ACTIVITY_TRIGGERED,
        Search(search={"links.type": "CAUSE", "links.target": suite_id}),
        "activityTriggered",
    )


def request_activity_finished(etos: ETOS, activity_id: str) -> Optional[dict]:
    """Request an activity finished event from graphql."""
    return get_one(
        etos,
        ACTIVITY_FINISHED,
        Search(search={"links.type": "ACTIVITY_EXECUTION", "links.target": activity_id}),
        "activityFinished",
    )


def request_activity_canceled(etos: ETOS, activity_id: str) -> Optional[dict]:
    """Request an activity event from graphql."""
    return get_one(
        etos,
        ACTIVITY_CANCELED,
        Search(search={"links.type": "ACTIVITY_EXECUTION", "links.target": activity_id}),
        "activityCanceled",
    )


def request_sub_test_suite_started(etos: ETOS, main_suite_id: str) -> Iterator[dict]:
    """Request test suite started from graphql."""
    yield from search_for(
        etos,
        TEST_SUITE_STARTED,
        Search(search={"links.type": "CAUSE", "links.target": main_suite_id}),
        "testSuiteStarted",
    )


def request_main_test_suites_started(etos: ETOS, activity_id: str) -> Iterator[dict]:
    """Request test suite started from graphql."""
    yield from search_for(
        etos,
        MAIN_TEST_SUITES_STARTED,
        Search(
            search={
                "links.type": "CONTEXT",
                "links.target": activity_id,
                "data.categories": {"$ne": "Sub suite"},
            }
        ),
        "testSuiteStarted",
    )


def request_test_suite_finished(etos: ETOS, test_suite_id: str) -> Optional[dict]:
    """Request test suite finished from graphql."""
    return get_one(
        etos,
        TEST_SUITE_FINISHED,
        Search(
            search={
                "links.type": "TEST_SUITE_EXECUTION",
                "links.target": test_suite_id,
            },
            last=1,
        ),
        "testSuiteFinished",
    )


def request_environment(etos: ETOS, ids: list[str]) -> Iterator[dict]:
    """Request environments from graphql."""
    yield from request_all(
        etos,
        ENVIRONMENTS,
        Search(search={"$or": [{"links.target": _id} for _id in ids]}),
        "environmentDefined",
    )


def request_artifacts(etos: ETOS, cause: str) -> Iterator[dict]:
    """Request artifacts from graphql."""
    yield from request_all(
        etos,
        ARTIFACTS,
        Search(search={"links.type": "CAUSE", "links.target": cause}),
        "artifactCreated",
    )


def request_test_case_finished(
    etos: ETOS, test_suite_id: str, last_event: Optional[dict] = None
) -> Iterator[dict]:
    """Request test case finished from graphql."""
    search = Search(search={"links.type": "CONTEXT", "links.target": test_suite_id})
    if last_event is not None:
        search["search"]["meta.time"] = {"$lt": last_event["meta"]["time"]}
    yield from request_all(
        etos,
        TEST_CASE_FINISHED,
        search,
        "testCaseFinished",
    )


def request_test_case_canceled(
    etos: ETOS, test_suite_id: str, last_event: Optional[dict] = None
) -> Iterator[dict]:
    """Request test case canceled from graphql."""
    search = Search(search={"links.type": "CONTEXT", "links.target": test_suite_id})
    if last_event is not None:
        search["search"]["meta.time"] = {"$lt": last_event["meta"]["time"]}
    yield from request_all(
        etos,
        TEST_CASE_CANCELED,
        search,
        "testCaseCanceled",
    )
