# Slice 6 measurement tooling verification

Status: corrected smoke and applicable backend/frontend checks passed; independent source review completed and its minor corrected. Final documentation/secret checks and the exact-tree merge gate remain pending. Full measurements have not run.

The source worktree starts at `fd8739dd08a263d6b35214f762e2198a7c650499`. Integrate the final upload and authentication implementation before running the probes. No full measurements have run. Earlier compile, typecheck, lint and database-name guard checks preceded substantial source edits and do not validate the current diff.

## Database and lock fixture confinement

The optional E7 fixture applies `ALTER DATABASE` only to its freshly created, loopback, owned end-to-end database before migration. It checks ownership and the database name, observes the requested value through a fresh connection, and verifies the administrative database setting is unchanged. It never changes a role, server setting or Go test database. The Go observer only reads `SHOW synchronous_commit` and requires `on`.

The presence lock helper owns a transaction in a dedicated psql child. Readiness is an explicit stdout handshake after acquiring the row lock; release sends COMMIT and EOF and awaits child exit. Its own session has a ten-second statement timeout and thirty-second idle transaction timeout solely as emergency bounds if setup or cleanup fails. These settings do not change application connections. No sleep or pg_sleep synchronizes the fixture. Readiness and exit timers are cleared. A cleanup timeout terminates the owned child and fails the run; it is not a workload result or an application crash experiment. Independent security review must examine the lock and child lifecycle, including spawn failure, blocked lock acquisition, broken stdin and deadline cleanup.

## Pending verification

After integration, run targeted formatting, TypeScript, ESLint, Go tagged compilation, tagged vet and tagged golangci-lint; the default gate alone may not include `slice6_measurement` files. Run the database guard tests, compare E7 on/off test inventories, and execute every documented small smoke runner and Go fixture through the real local services. Smoke output is explicitly not performance evidence. Browser selectors, all-page loading, native process accounting and resource cleanup remain unverified until those runs. Full workload measurements require the final production revision and an otherwise idle machine.

## Initial readiness review findings and source corrections

The initial standard review covered the complete measurement tooling diff and its fixture/resource lifecycle consequences. The initial security review covered the complete diff, including owned database/process confinement and cleanup. These remain independent review scopes; source corrections below are not a review pass.

- Major, security: a rejected held-presence release could skip process termination and database removal. Teardown now collects every release outcome, attempts every process stop and database removal, and returns aggregate errors. Tracked releases remove themselves in finally.
- Major, standard: browser/socket acquisition before a cleanup scope and sequential closing could leak later resources on failure. Each successful acquisition now immediately registers cleanup. A shared scope attempts all registered actions in reverse acquisition order and preserves operation and cleanup errors in an AggregateError. Socket churn failures and individual socket close failures cannot skip remaining socket, browser or deployment cleanup.
- Minor, standard: browser smoke upload metadata reported full-workload bytes. It now reports the actual buffer byte length.
- Minor, security: the held-download shutdown probe could close its HTTP server before cancellation and barrier release on a fatal path. One deferred cleanup now cancels, releases the barrier and closes any response before closing the owned server.

These corrections are source-only and unverified. No builds, tests, installs or smoke runs were executed for them. After integration and change checks, one fresh combined standard and full-security after-fix review is required. Both major fixes affect fixture resource ownership and cleanup trust boundaries; the security minor fix also changes fixture resource lifecycle. Production behavior, timeout values and retry policy are unchanged by these corrections.

## Integration preparation

The tooling was copied into a new worktree based on production commit `7026a62237db6d9e43a9630fd3f08abee31625ed`. The original worktree and all its evidence remain intact. Only the five tracked source/document diffs and nineteen owned new source/document files were transferred; environment files, caches and generated outputs were excluded. Existing development-guide and threat-model additions from production were preserved when appending the measurement sections.

Source inspection confirmed the merged upload-intent, capacity and cleanup-receipt schema names and phases. The traced harness uses the current constructor hook and does not replace production behavior. A source-level signature mismatch in two resource-counter calls was corrected by using a typed query over the fixture pool. Browser attachment labels and fifty-message pagination were checked against source; this is not runtime selector validation.

The integration base is not yet the final performance baseline: pending authentication, daily-accounting and interface changes must merge before full measurements. No checks have run against this integration tree.

Once the coordinator grants the exclusive check slot, run the following static and small fixture checks before any full workload:

```sh
pnpm --dir frontend install --offline --frozen-lockfile
(cd frontend && pnpm typecheck)
(cd frontend && pnpm lint)
(cd frontend && pnpm format:check)
(cd backend && go test -tags slice6_measurement ./internal/api -run '^$')
(cd backend && go vet -tags slice6_measurement ./internal/api)
(cd backend && golangci-lint run --build-tags slice6_measurement ./internal/api/...)
(cd frontend && pnpm test e2e/measurement-database.test.ts)
(cd frontend && pnpm e2e --config e2e/database-measurement.config.ts --list --project='*-on')
(cd frontend && pnpm e2e --config e2e/database-measurement.config.ts --list --project='*-off')
```

Use the existing development environment wrapper for commands needing local services. The smoke commands and exact runner parameters are in the development guide's measurement section; they require newly built production binaries and frontend output from this same integration tree. Install is offline and frozen only: no dependency changes or downloads are part of this work.

## First integrated static checks and source fixes

At integration base `7026a62237db6d9e43a9630fd3f08abee31625ed`, before the fixes in this section, the coordinator reported:

- `go test -tags slice6_measurement ./internal/api -run '^$'`: passed, 0.690 seconds, compile only.
- Tool probe and tagged Go vet: reported passed; their complete invocation/output remains in the coordinator's evidence, not independently rerun here.
- Tagged golangci-lint: failed with five reported diagnostics in `/private/tmp/ovc-measurements-integrated-lint.log`: two forbidden driver imports in the API test layer and three displayed `time.Now` findings. Repeated findings were capped in the output; source inspection found thirteen such calls in the newly added measurement files.

The opt-in driver tracer, pool construction and EXPLAIN execution now belong to `backend/internal/testdb/measurement.go`, guarded by `slice6_measurement`. The API tests use its typed capture surface and opaque query records, without driver imports. No new package, dependency or lint exception was introduced; the approved query-count/plan instrumentation remains outside production builds. All thirteen new measurement calls to `time.Now` now use the existing `clock.System` adapter. Unrelated code is unchanged.

