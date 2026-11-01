# Tutor mutation session re-check

Status: implementation complete; targeted runtime verification and static/documentation checks passed; full backend verification failed on an unchanged upload-recovery timing assertion. Readiness is blocked; independent reviews and merge gate remain pending. Approved-plan base: `ecd74d8e165b54f5cde41f45884a48cb122ea8b2`. Integrated and tested base: `5962f0466d56fbe33a541afb331fe679d73c969c`, preserving the subsequently merged Unicode-caption correction. This change addresses the approved plan's S6-SEC-01 finding; the cumulative slice record is maintained separately. Approved [plan](../plans/slice-6-tutor-session-recheck.md), 2026-11-01. Tier: standard. Security trigger met: authentication and authorization, attachment lifecycle and resource bounds (T-2/T-5/T-13/T-18/T-20/T-21/T-29/T-34).

## Caller and acceptance matrix

| Boundary | Acting operations | Identity and serialization | Evidence |
|---|---|---|---|
| API → tutoring cards and invitations | CreateCard, UpdateCard, WithdrawCard, RegenerateInvitation, SendInvitation | Required `domain.TutorActor` from the authenticated principal; `lockActingTutor` after existing earlier reads | `TestTutorMutationsRejectTheAuthenticatedSessionAfterBlockAndUnblock`; existing HTTP card/invitation tests |
| API → tutoring relationships | RemoveStudent, EndByTutor | Same actor; removal preserves student user-row before tutor profile | Same regression and existing deletion/block integration tests |
| API → tutoring groups and channel membership | CreateGroup, UpdateGroup, ArchiveGroup, UnarchiveGroup | Same actor; archive retries retain actor | Same regression and existing group HTTP tests |
| API → tutoring requests/code | AcceptRequest, DeclineRequest with/without block, Unblock, RegenerateCode | Same actor; student locks and earlier refusal precedence preserved | Same regression and existing join-request HTTP tests |
| API → schedule | CreateLessons, EditLesson, CancelLesson | Required actor in admission, execution, replay and each audience retry | `TestTutorLessonExecutionRechecksSessionAndRetainsCommittedAdmission`; existing HTTP schedule tests |
| API → chat | Send, Edit, Delete, Send replay | Existing `chat.Author` retains SessionID; profile then separate session read | Mutation regression and `TestTutorSessionGuardsRejectUnusableIdentitiesAndKeepFreshReplays`; existing message HTTP tests |
| API → files | BeginChatUpload, publication/settlement | Existing principal retained across S3 transfer; fresh session recheck in each phase | `TestTutorChatUploadDiscardedAfterBlockAndUnblockDuringCompletion`; unusable identity regression |
| Session owner and lifecycle | Missing, foreign, expired, already committed logout/reset | No optional session bypass; exact owner/revocation/expiry check | `TestTutorSessionGuardsRejectUnusableIdentitiesAndKeepFreshReplays` |
| Clock and lock wait | Expiry while HTTP request waits for profile | `InstantWithin(ctx, repos.TestClock)` after lock | `TestTutorSessionExpiryIsCheckedAfterWaitingForTheProfile` |

All four guards use the transaction-local clock reader. Existing business timestamps remain separate. Student paths, anonymous invitation/code acceptance, target-tutor checks, workers and operators do not receive invented tutor sessions. No package, dependency, public schema or extra lock is added.

Tutor message replay changes from read-only repeatable-read to read-write read-committed in order to take the existing profile lock before looking up the client key. Student replay retains the old transaction. Fresh replay still creates nothing and consumes no write slot; missing/revoked sessions must fail even for an existing key. The security readiness review must cover this transaction-mode change.

## Observability

Existing unauthenticated errors and upload rejection/cleanup diagnostics remain authoritative. No session identifier, token, message body or new log field is introduced. Business effects and outcome logs occur only after successful transactions. The mutation regression compares business rows, channels, messages, outbox, quota profile and persistent rate counters before/after refusal. In-memory message write slots retain their existing release behavior; upload admission checks session before the hourly slot.

## Verification

Source inspection, formatting and `git diff --check`: passed. Commands below ran on the integrated working tree in `/private/tmp/ovc-tutor-session-final`, with Go commands from `backend/`. The sanctioned wrapper was `/Users/klukashka/Work/ovc/tools/dev-env.sh`; actual local PostgreSQL and S3 were used. No compiler error, failed assertion or infrastructure failure occurred in the positive runs listed here.

