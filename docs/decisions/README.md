# Architecture decision records

Status: draft written 2026-10-20; pending line-by-line owner review.

One record per significant decision, in the MADR shape: context, decision, alternatives considered, consequences, where it is applied. Records are numbered in writing order and never renumbered; a record that changes an earlier one says so in its header (`Supersedes` / `Superseded by`), and the earlier record is kept with its status updated ([ADR-0001](0001-record-architecture-decisions.md)).

| Record | Decision | Status |
|---|---|---|
| [ADR-0001](0001-record-architecture-decisions.md) | Record significant decisions as numbered MADR files; decisions win over specifications | accepted |
| [ADR-0002](0002-implementation-stack.md) | FastAPI, PostgreSQL and React with uv, pnpm, ruff, pyright strict and Playwright | accepted |
| [ADR-0003](0003-embedded-video-provider.md) | Embed Jitsi Meet (BigBlueButton as alternative) behind a meeting provider adapter; never build a media stack | accepted |

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