The new testdb helper expands the affected paths to the full backend change-check row, in addition to explicitly tagged compile, vet and lint. Run these checks after receiving the exclusive slot; the source fixes are not validated by the earlier compile/vet passes. Gofmt was applied to the affected Go source successfully; no tests, builds or workloads were run during the fixes. The combined standard/full-security after-fix review must include the testdb helper and its capture/report boundary.

## Static verification before tutor-session priority work

The first local tool probe failed because this new worktree had no `bin` link. Linking the existing pinned main-worktree binaries resolved that setup issue; the next probe passed. The first tagged compilation was blocked by sandbox access to the Go cache, without running assertions. With the required cache access, tagged API and testdb compilation passed (0.690 and 1.022 seconds, no tests run). Tagged vet passed and tagged golangci-lint reported zero issues. Frozen offline frontend installation passed without downloads.

Frontend typecheck initially failed on Window extension casts, a Node performance identifier used in a browser callback and an out-of-scope cleanup registration. After source corrections, the typecheck command completed without diagnostics. Full frontend lint then failed with ten findings. An ESLint autofix pass resolved seven formatting/style findings and still reported three errors: an unnecessary conditional in the browser probe, require-await and prefer-optional-chain in the socket probe. These failures remain retained; the final frontend source has not completed the full static sequence or runtime validation. Format verification was not run. Work paused to prioritize the approved tutor-session gate correction; this is not a readiness claim.

Evidence is retained under `/private/tmp/ovc-measurements-static-*.log`, including preflight, compile, vet, lint, install, both typecheck attempts, frontend-lint and eslint-fix logs. Remaining source fixes require rechecking the final affected static checks before smoke workloads and the combined review. No full performance measurement was run.

## Static attempt before final production integration

On the integration tree based on `7026a62`, the explicitly authorized static sequence produced these retained local logs (all under `/private/tmp/ovc-measurements-static-`):

- `bash tools/check-tool-binaries.sh`: exit 1 (`preflight.log`), tools absent in the fresh worktree. After linking the existing pinned main `bin/`, exit 0 (`preflight-2.log`).
- `go test -tags slice6_measurement ./internal/api ./internal/testdb -run '^$'`: sandbox setup exit 1 (`compile.log`), Go cache inaccessible and zero assertions executed. With cache access, exit 0 (`compile-2.log`), API 0.690 s and testdb 1.022 s, no tests run.
- `go vet -tags slice6_measurement ./internal/api ./internal/testdb`: exit 0 (`vet.log`).
- `../bin/golangci-lint run --build-tags slice6_measurement ./internal/api/... ./internal/testdb/...`: exit 0, zero issues (`lint.log`). The earlier five-diagnostic lint failure is separately preserved.
- `pnpm --dir frontend install --offline --frozen-lockfile`: exit 0, reused 399, downloaded zero (`install.log`).
- `pnpm typecheck`: exit 2 (`typecheck.log`), two Window typing errors, Node performance shadowing in page evaluation and a misplaced socket cleanup registration. After source corrections, exit 0 (`typecheck-2.log`).
- `pnpm lint`: exit 1, ten errors (`frontend-lint.log`). Targeted eslint autofix exited 1 with three findings remaining (`eslint-fix.log`): browser sampling-state conditional, socket cleanup callback lacking await and delivery optional chaining. No runtime assertions ran. The remaining source fixes were deferred when the static slot ended.

Formatting verification, runtime smoke and all full measurements remained unrun. These checks do not validate the later tree below.

## Final production source integration

The new worktree is based on `baabaf179973fdfab1ee5f5fb6280a553219ec6d`, including tutor-session, immutable daily-accounting activation and Messages fixes. Only known source/document diffs were transferred. A three-way content merge of deployment.ts retained the production read-only activation query and its initial clock selection; auth fixture additions retained tutor service wiring and the new cleanup job. Previous worktrees and failed evidence remain intact. AGENTS and the verification skill are unchanged from the previously read integration base.

The three remaining lint findings were corrected in source without exclusions. Daily accounting is now reported separately from upload intent/capacity/receipt quiescence. Transfers assert 124 MB retained per student/tutor pair after all object deletion, and Go lifetime probes assert 100 successful 1-KB uploads remain charged in the daily ledger. Existing actors already satisfy daily/hourly/message bounds: they are not relying on refunds. Full transfer concurrency remains 32; success/refusal/disconnect loops remain 100 with ten rotating students. All participant setup precedes timing. Go probes read the actual activation and reject a fixture clock before it; they never update the activation or truncate its table.

A new opt-in daily-accounting cleanup fixture uses 500 synthetic students with 10,000 historical rows, observes each real worker pass and full drain, holds one owned student row to prove other students progress, then releases it. It checks the existing 100-student and 1,000-row per-pass bounds and retention of 500 current-day rows. Smoke reduces this to four students and twelve past rows. The production SQL batch size remains 100 and the job owns its existing 20-second deadline. This covers the causally added worker component without changing policy or production code.

All final-integration edits are source-only and unverified. Required next steps are fresh tool preflight, tagged compile/vet/lint over API and testdb, frontend type/lint/format, full backend checks, guard tests, every real-service smoke runner including daily cleanup, and one fresh combined standard/full-security after-fix review. Full performance measurements require a frozen final revision and otherwise idle machine.

Source formatting on the final integration completed successfully: gofmt on the changed Go probes and Prettier on the twelve affected frontend files, using the already installed formatter from the prior worktree. Prettier output is retained at `/private/tmp/ovc-measurements-final-format-source.log`. No dependencies were installed or copied into the final worktree, and no compiler, linter, test or workload ran in this source-only phase.

## Final-base static verification

The following commands ran sequentially against the measurement working tree based on `baabaf179973fdfab1ee5f5fb6280a553219ec6d`. Logs are retained under `/private/tmp/ovc-measurements-final-`; none of the earlier failures was overwritten. Commands run at repository root unless a working directory is given.

