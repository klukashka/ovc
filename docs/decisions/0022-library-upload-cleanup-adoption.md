# ADR-0022: Durable cleanup for library and prompt-image uploads

- Status: accepted, 2026-11-10
- Reviewed: decided by the project owner on 2026-11-10 with the [slice 8 plan](../../knowledge/plans/slice-8.md#4-owner-questions); wording pending owner review
- Deciders: project owner
- Related: [ADR-0019](0019-durable-upload-cleanup.md), [files](../../knowledge/specs/files.md#upload-flow), [data model](../../knowledge/specs/data-model.md#upload-cleanup-intent), [threat model](../threat-model.md) (T-13, T-20, T-34)
- Amends: ADR-0019, whose rule that every future upload context needs its own adoption decision this record applies to the `library` and `prompt_image` contexts

## Context

Slice 8 adds two upload contexts on the general streaming pipeline built for chat in slice 6: `library` (up to 100 MB per file, for materials) and `prompt_image` (up to 5 MB, for pictures in assignment prompts). Both are multipart uploads to object storage and can leave the same unresolved remote work as chat uploads. ADR-0019 decided the durable cleanup journal for chat only and requires a separate adoption decision for each future context. Its global capacity of 4,096 unresolved intents was sized for chat; a library upload of 100 MB can take up to the 10-minute read deadline, so library uploads sharing that capacity could make chat uploads answer `503`.

## Decision

The `library` and `prompt_image` contexts adopt the durable cleanup journal of ADR-0019 unchanged: the intent created atomically with the uploading attachment before storage work, one Complete authorized through a confirmed commit, publication that removes the intent, the bounded settlement and recovery budgets, at most two automatic cleanup generations, retained uncertainty, intent-routed deletion in the purge, and the local operator commands.

They get their own capacity class: at most 1,024 unresolved intents, shared by the two contexts and separate from chat's 4,096. Each intent records its class; creation reserves and terminal removal releases that class's capacity row. A full library class answers library uploads with structured `503 service_unavailable` before reading the body or starting storage, without a promised recovery time, while chat uploads are unaffected, and the reverse.

Uploads of both contexts need a tutor whose email is confirmed. The tutor purge, the 7-day cleanup and the immediate purge of an unconfirmed tutor route the tutor's `uploading` and `pending` library uploads through their intents, as for chat.

## Alternatives considered

- **Sharing chat's 4,096 intents:** simpler, but slow or stuck library uploads could exhaust chat's capacity and refuse chat uploads.
- **No durable journal for the library contexts:** a timeout or an unconditional delete could lose cleanup ownership after a purge or delete a successfully published file, the faults ADR-0019 was written to remove.

## Consequences

The journal and its recovery scan carry library intents beside chat ones; the operator commands inspect and retry both. A prolonged uncertain backlog can refuse new library uploads, an accepted availability trade-off, not a claim of complete physical erasure. Migration of the capacity row stops old writers first, as ADR-0019 requires.

## Where it is applied

[Files](../../knowledge/specs/files.md#durable-chat-upload-cleanup), [data model](../../knowledge/specs/data-model.md#upload-cleanup-intent), [API](../../knowledge/specs/api.md#files), [threat model](../threat-model.md), and the implementation's real PostgreSQL and local S3 regression checks.
