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
- Records carry `user_id` and `role` once authenticated, and `tutor_id` plus `relationship_id` or
  `group_id` where the operation is scoped to them. Worker records carry the job name and the outbox
  row identifier. Context is reset at the end of each request, connection, task or job so that
  concurrent work never shares context.

## Safe content

Never written to diagnostics at any level, including `DEBUG`, error and panic output:

- passwords, access and refresh tokens, signing secrets, provider tokens, signed join URLs, cookies
  and `Authorization` headers;
- invitation, email-confirmation, password-reset and unsubscribe tokens, callback path tokens,
  signed storage URLs, and the provider's `signed_parameters` value;
- chat message bodies, assignment instructions and prompts, answers and answer keys, submission
  text, feedback, tutor notes, uploaded file contents;
- email addresses and full names (log the user identifier instead);
- full request or response bodies, and full URLs of routes that carry tokens in their path (log the
  route template instead).

The reverse proxy and any static-file server log route paths with token segments and query strings replaced by a placeholder, so token pages and callback routes never leave tokens or routing keys in access logs.

Error records carry the event, a stable error kind or API error code, and the correlation fields. The
text of a wrapped error chain (`err.Error()`) is not logged verbatim, because library errors embed
request data (for example a PostgreSQL constraint detail that contains an email); an error type that is
known to carry no request data may expose a safe description through its own method. Panic records
carry the event, the stack and the type of the panic value, never the value itself. The recovery
middleware is the outermost HTTP handler, so the standard library's own panic logging never runs. File names of
uploads are logged only after truncation and only at `DEBUG`.

## Output and failure behavior

- Process entry points configure logging once from `OVC_LOG_LEVEL` and `OVC_LOG_FORMAT` (`json` for
  deployments, `console` for development). Libraries and packages use bound loggers and never
  configure handlers.
- Output is bounded: a long string value is cut with a marker, and a whole record over the size limit is replaced by a `logging.record_truncated` notice with the event name and the original size, so every line stays valid. Records are never dropped silently: a full queue counts the dropped records and reports the count on the next record and at close as `logging.records_dropped`. Values are resolved when the record is logged, not when it is written. Closing the logger waits at most 5 s for the sink and reports a timeout on standard error.
- A failing log sink never fails a request or blocks a request goroutine.
- `http.Server.ErrorLog` and every library logger are bridged into `log/slog`, so nothing writes
  unstructured output.
- Every goroutine the backend starts (WebSocket readers and writers, the `NOTIFY` listener, worker
  jobs) runs through one helper that recovers a panic, logs it as above and ends or restarts only that
  goroutine, so one failure never crashes the process. The log writer's own drain loop is the one
  exception: it recovers inline and counts a record whose formatting panicked as dropped, because it
  cannot log through itself.

## Component coverage

| Component | Required events from its first implementation |
|---|---|
| Application and worker processes | startup with configuration summary (no secrets), shutdown, recovered panic, failure to close the database handle (`db.close.failed` with a kind) |
| Accounts | sign-up, email confirmed, login succeeded and failed (no email in the record), throttled, refresh rotated, refresh reuse detected, logout and logout-all, password reset and change, account deleted, restored and purged. Event names so far: `auth.login.succeeded` (INFO), `auth.login.failed` (INFO, `reason` one of `unknown_account`, `password`, `deleted`, `blocked`, `changed`, the last when the password or the account changed between verification and the start of the session), `auth.login.throttled` (WARNING, `scope` `email` or `address`), `auth.refresh.rotated` (DEBUG), `auth.refresh.reuse_detected` (WARNING), `auth.logout` and `auth.logout_all` (INFO); `auth.signup.accepted`, `auth.signup.duplicate`, `auth.email.confirmed` (with `path` `signed_in` or `new_password`), `auth.email.limited` (a request over a per-account sending limit, with `purpose`), `auth.password.reset_requested`, `auth.password.reset`, `auth.password.changed` and `auth.password.change_refused` (INFO), `auth.request.throttled` (WARNING, `scope` `address`) and `auth.password.throttled` (WARNING); on unexpected failures `auth.register.failed`, `auth.verify_email.failed`, `auth.resend.failed`, `auth.password.forgot_failed`, `auth.password.reset_failed` and `auth.password.change_failed` (ERROR, with an error kind); on unexpected failures `auth.authenticate.failed`, `auth.login.error`, `auth.refresh.failed`, `auth.logout.failed`, `auth.logout_all.failed`, `users.me.failed` and `http.client_address.unknown` (ERROR, with an error kind); `auth.login.abandoned` (DEBUG) when the request is cancelled or times out during sign-in, and `http.response.write_failed` (WARNING) when an error body cannot be written. They carry the user identifier in its UUID form, never an email, password, token or cookie |
| Operator command | every command with operator system user, target user identifier and outcome |
| Relationships and groups | invitation created, accepted and refused with reason code; join request created and decided; relationship ended; group created, membership changed, archived |
| REST routers | one access record per request: method, route template, status, duration, request identifier |
| WebSocket gateway | connection opened, authenticated, closed with code and reason, frame rejected |
| Schedule | lesson or series created, edited with scope, split, cancelled; overlap warning returned; status changed, including automatic end by the status job; series horizon extended and skipped for inactive audiences |
| Chat | message persisted, fan-out completed with recipient count and duration |
| Meetings and recordings | meeting created and re-created, join authorized and refused with reason code, meeting ended (hint received, confirmed), callback accepted and rejected with status, reconciliation batch, recording copied, held for quota, retried, failed, expired and deleted, provider copy deleted, provider call failed |
| Homework | template saved, issue created, deadline changed and withdrawn, submission submitted (with late flag), autosave conflict, review revision published as checked, returned or updated, score override applied |
| Files | upload accepted, rejected and aborted with reason code, quota exceeded, reference added and removed, file deleted |
| Notifications and outbox | notification created, outbox row claimed, done (sent or obsolete), retried, lease reclaimed and dead with error code, digest sent, unsubscribe applied, email marked undeliverable, scheduled job run with batch size and duration. Outbox and job event names: `outbox.row.claimed` (DEBUG), `outbox.row.done` (INFO), `outbox.row.retried` (WARNING), `outbox.lease.reclaimed` (WARNING), `outbox.lease.lost` (WARNING, a late finisher whose lease was reclaimed), `outbox.row.dead` (ERROR), `job.run` (INFO, ERROR when the job failed), `smtp.connection.discard_failed` (WARNING, with an error kind, when an idle SMTP connection cannot be closed cleanly); their fields are the row identifier, lane, kind, attempts and error code, never a payload, recipient or message content |

Tests assert on event names and fields for the outcomes above, and assert that forbidden content
never appears in captured records.
