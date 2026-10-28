# Check budget: profile of the end-to-end suites

Measured on 2026-10-28 and 2026-10-29 for revision 3 of the [end-to-end shortening plan](../plans/e2e-shortening.md). The record answers where the time of `pnpm e2e` goes, and whether the plan's items A (more Playwright workers with smaller database pools) and B (Firefox's per-test overhead) shorten it. These are not budget measurements: no merge gate ran, because the gate runs only as part of a merge, and the Go caches were warm. The gate figures of the same period, 187 to 252 s with the end-to-end step at 2.1 to 2.5 minutes, are in the [slice 5 gates record](check-budget-2026-10-29-slice-5-gates.md); cold gates on an idle machine are still to be measured by the coordinator.

## Method

- **Machine:** 12 cores (8 performance and 4 efficiency cores), macOS, with the PostgreSQL and S3-compatible containers of the development guide running in Docker Desktop's virtual machine.
- **Runs:** every run is `tools/dev-env.sh pnpm e2e` from the `frontend/` folder of a git worktree, started only when the end-to-end run lock was free. The runs at `ba042d8` used the default list reporter. The runs at `3150810` added a reporter kept outside the repository that writes only timestamps and durations: the start of the first test, the end of every test and each worker's `deployment` fixture. The test files and the configuration were not changed.
- **Summed test time** per engine is the sum of the durations printed by the list reporter. **Excess over Chromium** sums, over the tests that run in both engines, the difference between the two durations.
- **Connections:** client backends in `pg_stat_activity`, excluding the sampler's own connection, sampled every 0.25 s through `tools/dev-env.sh` by one `psql` session that writes only times and counts. The figures give the peak overall and the peak on the `ovc_e2e_*_test` databases.
- **CPU:** `ps` sampled every 2 s, with `%CPU` summed per process family. On macOS one fully used core counts as 100 %, so 1200 % is the machine.
- **Load:** the load averages printed by `uptime` before and after each run, with the five busiest processes from `ps` before it. A high load at a run's start is the decaying average of the previous run. `ps` showed no other heavy process between runs: Docker's virtual machine, the window server and the editor stayed under 50 % together.
- **Firefox profile:** `--workers=1` in the Chromium and Firefox projects only, once with `--trace=on` and then without traces. The profile without traces uses a step reporter kept outside the repository that records each fixture's and hook's duration. Traces add cost of their own, so only the runs without traces are quoted.

## Baseline at `3150810`

