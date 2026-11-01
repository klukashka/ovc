# Upload recovery regression verification

Status: original correction verified but not merged; approved cursor extension implemented, runtime checks and independent combined review pending. Current base: `bf6ea79d5c999df1a8ad077cc25d172521ff4955`. Historical checks below concern the original correction based on `5962f0466d56fbe33a541afb331fe679d73c969c`, not this expanded tree. The owner approved the [redesigned plan](../plans/slice-6-upload-recovery-regression.md) on 2026-11-01 after a fresh security planning review and verification of its corrections. Standard tier; security trigger met through T-20/T-41 deadline and worker-item isolation evidence.

## Failure retained

The full backend race run of the separate tutor-session integration failed the unchanged `TestUploadRecoveryAdvancesPastLockedCandidate`: processed zero, 115.521125 ms, with two candidate timeouts. Its 50 ms candidate budget inadvertently required the healthy second database transaction to finish within 50 ms. The full run was not retried. The original log is `/private/tmp/ovc-tutor-session-backend.log`; the tutor-session verification record retains the exact failure and other package results. This separate test correction does not change any session guard or application behavior.

## Replacement evidence

`TestUploadRecoveryBoundsEachLockedCandidate` locks both intent rows with real PostgreSQL. It retains the 50 ms candidate, 200 ms pass and one-second overall bound, requires deadline failure, zero processed rows, no durable mutation/outbox and cursor wrap after the batch. Its negative control must remove only the per-candidate timeout; the pass then expires at the first row and the cursor assertion must fail.

`TestUploadRecoveryContinuesAfterOneCandidateFails` uses the existing production budgets and the sanctioned worker-item failure exception: its isolated database trigger raises a real PostgreSQL exception only when the first intent is revoked. It requires SQLSTATE P0001, rollback of the first intent, exactly one later processed candidate, one durable queued cleanup and cursor wrap. Its negative control must stop after the first error and fail later-progress assertions. Neither test introduces mocks, sleeps, retries, new deadline interfaces or production-budget changes.

The threat model differentiates these evidence boundaries with wording pending owner review. The original production-budget-only proposal was rejected during planning and replaced before source implementation; no exception to the prohibition on retry/timeout masking is requested or used. The separate cursor-across-budget test remains unchanged and is not declared fixed by this change.

Source formatting and `git diff --check` passed. Runtime checks, the two negative controls, full backend race/static/generation checks, documentation/scanner checks, fresh standard/full-security readiness reviews and the separate exact-tree gate/finish are pending. Heavy execution is serialized with the other active project; no unrun check is counted as passed. The 260-second ceiling remains unchanged.

## Independent source reviews

Separate fresh standard and full-security reviewers inspected the complete four-file diff against the base above and found no blocker/major/minor. They verified the two distinct failure mechanisms, SQLSTATE discrimination through a precise error interface, rollback/snapshot assertions, cursor wrap, the restricted test-only trigger and owned transaction/database cleanup. T-20/T-41 were examined; production authentication, diagnostics, credentials, rendering and storage behavior are unchanged. Both explicitly left runtime and negative controls unverified. No reviewer ran workloads or accepted limitations.

A source-only coordinator check replaced an initially added driver import with the precise `error` plus `SQLState() string` interface before either final verdict, preserving the assertion without introducing a forbidden package-layer edge. No linter rule or architecture restriction was relaxed.

## Initial runtime evidence

The unchanged corrected source passed `go test -race -count=1 ./internal/services/files -run 'TestUploadRecovery(BoundsEachLockedCandidate|ContinuesAfterOneCandidateFails)'`, exit 0, 2.521 seconds, through the main checkout's sanctioned environment wrapper with `caffeinate -i`. Both negative controls ran in separate source copies, preserving the implementation tree:

- Removing only the per-candidate timeout failed `TestUploadRecoveryBoundsEachLockedCandidate` at its intended whole-batch/cursor assertion, exit 1, package 1.278 seconds.
- Returning after the first candidate error failed `TestUploadRecoveryContinuesAfterOneCandidateFails` at its intended later-cleanup/state assertion, exit 1, package 1.203 seconds.

