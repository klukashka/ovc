# Non-functional requirements

Status: rewritten 2026-10-20 for the tutor-centric product and amended the same day after design review and for computer-first layouts (NFR-5.2); recordings and analytics marked as after the MVP on 2026-10-22 by the owner's decision, so requirements on recording copies and analytics apply once those land; pending line-by-line owner review. Numeric targets are proposals until the owner accepts them and a measured baseline exists for the component they constrain.

## NFR-1 Security and privacy

- **NFR-1.1** Every route and WebSocket frame enforces role and relationship scope on the server ([threat model](../threat-model.md)).
- **NFR-1.2** Passwords are stored only as Argon2id hashes. Tokens are stored as hashes (invitation tokens also encrypted for re-display) and never appear in logs. The only URLs that carry credentials are single-purpose links (invitation, confirmation, reset, unsubscribe, per-request conferencing join links) and 5-minute signed storage URLs, each bounded in lifetime and scope.
- **NFR-1.3** OVC collects only the personal data a feature needs, and the conferencing provider receives only the participant's name, an opaque participant identifier and a role.
- **NFR-1.4** All personal data is stored and processed in Russia ([ADR-0008](../decisions/0008-hosting-and-data-residency.md)), and no foreign service receives it.
- **NFR-1.5** Uploads are bounded in size, restricted to content-detected type allowlists, and counted against quotas.

## NFR-2 Performance

Proposed reference deployment for every target below: one API instance and one worker with 2 vCPU and 4 GB RAM each, PostgreSQL with 2 vCPU and 4 GB RAM on SSD, seeded with the NFR-3.1 data volume, measured from the same region. Targets are the 95th percentile over a declared workload, not a single sample.

- **NFR-2.1** Proposed: 95th-percentile latency under 200 ms for read routes and under 500 ms for write routes, excluding upload transfer, at MVP scale on the reference deployment.
- **NFR-2.2** Proposed: a chat message reaches every connected member of its channel within 1 second at the 95th percentile, with 2,000 connected sockets, 30-member group channels and 20 messages per second across the instance.
- **NFR-2.3** Upload, download and recording copy memory does not grow with file size.
- **NFR-2.4** Proposed: the initial student bundle stays under 250 KB compressed and the tutor bundle under 400 KB, excluding fonts. The builder, review and analytics load on demand.
- **NFR-2.5** Proposed: on a mid-range phone over a fast 4G profile, the student Home page shows the next lesson within 2.5 seconds (largest contentful paint).
- **NFR-2.6** (after the MVP, by the owner's decision of 2026-10-22) Proposed: analytics for an issue with 50 submissions and 100 questions return within 1 second at the 95th percentile on the reference deployment.

## NFR-3 Scale

- **NFR-3.1** MVP scale: 500 tutors, 5,000 students, 100 concurrent lessons, groups of up to 30 students, 2,000 concurrently connected users, on one API instance and one worker. Concurrent video capacity is bounded by the BigBlueButton host and is planned separately.
- **NFR-3.2** Default quotas: 5 GB of files per tutor, and 50 GB of recordings once recordings land after the MVP.

## NFR-4 Reliability

- **NFR-4.1** (the two-tutor pilot accepts losing up to about a minute of committed writes if its single VM's disk is lost, [ADR-0013](../decisions/0013-pilot-deployment.md), pending owner review) No acknowledged message, answer, submission, review or grade is lost: each is committed before success is reported.
- **NFR-4.2** No email or recording copy is lost across process restarts; delivery is at least once with idempotency keys ([ADR-0006](../decisions/0006-background-worker-and-email.md)).
- **NFR-4.3** A dropped WebSocket reconnects automatically and the client recovers missed data through REST.
- **NFR-4.4** The API and worker refuse to start on invalid configuration and shut down gracefully, finishing in-flight work.
- **NFR-4.5** Proposed: a lesson reminder is sent within 2 minutes of its due time while the worker runs, including when 1,000 other emails are queued and a recording copy is in progress.

## NFR-5 Accessibility and usability

- **NFR-5.1** The frontend meets WCAG 2.2 level AA in both themes, with contrast verified for every token pair ([tokens](../../knowledge/design/web/tokens.md#colours)).
- **NFR-5.2** Every page works from 360 px wide; every page, for both roles, is designed for computers first, and dedicated phone layouts follow in slice 10 ([ADR-0010](../decisions/0010-quiet-visual-language.md); amended 2026-10-20, pending owner review).
- **NFR-5.3** All times are shown in the viewer's timezone; every page and email exists in Russian and English.

## NFR-6 Maintainability and verification

- **NFR-6.1** Strict static checks and tests as defined in [AGENTS.md](../../AGENTS.md) pass before any change is done.
- **NFR-6.2** The full local check run finishes in under 10 minutes.
- **NFR-6.3** Every executable component emits structured diagnostics as defined in the [observability specification](../../knowledge/specs/observability.md).
- **NFR-6.4** The four [pilot scenarios](../../knowledge/design/web/pilot-scenarios.md) run as automated end-to-end tests before the MVP is complete.
