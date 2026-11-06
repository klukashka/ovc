# Durable chat upload cleanup verification

Status: implementation and targeted verification in progress; independent readiness reviews, integration verification and merge gate pending. No merge or slice closure is claimed. Finding: S6-BE-03, major. The owner approved [the complete protocol](../plans/slice-6-upload-cleanup-protocol.md) on 2026-11-01, including its explicit uncertainty, capacity and operator-receipt trade-offs. Source baseline: `778e292fa938bc5d33aa5db1f960ceb460b71767`; the separately merged student-lock fix at `fd8739dd08a263d6b35214f762e2198a7c650499` must be reconciled before final integration evidence.

## Scope and contracts

Standard tier; security trigger met by upload/storage, purge, cleanup retention, local operator authority, availability and diagnostics. [ADR-0019](../../docs/decisions/0019-durable-upload-cleanup.md) records the approved contract. A durable independent journal precedes storage, controls confirmed Complete/publication, survives purge and separates finite cleanup results from provider uncertainty. Global atomic capacity is 4,096. Existing content, session, scope and quota checks remain mandatory.

Request admission now precedes multipart body parsing. Malformed input releases an untouched reservation only when Create was never invoked, preserving the prior no-row outcome. A local pre-Create flag is set before the call; missing persisted MPU ID alone cannot justify removing an attempted Create's journal. Typed local operator commands inspect bounded keyset pages and request idempotent cycles; results and diagnostics expose no storage keys or MPU identifiers. API process lifetime cancellation is attached to the existing shutdown drain, and the service shares one bounded detached settlement/discard context.

The trust boundaries touched by this fix are storage publication/deletion and account-independent cleanup ownership, admission availability and privileged recovery. Threats: T-13, T-20, T-34, T-18/T-19 and T-22, with existing account/session scope T-2/T-21/T-29/T-44 and verification T-41 preserved. Exact loss of a PostgreSQL COMMIT acknowledgement and arbitrary remote-provider completion timing are unverified; durable-state fixtures do not claim transport fault injection.

## Coordinator checks so far

- `go test -race ./cmd/internal/startup -run '^TestOvcAdminUpload' -count=1`: passed, exit 0, 4.553 seconds. After adding attachment identity to `admin.command`, repeated for that changed audit assertion: passed, exit 0, 4.334 seconds. Real processes exercise same-UUID replay, active-cycle refusal, audit fields, no storage identifiers and 100-row keyset pages.
- `go test -race ./cmd/internal/startup -run '^TestUploadSettlementFinishesInsideProcessShutdownDrain$' -count=1`: passed, exit 0, 34.351 seconds. A real API process holds its real local-S3 Complete request, then encounters an actual PostgreSQL intent lock after release and SIGTERM. Settlement returns inside the existing 40-second drain and retains uncertain ownership. This test does not claim the exact lost-COMMIT-ack scenario. A later lint correction replaces direct system time access with the project clock adapter; rechecking the final tree remains required.
- Documentation check: passed once, including Markdown, links and Mermaid. Subsequent documentation edits require the final documentation check.
- A first lint run rejected direct `time.Now` in the new shutdown process test. The source was corrected to `clock.System`; the failed run is not reported as passed.

All commands used the main checkout's sanctioned environment wrapper and temporary `caffeinate -i` where appropriate. No environment file was opened or copied. Heavy commands ran sequentially. Other full-run commands remain deferred to the merge gate until explicitly recorded below.

## Pending verification

Collect the implementing agent's complete real-path acceptance mapping and all failed/corrected runs, reconcile the student-lock integration, finish affected checks, obtain fresh standard and full security reviews over the complete diff, apply the after-fix rule, then gate the exact merge tree. Full cumulative layer review and performance measurements remain separate closure obligations.

## Integration and additional verification

The final integration checkout starts at `fd8739dd08a263d6b35214f762e2198a7c650499`. The implementation was transferred without changing its source, with the student-lock and upload additions retained together in the account specification and threat model. A documentation table formatting correction and two additional real migration tests were included before readiness review.

