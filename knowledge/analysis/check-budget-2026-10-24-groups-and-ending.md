# Local check budget at the end of slice 3, after the groups pages and ending

Measured 2026-10-24 on the development machine (Apple Silicon, 12 cores, macOS, Docker Compose running), with Go build and test caches and the pnpm store warm, against the 10-minute budget in AGENTS.md and the 180-second threshold of the [slice 3 plan](../plans/slice-3.md) (section 6), on the last change of slice 3: the groups list and group page, the End dialog for active students and "Invite again" on ended cards, "Stop studying" on My tutors, the role-specific deletion consequence and the group filter on the Students page. Each command ran once, one after another, from one script that timed it with a monotonic clock, and passed; each row is rounded to the nearest tenth of a second, and the total adds the unrounded times. The machine was not idle: its load average stayed near 10 on 12 cores without any check running (a virtual machine, Docker Desktop and other applications), so the times are likely higher than on an idle machine. It repeats the [measurement after the invitation and reclaim pages](check-budget-2026-10-24.md), taken earlier the same day; this file carries a suffix because that one already holds the date.

| Check | Seconds |
|---|---|
| `go mod tidy -diff`, `gofmt`, `go vet` | 0.1, 0.0, 0.3 |
| golangci-lint | 1.1 |
| govulncheck | 2.3 |
| `sqlc diff` | 0.4 |
| `go test -race -count=1 ./...` (integration tests on PostgreSQL and the local storage server, process tests) | 82.3 |
| `pnpm install --frozen-lockfile`, `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `api:check`, `audit` | 0.1, 0.1, 8.5, 1.0, 3.6, 0.4, 0.6, 0.7, 0.9 |
| `pnpm e2e` (389 tests in Chromium, Firefox and WebKit, including the global setup that builds the binaries and the frontend) | 74.7 |
| documentation checks | 6.7 |
| secret guard hook checks, local settings wrapper check | 4.1, 0.7 |
| **Total** | **188.8** |

A second end-to-end run right after took 74.2 seconds, so the end-to-end time is stable within a second on this load. `typecheck` and `build` were fast because their incremental caches were warm from the runs just before.

- **Against the 10-minute budget:** about a third of it, so the budget holds.
- **Against the 180-second threshold:** 9 seconds above it. The Go tests are unchanged since the last measurement (83 to 82 seconds); the end-to-end suites grew from 53 to 75 seconds with the join-request, My tutors, groups and ending suites (307 to 389 tests). The slice plan therefore requires the suites to be consolidated before slice 4 adds any, never trimmed silently.
- **Candidates for the consolidation**, by the per-test times of the second run (the summed test time across the three browsers is 371 seconds, spread over the Playwright workers):
  - seeding: every test seeds its own confirmed tutor through sign-up and confirmation, and several seed 31 accounts (the "show more" tests of join requests, sent requests, blocked students and the member choice, 2 to 3.5 seconds each per browser); a tutor shared per file where tests do not mutate it, as the plan proposes, and smaller pages for the "show more" checks would cut most of it;
  - the slowest files are `account-edge-cases.spec.ts` (46 seconds of test time), `smoke.spec.ts` (41), `join-requests.spec.ts` (41), `signup.spec.ts` (40) and `invitations.spec.ts` (38);
  - a cold run was not measured, and no measurement was repeated on an idle machine.
