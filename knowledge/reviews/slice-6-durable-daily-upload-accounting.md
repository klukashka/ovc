# Durable daily chat upload accounting verification

Status: the preserved accounting implementation passed all 30 gate commands but failed the 260-second budget at 265.4 seconds and was not merged. The approved consolidation is source-prepared only; its runtime checks, sensitivity controls, fresh review and exact-tree gate remain pending. Historical passes below do not prove the consolidated test source. This record does not close slice 6 or claim merge readiness.

## Scope and inspected state

Approved [plan](../plans/slice-6-durable-daily-upload-accounting.md); implementation baseline `d0b91d44ade076cef116f15897e3499d9d8a443a`, which includes the tutor-session recheck. The scope is migration 21, aggregate queries and typed repositories, upload creation/Complete/publication, bounded historical cleanup, tutor purge, production worker registration, fixtures, regression tests and their contracts. Tier: standard. Security trigger: met, including admission, deletion, migration, resource bounds and privacy retention (T-13/T-18/T-19/T-20/T-21/T-29/T-34/T-41). The fix touches trust boundaries. Fresh independent standard and full-security source reviews completed; their minor findings are resolved by the targeted evidence below. Neither review grants merge readiness without the remaining checks.

The aggregate stores only student/tutor UUIDs, UTC day and bytes. The student FK references the already locked user; there is no tutor FK. Tutor purge removes aggregates after channel/member removal. Completion takes no new user/profile lock and commits bytes with publication. The new job owns a cancellable 20-second context including pool acquisition and SQL; it takes at most 100 ordered student locks before at most ten deletes of 100 aggregates. It neither touches storage nor changes the expiry job's deadline.

## Acceptance-to-evidence map

The named targeted runtime checks below **passed** on the historical source in the recorded race runs; renamed and combined tests are mapped separately below and have not yet run. The historical baseline and removed-budget controls failed as expected. Source-only reasoning and remaining full checks are identified separately.

| Acceptance | Named evidence |
|---|---|
| Real HTTP upload/send/delete/worker object and row cleanup cannot refund the 200 MB day | `TestDailyUploadCapSurvivesRealMessageAndObjectCleanup`: eight real 25,000,000-byte uploads, all eight physical-cleanup observations, then a refused ninth upload with zero charged quota. Baseline negative control below |
| Validation and exact quota refusal charge nothing | `TestDailyUploadAccountingRejectsInvalidContentAndQuotaWithoutCharging` |
| Independent students, tutors, tutor exemption and next-day allowance | `TestDailyUploadAllowancesAreIndependentAndResetNextDay`; existing `TestTheDailyAttachmentCapRefusesWithRateLimited` retains its cases with accounting-aware completed fixtures |
| Concurrent completions cannot exceed the daily cap | `TestConcurrentUploadCompletionsCannotExceedDailyAllowance`, with a real local-S3 part barrier |
| Duplicate settlement/ownership and transaction rollback | `TestDuplicateStudentSettlementPreservesOwnershipAndChargesOnce`; `TestDailyAccountingRollbackDoesNotChargeACompletion` exercises failure after accounting in the actual settlement path; existing upload reconciliation tests retain their ownership assertions |
| Expiry, channel erasure and ending do not refund bytes | `TestDailyUploadAccountingSurvivesExpiryErasureAndRelationshipEnding`: real upload and worker/ending paths; expiry and discarded-channel eligibility are arranged as ordinary fixture data |
| Creation waiting across midnight uses fresh transaction time | `TestQueuedUploadCreationUsesThePostLockDay`, waiting on the real student row lock |
| An upload crossing midnight keeps its creation day; cleanup waits for settlement | `TestUploadCrossingMidnightKeepsItsCreationDayUntilSettlement` |
| Locked students do not block others; uploading/current days survive | `TestDailyUploadCleanupRetainsUploadingDaysAndSkipsLockedStudents` |
| 100-student/1,000-row limits and cancellation while waiting for a connection | `TestDailyUploadCleanupAdmitsAtMostOneHundredStudents`, `TestDailyUploadCleanupIsBoundedByRowsAndStudents`, `TestDailyUploadCleanupCancellationCoversPoolAcquisition` |
| Own 20-second deadline | `TestDailyUploadCleanupOwnDeadlineBoundsPoolAcquisition` holds every real pool connection, uses a 25-second parent guard, and requires the service's own 20-second deadline to return within 21 seconds. It passed; removing only the own deadline in a separate copy failed at 25.001432209 seconds. The shorter parent-deadline test separately proves cancellation propagation |
| Production scheduler activation | `TestProductionWorkerRecoversAnUploadWithoutAnAttachment` additionally checks real `/__test__/tick`, successful `uploads.daily_cleanup` job state and removal of its old aggregate |
| Real migration backfill, rerun immutability, invalid-state rejection and rollback | `TestDailyUploadMigrationBackfillsRetainedBytesAndKeepsActivation`, `TestDailyUploadMigrationRollsBackItsSchema`, using real migration 21 over schema 20 |
| Pre-activation refusal before body reads, tutor exemption and exact post-activation success | `TestDailyUploadCutoverRefusesBeforeTheBodyAndActivatesAtMidnight`, reading the real migration activation and moving only the injected clock |
| Missing state fails closed before streaming | `TestStudentUploadsFailClosedWithoutAccountingMigration`, real schema 20 and the actual admission path |
| Old intents remain cleanup-only after activation | `TestPreCutoverStudentIntentsCannotCompleteOrPublishAfterActivation`, with a real local-S3 legacy completion fixture |
| Block/unblock and soft deletion/restore preserve totals | `TestDailyUploadAccountingSurvivesBlockAndSoftDeletion`, for both roles |
| Final student/tutor purge cannot leave/recreate aggregates or erase another account's ledger | `TestFinalAccountPurgesCannotRecreateDailyAccountingFromRacingUploads`, both roles, real paused S3 Complete and real purge |

