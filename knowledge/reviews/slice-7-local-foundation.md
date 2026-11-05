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

The independent standard review examined the complete 15-file diff and found no blocker, major or minor findings. Vulnerability analysis passed: govulncheck found no affected code or imported packages, and reported one module-level advisory without a reachable call. Staged Gitleaks found no leaks. Independent security review examined the complete implementation and final documentation changes, with no blocker, major or minor findings. It covered T-18/T-19/T-25 credential and diagnostic boundaries, T-20 resource/cancellation behavior, T-36 unchanged control surfaces and T-41 local-versus-remote evidence. Application authorization and remote provider/media boundaries remain outside L1. The final tier remains standard with the security trigger met; no security finding required a fix. The exact-tree merge gate remains pending; its command results and finish identity are recorded separately by the merge report.

Frontend and full integration checks not directly exercised by L1 remain deferred to the merge gate. Real BBB contract execution, media, embedding, host capacity and external processing destinations remain unverified pending the remote stage. Slice 6 accepted limitations remain in their [accepted-state record](slice-6-accepted-state.md); that acceptance does not waive a new failed check.
