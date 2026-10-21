# API specification

REST under `/api/v1` (omitted from the paths below), JSON in and out except uploads. Authentication is `Authorization: Bearer <access token>` unless marked otherwise. The WebSocket surface is in [realtime and video](realtime-and-video.md#chat). The frontend is a thin client over this API and holds no rules of its own. Behavior is defined in the domain specifications linked from each section; this file fixes paths, callers, codes and conventions.

**Who:**

| Value | Meaning |
|---|---|
| `public` | needs no session |
| `user` | any signed-in account |
| `unverified ok` | also open to accounts whose email is not confirmed; every other authenticated route requires a confirmed email and answers `403 email_not_verified` otherwise |
| `tutor` | a tutor acting on their own resources |
| `student` | a student acting on their own resources |
| `audience` | the lesson's tutor, or a student in its current audience |
| `roster` | the lesson's tutor, or a student in its frozen roster |
| `member` | a channel member |
| `author` | the author of the message |
| `provider` | the meeting provider, authenticated as described under [callbacks](realtime-and-video.md#callbacks) |

- Every identifier in a path is resolved within the caller's scope, and anything outside it is `not_found`.
- A student calling a tutor route, or the reverse, gets `not_found`.
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
  | 409 | `conflict` (stale version), `lesson_overlap`, `lesson_not_live`, `outside_join_window`, `not_editable`, `answer_frozen`, `already_linked`, `student_exists`, `relationship_not_active`, `audience_not_active`, `role_mismatch`, `invitation_invalid`, `invitation_email_mismatch`, `return_not_allowed`, `issue_has_submissions`, `channel_read_only`, `edit_window_passed` |
  | 410 | `gone` (provider callbacks only) |
  | 413 | `payload_too_large` |
  | 415 | `unsupported_media_type` |
  | 422 | `validation_failed`, `lesson_in_past`, `tutor_code_invalid`, `token_invalid` (an invalid, expired, used or superseded confirmation or reset token, or an account in a state the token does not apply to, with one identical answer for every cause) |
  | 429 | `rate_limited`, `tutor_unavailable`, with `Retry-After` |
  | 500 | `internal` |
  | 503 | `provider_unavailable`, `service_unavailable` (readiness, or a saturated password-hashing limiter) |
  | 507 | `quota_exceeded` |

  Every code is an `ErrorCode` constant in `internal/domain`, listed by `AllErrorCodes` with its status and message, and published as the enum of `ErrorBody.code` in the OpenAPI document; the frontend derives its `ErrorCode` type from that enum and must translate every code in both languages. A conventions test requires `AllErrorCodes` to list exactly the declared constants, and an unlisted value is sent as `internal`. A client that receives a code it does not know shows a generic message with the request identifier.

- **Pagination:** every list route except `/calendar`, `/home`, `/tutors` and `/groups` is keyset paginated (`?limit=&cursor=`, default 30, maximum 100). Responses include `next_cursor`, which is `null` at the end.
- **Identifiers** are UUIDs. **Timestamps** are ISO-8601 in UTC; the client converts them for display.
- **Idempotency:** `PUT` routes and the submit, check, return, accept, decline, end, cancel and withdraw routes are safe to retry; repeating one in the same state returns the current state. Create routes (`POST /lessons`, `/students`, `/groups`, `/issues`, `/library/templates`) accept an `Idempotency-Key` header, and a repeated key returns the first result.
- **Request identity:** every response carries `X-Request-ID`, which also appears in error bodies and diagnostics ([observability](observability.md#correlation)). An incoming `X-Request-ID` is kept only when it appears once and matches `^[A-Za-z0-9._-]{1,64}$`; otherwise the server generates a new one and never rejects the request.
- **Schemas** are the Go structs in `internal/schemas`, validated by huma and published as OpenAPI JSON at `/api/docs/openapi.json` only when `OVC_API_DOCS_ENABLED=true` (off by default and in production), with no documentation UI; `api openapi` prints the same document without reading any setting. Student-facing and tutor-facing structs are separate wherever their content differs ([answer visibility](homework.md#answer-visibility)).
- **Server limits:** the HTTP server sets `ReadHeaderTimeout` (10 s), `IdleTimeout` (120 s) and `MaxHeaderBytes` (16 KiB). Every request has a default read deadline of 30 s and a write deadline of 35 s, set per request through `http.ResponseController`, so a request whose body stalls until the read deadline still receives its structured error; JSON bodies are limited to 1 MiB and must arrive within 30 s (both also set on every huma operation, whose own defaults are 1 MiB and 5 s). A body that does not arrive in time is answered with `400 bad_request`. The API process waits up to 40 s for in-flight requests on shutdown, longer than any request may last. Upload routes raise the body limit and the read deadline to the [file limits](files.md) and stream the body instead of buffering it; the WebSocket route clears the deadline after the upgrade.
- **Paths:** a request whose path is not in canonical form (a repeated `/`, a `.` or `..` segment, or a trailing `/`) receives `404 not_found` instead of the standard library's redirect, so every non-2xx response keeps the error body.
- **Validation fields:** `fields` appears only on `422 validation_failed`. A field path never repeats a property name the client invented: an unexpected property is reported at `body` with the code `unexpected_property`.
- **Exceptions:** responses that Go's HTTP server writes before any handler runs (`431` for oversized headers, `400` for a malformed request line or URL, `501` for an unsupported transfer encoding) are plain text without the error body or the headers below; the deployment's reverse proxy answers or filters these first. Go also allows about 4 KiB beyond `MaxHeaderBytes` before refusing.
- **SQL:** every business query is written in `backend/queries/` and compiled by sqlc (from slice 2), with `sqlc.narg` and `sqlc.slice` for optional filters and lists. Infrastructure statements (the readiness ping, `current_database()`, the test clock, migration bookkeeping, test database creation) are named constants with bound parameters in the package that owns them; test database names are generated and quoted with `pgx.Identifier`. No SQL is built from client input, and `NOTIFY` is sent as `pg_notify($1, $2)` with bound parameters.
- **Headers:**
  - every response, including the static frontend, sends `Referrer-Policy: no-referrer` and `X-Content-Type-Options: nosniff`;
  - pages send a Content Security Policy with `default-src 'self'`, `script-src 'self'`, `object-src 'none'`, `base-uri 'none'`, `frame-ancestors 'none'`, `frame-src` limited to the conferencing subdomain, and `connect-src`, `img-src` and `media-src` limited to the OVC origin and the storage host; `style-src`, `font-src` and `form-action` are `'self'`, and until the conferencing subdomain and the storage host exist (slices 7 and 2) `frame-src` is `'none'` and the other sources are `'self'` only; the policy is defined once in `frontend/security-headers.ts`;
  - the fake test room is the only exception, and only under `OVC_TEST_CONTROL=1`.

## Accounts

Behavior: [accounts](accounts.md). Every operation declares its audience in code (`public`, `user`, `unverified ok`, or the refresh cookie), and a test sends each non-public operation a request without a token and expects `401 unauthenticated`. `user` operations answer `403 email_not_verified` to an unverified account. `POST /auth/login` and `POST /auth/refresh` require an `Origin` equal to `OVC_PUBLIC_URL` (`403 forbidden` otherwise), login requires `Content-Type: application/json`, and login, refresh and `/users/me` answer with `Cache-Control: no-store`. Login and refresh set `__Host-ovc_refresh` with `Path=/`, `Secure`, `HttpOnly`, `SameSite=Strict` and no `Domain`; a refused refresh and both sign-outs clear it.

| Method | Path | Who | Purpose |
|---|---|---|---|
| POST | `/auth/register` | public | Sign up as tutor or student, with consent, language, browser timezone and an optional tutor code; always `202` with no session |
| POST | `/auth/verify-email` | public | Confirm with `{token}` from a signed-in browser, or `{token, new_password}` from any other; returns the pending-link outcome |
| POST | `/auth/verify-email/resend` | user, unverified ok | Send a new confirmation email |
| POST | `/auth/login` | public | Email and password → access token and refresh cookie |
| POST | `/auth/refresh` | refresh cookie, exact `Origin` | Rotate the refresh token → new access token; `204` with no token when there is no cookie |
| POST | `/auth/logout` | user, unverified ok | Revoke the current session |
| POST | `/auth/logout-all` | user, unverified ok | Revoke all sessions |
| POST | `/auth/password/forgot` | public | Request a reset email (same response for any email) |
| POST | `/auth/password/reset` | public | Set a new password with a reset token |
| POST | `/auth/password/change` | user, unverified ok | Change password with the current one |
| GET | `/users/me` | user, unverified ok | Own profile, role, state, preferences |
| PATCH | `/users/me` | user, unverified ok | Name, avatar, language, timezone, theme; tutor bio and subjects |
| DELETE | `/users/me` | user, unverified ok | Delete own account (password required) |
| GET, PUT | `/users/me/email-preferences` | user, unverified ok | Optional email switches |
| GET | `/unsubscribe/preview?token=` | public | The email group name for the confirmation page, nothing else |
| POST | `/unsubscribe?token=` | public (signed token) | Turn off one email group; accepts the RFC 8058 body `List-Unsubscribe=One-Click`; idempotent |
| GET | `/tutor/code` | tutor, unverified ok | Current tutor code (inactive until confirmed) |
| POST | `/tutor/code/regenerate` | tutor | New code; the old one stops working |
| GET, PATCH | `/tutor/settings` | tutor, unverified ok | Retention, quotas and usage, onboarding state |
| GET | `/tutor/blocked-requesters` | tutor | Students blocked from sending requests |
| DELETE | `/tutor/blocked-requesters/{studentId}` | tutor | Unblock |
| GET | `/avatars/{id}` | public | Avatar image |

## Students and groups

| Method | Path | Who | Purpose |
|---|---|---|---|
| GET | `/students?state=&group=&q=` | tutor | Own relationships |
| POST | `/students` | tutor | Create a student card with a link invitation |
| GET | `/students/{id}` | tutor | Card: profile, groups, notes, lesson history, issues and scores, current link |
| PATCH | `/students/{id}` | tutor | Display name, email (while invited), notes |
| DELETE | `/students/{id}` | tutor | Withdraw an invited card |
| POST | `/students/{id}/invitation` | tutor | Regenerate the link invitation (invited or ended cards) |
| POST | `/students/{id}/invitation/send` | tutor | Email an invitation to the card's email |
| POST | `/students/{id}/end` | tutor | End the relationship |
| GET | `/students/{id}/progress` | tutor | Checked work over time |
| GET | `/invitations/{token}` | public | Tutor profile for the invitation page; identical response for every invalid token |
| POST | `/invitations/{token}/accept` | public or student | Accept with a new account (with browser timezone) or as the signed-in student |
| POST | `/join-requests` | student | Send a request with a tutor code |
| GET | `/join-requests` | tutor, student | Pending requests (tutor) or own requests (student) |
| POST | `/join-requests/{id}/accept` | tutor | Accept |
| POST | `/join-requests/{id}/decline` | tutor | Decline, optionally with `block: true` |
| GET | `/tutors` | student | Own tutors, active and ended, with profile and colour |
| GET | `/groups` | tutor | Own groups |
| POST | `/groups` | tutor | Create with name, colour, members |
| GET | `/groups/{id}` | tutor | Group detail, members, lessons, issues |
| PATCH | `/groups/{id}` | tutor | Rename, recolour, add or remove members |
| POST | `/groups/{id}/archive`, `/groups/{id}/unarchive` | tutor | Archive or restore |

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
| POST | `/uploads?context=&scope=` | user allowed to write to the scope (`channel:<id>` or `submission:<id>:<question id>`), unverified ok | Upload → `{id, filename, media_type, size_bytes}` |
| GET | `/attachments/{id}` | anyone who can see a referencing item | `{url, expires_at}` with a 5-minute signed URL |

## Health

| Method | Path | Who | Purpose |
|---|---|---|---|
| GET | `/health/live` | public | Process is running: `{"status":"ok"}`; no I/O |
| GET | `/health/ready` | public | Database reachable within 1 s (storage joins in slice 2): `{"status":"ready","checks":{"database":"ok"}}`, otherwise 503 `service_unavailable` without driver detail. Concurrent requests share one ping, and its result is reused for 1 s, so the route holds at most one database connection |

The end-to-end test control surface is not part of this API. It exists only in test deployments ([development](../../docs/development.md#end-to-end-test-harness)).
