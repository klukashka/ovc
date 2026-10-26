# Slice 4 review

The slice review of [roadmap slice 4](../../docs/roadmap.md#slice-4-realtime-and-in-app-notifications), run on 2026-10-26 over everything merged after the commit "Record the slice 3 review and close the slice" (`64c8fec`) up to "Pin Node 26.12.1, the first Node 26 release that bundles a fixed undici" (`08e72cb`), followed by the follow-up changes of [the slice 4 review follow-up plan](../plans/slice-4-review-follow-ups.md).

## Scope and reviews

One fresh independent reviewer per changed layer, against the clean `main` tree ([ADR-0011](../../docs/decisions/0011-tiered-verification.md)):

| Layer | Components | Profiles | Result |
|---|---|---|---|
| Backend | Per-lane worker loops; the PostgreSQL listener and revocation triggers (migration 00007); the WebSocket gateway; in-app notifications (migration 00008) and `entity.changed` producers; email preferences and unsubscribe (migration 00006); the test database template and the listener-test forwarder | Correctness, performance | One major (tutor deletion signalled each relationship with its own round trip), predicted and then measured; two minor findings |
| Frontend | Route splitting and `check:bundle`; the realtime client, live refetch and focus; the notification bell, panel and page; email settings and the unsubscribe page; the end-to-end harness changes | Correctness, browser, performance | No blocker or major; six minor findings |
| Security across the slice | Four trust-boundary families: authentication and revocation of live connections; authorization and scope of frames and notification rows; unsubscribe tokens, the new secret and email; the test control surface, agent configuration and dependencies, each in its own fresh context | Security | No blocker or major; eight minor findings |
| Development tooling and agent configuration | `AGENTS.md`, `.claude/`, `tools/`, the end-to-end harness, `frontend/scripts/`, `.gitleaks.toml`, the check-budget records, `docs/development.md` | Correctness, security | No blocker or major; six minor findings |

The integration boundaries were reviewed from both sides: the WebSocket frames and the notification API by the backend and the frontend reviewers, the listener and the gateway by the backend reviewer, the harness and the test control routes by the tooling and the security reviewers.

Every threat the slice touches was reported as examined, not affected or unverified: T-2, T-5, T-6, T-14, T-15, T-17, T-18, T-19, T-20, T-21, T-22, T-23, T-24, T-27, T-29, T-31, T-32, T-33, T-34, T-36, T-37, T-38, T-39, T-41, T-43, T-44 and T-45. The threat model's amendments pending owner review were compared with the code; the mismatches found are fixed below. T-28 and T-46 changed in the same range only through the decision records on web push and the modular monolith, with no code in this slice.

## Measurements

The [review measurements](../analysis/slice-4-review-measurements-2026-10-26.md) ran one at a time on an otherwise idle machine:

- **Passed:** 2,000 idle sockets on one API instance (about 47 KB of resident memory and 4 goroutines each, 1% of one core); one `entity.changed` to 2,000 sockets at a p95 of 12 ms from commit; listener loss detected in 18 to 38 ms on a dropped connection and 3.2 to 4.8 s on a silent one; notification queue usage of at most 0.003% during bursts; NFR-4.5's high-priority email sent within one free slot behind 1,000 slow ones; notification lock contention, re-check and sweep latency; `GET /notifications` at 5 million rows under 4 ms; flat listeners, DOM nodes and query-cache entries over 100 reconnect and panel cycles.
- **Failed, then fixed:** tutor deletion at 50,000 relationships took 7.9 s against the 0.95 s baseline; after the follow-up it took 1.00 s with 53 statements instead of 30,023.
- **Unverified, then improved, failed its pass mark and accepted:** the mass reconnect after `1012` recovered in 26 to 81 seconds at the review, with no target set; after the heartbeat moved to the listener's own connection and the client waits at least the listener's reconnect delay, 13 of 15 storms recovered in under 15 seconds and two in 23 seconds, both slowed by loopback connect timeouts of 2,000 clients and the server on one machine, so the repeat failed its pass mark of full recovery under 15 seconds on this single host and stays unverified over a real network. The owner accepted it and carried a measurement over a real network into slice 7.
- **The full local check run:** 237.9 seconds cold at the review, 186.7 seconds cold after the follow-ups ([record](../analysis/check-budget-2026-10-26-prebuilt-tools-and-seeding.md)).

## Fixed in follow-up changes

Each change had a standard reviewer and, where it met the security trigger, a security-profile reviewer; a major found by a re-review waited for the owner's decision on another reviewer.

1. **"Send a tutor deletion's relationship signals in one statement per chunk".**
2. **"Keep operator-blocked students' join requests out of un-suppression and the visible limit"**, with the owner's decision that such a request no longer takes a slot of the pending limit.
3. **"Bucket socket limits by IPv6 /64, decode unsubscribe tokens strictly and send heartbeats on the listener's connection"**, and **"Pass end of stream through the listener-test forwarder as a half-close"**, which also states that relay in the `AGENTS.md` exception.
4. **"Fix the frontend findings of the slice 4 review"**: focus in the notifications panel, typing kept through a failed refetch, errored queries refetched after a reconnect, the wall-clock token check, the delay after `1012`, and, with the owner's approval, `beforeunload` kept with a resume after 2 seconds.
5. **"Parse the release-age settings as YAML and tighten the bundle and harness checks"**.
6. **"Record the keep-alive proxy, the request-failure rule and the revoked-session socket close"**.
7. **"Share seeded actors across notification tests and remove clock-bound waits from unit tests"**, then, by the owner's decision, **"Build the scanning tools and sqlc once into bin and check them against tools/go.mod"** and **"Seed confirmed end-to-end accounts and sessions through test control routes"**.

Three flaky tests surfaced while the follow-ups ran (the realtime client's `4401` unit test, a notifications panel test and a session end-to-end test); "Share seeded actors across notification tests and remove clock-bound waits from unit tests" fixed their causes.

## Known limitations

The [slice 4 known limitations](slice-4-known-limitations.md) were re-verified by the layer reviewers and accepted by the owner on 2026-10-26. Two rows closed during the follow-ups: `label_deleted` on an accepted request whose card later ended, which T-34's residual already covers, and the shared keep-alive proxy connections, which T-38 now records. The operator-block cases left by `ovc-admin` were accepted by the owner on 2026-10-26, during the change that added them. The other rows added during the follow-ups (the group page refetch notice without its own component test, the jitter floor proven at one random value, the backoff reset on a resume, the IPv4-mapped test path, the untested heartbeat stall and signal during a heartbeat, the 31 join requests the end-to-end fixture seeds as a number in its code, and the mass reconnect carried into slice 7) were accepted by the owner on 2026-10-27.

## Carried over

| Item | Where it goes |
|---|---|
| The cold full local check run at 186.7 seconds, 6.7 seconds over the slice 4 threshold of 180 seconds | Slice 5's change B0, which must reach its 149-second target before slice 5's S2 ([owner decision of 2026-10-27](../plans/slice-4-review-follow-ups.md#revisions)) |
| The mass reconnect after `1012` over a real network | Slice 7, with its remote test server |
| The frontend heap growth of 4 to 6 KB per reconnect or panel cycle | Slice 8's performance work |
| 4.4 KB left of the 180 KB entry budget at the review | Slice 5's change B1, which loads only the active language |

## Unverified

- Behaviour behind a production reverse proxy and on the pilot deployment, which do not exist yet.
- Live conferencing, which slice 4 does not touch.
- `TCP_USER_TIMEOUT` on Linux, and a real `LISTEN` failure.
- Browser behaviour of downloads, `mailto:` links and slow navigations against the `beforeunload` resume.
