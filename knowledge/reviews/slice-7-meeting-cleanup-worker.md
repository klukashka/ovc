# Constructor-injected meeting cleanup worker

Status: change checks and independent standard/security reviews completed; ordinary exact-tree gate pending. Base commit: `25e2066bcb5583048675ecda32f8e79165e626b4`. Approved scope: stage 1 of [local completion](../plans/slice-7-local-completion.md). Tier: standard. Security trigger: provider authority, finite work, retained uncertainty and safe diagnostics.

## Scope and boundaries

The new meetings service reads an immutable outbox cycle receipt and current ledger evidence, performs Inspect/End without holding a business transaction or connection, and supplies a typed result to the real worker. The worker retains its effect timeout, panic handling and finite retry machinery and calls the existing atomic lease/revision-fenced completion. Generic lanes retain their previous handler and completion path. No database schema, provider contract or public route changes.

Prepared revoked attempts avoid provider work. A mismatching generation is retained as conflict. Started, uncertain and legacy-uncertain attempts remain unresolved after absence or successful End. Returned evidence can settle only after confirmed termination. Stale End refusal requires reinspection; changed generation conflicts, absence confirms termination, and an unchanged room retries. Runtime registration, scanning, Create coordination, operator commands, API/UI and remote acceptance remain pending implementation.

Boundary ownership for review: backend covers meetings service–repositories–provider and worker–outbox completion; security covers exact generation authority, uncertainty retention, cycle/revision/lease fences, diagnostics and bounded work. Existing provider and terminal persistence contracts remain applicable.

## Acceptance and evidence

`TestMeetingRecoveryWorker` exercises the real worker and SharedFake with real PostgreSQL: revoked prepared evidence, returned present/absent generation, started/uncertain/legacy retention, replacement refusal, eight failed attempts, a late acknowledgement during a real provider lock wait, missing cycle receipt refusal and execution with only one available pooled connection. `TestMeetingRecoveryWorkerDispatchRefusals` covers absent typed handler and wrong lane without panic/provider mutation. `TestMeetingRecoveryWorkerCancellation` cancels a real provider lock wait and checks retained evidence, safe failure diagnostics and released resources. Existing worker suites cover other lanes and panic/reclaim behavior; existing terminal tests cover atomic completion and expired leases.

Planned targeted command from `backend`: `go test ./internal/repositories ./internal/worker ./internal/adapters/meeting -run 'TestMeetingRecoveryWorker|TestMeetingRecoveryTerminal|TestMeetingRecoveryEighth|TestMeetingRecoveryReclaim|TestDispatcher|Test.*Lane|TestSharedFakeProviderContract' -count=1`. At initial implementation preparation the required checks and gate were pending; completed change-check evidence follows below. Formatting completed with gofmt. An initial compile-only invocation was blocked by sandbox access to the Go build cache, before test execution; it is not passing evidence.

Planned negative control, completed below: in an isolated copy remove the recorded-versus-inspected generation conflict branch, run the generation-conflict subtest, require a failure, then discard only the owned copy. Do not mutate the reviewed worktree.

Coverage limitation: the exact interleaving replacing a room between successful Inspect and End is not yet forced in the executor integration test. SharedFake has no injected barrier at that boundary; adding a behavior-replacing provider wrapper would violate the no-mocks rule. Existing provider contract tests enforce stale-generation End refusal, while this service's reinspection branch requires independent review. No remote semantics are inferred.

Threat map: T-20 bounded slots/deadline/retries, T-25 cycle and exact generation, T-34 retained uncertainty/capacity, T-18/T-19 diagnostics, T-36/T-41 inactive public surfaces and local-only evidence. No fixed security finding has yet been reported. Final tier/trigger, exact working tree and executable results must be refreshed before readiness.

## Completed change checks

Compilation of the changed packages passed after granting normal Go cache access. The targeted race command above passed in 10.09 seconds with unchanged source and three safe monitor samples. The full `go -C backend test -race -count=1 ./...` passed in 99.45 seconds with unchanged source and 21 safe samples. `gofmt`, `go vet ./...`, `golangci-lint run ./...`, `go mod tidy -diff`, `sqlc diff`, `govulncheck ./...`, documentation checks and `git diff --check` passed. Govulncheck reported no affected code; one required-module advisory was not reachable by the scanned code. Dependency scanning and all frontend checks remain deferred to the ordinary gate.

The isolated generation-guard removal failed the unchanged generation-conflict assertion: the mutated executor incorrectly reported settled/done with zero remaining capacity. The source worktree was unchanged, and the owned mutation was removed after retaining its failure log. This proves sensitivity of the initial generation guard only.

Independent standard review found no findings. Independent full-security review found no blocker or major and one minor coverage gap, preserved in [slice 7 known limitations](slice-7-known-limitations.md#cleanup-reinspection-interleaving). Both reviews examined the complete functional diff. This evidence update and limitation record add no executable or contract change after those reviews. No fixed security finding changes a trust boundary. The final tier remains standard and the security trigger remains met.

A preliminary offline documentation dependency install found an incomplete local cache; the ordinary frozen install then passed without a lockfile change. The earlier sandbox cache denial remains a setup failure, not a passing compilation. No application assertion failed on the unmodified tree. Main-script gate and finish remain pending at record preparation.
