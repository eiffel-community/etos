.. _tercc:

====================
ETOS Test Collection
====================

This page describes the ETOS test collection.

A test collection is how you trigger your tests. The test collection is a collection of recipes that are defined by the test automation engineers.
These recipes defines how ETOS will execute the tests.

This test collection is JSON file with several elements and can be quite tricky to create manually.


Structure
=========

Top-level properties
--------------------

.. code-block:: JSON

   [
      {
         "name": "Name of you suite",
         "priority": 1,
         "recipes": []
      }
   ]

.. list-table :: Top level properties
   :widths: 25 25 25
   :header-rows: 1

   * - Property
     - Type
     - Description
   * - name
     - String
     - Test suite name. Will be referenced in the main test suite started.
   * - priority
     - Integer
     - The execution priority. Unused at present and should be set to 1.
   * - recipes
     - List
     - The collection of test execution recipes. Each recipe is a test case.

Recipes
-------

.. code-block:: JSON

   [
      {
         "recipes": [
            {
               "constraints": [],
               "id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
               "testCase": {
                  "id": "test_case_name",
                  "tracker": "Name of test case tracker",
                  "url": "http://testcasetracker.com/test_case_name"
               }
            }
         ]
      }
   ]

.. list-table :: Recipe properties
   :widths: 25 25 25
   :header-rows: 1

   * - Property
     - Type
     - Description
   * - constraints
     - List
     - List of constraints describing how to execute this test case.
   * - id
     - String
     - Unique ID for this test case. Can be used to re-trigger failing test cases.
   * - testCase
     - Dictionary
     - Test case metadata.
   * - testCase/id
     - String
     - Name of this test case.
   * - testCase/tracker
     - String
     - The name of the tracker where this test case can be found.
   * - testCase/url
     - String
     - URL to the tracker for this test case.

Constraints
-----------

.. code-block:: JSON

   [
      {
         "recipes": [
            {
               "constraints": [
                  {
                     "key": "ENVIRONMENT",
                     "value": {
                        "MY_ENVIRONMENT": "my_environment_value"
                     }
                  },
                  {
                     "key": "COMMAND",
                     "value": "tox"
                  },
                  {
                     "key": "PARAMETERS",
                     "value": {
                        "-e": "py3"
                     }
                  },
                  {
                     "key": "TEST_RUNNER",
                     "value": "eiffel-community/etos-python-test-runner"
                  },
                  {
                     "key": "EXECUTE",
                     "value": [
                        "echo 'hello world'"
                     ]
                  },
                  {
                     "key": "CHECKOUT",
                     "value": [
                        "git clone https://github.com/eiffel-community/etos-client"
                     ]
                  }
               ]
            }
         ]
      }
   ]

.. list-table :: Constraint properties
   :widths: 25 25 25
   :header-rows: 1

   * - Property
     - Value
     - Description
   * - ENVIRONMENT
     - Dictionary
     - The environment key defines which environment variables that are needed for this test case execution.
   * - COMMAND
     - String
     - The command key defines which command to execute in order to run the specified test case.
   * - PARAMETERS
     - Dictionary
     - The parameters key defines which parameters you want to supply to the command that is executing the tests.
   * - TEST_RUNNER
     - String
     - Which test runner you need to execute the test cases in: See :ref:`etos-test-runner-containers` for more information.
   * - EXECUTE
     - List
     - The execute key defines a set of shell commands to execute before this test case.
   * - CHECKOUT
     - List
     - The checkout key defines how to checkout your test cases. The checkout values are executed in bash. This command is only executed once if it has already been executed.
