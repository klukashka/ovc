# Roadmap

Status: rewritten 2026-10-20 for the tutor-centric product, phone layouts added to slice 10 and slice reviews added the same day; pending line-by-line owner review.

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
  - a `uv` project and a FastAPI application factory with typed settings;
  - an async database session, and Alembic with a comment-free migration template;
  - health routes, the structured logging bootstrap and request identifiers;
  - the package-layer architecture test and the integration-test database fixture;
  - an empty worker process with `tick(now)` and the fake clock.
- **Frontend:** a `pnpm` project with Vite, React and strict TypeScript; Tailwind over the design tokens; self-hosted fonts; the router and i18n setup with `ru` and `en`; the typed API client; the public and role frames; ESLint and Prettier; a Playwright smoke test.
- **Infrastructure:** Docker Compose for PostgreSQL and a local S3-compatible server. Security scanning tools are selected and registered.
- **Done when** the health routes answer, the frontend renders its frames in both languages against them, and every slice-1 check passes.

## Slice 2: accounts, sessions and email

FR-1, FR-16, FR-17. Registration with consent, email confirmation, sign-in throttling, sessions with refresh rotation, password reset and change, sign out everywhere, profile and preferences, avatars through storage, account deletion and purge, and `ovc-admin`. The outbox, the worker dispatcher, the mail adapter (SMTP and in-memory sink) and the first email templates are built here.

## Slice 3: tutors, students and groups

FR-2, FR-3. Student cards, invitations, tutor code and join requests, relationship end and reactivation, groups, and the student's tutors page. Relationship-scoped authorization guards and their denial tests.

## Slice 4: realtime and in-app notifications

FR-13.1, FR-13.3. The WebSocket gateway with first-frame authentication, the connection registry and broadcaster, `notification.new` and `entity.changed` frames, the notifications panel, email preferences and one-click unsubscribe.

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

## Slice 9: issuing, completing and checking

FR-8, FR-9, FR-10. Issues with snapshots, deadlines and answer policies, the student assignment page with autosave, submission and automatic scoring, the review queue and review screen with corrected files, returning and resubmitting, the answer-key contract test, and homework emails.

## Slice 10: results, home pages and hardening

FR-11, FR-15.

- **Product:** results for students; analytics per issue and progress per student; Today with the first-run checklist; Home with the tutor filter.
- **Phone layouts:** dedicated phone layouts for every page and the phone navigation frame, replacing the interim one-column fallback ([ADR-0010](decisions/0010-quiet-visual-language.md)).
- **Hardening:** the accessibility pass, performance baselines and budgets for the non-functional requirements, and all four pilot scenarios passing in full.

## After the MVP

The [payment ledger](proposals/0001-payment-ledger.md), attendance from provider events, timed tests and multiple attempts, accounts with both roles, an administration web interface, push notifications, calendar export, thumbnails, malware scanning, and several API instances with a cross-instance broadcaster. Each starts as a [proposal](proposals/README.md) when it is designed but not scheduled.
