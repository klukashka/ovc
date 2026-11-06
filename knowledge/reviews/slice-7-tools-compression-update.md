# Tool compression update verification

Date: 2026-11-06. Base: `8664fabbae992844c6be1aef986dcb5b30b9e508`. Tier: standard; security trigger: T-39, T-19 and T-41. Scope: two tools-module versions/checksums and their decision/evidence records; no application source or test changes.

## Change checks

The documented pinned-tool build completed with exit 0. `tools/check-tool-binaries.sh` exited 0 and printed `bin/ matches tools/go.mod`. Both frozen offline dependency installations preserve their lockfiles. Public release dates satisfy ADR-0017; no additional module version changed.

- `.: bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod`: exit 0, 2.9 seconds.
- `.: bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod`: exit 0, 0.96 seconds.
- `.: bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml`: exit 0, 1.32 seconds.
- `.: bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml`: exit 0, 1.6 seconds.
- `backend: ../bin/golangci-lint run ./...`: exit 0, 4.82 seconds.
- `backend: ../bin/govulncheck ./...`: exit 0, 3.05 seconds.
- `backend: ../bin/sqlc -f sqlc.yaml diff`: exit 0, 1.29 seconds.
- `.: bin/gitleaks git --redact --no-banner --config .gitleaks.toml .`: exit 0, 1.82 seconds.
- `tools: go mod verify`: exit 0, 10.0 seconds.
- `tools: go mod tidy -diff`: exit 0, 0.14 seconds.

Conventions tests (`go test -race -count=1 ./internal/conventions`) passed in 2.610 seconds. Staged Gitleaks passed with no leaks. The first documentation check failed MD012 for one extra blank line in the threat-model addition; that formatting error was corrected and the complete documentation check passed. The failed result is preserved. Independent reviews are recorded separately before readiness; no review or gate pass is inferred from these checks. Other unchanged application checks, including the complete backend and E2E suites, are deferred to the ordinary full merge gate; they are not reported passed. Final outcomes and exact-tree merge/finish evidence must be collected before integration.

## Preserved preceding storage gate

The storage commit `816aaf0709edbfc7e22a18ae2cb96a64a8579ded`, tree `786610bda42257bb0e0cc22575587423e0cb500c`, failed its authorized gate on the same base: 13 commands passed, the tools OSV scan failed, and 16 commands did not run. Full backend race testing passed in 100.5 seconds, but frontend/E2E did not execute. Gate wall time was 101.7 seconds; the external monitor recorded 102.66 seconds, 22 safe observations and unchanged source. Neither the storage commit nor a failed gate was merged. The new dependency change requires its own gate; storage integration later requires another exact-tree gate.

```text
exit 0: pre: 0.7 s: .: tools/check-tool-binaries.sh
exit 0: C: 0.1 s: .: pnpm --dir tools/docs install --frozen-lockfile
exit 0: C: 11.8 s: .: tools/docs/check-docs.sh
exit 0: B: 2.4 s: .: bin/gitleaks git --redact --no-banner --config .gitleaks.toml .
exit 0: B: 0.1 s: .: bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .
exit 1: C: 2.9 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod
not run: C: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod
not run: C: .: bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml
not run: C: .: bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml
exit 0: B: 0.1 s: backend: go mod tidy -diff
exit 0: B: 0.1 s: backend: unformatted="$(gofmt -l .)" && test -z "$unformatted"
exit 0: B: 0.5 s: backend: go vet ./...
exit 0: B: 1.4 s: backend: ../bin/golangci-lint run ./...
exit 0: B: 5.6 s: backend: ../bin/govulncheck ./...
exit 0: A: 100.5 s: backend: ../tools/dev-env.sh go test -race -count=1 ./...
exit 0: B: 0.3 s: backend: ../bin/sqlc -f sqlc.yaml diff
not run: C: frontend: pnpm install --frozen-lockfile
not run: C: frontend: pnpm typecheck
not run: C: frontend: pnpm lint
not run: C: frontend: pnpm format:check
not run: C: frontend: pnpm test
not run: C: frontend: pnpm build
not run: C: frontend: pnpm check:classes
not run: C: frontend: pnpm check:bundle
not run: C: frontend: pnpm api:check
not run: C: frontend: pnpm audit --audit-level high
not run: e2e: frontend: ../tools/dev-env.sh pnpm e2e
not run: C: .: python3 .claude/hooks/check_guard_secrets.py
not run: C: .: tools/check-dev-env.sh
exit 0: B: 47.5 s: .: tools/check-merge-gate.sh
```
