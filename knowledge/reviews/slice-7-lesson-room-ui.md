# Lesson room interface verification

Status: implemented on 2026-11-07; the reviews section lists the independent reviews, and the exact-tree gate decides the merge. Base: the [navigable fake room](slice-7-fake-room.md) on `32173a2`.

Scope: section 4 of the [approved local plan](../plans/slice-7-local-completion.md): the lesson page's room states in Russian and English, the new-tab and embedded paths, End lesson and Restart room, the required `room_open` and `embed_allowed` fields, and the matching pilot scenario steps. Tier: standard, with the security trigger met (join links in the browser, framing policy, sign-out during a join).

## Decisions refining the plan

The owner accepted the refinements below on 2026-11-07.

- `took_place` is added to `LessonDetail` and `CalendarItem`, true once `live_at` is set, because the client could not tell "Ended" from "Did not take place" from `status` alone. It carries no personal data and is visible to both roles.
- `frame-src` stays `'none'` in production. Only the preview server, and only with `OVC_FRONTEND_TEST_ROOM_ORIGIN` set to an `https://127.0.0.1` origin, admits the fake room; any other value refuses to start.
- Join and End on Home and Today, and the lesson page layout, remain the design slice.

## Threats

T-25: `frontend/src/features/schedule/LessonRoom.tsx` keeps the link out of the query cache and storage, clears `opener`, refuses other schemes and drops late answers (`LessonRoom.test.tsx`, `roomState.test.ts`, `lesson room › the link stays out of browser storage, the console and other requests`). T-16: the preview-only frame policy (`scripts/security-headers.test.ts`, `frontend/e2e/smoke.spec.ts`).

## Known limitations

See "Lesson room interface residuals" in the [known limitations](slice-7-known-limitations.md), including the blocked new tab that the automated engines cannot produce.

## Reviews

- **Standard reviewer** over the complete diff: failed with four major findings. A restart confirmed during a pending join did nothing. An End that failed at the network level showed no reason. Most end-to-end tests broke Arrange–Act–Assert. The popup-refusal deviation from the plan was unresolved. Minor findings included a pending attempt surviving End, a stale refusal surviving a restart or a lost offer, a unit test that did not prove its late-answer claim, weak assertions, coverage gaps, fake transport drift and cleanup of shared state.
  - Fixed: End and Restart are unavailable while a join is pending. End shows a reason when the network fails. Leaving the room abandons the attempt. Restart clears the refusal, and a refusal is hidden once the join offer goes. The unit test waits for the late answer. The spec is split into Arrange–Act–Assert tests, with cleanup in `afterEach`. Calendar coverage, the fake contract and the End network failure are recorded as known limitations.
  - Decided by the owner: the popup-refusal deviation.
- **Security-profile reviewer** over the complete diff, replacing the scoped re-review and applying the standard checklist to the fixes: passed. It examined T-25, T-16, relationship scope, T-19 and the test control surface, and found three minor findings, all fixed or recorded.
  - "Hidden document" wording replaced by `pagehide`.
  - A configuration test proves the preview server's production frame policy.
  - The late successful answer after sign-out is recorded as a known limitation.
- **Fixes touching a trust boundary:** the End and Restart guard and the abandoned attempt concern join links (T-25). The configuration test and the T-16 wording concern framing.
