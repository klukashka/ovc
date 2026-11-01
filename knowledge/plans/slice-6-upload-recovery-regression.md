# Separate recovery deadline and progress regressions

Status: approved on 2026-11-01; independent security planning review passed after redesign; owner approved implementation on 2026-11-01 within this plan. No unchanged-tree rerun is authorized. Base: `5962f0466d56fbe33a541afb331fe679d73c969c`; the failure occurred in the uncommitted tutor-session integration tree based on this revision.

## Failure and intended correction

The complete backend race run failed only `TestUploadRecoveryAdvancesPastLockedCandidate`: processed zero instead of one, elapsed 115.521125 ms, with two candidate deadline errors. The test holds the first intent but changes the candidate budget to 50 ms and the whole recovery budget to 200 ms. Under the full run, the unblocked second candidate also exceeded 50 ms. Production budgets are one second per candidate and 30 seconds per pass. The changed tutor-session guards do not participate in this recovery path. The original failed run remains evidence; it is not an infrastructure-only failure and permits no unchanged-tree retry.

Separate the test's two responsibilities so successful database work is not required to finish inside an artificially short candidate timeout:

1. **Candidate deadline and cursor:** hold two ordered intent rows using real PostgreSQL transactions. Keep the existing 50 ms candidate and 200 ms pass budgets and the one-second overall watchdog. Require both candidates to fail, zero processed rows, unchanged intent state and a reset cursor after the candidate batch is exhausted. A negative control removing only the per-candidate timeout must fail: the outer pass expires at the first held row and leaves its cursor there instead of completing the batch. This preserves distinct evidence for the candidate timeout rather than accepting any outer cancellation as equivalent.
2. **Failure isolation and later progress:** use the normal production budgets, without custom shortened deadline overrides. Through the fixture's own connection, create a narrowly scoped `BEFORE UPDATE OF cleanup_requested` trigger on `upload_intents` that raises a real PostgreSQL error only for the first candidate when cleanup is requested. The allowed test-only worker-item error injection is defined in AGENTS.md and ADR-0009 item 8; it changes no other database behavior. Require the first item's error and rollback, exactly one later processed item and its durable cleanup outbox state. A separate negative control that stops at the first error must fail the later-progress assertion.

No application deadline, test watchdog, gate threshold or diagnostic policy is increased. The earlier draft proposed replacing the short deadline assertions with production-budget assertions; that alternative is withdrawn. The replacement keeps the short deadline evidence in its dedicated test and makes the independent progress test exercise an explicit failure without timing the completion of healthy database work at 50 ms. No retries, sleeps, fake repository, runtime patch, new deadline abstraction or broader interception exception are proposed.

Tier: standard; security trigger met through resource-deadline verification (T-20/T-41). A fresh full-security planning reviewer checks both negative controls, the distinction between candidate and pass cancellation, and the existing database-error exception before owner approval.

## Files, checks and acceptance

Change `backend/internal/services/files/upload_protocol_internal_test.go`, this plan, a verification record and a narrow pending-owner-review T-20/T-41 threat-model clarification distinguishing deadline evidence from item-failure isolation. Do not modify `RecoverUploads`, upload contracts, default budgets, production constructors or gate policy. Any needed small test helper remains local to the existing real PostgreSQL fixture and explicitly releases owned transactions; all setup and failures retain safe diagnostics.

After approval, run both negative controls in separate source copies, the two corrected real-database regressions, the complete backend race suite, static/generation checks, documentation and secret checks, independent standard/full-security readiness reviews, and a separate exact-tree gate. The reviewers verify that deadline evidence is preserved, the trigger matches the sanctioned worker failure-isolation exception, and neither negative control fails for an unrelated reason. Retain the original full-run failure and all later results. The tutor-session change then integrates the separately gated correction and receives affected final checks and its own gate.

The gate ceiling remains 260 seconds. If the correction cannot satisfy that ceiling, stop for a check-consolidation plan rather than silently relaxing it. This test change neither fixes nor accepts the separate daily-upload-accounting major.

## Security planning review

The initial production-budget-only proposal received a major planning finding and was withdrawn before implementation. The replacement follows the reviewer's specific two-test design and existing worker-item failure exception. It retains short candidate/pass deadlines and the overall watchdog while separating healthy database progress from those artificial deadlines. The original failing run and the rejected proposal remain recorded. The existing `TestUploadRecoveryRetainsCursorAcrossBudgetAndWraps` is not claimed fixed; any future failure must be retained and separately investigated.

A verification of the rewritten plan passed: the previous major planning finding is closed, both distinct negative controls are required, and no new exception to the testing policy is needed.