These are expected sensitivity failures, not failed positive checks and not retries. Logs are `/private/tmp/ovc-recovery-targeted.log`, `/private/tmp/ovc-recovery-negative-deadline.log` and `/private/tmp/ovc-recovery-negative-progress.log`. The copies are `/private/tmp/ovc-recovery-negative-deadline` and `/private/tmp/ovc-recovery-negative-progress`. The full backend run and remaining checks are still pending until recorded.

## Original correction change checks

On the corrected source, the full `go test -race -count=1 ./...` through the sanctioned main-checkout wrapper and `caffeinate -i` passed, exit 0; API 77.425 seconds and files-service tests 16.605 seconds. The actual full log is `/private/tmp/ovc-recovery-backend.log`. This is a new corrected-tree run, not an unchanged retry of the tutor-session failure. `go vet ./...`, `../bin/golangci-lint run ./...` (zero issues), `../bin/sqlc -f sqlc.yaml diff`, the full-backend `gofmt -l` check and `git diff --check` all passed, exit 0. Offline frozen documentation-tool installation and `tools/docs/check-docs.sh` passed, rendering all six diagram documents; logs are `/private/tmp/ovc-recovery-docs-install.log` and `/private/tmp/ovc-recovery-docs.log`.

Final tier remains standard and the security trigger remains met. Both independent source reviews passed without findings; negative controls and the complete backend run supply the runtime evidence they explicitly left pending. There is no production behavior or browser-facing change, so new browser journeys are not applicable to this regression-only change; the full existing suite and remaining scanners/checks are deferred to the exact-tree merge gate. Gate and finish evidence will be retained outside the tested tree. The original full-run failure is retained and no slice-closure claim is made.

## Retained gate failures and approved extension

The original correction was committed as `8c80a6884d3e475f42d8c058a0da564281166726` and remains unmerged on its original branch. Its first exact-tree gate completed all 30 commands, including 944 passing end-to-end tests, in 266.5 seconds. The script reported passed, but the owner-required 260-second ceiling failed; no merge was performed. The complete report is retained in the repository's common Git directory as `ovc-merge-reports/8c80a68-recovery-budget-exceeded.log`.

The owner explicitly authorized one unchanged-tree repeat. That repeat executed 29 commands and failed the backend check at the unchanged `TestUploadRecoveryRetainsCursorAcrossBudgetAndWraps`; end-to-end execution did not run. The first pass retained the first candidate rather than the assumed second, and the next pass also timed out. The complete report is retained as `ovc-merge-reports/8c80a68-recovery-repeat-failed.log`. This is an assertion failure, not an infrastructure-only failure; no further unchanged-tree repeat is authorized. The first gate's passing runtime assertions do not establish that the second gate or expanded tree passed.

The owner approved the [cursor extension](../plans/slice-6-upload-recovery-cursor.md) after its security planning review. The new coherent branch starts from the separately merged quota clarification at `bf6ea79d5c999df1a8ad077cc25d172521ff4955`, preserving the original branch and commit. The two previously corrected regressions remain intact. `TestUploadRecoveryBoundsPassWithLockedCandidate` isolates the unchanged 75 ms outer deadline with the production candidate budget, unchanged state/outbox and one-second overall bound. `TestUploadRecoveryRetainsCursorAfterCancellationAndWraps` observes the isolated database's only lock waiter before caller cancellation, checks the retained first identifier and unchanged snapshots/outbox, releases the lock and requires exactly two later candidates followed by wrap and a three-candidate pass. Both use real PostgreSQL; owned transaction cleanup is registered before lock setup, and failed observation cancels and joins the recovery goroutine before fixture teardown.

