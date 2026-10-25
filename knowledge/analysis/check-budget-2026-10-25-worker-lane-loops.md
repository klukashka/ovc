# Local check budget after the per-lane worker loops

Recorded 2026-10-25 for the per-lane worker loops change of the [slice 4 plan](../plans/slice-4.md) (section 9 asks for a record after every change that adds tests). The [consolidated measurement](check-budget-2026-10-25-consolidated.md) is the baseline: 161.8 seconds for the full local run after the journey-test split, 18 seconds under the 180-second threshold, of which 32.3 seconds for `go test -race -count=1 ./...`; its consolidated run measured 12.0 seconds for `internal/worker`.

## Method and its limits

- **Machine:** the same development machine as the baseline.
- **Load:** other sessions ran their own Go and end-to-end checks against the same PostgreSQL server throughout, with a load average between 11.7 and 23.6. The times below come from the change's own check runs, not from a dedicated measurement on an idle machine, so they bound the change from above and are not comparable with the baseline row by row. The measurement on an idle machine is still owed and belongs to the coordinator, who runs measurements one at a time.

## What the change adds

- **Go tests:** ten tests in `internal/worker` (the eight named in the plan, one for the continuous loop's poll path while some slots are busy, and one for a failed retry update ending the pass), the `testmail` variant of the mail adapter contract tests (three subtests), and one more scheduler pass awaited by the worker process test in `cmd/internal/startup`.
- **The lane tests run one after another**, like the existing worker tests. Run in parallel, eight of them with up to 14 concurrent effects each exhausted the PostgreSQL server's connections (`SQLSTATE 53300`) while other packages and other checkouts ran their tests, and failed together.

## Results under load

| Measurement | Time (s) |
|---|---|
| `internal/worker` alone, `-race` | 9.9 (new tests 5.2, existing tests 3.4 of summed test time) |
| `internal/worker` inside the full `go test -race -count=1 ./...` | 28.9 |
| `cmd/internal/startup` inside the full run | 33.1 |
| `internal/api` inside the full run, the longest package | 45.2 |
| Full `go test -race -count=1 ./...`, wall time | 48 |
| `pnpm e2e`, wall time, unchanged suites, two runs | 131.7 and 150.2 |

- **Go tests:** the new tests add up to about 5 seconds of sequential test time to `internal/worker` (5.2 seconds measured under load), which still finishes well before `internal/api`, the package that bounds the Go wall time; on an idle machine they are expected to add nothing measurable to the full Go run. The continuous-loop test waits for one poll of at most 1 second, and the process test for one scheduler pass of 1 second, which runs while its other process tests run.
- **End-to-end suites:** the change adds no end-to-end test. The suites took 131.7 and 150.2 seconds under load against 78.8 seconds in the baseline; the difference is the load.
- **Against the 180-second threshold:** not measurable under this load; the expected growth from this change is at most a few seconds, inside the plan's estimate for slice 4.
