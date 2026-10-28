# Revisit of `tools/check-changed.sh` at the close of slice 5

Written on 2026-10-29 for the project owner. [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md) deferred a script that derives the change checks from the diff, `tools/check-changed.sh`, and set its revisit at slice 5's close ([slice 5 plan](../plans/slice-5.md)), because the merge gate already catches a misjudged change-check table before `main`. This record is that revisit. It changes no check and implements nothing; the decision stays with the owner.

## Evidence from slice 5

The merge gate's attempt log and its command log folders in the per-user `ovc-e2e` cache folder, read only for their result and timing lines, hold 48 gates of the full run from 2026-10-26, when the gate was introduced, to 2026-10-28 (the [slice 5 gates record](check-budget-2026-10-29-slice-5-gates.md) gives the passing ones), 17 of them failed. The failures that their result lines name were:

- tests that failed and then passed, which the gate's rule treats as flaky-test bugs: Playwright tests in the realtime, students, join requests and schedule specs (the last one the date-dependent Schedule test the [slice 5 review](../reviews/slice-5-review.md) records), and Go tests such as `TestMismatchCounterRevokesAtFiveUnderConcurrency`;
- infrastructure or contention between the lanes: the storage contract tests failing within milliseconds, a listener package that ran into the 600-second test timeout, and the cold gate's `too many clients` named in the [critical path record](check-budget-2026-10-28-gate-critical-path.md);
- lane failures before the end-to-end suites whose result lines name no test (for example the first two attempts of the lesson effects change).

None of the failures that the result lines name is a check that the change's own change checks would have run had its table rows been chosen differently: the failing tests were either flaky or failed for reasons outside the change. The logs do not record which change checks each implementing agent ran, so this is weak evidence that the table was applied correctly, not a proof. The cost of a misjudged set stayed what ADR-0018 accepted: a failed gate and an after-fix round, never a broken `main`.

During the slice the change-check table itself was amended (for example the row for `frontend/e2e/` gained `tools/check-merge-gate.sh` when `frontend/e2e/run-lock.ts` changes), and the gate's list grew; a script would have had to change in the same changes, with self-tests of its own like those of `tools/check-merge-gate.sh`.

## Options for the owner

1. **Keep the script deferred (recommended).** Revisit it when a gate failure is traced to a change-check set chosen wrongly from the table, or when a hosted CI service is adopted (AGENTS.md's change workflow names that change), whichever comes first. No cost now; the gate keeps catching misjudged sets before `main`.
2. **An advisory script.** `tools/check-changed.sh` prints the table rows that the diff against `main` touches and the commands they name, without running them, and its self-test proves the mapping for each row. It removes the table lookup's judgement for implementing agents, but the table and the script must change together, and a row the script maps wrongly would mislead with the appearance of authority.
3. **A mandatory script.** The script runs the change checks it derives and its output becomes the readiness evidence. It is the most work and makes the script part of the verification policy, so it needs an amendment of ADR-0018 and of the [development guide](../../docs/development.md#change-checks-and-the-merge-gate), and it meets the security trigger as agent verification policy.

Recommendation: option 1, because slice 5 shows no failure that the script would have prevented, while the end-to-end shortening ([plan](../plans/e2e-shortening.md)) addresses the gate cost that slowed the slice.