| Check | Command | Result |
|---|---|---|
| Compilation | wrapper `go test -run '^$' ./internal/api ./internal/services/chat ./internal/services/files ./internal/services/tutoring ./internal/services/schedule` | passed, exit 0; schedule has no package-local tests, its service is exercised by API integration tests |
| Targeted regressions | wrapper `go test -race -count=1 -parallel=4 -run 'TestTutor(Mutations\|Session\|ChatUpload\|LessonExecution\|MessageReplays)' ./internal/api` | passed, exit 0; API 3.322 s |
| Pinned tool verification | `tools/check-tool-binaries.sh` | passed, exit 0 |
| Full backend race | wrapper `go test -race -count=1 ./...` | failed, exit 1; exact assertion below; no rerun |
| Formatting | `test -z "$(gofmt -l .)"` from `backend/` | passed, exit 0 |
| Vet | `go vet ./...` from `backend/` | passed, exit 0 |
| Lint | `../bin/golangci-lint run ./...` from `backend/` | passed, exit 0, zero issues |
| Generated queries | `../bin/sqlc -f sqlc.yaml diff` from `backend/` | passed, exit 0 |
| Documentation setup attempt | `tools/docs/check-docs.sh` | blocked, exit 127: local worktree lacked `tools/docs/node_modules/.bin/markdownlint`; no assertion ran |
| Documentation | `tools/docs/check-docs.sh`, after attaching the installed main-worktree tools with byte-identical `pnpm-lock.yaml` | passed, exit 0; 2,147 links, zero errors, six Mermaid documents |
| Staged Gitleaks, API contract check and remaining gate checks | coordinator-owned | deferred to the merge gate; not run by the implementer |
| Affected production browser journeys | coordinator-owned | deferred; not yet passed |
| Independent standard and full security reviews | coordinator-owned | pending |
| Exact-tree merge gate | coordinator-owned | pending |

The targeted tests cover 22 mutation operations, six unusable-session/lifecycle cases (missing, foreign, expired, logout, reset, deletion/restore), three lesson execution phases after committed admission, HTTP expiry after profile waiting, real S3 completion followed by worker cleanup, and fresh/missing/revoked message replays preserving the exact 30-write budget.

Negative control: a separate tracked-source copy at `/private/tmp/ovc-tutor-session-negative` retained the new identity API and tests but removed only the session/clock guard additions in the four service helpers. No checked-out implementation file or historical revision was rewritten. The wrapper ran `go test -race -count=1 -parallel=4 -run 'TestTutor(MutationsRejectTheAuthenticatedSessionAfterBlockAndUnblock|ChatUploadDiscardedAfterBlockAndUnblockDuringCompletion|SessionExpiryIsCheckedAfterWaitingForTheProfile)' ./internal/api`: exit 1, as required. The expiry request created a card with HTTP 201, the revoked uploader published with HTTP 201, and the mutation matrix changed business rows and returned success for multiple operations after block/unblock. This demonstrates sensitivity to the missing guard, rather than an unrelated failure. The corrected-tree targeted run passed before this isolated control; the corrected source was unchanged by the control.

The full backend run passed `internal/api` in 79.814 s, `internal/conventions`, command/startup tests and every other tested package except `internal/services/files`. Its only assertion failure was:

```text
--- FAIL: TestUploadRecoveryAdvancesPastLockedCandidate (1.81s)
    upload_protocol_internal_test.go:267: locked prefix stopped recovery: count 0 elapsed 115.521125ms error transaction: lock upload intent: timeout: context deadline exceeded
        transaction: lock upload intent: timeout: context already done: context deadline exceeded
FAIL ovc/backend/internal/services/files 13.740s
```

That unchanged test sets the recovery candidate deadline to 50 ms and the total pass to 200 ms, holds the first journal row and expects one later candidate to complete. Both candidates timed out in this run. Its recovery path does not call the changed tutor guards; the changed files-service fixture is a different test. This evidence is consistent with a timing-sensitive existing test under the full run, but is still a failed behavioral assertion, not an infrastructure pass or permission to retry. The recovery test and production budgets were not modified and the full run was not repeated. Resolution requires a separately approved change; a later isolated pass would not erase this failure.

Logs retained for local review: `/private/tmp/ovc-tutor-session-targeted.log`, `ovc-tutor-session-negative.log`, `ovc-tutor-session-tools.log`, `ovc-tutor-session-backend.log`, `ovc-tutor-session-vet.log`, `ovc-tutor-session-lint.log`, `ovc-tutor-session-sqlc.log`, `ovc-tutor-session-gofmt.log`, `ovc-tutor-session-docs.log` (missing-tool attempt) and `ovc-tutor-session-docs-ready.log`, all under `/private/tmp/`. The negative-control copy remains at `/private/tmp/ovc-tutor-session-negative`; the original implementation worktree remains at `/private/tmp/ovc-tutor-session-recheck` and was not rewritten. The fix directly touches the session/account trust boundary; independent security readiness review is required. No readiness or merge claim is made.

## Independent readiness source reviews

