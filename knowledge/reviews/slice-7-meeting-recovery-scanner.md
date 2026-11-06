# Constructor-injected meeting recovery scanner

Status: implementation reviewed, two minor test findings corrected and change checks passed; ordinary exact-tree gate pending, 2026-11-06.

Base: `99c86560a4f4f07ca63e4353cb31a7b048c5fffd`. Scope: scanner service, typed worker job name, PostgreSQL/SharedFake worker integration tests and corresponding specifications/threat coverage. Approved scope: [local completion plan](../plans/slice-7-local-completion.md), stage 1, bounded scheduling for existing eligible ledger obligations only.

Verification tier: standard. Security trigger: met, because this change queues provider cleanup, bounds resource use, deletes settled technical receipts and adds diagnostics (T-20/T-34/T-18/T-19). Independent standard and security reviews examined the functional diff; their minor test findings and post-fix evidence are recorded below. Implementation self-inspection is not an independent review.

## Acceptance and evidence

- `TestMeetingRecoveryScannerCursor`: at most 100 candidates, persisted cursor reconstruction, end-of-traversal wrap and no pending-cycle renewal.
- `TestMeetingRecoveryScannerFailedItemProgress`: real PostgreSQL trigger raises only the permitted item-specific failure; another candidate progresses, error is returned and wrap does not retry the failed item in that pass.
- `TestMeetingRecoveryScannerWorkerAndRetention`: injected real worker Job and provider lane with SharedFake; uncertainty stays retained without automatic renewal; settled receipt survives exactly seven days and is removed after the boundary.
- `TestMeetingRecoveryScannerCancellation`: actual cursor SQL lock wait is cancelled, no candidate work is admitted and database connections are released.
- `TestMeetingRecoveryScannerCursorConflict`: another scanner wins and wraps while the first has selected its page; stale cursor ownership prevents queueing and entropy consumption.
- `TestMeetingRecoveryScannerDatabaseDeadline`: actual SQL lock remains held beyond the single 20-second pass budget; the scanner expires before its outer 30-second context and releases its connection.
- `TestMeetingRecoveryScannerFinalCycle`: one definitive late acknowledgement admits exactly one final cycle, completed by the real worker.
- `TestMeetingRecoveryScannerReceiptBatch`: collection removes at most 100 old settled receipts per pass.

Execution checks are unverified at implementation handoff; the coordinator runs and records focused tests and applicable full backend/documentation checks. Other full-run checks are deferred to the merge gate. The independent reviews, exact-tree gate and finish are pending. Formatting was applied with `gofmt`; formatting alone is not behavioral evidence.

## Boundaries and remaining work

The scanner advances its committed cursor before queueing each candidate. A crash between those writes delays work until the next traversal, without forgetting the obligation. Existing `QueueCycle` supplies atomic outbox admission and durable finite-cycle enforcement. Existing cleanup execution and terminal persistence are unchanged. The injected entropy reader is serialized per scanner instance with cancellation-aware admission; application composition must supply independent readers or a concurrency-safe shared source across instances, as with `core.CryptoRandom`.

No process registration, Create dispatch, stale attached creating-row closure, public API/UI route, operator command or remote access is added. The cleanup executor's known minor reinspection coverage must be revisited before runtime activation. Pending runtime registration is not a passing acceptance criterion. Remote compatibility, remote quiescence and physical erasure remain unverified.

## Initial failed check and correction

The first focused race run failed `TestMeetingRecoveryScannerCancellation` at its combined cancellation/state/instantaneous-pool assertion; it ran for 23.66 seconds with unchanged source and six safe observations. The log did not identify which predicate failed, so exact historical causation remains unverified. Independent standard review reported one minor: immediate zero acquired connections after cancellation is not a valid deterministic lifecycle boundary. Installed pgxpool releases a closed connection through `puddle.Resource.Destroy`, which runs destruction asynchronously; resource accounting can remain acquired until that destructor completes. The existing repository cancellation tests already wait for this condition.

Both new cancellation/deadline checks now assert the cancellation result and unchanged state separately, then wait for zero acquired connections under their existing outer context, yielding with `runtime.Gosched` and no sleep. The 20-second production budget, 10/30-second outer test deadlines and zero-connection acceptance condition are unchanged. This corrects the independently demonstrated test defect; it does not claim reproduction of the unknown historical predicate. No production code changed in this correction. Corrected checks and full-security review remain pending.

The corrected focused run then reported the precise remaining failure: cancellation returned, cycle stayed zero, but cursor revision was two rather than one (23.38 seconds, unchanged source, six safe observations). This established an ordering gap independent of instantaneous pool accounting. The fixture released its lock after the driver returned, while `pgconn.asyncClose` sends the server cancellation and terminates the connection asynchronously. The blocked CAS could therefore finish before the server processed cancellation. Security review classified the fixture ordering gap as minor and found no evidence of an incorrect production outcome.

The two cancellation/deadline fixtures now capture the actual blocked PostgreSQL backend PID and, while still holding the lock, wait until that backend has no active statement or no longer exists. Only then do they release the lock. This explicitly establishes cancellation-before-unlock and preserves the revision/cycle assertions, resource checks and original deadlines. Both prior failed logs are retained. No application source or contract changed; corrected checks remain pending.

## Corrected verification result

The server-completion-barrier version passed all nine focused race tests in 23.60 seconds, including the real 20-second SQL deadline, with six safe observations and unchanged source. The complete `go -C backend test -race -count=1 ./...` then passed in 97.48 seconds with 21 safe observations and unchanged source. `gofmt`, `go vet ./...`, `golangci-lint run ./...`, documentation checks and `git diff --check` passed on the corrected version. Other full-run checks remain deferred to the gate.

Both independent reviews found no production defect. Their two minor findings are resolved by explicit resource/server completion conditions and the successful corrected checks, following the checks-only rule for minor fixes. The security reviewer examined the resource-accounting fix and validated the proposed server-completion ordering before it was implemented; the final test correction and evidence updates are distinguished from that reviewed fingerprint. Neither correction touches a production trust boundary or changes a contract. Both earlier failed executions remain failed and retained, including the unknown predicate of the first failure; later success does not erase them.

Final tier remains standard with the security trigger met. Process activation, stale attached creating-row coordination, operator surfaces and remote acceptance remain pending. The previous cleanup reinspection minor is unchanged and must be resolved before runtime activation. Exact-tree gate and finish remain pending at record preparation.
