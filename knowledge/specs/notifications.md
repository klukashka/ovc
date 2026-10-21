# Notifications specification

Behavior for FR-13 and the worker that delivers it. Decision: [ADR-0006](../../docs/decisions/0006-background-worker-and-email.md).

## Events

| Event | Recipients | In-app | Email | Email preference group |
|---|---|---|---|---|
| `invitation.created` | invitee email (no account needed) | — | yes | required |
| `email.confirmation` | the account | — | yes | required |
| `password.reset` | the account | — | yes | required |
| `signup.duplicate_attempt` | the existing account | — | yes | required |
| `relationship.ended` | student | yes | yes | required |
| `tutor.deleted` | each student of the tutor | — | yes | required |
| `group.archived` | group members | yes | yes | `lesson_changes` |
| `join_request.created` | tutor | yes | no | — |
| `join_request.declined` | student | yes | no | — |
| `invitation.accepted`, `join_request.accepted` | tutor, student | yes | no | — |
| `lesson.reminder_24h` | audience and tutor | no | yes | `lesson_reminders` |
| `lesson.reminder_1h` | audience and tutor | yes | yes | `lesson_reminders` |
| `lesson.moved`, `lesson.cancelled` | audience | yes | yes | `lesson_changes` |
| `lesson.series_changed` | audience, one per recipient listing all affected lessons | yes | yes | `lesson_changes` |
| `issue.created` | each student of the issue | yes | yes | `homework` |
| `issue.withdrawn` | each student of the issue | yes | no | — |
| `submission.submitted` | tutor, for work with manual questions | yes | yes | `submissions` |
| `submission.checked`, `submission.returned`, `submission.review_updated` (only when comments, corrected files or feedback changed) | student | yes | yes | `results` |
| `recording.available` | tutor and roster | yes | no | — |
| `recording.held_quota` | tutor | yes | yes | required |
| `recording.failed` | tutor | yes | no | — |
| `messages.digest` | a member with unread messages | — | yes | `message_digest` |

- A lesson is "moved" when its start or duration changes; other edits notify no one.
- **Audience of homework events:** the recipient of `issue.*` and `submission.*` events is the student who owns the submission, even after leaving the group. Fully automatic work becomes checked on submission and sends `submission.checked` with review revision 0.
- **Aggregated emails:**
  - when a relationship ends, the cancelled lessons are listed in the `relationship.ended` email, and no separate `lesson.cancelled` emails go out;
  - when a group is archived, in the `group.archived` email, likewise;
  - when a tutor deletes their account, in the `tutor.deleted` email, and no other email goes out for that deletion.
- Each event names its tutor, so the student's interface and emails carry the tutor label and colour.

## Channels