| Command | Result | Log suffix |
|---|---|---|
| `bash tools/check-tool-binaries.sh` | Exit 0; existing pinned main bin matches tools/go.mod | `preflight.log` |
| Backend: `go test -tags slice6_measurement ./internal/api ./internal/testdb -run '^$'` | Exit 0; API 1.223 s, testdb 0.718 s; no tests executed | `compile.log` |
| Backend: `go vet -tags slice6_measurement ./internal/api ./internal/testdb` | Exit 0 | `vet.log` |
| Backend: `../bin/golangci-lint run --build-tags slice6_measurement ./internal/api/... ./internal/testdb/...` | Exit 0, zero issues | `go-lint.log` |
| `pnpm --dir frontend install --offline --frozen-lockfile` | Exit 0, 399 reused, zero downloaded | `install.log` |
| Frontend: `pnpm typecheck` | Exit 0 | `typecheck.log` |
| Frontend: `pnpm lint` | Exit 0, including the project source/policy scripts | `frontend-lint.log` |
| Frontend: `pnpm format:check` | Exit 1, only measure-slice6-browser.ts formatting | `format-check.log` |
| Frontend: `pnpm exec prettier --write scripts/measure-slice6-browser.ts` | Exit 0; formatting-only correction | `format-fix.log` |
| Frontend: `pnpm format:check` | Exit 0 after that correction | `format-check-2.log` |
| Frontend: `pnpm test e2e/measurement-database.test.ts` | Exit 0; seven tests passed | `guard.log` |
| Frontend: `pnpm e2e --config e2e/database-measurement.config.ts --list --project='*-on'` | Exit 0; 947 tests, 26 files, inventory only | `e7-on-list.log` |
| Frontend: `pnpm e2e --config e2e/database-measurement.config.ts --list --project='*-off'` | Exit 0; 947 tests, 26 files, inventory only | `e7-off-list.log` |

A local Python comparison selected all inventory lines beginning with two spaces and `[`, removed only the project `-on`/`-off` suffix, required 947 entries and exact ordered equality, and exited 0 (`e7-inventory-comparison.log`). No E2E cases ran. The tagged compile requires no TestMain services in these packages. No runtime measurement, full backend run, production build, readiness re-review or merge gate was attempted in this phase. Static success and guard/inventory evidence do not establish runtime harness correctness or performance targets.

## First real-service smoke attempt

This run is smoke verification, not performance evidence. Source remained frozen on the final-base integration tree. `bash tools/check-tool-binaries.sh` exited 0 (`/private/tmp/ovc-measurements-smoke-preflight.log`). Production binaries and frontend were built through the existing harness:

```sh
/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i node --input-type=module -e 'import { prepareRun, takeRunLock, finishRun } from "./frontend/e2e/deployment.ts"; takeRunLock(process.env); try { prepareRun(); } finally { finishRun(); }'
```

Build exit was 0; `/private/tmp/ovc-measurements-smoke-build.log` has file creation timestamp 2026-11-01T20:13:24.700118Z and final write timestamp 20:13:27.404403Z (filesystem evidence, not an instrumented duration). No owner database migration ran.

The following command ran from backend through the root wrapper:

```sh
/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i go test -tags slice6_measurement ./internal/api -run '^TestSlice6' -count=1 -parallel=1 -timeout=30m -v -args -slice6-smoke
```

It exited 1, starting 2026-11-01T20:13:45Z and stopping 20:14:55Z; package time 69.760 seconds. Exact output and start/stop/exit are `/private/tmp/ovc-measurements-smoke-go.log` and `/private/tmp/ovc-measurements-smoke-go.status`.

Five probes failed:

- DailyUploadCleanup, ChannelList and Digest: `measurement channel seed failed` (0.97, 0.25 and 0.23 seconds respectively).
- DownloadLifetime: `measurement held download did not open` (60.26 seconds); the cancellation case advanced, but the subsequent held case failed. No conclusion about production leakage follows from this invalid fixture run.
- OfficeInspection: the bounded 10,000-entry, 8,214,964-byte archive returned 415 rather than expected 201. The refusal and remaining cleanup observations are not a passed inspection-bound measurement.

Seven probes passed their small assertions: PresenceCleanup, StudentChannelList, History, Erasure, UploadLifetime, TutorPurge and StudentPurge. Go continued the already-running command after the first failed test because the authorized command did not specify failfast. The coordinator was informed when failures were discovered; no CLI runner, fix or retry followed. The process completed naturally, with no termination signal and ordinary fixture cleanup; no cleanup errors were reported. A process-only check confirmed that its go, caffeinate and api.test PIDs no longer existed. This is not a separate database/object inventory audit.

All six CLI smokes remain unrun. The failed seed, held-download and office fixture paths require diagnosis and coordinated correction before further execution. The heavy slot was released; no performance result or readiness claim is made.

## Source corrections after first smoke failures

No production code or admission/inspection deadline was changed. The five failed smoke outcomes and seven passes above remain the evidence for the previous tree; the following fixes are not yet executed.

- `slice6_dataset_measurement_test.go`: compared the historical seed statements with migrations 00004, 00015 and 00017. Their relationship states, colour, identifiers and member counters match those constraints. Both branches of the membership UNION supplied untyped `$4` for timestamp columns; UNION resolves unknown output types before the surrounding INSERT supplies its target type. Timestamp parameters now explicitly cast to timestamptz in all seed stages. Failures report the fixed stage label and `core.ErrorKind`, including SQLSTATE when supplied by PostgreSQL, never raw errors, SQL, arguments or credentials. The previous generic failure did not retain SQLSTATE, so the exact runtime cause remains to be confirmed by the coordinated targeted smoke rather than claimed as observed.
- `slice6_lifetime_measurement_test.go`: the transparent S3 hold now relays an unchanged 64-KiB prefix, enough to pass downstream HTTP buffering, instead of 1 KiB. The held client request has a ten-second observation context. Production download timing, data, status and headers are unchanged; deferred cancellation, barrier release, response close and owned server close still run on failure.
- `slice6_office_measurement_test.go`: source inspection of `inspectOOXML` and its existing near-bound tests confirms that `relationshipsMaxBytes` is a shared 8-MiB inflated XML budget. The former accepted fixture necessarily exceeded it with two 8-MiB parts plus package relationships. The accepted fixture now uses one large relationship part plus the package root totaling exactly 8 MiB; the refusal fixture uses two parts plus the root totaling 8 MiB plus one byte. Both retain 10,000 entries, exactly 1-MiB content types and a central directory bounded by 4 MiB. A binary-part default content type makes filler OPC parts explicit. Metadata reports total relationship bytes including the root, actual directory bytes and archive bytes. This is an aggregate-relationship refusal; the separate 32-MiB outer-read budget is not falsely represented as isolated coverage.

Gofmt completed for the three modified Go probes. Development-guide wording was corrected to the actual inspection contract. No compiler, tests or workload ran during these corrections. Next execution is the coordinator-authorized five failed cases with failfast, then the complete Go smoke only after that passes, with CLI smokes still pending.

