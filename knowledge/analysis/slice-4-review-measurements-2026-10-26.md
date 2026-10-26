# Slice 4 review performance measurements

Measured on 2026-10-26 for the slice 4 review, on `main` at `08e72cb`, as listed in section 9 of the [slice 4 plan](../plans/slice-4.md) ("Performance measurements at the slice review"). The measurements ran one at a time on the development machine with no other session running checks.

## Machine and method

- **Machine:** Apple M4 Pro, 12 cores, 24 GB, macOS 26.2, Go 1.27.3, Node 26.12.1; Docker Desktop running the Compose PostgreSQL 18 and storage services. Before each measurement the one-minute load average was below 3; the values before and after are given with each measurement.
- **Backend harness:** temporary `_test.go` files in a separate detached git worktree at `08e72cb`, run through `tools/dev-env.sh`. They built the API exactly as the integration-test harness of `internal/api` does: the real handler on an in-process HTTP server, the real gateway and PostgreSQL listener, a fresh migrated test database, and a database pool of 10 connections (the production default `OVC_DB_POOL_SIZE`) carrying a statement-counting tracer. Data was seeded with SQL. Socket clients ran in a separate child process, so the API process's memory, goroutine and CPU figures do not include the clients. The worktree was removed afterwards; the harness files are not committed.
- **Frontend harness:** a temporary Playwright spec in the same worktree, run in Chromium against one deployment of the end-to-end harness.
- **Waiting:** measurement code waited on explicit conditions with deadlines. The only timed waits are the idle windows over which CPU was measured, the fixed 200 ms send time of the simulated email provider, and the clients' reconnection backoff, which copies the web client's.

## Summary

| Measurement | Result | Baseline or target | Verdict |
|---|---|---|---|
| Tutor deletion at 50,000 relationships | 7.9 s median (7.77 to 8.14 s); 30,023 statements | 0.95 s | failed |
| 2,000 idle sockets on one API instance | 4 goroutines, about 23 KB of heap and 47 KB of RSS per socket; 1.0 % of one core | none set | passed |
| One `entity.changed` to 2,000 sockets | p95 12.0 ms from commit | none set | passed |
| Re-check latency for one account | about 0.57 ms added (p95 0.69 ms) | none set | passed |
| Sweep at 2,000 sessions | about 18 ms; session lookup p50 2.0 ms | none set | passed |
| Listener loss detection | 18 to 38 ms when the connection drops; 3.2 to 4.8 s when it goes silent | 5 s after the last heartbeat | passed |
| Notification queue during bursts | at most 0.0029 % | none set | passed |
| Mass reconnect after `1012` | no heartbeat loss; 26 to 81 s until all 2,000 are ready again | none set | unverified |
| NFR-4.5 | the high-priority email was sent 214 to 389 ms after it was queued | one free slot, far under 2 minutes | passed |
| Notification lock contention | at most 7 waiting transactions; contention adds about 1 ms per request | milliseconds | passed |
| `GET /notifications` with 5 million rows | p95 4.0 ms (first page) | none set | passed |
| Bundle | 175.6 / 190.3 / 190.5 KB gzip | 180 / 250 / 400 KB | passed |
| Frontend, 100 reconnects and 100 panel cycles | listeners, DOM nodes and query-cache entries stay flat | flat | passed |
| Full local check run | 237.9 s cold, 177.5 s warm | 178 s slice target, 180 s threshold | failed when cold; passed when warm, by 0.5 s |

## Tutor deletion at 50,000 relationships

- **Method:** `DELETE /api/v1/users/me` for a tutor with the dataset of the [slice 3 measurement](../reviews/slice-3-known-limitations.md):
  - 30,000 active relationships, 10,000 ended relationships and 10,000 invited relationships, each invited one with an open invitation;
  - 200 groups holding 10,000 memberships (the slice 3 record does not give its membership count);
  - 40,000 confirmed students.

  Three samples were taken without a listener connected, and three with the real gateway and listener running, each on a fresh database. The tracer counted and timed every statement of the request.
