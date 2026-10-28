# Check budget: merge gates of slice 5

Recorded on 2026-10-29 from the merge gate's attempt log (`merge-gate.log`) and its command log folders (`logs.*`) in the per-user `ovc-e2e` cache folder, reading only their result and timing lines, as one record for the slice 5 changes whose gate figure no check-budget record gives. These are not budget measurements: every gate below ran with warm Go caches on a machine where other agent sessions were running their own checks, while D46's figures are cold gates on an idle machine ([slice 5 plan](../plans/slice-5.md), [critical path record](check-budget-2026-10-28-gate-critical-path.md)). The cold idle-machine run of the slice review is still pending ([slice 5 review](../reviews/slice-5-review.md#measurements)).

## Method

The attempt log holds one line per gate with its end time, base commit, tested tree, check list and result; the tested tree names the change, because `finish` proved that `main^{tree}` equals it after the merge. A gate's command log folder is created when its checks start, so a gate's wall time is approximated as the time from the folder's creation to the gate's line in the attempt log. This is about one second under the wall time the gate itself reports: the method gives 172 s for F4, whose merge report gave 173.3 s, and 229 s for F1, whose merge report gave 230 s, and 173 s for F3, whose record gives 174.6 s. The end-to-end figures are Playwright's own summary line in the log of the `e2e` entry, rounded by Playwright to a tenth of a minute. Lane wall times are printed only in the merge report, not in the logs, so this record gives them only where a merge report was recorded.

## Slice 5 changes

| Change | Commit | Tested tree | Gate wall time | `pnpm e2e` | Tests |
|---|---|---|---|---|---|
| S2 `feat/schedule-core` | `2a31009` | `68d6a24` | about 194 s | 2.0 min | 669 |
| S6 `feat/lessons-on-tutoring-routes` | `297810e` | `331a978` | 186.4 s ([critical path record](check-budget-2026-10-28-gate-critical-path.md)) | 2.0 min | 669 |
| S3 `feat/lesson-jobs-and-optional-emails` | `c1eba50` | `53b73c4` | 210.4 s ([critical path record](check-budget-2026-10-28-gate-critical-path.md)) | 2.3 min | 669 |
| S4 `feat/lesson-edits-and-cancellation` | `62eadc7` | `d37ff48` | about 155 s | 1.6 min | 669 |
| F1 `feat/frontend-schedule` | `9ac434a` | `3184b1a` | 230 s (merge report) | 174 s (2.9 min) | 743 |
| S5 `feat/lesson-effects-of-tutoring` | `084b9b6` | `d3915ba` | about 148 s (third attempt) | 1.7 min | 743 |
| F2 `feat/frontend-lesson-page` | `eb254ac` | `44e9cce` | about 154 s | 1.8 min | 788 |
| S7 `feat/lesson-audience-ids` | `41dda0c` | `539e57f` | about 172 s | 2.0 min | 788 |
| F3 `feat/frontend-lessons-on-tutoring-pages` | `09bd1da` | `72b8f9e` | 174.6 s ([its record](check-budget-2026-10-28-lessons-on-tutoring-pages.md#merge-gate): lanes A 45.5 s, B 39.6 s, C 47.6 s, end-to-end 126.0 s) | 2.1 min | 839 |
| F4 `feat/frontend-end-series-and-create-new` | `afd6d48` | `0782555` | 173.3 s (merge report: lanes A 48.4 s, B 39.9 s, C 49.8 s, end-to-end 122.4 s) | 2.0 min | 881 |

S5's first two attempts on the same tree failed in the lanes before the end-to-end suites, after about 70 s and 101 s. F3 and F4 are listed for completeness; their own records hold their gate figures. The F1 gate's end-to-end step, 174 s for 743 tests, is the longest of the slice; the gates after it ran the suites in 1.7 to 2.1 minutes with more tests, so F1's figure reflects the machine's load at the time rather than the suites alone.

## Follow-up changes of slice 5

| Change | Commit | Tested tree | Gate wall time | `pnpm e2e` | Tests |
|---|---|---|---|---|---|
| Socket wait in the teaching settings and group tests | `45cab1f` | `d61e239` | about 171 s | 1.9 min | 669 |
| Signed-in frame tests | `953f554` | `e6940a9` | about 169 s | 1.9 min | 788 |
| Settled Today queries before a sidebar reload | `db076de` | `fbc5780` | about 158 s | 1.9 min | 788 |
| By-day week tests on one week | `f8d6717` | `241abf6` | about 192 s | 2.3 min | 881 |
| Lesson jobs hardening | `84d26db` | `521c24a` | about 205 s | 2.5 min | 881 |
| End-to-end run lock held by the gate | `9935cb6` | `c4e2b5d` | about 252 s | 2.4 min | 881 |
| Current-password limit | `3411ae4` | `b03c9d0` | about 187 s (second attempt) | 2.2 min | 881 |
| Lesson occurrence date | `1f22c3b` | `70b3f6a` | about 187 s | 2.1 min | 881 |
| Lesson effects follow-ups | `4b246f6` | `b8f0fc8` | about 197 s | 2.3 min | 881 |

## Against the budget

The gates since the end of slice 5's planned changes took 187 to 252 s, every one above D46's 165 s target and the 180 s consolidation threshold, also allowing for the warm caches and the loaded machine; F4's 173.3 s was the last gate under the threshold. The end-to-end step is the critical path: it runs alone after the lanes, and the suites grew from 669 to 881 tests during the slice. The threshold being exceeded is the reason for the approved [end-to-end shortening plan](../plans/e2e-shortening.md), which measures three cold gates on an idle machine before and after its change and states them in its own record.
