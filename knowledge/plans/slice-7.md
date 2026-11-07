# Slice 7: video lessons — start and implementation outline

Status: preparation requested by the owner on 2026-11-05; remaining local sequence approved on 2026-11-06 in the [local completion plan](slice-7-local-completion.md); the owner approved local-first L1 in the [foundation plan](slice-7-local-foundation.md), with later local application stages subsequently approved in that plan; provisioning remains unapproved. No remote test host is available. Baseline: main `e201e16f7dcc3382af0ebb19487f07f1a57f3048`, with [slice 6 accepted limitations](../reviews/slice-6-accepted-state.md). The owner requested requirements and a plan for the missing host. No spending or remote mutation is authorized.

## Goal and scope

Implement FR-5.1–FR-5.3: authorized tutor/student video-room joins, meeting lifecycle, provider-confirmed callbacks/reconciliation, and lesson UI with an always-available new-tab path. Embedding remains disabled until the required live-browser evidence passes. Recordings remain after the MVP. Lesson presentation/library integration remains slice 8. Follow [ADR-0004](../../docs/decisions/0004-bigbluebutton-primary-provider.md), [ADR-0013](../../docs/decisions/0013-pilot-deployment.md), the [video contract](../specs/realtime-and-video.md#video), schedule join rule and lesson-page design.

The integrated L1 provides a typed provider contract and bounded constructor-injected in-memory fake. Schedule lessons and their page, a `fake` setting used by test-control guards, and a provider-call lane identifier also exist. L2a adds durable lifecycle storage; L2b adds the shared local provider independently. No application join flow or remote BBB adapter is wired; subsequent local application integration follows the approved local completion plan.

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
- The items of the [remote-activation checklist](slice-7-remote-activation-checklist.md) recorded by the slice review on 2026-11-07.

Checks requiring a host remain blocked. Infrastructure procurement and live participants/devices are prerequisites for those checks. The owner explicitly approved local-first development on 2026-11-05: L1 now precedes the remote probe, while real-provider acceptance and enabling embedding remain gated by remote evidence.

## Implementation sequence and later remote verification

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

L1 was integrated as `0236b0b1594f3c6d0d2ab21a226c4dc131cb652e` on 2026-11-05 under the exact one-change [ADR-0018 exception](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#one-change-exception-for-the-local-meeting-foundation). Its gate passed 29 commands and 978/979 E2E; the inherited password-reset failure remains unresolved and the gate remains failed. [Evidence and integration identity](../reviews/slice-7-local-foundation.md#owner-approved-exception-integration-2026-11-05) preserve this distinction. The mandatory separate documentation record merged as `94b0322bdfd7f394394dd7f547a8c4166a58d608` after its reviews and ordinary documentation gate/finish. L2a persistence has since merged; authorized joins and worker integration still need concrete approved implementation plans. Real BBB acceptance remains unverified.

## Authorized lifecycle stage

The creation coordinator and the authorized lesson lifecycle API are implemented on their change branches: [coordinator record](../reviews/slice-7-meeting-create-coordinator.md), [lifecycle record](../reviews/slice-7-meeting-lifecycle-api.md). The fake room, lesson UI and BigBlueButton adapter remain pending under the [local plan](slice-7-local-completion.md). Nothing here is remote evidence.

## Durable lifecycle stage

The owner approved [L2a meeting persistence](slice-7-meeting-persistence.md) on 2026-11-05: database constraints, typed repository transitions, real concurrency/migration/purge tests and ordinary reviews/gate. It adds no API, provider process or UI. The later local provider must share state between API and worker; independent L1 fake instances are not a shared provider. Authorization/session checks, provider cleanup and reconciliation remain activation prerequisites. Implementation and check evidence belongs in the [persistence record](../reviews/slice-7-meeting-persistence.md); approval alone is not a passing result.

## Shared local provider stage

L2a merged as `48c9fad7beab9867cf64765612fd46cfb2b38045` after an ordinary full gate: all 30 commands and 979 E2E cases passed, and finish confirmed the tested main tree. This result does not resolve the historical slice 6 failure cause.

The owner approved [L2b shared provider](slice-7-shared-fake.md) on 2026-11-05: PostgreSQL-owned fake infrastructure, cross-process capabilities, global bounds, real migration/concurrency/process tests and ordinary independent reviews/gate. This supplies shared state for later API/worker integration without an additional server. Authorized routes, the specified HTTP fake room, process wiring, purge/reconciliation and real BBB acceptance remain later work. Runtime evidence is recorded in the [L2b verification record](../reviews/slice-7-shared-fake.md).

## Provider generation discovery

L2b merged as `0f14087e92f2e9e89a3efd82209733aa011e0048` with an ordinary successful gate: all 30 commands and 979 E2E cases passed, and finish confirmed the tested tree. The owner next approved the [read-only discovery plan](slice-7-provider-discovery.md) on 2026-11-05. It adds lookup of the current provider generation using only its opaque identifier, without activating application joins. Durable cleanup ownership, late-Create handling and reconciliation must still precede API/worker activation. Evidence belongs in the [discovery verification record](../reviews/slice-7-provider-discovery.md).

## Durable recovery ownership stage

Provider discovery merged as `01c3f8e1e8b78e8316cf1e09718d7cf89e02e11a` after all 30 gate commands and 979 E2E cases passed; finish confirmed the tested main tree. The owner approved the [meeting recovery ledger](slice-7-meeting-recovery-ledger.md) on 2026-11-05. It preserves provider identity and single-attempt evidence independently of purged business metadata, refuses late publication and supplies bounded candidate reads. Independent security planning review passed with the provenance and downgrade clarifications incorporated. Implementation evidence belongs in the [verification record](../reviews/slice-7-meeting-recovery-ledger.md).

This storage stage does not activate cleanup, worker jobs or joins. The next plan must define the coordinator, capacity/retention and operator recovery before application activation. Local fake evidence still does not establish BBB compatibility or media acceptance.

The ledger merged as `be8290afed843561e1b02a6a93e96f54b7101cea` under the owner's exact [ADR-0018 exception](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#one-change-exception-for-the-meeting-recovery-ledger) on 2026-11-06: 29 gate commands passed, E2E passed 978/979, and the failed Chromium password-reset observation with unknown HTTP/console diagnostics remains unresolved. No retry or normal application finish was used. The [disposition plan](slice-7-ledger-known-failure-exception.md) requires the immediately following documentation reviews and ordinary docs-only gate/finish before further application implementation; that documentation result does not repair or certify the failed application suite.

## Approved local completion sequence

The owner approved the [complete remaining local sequence](slice-7-local-completion.md) on 2026-11-06 after independent standard/security planning reviews. The ledger exception documentation merged as `8664fabbae992844c6be1aef986dcb5b30b9e508` after its own ordinary documentation gate and finish. The next work implements recovery storage and coordinator, lifecycle API, guarded fake room, lesson UI/pilot coverage and the BBB adapter against local protocol fixtures. New contract deviations and failed-gate exceptions still need explicit decisions; routine coherent splitting within the approved contracts does not.

The first implementation change supplies capacity, durable cleanup-cycle outcomes, safe settlement and cursor persistence. Runtime reconciliation, provider lane, operator commands and authorized public routes remain pending until their corresponding changes integrate. Each change has independent reviews and its own ordinary gate. No remote provider is activated by these storage operations.

## Recovery persistence integration and terminal prerequisite

The compression dependency correction merged as `509fd35e3b565daae92da6c0d686d60169660c33`, then recovery storage as `658be0514fca80c5264cf9981547003fc1a4fa71`. Each passed all 30 ordinary gate commands and 979 E2E; both finishes confirmed exact tested/main tree equality. The earlier failures remain preserved in their records.

The next bounded terminal-persistence prerequisite merged as `d5961bfcd7a31c8070debb52c97498ca8d93d21d` under the owner's [exact exception](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#one-change-exception-for-meeting-cleanup-terminal-persistence): 29 gate commands passed, E2E passed 978/979, and the Chromium password-reset body-observation failure with unknown HTTP/console diagnostics remains unresolved. No repeat or ordinary application finish occurred. The immediately following mandatory [documentation disposition](slice-7-terminal-known-failure-exception.md) requires independent standard/security reviews and an ordinary docs-only gate/finish before continuing implementation.

The integrated prerequisite supplies atomic lease-fenced cleanup outcomes and expiry handling. Creation coordinator, bounded minute recovery traversal, operator commands, provider-lane activation and authorized meeting API/UI remain pending under the approved local sequence. No real server or media acceptance is claimed.

### Cleanup interleaving integration disposition

The local cleanup reinspection follow-up merged as `05a79d2207c1573617f9ba4838ae7325b0f2280a` from `5d3bfe31965a61cd861c3a10704c06dce41ea41b` under the [exact owner exception](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#one-change-exception-for-cleanup-interleaving-tests). Its 29 successful commands and 978/979 E2E are not a passing full gate. The password-reset timeout and unknown diagnostics remain unresolved; no repeat or ordinary application finish occurred. The [mandatory disposition record](slice-7-interleavings-known-failure-exception.md) requires independent standard/security review and an ordinary documentation gate/finish before further implementation. The remaining BBB-specific coverage limitation is retained; this integration does not close the slice or activate BBB.

### BigBlueButton adapter prepared locally

The [HTTP adapter](../specs/realtime-and-video.md#bigbluebutton-adapter-prepared-locally) exists behind the provider interface and passes the shared contract against a local TLS protocol fake. `bbb` remains refused at start-up. Real BBB acceptance remains unverified until the approved remote probe.
