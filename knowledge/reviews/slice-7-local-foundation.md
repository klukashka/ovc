# Slice 7 local meeting-provider foundation

## Scope and acceptance

Implementation base: `93dea3d5e9d00ac9770c969039581ef71880a64a`. The [approved plan](../plans/slice-7-local-foundation.md) permits L1: typed meeting values, the four-method provider interface, bounded injected fake, lifecycle and credential tests, architecture and documentation updates. It does not add database migrations, application routes, worker wiring, browser rooms or the BBB adapter.

Verification tier: standard. The security trigger applies to meeting credentials, diagnostics, resource bounds and the distinction between local and remote verification. Independent standard and security reviews are required over the complete change. Planning reviews completed before approval; they are not implementation acceptance.

## Implementation boundaries

The fake owns synchronized instance state. Creation returns a monotonic synthetic create time even under a frozen clock. Handles and synthetic links bind to a generation; links also bind the opaque participant identifier and role. End releases meeting, participant and link state. Configured capacities refuse overflow without eviction; lifetime generation and link counters are bounded scalars. Participant role bindings survive departure until end. These are local simulation semantics, not certified BBB behavior.

The provider emits fixed structured completion/failure events with operation and outcome or failure kind. Names, identifiers, raw errors and link values are excluded. The sensitive link uses the existing secret type with explicit consumer access. Cancellation prevents state publication before the synchronized transition; it does not interrupt an arbitrary injected blocking reader or mutex wait. L1 exposes no request route and its normal injected sources must finish their operations.

## Verification status

The five named acceptance suites passed with `go test -race -count=1 ./internal/adapters/meeting ./internal/domain`. The import graph selected 33 of 36 backend packages, including conventions and command packages; the complete `go test -race -count=1 ./...` then passed in 90.0 seconds through the sanctioned root environment wrapper. All 19 power/load observations were valid and the source identity was unchanged.

`gofmt`, `git diff --cached --check`, `go vet ./...`, and golangci-lint `./...` passed. The pinned tool-binary check passed. Documentation checks passed, including Markdown, links and Mermaid rendering. Initial sandbox attempts could not access the Go build cache or launch Chromium; neither produced a test assertion failure. The ordinary commands succeeded with the necessary local permissions, without changing checks.

The independent standard review examined the complete 15-file diff and found no blocker, major or minor findings. Vulnerability analysis passed: govulncheck found no affected code or imported packages, and reported one module-level advisory without a reachable call. Staged Gitleaks found no leaks. Independent security review examined the complete implementation and final documentation changes, with no blocker, major or minor findings. It covered T-18/T-19/T-25 credential and diagnostic boundaries, T-20 resource/cancellation behavior, T-36 unchanged control surfaces and T-41 local-versus-remote evidence. Application authorization and remote provider/media boundaries remain outside L1. The final tier remains standard with the security trigger met; no security finding required a fix. The exact-tree merge gate failed as recorded below; integration proceeded only under the explicit one-change owner exception, not a normal gate pass.

Frontend and full integration checks outside the targeted L1 change checks ran in the full gate below; its one failed E2E remains failed. Real BBB contract execution, media, embedding, host capacity and external processing destinations remain unverified pending the remote stage. Slice 6 accepted limitations remain in their [accepted-state record](slice-6-accepted-state.md); that acceptance does not waive a new failed check.

## Owner-approved exception integration, 2026-11-05

The owner explicitly accepted the unresolved password-reset E2E failure for commit `0236b0b1594f3c6d0d2ab21a226c4dc131cb652e` and tree `9044627e2c553903455a8f38848de3e0221f538e` only. Base main was `93dea3d5e9d00ac9770c969039581ef71880a64a`. Before the fast-forward, these exact identities and clean tracked worktree/index were checked. After it, main commit and tree equalled the approved candidate. Normal `finish` was not run because the gate failed; this was exception acceptance, not a successful normal gate/finish. The standard release command found no remaining gate lock.

