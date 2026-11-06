# Meeting cleanup reinspection interleavings

Status: change checks and independent reviews passed; the full gate failed and the exact follow-up merged under the owner-approved exception on 2026-11-06. Mandatory documentation verification is pending. Base: `5d3bfe31965a61cd861c3a10704c06dce41ea41b`. This follow-up addresses the minor cleanup reinspection coverage gap under stage 1 of the [approved local completion plan](../plans/slice-7-local-completion.md).

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

Formatting and whitespace: `gofmt -w backend/internal/repositories/meeting_recovery_interleavings_test.go` and `git diff --check` passed during preparation. At implementation handoff, backend/race/static/documentation checks and independent reviews were pending. Their completed evidence and subsequent exceptional integration follow.

## Completed verification

Compilation passed. The focused real PostgreSQL/SharedFake race tests passed in 6.62 seconds with three safe monitor samples and unchanged source. The complete `go -C backend test -race -count=1 ./...` passed in 100.22 seconds with 21 safe samples and unchanged source. Gofmt, vet, golangci-lint, documentation checks and whitespace checks passed.

An isolated mutation changed the post-End mismatching-generation branch to report success. The unchanged replacement test failed with settled ledger/receipt, done outbox and capacity zero instead of conflict/retained capacity. This proves sensitivity to the actual unsafe terminal outcome after reinspection. The owned mutation was removed after retaining the failure log; it never entered this change.

Independent standard and full-security reviews found no findings. Security confirmed closure of the reachable local fake interleaving gap, while retaining the provider-specific unchanged-generation outcome for BBB checks. The tests and production source have not changed since those reviews; only evidence/status wording is finalized. Final tier remains standard and security trigger remains met. No security fix changes a production trust boundary. At review completion, remaining full-run checks were deferred to the gate; its actual failed result and exceptional integration are recorded below.

## Owner-approved exception integration 2026-11-06

Base `5d3bfe31965a61cd861c3a10704c06dce41ea41b`; change `05a79d2207c1573617f9ba4838ae7325b0f2280a`; tested tree and verified post-merge `main^{tree}` both `c9b54f8ca08bf2135702f85e001f4a96e57e7c5e`. Gate wall time: 287.9 seconds, within the ten-minute budget. The monitor recorded 58 safe samples, valid observations and unchanged source. The gate failed: 29 commands passed; E2E passed 978/979. The Chromium password-reset test timed out at 30000 ms, with two unknown authentication HTTP diagnostics, one console diagnostic and route-cleanup errors after page/context closure.

Read-only inspection of the existing trace confirmed successful account-response JSON and terminal assertions at signup.spec.ts:882–883. The next await is refresh.finished() at line 884 and no next assertion occurred before timeout; this identifies an inferred stopping point, not its cause. It does not establish identity with historical body-observation failures or harmlessness. Raw traces remain private. The full safe gate, monitor and reviews are retained under `.git/ovc-merge-reports/05a79d2-cleanup-interleavings-failed/`.

The owner approved the [bounded disposition](../plans/slice-7-interleavings-known-failure-exception.md) after security planning review. Exact clean tracked identities were checked before the fast-forward; only pre-existing owner-owned `.agents/` and `.codex/` remained untracked on main. The run lock was absent. The merge advanced exactly base to head and the resulting tree matched. No rerun or ordinary finish occurred. Failed results remain failed; mandatory separately reviewed documentation and its ordinary docs-only gate/finish precede further application implementation. Future checks, remote acceptance and slice closure are not waived.

Every gate command and exit result:

```text
exit 0: pre: 0.2 s: .: tools/check-tool-binaries.sh
exit 0: C: 0.1 s: .: pnpm --dir tools/docs install --frozen-lockfile
exit 0: C: 12.4 s: .: tools/docs/check-docs.sh
exit 0: B: 2.7 s: .: bin/gitleaks git --redact --no-banner --config .gitleaks.toml .
exit 0: B: 0.1 s: .: bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .
exit 0: C: 3.0 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod
exit 0: C: 0.8 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod
exit 0: C: 1.3 s: .: bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml
exit 0: C: 1.0 s: .: bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml
exit 0: B: 0.1 s: backend: go mod tidy -diff
exit 0: B: 0.2 s: backend: unformatted="$(gofmt -l .)" && test -z "$unformatted"
exit 0: B: 0.7 s: backend: go vet ./...
exit 0: B: 1.2 s: backend: ../bin/golangci-lint run ./...
exit 0: B: 4.9 s: backend: ../bin/govulncheck ./...
exit 0: A: 104.7 s: backend: ../tools/dev-env.sh go test -race -count=1 ./...
exit 0: B: 1.8 s: backend: ../bin/sqlc -f sqlc.yaml diff
exit 0: C: 0.1 s: frontend: pnpm install --frozen-lockfile
exit 0: C: 7.3 s: frontend: pnpm typecheck
exit 0: C: 20.4 s: frontend: pnpm lint
exit 0: C: 3.2 s: frontend: pnpm format:check
exit 0: C: 11.7 s: frontend: pnpm test
exit 0: C: 1.2 s: frontend: pnpm build
exit 0: C: 0.8 s: frontend: pnpm check:classes
exit 0: C: 0.2 s: frontend: pnpm check:bundle
exit 0: C: 8.0 s: frontend: pnpm api:check
exit 0: C: 0.8 s: frontend: pnpm audit --audit-level high
exit 1: e2e: 182.0 s: frontend: ../tools/dev-env.sh pnpm e2e
exit 0: C: 5.6 s: .: python3 .claude/hooks/check_guard_secrets.py
exit 0: C: 1.0 s: .: tools/check-dev-env.sh
exit 0: B: 48.6 s: .: tools/check-merge-gate.sh
```
