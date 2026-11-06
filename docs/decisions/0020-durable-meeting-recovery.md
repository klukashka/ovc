# ADR-0020: Durable meeting recovery and retained uncertainty

- Status: accepted, 2026-11-06
- Reviewed: local implementation scope approved by the project owner on 2026-11-06; wording pending line-by-line owner review
- Deciders: project owner
- Related: [ADR-0004](0004-bigbluebutton-primary-provider.md), [ADR-0006](0006-background-worker-and-email.md), [ADR-0008](0008-hosting-and-data-residency.md), [approved local plan](../../knowledge/plans/slice-7-local-completion.md)
- Amends: ADR-0008's bounded-retention statement for minimal unresolved meeting recovery obligations and their explicit cleanup-cycle receipts; hosting and residency requirements are unchanged

## Context

A provider may create a room after its initiating request times out or after its business lesson is purged. A missing room observed once does not prove that a previously dispatched Create cannot appear later. Deleting the business row, retrying creation under the same identity or forgetting an exhausted outbox entry can lose cleanup responsibility or publish an unintended generation.

The local ledger already preserves one-attempt evidence and survives business purge. This decision defines its admission, settlement and recovery contract before application routes activate it. Local fake evidence does not establish remote provider guarantees.

## Decision

Reserve one of 4096 global unsettled obligations atomically with the business meeting and ledger creation. Open rooms also consume capacity. A full deployment refuses new creation with `503 service_unavailable` before the provider call. Backfill retains every existing obligation; a count above the limit blocks new admission without deleting records. Rollback releases an uncommitted reservation; terminal settlement releases a committed reservation exactly once.

Use the total database lock order: existing scope and lesson locks, capacity, business meeting, ledger, then outbox. Ledger-only recovery never subsequently locks business rows. No provider call holds a business transaction or connection. Only a committed Begin transition permits one Create dispatch per immutable cryptographically random provider ID. An uncertain outcome never authorizes another Create for that ID.

A returned provider generation is recorded before publication and remains recordable after closure or purge. Publication additionally requires current authorization, session, lesson and generation checks. Unreturned links are discarded after a failed recheck; cleanup ownership remains. No timestamp or lease expiry proves provider quiescence.

Each automatic cleanup cycle has one stable outbox identity, at most eight attempts with the existing backoff, and a durable terminal result independent of outbox retention. At most one additional final automatic cycle follows a definitive late Create acknowledgement. Exhaustion never replenishes itself. Local operator retries are explicit, revision-checked, separately identified bounded cycles with retained audit receipts; there is no force-forget or force-settle operation. Repeated deliberate operator actions can grow receipt history, so the obligation count is not a bound on all database bytes. Receipt inspection is indexed and paginated.

Cleanup inspects the provider identity and ends only the observed, exact generation. A conflicting generation is retained for investigation, never destroyed as a substitute for the owned generation. Only an irrevocably revoked prepared attempt or an acknowledged returned generation with confirmed End/absence and no possible later dispatch may settle. Started, uncertain and legacy-uncertain obligations remain unsettled even after absence or End of a discovered room. They retain capacity and cleanup responsibility until a separately approved reconciliation procedure establishes terminal evidence.

Recovery scans once per minute, at most 100 candidates per page, with a persistent compare-and-swap cursor and a 20-second monotonic pass deadline including database waits. Failed candidates do not pin progress; traversal wraps to reconsider new eligibility behind the cursor. Provider effects use the existing 30-second worker deadline and at most four concurrent effects per process. Multiple processes multiply that concurrency; this is not a global four-call promise.

Unresolved records retain only opaque provider identity, provider kind, attempt/generation evidence, state/revisions/timestamps and cleanup-cycle identities/results. Purge removes the business association. The journal never retains participant names, account identifiers, join capabilities, credentials or raw provider errors. Settled records and their cycle receipts may be removed after seven days in indexed batches of at most 100 within the same cleanup deadline. Unresolved records have no fixed retention deadline. This narrow amendment to ADR-0008 does not extend ordinary business-data or fake participant-name retention.

## Alternatives considered

- A finite deadline alone bounds the caller but does not fence a remote producer.
- Outbox retention alone cannot preserve cleanup ownership after its effect row expires.
- Unlimited automatic retries or automatic cycle recreation hide persistent failure and prevent a meaningful work bound.
- Treating an absent snapshot as final can lose a room created later.
- Force-settling ambiguous evidence releases capacity by discarding responsibility without proof.

## Consequences

Persistent uncertainty can exhaust all meeting admission slots. Remote physical erasure is not guaranteed while creation or deletion remains uncertain. These are explicit availability and retention trade-offs. Existing business scope and join-window rules remain authoritative; repository methods do not authorize a caller.

The first implementation change supplies persistence only. Coordinator execution, provider-lane composition, operator commands and authorized API/UI activation follow under the approved local plan and require their own checks. BBB activation remains disabled until the remote compatibility probe. No local test closes the remote acceptance criteria.

## Where it is applied

[Meeting data model](../../knowledge/specs/data-model.md#meeting-recovery-ledger), [video recovery](../../knowledge/specs/realtime-and-video.md#durable-recovery-ownership), [threat model](../threat-model.md), and the approved plan's real PostgreSQL, provider, worker and API acceptance tests.
