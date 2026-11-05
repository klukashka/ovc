# Slice 7 shared local provider verification

Base: `48c9fad7beab9867cf64765612fd46cfb2b38045`. Scope: [approved L2b plan](../plans/slice-7-shared-fake.md). Tier: standard; security trigger met by shared provider state, capabilities, resource lifecycle and diagnostics. The repository has no remote, issue tracker or CI; those refreshes are not applicable.

## Implementation and review scope

Migration 00023 adds adapter-owned state, rooms, participants and hashed link bindings. SharedFake preserves the Provider contract across independent database clients and processes. Existing L1 issuer checks remain intact; only its fixed diagnostic reporter is shared. No application route, provider wiring, setting, dependency or browser change is introduced.

Independent security planning review found no blocker or major. Two minor clarifications were incorporated before approval: existing-room creation remains idempotent despite clock failure, and injected entropy has explicit synchronized ownership outside database transactions.

Independent standard implementation review passed with no findings, covering the complete tracked/untracked diff, SQL, contracts, concurrency, bounded resource ownership, child process and documentation. The separate independent security implementation review also passed with no findings. It examined T-18/T-19/T-20/T-25/T-34/T-36/T-41 across credential parsing/storage, generation/role binding, limits, transaction cleanup, diagnostics, migration and test-process boundaries. No security finding required a fix; no trust boundary changed after these reviews. Final tier remains standard with the security trigger met. Only this evidence record was completed after review; executable file fingerprints remain unchanged.

## Check evidence

All commands ran against the implementation based on the base above. Two monitored runtime runs recorded identical before/after source fingerprints, valid sleep-protection observations and no unsafe sample. No automatic runtime repeat occurred.

| Check | Result | Evidence |
|---|---|---|
| Tool preparation | Passed | Five pinned tools copied from main with SHA-256 equality; `tools/check-tool-binaries.sh` exit 0 |
| Frozen frontend/documentation installs | Passed | Both exit 0; no lockfile changes |
| sqlc generation and diff | Passed | Both exit 0; generated infrastructure models included |
| Targeted vet | Passed | `go vet ./internal/adapters/meeting`, exit 0 |
| Initial targeted lint | Failed, corrected | Three gosec G115 conversions in shared counters; explicit range checks and signed limit comparison fixed them; no rule suppressed |
| Corrected targeted lint/vet | Passed | Both exit 0, lint reports zero issues |
| Provider runtime | Passed | `go -C backend test -race -count=1 ./internal/adapters/meeting`, exit 0; 12.06 seconds observed, four valid samples; covers L1 and SharedFake |
| Full backend static checks | Passed | `go mod tidy -diff`, gofmt empty-output assertion, `go vet ./...`, `golangci-lint run ./...`, `govulncheck ./...`, `sqlc diff`: each exit 0 |
| Full backend runtime | Passed | `go -C backend test -race -count=1 ./...`, exit 0; 93.57 seconds observed, 20 valid samples |
| Documentation checks | Passed | `tools/docs/check-docs.sh`, exit 0, including the completed review evidence |
| Staged secret scan | Passed | `bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .`, exit 0; no leaks |
| Frontend checks, E2E and remaining full-run checks | Deferred to merge gate | No frontend behavior changed; no older integration result substitutes for the exact-tree gate |
| Exact-tree merge gate and finish | Pending | No merge claim before successful gate and matching finish |

Govulncheck reported no vulnerabilities affecting this code and one advisory in a required module without a reachable affected call. This is not an assertion that every dependency is vulnerability-free; gate dependency scans remain mandatory.

## Acceptance coverage and limits

Real PostgreSQL tests cover the common Provider contract; concurrent initialization; mismatched limits; room/link/participant/generation bounds and arithmetic exhaustion; 16 competing link operations with exactly eight slots granted; persistent generation and role bindings; malformed, changed, foreign-namespace and stale capabilities; unavailability; cancellation of an observed row-lock wait; shared-clock use with one pooled connection; concurrent access to a non-thread-safe injected reader; SQL constraints and private-storage/log encodings. Migration 22 -> 23 -> 22 -> 23 preserves a pre-existing business row while removing populated new infrastructure on rollback.

The parent/child test exchanges fixture-only input through bounded stdin, observes shared create-time/status/presence and proves revocation after End. Child timeout/output-overflow failure paths were inspected through synchronous CommandContext.Run, bounded output and WaitDelay; they have no separate injected failure tests. Uncertain commits are documented rather than simulated through forbidden transport interception. Capacity counts scan bounded retained state; singleton serialization is intentionally not a BBB throughput model.

## Activation boundaries

Application routes, API/worker wiring, browser fake room, authorization and activation-time cleanup/reconciliation remain pending subsequent slice 7 changes. Active fake state remains bounded until End; automatic expiry is not promised. Independent deployments need independent namespace entropy; cloning state preserves provider identity. Real BBB compatibility, media, embedding and capacity remain unverified without the approved remote host. The L1 gate exception grants no exception here; historical failures remain preserved. No work was done to fit the paused 260-second slice 6 budget.
