# Local check budget in slice 3, after the invitation and reclaim pages

Measured 2026-10-24 on the development machine (Apple Silicon, macOS, Docker Compose running), with Go build and test caches and the pnpm store warm, against the 10-minute budget in AGENTS.md, on the change that adds the invitation page, the reclaim page, the unconfirmed-student access rule and the provisional card actions, including the fixes from its reviews ([slice 3 plan](../plans/slice-3.md), section 6). Each command ran once, one after another, and passed; each row is rounded to the nearest second, and the total adds the unrounded times. It repeats the [measurement at the end of slice 2](check-budget-2026-10-22.md).

| Check | Seconds |
|---|---|
| `go mod tidy -diff`, `gofmt`, `go vet` | under 1 each |
| golangci-lint | 1 |
| govulncheck | 2 |
| `sqlc diff` | under 1 |
| `go test -race -count=1 ./...` (integration tests on PostgreSQL and the local storage server, process tests) | 83 |
| `pnpm install --frozen-lockfile`, `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `api:check`, `audit` | under 1, 1, 7, 1, 3, under 1, 1, 1, 1 |
| `pnpm e2e` (307 tests in Chromium, Firefox and WebKit, including the global setup that builds the binaries and the frontend) | 53 |
| documentation checks | 7 |
| secret guard hook checks, local settings wrapper check | 4, 1 |
| **Total** | **166** |

The Go tests grew from 19 to 83 seconds with the slice 3 integration and race tests, and are now half of the run; the end-to-end suites grew from 41 to 53 seconds. The total stays below the 180 seconds at which the slice plan consolidates suites before adding more, with about 14 seconds to spare for the remaining slice 3 changes, so the next change that adds end-to-end suites should measure again. A cold run was not measured.
