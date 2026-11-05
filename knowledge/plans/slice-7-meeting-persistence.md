# Slice 7 L2a: durable meeting lifecycle foundation

Status: approved by the owner on 2026-11-05 against main 94b0322bdfd7f394394dd7f547a8c4166a58d608. Local-only implementation, no remote server, network research, provisioning or credentials required. L1 and its mandatory exception record have merged. The prior failed E2E remains failed; its one-change acceptance grants no future exception.

## Problem and outcome

L1 implements an injected in-memory provider, but there are no meeting rows or lifecycle repository. API and worker are separate processes: independently constructed L1 fakes would not share rooms. Do not wire them as if they were a shared provider. First implement the persistent lifecycle boundary as a small independently testable change. Next plans must separately define a shared local provider transport/state, then authorized API/worker integration. This change alone does not provide a join endpoint or usable video room.

Implement actual PostgreSQL storage for creating/open/closed meetings, at most one non-closed meeting per lesson, immutable provider identity and conditional transitions. Public lesson behavior and authorization remain unchanged. No provider calls, API schemas, frontend, outbox dispatch, process configuration, HTTP fake, callbacks or recording behavior are introduced.

## Schema and repository contract

Add migration 00022_meetings.sql after confirming the number is still free. Columns: typed UUID id, lesson_id FK, provider enum value (fake/bbb), unique opaque provider_meeting_id UUID, unique opaque routing_key UUID (not a credential), state, nullable positive bigint create_time, created_at/updated_at, first_join_at, ended_at and close_reason. No join URL, participant display name or provider secret is stored. Caller supplies identifiers and time using existing injected sources; SQL adds no ambient randomness or clock reads.

Constraints enforce valid states/reasons and zero UUID refusal, creating has no create_time/first_join/ending fields, open has positive create_time and no ending fields, closed has ended_at and a valid reason and may lack create_time when creation failed. first_join_at requires a positive create_time; timestamps are ordered consistently. Use a partial unique lesson index WHERE state <> 'closed', plus a complete lesson_id index for FK deletion of closed history; the partial live index cannot cover it. Do not add speculative query indexes. Student purge retains tutor-owned meeting history; tutor lesson deletion cascades local meeting metadata; no provider is activated in this change. Future activation must add provider cleanup before any path can create remote rooms, including tutor purge.

Define domain MeetingID, MeetingRoutingKey, stored-state/provider/close-reason enums; distinguish persistent lifecycle state from L1 provider status. Reuse ProviderMeetingID and ProviderCreateTime. Narrow repository operations: insert creating, get by lesson+meeting, get current non-closed meeting, open an expected creating row, record first join once on an expected open generation, and close an expected creating/open generation. All operations that mutate/read a named row include lesson_id and meeting id; wrong lesson cannot affect another row. Zero-row conditional outcomes are explicit typed outcomes, never silent success; repeat close is an explicit already-closed observation and cannot overwrite reason/time. Stale open/close/first-join attempts must not affect a replacement meeting or revive closed state. No general update API.

Repository methods use the existing transaction-bound query construction. Caller owns the lesson lock before meeting mutations; no new path reverses the documented audience/series/lesson order. Repository scope is not application authorization: future services must resolve caller relationship/session and acquire the required locks first. Direct PostgreSQL concurrent-insert tests also prove the unique constraint without relying on callers obeying locks.

The current specification's retention_days, recordingmarks and settled_at are explicitly deferred with their recording/callback/reconciliation consumers, not silently implemented with fabricated defaults. The existing tutor recording-retention setting is not implemented; no new retention policy is introduced. Update the data-model activation note to distinguish the executable MVP subset from the future schema. This proposed staging clarification is part of owner approval. Closed rows remain until lesson purge in L2a; no unbounded list API or maintenance loop is added. State retention/reconciliation is required before provider activation.

## Files and order

