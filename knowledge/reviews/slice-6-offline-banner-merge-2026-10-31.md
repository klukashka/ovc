# Offline banner merge evidence

F1 merged as `41bb7957dd0ffaf3c17c0ed6094b9a405c2814e5` from base `6670bf3fe444c03b889035e1fca7eefb2409d170`. Tested tree and main tree: `7951e1c54fd00f5260cd00e106efd688d7eed2c7`. The full gate passed in 235.4 seconds, below the temporary 260-second threshold; 902 end-to-end tests passed. The finish confirmed exact tree equality and released the lock. The merged branch was detached from its worktree and deleted with git branch -d.

The coordinator inspected other chats and process names before heavy work. An external project began Python checks during the gate; no exclusive-machine benchmark is claimed. All five prerequisite three-engine realtime series passed separately. Earlier failures and corrections remain in the [readiness record](slice-6-offline-banner-review.md).

## Full gate output

```text
merge gate: running pre, command logs in /Users/klukashka/Library/Caches/ovc-e2e/logs.9ArXth
merge gate: running A B C, command logs in /Users/klukashka/Library/Caches/ovc-e2e/logs.9ArXth
merge gate: running e2e, command logs in /Users/klukashka/Library/Caches/ovc-e2e/logs.9ArXth
merge gate report
mode: ff-only
change: feat/frontend-offline-banner
change head: 41bb7957dd0ffaf3c17c0ed6094b9a405c2814e5
base main: 6670bf3fe444c03b889035e1fca7eefb2409d170
tested tree: 7951e1c54fd00f5260cd00e106efd688d7eed2c7
scope: full
check list: full
go environment in .: GOVERSION=go1.27.3 GOTOOLCHAIN=auto GOFLAGS= GOWORK= GOENV=/Users/klukashka/Library/Application Support/go/env changed: none
go environment in backend: GOVERSION=go1.27.3 GOTOOLCHAIN=auto GOFLAGS= GOWORK= GOENV=/Users/klukashka/Library/Application Support/go/env changed: none
go environment in tools: GOVERSION=go1.27.3 GOTOOLCHAIN=auto GOFLAGS= GOWORK= GOENV=/Users/klukashka/Library/Application Support/go/env changed: none
node environment: node v26.12.1, pnpm 12.14.0, pnpm node-options, script-shell and shell-emulator: defaults
end-to-end run lock: /Users/klukashka/Library/Caches/ovc-e2e/run.lock, taken by this gate (pid 79702) before its first check and released after its last
earlier attempts for this tree: none
exit 0: pre: 0.5 s: .: tools/check-tool-binaries.sh
exit 0: C: 0.1 s: .: pnpm --dir tools/docs install --frozen-lockfile
exit 0: C: 11.8 s: .: tools/docs/check-docs.sh
exit 0: B: 1.8 s: .: bin/gitleaks git --redact --no-banner --config .gitleaks.toml .
exit 0: B: 0.1 s: .: bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .
exit 0: C: 2.9 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod
exit 0: C: 0.9 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod
exit 0: C: 1.3 s: .: bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml
exit 0: C: 1.0 s: .: bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml
exit 0: B: 0.1 s: backend: go mod tidy -diff
exit 0: B: 0.1 s: backend: unformatted="$(gofmt -l .)" && test -z "$unformatted"
exit 0: B: 2.5 s: backend: go vet ./...
exit 0: B: 3.5 s: backend: ../bin/golangci-lint run ./...
exit 0: B: 5.4 s: backend: ../bin/govulncheck ./...
exit 0: A: 85.1 s: backend: ../tools/dev-env.sh go test -race -count=1 ./...
exit 0: B: 1.7 s: backend: ../bin/sqlc -f sqlc.yaml diff
exit 0: C: 0.1 s: frontend: pnpm install --frozen-lockfile
exit 0: C: 0.8 s: frontend: pnpm typecheck
exit 0: C: 15.7 s: frontend: pnpm lint
exit 0: C: 1.9 s: frontend: pnpm format:check
exit 0: C: 7.4 s: frontend: pnpm test
exit 0: C: 0.7 s: frontend: pnpm build
exit 0: C: 0.8 s: frontend: pnpm check:classes
exit 0: C: 0.2 s: frontend: pnpm check:bundle
exit 0: C: 7.9 s: frontend: pnpm api:check
exit 0: C: 0.8 s: frontend: pnpm audit --audit-level high
exit 0: e2e: 148.7 s: frontend: ../tools/dev-env.sh pnpm e2e
exit 0: C: 5.2 s: .: python3 .claude/hooks/check_guard_secrets.py
exit 0: C: 1.1 s: .: tools/check-dev-env.sh
exit 0: B: 50.6 s: .: tools/check-merge-gate.sh
lane pre wall time: 0.6 s
lane pre command seconds: 0.5
lane A wall time: 85.1 s
lane A command seconds: 85.1
lane B wall time: 65.8 s
lane B command seconds: 65.8
lane C wall time: 60.5 s
lane C command seconds: 60.6
lane e2e wall time: 148.7 s
lane e2e command seconds: 148.7
phase pre wall time with the wait for its processes: 0.6 s
phase A B C wall time with the wait for its processes: 85.2 s
phase e2e wall time with the wait for its processes: 148.8 s
summed command seconds: 360.7
gate wall time: 235.4 s
cores: 12
command logs: /Users/klukashka/Library/Caches/ovc-e2e/logs.9ArXth
result: passed
next: in the checkout of main run git merge --ff-only feat/frontend-offline-banner, then merge-gate.sh finish
```

## Finish output

```text
merge gate finish
mode: ff-only
change: feat/frontend-offline-banner
base main: 6670bf3fe444c03b889035e1fca7eefb2409d170
main after the merge: 41bb7957dd0ffaf3c17c0ed6094b9a405c2814e5
tested tree: 7951e1c54fd00f5260cd00e106efd688d7eed2c7
main tree: 7951e1c54fd00f5260cd00e106efd688d7eed2c7
scope: full
check list: full
hash check: passed
check list check: passed
result: passed; the trees are equal and the lock is released
```
