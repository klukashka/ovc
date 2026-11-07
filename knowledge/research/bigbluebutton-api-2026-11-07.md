# BigBlueButton API facts for the HTTP adapter, checked on 2026-11-07

Evidence for the [prepared adapter](../specs/realtime-and-video.md#bigbluebutton-adapter-prepared-locally). This records documentation and source reading only: no BigBlueButton server was contacted, and no statement here is live compatibility evidence. It complements the [earlier integration facts](bigbluebutton-2026-10-20.md).

## Sources

- **Documentation:** the [official API reference](https://docs.bigbluebutton.org/development/api/), page version 4.0, read on the date above through targeted extraction of the sections used here, so unrelated sections were not reviewed.
- **Source:** the official repository's branch [`v4.0.x-release`](https://github.com/bigbluebutton/bigbluebutton/tree/v4.0.x-release), files `ApiController.groovy`, `ValidationService.java`, `MeetingExistsConstraint.java`, `MeetingEndedConstraint.java`, `MeetingEndedValidator.java`, `MeetingInfo.java` and `EndMeeting.java`. Source facts are implementation details of that branch, not a documented contract, and are marked below.

## Documented facts

- Every call carries a checksum over the call name, the query string without `checksum`, and the shared secret. `supportedChecksumAlgorithms` lists what the server accepts, by default SHA-1, SHA-256, SHA-384 and SHA-512; removing an algorithm refuses it. The page does not say how a caller names the algorithm.
- String values must be correctly URL-encoded UTF-8 and contain no control characters. Booleans are lowercase. The checksum example encodes a space as `+`.
- `create` accepts `GET` and `POST`; for `POST` the checksum covers the URL query string.
- `create` is idempotent for an active meeting: with identical other parameters it succeeds with `messageKey` `duplicateWarning`. Its response has `meetingID`, `createTime` and `hasBeenForciblyEnded`.
- `name` must have 2 to 64 characters. `meetingID` must have 2 to 256 characters, without commas, and should hold only ASCII letters, digits, dashes and underscores.
- `record` and `autoStartRecording` default to false; `allowStartStopRecording` defaults to true. The page warns against sending `autoStartRecording=false` with `allowStartStopRecording=false` because the moderator then cannot start recording. OVC wants exactly that effect in the MVP, so the warning is no contradiction.
- `meetingEndedURL` receives a `GET` with `meetingID` and `recordingmarks` appended to its existing query string.
- `meetingExpireIfNoUserJoinedInMinutes` defaults to 5, `endWhenNoModerator` to false and `endWhenNoModeratorDelayInMinutes` to 1.
- `getMeetingInfo` requires only `meetingID`. Its example response has `createTime`, `running`, `participantCount`, `moderatorCount`, `endTime`, `hasBeenForciblyEnded` and `recording`. The page does not document the answer for a missing meeting or how long an ended meeting stays visible.
- `end` requires only `meetingID`; `password` is deprecated. It has no `createTime` parameter. It is asynchronous: success means the request was sent, and the caller confirms with `getMeetingInfo` or `isMeetingRunning` a few seconds later.
- `join` requires `fullName` (up to 255 characters), `meetingID` and `role` (`MODERATOR` or `VIEWER`). When `createTime` is sent and differs from the session's, the server does not proceed. `userID` is echoed by `getMeetingInfo`. By default the browser is redirected to the client.

## Facts read from source only

- `ValidationService` maps `create` and `join` to `GET` requests. `getMeetingInfo` and `end` validate `meetingID` with a constraint whose default key is `notFound`. `EndMeeting` has no `createTime` field.
- `meetingForciblyEnded` is the default key of the constraint refusing an ended meeting. That validator tests `isForciblyEnded()` only.
- `create` looks up an existing meeting with `getNotEndedMeetingWithId`, so an ended identifier can create a new generation. It answers `duplicateWarning` when the passwords match or neither is sent, and otherwise `idNotUnique`. The adapter sends no passwords, so a repeat always answers `duplicateWarning`. A differing name or callback address is therefore not detected by the server, despite the documentation's wording.
- `buildQueryStringFromParamsMap` rebuilds a query from sorted keys with Java's UTF-8 URL encoder. That encoder keeps `*` and encodes `~`, the reverse of Go's `url.QueryEscape`. The adapter therefore sends sorted keys in the Java form, so raw and rebuilt queries are byte-identical.

## Consequences for the OVC contract

- **End cannot name a generation.** The adapter reads the current `createTime`, ends only when it matches, and refuses a difference. This is check-then-act, not atomic. It is sufficient only because OVC dispatches one Create per immutable provider identifier ([ADR-0020](../../docs/decisions/0020-durable-meeting-recovery.md)), so no second generation can appear for that identifier.
- **Replaying a Create could produce a second generation.** Go's transport may replay a `GET` on a reused connection. The adapter disables connection reuse, and a test proves one Create per call.
- **End success is not termination.** The adapter returns success only after reading absence or an ended state, within its deadline. Otherwise it reports a retryable failure.
- **Ended rooms may stay visible.** A positive `endTime` or `hasBeenForciblyEnded=true` maps to `not_found`.

## Unverified until the remote probe

Each item needs a live server: the answer and retention of `getMeetingInfo` after `end`, the time an `end` takes, the absence of recording controls, actual acceptance of the Java-form encoding and of SHA-256 with SHA-1 removed, TLS and redirect behavior of a real host, whether an ended generation can mask a live replacement in the `getMeetingInfo` and `end` lookup, whose finder was not read, and every item in the [earlier open probe list](bigbluebutton-2026-10-20.md#open-probe-items-for-slice-7).
