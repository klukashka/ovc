# Navigable fake room verification

Status: implemented on 2026-11-07, independent reviews recorded below; the exact-tree gate decides the merge. Base: the [authorized lifecycle routes](slice-7-meeting-lifecycle-api.md) on `e949f14`.

Scope: section 3 of the [approved local plan](../plans/slice-7-local-completion.md): `/__test__/fake-room`, the conversion of a shared-fake link into a loopback browser URL, the fake participant display name, and the embed, availability and end controls. Tier: standard. Security trigger: met (test control surface, join credentials, framing). No lesson UI, positive iframe journey or BigBlueButton adapter is included.

## Decisions refining the plan

Each is recorded in the [video specification](../specs/realtime-and-video.md#navigable-fake-room) or the [development guide](../../docs/development.md#end-to-end-test-harness); none changes an approved contract.

- The credential travels in the URL fragment, so the plan's "only where the join-link contract allows" reduces to no request URL at all; the query string stays empty.
- Room answers are `200` with `status` `absent` or `unavailable` instead of `404` and `503`. They describe the simulated provider, and a failing status would be a browser console error in every journey that ends a lesson, which the strict diagnostics would have to allow.
- A forged, changed, stale and ended capability answer alike, so the page tells `ended` from `refused` only by whether it has been admitted.
- The room origin is the typed test-control setting `OVC_FAKE_ROOM_ORIGIN`, defaulting to the API's own loopback address. A local probe on 2026-11-07 showed WebKit blocking a plain `http://127.0.0.1` frame on an HTTPS page while Chromium and Firefox load it, so the harness serves the room through an HTTPS forwarder of its own (`frontend/e2e/room-origin.ts`) that forwards only the room's paths unchanged.
- The embed override is an in-process switch, because the end-to-end deployment has one API process and the worker returns no `embed_allowed`.
- The provider-side end control takes the capability the test already holds, so it reads no business row.
- `TestSharedFakePrivacyAndStorage` no longer forbids the participant name in the fake's participant rows, as the plan's refinement of the earlier no-name storage requires; it still forbids it in logs, formats and link rows, and `TestFakeRoomContract` proves End releases it.

## Threats

T-36: routes registered only with `OVC_TEST_CONTROL=1`, loopback `Host` on every route, refusal per control (`TestFakeRoomRoutesExistOnlyWithTheFlag`, `TestFakeRoomContract/requests_are_bounded_and_typed`). T-25: fragment-only credential, digest storage, generation and role binding, stale links refused (`TestFakeRoomContract`, `meeting-room.spec.ts`). T-18, T-19: only the room's name, the participant's own name and role are shown; names and credentials absent from diagnostics (`TestFakeRoomContract/credentials_and_names_stay_out_of_diagnostics`). T-16, T-24, T-45: the room is a separate origin, though same-site with OVC on loopback, so its page requests may carry the `HttpOnly` refresh cookie; the routes ignore it, nothing logs it and the room's `POST`s omit credentials. It sets `frame-ancestors` to the configured OVC origin. T-11, T-12, T-26: no recording control, no callback bypass (`TestFakeRoomContract/no_recording_control_or_callback_bypass_exists`, `meeting-room.spec.ts`). T-20: 4 KiB bodies, existing room, link and participant caps. T-41: local evidence only.

## Acceptance

| Plan item | Evidence |
|---|---|
| Real HTTP contract | `TestFakeRoomContract`, `TestNavigableFakeOrigin` over a real server and PostgreSQL |
| Correct participant and role, scoped visibility | `meeting-room.spec.ts`: "each participant sees the neutral room with their own name and role only" |
| Malicious name rendering | "a hostile name renders as text", `TestFakeRoomContract/a_hostile_name_stays_data` |
| Stale and changed capability refusal | "a changed capability opens no room and stops", "a missing capability opens no room and asks nothing", "an oversized capability opens no room and asks nothing", "a restart ends the room that is open", "after a restart the old link is refused and the new link opens in the same page" |
| Cross-process entry and end | `TestFakeRoomAcrossProcesses` through two real API processes, "ending the lesson through the API ends the room" |
| Route absence | `TestFakeRoomRoutesExistOnlyWithTheFlag` |
| No recordings | "no recording control exists", `TestFakeRoomContract/no_recording_control_or_callback_bypass_exists` |
| No-referrer, cache and framing headers | "the room is served with the no-store, no-referrer and framing headers", `TestFakeRoomContract/the_page_and_its_assets_carry_the_room_headers` |
| Timer and resource cleanup | "refreshing runs once a second", "refreshing stops on leave", "refreshing resumes after entering again", "refreshing stops on disposal", "refreshing stops when the room ends", under a paused page clock |

Not covered here: a positive iframe journey needs `frame-src` on an OVC page, which the lesson UI stage adds with its test-only override; this change proves the headers and the HTTPS origin it will frame.

## Observability

No new event. The provider's `meeting.provider.completed` and `meeting.provider.failed` gain the fixed operation `room_view`, and the access log carries route patterns, so no capability reaches it.

## Independent reviews

The standard review found one blocker and one major finding, both fixed:

- Blocker: `dbgen` was stale after migration 00030, so `sqlc diff` failed. Regenerated.
- Major: a bulk replacement had added this change's status sentence to three unrelated sections of the threat model, breaking one citation. Those lines are restored and only the document's status line carries the sentence.

Its minor findings, fixed: interleaved phases in three end-to-end tests are split into separate tests; a cancelled room request no longer logs `testcontrol.failed` (`TestFakeRoomContract/a_cancelled_request_is_no_failure_in_diagnostics`); an oversized fragment is refused without a request; `core.IsFakeRoomOrigin` is the one origin check, so `Load` names the variable and refuses it in the worker as well; `TestJoinProviderFollowsTheFlag` covers the API's default origin; `TestNavigableFakeProviderContract` runs the provider contract; the untested and unspecified `pageshow` resume is removed. The remaining minor findings are in the [known limitations](slice-7-known-limitations.md#navigable-fake-room-residuals).

Forwarder: `frontend/e2e/room-origin.ts` is a TLS forwarder in front of the real API, on loopback, that passes the room's paths unchanged. It replaces no behavior, so it is recorded here and in the development guide, not as an exception of ADR-0009.

The security review, after those fixes, found no blocker or major finding. Its minor findings, fixed: `core.IsFakeRoomOrigin` now accepts only a bare origin with a valid port on `127.0.0.1` or `::1`, in any spelling of those addresses (`TestNavigableFakeOrigin`, `TestLoadChecksTheFakeRoomOrigin`); the threat wording above states that the cookie can reach the room and is ignored; every room route is tested against a foreign host and a wrong content type (`TestFakeRoomContract/requests_are_bounded_and_typed`), a cross-origin preflight is refused (`TestFakeRoomContract/a_cross-origin_preflight_is_not_allowed`) and the forwarder's path filter is tested ("the room origin forwards only the room"). None of these fixes touches a trust boundary beyond tightening a refusal; the origin check and the cookie wording concern T-16 and T-24. It reports T-16, T-18, T-19, T-20, T-24, T-25, T-36 and T-45 examined and T-11, T-12, T-26 and T-41 not affected. Not exercised: a live cross-origin attempt from a hostile page in a browser.

The scoped re-review of those fixes found one major finding, fixed: the origin cases of `TestLoadRefusesUnsafeTestControlCombinations` could not fail, because their base values were already refused for the storage endpoint, so nothing proved that `Load` refuses the origin. `TestLoadChecksTheFakeRoomOrigin` now isolates the origin, asserts that the refusal names the variable and covers the API and the worker. Its minor finding on `TestNavigableFakeProviderContract` is by design: that test proves delegation, and `TestJoinProviderFollowsTheFlag` proves the conversion. The late refusal of an unusable default origin is in the known limitations.

The owner then chose one more security-profile reviewer, limited to that test, the origin check and these records. It found no blocker or major finding: mutants that removed the check, limited it to one process, named another variable or dropped the flag condition all failed the test. Its minor findings are resolved by a port zero case and by the wording of the records above.
