# ADR-0019: Durable chat upload ownership and unresolved cleanup

- Status: accepted, 2026-11-01; amended by [ADR-0022](0022-library-upload-cleanup-adoption.md) on 2026-11-10, which adopts this journal for the `library` and `prompt_image` contexts with their own capacity class (pending owner review); amended by [ADR-0023](0023-homework-upload-cleanup-adoption.md) on 2026-11-11, which adopts this journal for the `answer` and `corrected` contexts with a third capacity class, `homework` (pending owner review)
- Reviewed: protocol and implementation scope approved by the project owner on 2026-11-01; wording pending line-by-line owner review
- Deciders: project owner
- Related: [ADR-0006](0006-background-worker-and-email.md), [ADR-0008](0008-hosting-and-data-residency.md), [files](../../knowledge/specs/files.md), [approved plan](../../knowledge/plans/slice-6-upload-cleanup-protocol.md)
- Amends: ADR-0008's general bounded-retention statement for the minimal unresolved technical cleanup journal and explicit operator receipts described here; hosting and data residency are unchanged

## Context

A chat multipart upload may complete after its attachment and account have been purged. An unbounded detached settlement can retain a request permit indefinitely, while simply timing it out can lose the last cleanup owner. Conversely, a failed database commit acknowledgement does not justify deleting an attachment that was successfully published. Storage cancellation is not proof of remote termination; [dated source evidence](../../knowledge/research/s3-multipart-cleanup-2026-11-01.md) describes the limits.

## Decision

Create a durable intent atomically with the uploading attachment before storage work. The journal has no account or attachment foreign key, account/channel identifier, filename, content or session credential. Its immutable random storage key and attachment identifier survive purge. Record an MPU identifier once, authorize exactly one Complete through a confirmed database commit, and disable SDK retries for Create and Complete. No transaction spans storage I/O.

Publication atomically makes the attachment pending, removes its intent and releases its capacity reservation. Reconciliation preserves matching pending/bound attachments whose ownership transferred, and requests cleanup for unpublished ones. Database unavailability leaves uncertainty rather than authorizing a destructive guess.

One monotonic 30-second budget bounds detached settlement/discard, including clock, pool and lock waits. The earlier process-drain deadline wins. Recovery and intent-aware storage effects each have a 30-second budget; recovery candidates have one second capped by the remaining pass. Recovery scans indexed batches of 200, at most 20 per pass, once per minute, with a constant-space keyset cursor retained across passes and advanced after failed candidates. Repeated process restarts can delay progress.

Global capacity is 4,096 unresolved intents, reserved and released transactionally. Full capacity refuses new chat uploads with structured `503 service_unavailable`, before reading the body or starting storage, without a recovery-time promise. Existing per-user and per-instance limits remain.

Cleanup uses the existing storage-delete outbox and its eight-attempt policy. There are at most two automatic generations: initial cleanup and one final generation following definite completion acknowledgement. The journal retains failure and uncertainty independently of outbox retention. Successful Delete, Abort, absence, age and lease expiry do not prove that an ambiguous Complete cannot arrive later. Unresolved intents may remain indefinitely and consume capacity. Automatic physical deletion is not guaranteed under unresolved remote work or persistent failure.

Only local audited operator commands inspect an intent or grant a new bounded cleanup cycle. Retry requires a caller-supplied attempt UUID, reused after an uncertain command outcome. Immutable receipts make replay idempotent after outbox retention. Active cycles are mutually exclusive, and delayed final automatic cleanup remains recorded. Receipts persist until the intent is safely removed, with no total count cap: the intent cap does not bound all journal bytes. UUID deduplication and active-cycle lookup are indexed; inspection uses keyset pages of at most 100 receipts. There is no administrative HTTP surface or force-forget command.

Old upload writers must stop before migration/adoption. Legacy unresolved uploads are conservatively retained as uncertain, never treated as fenced by migration time. More unresolved rows than capacity fail adoption visibly. Avatars retain their existing flow; future contexts require their own adoption decision. [ADR-0022](0022-library-upload-cleanup-adoption.md) is that decision for the `library` and `prompt_image` contexts (pending owner review). [ADR-0023](0023-homework-upload-cleanup-adoption.md) is that decision for the `answer` and `corrected` contexts (pending owner review).

## Alternatives considered

- A timeout alone releases the request but can lose cleanup ownership after purge.
- Ordinary outbox rows alone expire and cannot represent an unresolved remote producer indefinitely.
- Infinite retries or uncapped automatic journal creation hide persistent failure and permit unbounded automatic growth.
- Treating time or successful Abort/Delete as a fence assumes a provider guarantee that has not been established.

## Consequences

The system bounds request occupancy and automatic retries while retaining explicit cleanup responsibility. A prolonged uncertain backlog can refuse all new chat uploads. Deliberate operator attempts can grow receipt storage. These are accepted availability and retention trade-offs, not a claim of complete physical erasure. Future provider-specific terminal reconciliation needs evidence and a separate operational decision. Existing authorization, quota, content inspection and serving rules remain authoritative.

## Where it is applied

[Files](../../knowledge/specs/files.md), [data model](../../knowledge/specs/data-model.md), [account locks and operator commands](../../knowledge/specs/accounts.md), [worker](../../knowledge/specs/notifications.md), [observability](../../knowledge/specs/observability.md), [threat model](../threat-model.md), and the implementation's real PostgreSQL/local-S3 regression checks.
