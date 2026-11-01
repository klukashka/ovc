# Slice 6 erasure execution deadline

Status: implementation and backend race checks passed; final static/documentation checks and independent reviews pending. Baseline: `f9c7450928a17017bc34654159a5d27b9299147c`. Scope: `fix/chat-erasure-deadline`, including its uncommitted diff.

This follow-up addresses major finding S6-BE-01 of the backend slice review: the wall-clock budget previously checked only between operations and left SQL, pool acquisition and lock waits unbounded. The owner approved the implementation and retention clarification on 2026-11-01. Verification tier: standard. Security trigger: met, because erasure and retention map to T-20 and T-34. The fix touches the deletion/resource-lifetime boundary; authorization rules do not change.

## Implementation and acceptance evidence

One cancellable context starts before candidate selection and bounds the whole pass by the injected erasure budget, 20 seconds in production. The existing wall-clock checks, 100-channel candidate page, 500-message transactions and committed progress remain. A cancelled unfinished transaction rolls back; pending markers are cleared only on completion. Only confirmed completion emits `channel.erased`; the worker records a failed `job.run` with a safe timeout kind and can schedule later jobs and retry the erasure.

| Acceptance criterion | Check | Status |
|---|---|---|
| Profile and channel locks, the initial candidate SQL query, and exhausted pool acquisition respect the execution budget | `TestErasureDeadlineCancelsDatabaseWaits` | Passed in the final changed-package race run |
| Committed progress survives, the unfinished charge/reference transaction rolls back, retry releases quota once and queues one deletion per file | `TestErasureResumesAfterDeadline` | Passed in the final changed-package race run |
| Timeout does not stop later worker jobs, and the next minute completes pending work with safe diagnostics | `TestErasureDeadlineDoesNotStopWorkerScheduling` | Passed in the final changed-package race run |
| More than 100 channels and a channel with 501 messages drain across passes | `TestErasureDrainsMoreThanOneCandidatePage` | Passed in the final changed-package race run |
| Existing discard paths and injected wall-clock batching remain valid | `TestTheErasureJobErasesADiscardedChannelInBatches` | Passed in the final changed-package race run |

Retention now distinguishes immediate access revocation from scheduled erasure and eventual physical file deletion. Backlog, lock contention and storage failures may extend retention; dead deletion work needs operator recovery. Eventual progress assumes those failures and blocking transactions end, with no starvation-freedom claim under a permanently blocked first candidate. This is an approved contract correction, not evidence that the previous retention promise held.

## Verification status

- Formatting: `gofmt -w backend/internal/services/chat/erasure.go backend/internal/api/chat_erasure_deadline_test.go`, exit 0.
- Focused integration: from `backend/`, `/Users/klukashka/Work/ovc/tools/dev-env.sh go test ./internal/api -run 'TestErasure|TestTheErasureJobErasesADiscardedChannelInBatches' -count=1 -parallel=2`, exit 0 (5.034 seconds). An initial sandbox attempt exited 1 before tests because the Go build cache was not writable; the authorized unsandboxed run passed.
- Focused race check: from `backend/`, `/Users/klukashka/Work/ovc/tools/dev-env.sh go test -race ./internal/api -run 'TestErasure|TestTheErasureJobErasesADiscardedChannelInBatches' -count=1 -parallel=2`, exit 0 (5.144 seconds). This run preceded replacing the pool test's forbidden direct `pgxpool` import with acquisition of every connection in the fixture's existing pool; the final test is covered by the changed-package run below.
- `bash tools/check-tool-binaries.sh`, exit 0; pinned binaries matched. The ignored `bin` link resolves to the main checkout's installed tools; documentation dependencies have not been installed in this worktree.
- From `backend/`, `go vet ./...`, exit 0 before the pool-test import correction; final recheck pending.
- From `backend/`, `/Users/klukashka/Work/ovc/bin/golangci-lint run ./...`: first exited 1 on the test's forbidden `pgxpool` import; the corrected test uses the fixture's existing pool, and the second run exited 0 with zero issues.
- Final changed-package run: from `backend/`, `/Users/klukashka/Work/ovc/tools/dev-env.sh go test -race ./cmd/... ./internal/api ./internal/realtime ./internal/services/accounts ./internal/services/chat ./internal/services/files ./internal/services/notifications ./internal/services/tutoring ./internal/testcontrol ./internal/worker ./internal/conventions -count=1 -p=2 -parallel=2`, exit 0. The dependency set was derived from `go list -f '{{.ImportPath}} {{join .Deps " "}}' ./...`; architecture is included in conventions. Results: startup 13.167 seconds, API 142.314, realtime 1.448, accounts 1.404, chat 2.645, files 6.706, notifications 1.362, tutoring 3.114, testcontrol 2.412, worker 7.118, conventions 2.616. The API, worker and operator entry-point packages have no direct tests; their shared startup tests passed.
- `git diff --check`, exit 0. No remote is configured; hosted issue/PR/CI checks are not applicable.
- Final `go vet ./...`: exit 0 on the final test file. `bash tools/docs/check-docs.sh`: exit 0, Markdown checks, 614 unique links with zero errors and six Mermaid documents passed. Pinned frontend and documentation dependencies were installed from the local store with `pnpm --dir tools/docs install --offline --frozen-lockfile` and `pnpm --dir frontend install --offline --frozen-lockfile`, both exit 0. `bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .` passed with no leaks (exit 0).
- API schema/client and frontend end-to-end checks: not applicable to this internal execution-bound change; schemas, route behavior and immediate access revocation are unchanged.
- Independent standard review: passed over the complete tracked/untracked diff, with no findings. It confirmed S6-BE-01 is closed by the implementation and covered initial selection, pool/row waits, rollback/resume, scheduler recovery and retention wording.
- Independent security review: passed over the complete tracked/untracked diff, with no findings. T-20/T-34 were examined; T-13/T-21/T-44 are unaffected because authorization and discard semantics did not change. Safe diagnostics were examined. The fix touches the deletion/resource-lifetime trust boundary. Neither reviewer ran tests; execution evidence above is coordinator-owned. Reviewed source SHA-256: `e3bd0d826528f183f34637f90922ce05c361fda15a749e6bac8ef1039bb83587`; reviewed test SHA-256: `9252f3d38ff1c3b00e6377b9b657a8f7176a647a39f2b34c4fa9089729ae63d9`.
- Full-run checks outside the changed rows: deferred to the merge gate.
- Merge gate: unverified, not run; no commit or merge performed.
