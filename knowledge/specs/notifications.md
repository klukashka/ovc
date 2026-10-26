# Notifications specification

Behavior for FR-13 and the worker that delivers it. Decisions: [ADR-0006](../../docs/decisions/0006-background-worker-and-email.md), and [ADR-0014](../../docs/decisions/0014-students-join-through-their-tutor.md) for the relationship emails.

## Events

| Event | Recipients | In-app | Email | Email preference group |
|---|---|---|---|---|
| `invitation.created` | the card's email (no account needed) | — | yes | required |
| `email.confirmation` | the account | — | yes | required |
| `password.reset` | the account | — | yes | required |
| `signup.duplicate_attempt` | the existing account | — | yes | required |
| `relationship.ended` | the other party of the ending: the student when the tutor ends it, the tutor when the student ends it; nobody when a deletion ends it | yes | yes | required |
| `tutor.deleted` | each student whose relationship with the tutor was `active` at deletion and whose email is confirmed | — | yes | required |
| `group.archived` | group members | yes | yes | `lesson_changes` |
| `invitation.accepted` | the tutor whose invitation linked its card | yes | no | — |
| `join_request.created` | the tutor, once the request is visible to them | yes | no | — |
| `join_request.accepted` | the student who sent the request | yes | no | — |
| `join_request.declined` | the student who sent the request | yes | no | — |
| `lesson.reminder_24h` | audience and tutor | no | yes | `lesson_reminders` |
| `lesson.reminder_1h` | audience and tutor | yes | yes | `lesson_reminders` |
| `lesson.moved`, `lesson.cancelled` | audience | yes | yes | `lesson_changes` |
| `lesson.series_changed` | audience, one per recipient listing all affected lessons | yes | yes | `lesson_changes` |
| `issue.created` | each student of the issue | yes | yes | `homework` |
| `issue.withdrawn` | each student of the issue | yes | no | — |
| `submission.submitted` | tutor, for work with manual questions | yes | yes | `submissions` |
| `submission.checked`, `submission.returned`, `submission.review_updated` (only when comments, corrected files or feedback changed) | student | yes | yes | `results` |
| `recording.available` (after the MVP; the recording jobs below also wait for recordings) | tutor and roster | yes | no | — |
| `recording.held_quota` (after the MVP) | tutor | yes | yes | required |
| `recording.failed` (after the MVP) | tutor | yes | no | — |
| `messages.digest` | a member with unread messages | — | yes | `message_digest` |

- A lesson is "moved" when its start or duration changes; other edits notify no one.
- **Audience of homework events:** the recipient of `issue.*` and `submission.*` events is the student who owns the submission, even after leaving the group. Fully automatic work becomes checked on submission and sends `submission.checked` with review revision 0.
- **Aggregated emails:**
  - when a relationship ends, the cancelled lessons are listed in the `relationship.ended` email, and no separate `lesson.cancelled` emails go out;
  - when a group is archived, in the `group.archived` email, likewise;
  - when a tutor deletes their account, in the `tutor.deleted` email, and no other email goes out for that deletion.
