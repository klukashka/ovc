# API specification

REST under `/api/v1`, JSON in and out. Authentication is `Authorization: Bearer <access JWT>`
unless marked public. All list endpoints are paginated (`?limit=&cursor=`). Every error has the
structured body defined under [conventions](#conventions). The WebSocket surface is specified in
[realtime and video](realtime-and-video.md#chat). The frontend is a thin client over this API and
holds no rules of its own.

Legend: 🔓 public · 👤 any authenticated · 🎓 student · 🧑‍🏫 teacher (course owner) · 🛡️ admin.

## Auth

| Method | Path | Who | Purpose |
|--------|------|-----|---------|
| POST | `/api/v1/auth/register` | 🔓 | Create account (role teacher or student). |
| POST | `/api/v1/auth/login` | 🔓 | Email+password → access + refresh tokens. |
| POST | `/api/v1/auth/refresh` | 🔓* | Rotate refresh token → new access token. |
| POST | `/api/v1/auth/logout` | 👤 | Revoke the current refresh token. |
| GET  | `/api/v1/auth/me` | 👤 | Current user profile. |

\* requires a valid refresh token (cookie or body), not an access token.

## Users / profiles

| Method | Path | Who | Purpose |
|--------|------|-----|---------|
| GET | `/api/v1/users/{id}` | 👤 | Public-ish profile (teacher bio/subject). |
| PATCH | `/api/v1/users/me` | 👤 | Update own profile (name, bio, subject, timezone). |
| POST | `/api/v1/users/me/avatar` | 👤 | Upload avatar (multipart). |

## Courses

| Method | Path | Who | Purpose |
|--------|------|-----|---------|
| GET | `/api/v1/courses` | 👤 | Courses I teach or am enrolled in. |
| POST | `/api/v1/courses` | 🧑‍🏫 | Create a course. |
| GET | `/api/v1/courses/{id}` | member | Course detail. |
| PATCH | `/api/v1/courses/{id}` | 🧑‍🏫 | Update / archive. |
| POST | `/api/v1/courses/{id}/enroll` | 🎓 | Self-enroll via `join_code`. |
| GET | `/api/v1/courses/{id}/members` | member | List teacher + students. |
| POST | `/api/v1/courses/{id}/members` | 🧑‍🏫 | Invite/add a student. |
| DELETE | `/api/v1/courses/{id}/members/{userId}` | 🧑‍🏫 | Remove a student. |

## Calendar / lessons

| Method | Path | Who | Purpose |
|--------|------|-----|---------|
| GET | `/api/v1/courses/{id}/lessons` | member | Lessons for a course (date range filter). |
| GET | `/api/v1/calendar` | 👤 | My upcoming lessons across all courses. |
| POST | `/api/v1/courses/{id}/lessons` | 🧑‍🏫 | Schedule a lesson (+ optional video meeting). |
| PATCH | `/api/v1/lessons/{id}` | 🧑‍🏫 | Reschedule / edit / cancel. |
| POST | `/api/v1/lessons/{id}/join` | member | Authorize + return video join info (see below). |
| POST | `/api/v1/lessons/{id}/end` | 🧑‍🏫 | Mark ended (and end the room if provider supports). |

## Chat

REST for history and channel listing; live delivery over the WebSocket described in
[realtime and video](realtime-and-video.md#chat).

| Method | Path | Who | Purpose |
|--------|------|-----|---------|
| GET | `/api/v1/channels` | 👤 | My channels (course channels + DMs). |
| GET | `/api/v1/channels/{id}/messages` | member | Message history (keyset pagination). |
| POST | `/api/v1/channels/{id}/messages` | member | Send a message (also broadcast over WS). |
| POST | `/api/v1/channels/direct` | 👤 | Open/get a DM channel with another user. |
| PATCH | `/api/v1/messages/{id}` | author | Edit own message. |
| DELETE | `/api/v1/messages/{id}` | author / 🧑‍🏫 | Soft-delete. |

## Homework — assignments

| Method | Path | Who | Purpose |
|--------|------|-----|---------|
| GET | `/api/v1/courses/{id}/assignments` | member | List (students see only published). |
| POST | `/api/v1/courses/{id}/assignments` | 🧑‍🏫 | Create (draft or published), with prompt attachments. |
| GET | `/api/v1/assignments/{id}` | member | Detail + my submission status. |
| PATCH | `/api/v1/assignments/{id}` | 🧑‍🏫 | Edit / publish / set due date. |
| DELETE | `/api/v1/assignments/{id}` | 🧑‍🏫 | Delete (if no graded submissions). |

## Homework — submissions & grading

| Method | Path | Who | Purpose |
|--------|------|-----|---------|
| GET | `/api/v1/assignments/{id}/submissions` | 🧑‍🏫 | All submissions for grading. |
| GET | `/api/v1/assignments/{id}/submission` | 🎓 | My own submission. |
| PUT | `/api/v1/assignments/{id}/submission` | 🎓 | Create/update my submission (text + attachments). |
| POST | `/api/v1/assignments/{id}/submission/submit` | 🎓 | Mark submitted (locks until returned). |
| POST | `/api/v1/submissions/{id}/grade` | 🧑‍🏫 | Grade: points + feedback → status `graded`. |
| POST | `/api/v1/submissions/{id}/return` | 🧑‍🏫 | Return for revision → status `returned`. |

## Files / attachments

| Method | Path | Who | Purpose |
|--------|------|-----|---------|
| POST | `/api/v1/uploads` | 👤 | Upload a file (multipart) → `{id, filename, content_type, size_bytes}`. Validated (size, type). |
| GET | `/api/v1/attachments/{id}` | authorized | Download (authorized by the owning resource). |

Uploads are a two-step flow: upload the file to get an attachment id, then reference that id when
creating the assignment, submission or message. Until it is referenced, an attachment is owned by
its uploader alone and is deleted after a bounded time. The storage key is internal and never
returned. The download route re-checks authorization against the owning resource and never serves
a file from a client-built path.

## Conventions

- **Errors:** every non-2xx response has the body
  `{"error": {"code": "<stable_snake_case>", "message": "<human-readable>", "request_id": "<id>", "fields": [...]}}`.
  `code` values are an enum in `ovc.domain` and are what clients branch on; `message` is for
  display only; `fields` lists `{"path", "code", "message"}` for validation errors and is omitted
  otherwise. No stack traces, SQL or internal identifiers ever appear in an error.
- **Status codes:** 400 malformed request, 401 missing or expired token, 403 authenticated but not
  allowed on a resource whose existence is already known to the caller, 404 not found or not
  visible to the caller, 409 state conflict (for example grading a draft submission), 413 upload
  too large, 415 content type not allowed, 422 validation, 429 throttled.
- **Pagination:** keyset (`?limit=50&cursor=<opaque>`); responses include `next_cursor`, `null` at
  the end.
- **Identifiers** are UUIDs. **Timestamps** are ISO-8601 in UTC.
- **Idempotency:** `PUT /submission`, grade and return endpoints are safe to retry.
- **Request identity:** every response carries `X-Request-ID`; the same value appears in the error
  body and in diagnostics ([observability](observability.md#correlation)).
- Request and response schemas are the Pydantic models in `ovc.schemas`, published as OpenAPI at
  `/api/docs` when the service runs.
