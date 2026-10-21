# Roadmap

Status: rewritten 2026-10-20 for the tutor-centric product, phone layouts added to slice 10, slice reviews and the Go backend added the same day, slice 1 adjusted on 2026-10-21 (sqlc and `ovc-admin` moved to slice 2); slice 1 closed and follow-up items placed in slices 2, 8 and 10 on 2026-10-21 after its slice review; parts of FR-1 and FR-16 moved from slice 2 to slices 3 and 4 the same day with the slice 2 plan; pending line-by-line owner review.

Build order, slice by slice. Each slice is runnable and reviewable on its own and ends with its checks registered in the [development guide](development.md). A slice is done when:

- its requirements work end to end locally through the real backend, worker and frontend;
- its segments of the [pilot scenarios](../knowledge/design/web/pilot-scenarios.md) pass;
- its checks pass;
- the documents it affects are updated;
- its slice review ([ADR-0011](decisions/0011-tiered-verification.md)) over everything merged since the previous slice closed is resolved, and the closing commit is recorded under the slice as "Closed at `<commit>`". A commit may also be named by its subject when its identifier is not yet known.

## Slice 0: foundation and product definition

Repository, agent rules ([AGENTS.md](../AGENTS.md)), the [ovc-verify](../.claude/skills/ovc-verify/SKILL.md) skill, the two documentation layers, the [product overview](product.md), requirements, threat model, decision records, specifications and page design. No application code. Active checks: Markdown lint, links and anchors, Mermaid diagrams.

Closed at the commit "Introduce tiered verification", without a slice review: slice reviews start with slice 1.

## Slice 1: scaffold and local loop

- **Backend:**
  - a Go module with the API server built on huma and typed settings ([ADR-0012](decisions/0012-go-backend.md));
  - the `pgx` connection pool and `goose` SQL migrations applied by `api migrate`; `sqlc` arrives in slice 2 with the first repository, because slice 1 has no business queries;
  - health routes, the structured logging bootstrap and request identifiers;
  - the package-layer architecture test and the integration-test database fixture;
  - an empty worker process with `Tick(now)`, the shared test clock and the first test control routes; `ovc-admin` arrives in slice 2.
- **Frontend:** a `pnpm` project with Vite, React and strict TypeScript; Tailwind over the design tokens; self-hosted fonts; the router and i18n setup with `ru` and `en`; the typed API client; the public and role frames; ESLint and Prettier; a Playwright smoke test.
- **Infrastructure:** Docker Compose for PostgreSQL and a local S3-compatible server. Security scanning tools are selected and registered.
- **Done when** the health routes answer, the frontend renders its frames in both languages against them, and every slice-1 check passes.

Closed at the commit "Record the slice 1 review and close the slice", after the [slice 1 review](../knowledge/reviews/slice-1-review.md) and its three follow-up changes.

## Slice 2: accounts, sessions and email

FR-1, FR-16, FR-17. Registration with consent, email confirmation, sign-in throttling, sessions with refresh rotation, password reset and change, sign out everywhere, profile and preferences, avatars through storage, account deletion and purge, and `ovc-admin`. The outbox, the worker dispatcher, the mail adapter (SMTP and the database sink) and the first email templates are built here. Sign-up is for tutors only in this slice: students join through their tutor from slice 3 on ([proposal 0002](proposals/0002-student-onboarding.md)). Parts of FR-1 and FR-16 that need relationships or live connections move to later slices, as listed in slices 3 to 10.

