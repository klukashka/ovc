# Local check budget after route-level code splitting

Measured on 2026-10-26 on the branch of the route-level code splitting change (F0 of the [slice 4 plan](../plans/slice-4.md)). Section 9 of the plan asks for a record after every change that adds tests. The comparison point is the [record after the in-app notifications](check-budget-2026-10-25-in-app-notifications.md), which measured 178 seconds.

## What the change adds

- **Frontend component and unit tests:** 16 Vitest tests, which run in milliseconds:
  - seven for the bundle check;
  - five for lazily loaded pages: focus, failure, sibling routes and a page left while it loads;
  - two for the access rule deciding before a page module loads;
  - two for the token holders clearing the address before a page module loads.
- **End-to-end tests:** one smoke test, that the preview server serves no build manifest, which runs in each of the three engines. Three existing tests also record the addresses of lazily loaded chunks.
- **A new check:** `pnpm check:bundle`, which takes 0.1 to 0.2 seconds.

## Method and its limits

- **Machine:** the same development machine as the comparison record, with warm caches. The machine was not idle: the one-minute load average was 7.2 when the measured run started and 17.2 at its end, partly from the run's own parallel end-to-end workers. No reviewer or other check ran at the same time.
- **Commands:** one script ran the commands of the full local check run one after another and timed each one. The commands are those of the comparison record, plus `pnpm check:bundle`.
- **Result of the measured run:** every command exited 0.

## A regression found and removed by measuring

The first version of the change loaded pages through `React.lazy` and `Suspense`.

- **Symptom:** in three warm runs the end-to-end suites took 141 to 145 seconds. Base `main`, measured on the same machine the same night, took 107 seconds for its 559 tests.
- **Cause:** after a Suspense boundary shows its fallback, React waits about 300 ms before it reveals the content. That delay fell on the first visit of every page in every test.
- **Fix:** the change now loads each page module through an explicit loader, which shows the same loading state inside the route's page without the reveal delay.
- **Result:** the suites then took 97 seconds in one run, against 107 seconds for base `main`, and 94 seconds in the measured run.

## The sign-in race of the end-to-end harness

- **Symptom:** on the split build, Firefox sometimes logged a console error for a font load that it had aborted, in a test that signs in through the API.
- **Cause:** `signInThroughTheApi` opened `/` and navigated away before the lazily loaded sign-in page had finished loading its fonts.
- **Fix:** the helper now waits for the page heading and `document.fonts.ready` before signing in. The harness reports every console error as before.
- **Evidence:** three consecutive end-to-end runs on the fixed tree each passed 562 of 562 tests in 96 to 99 seconds, with no font error.

## Results

| Measurement | Time (s) |
|---|---|
| Full run as listed above, wall time | 179.8 |
| `go test -race -count=1 ./...`, wall time | 45.3 |
| `pnpm e2e`, wall time, 562 tests | 94.2 |
| Documentation checks | 6.6 |
| Four OSV-Scanner scans and both Gitleaks scans | 7.4 |
| Frontend checks before `pnpm e2e`, `pnpm check:bundle` included | 16.9 |
| Backend checks other than `go test` | 4.4 |
| Secret guard hook and local settings wrapper checks | 4.4 |

- **End-to-end suites:** 94 seconds with three more tests. This is within the 84 to 94 seconds that the earlier records measured.
- **Bundle sizes:** gzip, from `pnpm check:bundle`:
  - the JavaScript of the entry and its static imports is 170.1 KB, against the 180 KB limit of D44;
  - a student's Home loads 183.6 KB first, against 250 KB;
  - a tutor's Today loads 183.8 KB first, against 400 KB.
- **Against the thresholds:**
  - 179.8 seconds is under the 180-second consolidation threshold and 1.8 seconds above the plan's slice target of 178 seconds. The target assumes a warm, idle machine; this run ran on a loaded one.
  - F1 to F3 still add Playwright tests. The F1 realtime client, at an estimated 6 to 10 KB gzip, leaves little room under the 180 KB first-load limit.
  - The cold idle-machine run of the slice review remains the deciding measurement.
  - The 10-minute budget of NFR-6.2 is not at risk.

## Final tree

On the final tree, after the sign-in race fix, the full run took 187.0 seconds:

- every command exited 0;
- `go test` took 46.9 seconds;
- `pnpm e2e` took 98.0 seconds for 562 tests.

The one-minute load average was 18.6 when the run started, because another session was busy. The run is therefore above the 180-second threshold on a loaded machine, and its end-to-end time matches the three repeat runs. No suite was consolidated: on this machine the end-to-end time is not above base `main`'s 107 seconds. The slice review's idle-machine run decides whether consolidation is needed.
