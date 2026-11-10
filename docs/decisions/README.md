# Architecture decision records

Status: amended 2026-10-20 with ADR-0004 to ADR-0012, and 2026-10-22 with ADR-0013 and with ADR-0014 and its notes on ADR-0005 and ADR-0009, and 2026-10-26 with ADR-0015 and its notes on ADR-0006 and ADR-0008 and with ADR-0016, and with ADR-0017, pending owner review; amended 2026-10-27 with ADR-0018 (pending owner review); amended 2026-11-10 with ADR-0022 and its note on ADR-0019 (pending owner review); pending line-by-line owner review.

One record per significant decision, in the MADR shape: context, decision, alternatives considered, consequences, where it is applied. Records are numbered in writing order and never renumbered; a record that changes an earlier one says so in its header (`Supersedes` / `Superseded by`), and the earlier record is kept with its status updated ([ADR-0001](0001-record-architecture-decisions.md)).

| Record | Decision | Status |
|---|---|---|
| [ADR-0001](0001-record-architecture-decisions.md) | Record significant decisions as numbered MADR files; decisions win over specifications | accepted |
| [ADR-0002](0002-implementation-stack.md) | FastAPI, PostgreSQL and React with uv, pnpm, ruff, pyright strict and Playwright | accepted; backend and Python tooling superseded by ADR-0012 |
| [ADR-0003](0003-embedded-video-provider.md) | Embed an open-source conferencing provider behind a meeting provider adapter; never build a media stack | accepted; primary provider superseded by ADR-0004 |
| [ADR-0004](0004-bigbluebutton-primary-provider.md) | BigBlueButton is the primary provider; join links issued per request; recordings served only through OVC | accepted |
| [ADR-0005](0005-tutor-centric-domain.md) | One role per account; tutor–student relationships and tutor-owned groups; relationship-scoped authorization | accepted; relationship states and the student's access rule superseded in part by ADR-0014 |
| [ADR-0006](0006-background-worker-and-email.md) | A worker process with a PostgreSQL outbox and scheduled jobs; email over SMTP as the external channel | accepted; amended by ADR-0015 with web push as a second external channel (pending owner review) |
| [ADR-0007](0007-recurring-lessons.md) | Weekly series in the tutor's timezone with materialised occurrences and scoped edits | accepted |
| [ADR-0008](0008-hosting-and-data-residency.md) | All user data hosted in Russia; S3-compatible storage in the MVP; no foreign data processors | accepted; amended by ADR-0015 with the web push exception (pending owner review) |
| [ADR-0009](0009-owner-approved-exceptions-and-visibility.md) | Owner-approved exceptions (credentials in URLs, adapter state in PostgreSQL, closing slow sockets), product refinements, and tutor-only scores | accepted; the first bullet of item 4 (an emailed invitation confirms the email) superseded by ADR-0014 |
| [ADR-0010](0010-quiet-visual-language.md) | Quiet text-first visual language (Golos Text, Literata, olive accent, hairline rules), a sidebar for both roles, computers first | accepted |
| [ADR-0011](0011-tiered-verification.md) | Tiered verification: checks for trivial changes, one standard reviewer (plus a security reviewer when the security trigger is met) for routine changes, one fresh reviewer per changed layer when a slice completes, and at most one re-review after fixes (amended 2026-10-22, pending owner review) | accepted |
| [ADR-0012](0012-go-backend.md) | Go backend: huma on net/http, pgx with sqlc, goose migrations, slog, golangci-lint; frontend unchanged | accepted |
| [ADR-0013](0013-pilot-deployment.md) | Pilot deployment for two tutors with one-to-one lessons: one Yandex Cloud VM with PostgreSQL and WAL-G backups, a 4-vCPU VK Cloud BigBlueButton host, Yandex Postbox email | accepted |
| [ADR-0014](0014-students-join-through-their-tutor.md) | Students join through their tutor: one invitation link per card or a tutor code; a link made before confirmation is provisional and removable; the mailbox owner keeps or drops each link on reclaim; either party ends a relationship (pending owner review) | accepted |
| [ADR-0015](0015-web-push.md) | Web push from an installable web app in slice 10, with content-free payloads, session-bound subscriptions and push-service hosts fixed in code; off until the legal assessment before the pilot; a messenger bot later (pending owner review) | accepted; not enabled until the legal assessment and the decision record on minors are recorded |
| [ADR-0016](0016-modular-monolith.md) | A modular monolith: the API and the worker from one codebase and one database, enforced layers, the outbox and `LISTEN/NOTIFY` in PostgreSQL, one VM for the pilot, and the signals that justify extracting a service (pending owner review) | accepted |
| [ADR-0017](0017-dependency-release-age.md) | A three-day release age for dependencies until the first deployment, enforced by `pnpm lint`; 14 days, a review of the three-day window and updates only for a recorded reason from then on | accepted (pending owner review) |
| [ADR-0018](0018-targeted-checks-and-merge-gate.md) | Change checks per change from a table in the development guide, and the full local check run once per merge in a locked merge gate bound to the base commit and the tested tree; Markdown-only changes gate on the documentation and secret checks (pending owner review) | accepted |
| [ADR-0019](0019-durable-upload-cleanup.md) | Durable chat upload cleanup ownership, bounded automatic attempts, retained uncertainty and local operator recovery (pending wording review) | accepted, 2026-11-01; amended by ADR-0022 (pending owner review) |
| [ADR-0020](0020-durable-meeting-recovery.md) | Durable meeting admission, finite cleanup cycles, retained uncertainty and local operator recovery (pending wording review) | accepted, 2026-11-06 |
| [ADR-0021](0021-quiet-window-meeting-settlement.md) | Quiet-window settlement of uncertain meeting creation after 1 hour and provider absence (pending wording review) | accepted, 2026-11-07 |
| [ADR-0022](0022-library-upload-cleanup-adoption.md) | Library and prompt-image uploads adopt ADR-0019's durable cleanup journal with their own capacity of 1,024 unresolved intents (pending owner review) | accepted, 2026-11-10 |

## Writing a new record

Copy the skeleton below into `NNNN-short-title.md` with the next free number, fill every section, add a row above, and link the record from the specification sections that apply it.

```markdown
# ADR-NNNN: <title>

- Status: proposed | accepted, <date> | superseded by ADR-MMMM
- Reviewed: by the project owner, <date>
- Deciders: project owner
- Related: <other records, requirements, specifications>

## Context
## Decision
## Alternatives considered
## Consequences
## Where it is applied
```