- **In-app:**
  - a notification row per recipient, with event, tutor, a short structured payload (identifiers and titles, never message bodies or answers), creation time and read time;
  - rows are written by the producer (a service or a scheduler job) in its own transaction, with the recipients decided at that moment; email preferences never suppress them;
  - each row has the same idempotency key as its email ([keys](#outbox-and-worker)) and is inserted with `ON CONFLICT DO NOTHING`;
  - `notification.new` is published through the [broadcaster](realtime-and-video.md#broadcaster) only when a row was actually inserted, after commit, so rows created by the worker reach connected users too and repeated job runs never duplicate them;
  - `GET /notifications` returns newest first, 30 per page, with `unread_count`;
  - rows older than 90 days are deleted.
- **Email:** rendered and sent by the worker through SMTP ([email](#email)).

## Email

- **Templates** exist per event, in Russian and English, as HTML and plain text. They are rendered at send time in the recipient's language and timezone with ICU plural rules.
- **Content:** subject and body include only what the recipient may see; emails to students never contain points or scores. A message digest names channels and counts, never message text. Links point to OVC pages, never to provider or storage URLs.
- **Escaping:** user-written text (names, titles) is HTML-escaped in HTML bodies and appears in plain text only as text.
- **Headers:**
  - user-written text that reaches a header (names, titles) has CR and LF removed and is encoded with RFC 2047;
  - the `Message-ID` is derived from the outbox idempotency key, so a retried send keeps its identifier;
  - for token-bearing emails it also includes the token identifier, so a re-minted token is a distinct message.
- **Emails with tokens:**
  - invitation emails use the email-delivery invitation token, which is stored encrypted for this purpose ([invitations](accounts.md#invitations));
  - confirmation and reset emails have their token minted by the worker in a committed transaction just before sending, invalidating earlier tokens of the same purpose, so a retried send mints and commits a new token first;
  - no outbox payload ever contains a token.
- **Optional emails** carry one-click unsubscribe headers:
  - `List-Unsubscribe` points to `https://<OVC origin>/api/v1/unsubscribe?token=…`;
  - `List-Unsubscribe-Post: List-Unsubscribe=One-Click`;
  - both headers are covered by the DKIM signature.
- **Sender:** a fixed address on the OVC domain with SPF, DKIM and DMARC. The SMTP connection uses a 30-second timeout.
- **Bounces and complaints** reported by the provider mark the address undeliverable. That stops optional emails until the user saves their email preferences again. How bounces arrive depends on the provider chosen ([open questions](../architecture/detailed.md#open-implementation-questions)). The chosen provider must keep message logs for at most 30 days.
- **Tests** use the database mail sink, which the [test harness](../../docs/development.md#end-to-end-test-harness) can read.

## Dispatch-time checks

Each **email** is decided again when the worker sends it, not only when it is queued (in-app rows are not re-checked). The worker loads the recipients, preferences and subjects of a claimed batch in a few batched queries. It drops a row, marking it `done` with reason `obsolete`, when:

- the recipient is `blocked` or `deleted`, or the address is undeliverable for an optional email;
- the recipient's preference group is off; a missing preference row means on;
- the recipient is no longer in the audience the event was about (for lesson, issue and group events);
- the subject is obsolete:

  | Event | Obsolete when |
  |---|---|
  | Lesson reminder | The lesson is not `scheduled`, its schedule revision differs from the one in the key, or the send moment is more than 15 minutes past the reminder moment |
  | `lesson.moved` | The lesson's schedule revision has advanced again; the newer change sends its own email |
  | Confirmation | The email is already confirmed, or a newer confirmation request exists |
  | Password reset | A newer reset request exists |
  | `issue.created` | The issue is withdrawn |
  | `recording.held_quota` | The recording is no longer `held_quota` |
  | Message digest | The member read the messages it would list |
  | Invitation | The token is used, revoked or expired |

`lesson.cancelled` and `lesson.series_changed` are always sent while the recipient remains eligible.

## Reminders

- **Schedule:** a scheduler job runs every minute.
- **Window:** for each reminder kind with offset `o` (24 hours or 1 hour), it selects `scheduled` lessons whose start lies in `(now + o − 15 minutes, now + o]` and whose remembered reminder revision for that kind differs from their schedule revision. It uses a partial index on lesson start over scheduled lessons.
- **Rows:** it writes one in-app row (for the 1-hour reminder) and one outbox row per recipient, with idempotency key `(event, lesson, recipient, schedule revision)` and `ON CONFLICT DO NOTHING`. In the same transaction it records the lesson's reminder revision for that kind.
- **Progress:** a run keeps taking batches until no lesson in the window is left or its 20-second time budget is used. The revision marker means a large burst of lessons starting together is worked through completely, never reselected.
- **Lateness:** a reminder whose window has passed is never selected, which bounds lateness to 15 minutes without separate skip records.
- **Moved lessons** get a new revision and therefore new keys; reminders queued for the old revision become obsolete at dispatch.

## Message digest

- **Flag:** when a message is committed, each member whose `online_until` has passed gets `digest_pending_since` set, if it is not already set. Reading the member's channels up to their latest messages clears the flag, checked with one aggregate query over the member's channels.
- **Sending:** a scheduler job runs every minute. It selects members, through a partial index, who:
  - have `digest_pending_since` at least 10 minutes old;
  - have `online_until` at least 10 minutes in the past;
  - got their last digest at least 30 minutes ago.
- **Building the digest:**
  - the window runs from the later of the member's `last_seen_at` and the previous window's end, to the job's `now`;
  - it covers unread messages created within the window;
  - if there are none, the job clears the flag and sends nothing;
  - otherwise, in one transaction, the job writes the outbox row with key `(digest, member, window start)` and both window bounds in its payload, records the window end as the last digest time, and clears the flag;
  - the email renders exactly the messages in its window, and the next window starts at its end, so the same messages never appear in two digests.
- **Content:** channel names, tutor labels and counts.

## Preferences and unsubscribe

- **Preference groups:** each user has one switch per optional group: `lesson_reminders`, `lesson_changes`, `homework`, `results`, `submissions` (tutors) and `message_digest`. All are on by default.
- **Unsubscribe token:**
  - stateless, signed with a dedicated key (`OVC_UNSUBSCRIBE_SIGNING_SECRET`);
  - carries the purpose `unsubscribe`, the account, the group and an expiry 90 days after the email was sent;
  - it never signs the user in.
- **Two entry points:**
  - **Mail clients** performing RFC 8058 one-click unsubscribe send `POST /api/v1/unsubscribe?token=…` with the body `List-Unsubscribe=One-Click`.
  - **People** opening the link in the email body land on the web page `/unsubscribe?token=…`. It calls `GET /api/v1/unsubscribe/preview?token=…`, which returns only the group name. The page shows a confirmation button that sends the same `POST`.
- **Behavior:** the `POST` is idempotent and repeatable. Nothing changes on any `GET`, so link scanners cannot unsubscribe anyone.
- **Required emails** ignore preferences.

## Outbox and worker

- **Outbox rows** are written in the same transaction as the change that causes them. Each row has:
  - lane (`email`, `provider_call`, `recording_copy`, `storage_delete`) and priority (`high` for reminders, confirmation, reset and invitation emails; `normal` otherwise);
  - kind: the event name for emails, or the effect name;
  - payload of identifiers only;
  - idempotency key, unique, inserted with `ON CONFLICT DO NOTHING`;
  - status (`pending`, `in_progress`, `done`, `dead`);
  - attempts, next attempt time, lease token and lease expiry, last error code.
- **Idempotency keys:**

  | Event or effect | Key |
  |---|---|
  | Reminders, `lesson.moved` | `(event, lesson, recipient, schedule revision)` |
  | `lesson.cancelled` | `(event, lesson, recipient)` |
  | `lesson.series_changed` | `(event, operation identifier, recipient)`; the identifier is generated per request and kept in the payload |
  | `group.archived` | `(event, group, archived_at, recipient)` |
  | `relationship.ended` | `(event, relationship, ended_at)` |
  | `submission.submitted` | `(event, submission, submitted_at)` |
  | `submission.checked`, `submission.returned`, `submission.review_updated` | `(event, submission, review revision)` |
  | `signup.duplicate_attempt` | `(event, account, UTC date)` |
  | Confirmation and reset | `(event, email token request)`, a row recorded when the request is accepted |
  | Invitations | `(event, invitation)`; every send creates a new email-delivery invitation, so a resend is a new key |
  | Digests | `(digest, member, window start)` |
  | Recording copies | `(recording, attempt generation)` |
  | Everything else | `(event, subject, recipient)` |

- **Two loops**, each in its own task (the target design; slice 2 runs them as one sequential pass, described under "One pass" below):
  - **Dispatcher:**
    - **claiming:** for each lane, it claims up to that lane's free slots of due rows, high priority first, in a short transaction (`FOR UPDATE SKIP LOCKED`). It increments `attempts`, sets `in_progress` with a fresh lease token and a lease of 5 minutes (60 minutes for `recording_copy`, renewed while it streams), and commits. It polls every second; from slice 4 it is also woken by a `NOTIFY` when rows are inserted, over the listener connection that slice adds;
    - **SMTP:** connections are reused across sends, and the provider's sustained send rate is recorded when the provider is chosen, because NFR-4.5 depends on it;
    - **performing:** each effect runs outside any transaction, with a timeout shorter than its lease (30 seconds for email and provider calls);
    - **completing:** marking a row `done`, or scheduling its retry, is a conditional update on its lease token, so a reclaimed row's late finisher changes nothing;
    - **retries:** use exponential backoff from 30 seconds up to 1 hour, with jitter from an injected random source;
    - **dead rows:** after 8 attempts, or when a lease expires on a row that already has 8 attempts, the row becomes `dead` and an `ERROR` event is emitted. A failure that can never succeed (an outbox kind with no email defined, or a message the mail adapter refuses as malformed) makes the row `dead` at once with the error code `permanent`; an email builder treats a payload it cannot parse the same way. Where an entity owns the effect it records the failure (a recording becomes `failed`); dead emails have no entity effect;
    - **lanes:** each has its own concurrency limit (email 10, provider calls 4, recording copies 1, storage deletes 4), so long copies never delay emails;
    - **leases:** rows with an expired lease are reclaimed.
  - **One pass:** `Tick(now)` runs the due scheduler jobs, reclaims expired leases, then claims and performs due rows lane by lane until none remains at `now`, so a second tick at the same instant has no further effect. In slice 2 a lane claims a batch of up to its concurrency and waits for the whole batch before claiming again, and lanes drain one after another; per-lane loops that refill slots as they free up replace this before a long-running lane (`recording_copy`, slice 7) lands, so long copies never delay emails. A pass stops claiming after 4 minutes of wall time, below the 5-minute lease, so a row claimed late in a long pass never starts with an expired lease. A panic in an effect counts as a failed attempt. With `OVC_CLOCK=system` the worker process calls it every second; with `OVC_CLOCK=test` only `POST /__test__/tick` calls it, at the shared clock's instant, and the clock and tick routes exist only in that mode. On shutdown the worker claims nothing new and lets effects already started finish within their timeout.
  - **Message identifiers:** an email's `Message-ID` is derived from its outbox idempotency key and the host of `OVC_PUBLIC_URL`, so a retried send keeps the same identifier.
  - **Scheduler:** runs jobs whose due time has passed, recording each run in a job-run table, so hourly and daily jobs catch up once after downtime.
    - **Every minute:** reminders, digests, the lesson status job.
    - **Every 30 minutes:** recording reconciliation.
    - **Hourly:**
      - recording, upload, invitation and join-request expiry;
      - the 7-day transition of `held_quota` recordings to `failed`;
      - recording provider-copy cleanup;
      - cleanup of unverified accounts, rate-limit windows, notifications and the outbox (`done` rows after 7 days, `dead` rows after 30 days);
      - account purge.
    - **Daily:** series horizon extension and the automatic retry of held recordings.
- **Bounds:** every job processes fixed-size batches through a partial index on its due column, within a time budget per run, and continues where it stopped: jobs that select by a window keep progress markers (reminders) or cursors in the job-run table (reconciliation). Purges delete in batches of bounded size per transaction.
- **Testing:**
  - with `OVC_CLOCK=test` both loops are paused, and only the harness runs the worker;
  - `Tick(now)` runs one scheduler pass at `now` and then dispatches every due row until none remain;
  - running it twice with the same `now` has no extra effect.
