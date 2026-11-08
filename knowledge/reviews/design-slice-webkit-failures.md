# Design slice: WebKit end-to-end failures under load

Status: agent record, started 2026-11-08 under the [step L plan](../plans/design-step-l.md#8-the-two-items-carried-from-slice-7). It carries the slice 7 item "WebKit end-to-end failures under heavy load" ([owner dispositions](slice-7-known-limitations.md#owner-dispositions-2026-11-08)). The owner moved the load diagnosis of these failures to Slice 10 on 2026-11-08, which prepares the first deployment of OVC on a server; until then each failure is recorded here and goes to the owner. Each entry quotes only fields of the redacted `browser-diagnostics.json` and the Playwright error, never request bodies, cookies or addresses carrying tokens.

## Failures seen while preparing step L1 (not in a merge gate)

Both appeared in a whole `pnpm e2e` run of the step L1 working tree (three browsers, the shared Playwright workers), and both passed ten times in a row when repeated alone in WebKit on the same tree. Step L1 changes neither page's code: Schedule and Messages only lost a `wide` prop that the new default width replaces.

| Test | Browser | Phase | Evidence | Repeat alone |
|---|---|---|---|---|
| `schedule.spec.ts` › pilot-1 › step 6 Maria schedules eight lessons on Tuesdays and Thursdays, which Anna sees in Berlin time | WebKit | journey | The 30 s test timeout ran out inside `lessonAddresses` (`locator.evaluateAll` over the week's lesson links); no browser diagnostic record | 10 of 10 passed |
| `message-attachments.spec.ts` › pilot-3 › steps 8–11 Anna closes her tabs and receives one digest for one unread group message without a duplicate | WebKit | journey, route `messages` | Three `GET` requests of the Messages page (`/api/v1/channels/unread`, `/api/v1/channels/{id}`, `/api/v1/channels?…`) answered `401`, each with its console error | 10 of 10 passed |

Reading: both look like the load-dependent WebKit failures carried from slice 7. The second points at an access token that expired or was refused while the page loaded after the test moved the shared clock, which the refresh path normally recovers before the page's requests. No cause is proven yet. A failure of this kind in a design slice merge gate is diagnosed here from that gate's kept `browser-diagnostics.json` before the owner decides on a rerun.