Database COMMIT-acknowledgement loss remains unverified under the existing no-interception policy. Existing reconciliation checks prove durable state handling, not an injected network-level lost commit response. No new interception exception is used.

## Commands and results

- Initial `go test ./internal/api ./internal/services/files ./cmd/internal/startup -run '^$'` from the implementation's `backend/`: **blocked**, exit 1 before compilation, sandbox denied standard Go build-cache writes. This was not an assertion failure.
- `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go test ./internal/api ./internal/services/files ./cmd/internal/startup -run '^$'`: **passed**, exit 0, with all three packages reporting `[no tests to run]`. This checked an earlier intermediate source state; subsequent test/document additions require fresh compilation and execution.
- `gofmt` on affected handwritten Go and `bin/sqlc -f backend/sqlc.yaml generate`: **passed**, exit 0. The first generation attempt found an ambiguous tutor column; qualifying it fixed the query before successful generation. Generation is not runtime verification.
- `git diff --check`: **passed**, exit 0 on the current source preparation.
- Backend race/static/generation verification, affected browser journeys, documentation checks and scanners: **pending execution**. Other full-run commands are **deferred to the merge gate**, not passed. The exact-tree gate and finish remain pending; the 260-second ceiling is unchanged.

## Baseline negative control

The baseline checkout at `d0b91d44ade076cef116f15897e3499d9d8a443a` received only `backend/internal/api/daily_upload_lifecycle_test.go` (formatted blob `f2c5b0e246ffc2b71f7e5ca35a7acf46f1f5fe5f`). The command from that checkout was `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go -C backend test -race -count=1 -run '^TestDailyUploadCapSurvivesRealMessageAndObjectCleanup$' ./internal/api > /private/tmp/ovc-daily-baseline-negative.log 2>&1`, preceded by `./tools/check-tool-binaries.sh > /private/tmp/ovc-daily-baseline-tools.log`. The isolated lifecycle test **failed as expected**, exit 1: all eight object/row cleanup observations passed, but the ninth upload returned `201` instead of `429`, with charged quota zero and one new attachment remaining. Test duration was 10.52 seconds (package 11.212 seconds). The tools probe passed. Logs are retained at `/private/tmp/ovc-daily-baseline-tools.log` and `/private/tmp/ovc-daily-baseline-negative.log`.

