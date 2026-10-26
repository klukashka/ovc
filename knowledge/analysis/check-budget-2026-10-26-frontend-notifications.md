# Local check budget after the frontend notifications

Measured on 2026-10-26 on the branch of the frontend notifications (F2 of the [slice 4 plan](../plans/slice-4.md)). Section 9 of the plan asks for a record after every change that adds tests. The comparison point is the [record after the frontend realtime client](check-budget-2026-10-26-realtime-client.md), which measured 201.8 seconds. F3 has not merged, so this is not the final record of the slice; the final one, `check-budget-<date>-realtime.md`, comes from F3.

## What the change adds

- **Frontend component tests:** two Vitest tests of the bell in the signed-in frame (a click outside the panel returns focus to the bell; below 1024 px the top bar's bell opens the panel on its heading and `Esc` returns focus), 394 tests in all against 392; they run in milliseconds.
- **End-to-end tests:** 27 more runs in total, 646 against 619, nine browser tests in each of the three engines:
  - seven named by the plan's F2 criteria: the panel's latest 10, the panel's focus and `Esc`, opening a row, Mark all as read with a row that arrived meanwhile, the polite announcement and the badge in another tab, a label with markup, and the unverified student without a bell;
  - two more that keep each test in three phases: Show all notifications opening the page on its heading, and Show more moving focus to the first new row. Both reuse the 31 notifications seeded by the panel test in a serial group, so the seed runs once per engine.
- **Extended tests:** `pilot-1 › step 5`, `pilot-2 › steps 1–3`, `pilot-4 › step 1` and `pilot-4 › step 7` gained their notification assertions in place; `pilot-2` no longer reloads Maria's Today, because the request now appears live. Two smoke tests look for the Notifications item as a button, since it opens the panel.

## Method and its limits

- **Machine:** the same development machine as the comparison records, with warm caches.
- **Load:** the machine was not idle. Other sessions ran meanwhile: the one-minute load average was 13.6 at the start and 20.1 at the end, and the 15-minute load average was 12.5 and 13.4.
- **Discarded runs:** one earlier run on this tree started while another worktree's end-to-end run held the per-user run lock, so its end-to-end step was refused at once, and its Go tests failed on PostgreSQL's `too many clients` while other sessions used the shared server. It was not used.
- **Commands:** one script ran the commands of the full local check run one after another and timed each one, as in the comparison records.
- **Result of the measured run:** every command exited 0.

## Results

| Measurement | Time (s) |
|---|---|
| Full run, wall time | 237.4 |
| `go test -race -count=1 ./...`, wall time | 45.7 |
| `pnpm e2e`, wall time, 646 tests | 146.1 |
| Documentation checks | 6.8 |
| Four OSV-Scanner scans and both Gitleaks scans | 8.6 |
| Frontend checks before `pnpm e2e` | 18.3 |
| Backend checks other than `go test` | 4.8 |
| Secret guard hook and local settings wrapper checks | 6.6 |

## Bundle sizes

Gzip sizes from `pnpm check:bundle`:

- the JavaScript of the entry and its static imports is 174.6 KB, against the 180 KB limit of D44 and 171.2 KB before this change; the bell, its unread-count query, the arrival counter and the announcer stay in the first load, about 3.4 KB;
- the panel, the rows and the Notifications page load on demand, about 2.5 KB together;
- a student's Home loads 189.2 KB first, against 250 KB;
- a tutor's Today loads 189.4 KB first, against 400 KB.

## Against the thresholds

- **The full run is above the threshold.** 237.4 seconds is 57.4 seconds above the 180-second consolidation threshold and 59.4 seconds above the plan's slice target of 178 seconds.
- **Most of the growth is the end-to-end step on a busier machine.** It took 146.1 seconds for 646 tests, against 113.3 seconds for 619 tests in the comparison record: 4% more tests and 29% more time, while the load average during the run was about the same as or higher than during the comparison run. The Go tests took 45.7 seconds, as in the comparison record; this change adds none.
- **Not decided here.** As in the two previous records, a run on a loaded machine does not decide the consolidation. The coordinator decides whether to consolidate before the slice closes, or whether the slice review's cold run on an idle machine decides. The test-database template and shared seeded actors of the slice 5 plan's first change target this budget.
- **Candidates for consolidation in this change's tests:** the 31 requesters seeded for the panel tests (each registered, confirmed and turned into a visible request), and the per-test tutor of the six other notification tests, which could share one seeded tutor per file where they do not mutate it.
- **The 10-minute budget of NFR-6.2 is not at risk.**