- **Machine state:** load average 2.09 before and 2.52 after.
- **Results:**

| Sample | Without listener (s) | With listener (s) |
|---|---|---|
| 1 | 7.856 | 7.774 |
| 2 | 7.886 | 8.039 |
| 3 | 8.139 | 7.979 |

- **Statements:** 30,023 per request. Of these, 30,000 are `NotifySignal`, one `pg_notify` round trip for each relationship ended by the deletion, taking 6.79 s in total (about 226 µs each). The remaining 23 statements take about 1.1 s:

| Statement | Time |
|---|---|
| `InsertOutboxRows` | 0.41 s |
| `EndRelationshipsForTutorDeletion` | 0.30 s |
| `DiscardRelationshipsForTutorDeletion` | 0.09 s |
| `RevokeTutorInvitations` | 0.08 s |
| commit | 9 ms |

- **Other outcomes:**
  - 30,000 `tutor.deleted` outbox rows were queued;
  - the 10,000 relationships that were already ended were not signalled;
  - with the listener running, all 30,000 signals were delivered to the listener at commit.
- **Comparison:** about 8.3 times the 0.95 s baseline. Without the `pg_notify` round trips the request would take about 1.1 s, close to the baseline. The backend reviewer's prediction is confirmed: `signalTutorDeletion` in `services/tutoring/deletion.go` calls `Signals.Entity` once per relationship, and each call issues its own statement inside the transaction. The plan's "chunked signals" are not implemented for this path.
- **Likely fix:** issue all of a deletion's signals in one statement, for example `SELECT pg_notify(…) FROM unnest(…)`.
- **Verdict:** failed.

## WebSocket gateway under load

All gateway measurements used one API instance with 2,000 student accounts, each signed in once (so there are 2,000 sessions) and holding one authenticated socket. Each socket came from its own trusted forwarded address. The listener connected through the test forwarder of `internal/testdb/forwarder`, so that it could be made silent. Production timing applied: pings every 25 s, a lapse check every second, a sweep every 60 s, heartbeats every 2 s, and loss after 5 s. The run that produced the numbers below took 4.7 minutes, with a load average of 1.18 before and 1.19 after.

### Idle sockets

- **Ramp:** all 2,000 sockets were ready in 460 ms, with the client opening 100 at a time and no refused upgrades.
- **Memory and goroutines:** measured after a garbage collection.

| Measurement | 0 sockets | 2,000 sockets | Per socket |
|---|---|---|---|
| Goroutines | 23 | 8,024 | 4.00 (reader, writer, pinger, library) |
| Heap in use | 11.9 MB | 58.5 MB | 22.8 KB |
| RSS | 54.3 MB | 146.6 MB | 47.3 KB |

The heap figure is an upper bound, because it includes the harness's in-memory log of two records per socket.

- **CPU:** over a 65 s idle window, which included pings, 32 heartbeats, 65 lapse checks and one sweep, the API process used 724 ms of CPU, or 1.11 % of one core, against 0.09 % with no sockets. That is about 1.0 % of one core for 2,000 sockets. An earlier full run measured 0.96 %.
- **Cleanup:** after the client process was killed, the registry was empty and goroutines were back at 10 within the first check.

### One `entity.changed` to 2,000 sockets

- **Method:** 20 samples. Each sample is one transaction calling `Signals.Entity` for all 2,000 users, which the code splits into 20 notifications of 100 recipients. Delivery is measured as the time from the return of the commit to the client's receipt of the frame.
- **Results:** over all 40,000 deliveries, p50 5.9 ms, p95 12.0 ms, p99 17.8 ms and maximum 20.4 ms. The per-sample p95 had a median of 10.4 ms and a worst of 19.7 ms. The commit itself took a median of 5.7 ms. An earlier full run measured a p95 of 12.3 ms.
- **Verdict:** passed; no target was set.

### Re-check latency and sweep

