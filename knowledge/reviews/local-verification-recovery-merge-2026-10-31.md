# Local verification recovery merge evidence

The verification recovery merged by fast-forward on 2026-10-31 as `d23c7fd317e0323a0fa3646de928a7eaecf7f894`, based on `e95d27281587699df8286dac335927dd13488631`. The full gate exited 0; merge exited 0; finish exited 0 and confirmed equal trees. The complete command list and effective tool settings follow. Earlier gate failures are preserved in the [recovery review](local-verification-recovery-2026-10-31.md) and the presence completion review; none was silently relabelled as passing.

## Gate

```text
merge gate: running pre, command logs in /Users/klukashka/Library/Caches/ovc-e2e/logs.WfV2Aq
merge gate: running A B C, command logs in /Users/klukashka/Library/Caches/ovc-e2e/logs.WfV2Aq
merge gate: running e2e, command logs in /Users/klukashka/Library/Caches/ovc-e2e/logs.WfV2Aq
merge gate report
mode: ff-only
change: chore/verification-fixture-isolation
change head: d23c7fd317e0323a0fa3646de928a7eaecf7f894
base main: e95d27281587699df8286dac335927dd13488631
tested tree: a4c365984be6cc2ecf0ca0bac691ec24492c0946
scope: full
check list: full
go environment in .: GOVERSION=go1.27.3 GOTOOLCHAIN=auto GOFLAGS= GOWORK= GOENV=/Users/klukashka/Library/Application Support/go/env changed: none
go environment in backend: GOVERSION=go1.27.3 GOTOOLCHAIN=auto GOFLAGS= GOWORK= GOENV=/Users/klukashka/Library/Application Support/go/env changed: none
go environment in tools: GOVERSION=go1.27.3 GOTOOLCHAIN=auto GOFLAGS= GOWORK= GOENV=/Users/klukashka/Library/Application Support/go/env changed: none
node environment: node v26.12.1, pnpm 12.14.0, pnpm node-options, script-shell and shell-emulator: defaults
end-to-end run lock: /Users/klukashka/Library/Caches/ovc-e2e/run.lock, taken by this gate (pid 15681) before its first check and released after its last
earlier attempts for this tree: none
exit 0: pre: 0.2 s: .: tools/check-tool-binaries.sh
exit 0: C: 0.2 s: .: pnpm --dir tools/docs install --frozen-lockfile
exit 0: C: 10.0 s: .: tools/docs/check-docs.sh
exit 0: B: 1.8 s: .: bin/gitleaks git --redact --no-banner --config .gitleaks.toml .
exit 0: B: 0.1 s: .: bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .
exit 0: C: 2.1 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod
exit 0: C: 0.9 s: .: bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod
exit 0: C: 1.6 s: .: bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml
exit 0: C: 1.3 s: .: bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml
exit 0: B: 0.1 s: backend: go mod tidy -diff
exit 0: B: 0.1 s: backend: unformatted="$(gofmt -l .)" && test -z "$unformatted"
exit 0: B: 0.3 s: backend: go vet ./...
exit 0: B: 1.0 s: backend: ../bin/golangci-lint run ./...
exit 0: B: 3.9 s: backend: ../bin/govulncheck ./...
exit 0: A: 66.1 s: backend: ../tools/dev-env.sh go test -race -count=1 ./...
exit 0: B: 0.3 s: backend: ../bin/sqlc -f sqlc.yaml diff
exit 0: C: 0.1 s: frontend: pnpm install --frozen-lockfile
exit 0: C: 1.1 s: frontend: pnpm typecheck
exit 0: C: 15.1 s: frontend: pnpm lint
exit 0: C: 1.9 s: frontend: pnpm format:check
exit 0: C: 6.6 s: frontend: pnpm test
exit 0: C: 0.5 s: frontend: pnpm build
exit 0: C: 0.8 s: frontend: pnpm check:classes
exit 0: C: 0.2 s: frontend: pnpm check:bundle
exit 0: C: 0.8 s: frontend: pnpm api:check
exit 0: C: 1.5 s: frontend: pnpm audit --audit-level high
exit 0: e2e: 129.6 s: frontend: ../tools/dev-env.sh pnpm e2e
exit 0: C: 4.9 s: .: python3 .claude/hooks/check_guard_secrets.py
exit 0: C: 0.7 s: .: tools/check-dev-env.sh
exit 0: B: 45.7 s: .: tools/check-merge-gate.sh
lane pre wall time: 0.2 s
lane pre command seconds: 0.2
lane A wall time: 66.1 s
lane A command seconds: 66.1
lane B wall time: 53.2 s
lane B command seconds: 53.3
lane C wall time: 50.5 s
lane C command seconds: 50.3
lane e2e wall time: 129.6 s
lane e2e command seconds: 129.6
phase pre wall time with the wait for its processes: 0.2 s
phase A B C wall time with the wait for its processes: 66.1 s
phase e2e wall time with the wait for its processes: 129.6 s
summed command seconds: 299.5
gate wall time: 196.9 s
cores: 12
command logs: /Users/klukashka/Library/Caches/ovc-e2e/logs.WfV2Aq
result: passed
next: in the checkout of main run git merge --ff-only chore/verification-fixture-isolation, then merge-gate.sh finish

```

## Finish

```text
merge gate finish
mode: ff-only
change: chore/verification-fixture-isolation
base main: e95d27281587699df8286dac335927dd13488631
main after the merge: d23c7fd317e0323a0fa3646de928a7eaecf7f894
tested tree: a4c365984be6cc2ecf0ca0bac691ec24492c0946
main tree: a4c365984be6cc2ecf0ca0bac691ec24492c0946
scope: full
check list: full
hash check: passed
check list check: passed
result: passed; the trees are equal and the lock is released

```
