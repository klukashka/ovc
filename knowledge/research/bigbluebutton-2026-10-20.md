# BigBlueButton: integration facts checked on 2026-10-20

Evidence for [ADR-0004](../../docs/decisions/0004-bigbluebutton-primary-provider.md) and the [video specification](../specs/realtime-and-video.md#video). Facts are marked **verified** (read in the official documentation on the date above) or **unverified** (believed, to be confirmed by a probe in [roadmap slice 7](../../docs/roadmap.md#slice-7-video-lessons) before the adapter relies on them).

The [adapter facts checked on 2026-11-07](bigbluebutton-api-2026-11-07.md) add the exact protocol details the HTTP adapter relies on.

## Sources

- [API reference](https://docs.bigbluebutton.org/development/api/), version 4.0 selected.
- [Server installation](https://docs.bigbluebutton.org/administration/install/), version 4.0.
- [Moodle tracker MDL-75049](https://tracker-old.moodle.org/browse/MDL-75049) on recording access.

## Server

- **Verified.** The current release is 4.0, installed with `bbb-install.sh` on a clean Ubuntu 24.04 server. In-place upgrades from 2.6, 2.7 or 3.0 are not supported.
- **Verified.** Production minimum: 8 CPU cores with high single-thread performance, 16 GB RAM with swap, 500 GB disk with recordings (50 GB without), 250 Mbit/s symmetric bandwidth. Development minimum: 4 cores, 8 GB RAM, 50 GB disk.
- **Verified.** TCP 80 and 443 must be free on the host (no other web server), UDP 16384–32768 open, a hostname with a TLS certificate. Docker must be installed because document conversion runs sandboxed in it.
- **Consequence.** BigBlueButton cannot run on a developer's macOS machine. Local development and all automated tests use the fake meeting provider; live checks use a remote test server.

## API calls and parameters

- **Verified.** Every call carries a `checksum` computed from the call name, the query string and the shared secret. SHA-1 is the default; SHA-256, SHA-384 and SHA-512 are supported since 2.5, controlled by `supportedChecksumAlgorithms`.
- **Verified.** `create` parameters used by OVC:
  - `record`: allows recording. A moderator starts it unless `autoStartRecording` is set.
  - `allowStartStopRecording`: default true; must stay true for tutor-controlled recording once recordings land after the MVP; until then OVC sends `record=false` and `allowStartStopRecording=false` (decided 2026-10-22).
  - `meetingExpireIfNoUserJoinedInMinutes`, `endWhenNoModerator`, `endWhenNoModeratorDelayInMinutes`.
  - `disabledFeatures`: turns off unused client features.
  - `logoutURL`.
  - `preUploadedPresentation`: a URL, fetched and converted in the background.
  - `meetingEndedURL`: called with `GET` when the meeting ends, with `recordingmarks=true|false` appended.
- **Verified.** `meta_endCallbackUrl` is deprecated because `meta_*` values can be exposed to meeting users. **Consequence:** OVC never places secrets or personal data in `meta_*` values.
- **Verified.** `join` parameters:
  - `role`: `MODERATOR` or `VIEWER`; replaces the deprecated passwords.
  - `fullName`: required, up to 255 characters.
  - `userID`: echoed by `getMeetingInfo`.
  - `createTime`: must match the meeting's create time, so a join URL cannot be reused for a later meeting with the same identifier.
  - `errorRedirectUrl`: since 2.7.
  - `redirect=false`: returns XML with a `session_token` instead of redirecting.
- **Verified.** Meeting state: `isMeetingRunning`, `getMeetingInfo` (attendees, recording state), and `end`, which is asynchronous, so its effect is confirmed with `getMeetingInfo`.
- **Verified.** Recording calls:
  - `getRecordings`: filters `meetingID`, `recordID`, `state` (`processing`, `processed`, `published`, `unpublished`, `deleted`, `any`), `meta`, `offset`, `limit` (1–100), and reports `totalElements` since 2.7;
  - `publishRecordings`;
  - `deleteRecordings`: `recordID`, comma-separated;
  - `updateRecordings`: `recordID` plus `meta_*` to change metadata;
  - `getRecordingTextTracks`.

## Recording-ready signal

- **Verified.** With `meta_bbb-recording-ready-url` set at create time, the server POSTs a form field `signed_parameters` to that URL when a recording is ready. The field is an HS256 JWT signed with the shared secret, containing `meeting_id` and `record_id`.
- **Verified.** How the server reads the receiver's response:
  - 2xx: success;
  - 3xx: an error;
  - 401: secret mismatch;
  - 410: the recording was deleted.
- **Consequence.** OVC verifies the JWT, upserts by `record_id` idempotently, and also reconciles periodically with `getRecordings` in the worker in case a callback was lost or the backend was unreachable.

## Recording access

- **Verified (third-party source).** On a stock server, recording playback links are open to anyone who has the link. Access restriction ("protected recordings") is implemented by load balancers such as Scalelite, not by stock BigBlueButton.
- **Unverified.** The exact protected-recording parameters of Scalelite.
- **Unverified.** Whether the `video` playback format (a single rendered video file, from the separate video playback package) is available and stable on 4.0.
- **Consequence for OVC.** Never expose the stock playback link. The adapter's preferred path is to copy the rendered video file of each recording into OVC's own object storage and serve it only through an authorized OVC route with short-lived signed URLs. It then deletes the server-side copy once the transfer is verified. If the `video` format is unavailable, the fallback is to put the server behind a load balancer with protected recordings. The recordings probe after the MVP decides between the two and records the result here.

## Embedding the client

- **Verified.** The API documentation describes `redirect=false`, which lets an application show the client itself. It says nothing about iframes, permissions or framing headers.
- **Unverified.** Embedding in an iframe requires:
  - the iframe `allow` attribute for camera, microphone, display capture, fullscreen and autoplay;
  - a `frame-ancestors` policy on the BigBlueButton web server that admits the OVC origin;
  - same-site hosting (a subdomain of the OVC domain), so browsers that block third-party cookies keep the session.
- **Consequence.** "Open in a new tab" is required regardless of embedding. Embedding ships only if the slice 7 probe passes on current Chromium, Firefox, Safari on macOS and Safari on iOS.

## Open probe items for slice 7

Each item is unverified and is answered by the slice 7 probe on a remote test server, with the result recorded in this file.

- With `record=false`, `autoStartRecording=false` and `allowStartStopRecording=false` (OVC's MVP settings, decided 2026-10-22): a moderator sees no start-recording control, the server refuses to start one, and `getRecordings` with `state=any` is empty after the meeting.
- Capacity of the pilot host ([ADR-0013](../../docs/decisions/0013-pilot-deployment.md)): CPU, memory and audio quality on 4 vCPU / 8 GB with two simultaneous one-to-one lessons with webcams, and the VM's bandwidth against the official 250 Mbit/s.
- The exact names and effect of the host settings that disable recording by default (`disableRecordingDefault`) and turn off meeting event retention, so the host keeps no raw meeting events.
- The provider's retry behavior when a callback receives a non-2xx answer.
- Whether `join` with `redirect=false` and the returned session token can replace handing the browser a reusable join URL.
- What happens when two clients join with the same `userID`.
- Which external hosts the HTML5 client contacts (for example STUN servers), for data-residency review.
- Embedding on current Chromium, Firefox, Safari on macOS and Safari on iOS with the headers described above.

## Open probe items for recordings, after the MVP

- Whether the rendered single-file recording format is available on 4.0, and its file type; this decides between copying recordings and a protected-recordings load balancer.