- **Re-check latency:**
  - An `entity.changed` frame to one account arrived at p50 49 µs and p95 88 µs after the commit.
  - When the same transaction first sent a `recheck` for that account, which suspends its sockets until the session lookup returns, the frame arrived at p50 619 µs, p95 686 µs and at most 886 µs.
  - The re-check therefore adds about 0.57 ms (20 samples each).
- **Sweep at 2,000 sessions:**
  - the sweep's log record came 17.6 ms after its tick (16.5 ms in the earlier run), with 2,000 sockets checked and none closed;
  - a direct `SessionStandings` lookup of the 2,000 sessions took p50 2.0 ms and at most 7.2 ms over 10 calls.
- **Verdict:** passed.

### Listener loss detection and notification queue usage

- **Connection loss:** `pg_terminate_backend` on the listener's backend was detected 18 to 38 ms later, across 15 samples in five runs, and every socket received `1012` at once.
- **Silent loss:** the forwarder was made to stop relaying, so no error reached the listener. The heartbeat detected the loss after 3.15, 3.99, 4.22, 4.52 and 4.76 s in five runs, and the last `1012` reached a client within 40 ms of detection. This is within the design bound of `LossAfter` = 5 s after the last heartbeat arrived.
- **Notification queue during bursts:** 8 goroutines committed signals as fast as they could, each signal to all 2,000 users, while the queue usage was sampled every 5 ms:

| Burst | Committed in | Frames delivered | Highest `pg_notification_queue_usage()` |
|---|---|---|---|
| 200 signals (4,000 notifications) | 1.49 s | 400,000, all by 1.51 s | 0.0029 % |
| 1,000 signals (20,000 notifications) | 7.20 s | 2,000,000 | 0.0029 % |

The default queue is 8 GB. In both bursts no socket was closed, the listener was never lost, and the pool was saturated at 10 connections by the committing goroutines.

- **Verdict:** passed.

### Mass reconnect after `1012` with a pool of 10

- **Method:** the listener's backend was terminated, so the gateway closed all 2,000 sockets with `1012`. The client process then reconnected each socket with the web client's backoff: a random delay below `1 s × 2^attempt`, capped at 30 s, with the attempt counter reset only after a `pong`. Three variants ran in each run:
  - the client's backoff without the refetch;
  - the client's backoff with one `GET /api/v1/notifications` after each `ready`, standing in for the client's refetch;
  - no backoff on the first attempt, then 50 ms per attempt, with the refetch.

  The table below gives each variant across five runs.

| Variant | Listener back after | All 2,000 ready again after | Listener losses during the reconnect |
|---|---|---|---|
| Client backoff, no refetch | 1.0, 51.0, 3.6, 8.0, 1.2 s | 65.5, 80.9, 65.9, 64.1, 50.8 s | 0, 4, 0, 1, 0 (all `connect` timeouts) |
| Client backoff, refetch | 1.2, 8.0, 5.7, 51.0, 8.6 s | 66.3, 60.2, 64.2, 81.0, 63.8 s | 0, 1, 0, 4, 1 (all `connect` timeouts) |
| No first backoff, refetch | 1.0, 1.6, 1.6, 5.0, 3.1 s | 36.0, 34.1, 35.8, 25.7, 39.1 s | 0 |

- **Findings:**
  - **The reviewer's hypothesis was not reproduced.** No storm produced a heartbeat-kind loss, although the pool reached its 10 connections in almost every storm. Every additional loss was the listener's own reconnection timing out after its 5-second `ConnectTimeout`.
  - **Recovery is slow for another reason.** Sockets that reconnect while the listener is down are refused with `1013 listener_down`, between 146 and 5,925 such refusals per storm. Each refusal raises that client's backoff, so a listener gap of 1 to 8 s stretched full recovery to about a minute, with the median client back after 3 to 61 s.
  - **Single-host network stalls.** Many client dials failed with `connect: operation timed out` on loopback (575 to 5,015 per storm). The listener's reconnections timed out during the same storms. The peak was 5,436 `TIME_WAIT` sockets, against an ephemeral port range of 16,384, so the ports were not exhausted. The likely cause is the single-host loopback network stack of macOS (`kern.ipc.somaxconn` = 128), with 2,000 clients and the server on one machine.
  - **Busy refusals.** Without a first backoff, the 500-socket cap on unauthenticated sockets also refused upgrades with `503 busy` (up to 2,084 per storm).
