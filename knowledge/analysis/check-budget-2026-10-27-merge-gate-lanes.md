# Check budget: the merge gate's concurrent lanes

Measured on 2026-10-27 on the branch `chore/merge-gate-lanes` at `fc0d293` (the lanes change merged with `main` at `e9e563f`), through that commit's own `tools/merge-gate.sh gate --ff-only` on a temporary branch of the main checkout, three times in a row, alone on the machine (no other agent or measurement running; load averages of 2.5 to 2.9 before the first run).

- **Machine:** 12 cores, macOS, the PostgreSQL and S3-compatible containers of the development guide already running.
- **Cold:** each run had empty Go build and golangci-lint caches (`GOCACHE` and `GOLANGCI_LINT_CACHE` pointed at new empty folders); the pnpm store, `node_modules` and the Playwright browsers were kept, as in earlier records.
- **Figures:** the gate's own report: each command's seconds, each lane's wall time, the summed command seconds and the gate's wall time, the budget figure of D46 since its amendment of 2026-10-27.

## Results

| | Run 1 | Run 2 | Run 3 |
|---|---|---|---|
| Gate wall time | 159.2 s | 155.6 s | 161.3 s |
| Lanes A, B and C, wall time with the wait for their processes | 64.0 s | 61.4 s | 64.3 s |
| Lane A (`go test`) | 61.3 s | 58.9 s | 60.6 s |
| Lane B | 38.3 s | 35.9 s | 38.5 s |
| Lane C | 64.0 s | 61.3 s | 64.3 s |
| `pnpm e2e`, alone | 94.1 s | 93.2 s | 95.9 s |
| Summed command seconds | 258.3 s | 249.5 s | 259.6 s |
| Result | passed | passed | failed: one end-to-end test |

Against the sequential baseline of 178.7 to 186.7 s summed ([prebuilt tools record](check-budget-2026-10-26-prebuilt-tools-and-seeding.md)), the gate's wall time is 25 to 31 s shorter; it misses D46's 149 s by 6.6 to 12.3 s. Running side by side slows the commands: `go test` takes about 60 s instead of 42 s, frontend lint about 15 s instead of 10 s, `go vet` 9 to 12 s instead of 7 s, so the summed seconds grow by about 70 s. The critical path is lane C or lane A, then `pnpm e2e`, whose 93 to 96 s is above its earlier 86 to 89 s because the machine is still busy when it starts. `tools/check-merge-gate.sh` alone takes 23 to 24 s of lane C.

Run 3 failed `realtime › signing out everywhere in another browser signs this tab out with a message` in Firefox: data requests issued after the revocation answered `401` before the session ended. The same test fails about one run in three when run alone, so it is a flaky test independent of the lanes; it is fixed at its cause in its own change.

## Per command (seconds)

| Lane | Command | Run 1 | Run 2 | Run 3 |
|---|---|---|---|---|
| pre | `tools/check-tool-binaries.sh` | 0.2 | 0.2 | 0.2 |
| B | `pnpm --dir tools/docs install --frozen-lockfile` | 0.1 | 0.1 | 0.1 |
| B | `tools/docs/check-docs.sh` | 11.1 | 10.8 | 11.3 |
| B | `bin/gitleaks git --redact --no-banner --config .gitleaks.toml .` | 0.7 | 0.8 | 0.7 |
| B | `bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .` | 0.1 | 0.1 | 0.1 |
| B | `bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod` | 1.7 | 1.7 | 1.5 |
| B | `bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod` | 0.9 | 0.9 | 0.9 |
| B | `bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml` | 1.2 | 1.2 | 1.1 |
| B | `bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml` | 1.3 | 1.5 | 1.5 |
| B | `go mod tidy -diff` | 0.9 | 1.1 | 0.6 |
| B | `unformatted="$(gofmt -l .)" && test -z "$unformatted"` | 0.5 | 0.4 | 0.1 |
| B | `go vet ./...` | 11.0 | 9.0 | 12.4 |
| B | `../bin/golangci-lint run ./...` | 5.3 | 4.7 | 4.5 |
| B | `../bin/govulncheck ./...` | 3.5 | 3.4 | 3.6 |
| A | `../tools/dev-env.sh go test -race -count=1 ./...` | 61.3 | 58.9 | 60.6 |
| B | `../bin/sqlc -f sqlc.yaml diff` | 0.2 | 0.2 | 0.2 |
| C | `pnpm install --frozen-lockfile` | 0.1 | 0.1 | 0.1 |
| C | `pnpm typecheck` | 0.1 | 0.1 | 0.1 |
| C | `pnpm lint` | 15.2 | 14.9 | 15.7 |
| C | `pnpm format:check` | 1.8 | 1.9 | 1.8 |
| C | `pnpm test` | 12.9 | 11.8 | 13.1 |
| C | `pnpm build` | 0.6 | 0.9 | 0.8 |
| C | `pnpm check:classes` | 1.3 | 0.9 | 1.0 |
| C | `pnpm check:bundle` | 0.3 | 0.2 | 0.2 |
| C | `pnpm api:check` | 0.8 | 0.7 | 0.8 |
| C | `pnpm audit --audit-level high` | 0.8 | 0.9 | 0.9 |
| e2e | `../tools/dev-env.sh pnpm e2e` | 94.1 | 93.2 | 95.9 |
| C | `python3 .claude/hooks/check_guard_secrets.py` | 5.6 | 5.3 | 5.6 |
| C | `tools/check-dev-env.sh` | 0.7 | 0.5 | 0.7 |
| C | `tools/check-merge-gate.sh` | 24.0 | 23.1 | 23.5 |

## Decision

The owner decided on 2026-10-27 to accept a gate wall time of about 160 s, to set D46's target to 165 s measured as the gate's wall time with the 180 s threshold kept, to start slice 5's S2, and to shorten the critical path in a separate change beside S2 (more parallel `startup` tests, one `go build` for the three end-to-end binaries, traces without screenshots, and lanes balanced so that no lane outlasts `go test`).
