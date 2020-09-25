.. _etos-test-runner:

================
ETOS Test Runner 
================

The ETOS executor.

Github: `ETOS Test Runner <https://github.com/eiffel-community/etos-test-runner>`_

This page describes the ETOS test runner (ETR)

ETOS test runner, or short ETR, is the base test runner that executes tests defined in an ETOS test recipe collection.
The test collection contains information from the Test Execution Recipe Collection Created Event (`TERCC <https://github.com/eiffel-community/eiffel/blob/master/eiffel-vocabulary/EiffelTestExecutionRecipeCollectionCreatedEvent.md>`_) and also some additional meta data about the test environment.

Test execution
==============

When ETR executes tests it communicates the following events to the Eiffel message bus (RabbitMQ)

- `Test Suite Started <https://github.com/eiffel-community/eiffel/blob/master/eiffel-vocabulary/EiffelTestSuiteStartedEvent.md>`_
- `Test Case Triggered <https://github.com/eiffel-community/eiffel/blob/master/eiffel-vocabulary/EiffelTestCaseTriggeredEvent.md>`_
- `Test Case Started <https://github.com/eiffel-community/eiffel/blob/master/eiffel-vocabulary/EiffelTestCaseStartedEvent.md>`_
- `Test Case Finished <https://github.com/eiffel-community/eiffel/blob/master/eiffel-vocabulary/EiffelTestCaseFinishedEvent.md>`_
- `Test Suite Finished <https://github.com/eiffel-community/eiffel/blob/master/eiffel-vocabulary/EiffelTestSuiteFinishedEvent.md>`_
- `Confidence Level Modified <https://github.com/eiffel-community/eiffel/blob/master/eiffel-vocabulary/EiffelConfidenceLevelModifiedEvent.md>`_

ETR always executes one test suite and several test cases within that suite, thus sending only one test suite started/finished pair but several collections of test case triggered/started/finished

When the test suite has finished executing ETR send a Confidence Level Modified event signaling the Confidence level for the test suite it just executed. The confidence level event is typically in larger aggregations of eiffel events to give basis for CI pipeline driving activities.

Test artifacts
==============

When a test is executed generally there are test artifacts created. These artifacts are typically test results, test logs and specific logs and debugging information from the IUT (item under test). All of the test artifacts are uploaded to a log area , such as Jfrog Artifactory.
