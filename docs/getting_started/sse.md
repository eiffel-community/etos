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
# Following a testrun with SSE

ETOS streams the events and logs of a testrun over [Server-Sent Events (SSE)](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events). This is what the ETOS client uses to show progress while a testrun is running. This page describes the SSE `v2` event protocol (currently served under `v2alpha`) and the events it emits. The events are formally defined by the [messaging events JSON schema](https://github.com/eiffel-community/etos/blob/main/schemas/messaging/v2alpha/events.schema.json).

## Connecting

Connect to the SSE server to start receiving events for a testrun:

```
GET https://etos-api-instance/sse/v2alpha/events/{identifier}
```

The `identifier` is the testrun ID, the `tercc` value returned in the response when the testrun is started via the [API](api.md).

### Query parameters

- `filter`: Only receive events of a certain kind. A filter has the form `type.meta`, where `type` is the lower-cased event type (`message`, `status`, `report`, ...) and `meta` is an event-specific value: the log level for `message` (e.g. `info`, `error`), the service name for `status` (e.g. `etos-suite-runner`), and `*` for every other event. The parameter may be passed multiple times, e.g. `?filter=message.info&filter=message.error` to receive only info and error logs, or `?filter=status.etos-suite-runner` for status events from the suite runner.

### Resuming a stream

If the connection drops, reconnect and send the `id` of the last event you received in the `Last-Event-ID` HTTP header. The server then replays every event after that id so that none are missed:

```bash
curl -N \
  -H 'Last-Event-ID: 42' \
  'https://etos-api-instance/sse/v2alpha/events/{identifier}'
```

## Event format

Each event is sent as a standard SSE block with an `id`, an `event` type and a JSON `data` payload:

```
id: 1
event: message
data: {"message": "Starting testrun", "name": "etos", "level": "info", "@timestamp": "2026-08-31T10:00:00Z"}

```

The `id` is a monotonically increasing integer used for resuming a stream. The `event` field is one of the types below.

## Events

The events are split into events meant for the client to act on (server events) and events meant to be presented to the user (user events).

### Server events

| Event | Data | Description |
| --- | --- | --- |
| `ping` | none | Sent every 15 seconds to keep the connection alive. |
| `error` | none | The server encountered an error. The client should reconnect. |

### User events

| Event | Data type | Description |
| --- | --- | --- |
| `message` | `Log` | A user facing log message from ETOS. |
| `report` | `File` | A test case report file. |
| `artifact` | `File` | A test case artifact file. |
| `status` | `ServiceStatus` | The current status of an ETOS service. |
| `shutdown` | `Result` | The testrun has finished. This is always the last event. |
| `unknown` | none | An event that does not match any known type. |

## Data types

### Log

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `message` | string | yes | The log message. |
| `name` | string | yes | The name of the logger that produced the message. |
| `level` | string | no | Log level, e.g. `info` or `error`. Defaults to `info`. |
| `@timestamp` | string | yes | ISO 8601 timestamp of when the message was created (also published as `datestring`). |

A `Log` may contain additional context fields depending on the source of the log.

### File

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `url` | string | yes | The URL to the file. |
| `name` | string | yes | The name of the file. |
| `directory` | string | no | The directory the file belongs to. |
| `checksums` | object | no | A map of checksum algorithm to checksum value. |

### ServiceStatus

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | The name of the service. |
| `instance` | string | no | The specific instance of the service. |
| `version` | string | yes | The version of the service. |
| `status` | string | yes | The health of the service, either `ok` or `error`. |
| `message` | string | no | A message describing the status. |

### Result

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `conclusion` | string | yes | The conclusion of the testrun. One of `Successful`, `Failed`, `Aborted`, `TimedOut`, `Inconclusive`. |
| `verdict` | string | yes | The verdict of the testrun. One of `Passed`, `Failed`, `Inconclusive`, `None`. |
| `description` | string | no | A description of the result. |