No production source, budget, diagnostics or authorization contract changes. T-20/T-41 remain the security mapping, and the final tier remains standard with the security trigger met. Previously passed negative controls remain historical evidence for the unchanged candidate-deadline and failure-isolation tests. The expanded named regressions and three new negative controls completed as recorded below. Complete backend race/static/generation checks, docs/scanners and one fresh combined standard/full-security review remain pending. Heavy execution is serialized; only the targeted tests and negative controls have run for this extension. The new exact-tree gate and finish are deferred until these pass; its required ceiling remains 260 seconds.

## Cursor extension targeted runtime evidence

The expanded source passed the four named real-database regressions with `-race`, exit 0, package 2.992 seconds. Command, from the current branch's `backend/` directory:

```sh
caffeinate -i /Users/klukashka/Work/ovc/tools/dev-env.sh go test -race -count=1 ./internal/services/files -run 'TestUploadRecovery(BoundsEachLockedCandidate|ContinuesAfterOneCandidateFails|BoundsPassWithLockedCandidate|RetainsCursorAfterCancellationAndWraps)'
```

The complete output is `/private/tmp/ovc-recovery-cursor-targeted.log`. Three sequential negative controls used separate backend source copies without environment files, with the same wrapper, `caffeinate -i` and `go test -race -count=1 ./internal/services/files`:

- `/private/tmp/ovc-recovery-cursor-negative-pass`: replaced only `context.WithTimeout(ctx, s.deps.Uploads.Recovery)` with `context.WithCancel(ctx)` in `upload_recovery.go`; `-run '^TestUploadRecoveryBoundsPassWithLockedCandidate$'` failed at the intended overall-bound assertion, elapsed 1.012198709 seconds, zero processed and a candidate deadline error. Exit 1, package 1.899 seconds; log `/private/tmp/ovc-recovery-cursor-negative-pass.log`.
- `/private/tmp/ovc-recovery-cursor-negative-retention`: cleared `s.recoveryCursor` only inside the `bounded.Err() != nil` return branch; `-run '^TestUploadRecoveryRetainsCursorAfterCancellationAndWraps$'` failed at the retained-cursor assertion with `context canceled`, zero processed and the zero cursor. Exit 1, package 1.015 seconds; log `/private/tmp/ovc-recovery-cursor-negative-retention.log`.
- `/private/tmp/ovc-recovery-cursor-negative-wrap`: removed only the cursor reset inside `len(ids) < expireBatch`; the same cursor test failed at the next-pass wrap assertion with two processed, no error and the third identifier still retained. Exit 1, package 1.014 seconds; log `/private/tmp/ovc-recovery-cursor-negative-wrap.log`.

These are expected sensitivity failures, with no unrelated setup or compile failure, not retries or failed positive checks. The implementation tree's production source was never mutated. Source formatting and `git diff --check` passed. Full backend and remaining change checks, the fresh combined review and exact-tree gate remain pending; the targeted execution slot is released.

## Expanded correction independent review and full backend

A fresh independent reviewer applied the standard checklist and full security profile to all six changed files against `bf6ea79d5c999df1a8ad077cc25d172521ff4955`, including the extension and its integration with the original two regressions. The review passed without blocker, major or minor findings. It verified the actual positive/negative logs and isolated mutation diffs, observed-lock synchronization, cancellation/join before fixture teardown, durable snapshots/outbox and the exact continuation/wrap assertions. T-20/T-41 were examined; the other security-profile families and browser behavior are not affected. No production trust boundary changed. The reviewer ran no heavy workloads and did not count pending checks as passed.

The expanded tree passed `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go test -race -count=1 ./...` from `backend/`, exit 0. The complete log is `/private/tmp/ovc-recovery-cursor-backend.log`. `tools/check-tool-binaries.sh` passed, exit 0, in `/private/tmp/ovc-recovery-cursor-tools.log`. Procedural deviation: the coordinator launched the full backend run before this fresh tool probe, so it was not a completed preflight. The probe verified the same unchanged root binaries linked into this worktree while the run was in progress; no tool was rebuilt or replaced. The exact-tree gate must execute its normal required preflight before its checks. Static/generation, docs/scanners and the gate remain pending until recorded below.

## Expanded correction remaining change checks