Three runs in a row with 6 workers (Playwright's default on this machine), database pools of 5 and 897 tests: 326 in Chromium, 285 in Firefox and 286 in WebKit. Every run passed, with no `too many clients`.

| Run | Wall time | Load before → after | Summed test time C / F / W | Firefox over Chromium | WebKit over Chromium | Peak connections (end-to-end databases) |
|---|---|---|---|---|---|---|
| 1 | 133 s | 2.07 → 25.06 | 163.3 / 327.5 / 215.7 s | 168.3 s | 55.9 s | 43 (43) |
| 2 | 124 s | 12.98 → 18.86 | 153.8 / 305.0 / 214.9 s | 153.8 s | 63.2 s | 42 (42) |
| 3 | 119 s | 15.12 → 22.32 | 155.9 / 277.9 / 206.5 s | 124.7 s | 52.7 s | 42 (42) |

Mean 125 s, spread 14 s.

Median test time per engine: Chromium 0.45, 0.43 and 0.45 s; Firefox 0.82, 0.75 and 0.70 s; WebKit 0.69, 0.68 and 0.66 s. Summed test time per test, over all three engines, was 0.79, 0.75 and 0.71 s.

The five specs with the largest summed test time in run 3, over all three engines, carried 46 % of its 640 s:

| Spec | Summed time | Tests |
|---|---|---|
| `schedule` | 75.5 s | 92 |
| `students` | 63.6 s | 78 |
| `groups` | 57.0 s | 72 |
| `lesson` | 53.6 s | 81 |
| `join-requests` | 46.0 s | 54 |

### Where the wall time goes

| Phase | Run 1 | Run 2 | Run 3 |
|---|---|---|---|
| From the command's start to the first test's start (global setup with its builds included) | 3.8 s | 1.5 s | 1.4 s |
| Tests, first start to last end | 128.3 s | 122.7 s | 116.8 s |
| From the last test's end to the command's exit (global teardown included) | 0.6 s | 0.5 s | 0.6 s |
| `deployment` fixtures of the 18 deployments, summed | 15.9 s | 16.8 s | 17.2 s |
| Longest `deployment` fixture | 1.2 s | 1.7 s | 1.7 s |

The 18 `api migrate` runs of run 3 took 2.72 s summed, with a median of 0.15 s and a maximum of 0.28 s, timed from the first to the last line of each migrate log. The suites spend about 98 % of their wall time running tests.

### Builds of the global setup

Timed alone in a worktree of `3150810`, three rounds, at load averages of 1.8 to 2.0. Each round:

- built the three binaries with `go build -o <folder>/ ./cmd/api ./cmd/worker ./cmd/ovc-admin` from `backend/`, first into an empty Go build cache of its own and then again into the same cache;
- removed `frontend/dist` and ran `pnpm build` twice.

| Round | `go build`, empty cache | `go build`, warm cache | `pnpm build`, first | `pnpm build`, second |
|---|---|---|---|---|
| 1 | 6.8 s | 0.2 s | 2.5 s | 0.3 s |
| 2 | 6.4 s | 0.2 s | 0.3 s | 0.3 s |
| 3 | 6.3 s | 0.2 s | 0.3 s | 0.3 s |

The first `pnpm build` of the worktree took 2.5 s. Vite reported 0.16 s for its own build. Go's build cache is the only cache that matters: the Go module cache and `node_modules` were present in every round.

### CPU during the runs

| Process family | Run 2 | Run 3 |
|---|---|---|
| All processes, mean | 861 % | 846 % |
| Firefox | 253 % (29.4 %) | 256 % (30.3 %) |
| Docker's virtual machine (PostgreSQL and the S3-compatible server) | 125 % (14.5 %) | 124 % (14.7 %) |
| Playwright's runner and test workers (Node) | 107 % (12.5 %) | 112 % (13.2 %) |
| WebKit | 93 % (10.8 %) | 98 % (11.6 %) |
| Chromium | 65 % (7.6 %) | 64 % (7.6 %) |
| `vite preview` servers | 47 % (5.4 %) | 49 % (5.8 %) |
| API processes | 31 % (3.6 %) | 31 % (3.7 %) |
| Worker processes | 0 % | 0 % |
| Everything else (window server, editor, agent sessions) | 139 % (16.1 %) | 112 % (13.2 %) |

Run 1 has no CPU samples, because the CPU sampler was added after it.

## Item A: workers and pools, measured at `ba042d8`

Two runs per configuration, 897 tests, every run passed with no `too many clients`.

| Configuration | Wall time | Load before → after | Summed test time C / F / W | Firefox over Chromium | Peak connections |
|---|---|---|---|---|---|
| 6 workers, pools of 5 (`main`) | 147 s | 6.09 → 23.68 | 163.7 / 377.3 / 236.0 s | 216.2 s | not sampled |
| 6 workers, pools of 5 (`main`) | 141 s | 14.71 → 26.67 | 162.0 / 359.9 / 245.9 s | 200.7 s | 51 (43) |
| 8 workers, pools of 3 | 144 s | 26.13 → 25.83 | 207.7 / 495.0 / 310.1 s | 291.6 s | 41 (41) |
| 8 workers, pools of 3 | 128 s | 26.18 → 32.55 | 194.0 / 418.0 / 288.5 s | 227.6 s | 41 (41) |
| 9 workers, pools of 3 | 144 s | 25.83 → 35.95 | 225.7 / 558.4 / 358.0 s | 337.3 s | 46 (46) |
| 9 workers, pools of 3 | 136 s | 32.55 → 33.93 | 217.0 / 529.2 / 338.5 s | 316.7 s | 46 (46) |

The first run of `main` has no connection figure, because its sampler did not start.

Mean wall time: 144 s with 6 workers, 136 s with 8 and 140 s with 9. The spread within each configuration is up to 16 s. Summed test time grows in step with the worker count: Chromium's summed time per worker slot was 27.2 s with 6 workers, 25.1 s with 8 and 24.6 s with 9. The machine is saturated at 6 workers, so more workers lengthen every test instead of finishing sooner. The 8-worker gain of 8 s does not exceed the spread, so the plan's rule drops item A. Pools of 3 caused no failure and lowered the connection peak.

## Item B: Firefox's overhead, profiled at `ba042d8`

Without traces, with one worker:

| Specs | Engine | Tests | Summed | Body (API calls / assertions) | Before hooks (`page` fixture) | After hooks | `expect.poll` steps |
|---|---|---|---|---|---|---|---|
| schedule, signup, account edge cases, students, smoke | Chromium | 115 | 36.3 s | 19.2 / 9.3 s | 5.8 s (3.2 s) | 0.5 s | 0.65 s |
| schedule, signup, account edge cases, students, smoke | Firefox | 114 | 99.0 s | 33.9 / 9.1 s | 15.3 s (11.2 s) | 38.5 s | 0.60 s |
| realtime, live updates, lesson, teaching settings, notifications, groups, session | Chromium | 95 | 32.9 s | 15.2 / 9.9 s | 4.8 s (2.5 s) | 0.4 s | 2.95 s |
| realtime, live updates, lesson, teaching settings, notifications, groups, session | Firefox | 95 | 73.7 s | 21.1 / 11.5 s | 12.2 s (9.0 s) | 26.3 s | 4.51 s |

- **Polls:** the waits of `expect.poll` took at most 4.5 s summed per engine, so shorter intervals could remove only a part of that, far below the spread of a run. The `diagnostics` fixture took about 2 ms per test, and context creation 5 ms in Chromium and 13 to 24 ms in Firefox.
- **Context close:** Firefox's median after-hooks time is 6 ms. Every tenth context close of a Firefox browser stalls, and the stall grows with the number of contexts the browser has opened:
  - the first profile's stalls were at tests 9, 28, 38, 61, 71, 81, 91, 101 and 111, at 0.5, 1.7, 2.3, 4.1, 4.5, 5.1, 5.7, 6.4 and 7.2 s, 37.6 s in all;
  - the second profile's stalls were at tests 10 to 90, from 0.49 s to 5.39 s, 25.8 s in all.
- **Cause of the stall:** Playwright's Firefox build (`firefox-1543`) runs `nsIMemoryReporterManager.minimizeMemoryUsage` on every tenth context close (`TargetRegistry.js`, `destroy()`). A script outside the suite reproduced it:
  - on a bare page, every tenth close took 270 to 330 ms and the other closes about 3 ms;
  - on the production build served without an API, every tenth close took about 350 ms, with no growth over 60 contexts.

  The growth seen in the suite therefore depends on what the tests leave in the browser. Its trigger was not isolated.
- **What remains:** Firefox creates a page in about 0.10 s, as WebKit does, against 0.03 s for Chromium. Its test bodies run about 0.13 s slower per test than Chromium's at one worker, and Firefox takes about 30 % of the machine's CPU during a full run. This share inflates under contention and is a cost of the engine, not of the harness.
- **The stall at the gate's worker count:** with 6 workers each Firefox worker runs about 48 tests and so has four stalls. At the one-worker growth rate they add up to about 5 s per worker, 30 s summed, or about 5 s of wall time. This is an upper estimate: the base 0.3 s of each stall is Firefox's own and stays.

## Estimated fixed cost of a test

Not measured directly. The one-worker profile measured the before hooks (context, page and fixtures) at about 0.05 s per test in Chromium and 0.13 s in Firefox. Chromium's after hooks took about 0.005 s per test; Firefox's took 0.28 to 0.34 s per test, almost all of it the every-tenth-close stall, which is counted under the stall item rather than here. A full run's tests take about 1.1 to 1.6 times as long as at one worker (Chromium 0.47 to 0.50 s against 0.315 s per test, Firefox 0.98 to 1.15 s against 0.87 s). The measured parts therefore scale to about 0.08 to 0.21 s per test and engine. The rest of the 0.2 to 0.5 s estimate, a test's sign-in and its first navigation, was not timed and is an assumption. The figure is an estimate, to be replaced by a measurement before it decides anything.

## Chromium-only saving

The proposal's alternative of running some specs in Chromium only is bounded by the summed time of the two other engines: in run 3 of the baseline, Firefox's 277.9 s and WebKit's 206.5 s, about 484 s, or about 480 s if every browser spec ran in Chromium alone.

## Expected gains of the remaining items

| Item | Measured basis | Expected wall-time gain on `pnpm e2e` |
|---|---|---|
| D(a) migrated template | 18 migrations, 2.72 s summed, spread over 6 workers | about 0.5 s, less the cost of each clone |
| E' builds off the critical path | global setup with its builds at most 1.5 s with warm caches; `go build` 6.3 to 6.8 s from an empty cache, 0.2 s warm | at most 1.5 s warm; up to about 6.5 s in a gate whose Go cache holds no non-race build of the three commands |
| Firefox context-close stall | four growing stalls per Firefox worker at 6 workers | up to about 4 to 5 s, only if the growth can be removed |

Every one of these gains is below the 14 s spread of the baseline runs, and their sum stays below 12 s, against the 22 to 87 s by which recent gates exceeded D46's 165 s target.
