# Lesson lobby: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R9 `feat/redesign-lesson-lobby` of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

The screen is the pre-join state of the lesson page: what a tutor or a student sees between opening a lesson and entering the BigBlueButton room. The flows are an explicit "Check devices" opt-in, a local muted self-preview, camera, microphone and speaker selection and test, guidance and retry after a blocked camera or microphone, joining without any preview, the room state line, the phone layout, and the release of every local media track before the join.

OVC today has no lobby. The [Lesson page](../design/web/pages.md#lesson) drives a video area from join states ("Room opens at {time}", "Starting soon", "The room opens when your tutor joins", Join, Joining, in room embedded or in a new tab, Ended), with one join button that keeps focus through the whole join, a pre-opened new tab for pop-up safety, and the new-tab path below 1024 px and on phones. The side column "About the lesson" already holds a short "Before the lesson" hint (new tab, headphones, camera and microphone permission). Room state comes from `room_open` and `embed_allowed` in the lesson data, refreshed on `entity.changed`. The embedded room's iframe carries `allow="camera; microphone; display-capture; fullscreen; autoplay"` ([Embedding](../specs/realtime-and-video.md#embedding)); join links are bearer credentials that live only in a `POST` response body and are never kept ([ADR-0004](../../docs/decisions/0004-bigbluebutton-primary-provider.md)). Lesson status words are in [states.md](../design/web/states.md#lessons). The components are `frontend/src/features/schedule/LessonPage.tsx`, `JoinControl.tsx` and `LessonRoom.tsx`.

Owner's decisions that bind this screen (plan section 3): item 13 (preview locally; join only under the current server gates; show only lesson and room state the client already holds, live, starting soon or ended, never named participant presence; a roster needs a separate provider-interface plan); item 17 (only lesson-lobby Playwright tests may use Chromium fake media flags, Firefox fake media preferences and permission auto-grant through the browser context, with a new ADR-0009 item modelled on item 13); item 16 (one primary action); item 6 (no underlined action links). Plan section 5 drops the drawn "already in the room" and "not connected yet" messages. Section 8 criterion 8 names the proposed tests `preview stops after navigation and late permission`, `preview denial permits authorized fallback` and `lobby cannot bypass tutor gate`, plus no media request and zero `RTCPeerConnection` before Join and ended tracks at every transition.

Mockups at `8b3a8b0` (non-normative): `knowledge/design/web/prototype/redesign/lobby-student.html` (self-preview "Visible only to you" with microphone and camera toggles and device names, a note that the check stays in the browser and the room may ask for permission again, "Starting soon", "Join lesson", "Open in new tab", "Join without camera and microphone", materials), `lobby-tutor.html` (preview "Camera and microphone work", "Live now", a named student "in the room", "Attach material to the lesson"), `lobby-camera-blocked.html` (three steps: the lock icon left of the address, allow Camera and Microphone, close other apps using the camera; "Check again"; "How to do it in Safari and on iPhone"; joining without the check stays possible), `lobby-phone.html` (state line, preview, Join and join without devices).

## How the compared products solve it

### Google Meet (green room)

- The pre-join "green room self-check" shows a preview tile with the microphone, speaker and camera at its bottom; each has a drop-down arrow to change the device, and "More options" then "Settings" opens Audio and Video settings [1].
- Testing is per device: speak and watch the microphone bar move; "Speaker" then "Test speakers" plays a test sound; "Camera" shows the user in the preview tile; then "Join now" [1].
- Meet moved its permission request off page load: a Meet modal first asks whether the user wants to be seen and heard, and only the affirmative "Allow microphone and camera" triggers the browser prompt; first-join allow rates rose by 14% and fewer users chose "Block" [4].
- Users can join without granting access; a control in the meeting offers to allow both later, and Meet's dialog repeats that camera and microphone can be turned off at any time [4].
- A browser-level block persists for future calls, and recovery goes through the address bar's site settings icon or "Reset permissions"; the case study notes these settings are hard to find and need detailed instructions [4].
- The camera troubleshooting page tells users to click the camera icon in the address bar and choose "Allow on every visit", then covers Chrome site settings, operating system privacy settings (macOS, Windows, ChromeOS), closing other apps that use the camera and reloading Meet; Safari is covered only by a link to Apple's help, with no iPhone steps [3].
- The general join page says to grant microphone and camera permissions before the first meeting and to toggle Microphone and Camera before "Join now"; a waiting-room list exists for the host, not for the joining participant [2].

### Zoom (web app and desktop preview)

- Joining in a browser: choose "join from your browser" (a host setting must show the link), click "Allow" for microphone and camera, enter a name, click "Join" [7]. The permission is thus asked up front, not after an opt-in.
- The Web App's settings offer "Test Speaker" (a test tone), "Test Mic" (records and plays back), Speaker and Microphone drop-downs, "Mute my mic when joining a meeting" and "Turn off my video when joining a meeting" [6].
- Zoom's permission page says the user must allow at least the microphone or will not be heard, and gives operating-system steps for macOS, Windows, ChromeOS, Linux, Android and iOS Safari (Settings, Apps, Safari, Camera or Microphone, Ask); it gives no address-bar recovery steps [8].
- The desktop and mobile apps show a "Video Preview" dialog with "Join with Video" and "Join without Video" and an opt-out checkbox "Always show video preview dialog when joining a video meeting"; the web app's equivalent preview is not documented [5].
- A web test meeting lets users check video, audio and microphone before a real meeting [9].

### Microsoft Teams (pre-join screen)

- The pre-join screen offers audio sources "Computer audio", "Phone audio", "Room audio" and "Don't use audio" (joins muted); choosing Computer audio shows an "Audio settings" button, and changes apply when the user joins [11].
- On the desktop, the "Camera" toggle is set before joining, "Open device settings" opens Video Settings, and "Effects and avatars" opens backgrounds; on mobile, "Video is off" is tapped before joining [12]. Speaker and microphone choice and "Make a test call" live in Settings, Devices [11].
- Joining ends with "Join now"; a signed-in user joins right away or waits in a lobby until an organizer admits them [10].
- Teams on the web documents per-browser fixes for a blocked camera: Chrome site settings "Sites can ask to use your camera", Edge "Ask before accessing (recommended)" with a warning that turning it off blocks the camera, Firefox site permissions, and Safari website settings, then a refresh; it notes some settings are managed by an administrator [13].

### BigBlueButton (in-room comparator)

- On entering, the browser asks for microphone permission; the audio modal lets the user choose a microphone input and a speaker output, test audio feedback, and confirm with "Join Audio"; "Microphone" leads to an echo test where users listen for themselves, and "No microphone" joins listen-only [14].
- Join parameters control the flow: `bbb_auto_join_audio` (default true), `bbb_skip_check_audio` (default false, skips the echo test), `bbb_skip_check_audio_on_first_join`, `bbb_skip_echotest_if_previous_device` (3.0, ignored on mobile), `bbb_force_listen_only`, and for video `bbb_skip_video_preview`, `bbb_skip_video_preview_on_first_join` and `bbb_skip_video_preview_if_previous_device` [15].
- Listen-only via the audio prompt depends on the audio bridge; with LiveKit no separate listen-only option is offered unless forced [15].

### Browser platform guidance

- Asking for camera or microphone on page load leads most users to refuse; ask when the feature is first needed, and use the Permissions API to read granted, prompt or denied before calling `getUserMedia()` [17].
- `getUserMedia()` needs a secure context and user permission; a denial, an insecure page or a Permissions Policy block reject with `NotAllowedError`, no matching device with `NotFoundError`, a device held by the hardware or another app with `NotReadableError`; the promise may never settle if the user ignores the prompt, so the interface needs a pending state [16].
- Browsers must show an indicator while a device captures; stopping every track releases the device [16].
- Before permission, `enumerateDevices()` returns device identifiers without labels, and non-default devices only once permission is granted [18].
- Output selection with `HTMLMediaElement.setSinkId()` needs a secure context, may be gated by the `speaker-selection` Permissions Policy, and needs user permission for a non-default device through `selectAudioOutput()` [19]; the `AudioContext` variant exists from Chrome 110, and the full output list needs the microphone permission [20].

## Comparison

| Design question | Google Meet | Zoom (web) | Microsoft Teams | BigBlueButton | OVC decision/target |
| --- | --- | --- | --- | --- | --- |
| When the permission prompt appears | After an explicit "Allow microphone and camera" pre-prompt [4] | On the join page, up front [7] | Not documented for the web join | On entering the room, for the microphone [14] | Only after "Check devices"; never on page load (item 13) |
| Self-preview | Preview tile [1] | Desktop "Video Preview" [5]; web not documented | Camera toggle, preview not documented [12] | Webcam preview dialog in the room [15] | Local, muted, "Visible only to you"; never sent or recorded |
| Device pickers | Drop-down per device on the tile [1] | Speaker and Microphone drop-downs [6] | "Audio settings", "Open device settings" [11, 12] | Microphone and speaker drop-downs [14] | Camera, microphone, speaker; labels only after permission [18] |
| Microphone test | Level bar [1] | Record and play back [6] | Test call in Settings [11] | Echo test [14] | Local level meter; no recording |
| Speaker test | "Test speakers" sound [1] | "Test Speaker" tone [6] | Speaker choice on mobile [11] | Test feedback in the modal [14] | Test sound; output choice only where `setSinkId` works [19] |
| Join without devices | Allowed, add later [4] | "Join without Video" (desktop) [5] | "Don't use audio" [11] | "No microphone" [14] | "Join without camera and microphone", no preview needed |
| Blocked permission guidance | Address bar icon, OS settings, other apps, reload [3, 4] | OS steps per platform [8] | Per-browser steps [13] | Refresh, OS input check, permission [14] | Typed failure kinds, steps per kind, "Check again", fallback join |
| Who is in the room | Not shown to the joining participant in the cited pages | Not documented | Lobby admission [10] | In-room user list | Only live, starting soon, ended; no names (item 13) |
| Track release before join | Not documented | Not documented | Not documented | Not applicable | Every track stopped before the join request (criterion 8) |

## What OVC adopts

- Opt-in before the browser prompt → Google Meet's pre-prompt [4] and web.dev's ask-in-context guidance [17] → item 13; the lobby opens with "Check devices" and makes no `getUserMedia()` call before it, which also satisfies the no-media-request criterion.
- Preview tile with per-device pickers and per-device tests (level meter, test sound) → Google Meet [1], Zoom's test controls [6], BigBlueButton's modal [14] → the mockups' preview; the microphone test is a live level meter only, never a recording, as Zoom's play-back test would require.
- A first-class "join without devices" path → Meet [4], Zoom [5], Teams "Don't use audio" [11], BigBlueButton listen-only [14] → criterion 8's `preview denial permits authorized fallback`; the join button and its server gates stay the same element as today.
- Permission recovery as short, ordered steps plus a retry → Meet's troubleshooting [3], Teams on the web [13], Zoom's OS steps [8] → `lobby-camera-blocked.html`; steps are chosen by failure kind from `getUserMedia()` errors [16]: denied, no device, device busy, unsupported or insecure.
- A pending state for an unanswered prompt → MDN's note that the promise may never settle [16] → `preview stops after navigation and late permission`: a late grant after leaving the lobby stops its tracks at once.
- A reminder that the room asks again → BigBlueButton requests permission inside its own client [14] → the mockups' note that the room may ask for permission again, which stays because OVC's preview grant and the subdomain client's grant are separate origins.

## Where OVC differs and why

- No named presence. Meet and BigBlueButton show who is present in their own surfaces; OVC shows only the lesson state the client holds (item 13 and plan section 5), because OVC has no roster contract with the provider and students must not learn peers' identities ([ADR-0005](../../docs/decisions/0005-tutor-centric-domain.md)). The tutor mockup's "the student is in the room" and the student mockup's "the tutor has not connected yet" are dropped.
- Local-only preview. Unlike Zoom's recorded microphone test [6], nothing is recorded, uploaded or logged; device identifiers and labels never leave the page or enter logs (plan section 6, R9 row), and no `RTCPeerConnection` exists before Join.
- No preview gate. Zoom's desktop preview sits between the click and the meeting [5]; OVC's join never waits for a preview, and the existing join button, its focus behavior and its pre-opened tab stay as specified, because the pop-up rule needs the tab opened in the click handler.
- No in-lobby room settings. Teams-style audio source choices (phone, room audio) [11] and effects [12] are not offered; BigBlueButton owns in-room media, and OVC does not replace the provider's room controls (plan section 4).
- Speaker selection is conditional. Where `setSinkId()` or `selectAudioOutput()` is unavailable [19, 20], the speaker row offers only the test sound on the default output, not a picker.
- Tests use browser-provided synthetic media only, never a patched `getUserMedia()`, under the item 17 exception and the repository's no-mocks rule.
- Status changes (preview started, blocked, retry result) are announced as status messages without moving focus, per WCAG 2.2 4.1.3 [21].

## Open points for the task plan

- Which failure kinds the typed local state distinguishes (`NotAllowedError` from a prompt versus a persisted block, `NotFoundError`, `NotReadableError`, insecure context, Permissions Policy), and whether any safe event kind is logged at all.
- Whether "Check devices" first reads the Permissions API state [17] to skip the opt-in for an already granted origin, and how that behaves in Safari, where support differs (unverified).
- Whether the lobby offers camera and microphone separately (allowing audio only) or as one request.
- Exactly when tracks stop: on Join press before the join request, on route change, on `pagehide`, on account change, on the lesson becoming ended, and on a late grant after leaving.
- Whether OVC's own responses need an explicit `Permissions-Policy` for `camera`, `microphone` and `speaker-selection`, and whether that changes any header (the plan expects none).
- The phone layout's order (state line, preview, Join, join without devices) and whether the preview is collapsed by default on phones, given the new-tab path below 1024 px.
- Wording of per-browser guidance (Chrome, Firefox, Safari on macOS and iOS) without third-party product names such as the mockup's list of other conferencing apps.
- Whether a remembered device choice is kept per device in local storage, and whether that fits the storage allowlist.
- Speaker selection where `setSinkId()` or `selectAudioOutput()` is unavailable [19, 20]: whether the speaker row then offers only the test sound on the default output, and which supported browsers fall in that case.

## Unverified

- Google Meet's green room shows who is already in the meeting before joining; not confirmed by the pages read [1, 2].
- Zoom's web app shows a pre-join video preview with camera and microphone toggles; Zoom documents the preview only for desktop and mobile apps [5, 7].
- Teams on the web asks for camera and microphone on "Continue on this browser" and offers "Continue without audio or video"; seen only in search snippets, not on a fetched first-party page.
- Safari on iOS forgets camera permission between visits or asks on every visit; not found in a fetched source.
- `setSinkId()` support in Firefox and Safari; the MDN compatibility table did not load [19].
- The Permissions API's `camera` and `microphone` names are supported in Safari and Firefox.

## Sources

1. Google Meet — Check your video and audio before a meeting (green room self-check) — <https://support.google.com/meet/answer/10409699> — FP — checked 2026-11-27.
2. Google Meet — Join a video meeting — <https://support.google.com/meet/answer/9302870> — FP — checked 2026-11-27 — no pre-join preview details.
3. Google Meet — Troubleshoot camera issues in a meeting — <https://support.google.com/meet/answer/10621292?hl=en> — FP — checked 2026-11-27 — camera only, no iPhone steps.
4. web.dev — Less friction, more control: How Google Meet improved audio and video permissions — <https://web.dev/case-studies/google-meet-permissions-best-practices> — FP (Google case study about Meet) — checked 2026-11-27.
5. Zoom — Previewing your video before joining — <https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0061118> — FP — checked 2026-11-27 — desktop and mobile apps only.
6. Zoom — Changing settings in the Zoom Web App — <https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0059423> — FP — checked 2026-11-27.
7. Zoom — Joining a Zoom meeting — <https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0060732> — FP — checked 2026-11-27.
8. Zoom — Allowing microphone, camera, and screen share permissions in the Zoom Web App — <https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0078476> — FP — checked 2026-11-27 — no address-bar recovery steps.
9. Zoom — Joining a Zoom test meeting — <https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0063307> — FP — checked 2026-11-27 — search result summary only, page not fetched.
10. Microsoft Teams — Join a meeting in Microsoft Teams — <https://support.microsoft.com/en-us/teams/meetings/join-a-meeting-in-microsoft-teams> — FP — checked 2026-11-27.
11. Microsoft Teams — Manage audio settings in Microsoft Teams meetings — <https://support.microsoft.com/en-us/teams/meetings/manage-audio-settings-in-microsoft-teams-meetings> — FP — checked 2026-11-27.
12. Microsoft Teams — Use video in Microsoft Teams — <https://support.microsoft.com/en-us/teams/meetings/use-video-in-microsoft-teams> — FP — checked 2026-11-27 — preview and camera choice not described.
13. Microsoft Teams — Troubleshoot device features in Microsoft Teams on the web — <https://support.microsoft.com/en-us/teams/troubleshooting/troubleshoot-device-features-in-microsoft-teams-on-the-web> — FP — checked 2026-11-27.
14. BigBlueButton — Join the audio — <https://bigbluebutton.com/docs/help/user-guide/join-the-audio> — FP — checked 2026-11-27 — redirected from the former support address.
15. BigBlueButton — API Reference (join parameters) — <https://docs.bigbluebutton.org/development/api/> — FP — checked 2026-11-27 — first 100,000 characters read.
16. MDN — MediaDevices: getUserMedia() method — <https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getUserMedia> — FP (platform reference) — checked 2026-11-27.
17. web.dev — Capture audio and video in HTML5, and Recording video from the user — <https://web.dev/articles/getusermedia-intro>, <https://web.dev/articles/media-recording-video> — FP (platform guidance) — checked 2026-11-27 — search result summaries, pages not fetched.
18. MDN — MediaDevices: enumerateDevices() method — <https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/enumerateDevices> — FP (platform reference) — checked 2026-11-27 — empty labels inferred from the page's example output.
19. MDN — HTMLMediaElement: setSinkId() method — <https://developer.mozilla.org/en-US/docs/Web/API/HTMLMediaElement/setSinkId> — FP (platform reference) — checked 2026-11-27 — compatibility table did not load.
20. Chrome for Developers — Change the destination output device in Web Audio — <https://developer.chrome.com/blog/audiocontext-setsinkid> — FP (platform reference) — checked 2026-11-27.
21. W3C — Understanding Success Criterion 4.1.3: Status Messages (WCAG 2.2) — <https://www.w3.org/WAI/WCAG22/Understanding/status-messages.html> — FP (standard) — checked 2026-11-27.
