# B6 task plan: homework erasure and account deletion

**Status:** approved implementation plan, with owner amendments of 2026-11-12 and 2026-11-13.

**Implementation baseline:** B5 commit `ec4791ff874f29387283082bd465b47ca4fe420b`, branch `feat/homework-erasure-and-deletion`. The repository has no remote or hosted CI.

## 1. Problem and outcome

B4/B5 implement submissions, reviews, reclaim flags, purge batches and the release-only part of `homework.cleanup`. B6 completes their lifecycle:

- Every removal of a provisional or reclaim-pending link marks its one-to-one issues for erasure in the removal transaction.
- Marking immediately removes access for both roles, including download minting and previously issued download addresses.
- `homework.cleanup` erases marked homework, its notification rows and references, releases charges correctly, and queues storage deletion.
- Tutor deletion closes `not_started`, `draft` and `returned` submissions, deferring release of never-submitted answer files. Submitted and checked work remains readable through ended relationships until purge.
- Reclaim keeps preserve existing work and expose `issued_before_reclaim` consistently through every relevant API projection.
- Student purge preserves the tutor’s issue history for ordinary ended relationships.

This resolves the **B4 security major assigned to B6** in `knowledge/reviews/slice-9-known-limitations.md`: tutor deletion currently bypasses homework closure.

B6 implements backend lifecycle and interface data. New results pages remain with F4/slice 10; lesson-linked issue integration remains with B7.

## 2. Verification tier and security trigger

**Tier: Standard.** This is a substantive change within slice 9, not slice closure.

**Security trigger: met.** The change affects relationship authorization, account deletion, personal-data retention, attachment references and quota charges, signed download access, notification recipients and diagnostics.

Required reviews:

1. A fresh security-profile planning review before approval.
2. After implementation, a fresh standard reviewer over the complete B6 diff and a separate fresh security-profile reviewer.
3. Apply the repository’s after-fix rules without downgrading findings or treating self-review as independent review.

The plan includes the approved lifecycle and retention amendments below. The backend race suite (`go test -race -count=1 ./...`) passed with real PostgreSQL and S3 on 2026-11-13 before the test-instrumentation corrections described in section 12.

## 3. Affected components and specifications

- **Tutoring lifecycle:** `backend/internal/services/tutoring/` — removal, stopping studying, reclaim decisions and expiry, student deletion, tutor deletion and final purge effects.
- **Accounts purge coordinator:** `backend/internal/services/accounts/` — preserving final purge marking and draining historical submissions in bounded batches.
- **Homework:** `backend/internal/services/homework/` — marking, closure, cleanup, existing purge integration and reclaim projections.
- **Persistence:** `backend/queries/`, `backend/internal/repositories/`, generated `backend/internal/dbgen/`.
- **Worker:** `backend/internal/worker/` and production/test composition — one-minute scheduling, cancellation, failure isolation and typed cleanup counters.
- **Notifications:** subject-row deletion and, lifecycle filtering of historical student rows and unread counts.
- **Files:** existing reference release, download authorization, upload expiry and durable intent recovery; their integration is exercised rather than replaced.

Governing contracts:

- `knowledge/plans/slice-9.md`: B6, D14, D30–D35, D49, K8, K11, K24; D10, D37–D40 where they govern accounting, downloads and events.
- `knowledge/specs/homework.md`: readability, closure, erasure, cleanup, purges and reclaim.
- `knowledge/specs/accounts.md`: removal effects, deletion, purge and global lock order.
- `knowledge/specs/files.md`: reference ownership, charges, download checks and intent-routed deletion.
- `knowledge/specs/data-model.md`: markers, foreign keys, notification subject index and purge order.
- `knowledge/specs/api.md`: existing schemas, scope refusals and read-only behavior.
- `knowledge/specs/notifications.md`, `observability.md`, and `realtime-and-video.md`: scheduling, recipients, diagnostics and transactional signals.
- ADR-0005, ADR-0014, ADR-0006, ADR-0009 and ADR-0019/0023; verification follows ADR-0011/0018.

