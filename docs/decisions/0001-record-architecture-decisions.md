# ADR-0001: Record architecture decisions

- Status: accepted, 2026-10-20
- Reviewed: pending line-by-line owner review
- Deciders: project owner
- Related: [AGENTS.md](../../AGENTS.md#documentation)

## Context

OVC makes several consequential early choices (stack, conferencing, authentication, layering). Future contributors, human or agent, need to know why each was made so they neither reopen settled questions nor reverse a decision without knowing its trade-offs. Conversations that produced a decision are not available to them.

## Decision

Record every significant decision as one Markdown file in `docs/decisions/`, in the MADR shape: context, decision, alternatives considered, consequences, where it is applied. Records are numbered in writing order and never renumbered. A record that changes an earlier one says so in its header (`Supersedes` / `Superseded by`), and the earlier record is kept with its status updated. Undecided or unscheduled designs are written as [proposals](../proposals/README.md) first.

## Alternatives considered

- **Decisions in commit messages or conversations only:** not discoverable, and lost for a reader without that context.
- **One running design document:** decisions blur into description, and reversals leave no trace.

## Consequences

- Decisions are discoverable, dated and attributable; reversals are explicit.
- A small writing cost per decision.
- When a specification and a decision record disagree, the decision record wins until it is superseded.

## Where it is applied

[Decision index](README.md), [knowledge index](../../knowledge/README.md), [AGENTS.md](../../AGENTS.md#project).