1. backend/internal/domain/meetings.go, backend/migrations/00022_meetings.sql, backend/queries/meetings.sql, backend/sqlc.yaml and generated backend/internal/dbgen files. Reuse existing ID codecs; add only mappings needed by this schema.
2. backend/internal/repositories/meetings.go and meetings_test.go; backend/internal/repositories/store.go only for the new transaction-bound accessor. A narrow fixture helper in backend/internal/testdb/testdb.go may apply up/down migrations only to its already validated disposable Database instance/owned pool, never an arbitrary caller URL, using the existing goose dependency; include its focused real-database test if needed for round-trip coverage. Follow existing wrapping and typed not-found conventions, without logging raw database errors or SQL values.
3. knowledge/specs/data-model.md and realtime-and-video.md activation notes; knowledge/plans/slice-7.md (including removal of the stale initial claim that no adapter exists); knowledge/plans/slice-7-meeting-persistence.md and knowledge/reviews/slice-7-meeting-persistence.md; docs/threat-model.md T-20/T-25/T-34/T-41 staging and deletion boundary, wording pending owner review. No new package/layer, dependencies or settings.
4. Real database integration tests, generation/static checks, independent reviews and ordinary exact-tree full gate/finish. One coherent branch/commit. No L2b code before its separate plan approval.

## Checkable acceptance

- TestMeetingPersistenceConstraints: migrated PostgreSQL rejects duplicate non-closed rows per lesson, duplicate provider/routing IDs, invalid state/time combinations (including NULL cases that PostgreSQL CHECK would otherwise accept) and invalid references; different lessons work and a new generation can follow a closed row.
- TestMeetingPersistenceTransitions: real repository creating -> open -> first join -> closed; first join is set once; failed creation closes without fabricated create time; stale/mismatched generations and wrong lesson IDs change nothing; repeat closure preserves original reason/time.
- TestMeetingPersistenceConcurrency: two real transactions contend for one lesson, exactly one insertion succeeds; close versus open and close versus first-join preserve terminal state, with database-observed barriers/deadlines and no sleeps or runtime patching. Test both serialized operation orders deterministically, not a scheduler-dependent winner assertion.
- TestMeetingPersistenceRollbackAndPurge: rollback leaves no published transition, cancellation of a real lock wait does not mutate, deletion through the existing lesson-purge repository path removes owned metadata and leaves other tutors' rows intact. No provider cleanup claim is made.
- TestMeetingMigrationRoundTrip: use an isolated database migrated to 21, preserve representative existing lesson rows through up to 22, down to 21, then up; no owner/development database migration is executed by the agent. Down is destructive only for the new table in the disposable test database.
- Existing architecture/conventions and schedule/purge tests pass. No new application route or provider construction exists outside tests, and L1 contract/race tests continue to pass.

Use real PostgreSQL fixtures, injected clocks/randomness and existing AAA conventions. No fake database, arbitrary DB behavior replacement, sleeps or retries. Required negative controls are the natural rejected mutations/concurrent transactions; do not add an unrelated diagnostic framework.

## Verification, risks and coordination

Tier standard, full security trigger: provider identity/lifecycle, deletion and verification staging map to T-20/T-25/T-34/T-41; future authorization T-10/T-21 and callbacks/remote transport remain unimplemented, not certified. One security-profile planning review before owner approval; independent standard plus separate security implementation reviews before readiness. Observability: no new executable process or business event; repositories return wrapped failures to the eventual service owner, no raw content/logging. Future service emits business lifecycle events.

Change checks include pinned tools, sqlc generation/diff, migration tests, gofmt, go vet ./..., full lint, go test -race for changed/transitive importers plus conventions/cmd (derive from go list; use full backend when broad), govulncheck, documentation and staged Gitleaks. Full gate still runs frontend/E2E normally. A recurrence of the inherited password-reset failure stops integration under normal policy; no automatic repeat, suppression or reuse of L1 acceptance. The owner paused 260-second optimization; this plan does no budget tuning.

At most three working sessions including other chats; check active chats and host load before runtime. Only one heavy run at a time. Implementation is authorized only within this approved scope; changes to that scope require a new decision. Main has no remote or CI, so remote/PR status is not applicable. Main owner configuration directories remain untouched.

## Planning review

Independent security planning review passed with no blocker or major. Its minor T-34 deletion mapping was incorporated and rechecked. Implementation checkpoints include explicit SQL NULL refusals, full lesson FK indexing, immutable generation predicates and fixture-owned migration targets. The owner approved implementation after the planning review; runtime evidence is recorded separately.
