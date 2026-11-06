# Local meeting recovery activation verification

Status: change checks and independent reviews completed on 2026-11-06; exact-tree gate/finish pending. Base: `57bf16d4be98b6e7e367747a2f35b3a2943d9366`.

Scope: shared PostgreSQL fake runtime construction, four-slot cleanup lane, minute scanner, local status/inspect and revision-bound operator retries. Tier: standard. Security trigger: provider cleanup, operator authority, finite work and diagnostics. The [approved local plan](../plans/slice-7-local-completion.md) governs this change. No Create, public video route, UI, browser room or BBB activation is included.

## Acceptance and evidence

- `TestMeetingRecoveryRuntime`: production API test Tick composition, shared provider reconstruction, returned versus uncertain cleanup, exact capacity/receipt/outbox results, no renewed cycle on the next minute, registered four slots and safe diagnostics. Passed in the targeted process suite and full backend race run.
- `TestMeetingRecoveryAdminRevisionAndRefusals`: real CLI, required revision, stale/pending/missing refusals and safe audit. Passed in the targeted process suite and full backend race run.
- `TestMeetingRecoveryAdminConcurrentRetry`: two real CLI processes using the same evidence revision create one receipt and one outbox entry. Passed in the targeted process suite and full backend race run.
- `TestMeetingRecoveryAdminPagination`: real CLI status and receipt keyset pages across 101 rows. Passed in the targeted process suite and full backend race run.
- `TestMeetingRecoveryAdminSettledAndSubsequentCycle`: settled refusal and separately identified deliberate subsequent cycle preserving prior receipt. Passed in the targeted process suite and full backend race run.
- `TestMeetingRecoveryRuntimeRefusesDifferentProviderLimits`: real worker startup refuses a persisted default mismatch without changing the existing provider generation or disclosing names. Passed in the targeted process suite and full backend race run.
- Existing shared-fake contract tests retain exact cap, mismatch, generation exhaustion and cross-process behavior checks. Passed in the full backend race run.
- Compile-only `go test -c -o /private/tmp/ovc-activation-startup.test ./cmd/internal/startup` and `go build ./cmd/api ./cmd/worker ./cmd/ovc-admin`: passed on the complete implementation, including the mismatch test. Changed-file Markdownlint and `git diff --check`: passed. Runtime tests, full backend change checks, remaining documentation checks and secret scans: pending. Independent standard and security reviews: pending. Full exact-tree gate/finish: pending, all remaining checks deferred to that gate.

## Planned negative controls

In an isolated copy, remove the provider lane registration: `TestMeetingRecoveryRuntime` must fail its terminal ledger/provider assertions. Separately remove the evidence revision predicate from `QueueMeetingRecoveryCycle`: the stale-revision retry test must fail once the active cycle has ended. Do not run both mutations together or alter the normal checkout. Both controls were executed separately and failed the intended assertions as detailed below.

## Limitations

Status pages are observations, not a transaction snapshot. A confirmed retry commit followed by failed receipt read or output leaves a durable cycle; the operator must inspect again and cannot replay a stale revision into another cycle. Per-process random-source ownership follows the CLI composition. Provider generation/sequence exhaustion and existing-limit mismatch require explicit investigation, not silent resetting. Uncertain cleanup retains admission capacity. Remote erasure, Create coordination and public usability remain unverified or pending implementation.

## Completed checks and review disposition

The initial compile attempt found a missing canonical `ProviderMeetingID.String` method before tests executed. It was added consistently with existing domain identifiers; subsequent startup test compilation and API/worker/admin builds passed. An earlier sandbox cache-access refusal ran no tests. No runtime failure was retried or accepted as harmless in this change.

The initial six real process tests passed with race detection in 10.08 seconds of monitored execution (6.58 seconds reported by Go), with three safe samples and unchanged source. Standard review found two minor issues: startup-failure audit omitted the new fixed meeting subcommands, and older specification sentences still called runtime/operator registration pending. Both were corrected. `TestMeetingRecoveryAdminStartupFailureAudit` now runs all three commands against a genuinely absent local test database and verifies safe fixed-command failure diagnostics. The final full backend command `go -C backend test -race -count=1 ./...` passed in 102.63 monitored seconds with 22 safe samples and unchanged source, including the new startup-audit test and existing shared-fake contracts, worker/scanner and process suites.

The independent standard review found no blocker or major defect. A fresh full-security review covered the complete final implementation and applied the standard checklist to the two fixes; it passed without findings. Its reviewed 20-file fingerprint is `cc940d33325246fae8b84e39b966daa9ee097d591cffae938371c8660a41caf9`. Subsequent changes only update this verification record. The audit fix touches the safe diagnostic boundary; no production authorization, generation or cleanup invariant was weakened. Final tier is standard and the security trigger remains met.

In an isolated copy, removing provider-lane registration caused `TestMeetingRecoveryRuntime` to fail with pending cycle 1 and a still-created provider room, rather than the required terminal outcome. After restoring that code, replacing the exact queue revision predicate with a positive-revision check caused `TestMeetingRecoveryAdminSettledAndSubsequentCycle` to fail its bounded-cycle assertions; there was no SQL parameter failure. Both runs used the original assertions, race detection and monitored unchanged mutation trees. The expected failures and their logs were retained; both owned mutations were removed and never entered the implementation tree.

`go vet ./...`, golangci-lint, govulncheck, `go mod tidy -diff`, `sqlc diff`, changed Markdown lint, documentation checks and whitespace checks passed. Full-source formatting and final staged secret checks are recorded with the commit preparation. No remote provider request or real-server test ran. Local process evidence does not prove remote cleanup or BBB compatibility. The exact-tree gate and finish are still required; previous failed-gate exceptions do not apply to this change.

## Earlier gate of this change

The first full gate, on 2026-11-06 at base `57bf16d`, ran 29 commands successfully and failed two end-to-end cases of 979: the Chromium password-reset test and one Chromium console warning. Neither came from this change. Both causes were fixed on main afterwards, in `1651a84` and `b3570f9`.