This confirms the original accounting defect, not the fix. The real 200 MB lifecycle workload adds a measurable check cost; fixture/parallel execution may be optimized without reducing observations, limits or the gate ceiling.

## Fixture and operational limits

General Go fixtures keep their existing 2030 injected dates and the real immutable migration activation; no activation update, truncate, constraint disabling or schema preseed is used. Dedicated cutover tests read the actual activation. E2E reads it with a read-only query and starts its injected clock no earlier than that instant. Production and test-control paths have no bypass. Fresh migrations conservatively pause student uploads; already deleted historical bytes are not claimed to have been reconstructed. Operators stop old writers before adoption and rollback.

## Initial independent reviews and fixes

Two independent reviewers read the complete 34-file tracked/untracked scope over baseline `d0b91d44ade076cef116f15897e3499d9d8a443a`. The standard review's content-manifest SHA-256 was `64d6199759a30bd2bce82c90e8f05e4b2029c6fd7ee7029803a5f7bcb6822f17`; the security review's sorted `[path, gitHash]` compact-JSON SHA-256 was `07fd71230da465defe8d2bd81e14f958a4267da6de80ecbcfd621925d3038688`. These are different manifest encodings, not interchangeable tree hashes.

Neither found a new blocker or major production finding. Both examined atomic completion/accounting, lock order, cleanup bounds, cutover and immutable activation. The security review examined T-13/T-20/T-34, T-21/T-29, T-18/T-19 and T-41; other boundary families were not affected. Large-data query/progress performance remains for the slice-closure measurements, and COMMIT-acknowledgement loss retains its existing unverified limitation. At that reviewed state readiness failed because runtime fixture failures and remaining checks/deadline evidence were unresolved.

The standard reviewer classified the wrong ending status expectation and missing captured diagnostics as minor; the security reviewer also classified missing captured diagnostics as minor. The active-relationship ending expectation now follows the existing `200` contract. The cutover test captures its refusal's logs and checks the exact INFO event, allowed fields and actor/request correlation, absence of credentials, filenames, storage identity and `upload.accepted`. The missing-schema test captures its ERROR event, permits only time/level/message/safe kind, requires `sqlstate_42P01`, and rejects raw SQL details and extra successful outcomes. Both checks passed. These fixes change test evidence only and do not touch a trust boundary; there is no production or contract change after the independent review. Under the minor-fix rule they require checks, not another independent review.

## Targeted execution history

All commands below ran from `/private/tmp/ovc-daily-upload-accounting/backend`, prefixed with `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i`. Each used `go test -race -count=1`. Tool checks ran from the checkout root as the same wrapper prefix plus `bash tools/check-tool-binaries.sh`. Every redirected log listed below is retained.

| Package and exact `-run` expression | Initial result and log | Fixed result and log |
|---|---|---|
| `./internal/api -run 'Test(DailyUpload\|QueuedUploadCreation\|ConcurrentUploadCompletions\|UploadCrossingMidnight\|FinalAccountPurges)'` | **failed**, exit 1, 10.656 s; `ovc-daily-targeted-api.log`; only the new ending fixture expected 204 while the real endpoint returned its specified 200 | **passed**, exit 0, 12.100 s; `ovc-daily-targeted-fixed-api.log` |
| `./internal/services/files -run 'Test(StudentUploadsFailClosed\|DailyUpload\|DuplicateStudentSettlement\|PreCutoverStudent\|DailyAccountingRollback)'` | **failed**, exit 1, 1.224 s; `ovc-daily-targeted-files.log`; legacy fixture incorrectly recorded MPU after marking Complete started, violating its real phase constraint | **passed**, exit 0, 21.950 s; `ovc-daily-targeted-fixed-files.log`, including the real 20-second own-deadline check |
| `./cmd/internal/startup -run 'Test(DailyUploadMigration\|ProductionWorkerRecoversAnUpload)'` | **failed**, exit 1, 2.882 s; `ovc-daily-targeted-startup.log`; retained pending fixture lacked required `expires_at` | **passed**, exit 0, 3.778 s; `ovc-daily-targeted-fixed-startup.log` |