Fresh independent standard and full-security reviewers inspected the complete 33-file working diff against `5962f0466d56fbe33a541afb331fe679d73c969c`; neither found a new blocker, major or minor. Both confirmed S6-SEC-01 closed in the implementation and targeted regression evidence, while explicitly retaining the failed full backend status and missing browser/gate evidence. The standard reviewer recorded the reviewed path/content fingerprint `6f62dc9da4953f6e71bafa1b1135f2820ec043613cf6e477b62c58b91a8b3a05` before these review-result paragraphs were appended.

The security review traced required actor propagation, all four post-lock guards, transaction-local time, schedule admission/execution/retries/replay, tutor chat replay and upload rejection into durable cleanup. T-2/T-5/T-21/T-29, T-13/T-34, T-20 and T-18 were examined; unrelated credential-generation, password, rendering/CSP, conferencing, residency, dependency and test-control activation code is unchanged. The reviewer read the actual positive, negative-control and full-run logs but did not rerun commands. Audience retries were verified statically, without a new runtime revocation-between-retries scenario; HTTP expiry-after-profile-wait is exercised in tutoring, with analogous guards and the unusable-session matrix covering the other families. No global logout linearizability is claimed.

This fix touches the session/account trust boundary directly. No readiness-review finding required a correction. The separate recovery regression correction must pass and integrate before the complete backend check and gate can establish readiness on the final tree.

## Integration after recovery correction

The prerequisite recovery correction merged as `7026a62237db6d9e43a9630fd3f08abee31625ed`, exact tree `e0918de4d947427ab6579087eaaee137b082ecf4`, through a 250.0-second gate with all 30 commands and 944 E2E cases passing. Both failed correction commits and all three earlier gate failures remain retained. This tutor-session change is integrated into a new worktree from that main revision; the earlier worktrees remain preserved. All changed backend source and tests are byte-identical to the independently reviewed 5962-based implementation. The threat-model addition is appended to preserve the separately merged quota and recovery wording. No contract or behavior change was introduced by integration. Complete backend, affected production browser journeys and exact-tree gate evidence must cover this new tree; the original failed full-backend run remains recorded above.

## Integrated correction change checks

Before checks, `tools/check-tool-binaries.sh` passed. The complete `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go -C backend test -race -count=1 ./...` passed, exit 0; API 76.234 seconds, files 14.949 seconds. `go vet ./...`, `../bin/golangci-lint run ./...` (zero issues), `../bin/sqlc -f sqlc.yaml diff` and the empty `gofmt -l .` check passed, all exit 0. Frozen offline frontend installation and `pnpm api:check` passed without dependency or generated-client changes. Logs use `/private/tmp/ovc-tutor-integrated-{tools,backend,vet,lint,sqlc,gofmt,frontend-install,api-check}.log`. The earlier failed backend assertion remains historical evidence; this is the integrated tree containing the separately merged correction.

The affected production-browser check ran `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i pnpm e2e e2e/smoke.spec.ts e2e/messages.spec.ts e2e/message-attachments.spec.ts e2e/upload-transport.spec.ts e2e/students.spec.ts e2e/groups.spec.ts e2e/invitations.spec.ts e2e/join-requests.spec.ts e2e/ending.spec.ts e2e/teaching-settings.spec.ts e2e/schedule.spec.ts e2e/lesson.spec.ts e2e/live-updates.spec.ts` from `frontend/`. All 611 cases passed across Chromium, Firefox and WebKit in 1.7 minutes, exit 0; log `/private/tmp/ovc-tutor-integrated-e2e.log`. This covers the common smoke and the profile-locked tutor mutation families and their page/refetch consumers. Remaining full-run checks are deferred to the exact-tree gate.

A byte comparison confirmed all 26 changed backend source/test files equal the independently reviewed earlier implementation. The new main changes are the quota wording and recovery regression correction, with no overlapping production mutation. Both independent readiness reviews therefore still cover this unchanged implementation; no new review loop was started solely for integration. Final tier remains standard, security trigger met. The session guard fix directly touches the session/account trust boundary and closes S6-SEC-01 in code and real-path evidence, without claiming stronger logout linearizability. Final documentation/staged scanners, exact-tree gate and finish are required before merge and retained externally. No slice closure is claimed.

## Gate failure and approved verification follow-up

The integrated commit `edc448f165f02a81dad5cff931d0e90a9ef51e25` failed its exact-tree gate: 29 non-E2E commands passed and 943 browser cases passed, while the Chromium unknown-address logout case failed strict diagnostics on notifications/unread 401 responses. The wall time was 278.9 seconds against the unchanged 260-second ceiling. The common Git directory retains `ovc-merge-reports/edc448f-tutor-session-failed.log`; the old commit and branch remain intact. Neither merge nor unchanged retry occurred.

