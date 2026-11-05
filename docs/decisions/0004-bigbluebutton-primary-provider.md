# ADR-0004: BigBlueButton as the primary conferencing provider

- Status: accepted, 2026-10-20, amended 2026-10-22; host size amended for the pilot by [ADR-0013](0013-pilot-deployment.md)
- Reviewed: pending line-by-line owner review; amended 2026-10-22 by the owner's decision to move recordings after the MVP: until then OVC creates rooms with `record=false` and registers no recording-ready callback, so no recording exists on the conferencing host (pending owner review)
- Deciders: project owner
- Supersedes: [ADR-0003](0003-embedded-video-provider.md) in its choice of primary provider; the decision to embed a provider behind an adapter stands
- Related: [ADR-0008](0008-hosting-and-data-residency.md), FR-5, [BigBlueButton research](../../knowledge/research/bigbluebutton-2026-10-20.md)

## Context

Lessons need a shared whiteboard, presentation upload and recordings that participants can rewatch (FR-5). Jitsi Meet, the provider ADR-0003 chose, has a whiteboard but records only through a separate recording service. That service runs one recording at a time per instance, so it scales poorly. BigBlueButton is built for teaching: whiteboard, presentations and recording processing are part of the server. Its stock recording playback links are public to anyone holding the link, its `meta_*` values can be visible to meeting users, and its documentation does not describe iframe embedding.

## Decision

Amendment approved 2026-11-05; wording pending owner review: local-first slice 7 development may precede the remote probe, starting with the [local provider foundation](../../knowledge/plans/slice-7-local-foundation.md). This changes implementation order only. Fake and local protocol evidence do not establish live BigBlueButton compatibility, media behavior, embedding or host capacity. Deployment and enabling embedding retain their live verification prerequisites.

- BigBlueButton is the primary and only production provider in the MVP. The `MeetingProvider` interface and the fake provider remain; a Jitsi adapter is not built.
- The backend creates a meeting with a random identifier when the tutor joins a lesson (a lesson can have several meetings over its life, one open at a time), with `record=false`, `autoStartRecording=false` and `allowStartStopRecording=false` until recordings land after the MVP, and with `record=true` and tutor-controlled recording afterwards (amended 2026-10-22, pending owner review). It sets `meetingEndedURL`, and once recordings land also `meta_bbb-recording-ready-url`, pointing at OVC with a per-meeting routing key, which is not a secret because `meta_*` values may be visible to participants. No secret or personal data goes into `meta_*` values.
- **Join links are a narrow exception to "no credentials in URLs".** A BigBlueButton join URL works like a bearer credential. OVC creates one per join request, after checking authorization, with the participant's role and the meeting's `createTime`. It returns the link only in the response body of a `POST`, and never stores or logs it. The link is bound to one meeting but is not single-use within it; the slice 7 probe evaluates joining with a session token instead.
- **Recording-ready callbacks** (after the MVP; the route is not registered until then) are authenticated only by an `HS256` JWT signed with the shared secret whose meeting matches the routing key. They are processed idempotently by recording identifier, and a periodic reconciliation with `getRecordings` in the worker backs them up. The meeting-ended callback is unsigned by the provider, so OVC treats it as a hint, confirms it with the provider, and never ends a lesson because of it.
- **Recordings are never served from the stock public playback link.** After the MVP, OVC copies each rendered recording into its own storage and serves it only through an authorized route. If the recordings probe, run when recordings are planned, shows that the rendered video format is unavailable, the fallback is a protected-recordings load balancer.
- **"Open in a new tab" is always available.** Embedding the client in the lesson page ships only if the slice 7 probe passes on the supported browsers, including Safari on iOS.
- The server is self-hosted in Russia on a dedicated host ([ADR-0008](0008-hosting-and-data-residency.md)).

## Alternatives considered

- **Jitsi Meet with its recording service:** lighter to embed, but recording needs one recorder instance per concurrent recording, and the whiteboard is less suited to teaching.
- **Both providers selectable:** double the integration and contract tests for no user-visible gain in the MVP.
- **A managed conferencing service:** per-minute costs and a foreign data processor, which conflicts with hosting in Russia.

## Consequences

- One dedicated server (8 cores, 16 GB RAM, 500 GB disk with recordings) is operated alongside OVC, except in the two-tutor pilot, which runs a 4-vCPU host ([ADR-0013](0013-pilot-deployment.md), pending owner review); it cannot run on developer machines, so all local development and automated tests use the fake provider.
- OVC owns recording storage, its quota and its retention, which makes deletion and access control reliable.

## Where it is applied

[Video specification](../../knowledge/specs/realtime-and-video.md#video), [recordings](../../knowledge/specs/realtime-and-video.md#recordings), [threat model](../threat-model.md), [roadmap slice 7](../roadmap.md#slice-7-video-lessons).
