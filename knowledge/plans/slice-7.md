# Slice 7: video lessons — start and implementation outline

Status: preparation requested by the owner on 2026-11-05; overall implementation outline proposed; the owner approved local-first L1 in the [foundation plan](slice-7-local-foundation.md), not later application stages or provisioning. No remote test host is available. Baseline: main `e201e16f7dcc3382af0ebb19487f07f1a57f3048`, with [slice 6 accepted limitations](../reviews/slice-6-accepted-state.md). The owner requested requirements and a plan for the missing host. No spending or remote mutation is authorized.

## Goal and scope

Implement FR-5.1–FR-5.3: authorized tutor/student video-room joins, meeting lifecycle, provider-confirmed callbacks/reconciliation, and lesson UI with an always-available new-tab path. Embedding remains disabled until the required live-browser evidence passes. Recordings remain after the MVP. Lesson presentation/library integration remains slice 8. Follow [ADR-0004](../../docs/decisions/0004-bigbluebutton-primary-provider.md), [ADR-0013](../../docs/decisions/0013-pilot-deployment.md), the [video contract](../specs/realtime-and-video.md#video), schedule join rule and lesson-page design.

The integrated L1 provides a typed provider contract and bounded constructor-injected in-memory fake. Schedule lessons and their page, a `fake` setting used by test-control guards, and a provider-call lane identifier also exist. No application join flow, shared multiprocess fake or remote BBB adapter is wired. L2a implements durable lifecycle storage first; subsequent application and provider integration remains subject to separate approval.

Tier: slice for the overall plan and final cumulative review; individual implementation changes use standard with separate security review. Security trigger: meeting authorization, credential-bearing join links, outbound provider calls, untrusted callbacks, framing, remote personal-data processing and secret handling. Planning needs independent standard and full-security review before approval; no live check is represented as passed.

## Remote prerequisites and probe, before live-provider acceptance

Prepare the server specification from the [dated official-source check](../research/bigbluebutton-probe-prerequisites-2026-11-05.md). The existing pilot decision is a dedicated Russian-region 4-vCPU/8-GB/50-GB Ubuntu host for at most two simultaneous one-to-one lessons. Compare it to the vendor production recommendation and report the accepted capacity risk. Use separate BBB and test-OVC origins under a controlled same-site domain, valid TLS, key-only operator SSH and the exact provider-required firewall rules. Do not place a real production database or students on this test host.

Before provisioning obtain approval of exact host/provider/cost, domain, public exposure and secret-access method. No cloud purchase, credential handling, firewall change or installation occurs under this draft. Write a bounded probe protocol with named checks, fixed duration/sample count, cleanup, redaction and acceptance thresholds before running it; unresolved measurements remain unverified, not retries until green.

The probe must cover:

- Two simultaneous tutor/student lessons with webcams, audio, screen sharing and whiteboard on the pilot-size host; collect declared bandwidth/resource/audio-quality evidence and separately state hardware/network limitations.
- Supported Chromium, Firefox, actual Safari on macOS and actual Safari on iOS, including camera/microphone permission, embedding and new-tab fallback. Playwright WebKit is not a substitute for device evidence.
- No recording controls, no recordings created and no retained raw meeting events; confirm actual configuration names rather than relying on assumptions.
- SHA-256-or-stronger provider signing; join create-time binding, duplicate participant identifiers, session-token alternative and provider request/redaction behavior.
- Unsigned ended callback behavior and reconciliation; callback retry behavior recorded rather than relied upon.
- External network destinations, especially STUN/TURN, before admitting user data; authorized Russian processing only.
- The carried slice 4 2,000-socket reconnect test over a real network, with a distinct bounded plan and ownership of the OVC test deployment.

Checks requiring a host remain blocked. Infrastructure procurement and live participants/devices are prerequisites for those checks. The owner explicitly approved local-first development on 2026-11-05: L1 now precedes the remote probe, while real-provider acceptance and enabling embedding remain gated by remote evidence.

## Implementation sequence, with per-change approval and later remote verification

1. **Contracts and domain:** start from the video/API/schedule/data-model specifications and reconcile later probe findings before remote acceptance; propose any discrepancy before the affected implementation. Define typed meeting/participant states, migration constraints for at most one open meeting, settings and package-layer entries. Tests prove constraints, join windows and scoped membership.
2. **Adapters:** implement BigBlueButton and configured fake behind one interface, shared contract checks and the specified local HTTP API fake. Test checksum, required recording-off flags, bounded requests/responses, deadlines, malformed/provider failures and redacted diagnostics. Remote contract tests are opt-in and unverified until executed.
3. **Backend lifecycle:** tutor-only creation, race-safe concurrent joins, student refusal before live, end/restart, persisting first_join_at, stale creating reconciliation, provider-call worker tasks and unsigned hint confirmation. Exercise real API/PostgreSQL and clock-driven worker paths; preserve authorization/session rechecks and lock order. Do not hold business locks across provider waits unless explicitly contracted and reviewed.
4. **Frontend:** typed join response, live/room-open state, focused error/loading states, new-tab fallback and conditionally enabled iframe. Test both roles, denied/ended/restarted rooms, offline behavior, privacy and supported browser journeys. No join credential in caches, telemetry or logs.
5. **Acceptance:** per-change reviews/checks/gates, relevant slice 6 regression tracking, remote probe evidence, real-network reconnect and final per-layer reviews. Record all limitations and reconcile affected earlier findings; do not declare slice 7 complete on local fakes alone.

## Expected affected files and boundaries

Backend domain/repositories/services/API routes, migrations/queries, worker provider lane, core settings and adapter packages; frontend schedule lesson page and typed API schema; `.env.example`, package-layer map, video/schedule/API/data-model specifications, threat model and development guide. Exact files and checkable test names are finalized per small approved local change; remote findings may require separately approved follow-ups. No dependency is added without a recorded reason. Every new setting is typed and documented; secrets remain backend-only.

## Security coverage to refine before each implementation approval

| Threats | Required boundary and negative evidence |
| --- | --- |
| T-10, T-21 | Join authorization and current membership; refuse foreign lessons, removed members, early joins and students before the tutor. Recheck scope after relevant waits. |
| T-11, T-12, T-26 | Recording stays off and its callback route is absent; forged, duplicated and replayed ended hints never end a lesson without provider confirmation. Routing keys are not credentials. |
| T-16, T-24, T-45 | Restricted iframe origin and permissions; the conferencing subdomain cannot forge OVC refresh or WebSocket access. Embedding remains opt-in after real device proof. |
| T-18, T-19 | Only approved participant fields leave OVC; audit external media destinations and redact credentials from logs, artifacts and provider configuration. Approve a safe secret-loading mechanism before remote operations. |
| T-25 | Join addresses are issued only after authorization, never persisted by OVC, and bound to the meeting create time. Test old-link behavior after room replacement; report the provider's residual reuse behavior. |
| T-20 | Bound provider HTTP bodies, deadlines, concurrency and hint-triggered work; test unavailable provider, malformed responses, repeated hints and cleanup. |
| T-36 | Isolate disposable remote probe data and control surfaces from production; do not make fake-only local test-control exceptions available with the production provider. |
| T-41 | Separate live probe, local fake evidence and unverified outcomes; keep the accepted slice 6 gaps visible and preserve failed evidence. |

Each implementation plan must name its exact threat-model amendment, negative tests and cleanup assertions before approval. This outline does not approve production boundary changes; the present transition amendment covers only acceptance reporting and artifact privacy.

## Work coordination and immediate deliverable

At most three working sessions total including other projects; one heavy check at a time. Start with the approved local provider foundation; server prerequisites no longer block local implementation. Remote tests have explicit authority and safe load boundaries and do not consume the local computer concurrently with heavy suites. Record current slice 6 uncertainty without resuming its diagnostics automatically. Immediate acceptance is a durable slice 6 disposition and this proposed slice 7 plan with a clear missing-host prerequisite; it is not delivered video functionality.

## Local foundation progress

L1 was integrated as `0236b0b1594f3c6d0d2ab21a226c4dc131cb652e` on 2026-11-05 under the exact one-change [ADR-0018 exception](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#one-change-exception-for-the-local-meeting-foundation). Its gate passed 29 commands and 978/979 E2E; the inherited password-reset failure remains unresolved and the gate remains failed. [Evidence and integration identity](../reviews/slice-7-local-foundation.md#owner-approved-exception-integration-2026-11-05) preserve this distinction. The mandatory separate documentation record must pass its reviews and documentation gate before further application implementation. L2 persistence, authorized joins and worker integration still need a concrete approved implementation plan; real BBB acceptance remains unverified.

## Durable lifecycle stage

The owner approved [L2a meeting persistence](slice-7-meeting-persistence.md) on 2026-11-05: database constraints, typed repository transitions, real concurrency/migration/purge tests and ordinary reviews/gate. It adds no API, provider process or UI. The later local provider must share state between API and worker; independent L1 fake instances are not a shared provider. Authorization/session checks, provider cleanup and reconciliation remain activation prerequisites. Implementation and check evidence belongs in the [persistence record](../reviews/slice-7-meeting-persistence.md); approval alone is not a passing result.
