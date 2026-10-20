# ADR-0003: Embed an open-source conferencing provider for video lessons

- Status: accepted, 2026-10-20; primary provider superseded by [ADR-0004](0004-bigbluebutton-primary-provider.md) on 2026-10-20 (the embedding-behind-an-adapter decision stands)
- Reviewed: pending line-by-line owner review
- Deciders: project owner
- Related: [ADR-0002](0002-implementation-stack.md), [video specification](../../knowledge/specs/realtime-and-video.md#video), FR-5
- Superseded parts: the choice of Jitsi Meet as primary provider, token-gated Jitsi rooms and the public Jitsi instance for development ([ADR-0004](0004-bigbluebutton-primary-provider.md)); the course-based join authorization and the statement that students may be minors ([ADR-0005](0005-tutor-centric-domain.md)). The text below is kept as written for the record.

## Context

Lessons need multi-party live video with screen sharing and moderator controls, comparable to mainstream meeting products. Real-time media (WebRTC signaling, STUN and TURN, a selective forwarding unit, bandwidth adaptation, scaling) is the hardest and most operationally expensive part of such a system. Built naively it is reliable only for very small groups; built properly it is months of work and permanent operations. Students may be minors, so self-hosting the media path is valuable for privacy.

## Decision

Do not build a media stack. Embed an open-source conferencing platform and keep it behind one `MeetingProvider` adapter interface with a fake implementation for tests:

- **Jitsi Meet** is the primary provider: embeddable through its external API, self-hostable, with token-gated private rooms.
- **BigBlueButton** is the supported alternative: education-focused (whiteboard, breakout rooms, recordings), with a server-side create, join and end API; heavier to host.

The backend creates one meeting per lesson with a random room name, authorizes joins by course membership, makes the course teacher the moderator, and returns join details. Media never passes through the backend.

## Alternatives considered

- **Build WebRTC with our own signaling and media servers:** full control, but the largest engineering and operations risk in the product, for no user-visible gain at MVP scale.
- **Managed video SDK (for example LiveKit Cloud, Daily or Twilio):** reliable and feature-rich, but per-minute cost, an external processor for minors' data, and vendor lock-in.
- **Links to a third-party meeting product:** no integration work, but no control over who joins and no moderator mapping.

## Consequences

- A conferencing deployment must be operated or reached in production; development can use the public Jitsi instance or the fake provider.
- Private rooms require token-gated deployment configuration; without it, rooms rely on unguessable names, which is acceptable only for development.
- Recording and advanced features depend on the chosen deployment's configuration, not on OVC code.
- The adapter keeps the choice reversible; a managed SDK can replace it later through a new decision record.

## Where it is applied

[Video specification](../../knowledge/specs/realtime-and-video.md#video), [provider interface](../../knowledge/specs/realtime-and-video.md#provider-interface), [threat T-10](../threat-model.md#threats-and-controls), [roadmap slice 7](../roadmap.md#slice-7-video-lessons-and-recordings).
