# Browser lifecycle merge gate failure

Commit: 0880789c7482d1dfa6babbb893726ece5ded171b. Main did not advance.

29 commands passed; E2E failed with 978 passed and one failed. The Chromium reset
case failed at signup.spec.ts:882. Its account response has net::ERR_ABORTED;
response.json reported no data for the resource identifier. Cause is unverified.
The gate also exceeded the 260-second limit. No rerun or merge occurred.
The observer recorded 55 safe samples and unchanged source.

```text
change: chore/browser-diagnostics-document-scope
base main: 4d9ba1e5467c84b1a09c1737c00ea2c8e4c24271
tested tree: 3702707df4e3775414f2f28ef10efeb487b656d3
scope: full
check list: full
exit 0: pre: 0.2 s: .: tools/check-tool-binaries.sh
exit 0: C: 0.1 s: .: pnpm --dir tools/docs install --frozen-lockfile
exit 0: C: 10.7 s: .: tools/docs/check-docs.sh
exit 0: B: 2.1 s: .: bin/gitleaks git --redact --no-banner --config .gitleaks.toml .
exit 0: B: 0.1 s: .: bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .
exit 0: C: 1.5 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod
exit 0: C: 0.9 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod
exit 0: C: 1.6 s: .: bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml
exit 0: C: 1.0 s: .: bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml
exit 0: B: 0.1 s: backend: go mod tidy -diff
exit 0: B: 0.1 s: backend: unformatted="$(gofmt -l .)" && test -z "$unformatted"
exit 0: B: 0.4 s: backend: go vet ./...
exit 0: B: 3.6 s: backend: ../bin/golangci-lint run ./...
exit 0: B: 3.0 s: backend: ../bin/govulncheck ./...
exit 0: A: 93.3 s: backend: ../tools/dev-env.sh go test -race -count=1 ./...
exit 0: B: 0.3 s: backend: ../bin/sqlc -f sqlc.yaml diff
exit 0: C: 0.1 s: frontend: pnpm install --frozen-lockfile
exit 0: C: 0.1 s: frontend: pnpm typecheck
exit 0: C: 20.6 s: frontend: pnpm lint
exit 0: C: 2.6 s: frontend: pnpm format:check
exit 0: C: 9.1 s: frontend: pnpm test
exit 0: C: 1.0 s: frontend: pnpm build
exit 0: C: 0.8 s: frontend: pnpm check:classes
exit 0: C: 0.2 s: frontend: pnpm check:bundle
exit 0: C: 2.4 s: frontend: pnpm api:check
exit 0: C: 0.8 s: frontend: pnpm audit --audit-level high
exit 1: e2e: 174.2 s: frontend: ../tools/dev-env.sh pnpm e2e
exit 0: C: 5.1 s: .: python3 .claude/hooks/check_guard_secrets.py
exit 0: C: 0.7 s: .: tools/check-dev-env.sh
exit 0: B: 46.8 s: .: tools/check-merge-gate.sh
lane A wall time: 93.3 s
lane A command seconds: 93.3
lane B wall time: 56.5 s
lane B command seconds: 56.5
lane C wall time: 59.4 s
lane C command seconds: 59.3
summed command seconds: 383.5
gate wall time: 268.8 s
result: failed: a check failed
```

Hardware inventory after the run reports 12 physical and 12 logical CPUs. This was not
a cold measurement: no build, dependency or operating-system cache was cleared; prior
frontend checks and E2E had run. Go test used -race -count=1, disabling test-result
reuse while retaining build caches. Browser/application fixtures were created by the
normal harness. Exact cache residency was not measured.

## Registration consolidation observations

The original candidate passed the targeted registration command in 4.145 seconds
and the complete backend command in 87.293 seconds. Both used
`go test -json -race -count=1`, with the two named registration tables selected
for the targeted command and `./...` for the complete command.

The consolidated source passed all 15 registration cases in the targeted command
(10.129 seconds including compilation). Its complete backend command failed in
85.038 seconds: `TestProjectLintRulesFire` could not execute the new worktree's
missing `bin/golangci-lint`. No other test failure was reported. This was a
worktree preparation error: the required tool provenance preflight had been
omitted before that command. The failed duration is not a successful performance
comparison or evidence that the gate budget is met.

