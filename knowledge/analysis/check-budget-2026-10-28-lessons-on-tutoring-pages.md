# Check budget: lessons on the tutoring pages

Recorded on 2026-10-28 on the branch `feat/frontend-lessons-on-tutoring-pages` (slice 5's F3, the last planned change of the slice, based on `main` at `eb254ac`) from the change checks run while the change was implemented. These are not budget measurements: the machine was not idle (the load average was about 23 at the end of the full end-to-end run, with other agent sessions running their own checks), and the caches were warm. The slice plan's final figure, the full local check run with the security scans included against the 178 s target, is the merge gate's run of this change; the coordinator adds it to this record from the merge report. The cold idle-machine run belongs to the slice review ([merge-gate record](check-budget-2026-10-28-gate-critical-path.md)).

## First load

`pnpm check:bundle` with its limits unchanged:

| Measure | F1 (`feat/frontend-schedule`) | F3 | Limit |
|---|---|---|---|
| Entry and its static imports, with the largest language chunk | 171.0 KB (160.8 + 10.2) | 172.8 KB (161.7 + 11.1) | 180 KB |
| A student's Home | 186.3 KB | 187.8 KB | 250 KB |
| A tutor's Today | 186.5 KB | 188.4 KB | 400 KB |

The growth since F1 includes the lesson page change (F2) and this change. This change adds to the first load the texts of the tutoring pages' lessons and of the archive confirmation in the language chunks, the provisional-link check of the route gate and the sidebar (a `GET /tutors` query for an unconfirmed student only), and three more query keys in the `lesson` case of the live refetch mapping. The lesson lists, the "Schedule a lesson" action and the New lesson dialog load with the student card, Students, Groups and group page chunks, on demand.

## End-to-end suites

| Run | Tests | Wall time | Result |
|---|---|---|---|
| `ending`, `groups`, `students`, `schedule` and `notifications` specs in three engines | 281 | 47.5 s | passed |
| Full `pnpm e2e` | 821 | 1.8 min (106 s with the harness start) | passed |
| `students` and `ending` specs in three engines, after the Show more test was added | 105 | 21.3 s | passed |
| After the review fixes: `ending`, `students` and `groups` specs in three engines | 180 | 34.4 s | passed |
| After the review fixes: `schedule` and `notifications` specs in three engines | 116 | 26.4 s | passed |

The change adds 16 Playwright tests per engine (12 in its first commit, 4 after the review): pilot 4 steps 2 and 4 (step 1 is extended in place), pilot 3 step 5, four group tests (the archive confirmation's opening, dismissal and confirmation, and "Schedule a lesson" on the group page), three student card and Students tests, after the review a failed "Show more" in the card's Lessons and three focus moves when a live refetch removes a focused next lesson, the unconfirmed student's Schedule and the lesson change row of the notifications. Each seeds through the API in under a second, so on a loaded machine they add a few seconds to the end-to-end lane, which stays at 1.8 minutes.

## Merge gate

The merge gate's run of this change on 2026-10-28 passed in 174.6 s in total, under the slice plan's 178 s target. Its lanes took 45.5 s (A), 39.6 s (B) and 47.6 s (C), and the end-to-end lane 126.0 s with 839 tests.
