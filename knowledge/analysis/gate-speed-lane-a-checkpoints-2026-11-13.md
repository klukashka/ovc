# Lane A of the merge gate: what bounds the API package's tests, and two fixture changes measured and rejected

Recorded on 2026-11-13. The gate of `main` at `c3d3e1b` took 567.6 s (lane A 215.1 s, the end-to-end run 351.1 s), above the 540 s at which decision D4 of the [slice 9 plan](../plans/slice-9.md) calls for a consolidation before further feature changes merge. On 2026-11-13 the owner decided to make a second D4 consolidation now, before the B5, F2 and B6 changes merge, and approved a task plan that targets lane A; the slice plan, which lists consolidations C0 and C1, does not yet record this decision or the 567.6 s gate. That plan diagnosed lane A first and then measured two changes to the test database fixture as prototypes. Both missed their go criterion. As the plan required, this record is the only file the change commits; no code changed.

Earlier records on lane A:

- [the durability settings, rejected](gate-speed-postgres-2026-11-12.md), which also proposed dropping test databases outside the test's own cleanup, candidate 2 below;
- [the test-database pool, a no-go](gate-speed-test-database-pool-2026-11-12.md).

## Method

- **Machine:** 12 logical cores, macOS, Docker Desktop, PostgreSQL 18.6 from `docker-compose.yml` with `main`'s settings throughout.
- **Command:** `tools/dev-env.sh go test -race -count=1 -json`, either for `./internal/api/` alone or for `./...`, from `backend/`. Lanes B and C did not run beside it.
- **Isolation:** the coordinating session held back every merge gate, end-to-end run and full backend run during each window. No `go test` or Playwright process was running at any start.
- **Load:** each run started only once the one-minute load average was below 6.
- **Recorded per run:**
  - wall time;
  - user and system CPU time (`/usr/bin/time -p`);
  - per-test times (from `go test -json`);
  - the number of TCP sockets in `SYN_SENT` and `TIME_WAIT`, sampled every second;
  - the container's checkpoint log lines for the run.
- **Power** at each start (`pmset -g batt`):
  - the diagnosis window ran on battery;
  - the `FILE_COPY` window ran on mains power;
  - the deferred-drop window ran on battery.
- **Pairing:** prototype runs alternated with runs of a detached worktree of `main` (`c3d3e1b`) in the same window, so each comparison is paired.

## Diagnosis

Measured on 2026-11-13 from 00:15 MSK on `main` at `162258f`; the commits from there to `c3d3e1b` change only `frontend/e2e` and `docs/development.md`, so nothing lane A runs:

| Run | Load | Wall | CPU user+sys | Checkpoints | Write / sync / total checkpointer time | `SYN_SENT` max |
|---|---|---|---|---|---|---|
| `./...` | 3.60 | 151 s (`internal/api` 148.8 s) | 232 s | 321 | 22 / 72 / 101 s | 2 |
| `internal/api`, default `-parallel` (12) | 5.98 | 115 s | 168 s | 318 | 17 / 43 / 64 s | 1 |
| `internal/api`, `-parallel 24` | 5.75 | 233 s | 181 s | 273 | 20 / 174 / 199 s | 5 |
| `internal/api`, `-parallel 6` | 5.52 | 137 s | 158 s | 761 | 15 / 44 / 63 s | 1 |

- **The API package is not bound by the CPU.**
  - Its test binary used about 1.5 of 12 cores (168 s of CPU in 115 s).
  - Its 836 tests summed 765 s, a mean of 0.91 s, and 18 tests that do not call `t.Parallel` ran first, for 12 to 14 s.
- **Its tests wait on PostgreSQL's single checkpointer.** Every test's cleanup runs `DROP DATABASE … WITH (FORCE)`, which requests an immediate checkpoint and waits for it (the checkpoint kind `immediate force wait` in the [durability record](gate-speed-postgres-2026-11-12.md)). Each checkpoint wrote about 4,000 buffers, about one freshly cloned database, and synced the files written since the last one. That is 1.27 million buffers, about 9.7 GiB, per run of the package.
- **More concurrent tests made the package slower, not faster.**
  - At `-parallel 24` the wall time doubled to 233 s and the checkpointer's sync time grew from 43 s to 174 s.
  - The mean test took 3.7 s against 0.91 s.
  - More databases alive at once leave more files for each checkpoint to sync: about 1,540 files per checkpoint at `-parallel 24` against about 730 at the default.
- **Fewer concurrent tests made it slower too.** At `-parallel 6` the package took 137 s: the mean test took 0.56 s, but drops coalesced less (761 checkpoints), so the default of 12 is near the best this setting can reach.
- **Splitting the API package into several packages was therefore not attempted.** `go test` would run them side by side, which raises the number of concurrent tests and live databases as `-parallel` does. It would also move 115 test files and their shared helpers.
- **Lane A takes longer inside the gate than alone:** 215.1 s in the `c3d3e1b` gate against 151 s here. Lanes B and C then compete for the same disk, CPU and Docker virtual machine. These runs do not measure that contention.

## Go criterion

