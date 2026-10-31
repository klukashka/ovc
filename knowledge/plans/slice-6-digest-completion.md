# Message digest completion

Status: implementation approved 2026-10-31; execution evidence pending.
Base: `6670bf3fe444c03b889035e1fca7eefb2409d170`.

## Scope and verification

Implement S6 of [slice 6](slice-6.md): the minute digest job, dispatch-time email rendering, bookkeeping, read-marker cleanup and purge. Standard tier; the security trigger is met by email content, unsubscribe credentials, recipient scope, resource bounds and diagnostics. Independent security planning review accepted this design, including the revision protocol below. Implementation needs fresh standard and security reviews and its own full merge gate. No new dependency or HTTP endpoint is planned. Bounce intake remains deferred to slice 10.

## Transaction protocol

Add `digest_state` without a user foreign key, as the data model requires, and a positive sequence-backed bigint `revision` to `digest_pending`. Every new-message producer upserts pending recipients in user-identifier order, assigning a new sequence-backed revision on conflict while retaining the earliest pending time. Idempotent replays do not increment it.

Revisions never repeat across deletion and recreation of a pending row. Capture the pending revision before enumerating channels. First lock the existing recipient user FOR KEY SHARE, skipping a missing account, to prevent bookkeeping recreation after purge. Then lock channels in identifier order, then the affected member rows in identifier order, then digest state and pending. Recheck eligibility and the revision after locking. A missing pending row skips the candidate. A changed revision rolls back the entire transaction and defers the candidate; never acquire additional channel or member locks after pending. Per-member state serializes overlapping worker runs, including different business instants. Count eligible messages by sequence above both read and digested markers, after last-seen, with a body and without deletion or erasure. Commit bounds, marker advancement, digest state, outbox and flag deletion together.

A send in a newly added channel either advances the revision before the digest checks it, or waits on the pending update and recreates the flag after the digest deletes it. Both orders must be proven with the real database. Reading captures the revision before the unread check and conditionally deletes only that revision after member updates; a racing send is retained through version recheck or post-delete upsert. Purges delete both bookkeeping tables while holding the existing channel locks.

The job runs each minute, selects 200 candidates per batch and ends within a cancellable 20-second execution deadline, including connection and SQL lock waits. Business time controls the ten-minute pending/offline thresholds and thirty-minute spacing; a separate injected wall clock controls execution. A failed item is reported and does not silently hide work. Tests exercise the real worker, database, mail sink and injected clocks.

## Dispatch and acceptance

The typed outbox payload holds recipient and per-channel sequence bounds, with idempotency keyed by event, recipient and job time. Dispatch checks current active membership and current unread, body-bearing, nondeleted messages within those bounds. A student also excludes sequences at or below the retained provisional-through boundary, preventing pre-reclaim queued mail after keep; tutors retain their own history policy. One aggregate query returns at most twenty labels plus the total eligible-channel count, avoiding per-channel SQL fanout. Render recipient-specific labels, at most twenty channel lines and the additional count, in Russian or English. Escape HTML and strip unsafe label characters as existing lesson emails do. Never include message text, previews, senders or filenames. Existing account eligibility, preferences and render-time unsubscribe tokens apply; no daily-cap reservation applies. Empty current results produce `email.dropped` with `obsolete_digest`. Diagnostics carry safe kinds and counts only.

All thirteen named S6 acceptance tests in the slice plan must pass, plus concurrent new/re-added membership and sends in both pending-lock orders, conditional read clearing, overlapping jobs, cancellation while locks are held, suspended/reclaimed membership, purge, two-API aggregate presence and migration up/down/up. No notification is created in the in-app inbox. Multi-instance clock tests acknowledge the same absolute instant on every API before sending or ticking.

## Files and documentation

Change migrations, queries, generated sqlc, domain types, repositories, chat service, notification builders/templates, localized core text, startup composition, purge and relevant integration tests. Update the notifications, realtime and data-model specifications and D17/D36/D63 in the slice plan for the approved revision protocol. Update threat coverage for T-14, T-17, T-18, T-20, T-21, T-27, T-34 and T-44 as applicable, marking changed human-facing wording pending owner review. Record findings and command evidence under `knowledge/reviews/`.

## Execution and checks

S6 and F1 use separate worktrees and file ownership. The coordinator owns shared documentation, scheduling, reviews and merges. At most three active sessions including the coordinator, considering other work on the device. Heavy checks, backend race tests, builds, browser series and gates are serialized machine-wide; inspect chats, processes and locks before starting. Measurements overlap no other checks.

Run tool-binary verification, formatting, vet, lint, sqlc diff and the whole backend race suite because queries change; documentation checks and Gitleaks apply. Remaining full-run checks are deferred to the merge gate. Each change has its own gate and finish with exact tree evidence, at most 260 seconds under the slice's temporary budget. A failed assertion is fixed, never hidden by retries. This does not close slice 6.