## Corrected smoke attempt and new upload-lifecycle failure

The pre-run scope manifest is `/private/tmp/ovc-measurements-corrected-scope.txt`: base `baabaf179973fdfab1ee5f5fb6280a553219ec6d`, SHA-256 `eb2b38cb1c16bad48afcfc86ed266560f71eadf9ed3f84ef3b6801a8669fa3ac` over sorted changed/untracked source and documentation names and bytes, each separated by NUL. This hash precedes this evidence update.

Fresh `bash tools/check-tool-binaries.sh`, backend `go test -tags slice6_measurement ./internal/api ./internal/testdb -run '^$'`, `go vet -tags slice6_measurement ./internal/api ./internal/testdb`, and `../bin/golangci-lint run --build-tags slice6_measurement ./internal/api/... ./internal/testdb/...` all exited 0. Logs are `/private/tmp/ovc-measurements-corrected-{preflight,compile,vet,lint}.log`; compile API took 0.672 s with no tests, testdb was cached, lint reported zero issues.

Both commands below ran from backend under `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i` with `go test -tags slice6_measurement ./internal/api`, `-failfast -count=1 -parallel=1 -timeout=30m -v -args -slice6-smoke`:

- `-run '^TestSlice6(DailyUploadCleanup|ChannelList|Digest|DownloadLifetime|OfficeInspection)Measurement$'`: exit 0, five cases passed, package 3.492 s, 2026-11-01T20:24:57Z to 20:25:02Z. Logs and timestamps: `/private/tmp/ovc-measurements-corrected-targeted.log` and `.status`. The accepted office archive returned 201 and the aggregate-relationship refusal returned 415. No old SQLSTATE was recovered; the timestamp diagnosis remains source-derived.
- `-run '^TestSlice6'`: exit 1, package 4.522 s, 20:25:13Z to 20:25:19Z. Logs and timestamps: `/private/tmp/ovc-measurements-corrected-all.log` and `.status`. DailyUploadCleanup, ChannelList, Digest, PresenceCleanup, StudentChannelList, History, Erasure and DownloadLifetime passed. UploadLifetime failed at its immediate `assertLeftNothing` after disconnect: an incomplete multipart upload remained. The later named worker-driven cleanup helper had not yet run at that assertion. This observation is a failed probe, not a production leak verdict or an infrastructure retry.

Failfast stopped the command before office and purge probes in that run. No CLI smoke, source correction or repeat followed. The process exited normally; a process-name-only inventory found no go, caffeinate or api.test processes remaining. Ordinary fixture cleanup reported no error, but no separate database/object inventory audit was performed. The coordinator received the failure and the heavy slot was released. Both runs are smoke-only, not performance measurements.

## Upload quiescence correction and CLI smoke progression

The disconnect case now captures its real stored-upload key before ending the body, runs the existing worker-driven 30-second cleanup condition, then calls the original full `assertLeftNothing` on that captured upload. No assertion, resource class, production deadline or admission limit was removed. The helper also verifies the captured key's object and multipart state before returning. This fixes the premature physical-cleanup assertion without assuming that an HTTP failure means durable cleanup has finished.

Gofmt and the same tagged API/testdb compile, vet and golangci-lint commands passed (all exit 0; logs `/private/tmp/ovc-measurements-lifecycle-{compile,vet,lint}.log`). API compile was 0.677 s with no tests; testdb cached; lint zero issues.

From backend, with the root wrapper and caffeinate, `go test -tags slice6_measurement ./internal/api -failfast -count=1 -parallel=1 -timeout=30m -v -args -slice6-smoke` ran with:

- `-run '^TestSlice6UploadLifetimeMeasurement$'`: exit 0, package 2.733 s, 2026-11-01T20:29:04Z to 20:29:09Z. Exact log/status: `/private/tmp/ovc-measurements-lifecycle-targeted.log` and `.status`.
- `-run '^TestSlice6'`: exit 0, all twelve Go smoke cases passed, package 10.414 s, 20:29:17Z to 20:29:28Z. Exact log/status: `/private/tmp/ovc-measurements-lifecycle-all.log` and `.status`.

Next, from frontend under `/Users/klukashka/Work/ovc/tools/dev-env.sh caffeinate -i`, the CLI runners each performed their existing production harness preparation:

- `node scripts/measure-slice6-contexts.ts firefox messages on --smoke`: exit 0, 20:29:38Z to 20:30:02Z; `/private/tmp/ovc-measurements-cli-contexts.log` and `.status`. One warm-up and two measured smoke contexts, no console/page/request failures, and zero remaining contexts/pages were observed. The logged socket-close counter was zero; that observer limitation must not be interpreted as proof that the browser retained live sockets after context destruction.
- `node scripts/measure-slice6-sockets.ts --smoke`: exit 1, 20:30:15Z to 20:31:03Z; `/private/tmp/ovc-measurements-cli-sockets.log` and `.status`. Sixty lightweight sockets and two browser observers sent 61 messages with zero refusals and zero unexpected closes, but three reconnect batches failed and zero reconnects completed. This failed sample's latency data does not establish delivery targets. The generic reconnect failure does not identify its root cause; diagnosis remains pending.

The socket failure stopped the sequence. Transfers, independent presence, upload-process and browser CLI smokes remain unrun. No fix or retry followed that failure. Processes exited normally and a process-name-only inventory found no final-worktree executable or caffeinate remaining; no independent object/database audit is claimed. The source code hash, excluding changing evidence documentation, is recorded in `/private/tmp/ovc-measurements-lifecycle-code-scope.txt`. All earlier failures remain retained. These are smoke runs only, not full measurements; the heavy slot was released after recording the result.

## Socket close observation correction

Source tracing found that the shared application `webSocketFactory.close` intentionally clears its `onclose` handler before requesting close. The measurement runner used that detached handler as its only close-completion signal, so its ten-second close wait necessarily expired before reconnect authentication could begin. The failed smoke did not observe server admission refusals; the gateway sends `ready` only after authentication, session recheck, presence preparation and account/socket admission. No production contract changes are needed.

The opt-in socket runner now installs a once-only native close listener independent of application handler detachment, records the actual close event and code, and updates actor state only for the same socket generation. It still waits for actual close and actual server `ready` with the original ten-second deadlines. Actor selection overlap remains an explicit failed observation rather than silently reducing the requested ten-actor churn batches. Safe per-actor stage/kind diagnostics and pre-ready close codes supplement batch failures; raw errors, frames, credentials and content are never emitted. All workload sizes, timers, admission limits and final assertions remain unchanged. The earlier exit-one sample remains retained. These source corrections await fresh checks and a single coordinated smoke attempt.