- **Comparison:** no target was set. Whether the listener's reconnection also stalls when clients arrive from many hosts is not established by this machine.
- **Possible improvements:** jitter or a delay before reconnecting after `1012`, or a shorter listener backoff, would remove most of the `1013` refusals and the backoff growth they cause. The listener heartbeat could be given a dedicated connection, so that pool saturation can never delay it.
- **Verdict:** unverified for a production network. The heartbeat feedback through the shared pool was examined and not observed; the slow recovery is recorded as a finding.

## NFR-4.5: a high-priority email behind 1,000 queued emails

- **Method:** the real worker ran with `Run` and the system clock, with production lane sizes (10 email slots and 4 storage-delete slots). The queue held:
  - 1,000 normal emails, each taking 200 ms to send;
  - 300 storage deletes, each taking 20 ms.

  Once all 10 email slots were busy, one high-priority email was queued at each of +1, +4, +8, +12 and +16 s. The worker's own polling claimed it; nothing woke the worker.
- **Machine state:** load average 1.16 before and 1.32 after.
- **Results:**

| Queued at | Normal emails sent before | Claimed after | Sent after |
|---|---|---|---|
| +1 s | 40 | 14 ms | 214 ms |
| +4 s | 190 | 77 ms | 277 ms |
| +8 s | 390 | 186 ms | 386 ms |
| +12 s | 580 | 83 ms | 283 ms |
| +16 s | 780 | 189 ms | 389 ms |

- **Storage deletes:** 180 of the 300 were done within the first second and all of them by 4 s, while the email backlog drained in 20.7 s.
- **Comparison:** the high-priority email was always claimed when the next slot freed, within 200 ms. This meets the plan's target (within one free slot, far under 2 minutes) and the 2-minute bound of NFR-4.5. The recording-copy part of NFR-4.5 does not exist yet.
- **Verdict:** passed.

## Notification lock contention

- **Method:** one API instance with the gateway and listener, a pool of 10, and a tracer. A separate connection sampled `pg_locks` continuously for transactions waiting on the notification lock (locktype `object`, class 1262, object 0). Requests in each round started together.
- **Machine state:** load average 1.22 before and 1.22 after.
- **Results:**

| Workload | p50 | p95 | Wall | Waiting on the notification lock |
|---|---|---|---|---|
| 50 tutor-only card edits (`PATCH`, 1 signal each), 3 rounds | 30–35 ms | 44–49 ms | 47–50 ms | at most 2–4, mean ≤ 0.21 |
| 50 endings (`POST …/end`, 2 signals each), 3 rounds | 39–43 ms | 59–66 ms | 60–68 ms | at most 2–4, mean ≤ 0.17 |
| 25 sign-outs (revocation trigger), listener heartbeats only | 8.5 ms | 10.3 ms | 10.7 ms | at most 6, mean 1.96 |
| 25 sign-outs while 8 goroutines sent 3,729 extra heartbeats per second | 10.1 ms | 11.9 ms | 12.5 ms | at most 7, mean 2.26 |
| Purge batch of 100 deleted accounts, each with a session (one `recheck` each) | 432 ms in all, 4.3 ms per account | | | none |

