# Check budget: the merge gate's critical path shortened

Measured on 2026-10-28 on the branch `chore/merge-gate-critical-path` at `ea316f5` (the change merged with `main` at `c1eba50`, which holds slice 5's S2, S3 and S6), through that commit's own `tools/merge-gate.sh gate --ff-only` on temporary branches of the main checkout, three times in a row, with no other agent running.

- **Machine:** 12 cores, macOS, the PostgreSQL and S3-compatible containers of the development guide running. Load averages of 9 before the first run, from work of another project on the same machine that had just finished.
- **Cold:** each run had empty Go build and golangci-lint caches; the pnpm store, `node_modules` and the Playwright browsers were kept, as in earlier records.

## Results

| | Run 1 | Run 2 | Run 3 |
|---|---|---|---|
| Gate wall time | 171.2 s | 157.3 s | 181.1 s |
| Lane A (`go test`) | 67.7 s | 63.4 s | 71.2 s |
| Lane B | 61.8 s | 50.0 s | 55.5 s |
| Lane C | 48.5 s | 44.7 s | 49.9 s |
| `pnpm e2e`, alone | 102.4 s | 92.8 s | 108.8 s |
| Summed command seconds | 280.3 s | 251.4 s | 285.3 s |
| Result | passed | passed | passed |

Lanes B and C finish before lane A in every run, as the plan requires. The gate's wall time misses D46's 165 s in two runs and the 180 s threshold in one, and does not beat the [lanes record](check-budget-2026-10-27-merge-gate-lanes.md)'s 155.6 s; since that record, S2, S3 and S6 added backend and end-to-end tests: `main` at `c1eba50` took 186.4 s and 210.4 s in the gates of S6 and S3 with warm Go caches, and its cold gate now fails `go test` with `too many clients` before the end-to-end suites, so no cold baseline of `main` exists. `pnpm e2e`, alone after the lanes, is now the critical path at 93 to 109 s.

## Items of the plan

- **Parallel `startup` tests:** the package took 19.7 s on `main` and 8.5 to 11 s fully parallel; bounded to three process-test slots to stay within PostgreSQL's 100 connections, it takes about 12 s alone.
- **One `go build`** in the tests and the end-to-end setup: no separate figure.
- **Traces without screenshots:** measured on 2026-10-27 with three `pnpm e2e` runs each, 104.3 / 101.6 / 102.7 s without screenshots against 117.9 / 105.3 / 105.2 s with them; the mean gain of about 5 s is below the 13 s spread, so by the plan's rule the item is dropped.
- **Lanes balanced:** as above.
- **Connections:** child pools of 5, three process-test slots, test-database pools of 4 with one named larger pool for lock contention, and idle administrative connections released after a second lowered the sampled peak of a full `go test -race ./...` from 98 to 75 of 100; the owner accepted 75 on 2026-10-27.

## Decision

The owner decided on 2026-10-28 to merge this change, which also brings `main` back to a passing gate, and to shorten the end-to-end suites next under their own plan, keeping D46's 165 s target and the 180 s threshold. The tools module's `golang.org/x/net` upgrade to 0.62.0 that first came with this change was reverted, because 0.62.0 was younger than the 3-day release age of [ADR-0017](../../docs/decisions/0017-dependency-release-age.md); by the owner's decision of 2026-10-28, GO-2026-6860 and GO-2026-6866 are ignored in `tools/osv-scanner.toml` until 2026-10-31, when 0.62.0 is three days old ([triage](../research/tools-x-net-advisories-2026-10-28.md)).
