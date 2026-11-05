# Meeting recovery ledger verification

Base: `01c3f8e1e8b78e8316cf1e09718d7cf89e02e11a`. Tier: standard; security trigger met by provider ownership, generation evidence and purge. Scope: [approved plan](../plans/slice-7-meeting-recovery-ledger.md).

Independent security planning review passed with no blocker or major finding. Two minor clarifications were incorporated before implementation: explicit immutable legacy/attempt provenance and serialized nonempty downgrade refusal. Evidence revision excludes the foreign-key SET NULL change; dispatch/publication recheck actual current state. Pagination uses an immutable key and the full cleanup predicate in its index.

Implementation and change checks passed. Fresh independent standard and separate full-security implementation reviews passed over the complete tracked/untracked diff based on the revision above, with no blocker, major or minor finding. Both reviewed the migration, SQL, repository, generated bindings, domain types, actual purge integration tests and specifications; only factual verification evidence was updated afterward. No security finding required a fix. Final classification remains standard with the security trigger met. At commit preparation gate and finish were pending; the subsequently failed gate and newly approved exact exception are recorded below. No earlier exception was reused.

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
| Frontend, E2E and remaining full-run checks | Subsequently executed by the full gate: 29 commands passed; E2E failed with 978/979 passed, as recorded below |

Real PostgreSQL tests prove atomic two-row creation and rollback, including a ledger uniqueness conflict after business insertion; conditional dispatch and immutable acknowledged generations; conservative migration/provenance and refusal of nonempty downgrade; cleanup/late-completion races in both orders with observed row-lock barriers; cancellation and eventual connection release; unchanged-revision refusal after FK nulling; replacement isolation; and indexed keyset pages over 105 mixed candidates. Existing API worker-driven purge tests prove final tutor purge preserves the orphan record and student purge preserves tutor history. No provider cleanup is inferred from those database results.

Two declared negative controls ran once each in separate copies of the same source. Removing the Open cleanup predicate made `TestMeetingRecoveryTransitions/publication_requires_returned_generation_without_cleanup/cleanup` fail with `unproven or cleanup generation published` (command exit 1, 3.36 seconds, two safe observations). Replacing the recovery foreign key's SET NULL action with CASCADE made `TestMeetingRecoveryPurge/unchanged_revision_cannot_dispatch_or_publish_after_purge` fail with `get meeting recovery: not found` (command exit 1, 2.79 seconds, two safe observations). These are successful sensitivity controls, not passing application runs. The approved tree was unchanged and its targeted/full runs passed.

The standard reviewer verified the available indexed access path and resource consequences; this is not a measured planner/latency claim. The security reviewer examined T-18/T-19/T-20/T-25/T-34/T-36/T-41 and the deferred activation boundary. Neither reviewer reran runtime checks. No assertion, timeout, dependency, diagnostic policy or gate budget was relaxed.

## Owner-approved exception integration 2026-11-06

- Base main: `01c3f8e1e8b78e8316cf1e09718d7cf89e02e11a`.
- Change and main after fast-forward: `be8290afed843561e1b02a6a93e96f54b7101cea`.
- Tested tree and main tree after fast-forward: `98068b9b3d6bb26f4b0c573ed838ba7c8e741202`.
- Gate executed on 2026-11-05: **failed**, 29 commands exit 0, E2E exit 1, 978/979 cases passed. Wall time 266.3 seconds; summed command time 396.8 seconds. This was not a general ten-minute budget failure.
- Monitoring: 267.12 seconds, 54 safe samples, unchanged source. Exact clean change head/tree, unchanged target base and absence of live gate locks were checked before integration; independent post-merge checks confirmed the exact main commit and tree. Main's unrelated untracked agent-configuration directories were not included or changed.

The one failed case was Chromium's password reset in an already signed-in browser. At `frontend/e2e/signup.spec.ts:882`, `response.json()` failed because DevTools Network.getResponseBody had no data for the resource identifier. Diagnostics also recorded four unknown authentication HTTP errors and two console errors. These are failures, not suppressed diagnostics. The same test and primary symptom occurred previously, but a common cause and harmlessness remain unverified. Raw traces remain private; only safe categories and counts are retained here.

On 2026-11-06 the owner explicitly accepted this limited uncertainty for this exact commit/tree and authorized fast-forward without a rerun, together with the immediate mandatory documentation follow-up. Security planning review of that disposition passed without findings. The application gate remains failed. Normal finish was not used to certify it; identity checks do not replace a passing gate. The merged feature branch was deleted normally after its worktree was detached. No deployment, remote-provider acceptance, slice completion or future failed merge is authorized by this decision.

### Every command from the failed gate

```text
exit 0: pre: 0.6 s: .: tools/check-tool-binaries.sh
exit 0: C: 0.1 s: .: pnpm --dir tools/docs install --frozen-lockfile
exit 0: C: 11.6 s: .: tools/docs/check-docs.sh
exit 0: B: 2.4 s: .: bin/gitleaks git --redact --no-banner --config .gitleaks.toml .
exit 0: B: 0.1 s: .: bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .
exit 0: C: 2.9 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod
exit 0: C: 0.8 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod
exit 0: C: 3.0 s: .: bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml
exit 0: C: 1.3 s: .: bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml
exit 0: B: 0.1 s: backend: go mod tidy -diff
exit 0: B: 0.1 s: backend: unformatted="$(gofmt -l .)" && test -z "$unformatted"
exit 0: B: 0.5 s: backend: go vet ./...
exit 0: B: 0.9 s: backend: ../bin/golangci-lint run ./...
exit 0: B: 2.9 s: backend: ../bin/govulncheck ./...
exit 0: A: 98.1 s: backend: ../tools/dev-env.sh go test -race -count=1 ./...
exit 0: B: 0.3 s: backend: ../bin/sqlc -f sqlc.yaml diff
exit 0: C: 0.1 s: frontend: pnpm install --frozen-lockfile
exit 0: C: 7.0 s: frontend: pnpm typecheck
exit 0: C: 20.7 s: frontend: pnpm lint
exit 0: C: 2.8 s: frontend: pnpm format:check
exit 0: C: 11.8 s: frontend: pnpm test
exit 0: C: 1.1 s: frontend: pnpm build
exit 0: C: 0.8 s: frontend: pnpm check:classes
exit 0: C: 0.2 s: frontend: pnpm check:bundle
exit 0: C: 4.3 s: frontend: pnpm api:check
exit 0: C: 0.6 s: frontend: pnpm audit --audit-level high
exit 1: e2e: 166.6 s: frontend: ../tools/dev-env.sh pnpm e2e
exit 0: C: 5.0 s: .: python3 .claude/hooks/check_guard_secrets.py
exit 0: C: 1.0 s: .: tools/check-dev-env.sh
exit 0: B: 49.1 s: .: tools/check-merge-gate.sh
```

### Mandatory documentation follow-up

This separate documentation change records the exact approved exception in ADR-0018, boundary 9/T-41, this evidence and the slice plan. Fresh independent standard and full-security reviews of the complete five-file documentation diff passed with no findings. Documentation checks and staged Gitleaks passed, exit 0; only this factual evidence sentence changed after review. Tier remains standard with the security trigger met; no fixed security finding requires a trust-boundary assessment. Ordinary docs-only gate and finish remain pending at documentation commit preparation. That gate certifies only its own documentation tree. Further application implementation waits for this follow-up to complete.