Log paths in this table are under `/private/tmp/`. Initial and fixed tool probes both passed, exit 0 (`ovc-daily-targeted-tools.log`, `ovc-daily-targeted-fixed-tools.log`). Initial failures were corrected in test arrangement/expectations, not relabeled as infrastructure failures. Legacy fixture now records its MPU, authorizes its simulated legacy Complete through the real repository, completes real local S3 storage, and then exercises the new post-activation refusal. Pending backfill fixtures provide expiry only for pending state.

The fixed tests used the changed-file SHA-256 manifest `/private/tmp/ovc-daily-targeted-fixed-manifest.txt`, manifest digest `9244031b89438defb6b41e695bb94fb8fe5e9d3a2f0c758b86f1093feb478633`. The subsequent edits to this verification record record those results and do not change tested executable source. No commit, merge, full-run check or gate result is implied.

## Own-deadline sensitivity control

A separate source copy, `/private/tmp/ovc-daily-budget-negative-ecqc7aj9`, differs in exactly the cleanup deadline constructor: `context.WithTimeout(ctx, dailyUploadCleanupBudget)` becomes `context.WithCancel(ctx)`. No other executable source or test was changed in the control. From that copy's `backend/`, `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go test -race -count=1 ./internal/services/files -run '^TestDailyUploadCleanupOwnDeadlineBoundsPoolAcquisition$' > /private/tmp/ovc-daily-budget-negative.log 2>&1` **failed as expected**, exit 1, package 25.921 s. The assertion reported zero deletions, deadline exceeded and elapsed `25.001432209s`, beyond its 21-second ceiling. Its independent parent guard ended the operation and released the held pool connections. This proves sensitivity to removal of the actual service budget without changing production timing parameters or using test doubles.

## Full backend and static execution

The executable source remained unchanged from the fixed targeted manifest. A fresh tools probe passed (`ovc-daily-full-tools.log`). From the checkout root, `/usr/bin/time -p /Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go -C backend test -race -count=1 ./...` passed, exit 0, wall 88.88 seconds. The API package took 85.245 seconds, files 36.295 seconds and startup 47.194 seconds. All package results are retained in `/private/tmp/ovc-daily-full-backend.log`. This is not a merge-gate duration or proof of its 260-second budget.

The following commands ran sequentially from `backend/`, each with `/usr/bin/time -p /Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i` and output redirected to the named log under `/private/tmp/`:

| Command | Result | Wall time | Log |
|---|---|---|---|
| `bash -c 'unformatted="$(gofmt -l .)" && test -z "$unformatted"'` | passed, exit 0 | 0.07 s | `ovc-daily-full-gofmt.log` |
| `go vet ./...` | passed, exit 0 | 0.86 s | `ovc-daily-full-vet.log` |
| `../bin/golangci-lint run ./...` | failed, exit 1 | 3.88 s | `ovc-daily-full-lint.log` |

The lint failure is `forbidigo` on `time.Now()` at `daily_uploads_internal_test.go:183`, in the own-deadline elapsed-time measurement. Execution stopped on this first failure without changing source or repeating checks. SQLc diff and module tidy diff remain unverified; frontend, E2E, documentation checks and the merge gate have not run in this phase. The recorded backend pass precedes any subsequent correction to this test.

## Final clock measurement and frontend checks

The test-only lint finding was corrected using `clock.System` for both elapsed-time observations; that adapter preserves monotonic time. No service, schema, query or timing parameter changed. After another passing tools probe, the real `TestDailyUploadCleanupOwnDeadlineBoundsPoolAcquisition` race run passed on this final test source. Vet, golangci-lint (zero issues) and sqlc diff passed, exit 0. Logs: `/private/tmp/ovc-daily-clockfix-{tools,deadline,vet,lint,sqlc}.log`. `go mod tidy -diff` was not run separately and remains deferred to the gate; module files did not change. The successful complete backend run above precedes the test-only clock adapter correction and must not be described as an exact run of the newer test source. Final exact-tree coverage remains mandatory in the gate.

