# Meeting cleanup reinspection interleavings

Status: change checks and independent standard/security review passed on 2026-11-06; ordinary exact-tree gate pending. Base: `5d3bfe31965a61cd861c3a10704c06dce41ea41b`. This follow-up addresses the minor cleanup reinspection coverage gap under stage 1 of the [approved local completion plan](../plans/slice-7-local-completion.md).

Tier: standard; security trigger met through provider-generation, cleanup and diagnostic evidence. Production behavior, contracts, diagnostics and runtime activation are unchanged. The existing threat-model mapping remains T-25/T-34 for generation-bound termination and retained cleanup evidence, T-20 for bounded waits and connection release, T-18/T-19 for safe diagnostics, and T-41 for local-versus-remote evidence.

## Test mechanism and acceptance

Two real SharedFake clients share the fixture-owned PostgreSQL database. The cleanup client receives a JSON logger with a bounded output writer that retains actual diagnostic bytes and waits at two explicit barriers: the first successful Inspect diagnostic and the stale End not_found diagnostic. SharedFake emits each after its database transaction has released. The independent client uses a different logger and ends/creates rooms through the ordinary provider methods. No provider wrapper, returned-result fabrication, HTTP interception, database behavior replacement or sleep is used. A ten-second context releases both writer waits on every exit; the existing recovering execution helper joins the worker. No production timeout changes.

`TestMeetingRecoveryWorkerReinspection` drives real Tick and checks ledger, durable cycle receipt, outbox status and capacity for three sequences:

- A replacement is created after the initial Inspect transaction; stale End refuses it; reinspection preserves it and records conflict.
- That replacement is ended after stale End refuses and before reinspection; absence settles the returned generation and releases capacity.
- The real shared provider becomes unavailable after stale End refuses; reinspection failure retains pending evidence and capacity and schedules retry rather than success. Availability is restored only after Tick completes, allowing an independent inspection to prove the replacement survived.

`TestMeetingRecoveryReinspectionCancellation` cancels the explicitly supplied Cleanup context after stale End refuses and before reinspection. It checks context.Canceled, unchanged pending evidence/capacity, replacement survival, failure diagnostics and no acquired pool connection. This is a direct service/repository integration case; it does not claim that Tick caller cancellation interrupts an already draining effect.

All cases require exactly two real Inspect diagnostics, no retained acquired database connection, and no private fixture name, provider identifier, generation, attempt or payload in diagnostics. The barriers establish the actual interleaving; no remote guarantee follows.

## Remaining coverage and verification

The original-generation-still-present outcome after End not_found is unreachable through SharedFake's monotonic generation and idempotent absent-End semantics. It remains unverified for the later BBB contract; see [known limitations](slice-7-known-limitations.md#cleanup-reinspection-interleaving). Remote provider activation remains blocked pending its separately required evidence.

Formatting and whitespace: `gofmt -w backend/internal/repositories/meeting_recovery_interleavings_test.go` and `git diff --check` passed during preparation. At implementation handoff, backend/race/static/documentation checks and independent reviews were pending. Their completed evidence follows; ordinary exact-tree gate/finish remain pending.

## Completed verification

Compilation passed. The focused real PostgreSQL/SharedFake race tests passed in 6.62 seconds with three safe monitor samples and unchanged source. The complete `go -C backend test -race -count=1 ./...` passed in 100.22 seconds with 21 safe samples and unchanged source. Gofmt, vet, golangci-lint, documentation checks and whitespace checks passed.

An isolated mutation changed the post-End mismatching-generation branch to report success. The unchanged replacement test failed with settled ledger/receipt, done outbox and capacity zero instead of conflict/retained capacity. This proves sensitivity to the actual unsafe terminal outcome after reinspection. The owned mutation was removed after retaining the failure log; it never entered this change.

Independent standard and full-security reviews found no findings. Security confirmed closure of the reachable local fake interleaving gap, while retaining the provider-specific unchanged-generation outcome for BBB checks. The tests and production source have not changed since those reviews; only evidence/status wording is finalized. Final tier remains standard and security trigger remains met. No security fix changes a production trust boundary. Other full-run checks are deferred to the ordinary exact-tree gate; gate/finish remain pending at record preparation.
