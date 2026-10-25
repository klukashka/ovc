# Roadmap

Status: rewritten 2026-10-20 for the tutor-centric product, phone layouts added to slice 10, slice reviews and the Go backend added the same day, slice 1 adjusted on 2026-10-21 (sqlc and `ovc-admin` moved to slice 2); slice 1 closed and follow-up items placed in slices 2, 8 and 10 on 2026-10-21 after its slice review; parts of FR-1 and FR-16 moved from slice 2 to slices 3 and 4 the same day with the slice 2 plan; recordings and analytics moved after the MVP on 2026-10-22 by the owner's decision, and the pilot deployment items added to slices 7 and 10 the same day (ADR-0013), and the worker's per-lane refill loops placed in slice 4 the same day; slice 2 closed on 2026-10-22 after its slice review, with follow-up items placed in slices 3, 4 and 10; the slice 3 plan approved the same day, with its deferred items placed in slices 4, 5, 6, 9 and 10; slice 3 closed on 2026-10-25 after its slice review, with the dependency upgrades and the suite consolidation placed before slice 4 and the per-account purge budget in slice 5; the slice 4 plan approved on 2026-10-25, with live connections narrowed rather than closed on lost scope, route-level bundle splitting moved from slice 8 to slice 4, and its deferred items placed in slices 5, 6, 8 and 10 and after the MVP (pending owner review); pending line-by-line owner review.

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