Frozen offline frontend installation, typecheck and lint passed. The first format check failed only `frontend/e2e/deployment.ts`; Prettier corrected that file without a behavior change. The final format check, all 665 component tests, build, classes, bundle and generated API checks passed, exit 0. Logs: `/private/tmp/ovc-daily-frontend-{install,typecheck,lint,format-check,format-fix}.log` and `/private/tmp/ovc-daily-frontend-final-{format-check,test,build,check-classes,check-bundle,api-check}.log`. The prior format failure is retained. Full E2E is running separately with sleep prevention and exclusive heavy-check execution; no result is yet claimed.

## Complete browser result and final readiness scope

The sanctioned wrapper ran `caffeinate -i pnpm --dir frontend e2e` from the accounting checkout. All 944 E2E cases passed in 2.6 minutes, exit 0, covering the production browser journeys and the deployment's read-only activation clock alignment. Log: `/private/tmp/ovc-daily-full-e2e.log`. No diagnostic allowance, retries, deadline increase or fixture activation rewrite was used. The complete backend run above also exercised the existing startup/core/testcontrol refusals required for the E2E-path change.

Final tier remains standard and the security trigger remains met. Both initial independent reviews found no new blocker/major; their minor findings were corrected and verified by the targeted runtime/static checks. The ledger fix directly changes upload admission, completion/accounting and account-deletion trust boundaries, resolving the S6-SEC-02 refund mechanism in the real lifecycle test. Remaining COMMIT acknowledgement-loss and large-data performance limitations are explicitly unverified and remain for the existing protocol/slice reconciliation; they are not covered by passing targeted tests. The exact-tree gate and finish are required before merge. Documentation and staged secret checks run before the authorized coherent commit; their complete evidence and the gate result are retained externally so the tested tree is not edited after gating.

## Consolidated source and assertion mapping

The [approved consolidation plan](../plans/slice-6-daily-accounting-consolidation.md) applies over main `d0b91d44ade076cef116f15897e3499d9d8a443a` in a new branch. The preserved accounting commit `5b531bda073a55a2b594cc1442d4bafd32ec438a` and its failed budget result remain intact; [budget evidence](../analysis/slice-6-daily-upload-accounting-budget.md) records all timings and their limits. Source comparison against that commit found changes only in the three named test files before these documentation updates; production, schema, generated queries and E2E source were byte-identical.

`dailyUploadParties` in `daily_upload_lifecycle_test.go` contains only tutor ID/token, student ID/token and relationship card. It performs the same real sign-in, account naming/language and accepted relationship arrangement as `activeStudentInAGroup`, omitting only group creation. The twelve expanded explicit arrangements were: lifecycle (1), cutover (1), invalid-content/quota (1), student-bound (1), midnight (1), row-bound (1), queued creation (1), concurrent completion (1), expiry/erasure/ending (3), block/soft deletion (1). The cleanup skipped-student case removes the thirteenth group through replacement of its private use of `sharedChatWorld`; it keeps `h.squat` with the same emails and therefore the same provisional/unverified account setup. Its unused second tutor/relationship is omitted. Shared fixtures and the independence/final-purge uses of `sharedChatWorld` are unchanged. Combining the two bound tests leaves one paired fixture for their two observations; combining the two pool tests removes a second harness lifecycle. Exactly two harness lifecycles are removed, with no schema/data resets or between-observation re-arrangement.

