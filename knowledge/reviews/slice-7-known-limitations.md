# Slice 7 known limitations

Status: working review inventory. Entries are evidence for cumulative slice review, not automatic owner acceptance or permission to close the slice.

## Cleanup reinspection interleaving

Severity: minor, identified by the independent security review of the constructor-injected cleanup executor based on `25e2066bcb5583048675ecda32f8e79165e626b4`.

The original executor tests did not deterministically replace a room between successful Inspect and End. The follow-up adds `TestMeetingRecoveryWorkerReinspection` for replacement conflict, replacement ended before reinspection, and provider unavailability during reinspection, plus `TestMeetingRecoveryReinspectionCancellation` for explicit cleanup-context cancellation. They use real PostgreSQL, two constructor-injected SharedFake clients and bounded backpressure on actual provider diagnostics after the provider transaction releases. Provider responses and state transitions are not replaced. Focused race tests and the full backend run passed; independent standard and security reviews found no findings and confirmed the reachable local fake coverage. The follow-up merged as `05a79d2207c1573617f9ba4838ae7325b0f2280a` under the [exact owner exception](../plans/slice-7-interleavings-known-failure-exception.md) after a failed gate: 29 commands passed and E2E passed 978/979. The Chromium password-reset timeout and unknown HTTP/console diagnostics remain unresolved; no ordinary application finish occurred. The mandatory documentation gate certifies only its documentation tree.

An unchanged original generation after a stale-End refusal cannot be produced by the current SharedFake contract: End returns not_found for a present different generation, absent End succeeds, and replacement generations strictly increase. Recreating the original generation or fabricating a refusal would replace provider behavior. The corresponding service branch remains unverified and must be checked against the later BBB protocol implementation and remote-provider contract before that provider is activated. Local fake coverage cannot certify remote cleanup semantics.

Disposition: the reachable local fake interleaving coverage gap is closed by the executed follow-up and independent reviews, with integration authorized only by the exact failed-gate exception above, not by an ordinary passing gate. The remaining provider-specific unchanged-generation outcome is retained for the named slice 7 BBB adapter and remote-activation follow-up. This is not owner acceptance for slice closure. Backend and security cumulative reviewers must recheck this entry. See the [original implementation evidence](slice-7-meeting-cleanup-worker.md) and [interleaving follow-up](slice-7-cleanup-interleavings.md).

## Meeting lifecycle residuals

Recorded 2026-11-06 with the authorized lifecycle change; each is an item for the cumulative reviewers, not an owner acceptance.

- **Roster-only viewers get no room frames.** `entity.changed` for room and lesson transitions names the tutor and the current audience held under locks, as the existing lesson paths do. A former member sees the change on the next read. Severity: minor.
- **A restart that cannot confirm End blocks the room.** While the provider stays unavailable, a tutor join answers `503` until End or the cleanup settlement confirms the old generation; an exhausted cleanup needs the operator retry or a working provider. The lesson can still end. This is the approved "publish no replacement" contract. Severity: minor.
- **Stale rows near the head of a page.** The scanner's abandoned-row pass reads the oldest 100 and a failing row stays at the head. Failures stay visible through the failed job. Severity: minor.
- **Unreachable revocation conflict.** The `conflict` branch of `Rooms.revoke`, a current generation that differs from the revoked one, cannot be produced by the shared fake, because application provider identifiers are never reused. It answers `503` and must be checked against the BigBlueButton adapter. Severity: minor.
- **Budget of the live status pass.** It shares the status job's 20-second budget after the scheduled pass and inherits the recorded overrun limitation of the [slice 5 known limitations](slice-5-known-limitations.md). Severity: minor.

## Uncertain obligations never close

Recorded 2026-11-07 with the owner's decision on the per-lesson pause. Severity: major for remote activation; it blocks that step.

