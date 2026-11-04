# Browser lifecycle full gate accepted with a one-time duration exception

Date: 2026-11-04. Standard verification tier with security trigger T-41 for the
recorded policy exception. The owner explicitly accepted the completed gate's
263.6-second duration; the general 260-second ceiling remains unchanged. See the
[normative exception](../plans/slice-6.md#one-time-gate-duration-exception-2026-11-04).
The exception plan received independent security planning review; its major
finding required immediate normative recording rather than an optional future
evidence-only note. That clarification was verified before owner approval.

## Identity and outcome

- Base main: `4d9ba1e5467c84b1a09c1737c00ea2c8e4c24271`.
- Candidate and merged main: `a3b7d60d19efcba8d93acbc156fbc1f56cac1902`.
- Tested tree and main tree: `487083eb725fb48be26b60b8ee7289bddf403d30`.
- All 30 commands exited 0; all 979 E2E cases passed.
- The script's result was passed; the separate slice ceiling was exceeded by
  3.6 seconds. Only the explicit owner exception permitted merge.
- The observed wrapper took 264.327 seconds, with 54 safe samples and unchanged
  source. No gate retry, amend or change to the tested tree occurred.
- Fast-forward merge and finish both exited 0. Finish confirmed tree equality
  and released the successful gate lock. The merged working branch was deleted
  normally; its checkout and commit, and the prior 0880789 branch, are preserved.

The earlier [failed gate](check-budget-2026-11-04-browser-lifecycle-gate.md),
including its ERR_ABORTED and 268.8-second duration, remains failed. The later
successful diagnostic did not establish its cause. The owner accepted only that
limited historical uncertainty for this gate; no browser error allowance was
introduced. Registration and upload fixture consolidation evidence remains in
that record and the [upload fixture record](check-budget-2026-11-04-upload-fixtures.md).

## Environment and timing interpretation

The device has 12 physical and 12 logical CPUs. Compiler, dependency and operating
system caches were retained; frontend dependencies were installed in the new
worktree before the gate. No global caches were cleared, and exact residency was
not measured. This was not a cold measurement. Go tests used -race -count=1;
normal API/browser fixtures were created by the unchanged harness. Initial load
averages were approximately 1.70, 2.23 and 2.92. The other known chat was idle;
no other heavy verification was launched. Owned sleep/display assertions and
continuous monitoring were valid throughout. This does not prove zero background
OS activity or transfer standalone backend savings to concurrent gate lanes.

## Complete command report

```text
change: chore/browser-lifecycle-gate-followups
base main: 4d9ba1e5467c84b1a09c1737c00ea2c8e4c24271
tested tree: 487083eb725fb48be26b60b8ee7289bddf403d30
scope: full
check list: full
exit 0: pre: 0.2 s: .: tools/check-tool-binaries.sh
exit 0: C: 0.0 s: .: pnpm --dir tools/docs install --frozen-lockfile
exit 0: C: 10.3 s: .: tools/docs/check-docs.sh
exit 0: B: 2.1 s: .: bin/gitleaks git --redact --no-banner --config .gitleaks.toml .
exit 0: B: 0.1 s: .: bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .
exit 0: C: 5.8 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod
exit 0: C: 0.9 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod
exit 0: C: 1.6 s: .: bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml
exit 0: C: 1.1 s: .: bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml
exit 0: B: 0.1 s: backend: go mod tidy -diff
exit 0: B: 0.1 s: backend: unformatted="$(gofmt -l .)" && test -z "$unformatted"
exit 0: B: 0.3 s: backend: go vet ./...
exit 0: B: 1.0 s: backend: ../bin/golangci-lint run ./...
exit 0: B: 6.6 s: backend: ../bin/govulncheck ./...
exit 0: A: 91.8 s: backend: ../tools/dev-env.sh go test -race -count=1 ./...
exit 0: B: 3.3 s: backend: ../bin/sqlc -f sqlc.yaml diff
exit 0: C: 0.1 s: frontend: pnpm install --frozen-lockfile
exit 0: C: 6.9 s: frontend: pnpm typecheck
exit 0: C: 20.2 s: frontend: pnpm lint
exit 0: C: 2.9 s: frontend: pnpm format:check
exit 0: C: 9.6 s: frontend: pnpm test
exit 0: C: 1.1 s: frontend: pnpm build
exit 0: C: 1.0 s: frontend: pnpm check:classes
exit 0: C: 0.2 s: frontend: pnpm check:bundle
exit 0: C: 7.9 s: frontend: pnpm api:check
exit 0: C: 0.8 s: frontend: pnpm audit --audit-level high
exit 0: e2e: 170.5 s: frontend: ../tools/dev-env.sh pnpm e2e
exit 0: C: 4.8 s: .: python3 .claude/hooks/check_guard_secrets.py
exit 0: C: 1.0 s: .: tools/check-dev-env.sh
exit 0: B: 49.9 s: .: tools/check-merge-gate.sh
lane A wall time: 91.8 s
lane A command seconds: 91.8
lane B wall time: 63.6 s
lane B command seconds: 63.5
lane C wall time: 76.4 s
lane C command seconds: 76.2
summed command seconds: 402.2
gate wall time: 263.6 s
result: passed
```

## Finish report

```text
merge gate finish
mode: ff-only
change: chore/browser-lifecycle-gate-followups
base main: 4d9ba1e5467c84b1a09c1737c00ea2c8e4c24271
main after the merge: a3b7d60d19efcba8d93acbc156fbc1f56cac1902
tested tree: 487083eb725fb48be26b60b8ee7289bddf403d30
main tree: 487083eb725fb48be26b60b8ee7289bddf403d30
scope: full
check list: full
hash check: passed
check list check: passed
result: passed; the trees are equal and the lock is released
```

## Remaining obligations

The exception does not authorize future over-budget runs, weaken assertions or
replace any cold sample. Final measurement integration, the complete performance
matrix, review reconciliation, known-limitations dispositions and the owner's
post-slice budget decision remain outstanding. This separate documentation
change records the decision without modifying the tested application commit and
must complete its own reviews/checks/gate/merge/finish before measurement work
continues. Raw command logs and monitoring remain in private local artifacts;
this record contains the full safe command and identity evidence needed later.

## Verification-policy threat record

The standard readiness review identified a major omission: verification policy
is trust boundary 9, so this exception also requires a threat-model update in
the same change. The owner approved the narrow scope addition on 2026-11-04
after security planning review. The threat model now records the exact exception
under T-41 with unchanged general controls. This fix touches the verification
trust boundary, not application behavior. The pending fresh security readiness
review covers all three files and applies the standard checklist to this fix.
