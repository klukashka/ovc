# Reusing test databases as a merge-gate speed-up: prototype measured, no-go

Recorded on 2026-11-12 as the second follow-up to decision D2 of the [slice 9 plan](../plans/slice-9.md), after the [PostgreSQL durability measurement](gate-speed-postgres-2026-11-12.md) found that each integration test's `DROP DATABASE` forces a checkpoint that its cleanup waits for. The owner approved a plan to reuse test databases from a pool instead of dropping them, starting with a throwaway prototype measured against a go/no-go criterion. The prototype missed the criterion, so the full design was not built and no code changed.

## The prototype

- `testdb.New` took an idle database from a process-wide pool, or cloned the process template when the pool was empty.
- Its cleanup, after the test's pool closed, opened a one-connection pool to the database. Through it, the cleanup truncated the tables that were empty in the template, with `RESTART IDENTITY` and without `CASCADE`. It then compared a content digest of each table the migrations seed with the template's.
- It renamed the database to a fresh `ovc_pool_<12 hex>_test` name through the administrative pool (during run 6 the name was switched to `ovc_<12 hex>_test`, see Results). PostgreSQL refuses the rename while another session is connected, so the rename doubles as the check that no session remains. It then returned the database to the pool.
- Any error or digest mismatch dropped the database as before.
- `NewNamed`, `NewMigratedTo` and `NewForLockContention` were unchanged.
- The safety parts of the approved design were left out: the catalog fingerprint, the `Disposable` type, the owner-aware sweep and the threat-model update.

## Go/no-go criterion

Every run passes, `SYN_SENT` stays at or below 5, and lane A is at least 20 s faster on average than with the current fixture.

## Method

- Machine: 12 logical cores, macOS, on battery, under `caffeinate -dis`; `pmset -g log`, read after the window, showed no sleep or dark wake in it.
- Tree: `main` at `bff363e`.
- The coordinating session held back every other heavy run.
- Runs: three `go test -race -count=1 ./...` from `backend/` through `tools/dev-env.sh` with the current fixture, then three with the prototype, back to back.
- Sockets: host-wide TCP sockets in `TIME_WAIT` and `SYN_SENT` were sampled every second with `netstat -an -p tcp`, with no filter by destination. Checkpoints were read from the container log; the table gives their count and the sum of their `total` times.

## Results

| Run | Fixture | Start (UTC) | Load (1 min) | Wall | `internal/api` | Result | Checkpoints (summed time) |
|---|---|---|---|---|---|---|---|
| 1 | current | 15:10:22 | 3.97 | 158 s | 150.4 s | passed | 320 (108.9 s) |
| 2 | current | 15:13:00 | 11.04 | 178 s | 176.5 s | passed | 277 (121.6 s) |
| 3 | current | 15:15:59 | 11.47 | 215 s | 213.1 s | passed | 292 (148.5 s) |
| 4 | pool | 15:19:34 | 12.44 | 195 s | 188.5 s | failed | 101 (166.7 s) |
| 5 | pool | 15:22:49 | 17.29 | 166 s | 164.4 s | failed | 113 (135.6 s) |
| 6 | pool | 15:25:35 | 9.70 | 179 s | failed | failed | 127 (139.1 s) |

- **Speed.**
  - The pool cut the forced checkpoints from about 300 to about 110 per run, and the buffers they wrote from about 1.2–1.3 million to about 0.33–0.35 million per run, so each checkpoint was smaller (about 3,000 buffers against about 4,300) and writing took 8–11 s instead of 23–31 s.
  - Their sync time rose from about 0.3 s to about 1.2 s per checkpoint (120–149 s per run against 78–107 s), so the summed checkpoint time rose from 126 s to 147 s on average. Why the syncs grew longer is not known.
  - Wall time was 180 s on average with the pool against 184 s without it, within the spread, which was wide on battery (158 to 215 s for the same tree). The pooled runs also started at a higher load (13.1 against 8.8 on average), and run 6's time includes ten tests that each waited about 10 s for a storage timeout.
- **Failures.**
  - `TestSharedFakeAcrossProcesses` (`internal/adapters/meeting`) failed in every pooled run. Its child process accepts only database names of the form `ovc_<12 hex>_test`, and the prototype named its databases `ovc_pool_…`. That is the likely cause, an artefact of the prototype rather than of reuse, but the child's output is withheld, so it is unverified. To remove it, the name was switched to `ovc_<12 hex>_test` while run 6 was already compiling, so run 6 ran partly under each name; the test still failed in run 6, which fits its package having been built before the switch.
  - In run 6, ten `internal/api` tests failed after about 10 s each with `test storage: timeout` against the local S3-compatible server.
- **Sockets.**
  - `SYN_SENT` peaked at 19, 34 and 14 in runs 4, 5 and 6 against at most 2 with the current fixture. Runs 4 and 5 had no connection failures; run 6, the only run with failures (the storage timeouts above), had the lowest peak, which weakens the link between these peaks and the failures.
  - Two changes could raise the connection rate: the one-connection reset pool each test now opens, and cleanups that no longer wait on a checkpoint, so tests start and connect sooner. The administrative pool was already used for every drop before, so the rename adds no new kind of connection.
  - The pattern matches the stall the durability measurement recorded, but the sampler counts all TCP sockets, not only loopback ones, so this is not proven.
- **Leftovers.** Each test process left its idle pooled databases behind. The three pooled runs left 186 named `ovc_pool_…`, dropped afterwards through the fixture's administrative pool (owned databases only, without `FORCE`). Databases that run 6 renamed after the name switch look like ordinary test databases and were not swept; they stay until removed with the other leftovers of failed test runs.

## Conclusions

- No-go on all three counts: the pooled runs failed, `SYN_SENT` exceeded 5, and lane A was not faster.
- Reducing the forced checkpoints by reuse did not shorten lane A on this machine: the checkpoints became fewer and smaller, but their syncs grew longer, and the connection count likely rose with a higher test rate, which was not measured.
- A further lane A change would likely need to reduce the connections per test, for example through shared pools, rather than move work between PostgreSQL steps. That rests on the unproven link between connection rate and the stalls.
- The end-to-end run remains the larger part of the gate.

## What this record does not cover

- The runs were on battery and the spread was wide; a mains-power repeat could show a small gain, though one large enough to meet the criterion is unlikely with the stall present.
- The prototype left out the safety parts of the approved design, so its numbers do not measure their cost.
- That the coordinating session held back other heavy runs during the window rests on its message, not on a log.