**Resolved 2026-11-07** by the [quiet-window settlement](../specs/realtime-and-video.md#quiet-window-settlement) of [ADR-0021](../../docs/decisions/0021-quiet-window-meeting-settlement.md): a started or uncertain obligation settles once a cleanup cycle claimed at least 1 hour after dispatch observes the provider identity absent. The remaining residuals (an unreachable provider keeps obligations counted, a Create taking effect after the window, the provider's self-expiry unverified remotely) are recorded in that decision and in the threat model. The text below is kept as the original record.

A started or uncertain obligation never settles, because remote quiescence cannot be proved, and it counts against the tutor's limit of 64 and the global 4096. The 2-minute pause per lesson only slows this: a tutor with several lessons, or one lesson over about two hours of failures, can still reach the limit for good, and then cannot open any room. Locally the only source is the fake's simulated outage or an expired Create budget. **Remote activation is the point where this must be resolved:** a real provider must not be activated until a procedure for closing uncertain obligations exists. Attempts that crashed, and attempts whose uncertainty mark itself failed, stay `started` and start no pause, so they consume the limit without that slowdown. This is not owner acceptance for production.

## Clock step residual

Severity: minor. A clock stepped back behind a row's `updated_at` can no longer strand a close, but Open and first join keep their guards. A refused Open retires the row and creates one extra provider room. A refused first join discards the link and answers `503` until the clock catches up. A clock stepped back behind an uncertain outcome ends that lesson's pause early, because an outcome later than the current instant does not count. No room is stranded and cleanup keeps ownership.

## Realtime client connection limits

Recorded 2026-11-06 from the independent reviews of closing a connecting realtime socket only after it opens. Both are minor and neither blocks local slice 7 work.

- **A hung handshake delays the next socket.** The client no longer closes a connecting socket, so if the upgrade stalls, a new socket to the same address can queue behind it until the browser gives up. A local probe showed the next socket still connecting after 5 seconds in Chromium, Firefox and WebKit. Only live updates wait, REST is unaffected, and the client had no connect timeout before. The browser's own timeout was not measured. Accepted because an injected deadline would add a console message on an already broken path; revisit with the real-network reconnect measurement of the remote probe.
- **A socket opening during an account adoption can send the old token.** `adopt` advances the session epoch before it notifies listeners, so a still-connecting socket of the old client that opens in that window sends `auth` with the old account's own token, and non-chat frames can trigger refetches, which REST answers under the new token. No cross-account data was found on this path, and the server still validates that token. This predates the change above. A later change should add the epoch check to the client's `onOpen` guard, with a test.

## Navigable fake room residuals

Recorded 2026-11-07 with the [navigable fake room](slice-7-fake-room.md); each is an item for the cumulative reviewers, not an owner acceptance.

- **Removing a room frame can abort a refresh.** The room refreshes once a second, and the lesson UI stage must remove the frame on leave, on switch and on end. Removing it during a request aborts that request with no navigation, which the strict end-to-end diagnostics report, while an expected abort may not happen at all. Tests that dispose of a frame need a deterministic rule, for example leaving the room first or pausing the page clock. The lesson UI stage owns this. Severity: minor.
- **Closing a tab leaves the participant present.** A closed page sends no leave, so the fake keeps `present = true` until a leave or the end of the room, and `MeetingInfo` keeps counting that participant. The only service use found is the recovery snapshot validation, so nothing depends on it today. A later scenario that asserts participant counts after a tab closes would see the stale count. Severity: minor.
- **An unusable default room origin fails late.** With the flag `Load` accepts `OVC_HTTP_ADDR` values that give no usable room origin, for example port 0, a named or empty port, another address of `127.0.0.0/8`, an IPv4-mapped address or a zone, and the default room origin built from them is refused. The API then stops at composition with a generic invalid input failure that names no variable. It fails closed and only under a local test configuration. Severity: minor.
- **No resume after a back-forward restore.** The room stops refreshing on `pagehide` and does not resume if the browser restores the page from the back-forward cache, so the page shows its last state until it is reloaded. The room is served `no-store` and exists only for tests. Severity: minor.

## End-to-end readiness failures seen only under a stress run

Severity: minor, not assessed by a reviewer. Change: the readiness fix for cancelled writes in `frontend/e2e/navigation-readiness.ts`. Repeating one Chromium test of `session.spec.ts` 400 times with 12 workers also produced `GET /api/v1/health/ready` answered `503` after about one second (`response_refused_or_duplicate`), reads still pending when the 5-second readiness wait ended, and a failed `ovc-admin users block`. Their causes are not established. They may be overload from the stress run itself, because the same test passed 60 of 60 repetitions at ordinary load, and the failures clustered at the same moments. They were accepted because no full gate has been shown to fail from them. Recheck them at the slice review.

## End-to-end readiness and cancelled writes

Severity: minor, from the standard review. Change: the readiness fix for cancelled writes. A non-GET request cancelled without any response no longer fails the readiness observer. On a page with watched diagnostics such a cancellation still fails at teardown, unless the existing navigation, close or expected-abort rules excuse it. The second and third tabs of `session.spec.ts:197` and `:220` have a readiness observer without watched diagnostics, so an unanswered cancelled refresh there is not detected. Readiness also no longer shows that the server has finished a cancelled write. The owner accepted this on 2026-11-07, because it is unknown whether the observed cancellation had a response before it. The cause of the cancellation is also not established. Recheck both at the slice review.

## Lesson room interface residuals

Recorded 2026-11-07 with the lesson room interface; each is an item for the cumulative reviewers, not an owner acceptance.

- **A blocked new tab has no end-to-end proof.** The approved plan asks for popup refusal in all three engines. With popup blocking switched on (`--disable-popup-blocking` removed in Chromium, `dom.disable_open_during_load` in Firefox) and a click without user activation, `window.open` still returned a tab in Chromium, Firefox and WebKit under Playwright, so the refusal could not be produced there, and replacing `window.open` is a forbidden mock. `LessonRoom.test.tsx` proves the behavior with an injected `OpenRoomTab` that returns no tab: no join request, the notice is shown and focus stays. Severity: minor. The owner accepted the component test in place of the end-to-end proof on 2026-11-07.
- **Embedded rooms are left before their frame is removed in tests.** The fake room refreshes once a second, so `frontend/e2e/lesson-room.spec.ts` presses Leave before each disposal to keep the strict request diagnostics deterministic. Disposal of a frame that is still refreshing is not exercised end to end. Severity: minor.
- **Firefox warns about the `autoplay` permission.** The specified `allow` value contains `autoplay`, which Firefox does not know, and each frame logs a console warning there. The embedded suite expects that exact warning in Firefox only, through `expectConsoleWarning`. Severity: minor.
- **Join on Home and Today waits for the design slice.** Pilot scenario 1 steps 9 and 11 are proven on the lesson page until Home gets its Join control. Severity: minor.
- **The fake transport's lesson writes have no contract test.** `join`, `end` and `restart-room` in `frontend/src/lib/api/fakeTransport.ts` are not in `authTransportContract`, and the fake opens a room on a student's join of a scheduled lesson where the API answers `lesson_not_live`. The component tests use them only in the orders the API allows. Severity: minor.
- **An End answer that never arrives has no browser test.** A network failure of `POST /lessons/{id}/end` keeps the dialog open with its reason, but jsdom has no modal dialog and the end-to-end rules allow no aborted request outside the language test, so it is proven by reading only. Severity: minor.
- **Calendar items with `took_place` are proven by the API only.** No component test renders an ended calendar item that took place, and no backend test asserts `took_place` on `GET /calendar`. Severity: minor.
- **A successful late answer after sign-out has no browser test.** The end-to-end sign-out test holds the join request, so it reaches the API after sign-out and gets `401`; the client dropping a successful answer that arrives after an account change is proven by the component test "unmounting, as an account change does, closes the tab and drops the late answer". Holding a response is not allowed under the no-mocks rule. Severity: minor.
