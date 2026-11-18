# Lane A of the merge gate: a per-test profile, and parallel parent tests measured as slower

Recorded on 2026-11-18 for the gate consolidation that followed the sub-slice 10.1 slice review.

## Why this was measured

- **The consolidation threshold was crossed.** At that review the last three passing merge gates on mains power took 597.1, 607.3 and 645.1 s, taken from their merge reports, because `merge-gate.log` recorded no wall time before 2026-11-17. They supersede the earlier median of 571.0 s in the known-limitations file. Their median, 607.3 s, is above ADR-0018's 540 s consolidation threshold, and two of the three were above NFR-6.2's 600 s limit; ADR-0018's owner question covered both.
- **Lane A sets most of the gate.** A gate's length is lane A plus the end-to-end run. Lane A is set by `go test -race` of `internal/api`, which took 146.5 to 200.2 s across the 10.1 gates.
- **The approved task plan:** profile the package once, try only the levers the profile supports, and merge a lever only if three alternating pairs show at least 15 % less wall time on `./...` and on `internal/api`.

The [lane A checkpoint record](gate-speed-lane-a-checkpoints-2026-11-13.md) and the two 2026-11-12 records had already ruled out a higher or lower `-parallel`, more packages, deferred drops, file-copy clones, durability off and database reuse. None of these was tried again.

## Method

- **Machine:** 12 logical cores, macOS, Docker Desktop, PostgreSQL 18.6 with `main`'s settings. Every run was on AC power under `caffeinate -dis`.
- **Load condition:** each run started only once both the one-minute and the five-minute load average were below 6.
- **Other activity:** 5 containers were running, 3 of them unrelated to the project. The coordinating session held back every other merge gate, end-to-end run and backend run during each window.
- **Command:** `go test -race -count=1 -json ./...` from a detached worktree's `backend/`, through the main checkout's `tools/dev-env.sh`, which loads the owner's settings and runs the command in the current folder.
- **Per-test analysis:** taken from the `-json` events.
  - A leaf test is a test with no subtests. Its interval is its end time minus its `Elapsed`.
  - Concurrency is the summed leaf time per 10-second bucket.
- **Sockets:** `SYN_SENT` was sampled every second with `netstat -an -p tcp`.

## Profile of `main` at `65a9289`

Run on 2026-11-18 from 22:18:37 MSK at load 5.02 / 5.94; rc 0, wall 173 s, `SYN_SENT` peak 17.

| Package | Time |
|---|---|
| `internal/api` | 155.0 s (package elapsed) |
| `internal/repositories` | 52.2 s |
| `internal/adapters/meeting` | 42.5 s |
| `cmd/internal/startup` | 42.5 s |
| `internal/services/files` | 37.5 s |
| `internal/worker` | 15.7 s |

`internal/worker` took 57.9 s in one 10.1 gate. That time did not recur here.

**`internal/api` has 904 top-level tests, 1,422 leaf tests and 1,687.5 s of summed leaf time.**

- **Leaf concurrency per 10 s:** 6.9, 6.6, 8.4, then 11.5 to 12.0 to the end, and 4.4 in the last bucket. The package ran at its `-parallel` cap of 12 for about 85 % of the 153.8 s from its first test start to its last test end.
- **A serial phase of 24.7 s.** The first 24.7 s were taken by 19 top-level tests that do not call `t.Parallel`. They run one after another before the parallel tests resume. Their own subtests run in parallel, so concurrency averaged about 7. These tests are:
  - the eight meeting lifecycle tests;
  - the presentation URL test;
  - two erasure deadline tests;
  - two daily-upload-accounting tests;
  - six digest and student-deletion tests.

  None does any setup before its subtests. Each subtest calls `t.Parallel` and builds its own harness and database.
- **Fixed cost dominates leaf durations.** 1,022 of the 1,422 leaves took 0.5 to 1.5 s. That is the fixed cost of a test: a clone from the template, a harness, and a drop that waits for a checkpoint.
- **The long tests do real work.** They are loops of sixty requests against hourly limits, about 5 s each. `TestTemplateAutosaveBeyondHourlyWriteLimit` took 14.7 s and `TestDailyUploadCapSurvivesRealMessageAndObjectCleanup` 12.3 s. No test waits out a fixed real-time interval.
- **A fresh clone is 11.9 MB.** That covers 773 relations: catalogs of 11.6 MB counted with the shared catalogs, and a user schema of 2.2 MB. The test role is a superuser.
  - An empty PostgreSQL 18 database is about 7.5 MB, so vacuuming the template's catalogs could save at most about 2 MB per clone.
  - The checkpoint record found most checkpoint time in syncs, which follow the number of files, not their bytes.
  - Shrinking the template was therefore not prototyped.
- **How the size was read.** A throwaway test in `internal/testdb`, never committed, read the sizes:
  - `pg_database_size(current_database())`;
  - the count of `pg_class`;
  - the sum of `pg_total_relation_size` over the catalog and over the other schemas;
  - `rolsuper` of `current_user`.

  It printed only integers and error kinds.

