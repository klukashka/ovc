# Local check budget after email settings and the unsubscribe page

Measured on 2026-10-26 on the branch of the email settings and unsubscribe page change (F3 of the [slice 4 plan](../plans/slice-4.md)). Section 9 of the plan asks for a record after every change that adds tests. The in-app notifications bell (F2) has not merged yet, so this is not the final record of the slice. The comparison point is the [record after the frontend realtime client](check-budget-2026-10-26-realtime-client.md), which measured 201.8 seconds.

## What the change adds

- **Frontend component tests:** five Vitest tests, which run in milliseconds:
  - three for the unsubscribe page: the result taking focus with the token kept out of the query cache, a refused confirmation keeping its button, and a link that became invalid before confirming;
  - two for Settings → Notifications: a failed save keeping focus on its switch, and Retry moving focus back to it.
- **A transport contract scenario:** an unsubscribe link previews its group and turns it off only on `POST`, and a tampered link is `token_invalid`. It runs against the fake in Vitest and against the real backend in Chromium only, without a browser.
- **End-to-end tests:** four browser tests in each of the three engines, the four that the plan names for F3.
- **In total:** 13 more end-to-end runs, 632 against 619.

## Method and its limits

- **Machine:** the same development machine as the comparison records, with warm caches.
- **Load:** the machine was not idle. Other sessions were running checks of their own. The one-minute load average was 3.4 at the start and 22.8 at the end. The 15-minute load average was 12.0 at the start and 14.2 at the end.
- **Commands:** one script ran the commands of the full local check run one after another and timed each one, as in the comparison records.
- **Result of the measured run:**
  - Every command exited 0 except `go test`.
  - `go test` failed on PostgreSQL `53300` (too many connections) while creating per-test databases. Another session's checks were using the same Compose PostgreSQL at the time.
  - This change touches no backend code. Run again alone right afterwards, `go test -race -count=1 ./...` exited 0 in 45.0 seconds. The table counts that rerun instead of the failed attempt.

## Results

| Measurement | Time (s) |
|---|---|
| Full run, wall time, with the `go test` rerun in place of the failed attempt | 257.2 |
| Full run as measured, with the failed `go test` attempt | 310.2 |
| `go test -race -count=1 ./...`, wall time, rerun | 45.0 |
| `pnpm e2e`, wall time, 632 tests | 164.3 |
| Documentation checks | 6.5 |
| Four OSV-Scanner scans and both Gitleaks scans | 8.2 |
| Frontend checks before `pnpm e2e` | 20.1 |
| Backend checks other than `go test` | 7.6 |
| Secret guard hook and local settings wrapper checks | 5.0 |

## Bundle sizes

Gzip sizes from `pnpm check:bundle`:

| Measure | Size (KB) | Limit (KB) | Change |
|---|---|---|---|
| JavaScript of the entry and its static imports | 172.2 | 180 (D44) | +1.0 KB, from the new interface strings in both languages, which the entry holds |
| A student's Home, first load | 186.6 | 250 | |
| A tutor's Today, first load | 186.8 | 400 | |

Both new pages load on demand; 18 page modules now do.

## Against the thresholds

- **The full run is above the threshold.** At 257.2 seconds it is 77 seconds above the 180-second consolidation threshold.
- **Most of the increase is machine load, not new tests:**
  - in an end-to-end run of only this change's spec and the transport contract, the 13 new runs took 8.7 seconds of wall time;
  - the end-to-end suites took 164.3 seconds for 632 tests, against 113.3 seconds for 619 tests in the comparison record, and the machine was busier during this run.
- **Not decided here.**
  - As in the two comparison records, a run on a loaded machine does not decide the consolidation.
  - The test-database template change (B0) is meant to speed the suites up and is in progress.
  - The final record of the slice comes from whichever of F2 and F3 merges last. The cold run on an idle machine at the slice review decides whether suites are consolidated.
- **The 10-minute budget of NFR-6.2 is not at risk.**
