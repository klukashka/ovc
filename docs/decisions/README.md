# Architecture decision records

Status: amended 2026-10-20 with ADR-0004 to ADR-0008; pending line-by-line owner review.

One record per significant decision, in the MADR shape: context, decision, alternatives considered, consequences, where it is applied. Records are numbered in writing order and never renumbered; a record that changes an earlier one says so in its header (`Supersedes` / `Superseded by`), and the earlier record is kept with its status updated ([ADR-0001](0001-record-architecture-decisions.md)).

| Record | Decision | Status |
|---|---|---|
| [ADR-0001](0001-record-architecture-decisions.md) | Record significant decisions as numbered MADR files; decisions win over specifications | accepted |
| [ADR-0002](0002-implementation-stack.md) | FastAPI, PostgreSQL and React with uv, pnpm, ruff, pyright strict and Playwright | accepted |
| [ADR-0003](0003-embedded-video-provider.md) | Embed an open-source conferencing provider behind a meeting provider adapter; never build a media stack | accepted; primary provider superseded by ADR-0004 |
| [ADR-0004](0004-bigbluebutton-primary-provider.md) | BigBlueButton is the primary provider; join links issued per request; recordings served only through OVC | accepted |
| [ADR-0005](0005-tutor-centric-domain.md) | One role per account; tutor–student relationships and tutor-owned groups; relationship-scoped authorization | accepted |
| [ADR-0006](0006-background-worker-and-email.md) | A worker process with a PostgreSQL outbox and scheduled jobs; email over SMTP as the external channel | accepted |
| [ADR-0007](0007-recurring-lessons.md) | Weekly series in the tutor's timezone with materialised occurrences and scoped edits | accepted |
| [ADR-0008](0008-hosting-and-data-residency.md) | All user data hosted in Russia; S3-compatible storage in the MVP; no foreign data processors | accepted |

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
