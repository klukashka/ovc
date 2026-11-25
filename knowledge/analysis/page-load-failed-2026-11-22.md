# The page-load failure family of the end-to-end tests: cause and runs

Recorded on 2026-11-22 for change X5 of the [slice 10 plan](../plans/slice-10.md), `fix/page-load-failed` (D48), against `main` at `32429b4` ("Build the phone layout of Messages below 768 px"). It gives the family D36's status: reproduced and fixed by a named change (on `main` 8 of 240 runs failed, with the fix 0 of 240).

## The family

End-to-end tests whose body passed failed in the browser diagnostics fixture with an unexpected `console_error` classified `unknown`, which the redacted diagnostics attachment names as `page_load_failed TypeError` from the main `index-*.js` bundle:

- `[firefox-3] e2e/lesson-room.spec.ts:473` ("signed out, the lesson address asks to sign in and shows nothing private") on the page `/`, in a measurement run of 10.1 and in the merge gate of `chore/entry-budget-phone-frame` on 2026-11-21;
- `[firefox-2] e2e/account-edge-cases.spec.ts:152` ("signing out everywhere ends the sessions of other browsers") in the merge gate of the verification-policy follow-up on 2026-11-18;
- `[webkit-1] e2e/desktop-layout.spec.ts:382` (the public pages group's 360 px sweep) on the page `/invite`, in the merge gate of G2 on 2026-11-22.

`[firefox-4] e2e/reclaim.spec.ts:144` (at `main`'s `32429b4` the same test starts at line 195, "after the choice Continue leads Home to the kept tutor and the choice page has nothing left"), which failed on 2026-11-20 with two unexpected `console_error` records and one `console_warning`, is the same mechanism with the feature-section text loader: its records name `language_load_failed TypeError` and a Firefox "Loading failed for the module" warning on `/home` (reproduced below).

## Cause

- Each route's page module loads on demand. While it loads, `frontend/src/routes/LazyPage.tsx` already renders the route's `Page` with its heading and the delayed loading state, `DelayedLoading`, which carries `role="status"` and `aria-busy="true"` ([pages](../design/web/pages.md), shared behavior). Feature-section texts (`frontend/src/lib/i18n/featureTexts.ts`) render the same loading state in the panels that wait for them.
- The tests took the heading, or other content visible before a panel's texts arrived, for a loaded page: the shared `pageSettled` helper of `frontend/e2e/accounts.ts` waited only for `main h1`; the 360 px sweeps of `frontend/e2e/desktop-layout.spec.ts` waited for `main h1` (an entry without its own measurements, such as `/invite`, navigated on at once), which even the session-pending status page shows before the route renders; the signed-out lesson test waited for the "Вход" heading; the reclaim test waited for Home's tutor list. Each then called `page.goto` or `page.reload`.
- In Firefox and WebKit the old document's pending module request was cancelled once the new navigation started, while the old document still ran script (observed in the deterministic diagnostic below), so the pending dynamic import rejected with a `TypeError` and the app logged `page_load_failed` (or `language_load_failed` for a text section). Chromium did not reject it in any run, consistent with cancelling only when the new document commits.
- The app's log is right for a real chunk failure, and nothing in the app is wrong: the defect is test synchronization.

## Fix

`pageSettled` waits for the route's `Page` heading (`main article h1`, so the session-pending status page no longer counts) and then, through the new `pageLoaded`, for no `main [aria-busy="true"]`. The sweeps wait for `main article h1` instead of `main h1` and call `pageLoaded` after their readiness wait; the signed-out lesson test calls `pageSettled` and the reclaim test `pageLoaded` before they navigate. The other callers of `pageSettled` (`session.spec.ts`, `signup.spec.ts`, `account-edge-cases.spec.ts` and `realtime.ts`) gain the wait without change. A new test in `frontend/e2e/session.spec.ts` holds the sign-in page module with a `page.route` handler that only holds the request and continues it unchanged, and proves that `pageSettled` resolves only after the module is released. No expected-error list, retry or timeout was added, and the app is unchanged.

A scan of `frontend/e2e/` for a `page.goto` or `page.reload` whose preceding statements wait only on a heading found no further site. Two kinds of site are not covered by that scan and stay with 10.5 H3: waits through helpers that end in a heading, and waits on content of a page whose panels load feature-section texts (`useFeatureTexts`: Home, Library, Group, Student card, Issued, the lesson's issues, Assignment, Homework and Reclaim) followed by a navigation, the reclaim test's pattern.

## Runs

Every run went through the main checkout's `tools/dev-env.sh` with `--trace=off --retries=0`, on AC power, with the frontend built from `32429b4` (the app is the same on both trees). A series is the four tests in `chromium`, the four `firefox-*` and the four `webkit-*` projects with `--repeat-each=10`, 120 runs. "Main" is `main`'s copies of `accounts.ts`, `desktop-layout.spec.ts`, `lesson-room.spec.ts` and `session.spec.ts` put back in the change's tree (the reclaim wait was added after the main series, from their records); the loaded series ran beside 12 `yes` processes, stopped by their recorded process identifiers when the series ended.

| Run | Start (UTC), load at start → end | Result | Records |
|---|---|---|---|
| Deterministic diagnostic, temporary and not committed: hold the sign-in page module, then `page.goto('/signup')` | 2026-11-22, load 1.53 2.95 6.19 | `chromium` passed; `firefox-3` and `webkit-3` failed | one `console_error` `page_load_failed TypeError` from `assets/index-ysLYZ-1U.js` on `/` in each engine, the same text and bundle as the G2 gate's record |
| Negative control: the new session test against the old `pageSettled` | 2026-11-22, load 1.58 2.87 6.08 | failed in all three engines; with the fix it passed in all three | — |
| Main, idle | 10:23:26, 1.70 2.62 5.71 → 14.60 5.96 6.69 | 2 failed, 118 passed | `[webkit-3] lesson-room.spec.ts:473`: `page_load_failed TypeError` on `/`; `[firefox-4] reclaim.spec.ts:195`: two `language_load_failed TypeError` and two module load warnings on `/home` |
| Main, loaded | 10:25:37, 6.55 5.16 6.35 → 50.54 26.83 15.17 | 6 failed, 114 passed | `lesson-room.spec.ts:473`, four times in `firefox-3` and twice in `webkit-3`: `page_load_failed TypeError` on `/` |
| Fix, idle (before the sweeps' `main article h1`, added after these series) | 10:37:50, 1.32 5.79 8.89 → 19.15 9.18 9.85 | 120 passed | none |
| Fix, loaded (before the sweeps' `main article h1`) | 10:39:19, 15.25 8.81 9.71 → 41.10 26.20 17.00 | 120 passed | none |

`desktop-layout.spec.ts:382` and `account-edge-cases.spec.ts:152` did not fail in the two main series (20 runs of each per engine), so for them the status is "not reproduced in 60 runs each at the loads above" before the fix; both share the mechanism, shown by the deterministic diagnostic, and their waits are fixed with the others.

## Addendum, 2026-11-25: feature-section texts with no loading state

Recorded for the follow-up change `fix/feature-text-load-waits` after H2's merge gate on 2026-11-24, where `[webkit-1] e2e/ending.spec.ts:341` (pilot-4 step 7) failed with one `console_error` `language_load_failed TypeError` on `/students/<card>` (row of 2026-11-24 in [the slice 10 known limitations](../reviews/slice-10-known-limitations.md)). It resolves the second site kind that X5's scan left to 10.5 H3: waits on a page whose panels load feature-section texts, followed by a navigation.

### The gap

X5's `pageLoaded` waited for no `main [aria-busy="true"]`. Seven callers of `useFeatureTexts` (`frontend/src/lib/i18n/featureTexts.ts`) rendered nothing busy while their section loaded, so the wait passed while the import was still in flight:

| Caller | Section | Shown while loading, before this change |
|---|---|---|
| `features/notifications/NotificationRows.tsx` (the bell panel, outside `main`) | `notificationEvents` | placeholder row text |
| `features/students/StudentCardPage.tsx` | `homework` | the card without its homework tab content |
| `features/groups/GroupPage.tsx` | `homework` | the group without its homework content |
| `features/library/LibraryPage.tsx` | `homework` | the page without its homework tabs and issue action |
| `features/builder/BuilderPage.tsx` | `homework` | the builder without its issue section |
| `features/homework/LessonIssues.tsx` | `homework` | nothing |
| `features/assignment/LessonWork.tsx` | `assignment` | nothing |

Every other caller, `MyDataPage.tsx` added by E2 included, renders `DelayedLoading` inside `main` and was already covered.

### Fix

- `useFeatureTexts` returns `pending` (neither loaded nor failed).
- Each caller in the table puts `aria-busy` on the container its late content belongs to, with no visible change: the notification list or table, the card's section tabs (`RouteTabs busy`), the group's columns (`PageColumns busy`), Library's main grid and the builder's action row. The lesson's issues and work sections render `DelayedLoading` while pending.
- `pageLoaded` waits for no `[aria-busy="true"]` in the whole document, so the header's bell panel counts.
- Step 7 of `ending.spec.ts` calls `pageLoaded` before `page.reload()`.
- Two tests in `frontend/e2e/session.spec.ts` hold a feature text chunk with a `page.route` handler that only holds the request and continues it unchanged. One holds it on the student card's section tabs, the other in the open bell panel. Each proves that the region is busy and that `pageLoaded` resolves only after the chunk is released.
- Component tests in `NotificationRows.test.tsx` assert the list and table are busy until their texts load, and not busy after a failure.
- No expected-error list, retry or timeout was added, and `reportLanguageLoadFailure` is unchanged.
- The marks cover each page's loaded view. The student card's and the group's not-found, gone and load-failure views, the Library's not-found view and the builder's view when one of its two sections failed while the other still loads start the same loads with nothing busy. This is recorded in [the slice 10 known limitations](../reviews/slice-10-known-limitations.md) and carried to 10.5 H3.

### Sweep

A scan of `frontend/e2e/` for a `page.goto` or `page.reload` that follows a page from the table, or an opened bell, without a `pageLoaded` or `pageSettled` wait found eight sites. Each gained `pageLoaded` before its navigation:

- `groups.spec.ts`, two sites;
- `invitations.spec.ts`, two sites;
- `join-requests.spec.ts`, one site;
- `students.spec.ts`, three sites.

It also flagged four sites that stay unchanged: `groups.spec.ts:886`, `homework.spec.ts:188` and `:213`, and `lesson-room.spec.ts:91`. Each is the first navigation of a helper function, not a navigation away from a loaded page. The scan was rerun on `main` at `4fe2093` (after H2, M2 and E2) and found no new site.

### Runs

Every run went through the main checkout's `tools/dev-env.sh` with `--trace=off --retries=0` on AC power, on 2026-11-25 (times in UTC), against `main` at `0d37d62` with the change's working tree.

The deterministic diagnostic was a temporary spec, not committed. It held every `texts.(en|ru)-*.js` request until the test released it, with a handler that only holds and then continues the request.

| Run | Start (UTC), load at start → end | Result | Records |
|---|---|---|---|
| Diagnostic: hold the card's feature texts, reload the student card, release | 07:52, 2.49 | `chromium` passed; `firefox-2` and `webkit-2` failed | one `console_error` `language_load_failed TypeError` from `assets/i18n-*.js` in each engine, the H2 gate's text and bundle; Firefox adds two module load warnings for the `IssueAssignment` chunk, which holds the `homework` loaders |
| Diagnostic: open the bell on the card, hold its texts, press Escape, reload, release | 07:52, 2.49 | passed in `chromium` and `webkit-2`; failed in `firefox-2` | two module load warnings for `NotificationRows`, no `language_load_failed`: the closed panel is unmounted and reports nothing, so this is not the gate's trigger |
| New session tests with the fix | 07:54, 5.23 | 6 passed in three engines | — |
| Negative control: the new tests with `main`'s `pageLoaded` | 07:54, 4.15 | the bell panel test failed in all three engines; the card test passed, its tabs being inside `main` | — |
| Negative control: the new tests with `main`'s application code | 07:55, 3.30 | 6 failed (no busy mark) | — |

Each series ran all of `ending.spec.ts` (its pilot-4 steps are serial, so step 7 needs its seed and earlier steps), 15 tests in `chromium`, `firefox-*` and `webkit-*` with `--repeat-each=10`: 450 runs, 30 of step 7. "Main" is `main`'s copies of every file the change touches, put back in the change's tree. The loaded series ran beside 12 `yes` processes, stopped by their recorded process identifiers when the series ended.

| Series | Start (UTC), load at start → end | Result | Step 7 |
|---|---|---|---|
| Fix, idle | 07:56:35, 2.29 3.02 2.47 → 24.22 11.22 5.80 | 450 passed | 30 passed |
| Main, idle | 07:58:49, 24.22 11.22 5.80 → 20.87 15.13 8.20 | 450 passed | 30 passed |
| Main, loaded | 08:01:05, 20.87 15.13 8.20 → 59.66 42.54 23.24 | 450 passed | 30 passed |
| Fix, loaded | 08:06:50, 59.66 42.54 23.24 → 42.32 41.74 29.25 | 450 passed | 30 passed |

The gate failure was not reproduced on `main` in 60 runs of step 7 at the loads above. The mechanism is shown by the deterministic diagnostic, which reproduces the gate's record exactly. The fix series had no failure.
