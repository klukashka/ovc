# Check budget: the frontend Schedule pages

Recorded on 2026-10-28 on the branch `feat/frontend-schedule` (slice 5's F1, based on `main` at `b00d5da`) from the change checks run while the change was implemented. These are not budget measurements: the machine was not idle (other agent sessions were running their own checks and end-to-end runs, which held the end-to-end run lock in turn), and the Go caches were warm. The coordinator's cold merge-gate runs on an idle machine remain the measurement of the check budget ([merge-gate record](check-budget-2026-10-28-gate-critical-path.md)).

## First load

`pnpm check:bundle` with its limits unchanged:

| Measure | `main` at `b00d5da` | F1 | Limit |
|---|---|---|---|
| Entry and its static imports, with the largest language chunk | 169.6 KB (160.2 + 9.4) | 171.0 KB (160.8 + 10.2) | 180 KB |
| A student's Home | 184.3 KB | 186.3 KB | 250 KB |
| A tutor's Today | 184.5 KB | 186.5 KB | 400 KB |

The Schedule page, its grids, the New lesson dialog and the calendar helpers load on demand in their own chunk of about 8.3 KB gzip; the first load grows by the Schedule's interface texts in the language chunks (0.8 KB for Russian) and by the `lesson` case of the live refetch mapping, four icons and the frontend field code `too_many`. The figures are those after the review fixes; the first commit measured the same entry and 0.1 KB less for each page.

## End-to-end suites

| Run | Tests | Wall time | Result |
|---|---|---|---|
| Full `pnpm e2e`, the change's first commit | 708 | 1.9 min | passed |
| Full `pnpm e2e`, the change's first commit with the overlap and paging tests split | 717 | 1.8 min | passed |
| `schedule.spec.ts` and `ending.spec.ts` three times each in three engines, after the review fixes | 279 | 1.0 min | passed |
| Full `pnpm e2e`, after the review fixes | 729 | 1.8 min | passed |

F1 adds 20 Playwright tests per engine in `schedule.spec.ts` (the schedule tests, pilot 1 steps 6 and 7, and pilot 3's seed and steps 3 and 4) and extends the pilot 4 seed; that file ran in about 8 s for one engine and about 16 s for three, so on a loaded machine they add a few seconds to the end-to-end lane.