- **Notes:**
  - The commit of an ending took 0.85 ms on average.
  - Request latency in the 50-request rounds is set by the pool of 10, not by the notification lock.
  - The purge issued about 13 statements per account.
  - Every route that writes sends at least one signal (a tutor-only edit still signals the tutor's own sockets), so there is no variant without signals. The comparison is one signal against two, and the second signal adds about 9 ms to p50, most of it queueing for the pool.
- **Verdict:** passed. Contention is in milliseconds, as section 9 of the plan expected.

## `GET /notifications` at 10,000 rows for one account and 5 million in the table

- **Method:**
  - The tutor has 10,000 notifications, half of them unread, naming 50 real relationships.
  - 4,990,000 rows of other accounts were seeded in 2 min 28 s, followed by `VACUUM ANALYZE`.
  - The first page was requested 50 times, and the second page (by cursor) 50 times. Both queries were also run under `EXPLAIN (ANALYZE, BUFFERS)`.
- **Machine state:** load average 1.19 before and 1.68 after.
- **Results:**
  - The first page has 30 items, `unread_count` 100 (the cap) and 8,264 bytes.
  - First page: p50 2.3 ms, p95 4.0 ms, at most 7.9 ms. Second page: p50 2.2 ms, p95 3.4 ms, at most 3.6 ms.
  - Per request, `ListNotifications` took 0.33 ms, `ListNotificationRelationships` 0.33 ms and `CountUnreadNotifications` 0.26 ms. These include the round trip.
  - The page query is an index scan of `notifications_user_list` touching 4 buffers in 0.009 ms. The unread count is an index-only scan of `notifications_user_unread` touching 4 buffers in 0.016 ms.
- **Verdict:** passed.

## Frontend

### Bundle sizes

`pnpm check:bundle` in the cold full run (the warm run gave the same figures):

| Measurement | Size (gzip) | Limit |
|---|---|---|
| Entry and its static imports | 175.6 KB | 180 KB (D44) |
| First load of a student's Home | 190.3 KB | 250 KB (NFR-2.4) |
| First load of a tutor's Today | 190.5 KB | 400 KB |

The entry is 4.4 KB under its limit. All 19 page modules load on demand.

**Verdict:** passed.

### Reconnect and panel cycles

- **Method:** Playwright in Chromium against one end-to-end deployment. A tutor with 31 unread notifications had Today open, with `page.clock` installed.
  - **Reconnect cycle:** `POST /__test__/realtime/close` with `1012`, the page clock run forward by 31 s, then a wait for the refetch of `/api/v1/join-requests` and for one socket in the presence route.
  - **Panel cycle:** a click on the bell, a wait until the panel's heading has focus and 10 rows are shown, then Escape and a wait until the panel is hidden.
  - **Sampling:** every 10 cycles, after a forced garbage collection, `Performance.getMetrics` gave the event listeners, DOM nodes and JavaScript heap. The query-cache entry count was read from the application's query client through a minimal React devtools hook installed by `addInitScript`.
- **Machine state:** load average 1.82 before and 2.23 after.

| Phase | Event listeners | DOM nodes | Query-cache entries | JavaScript heap |
|---|---|---|---|---|
| Start | 278 | 2,857 | 4 | 5.06 MB |
| After 10 to 100 reconnects | 278 at every sample | 2,857 at every sample | 4 at every sample | 5.28 → 5.42 MB |
| After 10 to 100 panel cycles | 278 at every sample | 2,877 at every sample | 4 at every sample | 5.96 → 6.47 MB |

- **Notes:**
  - The 20 extra DOM nodes appear with the first opening of the panel and stay constant afterwards.
  - The 100 reconnect cycles took 2.8 s and the 100 panel cycles 5.0 s.
  - The heap grew by about 3.6 KB per reconnect cycle and about 5.7 KB per panel cycle after forced collections. The cause was not established. The growth is small but steady, and a longer run would show whether it levels off.
- **Verdict:** passed for the declared criteria (flat listeners, DOM nodes and query-cache entries).

## Full local check run

- **Method:** one script ran every command of the [development guide](../../docs/development.md), one after another, and timed each with `time.monotonic()`:
  - the documentation checks;
  - Gitleaks over history and over staged changes, and the four OSV-Scanner scans;
  - the build of the pinned linters and the backend checks, with `go test` through `tools/dev-env.sh`;
  - the frontend checks with `check:bundle` and `pnpm e2e` through `tools/dev-env.sh`;
  - the secret guard hook checks and the local settings wrapper check.

  Every command exited 0, and the end-to-end suites passed 659 tests in both runs.
- **Cold and warm:** in the cold run, Go and golangci-lint started from empty caches (`GOCACHE` and `GOLANGCI_LINT_CACHE` set to new empty folders). The pnpm store, `node_modules` and the Vite and TypeScript caches were left as they were. The warm run followed directly.
- **Machine state:**
  - **Cold run:** load average 1.45 before and 23.45 after (the end-to-end suites saturate the machine).
  - **Warm run:** started once the one-minute load average fell below 3. It was 2.91 before, with a 15-minute load average of 5.15 still left over from the cold run, and 15.31 after.

| Command | Cold (s) | Warm (s) |
|---|---|---|
| Documentation checks (install and `check-docs.sh`) | 6.1 | 6.0 |
| Gitleaks over history, over staged changes | 5.9, 0.5 | 0.9, 0.2 |
| OSV-Scanner: tools, backend, frontend, documentation tools | 13.9, 2.0, 1.7, 1.7 | 1.9, 1.4, 1.6, 1.7 |
| Build of the pinned linters | 5.3 | 0.4 |
| `go mod tidy -diff`, `gofmt`, `go vet` | 0.5, 0.0, 4.1 | 0.0, 0.0, 0.3 |
| golangci-lint, govulncheck | 4.3, 2.7 | 1.8, 2.5 |
| `go test -race -count=1 ./...` | 41.8 | 28.4 |
| `sqlc diff` | 14.2 | 0.4 |
| `pnpm install`, `typecheck`, `lint`, `format:check`, `test` | 0.1, 0.1, 9.4, 1.2, 4.0 | 0.1, 0.1, 9.2, 1.1, 3.9 |
| `build`, `check:classes`, `check:bundle`, `api:check`, `audit` | 0.4, 0.6, 0.1, 0.6, 0.5 | 0.3, 0.6, 0.1, 0.5, 0.5 |
| `pnpm e2e` (659 tests) | 111.5 | 108.8 |
| Secret guard hook checks, local settings wrapper check | 3.8, 0.6 | 4.0, 0.7 |
| **Total** | **237.9** | **177.5** |

- **Slowest Go packages in the cold run:** `internal/api` 25.2 s, `cmd/internal/startup` 21.6 s and `internal/worker` 14.3 s.
- **Where the cold time goes:** cold caches add about 60 s, mostly in compiling the Go tools and tests: the OSV-Scanner and Gitleaks tools, `sqlc` and the race-enabled test binaries.
- **Against the targets:**
  - **Cold:** 237.9 s is 58 s over the 180-second threshold. It is far above the 161.8 s consolidated baseline, which was a warm run.
  - **Warm:** 177.5 s is 0.5 s under the 178-second slice target and 2.5 s under the threshold, but 16 s above the consolidated baseline. The growth is in `pnpm e2e`: 108.8 s now against 78.8 s in the baseline, for 659 tests instead of 559.
  - **Slice 5 target:** D46 of the [slice 5 plan](../plans/slice-5.md) asks for at most 149 s at the slice 4 closing commit; neither run meets it.
  - **NFR-6.2:** the 10-minute budget is not at risk.
- **Slice 3 known limitation:** this replaces the slice 3 known-limitations row on the consolidated run, which was measured once on a loaded machine, with idle-machine figures. The warm run is close to the threshold and the cold run is above it.
- **Verdict:**
  - failed for the cold run against the 180-second threshold and the 178-second target;
  - passed for the warm run against both, by a small margin;
  - failed for both runs against the 149-second target of slice 5 D46.

## Limitations

- **Single-host network:** the gateway clients and the server shared one macOS host and its loopback network stack. The reconnect storm's dial and listener timeouts may not occur when clients come from many hosts.
- **Heap figures:** the API's per-socket heap figure includes the harness's in-memory log records.
- **Lock-contention baseline:** no route that writes is free of signals, so the contention comparison is one signal against two.
- **Deletion dataset:** the membership count of the slice 3 dataset is not recorded. This dataset has 10,000 memberships, which takes 7 ms to delete and does not affect the comparison.
- **Cold run:** only the Go and golangci-lint caches were emptied; the frontend caches stayed warm.