`go vet ./...`, `../bin/golangci-lint run ./...` (zero issues), `../bin/sqlc -f sqlc.yaml diff`, the empty `gofmt -l .` result and `git diff --check` passed, all exit 0. Their logs use `/private/tmp/ovc-recovery-cursor-{vet,lint,sqlc,gofmt}.log`. The full backend results include API 80.256 seconds and files 17.103 seconds. `tools/docs/check-docs.sh` passed, exit 0, including links and all six diagram documents, in `/private/tmp/ovc-recovery-cursor-docs.log`. The initial sandbox-only offline docs-tool installation lacked a tarball in its selected temporary cache; the same frozen offline install with host-cache access passed, without manifest changes, in `/private/tmp/ovc-recovery-cursor-docs-install-host.log`.

Final tier is standard, security trigger met. The combined after-fix review and applicable source/runtime change checks passed. Staged secret scanning is performed immediately before the authorized commit and is also mandatory in the gate. Existing full browser journeys, dependency scans and remaining full-run checks are deferred to the exact-tree gate; no browser behavior changed. The gate has not run for this tree, and no merge or slice closure is claimed. Both previous failed gate reports remain retained.

## Approved deadline fixture consolidation

The expanded correction's gate at `3a1e865c12ff171b735662299d1cff07d0dc4cd7` passed all 30 commands and 944 E2E cases but took 260.7 seconds, exceeding the unchanged 260-second ceiling. Its report is retained as `ovc-merge-reports/3a1e865-recovery-cursor-budget-exceeded.log` in the common Git directory. No merge or unchanged retry occurred; supported release freed the completed gate lock. The owner approved the [fixture consolidation](../plans/slice-6-recovery-deadline-consolidation.md) after independent security planning review. Both earlier commits and branches remain intact.

The coherent correction is prepared on a new branch from `bf6ea79d5c999df1a8ad077cc25d172521ff4955`. It shares one isolated real PostgreSQL/local-S3 fixture between the candidate and pass deadline operations. All observations are captured by value before the next operation; the candidate budget is explicitly restored to the production constant before the 75 ms pass. The service cursor is never reset manually. Both other regressions remain byte-for-byte unchanged.

| Previous assertion | Consolidated observation and assertion |
|---|---|
| Candidate error is deadline exceeded, count zero, elapsed at most one second | `candidateErr`, `candidateCount`, `candidateElapsed`, checked together after both operations |
| Candidate batch visits both held rows and wraps | saved `candidateCursor` is zero |
| Candidate timeout leaves both intents unchanged and outbox empty | saved `candidateFirst`, `candidateLater`, `candidateOutbox`, compared with initial snapshots and zero |
| Pass error is deadline exceeded, count zero, elapsed at most one second | `passErr`, `passCount`, `passElapsed`, checked independently |
| Pass expires during selection or on first held intent | saved `passCursor` is zero or the first identifier |
| Pass leaves durable state/outbox unchanged | saved `passFirst`, additional `passLater`, `passOutbox`, compared with initial snapshots and zero |

There is still one arrange phase, one act phase with both operations and observations, and one assertion phase. No intermediate assertion, new timeout, retry, skip or production change was introduced. This removes one fixture lifecycle; it does not establish a measured reduction of the longest package-scheduling path or guarantee a gate margin. Expanded historical review/check evidence above does not count as this new tree's runtime or independent-review result. New positive tests, two repeated deadline sensitivity controls, full backend/static/docs checks, fresh combined after-fix review and gate are pending.

## Consolidated deadline targeted execution

On the consolidated source, `tools/check-tool-binaries.sh` passed, exit 0 (`bin/ matches tools/go.mod`). The three final recovery regressions passed, exit 0, package 2.405 seconds, using this command from `/private/tmp/ovc-recovery-consolidated/backend`:

```sh
caffeinate -i /Users/klukashka/Work/ovc/tools/dev-env.sh go test -race -count=1 ./internal/services/files -run 'TestUploadRecovery(BoundsCandidateAndPassDeadlines|ContinuesAfterOneCandidateFails|RetainsCursorAfterCancellationAndWraps)'
```

