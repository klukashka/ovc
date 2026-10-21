# Local check budget, measured with the sign-in and sessions change

Measured 2026-10-21 on the development machine (Apple Silicon, macOS, Docker Compose running), with Go build and test caches and the pnpm store warm, against the 10-minute budget in AGENTS.md. Every command passed.

| Check | Seconds |
|---|---|
| `go mod tidy -diff`, `gofmt`, `go vet` | under 1 each |
| golangci-lint | 2 |
| govulncheck | 2 |
| `go test -race -count=1 ./...` (integration tests on PostgreSQL, process tests) | 8 |
| `sqlc diff` | under 1 |
| `pnpm typecheck`, `lint`, `format:check`, `test`, `build`, `api:check`, `audit` | 1, 3, under 1, 2, under 1, 1, 1 |
| `pnpm e2e` (Chromium, Firefox, WebKit) | 11 |
| documentation checks | 6 |
| secret guard hook checks, local settings wrapper check | 3, under 1 |
| **Total** | **40** |

A cold run, after clearing the caches or upgrading the toolchain, also builds the Go toolchain modules, the linters and the Playwright browsers; it was not measured here and is expected to stay within the budget. The measurement is repeated with the end-to-end harness change and at the end of slice 2.
