# Local PostgreSQL durability settings as a merge-gate speed-up: measured and rejected

Recorded on 2026-11-12 as the first follow-up to decision D2 of the [slice 9 plan](../plans/slice-9.md). D2 caps the merge gate at 480 s and, as written, measures three consecutive gates of the C0 change's tree. Because C0 had already merged, the owner decided on 2026-11-11 to verify D2 instead on three consecutive full-run gates of the slice's following changes, counted from the B2 change, recording the load average at each start, and to make a follow-up speed-up if any one exceeded 480 s; the slice plan does not yet record that reading. The first of these gates (the B2 change, tree `003b39d`) took 440.6 s at a load average of 3.06; the second (the B3 change, tree `f5cf613`) took 503.9 s at 5.22. The next passing gate, of the change merged as `a5dfb41`, ran lane A for 182 s and the end-to-end run for 375 s, so it exceeded the ceiling too (first table below).

The candidate measured here was running the local PostgreSQL container with `fsync=off`, `full_page_writes=off` and `synchronous_commit=off`. It shortens the backend test run, but with it the backend test run failed with loopback connection timeouts, so it was not adopted. The owner decided on 2026-11-12 to drop it and to target the end-to-end suite next. No repository file other than this record changed.

## Where lane A spends its time

The gate's critical path is lane A (`go test -race -count=1 ./...`) followed by `pnpm e2e`.

| Gate | Lane A | `internal/api` | End-to-end run (tests) |
|---|---|---|---|
| B2, tree `003b39d`, passed | 133.7 s | 131.6 s | 305.4 s (1789) |
| B3, tree `f5cf613`, passed | 158.1 s | 151.4 s | 344.5 s (1789) |
| the change at `a5dfb41`, tree `c8964dc`, failed attempt at 07:51 UTC | — | 165.0 s | — |
| the change at `a5dfb41`, tree `4392287`, passed at 08:17 UTC | 182 s | 179.5 s | 375 s (1894) |
| B4, 2026-11-12, passed (518.3 s) | 157.8 s | 151.0 s | 359.0 s (1894) |

Lane A is set by `internal/api`. Between the B2 and B3 gates the end-to-end run kept the same 1789 tests and grew by 39 s, so that growth was machine load, not suite growth; lane A grew by 24 s, from both load and B3's added backend tests, which these logs cannot separate.

Every integration test creates its own database from the test process's template and drops it synchronously in its cleanup (`backend/internal/testdb/testdb.go`), so the test's `t.Parallel` slot is held until the drop returns. On PostgreSQL 18 each `DROP DATABASE` requested an immediate checkpoint and waited for it: the container's log (checkpoint logging is on by default) shows only checkpoints of the kind `immediate force wait` during the backend tests. For lane A of the B3 gate (21:58:13 to 22:00:51 UTC on 2026-11-11, 158 s):

- 276 checkpoints;
- 109.6 s of time in the single checkpointer, of which 81.2 s was syncing 188,332 files and 21.6 s writing;
- 1.22 million buffers written (about 9.3 GiB).

The end-to-end run of the same gate (22:00:51 to 22:06:36 UTC) caused 50 checkpoints and 24.5 s of checkpointer time.

## Method

- Machine: 12 logical cores, macOS, Docker Desktop with 12 CPUs for its virtual machine, PostgreSQL 18.6 from `docker-compose.yml`. Measured on `main` at `a5dfb41` (window 1) and `7a3c5b7` (windows 2 and 3), 1894 end-to-end tests. In each window the coordinating session held back every merge gate, end-to-end run and full backend run.
- Commands: `go test -race -count=1 ./...` from `backend/` and `pnpm e2e --trace=off` from `frontend/`, each through `tools/dev-env.sh`, one after the other as the gate runs them; lanes B and C were not run. Windows 2 and 3 ran under `caffeinate -dis` on mains power. Wall time is `date +%s` around each command; the one-minute load average was read at each start.
- Checkpoints: counted and summed from the container log over each `go test` run.
- Sockets: the number of TCP sockets in `TIME_WAIT` and `SYN_SENT` (`netstat -an -p tcp`) sampled every second in windows 2 and 3.
- The setting was applied by recreating the container from a `docker-compose.yml` with `command: ["postgres", "-c", "fsync=off", "-c", "full_page_writes=off", "-c", "synchronous_commit=off"]`, on the same `ovc_postgres-data` volume, and removed by recreating it from `main`'s file. The container was left in `main`'s configuration after each window.
- Window 1's runs with the setting are excluded from the tables: the machine entered clamshell sleep on battery two minutes after its first run. That first `go test` run, about 60 s after the recreate and before the sleep, failed the same way as runs 6 and 9 below: 9 tests, 6 with loopback connection timeouts, 2 with `read the database name: network` and 1 with a `500 internal` answer. `pmset -g log` shows no sleep or dark wake in windows 2 and 3.

## Results

`go test -race -count=1 ./...`:

| Run | Configuration | Start (UTC) | Load (1 min) | Wall | `internal/api` | Result | Checkpoints | Checkpointer time (sync) | `SYN_SENT` max | `TIME_WAIT` max |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | default | 11:07:47 | 4.24 | 125 s | 123.4 s | passed | 325 | 83.4 s (62.1 s) | not sampled | not sampled |
| 2 | default | 11:14:54 | 21.03 | 129 s | 126.8 s | passed | 350 | 83.3 s (59.6 s) | not sampled | not sampled |
| 3 | default | 11:21:46 | 18.13 | 126 s | 124.2 s | passed | 319 | 86.0 s (62.2 s) | not sampled | not sampled |
| 4 | default | 13:50:30 | 16.26 | 136 s | 119.5 s | passed | 398 | 86.7 s (62.6 s) | 1 | 2662 |
| 5 | default, 2 s after a recreate | 14:14:03 | 11.99 | 158 s | 155.7 s | passed | 300 | 111.3 s (77.3 s) | 2 | 2216 |
| 6 | durability off, first run, 33 s after a recreate | 13:53:19 | 6.11 | 91 s | failed after 89.3 s | **9 failed** | 1381 | 14.0 s (1.4 s) | 37 | 3208 |
| 7 | durability off | 13:59:39 | 27.32 | 99 s | 97.1 s | passed | 1359 | 14.4 s (1.4 s) | 18 | 3239 |
| 8 | durability off | 14:06:08 | 21.01 | 106 s | 103.9 s | passed | 1330 | 15.0 s (1.3 s) | 19 | 2512 |
| 9 | durability off, first run, 300 s after a recreate | 14:21:44 | 2.49 | 94 s | failed after 91.8 s | **4 failed** | 1296 | 19.4 s (1.3 s) | 16 | 3069 |
| 10 | durability off | 14:23:18 | 8.56 | 110 s | 108.4 s | passed | 1331 | 15.3 s (1.3 s) | 30 | 2992 |

`pnpm e2e --trace=off`, each 1894 passed:

| Configuration | Wall times | Mean |
|---|---|---|
| default (window 1) | 295 s, 274 s, 263 s | 277 s |
| durability off (window 2) | 289 s, 290 s, 303 s | 294 s |

- **Speed.** With durability off the checkpointer's sync time falls to about 1.4 s per run and its total to 14 to 19 s. The number of checkpoints rises 3.3 to 4.6 times, probably because cheaper checkpoints let fewer drop requests coalesce into one. The passing runs of `go test` took 105 s on average against 135 s for the five default runs (129 s without run 5), about 24 to 30 s less. The end-to-end run did not get faster; its difference is within the run-to-run spread.
- **Failures.** All three first runs after a recreate with durability off failed (runs 6 and 9, and window 1's excluded run); all three later runs (7, 8 and 10) passed; all five default runs passed. The failures are `dial tcp 127.0.0.1:<port>: connect: operation timed out` against the test's own in-process HTTP server and `dial tcp 127.0.0.1:55432 ... timeout` against the PostgreSQL port Docker publishes, plus `read the database name: network` from the test database fixture (a network-kind error, most likely the same failed dial, though the log does not say) and, in run 9, one `500 internal` answer in `TestChatDownloadDeadlineCoversStorageBody` whose cause the log does not show. In run 6 each failing test took 7.8 to 9 s; the logs carry no timestamps, so the failures cannot be placed against the socket samples.
- **Sockets.** `TIME_WAIT` stayed under 3,300 of the 16,384 ephemeral ports in every run, so the ports were not exhausted. `SYN_SENT` stayed at 0 to 2 in the two sampled default runs and rose to 16 to 37 in every sampled run with durability off, the passing ones included, in several bursts per run: new connections to loopback listeners waited seconds for an answer. Both in-process listeners and Docker's published port were affected. The [loopback connection stall analysis of 2026-10-26](e2e-loopback-connection-stalls-2026-10-26.md) found the same pattern on this machine during end-to-end runs (7.8 s connection timeouts, 17 to 21 sockets in `SYN_SENT`, every process and destination stalling together) and attributed it, unverified, to the packet filter of the machine's VPN kill switch keeping state for loopback connections and dropping new connections while its state table is full, which grows with the connection rate. A per-listener backlog limit (`kern.ipc.somaxconn`, 128 here) would not stall separate listeners together, so the host-wide cause is the more likely one here too. Neither was proven.
- **The recreate.** Run 5 shows that recreating the container with the default settings does not cause the stall. Run 9 shows that waiting 300 s after recreating it with durability off does not avoid it. These runs cannot tell whether the failures need both durability off and a first run after a recreate, because every failing run was such a first run; a merge gate does not recreate the container. The rejection does not rest on that question: the stall's precursor, `SYN_SENT` far above the default runs' level, appeared in every sampled run with durability off.

## Conclusions

- Durability off saved about 24 to 30 s of lane A on this machine, and with it the backend test run failed with loopback connection timeouts in three of six runs, each the first run after a recreate, and showed connection stalls in every sampled run; a flaky gate is a bug under `AGENTS.md`. Rejected.
- A reliable saving of that size would not meet D2 on its own: the B4 gate took 518.3 s on the default settings, and 518.3 s less 30 s is 488 s.
- Any change that makes lane A open loopback connections faster (more parallel tests, more packages running at once) can meet the same stall. Its measurement should sample `SYN_SENT` as this record does.
- The forced checkpoints remain lane A's largest single cost: about 85 s of serialized checkpointer time per default run, which every test's cleanup waits on. Removing that wait without raising the connection rate, for example by dropping test databases outside the test's own cleanup, was not tried here.
- The end-to-end run is the larger part of the gate and the target of the next change.

## What this record does not cover

- Lanes B and C were not run beside lane A, so these lane A times are lower than a gate's.
- No machine setting was changed: neither the VPN client's packet filter nor `kern.ipc.somaxconn`. Whether either removes the stall is unknown.
- `synchronous_commit=off` on its own, which keeps crash safety, was not measured.
- The gate figures in the first table come from log file times and Playwright's summary lines (5.1m, 5.7m, 6.2m and 6.0m), so they are accurate to a few seconds.
