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

If the connection drops, or the server closes the stream before the `shutdown` event, reconnect and send the `id` of the last event you received in the `Last-Event-ID` HTTP header. The server then replays every event after that id so that none are missed:

```bash
curl -N \
  -H 'Last-Event-ID: 42' \
  'https://etos-api-instance/sse/v2alpha/events/{identifier}'
```

A client may receive an event more than once around a reconnect. Drop every event with an `id` that is not greater than the last `id` you received.

If the events after `Last-Event-ID` are no longer stored by ETOS, for instance because they have expired, the server sends a non-retryable `error` event and closes the stream. The client cannot receive all events of the testrun and must not reconnect. The server sends the same error if the event stream is empty. A `Last-Event-ID` that is ahead of the last event in a non-empty stream cannot be detected, and the server then waits for new events after that id; only send ids received from the server.

## Event format

Each event is sent as a standard SSE block with an `id`, an `event` type and a JSON `data` payload:

```
id: 4711
event: message
data: {"message": "Starting testrun", "name": "etos", "level": "info", "@timestamp": "2026-08-31T10:00:00Z"}

```

The `id` is the position of the event in the ETOS event stream plus one, and is used for resuming a stream. Ids are strictly increasing but not contiguous: the stream contains events for all testruns and events removed by a filter, so gaps between ids are expected and do not mean that events were lost. Treat the `id` as opaque and do not expect consecutive ids. The `event` field is one of the types below.

## Events

The events are split into events meant for the client to act on (server events) and events meant to be presented to the user (user events).

### Server events

| Event | Data | Description |
| --- | --- | --- |
| `ping` | none | Sent every 15 seconds to keep the connection alive. If the server has passed events that were not sent to the client, the ping has an `id` that the client should store as its last event id, so that a reconnect does not have to scan those events again. |
| `error` | `{"retry": bool, "reason": string}` | The server encountered an error. If `retry` is `true`, the client should reconnect with `Last-Event-ID`. If `retry` is `false`, the client must not reconnect; this happens, for instance, when the events after `Last-Event-ID` have expired. |

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
| `@timestamp` | string | yes | ISO 8601 timestamp of when the message was created. |

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