## 4. Approved decisions

### Existing decisions to implement

**Removal and marking.** Cover tutor removal, provisional “Stop studying”, explicit reclaim drop, keep treated as drop, reclaim expiry, seven-day cleanup, immediate unverified-account deletion, confirmed student deletion with pending links, and tutor deletion’s provisional/reclaim-pending discards.

Capture the relationship’s **previous state under its lock**. Mark only its one-to-one issues. For final submission frames, apply D14 to the state immediately before removal: a previously readable provisional student receives the final hint; a reclaim-pending student receives none. Removal remains silent: no new homework email or in-app notification.

**Global transaction phases.** Complete relationship, invitation, request, group, channel, series and lesson work for the entire affected set before locking homework rows. Then lock issues and submissions in identifier order. Lifecycle marking and closure never lock or write review revisions. Do not add marking inside a per-link helper that later returns to another link’s lesson locks.

**Tutor deletion.** Use bulk closure and marking after all lesson effects. Do not call `CloseOnEnding` once per relationship or acquire student user locks. Preserve batched signaling and `TestTutorDeletionStatementsDoNotGrowWithItsRelationships`.

**Retention.** Closing is not erasure. Preserve previously submitted answer references and published reviews. Release only answer references created strictly after the latest `submitted_at`, or all answer references when it is null. Reactivation reopens nothing.

**Reclaim.** Keep the flag’s existing write in the reclaim transaction. Verify the explicit keep and invitation-merge keep. The invitation contract allows only invited and ended cards, not the same reclaim-pending card (owner decision, 2026-11-13). Newly issued work after a keep retains the default false flag.

### Approved lifecycle decisions

- Keep the existing purge order. The seven-day unverified cleanup marks provisional issues only in the final `OnAccountPurged` transaction. A purged student receives no final submission frame. Student batches leave current provisional and reclaim-pending work until that final transaction; confirmation before it keeps the link and that work.
- Immediate unverified deletion is the `DeleteAccount -> purge(..., 0)` path. A current provisional submission can survive the user row and cleanup must not join users. Previously discarded submissions are a possible non-provisional history even on an unverified account: drain these through `StudentBatch` in separate transactions before the final immediate purge, checking the password and unverified state under the user lock each time. This conditionally authorized path uses the same bounded child stages as cleanup, preserving the tutor's issue and snapshot.
- Bound each deleted row kind to 500 per transaction. Drain children in resumable stages and keep markers until completion. Select exact composite reference keys. File projections and byte totals reflect each committed reference removal, including between stages.
- Hide historical student homework notification rows and unread counts during reclaim-pending/discarded relationships or issue marking, before pagination and counting. Keep stored rows and `read_at` through reclaim so keeping restores them without replay. The lifecycle filter keeps `issue.withdrawn` visible without a title or link. Erasure deletes notification subjects before submissions.
- Preserve the email contract. Already queued `submission.checked`, `submission.returned` and `submission.review_updated` can still name a deleted tutor while the submission remains readable; record this residual in T-18 and the slice limitations.
- Cleanup locks only the charged profile, issues, submissions, revisions and attachments, in that order. It never locks users or relationships and rechecks existence and markers under lock. Tutor purge serializes through the same profile lock.

## 5. Files to change

Expected implementation files:

- `backend/internal/services/homework/closing.go`
- New `backend/internal/services/homework/erasure.go`
- `backend/internal/services/homework/purge.go` where approved purge integration requires it
- `backend/internal/services/tutoring/links.go`
- `backend/internal/services/tutoring/ending.go`
- `backend/internal/services/tutoring/reclaim.go`
- `backend/internal/services/tutoring/deletion.go`
- `backend/internal/services/accounts/deletion.go`
- `backend/internal/services/accounts/tutoring_effects.go` for the historical student homework purge interface
- New `backend/queries/homework_cleanup.sql` and `backend/internal/repositories/homework_cleanup.go`
- `backend/queries/answers.sql`, `issues.sql`, `reviews.sql`, `notifications.sql` as required
- Corresponding repository wrappers and generated `backend/internal/dbgen/*`
- `backend/internal/domain/submissions.go` for precise cleanup result/count types
- `backend/internal/worker/worker.go`
- `backend/cmd/internal/startup/compose.go`
- `backend/internal/api/auth_test.go` for matching worker composition

