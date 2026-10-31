# Chat presence notification-rate check

Checked on 2026-10-31 in the isolated presence completion worktree, based on main `e95d272` with the uncommitted S5 completion diff. This is the D61 quick check, not the slice-wide NFR-2.2 load test.

## Workload and environment

200 real accounts: 20 tutors and nine students per tutor (180 students), correcting the original inconsistent account-count description. Real local API gateways, PostgreSQL listener, database, HTTP message sends and WebSocket clients; a fake password hasher belongs to the existing integration fixture. One gateway and then two independently identified gateway instances shared the database and pool inside one Go integration-test process. Each gateway used its own real PostgreSQL listener; the second HTTP test server served the real WebSocket gateway, while message sends used the primary full API. This is not a measurement of two independent operating-system API processes. Every account repeatedly disconnected, waited for its local release, reconnected and sent typing frames. A tutor sent a real message at least three seconds apart. The shared business clock stayed fixed; scheduling and latency used system time. The two-API run distributed accounts between APIs; separate deterministic tests cover an account connected to both.

The machine has 12 cores. No other test run, gate, build or measurement was started during either workload. Codex listed only the coordinating chat as active; the observed Claude Code process used another project and was idle in the process snapshot. These are snapshots, not a cross-application exclusion guarantee. Tool versions before the separate tools update: Go 1.27.3, Node 26.12.1 and pnpm 12.14.0.

```bash
go test -tags presence_measurement ./internal/api -run '^TestPresenceNotificationRateCheck$' -count=1 -v
```

Executed from backend through the original checkout's sanctioned dev-env wrapper. Exit 0. The compiler build tag keeps the one-minute workload out of the ordinary checks. An initial attempt failed to compile due to an unused import, corrected before a workload ran.

## Results

```text
=== RUN   TestPresenceNotificationRateCheck
=== RUN   TestPresenceNotificationRateCheck/1-instances
    presence_measurement_test.go:89: accounts=200 instances=1 elapsed_s=65.207 reconnects=5600 notifications_per_s=120.830 send_http_p50_ms=4.906 send_http_p95_ms=7.430 pending_accounts=0
=== RUN   TestPresenceNotificationRateCheck/2-instances
    presence_measurement_test.go:89: accounts=200 instances=2 elapsed_s=60.027 reconnects=4600 notifications_per_s=108.101 send_http_p50_ms=5.954 send_http_p95_ms=8.952 pending_accounts=0
--- PASS: TestPresenceNotificationRateCheck (133.16s)
    --- PASS: TestPresenceNotificationRateCheck/1-instances (69.58s)
    --- PASS: TestPresenceNotificationRateCheck/2-instances (63.58s)
PASS
ok      ovc/backend/internal/api    133.875s

```

## Interpretation and limits

The rate is the gateway listener delivery-record count divided by elapsed time and API listener count; it estimates unique database notifications, including message signals. Send latency is complete HTTP send latency, including authentication, commit and response work: a conservative upper bound on commit contribution, not an isolated COMMIT measurement. The p50/p95 values summarize observed sends, not sustained message throughput. End-of-cycle pending-account samples were zero; peak writer queue was not sampled independently. A deterministic row-lock test proves exactly four concurrent presence writers, and capacity tests prove refusal and retention of accepted releases under database failure.

No reconnect or typing assertion failed, and send upper bounds showed no notification-lock contention at this workload. The slice review still owns the 2,000-socket, group-fanout, sign-out latency, listener queue and browser/refetch measurements.