| Historical assertion or case | Consolidated evidence and preserved observation |
|---|---|
| `TestDailyUploadCapSurvivesRealMessageAndObjectCleanup` | Same test name and actual eight 25,000,000-byte upload/send/delete/worker cycles. Each saved cycle still asserts zero attachment rows and absent S3 object. Ninth response still asserts 429 and nonempty Retry-After, zero charged quota and no remaining uploads. Only the direct pair's type/setup changed; the file remains baseline-compatible |
| Cutover | Same before-body 503, Retry-After 60, Connection close, zero rows, tutor 201, post-activation student 201 and total 8; captured INFO event, allowed fields/correlation and forbidden-content assertions unchanged |
| Invalid content and quota | Same 415, 507 and zero total |
| Midnight crossing | Same before/after cleanup errors and counts 0/1, successful upload, previous-day total 12 and current-day total 0 |
| Queued creation | Same admission/read success and fresh post-lock creation instant |
| Concurrent completion | Same second upload 201, racing first upload 429 and exact daily allowance |
| Expiry, erasure, relationship ending | All three cases remain; same retained total 4 and gone attachment for expiry/erasure, real ending response 200 |
| Block/unblock and soft deletion/restore | Both roles and all four saved observations remain, each total 6 |
| Cleanup skipping locked student and retaining uploading/current days | Same saved error/count and values: success, deleted 1, uploading day 40, other student's removed total 0, current day 60, locked historical day 30 |
| `TestDailyUploadCleanupAdmitsAtMostOneHundredStudents` | First operation of `TestDailyUploadCleanupIsBoundedByRowsAndStudents`: saved `studentsErr`, `studentsReadErr`, `studentsDeleted == 100`, `otherStudentsRemaining == 1`, plus `lockedRowsRemaining == 1001`. The separate student's real user row is locked through this observation |
| Historical `TestDailyUploadCleanupIsBoundedByRowsAndStudents` | Second operation of the same named combined test: release only that lock, preserve the same `authStart`, save `rowsErr`, `rowsReadErr`, `rowsDeleted == 1000`, `rowsRemaining == 2`. There are 1002 rows before this operation: the original 1001 distinct historical days plus the remaining other student's row. Thus the original row ceiling remains observable without resetting data |
| `TestDailyUploadCleanupCancellationCoversPoolAcquisition` | First operation of `TestDailyUploadCleanupCancellationAndOwnDeadlineBoundPoolAcquisition`: fresh 50 ms context and independent saved count/error/elapsed; assert zero deletions and DeadlineExceeded |
| `TestDailyUploadCleanupOwnDeadlineBoundsPoolAcquisition` | Second operation of that combined pool test: create a fresh 25-second parent immediately before the call; save separate count/error/elapsed; assert zero deletions, DeadlineExceeded and elapsed at most 21 seconds for the actual 20-second service budget. The same real pool stays held and is released by defer |
| Other acceptance rows above | Independence across students/tutors/day, duplicate settlement, rollback, pre-cutover intent refusal/cleanup, missing-schema ERROR diagnostics, both final account purge races, migration and real worker composition source remain unchanged |

Both combined tests complete all Act operations and save both results before independent assertions. An early assertion failure cannot suppress the second observation. Existing lock-helper cleanup rolls back an unreleased held transaction; the pool helper is always released by defer. Timing uses `clock.System`, preserving monotonic elapsed-time measurement; no application fake clock or deadline is changed.

Source-only checks: `gofmt -w backend/internal/api/daily_upload_lifecycle_test.go backend/internal/api/daily_upload_accounting_test.go backend/internal/services/files/daily_uploads_internal_test.go` and `git diff --check` passed, exit 0. No compilation, runtime test, static analyzer, frontend check or gate has run on this consolidation. Required sensitivity controls remain **unverified**: baseline accepts the ninth upload after all eight cleanup observations; removing only the own deadline fails the long elapsed assertion while the short cancellation observation remains independent; removing only student admission LIMIT fails the first bound observation; removing only the ten-batch ceiling fails the second bound observation. Positive checks and fresh combined standard/full-security review remain pending.

## Consolidation targeted checks and sensitivity results

The source-frozen 36-file changed-content manifest is `/private/tmp/ovc-daily-consolidated-source-manifest.txt`, SHA-256 `a5d37ef68521c8da890a53b6ee504a0f0fddc794f3834942600ad1c95c297eb4`. Its test file SHA-256 values are `7b25894fff4e3b064eae97515742a9d07c5b79c9bbf0620235bfa471dd13a84a` (lifecycle), `f2eaa27da24ced11ac8944339f2d18550fa51ba4d8b0e80bf96beb75420cacbc` (API accounting), and `3d97192966a13b6ccf7e42ffd4691a8e9970e57fa5197e1f62cb8a833c59a487` (files accounting). The following results apply to that executable source; subsequent edits record evidence only.

