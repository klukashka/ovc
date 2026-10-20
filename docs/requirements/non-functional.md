# Non-functional requirements

Status: draft written 2026-10-20; pending line-by-line owner review. Numeric targets are proposals until the owner accepts them and a measured baseline exists for the component they constrain.

## NFR-1 Security and privacy

- **NFR-1.1** Every route and WebSocket frame enforces role and ownership on the server ([threat model](../threat-model.md)).
- **NFR-1.2** Passwords are stored only as Argon2id hashes; tokens are never placed in URLs or logs.
- **NFR-1.3** Students may be minors: only the personal data a feature needs is collected, and none of it is sent to the conferencing provider beyond a display name.
- **NFR-1.4** Uploads are bounded in size and restricted to an allowlist of content types.

## NFR-2 Performance

- **NFR-2.1** Proposed: 95th-percentile latency under 200 ms for read routes and under 500 ms for write routes, excluding upload transfer time, at MVP scale on the reference deployment.
- **NFR-2.2** Proposed: a chat message reaches every connected member of its channel within 1 second at the 95th percentile.
- **NFR-2.3** Upload and download memory use does not grow with file size.
- **NFR-2.4** Proposed: the initial frontend bundle stays under 300 KB compressed, excluding the conferencing embed, which loads only when joining a lesson.

## NFR-3 Scale

- **NFR-3.1** MVP scale: up to 50 concurrent lessons, 200 members per course, 2,000 concurrently connected users on one backend instance. Conferencing capacity is the provider deployment's concern.

## NFR-4 Reliability

- **NFR-4.1** No acknowledged chat message, submission or grade is lost: each is committed to the database before success is reported.
- **NFR-4.2** A dropped WebSocket reconnects automatically and the client recovers missed messages from history.
- **NFR-4.3** The backend refuses to start on invalid configuration and shuts down gracefully, closing connections.

## NFR-5 Accessibility and usability

- **NFR-5.1** The frontend meets WCAG 2.2 level AA: keyboard operation, visible focus, labelled controls, sufficient contrast.
- **NFR-5.2** Layouts work at phone, tablet and desktop widths.
- **NFR-5.3** All times are shown in the viewer's timezone.

## NFR-6 Maintainability and verification

- **NFR-6.1** Strict static checks and tests as defined in [AGENTS.md](../../AGENTS.md) pass before any change is done.
- **NFR-6.2** The full local check run finishes in under 10 minutes.
- **NFR-6.3** Every executable component emits structured diagnostics as defined in the [observability specification](../../knowledge/specs/observability.md).