`go test -race ./cmd/internal/startup -run '^TestUploadMigration' -count=1` passed, exit 0, 4.787 seconds on this integration checkout. The backfill preserves legacy uploading rows with and without MPU identifiers and deleted chat rows with MPU identifiers, excludes unrelated avatar/deleted rows, and sets the capacity count correctly. A fixture with 4,097 unresolved uploads makes the real API migration command fail; its transaction leaves all legacy rows and the prior schema intact.

Before integration, the implementation checkout passed targeted race checks for conventions, files, API and storage (1.464, 6.985, 6.586 and 1.294 seconds respectively), full conventions (0.722 seconds), `golangci-lint run ./...` (zero issues) and `sqlc -f sqlc.yaml diff` (exit 0). These results do not substitute for final integration checks.

### Acceptance coverage

- Purge, expiry and late Complete: `TestChatUploadCleanupSurvivesPurgeAndDatabaseTimeout`, `TestUploadExpiryDoesNotFenceRemoteCompletion`.
- Transferred bound/pending ownership and unpublished cleanup: `TestUploadReconciliationPreservesTransferredOwnership`, `TestUploadStaleIntentCleanupPreservesPendingOwnership`, `TestUploadReconciliationDeletesUnpublishedObject`.
- Confirmed authorization and real lock timeout: `TestUploadCompleteRequiresConfirmedIntentTransition`, `TestUploadCompleteAuthorizationWaitCannotGrantPermission`.
- Finite generations, stale finalizers and retention: `TestUploadIntentCleanupGenerationsAreFinite`, `TestUploadIntentSurvivesDeadOutboxRetention`.
- Atomic capacity and pre-body HTTP refusal: `TestUploadIntentCapacityIsAtomic`, `TestUploadIntentCapacityRefusesWithoutReadingTheBody`.
- Clock, pool, account/channel and drain deadlines: `TestUploadSettlementDeadlineCoversPoolWaits` and `TestUploadSettlementBoundsItsClockAndIntentLockWaits`, `TestUploadSettlementAccountAndChannelWaitsReleasePermit`, `TestUploadSettlementHonorsEarlierProcessDrain`, plus the real process shutdown test above.
- Immutable deletion identity: `TestUploadIntentIdentityGuardsDeletion` and the pending/bound ownership tests.
- Late Create, NULL-to-ID recheck and already completed cleanup: `TestUploadCleanupAcceptsLateCreateIdentifier`, `TestUploadCleanupRechecksLateCreateDuringDelete`, `TestUploadCleanupBeforeCreateReturns`.
- Bounded ID persistence with no part/Complete invocation: `TestUploadCreateIdentifierPersistenceHasOneSettlementBudget`.
- Operator replay, exclusivity, old receipts after new cycles and retention: `TestOperatorUploadRetryIsIdempotentAndExclusive`, `TestOperatorUploadRetryRetainsOldReceiptsAfterNewCycleAndRetention`, plus the real operator process tests above.
- Recovery progress and wrap: `TestUploadRecoveryAdvancesPastLockedCandidate`, `TestUploadRecoveryRetainsCursorAcrossBudgetAndWraps`.

### Failed development runs retained

Initial fixture failures included missing signing-key injection and incorrectly short ordinary-fixture operation budgets. The ordinary fixture now uses production defaults; deliberate deadline tests keep their short injected budgets and original assertions. The HTTP capacity check was corrected to inspect the parsed response's `Close` property, since Go normalizes the Connection header. Existing malformed-upload assertions exposed retained reservation rows; the implementation added abandonment only before Create was invoked. A new generation-identity assertion exposed a retryable missing-outbox error for a prohibited future generation; validation now rejects it permanently before lookup, without deleting the object. Lint rejected forbidden pgx imports in two service tests; explicit rollback ownership replaced those imports. A conventions run failed because the isolated checkout lacked the installed tool directory; the existing ignored tool directory was linked before the successful check. None of these failed runs is counted as passing evidence.

The first integrated `go test -race ./...` failed (exit 1). The existing `TestAPurgeAfterADeadDeletionStillDeletesTheObject` exposed loss of the distinct ordinary `:purge` deletion identity; the independent standard review separately identified the same behavior. The existing age-before-settlement test still expected the replaced `:completed` key, so its subsequent physical-cleanup assertion was not reached. The conventions package also started before the isolated checkout's installed-tool link was available and reported a missing `golangci-lint` executable. These are failed results, not a passing integration run. Startup tests passed in 37.847 seconds, including the migration and real shutdown checks. The complete output is retained locally as `/private/tmp/ovc-upload-final-backend.log`; final fixes and a new affected-check run are required.