Fresh tool preflight, frontend typecheck, lint and format:check all exited 0; source Prettier also exited 0. Logs are `/private/tmp/ovc-measurements-reconnect-{preflight,typecheck,lint,format,format-source}.log`. The source hash before runtime is `/private/tmp/ovc-measurements-reconnect-code-scope.txt`. Lint and format verification briefly overlapped; no runtime workload overlapped either check.

The first sandboxed socket invocation exited 1 before build/protocol output in 0.177 seconds (`/private/tmp/ovc-measurements-reconnect-sockets.log` and `.status`). A read-only filesystem W_OK probe confirmed write access denied for the machine run-lock directory under the owner cache. No workload assertions executed in that setup failure. With sandbox access corrected, the sanctioned wrapper/caffeinate command ran `node scripts/measure-slice6-sockets.ts --smoke` once; output and timestamps are `/private/tmp/ovc-measurements-reconnect-sockets-runtime.log` and `.status`. It exited 1 after a new delivery assertion failure. Twenty reconnects completed in approximately 31–46 ms, with zero reconnect failures, unexpected closes or HTTP refusals. The native close-observation correction therefore exercised the real reconnect path. The delivery deadline failed for one steady message: 20 recipients observed against 30 intended. The summary recorded 60 sends, one error and 20 reconnects. This is a failed smoke sample, not delivery performance evidence; the missing recipients require causal diagnosis before another execution. Transfers, independent presence, upload-process and browser smokes were not started. No unchanged retry or further source correction followed the assertion.

The corrected service run started at 2026-11-01T20:38:28.930620Z and exited at 20:39:07.647457Z. A process-name-only inventory after exit found no matching measurement runner, caffeinate, api.test or final-worktree executable; ordinary cleanup emitted no separate cleanup error. This is not an independent database/object inventory audit. The sole incomplete delivery was the second group's sequence 11. The retained log does not identify its ten missing actor indices or their socket generations, so it cannot prove the exact close/send race. The realtime specification explicitly recovers missed data through REST after ready; lightweight frame-only accounting does not yet prove that recovery. No assertion was relaxed and no recovery result is claimed.

## Churn-aware completion accounting

The socket runner preserves intended recipients and their original generations. It records actual close-request, close and ready times plus every recipient's frame, interruption and recovery disposition. Only a missing recipient whose original generation was interrupted and actually closed, and whose replacement generation reached ready, can use a real authorized REST `after_seq` request that verifies the exact message identifier. Fetch uses an AbortSignal bounded by the remaining shared ten-second deadline; response parsing and completion recheck the same deadline. Stable missing recipients still fail. No denominator is removed. Interrupted or frame-incomplete messages retain observed frame maxima separately but report the all-recipient maximum as incomplete, excluding them from passing frame p95 claims. This custom REST recovery is never described as production refetch or UI latency. Development-guide wording is updated within its pending owner-review section.

The initial new typecheck failed on null/array diagnostic fields, because measurement records accept primitive values only; a second attempt retained one null-field diagnostic. Per-recipient events and explicit string sentinels resolved those failures; the third typecheck exited 0. The first lint exited 0, the second failed on one unnecessary nullish fallback immediately after assigning ready time, and that fallback was removed. The third lint exited 0. Logs remain under `/private/tmp/ovc-measurements-catchup-`. No runtime was retried during those source corrections.

## Churn accounting smoke and independent runner progression

Final catch-up source checks passed: tool preflight, third frontend typecheck, third frontend lint and format:check exited 0. Source scope is `/private/tmp/ovc-measurements-catchup-code-scope.txt`. The changed-source socket smoke ran once through the sanctioned root dev-env wrapper and caffeinate, starting 2026-11-01T20:42:26.261367Z and exiting 0 at 20:42:55.548110Z (`/private/tmp/ovc-measurements-catchup-sockets.log` and `.status`). It sent 60 messages with zero refusals/errors/unexpected closes and completed twenty reconnects. For second-group sequence 11, ten actors (40–49) had their original generation actually close and their replacement reach ready; all ten exact messages were verified through custom REST recovery in the shared deadline. The all-recipient frame maximum remains incomplete for that message, not a passed latency target.

Transfers smoke (`node scripts/measure-slice6-transfers.ts --smoke`) exited 0, 20:43:14.240160Z to 20:43:39.939317Z; `/private/tmp/ovc-measurements-cli-transfers.log` and `.status`. Daily accounting observed and expected 229,376 bytes across eight pairs. Unsupported resource counters remain explicitly unverified. A process-name-only inventory found no matching owned runner/processes before proceeding.

The first presence launch was rejected by automatic approval review because the preceding read occurred before the transfers status file was written, leaving its successful completion insufficiently evidenced to that review. No presence process ran from that rejected launch. A subsequent read proved transfers exit 0 and no matching owned processes; the authorized next-runner launch then proceeded with an explicit prior-status check. This is a blocked launch, not a measurement failure or an unchanged workload retry.

Presence smoke (`node scripts/measure-slice6-presence.ts --smoke`) emitted `presence_expiry_seed` for cycle zero, then `measurement.failed` with `presence_run_failed`, and exited 1. Logs and exact start/stop are `/private/tmp/ovc-measurements-cli-presence.log` and `.status`. It did not reach the first per-cycle process result. This failed smoke requires diagnosis; no successful expiry, release-backlog or quiescence evidence is claimed. Upload-process and browser smokes were not started. No source change or retry followed this new failure. All results here are smoke verification, not full performance evidence.

## Presence fixture scheduling correction

The presence run exited 1 at 2026-11-01T20:45:15.251927Z after starting at 20:44:22.123377Z. A process-name-only inventory after exit found no matching owned runner or executable; no independent database/object audit is claimed. Source inspection establishes that `cmd/worker` intentionally waits without scheduling jobs under `OVC_CLOCK=test`, while the clock route advances time and flushes local presence but never ticks jobs. Safe event-only parsing of the owned worker log found one process.started, one process.stopped and zero job.run records. Thus the former expiry poll had no scheduled worker to remove its synthetic expired lease.

