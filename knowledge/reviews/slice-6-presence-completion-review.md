# Presence completion review

Status: pre-merge readiness evidence on 2026-10-31. Implementation, independent reviews and change checks are complete; the final exact-tree merge gate is the remaining step when this record is written. The expanded per-instance plan and the operator/purge lock exception are approved. Source implementation `0afee87`, recovered onto base `e95d272` in `feat/chat-presence-completion`.

## Independent static findings

- Major, standard and security: `backend/internal/realtime/presence.go`, lines 82–100 in the recovered source, logs a failed persistence attempt but still removes leaving sockets and forgets the account. The registry can report offline while the persisted presence remains online. The failure path needs a real integration test and a defined recovery lifecycle.
- Major, standard: `backend/internal/realtime/socket.go`, lines 454–455, enqueues ready before asynchronous initial persistence. A send immediately after ready can treat the account as offline. Existing tests wait separately for persisted presence and do not prove the ready boundary.
- Contract conflict, both reviewers: `backend/queries/presence.sql` unconditionally overwrites the account's shared presence. Closing the last local socket can mark an account offline while another API still has its socket. This follows the existing per-instance closing algorithm, so a correction requires an approved contract amendment rather than silently changing the implementation.
- Existing minor limitation: presence's user-row key-share locks also wait for operator and purge update locks, contrary to the broad promise that they wait only for deletion. A contract clarification or a separately approved operator-lock change is pending.
- Performance risk, standard: the global presence-write mutex serializes account writes and its wait is outside the individual database timeout. Reconnect churn with database failures needs examination alongside lifecycle fixes.

These are preliminary independent findings over the recovered implementation, not a final review of a fixed diff. No finding has been downgraded. Both reviewers read source without running checks or editing files. No demonstrated typing authorization or student-presence privacy bypass was found; that observation is not a runtime security verdict.

## Executed evidence

`tools/check-tool-binaries.sh`: exit 0, `bin/ matches tools/go.mod`.

From `backend/`, using the original checkout's sanctioned `tools/dev-env.sh` to load local settings without opening or copying environment files:

```bash
go test -race ./internal/api -run 'Test(Presence|TheLastSocket|AClockAdvance|Typing|StudentsReceiveAnonymous|APresenceBatch|TheGatewaysTimers)' -count=1
```

Exit 0; `ok ovc/backend/internal/api 5.813s`. This targeted pass does not cover the findings above. Full change checks, notification-rate measurements, final independent reviews, merge gate and finish are not run. No implementation correction, commit or merge has been made.

## Isolation and tool setup

The original source branch remains intact. A separate worktree integrates its diff with current main, and keeps dependencies and pinned binaries local. Offline pnpm installation failed because the temporary-volume store lacked tarballs; existing identical-lockfile dependency directories were copied from the original checkout instead. No lockfile or dependency version was changed. Machine snapshots showed Claude Code working in another project and no active test runner; those observations must be refreshed before further checks.

## Expanded per-instance plan review

The proposed per-instance lease design received independent correctness and security planning reviews on 2026-10-31. The correctness review identified two major planning gaps: a missing multi-instance test-clock protocol and no total bound on pending account records. Both were corrected in the plan. A fresh scoped reviewer applied the standard checklist to the fixes and the full security profile to the affected boundaries, explicitly confirmed closure of both major planning findings and returned passed for planning, with no new blocker or major finding. The reviewed plan SHA-256 is `7fecd4fd631291f745b64118d5d4f0c2125b40fedae8e06c38c01b3aa9645cbb`.

The security review also required bounded admission, transactional deferred signal emission, monotonic extension and an execution deadline independent of frozen business time; the revised plan contains these constraints and acceptance tests. Threats examined: T-20, T-21, T-29, T-36, T-44, T-18 and T-34. Runtime behavior remains pending implementation and verification.

The owner approved documenting short operator/purge lock waits without changing the locks. Approval of the expanded per-instance implementation is pending. Markdownlint over the plan and this record passed before this review-status addition. No application source was changed for the expanded design.

## Implementation and independent after-fix review

The implemented model adds per-process lease rows and an account summary serialized by the summary lock. The gateway uses four writers and a 4096-account total capacity with bounded queue admission, durable presence before ready, retained failed releases and explicit local test-control errors. Expiry cleanup is bounded and rechecks concurrent renewals and account purges. Both clock operations acknowledge local writes; a multi-instance controller must complete the absolute-time protocol before assertions or worker execution.

The independent after-fix reviewer applied the standard checklist to the changes and the full security profile to the complete diff, confirmed closure of the two original major findings, and found no new blocker or major. Both fixes touch trust boundaries: readiness touches authentication; pending-release handling touches presence lifecycle and test control. Minor findings concerned missing diagnostic counts and missing negative coverage. The implementation now records pending/unreleased counts and distinguishes announcement failure; tests cover reconnect before recovery, shutdown with an unresolved release, forbidden diagnostics, and the four-writer bound. These minor corrections require checks, not another review round under the repository policy.

