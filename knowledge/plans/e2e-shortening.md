# Plan: shorten the end-to-end suites on the merge gate's critical path

Status: approved by the owner on 2026-10-28 with items A and B; item C is not done; item D is limited to the migrated template, built only if items A and B leave a gate above 165 seconds. Revision 2, against `main` at `9935cb6`, after a security-profile review of revision 1: item B keeps the harness's failure checks with a self-test, the batched seeding route needs its own plan amendment, and the template, connection and check criteria are stated as commands. The owner decided on 2026-10-28 that this plan covers more Playwright workers with smaller database pools and the removal of Firefox's per-test overhead, and that the other items below are options for the owner to choose.

## 1. Problem

The merge gate holds the end-to-end run lock for its whole run and runs `pnpm e2e` alone after lanes A, B and C, so the suites set the gate's critical path. Recent gates took 190 to 252 s, of which `pnpm e2e` took 136 to 150 s, against D46's 165 s target and 180 s threshold ([slice 5 plan](slice-5.md), [critical path record](../analysis/check-budget-2026-10-28-gate-critical-path.md), [slice 5 review](../reviews/slice-5-review.md)). The suites hold 881 tests in three engines on 6 workers (Playwright's default, half of 12 cores). Summed test time per engine was chromium 174 s for 318 tests, firefox 270 s for 281 and webkit 210 s for 282 in a passing run of 2026-10-28, and 164 / 339 / 221 s in another. Playwright dispatches the three projects from one queue, so the phases are packed: wall time is about the summed time over 6 plus about 3 s. On the 281 tests that run in every engine, Firefox's median test is 0.2 to 0.33 s slower than Chromium's and its tenth percentile 0.42 to 0.54 s against 0.27 to 0.29 s: Firefox costs 98 to 178 s of summed time beyond Chromium, a fixed per-test overhead rather than slow tests. Each worker owns a deployment against PostgreSQL's `max_connections` of 100, which bounds the worker count.

## 2. Outcome

The gate's wall time is at most 165 s in three cold runs, with every test, engine and assertion kept and no timeout raised. Items A and B are in scope; C and D are options the owner chooses; E is listed as not recommended.

**A. More workers with smaller per-deployment pools.** `frontend/e2e/deployment.ts` sets `OVC_DB_POOL_SIZE` to 3 instead of 5 for the API and the worker, and `frontend/playwright.config.ts` sets `workers` to a fixed number, so the connection count does not grow with the machine's cores. 8 and 9 workers are each measured; the lower count is chosen unless 9 gains more than the spread of the runs. A pool of 4 is the fallback if pools of 3 slow the suites down. Production defaults (API 20, worker 10) do not change.

- Connections: a deployment holds API 5 + worker 5 + 1 listener = 11 today, 66 at 6 workers; with pools of 3 it holds 7, 63 at 9 workers and 56 at 8. Transients come on top: `createdb`, `migrate` and `dropdb` of starting and stopping deployments (a deployment of the next engine can start while the previous engine's stops, briefly doubling a slot) and each `ovc-admin` call, whose pool keeps its default of 20. The computed upper bound at 9 workers, every slot doubled at once and one `ovc-admin` per slot, is 9 × (2 × 7 + 20) = 306, far above 100; the sampled peak decides, and its limit is in section 6.
- Pool starvation: the worker's lanes run up to 10 email and 4 deletion jobs at once, and `/__test__/tick` runs a worker in-process on the API's pool, so pools of 3 queue them. Starvation shows as failures, not as hidden waits: the claim and effect timeouts are 30 s and the connect timeout 5 s, and none changes. The hidden-bug risk is `session.spec.ts` "two tabs refreshing at once keep the session", whose concurrent refreshes may serialise on a pool of 3 and stop exercising the race; `TestConcurrentRefreshesWithOneCookieRotateOnceAndRevokeAsReuse` remains the coverage of the rotation race.
- Expected gain: 6 to 9 workers would cut the packed wall time by a third, about 45 s at today's figures, with perfect scaling; with the CPU contention of 9 browsers, 9 deployments and their preview servers on 12 cores, 15 to 35 s (8 workers: 10 to 25 s).

**B. Firefox's per-test overhead profiled and removed.** First a profile, without any committed change: the slowest common specs (`schedule`, `signup`, `account-edge-cases`, `students`, `smoke`) run with `--workers=1 --trace=on` in Chromium and Firefox, and the traces' fixture timings split each test's time into context and page creation, the `diagnostics` auto fixture, `signedInPage` and seeding, and socket-ready waits. Traces add their own cost, so the profile only ranks causes. Candidate fixes, each kept only when it removes measured time:

- socket-ready and presence waits (`expectRefetchedAfterTheSocketIsReady`, `waitForSockets` in `frontend/e2e/realtime.ts`) poll at `expect.poll`'s default intervals of 100, 250, 500 and 1000 ms, so a condition met at 110 ms is seen at 350 ms; explicit short intervals keep every deadline, matcher and message;
- context options that cost Firefox time at creation and that no test relies on, found by the profile;
- the `diagnostics` fixture is rewired only if the profile names it as a main cost. It carries T-37's control that the smoke fails on any CSP violation, console error, page error or unexpected failed response, so any rewiring comes with the harness self-test of section 6. Moving its init script from each page to each context likely saves nothing, since each test opens one page.
- Expected gain: half of Firefox's excess is 50 to 90 s of summed time, 6 to 15 s of wall time; fixes that also help WebKit (35 to 56 s over Chromium) add to it.

**C. Option: engine phases concurrent.** Playwright already interleaves projects without dependencies from one queue, so the only idle time is at the phase boundaries. Expected gain: at most about 3 s.

**D. Option: duplicated setup consolidated.** (a) Every deployment runs `api migrate` on a fresh database, and at least 18 deployments start per run (6 workers × 3 engines); a database migrated once in the global setup and cloned with `createdb -T` removes 17 of those migrations. Expected gain: a few seconds, measured from the migrate logs first. Constraints: its name ends in `_test` and can never match the Go fixture's cleanup pattern `ovc_template_<secs>_<rand>_test`; it is created only after `readE2ESettings`' loopback checks, migrated by `api migrate` with the deployment's test-control environment, and every connection to it is closed before it is cloned; the global teardown drops it under `assertStillHeld`, and stale templates of crashed runs are removed only by the run-lock holder. It adds no route and no setting, so T-36 needs no change. (b) The specs call `seedVerifiedUser` 150 times and `seedActiveStudent` 50 times per engine; a batched seeding route would save round trips but adds to the test control surface. It needs its own plan amendment and security-profile review before implementation: the route exists only under `settings.TestControl` with the seeder and behind the test-control guard, with a body limit and a bounded batch; the absence tests (`backend/internal/api/api_test.go` near line 171, `backend/cmd/internal/startup/process_test.go` near line 174) cover it; T-36 and the harness's route table in `docs/development.md` are updated.

**E. Not recommended: Chromium-only specs.** Running some specs in Chromium alone would save up to 480 s of summed time, but it drops engine coverage of the supported browsers, which the owner excluded and the definition of done forbids.

## 3. Measurement

A baseline of three cold gate runs at `main` first, then three after the change, all on an idle machine with no other agent running, cold as in the [critical path record](../analysis/check-budget-2026-10-28-gate-critical-path.md). A new record `knowledge/analysis/check-budget-<date>-e2e-shortening.md` states, for every run: gate wall time, each lane, the `pnpm e2e` step, summed command seconds, summed test seconds and test count per engine, Firefox's excess over Chromium on the common tests, the core count, load averages, what the cold run emptied, and the peak of client connections in `pg_stat_activity` sampled every 0.2 to 0.3 s. The sampler runs through `tools/dev-env.sh` and writes only counts and times, never a connection string. Each item's own gain comes from three `pnpm e2e` runs with and three without it; an item whose mean gain does not exceed the spread of its runs is dropped, as in the [critical path plan](merge-gate-critical-path.md).

## 4. Tier and security trigger

- **Tier: standard.** One fresh standard reviewer over the complete diff.
- **Security trigger: met.** T-36: the change alters the end-to-end deployments that run with the test control surface, and item D(b) would add a route to it. T-37: item B touches the harness's failure checks, which the smoke's CSP and console control relies on. T-41: the gate's critical path and its evidence change. T-20 examined and not affected: production pool defaults are unchanged. A security-profile reviewer reviewed revision 1 of this plan, and one reviews the diff before readiness. No threat-model entry changes unless item D(b) is approved.

## 5. Files

- `frontend/playwright.config.ts` (`workers`); `frontend/e2e/deployment.ts` (pool size; item D(a)'s template); `frontend/e2e/global-setup.ts` and `global-teardown.ts` (item D(a) only).
- `frontend/e2e/realtime.ts`, `frontend/e2e/accounts.ts`, and `frontend/e2e/fixtures.ts` with a harness self-test spec if the fixture is rewired (item B, as the profile decides).
- `docs/development.md`, the parallelism bullet of the end-to-end harness: worker count and pool size, pending owner review.
- `knowledge/analysis/check-budget-<date>-e2e-shortening.md`; this plan's outcome line.
- `knowledge/plans/merge-gate-critical-path.md` already says that its pool of 5 matched the end-to-end deployment when it was written.

## 6. Acceptance criteria

- `pnpm e2e` passes with 881 tests, 318 in chromium, 281 in firefox and 282 in webkit, plus exactly the tests of a harness self-test if item B rewires the fixture (counted per engine in the record); `pnpm exec playwright test --list` before and after differs only by those self-test lines.
- If item B changes `frontend/e2e/fixtures.ts`, the self-test proves in all three engines that the fixture still fails a test on each of: a CSP violation, a `console.error`, a page error, an unexpected 4xx or 5xx response from the deployment's origin, and an expected abort that does not happen.
- `git diff main -- frontend/ | grep -E '^[-+][^-+].*(timeout|retries|grep|testIgnore|grepInvert|workers)'` prints only the added `workers` line and, for item B, `intervals` lines that keep their poll's `timeout` value unchanged; no `timeout` is added. `readinessDeadlineMilliseconds`, `stopDeadlineMilliseconds`, `pollTimeoutMilliseconds` and the deployment fixture's `timeout: 150_000` are unchanged.
- `grep -o 'expect(' frontend/e2e/*.ts | wc -l` and `grep -o 'expect.poll(' frontend/e2e/*.ts | wc -l` print the same numbers before and after, or more by the self-test only.
- Shorter `expect.poll` intervals with the same deadline, matcher and message are not a weakened check. A changed poll's condition must be monotone: `waitForSockets(...).toBe(n)` could pass on a transient count during a reconnect, so it changes only with a condition that cannot pass transiently. A test that fails with shorter intervals is a bug fixed at its cause.
- No run shows `too many clients`; the sampled peak of a `pnpm e2e` run is no higher than the baseline's peak and at most 80. An end-to-end run beside another session's full `go test` (sampled peak 75) can exceed 100 regardless; the gate's run lock prevents that during a gate, and outside it the record states the combined peak.
- `pnpm e2e` passes three times in a row at the chosen worker count with no failed test, WebKit's page errors from cancelled fetches included.
- In three cold gate runs every run passes and its wall time is at most 165 s, and the `pnpm e2e` step's median is at most 95 s.
- `tools/docs/check-docs.sh` passes; the merge goes through the gate.

## 7. Order, risks and checks

- Order: after the gate's run-lock change, which has merged; baseline gate runs and item B's profile; item A (pool, then 8 and 9 workers); item B's fixes one at a time; if the target is still missed, the owner decides on C and D with the measurements; three cold gate runs; reviews; the gate.
- **Connection exhaustion and pool starvation:** item A's arithmetic and the peak criterion above.
- **CPU saturation and flakiness.** More workers lengthen each test; past saturation the wall time rises and timing-sensitive tests flake. A flake is a bug fixed at its cause, never retried; the three consecutive passes guard it.
- **WebKit page errors from cancelled fetches** grow more likely under load; they keep failing tests as now.
- **A weakened harness check** (item B): the self-test and the counts above.
- Not changed: NFR-6.2, D46's target and threshold, ADR-0018, the gate's command list, the retries of 0, production pool defaults.

## 8. Owner decisions (2026-10-28)

- Item C: not done, because the phases are already packed (the recommended option).
- Item D: the migrated template only, and only if items A and B miss 165 s (the recommended option); no batched seeding route.
