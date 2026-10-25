# Local check budget before slice 4, after consolidating the suites

Measured 2026-10-25 on the development machine against the 10-minute budget in AGENTS.md (NFR-6.2) and the 180-second consolidation threshold of the [slice 3 plan](../plans/slice-3.md) (section 6), before and after the change that consolidates the Go and end-to-end suites and splits the journey-style end-to-end tests outside the pilot scenarios into tests with three Arrange–Act–Assert phases. The [measurement after the groups pages and ending](check-budget-2026-10-24-groups-and-ending.md) had found 188.8 seconds without the security scans.

## Method

- **Machine:** Apple M4 Pro, 12 cores, 24 GB, macOS 26.2; Docker Desktop with 12 CPUs and 8 GB for its virtual machine, running the Compose PostgreSQL and storage services. Go build and test caches, the pnpm store and the incremental TypeScript and Vite caches were warm.
- **Commands:** the full local check run as the [development guide](../../docs/development.md) lists it, one command after another, in this order: the documentation checks; Gitleaks over history and over staged changes; the four OSV-Scanner lockfile scans; the backend checks (`go mod tidy -diff`, `gofmt`, `go vet`, golangci-lint, govulncheck, `go test -race -count=1 ./...` through `tools/dev-env.sh`, `sqlc diff`); the frontend checks (`pnpm install --frozen-lockfile`, `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `api:check`, `audit --audit-level high`, then `pnpm e2e` through `tools/dev-env.sh`); the secret guard hook checks and the local settings wrapper check.
- **Timing:** one script ran each command as a child process and timed it with Python's `time.monotonic()`, recorded each exit code (all 0 in both runs) and `uptime` before and after the run. Go tests ran with `-json` and the end-to-end suites with the `list` and `json` reporters, so per-package, per-test, per-file and per-browser times come from the same runs; neither flag changes what runs.
- **Load:** the machine was not idle. Processes of another project and a browser kept running throughout; the load average was 1.9 before the baseline and 3.0 before the final run, and reached 13.6 and 15.2 by their ends, mostly from the checks themselves (the end-to-end suites saturate the machine). Each command ran once per measurement, so the rows are indicative to within a few seconds; a second end-to-end run on the final code took 1.4 minutes.

## Results

| Check | Baseline (s) | Final (s) |
|---|---|---|
| documentation checks | 5.7 | 6.7 |
| Gitleaks over history, over staged changes | 0.9, 0.2 | 1.0, 0.2 |
| OSV-Scanner: tools, backend, frontend, documentation tools | 2.9, 1.5, 2.0, 2.4 | 2.3, 1.5, 1.9, 1.8 |
| `go mod tidy -diff`, `gofmt`, `go vet` | 0.1, 0.2, 0.5 | 0.1, 0.0, 0.3 |
| golangci-lint, govulncheck, `sqlc diff` | 3.3, 3.5, 1.0 | 1.0, 3.3, 0.3 |
| `go test -race -count=1 ./...` | 86.7 | 31.9 |
| `pnpm install --frozen-lockfile`, `typecheck`, `lint`, `format:check`, `test` | 0.1, 0.1, 8.4, 1.0, 4.3 | 0.1, 0.2, 8.6, 1.0, 4.0 |
| `build`, `check:classes`, `api:check`, `audit` | 0.4, 0.6, 0.6, 1.4 | 0.4, 0.6, 0.6, 1.3 |
| `pnpm e2e` | 76.3 | 78.8 |
| secret guard hook checks, local settings wrapper check | 3.8, 0.7 | 3.9, 0.9 |
| **Total** | **208.4** | **152.5** |

The total adds the unrounded times. The baseline is 20 seconds above the earlier 188.8 seconds mostly because it includes the security scans for the first time (about 11 seconds) and the Go tests ran 4 seconds longer.

- **Against the 180-second threshold:** 28 seconds above it before, 27 seconds below it after.
- **Against the 10-minute budget:** about a quarter of it.

### Go tests

| Package | Baseline (s) | Final (s) |
|---|---|---|
| `internal/api` | 85.8 | 31.1 |
| `cmd/internal/startup` | 12.8 | 20.9 |
| `internal/worker` | 6.6 | 12.0 |
| `internal/repositories` | 9.1 | 7.1 |
| every other package | under 6.1 | under 9.0 |

Before the change no Go test ran in parallel within its package, so `internal/api`, whose 302 tests summed to 82.8 seconds, set the length of the whole run while the other packages finished early. After it, its tests and the subtests that build their own harness run in parallel, and the package is limited by the shared PostgreSQL server rather than by any one test: its longest test takes 6.3 seconds. The other packages take longer than before because they now run while `internal/api` loads the machine.

### End-to-end suites

| | Baseline | Final |
|---|---|---|
| Tests (Chromium, Firefox, WebKit) | 449 | 505 |
| Summed test time (s) | 379.5 | 393.8 |
| Chromium, Firefox, WebKit (s of test time) | 98.0, 153.8, 127.8 | 101.7, 163.0, 129.2 |
| Wall time of the suite (s) | 75.9 | 78.4 |

| File | Baseline (s of test time, tests) | Final (s of test time, tests) |
|---|---|---|
| `groups.spec.ts` | 61.7, 39 | 51.8, 51 |
| `account-edge-cases.spec.ts` | 40.5, 51 | 41.3, 57 |
| `signup.spec.ts` | 37.9, 51 | 40.4, 51 |
| `join-requests.spec.ts` | 37.8, 24 | 53.0, 48 |
| `invitations.spec.ts` | 33.6, 33 | 32.6, 36 |
| `smoke.spec.ts` | 32.0, 66 | 28.0, 63 |
| `accessibility.spec.ts` | 31.0, 39 | 30.2, 42 |
| `students.spec.ts` | 26.1, 33 | 28.4, 33 |
| `ending.spec.ts` | 21.5, 24 | 25.0, 30 |
| `session.spec.ts` | 21.3, 39 | 23.3, 39 |
| `teaching-settings.spec.ts` | 17.6, 18 | 21.7, 21 |
| `reclaim.spec.ts` | 14.4, 12 | 13.4, 12 |
| `session-transport.spec.ts`, `session-cookie.spec.ts`, `api-contracts.spec.ts` | 4.2, 20 | 4.7, 22 |

The suite is CPU-bound: the six Playwright workers stayed busy for about 63 of the 72 seconds of the baseline, and a trial with eight workers took 79 seconds instead of 76. The browsers, the Playwright and preview Node processes, the API processes (password hashing) and PostgreSQL share the 12 cores. Net, the summed test time grew by 14 seconds with 56 more tests: the split tests add their own seeding and page loads, and the shared seed and the Chromium-only API checks below take part of that back.

## Consolidations

- **Parallel Go integration tests.** Every top-level test of `internal/api` and `internal/repositories`, and every subtest of `internal/api` that builds its own harness, calls `t.Parallel()`. Each still creates its own database, harness and worker, so no assertion and no fixture changed; `go test -race -count=5 ./...` passed. Two shared resources had to be made safe for it:
  - the harness of `internal/api` seeds its deterministic random source from the test's name instead of from zero, because every test used to derive the same storage keys and parallel tests overwrote each other's objects in the shared `ovc-test` bucket;
  - the test database fixture (`internal/testdb`) creates and drops databases through one administrative connection pool per test process instead of two new connections per database, and connects to the loopback test server without first offering TLS (`sslmode=disable` when the URL names no `sslmode`; the server refuses TLS, so each connection used to open two TCP connections). Without this the parallel run failed about one time in two with connection timeouts, because Docker Desktop's port forwarder dropped connection attempts under the churn. The fixture still refuses non-loopback hosts before connecting.
- **One seeded tutor with 31 active students per worker** in `groups.spec.ts`, shared by the three tests that need more than one page of students (show more in the member choice, show more that adds no visible student, adding to a group that filled up meanwhile). Each test creates its own group and none changes the students or their relationships, so the shared tutor is not mutated in a way another test can see; the three tests run in order in one worker (`mode: 'default'`) so that the seed is made once per browser, and every assertion is kept.
- **API-only checks in Chromium only**, as the slice plan prescribes: the preview server's cross-origin refusal (moved from `smoke.spec.ts`) and the FR-1.3 role-mismatch refusals of the API (moved from the role-mismatch test of `account-edge-cases.spec.ts`) now live in `api-contracts.spec.ts`, which, like `session-transport.spec.ts`, uses no browser and runs in the Chromium project only. Both keep every assertion.

Rejected after measuring: eight Playwright workers (no gain, see above), and `synchronous_commit = off` on the test databases (no gain for the Go tests).

## Journey tests split

Each test below now has three Arrange–Act–Assert phases; every assertion of the original is kept, either in the assertion phase of one of the new tests or as a readiness check in the arrangement of a test that needs the earlier state.

- `join-requests.spec.ts` › my tutors › past tutors and code entry became seven tests: past tutors list an ended relationship and a closed account; a code that is no longer valid is refused with an alert that takes focus; a valid code sends a request, empties the field and lists it as sent; a request the tutor declined with a block shows as not accepted; the code of a tutor who blocked the student lists a new sent request that the tutor never sees; and, from the tutor's side, decline and block on Students moves focus to the notice and empties the list, and a new request of a blocked student does not appear on Students.
- `join-requests.spec.ts` › join requests › show more, a plain decline and a decision on a request that changed meanwhile became three tests, one per behavior, each with its own busy-control check.
- `account-edge-cases.spec.ts` › FR-1.3 role mismatch became the API refusals in `api-contracts.spec.ts` and three page tests: a tutor on My tutors, a student on Today, a student on Students.
- `accessibility.spec.ts` › a busy save keeps focus on its button became one test for the preferences save and one for the password change.
- `groups.spec.ts` › groups › archive and unarchive, removal became five tests: removal, archiving, unarchiving, the Add students dialog of an unarchived group, and adding a student to an unarchived group.
- `ending.spec.ts` › my tutors › stop studying became three tests: the dialog opens on Cancel and names leaving the groups, Cancel returns focus to the menu button, and stopping ends the relationship, leaves the groups and emails the tutor once.
- `invitations.spec.ts` › students › two stale refusals in a row became the first refusal (withdrawing an invitation accepted meanwhile) and the second refusal while the changed-card notice shows.
- `teaching-settings.spec.ts` › each unblock of a student unblocked meanwhile is refused became the first refused unblock and a second refused unblock while the alert shows.

The pilot step tests keep their layout. A scan of the specs for an action after an assertion past the first act found 24 other non-pilot tests that still interleave act and assert steps; they are listed in the [slice 3 known limitations](../reviews/slice-3-known-limitations.md).
