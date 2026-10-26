# Local check budget after the frontend realtime client

Measured on 2026-10-26 on the branch of the frontend realtime client (F1 of the [slice 4 plan](../plans/slice-4.md)). Section 9 of the plan asks for a record after every change that adds tests. The comparison point is the [record after route-level code splitting](check-budget-2026-10-26-route-splitting.md), which measured 179.8 seconds, and 187.0 seconds on its final tree.

## What the change adds

- **Frontend unit and component tests:** 27 Vitest tests, which run in milliseconds:
  - seven for the realtime client: refresh on `4401`, in-band tokens, backoff and refetch on `ready`, the reset of the backoff after a pong, the 60-second cap after `4429`, a missing pong and the session epoch;
  - fifteen realtime contract scenarios run against the fake gateway;
  - three for the session (the refresh's reasons, the `start` epoch fix, a socket refusal during the tab's own sign-out), one for the query client that lets a running fetch finish, and two for the end-to-end harness's rule on cancelled requests.
- **End-to-end tests:** 57 more runs in total, 619 against 562:
  - fifteen realtime contract scenarios against the real gateway, in Chromium only and without a browser;
  - fourteen browser tests in each of the three engines: six realtime tests (the socket address, reconnects after `4429` and `1012`, sign-out everywhere, live join requests, a tutor removed from My tutors) and eight live-update focus tests.
- **Changed tests:** nine stale-answer tests now hold the account's 20 socket slots through the `socketSlots` fixture before their page loads, which opens 20 authenticated sockets per test. Three tests were adjusted to the live behaviour:
  - the blocked account is signed out live;
  - the expired-token test advances the clock by 15 minutes 10 seconds;
  - Boris's Home shows Maria without a reload.

## Method and its limits

- **Machine:** the same development machine as the comparison records, with warm caches.
- **Load:** the machine was not idle. Other sessions had loaded it heavily just before: the 15-minute load average was 47.6 when the run started and 40.6 at its end, and the one-minute load average was 8.4 at the start and 15.4 at the end.
- **Discarded runs:** earlier attempts on this tree under a load average of 20 to 98 failed on timeouts of `api migrate`, PostgreSQL's `too many clients` and font downloads in unrelated tests, so they were not used.
- **Commands:** one script ran the commands of the full local check run one after another and timed each one, as in the comparison records.
- **Result of the measured run:** every command exited 0.

## Results

| Measurement | Time (s) |
|---|---|
| Full run, wall time | 201.8 |
| `go test -race -count=1 ./...`, wall time | 45.6 |
| `pnpm e2e`, wall time, 619 tests | 113.3 |
| Documentation checks | 6.9 |
| Four OSV-Scanner scans and both Gitleaks scans | 8.1 |
| Frontend checks before `pnpm e2e` | 17.9 |
| Backend checks other than `go test` | 4.5 |
| Secret guard hook and local settings wrapper checks | 4.5 |

## Bundle sizes

Gzip sizes from `pnpm check:bundle`:

- the JavaScript of the entry and its static imports is 171.2 KB, against the 180 KB limit of D44;
- the realtime client, about 2.5 KB gzip, loads on demand after the first sign-in, outside the first load;
- a student's Home loads 185.4 KB first, against 250 KB;
- a tutor's Today loads 185.7 KB first, against 400 KB.

## Against the thresholds

- **The full run is above the threshold.** 201.8 seconds is 21.8 seconds above the 180-second consolidation threshold and 23.8 seconds above the plan's slice target of 178 seconds.
- **The end-to-end suites grew in proportion.** They took 113.3 seconds for 619 tests, against 94 to 98 seconds for 562 tests in the comparison record. That is about 15% more time for 10% more tests, on a machine that was busier during this run.
- **Not decided here.** As in the route-splitting record, a run on a loaded machine does not decide the consolidation. The coordinator decides whether to consolidate before F2 adds its tests, or whether the slice review's cold run on an idle machine decides.
- **Candidates for consolidation:**
  - the per-test seeding of a tutor in the fifteen contract scenarios;
  - the three join-request focus tests, which seed three, two and one requesters.
- **The 10-minute budget of NFR-6.2 is not at risk.**
