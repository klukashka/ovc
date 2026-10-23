# API specification

REST under `/api/v1` (omitted from the paths below), JSON in and out except uploads. Authentication is `Authorization: Bearer <access token>` unless marked otherwise. The WebSocket surface is in [realtime and video](realtime-and-video.md#chat). The frontend is a thin client over this API and holds no rules of its own. Behavior is defined in the domain specifications linked from each section; this file fixes paths, callers, codes and conventions.

In the MVP the recordings routes and the recording-ready callback, `GET /students/{id}/progress` and `GET /issues/{id}/analytics` are not registered, and `/tutor/settings` returns and accepts no recording retention or recording quota and usage; they come [after the MVP](../../docs/roadmap.md#after-the-mvp) by the owner's decision of 2026-10-22.

**Who:**

| Value | Meaning |
|---|---|
| `public` | needs no session |
| `user` | any signed-in account |
| `unverified ok` | also open to accounts whose email is not confirmed; every other authenticated route requires a confirmed email and answers `403 email_not_verified` otherwise |
| `tutor` | a tutor acting on their own resources |
| `student` | a student acting on their own resources |
| `role check` | open to both roles at the audience level; the service refuses the other role with `409 role_mismatch` ([FR-1.3](../../docs/requirements/functional.md#fr-1-accounts)) |
| `audience` | the lesson's tutor, or a student in its current audience |
| `roster` | the lesson's tutor, or a student in its frozen roster |
| `member` | a channel member |
| `author` | the author of the message |
| `provider` | the meeting provider, authenticated as described under [callbacks](realtime-and-video.md#callbacks) |

- **Refusal order:** audience refusals come first and depend only on the caller: the other role gets `404 not_found`, with the same body and headers as an unknown route, then an unconfirmed email gets `403 email_not_verified`. Both are checked before the body is read, so a caller of the other role gets `404` whatever the body holds: missing required fields, wrong lengths, types or formats, an unexpected property, another content type, or JSON that is not well formed. Then comes the request itself: a body that is not well-formed JSON answers `400`, and the operation's schema checks answer `422 validation_failed` (required fields, lengths, patterns, types, `unexpected_property`). These checks depend only on the request and reveal nothing about other accounts' data. A malformed path identifier answers `404`. Then every identifier in a path or a body is resolved within the caller's scope, and anything outside it is `not_found`, with the same body as a random identifier. Then come `409 role_mismatch` and state refusals (`409`), then the remaining validation the service performs (`422`), so a state refusal never confirms that an out-of-scope identifier exists. Routes whose order differs say so in their row and in the per-route orders of the [accounts specification](accounts.md#invitations), for example the new-account path of `POST /invitations/{token}/accept` and `POST /join-requests`, which answers `422 tutor_code_invalid` before `409 already_linked`.
- A student calling a tutor route, or the reverse, gets `not_found`; the only role-specific refusal is `409 role_mismatch` on invitation acceptance and code entry, which both roles reach.
- Every authenticated request checks the session and account state ([roles and accounts](accounts.md#roles-and-accounts)).

## Conventions

- **Errors:** every non-2xx response has the body
  `{"error": {"code": "<stable_snake_case>", "message": "<English fallback>", "request_id": "<id>", "fields": [...], "details": {...}}}`.
  - `code` values are an enum in `internal/domain`. The frontend translates them; `message` is a developer fallback, never shown as is.
  - `fields` lists `{"path", "code"}` for validation errors.
  - `details` carries structured data for a few codes, for example the conflicting lessons of `lesson_overlap`.
  - No stack traces, SQL or internal identifiers appear in an error, and no submitted value is echoed back: huma's default problem-details error is replaced through `huma.NewError` with this shape, and `fields` carries paths and codes only.
- **Error codes:**

  | Status | Codes |
  |---|---|
  | 400 | `bad_request` |
  | 401 | `unauthenticated`, `token_expired`, `invalid_signature` (provider callbacks), `invalid_credentials` (a wrong password, an unknown email or a deleted account, with one identical answer) |
  | 403 | `email_not_verified`, `account_blocked` (only after the correct password, or for an existing session), `forbidden` (allowed to see, not to change) |
  | 404 | `not_found` |
  | 405 | `method_not_allowed` |
  | 409 | `conflict` (stale version), `lesson_overlap`, `lesson_not_live`, `outside_join_window`, `not_editable`, `answer_frozen`, `already_linked`, `student_exists`, `relationship_not_active`, `audience_not_active`, `role_mismatch`, `invitation_invalid`, `invitation_email_mismatch`, `relationship_not_provisional` (removal of a card that is not `provisional` or `reclaim_pending`), `return_not_allowed`, `issue_has_submissions`, `channel_read_only`, `edit_window_passed` |
  | 410 | `gone` (provider callbacks only) |
  | 413 | `payload_too_large` |
  | 415 | `unsupported_media_type` |
  | 422 | `validation_failed`, `lesson_in_past`, `tutor_code_invalid`, `token_invalid` (an invalid, expired, used or superseded confirmation or reset token, or an account in a state the token does not apply to, with one identical answer for every cause) |
  | 429 | `rate_limited`, `tutor_unavailable`, with `Retry-After` |
  | 500 | `internal` |
  | 503 | `provider_unavailable`, `service_unavailable` (readiness, a saturated password-hashing limiter, or a full avatar decoding budget) |
  | 507 | `quota_exceeded` |

  Every code is an `ErrorCode` constant in `internal/domain`, listed by `AllErrorCodes` with its status and message, and published as the enum of `ErrorBody.code` in the OpenAPI document; the frontend derives its `ErrorCode` type from that enum and must translate every code in both languages. A conventions test requires `AllErrorCodes` to list exactly the declared constants, and an unlisted value is sent as `internal`. A client that receives a code it does not know shows a generic message with the request identifier.

- **Pagination:** every list route except `/calendar`, `/home`, `/tutors` and `/groups` is keyset paginated (`?limit=&cursor=`, default 30, maximum 100). Responses include `next_cursor`, which is `null` at the end. `/tutors` is bounded by the relationships tutors create for the student, and `/groups` by the limit of 200 groups per tutor ([groups](accounts.md#groups)).
- **Identifiers** are UUIDs. **Timestamps** are ISO-8601 in UTC; the client converts them for display.
- **Idempotency:** `PUT` routes and the submit, check, return, accept, decline, end, cancel, archive, unarchive and withdraw routes are safe to retry; repeating one in the same state returns the current state, and a repeated withdrawal of a student card (`DELETE /students/{id}`) answers `204`. Removal of a provisional student is not retry-safe: a repeat names a discarded card and answers `not_found`. Create routes (`POST /lessons`, `/students`, `/groups`, `/issues`, `/library/templates`) accept an `Idempotency-Key` header, and a repeated key returns the first result; for cards and groups the key is stored per tutor with the created row.
- **Request identity:** every response carries `X-Request-ID`, which also appears in error bodies and diagnostics ([observability](observability.md#correlation)). An incoming `X-Request-ID` is kept only when it appears once and matches `^[A-Za-z0-9._-]{1,64}$`; otherwise the server generates a new one and never rejects the request.
- **Schemas** are the Go structs in `internal/schemas`, validated by huma and published as OpenAPI JSON at `/api/docs/openapi.json` only when `OVC_API_DOCS_ENABLED=true` (off by default and in production), with no documentation UI; `api openapi` prints the same document without reading any setting. Student-facing and tutor-facing structs are separate wherever their content differs ([answer visibility](homework.md#answer-visibility)).
- **Server limits:** the HTTP server sets `ReadHeaderTimeout` (10 s), `IdleTimeout` (120 s) and `MaxHeaderBytes` (16 KiB). Every request has a default read deadline of 30 s and a write deadline of 35 s, set per request through `http.ResponseController`, so a request whose body stalls until the read deadline still receives its structured error; JSON bodies are limited to 1 MiB and must arrive within 30 s (both also set on every huma operation, whose own defaults are 1 MiB and 5 s). A body that does not arrive in time is answered with `400 bad_request`. The API process waits up to 40 s for in-flight requests on shutdown, longer than any request may last. Upload routes raise the body limit and the read deadline to the [file limits](files.md) and stream the body instead of buffering it; the WebSocket route clears the deadline after the upgrade.
- **Paths:** a request whose path is not in canonical form (a repeated `/`, a `.` or `..` segment, or a trailing `/`) receives `404 not_found` instead of the standard library's redirect, so every non-2xx response keeps the error body.
- **Validation fields:** `fields` appears only on `422 validation_failed`. A field path never repeats a property name the client invented: an unexpected property is reported at `body` with the code `unexpected_property`.
- **Exceptions:** responses that Go's HTTP server writes before any handler runs (`431` for oversized headers, `400` for a malformed request line or URL, `501` for an unsupported transfer encoding) are plain text without the error body or the headers below; the deployment's reverse proxy answers or filters these first. Go also allows about 4 KiB beyond `MaxHeaderBytes` before refusing.
- **SQL:** every business query is written in `backend/queries/` and compiled by sqlc (from slice 2), with `sqlc.narg` and `sqlc.slice` for optional filters and lists. Infrastructure statements (the readiness ping, `current_database()`, the test clock, migration bookkeeping, test database creation) are named constants with bound parameters in the package that owns them; test database names are generated and quoted with `pgx.Identifier`. No SQL is built from client input, and `NOTIFY` is sent as `pg_notify($1, $2)` with bound parameters.
- **Headers:**
  - every response, including the static frontend, sends `Referrer-Policy: no-referrer` and `X-Content-Type-Options: nosniff`;
  - every API response with a status of 400 or above sends `Cache-Control: no-store`;
  - pages send a Content Security Policy with `default-src 'self'`, `script-src 'self'`, `object-src 'none'`, `base-uri 'none'`, `frame-ancestors 'none'`, `frame-src` limited to the conferencing subdomain, and `connect-src`, `img-src` and `media-src` limited to the OVC origin and the storage host; `style-src`, `font-src` and `form-action` are `'self'`, and until the conferencing subdomain and the storage host exist (slices 7 and 2) `frame-src` is `'none'` and the other sources are `'self'` only; the policy is defined once in `frontend/security-headers.ts`;
  - the fake test room is the only exception, and only under `OVC_TEST_CONTROL=1`.

## Accounts

Behavior: [accounts](accounts.md). Every operation declares its audience in code (`public`, `user`, `user, unverified ok`, `tutor`, `tutor, unverified ok`, `student`, `student, unverified ok`, or the refresh cookie), and a test sends each non-public operation a request without a token and expects `401 unauthenticated`. A role audience refuses the other role with `404 not_found` first; then `user`, `tutor` and `student` operations answer `403 email_not_verified` to an unverified account. `POST /auth/login`, `POST /auth/refresh` and `POST /auth/verify-email` (which can sign a browser in) require an `Origin` equal to `OVC_PUBLIC_URL` (`403 forbidden` otherwise), every route with a body requires `Content-Type: application/json` (`415 unsupported_media_type` otherwise), and the authentication routes and `/users/me` answer with `Cache-Control: no-store`. Routes marked "public, optional bearer" authenticate a bearer token when one is sent, answering `401` or `403` like any authenticated route when it is not valid, and otherwise run anonymously. Validation refusals name the field as `body.<property>`; the account routes use the field codes `invalid`, `required`, `not_allowed`, `too_short`, `too_long` and `incorrect`. Login and refresh set `__Host-ovc_refresh` with `Path=/`, `Secure`, `HttpOnly`, `SameSite=Strict` and no `Domain`; a refused refresh and both sign-outs clear it.

| Method | Path | Who | Purpose |
|---|---|---|---|
| POST | `/auth/register` | public | Sign up with name, email, password, language, browser timezone and consent; `202` with no session, the same for a new and a taken email. `role: student` also needs `tutor_code` (`422 validation_failed` with `body.tutor_code`: `required`), checked after the per-IP request slot and before validation and hashing: an unknown, regenerated or inactive code answers `422 tutor_code_invalid` and creates nothing; a new student account gets a held join request ([accounts](accounts.md#registration-and-email-verification)); `429` over the per-IP limit |
| POST | `/auth/verify-email` | public, optional bearer | Confirm with `{token}` from a browser signed in to the token's account, or `{token, new_password}` from any other (`422 validation_failed` with `body.new_password`: `required` when the password is missing, which does not use the token). Answers `200` with `{outcome}`, one of `choice_required`, `linked`, `request_sent`, `code_invalid` or `null`, chosen by the precedence in [accounts](accounts.md#registration-and-email-verification); from another browser (a reclaim) also `{access_token, expires_in}` and the refresh cookie |
| POST | `/auth/verify-email/resend` | public, optional bearer | Send a new confirmation email: `{}` with a bearer token for the signed-in account (`429` with `Retry-After` within the sending limits), or `{email}` without one, which always answers `202` |
| POST | `/auth/login` | public | Email and password → access token and refresh cookie |
| POST | `/auth/refresh` | refresh cookie, exact `Origin` | Rotate the refresh token → new access token; `204` with no token when there is no cookie |
| POST | `/auth/logout` | user, unverified ok | Revoke the current session |
| POST | `/auth/logout-all` | user, unverified ok | Revoke all sessions |
| POST | `/auth/password/forgot` | public | Request a reset email with `{email}`; always `202`, the same for any email |
| POST | `/auth/password/reset` | public | Set a new password with `{token, new_password}`; `200` with `{outcome}`, which is `choice_required` after a reclaim of an unverified account and `null` otherwise; the person signs in afterwards |
| POST | `/auth/password/change` | user, unverified ok | Change the password with `{current_password, new_password}`; `204`; a wrong current password answers `422 validation_failed` (`body.current_password`: `incorrect`) |
| GET | `/users/me` | user, unverified ok | Own profile, role, state, preferences, `avatar_id` (or `null`), `reclaim_pending` (true while links or requests wait for the owner's choice after a reclaim, derived from those rows), and for tutors `bio` and `subjects` |
| PATCH | `/users/me` | user, unverified ok | Any of `name`, `language`, `timezone`, `theme`, `avatar_id` (a pending avatar upload of the caller, or `null` to remove the avatar), and for tutors `bio` (up to 1000 characters) and `subjects` (up to 10, each up to 60 characters, no repeats); answers like `GET`. A student sending `bio` or `subjects` gets `not_allowed`, an unknown timezone or someone else's upload `invalid` |
| DELETE | `/users/me` | user, unverified ok | Delete own account with `{password}`; `204` with the refresh cookie cleared. A wrong password answers `422 validation_failed` (`body.password`: `incorrect`), counted with the other current-password checks (`429` after five in 15 minutes) |
| GET, PUT | `/users/me/email-preferences` | user, unverified ok | Optional email switches |
| GET | `/users/me/reclaim` | user | The items waiting for the owner's choice: `{links: [{relationship_id, tutor: {name, avatar_id, subjects}}], requests: [{request_id, tutor: {name, avatar_id, subjects}}], choose_by}` ([reclaim](accounts.md#provisional-links-and-reclaim)) |
| POST | `/users/me/reclaim` | user | Keep or drop every waiting item with `{keep_links, drop_links, keep_requests, drop_requests}`, each a list of identifiers, every waiting item named exactly once → `{outcome}`; when nothing waits (a repeat, or after the deadline), `200 {outcome: null}` without changes. `404` for an identifier that is not one of the caller's waiting items, checked before `422 validation_failed` for an item missing or named twice; `401` when the session was revoked before the account row was locked |
| GET | `/unsubscribe/preview?token=` | public | The email group name for the confirmation page, nothing else |
| POST | `/unsubscribe?token=` | public (signed token) | Turn off one email group; accepts the RFC 8058 body `List-Unsubscribe=One-Click`; idempotent |
| GET, PATCH | `/tutor/settings` | tutor, unverified ok | Retention, quotas and usage, onboarding state |
| GET | `/avatars/{id}` | public | Avatar image of an active or unverified account, or `404` for anything else, including a malformed identifier; `503 service_unavailable` with `Retry-After` while the instance serves its limit of avatars at once ([avatars](files.md#avatars)) |

## Students and groups

Behavior: [relationships](accounts.md#tutorstudent-relationships), [invitations](accounts.md#invitations), [provisional links and reclaim](accounts.md#provisional-links-and-reclaim), [tutor code and join requests](accounts.md#tutor-code-and-join-requests), [ending](accounts.md#ending-a-relationship), [groups](accounts.md#groups).

- **Student card** (`StudentCard`, tutor-facing): `{id, state, display_name, email, account: {name, email, avatar_id, email_confirmed} | null, groups: [{id, name, colour}], notes, invitation: {url, expires_at, status: open | expired | revoked_after_mismatches} | null, linked_at, ended_at, ended_by, created_at}`. `account` is `null` while the card is `invited` or `reclaim_pending`. `groups` lists the card's groups that are not archived. `invitation` describes the card's open invitation, or, when it has none, its latest invitation if that was revoked after repeated wrong emails, and is `null` otherwise and for every card that is not `invited` or `ended`; `url` is set only while the status is `open`, so a link is shown again only while it can be used (`null` also when the stored ciphertext cannot be opened, which `invitation.unreadable` records). `StudentCardSummary` omits `notes` and `invitation`.
- **Error details:** `student_exists` carries `details.student_id`, the identifier of the tutor's existing card with that email.
- **The student's tutor** (`MyTutor`, student-facing): `{relationship_id, state: active | provisional | ended, colour, tutor: {name, avatar_id, subjects, bio}, ended_at, ended_by, account_closed_until}`. `account_closed_until` is set while the tutor's account is `deleted`, to the deletion time plus 30 days (the purge), and is `null` otherwise. Student-facing structs never contain the card's display name, notes or email, or other students.
- **Responses that carry or accept an invitation link** send `Cache-Control: no-store`.
- **Refusal order:** as in the [conventions above](#api-specification): role (`404`) and email confirmation (`403`) before the body is read, then parsing and schema validation (`400`, `422`), scope (`404`), `role_mismatch` and state (`409`), then the service's remaining validation (`422`), except where a row says otherwise.

| Method | Path | Who | Request | Success | Refusals |
|---|---|---|---|---|---|
| GET | `/tutor/code` | tutor, unverified ok | — | `{code, active}`: `code` in its display form `XXXX-XXXX`, `active` false while the tutor's email is unconfirmed | — |
| POST | `/tutor/code/regenerate` | tutor | — | `{code, active: true}`; the old code stops working | 403 `email_not_verified`; 429 `rate_limited` (10 per UTC day, with `Retry-After` until the next day) |
| GET | `/students?state=&group=&q=&limit=&cursor=` | tutor | — | `{items: [StudentCardSummary], next_cursor}`, newest first, `no-store`; `state` is `active` (with `provisional` and `reclaim_pending`), `invited` or `ended`, and without it every card that is not discarded is listed; `group` is a group identifier, `q` at most 254 characters, and `cursor` the opaque `next_cursor` of the previous page | 422 `validation_failed` for a bad filter or cursor |
| POST | `/students` + `Idempotency-Key` | tutor | `{display_name, email?}` | 201 card with its link, `no-store`; a repeated key returns the card it created, as it is now, whatever the body; it is checked after the schema validation and before the refusals that follow it below | 403 `email_not_verified`; 404 for a repeated key whose card was since withdrawn or discarded; 409 `student_exists` (`details.student_id`); 422 `validation_failed` (also an `Idempotency-Key` that is not 1–200 printable ASCII characters without spaces); 429 `rate_limited` (50 cards per UTC day, counting only created cards, with `Retry-After` until the next day) |
| GET | `/students/{id}` | tutor | — | card, `no-store` | 404 |
| PATCH | `/students/{id}` | tutor | `{display_name?, email?, notes?}`; `email: null` removes the email | card, `no-store`; a changed email of an invited card, including a removed one, revokes its link and issues a fresh one; an email equal to the current one after normalisation is no change | 404; 409 `not_editable` (the email unless invited); 409 `student_exists`; 422 `validation_failed`; 429 `rate_limited` (50 email changes per UTC day) |
| DELETE | `/students/{id}` | tutor | — | 204, also when repeated for a card this tutor withdrew | 404 (including any other discarded card); 409 `relationship_not_active` for a provisional, reclaim-pending, active or ended card |
| POST | `/students/{id}/invitation` | tutor | — | `{invitation: {url, expires_at, status: open}}`, `no-store`; regenerates the link; the link of an `ended` card is bound to its linked account | 404; 409 `relationship_not_active` unless invited or ended; 429 `rate_limited` (50 regenerations per tutor per UTC day, with `Retry-After` until the next day) |
| POST | `/students/{id}/invitation/send` | tutor | no body | 202 with an empty body, `no-store`; an expired open link is revoked (`expired_on_send`) and a fresh one sent, and a card with no open link (an ended card after ending, or a link revoked after repeated wrong emails) gets a fresh one, counted against the regeneration limit; over the recipient cap, or for a blocked or deleted recipient account, the send is dropped silently with the same answer, still counts toward the tutor's budget and still refreshes an expired link | 404; 409 `relationship_not_active` unless invited or ended; 422 `validation_failed` (`card.email`: `required` when the card has no email); 429 `rate_limited` (the tutor's own budget, or the regeneration limit when a fresh link is needed; never for the recipient; with `Retry-After` until the next UTC day) |
| POST | `/students/{id}/end` | tutor | — | card (idempotent when ended) | 404; 409 `relationship_not_active` for an invited, provisional or reclaim-pending card |
| POST | `/students/{id}/remove` | tutor | — | `{new_card: StudentCard}`, `no-store` | 404; 409 `relationship_not_provisional` unless provisional or reclaim-pending |
| GET | `/students/{id}/progress` | tutor | — | Checked work over time (after the MVP) | 404 |
| GET | `/invitations/{token}` | public | — | `{tutor: {name, avatar_id, subjects}}`, `no-store` | 404 for every unknown, expired, used, revoked or malformed token and for the link of a tutor who is not active, with one identical body |
| POST | `/invitations/{token}/accept` | public, optional bearer, role check | without a bearer `{name, email, password, language, timezone, consent}`; with a bearer `{}` | without a bearer `202 {}`, identical for new and taken emails; with a bearer `200 {tutor: MyTutor}`; `no-store` | 409 `invitation_invalid`; 409 `invitation_email_mismatch` (counted; the fifth revokes the link); 409 `role_mismatch`; 403 `email_not_verified` (an unconfirmed account that already holds a provisional link); 422 `validation_failed` (a missing sign-up field without a bearer: `required`, timezone included as on `/auth/register`; sign-up fields sent with a bearer: `not_allowed`); 429 `rate_limited` (the shared per-IP request counter, without a bearer only). With a bearer the order is: sign-up fields (`422`), `role_mismatch`, `email_not_verified`, `invitation_invalid`, then `invitation_email_mismatch` ([accounts](accounts.md#invitations)) |
| POST | `/join-requests` | user, unverified ok, role check | `{code}` | 200 `{request: SentJoinRequest}`, identical for a created, existing, held and suppressed request | 409 `role_mismatch`; 422 `tutor_code_invalid`; 409 `already_linked`; 429 `tutor_unavailable` (confirmed students only); 429 `rate_limited` (10 per student per hour) |
| GET | `/join-requests` | tutor | `limit`, `cursor` | `{items: [{id, student: {name, email, avatar_id}, created_at, expires_at}], next_cursor}`, visible pending requests, oldest first | — |
| GET | `/join-requests/sent` | student, unverified ok | `limit`, `cursor` | `{items: [{id, state: pending \| accepted \| not_accepted \| expired, created_at, decided_at}], next_cursor}` (`SentJoinRequest`), newest first | — |
| POST | `/join-requests/{id}/accept` | tutor | — | card (idempotent when accepted); reactivates or links the student's card and revokes its open link, or creates a card named with a copy of the student's account name and no email | 404; 409 `not_editable` |
| POST | `/join-requests/{id}/decline` | tutor | `{block?: bool}` | `{id, state}` (idempotent) | 404; 409 `not_editable` |
| GET | `/tutor/blocked-requesters` | tutor | `limit`, `cursor` | `{items: [{student_id, name, avatar_id, blocked_at}], next_cursor}` | — |
| DELETE | `/tutor/blocked-requesters/{studentId}` | tutor | — | 204; the student's requests suppressed by the block expire | 404 |
| GET | `/tutors` | student, unverified ok | — | `{tutors: [MyTutor]}`, active, provisional and ended; `reclaim_pending` links are left out; current links (`active`, `provisional`) first, then ended ones, each by linking time and identifier; `no-store` | — |
| POST | `/tutors/{relationshipId}/end` | student, unverified ok | — | `MyTutor` (idempotent when ended); on a provisional link the removal effects and `204` | 404 for another account's relationship and for the caller's own invited, discarded or reclaim-pending relationship |
| GET | `/groups` | tutor | — | `{groups: [GroupSummary]}` with `member_count` and the first five members | — |
| POST | `/groups` + `Idempotency-Key` | tutor | `{name, colour, member_ids}` (at most 30) | 201 group | 404 (any member outside scope); 409 `relationship_not_active` (`details.relationship_ids`); 422 `validation_failed` (including the 201st group) |
| GET | `/groups/{id}` | tutor | — | `{id, name, colour, archived_at, members: [{relationship_id, display_name, name, avatar_id, added_at}]}`; lessons and issues arrive with slices 5 and 9 | 404 |
| PATCH | `/groups/{id}` | tutor | `{name?, colour?, add?, remove?}` | group | 404; 409 `relationship_not_active`; 409 `audience_not_active` (adding to an archived group); 422 `validation_failed` (`too_long` beyond 30 members) |
| POST | `/groups/{id}/archive`, `/groups/{id}/unarchive` | tutor | — | group (idempotent) | 404 |

The transactions behind `POST /invitations/{token}/accept` with a bearer, `POST /join-requests`, `POST /tutors/{relationshipId}/end`, `POST /users/me/reclaim`, `POST /auth/verify-email` from the account's own browser, and every write open to unconfirmed accounts (`PATCH /users/me`, the avatar upload and its settling, and `PUT /users/me/email-preferences` from slice 4) re-check the session under the account's row lock and answer `401 unauthenticated` when it was revoked meanwhile ([locks and the session re-check](accounts.md#locks-and-the-session-re-check)). As implemented so far, `POST /auth/verify-email` from the account's own browser and `POST /invitations/{token}/accept` with a bearer re-check the session; `PATCH /users/me`, the avatar upload and its settling gain it with the reclaim choice, together with `TestRequestAuthenticatedBeforeReclaimCannotActAfterIt`, and the other routes when they land.

## Schedule

Behavior: [schedule](schedule.md).

| Method | Path | Who | Purpose |
|---|---|---|---|
| GET | `/calendar?from=&to=&tutor=` | user | Lessons in a range of at most 62 days |
| POST | `/lessons` | tutor | Create a one-off lesson or a series; `409 lesson_overlap` until `allow_overlap` |
| GET | `/lessons/{id}` | audience or roster | Lesson page data, including the second timezone, `room_open` and `embed_allowed` |
| PATCH | `/lessons/{id}` | tutor | Edit with `scope`: `this` or `following`; `409 lesson_overlap` until `allow_overlap` |
| POST | `/lessons/{id}/cancel` | tutor | Cancel with `scope`: `this` or `following` |
| POST | `/lessons/{id}/materials` | tutor | Attach or detach library materials |
| POST | `/lessons/{id}/join` | audience or roster | Join link ([join flow](realtime-and-video.md#join-flow)) |
| POST | `/lessons/{id}/end` | tutor | End the lesson; the meeting ends through the outbox |
| POST | `/lessons/{id}/restart-room` | tutor | End the open meeting and start a new one; earlier join links stop working |
| GET | `/lessons/{id}/recordings` | roster | Recordings with state and expiry |
| GET | `/recordings/{id}/playback` | roster | `{url, expires_at}` with a 5-minute signed media URL |
| POST | `/recordings/{id}/retry` | tutor | Retry a copy held by the quota |
| DELETE | `/recordings/{id}` | tutor | Delete a recording |
| POST | `/integrations/bbb/{routing_key}/recording-ready` | provider | Recording-ready callback: 200, 401 or 410 |
| GET | `/integrations/bbb/{routing_key}/ended` | provider | Meeting-ended hint: always 200 |

## Chat

Behavior: [chat](realtime-and-video.md#chat).

| Method | Path | Who | Purpose |
|---|---|---|---|
| GET | `/channels` | user | Own channels with last message, unread count, writability |
| GET | `/channels/{id}/messages` | member | History, keyset paginated |
| POST | `/channels/{id}/messages` | member | Send (also delivered over WebSocket) |
| PATCH | `/messages/{id}` | author | Edit within 24 hours |
| DELETE | `/messages/{id}` | author; tutor in their group channel | Delete |
| POST | `/channels/{id}/read` | member | Move the read marker |

## Homework

Behavior: [homework](homework.md).

| Method | Path | Who | Purpose |
|---|---|---|---|
| GET | `/library?folder=&q=` | tutor, unverified ok | Folders, materials and templates |
| POST | `/library/folders` | tutor, unverified ok | Create folder |
| PATCH, DELETE | `/library/folders/{id}` | tutor, unverified ok | Rename, move, delete an empty folder |
| POST | `/library/materials` | tutor, unverified ok | Create a material from an uploaded attachment |
| PATCH, DELETE | `/library/materials/{id}` | tutor, unverified ok | Rename, move, delete |
| POST | `/library/templates` | tutor, unverified ok | Create a template |
| GET | `/library/templates/{id}` | tutor, unverified ok | Template with answer keys |
| PUT | `/library/templates/{id}` | tutor, unverified ok | Replace title and blocks (with `version`) |
| POST | `/library/templates/{id}/duplicate` | tutor, unverified ok | Copy |
| DELETE | `/library/templates/{id}` | tutor, unverified ok | Delete |
| POST | `/issues` | tutor | Issue a template to an active relationship or a group |
| GET | `/issues?status=active\|withdrawn&group=&student=&needs_checking=` | tutor | Issued work with progress counts |
| GET | `/issues/{id}` | tutor | Issue with snapshot and submissions |
| PATCH | `/issues/{id}` | tutor | Change the deadline or the linked lesson |
| POST | `/issues/{id}/withdraw` | tutor | Withdraw while nothing was submitted |
| GET | `/issues/{id}/analytics` | tutor | Analytics |
| GET | `/assignments?status=todo\|submitted\|checked\|closed&tutor=` | student | Own submissions across tutors (`todo` covers not started, draft and returned) |
| GET | `/submissions/{id}` | student (own), tutor | Student schema or tutor schema of a submission |
| PUT | `/submissions/{id}/answers` | student | Autosave changed answers with `version` and a save identifier |
| POST | `/submissions/{id}/submit` | student | Submit or resubmit |
| GET | `/review-queue?issue=&group=&student=` | tutor | Submissions needing checking, oldest first |
| PUT | `/submissions/{id}/review` | tutor | Save the draft review revision |
| POST | `/submissions/{id}/check` | tutor | Publish the named draft revision and mark checked |
| POST | `/submissions/{id}/return` | tutor | Publish the named draft revision and return for revision |

## Notifications and home pages

| Method | Path | Who | Purpose |
|---|---|---|---|
| GET | `/notifications` | user, unverified ok | In-app notifications, newest first, with `unread_count` |
| POST | `/notifications/read` | user, unverified ok | Mark some or all as read |
| GET | `/home?tutor=` | user, unverified ok | Tutor: Today data. Student: Home data, optionally filtered by tutor: the next lesson, submissions to do, the last five checked submissions, each with a feedback excerpt and a corrected-file flag taken only from the latest published review revision ([homework](homework.md#answer-visibility)) and never points (the excerpt is the overall feedback as plain text: Markdown markers removed, link text kept without the address, paragraphs and list items joined by single spaces, code kept as its text; kept whole when it has at most 140 characters, otherwise cut at the last word boundary within 139 characters (or at 139 characters when the first word is longer) and followed by an ellipsis, so it never exceeds 140 characters; `null` without overall feedback), lessons starting within the next seven days under the same visibility as `/calendar` ([schedule](schedule.md)) without cancelled ones, and the student's active tutors (name, avatar, colour, subjects, direct channel id). The `tutor` filter applies to every part except the tutors. One request per page load |

## Files

Behavior: [files](files.md).

| Method | Path | Who | Purpose |
|---|---|---|---|
| POST | `/uploads?context=&scope=` | user allowed to write to the scope (`channel:<id>` or `submission:<id>:<question id>`), unverified ok | Upload a `multipart/form-data` body with one `file` part → `201` with `{id, filename, media_type, size_bytes}`. Slice 2 accepts only `context=avatar` ([avatars](files.md#avatars)): `400 bad_request` for a malformed body, `413 payload_too_large`, `415 unsupported_media_type`, `429 rate_limited`, `503 service_unavailable` with `Retry-After` when the decoding budget is full, and `422 validation_failed` (`body.file`: `required`) for a missing or extra part |
| GET | `/attachments/{id}` | anyone who can see a referencing item | `{url, expires_at}` with a 5-minute signed URL |

## Health

| Method | Path | Who | Purpose |
|---|---|---|---|
| GET | `/health/live` | public | Process is running: `{"status":"ok"}`; no I/O |
| GET | `/health/ready` | public | Database and storage bucket each reachable within 1 s: `{"status":"ready","checks":{"database":"ok","storage":"ok"}}`, otherwise 503 `service_unavailable` without driver detail, and the `health.ready.failed` record names the failed check. Concurrent requests share one check, and its result is reused for 1 s, so the route holds at most one database connection |

The end-to-end test control surface is not part of this API. It exists only in test deployments ([development](../../docs/development.md#end-to-end-test-harness)).