- Each event names its tutor, so the student's interface and emails carry the tutor label and colour.
- **Slice 3** sends the `invitation.created`, `relationship.ended` and `tutor.deleted` emails; `relationship.ended` and `tutor.deleted` carry no lesson list until slice 5, and `group.archived` waits for slice 5. **Slice 4** adds the notification table and the in-app rows of the slice 3 events ([in-app producers](#in-app-producers-in-slice-4)); the first optional email, and with it the unsubscribe headers and the dispatch-time preference check, arrives with slice 5.
- **Unconfirmed recipients:** no notification email reaches an address whose account is `unverified`: at dispatch every email other than the account emails (confirmation, reset, duplicate sign-up notice) and invitation emails is dropped for such a recipient, including `relationship.ended` and `tutor.deleted`, and removing a provisional link sends nothing.

## Channels

- **In-app:**
  - a notification row per recipient stores identifiers only, never names, titles, message bodies or answers: the recipient, the tutor, the student the event concerns, the event, its subject (the relationship or the join request) and the relationship for the tutor colour, with creation time and read time ([data model](data-model.md#notification)); labels are resolved when the rows are read ([labels and links](#labels-and-links)), so no row keeps a copy of a name that a purge or a later edit should remove;
  - rows are written by the producer (a service or a scheduler job) in its own transaction, with the recipients decided at that moment from the rows it holds locked; email preferences never suppress them;
  - each row has an idempotency key, the same as its email's where the event has one ([keys](#outbox-and-worker)), and is inserted with `ON CONFLICT DO NOTHING`;
  - `notification.new` is signalled through the [broadcaster](realtime-and-video.md#broadcaster) in the same transaction, only when a row was actually inserted, and is delivered once that transaction commits, so rows created by the worker reach connected users too and repeated job runs never duplicate them;
  - **purge:** once the purge of an account commits (the 30-day purge, the 7-day cleanup of an unconfirmed account, or the immediate purge of an unconfirmed account its owner deletes), no notification row names that account as recipient, tutor or student. The rows have no foreign keys; the purge deletes them through indexes inside its batches, and every producer of a row that names an account holds a lock that the purge of that account also takes (the account's user row, its tutor profile, or the relationship or join request row the purge discards or deletes) and checks the state again under it, so a producer waiting behind a purge finds its subject gone and inserts nothing; the final purge transaction deletes the rows naming the account after the user row, whose deletion cascades to the account's join requests, so a producer holding one of those rows (an un-suppression) has either committed before that statement reads the table or finds no row to change;
  - **retention:** the hourly `notifications.cleanup` job deletes rows older than 90 days through the `created_at` index, in batches of 1,000, at most 10 batches per run, oldest first, and the next run resumes where it stopped.
- **Email:** rendered and sent by the worker through SMTP ([email](#email)).
- **Web push** (from slice 10, off until its gate opens): decided in [ADR-0015](../../docs/decisions/0015-web-push.md), including the send-time recipient check, and specified with the slice 10 plan.

### In-app producers in slice 4

Each row below is written in the producer's transaction with `ON CONFLICT DO NOTHING`, and `notification.new` is signalled only when the row was inserted:

| Event | Recipient | Produced when | Key |
|---|---|---|---|
| `invitation.accepted` | the tutor | an invitation links its card: a new account (also while the link is provisional, when the card changes to "Email not confirmed"), a signed-in acceptance, a merge, a reactivation; no second row when the email is later confirmed | `(invitation.accepted, invitation, tutor)` |
| `join_request.created` | the tutor | a request becomes visible to the tutor: created visible, released at confirmation or by the owner's keep after a reclaim, un-suppressed, or reopened in place after an unblock | `(join_request.created, request, tutor)` |
| `join_request.accepted` | the student | the tutor accepts | `(join_request.accepted, request, student)` |
| `join_request.declined` | the student | the tutor declines, with or without a block, as one identical row | `(join_request.declined, request, student)` |
| `relationship.ended` | the other party | either party ends the relationship; nobody when a deletion ends it | `(relationship.ended, relationship, ended_at)`, the key of its email |

- A held or suppressed request produces nothing for the tutor, and a block, a suppression or an unblock produces nothing for the student ([T-33](../../docs/threat-model.md#threats-and-controls)).
- Tutor deletion and student deletion insert no notification rows.

### Labels and links

`GET /notifications` resolves each row's label and link at read time, under the rules every other route applies to the caller:

| Event | Recipient | Label | Colour | Link |
|---|---|---|---|---|
| `invitation.accepted` | tutor | the card's own display name, never the account name a possibly squatting account chose ([T-44](../../docs/threat-model.md#threats-and-controls)) | `null` | `student_card` with the relationship, while the card is in the tutor's scope |
| `join_request.created` | tutor | for an accepted request of an account that is not operator-blocked, the display name of the tutor's own card for that student while the card is not discarded, never the live account name, and `null` with `label_deleted: true` while the requesting account is `deleted`, which the interface shows as "Deleted account"; for a visible pending request, the request's account name; in every other case (declined, expired, closed by an unblock, a deleted requester whose request was not accepted, or an operator-blocked account) `null` with `label_deleted: false`, so the name never outlives the tutor's scope | `null` | `join_requests` with the request, while the request is visible and pending |
| `relationship.ended` | tutor | the card's own display name | `null` | `student_card` with the relationship, while the card is in the tutor's scope |
| `join_request.accepted` | student | the tutor's account name, while My tutors lists the relationship | the relationship's colour, likewise | `my_tutors` with the relationship, while My tutors lists it |
| `join_request.declined` | student | `null`, because the student's sent requests carry no tutor name ([accounts](accounts.md#tutor-code-and-join-requests)) and no relationship formed (the exception in FR-2.6) | `null` | `my_tutors` with the join request's identifier, which opens the sent requests; every other `my_tutors` link carries a relationship identifier |
| `relationship.ended` | student | the tutor's account name, while My tutors lists the relationship | the relationship's colour, likewise | `my_tutors` with the relationship, while My tutors lists it |

- A link whose subject is outside the caller's current scope is `null`, and the row is shown without a link. On `invitation.accepted` and `relationship.ended` rows, a tutor's card label is the card's own display name while the card row exists, also after it is discarded; its link is `null` once the card is discarded.
- `label_deleted` is `true` only for a `join_request.created` row of an accepted request whose requesting account is `deleted`; the label of that row is `null`, and a deleted requester of a request that was not accepted is shown like any request outside the tutor's scope. The interface renders its own "Deleted account" text ([states](../design/web/states.md#notifications)).
- Labels are user-written names: the interface renders them as text, never as HTML or links ([T-14](../../docs/threat-model.md#threats-and-controls)), and the row's text never interpolates them ([states](../design/web/states.md#notifications)).

### Notification routes

- **`GET /notifications?limit=&cursor=`** answers `{items: [{id, event, created_at, read_at, label, label_deleted, colour, link}], next_cursor, unread_count}` with one schema for both roles: `label` is a string or `null`, `label_deleted` a boolean ([labels and links](#labels-and-links)), `colour` a palette key or `null` (students only), and `link` is `{kind: student_card | join_requests | my_tutors, id}` or `null`. Newest first, keyset paginated on `(created_at, id)`, 30 per page by default and at most 100 (the panel asks for 10), `Cache-Control: no-store`. `unread_count` counts at most 100 unread rows, and the interface shows "99+" above 99.
- **`POST /notifications/read`** takes exactly one of `{ids}` (1 to 100 of the caller's rows) or `{through}` (one of the caller's rows: that row and every row of the caller older than it by `(created_at, id)`), and answers `{unread_count}`. It is idempotent. An identifier that is not one of the caller's rows answers `404 not_found` like a random one, before `422 validation_failed` for both fields or neither. It is a write open to unconfirmed accounts, so it locks the account's row and checks the session again ([locks and the session re-check](accounts.md#locks-and-the-session-re-check)), and, when it marked at least one row, it signals `entity.changed` with kind `notification` to the caller's own sockets. An empty `ids` list answers `422` (`invalid`), and one beside `through` counts as both fields. Accepted gap: a row whose `created_at` precedes the `through` row but whose producer commits after the list was shown is marked read without having been shown; the window is one producer transaction, and the row's `notification.new` still arrives, so the client refetches and lists it as read. Closing the gap would need a commit-ordered sequence, which PostgreSQL does not offer without serialising producers.

## Email

- **Templates** exist per event, in Russian and English, as HTML and plain text. They are rendered at send time in the recipient's language and timezone with ICU plural rules. The account emails (confirmation, reset, duplicate sign-up) share one HTML and one text template in `internal/services/notifications/templates`, rendered with `html/template` and `text/template`, and take their wording from the catalogue in `internal/core`; they contain no user-written text and no counts, so plural rules arrive with the first email that shows a count.
- **Content:** subject and body include only what the recipient may see; emails to students never contain points or scores. A message digest names channels and counts, never message text. Links point to OVC pages, never to provider or storage URLs.
- **Escaping:** user-written text (names, titles) is HTML-escaped in HTML bodies and appears in plain text only as text.
- **Relationship emails:** the invitation email is written in the language of the account that holds the card's email, otherwise in the tutor's; the tutor's name appears only in its body, as escaped plain text, never as a link or in the subject, and the only link is the OVC invitation link. The `relationship.ended` email to a tutor names the student by the tutor's own card display name, escaped, never by the current account name; for a card created from a join request that is the tutor-owned copy taken at accept time. The `relationship.ended` email is written in the recipient's language; to a student it names the tutor by the tutor's account name and links to My tutors, and to a tutor it links to the student card; its payload holds only the relationship identifier and `ended_at`, from which the worker reads everything else at dispatch. The `tutor.deleted` email is queued in the deletion's transaction for each student whose relationship the deletion ended and whose email is confirmed at that moment, is written in the student's language, names the tutor by the tutor's account name, escaped, and links only to My tutors; the worker reads the tutor and the recipient at dispatch. The three slice 3 emails share one new HTML and text template pair, `relationship.html` and `relationship.txt` in `internal/services/notifications/templates`, with their wording in the catalogue in `internal/core`; the user-written name stands in its own paragraph, with runs of whitespace and line breaks collapsed to single spaces.
- **Headers:**
  - user-written text that reaches a header (names, titles) has CR and LF removed and is encoded with RFC 2047;
  - the `Message-ID` is derived from the outbox idempotency key, so a retried send keeps its identifier;
  - for token-bearing emails it also includes the token identifier, so a re-minted token is a distinct message.
- **Emails with tokens:**
  - invitation emails carry the card's one open invitation link, whose token the worker decrypts from the stored ciphertext at dispatch ([invitations](accounts.md#invitations)); a row whose ciphertext the current key cannot open becomes `dead` with `permanent`; the payload holds the invitation identifier and the send count, and the `Message-ID` also includes the invitation identifier;
  - confirmation and reset emails have their token minted by the worker in a committed transaction just before sending, invalidating earlier tokens of the same purpose, so a retried send mints and commits a new token first;
  - no outbox payload ever contains a token.
- **Optional emails** carry one-click unsubscribe headers, from the first optional email in slice 5; the token is minted at render time and never stored in an outbox payload:
  - `List-Unsubscribe` points to `https://<OVC origin>/api/v1/unsubscribe?token=…`;
  - `List-Unsubscribe-Post: List-Unsubscribe=One-Click`;
  - both headers are covered by the DKIM signature.
- **Sender:** a fixed address on the OVC domain with SPF, DKIM and DMARC. The SMTP connection uses a 30-second timeout.
- **Bounces and complaints** reported by the provider mark the address undeliverable. That stops optional emails until the user saves their email preferences again. How bounces arrive depends on the provider chosen ([open questions](../architecture/detailed.md#open-implementation-questions)). The chosen provider must keep message logs for at most 30 days.
- **Tests** use the database mail sink, which the [test harness](../../docs/development.md#end-to-end-test-harness) can read.

## Dispatch-time checks

Each **email** is decided again when the worker sends it, not only when it is queued (in-app rows are not re-checked). The worker loads the recipients, preferences and subjects of a claimed batch in a few batched queries. It drops a row, marking it `done` with reason `obsolete`, when:

- the recipient is `blocked` or `deleted`, or the address is undeliverable for an optional email;
- the recipient is `unverified` and the email is neither an account email (confirmation, reset, duplicate sign-up notice) nor an invitation email; each email builder reports the state of the recipient's account (for an invitation, the account that holds the card's email, if any), and the dispatcher applies the account-state parts of this rule and the previous one (blocked, deleted and unverified recipients) to every kind; it does not yet check undeliverable addresses for optional emails, which arrive with the first optional email;
- the recipient's preference group is off; a missing preference row means on (from the first optional email in slice 5, which brings this check);
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
  | Invitation | The invitation is used, revoked or expired, its send count has moved on, or the tutor is no longer `active` |
  | `relationship.ended` | The relationship is no longer `ended` at the `ended_at` the email was queued for: it was reactivated, or reactivated and ended again, which queues its own email |
  | `tutor.deleted` | The tutor is no longer `deleted` |
  | Any event | The subject row no longer exists (it was purged) |

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

Slice 4 stores preferences, mints and verifies tokens, and serves the routes and pages; the worker mints tokens from slice 5, when the first optional email carries them.

- **Preference groups** per role, listed in this fixed order and all on by default:
  - tutors: `lesson_reminders`, `submissions`, `message_digest`;
  - students: `lesson_reminders`, `lesson_changes`, `homework`, `results`, `message_digest`.
- **Routes:** `GET /users/me/email-preferences` and `PUT /users/me/email-preferences` use `{groups: [{group, enabled}]}`, listing the caller's role's groups in that order, one schema for both roles. `PUT` takes a partial list and answers the full one; a group of the other role answers `422 validation_failed` with `not_allowed`, and a group named twice with `invalid`. A save writes one row per named group and locks the account's row and checks the session again first ([locks and the session re-check](accounts.md#locks-and-the-session-re-check)). Both answer with `Cache-Control: no-store`. Saving preferences also clears an undeliverable mark once bounce intake exists (slice 10).
- **Unsubscribe token:**
  - `v1.<payload>.<mac>`, each part in unpadded base64url: the payload is the account identifier (16 bytes), the group (1 byte: 1 `lesson_reminders`, 2 `lesson_changes`, 3 `homework`, 4 `results`, 5 `submissions`, 6 `message_digest`; these codes never change while tokens that carry them can be valid) and the expiry (8 bytes, Unix seconds, big-endian); the MAC is an untruncated HMAC-SHA-256 over the payload under a key derived by HKDF-SHA-256 from `OVC_UNSUBSCRIBE_SIGNING_SECRET` with the info string `unsubscribe`, so the purpose is bound to the key;
  - minted at dispatch, through the signer in `internal/core`, with an expiry 90 days after the send; it is never stored, never logged and never placed in an outbox payload;
  - parsed strictly, and its MAC compared in constant time, before any query, so a forged token costs no database work;
  - its payload is readable, not encrypted: it shows the account's opaque identifier to anyone holding the link;
  - it never signs the user in, sets no cookie and returns no token.
- **The key:** `OVC_UNSUBSCRIBE_SIGNING_SECRET` is a dedicated secret of at least 32 bytes, required by the API and the worker from the change that adds it and refused when it equals `OVC_JWT_SECRET` or `OVC_FIELD_ENCRYPTION_KEY`; rotating the JWT secret signs everyone out by design and must not also invalidate 90-day unsubscribe links ([configuration](../architecture/detailed.md#configuration)).
- **Two entry points:**
  - **Mail clients** performing RFC 8058 one-click unsubscribe send `POST /api/v1/unsubscribe?token=…` with the body `List-Unsubscribe=One-Click`.
  - **People** opening the link in the email body land on the web page `/unsubscribe?token=…`, which follows the [token page rule](../design/web/navigation.md#token-pages). It calls `GET /api/v1/unsubscribe/preview?token=…`, which answers only `{group}` and changes nothing. The page shows a confirmation button that sends the same `POST`.
- **`POST /unsubscribe?token=`** accepts no body, or a body of at most 1 KiB that is `application/x-www-form-urlencoded` or `multipart/form-data` (media types parsed with their parameters, such as `charset` or `boundary`) holding exactly the field `List-Unsubscribe` with the value `One-Click`, and answers `204`. Another content type answers `415 unsupported_media_type`; a form or multipart body without that field, with another value or with any other field answers `422 validation_failed` (`body.List-Unsubscribe`). It upserts the one preference row without locking the user row; the foreign key takes `FOR KEY SHARE`, so a concurrent purge either finishes first, and the upsert's foreign-key violation answers `422 token_invalid`, or waits for the upsert.
- **Refusals,** the same on the preview and on the `POST`: every invalid, expired, forged or malformed token, and a token of an account that no longer exists or is `deleted` or `blocked`, answers `422 token_invalid` and changes nothing.
- **Order of checks:** the token's format and MAC (no query); then, for a token with a valid MAC, a limit of 60 requests per hour per account and group counted across both routes (`429 rate_limited` with `Retry-After`), because mail providers send one-click `POST`s from shared addresses, so a per-address limit would refuse unrelated recipients; then the account and the change. An invalid token costs no query and is refused before any limit; a per-address limit on invalid tokens is added only if a measurement asks for it.
- **Behavior:** the `POST` is idempotent and repeatable. Nothing changes on any `GET`, so link scanners cannot unsubscribe anyone. An unsubscribe changes only the preference, never an undeliverable mark. Both routes send `Cache-Control: no-store` and `Referrer-Policy: no-referrer`; access records carry only route templates, and the reverse proxy planned for slice 10 must redact the `token` query parameter of `/unsubscribe` and of every path under `/api/v1/unsubscribe` (the confirmation and its preview) in its logs.
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
  | Invitations | `(invitation.created, invitation, send count)`; each send of the same open link increments the count, so a resend is a new key |
  | `tutor.deleted` | `(event, tutor, deleted_at, recipient)`; the payload holds the tutor, `deleted_at` in microseconds and the recipient |
  | Digests | `(digest, member, window start)` |
  | Recording copies | `(recording, attempt generation)` |
  | Everything else | `(event, subject, recipient)` |

- **The scheduler loop and one loop per lane**, each in its own goroutine:
  - **Dispatcher:**
    - **claiming:** each lane has a slot count equal to its concurrency. Its loop claims up to its free slots of due rows, high priority first, in a short transaction (`FOR UPDATE SKIP LOCKED`), starts each claimed effect at once, and claims again as soon as a slot frees; when nothing is due it polls every second. A claim increments `attempts`, sets `in_progress` with a fresh lease token and a lease of 5 minutes (60 minutes for `recording_copy`, renewed while it streams), and commits. A row is claimed only when a slot is free, so a claimed row starts at once; a slot is released only after the row's completion or retry update, so one worker never claims a row twice. A claim that takes longer than 30 seconds fails like any other claim, in the continuous loops and in `Tick`. In the continuous loops a row's completion or retry is timed at the instant its effect finishes; in `Tick` it uses `now`. Lanes run side by side, so a slow email never stalls its own lane beyond its slot or another lane at all. The lanes registered in slice 4 are `email` (10 slots) and `storage_delete` (4);
    - **no `NOTIFY` wake-up:** a wake-up when rows are inserted is deferred until a measurement asks for it. It would need a second listening connection, in the worker, with its own lifecycle and loss handling, and a `pg_notify` in every transaction that inserts an outbox row would put each such commit on PostgreSQL's notification lock; polling every second keeps a reminder far inside NFR-4.5's two minutes;
    - **SMTP:** connections are reused across sends, and the provider's sustained send rate is recorded when the provider is chosen, because NFR-4.5 depends on it;
    - **performing:** each effect runs outside any transaction, with a timeout shorter than its lease (30 seconds for email and provider calls);
    - **completing:** marking a row `done`, or scheduling its retry, is a conditional update on its lease token, so a reclaimed row's late finisher changes nothing;
    - **retries:** use exponential backoff from 30 seconds up to 1 hour, with jitter from an injected random source;
    - **dead rows:** after 8 attempts, or when a lease expires on a row that already has 8 attempts, the row becomes `dead` and an `ERROR` event is emitted. A failure that can never succeed (an outbox kind with no email defined, or a message the mail adapter refuses as malformed) makes the row `dead` at once with the error code `permanent`; an email builder treats a payload it cannot parse the same way. Where an entity owns the effect it records the failure (a recording becomes `failed`); dead emails have no entity effect;
    - **lanes:** each has its own concurrency limit (email 10, provider calls 4, recording copies 1, storage deletes 4), so long copies never delay emails;
    - **the scheduler loop** runs due jobs and reclaims expired leases every second;
    - **failures:** a failure of the worker's own infrastructure (claiming, completing) ends the run with an error; on shutdown no lane claims, and effects already started finish within their timeout;
    - **leases:** rows with an expired lease are reclaimed.
  - **One pass:** `Tick(now)` runs the due scheduler jobs and reclaims expired leases, then runs every lane's loop side by side until no row is due at `now` and none is in flight, so a second tick at the same instant has no further effect. A pass stops claiming after 4 minutes of wall time, below the 5-minute lease, so a row claimed late in a long pass never starts with an expired lease; this claim budget applies to `Tick` only, because the continuous loops claim only into free slots. A panic in an effect frees its slot and counts as a failed attempt. With `OVC_CLOCK=system` the worker process runs the continuous loops; with `OVC_CLOCK=test` only `POST /__test__/tick` runs `Tick`, at the shared clock's instant, and the clock and tick routes exist only in that mode. On shutdown the worker claims nothing new and lets effects already started finish within their timeout.
  - **Message identifiers:** an email's `Message-ID` is derived from its outbox idempotency key and the host of `OVC_PUBLIC_URL`, so a retried send keeps the same identifier.
  - **Scheduler:** runs jobs whose due time has passed, recording each run in a job-run table, so hourly and daily jobs catch up once after downtime. A job that fails is recorded as `failed` and reported as `job.run` at ERROR with an error kind, and the pass continues with the other jobs and the lanes (an owner decision): only a failure of the worker's own infrastructure, such as recording a job run or claiming rows, ends the pass. A purge that cannot remove one account skips past it within the run, so the accounts behind it are still purged.
    - **Every minute:** reminders, digests, the lesson status job.
    - **Every 30 minutes:** recording reconciliation.
    - **Hourly:**
      - recording and upload expiry; `invitations.expire` (500 per run; clears the ciphertext of expired invitations), `join_requests.expire` (batches of 200, at most 5 per run; a failing request is skipped, and a run that stops at its bound is resumed where it stopped by the next run) and `reclaim.expire` (batches of 100 accounts, at most 1,000 per run; drops reclaim items left unchosen for 7 days);
      - the 7-day transition of `held_quota` recordings to `failed`;
      - recording provider-copy cleanup;
      - cleanup of unverified accounts, rate-limit windows, notifications (`notifications.cleanup`, rows older than 90 days, as under [channels](#channels)) and the outbox (`done` rows after 7 days, `dead` rows after 30 days, each status deleted by its own statement so both use the partial index);
      - cleanup of sessions that expired or were revoked more than a day ago (`sessions.cleanup`) and of email requests older than 7 days with their tokens (`email_requests.cleanup`);
      - account purge.
    - **Daily:** series horizon extension and the automatic retry of held recordings.
- **Bounds:** every job processes fixed-size batches through an index on its due column (partial where most rows are never due), within a time budget per run, and continues where it stopped: jobs that select by a window keep progress markers (reminders) or cursors in the job-run table (reconciliation). Purges delete in batches of bounded size per transaction.
- **Testing:**
  - with `OVC_CLOCK=test` the scheduler and lane loops are paused, and only the harness runs the worker;
  - `Tick(now)` runs one scheduler pass at `now` and then dispatches every due row until none remain;
  - running it twice with the same `now` has no extra effect.