The harness now calls the existing real worker Tick route after seeding and advancing the expiry clock. Its HTTP abort and the original expiry condition share the unchanged thirty-second total deadline. No scheduler or production setting changes. Final release retry continues through the existing clock-triggered `ExtendPresence`, which queues pending revisions and waits for confirmed persistence; no row deletion or synthetic success bypass is introduced. Native close observation is extracted without semantic change into a measurement-only helper shared by sockets and presence. Presence registers each connection for cleanup before awaiting readiness and uses the existing aggregate cleanup scope so a failed close/release cannot skip later socket or deployment cleanup. A binary frame is a failed sample, never a fabricated close event. Safe stage diagnostics distinguish fixture/condition failures without raw error text. The development guide explains explicit test-clock worker driving in its pending-review measurement section. These source corrections await fresh static checks and one changed-source presence smoke.

## Corrected presence smoke and upload-process failure

Presence correction checks: preflight/typecheck/format passed; the first lint failed on prefer-const and a void-expression callback, both corrected. Fresh second typecheck/lint/format passed. Exact logs are `/private/tmp/ovc-measurements-presence-fix-*.log`; code scope is `/private/tmp/ovc-measurements-presence-fix-code-scope.txt`. The one changed-source presence smoke exited 0 from 2026-11-01T20:48:11.789447Z to 20:48:36.438775Z (`presence-fix-runtime.log` and `.status`). Both cycles retained one live lease, online state and one primary socket after the independent peer stopped. The pending release backlog was observed and then drained to zero leases. The shared socket helper is a semantic-preserving extraction of already exercised native close observation, so the socket workload was not repeated.

Upload-process smoke ran next from 20:48:49.931617Z to 20:49:33.838460Z and exited 1; `/private/tmp/ovc-measurements-cli-upload-process.log` and `.status`. Cancel mode completed with transport outcome zero and zero capacity, receipts and uploading attachments. Shutdown mode produced no cycle result before `upload_process_run_failed`. Its exact failure stage was not logged. A safe event/kind-only parse of the owned peer log found process.started but no process.stopped or failure diagnostic. The fixture reuses the same peer log name across peers, so that file does not retain the earlier cancellation peer's history. A process-name-only inventory after exit found no matching owned processes. Browser smoke remains unrun. No production fix, timeout change, exit-policy exception or unchanged retry followed.

Source diagnosis identifies a timing mismatch, not an observed production verdict: shutdown mode keeps its client request stream open and awaits peer.stop before cancelling it in finally. The shared harness stop deadline is twenty seconds, followed by SIGKILL if late; production API shutdown has a forty-second context and stops uploads at that context's deadline. The existing process shutdown integration test instead releases its held server-side barrier immediately after SIGTERM and asserts normal drain below forty seconds; it does not exercise an indefinitely held request stream. The generic top-level measurement failure cannot by itself prove the exact stop outcome. Any timeout alignment or altered shutdown measurement semantics needs explicit coordination before execution.

## Independent browser smoke

After the upload-process failure, the coordinator separately authorized the first independent browser smoke without retrying the upload workload. `node scripts/measure-slice6-browser.ts --smoke` ran through the sanctioned wrapper/caffeinate, 2026-11-01T20:51:44.807795Z to 20:52:39.463930Z, and exited 1. Exact output/status are `/private/tmp/ovc-measurements-cli-browser.log` and `.status`. Chromium reported two channel switches, eight pictures and four 4,096-byte uploads, then `browser_run_failed`; no browser cleanup or Chromium DOM-counter record was emitted. These observations do not identify the exact failed selector or subsequent operation because the top-level failure remains generic. No later engine result, final quiescence or browser success is claimed. A process-name-only inventory after exit found no matching owned measurement/caffeinate/api.test/final-worktree executable; no independent object/database audit was performed. No source correction or retry followed.

A narrow proposed shutdown-window plan is retained in `knowledge/plans/slice-6-measurement-shutdown-window.md`, pending fresh security planning review and explicit owner approval because its fixed measurement-only observation window would increase a check timeout. No shutdown source semantics or timeout was changed. The twenty-second ordinary stop and five-second kill deadlines remain unchanged. The full workloads and change checks remain pending, as do the combined independent after-fix review and exact-tree merge gate.

## Browser navigation selector correction

Source tracing establishes a concrete invalid selector after uploads: the Settings navigation link targets `/settings/profile`; that route's title is `page.settingsProfile`, translated to Profile, and Page renders it as the level-one heading. The former exact Settings heading wait could never match this route. Attachment removal's Remove file label and synchronous queue removal match the existing selector; Chromium-only CDP setup uses the supported context session surface. The harness now waits for the exact level-one Profile heading without changing timeouts or product behavior. Safe per-action stage labels and fixed timeout/operation-failed categories preserve failure context without raw exception text. The earlier browser exit-one sample remains retained; no earlier passing action is treated as full browser success. Source checks and one coordinated changed-source smoke remain pending.

## Approved shutdown observation implementation

The owner approved the narrow shutdown-window plan on 2026-11-01 after its independent security planning review. Source now exposes only a named upload-drain option on owned measurement peers; it selects exactly 45 seconds. Default stop remains 20 seconds and kill observation remains 5 seconds. Neither nonzero exit nor forced termination is accepted. Measurement peers emit fixed safe outcome classes and numeric exit/signal observations, and each peer receives a distinct log name. Upload outcomes have a ten-second bounded observation after cancellation; nested aggregate scopes preserve operation errors and attempt request and deployment cleanup. The `--smoke --shutdown-window-control` mode selects the unchanged 20-second shutdown path on one held admitted request and retains exit-one failure; it is not a passing workload and cannot select a full series. Cancellation and shutdown full-series counts remain one hundred each and three samples; no full series was authorized. Source checks, one authorized negative control, changed shutdown smoke and corrected browser smoke remain pending.

The coordinator separately ran `go test -race -count=1 ./...` over the unchanged backend sources and reported exit 0, with API 82.548 s, files 32.368 s, startup 39.370 s, testdb 4.880 s and worker 18.959 s. Exact full output is `/private/tmp/ovc-measurements-change-backend.log`; its preflight passed in `/private/tmp/ovc-measurements-change-preflight.log`. This evidence does not validate subsequent frontend source changes or opt-in tagged probes.

## Approved shutdown-window controls and corrected browser verification