The owner approved [the verification follow-up](../plans/slice-6-tutor-gate-followups.md) on 2026-11-01 after independent security planning review. Its new branch starts at `7026a62237db6d9e43a9630fd3f08abee31625ed` and carries the unchanged production guard implementation. The follow-up synchronizes only the unknown-address logout journey and consolidates only the named tutor-session arrangements. Runtime controls, checks, a fresh combined standard/security review and one new gate remain pending for this tree. Earlier passing checks do not establish the new tree's readiness.

## Approved tutor-session fixture consolidation

Source status, 2026-11-01: implemented under the approved [gate follow-up plan](../plans/slice-6-tutor-gate-followups.md), on `7026a62237db6d9e43a9630fd3f08abee31625ed`. This section records source correspondence only. Runtime equivalence, new negative-control sensitivity, full checks, combined standard/security review and sufficient gate savings remain unverified. Earlier failures and passing runs above are unchanged historical evidence.

The local `directTutorSessionParties` fixture retains real tutor/student sign-in, account setup and the actual signed-in relationship acceptance. It creates no group. It is used only by the identity/lifecycle cases and the replay-budget test. The complete mutation matrix and S3 completion test retain their existing group-bearing fixtures; other helpers and tests are unchanged.

| Original arrangement | Replacement | Harnesses before → after | Groups before → after |
|---|---|---|---|
| Complete twenty-two-operation mutation matrix | Unchanged | 1 → 1 | 1 → 1 |
| HTTP expiry after profile wait | Unchanged | 1 → 1 | 0 → 0 |
| S3 completion followed by block/unblock and cleanup | Unchanged | 1 → 1 | 1 → 1 |
| Missing, foreign-owner and expired identities | One `identities` fixture with three independently saved observations | 3 → 1 | 3 → 0 |
| Logout, reset, deletion/restore | Three isolated harnesses with the local direct fixture | 3 → 3 | 3 → 0 |
| Lesson admission/execution, create/edit/cancel | Unchanged | 3 → 3 | 0 → 0 |
| Message replay/refusal write budget | Same test using the local direct fixture | 1 → 1 | 1 → 0 |
| Total | Two harness lifecycles and seven group arrangements removed | 13 → 11 | 9 → 2 |

### Assertion correspondence

Each `tutorGuardObservation` stores a case name, the four returned errors and complete before/after snapshots. The Act loop finishes collecting every observation before the Assert phase begins. Snapshots are not logged, business rows are not restored and counters are not reset. A later observation cannot overwrite an earlier observation or conceal its changed snapshot.

| Earlier case/assertion | New observation/assertion | Preserved condition |
|---|---|---|
| `missing` four guard errors and snapshot | `identities`, observation `missing`, `assertRefused` | Session ID `{99}` is nonexistent; account remains the real tutor |
| `foreign` four guard errors and snapshot | `identities`, observation `foreign`, `assertRefused` | Session belongs to the real authenticated student, not the tutor |
| `expired` four guard errors and snapshot | `identities`, observation `expired`, `assertRefused` | Captured real tutor session has `expires_at = authStart` |
| `logout` four guard errors and snapshot | Isolated `logout` observation, `assertRefused` | Real accounts-service logout revokes the captured session |
| `reset` four guard errors and snapshot | Isolated `reset` observation, `assertRefused` | Real forgot-password, worker mail and password-reset path revoke the session |
| `restore` four guard errors and snapshot | Isolated `restore` observation, `assertRefused` | Real account deletion followed by operator restore does not revive the session or discarded links |
| Fresh message replay error and message identity in missing/foreign/expired | One shared successful replay before tutor-session expiry, asserted after all three observations | Nil error and the original message ID; identical fixture, key and request shared only for these three cases |
| Fresh lesson replay error, result length and lesson identity in missing/foreign/expired | One shared successful replay before expiry, asserted after all three observations | Nil error, exactly one lesson and the original lesson ID |
| Fresh message and lesson replay controls in logout/reset/restore | Same controls in each isolated harness, before its account-wide action and asserted after its observation | Original error, cardinality and identity checks unchanged |
| Replay budget: 35 iterations of fresh, missing and revoked replay | Unchanged loop and final `wrongReplay` assertion | Each fresh replay returns the original message; each missing/revoked replay returns `chat.ErrUnauthenticated` |
| Replay budget: 29 subsequent sends and thirty-first write | Unchanged `writeErrors` and `ThrottledError` assertions | All 29 succeed; the next write is rate limited |
| Mutation, HTTP lock-wait, S3 cleanup, lesson admission/execution assertions | Original test bodies unchanged | Twenty-two refusals, exact business snapshots, safe outcomes, HTTP 401, durable cleanup and committed lesson-attempt accounting retained |

