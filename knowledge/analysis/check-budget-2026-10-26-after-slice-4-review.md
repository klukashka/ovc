# Local check budget after the slice 4 review follow-ups

Measured on 2026-10-26 for the end-to-end consolidation change of the [slice 4 review follow-ups](../plans/slice-4-review-follow-ups.md), on its branch based on `main` at `b17ad9b`, which holds every other follow-up change. The acceptance criterion is a cold full local check run on an idle machine, the security scans included, at 180 seconds or less; slice 5's D46 target of 149 seconds ([slice 5 plan](../plans/slice-5.md)) is reported against it. The comparison point is the full local check run of the [slice 4 review measurements](slice-4-review-measurements-2026-10-26.md#full-local-check-run): 237.9 seconds cold and 177.5 seconds warm.

## Method

- **Machine:** Apple M4 Pro, 12 cores, 24 GB, macOS 26.2, Go 1.27.3, Node 26.12.1; Docker Desktop running the Compose PostgreSQL 18 and storage services. No other session ran checks.
- **Commands:** one script ran every command of the [development guide](../../docs/development.md) one after another and timed each with Python's `time.monotonic()`: the documentation checks, Gitleaks over history and over staged changes, the four OSV-Scanner scans, the build of the pinned linters, the backend checks with `go test` through `tools/dev-env.sh`, the frontend checks with `check:bundle` and `pnpm e2e` through `tools/dev-env.sh`, and the secret guard hook and local settings wrapper checks. `pnpm e2e` ran with the `dot` and `json` reporters, so per-file, per-test and per-worker times come from the same run; the reporters do not change what runs. Every command exited 0 in both runs, and the end-to-end suites passed 660 tests.
- **Cold and warm:** as in the comparison record, the cold run started Go and golangci-lint from empty caches (`GOCACHE` and `GOLANGCI_LINT_CACHE` set to new empty folders), and left the pnpm store, `node_modules` and the Vite and TypeScript caches as they were. The warm run followed once the one-minute load average was below 2.5 again.
- **Machine state:** each run started with a one-minute load average below 3, but the 15-minute load average was still raised by the profiling runs and the unit-test stress runs before them: 2.88, 24.95 and 30.49 before the cold run, 2.46, 10.68 and 21.24 before the warm run. The end-to-end suites raise the load average to about 16 to 24 by their end.

## Results

| Command | Cold (s) | Warm (s) | Review cold (s) | Review warm (s) |
|---|---|---|---|---|
| Documentation checks (install and `check-docs.sh`) | 6.0 | 6.4 | 6.1 | 6.0 |
| Gitleaks over history, over staged changes | 6.0, 0.6 | 0.7, 0.2 | 5.9, 0.5 | 0.9, 0.2 |
| OSV-Scanner: tools, backend, frontend, documentation tools | 13.0, 2.0, 1.6, 1.7 | 1.9, 1.5, 1.4, 1.6 | 13.9, 2.0, 1.7, 1.7 | 1.9, 1.4, 1.6, 1.7 |
| Build of the pinned linters | 5.2 | 0.3 | 5.3 | 0.4 |
| `go mod tidy -diff`, `gofmt`, `go vet` | 0.4, 0.0, 4.3 | 0.0, 0.0, 1.3 | 0.5, 0.0, 4.1 | 0.0, 0.0, 0.3 |
| golangci-lint, govulncheck | 4.3, 2.8 | 2.6, 2.8 | 4.3, 2.7 | 1.8, 2.5 |
| `go test -race -count=1 ./...` | 44.2 | 34.8 | 41.8 | 28.4 |
| `sqlc diff` | 13.7 | 0.4 | 14.2 | 0.4 |
| `pnpm install`, `typecheck`, `lint`, `format:check`, `test` | 0.0, 0.1, 9.1, 1.3, 4.1 | 0.0, 0.1, 10.4, 1.4, 4.5 | 0.1, 0.1, 9.4, 1.2, 4.0 | 0.1, 0.1, 9.2, 1.1, 3.9 |
| `build`, `check:classes`, `check:bundle`, `api:check`, `audit` | 0.3, 0.6, 0.1, 1.2, 0.5 | 0.3, 0.6, 0.1, 0.5, 0.5 | 0.4, 0.6, 0.1, 0.6, 0.5 | 0.3, 0.6, 0.1, 0.5, 0.5 |
| `pnpm e2e` (660 tests) | 108.9 | 115.2 | 111.5 | 108.8 |
| Secret guard hook checks, local settings wrapper check | 4.1, 0.9 | 4.2, 0.8 | 3.8, 0.6 | 4.0, 0.7 |
| **Total** | **237.1** | **194.8** | **237.9** | **177.5** |