Fresh tool preflight and source formatting passed. The first frontend typecheck failed on optional function narrowing across a cleanup callback; the first lint failed on a void-expression timeout callback. Both were corrected, with their failure logs retained. Final typecheck, lint and format:check exited 0. Logs are `/private/tmp/ovc-measurements-window-{preflight,typecheck,typecheck-2,typecheck-3,lint,lint-2,lint-3,format,source-format,source-format-2,source-format-3}.log`. The final source fingerprint and per-file SHA-256 list are `/private/tmp/ovc-measurements-window-code-scope.txt`, based on `baabaf179973fdfab1ee5f5fb6280a553219ec6d`. Source did not change during the following three runs. These checks precede staging of new files; Git-file-list-based policy/scanner checks must be repeated after the full owned scope is staged.

Each command ran from frontend under the root `tools/dev-env.sh` wrapper and caffeinate, sequentially, with complete log and timestamp/status files retained:

- `node scripts/measure-slice6-upload-process.ts --smoke --shutdown-window-control`: intentional negative-control exit 1, 2026-11-01T20:58:21.447132Z to 20:59:04.636384Z. `/private/tmp/ovc-measurements-window-negative-control.log` and `.status` record `peer_stop` outcome `forced_exit`, default window, SIGKILL, and failure stage `shutdown_peer`. This proves sensitivity of the unchanged twenty-second path; it remains a failed lifecycle sample and is never counted as workload success. Its distinct fixture label preserves control logs. No unrelated failure event was reported. Post-exit process-name-only inventory found no matching owned process.
- `node scripts/measure-slice6-upload-process.ts --smoke`: exit 0, 20:59:27.356086Z to 21:00:29.825641Z. `/private/tmp/ovc-measurements-window-upload-process.log` and `.status`. Cancellation used the unchanged default window and exited cleanly. Shutdown used only the approved measurement upload-drain window, exited normally with code zero, and returned HTTP 400 for the interrupted upload; its cycle duration was 38.439 seconds. Both cases observed zero upload-intent capacity, cleanup receipts and uploading attachments. Object/multipart counters remain unverified in this process probe, separately exercised by the Go lifetime probes. No forced or nonzero exit was accepted.
- `node scripts/measure-slice6-browser.ts --smoke`: exit 0, 21:00:37.708515Z to 21:01:09.828709Z. `/private/tmp/ovc-measurements-browser-corrected.log` and `.status`. Chromium, Firefox and WebKit each completed the smaller history, two switches, eight pictures and four 4,096-byte uploads, navigation, and cleanup observations. Completed requests equaled started requests (81/81, 81/81 and 80/80), active downloads and diagnostics were zero; Chromium DOM counters were observed. WebKit layout shift remains explicitly unsupported; heap/timer retainers and bounded query caches remain unverified.

A final process-name-only inventory after browser exit found no matching owned measurement/caffeinate/api.test/final-worktree executable. No independent database/object inventory audit is claimed. All ordinary small runners now have successful exercised assertions on the corrected source or semantically unchanged scopes; every preceding failed run remains retained. These are smoke checks only. Full workload series were not run. Source is frozen for staging, full applicable change checks and the fresh combined independent standard/full-security after-fix review; no commit or merge occurred.

## Staged backend change checks

After all 27 owned files were staged, the backend check scope remained based on `baabaf179973fdfab1ee5f5fb6280a553219ec6d`. `/private/tmp/ovc-measurements-readiness-scope.json` records SHA-256 `2bd3eeaea1a13c1953951a76b45a23c1d845e7160811e18c4d44e3a5bf284edf` of the staged binary diff and the exact sequential commands. No source or evidence changed while these checks ran. Each command has a separate `/private/tmp/ovc-measurements-readiness-<name>.log` and `.status` with UTC start/stop and exit:

- Root `bash tools/check-tool-binaries.sh`: exit 0, 0.190 s (`preflight`).
- Backend `unformatted="$(gofmt -l .)" && test -z "$unformatted"`: exit 0, 0.068 s (`gofmt`).
- Backend `go vet ./...`: exit 0, 0.925 s (`vet`).
- Backend `../bin/golangci-lint run ./...`: exit 0, 3.371 s (`go-lint`).
- Backend `../bin/sqlc -f sqlc.yaml diff`: exit 0, 0.215 s (`sqlc`).
- Backend `go test -race -count=1 ./internal/conventions`: exit 0, 3.007 s (`conventions-race`); this fresh run includes newly staged measurement files in Git-file-list checks.
- Backend `go test -race -tags slice6_measurement ./internal/api -run '^TestSlice6' -failfast -count=1 -parallel=1 -timeout=30m -v -args -slice6-smoke`: exit 0, 20.039 s command time and 16.135 s package time (`tagged-smoke-race`), all twelve small cases passed.

The full default backend race run recorded above and these checks cover the unchanged backend source. Subsequent frontend-only lifecycle corrections do not invalidate that backend evidence. Frontend, documentation and scanner checks remain pending on the final staged source. An attempted `git write-tree` identity probe was denied by the filesystem sandbox before creating an index lock; the read-only staged diff fingerprint above was used instead, with no repository mutation.

## Reconnect batch lifecycle minor correction

The fresh combined reviewer found one minor lifecycle issue: Promise.all could settle a failed churn batch before its sibling actor reconnects settled, allowing cleanup to outlive batch ownership. The runner now uses Promise.allSettled for each actor batch and counts batch errors only after every actor settles. Per-actor failure diagnostics, reconnect success counts, ten-actor batches, timer cadence, ten-second deadlines and final error assertions remain intact. Rejected actor count is recorded alongside the failed batch; no failure is excluded. Runtime verification of this minor correction remains pending. The minor-fix rule requires checks, not another independent review round.

## Fresh combined standard and full-security review

The independent reviewer inspected all 27 staged files (4,183 additions and 10 deletions), staged diff SHA-256 `2bd3eeaea1a13c1953951a76b45a23c1d845e7160811e18c4d44e3a5bf284edf` and manifest fingerprint `a701ebd93aa4995843426363480ae8e667ccd93add16d836582b538dccab2c27`; all 22 executable files matched the window-code-scope manifest. It read the complete source/contracts and retained runtime evidence, used read-only Git and Python hashes, and performed no edits, heavy workload or delegated review. Verdict: no new blocker or major; one minor reconnect-batch ownership finding, corrected above. Readiness remained unverified pending applicable checks and the exact-tree gate.

The reviewer closed the original major held-presence cleanup finding: release outcomes, process stops and owned database removal are independently attempted through aggregate teardown. That fix touches the fixture resource trust boundary. It closed the major browser/socket acquisition and serial-cleanup finding through immediate cleanup registration and aggregate cleanup; concurrent batch ownership received the separate minor finding above. The buffer.byteLength reporting minor was closed. The held-download security minor was closed by cancellation, barrier release, body close and server close ordering; that fix touches the fixture resource-lifecycle trust boundary. No security finding was downgraded.