For every one of the six identity/lifecycle observations, the four exact comparisons remain `accounts.ErrUnauthenticated` for card creation, `chat.ErrUnauthenticated` for message replay, `schedule.ErrUnauthenticated` for lesson replay and `accounts.ErrUnauthenticated` for upload admission: 24 guard comparisons and six full snapshot equalities. The fresh-control predicates remain unchanged; their identical setup runs four times rather than six, with one result shared by the three immutable-identity variants. The snapshot query, service requests and upload-admission cleanup are unchanged. The profile-lock/session guards and public contracts have no follow-up edits.

Source-only checks: `gofmt -w backend/internal/api/tutor_session_recheck_test.go` and `git diff --check` passed, exit 0. No compiler, test, browser, scanner, performance, commit or gate command ran during this implementation step. The earlier independent reviews cover the preserved production implementation; the changed verification arrangements still require the approved fresh combined review and new execution evidence.

## Logout readiness sensitivity controls

Isolated source copy `/private/tmp/ovc-tutor-logout-controls` starts at `edc448f165f02a81dad5cff931d0e90a9ef51e25`. Its added `frontend/e2e/logout-readiness-control.spec.ts` holds only actual GETs to the exact notifications/unread paths after the real ready frame, then continues every request unchanged. Production source and canonical tests are untouched. No prior completed GET is required.

After a successful pinned-tool preflight and frozen offline install, the sanctioned wrapper ran `caffeinate -i pnpm --dir frontend e2e e2e/logout-readiness-control.spec.ts --grep negative`. All three negative cases failed only the existing strict diagnostics after real logout returned 204 and the navigation assertions succeeded: Chromium observed unread `net::ERR_ABORTED`; Firefox observed unread `NS_BINDING_ABORTED`; WebKit observed both notifications and unread HTTP 401. Exit 1 is the expected sensitivity result, not a passing application run. Log: `/private/tmp/ovc-tutor-controls-negative.log`. Only WebKit reproduced HTTP 401; the other engines demonstrated cancellation sensitivity.

The same command with `--grep positive` passed all three engines, exit 0, 3.7 seconds. It started the synchronized navigation while both paths were held, asserted the readiness wait had not completed and zero logout requests had started, released the unchanged requests, then required logout 204 and both original navigation assertions under ordinary strict diagnostics. Log: `/private/tmp/ovc-tutor-controls-positive.log`. The corrected canonical accessibility suite and full E2E run remain pending for the consolidated implementation tree.

## Consolidated backend execution evidence

Executed on the unchanged consolidated backend source in `/private/tmp/ovc-tutor-gate-followups`, base `7026a62237db6d9e43a9630fd3f08abee31625ed`. The command sequence was tools probe, targeted positive race suite, isolated negative suite, full backend race, vet, lint, sqlc and formatting. Heavy runs were serialized under the assigned slot; Go test commands used `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i` from `backend/`, retaining the existing local service configuration. No setup failure occurred in this sequence.

| Command | Exit and evidence |
|---|---|
| `tools/check-tool-binaries.sh` | 0, passed |
| wrapper `go test -race -count=1 -parallel=4 -run 'TestTutor(Mutations\|Session\|ChatUpload\|LessonExecution\|MessageReplays)' ./internal/api` | 0, passed, API 5.471 s |
| Same targeted command in the isolated negative source copy | 1, expected sensitivity failures, API 1.926 s; details below |
| wrapper `go test -race -count=1 ./...` | 1, failed, API 82.011 s; connection failures below; no rerun |
| `caffeinate -i go vet ./...` | 0, passed |
| `caffeinate -i ../bin/golangci-lint run ./...` | 0, passed, zero issues |
| `caffeinate -i ../bin/sqlc -f sqlc.yaml diff` | 0, passed |
| `test -z "$(gofmt -l .)"` | 0, passed |
| `git diff --check` | 0, passed |

Logs are `/private/tmp/ovc-tutor-followups-{tools,targeted,negative,backend,vet,lint,sqlc}.log`. Formatting and whitespace checks returned no output. The full run passed every other tested package, including files (16.114 s), startup (37.928 s), core, testcontrol and conventions; the relevant startup/control tests therefore ran within that full check, but the aggregate backend result remains failed.

The separate negative copy `/private/tmp/ovc-tutor-followups-negative` contains the consolidated tests and current main recovery corrections. A byte comparison against the implementation confirmed exactly four differing files, containing only removal of the new session/clock checks in `chat/release.go`, `files/chat_uploads.go`, `tutoring/deletion.go` and `schedule/schedule.go`. Identity propagation and all tests remain intact. Every saved `missing`, `foreign` and `expired` observation independently failed all four refusal comparisons and its snapshot equality; logout and reset did likewise. Restore incorrectly allowed card/message/lesson operations while upload answered channel-read-only instead of unauthenticated, and its snapshot changed. All three post-admission lesson executions returned success after logout. The HTTP expiry and S3 block/unblock controls again answered 201, and the twenty-two-operation matrix changed business state. The replay-budget test executed its complete 35 replay/refusal iterations, 29 subsequent writes and final write before failing its `wrongReplay` assertion; its later rate assertions are not claimed to pass in the negative run. The positive run executes and passes all retained replay and rate assertions. No unrelated or infrastructure failure occurred in the negative control.