The initial independent standard review found two major issues: production scheduler registration was missing for `uploads.recover`, and ordinary purge lost its separate cleanup identity. It also confirmed that the old completed-upload assertion needs to follow the approved generation identity while retaining physical cleanup checks. Its source review is complete for those findings, but full governing-document coverage was not completed because some reads were truncated; a completed document review remains required. No positive readiness verdict was given.

## Initial independent security review and corrections

The separate full security-profile review independently confirmed both major findings. It examined storage/publication and scope (T-13, T-2/T-5/T-6/T-21/T-29/T-44), resource bounds and purge retention (T-20/T-34), local operator authority (T-22), diagnostics/minimization (T-18/T-19) and test-policy boundaries (T-41). No additional blocker/major was found. Its source fingerprint for 41 changed backend files was SHA-256 `4504ba67afb4d455ffc83c64960afbb09be93e5186ff2e03934f378c31acb7ea`, calculated over sorted `path\0bytes\0`. It did not run runtime checks and did not claim readiness.

Both fixes touch the storage/deletion trust boundary. Production composition now registers `uploads.recover` once per minute. Ordinary deletion retains its original identity, while purge without an intent again receives the separate `:purge` identity; intent-owned cleanup still uses its bounded generations. The age-before-settlement test now asserts generation 1, retaining its refusal and real object/MPU assertions and adding terminal intent/capacity release assertions.

`TestProductionWorkerRecoversAnUploadWithoutAnAttachment` exercises the real API process's configured worker through its existing test-only Tick route. Its negative control failed before scheduler registration (exit 1, 2.826 seconds). After the correction, the targeted race run of this test, `TestAPurgeAfterADeadDeletionStillDeletesTheObject` and `TestAMemberRemovedMidStreamLeavesNoObject` passed: startup 3.419 seconds, files 2.141 seconds, API 2.329 seconds, exit 0. An intermediate run caught a missing argument to the existing API test count helper and failed compilation; that failure is retained separately. The recovery test verifies the durable job record, processed cleanup and retained uncertainty/capacity rather than invoking recovery directly.

The integrated pre-correction lint and documentation checks passed, but a fresh full backend run and post-correction static checks remain required. A fresh combined security/standard review of the fixes remains pending under the after-fix rule.

## Readiness review outcome

The standard reviewer completed the previously truncated applicable contract reads, including the full standard checklist and relevant threat rows. It found one additional minor contradiction in T-20's old absolute promise that no multipart parts outlive a row. The threat model now states bounded cleanup and the accepted lifecycle-dependent late/unrecorded-Create residual.

A fresh independent reviewer applied the standard checklist and full security profile to the fixes and their integration edges. It confirmed both major findings and the minor contradiction closed, with no new findings in that scope. The reviewed 58-file working-tree fingerprint was SHA-256 `c3261cce5bce9045ee35afecb8628d22bdc84925e740c88e68167baa977338b6` over sorted `path\0bytes\0`; this final evidence paragraph was appended afterward. The reviewed source remains unchanged. Both security fixes touch storage/deletion trust boundaries; no security severity was downgraded.

The corrected integration tree passed `go test -race ./...` (exit 0): startup 35.489 seconds, API 73.399 seconds, conventions 2.733 seconds, files 9.116 seconds; unchanged packages could use Go's normal cache. The merge gate still runs `-count=1`. The sequential `go vet ./...`, `golangci-lint run ./...`, `sqlc -f sqlc.yaml diff` and documentation check all passed (exit 0, fail-fast shell); lint reported zero issues and the link check reported zero errors. No performance or browser result is inferred from these checks.

Final tier remains standard with security trigger met. S6-BE-03's approved implementation and readiness reviews are resolved, subject to the exact-tree merge gate and finish. All remaining full-run commands, including dependency/security scanners and frontend/browser checks, are deferred to the merge gate. Cumulative slice reviews, performance measurements and closure acceptance remain outstanding. No slice-closure claim is made.