Threat coverage:

- T-18 examined: counts, durations and ordinals without credentials, bodies, signed addresses or raw exception chains; query plans are restricted channel/message SELECTs and contain no authentication arguments.
- T-19 examined: no environment-file reads, subprocess passwords travel in private environment rather than arguments, and reports remain outside served output. External evidence-directory ownership/mode remains unverified.
- T-20 examined: four uploads per user, 32 concurrent uploads and 64 downloads, 124 MB retained per pair, rotating actors and nonrefundable daily accounting; daily cleanup retains its at-most-100-student/1,000-row bounds, lock-progress observation and current-day retention. The reconnect minor concerned fixture cleanup ownership.
- T-36 examined: opt-in local fresh database name/ownership/loopback checks, read-only administrative settings and ownership queries, ALTER only on the owned database, no role/server/global changes or new routes.
- T-41 examined: fixed 45-second opt-in window, unchanged default twenty-second stop and five-second kill observation, rejection of forced/nonzero exits, and explicit separation of smoke/full workloads and unsupported observers.
- T-13/T-34 examined: upload authorization, durable cleanup and real local S3 integration, with no production deletion-semantic change.
- Other security families were not affected: password/token issuance, single-purpose links, answer keys/scores, provider integration, CSP/framing, production rendering, dependencies and foreign data processors were unchanged.

The reviewer inspected the twenty-second negative-control failure, normal forty-five-second shutdown, three-engine browser success, socket/presence success and custom REST versus production-refetch distinction, incomplete frame classification, tagged Go race success and full backend log. Pending evidence includes full frontend/default E2E/scanners/gate. Full performance, unsupported heap/timer/cache/queue/commit observers and an independent post-run database/object inventory audit remain unverified. The review does not turn any unsupported observation or earlier failed sample into a passing target.

## Final frontend and reconnect-minor checks

The reconnect correction changes only actor-task settlement and records the number of rejected actors after all settle. Source inspection confirms that cleanup cannot finish waiting for a batch while its sibling actor tasks remain unsettled. No severity, timeout, admission limit or failure predicate changed. Under the minor-fix rule no additional review round ran.

The final source is based on `baabaf179973fdfab1ee5f5fb6280a553219ec6d`. `/private/tmp/ovc-measurements-readiness-frontend-scope.json` records the exact commands and staged binary diff SHA-256 `a16087739d0dfb1bc04544892cc390116a4ab0a3c1a9ea44f3b4023f7ea573b9`. Source and evidence remained unchanged during the sequence. This section and the status header were added afterward; executable files are unchanged. Individual logs and UTC start/stop/exit records use `/private/tmp/ovc-measurements-readiness-<name>.log` and `.status`.

- Root `bash tools/check-tool-binaries.sh`: exit 0, 0.152 s (`frontend-preflight`).
- Frontend `pnpm typecheck`: exit 0, 0.903 s (`typecheck`).
- Frontend `pnpm lint`: exit 0, 13.871 s (`frontend-lint`), including source-policy checks over the staged new files.
- Frontend `pnpm format:check`: exit 0, 1.696 s (`format`).
- Frontend `pnpm test`: exit 0, 5.942 s (`frontend-test`), 64 files and 684 tests passed.
- Frontend `pnpm build`: exit 0, 0.353 s (`build`).
- Frontend `pnpm check:classes`: exit 0, 0.676 s (`classes`).
- Frontend `pnpm check:bundle`: exit 0, 0.118 s (`bundle`); entry plus largest language is 175.5 KB, below the 176 KB consultation point and 180 KB hard limit. Entry, Home and Today are 164.1, 190.9 and 191.4 KB respectively; no bundle-budget change was made.
- Frontend `pnpm api:check`: exit 0, 2.310 s (`api`).
- Frontend `node scripts/measure-slice6-sockets.ts --smoke` through the existing root dev-env wrapper and caffeinate: exit 0, 2026-11-01T21:07:49.713498Z to 21:08:18.957915Z (`sockets`); 60 sends, 20 reconnects, zero errors, refusals or unexpected closes. The incomplete-frame/custom-REST distinction remains intact. This is a small functional smoke, not a full workload or a passing delivery-latency target.
- Frontend default `pnpm e2e` through the existing root dev-env wrapper and caffeinate: exit 0, 2026-11-01T21:08:47.021919Z to 21:11:16.391419Z, 149.3695 s (`e2e`); all 947 cases passed. Before execution the other loaded project chats were idle and a process-name-only snapshot showed no other owned workload. Coordination evidence is `/private/tmp/ovc-measurements-readiness-e2e-coordination.txt`.

The earlier full backend race run and staged backend checks still cover the unchanged backend files. No assertion failure was retried on an unchanged tree. All earlier failures and the intentional twenty-second negative control remain retained. Tier remains standard with the security trigger, due to local process/database instrumentation, resource lifecycles, diagnostics and the opt-in observation window. The original major security cleanup fix and minor held-download fix touch fixture resource-lifecycle trust boundaries; production authorization is unchanged.

Documentation checks and staged Gitleaks must run after staging this evidence update. Other full-run commands outside the affected change-check rows, including dependency/advisory scans and gate self-tests, remain deferred to the merge gate, not passed here. The exact-tree gate and finish remain pending. E7/E8 full series, large workloads, three cold gates, final cumulative review reconciliation and slice closure remain outstanding; no performance target or post-slice budget is accepted by these functional checks.

## Documentation check setup and formatting correction

The final preflight passed. The first documentation invocation exited 127 before assertions because this worktree lacked `tools/docs/node_modules/.bin/markdownlint`; `/private/tmp/ovc-measurements-readiness-docs.log` and `.status` retain that setup failure. `pnpm --dir tools/docs install --offline --frozen-lockfile` then exited 0 in 1.517 seconds without source or lockfile changes. The next actual documentation run exited 1 in 1.368 seconds: MD012 found two consecutive blank lines at line 65 of this record. `/private/tmp/ovc-measurements-readiness-docs-2.log` and `.status` retain the assertion failure. The correction removes exactly one blank line; no rule, check or timeout was weakened. Final documentation and staged secret checks must run on the restaged record. Neither earlier invocation is reported as passed, and no failed gate has been retried.