The full positive run failed eleven existing API subcases on connection establishment, rather than a changed tutor-session predicate. Failures included three daily-attachment-cap cases, both reclaim-expiry ordering cases, two reclaim keep/drop cases, both relationship-ending notification cases, the operator-block-before-invitation-send case and the pending-upload completion/quota case. The log contains eleven named failed subcases; all failure lines are retained rather than inferred from their parent totals. Representative exact errors:

```text
chat_uploads_test.go:595: Post "http://127.0.0.1:63947/api/v1/auth/login": dial tcp 127.0.0.1:63947: connect: operation timed out
reclaim_test.go:673: outbox lane storage_delete: claim outbox rows: failed to connect to `user=ovc database=ovc_46b971a226b4_test`: 127.0.0.1:55432 (127.0.0.1): dial error: timeout: dial tcp 127.0.0.1:55432: connect: operation timed out
```

The affected in-process HTTP ports ranged from 63926 to 63947; PostgreSQL also failed to accept connections at port 55432. No consolidated tutor-session test appears in the failure list. These observations do not identify the underlying host/network cause or authorize an unchanged rerun. No timeout, rate threshold or assertion was changed, no failed command was repeated, and no commit or merge was made. Readiness and sufficient gate savings remain unverified pending resolution and the remaining approved checks/review.

### One controlled full-backend rerun

After the initial connection failures, read-only infrastructure checks found both local OVC containers healthy with four-day uptime and PostgreSQL accepting connections through `pg_isready`. No sleep event was found in the inspected failure interval; `caffeinate` had been active. These observations did not establish a cause. One controlled change-check rerun was explicitly authorized within the approved verification work; this was not a merge-gate retry and invoked no gate exception. Source, tests, concurrency flags and deadlines were unchanged.

The same root-wrapper command, `caffeinate -i go test -race -count=1 ./...` from `backend/`, failed again with exit 1. Log: `/private/tmp/ovc-tutor-followups-backend-controlled.log`. API took 77.140 s; every other tested package passed, including startup (39.035 s) and files (13.355 s). Existing API failures again included local HTTP and PostgreSQL connection timeouts and fixture database-name reads failing with `network`. This time there were also two explicit lock-wait assertions, so the result must not be described as solely connection failures without failed assertions:

```text
--- FAIL: TestTutorStateChangeRacingALinkHasOneOutcome/the_tutor's_deletion_before_signed-in_acceptance (10.55s)
    deletion_effects_test.go:866: 1 requests wait on the held row, directly or behind another waiter, instead of 2
--- FAIL: TestTutorStateChangeRacingALinkHasOneOutcome/the_operator's_block_before_signed-in_acceptance (10.56s)
    deletion_effects_test.go:866: 1 requests wait on the held row, directly or behind another waiter, instead of 2
```

Other failing cases were the three removal-like lesson-effect cases, both unverified-linked-account deletion/cleanup cases, the group-lesson edit with changed members, both deletion-ending notification cases and the two request-visibility notification cases. No consolidated tutor-session test appeared in the failure list. The source of the recurrent host/network failures and the relationship of those failures to the waiter assertions remain unverified. No further run, code edit, timeout adjustment or commit followed; the heavy-check slot was released. Both failed full-run logs remain evidence, and the change is not ready to merge.

## Frontend verification after interruption

The final frontend source passed `pnpm --dir frontend typecheck`, `lint` and `format:check`, exit 0. The initial component run failed one lint-rule case out of 665: its five-second timeout elapsed after 571732 milliseconds while the machine was asleep. Power-management evidence records idle sleep at 20:10:51 on 2026-11-01 for 573 seconds and wake at 20:20:24 (Europe/Moscow), overlapping the run started at 20:10:48. The failed log remains `/private/tmp/ovc-tutor-followups-frontend-test.log`.

A sleep-protected sequential check using `caffeinate -i` passed all 665 component tests and then build, classes, bundle and generated API checks, each exit 0. Logs: `/private/tmp/ovc-tutor-followups-protected-{test,build,check-classes,check-bundle,api-check}.log`. Code, assertions and timeouts were unchanged. This confirmed sleep explanation applies only to this component run; the earlier two backend failures remain unexplained and failed. Full E2E and independent review are pending.

## Combined after-fix review