- **Cold run without the end-to-end suites:** 128.2 seconds. A separate cold run of the same commands without `pnpm e2e`, before the change, took 139.6 seconds; its `typecheck` started from an empty TypeScript build cache (4.1 seconds) and its `build linters` took 8.0 seconds.
- **The warm run was slower than the review's warm run** in `go test` (34.8 against 28.4 seconds) and `pnpm e2e` (115.2 against 108.8 seconds), with the 15-minute load average still at 21 from earlier runs. The two runs of this record differ by 6.3 seconds in `pnpm e2e` on the same tree, which bounds what a single run can show.

## Where the end-to-end time goes

Profiled from the Playwright JSON report and the API logs of one run before the change (123.4 seconds, 660 tests):

- **Projects run one after another.** Playwright's six workers take the tests in project order, so Chromium ran from 0 to 31 seconds, Firefox from 31 to 79 and WebKit from 79 to 116; each project boundary starts six new workers, each with its own deployment.
- **Summed test time:** 598 seconds, 157 in Chromium, 248 in Firefox and 193 in WebKit. Each of the six worker slots was busy for 97 to 104 of the 116 seconds, so the suites are bound by the processor, as the [consolidated record](check-budget-2026-10-25-consolidated.md) found.
- **Password hashing dominates the API's work.** Across the run the API logs record:

| Route | Requests | Total (s) | Mean (ms) |
|---|---|---|---|
| `POST /api/v1/auth/register` | 1,246 | 224.5 | 180 |
| `POST /api/v1/auth/login` | 2,302 | 200.8 | 87 |
| `POST /api/v1/auth/verify-email` | 1,419 | 150.7 | 106 |
| `POST /api/v1/groups` | 663 | 50.2 | 76 |
| `POST /__test__/tick` | 880 | 23.0 | 26 |
| every other route except the socket upgrade | 9,692 | 68.3 | 7 |

Registration, sign-in and confirmation with a new password each run one Argon2id hash with the production parameters; together they are 576 seconds of API time, beside the 598 seconds of summed test time. Almost all of them are seeding: every seeded tutor registers and confirms with a new password, every seeded student registers, signs in and confirms, and most helpers of `frontend/e2e/tutoring.ts` sign in again for each call.

- **Extra workers:** `notifications.spec.ts` extended the test with its own worker-scoped fixture, which gives its tests a different worker hash, so Playwright started six additional workers per project for its 9 tests, 18 additional deployments per run, at the end of each project. Its three tests that share 31 seeded join requests ran in three different workers, so the 31 students were seeded three times per browser.
- **Per file:** `join-requests.spec.ts` 72.1 seconds of summed test time, `groups.spec.ts` 59.8, `smoke.spec.ts` 48.9, `students.spec.ts` 47.4, `account-edge-cases.spec.ts` 46.2, `invitations.spec.ts` 44.6 and `signup.spec.ts` 42.0; the realtime contract suite takes 2.2 seconds in all, so its per-scenario tutor seeding costs less than a second of wall time.

## What the change does

- **One shared seed of Maria with 31 join requests** as a worker-scoped fixture of the base test in `frontend/e2e/fixtures.ts`, like the other shared actors. The three notification tests that read 31 notifications and the join-request test of Show more on Today, which needs exactly one page of 30 plus one, use it. None of them decides a request or marks a notification read before the last of them reads it: the notification tests run in order in one worker (`mode: 'default'`), and Show more on Today only loads the next page. Every assertion is kept.
- **No more extra workers:** `notifications.spec.ts` uses the shared test directly, so its tests run in the ordinary workers. A run now starts 18 workers instead of 36.
- **Not changed, after profiling:** the realtime contract suite (2.2 seconds in all) and the two Show more tests of My tutors, whose seeds differ in what the extra page holds and which seed no accounts beyond three each.

Effect in the measured runs: summed test time 578 and 611 seconds against 598, and wall time 108.6 and 114.9 seconds in the reporter against 123.4 before the change and 111.5 in the review's cold run. By construction each browser seeds Maria's 31 requesters once or twice instead of four times, about 190 to 280 fewer seeded requesters per run, each of them a registration, a sign-in and a confirmation. The wall-time gain is within the run-to-run spread of about 6 seconds.

## Unit-test waits

The unit tests of the realtime client, the realtime page wiring, the live refetch and the following query client waited with a wall-clock deadline: `Date.now()` plus 2 seconds, polled every 5 ms. All the work they wait for runs on promises, `setTimeout(0)` callbacks of the query library and the injected manual schedulers, so the deadline was the only part of their own waits that depended on real time, and a process starved under load could pass it while the awaited work was still queued. Vitest's default per-test timeout of 5 seconds still bounds every test as a safety limit; the one recorded failure of the 4401 test was reported as a timeout, without saying whether the wait's deadline or the runner's timeout ended it. They now share `frontend/src/lib/eventually.ts`, which re-checks the condition after each event-loop turn and gives up after 100 turns; no wait reads the clock. Each wait needed at most one turn in a measured run. The panel tests of `NotificationsPanel.test.tsx` load the panel module before rendering, because the first test otherwise waited, with Testing Library's one-second wall-clock deadline, for the module's transform and failed under load.

