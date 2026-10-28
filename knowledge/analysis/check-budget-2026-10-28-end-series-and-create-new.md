# Check budget: end a series and create a new one

Recorded on 2026-10-28 on the branch `feat/frontend-end-series-and-create-new` (slice 5's F4, based on `main` at `09bd1da`) from the change checks run while the change was implemented. These are not budget measurements: the machine was not idle (the load average was about 19 at the end of the full end-to-end run, with other agent sessions running their own checks), and the caches were warm. The slice plan's final figure, the full local check run with the security scans included against the 178 s target, is the merge gate's run of this change; the coordinator adds it to this record from the merge report. The cold idle-machine run belongs to the slice review ([merge-gate record](check-budget-2026-10-28-gate-critical-path.md)).

## First load

`pnpm check:bundle` with its limits unchanged:

| Measure | F3 (`feat/frontend-lessons-on-tutoring-pages`) | F4 | Limit |
|---|---|---|---|
| Entry and its static imports, with the largest language chunk | 172.8 KB (161.7 + 11.1) | 172.8 KB (161.6 + 11.2) | 180 KB |
| A student's Home | 187.8 KB | 187.7 KB | 250 KB |
| A tutor's Today | 188.4 KB | 188.3 KB | 400 KB |

The change adds to the first load only the texts of the button and its confirmation in the language chunks. The confirmation and the New lesson dialog load with the lesson page's chunk, on demand.

## End-to-end suites

| Run | Tests | Wall time | Result |
|---|---|---|---|
| `lesson` spec in three engines | 57 | 15.9 s | passed |
| Full `pnpm e2e` | 857 | 2.0 min (122 s with the harness start) | passed |

The change adds 6 Playwright tests per engine, 18 in all, to the lesson spec: the confirmation's opening on its dismiss button, the dismissed confirmation's focus return (a new case of the existing focus-return loop), the cancellation of this and the following lessons with the New lesson dialog prefilled for a one-to-one series and for a group series, and the New lesson dialog saved and dismissed after the ending, each closing onto the "Lesson cancelled" notice. Each seeds through the API in under a second, so on a loaded machine they add a few seconds to the end-to-end lane.

## After the review fixes

The fixes add 8 Playwright tests per engine, 24 in all, to the lesson spec (the cross-timezone prefill and its saved instants, a lesson cancelled meanwhile, a refused ending, a one-off lesson and a student's series lesson without the button, a refused new series and its dismissal with Cancel), and 4 frontend unit tests of the timezone conversion of the prefill (`seriesTiming.test.ts`). `pnpm check:bundle` is unchanged within 0.1 KB: entry 172.8 KB (161.7 + 11.2), a student's Home 187.7 KB, a tutor's Today 188.3 KB.

| Run | Tests | Wall time | Result |
|---|---|---|---|
| `lesson` spec in three engines | 81 | 19.7 s | passed |
| Full `pnpm e2e` | 881 | 2.0 min (120 s with the harness start, load average about 19) | passed |

## After the second review

The fixes add 3 frontend unit tests of `localInstant` (a skipped and a repeated local time, east and west of UTC, and an ordinary one) and no Playwright test; the cross-timezone tests now use a zone without daylight saving. `pnpm check:bundle` is unchanged.

| Run | Tests | Wall time | Result |
|---|---|---|---|
| `lesson` spec in three engines | 81 | 19.2 s | passed |
| Full `pnpm e2e` | 881 | 1.9 min (114 s with the harness start, load average about 18) | passed |