The one approved fresh combined standard/full-security review completed without a new blocker, major or minor in the corrections. It reviewed the complete 35-file scope against main `7026a62237db6d9e43a9630fd3f08abee31625ed`, compared the preserved implementation with `edc448f`, and confirmed the mapping, real foreign session, saved observations, replay/rate predicates and unchanged diagnostics. Reviewed source hashes: tutor regression `82a41d4099eb16b37d9405282b4b21b3140bc8b5`, accessibility `6f46f9f6c60d1a508cd66147462a666c861d9d49`. The reviewer examined T-2/T-5/T-21/T-29/T-13/T-34/T-18/T-20/T-41 and found no new trust-boundary change from fixture consolidation. Existing production session guards directly affect the session/account boundary.

The review explicitly returned failed readiness due to both full backend failures and pending runtime evidence. It did not claim the two lock-wait assertions were caused by networking. It verified the browser controls and targeted backend sensitivity; it did not rerun heavy checks. No new source fix or additional review round was requested. Later successful frontend checks are recorded separately and do not erase the backend failures. A full slice review and performance proof remain outstanding.

## Complete browser run and isolated lock diagnostic

The sleep-protected full E2E command through the sanctioned wrapper passed all 944 cases in 2.8 minutes, exit 0, including the corrected canonical unknown-address logout case in Chromium, Firefox and WebKit. Log: `/private/tmp/ovc-tutor-followups-e2e.log`. No retries, diagnostic allowances or deadline changes were used.

After E2E, an isolated diagnostic command through the sanctioned wrapper ran `caffeinate -i go -C backend test -race -count=1 -run '^TestTutorStateChangeRacingALinkHasOneOutcome$' ./internal/api`, exit 0. Log: `/private/tmp/ovc-tutor-followups-lock-diagnostic.log`. This is diagnostic evidence for the family containing the two failed waiter assertions. It does not replace the failed complete runs, establish the root cause or permit readiness/merge. A third full backend run and the new gate have not been started.

Documentation installation passed offline with the frozen lockfile. The first documentation check failed MD012 on four extra blank lines in this record; those formatting defects were corrected without changing source or acceptance criteria. The failed output remains `/private/tmp/ovc-tutor-followups-docs.log`.

## Bounded backend connectivity diagnostic

Executed on 2026-11-01 under the separately approved [connectivity diagnostic plan](../plans/slice-6-backend-connectivity-diagnostics.md). The temporary implementation is not part of the tutor change and has not been approved for retention or merge. Standard tier and security trigger T-18/T-20/T-41 apply to diagnostic safety and resource lifecycle; no new readiness review or production change is claimed.

The disposable copy `/private/tmp/ovc-backend-connect-diagnostic` contains the approved source at main `7026a62237db6d9e43a9630fd3f08abee31625ed` plus the uncommitted tutor changes. A manifest of 938 tracked and nonignored files was recorded before copying; ignored environment files were neither read nor copied. The diagnostic and negative-control copies each differ from that manifest only in `backend/internal/api/deletion_effects_test.go`. The original worktree matched every manifest entry again after execution and before this record was appended. Manifest: `/private/tmp/ovc-backend-connect-source-manifest.json`, SHA-256 `45085e1334ef1920bf0bbccb32b00f8671dccdf53f2a9f86109ff1504b0e4fa6`. Original deletion test SHA-256: `125f3285abfcdd7e0ba4f3bd7407248eb98adaea463c4befa7e1f6a9b89d6e4f`; diagnostic deletion test: `1a90f60a30d6948c825e1aa41e9a16206afb68f0681669f688fe9d560084cf7d`. Patch: `/private/tmp/ovc-backend-connect-diagnostic.patch`.

A wrapper calls the unchanged `waitForLockWaiters(t, 2)`. Its deferred failure observation nonblockingly classifies the buffered second request as pending/absent, a safe `core.ErrorKind` transport category, or an HTTP status. Any observed value is preserved in a replacement buffered channel. Raw errors, bodies, tokens, addresses, identifiers and SQL are not emitted by the diagnostic; ordinary successful response assertions remain unchanged. The original ten-second waiter deadline and failure predicate are unchanged. Structured HTTP error-code decoding was omitted to avoid unsafe response helpers. Supplementary database/pool observations are unavailable: the failure boundary exhausts the original deadline, so no supplementary query, driver import, testdb API or new diagnostic deadline was added.

Preflight `tools/check-tool-binaries.sh` passed, exit 0. The initial unprivileged process/container probe could not access the Docker socket or process list; a permitted elevated read-only probe confirmed both OVC containers healthy, PostgreSQL accepting connections, 12 logical CPUs and no overlapping test executable in the observed process summary. The coordinating session reserved the heavy-check slot. Test commands used the sanctioned root wrapper and `caffeinate -i`; no concurrency, timeout, retry or network setting changed.

Commands and results:

- `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go -C /private/tmp/ovc-backend-connect-negative/backend test -race -count=1 -run "^TestTutorStateChangeRacingALinkHasOneOutcome/the_operator's_block_before_signed-in_acceptance$" ./internal/api`: exit 1, expected sensitivity failure, package 11.276 seconds. The isolated control omits the actual second request, leaving a real pending channel. The original assertion fails with one waiter instead of two; the safe observation reports `pending_or_absent`. Cleanup completes without hanging. Log: `/private/tmp/ovc-backend-connect-negative.log`.
- `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go -C /private/tmp/ovc-backend-connect-diagnostic/backend test -race -count=1 -run '^TestTutorStateChangeRacingALinkHasOneOutcome$' ./internal/api`: exit 0, package 2.389 seconds. All original family assertions pass. Log: `/private/tmp/ovc-backend-connect-positive.log`.
- `python3 /private/tmp/ovc-backend-connect-observer.py`: exit 0. It invokes exactly once `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go -C /private/tmp/ovc-backend-connect-diagnostic/backend test -race -count=1 -json ./internal/api`, with a 240-second outer process bound. The direct subprocess exit is 0, independent of any pipeline. API passes in 70.181 seconds: 1118 test/subtest pass events and one package pass event, zero failures and zero failure-diagnostic emissions. JSON log: `/private/tmp/ovc-backend-connect-api.jsonl`, SHA-256 `5b4fb571eb2b8280fc20aacd6f559d7665d15858556d82d264fba04d68b8d287`; exact subprocess command/exit: `/private/tmp/ovc-backend-connect-command.log`.
- `gofmt -w` on the diagnostic and control deletion test files: exit 0. Original-worktree `git diff --check`: exit 0. The full source-manifest comparison: no original-worktree change before this evidence append.

The external observer recorded only timestamped aggregate host TCP states and process counts every five seconds, with bounded read-only subprocesses. It stores no process arguments, addresses or packet payloads. Its stop event and thread join run in `finally`; both completed on command exit. All 14 samples have successful process/socket collection, with a maximum of one Go process, one API test process and one caffeinate process among the monitored executable names. Aggregate host ranges were 895–2871 established TCP connections, 0–12 SYN_SENT and 0–1 TIME_WAIT. These include unrelated applications, do not locate a failing connection and establish no causal relation. Log: `/private/tmp/ovc-backend-connect-host.jsonl`, SHA-256 `4898bd4ce86c32f782d4d065c1d806eb70f4e041dcdfc6ef4c18c6f682105454`.

The negative control proves the original failure and cleanup remain sensitive to an absent request; the positive family and one API-package run did not reproduce either reported failure. Root cause and the relation between the earlier TCP connect failures and two waiter assertions remain unverified. Both complete backend failures remain failed evidence. This isolated API pass is not a passing full-backend check, readiness result, performance proof or merge-gate pass. No retry, third complete backend run, gate, commit or merge was performed. Any durable fix or retained diagnostic requires a separately approved scope and its required verification.

## Approved cross-package observation

The owner approved one full-backend observation on 2026-11-01. Main remained `7026a62237db6d9e43a9630fd3f08abee31625ed`; the source hashes stayed unchanged. Another project chat was idle, pinned tools matched, local PostgreSQL and storage were healthy and PostgreSQL accepted connections before execution. The original tutor worktree was used, without the disposable diagnostic patch and with normal package/test parallelism.

`/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go -C /private/tmp/ovc-tutor-gate-followups/backend test -race -count=1 -json ./...` passed, exit 0, without watchdog expiry. All tested packages passed; API 84.420 seconds, files 18.325 seconds, startup 38.166 seconds. No test failure event occurred. The five-second aggregate collector completed stop/join. Logs are new files `/private/tmp/ovc-backend-cross-package-{tests.jsonl,host.jsonl,command.log}` and the owned runner is `/private/tmp/ovc-backend-cross-package-observer.py`; earlier evidence was not overwritten.

This is a successful complete check of this source state. It does not establish the cause of either earlier backend failure, prove that networking or concurrent packages caused the waiter assertions, or erase either failed result. No code fix for the unexplained failures was made. No repeated observation, gate, commit or merge was performed under the diagnostic approval. Root-cause uncertainty remains open for the readiness decision and slice reconciliation.

## Owner readiness decision

On 2026-11-01 the owner explicitly accepted the unresolved cause of the two historical backend failures for proceeding to one previously planned exact-tree gate, with the unchanged 260-second ceiling. This is a bounded readiness decision, not a claim that the failures were fixed or infrastructure-only. Every failed result remains in this record. A new gate failure or budget overrun prevents merging and requires a new decision; no unchanged repeat is authorized. Root-cause uncertainty remains part of slice reconciliation.

Final tier is standard and the security trigger remains met. The approved combined after-fix review found no new source findings; its runtime gaps have successful subsequent frontend, full E2E and complete backend evidence as recorded above. The session guard fix directly changes the session/account trust boundary. Documentation and staged secret scanning must pass before committing; exact gate/finish evidence will be retained externally without editing the tested tree.
