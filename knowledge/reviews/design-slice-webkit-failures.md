# Design slice: WebKit end-to-end failures under load

Status: agent record, started 2026-11-08 under the [step L plan](../plans/design-step-l.md#8-the-two-items-carried-from-slice-7). It carries the slice 7 item "WebKit end-to-end failures under heavy load" ([owner dispositions](slice-7-known-limitations.md#owner-dispositions-2026-11-08)). The owner moved the load diagnosis of these failures to Slice 10 on 2026-11-08, which prepares the first deployment of OVC on a server; until then each failure is recorded here and goes to the owner. Each entry quotes only fields of the redacted `browser-diagnostics.json` and the Playwright error, never request bodies, cookies or addresses carrying tokens.

## Failures seen while preparing step L1 (not in a merge gate)

Both appeared in a whole `pnpm e2e` run of the step L1 working tree (three browsers, the shared Playwright workers), and both passed ten times in a row when repeated alone in WebKit on the same tree. Step L1 changes neither page's code: Schedule and Messages only lost a `wide` prop that the new default width replaces.

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `schedule.spec.ts` › pilot-1 › step 6 Maria schedules eight lessons on Tuesdays and Thursdays, which Anna sees in Berlin time | WebKit | journey | The 30 s test timeout ran out inside `lessonAddresses` (`locator.evaluateAll` over the week's lesson links); no browser diagnostic record | 10 of 10 passed |
| `message-attachments.spec.ts` › pilot-3 › steps 8–11 Anna closes her tabs and receives one digest for one unread group message without a duplicate | WebKit | journey, route `messages` | Three `GET` requests of the Messages page (`/api/v1/channels/unread`, `/api/v1/channels/{id}`, `/api/v1/channels?…`) answered `401`, each with its console error | 10 of 10 passed |

Reading: both look like the load-dependent WebKit failures carried from slice 7. The second points at an access token that expired or was refused while the page loaded after the test moved the shared clock, which the refresh path normally recovers before the page's requests. No cause is proven yet. A failure of this kind in a design slice merge gate is diagnosed here from that gate's kept `browser-diagnostics.json` before the owner decides on a rerun.

## Failures seen while preparing step D2a (not in a merge gate)

Two whole `pnpm e2e` runs of the step D2a working tree showed four WebKit failures besides the change's own fixed tests, two of them a pair; each passed at least ten times in a row when repeated alone in WebKit on the same tree.

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `schedule.spec.ts` › pilot-1 › step 6 Maria schedules eight lessons on Tuesdays and Thursdays, which Anna sees in Berlin time | WebKit | journey | `browserContext.newPage: Test ended`: the 30 s test timeout ran out while opening Anna's page, as in the step L1 record above | 10 of 10 passed |
| `session.spec.ts` › two tabs refreshing at once keep the session | WebKit | journey | The 30 s test timeout ran out inside a `page.evaluate` that waits for both tabs; one unproven reading is that Today, which now reads three more paths before it is ready, lengthens that wait under load | 10 of 10 passed |
| `upload-transport.spec.ts` › an expired multipart upload refreshes once, removal aborts, and changing accounts clears a held upload | WebKit | journey | The 30 s test timeout ran out on the "Sign out" click; the test that ran next in that run failed at once with `browserContext.newPage: Target page, context or browser has been closed` | 10 of 10 passed alone |
| `today.spec.ts` › a new tutor sees the three undone checklist steps | WebKit | before the test body | `browserContext.newPage: Target page, context or browser has been closed`, only when it ran right after the upload test above | 15 of 15 passed alone |

Pairing check: the upload test followed by the existing `smoke.spec.ts` in one WebKit worker (`--workers=1 --repeat-each=8`, 184 tests). Corrected on 2026-11-09: the first failure of that run was a smoke test (test 17 of 184, a `boundingBox` wait that ran out), before the upload test ran (test 23); from then on page loads timed out in every following test, the upload test included. So the upload test did not start the failures. The run shows only the order of failures, consistent with WebKit degrading over a long run in one worker; the mechanism is not established. The owner decided on 2026-11-09 that this belongs with the other WebKit load diagnoses in Slice 10, and no separate fix is made now.

## The first merge gate of step D2a

The first gate of step D2a (commit `1b6b99f` on base `dc5ab40`, tested tree `d20806b`) failed on one WebKit test on 2026-11-08; the owner approved a rerun of the same tree, which passed, and the change merged.

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `message-attachments.spec.ts` › pilot-3 › steps 14–15 Anna unsubscribes through the digest and receives no new digest while offline | WebKit | journey | The message send answered `201`, then waiting for the composer to become writable again ran out the 30 s test timeout; no `browser-diagnostics.json` was kept because no console or HTTP error was recorded, so the evidence is the gate's `failed-tests.txt` and its end-to-end log | 10 of 10 passed; the whole `pnpm e2e` on the same tree before the gate passed 1374 of 1374 |

## Failures seen while preparing step D2b (not in a merge gate)

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `message-attachments.spec.ts` › pilot-3 › steps 8–11 Anna closes her tabs and receives one digest for one unread group message without a duplicate | WebKit | journey, route `messages` | Three HTTP errors with their console errors on the Messages page, the same signature as the step L1 entry above | 10 of 10 passed |

## A merge gate of the lesson room focus and Today settle test fix

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `schedule.spec.ts` › pilot-1 › step 6 Maria schedules eight lessons on Tuesdays and Thursdays, which Anna sees in Berlin time | WebKit | sign-in through the API | `pageSettled` waited for `main h1` until the 30 s test timeout ended the page; the change touched only a component test of the lesson room and the overlay smoke tests | Not repeated alone; the owner approved one gate rerun on 2026-11-09, which passed on the same tree `6aae58aa3e6f0a00afebd5d9886765b94be9820c` |

The same class as the WebKit page load failures above, carried into slice 10 with them.

## Failures seen while preparing step D4 (not in a merge gate)

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `message-attachments.spec.ts` › pilot-3 › steps 8–11 Anna closes her tabs and receives one digest for one unread group message without a duplicate | WebKit | journey, route `messages` | Four HTTP errors with four console errors on the Messages page, the signature of the step L1 and step D2b entries; step D4 does not touch Messages | Not repeated alone; the whole run otherwise passed (1442 tests) |

## Failures seen while preparing step D6 (not in a merge gate)

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `students.spec.ts` › student card › notes typed and reloaded before the save keep nothing in storage or the address (from step D5) | WebKit | journey | One page error during the reload; the redacted diagnostics do not keep its text, so the cause is not established; a reload while an autosave is in flight is the suspected moment | 15 of 15 passed alone (`--repeat-each=15`) |

## The first merge gate of the notes autosave per-card fix

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `messages.spec.ts` › text messages › editing and deleting own messages, and the tutor deleting a member's message | WebKit | journey, route `messages` | The click on the message menu's Delete waited 30 s for the button to become visible, enabled and stable; the change touched only the student card | 10 of 10 passed alone (`--repeat-each=10`); the owner approved one gate rerun on 2026-11-09 |

The same class as the WebKit failures under load above, carried into slice 10 with them.

## The first merge gate of the design slice frontend small fixes

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `messages.spec.ts` › text messages › editing and deleting own messages, and the tutor deleting a member's message | WebKit | journey, route `messages` | The same failure as in the first gate of the notes autosave per-card fix: the click on the message menu's Delete for the member's message waited 30 s for the button to become visible, enabled and stable; neither change touches Messages' menu | 20 of 20 passed with four workers (`--repeat-each=20 --workers=4`), after 10 of 10 alone; the owner approved one gate rerun on 2026-11-10 |

This test has now failed the same way in two merge gates of unrelated changes and never alone, so it is recorded as a flaky test with an unknown cause in the known limitations and carried into slice 10's WebKit diagnosis.

## The first merge gate of the meeting backend small fixes

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `lesson-room.spec.ts` › the lesson room through a new tab › lesson room › signed out, the lesson address asks to sign in and shows nothing private | WebKit | journey | One console error, `page_load_failed TypeError`, from the frontend's own load-failure log: a request of the page load ended with a network error; the change touched only the backend | Not repeated alone; the owner approved one gate rerun on 2026-11-10 |

The same class as the WebKit page load failures under load above, carried into slice 10 with them.

## The first merge gate of the presence capacity test fix (Firefox under memory pressure)

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| Eight tests in `groups.spec.ts`, `home.spec.ts` and `invitations.spec.ts` | Firefox | journey and test API calls | The end-to-end lane ran 17.6 minutes instead of about 5; test API calls ended with `TimeoutError` and pages with test timeouts; the machine used 3.4 of 5 GB of swap with a load average near 15; the change touched only a backend test | Not repeated alone; the owner approved one gate rerun on 2026-11-10 |

Not a WebKit failure, but recorded here with the other failures under load, which slice 10 diagnoses.

## The merge gate of the meeting backend small changes on the moved main (Firefox under memory pressure)

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| Fourteen tests, among them `desktop-layout.spec.ts` › no sideways scroll at 1024 px | Firefox | journey | The end-to-end lane ran 23.5 minutes; Firefox's own automation layer logged internal errors (`NS_ERROR_FAILURE` from its remote agent); the machine kept 3.4 of 5 GB of swap with a load average near 20; the change touched only the backend | Not repeated alone; the owner chose a gate rerun without restarting the machine on 2026-11-10 |

## The next merge gate of the meeting backend small changes

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `session.spec.ts` › the sixth failed sign-in within 15 minutes says when to try again | WebKit | journey | One HTTP error beyond the expected refusals; the run took the usual 5 minutes; the change touched only the backend | Not repeated alone; the owner approved a rerun on 2026-11-10 and, until the slice closes, one rerun per gate when only a browser test unrelated to the change fails, each case recorded here and in the merge report |

The standing approval of one rerun per gate for an unrelated browser failure ended when the design slice closed on 2026-11-10; from then on the rerun rule of AGENTS.md and ADR-0018 applies unchanged.