Complete output: `/private/tmp/ovc-recovery-consolidated-targeted.log`. This is evidence for the consolidated source, not a speedup measurement or a full-gate result.

Two negative controls ran sequentially in separate source copies containing no environment files. Each used the following command from its own `backend/` directory:

```sh
caffeinate -i /Users/klukashka/Work/ovc/tools/dev-env.sh go test -race -count=1 ./internal/services/files -run '^TestUploadRecoveryBoundsCandidateAndPassDeadlines$'
```

- `/private/tmp/ovc-recovery-consolidated-negative-candidate` replaced only `context.WithTimeout(bounded, s.deps.Uploads.Candidate)` with `context.WithCancel(bounded)` in `upload_recovery.go`. It failed the saved candidate cursor/whole-batch assertion: `candidate deadlines did not allow the whole locked batch to be examined`. Exit 1, package 1.270 seconds; complete log `/private/tmp/ovc-recovery-consolidated-negative-candidate.log`.
- `/private/tmp/ovc-recovery-consolidated-negative-pass` replaced only `context.WithTimeout(ctx, s.deps.Uploads.Recovery)` with `context.WithCancel(ctx)`. It failed the saved pass overall-bound assertion: zero processed, elapsed 2.00447225 seconds, two candidate deadline errors. Exit 1, package 3.036 seconds; complete log `/private/tmp/ovc-recovery-consolidated-negative-pass.log`.

Both failures are the required sensitivity results, not unrelated compile/setup failures or retries. The production implementation in the consolidated worktree was never changed. The existing cancellation, wrap and item-error-isolation tests and their previous negative-control evidence are unchanged. Full backend/static/generation/docs/scanner checks, independent combined review and the new exact-tree gate remain pending for this consolidated tree. The targeted heavy-execution slot is released.

## Consolidation independent after-fix review

The owner-approved fresh combined standard/full-security reviewer inspected the complete seven-file diff against `bf6ea79d5c999df1a8ad077cc25d172521ff4955` and separately compared it with `3a1e865`. No blocker, major or minor finding was found. The reviewer verified the assertion mapping, all unchanged budgets, saved cursor-by-value, absence of manual cursor reset, immediate rollback and AAA; the isolated error and cancellation/wrap regressions are unchanged. T-20/T-41 were examined and no production trust boundary changed. It read the actual three positive tests and both negative-control logs, inspected the single-timeout mutation diffs and confirmed the expected failures. Source hashes recorded by `git hash-object` were protocol `18288fee11ecde2da80d9d101c9a4ee9f8eb0fb3` and recovery `d3ebf8b57c8e3d61089d3710229d0d7e13b01d39`. The reviewer ran no heavy workloads and explicitly left full backend/static/docs/scanners/gate and critical-path savings unverified pending coordination.

## Consolidated correction final change checks

The full `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go test -race -count=1 ./...` from `backend/` passed, exit 0; API 75.965 seconds, files 15.571 seconds. `tools/check-tool-binaries.sh` passed before these checks. `go vet ./...`, `../bin/golangci-lint run ./...` (zero issues), `../bin/sqlc -f sqlc.yaml diff` and the empty `gofmt -l .` check all passed, exit 0. Logs are `/private/tmp/ovc-recovery-consolidated-{backend,vet,lint,sqlc,gofmt}.log`. Frozen offline docs-tool installation passed with the existing host cache and no manifest change. The final source hashes match the independent review. These separate runs are behavioral evidence, not controlled before/after performance measurements.

The final tier remains standard and the security trigger remains met. The approved combined after-fix review passed, and no production trust boundary was changed. Final documentation and staged secret checks run immediately before the authorized commit; their evidence and the exact-tree gate/finish are retained externally without editing the tested tree. Existing whole browser journeys, dependency scans and other full-run commands remain deferred to that gate. No merge is claimed before every gate command passes, its wall time is at most 260 seconds and finish proves exact tested/main tree equality. All three earlier failed gate results remain retained.