From the consolidated checkout, `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i bash tools/check-tool-binaries.sh` passed, exit 0, logged to `/private/tmp/ovc-daily-consolidated-tools.log`. The following commands ran sequentially from its `backend/`, prefixed with `/usr/bin/time -p /Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i`:

| Command | Result | Package / wall seconds | Log under `/private/tmp/` |
|---|---|---|---|
| `go test -race -count=1 ./internal/api -run 'Test(DailyUpload\|QueuedUploadCreation\|ConcurrentUploadCompletions\|UploadCrossingMidnight\|FinalAccountPurges)'` | passed, exit 0 | 12.123 / 20.26 | `ovc-daily-consolidated-api.log` |
| `go test -race -count=1 ./internal/services/files -run 'Test(StudentUploadsFailClosed\|DailyUpload\|DuplicateStudentSettlement\|PreCutoverStudent\|DailyAccountingRollback)'` | passed, exit 0 | 21.962 / 23.20 | `ovc-daily-consolidated-files.log` |

Three separate backend source copies contain only Git-listed tracked/nonignored untracked backend files, excluding env files. No application source changed in the working branch. A byte comparison confirmed the exact mutations below; unified diffs and complete copied-backend SHA-256 manifests are retained under `/private/tmp/ovc-daily-consolidated-<control>-*.diff` and `ovc-daily-consolidated-<control>-manifest.txt`.

| Control and isolated root under `/private/tmp/` | Exact mutation | Manifest SHA-256 |
|---|---|---|
| own-budget: `ovc-daily-consolidated-own-budget-pn0s1u5i` | Only `internal/services/files/daily_uploads.go`: `context.WithTimeout(ctx, dailyUploadCleanupBudget)` to `context.WithCancel(ctx)`; gofmt passed | `9f63ff52c014928d8520a2fbdb4c4a4ec3c677557de1c9db40f0de5cbc2ac4c9` |
| student-limit: `ovc-daily-consolidated-student-limit-pb8e6otc` | Only `queries/daily_uploads.sql` admission clause `ORDER BY users.id LIMIT 100 FOR UPDATE OF users SKIP LOCKED;` loses `LIMIT 100`; pinned sqlc regeneration changes only `internal/dbgen/daily_uploads.sql.go` | `2dc3deecd8e9f56145ed81c27d02c8d11d52570d761b86c63bb74f9fda48f9fe` |
| row-limit: `ovc-daily-consolidated-row-limit-cxpwmg1d` | Only `internal/services/files/daily_uploads.go`: `for range 10` to `for range 11`; gofmt passed | `7c697fd341bebb37ac7844383c5d70809e5b3c8813b5bb91f8c3211395e8fe0e` |

Student-limit generation ran from its copied `backend/` as `/usr/bin/time -p /Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i /private/tmp/ovc-daily-accounting-consolidated/bin/sqlc -f sqlc.yaml generate`, exit 0; log `ovc-daily-consolidated-student-limit-generate.log`. Generated code was not edited by hand. The deletion-batch SQL limit remains 100 in this control.

From each copied `backend/`, the following commands used the same time/wrapper/caffeinate prefix:

