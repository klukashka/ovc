# Synchronize the initial Messages refetch before the phone journey

Status: approved by the project owner on 2026-11-01 after security planning review. This extends the approved Messages edit-conflict change's verification scope; implementation is authorized.

## Observed failure

The targeted production browser run in the edit-conflict worktree passed 14 of 15 cases, including the new conflict continuation in Chromium, Firefox and WebKit. The existing exchange journey on the phone-sized Chromium viewport failed strict diagnostics on `GET /api/v1/channels?q=` with `net::ERR_ABORTED`. The trace places that list request at 14:52:36.052, route navigation at 36.080 and history loading at 36.129, before any editing/conflict action. Route unmount cancels the still-active initial list refetch. The failure remains unresolved even if a later broader run passes; diagnostic allowances are not relaxed.

## Minimal change and proof

In `frontend/e2e/messages.spec.ts`, reuse the existing `trafficAfterReady` and `expectRefetchedAfterTheSocketIsReady` helpers from `realtime.ts`. Register observation for `/api/v1/channels` before navigating the student's page to Messages; wait for WebSocket ready and completion of the resulting initial list reads before the phone journey clicks its conversation. Preserve every existing exchange, unread, focus, privacy and diagnostics assertion and all timeouts. No application behavior, diagnostic filter, retry or test-control route changes.

A negative control in a separate source copy holds only the real post-ready channel-list request through the existing sanctioned route-hold mechanism, later continuing it unchanged. Without the synchronization, navigation must cancel that request and trigger the original strict diagnostic failure. With the synchronization, navigation must remain pending until the response finishes and then complete without that failure. Do not use sleep, `networkidle`, response substitution, changed headers/URL/body, transport patching or a new abort allowance.

Scope: the existing journey in `messages.spec.ts`, this approved plan, the Messages verification record and a narrow pending-owner-review threat-model note for T-41 explaining preserved strict diagnostics and the explicit readiness boundary. Standard tier, security trigger assumed met through security-oriented test diagnostics. A separate full-security planning review precedes owner approval. Include the extension in the current Messages correction's one fresh combined standard/full-security after-fix review, along with the already identified fresh-read and retry fixes; do not start an unrelated review loop.

After approval, execute the negative control and corrected journey across all three supported browser projects, all mandatory frontend/E2E-path checks including the whole E2E suite and backend test-control refusals, documentation/scanners and the separate exact-tree gate. Retain the earlier targeted failure and any broader-run results with their actual tree. The 260-second gate ceiling and the original assertion remain unchanged. If the trace-based diagnosis is not reproduced, stop and reassess the cause rather than adding the wait as an unexplained workaround.

## Security planning review

The independent reviewer found no blocker/major/minor. The helper matches exact pathname, covering the query string without matching channel detail routes. Because its completion tracking also observes request failures, the unchanged strict diagnostic fixture remains essential. Negative control must track the held real Request and preserve the same SPA click; substituting full-document navigation could invoke a different existing diagnostic allowance. The hold continues unchanged under ADR-0009 item 9. Runtime proof remains pending.
