# Threat model

Status: draft written 2026-10-20; pending line-by-line owner review.

The assets OVC protects, the actors who might threaten them, the trust boundaries, and the controls that answer each threat. Every change that touches a trust boundary maps itself to the threat identifiers here and updates this file in the same change ([AGENTS.md](../AGENTS.md#security)).

## Assets

- Accounts and sessions (credentials, refresh tokens).
- Student personal data, including data about minors: names, emails, avatars, presence.
- Academic records: submissions, attached files, grades and feedback.
- Course content: assignments, attached files, chat history.
- Live lessons: who can see and hear a class.
- Platform secrets: token signing keys, conferencing and storage credentials.

## Actors

- **Anonymous internet user** with no account.
- **Student** of one course trying to reach another course or another student's work.
- **Teacher** trying to reach another teacher's course.
- **Former member** removed from a course, or with a stolen or stale token.
- **Malicious uploader** using files or message content as an attack vehicle against other users or the server.

## Trust boundaries

1. Browser ↔ backend (REST and WebSocket): all client input is untrusted.
2. Backend ↔ conferencing server: the backend is the only party that may assign moderator rights.
3. Backend ↔ file storage: storage keys are internal; clients never address storage directly.
4. Browser ↔ conferencing server: media flows outside OVC; access depends on room secrecy and provider tokens.

## Threats and controls

| ID | Threat | Controls |
|---|---|---|
| T-1 | Credential stuffing or brute-force login | Argon2id hashing, login throttling, generic failure messages |
| T-2 | Stolen or replayed refresh token | Rotation on every refresh, reuse detection revokes the session, tokens never in URLs or logs |
| T-3 | Horizontal access: a student reads another student's submission, or posts to a course they are not in | Ownership guards in shared dependencies on every route and frame; 404 for invisible resources; integration tests for each denial |
| T-4 | Vertical access: a student performs teacher actions, or a teacher acts on another teacher's course | Role and course-ownership guards; tests per teacher-only route |
| T-5 | Uninvited participant joins a lesson | Unguessable room names, join authorization by membership, backend-signed provider tokens in production |
| T-6 | Malicious file upload (oversized, disguised type, path traversal) | Streaming size limit, content-based type allowlist, random storage keys, downloads authorized by owner, `nosniff` and attachment disposition |
| T-7 | Script injection through messages, names, file names, instructions or feedback | Rendered as text; any rich-text rendering sanitized |
| T-8 | Cross-site request forgery against cookie-held credentials | Bearer tokens for API calls; if the refresh token moves to a cookie, `SameSite=Strict`, `HttpOnly`, `Secure` and a CSRF check on the refresh route |
| T-9 | Personal data leaks to logs, URLs or third parties | Safe-content rules in the [observability specification](../knowledge/specs/observability.md#safe-content); the provider receives only display names |
| T-10 | Secret disclosure | Settings from the environment, `.env` ignored and unreadable by agents, no secrets in diagnostics |
| T-11 | Resource exhaustion through connections, uploads or chat volume | Bounded uploads, WebSocket authentication timeout, per-user message rate limits, bounded pagination |

## Out of scope

- The security of the conferencing deployment itself (its own hardening, recording storage).
- Compromise of the host or database by an operator.
- Academic integrity concerns such as plagiarism.