| Control command | Result and observed sensitivity | Package / wall seconds | Log under `/private/tmp/` |
|---|---|---|---|
| own-budget: `go test -race -count=1 ./internal/services/files -run '^TestDailyUploadCleanupCancellationAndOwnDeadlineBoundPoolAcquisition$'` | expected failure, exit 1: long observation returned zero deletions and DeadlineExceeded after `25.0014825s`, exceeding 21 seconds; short cancellation assertion passed; parent guard released the fixture safely | 25.924 / 28.87 | `ovc-daily-consolidated-own-budget.log` |
| student-limit: `go test -race -count=1 ./internal/api -run '^TestDailyUploadCleanupIsBoundedByRowsAndStudents$'` | expected failure, exit 1: first observation deleted 101, other students 0, locked rows 1001. Second observation also correctly reports the propagated difference: deleted 1000, remaining 1 instead of 2. Both read/cleanup errors nil | 1.451 / 9.05 | `ovc-daily-consolidated-student-limit.log` |
| row-limit: `go test -race -count=1 ./internal/api -run '^TestDailyUploadCleanupIsBoundedByRowsAndStudents$'` | expected failure, exit 1: first observation passed; second deleted 1002, remaining 0, with nil read/cleanup errors | 1.628 / 9.96 | `ovc-daily-consolidated-row-limit.log` |

The source branch's `git diff --check` passed again, exit 0. No unexpected failure, timeout increase, weakened assertion or diagnostic allowance occurred. Baseline lifecycle sensitivity with the consolidated direct fixture is still pending separately. Full backend/static/frontend/E2E/documentation checks, the fresh combined independent review and exact-tree gate remain pending for the consolidation; earlier passes are retained as historical evidence, not promoted to this source state.

## Consolidated lifecycle baseline control

The separate baseline worktree `/private/tmp/ovc-daily-consolidated-baseline` remains at `d0b91d44ade076cef116f15897e3499d9d8a443a`, with only the new baseline-compatible lifecycle test/helper added. Its lifecycle file Git hash is `8fb2cf485f4200e2998a16b7194b905585df4278`. A fresh pinned-tool probe passed. From that checkout, `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go -C backend test -race -count=1 -run '^TestDailyUploadCapSurvivesRealMessageAndObjectCleanup$' ./internal/api` failed as expected, exit 1, package 9.662 seconds. All eight saved physical cleanup observations passed; the ninth upload returned 201 instead of the expected 429, quota zero and one remaining attachment. Logs: `/private/tmp/ovc-daily-consolidated-baseline-{tools,negative}.log`. Together with the three isolated bound controls above, this completes the approved sensitivity evidence for the consolidated test source. The elapsed values are not controlled speedup measurements.

## Consolidation readiness review and complete backend

The approved fresh combined standard/full-security reviewer inspected the complete 36-file staged scope against `d0b91d44ade076cef116f15897e3499d9d8a443a` and the six-file correction relative to `5b531bd`. No new blocker, major or minor was found. Reviewed staged-diff SHA-256: `b31afcba1a8b2bb3a397d7cf5d682ffefc164a080b293fc0f3a1649398529121`; staged-content manifest SHA-256: `adabcc958cff63e56a082af57e2725827aa249198e47ca1d6ab8f0613888945f`. The reviewer verified all saved observations, real identity states, source-only control mutations and actual positive/negative logs, and examined T-13/T-20/T-34/T-41 with the unchanged T-18/T-19/T-21/T-29 integration boundaries. Production trust boundaries did not change through this test consolidation. No additional review round was started.

After a fresh tool probe, `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go -C backend test -race -count=1 ./...` passed, exit 0, on the consolidated source: API 86.796 seconds, files 34.820 seconds, startup 37.380 seconds. Logs: `/private/tmp/ovc-daily-consolidated-full-{tools,backend}.log`. Sequential empty-gofmt check, `go vet ./...`, `../bin/golangci-lint run ./...` and `../bin/sqlc -f sqlc.yaml diff`, from backend, all passed, exit 0; logs use the same prefix with `gofmt`, `vet`, `lint`, `sqlc`.

Production, migration, queries, generated bindings, configuration and all frontend/E2E source remain byte-identical to the previously checked `5b531bd` implementation. Its passing frontend and 944-case E2E evidence is retained above; no new standalone frontend/browser run is claimed for this test-only consolidation. The exact-tree gate will rerun all those commands and the complete backend before any merge. Dependency scans, module tidy, full history/staged secret checks and the remaining full-run commands are deferred to that gate except the separately required staged secret check before commit. Documentation is checked after recording this evidence. Gate duration and critical-path savings remain unverified; all earlier failures remain retained.
