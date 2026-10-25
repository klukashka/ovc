# Local check budget after the WebSocket gateway

Measured 2026-10-25 on the branch of the WebSocket gateway change (S4 of the [slice 4 plan](../plans/slice-4.md)), which adds about 45 Go tests: integration tests of the gateway in `internal/api`, one process test in `cmd/internal/startup` and one unit test in `internal/realtime`. It adds no end-to-end test. Section 9 of the plan asks for a record after every change that adds tests; the [record after the worker lanes, realtime signals and email preferences](check-budget-2026-10-25-after-s2-s3-s6.md) is the comparison point, at 169.8 seconds.

## Method and its limits

- **Machine:** the same development machine, with warm Go, pnpm, TypeScript and Vite caches. The run started once the one-minute load average had dropped below 4 after an earlier check run; `uptime` reported 3.7 at the start and 20.9 at the end, from the checks themselves. No other session ran checks.
- **Commands:** one script ran, one after another, the documentation checks, Gitleaks over the history and over staged changes, the four OSV-Scanner lockfile scans, the backend checks (`go mod tidy -diff`, `gofmt`, `go vet`, golangci-lint, govulncheck, `go test -race -count=1 ./...` through `tools/dev-env.sh`, `sqlc diff`), the frontend checks including `pnpm install --frozen-lockfile` and `pnpm e2e` through `tools/dev-env.sh`, the secret guard hook check and the local settings wrapper check, each timed separately. Every command exited 0.
- **One run, warm:** the cold idle-machine run belongs to the slice review. An earlier run on the same tree, with another test process running during the end-to-end suites, took 188.6 seconds; it is not the measurement.

## Results

| Measurement | Time (s) |
|---|---|
| Full run as listed above, wall time | 179.5 |
| `go test -race -count=1 ./...`, wall time | 41.9 |
| `internal/api`, the longest package | 40.7 |
| `cmd/internal/startup` | 28.7 |
| `internal/worker` | 20.0 |
| `pnpm e2e`, wall time, 559 tests | 94.3 |
| Documentation checks | 6.5 |
| Four OSV-Scanner scans and both Gitleaks scans | 7.5 |
| Frontend checks before `pnpm e2e` | 18.1 |
| Backend checks other than `go test` | 5.3 |
| Secret guard hook and local settings wrapper checks | 4.9 |

- **Go tests:** 41.9 seconds against 38.2, as the plan estimated for S4's share (the gateway tests run in parallel, each with its own database, listener and short injected timeouts). `internal/api` still bounds the wall time; the new process test runs beside the other process tests in `cmd/internal/startup`, which grew from 23.7 to 28.7 seconds and still finishes well before `internal/api`.
- **End-to-end suites:** 94.3 seconds against 91.1, with no end-to-end test added; the earlier records already showed run-to-run differences of this size.
- **Against the thresholds:** 179.5 seconds is under the 180-second consolidation threshold but above the plan's target of at most 178 seconds at the end of the slice, with S5 and F0–F3 still to add tests. S5 adds Go tests to `internal/api`, which bounds the Go wall time, so it should consolidate harnesses as it adds them (for example, several gateway or notification assertions per harness) and measure again; a run above 180 seconds requires consolidation in that change. The 10-minute budget of NFR-6.2 is not at risk.

## Second run after the review fixes

The same script ran again on the tree with the review fixes, starting at a one-minute load average of 3.6: every command exited 0, the full run took 169.5 seconds, `internal/api` 38.8 seconds and `pnpm e2e` about 84 seconds. The two runs bracket the change's cost between them; with run-to-run differences of up to 10 seconds, mostly in the end-to-end suites, a single warm run cannot place the slice under the 178-second target with confidence, and the cold idle-machine run of the slice review remains the deciding measurement.
