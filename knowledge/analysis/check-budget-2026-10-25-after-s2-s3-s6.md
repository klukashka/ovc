# Local check budget after the worker lanes, the realtime signals and the email preferences

Measured 2026-10-25 on `main` after three slice 4 changes merged: the per-lane worker loops, the realtime signals with the PostgreSQL listener, and the email preferences with one-click unsubscribe. Section 9 of the [slice 4 plan](../plans/slice-4.md) asks for a record after every change that adds tests; the [worker lanes record](check-budget-2026-10-25-worker-lane-loops.md) and the email preferences change had only times measured under load from other sessions. This is the measurement on an otherwise idle machine that both deferred. The [consolidated measurement](check-budget-2026-10-25-consolidated.md) is the baseline: 161.8 seconds for the full local run, of which 32.3 seconds for `go test -race -count=1 ./...`.

## Method and its limits

- **Machine:** the same development machine as the baseline, with warm Go, pnpm, TypeScript and Vite caches. No other session ran checks; `uptime` reported a load average of 6.5 just before the run, from the check run that had finished a few minutes earlier, and 11.9 at its end, from the checks themselves.
- **Commands:** one script ran, one after another, the backend checks (`go mod tidy -diff`, `gofmt`, `go vet`, golangci-lint, govulncheck, `sqlc diff`, `go test -race -count=1 ./...` through `tools/dev-env.sh`), the frontend checks (`typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `api:check`, `audit`, then `pnpm e2e` through `tools/dev-env.sh`), the documentation checks, the secret guard hook checks, the four OSV-Scanner lockfile scans and Gitleaks over staged changes. Each backend and scanning command and the end-to-end run were timed separately; the frontend commands before `pnpm e2e` were timed together with the rebuild of the pinned linters.
- **Differences from the baseline:** the run did not include `pnpm install --frozen-lockfile`, Gitleaks over the history or the local settings wrapper check, each of which took a few seconds or less in the baseline. The run is warm, not cold; the cold run belongs to the slice review.

## Results

| Measurement | Time (s) |
|---|---|
| Full run as listed above, wall time | 169.8 |
| `go test -race -count=1 ./...`, wall time | 38.2 |
| `internal/api`, the longest package | 37.0 |
| `cmd/internal/startup` | 23.7 |
| `internal/worker` | 20.1 |
| `pnpm e2e`, wall time, 559 tests | 91.1 |
| Documentation checks | 6.5 |
| Four OSV-Scanner scans and staged Gitleaks | 7.1 |
| Frontend checks before `pnpm e2e`, the linter rebuild and the guard hook checks, together | about 22 |

- **Go tests:** 38.2 seconds against 32.3 in the baseline. `internal/api` still bounds the wall time; `internal/worker` grew from 12.0 to 20.1 seconds with the lane tests, which run one after another, and still finishes well before `internal/api`.
- **End-to-end suites:** 91.1 seconds against 78.8 in the baseline, with no end-to-end test added by these changes; the baseline's own second run took about 84 seconds, and the cause of the remaining few seconds is not established from one run.
- **Against the thresholds:** 169.8 seconds is under the plan's target of at most 178 seconds at the end of the slice and under the 180-second threshold, with 8 seconds left for S4, S5 and F0–F3, which add tests of their own, so the target is likely to be exceeded unless those changes consolidate tests as they add them. The 10-minute budget of NFR-6.2 is not at risk.
