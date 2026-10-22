# Local check budget at the end of slice 2

Measured 2026-10-22 on the development machine (Apple Silicon, macOS, Docker Compose running), with Go build and test caches and the pnpm store warm, against the 10-minute budget in AGENTS.md, after the slice 2 review fixes. Each command ran once, one after another, and passed; each row is rounded to the nearest second, and the total adds the unrounded times. It repeats the [earlier measurement](check-budget-2026-10-21.md) with the per-worker end-to-end deployments and the slice 2 suites.

| Check | Seconds |
|---|---|
| `go mod tidy -diff`, `gofmt`, `go vet` | under 1 each |
| golangci-lint | 1 |
| govulncheck | 4 |
| `sqlc diff` | under 1 |
| `go test -race -count=1 ./...` (integration tests on PostgreSQL and the local storage server, process tests) | 19 |
| `pnpm typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `api:check`, `audit` | under 1, 5, 1, 3, under 1, 1, 2, 1 |
| `pnpm e2e` (241 tests in Chromium, Firefox and WebKit, including the global setup that builds the binaries and the frontend) | 41 |
| documentation checks | 7 |
| secret guard hook checks, local settings wrapper check | 4, under 1 |
| **Total** | **87** |

The end-to-end suites are now about half of the run, and the Go tests about a fifth. The budget leaves room for roughly six more slices of the same growth before the suites need consolidating or more parallel workers. A cold run, after clearing the caches or upgrading the toolchain, also builds the Go toolchain modules, the linters and the Playwright browsers; it was not measured. The integration tests' storage bucket `ovc-test` is never emptied: it held about 3 MB after the slice 2 runs.
