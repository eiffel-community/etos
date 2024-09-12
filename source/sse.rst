.. _sse:

############
SSE protocol
############

ETOS communicates to clients using SSE (server sent events). For now ETOS only sends log messages using the protocol but we are adding more data about testruns into the protocol.

.. list-table:: SSE Protocol
  :widths: 25 25 25 25
  :header-rows: 1

  * - Name
    - Why
    - Example
    - State
  * - Ping
    - Keep the connection alive
    - {id: None, event: Ping, data: ""}
    - Implemented
  * - Shutdown
    - Server wants the client to shut down the connection
    - {id: None, event: Shutdown, data: "ESR has shut down"}
    - Implemented
  * - NotFound
    - The server cannot find the ESR instance, retry after a while
    - {id: None, event: NotFound, data: "ESR not found"}
    - Suggested
  * - Error
    - The server got an error. Retry may be possible
    - {id: None, event: Error, data: '{"retry": bool, "reason": "Some sort of error"}}
    - Suggested
  * - Message
    - A user log message from ETOS
    - {id: 1, event: message, data: "{'message': 'a message', '@timestamp': '%Y-%m-%dT%H:%M:%S.%fZ', 'levelname': 'INFO', 'name': 'ESR'}"}
    - Implemented
  * - Artifact
    - An artifact to download
    - {id: 1, event: Artifact, data: "{'url': 'http://download.me', 'name': 'filename.txt', 'directory': 'MyTest_SubSuite_0', 'checksums': {'SHA-224': '<hash>', 'SHA-256': '<hash>', 'SHA-384': '<hash>', 'SHA-512': '<hash>', 'SHA-512/224': '<hash>', 'SHA-512/256': '<hash>'}}"}
    - Implemented
  * - Report
    - A report to download
    - {id: 1, event: Report, data: "{'url': 'http://download.me', 'name': 'filename.txt', 'checksums': {'SHA-224': '<hash>', 'SHA-256': '<hash>', 'SHA-384': '<hash>', 'SHA-512': '<hash>', 'SHA-512/224': '<hash>', 'SHA-512/256': '<hash>'}}"}
    - Implemented
  * - TestCase
    - A test case execution
    - {id: 1, event: TestCase, data: "{'id': 'uuid', 'triggered': True, 'started': True, 'finished': False}"}
    - Suggested