Also carried over from the [slice 1 review](../knowledge/reviews/slice-1-review.md#carried-over): route guards with the wrong-role, anonymous and unverified rules replacing the [interim behavior](../knowledge/design/web/navigation.md#interim-behavior-before-sign-in-exists); token pages that read and remove their token; the `next` parameter rule; lint rules against unsafe DOM sinks and Web Storage outside the preferences store, landing before the access token reaches the browser; transport deadlines kept on the system clock when session expiry starts using the shared test clock; error codes published as an enum; the test-control tick and worker clock contract; Playwright traces once credentials exist; the `system` theme following scheme changes; a check for Tailwind classes that produce no CSS; and the test database fixture's error text and admin URL check.

## Slice 3: tutors, students and groups

FR-2, FR-3, and from slice 2 FR-1.2 and student sign-up through an invitation or a tutor code ([proposal 0002](proposals/0002-student-onboarding.md)), FR-1.3, the confirm-before-inviting gate of FR-1.1, and the effects of account deletion in FR-1.7 on relationships, groups, student cards, invitations and join requests, with the `tutor.deleted` email. Each later slice applies the deletion effects and purge rules of FR-1.7 to the entities it introduces: lesson cancellation, series end and the cancelled lessons listed in `tutor.deleted` in slice 5, the "Deleted user" sender of messages in slice 6, and read-only access to recordings and results for 30 days in slices 7, 9 and 10. Student cards, invitations, tutor code and join requests, relationship end and reactivation, groups, and the student's tutors page. Relationship-scoped authorization guards and their denial tests.

## Slice 4: realtime and in-app notifications

FR-13.1, FR-13.3, and from slice 2 the closing of live connections on sign-out, deletion and blocking in FR-1.4, FR-1.7 and FR-16.1 ([accounts](../knowledge/specs/accounts.md#roles-and-accounts)). The WebSocket gateway with first-frame authentication, the connection registry and broadcaster, `notification.new` and `entity.changed` frames, the notifications panel, email preferences and one-click unsubscribe.

## Slice 5: schedule and reminders

FR-4, FR-13.2 (lesson emails). One-off lessons and weekly series in the tutor's timezone, scoped edits and cancellations, overlap warnings, calendars, lesson status, the lesson page without video, and lesson reminders and change emails.

## Slice 6: chat and files

FR-12, FR-14, the message digest. Direct and group channels, messages with attachments and previews, edit and delete, read markers and unread counts, presence and typing, upload contexts, quotas and usage, authorized downloads.

## Slice 7: video lessons and recordings

FR-5.

- **Probe first:** run the BigBlueButton probe on a remote test server (rendered recording format, embedding on every supported browser) and record the results in the [research file](../knowledge/research/bigbluebutton-2026-10-20.md).
- **Then build:** the BigBlueButton and fake adapters with shared contract tests, meetings and join links, callbacks and reconciliation, recording copy, playback, retention and deletion, and the embedded room with the new-tab fallback.

## Slice 8: library and builder

FR-6, FR-7. Folders, materials, templates, all eight block types, normalisation version 1, restricted Markdown rendering, and the builder with preview.

Before the builder pages land, the frontend splits its bundle by route and vendor and loads only the active language, to stay within [NFR-2.4](requirements/non-functional.md).

## Slice 9: issuing, completing and checking

FR-8, FR-9, FR-10. Issues with snapshots, deadlines and answer policies, the student assignment page with autosave, submission and automatic scoring, the review queue and review screen with corrected files, returning and resubmitting, the answer-key contract test, and homework emails.

## Slice 10: results, home pages and hardening

FR-11, FR-15.

- **Product:** results for students; analytics per issue and progress per student; Today with the first-run checklist; Home with the tutor filter.
- **Phone layouts:** dedicated phone layouts for every page and the phone navigation frame, replacing the interim one-column fallback ([ADR-0010](decisions/0010-quiet-visual-language.md)).
- **Hardening:** the accessibility pass, performance baselines and budgets for the non-functional requirements, and all four pilot scenarios passing in full.
- **Production serving** (before OVC is first deployed, at the latest here): the reverse proxy reproduces the page headers from `frontend/security-headers.ts`, caches hashed assets for a long time, compresses responses, answers the Go server's own protocol errors, redacts token segments in its access logs and never forwards `/__test__/`; Home stops polling the database-pinging readiness route; icons grow to 20 px on phones; the log queue keeps warnings and errors ahead of access records under a flood ([slice 1 review](../knowledge/reviews/slice-1-review.md#carried-over)).

## After the MVP

The [payment ledger](proposals/0001-payment-ledger.md), attendance from provider events, timed tests and multiple attempts, accounts with both roles, an administration web interface, push notifications, calendar export, thumbnails, malware scanning, and several API instances with a cross-instance broadcaster. Each starts as a [proposal](proposals/README.md) when it is designed but not scheduled.