Both observed batches retained their sources unchanged and every monitoring
sample was safe. Original and consolidated runs retained Go build caches; neither
was a cold measurement. The new source needed compilation, so targeted durations
are not isolated fixture-cost measurements.

The five existing pinned tool binaries were subsequently copied from the original
candidate's ignored bin directory, each with matching SHA256. The new worktree's
`tools/check-tool-binaries.sh` passed with `bin/ matches tools/go.mod`. No source,
assertion, timeout or retry setting changed. The failed full-backend run remains
failed; one corrected execution requires a separate owner decision. Chromium
diagnostic preparation is independent and remains within its original approval.

The owner approved one corrected complete backend execution on 2026-11-04,
with the same source and checks after tool preparation. This does not authorize
a new gate or merge. The prior failed execution remains part of the record.

The corrected complete backend execution passed (exit 0) in 86.128 seconds.
The monitored wrapper took 86.315 seconds with 19 safe samples and unchanged
source. Against the original successful 87.293-second backend execution, the
observed difference is 1.165 seconds. Single observations, retained build caches
and different gate lane contention do not establish a guaranteed improvement.
This evidence does not justify claiming recovery of the gate's 8.8-second deficit.
The targeted baseline and changed logs contain exactly the same 15 named passing
subtests. The previous failed 85.038-second execution is not used as a performance
success.

## Bounded reset diagnostic and negative control

The isolated Chromium diagnostic passed once (exit 0; observed command 6.828
seconds, three safe samples). Original candidate and diagnostic source hashes
were unchanged. Before execution, TypeScript, ESLint, Prettier and Vite build
passed; the initial ESLint's five formatting/style findings and its failure were
preserved and fixed without changing behavior. The test list contained exactly
one Chromium reset case.

The bounded observation correlated the one post-reset loadMe operation with the
account GET. Its 401 response arrived after the session epoch had changed; the
owner drained 124 bytes, observed EOF, released the reader and settled without
an abort. Chromium and Playwright observed finished events for both account and
refresh refusals; the JSON lookup succeeded before cleanup. All original JSON,
status, terminal and UI assertions passed. No overflow, observer failure,
identity ambiguity or cleanup failure occurred. Browser and Node clock domains
remain unaligned: this does not establish cross-source causal timing. This
success does not explain the historical ERR_ABORTED or authorize acceptance,
a gate, a production change or another diagnostic execution.

The canonicalization negative control in a separate source copy changed only
the first resolveZone fallback to Europe/Moscow. It returned exit 1 as intended:
all six timezone assertions failed with HTTP 202, a successful repository lookup
and stored Europe/Moscow; all nine invalid-input cases passed. The observed
command took 8.991 seconds, with three safe samples and unchanged original source.
This confirms the retained assertions detect incorrect canonicalization; no
production code was changed by the control.

Private execution artifacts are retained outside served roots for at most seven
days. They are named registration-baseline-results, registration-changed-results,
registration-corrected-results and registration-negative-results under the private
temporary directory; corresponding observed directories retain monitoring and
source snapshots. reset-owner-diagnostic-1 retains the bounded observer record
and manifest; the isolated diagnostic copy retains its full local trace.

## Consolidation review and static checks

A fresh independent combined standard/security review inspected the consolidation
delta against 0880789, including its integration and rate-limit isolation. It
reported no blocker or major finding. Its minor finding requested hardware and
cache conditions in this record; those conditions are now stated above. The
review confirmed preserved assertions and unique case identities. Review was
static; the execution evidence is recorded separately above.

Gofmt and go vet passed. The initial sandboxed golangci-lint command exited 5
during package loading, reporting no Go files to analyze; go list reported no
package errors. The same lint command with access to host caches passed (exit 0),
without source or configuration changes. The initial tool failure is preserved;
its exact environmental cause was not established. No backend assertion was
rerun by this lint invocation.

Readiness remains blocked by the historical reset uncertainty and lack of
evidence that the complete gate fits its unchanged 260-second budget. No new
full acceptance E2E, gate, commit or merge has been performed.