The owner also explicitly permitted temporary deferral of the committed policy record until this mandatory separate documentation change. [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#one-change-exception-for-the-local-meeting-foundation) records the one-change exception. This documentation change requires its own independent standard/security reviews and ordinary documentation gate/finish before further application implementation. It does not rerun or certify the failed application gate.

The gate passed 29 commands and 978 of 979 E2E cases. The Chromium password-reset test failed at `frontend/e2e/signup.spec.ts:882`: `Network.getResponseBody` could not provide the account 401 body. Subsequent diagnostics recorded two HTTP errors and one console error. This matches the previously accepted failure surface, but identical root cause and harmlessness are not proven. Authentication and frontend executable code were unchanged; the new provider has no application callers. The unresolved uncertainty remains tracked for slice 10 deployment hardening. No assertion, retry, timeout, scanner or future gate requirement is weakened.

Gate wall time was 262.8 seconds; summed command time 405.0 seconds, on 12 cores. Lane wall times: pre 0.9, A 97.7, B 61.5, C 81.8, E2E 163.3 seconds. These are observed timings, not cold performance acceptance or a new budget. The monitor recorded 54 valid power/load samples and unchanged sources. An earlier stale-manifest preflight refused before launching any gate command; it was not an E2E attempt. No retry followed the failed gate.

Private raw traces/logs remain local under the existing seven-day maximum retention rule; no credentials, bodies or raw traces are included here. The scalar record below is durable; temporary artifacts are not evidence-retention guarantees.

### Exact-tree gate command results

| Command | Working directory | Exit |
| --- | --- | --- |
| `tools/check-tool-binaries.sh` | `.` | 0 |
| `pnpm --dir tools/docs install --frozen-lockfile` | `.` | 0 |
| `tools/docs/check-docs.sh` | `.` | 0 |
| `bin/gitleaks git --redact --no-banner --config .gitleaks.toml .` | `.` | 0 |
| `bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .` | `.` | 0 |
| `bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod` | `.` | 0 |
| `bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod` | `.` | 0 |
| `bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml` | `.` | 0 |
| `bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml` | `.` | 0 |
| `go mod tidy -diff` | `backend` | 0 |
| `unformatted="$(gofmt -l .)" && test -z "$unformatted"` | `backend` | 0 |
| `go vet ./...` | `backend` | 0 |
| `../bin/golangci-lint run ./...` | `backend` | 0 |
| `../bin/govulncheck ./...` | `backend` | 0 |
| `../tools/dev-env.sh go test -race -count=1 ./...` | `backend` | 0 |
| `../bin/sqlc -f sqlc.yaml diff` | `backend` | 0 |
| `pnpm install --frozen-lockfile` | `frontend` | 0 |
| `pnpm typecheck` | `frontend` | 0 |
| `pnpm lint` | `frontend` | 0 |
| `pnpm format:check` | `frontend` | 0 |
| `pnpm test` | `frontend` | 0 |
| `pnpm build` | `frontend` | 0 |
| `pnpm check:classes` | `frontend` | 0 |
| `pnpm check:bundle` | `frontend` | 0 |
| `pnpm api:check` | `frontend` | 0 |
| `pnpm audit --audit-level high` | `frontend` | 0 |
| `../tools/dev-env.sh pnpm e2e` | `frontend` | 1 |
| `python3 .claude/hooks/check_guard_secrets.py` | `.` | 0 |
| `tools/check-dev-env.sh` | `.` | 0 |
| `tools/check-merge-gate.sh` | `.` | 0 |

### Exception-record verification

Independent standard and full-security reviews examined the complete four-document amendment against the integrated L1 commit, including the exact gate log and owner-approved exception. Both found no blocker, major or minor findings. The standard review confirmed all 30 command rows against the original log. Tool-binary validation, documentation checks, diff checks and staged Gitleaks passed. The final tier remains standard with the T-41 security trigger; the ordinary documentation gate and finish are required for this record and their exact-tree results are retained in the repository merge report. This does not change the failed application gate outcome.
