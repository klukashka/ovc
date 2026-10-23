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
- Records carry `user_id` and `role` once authenticated: the request middleware binds the
  authenticated account's identifier (its UUID, never an email) and role to every later record of the
  request, the access record included, adding each field only to a record that does not name it
  itself. Within a request, `user_id` and `role` name the acting account; a record about another
  account names it with a field such as `target_user_id`. Records also carry `tutor_id` plus `relationship_id` or
  `group_id` where the operation is scoped to them. Worker records carry the job name and the outbox
  row identifier. Context is reset at the end of each request, connection, task or job so that
  concurrent work never shares context.

## Safe content

Never written to diagnostics at any level, including `DEBUG`, error and panic output:

- passwords, access and refresh tokens, signing secrets, provider tokens, signed join URLs, cookies
  and `Authorization` headers;
- invitation, email-confirmation, password-reset and unsubscribe tokens, tutor codes, callback path tokens,
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
| Accounts | sign-up, email confirmed, login succeeded and failed (no email in the record), throttled, refresh rotated, refresh reuse detected, logout and logout-all, password reset and change, account deleted, restored and purged. Event names so far: `auth.login.succeeded` (INFO), `auth.login.failed` (INFO, `reason` one of `unknown_account`, `password`, `deleted`, `blocked`, `changed`, the last when the password or the account changed between verification and the start of the session), `auth.login.throttled` (WARNING, `scope` `email` or `address`), `auth.refresh.rotated` (DEBUG), `auth.refresh.reuse_detected` (WARNING), `auth.logout` and `auth.logout_all` (INFO); `account.profile.updated` (INFO); `auth.signup.accepted`, `auth.signup.duplicate`, `auth.email.confirmed` (with `path` `signed_in` or `new_password`), `auth.email.limited` (a request over a per-account sending limit, with `purpose`), `auth.password.reset_requested`, `auth.password.reset`, `auth.password.changed` and `auth.password.check_refused` (a wrong current password on change or deletion) (INFO), `account.deleted` (INFO, `purge` `immediate` or `scheduled`), `account.blocked`, `account.unblocked`, `account.restored` and `account.quota_changed` (INFO, from `ovc-admin`), `admin.command` (INFO: `operator_uid`, `operator`, `subcommand`, `outcome` one of `ok`, `refused`, `usage`, `failed`, the target `user_id` when known and an error `kind` on failure, never the email), `account.purged` (INFO), `account.purge_failed` (ERROR, with an error kind), `auth.request.throttled` (WARNING, `scope` `address`) and `auth.password.throttled` (WARNING); on unexpected failures `auth.register.failed`, `auth.verify_email.failed`, `auth.resend.failed`, `auth.password.forgot_failed`, `auth.password.reset_failed`, `auth.password.change_failed` and `users.me.delete_failed` (ERROR, with an error kind); on unexpected failures `auth.authenticate.failed`, `auth.login.error`, `auth.refresh.failed`, `auth.logout.failed`, `auth.logout_all.failed`, `users.me.failed` and `http.client_address.unknown` (ERROR, with an error kind); `auth.login.abandoned` (DEBUG) when the request is cancelled or times out during sign-in, and `http.response.write_failed` (WARNING) when an error body cannot be written. They carry the user identifier in its UUID form, never an email, password, token or cookie |
| Files | `upload.accepted` (INFO, with context, media type and size), `upload.rejected` (INFO, `reason` one of `busy`, `pending`, `hourly`, `too_large`, `unsupported`, `decode_busy`, or `session_revoked` for an upload discarded at settling because its session was revoked while the object was stored), `upload.aborted` (INFO, with an error kind, for a body that failed to arrive or a failure while processing it), `file.reference.added` and `file.reference.removed` (INFO, with the reference kind), `file.deleted` (INFO); on unexpected failures `uploads.create.failed`, `avatars.get.failed` (ERROR, with an error kind, or kind `size_mismatch` when a stored avatar does not match its recorded size) and `users.me.update_failed` (ERROR, with an error kind), and `avatars.object_missing` (WARNING) when a servable avatar's object is not in storage, which answers `404` like any other missing avatar; `avatars.get.abandoned` (DEBUG, with an error kind) when writing to the client fails or the client goes away while an avatar streams, `avatars.get.timed_out` (WARNING) when the 30-second read deadline passes first, which a slow client or a slow storage read can cause, and `avatars.get.close_failed` (WARNING, with an error kind) when the storage object cannot be closed after streaming. They carry the user and attachment identifiers in UUID form, never a file name, storage key or file content |
| Operator command | every command with operator system user, target user identifier and outcome |
| Relationships and groups | Owned by the tutoring service, with `user_id`, `role`, `tutor_id` and `relationship_id` or `group_id`, never emails, names, notes, tokens or codes; access records name the route template `/api/v1/invitations/{token}`, never the token, also for malformed tokens. Event names (INFO unless noted): `card.created`, `card.updated` (the changed field names only, comma-separated in `fields`), `card.withdrawn`, `invitation.regenerated`, `invitation.unreadable` (WARNING, with an error kind, when a card's stored link cannot be opened and is shown without its address), `invitation.send_queued`, `invitation.send_dropped` (`reason` `recipient_cap` or `recipient_unavailable`; never shown to the tutor), `invitation.send_limited`, `invitation.accepted` (`path` `new_account` or `signed_in`, `effect` `provisional`, `linked`, `reactivated` or `merged`), `invitation.refused` (`reason` `invalid`, `email_mismatch`, `role_mismatch` or `not_verified`), `invitation.revoked_after_mismatches` (WARNING), `relationship.confirmed`, `relationship.reclaim_pending`, `relationship.reclaim_decided` (`kept` and `dropped` counts, `cause` `owner` or `timeout`), `relationship.removed` (`cause` `tutor`, `student`, `cleanup`, `reclaim` or `tutor_deleted`; `cleanup` covers every purge of an unconfirmed account, by the 7-day job or at once on its own deletion; `student` also covers the deletion of a confirmed account whose link waits for the owner's choice; `reclaim` covers a link the owner dropped, kept while its tutor was not `active`, or left to `reclaim.expire`; `tutor_deleted` is a reclaim-pending link discarded without a new card by its tutor's deletion), `relationship.ended` (`by` `tutor`, `student` or `deletion`), `relationship.reactivated`, `tutor_code.regenerated`, `join_request.throttled` (WARNING, a student over 10 codes per hour), `join_request.created` (`visibility` `visible`, `held` or `suppressed`, with `join_request_id`), `join_request.released` (`visibility` `visible` or `suppressed`), `join_request.reopened` (with `join_request_id` and `visibility` `visible` or `suppressed`, when a code entry reopens a request an unblock closed), `join_request.unsuppressed` (with `join_request_id`), `join_request.decided` (`decision` `accepted`, `declined` or `blocked`, with `join_request_id`; an accept also carries `relationship_id` and `effect` `linked`, `reactivated` or `created`, and a card it creates is also recorded as `card.created`), `blocked_requester.removed` (with `closed`, the count of the student's requests the unblock closed), `join_requests.expire_failed` (ERROR, with `join_request_id` and an error kind) names a request the expiry job skipped, a code entry that keeps a request waiting for the owner's choice is recorded as `relationship.reclaim_decided` with `kept` 1, `group.created` (with `members`, the count of initial members), `group.updated` (the changed field names only, `name` or `colour`, comma-separated in `fields`), `group.membership_changed` (`added` and `removed` counts), `group.archived` and `group.unarchived` (each with `members`, the current member count, and only when the state changed), all with `group_id`; the group routes report unexpected failures as `groups.list.failed`, `groups.create.failed`, `groups.get.failed`, `groups.update.failed`, `groups.archive.failed` and `groups.unarchive.failed` (ERROR, with an error kind); group names are never logged; the jobs `invitations.expire`, `join_requests.expire` and `reclaim.expire` report through `job.run`, and `reclaim.expire_failed` (ERROR, with `user_id` and an error kind) names an account the expiry job skipped; the reclaim routes report unexpected failures as `users.me.reclaim.failed` and `users.me.reclaim.decide_failed` (ERROR, with an error kind); the join-request routes report unexpected failures as `join_requests.create.failed`, `join_requests.list.failed`, `join_requests.sent.failed`, `join_requests.accept.failed`, `join_requests.decline.failed`, `tutor.blocked_requesters.list.failed` and `tutor.blocked_requesters.delete.failed` (ERROR, with an error kind); unexpected failures `<route>.failed` (ERROR, with an error kind) |
| REST routers | one access record per request: method, route template, status, duration, request identifier |
| WebSocket gateway | connection opened, authenticated, closed with code and reason, frame rejected |
| Schedule | lesson or series created, edited with scope, split, cancelled; overlap warning returned; status changed, including automatic end by the status job; series horizon extended and skipped for inactive audiences |
| Chat | message persisted, fan-out completed with recipient count and duration |
| Meetings and recordings | meeting created and re-created, join authorized and refused with reason code, meeting ended (hint received, confirmed), callback accepted and rejected with status, reconciliation batch, recording copied, held for quota, retried, failed, expired and deleted, provider copy deleted, provider call failed |
| Homework | template saved, issue created, deadline changed and withdrawn, submission submitted (with late flag), autosave conflict, review revision published as checked, returned or updated, score override applied |
| Files | upload accepted, rejected and aborted with reason code, quota exceeded, reference added and removed, file deleted |
| Notifications and outbox | notification created, outbox row claimed, done (sent or obsolete), retried, lease reclaimed and dead with error code, digest sent, unsubscribe applied, email marked undeliverable, scheduled job run with batch size and duration. Outbox and job event names: `outbox.row.claimed` (DEBUG), `outbox.row.done` (INFO), `outbox.row.retried` (WARNING), `outbox.lease.reclaimed` (WARNING), `outbox.lease.lost` (WARNING, a late finisher whose lease was reclaimed), `outbox.row.dead` (ERROR), `job.run` (INFO, ERROR when the job failed), `smtp.connection.discard_failed` (WARNING, with an error kind, when an SMTP connection that is idle, or was cut off by its deadline after the server accepted the message, cannot be closed cleanly; a connection already closed or past its deadline is not reported); their fields are the row identifier, lane, kind, attempts and error code, never a payload, recipient or message content |

Tests assert on event names and fields for the outcomes above, and assert that forbidden content
never appears in captured records.