| Stress run | Load average at the end | Failed runs |
|---|---|---|
| Before the change: two stress runs of the whole unit suite, 24 times each, 6 at a time, beside 12 processor-bound processes | 67 and 99 | 0 |
| With the new waits, before the panel module was loaded first: the whole unit suite 48 times, 8 at a time, beside 16 processor-bound processes | 142 | 7, all in the panel test `Retry after a failed load moves focus to the panel heading once the failure is gone` |
| After the change: the four realtime and query files 300 times, 8 at a time, beside 16 processor-bound processes | 40 | 0 |
| After the change: the whole unit suite 48 times, 8 at a time, beside 16 processor-bound processes | 138 | 0 |

The 4401 test's single timeout was not reproduced before the change: under that stress its longest run took 247 ms, and 392 ms after it. The fix removes the only wall-clock dependency of its waits; it does not rest on a reproduction.

## A flaky end-to-end test found on the way

A full run after the change failed once in `session.spec.ts`, in the test that an expired access token is refreshed once and the request is retried; alone it failed 3 times in 30 in Chromium on the unchanged test. The API logs showed two causes, both from advancing the shared clock by 910 seconds too early: the page's realtime socket sent its first-frame authentication after the advance and was closed as `invalid_token`, or the refetch the client runs after the socket's `ready` frame was still in flight and answered `401 token_expired`. In both cases the client refreshed before the sign-out, so the expected `logout 401` never came. The test now waits, before it advances the clock, until the page has received the `ready` frame, each of the Today page's three queries has been requested after it and answered, and no API request is pending. It then passed 150 of 150 repetitions across the three browsers, and the full suites passed 660 of 660 tests. The record's timed runs ran before this fix; it adds no measurable time.

## Against the targets

- **180 seconds, cold:** 237.1 seconds, 57 seconds over. **Failed.**
- **149 seconds of slice 5 D46:** failed for the cold run by 88 seconds and for the warm run by 46 seconds.
- **NFR-6.2:** the 10-minute budget is not at risk.
- **Idle machine:** both runs started with a one-minute load average below 3, but with the 15-minute load average still raised by earlier runs (30.49 before the cold run, against 1.45 before the comparison record's cold run). This can bias the totals by a few seconds, far less than the 57-second miss, but it makes the comparison of end-to-end wall times with the earlier record weak.
- **Why consolidation cannot reach 180 seconds:** the cold run without the end-to-end suites takes 128 seconds, so 180 seconds would leave the suites 52 seconds, less than half of their 109 to 115 seconds. Removing whole seeds of shared read-only actors, as this change and B0 did, saves a few seconds each; the remaining time is seeding through the real sign-up, confirmation and sign-in of mostly mutating tests, and the processor time of three browsers.

## Options for the owner

Each would be its own change with its own plan; none drops coverage.

1. **Build the Go tools into `bin/`.** Build Gitleaks, OSV-Scanner and sqlc into the ignored `bin/` with the linters, in the one `go build` the guide already runs once per tools upgrade, and run them from there. In the cold run, the tools' compilation inside `go tool` took about 31 seconds (OSV-Scanner 11, sqlc 13, Gitleaks 5, beyond their warm times); one parallel build would pay part of that once per upgrade instead of on every cold cache. This is a tooling change.
2. **Seed accounts through a test control route.** A `/__test__/` route, present only with `OVC_TEST_CONTROL=1`, would create confirmed accounts with a password hash computed once per deployment, and the helpers would reuse one access token per account until it expires. It removes most of the 576 seconds of hashing time, so it is likely to save tens of seconds of wall time. It meets the security trigger (the test control surface of T-36, authentication), needs T-36 and the guide extended and a test that the route is absent without `OVC_TEST_CONTROL=1`, and keeps the sign-up and confirmation journeys that test those flows on the real routes.
3. **Cheaper password hashing in the end-to-end deployments,** through a setting allowed only with `OVC_TEST_CONTROL=1`. Similar savings to option 2 with less harness work, but the suites would no longer run the production Argon2id parameters, and it changes authentication settings, so it needs a decision record. The setting would live in the production settings object, so T-1's defence against credential stuffing, Argon2id with the production parameters, would then also depend on the T-36 refusals that guard `OVC_TEST_CONTROL=1`, and the suites would no longer exercise the hashing limiter at realistic cost.
4. **Run independent check groups in parallel,** for example the documentation checks and security scans beside the backend checks, and measure the target on that runner. It changes the method of every earlier record.
5. **Measure the target warm,** with the cold run reported beside it, as the slice 3 threshold was first measured. The warm run of this record took 194.8 seconds, above 180 seconds as well.

Options 1 and 2 together are the most likely to reach 180 seconds cold; none of them is likely to reach 149 seconds on its own.