For historical-notification filtering, also update `backend/internal/services/notifications/inbox.go` and notification repository queries/wrappers.

Tests:

- New `backend/internal/api/homework_erasure_test.go`
- New `backend/internal/api/homework_deletion_test.go`
- New `backend/internal/api/homework_lifecycle_races_test.go`
- Extend `issue_lifecycle_test.go`, `submission_files_test.go`, `submission_contract_test.go`
- Extend `backend/internal/services/tutoring/deletion_signals_test.go`
- Extend `backend/internal/worker/worker_test.go`
- Add repository cleanup tests for batch selection and index/query behavior.

Documentation:

- Durable task record: `knowledge/plans/slice-9-b6.md`
- Relevant lifecycle/batch/notification clarifications in the specifications above
- `knowledge/reviews/slice-9-known-limitations.md`
- `docs/threat-model.md`, with changed wording marked **pending owner review**

The marker columns and notification subject index already exist in migration `00038_homework_issues.sql`. The owner amendment of 2026-11-13 adds migration `00042_attachment_uploader_retention.sql` for nullable uploaders with `ON DELETE SET NULL` and a downgrade that refuses while null uploaders remain. No public API shape change, new dependency or frontend implementation is planned.

## 6. Order of work

1. Apply the owner amendments, refresh the B5/main baseline, and record the approved plan and contract clarifications.
2. Add repository operations for ordered marking, bulk closure, bounded erasure stages and notification deletion.
3. Integrate lifecycle effects using the global phases above. Preserve previous relationship states and emit signals transactionally.
4. Implement tutor deletion closure/marking and final-purge marking and conditional historical-submission batches.
5. Complete cleanup:
   - Two marked-row selection probes on an empty run.
   - One cancellable 20-second budget covering pool acquisition, transactions and lock waits.
   - Drain both erasure and release work through bounded transactions.
   - Remove references before their owning child rows; respect the revision/submission circular key.
   - Remove homework notification subjects before deleting their submissions.
   - Delete an issue and subtract its snapshot bytes exactly once in the same transaction.
   - Preserve progress after committed batches; retain markers after incomplete work or rollback.
   - Skip failed candidates for the remainder of the pass, continue independent work where possible, and return aggregate failure.
6. Integrate typed counters with the worker’s existing `job.run`; keep one reporting owner.
7. Verify reclaim projections and implement the chosen notification behavior.
8. Add the named integration tests, run change checks, obtain implementation reviews and resolve findings.
9. Refresh the exact merge target and follow the repository’s commit/merge gate workflow only during the subsequently authorized implementation phase.

## 7. Observability impact

After successful commit:

- `homework.issues.marked_for_erasure`: `tutor_id`, `relationship_id`, newly marked issue `count`.
- `homework.submissions.closed`: relationship and count, with `cause: tutor_deleted` for tutor deletion.
- One scheduler-owned `job.run` for `homework.cleanup`, carrying:
  - `issues_erased`: issues actually deleted;
  - `submissions_released`: submissions whose release work completed;
  - `references_removed`: actual reference rows removed, not distinct attachments.

Failed/rolled-back transactions contribute no success counts or lifecycle success records. Partial committed progress remains reported alongside the safe failure kind.

Retain existing file-reference, storage-outbox and purge diagnostics. Never log content, names, filenames, scores, tokens, storage keys or raw database errors. Physical deletion is reported by its existing storage owner, not inferred from successful queueing.

## 8. Threat-model mapping

Update `docs/threat-model.md` in the same change, marking wording pending owner review and naming the tests that prove implemented controls:

- **T-5/T-6:** lifecycle effects remain tutor-scoped; another tutor sharing the student and other students are unaffected.
- **T-21/T-29:** atomic closure/marking, previous-state recipient selection, no new homework hints to reclaim-pending students, and ordered bulk deletion effects.
- **T-27:** all three homework token kinds refuse after marking, including addresses minted beforehand. Preserve the accepted pre-mark reclaim residual for snapshot/review addresses.
- **T-34:** resumable erasure, notification removal, purge history preservation, last-reference deletion and retained durable cleanup ownership.
- **T-44:** every provisional removal and reclaim keep/drop path, the fixed reclaim cue, and the selected historical-notification policy.
- **T-20:** bounded child batches, two idle probes, shared deadline, snapshot accounting and preservation of shared-file charges.
- **T-18/T-19:** safe committed counters and diagnostics without content or credentials.

State explicitly that bounded database cleanup and queued storage deletion do **not** guarantee physical deletion within one minute or one worker run. Backlogs, contention, dead storage work and unresolved uploads retain their existing residuals.

## 9. Checkable acceptance criteria

All new behavioral tests use real services, PostgreSQL, HTTP/WebSocket paths and the worker; file lifecycle tests use local S3. Use injected business time and explicit barriers, never sleeps or patched behavior.

1. **`TestDroppedProvisionalWorkErased`**  
   Table-driven coverage of every D32 path, including both tutor-deletion discard states and keep-treated-as-drop. Assert marking, immediate access loss, exact final recipients and no synchronous charge release of current provisional work. Include a shared student with another active tutor and preserve that tutor's work and charges. Cover orphan submissions, answer references and charges after immediate unverified deletion. A small uncontended fixture is erased in one cleanup run.

2. **`TestMarkedHomeworkIsUnavailableToBothRoles`**  
   Exercise tutor lists/counts, issue/submission reads, edits, withdrawal, review writes, corrected uploads and issue-create key replay; student lists, reads and writes; and mint/serve checks for all three reference kinds. Previously minted addresses return `404` after marking.

3. **`TestStudentPurgeKeepsTutorIssues`**  
   Purge an ended student’s submissions, answers, reviews and references while preserving tutor issues, snapshot references and snapshot accounting. Provisional-removal issues are marked instead.

4. **`TestReclaimPendingHidesWork`**  
   No assignment/read/write/upload/mint access, no newly produced homework notification or student frame, and no homework email dispatch. Assert historical rows and counts according to the approved choice.

5. **`TestReclaimKeepCuesWork`**  
   Cover both supported keep paths: explicit keep and invitation merge. Preserve answers and open status; expose the cue in both submission schemas, assignments, issue list/detail and review queue. New work has no cue; keep-treated-as-drop marks old work. No projection carries the earlier holder's name or account data beside the cue.

6. **`TestTutorDeletionClosesWork`**  
   Close all three open states once, advancing versions appropriately; preserve submitted/checked states and readable content until purge. Refuse new writes, preserve submitted files, and release only never-submitted files during cleanup. Previously minted released-answer addresses return 404. Preserve another active tutor's work, frames and charges.

7. **`TestHomeworkCleanupPreservesSharedAttachments`**  
   Preserve attachments and charges held by another issue, template or revision. Last-reference removal queues deletion once; actual worker storage handling removes only unreferenced objects.

8. **`TestHomeworkCleanupDeletesNotificationSubjects`**  
   Remove all six homework event kinds for erased submissions, for both recipients, without deleting unrelated notifications.

9. **`TestHomeworkCleanupBatchesChildrenAndResumes`**  
   Include more than 500 children under one issue, not merely 501 issues. Assert the approved transaction bound, continued progress, marker retention and exact final snapshot total.

10. **`TestHomeworkCleanupRollbackAndRetry`**  
    Cause a real database failure using a test-created trigger through the test's own connection, under ADR-0009 item 8, named by the fixture helper. Assert atomic rollback of references, charges, outbox writes and accounting; a subsequent run completes without double subtraction.

11. **`TestHomeworkCleanupDeadlineDoesNotStopWorkerScheduling`**  
    Hold a real row lock; prove cancellation bounds database waiting, reports failure and permits later scheduled jobs to run. After release, a later run completes.

