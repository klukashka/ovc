# Presence shutdown diagnostic outcome

Date: 2026-11-06. Status: the single approved diagnostic execution passed; historical failure cause remains unverified. This record is not application acceptance, a production fix, a gate result or permission to merge.

## Scope and preflight

The [approved diagnostic plan](../plans/slice-7-presence-shutdown-diagnostic.md) added four passive observations only to the existing presence admission/shutdown test, in an isolated worktree. Every copied application and existing test file matched the meeting implementation except the seven observation/timing additions to that one test; one new test-only helper supplied fixed fields. Original assertions, 100-millisecond operation timeout, one-second outer shutdown deadline, race detection and normal parallelism were unchanged. The diagnostic helper and calls are not part of the application change.

Independent standard and full-security pre-execution reviews passed without findings. Formatting, go vet, strict lint, diff whitespace and staged Gitleaks passed. There were no new database queries, waits, goroutines, timers or retries. A single synchronized log copy per stage was reduced to predefined event/kind counters; IDs, credentials, raw logs and arbitrary error strings were not exported by the helper. Observations are sequential across components and can perturb scheduling; they are not a globally atomic snapshot.

## Execution

Command: `go -C backend test -race -json ./...`, through the sanctioned root `tools/dev-env.sh` wrapper and the owned sleep/source monitor. Exit 0, 96.47 seconds, all 2102 named tests/subtests passed, none failed or skipped. Exactly four snapshots were present. All 21 monitor samples were safe; source identity was unchanged. The private output directory was mode 0700 and the log mode 0600. The run used battery power with owned idle/display sleep protection; no power-source change invalidated the monitor.

| Observation | Connected account | Waiting account | Failed presence batches | Incomplete shutdown events | Acquired connections |
| --- | --- | --- | --- | --- | --- |
| Before admission | online, one socket, not pending | offline, zero sockets, not pending | 0 | 0 | 2 |
| After admission refusal and lock release | online, one socket, not pending | offline, zero sockets, pending | 2 timeout events | 0 | 2 |
| After shutdown | offline, zero sockets, not pending | offline, zero sockets, not pending | same 2 timeout events | 0 | 0 |
| After scheduler advance | unchanged | unchanged | unchanged | 0 | 0 |

Admission lasted 110.91 ms; shutdown lasted 20.04 ms. Neither the cancellation nor deadline state of the outer shutdown context was set. There were no release-pending or announcement-failure events in these observations, no new events after scheduler advancement and no unknown error-category count. The original durable lease/summary and timer assertions passed. Two admission-time failed-batch events do not identify two accounts or establish an ordering beyond the observed stage; no stronger attribution is made.

## Conclusion and remaining decision

The observed pending admission recovered during shutdown. This run did not reproduce the previous retained-lease assertion and cannot identify its cause. In particular, the early-return hypothesis in ExtendPresence is neither proven as the historical cause nor disproven for other orderings. No production change follows from non-reproduction alone.

The earlier ordinary 2101/2102 backend failure and documentation-browser ConnectionClosedError remain recorded in the [storage verification record](slice-7-meeting-recovery-storage.md). No further runtime attempt, documentation retry, gate or merge is authorized by this diagnostic outcome. A separately approved bounded readiness decision or causal-fix plan is needed to proceed. Remote conferencing evidence remains unverified.

Exact diagnostic files: `chat_presence_capacity_test.go` SHA-256 `1ead9784f4bf98875cddf8523763f29a3b39e80c4d920fd04a0c62cb3ddca605`; `presence_shutdown_observation_test.go` SHA-256 `0cb2ba1c9cce6ecdcae5313ed2c8ab3e3c797c00c21abb85a538a22d6b60c5dd`. Raw full-suite output and source/monitor manifests stay private outside served directories and repository content. Only fixed safe counts, durations and revision evidence appear here.

## Later deadline change

Later change, recorded 2026-11-07 by the slice 7 slice review: commit `3023459` ("Remove load and race assumptions from presence and capacity tests"), approved by the project owner on 2026-11-07, raised the presence-operation deadline of six presence tests from 100 ms to 2 s: `TestPendingPresenceAccountsHaveATotalCapacity`, `TestPresenceOperationsRespectAdmissionAndShutdownDeadlines`, `TestReadyFailsWhenPresenceCannotCommit`, `TestAFailedPresenceReleaseRemainsPending`, `TestShutdownWithPendingPresenceReportsIncompleteRelease` and `TestChatPresenceFailureRecordsCarryNoBodiesNamesOrAddresses`. The deadline also applied to the connection each test makes during setup, so on a busy machine the socket closed with 1012 before the test began. The admission bound of `TestPresenceOperationsRespectAdmissionAndShutdownDeadlines` scales with it, from 1 s to twice the 2-second deadline. The one-second outer shutdown deadline is unchanged. This change did not establish the cause of the earlier retained-lease failure described here.
