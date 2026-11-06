# Lesson form layout verification

Status: verified; merged through the merge gate.

## Approved scope

The lesson creation and editing forms place the date on its own full-width row.
Start time and duration share two equal columns at the tablet breakpoint and above,
and stack on narrower screens. Their labels, controls and validation messages use
shared grid rows. Recurrence fields and scheduling semantics remain unchanged.
Verification tier is standard. The presentation-only change does not meet the
security trigger and adds no diagnostics or logging to the application.

Implemented on `57bf16d4be98b6e7e367747a2f35b3a2943d9366`; rebuilt without change on later `main`, after the password-reset fix, the student tab labels and a test race fix in the messages suite.

## Evidence

- TypeScript, ESLint, formatting, 898 component tests, build, class validation and
  bundle checks passed before the browser run. Later test-only corrections have
  passed TypeScript; targeted ESLint also passed; the final formatting check is recorded below.
- Independent standard source review found no findings; runtime evidence was
  explicitly outside its verdict.
- The first focused browser run executed 48 cases in 27.4 seconds: 28 passed,
  20 failed. Source remained unchanged and all seven monitoring samples were safe.
- All exercised field geometry assertions passed, including full-width date,
  aligned time and duration, and no horizontal overflow at 320 pixels.
- Twelve invalid-duration cases failed strict diagnostics because their expected
  PATCH 422 had not been declared. The test now declares that exact request.
- Four WebKit series cases assumed pointer selection focuses a radio. The test now
  focuses the radio and selects it with Space, preserving the focus assertion.
- Four WebKit creation cases reported two unknown console errors each. Their
  retained traces contain test events but no browser console events; the cause
  was initially unverified and was subsequently established by the approved
  diagnosis below. No diagnostic allowance was added.
- Retained WebKit screenshots visually show the complete year 2026 at desktop and
  320-pixel widths. Later trace frames also visually confirmed the complete year
  in Chromium and Firefox at both widths.

The first browser run is retained privately under
`/private/tmp/ovc-layout-focused-observed`, with artifacts in the worktree's ignored
`frontend/test-results/playwright/run-76180` directory. Raw artifacts are not
repository content.

## Approved bounded diagnosis

Run only the Russian desktop creation case in WebKit once with `--trace=on`,
unchanged assertions, retries disabled and existing deadlines. Preserve the first
run's artifacts in an owner-private directory before the harness clears its own
output. Capture remains inside owner-private directories and is inspected locally.
Identify the console errors without suppressing or broadly allowing them. This
run is diagnostic, not acceptance; an unexplained failure stops further runs.
The owner approved this diagnostic run. It reproduced the two console errors.
Both occur immediately after the test calls `dialog.screenshot()`. The installed
Playwright 1.63.0 screenshot preparation inserts an inline `body {}` stylesheet
when WebKit synchronizes animations; CSP refuses it and the application reports
`csp_violation`. The added screenshot call has been removed from the test.
CSP and strict diagnostics are unchanged. The diagnostic trace and original
artifacts are preserved in separate owner-private directories.

## Final checks and preview

- Corrected focused run: all 48 cases passed in 17.0 seconds, all five monitoring
  samples safe and source unchanged.
- Backend test-control startup refusal and route absence checks passed in
  `internal/core` and `cmd/internal/startup` with the race detector.
- Full E2E: 1026 passed and one failed in 2.9 minutes, all 36 monitoring samples
  safe and source unchanged. All 48 new layout cases passed in the full run.
- The unchanged Chromium password-reset test failed at `signup.spec.ts:882`:
  DevTools returned no response body for the resource. Commit `1651a84` fixed
  that failure outside this change, and the merge gate reran the whole suite.
- Independent standard review found no implementation finding. Its minor
  documentation finding about stale diagnostic/visual status is corrected here.

The merge gate records the final run of every check, Gitleaks included.

## Gate rerun exception

The merge gate of this change failed on 2026-11-06 on backend tests this change
does not touch: `TestPresenceOperationsRespectAdmissionAndShutdownDeadlines`,
`TestAFailedPresenceReleaseRemainsPending` and
`TestUploadSettlementDeadlineCoversClockAndPoolWaits`, each at a deadline, with a
five-minute load average of 18 on 12 cores. The same backend passed the gate
minutes earlier at `00e201d`. Load is the suspected cause, not a proven one. The
owner approved one rerun on an idle machine for this change only; it exempts no
check and sets no precedent. Commit `39d5385` replaced the upload test, whose
failure was a test race; the two presence tests stay open as flaky.