12. **`TestHomeworkCleanupIdleUsesTwoMarkedRowProbes`**  
    Assert two candidate queries against the marked-row predicates, no scan loop over unmarked homework and no mutation transaction on an empty cleanup. EXPLAIN both actual probe statements against issues_erase_pending_idx and submissions_release_pending_idx and count exactly two autocommit statements, no BEGIN and no COMMIT, through the dedicated pool’s pgx tracer.

13. **`TestHomeworkRemovalRacesWrites`** and **`TestTutorDeletionRacesHomeworkWrites`**  
    Exercise both serialized orders against real issuing, autosave, submission, review publication and held upload completion. Assert outcomes, recipients, accounting and absence of deadlock; also race cleanup with PurgeTutorBatch.

14. **`TestUnverifiedConfirmationBeforeFinalPurgeKeepsWork`**  
    Prove that confirmation before the final purge transaction keeps the provisional link and its work, after the ordinary student batch has left that work intact.

15. **`TestHomeworkLifecycleRecordsCarryNoContent`**  
    Assert event fields, all three cleanup counters, failure reporting and absence of seeded sensitive content.

Retain and extend `TestTheStudentSubmissionReadabilityCopiesAgree`, `TestAnswerKeyContract`, `TestEndingClosesOpenWork`, tutor/student purge tests and `TestTutorDeletionStatementsDoNotGrowWithItsRelationships`. The security reviewer must explicitly confirm closure of the B4 major.

## 10. Risks

- Per-link integration can invert locks across multiple tutors or lessons; global phases and real concurrency tests are essential.
- Child fan-out can defeat a root-only batch bound.
- Reference sharing can cause premature deletion or duplicate quota subtraction.
- The final purge must not discard work after confirmation has made the account ineligible.
- Notification filtering can accidentally suppress `issue.withdrawn` or produce inconsistent pagination/unread counts.
- Cleanup and purge may select the same work; locked rechecks must make either order safe.
- Scheduler result changes can affect unrelated jobs; preserve their behavior and test the shared reporting path.
- Added integration tests consume lane A’s budget. Planning estimate: **20–40 seconds additional lane A time**, **no new browser scenarios**; this is unmeasured. D4’s 540-second consolidation trigger and the 600-second limit remain binding.

## 11. Change checks to run

**Required full verification:**

From the repository root:

```bash
tools/check-tool-binaries.sh
tools/docs/check-docs.sh
bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .
```

From `backend/`:

```bash
unformatted="$(gofmt -l .)" && test -z "$unformatted"
go vet ./...
../bin/golangci-lint run ./...
../tools/dev-env.sh go test -race -count=1 ./...
../bin/sqlc -f sqlc.yaml diff
```

SQL changes require the **whole backend race-test suite**, including conventions and command/process tests. Regenerate sqlc output during implementation before checking its diff.

Because page-visible behavior changes, also run `pnpm --dir frontend api:check` and the affected existing browser suites through the sanctioned environment wrapper: `smoke`, `reclaim`, `students`, `settings`, `homework` and `notifications`; include assignment/review suites if they exist on the refreshed implementation baseline. Confirm exact filenames before invocation.

Checks outside the touched rows—such as unchanged dependency scans and unrelated frontend suites—are **deferred to the merge gate**, never reported passed.

Before merging, run current `main`’s `tools/merge-gate.sh` from a scratch folder on the exact proposed tree, then its `finish` step. Report the base, tested tree, resulting `main^{tree}`, every command result and total duration. Follow the existing failure/rerun rules.

## 12. Implemented controls and verification

The full backend suite, `go test -race -count=1 ./...`, passed with real PostgreSQL and S3 on 2026-11-13. That result covers the B6 implementation before the subsequent test-instrumentation corrections; it is not execution evidence for modified test assertions. Standard and security review covered the complete implementation and the lifecycle corrections. The remaining revision-lock performance risk is recorded in [the slice limitations](../reviews/slice-9-known-limitations.md).

