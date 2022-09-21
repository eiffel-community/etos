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


def request(etos, query):
    """Request graphql in a generator.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param query: Query to send to graphql.
    :type query: str
    :return: Generator
    :rtype: generator
    """
    wait_generator = etos.utils.wait(etos.graphql.execute, query=query, timeout=60)
    yield from wait_generator


def request_suite(etos, suite_id):
    """Request a tercc from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param suite_id: ID of execution recipe.
    :type suite_id: str
    :return: Response from graphql or None
    :rtype: dict or None
    """
    for response in request(etos, TEST_SUITE % suite_id):
        try:
            _, tercc = next(
                etos.graphql.search_for_nodes(
                    response, "testExecutionRecipeCollectionCreated"
                )
            )
        except StopIteration:
            return None
        return tercc
    return None


def request_activity(etos, suite_id):
    """Request an activity event from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param suite_id: ID of execution recipe triggering this activity.
    :type suite_id: str
    :return: Response from graphql or None
    :rtype: dict or None
    """
    for response in request(etos, ACTIVITY_TRIGGERED % suite_id):
        if response:
            try:
                _, activity = next(
                    etos.graphql.search_for_nodes(response, "activityTriggered")
                )
            except StopIteration:
                return None
            return activity
    return None


def request_activity_canceled(etos, activity_id):
    """Request an activity event from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param activity_id: ID of the activity beeing canceled.
    :type activity_id: str
    :return: Response from graphql or None
    :rtype: dict or None
    """
    for response in request(etos, ACTIVITY_CANCELED % activity_id):
        if response:
            try:
                _, activity_canceled = next(
                    etos.graphql.search_for_nodes(response, "activityCanceled")
                )
            except StopIteration:
                return None
            return activity_canceled
    return None


def request_test_suite_started(etos, activity_id):
    """Request test suite started from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param activity_id: ID of activity in which the test suites started
    :type activity_id: str
    :return: Iterator of test suite started graphql responses.
    :rtype: iterator
    """
    for response in request(etos, TEST_SUITE_STARTED % activity_id):
        if response:
            for _, test_suite_started in etos.graphql.search_for_nodes(
                response, "testSuiteStarted"
            ):
                yield test_suite_started
            return None  # StopIteration
    return None  # StopIteration


def request_main_test_suites_started(etos, activity_id):
    """Request test suite started from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param activity_id: ID of activity in which the test suites started
    :type activity_id: str
    :return: Iterator of test suite started graphql responses.
    :rtype: iterator
    """
    for response in request(etos, MAIN_TEST_SUITES_STARTED % activity_id):
        if response:
            for _, test_suite_started in etos.graphql.search_for_nodes(
                response, "testSuiteStarted"
            ):
                yield test_suite_started
            return None  # StopIteration
    return None  # StopIteration


def request_test_suite_finished(etos, test_suite_id):
    """Request test suite finished from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param test_suite_id: Test suite started ID of which finished to search for.
    :type test_suite_id: list
    :return: Test suite finished graphql response.
    :rtype: dict
    """
    for response in request(etos, TEST_SUITE_FINISHED % test_suite_id):
        if response:
            try:
                _, test_suite_finished = next(
                    etos.graphql.search_for_nodes(response, "testSuiteFinished")
                )
            except StopIteration:
                return None
            return test_suite_finished
    return None


def request_announcements(etos, ids):
    """Request announcements from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param ids: list of IDs of which announcements to search for.
    :type ids: list
    :return: Iterator of announcement published graphql responses.
    :rtype: iterator
    """
    or_query = "{'$or': ["
    or_query += ", ".join([f"{{'links.target': '{_id}'}}" for _id in ids])
    or_query += "]}"
    for response in request(etos, ANNOUNCEMENTS % or_query):
        if response:
            for _, announcement in etos.graphql.search_for_nodes(
                response, "announcementPublished"
            ):
                yield announcement
            return None  # StopIteration
    return None  # StopIteration


def request_environment(etos, ids):
    """Request environments from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param ids: list of IDs of which environments to search for.
    :type ids: list
    :return: Iterator of environnent defined graphql responses.
    :rtype: iterator
    """
    or_query = "{'$or': ["
    or_query += ", ".join([f"{{'links.target': '{_id}'}}" for _id in ids])
    or_query += "]}"
    for response in request(etos, ENVIRONMENTS % or_query):
        if response:
            for _, environment in etos.graphql.search_for_nodes(
                response, "environmentDefined"
            ):
                yield environment
            return None  # StopIteration
    return None  # StopIteration


def request_artifacts(etos, context):
    """Request artifacts from graphql.

    :param etos: Etos Library instance for communicating with ETOS.
    :type etos: :obj:`etos_lib.etos.ETOS`
    :param context: ID of the activity used in CONTEXT.
    :type context: str
    """
    for response in request(etos, ARTIFACTS % context):
        if response:
            for _, artifact in etos.graphql.search_for_nodes(
                response, "artifactCreated"
            ):
                yield artifact
            return None  # StopIteration
    return None  # StopIteration
