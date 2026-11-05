# Slice 7: generation discovery before application activation

Status: approved by the owner on 2026-11-05 against main 0f14087e92f2e9e89a3efd82209733aa011e0048.

## Problem and outcome

The existing Provider.MeetingInfo requires an already known create time. CreateMeeting may commit provider state and then return an error without the create time; this uncertain outcome is explicitly documented by L2b. A caller retaining only its durable provider meeting identifier cannot currently discover the created generation. Closing a stale creating business row without retaining a way to inspect/clean up the provider could orphan a room.

Add a narrow read-only inspection operation to the provider contract and both local implementations. It takes only a typed opaque ProviderMeetingID and returns the current MeetingInfo, including its create time. This is one missing prerequisite of the future durable create/cleanup and authorized join flow, not a claim to implement that flow. No API route, frontend, worker/process wiring, provider activation, migration, retry loop or external server is added. L2b remains merged with an ordinary successful gate; update its progress status in the slice plan.

## Exact contract

Add InspectMeeting(ctx, id) (domain.MeetingInfo, error) to Provider. Reject the zero identifier as invalid_input. For a valid identifier without a room, return MeetingInfo{Status: MeetingNotFound, CreateTime: 0, Participants: 0} and nil error, matching the absence convention of MeetingInfo; do not return FailureNotFound for this case. For an existing room return created/running, its positive current create time and participant count as one consistent snapshot. Only existing opaque provider identifiers are inputs; no lesson/user identifier or title is substituted.

Inspection never creates or joins a room, issues credentials, changes availability, records first_join_at, closes a business row, or ends a room. Both implementations preserve their existing cancellation/unavailability and fixed diagnostic conventions. Add a fixed inspect operation name, with no identifiers/names/capabilities in diagnostics. No credential, participant name or raw database error appears in the result.

Fake takes its existing mutex; SharedFake reads using its existing serialized transaction boundary, with no new table or business query. An absent snapshot is not a terminal proof: a concurrent unfinished creation may still appear later. A found snapshot likewise grants no authority and can become stale immediately after return. Existing MeetingInfo keeps requiring the exact generation; EndMeeting, JoinLink, Enter and Leave keep their current generation checks. A caller that discovered generation A and races replacement B cannot end/join B using A. Do not add destructive EndByID, wildcard create times or an internal automatic Create retry.

Future application activation must separately approve and implement durable creation/cleanup ownership that survives lesson purge, bounded worker reconciliation, handling of an in-flight or uncertain Create, authorization/session rechecks, participant-ID derivation, HTTP fake-room credential exchange and navigable links. Discovery alone resolves none of these lifetime/authorization obligations. The existing statement that creating older than one minute is treated as failed must gain an activation note: closing such a business row must not discard durable provider cleanup/reconciliation ownership, and absence from one inspection is insufficient to settle an unfinished creation. No concrete new retry or retention policy is approved here.

BBB remains unimplemented. The future BBB adapter must implement read-only discovery by its opaque meeting identifier and prove returned generation semantics through its local protocol contract and remote acceptance. No external fact or live compatibility claim is added now.

## Files and order

1. backend/internal/adapters/meeting/provider.go, fake.go and shared_fake.go: interface, implementations and fixed diagnostic operation. Reuse a small shared status-calculation helper only if it preserves current semantics; no unrelated refactor.
2. contract_test.go: common inspection cases run against both factories. fake_test.go and shared_fake_test.go/shared_fake_boundaries_test.go as needed for cancellation, read-only state and generation races. shared_fake_process_test.go verifies a child with only the opaque identifier can observe the parent's create time; do not expand the child protocol beyond one explicit inspection operation or remove its existing exact-generation/revocation assertions. No new dependencies or fixture framework.
3. knowledge/specs/realtime-and-video.md provider/recovery activation notes, knowledge/plans/slice-7.md progress/dependencies, knowledge/plans/slice-7-provider-discovery.md and knowledge/reviews/slice-7-provider-discovery.md. docs/threat-model.md T-18/T-19/T-20/T-25/T-34/T-36/T-41 covers discovery versus authority and uncertain creation; changed wording pending owner review. knowledge/specs/observability.md adds inspect to safe fixed provider operations if enumerated there. No other docs/code scope.
4. Change checks, separate fresh standard/security implementation reviews, ordinary exact-tree gate, ff-only merge and finish. One coherent branch/commit.

## Checkable acceptance

- TestMeetingProviderContract and TestSharedFakeProviderContract run the same added cases: missing room, zero ID, created room, correct positive create time, End followed by absence. Existing generation-bound and capability tests still pass.
- TestMeetingProviderInspectionReadOnly exercises each local implementation after Enter/Leave, checks exact current status/count, and compares all room/link/participant/generation/sequence state before and after inspection. Inspection allocates no room, token or participant and cannot change first-join business state because it never accesses business tables.
- TestMeetingProviderInspectionGenerationSafety discovers generation A, ends A, recreates B under the same provider ID, and proves stale MeetingInfo/End/Join requests using A cannot affect B; the next inspection reports B. Use deterministic explicit ordering, plus concurrent race coverage through existing injected sources and real PostgreSQL barriers, without sleeps or scheduler-dependent winner assertions.
- TestSharedFakeInspectionCancellation blocks the existing provider-state row with a real transaction, observes the inspection waiter, cancels it, and proves connection/transaction release and unchanged state. In-memory cancellation/unavailability and shared unavailability return the existing typed results without mutation.
- TestSharedFakeAcrossProcesses gains discovery using only the provider identifier and checks exact create time/status/count against the parent. Existing credential import/revocation assertions remain. Bound input/output, deadlines, safe failure output and process reaping remain unchanged.
- Structured-log checks cover inspect success and failure without room ID, title, participant data, link data or raw errors. No application constructor or route uses InspectMeeting in this stage.

No test fabricates a commit outcome or treats a single absent snapshot as proof of eventual absence. Uncertain commit behavior remains explicitly unverified; real read-only discovery and stale-generation safety are the acceptance claims.

## Verification and coordination

Tier standard; security trigger met by provider identity/lifecycle, diagnostics and cleanup assumptions. Independent security-profile planning review before approval; separate independent standard and full-security implementation reviews before readiness. Follow AGENTS.md and ovc-verify, including failed-check rules. All new runtime checks are pending implementation.

Run pinned-tool verification, gofmt, full vet/lint/govulncheck, relevant provider race/integration tests and transitive importer/conventions checks (full backend when broad), documentation and staged Gitleaks. Full browser/E2E and remaining full-run checks run in the ordinary merge gate; no prior exception is reused. Do not work on the paused 260-second target.

At most three working sessions including other chats. Check actual load and other test runs before runtime; heavy checks sequential. No remote/CI/issue tracker exists. Main owner configuration directories and prior failed evidence remain untouched.

Main risk is overclaiming discovery as recovery: the API must remain inactive until durable ownership and reconciliation are separately delivered. This small change intentionally does not introduce a premature cleanup policy or make a read-only provider handle an authorization token.

## Planning review

Independent security planning review passed with no remaining findings. The missing-room result was clarified as MeetingNotFound with nil error. Review covered discovery versus authority, stale generation safety, safe diagnostics and activation prerequisites; no runtime checks were run.
