# Meeting recovery ledger verification

Base: `01c3f8e1e8b78e8316cf1e09718d7cf89e02e11a`. Tier: standard; security trigger met by provider ownership, generation evidence and purge. Scope: [approved plan](../plans/slice-7-meeting-recovery-ledger.md).

Independent security planning review passed with no blocker or major finding. Two minor clarifications were incorporated before implementation: explicit immutable legacy/attempt provenance and serialized nonempty downgrade refusal. Evidence revision excludes the foreign-key SET NULL change; dispatch/publication recheck actual current state. Pagination uses an immutable key and the full cleanup predicate in its index.

Implementation and change checks passed. Fresh independent standard and separate full-security implementation reviews passed over the complete tracked/untracked diff based on the revision above, with no blocker, major or minor finding. Both reviewed the migration, SQL, repository, generated bindings, domain types, actual purge integration tests and specifications; only factual verification evidence was updated afterward. No security finding required a fix. Final classification remains standard with the security trigger met. Gate and finish remain pending at commit preparation; no historical failed-gate exception applies.

Pinned tools were copied from the main worktree with SHA-256 equality, and `tools/check-tool-binaries.sh` passed. Runtime sources stayed unchanged during the observed checks. Each run used temporary owned sleep/display assertions, an open lid and continuous safe observations; the computer was externally powered at preflight. Other active work was checked before heavy runs, which ran sequentially.

Frontend and documentation dependencies installed from the local cache with frozen lockfiles, exit 0. The first documentation command passed link checking but could not launch its browser in the restricted environment (`Failed to launch the browser process: Code: null`); it is not recorded as a passing full check. The command then ran with browser execution permission and passed, exit 0. No document assertion, timeout or policy was changed to obtain that result.

No application route, worker job or provider call is activated. The ledger preserves uncertainty but does not implement remote cleanup, admission capacity, retention or operator recovery. These remain activation prerequisites; BBB/media behavior is unverified.

## Verification evidence

| Check | Result |
| --- | --- |
| `go test ./internal/repositories ./internal/api -run '^$' -count=1` | Passed, exit 0; compilation only, not behavioral evidence |
| `go -C backend test -race -count=1 ./internal/repositories -run '^TestMeeting'` | Passed, exit 0; 8.67 seconds observed, three safe monitor samples |
| `go -C backend test -race -count=1 ./...` | Passed, exit 0; 98.86 seconds observed, 21 safe monitor samples |
| `go mod tidy -diff`, empty `gofmt -l .`, `go vet ./...`, `golangci-lint run ./...`, `govulncheck ./...`, `sqlc -f sqlc.yaml diff` from backend | Passed, exit 0 for every command; no affected call found by govulncheck, one advisory remains in a required module without an affected reachable call |
| Documentation and staged Gitleaks | Final documentation passed, exit 0, after the browser-launch restriction described above; staged Gitleaks passed, exit 0, no leaks |
| Frontend, E2E and remaining full-run checks | Deferred to the ordinary exact-tree merge gate |

Real PostgreSQL tests prove atomic two-row creation and rollback, including a ledger uniqueness conflict after business insertion; conditional dispatch and immutable acknowledged generations; conservative migration/provenance and refusal of nonempty downgrade; cleanup/late-completion races in both orders with observed row-lock barriers; cancellation and eventual connection release; unchanged-revision refusal after FK nulling; replacement isolation; and indexed keyset pages over 105 mixed candidates. Existing API worker-driven purge tests prove final tutor purge preserves the orphan record and student purge preserves tutor history. No provider cleanup is inferred from those database results.

Two declared negative controls ran once each in separate copies of the same source. Removing the Open cleanup predicate made `TestMeetingRecoveryTransitions/publication_requires_returned_generation_without_cleanup/cleanup` fail with `unproven or cleanup generation published` (command exit 1, 3.36 seconds, two safe observations). Replacing the recovery foreign key's SET NULL action with CASCADE made `TestMeetingRecoveryPurge/unchanged_revision_cannot_dispatch_or_publish_after_purge` fail with `get meeting recovery: not found` (command exit 1, 2.79 seconds, two safe observations). These are successful sensitivity controls, not passing application runs. The approved tree was unchanged and its targeted/full runs passed.

The standard reviewer verified the available indexed access path and resource consequences; this is not a measured planner/latency claim. The security reviewer examined T-18/T-19/T-20/T-25/T-34/T-36/T-41 and the deferred activation boundary. Neither reviewer reran runtime checks. No assertion, timeout, dependency, diagnostic policy or gate budget was relaxed.