The approved plan set the same criterion for each candidate, measured paired against `main`: every run passes, `SYN_SENT` stays at or below 5, and the mean wall time of `internal/api` is at least 25 % below `main`'s. A candidate that met it would then also have had to bring `go test -race ./...` at least 20 % below `main` and leave no `ovc_%_test` database other than templates and no session in one after the runs. When both candidates missed it, the plan required stopping with this record.

## Candidate 1: clone test databases with `STRATEGY FILE_COPY`

The fixture's clone statement became `CREATE DATABASE … TEMPLATE … STRATEGY FILE_COPY`.

- The default strategy, `WAL_LOG`, copies the template page by page through shared buffers and the WAL.
- `FILE_COPY` copies its files and requests a checkpoint before and after the copy.

Window: 2026-11-13 from 01:52 MSK, mains power, load 5.16 to 5.97.

| Pair | `main` `internal/api` | `FILE_COPY` `internal/api` | `SYN_SENT` max (`main` / `FILE_COPY`) |
|---|---|---|---|
| 1 | 127 s | 116 s | 1 / 11 |
| 2 | 112 s | 106 s | 0 / 11 |
| 3 | 123 s | 103 s | 43 / 12 |
| Mean | 120.7 s | 108.3 s | |
| `./...` | 131 s | 132 s | 12 / 17 |

- All eight runs passed.
- Checkpoints rose from about 315 to about 1,900 per run of the API package. The buffers they wrote fell from about 1.25 million to about 0.26 to 0.33 million, and the checkpointer's total time stayed about the same: 62 to 73 s against 64 to 69 s for `main` in the same window.
- `internal/api` was 10 % faster, and the whole run was not faster at all.
- **No-go:** the go criterion was 25 % on `internal/api`.
- `SYN_SENT` also exceeded the criterion's 5 in every `FILE_COPY` run. In the same window `main` itself peaked at 43 and 12, so in this window the criterion did not separate the candidate from `main`.

## Candidate 2: drop test databases after the test, from a bounded queue

- **Prototype.** A test's cleanup in the API package closed its database to new connections (`ALTER DATABASE … WITH ALLOW_CONNECTIONS false`) and queued it. A queue of at most 12 databases fed four goroutines that each ran the `DROP DATABASE … WITH (FORCE)`.
- **Drain.** A `TestMain` in the package drained the queue before the process exited, and every other package kept today's synchronous drop.
- **The idea.** The test's parallel slot is freed at once, and concurrent drops can share checkpoints.

Window: 2026-11-13 from 11:13 MSK, on battery, load 2.27 to 5.87.

| Pair | `main` `internal/api` | Deferred drops `internal/api` |
|---|---|---|
| 1 | 120 s | 250 s |
| 2 | 112 s | 212 s |
| 3 | 105 s | 136 s |
| `./...` | 128 s | 215 s |

- All eight runs passed, and `SYN_SENT` peaked at 1 to 8.
- With deferred drops the package caused about 620 checkpoints, which wrote about 1.8 million buffers and spent 105 to 198 s syncing, against 43 s in the `main` run of the first pair.
- The tests' slots were freed, but by the design's bound up to about 28 databases could be alive at once: twelve running tests, twelve queued and four being dropped. The checkpoints synced about 1,420 to 1,440 files each against about 620 to 780 for `main`, the effect `-parallel 24` showed.
- **No-go:** the package was slower in every pair.

## Conclusions

- On this machine lane A's API package is bound by PostgreSQL's checkpointer. Every test's drop waits for a checkpoint, and each checkpoint's cost grows with the number of test databases alive at once.
- **More concurrency lengthens lane A, and the default concurrency is near its best.** That rules out more parallel tests, more packages and deferred drops, each of which keeps more databases alive.
- **Copying databases by file did not reduce checkpoint time enough:** fewer buffers but six times the checkpoints.
- Together with the two earlier records, no lane A change tried so far is usable:
  - durability off saved about 20 % but made the runs fail with loopback connection timeouts;
  - database reuse was not faster and its runs failed;
  - `FILE_COPY` saved 10 % of the API package and nothing of the whole run;
  - deferred drops were slower.
- **Remaining levers outside the fixture:**
  - fewer test databases, that is tests sharing a database, which changes test isolation and needs its own decision;
  - the end-to-end run, which is 351 s of the 568 s gate;
  - the machine: the Docker virtual machine's disk and the loopback stall, neither changed here.
- `SYN_SENT` above 5 appeared in `main`'s own runs in the `FILE_COPY` window (43 and 12). A criterion of at most 5 no longer separates a candidate from the baseline on this machine, and a later measurement needs a paired criterion instead.

## What this record does not cover

- Two of the three windows ran on battery. The pairs are compared within one window, and the deferred-drop result is far outside the spread, but absolute times on battery are not comparable with gate times.
- Lane A was not measured inside a gate, beside lanes B and C.
- The leftover check (no `ovc_%_test` database other than templates after the runs) is unverified: it was not run. Neither prototype is committed.
- Other queue sizes and worker counts for deferred drops were not measured. A smaller queue keeps fewer databases alive, but it brings back the wait it was meant to remove.
- `shared_buffers`, the checkpointer settings and a tmpfs tablespace were not tried. Each changes the container's configuration, which the plan did not cover.
