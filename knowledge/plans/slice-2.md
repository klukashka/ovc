# Slice 2 implementation plan

The approved plan for [roadmap slice 2](../../docs/roadmap.md#slice-2-accounts-sessions-and-email), written on 2026-10-21 by a planning agent from the specifications, then amended after a standard and a security planning review and the owner's decisions. Revision 1 is the draft; revision 2 (the amendments) overrides it where they differ. The identifiers D1 to D22, Q1 to Q10, the changes C0 to F5 and the amendments 1 to 34 are defined in this file.

## 1. Problem, outcome and scope

**Problem.** OVC has frames but no accounts. All pages open anonymously, and shared pages take their frame role from the router's link state. No business table, repository, sqlc setup, outbox, mail, storage adapter or `ovc-admin` exists. The worker's `Tick` does nothing except log at DEBUG.

**Outcome.** A person can do all of the following end to end through the real API, worker, PostgreSQL, SeaweedFS and the production frontend build:

- sign up as a tutor or a student, giving consent, language and browser timezone;
- receive the confirmation email through the outbox, the worker and the mail adapter, and confirm it, including the squatting-safe "set a new password" path;
- sign in (throttled), stay signed in through rotating refresh cookies, sign out, and sign out everywhere;
- reset and change their password;
- edit profile and preferences, and upload an avatar;
- delete their account, which is purged after 30 days.

The operator can list, block, unblock and restore users and set tutor quotas with `ovc-admin`. Route guards replace the interim link-state frame role.

**In scope:**

- FR-1.1: sign-up for tutors and for students without a code; consent recorded.
- FR-1.4, FR-1.5, FR-1.6 (bio and subjects included, avatar included), FR-1.7 (the account's own data), FR-1.8.
- FR-16.1, except closing live connections.
- FR-17.1 and FR-17.2, for every page and email that exists.
- From FR-13.2, the email events `email.confirmation`, `password.reset` and `signup.duplicate_attempt`.
- The outbox (lanes `email` and `storage_delete`), the dispatcher, the scheduler with the job-run table, and the jobs listed in section 5.
- The mail adapters (SMTP and database sink), the storage adapters (S3 and the in-memory fake for contract tests), the clock and random sources, and readiness with storage.
- The slice 1 carried-over items listed in section 7.

**Deferred to slice 3:**

- invitations (sign-up through an invitation, reserved invitations, the `/invite/{token}` page);
- tutor codes, including the `code` column, `pending_tutor_code`, `GET /tutor/code` and the code field on student sign-up;
- join requests and relationships; FR-1.3 role mismatch;
- the deletion effects on relationships, groups, cards and `tutor.deleted` emails;
- the `email_not_verified` gate exercised on a real route (slice 2 has no route that is not marked unverified ok);
- the pending-link outcome in verify and reset, which always returns `null` in slice 2.

**Deferred to slice 4:**

- the WebSocket gateway, the broadcaster revocation signal, and closing sockets with `4401` on logout, blocking or deletion;
- email preferences (`/users/me/email-preferences`, `/settings/notifications`), unsubscribe tokens and pages, and `OVC_UNSUBSCRIBE_SIGNING_SECRET`;
- in-app notification rows;
- the NOTIFY wake of the dispatcher, if decision D14 is accepted.

**Deferred to later slices:**

- `/tutor/settings` and the Teaching settings page: slices 3, 6 and 7;
- the general upload pipeline (S3 multipart, office containers, quota charges, the `chat`, `answer` and other contexts): slice 6;
- signed storage URLs and their CSP host: slice 6 or 7 (depends on D12);
- the first-run checklist assertions in pilot scenario 1: slice 10;
- `OVC_FIELD_ENCRYPTION_KEY`: slice 3;
- bounce handling: deferred to the SMTP provider choice (conflict 8).

## 2. Verification tier and security trigger

- **Tier:**
  - the plan itself is a slice plan: one standard reviewer before work starts;
  - each change below is a standard-tier change: one standard reviewer each, plus a security reviewer where the trigger is met;
  - once every change has merged, the slice tier runs the full component-by-profile matrix over the diff since d3f2f01.
- **Security trigger: met.** The plan therefore also gets a separate security-profile reviewer before approval. Reasons, all named examples in AGENTS.md:
  - authentication, sessions and tokens in links (confirmation, reset);
  - the refresh cookie and the CSRF origin check;
  - rate limits;
  - email content and the SMTP channel;
  - uploads and public avatar downloads;
  - purge and deletion;
  - `ovc-admin`;
  - the end-to-end test control surface (mail sink route, tick contract);
  - new dependencies;
  - redaction of new secrets;
  - a change to AGENTS.md (the sqlc annotation rule).
- **Threats touched:** T-1, T-2, T-3, T-4, T-13, T-15, T-18, T-19, T-20, T-22, T-23, T-24, T-27, T-28, T-31, T-34, T-35, T-36, T-37, T-39, T-41.
- **Per change** (section 10): every change except C2b and F2 meets the trigger and gets a security reviewer.

## 3. Decisions for the owner

The library facts below were not checked against the registries when the plan was written. Before adoption, re-check each one against the registry and record it in `knowledge/research/backend-libraries-<date>.md`, with the date checked, for:

- the chosen version and its publish time from `proxy.golang.org/<module>/@v/<version>.info` (it must be on or before 2026-10-07, which is two weeks before today, 2026-10-21; Go has no pnpm-style age gate, so this is checked by hand);
- the licence;
- the last release and commit;
- open OSV advisories;
- compatibility with Go 1.27.

**D1. Password hashing library.**

- (A, recommended) `golang.org/x/crypto/argon2` (BSD-3, maintained by the Go team), with OVC's own PHC-string encode, parse and verify, and rehash on login when the parameters change.
- (B) `github.com/alexedwards/argon2id` (MIT; a thin wrapper with low release activity).
- (C) `github.com/matthewhartstonge/argon2`.
- Trade-off: A adds no third-party code but needs about 60 lines of encoding, which get a unit test. B saves those lines but adds a small, rarely updated dependency.
- Re-check the version, and OSV GO-2026-5932 (deprecated `openpgp` in x/crypto). x/crypto will enter `backend/go.mod`, and the backend's OSV scan runs with `--no-call-analysis=go`, so expect this finding. It needs a `backend/osv-scanner.toml` entry with a reason and an `ignoreUntil` date.
- **Parameters:**
  - (A, recommended) OWASP m=19 MiB, t=2, p=1, with a process-wide limiter of at most GOMAXPROCS concurrent hashes. This bounds memory on the 4 GB reference host (T-20) and keeps NFR-2.1 write latency.
  - (B) RFC 9106 m=64 MiB, t=3, p=4: stronger, but about 3 to 4 times the cost per hash.

**D2. Access-token approach.**

- (A, recommended) `github.com/golang-jwt/jwt/v5` (MIT, maintained), HS256 only:
  - checked with `WithValidMethods([HS256])`, issuer and audience, and `WithTimeFunc` bound to the injected clock;
  - claims `sub`, `sid`, `iat`, `exp`;
  - the same library verifies the BigBlueButton recording-ready HS256 JWT in slice 7 (T-12).
- (B) `github.com/go-jose/go-jose/v4` (Apache-2.0): broader and heavier.
- (C) A standard-library-only HMAC token. It adds no dependency, but the token handling is hand-written, and dropping "JWT" changes the stack row that ADR-0012 names, so it needs a new decision record.
- Note: every request already does a session lookup, so the JWT saves no database access; it only binds `sid` and expiry cryptographically.

**D3. SMTP client.**

- (A, recommended) `github.com/emersion/go-smtp` (MIT):
  - its client handles STARTTLS or implicit TLS, the 30 s command and submission timeouts, and connection reuse;
  - its server package is used only in tests, as the in-process SMTP peer for the adapter's contract tests;
  - MIME is composed by OVC with `mime/multipart` and `mime.BEncoding`, so OVC controls `Message-ID`, RFC 2047 and CR/LF stripping;
  - re-check its `go-sasl` dependency.
- (B) Standard-library `net/smtp`, which is frozen, plus a hand-written in-process test SMTP server: no dependency, more test code.
- (C) `github.com/wneessen/go-mail` (MIT): high-level and context-aware, but it builds MIME itself, so OVC has less control over headers.
- DKIM: (A, recommended) the provider signs; (B) OVC signs with `emersion/go-msgauth`. This must be confirmed when the provider is chosen.

**D4. S3 client.**

- (A, recommended) `github.com/aws/aws-sdk-go-v2/service/s3`, plus `aws` and `credentials` (Apache-2.0, maintained by AWS, already used in the SeaweedFS probe). Constraints:
  - build `aws.Config` by hand, with static credentials from settings; never import `config`, which reads `AWS_*` environment variables and `~/.aws` files (an environment read outside `internal/core`, T-19);
  - use path-style addressing;
  - set `RequestChecksumCalculation` and `ResponseChecksumValidation` to `WhenRequired`, because the CRC32 defaults break many S3-compatible providers.
- (B) `github.com/minio/minio-go/v7` (Apache-2.0, smaller). Its maintenance after the MinIO server repository was archived in April 2026 must be re-checked; that is the risk.
- AWS releases ship almost weekly, so pin a release at least 14 days old.

**D5. sqlc installation.**

- (A, recommended) Pin `github.com/sqlc-dev/sqlc/cmd/sqlc` (MIT) as a Go `tool` in `tools/go.mod`, as the development guide already says. The build uses cgo for `pg_query_go`, the first build is slow, and the tools module grows, which adds more OSV triage.
- (B) A release binary pinned by checksum, as with lychee.
- Re-check that the sqlc parser (PostgreSQL 17 grammar) accepts everything the migrations use. Keep the schema to syntax PostgreSQL 17 understands.

**D6. Email templates and plurals.**

- (A, recommended) `html/template` and `text/template` files embedded with `//go:embed`, a Go message catalogue per language, and plural categories from `golang.org/x/text/feature/plural` (x/text is already an indirect dependency).
- (B) `github.com/nicksnyder/go-i18n/v2` (MIT).
- (C) An own ICU subset.

**D7. Error-code enum scope.**

- (A, recommended) Define every code in the API specification's table in `internal/domain` now, plus `internal`, plus the new slice 2 codes. Publish them as the OpenAPI enum and translate all of them in the frontend once.
- (B) Only the codes emitted today, extended each slice.

**D8. Codes for the new refusals.**

- **Failed sign-in** (wrong password, unknown email, deleted account): (A, recommended) a new `401 invalid_credentials`; (B) `401 unauthenticated`, which the client cannot tell apart from an expired session.
- **Invalid, expired, used or superseded confirmation or reset token:** (A, recommended) a new `422 token_invalid` with one identical response for every cause; (B) `404 not_found`.
- **Wrong current password on change or delete:** (A, recommended) `422 validation_failed` with field `current_password` and code `incorrect`; (B) `403 forbidden`. A 401 would trigger the client's refresh path, so it is not an option.
- **Blocked account with the correct password:** `403 account_blocked`. Only someone who knows the password learns the state.
- **Refresh with a wrong or missing `Origin`:** `403 forbidden`.

**D9. Resend after sign-up without a session.** `/auth/register` returns no session, `/auth/verify-email/resend` requires a user, and the page shows a resend button (conflict 3).

- (A, recommended) `POST /auth/verify-email/resend` also accepts a public body `{email}` and always answers `202`. It sends only for an existing unverified account, under the per-account sending limits and the per-IP limit of 10 per hour.
- (B) The "Check your email" page offers "Sign in" instead; resend appears only when signed in.
- (C) Auto-sign-in after register. Rejected: a failed login after a duplicate sign-up reveals that the email exists (T-3).

**D10. Contract for the test-control tick and the worker clock** (carried over from slice 1).

- (A, recommended) Mount `/__test__/clock` and `/__test__/tick` only when `OVC_CLOCK=test`; mount `/__test__/mail` whenever `OVC_TEST_CONTROL=1`.
- (B) Tick at system time when `OVC_CLOCK=system`. This races the running worker loops. `SKIP LOCKED` keeps it safe, but results become nondeterministic.

Record the choice in `docs/development.md` and in the notifications specification.

**D11. Test-only tables** (conflict 2).

- (A, recommended) Keep `test_clock` and the new `mail_sink` in the normal migration set. Amend data-model.md to say these tables exist everywhere but are written only by test adapters. Add a settings rule that `OVC_MAIL_BACKEND=sink` requires a database name matching `_test` or `_dev`, so a production database never accumulates emails that contain tokens.
- (B) A separate goose source with its own version table, applied by `api migrate` only with `OVC_TEST_CONTROL=1` and by `testdb`. `00001_test_clock` moves there, with a drop in the main set. This matches the current text but costs more.

**D12. How avatars are served.**

- (A, recommended) `GET /avatars/{id}` streams the object through the API with:
  - `Content-Type` from detection, `nosniff`, `Content-Disposition: inline`;
  - `Cache-Control: public, max-age=86400, immutable`.

  It serves only attachments that are currently referenced as an avatar; anything else is `404`. The CSP stays `'self'`.
- (B) Answer with a 5-minute signed storage URL. This adds the storage host to `img-src` in slice 2, breaks caching, and conflicts with the "API never redirects" rule.

**D13. Client IP for per-IP limits** (T-20 requires the proxy's address, never a client header; the configuration table has no setting for it).

- (A, recommended) A new `OVC_TRUSTED_PROXY_CIDRS`, empty by default. When the peer address is inside it, take the right-most `X-Forwarded-For` entry; otherwise use `RemoteAddr`.
- (B) Always use `RemoteAddr`. Behind a proxy, every user then shares one counter.

**D14. Dispatcher wake.**

- (A, recommended) Poll every second in slice 2, and add the `NOTIFY` wake with slice 4's listener connection, amending the notifications specification.
- (B) Build a `LISTEN` adapter now.

**D15. Tutor code at student sign-up.**

- (A, recommended) The field is absent from the slice 2 schema and arrives in slice 3.
- (B) Accept it and store it unevaluated.

**D16. End-to-end deployments.**

- (A, recommended) As the development guide requires from the first pilot suite: each Playwright worker gets its own database, bucket, API, worker and `vite preview` on derived ports, with `OVC_TEST_CONTROL=1`, `OVC_CLOCK=test` and `OVC_PUBLIC_URL` set to that worker's preview origin.
- (B) Keep one shared deployment for now (contradicts the development guide).

**D17. `__Host-` and `Secure` cookies over `http://127.0.0.1`** (WebKit has historically refused `Secure` cookies on loopback HTTP).

- (A, recommended) Probe this in F0. If any browser drops the cookie, serve preview over HTTPS with a per-run self-signed certificate and `ignoreHTTPSErrors`.
- (B) Run the account suites in fewer browsers. That reduces coverage and needs explicit owner approval.

**D18. Playwright traces** (carried over).

- (A, recommended) Keep `retain-on-failure`. Traces contain only generated test credentials, stay in the ignored `frontend/test-results/`, and are never shared; document this in the development guide and T-19.
- (B) Turn traces off.

**D19. Refresh races between tabs.** Concurrent refreshes would trigger reuse detection and revoke the session.

- (A, recommended) Serialise refresh per browser with the Web Locks API (`navigator.locks.request('ovc-refresh')`). A waiting tab then sends the already-rotated cookie.
- (B) A server grace window for the previous token. This weakens T-2.

**D20. Consent version.**

- (A, recommended) A constant in `internal/domain`, bumped by a code change when the privacy policy changes.
- (B) A settings variable.

**D21. Hashing rate-limit subjects.**

- (A, recommended) Store SHA-256 of the normalised email (and of the IP) as `subject`, so no email sits in counters (T-18).
- (B) Store the plain values.

**D22. Level of the `auth.refresh.rotated` event.**

- (A, recommended) DEBUG. It is periodic, like polling, which the observability specification keeps below INFO.
- (B) INFO.

## 4. Data model, migrations and sqlc

**Migration `00002_accounts.sql`.** Goose markers only; no comments. All tables in one change (C3), so parallel branches never clash on goose version numbers.

- **Extension:** `CREATE EXTENSION IF NOT EXISTS citext`. The migrate role needs the right to create it; check this with the provider.
- **`users`:**
  - columns: `id uuid pk`, `role text check in (tutor, student)`, `state text check in (unverified, active, blocked, deleted)`, `state_before_block text null check in (unverified, active)`, `email citext unique not null check length<=254`, `email_verified_at`, `password_hash text not null`, `name text check 1..100 chars`, `avatar_attachment_id uuid null`, `language check in (ru, en)`, `timezone text not null`, `theme check in (light, dark, system)`, `consent_version text not null`, `consent_at`, `deleted_at`, `created_at`, `updated_at`;
  - check: `state_before_block` is not null if and only if `state = 'blocked'`;
  - partial index `(deleted_at)` where `state = 'deleted'`;
  - partial index `(created_at)` where `state = 'unverified'`;
  - columns added by later slices: `pending_tutor_code` and `reserved_invitation_id` (slice 3), `online_until`, `last_seen_at`, `digest_pending_since` and `last_digest_at` (slices 4 and 6), and `email_undeliverable_at` (with the provider choice). Data-model.md gets a note saying which slice adds each column.
- **`tutor_profiles`:** `user_id pk fk users on delete cascade`, `bio text check <=1000`, `subjects text[] check cardinality<=10`, `recording_retention_days check in (7,30,90,180,365) default 90`, `file_quota_bytes`, `recording_quota_bytes`, `file_usage_bytes default 0`, `recording_usage_bytes default 0`, `onboarding_dismissed_at`. `code` is added in slice 3.
- **`sessions`:** `id uuid pk` (the `sid` claim), `user_id fk`, `refresh_token_hash bytea unique`, `previous_token_hash bytea null`, `expires_at`, `revoked_at`, `last_used_at`, `user_agent_summary text check <=120`, `created_at`. Indexes: `(user_id)`, and `(previous_token_hash)` for reuse lookup; the data model does not list the latter, so add it.
- **`email_requests`:** `id`, `user_id fk`, `purpose check in (verify_email, reset_password)`, `created_at`; index `(user_id, purpose, created_at desc)`.
- **`email_tokens`:** `id`, `request_id fk`, `user_id fk`, `purpose`, `token_hash bytea unique`, `expires_at`, `used_at`, `created_at`; index `(user_id, purpose)`.
- **`rate_limit_counters`:** `purpose`, `subject`, `window_start`, `count`; primary key on the three; index `(window_start)`.
- **`attachments`:** the files-spec columns. `context` is constrained to the six contexts, but only `avatar` is accepted in slice 2. `scope_kind`, `scope_id` and `scope_question_id` are nullable. `quota_tutor_id` is null for avatars. `storage_key` is unique. `state` is `pending`, `bound` or `deleted`. `expires_at` has a partial index where `state = 'pending'`.
- **`attachment_references`:** `(attachment_id, ref_kind, ref_id, question_id)` primary key; index `(ref_kind, ref_id)`. `question_id` is a nullable column in the key; use a uuid sentinel or a unique index with `NULLS NOT DISTINCT`, which PostgreSQL 15+ supports; confirm sqlc parses it. Quota charges arrive in slice 6.
- **`outbox`:**
  - `id`, `lane check in (email, provider_call, recording_copy, storage_delete)`, `priority smallint` (0 high, 1 normal, so ordering is explicit rather than alphabetical), `kind`, `payload jsonb`, `idempotency_key text unique`, `status`, `attempts`, `next_attempt_at`, `lease_token uuid`, `lease_expires_at`, `last_error_code`, `done_reason check in (sent, obsolete)`, `done_at`, `created_at`;
  - partial indexes `(lane, priority, next_attempt_at)` where pending, `(lease_expires_at)` where in progress, and `(done_at)` where `status in ('done', 'dead')`.
- **`job_runs`:** `job text pk`, `last_run_at`, `last_result`.
- **`mail_sink`** (adapter-owned, D11): `id`, `recipient`, `subject`, `text_body`, `html_body`, `links text[]`, `message_id`, `sent_at`; index `(recipient, sent_at)`.

**Mapping to data-model.md:**

- User, Tutor profile, Session, Email token and request, Rate limit counter, Attachment and reference, Outbox and Job run are created in slice 2.
- Notification and Email preference come in slice 4.
- Purge rows are implemented for every table above (section 5); later slices extend purge with their own tables.

**sqlc:**

- `backend/sqlc.yaml`:
  - `engine: postgresql`, `schema: migrations`, `queries: queries`;
  - generated Go package `dbgen` at `internal/dbgen`, with `sql_package: pgx/v5`, `emit_pointers_for_null_types: true`, `emit_enum_valid_methods: false`;
  - overrides: `citext` to `string`, `uuid` to `github.com/jackc/pgx/v5/pgtype.UUID` or `[uuid]byte`, and domain named types (`domain.Role`, `domain.AccountState` and others) through column overrides, which `internal/dbgen` may import.
- Queries live in `backend/queries/*.sql`, one file per aggregate: `users.sql`, `sessions.sql`, `email_tokens.sql`, `rate_limits.sql`, `attachments.sql`, `outbox.sql`, `jobs.sql`. Use `sqlc.narg` and `sqlc.slice` for filters in `ovc-admin users list`.
- The check `go -C tools tool sqlc -f ../backend/sqlc.yaml diff` exits 0. Register it in `docs/development.md` and in `knowledge/reviews/ai-development-setup.md`.
- `internal/dbgen` is already skipped by the conventions test. The SQL comment test must accept sqlc annotations (conflict 1).

## 5. Backend work by package and layer

Each new package uses an existing row of the package-layer table: `internal/adapters/*`, `internal/services/*`, `internal/repositories`, `internal/dbgen`, `internal/admin`, `cmd/*`. Add new rows for `internal/teststorage` (test-only) and `internal/testmail` (test-only), or fold both into `internal/testdb`.

**`internal/domain`:**

- named types: `UserID`, `SessionID`, `Role`, `AccountState`, `Language`, `Theme`, `EmailPurpose`, `OutboxLane`, `OutboxStatus`, `OutboxPriority`, `DoneReason`, `AttachmentContext`, `AttachmentState`, `RefKind`, `NotificationEvent` (`email.confirmation`, `password.reset`, `signup.duplicate_attempt`) and `ConsentVersion` (D20);
- pure rules, each with a unit test:
  - the fixed-window throttle (counts, window start, `Retry-After`);
  - the account state machine (block from unverified or active and remember it; deleted cannot be blocked; unblock returns to the remembered state; restore from deleted to active or unverified; delete);
  - backoff with jitter (30 s to 1 h, eight attempts);
  - password length 10 to 128, counted in runes;
- error codes:
  - `AllErrorCodes()` (D7) with `Status()` and `Message()` for each new code;
  - a conventions test parses the `ErrorCode` constants and requires `AllErrorCodes` to list exactly them.

**`internal/core`:**

- **Settings.** Validation becomes per process: `api`, `worker`, and a new `admin`, which needs only the database and logging. New variables, all in `backend/.env.example` and the configuration table, with secrets held in `Secret`:
  - `OVC_PUBLIC_URL`: absolute URL; its origin is the exact refresh `Origin` and the base of email links;
  - `OVC_JWT_SECRET` (Secret, at least 32 bytes);
  - `OVC_ACCESS_TOKEN_TTL_SECONDS` (default 900) and `OVC_REFRESH_TOKEN_TTL_SECONDS` (default 2592000);
  - `OVC_STORAGE_ENDPOINT`, `OVC_STORAGE_REGION` (new, conflict 6), `OVC_STORAGE_BUCKET`, `OVC_STORAGE_ACCESS_KEY` (Secret) and `OVC_STORAGE_SECRET_KEY` (Secret);
  - `OVC_STORAGE_BACKEND`: only `s3`; no `memory` value (conflict 6);
  - `OVC_SMTP_HOST`, `OVC_SMTP_PORT`, `OVC_SMTP_TLS` (`starttls` or `implicit`; new), `OVC_SMTP_USERNAME`, `OVC_SMTP_PASSWORD` (Secret) and `OVC_MAIL_FROM`;
  - `OVC_MAIL_BACKEND`: `smtp` or `sink`; `sink` needs a `_test` or `_dev` database (D11), and `OVC_TEST_CONTROL=1` still needs `sink`;
  - `OVC_DEFAULT_FILE_QUOTA_BYTES` and `OVC_DEFAULT_RECORDING_QUOTA_BYTES` (default 5 GB and 50 GB);
  - `OVC_TRUSTED_PROXY_CIDRS` (D13).
- `OVC_CORS_ORIGINS` is not added (conflict 5).
- `LogStarted` summarises the new settings without secrets.
- Security primitives:
  - `RandomSource`: an `io.Reader`, `crypto/rand` in production, a deterministic reader in unit tests;
  - token minting: 32 bytes, base64url;
  - SHA-256 hashing;
  - the Argon2id hasher (D1) with a concurrency limiter and a dummy-hash verify for unknown emails, so timing does not reveal accounts;
  - the JWT issuer and verifier (D2).
- Email i18n catalogue (D6), if the owner keeps the module map's "i18n of emails" in core (conflict 10).

**`internal/adapters`:**

- **`clock`.** Unchanged interface. Services receive `clock.Source`; the API keeps `clock.System{}.Now` for transport deadlines and access-log timing (T-20).
- **`mail`.** `Mailer.Send(ctx, Message) error`.
  - `Message` holds `To`, `Subject`, `Text`, `HTML`, `MessageID` and `Headers`.
  - `SMTP` (D3) uses a 30 s timeout and a reused connection with `RSET` between messages.
  - `Sink` writes to `mail_sink` through the `core` pool and extracts `https?://` links.
  - Header building strips CR and LF from user text and encodes it with RFC 2047. `Message-ID` is `<sha256(idempotency key[, token id])@<public host>>`.
- **`storage`.** `Store` with `Put(ctx, key, reader, size, contentType)`, `Open(ctx, key)` (a stream), `Delete`, `Ping` (a bucket HEAD within 1 s).
  - `S3` (D4).
  - `Memory`, for contract and unit tests only.
  - `PresignGet` is added in slice 6.

**`internal/repositories`:**

- `Users`, `TutorProfiles`, `Sessions`, `EmailTokens`, `RateLimits`, `Attachments`, `Outbox`, `JobRuns`, each wrapping `dbgen`;
- a transaction helper (`WithTx(ctx, func(Queries) error)`);
- `Outbox.Insert` uses `ON CONFLICT (idempotency_key) DO NOTHING`;
- `Outbox.Claim(lane, limit, now, leaseToken, leaseUntil)` uses `FOR UPDATE SKIP LOCKED` ordered by `priority, next_attempt_at`;
- `Complete` and `Retry` are conditional on `lease_token`;
- `ReclaimExpired`;
- the purge statements run in bounded batches.

**`internal/services/accounts`:**

- **`Register`:**
  - validates the input;
  - hashes the password even for a duplicate email;
  - for a new email, in one transaction: inserts the user (unverified), inserts the tutor profile with the default quotas, inserts an email request (`verify_email`) and an outbox row (`email.confirmation`, high priority, key `(event, request id)`);
  - for a duplicate email: inserts an outbox row `signup.duplicate_attempt` with key `(event, account, UTC date)` and `ON CONFLICT DO NOTHING`;
  - enforces the per-IP limit of 10 per hour;
  - returns the same result for a new and a duplicate email.
- **`Login`:** per-email (more than 5 failures in 15 min) and per-IP (more than 20 failures) throttles are checked before verifying the password; dummy-hash verify for an unknown email; refuses `deleted` as invalid credentials and `blocked` as `account_blocked`; creates a session; rehashes the password if its parameters are stale.
- **`Refresh`:** exact `Origin` equality; hash lookup; rotation; reuse of `previous_token_hash` revokes the session and logs `auth.refresh.reuse_detected` at WARN; checks expiry against the injected clock.
- **`Authenticate(token)`:** one indexed lookup of the session and user by `sid`. The session must not be revoked or expired, and the account must be active or unverified. Results: `token_expired`, `unauthenticated` or `account_blocked`.
- **`Logout` and `LogoutAll`.**
- **`ConfirmEmail(token, optional caller session, optional new password)`:**
  - from a signed-in session of the token's own account: activate the account and revoke every other session;
  - otherwise a new password is required: set it, activate, revoke all sessions, and create a new session.
- **`ResendConfirmation`** (authenticated, or public `{email}` per D9) and **`ForgotPassword`:** both always succeed outwardly. Sending limits are counted per account (combined across both purposes): 1 per 60 s, 5 per hour, 10 per day. Exceeding them is silent for `forgot` (conflict 18); the per-IP limit gives 429.
- **`ResetPassword`:** checks the token; sets the password; confirms the email if unverified; revokes all sessions; does not sign in.
- **`ChangePassword`:** checks the current password and revokes every other session.
- **`UpdateProfile`:** name, avatar (bind and unbind through the files service), language, timezone (checked with `time.LoadLocation` against the embedded `time/tzdata`), theme; tutor-only bio and subjects (a student sending them gets `validation_failed` with code `not_allowed`).
- **`DeleteAccount(password)`:** state `deleted`, `deleted_at = now`, all sessions revoked.
- **Operator operations:** `ListUsers`, `Block`, `Unblock`, `Restore`, `SetQuota`, each using the state machine and row locks.
- **`PurgeAccount(id)`:** re-checks the state under `FOR UPDATE`, then deletes sessions, email requests and tokens, the tutor profile, avatar references (writing `storage_delete` outbox rows when a reference was the last one) and the user row.

**`internal/services/notifications`:**

- builds an email per outbox row;
- dispatch-time checks: a blocked or deleted recipient; for confirmation, "already confirmed" or a newer request exists; for reset, a newer request exists;
- token minting at dispatch: commit a new `email_tokens` row, invalidating earlier unused tokens of the same purpose, before calling `Send`;
- renders the templates for `email.confirmation`, `password.reset` and `signup.duplicate_attempt` in the recipient's language and timezone, as HTML and text;
- links: `<public URL>/verify-email?token=…` and `<public URL>/reset-password?token=…`;
- payloads contain identifiers only.

**`internal/services/files`:**

- `UploadAvatar(ctx, user, part)`:
  - reads at most 5 MiB + 1 byte from the streamed multipart part into a bounded buffer (a 5 MiB file fits in one S3 part; memory is bounded by 5 MiB × 32 concurrent uploads, below the specification's 288 MiB);
  - detects JPEG, PNG or WebP from magic bytes and requires the extension to agree, otherwise `unsupported_media_type`;
  - stores the object under a random key and creates a `pending` attachment that expires 24 h later;
  - enforces concurrency: 4 per user and 32 per instance, then `rate_limited`;
- `Bind` and `Unbind` the avatar reference;
- `OpenAvatar(id)`: only an attachment with a live `avatar` reference;
- `ExpirePending`.

**`internal/api`:**

- An operation helper declares each operation's audience: `public`, `user` or `unverified ok`. A shared huma middleware authenticates bearer tokens through the accounts service. A test enumerates every registered operation and asserts that it declares an audience and that each non-public operation answers 401 without a token.
- Routes and their refusal codes:
  - `POST /api/v1/auth/register` → 202; refusals 422 `validation_failed` and 429 `rate_limited`.
  - `POST /auth/verify-email` with `{token}` or `{token, new_password}`; optional bearer → 200 with `{access_token, expires_in, outcome: null}`, the `Set-Cookie` refresh cookie when a session is created, or 200 with no new session for the signed-in path. Refusals: 422 `token_invalid`, and 422 `validation_failed` with field `new_password` code `required`.
  - `POST /auth/verify-email/resend` → 202; refusal 429.
  - `POST /auth/login` → 200 with `{access_token, expires_in}` and `Set-Cookie: __Host-ovc_refresh=…; Path=/; Secure; HttpOnly; SameSite=Strict; Max-Age=…`. Refusals: 401 `invalid_credentials`, 403 `account_blocked`, 429 `rate_limited` with `Retry-After`.
  - `POST /auth/refresh` → 200 with a rotated cookie, or 204 with no body when no cookie is sent. Refusals: 403 `forbidden` for a bad `Origin`; 401 `unauthenticated` for an unknown token or detected reuse, with the cookie cleared.
  - `POST /auth/logout` and `POST /auth/logout-all` → 204 with the cookie cleared.
  - `POST /auth/password/forgot` → 202; refusal 429.
  - `POST /auth/password/reset` → 204; refusals 422 `token_invalid` and 422 `validation_failed`.
  - `POST /auth/password/change` → 204; refusal 422 `validation_failed` (`current_password`: `incorrect`).
  - `GET /users/me`, `PATCH /users/me`.
  - `DELETE /users/me` with `{password}` → 204; refusal 422.
  - `POST /uploads?context=avatar` (unverified ok). A huma operation with a declared multipart schema reads the body with `humago.Unwrap` and `mime/multipart.Reader`; never `ParseMultipartForm`, which spools to disk. It raises `MaxBodyBytes` and sets an avatar-specific read deadline of 60 s. Refusals: 413 `payload_too_large`, 415 `unsupported_media_type`, 429 `rate_limited`, 422.
  - `GET /avatars/{id}` (public, D12) → 200 with the image, or 404.
- `/health/ready` adds `checks.storage`; the frontend `Readiness` component consumes it.
- `ErrorBody.Code` publishes the enum through a `huma.SchemaTransformer` in `internal/schemas`, built from `domain.AllErrorCodes()`.
- `Retry-After` is returned through the error type.

**`internal/worker`:**

- `Tick(now)`: one scheduler pass at `now`, then dispatch lane by lane until no due rows remain.
- Lanes in slice 2: `email` (concurrency 10) and `storage_delete` (concurrency 4).
- Lease 5 min; reclaim expired leases.
- Each effect runs under a system-clock context timeout of 30 s.
- Backoff uses the injected random source; a row becomes `dead` after 8 attempts, with an ERROR event.
- `Run` claims through the same code every second while `OVC_CLOCK=system`.
- Scheduler jobs, recorded in `job_runs` so a missed run catches up once:
  - hourly `accounts.cleanup_unverified`: unverified for 7 days → immediate purge;
  - hourly `accounts.purge`: deleted for 30 days;
  - hourly `ratelimit.cleanup`;
  - hourly `outbox.cleanup`: `done` older than 7 days, `dead` older than 30 days;
  - hourly `uploads.expire`.

  Each job runs bounded batches within a time budget per run.

**`internal/testcontrol`:**

- `POST /__test__/mail` with `{to}` returns `[{subject, text, html, links, sent_at}]` from the sink adapter, under the same loopback, JSON and 4 KiB guard as the existing routes;
- the clock and tick routes follow D10;
- the worker passed to it is wired with the real services and the sink.

**`cmd/api` and `cmd/worker`.** These compose the stores and the mailer from settings, including the shared clock under `OVC_CLOCK=test`, which reaches the services but never the transport deadlines.

**`cmd/ovc-admin` and `internal/admin`:**

- subcommands exactly as in the accounts specification;
- output goes to stdout through `fmt.Fprintln(os.Stdout, …)`, and messages for an unknown email, a wrong role or an impossible state change exit with code 1;
- the audit event `admin.command` carries the operator's `os/user.Current().Username`, the target user identifier, the subcommand and the outcome, never the email;
- the revocation-signal publish is a no-op until slice 4; the specification is noted accordingly.

**Observability events** (names added to the observability specification):

- `auth.signup.accepted`, `auth.email.confirmed`, `auth.login.succeeded`, `auth.login.failed` (with `reason`), `auth.login.throttled`;
- `auth.refresh.rotated` (level per D22), `auth.refresh.reuse_detected` (WARN);
- `auth.logout`, `auth.logout_all`, `auth.password.reset_requested`, `auth.password.reset`, `auth.password.changed`;
- `account.deleted`, `account.restored`, `account.blocked`, `account.unblocked`, `account.purged`;
- `admin.command`;
- `outbox.row.claimed`, `outbox.row.done` (sent or obsolete), `outbox.row.retried`, `outbox.lease.reclaimed`, `outbox.row.dead` (ERROR, with an error code);
- `job.run` (batch size, duration);
- `upload.accepted`, `upload.rejected` (reason code), `file.reference.added`, `file.reference.removed`, `file.deleted`.

Records never contain an email, name, password, token or cookie.

## 6. Frontend work

- **F1 lint rules first, before any token code.** Rules in `frontend/eslint.config.ts`, each proven by `scripts/eslint-rules.test.ts`:
  - refuse the `dangerouslySetInnerHTML` JSX attribute, assignment to `innerHTML` or `outerHTML`, `insertAdjacentHTML`, `document.write` and `writeln`, `document.cookie`, and `indexedDB`;
  - refuse `localStorage` and `sessionStorage` (identifier, `window.` and `globalThis.` forms) outside `src/lib/preferences/`;
  - move `window.localStorage` out of `main.tsx` into `lib/preferences/browser.ts`;
  - `public/theme-init.js` stays as it is, outside `src/`.

  A Playwright check after the account journeys asserts that `localStorage` keys are a subset of `{ovc.language, ovc.theme, ovc.sidebar}`, that `sessionStorage` is empty, and that `document.cookie` is `''`.
- **Session.**
  - `lib/api/session.ts` holds the access token in a closure, never in storage.
  - An openapi-fetch middleware adds `Authorization`.
  - Single-flight refresh, serialised across tabs with Web Locks (D19); it runs on 401 `token_expired`, retries the request once, and also refreshes proactively from `expires_in`.
  - At startup it calls `POST /auth/refresh`; a 204 means signed out and is not an error.
  - Then `GET /users/me` returns the role, the state and the preferences. The account's language and theme are applied and written to the preferences store.
  - Signing out calls `POST /auth/logout`, clears the TanStack Query cache and navigates to `/`.
  - `account_blocked` signs the user out with a message.
- **Route guards** in `routes/` and `features/auth`:
  - anonymous visitors on a private address go to `/?next=<path+search>`;
  - the wrong role shows Not found;
  - shared addresses use the signed-in role's frame;
  - a signed-in user at `/` goes to `/today` or `/home`;
  - an unverified student sees only the confirmation state, `/verify-email` without a token ("Check your email" with resend);
  - an unverified tutor sees a confirmation banner in the frame.

  Delete `frameRoleState` and `roleState` and the link-state `state={…}` props. Remove the "Interim behavior before sign-in exists" section from navigation.md.
- **`next` rule.** `safeNext(value, origin)` accepts a value only if it starts with exactly one `/`, is not `//` or `/\`, and `new URL(value, origin).origin === origin`; otherwise it is ignored. A unit test covers `//evil`, `/\evil`, `https://evil`, `/%2F%2Fevil`, `javascript:` and encoded backslashes.
- **Token pages** (`/verify-email`, `/reset-password`):
  - read the token once in a state initializer;
  - remove it immediately with `navigate({search: ''}, {replace: true})`, which performs `history.replaceState` through the router;
  - only then POST;
  - after a reload without a token, show "open the link again";
  - the verify page shows the password form when the API answers 422 `new_password`/`required`;
  - outcome messages are omitted until slice 3;
  - a component test proves the address bar no longer carries the token.
- **Pages:**
  - Sign in: generic failure message; throttle message "Too many attempts, try again in {minutes}" from `Retry-After`.
  - `/signup` role choice, then `/signup/tutor` and `/signup/student`: name, email, password with a strength hint, language preselected, browser timezone sent automatically, consent checkbox with a privacy-policy link (target is open question Q5).
  - "Check your email", with resend disabled for 60 s (D9).
  - `/forgot-password` and `/reset-password`.
  - Settings: `/settings/profile` (name, avatar upload with inline refusal messages, bio and subjects for tutors) and `/settings/account` (language, timezone list from `Intl.supportedValuesOf('timeZone')`, theme, change password, sign out everywhere, delete account behind a confirmation dialog that names the consequence and asks for the password).
- **`system` theme.** When the preference is `system`, a `matchMedia('(prefers-color-scheme: dark)')` change listener updates `data-theme`; it is removed when the preference changes.
- **Error codes.** `type ErrorCode = components['schemas']['ErrorBody']['code']`. `Resources` gains `errors: Record<ErrorCode, string>` in `ru` and `en`, so `pnpm typecheck` fails on any missing translation. An unknown code shows a generic message with the request identifier.
- **Tailwind class check (F2).** A `pnpm` script in `lint` or `build`:
  - collects static class tokens from `className` string literals with the TypeScript compiler API;
  - builds the CSS;
  - fails on any token that has no selector in `dist/assets/*.css`.

  A fixture test proves it fires. Dynamic class composition must stay literal.

## 7. Carried-over slice 1 items and how each is done

| Item | How |
|---|---|
| Route guards, the signed-in role replacing the link-state role, token pages, the `next` rule | Section 6, change F3 (guards and `next`) and F4 (token pages); navigation.md interim section removed |
| DOM-sink and Web Storage lint rules, plus the stored-keys smoke | F1, merged before F3 |
| Error codes published as an enum | C2b: `AllErrorCodes`, the schema transformer, the generated frontend union, the typed translation map |
| Whether tick requires `OVC_CLOCK=test` | D10 in C4; recorded in `docs/development.md` and notifications.md; tests prove the routes are absent under the other mode |
| Playwright traces | D18 in F0; documented in the development guide and T-19 |
| `system` theme following scheme changes | F5, with a Playwright test using `page.emulateMedia({colorScheme})` |
| Tailwind classes that produce no CSS | F2 |
| Test fixture error text and admin-URL loopback check (T-36) | C2a: `testdb` reports `core.ErrorKind(err)` and the database name, never `%v` of pgx errors; the admin URL host must be a loopback IP literal, `localhost` or a Unix socket, refused otherwise, as in `frontend/e2e/deployment.ts` |
| Transport deadlines on the system clock (T-20) | C5: `api.Options.Now` stays system; services get `clock.Source`. Test `TestTransportDeadlinesIgnoreTheSharedClock` sets the shared clock to 2000-01-01 and to 2100-01-01 and asserts that a request completes and an expired-token check follows the shared clock |

## 8. Tests

**Integration tests (Go).** Real `httptest` server, real PostgreSQL through `testdb`, a per-test SeaweedFS bucket through a new `teststorage` helper, `Tick(now)` with the shared clock, and the sink. Per route and job:

- **register:** identical status, body and headers for a new and a duplicate email; one duplicate notice per UTC day; none for a deleted account; validation (9 or 129 characters, no consent, a bad timezone or language); the 11th request from one IP in an hour gets 429 with `Retry-After`.
- **verify-email:** the signed-in path revokes the other sessions; the fresh-browser path requires a password, sets it, revokes every session and signs in; 24 h expiry by the clock; used and superseded tokens get `token_invalid`; the database holds no raw token outside `mail_sink`.
- **resend and forgot:** limits of 60 s, 5 per hour and 10 per day per account, combined across purposes; an active account gets no email; identical responses for unknown emails.
- **login:**
  - the cookie attributes are exactly `__Host-ovc_refresh`, Path `/`, `Secure`, `HttpOnly`, `SameSite=Strict`, no `Domain`;
  - the 6th failure in 15 min is throttled even with the right password, and the window resets by the clock;
  - the 21st failure per IP is throttled;
  - blocked and deleted accounts are refused;
  - the timing path is exercised (an unknown email still verifies a hash).
- **refresh:** rotation; reuse revokes the session and logs WARN; no cookie gives 204; an `Origin` that is wrong, missing, or a same-site subdomain gives 403; expiry after 30 days by the clock.
- **access token:** expired after 15 min gives `token_expired`; a revoked session, a tampered token, `alg: none` and RS256 give `unauthenticated`.
- **logout and logout-all.**
- **reset and change password:** each revokes the sessions it should; reset confirms an unverified email.
- **users/me:** GET and PATCH validation, including tutor-only fields.
- **avatars:**
  - 5 MiB + 1 gives 413 while streaming;
  - SVG, GIF and text give 415, and so does a mismatched extension;
  - binding works; public GET of a bound avatar works; an unbound or another user's pending id gives 404;
  - replacing an avatar deletes the old object after `Tick`;
  - pending uploads expire after 24 h.
- **delete account:** a wrong password gives 422; a deleted account is refused at sign-in; purge at 30 days removes the rows and the object; restore before purge works.
- **cleanup of unverified accounts** at 7 days.
- **dispatcher:**
  - claim and lease; retry backoff from the fake random source; dead after 8 attempts with an ERROR event; lease reclaim;
  - a late finisher with a stale lease token changes nothing;
  - `Tick` twice with the same `now` has no extra effect;
  - the obsolete reasons for each event.
- **email content:**
  - recipient language;
  - the `Message-ID` is stable across retries but differs per token;
  - a name containing CR or LF is stripped and RFC 2047-encoded in headers and HTML-escaped in the body;
  - links point only to `OVC_PUBLIC_URL`;
  - no token in any outbox payload.
- **test control:**
  - the mail route is absent without `OVC_TEST_CONTROL=1`;
  - clock and tick are absent unless `OVC_CLOCK=test` (D10);
  - the sink is refused against a database that is not `_test` or `_dev` (D11).
- **observability:** a captured-log assertion over the account flows finds no password, token, cookie, email or name, and finds the required event names.
- **ovc-admin process tests** (build the binary, run it with an explicit environment):
  - list filters;
  - block revokes sessions; blocking a deleted account exits 1;
  - unblock returns to the previous state;
  - restore returns to `active` or `unverified`;
  - set-quota refuses a student;
  - an unknown email exits 1;
  - the audit event carries the operator's system user.
- **readiness** reports `storage` and gives 503 when the bucket is unreachable.

**Unit tests, only for intricate logic:**

- throttle windows and `Retry-After`;
- the backoff and jitter schedule;
- the account state machine;
- Argon2 PHC encode, parse and the rehash decision;
- the email header encoder;
- magic-byte detection;
- `safeNext` (frontend);
- the refresh single-flight with locks (frontend, using an injected lock and fetch fake through constructor injection, no `vi.*`).

**Contract tests for fakes:**

- `mail`: one shared suite run against the SMTP adapter (against the in-process SMTP server, D3) and the database sink;
- `storage`: one shared suite run against the S3 adapter on SeaweedFS and the in-memory store (put, open, delete, missing key, ping).

**Playwright** (per-worker deployments, D16):

- pilot scenario 1, steps 1 to 3: register; the confirmation email is in the sink; confirm in a fresh context with a new password, which signs in and lands on Today; `PATCH /users/me` saves the bio. The checklist assertions are `pending implementation` until slice 10.
- pilot scenario 4, step 8: an anonymous visit to a private page redirects to `/?next=…`, and `GET /users/me` gives 401.
- scenario 3, step 1 (sign-up and confirmation part) through the same shared seed helper `seedVerifiedUser`.
- the account edge-case suite, browser parts:
  - sign out, and sign out everywhere across two contexts;
  - reset through the sink link, and change password;
  - delete account;
  - the throttle message;
  - the token removed from the address bar;
  - `next` honoured, and refused for `//evil`;
  - a student address visited by a tutor shows Not found;
  - an unverified tutor sees the banner;
  - email language follows the account;
  - the `system` theme follows the scheme;
  - the stored-keys check.
- the slice 1 smoke tests rewritten to use signed-in actors instead of link state.

**Test-control additions:** `POST /__test__/mail` and the clock and tick gating; the harness creates a bucket per worker with `docker compose exec storage weed shell` (`s3.bucket.create`).

## 9. Threat-model and documentation updates (in the change that implements each)

**`docs/threat-model.md`** (marked pending owner review):

- T-1: Argon2 parameters, the hash concurrency limiter, the dummy hash.
- T-2: rotation and reuse detection, the generation MAC (a forged stale-generation cookie cannot revoke a session), and the Web Locks multi-tab rule.
- T-3: identical register and forgot responses including timing, and the public resend (if D9 A).
- T-4: tokens minted by the worker and superseded.
- T-13 and T-35: avatar uploads and serving.
- T-15 and T-24: the implemented `Origin` check and cookie attributes; still no CORS.
- T-18: subject hashing, and no email or name in logs.
- T-19: the new secrets; the AWS SDK built without environment or shared-file reads; traces (D18).
- T-20: the hash limiter, avatar upload concurrency, client-IP derivation (D13), deadlines on the system clock.
- T-22: the `ovc-admin` audit event.
- T-23: the squatting flow as implemented.
- T-27 and T-28: the frontend implementation.
- T-31: sending limits and header encoding.
- T-34: which tables slice 2 purges, and that the rest are pending.
- T-36: the sink database-name rule, tick gating, testdb loopback.
- T-37: the lint rules and the memory-only token.
- T-39: new dependencies and the OSV triage for the backend.
- T-41: the AGENTS.md amendment.
- Update the status line.

**Other files under `docs/`:**

- `docs/development.md`: `sqlc diff`; `ovc-admin` usage; the new environment variables; the harness changes (mail route, tick contract, per-worker deployments, HTTPS preview if D17 needs it, trace policy); the Tailwind check; the lint rules.
- `docs/requirements/functional.md`: FR-1.2 wording on the optional code, if the owner chooses that resolution (conflict 7).
- `docs/roadmap.md`: the closing line after the slice review.

**`AGENTS.md`:** allow sqlc query annotations (conflict 1), via an owner-approved change with a security reviewer.

**`knowledge/`:**

- `api.md`: the codes table (`internal`, `invalid_credentials`, `token_invalid`), the request and response shapes of the verify, login and refresh routes, the resend variant, the avatar route, and readiness with storage.
- `accounts.md`: unverified gating as implemented, the consent version, and the sending-limit silence on forgot.
- `notifications.md`: the tick contract, the NOTIFY timing (D14), and the template location.
- `data-model.md`: which slice adds each column, the test-only tables (D11), the `previous_token_hash` index, subject hashing, and priority as smallint.
- `files.md`: avatar serving and the avatar read deadline.
- `observability.md`: the event names.
- `architecture/detailed.md`: stack rows with the chosen libraries; the configuration table (new variables and removals); package layers for the new test-only packages; the open-questions row for bounces; the module map's email i18n split.
- `design/web/navigation.md`: remove the interim section.
- `design/web/pages.md`: the resend behaviour and the unverified-student confirmation state.
- `research/backend-libraries-<date>.md`: the dated library facts.
- `reviews/ai-development-setup.md`: newly activated checks.
- `reviews/slice-2-review.md`: written at the end of the slice.

## 10. Sequence of changes

One branch and one commit each. "∥" marks changes that can run in parallel once their dependencies have merged.

| # | Branch | Content | Depends on | Security reviewer |
|---|---|---|---|---|
| C0 | `docs/slice-2-plan` | this plan; the roadmap moves of FR-1 and FR-16 parts; proposal 0002 | — | yes |
| C1 | `chore/allow-sqlc-annotations` | AGENTS.md rule for `-- name: <Name> :one\|:many\|:exec\|:execrows`; the conventions SQL test accepts exactly that form; T-41 note | — | yes |
| C2a | `fix/test-fixture-errors-and-loopback` | `testdb` error kinds and loopback admin URL; T-36 | — ∥ C1 | yes |
| C2b | `feat/error-code-enum` | `AllErrorCodes` for the D7 scope plus the D8 codes; OpenAPI enum; frontend `ErrorCode` union and typed `errors` translations; api.md table | — ∥ C1, C2a | no (contract only) |
| F1 | `feat/frontend-dom-sink-and-storage-lint` | Lint rules, fixtures, preferences module move | — ∥ all of the above | yes |
| F2 | `feat/frontend-tailwind-class-check` | Class check and fixture | — ∥ | no |
| C3 | `feat/accounts-schema-and-sqlc` | sqlc tool pin (D5); `sqlc.yaml`; migration 00002 with all slice 2 tables; `internal/dbgen`; `internal/repositories` with repository integration tests; `sqlc diff` registered; data-model.md | C1 | yes (dependency, schema of credentials) |
| C4 | `feat/outbox-worker-and-mail` | mail adapters (D3) with contract tests; random source; email templates infrastructure (D6); `services/notifications` dispatch core; worker dispatcher, scheduler, `job_runs`, outbox cleanup; `/__test__/mail`; tick contract (D10); sink guard (D11); SMTP and mail settings; notifications.md; development.md; T-31, T-36 | C3 | yes |
| C5 | `feat/auth-sessions` | Argon2 (D1); JWT (D2); `services/accounts` (register, login, refresh, logout, logout-all, authenticate, throttles, the confirmation outbox rows); audience middleware; `GET /users/me`; settings (public URL, JWT, TTLs, trusted proxies D13); the T-20 deadline test; T-1, T-2, T-3, T-15, T-24 | C2b, C4 | yes |
| C6 | `feat/email-confirmation-and-passwords` | verify-email (both paths), resend (D9), forgot, reset, change; minting at dispatch; three RU/EN templates; T-4, T-23 | C5 | yes |
| C7 | `feat/storage-and-avatars` | S3 adapter (D4) plus memory fake with contract tests; `teststorage`; readiness with storage; `services/files` avatar; `POST /uploads?context=avatar`; `GET /avatars/{id}` (D12); `PATCH /users/me`; the `storage_delete` lane; `uploads.expire`; T-13, T-35 | C5 ∥ C6 | yes |
| C8 | `feat/account-deletion-and-purge` | `DELETE /users/me`; purge; unverified cleanup; rate-limit cleanup; T-34 | C6, C7 | yes |
| C9 | `feat/ovc-admin` | `cmd/ovc-admin`, `internal/admin`, the operator service operations, process tests; T-22 | C8 (restore needs deletion) ∥ F-changes | yes |
| F0 | `chore/e2e-per-worker-harness` | per-worker deployments (D16); `OVC_TEST_CONTROL=1` and `OVC_CLOCK=test`; bucket per worker; sink helper; cookie probe and HTTPS if needed (D17); trace policy (D18) | C4 (mail route), C5 (cookie probe) | yes |
| F3 | `feat/frontend-session-and-guards` | session store with Web Locks (D19); guards; `next`; removal of link-state; sign-in and sign-out; smoke rewritten; scenario 4 step 8 | F1, F0, C5 | yes |
| F4 | `feat/frontend-signup-and-token-pages` | sign-up pages; check-your-email; verify and reset token pages; forgot; error translations in use; scenario 1 steps 1–2; scenario 3 step 1 (sign-up and confirmation part) | F3, C6 | yes |
| F5 | `feat/frontend-settings` | profile and account settings, avatar, password change, sign out everywhere, delete; `system` theme listener; scenario 1 step 3; edge-case suite | F4, C7, C8 | yes |

**Critical path:** C0 → C1 → C3 → C4 → C5 → C6 → C8 → F5. Merge C2b before C5. After C5 merges, C7 and C6 can run in parallel, and so can F0 and F3. C9 runs in parallel with F4 and F5. The slice review starts after F5 and C9 have merged.

## 11. Acceptance criteria

Every command exits 0:

- from `backend/`: `go mod tidy -diff`, `test -z "$(gofmt -l .)"`, `go vet ./...`, `../bin/golangci-lint run ./...`, `../bin/govulncheck ./...`, `go test -race -count=1 ./...`, `go -C ../tools tool sqlc -f ../backend/sqlc.yaml diff`;
- from `frontend/`: `pnpm typecheck`, `lint`, `format:check`, `test`, `build`, `api:check`, `audit --audit-level high`, `e2e`;
- `tools/docs/check-docs.sh`, Gitleaks (history and staged), and OSV-Scanner over every lockfile, with dated triage for each accepted finding.

Named tests that pass, among others:

- `TestRegisterAnswersIdenticallyForNewAndExistingEmail`
- `TestConfirmFromAnotherBrowserRequiresPasswordAndRevokesSessions`
- `TestLoginThrottlesTheSixthFailureWithin15Minutes`
- `TestRefreshReuseRevokesTheSession`
- `TestForgedStaleGenerationIsRefusedWithoutRevoking`
- `TestRefreshRejectsForeignOrigin`
- `TestRefreshWithoutCookieAnswers204`
- `TestAccessTokenExpiresByTheSharedClock`
- `TestTransportDeadlinesIgnoreTheSharedClock`
- `TestEveryOperationDeclaresAnAudience`
- `TestConfirmationTokenIsMintedAtDispatchAndSupersedesEarlierOnes`
- `TestDispatcherMarksRowDeadAfterEightAttempts`
- `TestTickTwiceWithSameNowHasNoExtraEffect`
- `TestMailContract/{smtp,sink}`
- `TestStorageContract/{s3,memory}`
- `TestAvatarRefusesOversizeAndDisguisedFiles`
- `TestUnboundAttachmentIsNotServedAsAvatar`
- `TestPurgeRemovesAccountRowsAndAvatarAfter30Days`
- `TestUnverifiedAccountIsRemovedAfter7Days`
- `TestTestControlMailRouteAbsentWithoutFlag`
- `TestSinkRefusedForProductionDatabaseName`
- `TestOvcAdmin{List,Block,Unblock,Restore,SetQuota,UnknownEmail}`
- `TestAccountFlowsLogNoSecretsOrEmails`
- `TestTestDatabaseFixtureRefusesNonLoopbackAdminURL`
- `TestErrorCodesEnumListsEveryConstant`

Endpoint behaviour:

- `POST /api/v1/auth/register` returns 202 with an empty body for a new and an existing email alike.
- `POST /auth/login` sets exactly `__Host-ovc_refresh` with `HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/` and no `Domain`.
- `POST /auth/refresh` without a cookie returns 204 with no body.
- `GET /users/me` without a token returns 401 with an `error.code` in the published enum.
- `GET /api/v1/health/ready` returns `checks.storage: "ok"`.
- The OpenAPI document's `ErrorBody.code` has an `enum`.

Playwright: `pilot-1 › steps 1–3`, `pilot-4 › step 8 anonymous redirect`, the `account edge cases` suite, and `stored keys are known preferences` pass in Chromium, Firefox and WebKit, and `pnpm lint` fails on each fixture in `eslint-rules.test.ts`.

`ovc-admin users block <email>` exits 0, and a later login gives 403 `account_blocked`. `ovc-admin users block <unknown>` exits 1.

## 12. Risks, open questions and conflicts

**Conflicts between specifications, or between a specification and the code.** None of these is resolved silently.

1. AGENTS.md forbids SQL comments except goose markers, and `TestSQLHasNoCommentsBeyondGooseMarkers` enforces it, but sqlc requires `-- name:` annotations in `backend/queries/`.
2. data-model.md says test-only tables are "never created by production migrations", but `backend/migrations/00001_test_clock.sql` is in the normal set (D11).
3. `POST /auth/register` returns no session and resend is "user, unverified ok", yet pages.md shows a resend button right after submitting (D9).
4. The API code table has no code for a failed login, an invalid confirmation or reset token, or a wrong current password; `internal` (500) exists in `internal/domain/errors.go` but not in the specification table (D8).
5. The configuration table lists `OVC_CORS_ORIGINS`, while T-24 says no CORS is configured until a separate origin needs one.
6. The configuration table has no storage region, SMTP TLS mode or trusted-proxy setting, and offers `OVC_STORAGE_BACKEND=memory` "in tests", while AGENTS.md limits in-memory storage to unit and contract tests.
7. FR-1.2 reads as if a student sign-up needs an invitation or a code; pages.md and accounts.md make the code optional.
8. The open-questions table puts bounce handling in "Slice 2, with the provider choice", while the provider is chosen "Before first deployment". Recommendation: move it there and defer `email_undeliverable_at`.
9. notifications.md wakes the dispatcher with `NOTIFY` now, while the `LISTEN` infrastructure arrives with the slice 4 broadcaster (D14).
10. The module map gives `internal/core` "i18n of emails" and `internal/services/notifications` "email composition". Recommendation: the catalogue and plural rules in core; templates and rendering in notifications.
11. `internal/testcontrol` tick reads the shared clock regardless of `OVC_CLOCK`. Local development runs `OVC_TEST_CONTROL=1` with `OVC_CLOCK=system`, so tick either fails with "clock not set" or races the worker loops (D10).
12. `users.avatar_attachment_id` and an `avatar` attachment reference both record the avatar. Recommendation: keep both, written in one transaction, with the reference authoritative for serving.
13. The observability specification requires a "refresh rotated" event, but periodic activity must not log at INFO (D22).
14. The API says every route not marked unverified ok answers `email_not_verified`, but slice 2 has no such route, so that gate gets live tests only in slice 3.
15. detailed.md says the development mail sink is "viewable through a development-only page", while the development guide says the sink is readable only through the harness route.
16. accounts.md says forgot "always returns the same response, within the sending limits". Recommendation: the per-account limit is silent and only the per-IP limit returns 429.
17. Pilot scenario 1 step 2 expects the first-run checklist (FR-15.2, slice 10). Slice 2 can only assert landing on Today.
18. The current e2e harness runs with `OVC_TEST_CONTROL=0` and one shared deployment, while the development guide requires per-worker deployments with test control from the first pilot suite in slice 2 (D16).

**Risks:**

- **Secure cookies on loopback HTTP** in WebKit, and possibly elsewhere (D17). Probe this first in F0.
- **Test duration and the 10-minute budget (NFR-6.2).** Argon2 under `go test -race` may be slow. Measure in C5 before adding more tests. If needed, inject the parameters and consolidate the tests that hash; never weaken production parameters silently, and any test-only parameters need owner approval.
- **Per-IP limits in end-to-end tests.** Suites seed many accounts from 127.0.0.1, and the sign-up limit is 10 per hour. Seed helpers advance the shared clock or spread seeding across workers. Do not add a bypass.
- **The sqlc tool build** (cgo, minutes on first build) and a larger tools module, which means more OSV triage.
- **The aws-sdk-go-v2 dependency tree** and its fast release cadence against the 14-day age rule; its checksum defaults with S3-compatible providers.
- **`DELETE` with a JSON body** may be stripped by some proxies; a `POST /users/me/delete` fallback would change the API specification.
- **The refresh-reuse false positive** when a refresh response is lost (a tab closed mid-request): the session is revoked and the user signs in again. Accepted residual risk, to record under T-2.
- **Tailwind class check.** Extraction may miss composed class names; the convention is that class names are literal strings.
- **Smoke rewrite.** Replacing the link-state frames rewrites about ten smoke tests, which then depend on seeded accounts.

**Open questions for the owner, beyond D1–D22:**

- **Q1.** Maximum length of a subject string for tutors: 60 characters? Bio is 1,000; the number of subjects is at most 10.
- **Q2.** Should an access token or session record a user-agent summary, and in what form (browser family only)?
- **Q3.** Verify-email response when the caller is already signed in to another account: treat it as "another browser" and require a password? Recommended: yes.
- **Q4.** Whether "Sign up" pages are reachable while signed in. Recommended: redirect to home, like `/`.
- **Q5.** Privacy-policy link target: a frontend route `/privacy` with owner-supplied text, or a fixed external address? The frontend reads no settings.

## Slice 2 plan, revision 2: amendments after the planning reviews

These amendments, made after the standard and security planning reviews, override the matching parts of revision 1 (sections 1 to 12 above). Revision 1 text they supersede includes: C5 containing `Register` and the critical path in section 10 (amendment 17); D12's public immutable caching and the 5 MiB limit (amendment 13); `users.avatar_attachment_id` and conflict 12 (amendment 14); D13's right-most entry (amendment 9); D21's plain hashes (amendment 2); the sessions schema in section 4 and the `Refresh` description in section 5 (amendment 3 and the refresh cookie MAC note); student sign-up, D15, pilot scenario 2 step 2, the unverified student confined (sections 6 and 8) and amendment 21 for students, and the FR-1.2 wording item in section 9 (amendment 34 and conflict 7).

### A. Sessions, tokens and throttling

- **1.** **Atomic throttling.** Every counter (sign-in per email and per IP, register, forgot, resend, current-password checks) reserves its slot atomically before any hashing: `INSERT … ON CONFLICT (purpose, subject, window_start) DO UPDATE SET count = count + 1 RETURNING count`, refusing once over the limit. A successful sign-in returns its reserved slot. Test: 20 parallel wrong-password sign-ins for one email perform at most 5 password verifications.
- **2.** **Throttle subjects** are HMAC-SHA-256 with a key derived by HKDF from `OVC_JWT_SECRET` (purpose "throttle"), not plain hashes; IPv6 addresses are bucketed by /64; no raw IP is logged. Windows: per-email sign-in 15 minutes; per-IP sign-in 15 minutes; sign-up and reset requests share one per-IP counter of 10 per hour; resend is counted with them. Email normalisation matches `citext` (lower-case, trimmed).
- **3.** **Refresh tokens carry a generation.** The cookie value is `<sid>.<generation>.<random>`. The session row stores the current generation and the hash of the current random part. Presenting an authentic `sid` with a stale generation revokes the session (replay of any earlier token, not only the previous one). Rotation is a compare-and-swap `UPDATE … WHERE id = $sid AND generation = $g AND refresh_hash = $h AND revoked_at IS NULL AND expires_at > $now RETURNING`, so concurrent refreshes cannot both rotate. Refresh also checks the account state (blocked → `403 account_blocked`, deleted → `401 unauthenticated`, both clear the cookie). Expiry is absolute: 30 days from sign-in, never extended by rotation. Test: T0 → T1 → T2, then T0 replayed revokes the session.
- **4.** **Email tokens are state-bound.** Confirming accepts only an `unverified` account; resetting accepts only `unverified` or `active`; any other state answers the same `422 token_invalid`. Blocking, deletion, confirmation, reset and password change invalidate every unused email token of both purposes. Consumption is one conditional `UPDATE … SET used_at = $now WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $now RETURNING`. The "new password required" answer does not consume the token. Tests cover each refused state and the 1-hour reset expiry.
- **5.** **Silence on public routes.** Per-account sending limits are silent on every public route (forgot, public resend); only the per-IP limit answers `429`. The authenticated resend may answer `429`. The public resend joins the identical-response tests. Residual risk recorded under T-31: an attacker can use up a victim's daily confirmation and reset budget.
- **6.** **Current-password checks** (change, delete) count failures per account under the sign-in failure limit and answer `429` with `Retry-After`.
- **7.** **Hashing cost.** The Argon2id limiter's wait is bounded (5 s, and never past the request deadline), then answers `503 service_unavailable`. Every route orders its work as throttle → token and state validity → hash: verify and reset hash only after the token is valid; register hashes only after the per-IP check. The dummy hash for unknown emails is computed at start-up with the current parameters; the deleted-account sign-in path verifies a hash too.
- **8.** **JWT.** `exp` and `iat` required, leeway at most 5 s, `sub` must match the session's user, golang-jwt v5 at 5.2.2 or later, a separate verifier instance and key for the slice 7 conferencing JWT, `OVC_ACCESS_TOKEN_TTL_SECONDS` bounded to 60–3600 and the refresh TTL to 1–90 days. Login, refresh, verify and `/users/me` responses send `Cache-Control: no-store`.
- **9.** **Client IP (D13).** All `X-Forwarded-For` lines are joined and walked from the right, skipping entries inside `OVC_TRUSTED_PROXY_CIDRS`; an unparsable entry falls back to the peer address; settings refuse `0.0.0.0/0` and `::/0`. The proxy appending or overwriting the header becomes a deployment obligation under T-20. End-to-end seed helpers send `X-Forwarded-For` through a trusted `127.0.0.1` so seeding does not hit the per-IP sign-up limit and the shared clock is never advanced for seeding.
- **10.** **Request shape.** Body routes refuse a missing `Content-Type`; `/auth/login` gets the same exact-Origin check as refresh, because it sets the cookie.
- **11.** **Email addresses** are validated with `net/mail` and refused with CR, LF or control characters; `OVC_PUBLIC_URL` must be an origin only (no path, query or user info) and `https` unless `OVC_TEST_CONTROL=1`.

### B. Email content, avatars and purge

- **12.** **No user text in account emails.** Confirmation, reset and duplicate-sign-up emails contain no user-supplied text (no name in subject or body); a test asserts it. Names appear only in later emails between people who know each other.
- **13.** **Avatars.** Metadata is removed by re-encoding the decoded image (JPEG, PNG and WebP decoded with the standard library and `golang.org/x/image/webp`, written back as PNG or JPEG), which also drops EXIF, XMP and text chunks; `image.DecodeConfig` refuses images above 4096 × 4096 before decoding. Per user at most 3 pending avatar uploads and 20 uploads per hour. `GET /avatars/{id}` answers `404` for a malformed id, an unbound attachment, or an owner that is blocked or deleted, and serves with `Cache-Control: private, max-age=86400`, `Content-Security-Policy: default-src 'none'; sandbox`, `Cross-Origin-Resource-Policy: same-origin`, `Content-Disposition: inline` without a filename. Attachment ids come from `crypto/rand`. The avatar limit is 5 MB as files.md says; the body is buffered in memory (bounded) rather than uploaded with S3 multipart, the `sha256` column is computed, and files.md records this for avatars.
- **14.** **Purge is complete.** The migration states every `ON DELETE` action; `users.avatar_attachment_id` is dropped in favour of the attachment reference alone, which removes the circular key. Purge deletes every attachment whose uploader is the account (pending, bound and `deleted`), writing `storage_delete` outbox rows for objects still held, then sessions, email requests and tokens, the tutor profile and the user row. Dispatch marks a row `done`/`obsolete` when its recipient no longer exists. Throttle rows are pseudonymous and expire with their window. A test asserts no row owned by the purged account remains.

### C. Structure and sequence

- **15.** **Worker through services.** A new `internal/services/outbox` owns claim, lease, complete, retry, reclaim and cleanup, and the job-run table; `services/notifications` builds and sends emails; `services/files` performs `storage_delete`; `services/accounts` owns purge, unverified cleanup and rate-limit cleanup. The worker imports only services. The module map and package-layer table gain the new rows in C4.
- **16.** **Operator operations** live in a small `services/operators` that needs only repositories and logging, so `ovc-admin` needs only the database and logging settings.
- **17.** **Sequence corrected.** `OVC_PUBLIC_URL` moves into C4. C5 delivers sign-in, refresh, sign-out and `/users/me` but no sign-up; `Register`, the confirmation and duplicate-sign-up outbox rows, minting and their templates move together into C6, so `main` never queues emails it cannot send. Critical path: C1 → C3 → C4 → C5 → C6 → F0 → F3 → F4 → F5; C7 runs alongside C6; C8 needs C6 and C7; C9 runs alongside the frontend changes. F3 depends on F0. C2b gets a security reviewer.
- **18.** **Roadmap first.** A first docs change (C0, `docs/slice-2-plan`, before C1 on the critical path) records this plan and moves the deferred parts of FR-1 and FR-16 into the roadmap entries of slices 3 and 4 (FR-1.2, FR-1.3, the confirm-before-inviting gate, the relationship effects of deletion, closing live connections; the message effects of deletion go to slice 6), corrects the roadmap's "in-memory" sink to the database sink, and is marked pending owner review.
- **19.** **Check budget.** A new acceptance criterion: the full local check run, end-to-end suites included, finishes under 10 minutes, measured in C5 and again in F0 and at the end of the slice, recorded in `knowledge/analysis/`.

### D. Frontend and the rest

- **20.** **Verify page waits for the session.** The verify POST runs only after the start-up refresh has settled and is sent once (guarded against StrictMode double effects). A Playwright test opens the link in a new tab of a signed-in context and asserts the signed-in path: no password prompt, other sessions revoked.
- **21.** **Unverified students** keep sign-out, the language switch, account settings and deletion reachable while confined to the confirmation state.
- **22.** **safeNext** navigates to the parsed URL's `pathname + search + hash`, never the raw string; tests add tab, newline and `%5C` cases. T-27 records that the visited token URL stays in the browser's own history despite `replaceState`, and that token pages go live before the production proxy redacts its logs (slice 10).
- **23.** **Lint (F1)** also refuses computed access (`window['localStorage']`, `self.localStorage`, `document['cookie']`), `eval`, `new Function`, `srcdoc`, `createContextualFragment` and `DOMParser`.
- **24.** **F2** runs as its own script after `pnpm build`, not inside `lint`.
- **25.** **Language default:** a browser language other than Russian or English defaults to English on sign-up and in `theme-init.js`/`main.tsx`.
- **26.** **Test control.** The mail sink reuses `VerifyTestControlHosts` and `VerifyTestControlDatabase` in both processes (not only a name rule at load time); the mail route stays outside huma and the OpenAPI document; the self-signed preview certificate is generated per run into an ignored directory and never installed into a trust store; `__Host-` and `Secure` are never relaxed in test mode; T-19 notes that Gitleaks never scans the working tree where traces live.
- **27.** **Dependencies are enforced.** depguard: x/crypto and the JWT library only in `internal/core`; the AWS SDK and the SMTP client only in `internal/adapters`; `aws-sdk-go-v2/config`, `credentials/ec2rolecreds` and `feature/ec2/imds` banned; go-smtp's server package only in tests.
- **28.** **C1** accepts exactly `^-- name: [A-Z][A-Za-z0-9]* :(one|many|exec|execrows)$`, only in `backend/queries/*.sql`; migrations stay limited to goose markers.
- **29.** **ovc-admin audit** logs the numeric uid and the user name, escapes `%` and `_` in `--email`, and the operator guide asks operators to use personal accounts.
- **30.** **Observability:** add `upload.aborted`; bind `user_id` and `role` to the request log context after authentication; the worker finishes in-flight sends on shutdown within its timeout (NFR-4.4). T-14, T-17, T-38 and T-40 are mapped and updated where touched. The log-queue flood item stays in slice 10 and the plan says so.
- **31.** **SMTP** uses a small connection pool sized to the email lane, not a single connection.
- **32.** **Repository tests** cover only intricate queries (claim and lease, conditional updates, batched purge, throttle reservation).
- **33.** **Conflicts added:** api.md expects the storage host in the CSP from slice 2 (D12 keeps `'self'`; api.md is updated); avatar size in MB per files.md.

#### Students join through their tutor

- **34.** Following [proposal 0002](../../docs/proposals/0002-student-onboarding.md), slice 2 ships sign-up for tutors only: `/signup` offers "I teach" and, for "I study", the message to ask the tutor for a link or code; `/signup/student`, the student branch of `POST /auth/register`, D15 and pilot scenario 2 step 2 (a student signing up with a code) move to slice 3, and so do the end-to-end checks that need a student account (an unverified student confined to the confirmation state). Pilot scenario 3 step 1 keeps its tutor sign-up and confirmation part in slice 2. End-to-end tests create accounts only through the public API; no `/__test__/` route creates accounts. FR-1.2 keeps its current wording until slice 3 rewrites it (conflict 7 is resolved by the proposal), and the register route refuses `role: student` with `validation_failed`, and keeps refusing it from slice 3 on, when students sign up only through an invitation or a tutor code; that refusal reveals nothing about existing accounts. The change that implements the register route marks this tutor-only behaviour in `knowledge/specs/api.md` and `knowledge/specs/accounts.md`.

### Implementation notes

- **C3** builds the schema, sqlc, the generated package and the repositories with the queries the plan names, and tests the intricate ones (claim and lease, conditional updates, throttle reservation). The batched purge statements and their test move to C8, which implements purge. `outbox.kind` and `job_runs.job` get named types now; C4 defines their values. Reclaiming an expired lease makes a row `dead` once its attempts are used, and outbox cleanup takes separate cutoffs for `done` and `dead` rows, as the notifications specification requires.
- **Refresh cookie MAC** (owner decision, 2026-10-21): the cookie value becomes `<sid>.<generation>.<random>.<mac>`, superseding the format of amendment 3. The MAC is HMAC-SHA-256 over the session identifier and generation, keyed by HKDF-SHA-256 from `OVC_JWT_SECRET` with info `refresh-generation`. Only a cookie with a valid MAC and an older generation counts as reuse and revokes the session; C5 implements it, updates `knowledge/specs/accounts.md` and T-2, and adds `TestForgedStaleGenerationIsRefusedWithoutRevoking`. C5 also specifies: the MAC is verified before the session is looked up; a refresh whose conditional update matches no row is re-checked against the current row and classified by the same rules, so losing a race counts as reuse (the Web Locks rule keeps tabs from racing), and T-2 lists this with the lost-response false positive; and the canonical cookie encoding (the MAC input bytes, base64url without padding, a 32-byte MAC, strict parsing of every segment), with a refusal test for malformed cookies.

- **C4** builds the mail adapters with their contract tests, the random source, `services/outbox`, the notifications dispatch core, the worker pass and the outbox cleanup job, the mail sink route and the sink guard, the SMTP and mail settings and `OVC_PUBLIC_URL`. The email templates infrastructure (D6) moves to C6 with the first templates, so no unused rendering code lands before an email exists. The rate-limit cleanup job moves to C5 with the throttles, the `storage_delete` lane and `uploads.expire` to C7, and the unverified cleanup and purge jobs to C8. depguard restricts go-smtp and go-sasl to `internal/adapters`; it cannot separate go-smtp's server from its client, which share one package.

- **C5** delivers sign-in, refresh with the generation MAC, sign-out, sign-out everywhere, authentication with the audience middleware, `GET /users/me`, the session settings and the rate-limit cleanup job. `Register` stays in C6. A repository-level block operation and an optional confirmation time on user creation let tests seed blocked and active accounts; `ovc-admin` reuses the block operation in C9. golang-jwt's key function returns an empty interface, so the signature is verified explicitly (research note). Binding `user_id` and `role` to the access log (amendment 30) moves to F3, with the frontend session work. The check budget was measured with this change ([check budget](../analysis/check-budget-2026-10-21.md)). The throttle window is the domain rule `FixedWindow`; `TestTransportDeadlinesIgnoreTheSharedClock` runs the real API binary with the shared clock; a repository-level delete mark seeds deleted accounts for tests, which C8 reuses.

- **C6** delivers sign-up for tutors, both confirmation paths, the public and signed-in resend, forgot, reset and change, token minting at dispatch and the three account emails in Russian and English. `POST /auth/verify-email` and the resend route use a new audience, "public, optional bearer"; the confirmation route checks the exact `Origin` because it can set the refresh cookie. The email wording is a catalogue in `internal/core` and the two templates live in `internal/services/notifications`; the account emails show no counts, so the `x/text` plural rules (D6) arrive with the first email that does. The default tutor quotas become API settings now, because sign-up creates the tutor profile. A request over a per-account sending limit rolls back its own counter increments, so refused requests do not use up the budget. The per-account sending limits are keyed by the normalised email, and forgot and the public resend count them for unknown emails too, so the timing difference between a known and an unknown email shrinks to two inserts; the owner accepted that residual difference instead of the identical timing of section 9. A password change of an unverified account keeps the emailed links, so a squatter cannot cancel the mailbox owner's confirmation.

- **C7** delivers the S3 adapter and the in-memory store with one contract test, readiness with storage, avatar upload, binding through `PATCH /users/me` (with the tutor bio and subjects), public serving, the `storage_delete` lane and the `uploads.expire` job. Integration tests share one local bucket, `ovc-test`, with random keys instead of a bucket per test, because SeaweedFS allocates volumes per bucket and many short-lived buckets would exhaust them; the end-to-end deployment uses `ovc-e2e`, created through the storage container's shell. The local keys reach the tests only through `tools/dev-env.sh`, so the backend tests and the end-to-end suites now run through it. The hourly upload limit is implemented but not exercised by a test, because three pending uploads reach the pending limit first. `GET /users/me` and the avatar route read the tutor profile, so every tutor needs one; tests seed it with the account.

- **C8** delivers `DELETE /users/me`, the immediate purge of an unverified account (Q6), the hourly `accounts.purge` and `accounts.cleanup_unverified` jobs, and purge of every table slice 2 creates. The rate-limit cleanup job already landed in C5. Purge runs in a small `accounts.Purger` that needs only the store, the upload purger and logging, so the worker does not need the session keys. `storage_delete` payloads now carry the storage key, because purge deletes the attachment rows before the worker deletes their objects. The current-password check is shared by password change and deletion.

- **C9** delivers `cmd/ovc-admin`, `internal/admin` and `internal/services/operators` with the account state machine as a domain rule, process tests for every subcommand, and the audit event. Since deleting an unverified account purges it at once (Q6), restore only ever returns an account to `active`; unblock into `unverified` restarts the 7-day clock (Q8). Block also discards pending email requests and tokens (amendment 4).

- **F1** adds the DOM-sink, cookie, IndexedDB and Web Storage lint rules for `src/` and `public/`, with a fixture test per rule, and moves the `window.localStorage` read into `src/lib/preferences/store.ts`, which exposes only the typed preference store; a check refuses files under `src/` that ESLint does not cover. The Playwright check of stored keys lands with the first account journeys (F3).

- **F2** adds `pnpm check:classes`, run after `pnpm build` (amendment 24): the TypeScript compiler resolves constants named in `className`, including constants imported from other modules; fragments joined at run time and the arguments of function calls are skipped rather than guessed.

- **F0** gives every Playwright worker its own database, API, worker and `vite preview` (D16) with `OVC_TEST_CONTROL=1`, `OVC_CLOCK=test` and the trusted proxy `127.0.0.1/32`, and a `control` fixture for the mail sink, ticks and the clock. The cookie probe (D17) found that WebKit drops the `Secure` refresh cookie over plain loopback HTTP, so the preview is served over HTTPS with a per-run self-signed certificate (amendment 26). Deployments share the bucket `ovc-e2e` instead of one bucket per worker, for the reason given under C7. Traces stay `retain-on-failure` (D18). The full check run, end-to-end suites included, took about 55 seconds.

### E. New owner questions

- **Q6, deleting an unverified account**, raised by the security planning review: purge it immediately and free the email (recommended), so a squatter cannot lock the real owner out for 30 days.
- **Q7, Argon2 parameters in tests:** allow tests to pass cheaper parameters through the constructor while production keeps the OWASP parameters (recommended), or keep production parameters everywhere and accept slower tests.
- **Q8, an account restored or unblocked into `unverified` after 7 days** would be removed by the next cleanup: reset its 7-day clock on restore or unblock (recommended), or accept the removal.
- **Q9, an unknown browser timezone** at sign-up: fall back to `UTC` and show the timezone in settings for correction (recommended), or refuse sign-up.
- **Q10, password Unicode normalisation:** none, compare bytes exactly (recommended, as most systems do), or NFC.
- Recommendations for Q1, Q2 and Q5: subject length 60 characters; no user-agent summary is collected (data minimisation); a frontend `/privacy` page with text the owner supplies.

### F. Owner decisions (2026-10-21)

- D1–D4: the recommended libraries — `golang.org/x/crypto/argon2` (Argon2id, OWASP m=19 MiB, t=2, p=1), `github.com/golang-jwt/jwt/v5` (HS256 only), `github.com/emersion/go-smtp`, `github.com/aws/aws-sdk-go-v2` without environment or shared-file configuration; sqlc as a Go tool. Versions are to be re-checked against the registries (older than two weeks, licence, OSV) and recorded in `knowledge/research/backend-libraries-<date>.md` before adoption.
- D5–D22 and the resolutions of the specification conflicts: as recommended, with the amendments above.
- Q6: deleting an unverified account purges it immediately and frees the email.
- Q7: tests may pass cheaper Argon2 parameters through the constructor; production always uses the OWASP parameters, and one test asserts them.
- Q8: restoring or unblocking an account into `unverified` restarts its 7-day clock.
- Q9: an unknown browser timezone falls back to `UTC`, correctable in settings.
- Q10, Q1, Q2: passwords compared byte for byte without Unicode normalisation; subjects up to 60 characters; no user-agent summary is collected.
- Q5: a frontend `/privacy` route; slice 2 ships a placeholder and the owner supplies the text before launch; the consent version is a constant in code.
- Q3: following a confirmation link while signed in to another account is treated as another browser: a new password is required.
- Q4: sign-up pages redirect a signed-in user to their home page.
- Conflict 7: students join only through their tutor ([proposal 0002](../../docs/proposals/0002-student-onboarding.md)); FR-1.2 keeps its current wording until slice 3 rewrites it.
- Conflict 15: the development mail sink is read only through the test control route; the architecture document drops the development-only viewer page.
- Refresh cookie MAC: the refresh cookie carries a MAC over the session identifier and generation, so a forged stale-generation cookie cannot revoke a session (see the implementation notes).
