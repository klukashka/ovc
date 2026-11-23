# ADR-0023: Durable cleanup for answer and corrected-file uploads

- Status: accepted, 2026-11-11; amended by [ADR-0025](0025-export-upload-cleanup-adoption.md) on 2026-11-23, which adds a fourth capacity class, `export`, of 256 unresolved intents for export archives (pending owner review)
- Reviewed: decided by the project owner on 2026-11-11 with the [slice 9 plan](../../knowledge/plans/slice-9.md#4-owner-questions); wording pending owner review
- Deciders: project owner
- Related: [ADR-0019](0019-durable-upload-cleanup.md), [ADR-0022](0022-library-upload-cleanup-adoption.md), [files](../../knowledge/specs/files.md#upload-flow), [data model](../../knowledge/specs/data-model.md#upload-cleanup-intent), [threat model](../threat-model.md) (T-13, T-20, T-34)
- Amends: ADR-0019, whose rule that every future upload context needs its own adoption decision this record applies to the `answer` and `corrected` contexts; ADR-0022, whose two capacity classes, chat's and the library's, this record joins with a third

## Context

Slice 9 adds two upload contexts on the general streaming pipeline: `answer`, written by a student for a file question of their own submission (often a phone photo), and `corrected`, written by the issue's tutor for a corrected file attached to a review. Both are multipart uploads to object storage of up to 25 MB per file and can leave the same unresolved remote work as chat and library uploads. ADR-0019 requires a separate adoption decision for each new context, and ADR-0022 gave the library contexts their own capacity class so that slow library uploads cannot refuse chat uploads. Answer uploads are written by students, possibly many at once before a deadline, so their backlog behaves differently from a tutor's library uploads.

## Decision

The `answer` and `corrected` contexts adopt the durable cleanup journal of ADR-0019 unchanged: the intent created atomically with the uploading attachment before storage work, one Complete authorized through a confirmed commit, publication that removes the intent, the bounded settlement and recovery budgets, at most two automatic cleanup generations, retained uncertainty, intent-routed deletion in the purges, and the local operator commands.

They get a third capacity class, `homework`: at most 1,024 unresolved intents, shared by the two contexts and separate from chat's 4,096 and the library's 1,024. Each intent records its class; creation reserves and terminal removal releases that class's capacity row. A full homework class answers answer and corrected uploads with structured `503 service_unavailable` before reading the body or starting storage, without a promised recovery time, while chat and library uploads are unaffected, and the reverse.

Uploads of both contexts need a caller whose email is confirmed and are charged to the issue's tutor. The tutor purge, the student purge, the 7-day cleanup and the immediate purge of an unconfirmed account route the account's `uploading` and `pending` homework uploads through their intents, as do pending uploads whose scoped submission no longer exists.

## Alternatives considered

- **Sharing the library class of 1,024:** one class fewer, but a burst of slow or stuck student answer uploads before a deadline could exhaust it and refuse the tutors' library and prompt-image uploads, and the reverse.
- **Sharing chat's 4,096 intents:** rejected for the reason ADR-0022 gives for the library contexts.
- **No durable journal for the homework contexts:** a timeout or an unconditional delete could lose cleanup ownership after a purge or delete a successfully published file, the faults ADR-0019 was written to remove.

## Consequences

The journal and its recovery scan carry homework intents beside chat and library ones; the operator commands inspect and retry all three classes. A prolonged uncertain backlog can refuse new answer and corrected uploads, an accepted availability trade-off, not a claim of complete physical erasure; students can still answer every question that is not a file question. Migration of the capacity row stops old writers first, as ADR-0019 requires.

## Where it is applied

[Files](../../knowledge/specs/files.md#durable-chat-upload-cleanup), [data model](../../knowledge/specs/data-model.md#upload-cleanup-intent), [API](../../knowledge/specs/api.md#files), [threat model](../threat-model.md), and the implementation's real PostgreSQL and local S3 regression checks.
