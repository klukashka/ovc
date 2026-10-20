# Observability specification

Structured diagnostics for every executable OVC component. Diagnostics are for operating and
debugging the system; they are never the source of truth for application state and never an audit
record of academic data.

## Events and levels

- Logs are structured records with a stable dotted event name (`auth.login.succeeded`,
  `chat.message.persisted`, `ws.connection.closed`, `meeting.room.created`, `upload.rejected`) and
  typed fields. Free-form message strings are not the contract; event names and field names are.
- Each event has one reporting owner: the layer that decides the outcome. A service logs the
  business outcome; a router does not log it again. Repositories do not log successful queries.
- Levels:

| Level | Use |
|---|---|
| `ERROR` | A request or background operation failed in a way an operator must look at |
| `WARNING` | A degraded but handled condition: provider timeout with a retry, rejected upload, refresh-token reuse |
| `INFO` | Lifecycle and business outcomes at a rate proportional to user actions |
| `DEBUG` | Detail for local investigation; off by default outside development |

- Polling, heartbeats and `ping`/`pong` frames never log at `INFO`.

## Correlation

- Every HTTP request gets a request identifier, taken from a valid incoming `X-Request-ID` or
  generated, bound to the logging context for the duration of the request, returned in the response
  header and in error bodies ([API conventions](api.md#conventions)).
- Every WebSocket connection gets a connection identifier bound for its lifetime; every inbound frame
  gets its own frame identifier.
- Records carry `user_id` and `role` once authenticated, and `course_id` where the operation is
  course-scoped. Context is reset at the end of each request, connection or task so that concurrent
  work never shares context.

## Safe content

Never written to diagnostics at any level, including `DEBUG` and exception output:

- passwords, access and refresh tokens, signing secrets, provider tokens, signed join URLs, cookies
  and `Authorization` headers;
- chat message bodies, assignment instructions, submission text, feedback, uploaded file contents;
- email addresses and full names (log the user identifier instead);
- full request or response bodies.

Exception handlers log the exception type, the event and the correlation fields; exception messages
from libraries that may embed request data are not logged verbatim. File names of uploads are logged
only after truncation and only at `DEBUG`.

## Output and failure behavior

- Process entry points configure logging once from `OVC_LOG_LEVEL` and `OVC_LOG_FORMAT` (`json` for
  deployments, `console` for development). Libraries and packages use bound loggers and never
  configure handlers.
- Output is bounded: a record over a size limit is truncated with a marker, never dropped silently.
- A failing log sink never fails a request or blocks the event loop.

## Component coverage

| Component | Required events from its first implementation |
|---|---|
| Application process | startup with configuration summary (no secrets), shutdown, unhandled exception |
| Authentication | login succeeded and failed (no email in the record), refresh rotated, refresh reuse detected, logout |
| REST routers | one access record per request: method, route template, status, duration, request identifier |
| WebSocket gateway | connection opened, authenticated, closed with code and reason, frame rejected |
| Chat | message persisted, fan-out completed with recipient count and duration |
| Meetings | room created, join authorized and refused, room ended, provider call failed |
| Homework and uploads | assignment published, submission submitted, graded and returned, upload accepted and rejected with reason code |

Tests assert on event names and fields for the outcomes above, and assert that forbidden content
never appears in captured records.
