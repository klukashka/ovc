# ADR-0007: Recurring lessons as a series with materialised occurrences

- Status: accepted, 2026-10-20
- Reviewed: pending line-by-line owner review
- Deciders: project owner
- Related: FR-4, [schedule specification](../../knowledge/specs/schedule.md), [ADR-0006](0006-background-worker-and-email.md)

## Context

Tutors teach the same student at the same local time every week. Tutors and students live in different timezones, and some timezones observe daylight-saving time. Individual lessons of a series are moved, cancelled and given their own descriptions, materials, assignments and recordings, so each one must be a real row that other data can reference.

## Decision

- A **lesson series** stores:
  - the participants (one student or one group);
  - weekdays and a local start time;
  - the duration;
  - the tutor's IANA timezone at creation;
  - a start date and either an end date or a lesson count.
- **Occurrences are materialised as lesson rows**, for the full series when it has a count or ends within 12 weeks, and otherwise on a rolling 12-week horizon extended by the worker. Each occurrence's UTC start is computed from the local time in the series' timezone, so a daylight-saving change keeps the local time and moves the UTC instant.
- **Edits are scoped.** "This lesson" changes one occurrence, which then keeps its own values and no longer follows later series edits for those fields. "This and following" splits the series at that occurrence: the original ends before it, and a new series starts with the new values. Past occurrences never change.
- **Cancelling** an occurrence keeps the row with status `cancelled`. Cancelling the series cancels future occurrences.

## Alternatives considered

- **A recurrence rule expanded only when read:** no rows for individual occurrences to reference, and exceptions must be stored separately; every query must expand rules.
- **Fixed UTC times:** lessons would shift by an hour twice a year in timezones with daylight-saving time.

## Consequences

- Recurrence logic (local-to-UTC conversion across daylight-saving changes, series splitting) is intricate and gets dedicated unit tests, which `AGENTS.md` allows for this kind of logic.
- The worker owns horizon extension; a stopped worker eventually leaves long-running series without future occurrences, which monitoring must flag.

## Where it is applied

[Schedule](../../knowledge/specs/schedule.md), [data model](../../knowledge/specs/data-model.md), [API](../../knowledge/specs/api.md#schedule).