## Candidate: run the 19 serial parent tests in parallel

**The change:** `t.Parallel()` as the first statement of each of the 19 parents. Nothing else changed. The expectation was about 10 s less, the idle share of the serial phase: (12 − 7)/12 × 25 s.

**Repeat run.** `go test -race -count=10` of the 19 tests on the candidate tree, started 22:43:58 MSK at load 2.90 / 5.99: rc 0, 92 s, no failure.

**Paired runs.** The commits from `65a9289` to `528db96` change only `docs/` and `knowledge/`, so lane A is the same code as in the profile. `main` at `528db96` (tree `cf3bee4`) against the same tree with only the 19 insertions (tree `4257a52`), alternating which tree ran first:

| Run | Start (MSK) | Load at start | `./...` wall | `internal/api` | Summed leaf time | `SYN_SENT` peak |
|---|---|---|---|---|---|---|
| Pair 1, `main` | 22:57:30 | 3.30 / 5.65 | 135 s | 133.9 s | 1,396.2 s | 13 |
| Pair 1, candidate | 23:00:41 | 4.12 / 5.98 | 147 s | 145.8 s | 1,675.5 s | 1 |
| Pair 2, candidate | 23:03:58 | 3.65 / 5.96 | 145 s | 143.4 s | 1,651.6 s | 2 |
| Pair 2, `main` | 23:07:13 | 3.49 / 5.96 | 140 s | 138.4 s | 1,450.4 s | 1 |
| Pair 3, `main` | 23:11:34 | 3.07 / 5.94 | 144 s | 142.8 s | 1,509.2 s | 1 |
| Pair 3, candidate | 23:14:28 | 4.44 / 5.97 | 148 s | 147.3 s | 1,703.5 s | 1 |
| **Mean, `main`** | | | **139.7 s** | **138.4 s** | | |
| **Mean, candidate** | | | **146.7 s** | **145.5 s** | | |

- Every run passed.
- **The candidate filled all 12 slots from the first seconds.** Concurrency was 7.8 to 9.6 in the first bucket and about 12 from the second. On `main` the first three buckets were 3.7 to 8.7.
- **But every test took longer.** Summed leaf time rose by 13 to 20 %, and the package's wall time rose by about 5 %.
- **The cause is the one the checkpoint record found:** more test databases alive at once make each checkpoint sync more files. The serial phase was not idle capacity. It was relief for the checkpointer.
- **Verdict: no-go.** The candidate was slower in every pair.

**Attempts left out of the table.** Neither measured the candidate against `main` validly. Both are kept here.

- **Pair at 22:45:56 and 22:50:13 MSK.** It ended with rc 1 on both trees. `TestProjectLintRulesFire` in `internal/conventions` found no `bin/golangci-lint` in the detached worktrees. `internal/api` passed in both: 153.6 s on `main`, 153.3 s on the candidate, summed leaf time 1,673.1 s against 1,782.7 s. A second pair was stopped while it ran, and the tool binaries were copied into both worktrees.
- **A series at 22:57:01 MSK** ran no test at all, because of a quoting error in the measurement loop.

## Conclusions

- **On this machine `internal/api` is throughput-bound on PostgreSQL's checkpointer.** That holds both at the start of the run and in its parallel phase. A schedule that keeps more tests, and so more databases, running at once lengthens the package. That now includes running the serial parent tests in parallel, as it already included a higher `-parallel`, more packages and deferred drops.
- **None of the levers that this profile and the earlier lane A records examined reaches 15 % while keeping one database per test.**
  - No test waits out a fixed interval.
  - The template has little to shrink.
  - Running the parents in parallel made the package slower.
- **What remains:**
  - **Fewer test databases,** that is subtests sharing a database. This changes test isolation and is to be written up as deferred proposal 0005, its own change.
  - **The end-to-end run,** about 400 s of a 600 s gate.
  - **The machine.** In the 10.1 gates the night gates ran the end-to-end suites in about 363 s, the afternoon gates in about 402 s.

## What this record does not cover

- **Lane A was not measured inside a gate,** next to lanes B and C. Inside a gate it takes 1.1 to 1.4 times its standalone time.
- **Other changes to the serial parents were not tried,** such as making only some of them parallel. The paired result points the same way for any change that raises the number of live databases.
- **`shared_buffers`, the checkpointer settings and a tmpfs tablespace were not tried.** Each changes the container's configuration.

## Owner decision

On 2026-11-18 the owner decided:

- **The parallel parents are not merged.** The 19 top-level tests keep running serially, because the paired runs measured the change as slower.
- **Only this record merges,** with a row in `knowledge/reviews/slice-10-known-limitations.md` saying the 15 % target was not reached.
- **Subtests sharing a test database** are to be written up as proposal 0005 in `docs/proposals/`, deferred, as its own change.
