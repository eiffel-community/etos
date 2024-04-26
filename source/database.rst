.. _database:

##################
Database structure
##################

Since the change to using ETCD as a database and to use it more widely in ETOS we have created a table of paths that we are using in ETCD.

.. list-table:: Title
  :widths: 50 50
  :header-rows: 1

  * - Path
    - Description
  * - /environment/{task_id}/suite-id
    - Backward Compatibility - The suite ID for a celery task
  * - /environment/{UUID:environment}/testrun-id
    - Backward Compatibility - The test run ID for an environment
  * - /environment/{UUID:environment}/suite-idi
    - Backward Compatibility - The suite ID for an environment
  * - /environment/provider/execution-space
    - Available execution space providers, registered in the environment provider
  * - /environment/provider/execution-space/{NAME}
    - A specific execution space provider, registered in the environment provider
  * - /environment/provider/iut
    - Available IUT providers, registered in the environment provider
  * - /environment/provider/iut/{NAME}
    - A specific IUT provider, registered in the environment provider
  * - /environment/provider/log-area
    - Available log area providers, registered in the environment provider
  * - /environment/provider/log-area/{NAME}
    - A specific log area provider, registered in the environment provider
  * - /testrun/{UUID:tercc}
    - Information about an ETOS testrun
  * - /testrun/{UUID:tercc}/tercc
    - The TERCC that is used for an ETOS testrun
  * - /testrun/{UUID:tercc}/sse
    - Logs and events for the SSE stream
  * - /testrun/{UUID:tercc}/environment-provider/task-id
    - Celery task ID for the environment provider
  * - /testrun/{UUID:tercc}/config
    - Testrun configuration
  * - /testrun/{UUID:tercc}/config/timeout/execution-space
    - Timeout for checking out execution spaces
  * - /testrun/{UUID:tercc}/config/timeout/iut
    - Timeout for checking out IUTs
  * - /testrun/{UUID:tercc}/config/timeout/log-area
    - Timeout for checking out log area
  * - /testrun/{UUID:tercc}/config/timeout/total
    - Total timeout for a full etos testrun
  * - /testrun/{UUID:tercc}/provider/execution-space
    - Execution space provider to use for the testrun
  * - /testrun/{UUID:tercc}/provider/iut
    - IUT provider to use for the testrun
  * - /testrun/{UUID:tercc}/provider/log-area
    - Log area provider to use for the testrun
  * - /testrun/{UUID:tercc}/provider/dataset
    - Dataset to use for the testrun
  * - /testrun/{UUID:tercc}/suite/{UUID:testsuite}/subsuite/{UUID:environment}/iut
    - IUT to use for a specific sub suite, updated by providers
  * - /testrun/{UUID:tercc}/suite/{UUID:testsuite}/subsuite/{UUID:environment}/execution-space
    - Execution space to use for a specific sub suite, updated by providers
  * - /testrun/{UUID:tercc}/suite/{UUID:testsuite}/subsuite/{UUID:environment}/log-area
    - Log area to use for a specific sub suite, updated by providers
  * - /testrun/{UUID:tercc}/suite/{UUID:testsuite}/subsuite/{UUID:environment}/suite
    - The suite that we send to the etos test runner
