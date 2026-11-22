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
