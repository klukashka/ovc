# Local check budget after prebuilt scanning tools and end-to-end account seeding

Measured 2026-10-26 on `main` with changes 7b (the scanning tools and sqlc built once into `bin/`, with `tools/check-tool-binaries.sh`) and 7c (end-to-end accounts and sessions seeded through test control routes) of the [slice 4 review follow-up plan](../plans/slice-4-review-follow-ups.md). The [record after the slice 4 review](check-budget-2026-10-26-after-slice-4-review.md) measured 237.1 seconds cold before both, and the [account seeding record](check-budget-2026-10-26-account-seeding.md) 221.0 seconds with 7c alone.

## Method

One script ran every command of the [development guide](../../docs/development.md)'s full local check run in sequence and timed each with `time.monotonic()`. The Go build cache and the golangci-lint cache were fresh empty folders; the pnpm store and the frontend caches were warm, as in the earlier cold records. No other session or agent ran; `uptime` reported a load average of 5.20 just before the run and 17.69 at its end, from the run itself, so the machine was not perfectly idle at the start. Every command exited 0, and the end-to-end suites passed 660 of 660.

## Results

| Command | Time (s) |
|---|---|
| Documentation checks | 5.8 |
| Gitleaks over history and over staged changes | 0.5 |
| Four OSV-Scanner lockfile scans | 5.4 |
| `tools/check-tool-binaries.sh` | 0.5 |
| `go mod tidy -diff`, `gofmt`, `go vet` | 7.8 |
| golangci-lint, govulncheck | 7.3 |
| `go test -race -count=1 ./...` | 43.1 |
| `sqlc diff` | 1.3 |
| Frontend checks before the end-to-end suites | 20.7 |
| `pnpm e2e` | 89.2 |
| Secret guard hook and local settings wrapper checks | 4.9 |
| **Full run** | **186.7** |

- Against the 180-second threshold of the slice 4 plan: 6.7 seconds over. Against slice 5's D46 target of 149 seconds: 37.7 seconds over.
- The scanners no longer compile on a cold run: the four OSV scans and both Gitleaks scans take under 6 seconds together.
- The end-to-end suites and the Go tests are 71% of the run; the end-to-end suites spend most of their remaining time in the browsers, since account seeding removed most password hashing from setup.