Also carried over from the [slice 1 review](../knowledge/reviews/slice-1-review.md#carried-over): route guards with the wrong-role, anonymous and unverified rules replacing the slice 1 interim behavior with the [navigation rules](../knowledge/design/web/navigation.md#rules); token pages that read and remove their token; the `next` parameter rule; lint rules against unsafe DOM sinks and Web Storage outside the preferences store, landing before the access token reaches the browser; transport deadlines kept on the system clock when session expiry starts using the shared test clock; error codes published as an enum; the test-control tick and worker clock contract; Playwright traces once credentials exist; the `system` theme following scheme changes; a check for Tailwind classes that produce no CSS; and the test database fixture's error text and admin URL check.

Closed at the commit "Record the slice 2 review and close the slice", after the [slice 2 review](../knowledge/reviews/slice-2-review.md) and its ten follow-up changes (pending owner review).

## Slice 3: tutors, students and groups

FR-2, FR-3, and from slice 2 FR-1.2 and student sign-up through an invitation or a tutor code ([proposal 0002](proposals/0002-student-onboarding.md)), FR-1.3, the confirm-before-inviting gate of FR-1.1, and the effects of account deletion in FR-1.7 on relationships, groups, student cards, invitations and join requests, with the `tutor.deleted` email. Each later slice applies the deletion effects and purge rules of FR-1.7 to the entities it introduces: lesson cancellation, series end and the cancelled lessons listed in `tutor.deleted` in slice 5, the "Deleted user" sender of messages in slice 6, and read-only access to results for 30 days in slices 9 and 10 (and to recordings once they land after the MVP). Student cards, invitations, tutor code and join requests, relationship end and reactivation, groups, and the student's tutors page. Relationship-scoped authorization guards and their denial tests.

Also carried over from the [slice 2 review](../knowledge/reviews/slice-2-review.md#accepted-minor-findings): the purge race test settles its upload goroutine before the test database is dropped, also after a failure (pending owner review).

Built as the [slice 3 plan](../knowledge/plans/slice-3.md#1-problem-outcome-and-scope) (pending owner review).

Closed at the commit "Record the slice 3 review and close the slice", after the [slice 3 review](../knowledge/reviews/slice-3-review.md) and its six follow-up changes; the dependency upgrades and the suite consolidation of its [follow-up plan](../knowledge/plans/slice-3-review-follow-ups.md) land before slice 4 adds code and tests, and the purge budget per account moves to slice 5 (pending owner review).

## Slice 4: realtime and in-app notifications

FR-13.1, FR-13.3, and from slice 2 the closing of live connections on sign-out, deletion and blocking in FR-1.4, FR-1.7 and FR-16.1 ([accounts](../knowledge/specs/accounts.md#roles-and-accounts)). The WebSocket gateway with first-frame authentication, the connection registry and broadcaster, `notification.new` and `entity.changed` frames, the notifications panel, email preferences and one-click unsubscribe. The worker's lanes run side by side with per-lane loops that refill slots as they free up, before lesson reminders depend on prompt delivery (NFR-4.5).

Built as the [slice 4 plan](../knowledge/plans/slice-4.md#1-problem-outcome-and-scope): signals written inside the committing transaction and delivered by every API instance from its own listening connection, revocation signalled by database triggers so that every revoking path closes its sockets, a socket that lives no longer than its access token, identifier-only frames whose recipients are decided under locks, in-app rows that store identifiers and resolve labels at read time, and a dedicated unsubscribe signing secret (pending owner review).

Also from the [slice 3 review](../knowledge/reviews/slice-3-review.md#carried-over): the frontend loads each route's page module on demand, so the student's first page stays inside the bundle budget of [NFR-2.4](requirements/non-functional.md) before slice 5 adds pages; loading only the active language stays in slice 8 (pending owner review).

Also carried over from the [slice 2 review](../knowledge/reviews/slice-2-review.md#accepted-minor-findings): the frontend session's `start` checks the epoch before it ends the session as unavailable, once the revocation signal can replace the session while `start` runs (pending owner review).

Also from slice 3 ([slice 3 plan](../knowledge/plans/slice-3.md#1-problem-outcome-and-scope)): in-app notification rows for accepted invitations, join requests and ended relationships, and narrowing live connections when a provisional student is removed, a relationship ends or a reclaim drops a link: no socket closes, and nothing more about the relationship reaches the party who lost it, because every frame's recipients are decided from relationship rows held under locks (pending owner review).

## Slice 5: schedule and reminders

FR-4, FR-13.2 (lesson emails). One-off lessons and weekly series in the tutor's timezone, scoped edits and cancellations, overlap warnings, calendars, lesson status, the lesson page without video, and lesson reminders and change emails.

Also from slice 3 ([slice 3 plan](../knowledge/plans/slice-3.md#1-problem-outcome-and-scope)): the `group.archived` email; the lesson effects of ending a relationship, removing a provisional student, removing a group member, archiving a group and deleting a tutor, with the cancelled lessons listed in `relationship.ended` and `tutor.deleted`; and one-to-one lessons only for a provisional student (pending owner review).

Also from the [slice 3 review](../knowledge/reviews/slice-3-review.md#carried-over): each account gets its own share of a purge run's budget, so one large tutor purge no longer delays the purge of other accounts by whole hourly runs (pending owner review).

Also from the [slice 4 plan](../knowledge/plans/slice-4.md#1-problem-outcome-and-scope): the one-click unsubscribe headers, the dispatch-time preference check and minting tokens into emails at render time arrive with the first optional email, and `entity.changed` gains the kind `lesson` (pending owner review).

## Slice 6: chat and files

FR-12, FR-14, the message digest. Direct and group channels, messages with attachments and previews, edit and delete, read markers and unread counts, presence and typing, upload contexts, quotas and usage, authorized downloads.

Also from the [slice 4 plan](../knowledge/plans/slice-4.md#1-problem-outcome-and-scope): the chat frames (`message.*`, `read.updated`, `typing`, `presence`), the per-instance membership cache, persisted presence (`online_until` and `last_seen_at`), the `channel` kind of `entity.changed`, and the shared offline banner with actions disabled while offline (pending owner review).

Also from slice 3 ([slice 3 plan](../knowledge/plans/slice-3.md#1-problem-outcome-and-scope)): read-only chat after a relationship ends or a group is archived, "Deleted user" senders, only the direct channel for a provisional student (T-44), erasing a provisional period's chat messages when a reclaim drops the link, and `/tutor/settings` with the rest of the Teaching settings page for quotas and usage (pending owner review).

## Slice 7: video lessons

FR-5.1 to FR-5.3; recordings (FR-5.4 to FR-5.6) come after the MVP.

- **Probe first:** run the BigBlueButton probe on a remote test server (embedding on every supported browser, and the pilot host's capacity on 4 vCPU / 8 GB with two simultaneous one-to-one lessons with webcams, [ADR-0013](decisions/0013-pilot-deployment.md)) and record the results in the [research file](../knowledge/research/bigbluebutton-2026-10-20.md).
- **Then build:** the BigBlueButton and fake adapters with shared contract tests, meetings and join links, the callbacks and reconciliation that lesson status needs, and the embedded room with the new-tab fallback.

## Slice 8: library and builder

FR-6, FR-7. Folders, materials, templates, all eight block types, normalisation version 1, restricted Markdown rendering, and the builder with preview.

Before the builder pages land, the frontend splits the builder's vendor code into its own chunk and loads only the active language, to stay within [NFR-2.4](requirements/non-functional.md); route-level splitting is done in slice 4 (moved there by the slice 4 plan, pending owner review).

## Slice 9: issuing, completing and checking

FR-8, FR-9, FR-10. Issues with snapshots, deadlines and answer policies, the student assignment page with autosave, submission and automatic scoring, the review queue and review screen with corrected files, returning and resubmitting, the answer-key contract test, and homework emails.

Also from the [slice 4 plan](../knowledge/plans/slice-4.md#1-problem-outcome-and-scope): the `issue` and `submission` kinds of `entity.changed` (pending owner review).

Also from slice 3 ([slice 3 plan](../knowledge/plans/slice-3.md#1-problem-outcome-and-scope)): closing unsubmitted work when a relationship ends, erasing a provisional period's submissions when a reclaim drops the link, and the student card's Homework tab (pending owner review).

## Slice 10: results, home pages and hardening

FR-11.1, FR-15; analytics (FR-11.2 to FR-11.4) come after the MVP.

- **Product:** results for students, including 30 days of read-only results after a tutor's deletion; Today with the first-run checklist; Home with the tutor filter and `GET /home` (the read-only results and `GET /home` from slice 3, pending owner review).
- **Phone layouts:** dedicated phone layouts for every page and the phone navigation frame, replacing the interim one-column fallback ([ADR-0010](decisions/0010-quiet-visual-language.md)).
- **Hardening:** the accessibility pass, performance baselines and budgets for the non-functional requirements, and all four pilot scenarios passing in their MVP form.
- **Pilot deployment** ([ADR-0013](decisions/0013-pilot-deployment.md)): WAL-G backups of the application VM's PostgreSQL with a 30-day window, the encryption key kept off the VM and a restore tested on a fresh VM from it, the non-superuser database role and bounded PostgreSQL logs, the bounce intake from Yandex Postbox's Data Streams events, the host firewall and loopback-only PostgreSQL, and confirmation that Postbox keeps message logs for at most 30 days, before the first deployment.
- **Production serving** (before OVC is first deployed, at the latest here): the reverse proxy reproduces the page headers from `frontend/security-headers.ts`, caches hashed assets for a long time, compresses responses, answers the Go server's own protocol errors, redacts token segments in its access logs and never forwards `/__test__/`; Home stops polling the database-pinging readiness route; icons grow to 20 px on phones; the log queue keeps warnings and errors ahead of access records under a flood ([slice 1 review](../knowledge/reviews/slice-1-review.md#carried-over)).

Also from the [slice 4 plan](../knowledge/plans/slice-4.md#1-problem-outcome-and-scope): the undeliverable mark from the bounce intake and its clearing by a preference save, and the reverse proxy's redaction of the `token` query parameter of `/unsubscribe` and `/api/v1/unsubscribe` (pending owner review).

Also carried over from the [slice 2 review](../knowledge/reviews/slice-2-review.md): language choices on public pages saved in the order they were made; a backend load test on the pilot deployment repeating the slice 2 sign-in and avatar measurements; a real SMTP connection pool under load; and leaks across real process start and stop (pending owner review).

## After the MVP

Lesson recordings (FR-5.4 to FR-5.6, with their storage email in FR-13.2) and homework analytics (FR-11.2 to FR-11.4), moved out of the MVP by the owner on 2026-10-22 to reach it sooner, come first; the recording settings, quota columns, `recording_copy` outbox lane and `ovc-admin tutors set-quota --recordings` built in slice 2 stay in place, unused, until then, and the data model, files, accounts and non-functional documents and the BigBlueButton research file that still describe recordings and analytics without an after-the-MVP note are corrected when they are planned (pending owner review). Correcting a mistyped email of an unconfirmed account also comes after the MVP (from the slice 3 plan, pending owner review). Then the [payment ledger](proposals/0001-payment-ledger.md), attendance from provider events, timed tests and multiple attempts, accounts with both roles, an administration web interface, push notifications, calendar export, thumbnails, malware scanning, and several API instances with a cross-instance broadcaster beyond PostgreSQL `NOTIFY`. Without a slice: the dispatcher's `NOTIFY` wake-up, when a measurement asks for it, and `entity.changed` to students when a tutor edits their own profile, when a page needs it (both from the slice 4 plan, pending owner review). Each starts as a [proposal](proposals/README.md) when it is designed but not scheduled.