### Account deletion and retained uploads

Immediate unverified deletion commits a preliminary homework batch separately only when that batch makes progress. A zero-progress check and final purge share one transaction and retain the user lock throughout. Every iteration rechecks the password and account state. `TestRemovalRacingThePurgeOfTheAccountAnswersNotFound` and `TestNoNotificationNamesAPurgedAccountAfterAConcurrentProducer` prove waiting relationship mutations cannot pass a completed purge.

Migration `00042_attachment_uploader_retention.sql` retains bound answer files through final user removal with a nullable uploader and `ON DELETE SET NULL`. Generic purge preserves their references and tutor charges; homework cleanup releases them under D30. Its downgrade first restores `NOT NULL`, refusing while retained null uploaders exist. The final-purge cases of `TestDroppedProvisionalWorkErased` prove user absence, retained bound files and charges, pending-upload deletion, and subsequent cleanup queueing. Queueing does not certify physical erasure.

`TestPendingAnswerRequiresUploader` isolates the CHECK constraint on a pending answer file. `TestAnswerBindingWithAnotherUploaderIsRefused` keeps the pending state and submission scope eligible while changing only the uploader, and asserts binding refusal without a new reference. These replace an assertion whose bound state masked the uploader predicate.

### Bounded purge and lifecycle visibility

Student purge shares cumulative per-kind limits of 500 across selected submissions, locking profiles, issues, submissions and revisions in identifier order before child deletion, then attachments before charge release. `TestStudentPurgeDrainsSeveralSubmissionsPerBatch` proves multi-submission progress; `TestStudentPurgeSharesAnswerBudgetAndResumes` proves the shared answer budget and resumability. Revision deletion is bounded, but revision lock acquisition can exceed that budget, as recorded in the slice limitations.

Immediate unverified deletion permits at most 20 batch transactions per request. `TestUnverifiedDeletionBatchBudgetIsRetryable` proves `503 service_unavailable`, `Retry-After: 1`, retained progress and successful continuation.

`TestHomeworkNotificationVisibilityQueriesAgree` checks all six visibility queries for marked and discarded work. `TestReclaimPendingHidesWork` proves a stored read timestamp survives keep. `TestReclaimKeepCuesWork` covers explicit keep and invitation merge; the invitation contract permits invited and ended cards, not a reclaim-pending card.

`TestMarkedHomeworkIsUnavailableToBothRoles` checks the queue count before and after marking. `TestMarkedHomeworkLeavesStudentCardProgress` checks progress through the existing issue-list student filter, including preservation of another issue. Lifecycle marking and closure never lock or write revisions; erasure locks revisions before attachments.

### Idle cleanup instrumentation

`TestHomeworkCleanupIdleUsesTwoMarkedRowProbes` records every query on a dedicated single-connection pool. It requires exactly two autocommit candidate statements and no BEGIN or COMMIT, then checks their EXPLAIN plans against the two partial indexes. `QueryExecModeExec` avoids separate statement-cache preparation transactions. Database-wide `pg_stat_database.xact_commit` is not a suitable exact counter because unrelated connections and autovacuum can publish statistics between observations.

### Verification boundaries

The test-instrumentation corrections require execution of the modified tests before their assertions count as passing evidence. Backend build, vet, lint, sqlc consistency and documentation checks remain the static checks; browser checks and the merge gate remain required before merge. The verification tier is Standard and the security trigger is met for T-20/T-34/T-44. The corrections change test coverage and documentation, not production authorization or retention predicates. Diagnostic ownership and fields are unchanged: cleanup counters describe committed work, and account deletion logs success only after completion.

Static verification on 2026-11-13: `go build ./...`, `go vet ./...`, `golangci-lint run ./...` (zero issues), `sqlc -f sqlc.yaml diff`, formatting and `git diff --check` passed. `tools/docs/check-docs.sh` was blocked with exit 127 because `tools/docs/node_modules/.bin/markdownlint` was absent. The modified database tests have not been executed.
