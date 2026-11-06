# Presence shutdown failure: bounded diagnostic observation

Status: approved by the project owner on 2026-11-06. Implementation branch `feat/meeting-recovery-runtime`, base `8664fabbae992844c6be1aef986dcb5b30b9e508`. The approved meeting recovery storage is complete but not merge-ready: its final full backend run passed 2101 named tests/subtests and failed `TestPresenceOperationsRespectAdmissionAndShutdownDeadlines`. No gate or merge has run. The earlier full run passed all 2102 before the later cleanup-priority correction; that earlier result does not certify the final tree.

## Established evidence and question

The failed assertion at `backend/internal/api/chat_presence_capacity_test.go:120` observed one connected account lease still retained after shutdown, with unchanged online-until/last-seen values. The monitor observed unchanged sources, safe owned sleep/display assertions and open lid throughout 74.28 seconds. These observations do not establish the failure's cause or exclude ordinary scheduling/database contention. The presence implementation and test were not modified by the meeting change.

The existing test sets a 100-millisecond presence-operation deadline, deliberately blocks the other account's admission, releases its real row lock, then calls Shutdown under a one-second outer deadline. Shutdown waits for socket/loop work and calls ExtendPresence, which returns on the first failed account before stopping presence workers. This gives concrete hypotheses: an existing pending operation or fresh release exceeds its deadline; one account failure stops the other account's release; or the outer shutdown deadline expires. None is yet a demonstrated cause of the historical assertion. The in-memory structured presence diagnostics were not printed by that test and cannot be recovered from the completed process.

## Proposed scope

Use a separate disposable worktree containing the exact current application source and tests, then add passive observations only to the single existing test and a test-only helper. No production edit, changed database behavior, mock, request interception, gate, full E2E or merge is authorized.

Record at most four fixed snapshots: before the deliberately blocked admission, after releasing its row lock and before Shutdown, immediately after Shutdown, and after the existing scheduler advance. Each snapshot contains only:

- fixed stage and fixed account-position labels (connected/waiting), counts/booleans from the existing Gateway.Presence snapshots;
- elapsed admission/shutdown durations and whether the existing shutdown context is cancelled/deadlined;
- counts of the existing structured events presence.batch.failed, presence.release.pending, presence.announce.failed, presence.shutdown.incomplete and realtime.shutdown.incomplete, with separate counters for the existing safe error-kind categories and one unknown-category count;
- fixture pool acquired/idle/total connection counts and aggregate acquisition/cancellation counters.

Use exact predefined fragments with the existing synchronized log buffer; do not print raw log lines, arbitrary strings, account/provider/database identifiers, URLs, credentials, SQL text, bodies, errors or traces. Do not introduce a goroutine, timer, database query, additional wait or retry. All existing assertions, the 100-millisecond operation and one-second shutdown deadlines, normal Go package/test parallelism and race detection remain unchanged. Diagnostic scheduling overhead is a residual: a passing run alone will not establish a fix or historical causality.

## Order and acceptance

1. Independent security-profile planning review, then owner approval because the failure is outside the meeting implementation scope and a new run follows an unexplained failed assertion.
2. Prepare the isolated copy; show the exact test-only diff and verify application source equality against the implementation tree. Run formatting, go vet and strict lint on the diagnostic change before execution. Do not silently relax a diagnostic check that fails.
3. Refresh other chats/process load; run exactly one `go -C backend test -race -json ./...` through the sanctioned root dev-env wrapper, with the existing owned sleep protection/source identity monitor. Retain normal parallelism to preserve the failed workload; no additional targeted or repeated run is implied.
4. Save private raw test output, source identities, monitor status and safe diagnostic snapshots. Preserve both previous full-run outcomes. Evaluate the fixed hypotheses only against observed evidence; another failure or a non-reproduction does not grant another run.
5. Prepare a concrete fix if supported, or a bounded next decision. An actual presence production/test semantic fix requires a new approved scope. This diagnostic result is not acceptance and authorizes neither the meeting merge nor a gate.

Tier: standard; security trigger T-18/T-19 diagnostics and T-20/T-21 presence lifecycle plus T-41 verification evidence. The implementation is a temporary local diagnostic artifact; normal independent standard/security review of its exact observation diff precedes execution, sequentially while another project is active. At most three working sessions overall and one heavy workload. No 260-second optimization, timeout increase or weakening of assertions.
