# Slice 7: quiet-window settlement of uncertain meeting creation

Status: approved by the owner on 2026-11-07 against main 3a996bb, with a 1-hour window and no force-settle command.

## Problem and outcome

Under [ADR-0020](../../docs/decisions/0020-durable-meeting-recovery.md), `started`, `uncertain` and legacy `uncertain` ledger entries never settle. They count against the tutor's limit of 64 and the global limit of 4096 forever, which the [known limitations](../reviews/slice-7-known-limitations.md#uncertain-obligations-never-close) record as blocking real-provider activation. The outcome is a deterministic settlement procedure, [ADR-0021](../../docs/decisions/0021-quiet-window-meeting-settlement.md), that releases these obligations once the provider is reachable and the dispatch is old enough, while keeping work bounded and keeping cleanup ownership of any room that appears.

## Contract

- Migration 00031 adds `dispatched_at` (set by Begin, backfilled from `updated_at`), `settlement` (`revoked`, `confirmed_end`, `quiet_absence`, backfilled for settled rows) and `reconcile_cycle_used`, and replaces the settlement check so that `quiet_absence` requires unknown-outcome evidence without a create time and `settled_at` at least 1 hour after `dispatched_at`.
- `SettleMeetingRecovery` accepts unknown-outcome evidence only with an evaluation instant at least 1 hour after `dispatched_at` and not later than the settlement instant. The worker's claim instant is the evaluation instant.
- The cleanup executor settles unknown-outcome evidence after confirmed absence or exact-generation End only when the window has passed at the claim instant.
- The scanner and `QueueMeetingRecoveryCycle` admit one automatic reconciliation cycle per unknown-outcome obligation after the window; the final-cycle allowance stays with returned evidence. Operator retries apply the same settlement rule.
- The coordinator does not dispatch a Create whose permit is older than the 15-second Create budget, and makes one best-effort End when a Create returns after settlement.
- Diagnostics: INFO `meeting.recovery.settled` (`outbox_id`, `settlement`), WARNING `meeting.room.created_after_settlement` (`ended`, error `kind`), `settlement` on `meeting.cleanup.evaluated`.
- `ovc-admin meetings status` and `inspect` print `settlement`, `dispatched_at`, `reconcile_cycle_used` and `operator_action`.

## Checkable acceptance

All tests use real PostgreSQL, the shared fake provider and the worker through `Tick`, without mocks; `TestMeetingQuietSettlement` lives in `backend/internal/repositories/meeting_quiet_settlement_test.go`.

1. An uncertain attempt stays unsettled after its first cycle; a scan one second before the window queues nothing; a scan at the window queues one cycle that settles it as `quiet_absence`; global capacity and the tutor count drop to zero; `meeting.recovery.settled` is logged with safe fields only.
2. A room that appears after the first cycle is ended by its exact generation and the entry then settles.
3. With the provider unavailable, the reconciliation cycle exhausts without settling and `status` reports `operator_action=true`; an operator retry with the provider available settles it.
4. An operator cycle before the window ends unresolved.
5. A crashed `started` attempt and legacy `uncertain` evidence settle after the window.
6. A cycle claimed before the window ends unresolved, spends the reconciliation cycle once, and the scanner and the repository refuse another automatic cycle; that completion later than the window cannot settle evidence evaluated before it is proven by the repository fence of criterion 9.
7. A dispatch instant after the evaluation instant never settles.
8. A late acknowledgement after settlement changes neither revision, state, settlement nor capacity; a Create returning after settlement through the coordinator is ended, not published, and logged.
9. The repository refuses a missing, early or future evaluation instant and observed generations for unknown-outcome evidence; the check constraint refuses forged early, mismatched, unmarked and undispatched settlements.
10. A permit older than the Create budget dispatches no Create and leaves uncertain evidence.
11. `TestMeetingQuietSettlementMigration`: backfill and refused downgrade with evidence, and an empty round trip.
12. `TestMeetingQuietSettlementReadmitsTutor` in `backend/internal/api`: a tutor at 64 unsettled obligations is refused 55 minutes after dispatch, a worker tick then still settles nothing, and after the window one worker tick readmits the tutor.
13. Specifications, the data model, ADR-0021, the threat model and the known-limitations entry change in the same commit; change checks and the merge gate pass.

## Verification

Standard tier with the security trigger met (provider effects, worker scheduling, operator command semantics, admission capacity, retention, an ADR amendment and the threat model). A security-profile planning review ran before approval; its findings (evaluation instant fenced in SQL and the constraint, late Create after settlement, permit age before dispatch, backfill wording, spent reconciliation, INFO settlement event, threat-model text) are part of this plan. Implementation gets a standard reviewer and a security-profile reviewer over the complete diff, then the merge gate.