Latest targeted race command covers the presence, typing, scheduler, readiness, failure, capacity, shutdown, delayed-emission and cleanup families in the real API and chat service. Exit 0: API 3.694 seconds, chat service 3.059 seconds. This includes the new concurrency and safe-diagnostics tests. SQL generation comparison exited 0. Earlier targeted failures exposed test adaptation and fixture-query mistakes; each was corrected before rerunning. The full backend race run passed the S5 packages but failed the unrelated calendar-dependent admin-restore test, whose separately approved fix is being verified. No failed assertion was treated as infrastructure or retried without a correction.

The separate admin fix gate failed on the expired tools-module x/net advisory exceptions. Its backend race checks passed. An independently reviewed, separately approved dependency update removes that blocker; no unchanged-tree gate rerun is authorized by this failure. Full S5 checks, the complete D61 record, and the exact-tree gate/finish remain pending.

## Additional completed checks

Frontend typecheck, lint, Prettier, component tests, build, class check, bundle check and API generation comparison all exited 0. Component tests: 54 files and 579 tests. Entry JavaScript plus the largest language chunk: 172.8 KB gzip against 180 KB; student Home first load: 187.8 KB against 250 KB; tutor Today: 188.4 KB against 400 KB. Backend vet and lint exited 0, including lint with the opt-in measurement build tag.

`TestPresenceMigrationCanBeRolledBackAndReapplied` passed through real goose Up, Down and Up against the test database (startup race invocation, 5.835 seconds). The expanded D61 quick check passed for one and two APIs; the [rate record](../analysis/chat-presence-rate-2026-10-31.md) keeps the workload, outputs and measurement limits.

The first documentation-check attempt found hard tabs copied from the Go test output into the rate record; these were converted to spaces. Documentation checks must pass again on the final tree. Minor lint failures in new test helpers and time-source defaults were corrected without weakening lint policy. Full exact-tree checks remain pending the independently approved verification-baseline recovery.

The first complete S5 end-to-end run finished with 901 passing tests and one failing realtime test: its presence helper threw on a normal in-flight persistence response. The test-control route now uses the existing `503 service_unavailable` structured code for pending presence; the helper polls only that typed temporary state, and still throws on any other failure or malformed unavailable response. This is a correction to the observed failure, not an unchanged flaky-test retry. Targeted and full follow-up results remain to be collected.

All 38 affected realtime end-to-end tests passed after the explicit pending-presence handling correction (14.0 seconds, all three engines). The first full run's 901-pass/one-failure result remains recorded; no unchanged test retry was used. The verification-baseline gate later failed its wrapper self-test because the coordinator had inherited the full development environment into the worktree gate. A clean gate must retain only the fixture database/storage settings, not application settings or the root development-database password. The earlier assertion failure remains a failure and requires an explicit owner-approved repeat rather than being called infrastructure.

The owner explicitly authorized one repeat of the unchanged verification-baseline tree with the corrected environment on 2026-10-31. Only `OVC_TEST_DATABASE_ADMIN_URL`, `OVC_DEV_STORAGE_ACCESS_KEY`, `OVC_DEV_STORAGE_SECRET_KEY` and `OVC_DEV_STORAGE_PORT` are retained from the sanctioned wrapper for that gate; all other OVC settings are removed from its command environment without printing values. This authorization is specific to the diagnosed launch error and is not a standing relaxation of the merge gate's after-failure rule. The original failed assertion and every earlier attempt remain in the report.

The verification-baseline recovery subsequently merged as `d23c7fd` after its full gate passed in 196.9 seconds; finish confirmed tested and main tree `a4c365984be6cc2ecf0ca0bac691ec24492c0946`. The [merge evidence](local-verification-recovery-merge-2026-10-31.md) records every command and exit result. S5 is integrated separately on this repaired baseline. A final aggregation edge case was tightened: an older sampled release or expired-lease cleanup must not place the offline instant before recorded activity; dedicated real-database regressions and generated SQL checks are required for this correction.

The same independent after-fix reviewer completed its scoped review on the final `d23c7fd` baseline, confirmed closure of both minor findings and found no new findings. It also examined the typed pending-response correction, the two offline-time regressions, migration rollback/reapply coverage and the stated measurement limits, without restarting the full review or running tests. Its final scoped verdict is passed; the original major closures remain confirmed. Full exact-tree checks remain the coordinator's responsibility.

On the repaired final integration base, the first full backend race run failed one existing notification-purge case while opening a database connection: `dial tcp 127.0.0.1:55432: connect: operation timed out`. Startup, migrations, chat service (including both offline-time regressions), worker and the other packages passed. This was a database connection failure, not a failed business-value assertion. One infrastructure recheck is recorded separately; no application timeout, test timeout or assertion was changed.

## Final change-check result

The single infrastructure recheck of the full backend race suite exited 0 on the final integration base: API 61.344 seconds, chat service 5.533 seconds, startup 21.587 seconds. Backend vet, lint in ordinary and measurement-tag modes, SQL generation comparison, staged Gitleaks and documentation checks all exited 0. The latter checked 605 unique links with no errors. Both offline-time regression cases are included in the passing chat-service suite. The temporary advisory acceptances, wrapper-isolation exception and fixed restore test from main are preserved unchanged by S5.

The same after-fix review is complete and passed; minor corrections were checked without introducing another fresh review round. The full exact-tree gate, not a compile-only check or a self-review, decides whether this commit may merge.
