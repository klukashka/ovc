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

Pairing check: the upload test followed by the existing `smoke.spec.ts` in one WebKit worker (`--workers=1 --repeat-each=8`) failed the upload test and then nine smoke tests with the same closed-browser error. Observed order, not a proven mechanism: after the upload test fails or times out, the tests that follow it fail to open a page. The cause predates step D2a and is not established; the owner chose on 2026-11-08 to investigate and fix it in a separate change after step D2.
