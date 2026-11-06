# Meeting creation coordinator verification

Status: implemented and targeted tests passed on 2026-11-06; independent reviews and the exact-tree gate pending. Base: `088ca54`.

Scope: section 1 of the [approved local plan](../plans/slice-7-local-completion.md), the remaining creation and stale-business coordination. Tier: standard. Security trigger: met (provider authority, cleanup ownership, finite work). No public route, process registration of Create or provider activation is added by this change.

## Threats

T-20: five planning rounds, a 15-second call budget, a 10-second evidence budget, at most 100 abandoned rows per scanner pass. T-34: a returned or uncertain generation survives purge without names or capabilities. T-25 and T-41: generation binding is retained; fake evidence proves nothing about the remote provider.

## Acceptance

- `TestMeetingCreateOwnership`: dispatch persisted before Create, one room for concurrent callers, the await deadline measured by a separate instance, stale replacement, both cancel and commit orderings, provider failure without retry, lost authorization, late acknowledgement and its single final cycle, capacity without leaks, entropy failure, absent-room replacement and scanner closure.
- `TestMeetingRecoveryPurgeRace`: a generation returned after purge is cleaned through its orphan ledger; an uncertain orphan survives receipt cleanup with its capacity.

Section 1's remaining named tests are already on main: `TestMeetingRecoveryWorker`, `TestMeetingRecoveryCapacity`, `TestMeetingRecoveryCursor`. `TestMeetingRecoveryAdmin` is the prefix of five tests: `TestMeetingRecoveryAdminRevisionAndRefusals`, `TestMeetingRecoveryAdminConcurrentRetry`, `TestMeetingRecoveryAdminPagination`, `TestMeetingRecoveryAdminSettledAndSubsequentCycle` and `TestMeetingRecoveryAdminStartupFailureAudit`.

The orderings use a clock gate injected into the real shared fake and a provider diagnostic that cancels the caller after the provider transaction commits. No provider response is replaced.

## Owner decisions of 2026-11-06

- A limit of 64 unsettled obligations per tutor, refused with 503 before any capacity reservation; the global 4096 stays (`TestMeetingCreateOwnership/the_tutor_limit_refuses_before_any_reservation`).
- A per-lesson pause of 2 minutes after an uncertain outcome, decided 2026-11-07 (`.../a_fresh_uncertain_outcome_pauses_only_its_lesson`, `.../an_outcome_that_is_not_uncertain_starts_no_pause`). The remaining exposure is in the [known limitations](slice-7-known-limitations.md#uncertain-obligations-never-close).
- The provider's room name is the neutral constant `OVC lesson`, never the lesson title (`TestMeetingCreateOwnership/the_room_name_is_the_neutral_constant`).

## Review findings fixed

- Caller cancellation no longer creates uncertainty: Create runs detached under its own 15-second budget (`.../caller_cancellation_during_Create_creates_no_uncertainty`; a Create past its budget stays uncertain).
- A definitive input refusal cannot reach the provider: the room name is a valid constant. Every provider failure stays uncertain, because the contract does not prove that a refusal left no room.
- A clock stepped back cannot strand a close: the close records the later instant, and a refused close is an error that rolls back (`.../a_clock_stepped_back_cannot_strand_a_close`, `TestMeetingPersistenceTransitions/backwards_time_cannot_strand_a_close`). This replaces the earlier assertion that a close with a backward instant refuses.
- The scanner's `closed` count covers only rows it closed (`.../the_scanner_counts_only_rows_it_closed`).

- A cancel during the reserving transaction burns no slot (`.../a_cancel_during_the_reserving_transaction_burns_no_slot`), and two joins at the last slot admit one (`.../two_joins_at_the_last_slot_admit_one`).

Blank lesson titles are left alone: the [schedule specification](../specs/schedule.md#titles-and-descriptions) requires a length of 1 to 120 and no control characters, and it does not settle whitespace-only titles. With the neutral name they no longer reach the provider.

## Negative controls

Each ran in a separate disposable copy of the backend, with the original assertions and race detection; none entered the implementation tree.

- Publication no longer stops on a refused guard: `TestMeetingCreateOwnership/lost_authorization_records_but_never_publishes` fails with the refusal replaced by success.
- Cleanup settles started and uncertain evidence and the settlement predicate admits it: `TestMeetingCreateOwnership/an_older_creating_row_closes_and_keeps_ownership` fails with "absence settled uncertain evidence".
- Create runs under the caller's context again: `.../caller_cancellation_during_Create_creates_no_uncertainty` fails with an uncertain ledger.
- The tutor limit check is disabled: `.../the_tutor_limit_refuses_before_any_reservation` fails because the sixty-fifth obligation is admitted.
- The reserving transaction runs under the caller's context again: `.../a_cancel_during_the_reserving_transaction_burns_no_slot` fails because no meeting row and no Create exist.
- The test guard's tutor profile lock is removed: `.../two_joins_at_the_last_slot_admit_one` fails because both joins are admitted while the capacity row is held.
- The uncertain pause check is disabled: `.../a_fresh_uncertain_outcome_pauses_only_its_lesson` fails because the paused call writes.

## Limitations

Provider failure retains capacity indefinitely, as ADR-0020 states. A permanently failing abandoned row stays at the head of the scanner's page; at most 100 such rows could hide others, and each failure stays visible through the failed job. The publication guard and closure take no capacity lock.
