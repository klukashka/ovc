#!/usr/bin/env python3
from __future__ import annotations

import fnmatch
import hashlib
import importlib.util
import json
import os
import re
import subprocess
import sys
import tempfile

hook = os.path.join(os.path.dirname(os.path.abspath(__file__)), "guard_secrets.py")
refused = [
    "docker inspect ovc-postgres-1",
    "docker container inspect ovc-postgres-1",
    "docker compose config",
    "DOCKER_HOST=x docker compose config",
    "docker compose config | grep host_ip",
    "docker exec ovc-postgres-1 env",
    "docker compose exec postgres cat /proc/1/environ",
    "docker compose exec postgres sh -c 'echo $POSTGRES_PASSWORD'",
    "docker compose exec postgres sh -c \"env | grep PG\"",
    "docker compose exec postgres sh -c 'env|grep PG'",
    "printenv OVC_TEST_DATABASE_ADMIN_URL",
    "env",
    "env | grep OVC",
    "env -u HOME",
    "/usr/bin/env",
    "export",
    "export -p",
    "declare -p",
    "declare -x",
    "typeset -x",
    "compgen -v",
    "set",
    "bash -c env",
    "sh -c printenv",
    "command env",
    "(env)",
    "{ env; }",
    "nohup env",
    "time env",
    "xargs env",
    "bash <<'EOF'\nenv\nEOF",
    "bash <<EOF\nprintenv\nEOF",
    "python3 -c 'import os; print(os.environ)'",
    "node -e 'console.log(process.env)'",
    "ps eww -p 1",
    "ps -E",
    "awk 'BEGIN{print ENVIRON[\"PGPASSWORD\"]}'",
    "cat <<< \"$OVC_DEV_POSTGRES_PASSWORD\"",
    "launchctl getenv PATH",
    "echo $OVC_DEV_POSTGRES_PASSWORD",
    "printf '%s' \"$OVC_TEST_DATABASE_ADMIN_URL\"",
    "bash -lc env",
    "bash -xc env",
    "sh -ec 'printenv'",
    "bash -euc 'printenv | sort'",
    "zsh -lc 'export -p'",
    "bash -lc 'echo $OVC_DEV_POSTGRES_PASSWORD'",
    "bash -c -- env",
    "docker exec ovc-postgres-1 sh -ec 'env | grep PG'",
    "docker compose exec postgres bash -lc 'echo $POSTGRES_PASSWORD'",
    "python3 - <<'EOF'\nimport os\nprint(os.environ)\nEOF",
    "node <<'EOF'\nconsole.log(process.env)\nEOF",
    "uv run python -c 'import os;print(os.environ)'",
    "cat <<EOF\n$OVC_DEV_POSTGRES_PASSWORD\nEOF",
    "echo \"$(env)\"",
    "echo `printenv`",
    "nice -n 5 env",
    "timeout -s KILL 5 env",
    "env -iC / printenv",
    "eval env",
    "ps -AE",
    "ps -wwE -p 1",
    "docker compose config --environment --services",
    "docker compose config --images --format json",
    "docker compose convert",
    "docker buildx bake --print",
    "launchctl export",
    "watch -n 1 env",
    "xargs -I {} env",
    "uv run --with x env",
    "sudo -u postgres env",
    "perl -e 'print $ENV{PGPASSWORD}'",
    "ruby -e 'puts ENV.fetch(\"PGPASSWORD\")'",
    "python3 -c 'from os import environ; print(environ)'",
    "grep x <<< \"$foo\"\nenv",
    "echo $((1<<2))\nenv",
    "ps ww e",
    "tools/dev-env.sh env",
    "./tools/dev-env.sh printenv",
    "tools/dev-env.sh sh -c printenv",
    "tools/dev-env.sh bash -c 'echo $OVC_DATABASE_URL'",
    "tools/dev-env.sh export -p",
    "tools/dev-env.sh python3 -c 'import os;print(os.environ)'",
    "tools/dev-env.sh docker compose config",
    "bash tools/dev-env.sh docker compose config",
    "sh tools/dev-env.sh printenv",
    "bash tools/dev-env.sh python3 -c 'import os;print(os.environ)'",
    "bash tools/dev-env.sh node -e 'console.log(process.env)'",
    "bash -x tools/dev-env.sh true",
    "bash -xv tools/dev-env.sh go version",
    "bash -o xtrace tools/dev-env.sh true",
    "SHELLOPTS=xtrace tools/dev-env.sh true",
    "env SHELLOPTS=xtrace tools/dev-env.sh true",
    "BASH_ENV=/tmp/x tools/dev-env.sh true",
    "tools/dev-env.sh sh -x -c 'go version'",
    "tools/dev-env.sh bash -x scripts/run.sh",
    "tools/dev-env.sh bash -o pipefail -x run.sh",
    "tools/dev-env.sh bash +e -x run.sh",
    "tools/dev-env.sh bash -c 'set -x; true'",
    "tools/dev-env.sh sh -c 'set -o xtrace; go version'",
    "bash -O extglob tools/dev-env.sh docker compose config",
    "bash --norc tools/dev-env.sh docker compose config",
    "bash -- tools/dev-env.sh docker compose config",
    ". tools/dev-env.sh docker compose config",
    "source tools/dev-env.sh docker compose config",
    "exec -a x tools/dev-env.sh docker compose config",
    "tools/dev-env.sh bash -c 'set -e -x; true'",
    "tools/dev-env.sh bash -c 'set -o errexit -o xtrace; true'",
    "tools/dev-env.sh bash -c 'set -eo xtrace; true'",
    "tools/dev-env.sh bash -c 'shopt -os xtrace; true'",
    "tools/dev-env.sh zsh -c 'setopt xtrace; true'",
    "bash --rcfile x tools/dev-env.sh docker compose config",
    "bash --init-file x tools/dev-env.sh docker compose config",
    "tools/dev-env.sh pnpm exec printenv",
    "tools/dev-env.sh pnpm --dir frontend exec env",
    "tools/dev-env.sh pnpm -C frontend exec -- env",
    "pnpm dlx printenv",
    "tools/dev-env.sh npx printenv",
    "npx -y env",
    "pnpx --package x env",
    "tools/dev-env.sh pnpm exec -c 'printenv | sort'",
    "pnpm exec --shell-mode 'env | sort'",
    "pnpm dlx -c 'printenv | sort'",
    "pnpm --reporter silent exec printenv",
    "pnpm --loglevel error exec printenv",
    "pnpm --package x dlx env",
    "pnpm printenv",
    "tools/dev-env.sh pnpm run env",
    "npx --registry https://registry.example printenv",
    "npx -w frontend printenv",
    "npx --prefix frontend printenv",
    "npm exec printenv",
    "npx -c 'env | sort'",
    "npx --call=env",
    "ZDOTDIR=/tmp/z tools/dev-env.sh zsh -c true",
    "ENV=/tmp/x tools/dev-env.sh sh -i -c true",
    "tools/dev-env.sh nice env SHELLOPTS=xtrace bash -c true",
    "tools/dev-env.sh timeout 5 env BASH_ENV=/tmp/x bash -c true",
    "tools/dev-env.sh pnpm exec env SHELLOPTS=xtrace bash -c true",
    "HOME=/tmp/h tools/dev-env.sh zsh -c true",
    "tools/dev-env.sh env HOME=/tmp/h bash -i -c true",
]
allowed = [
    "env -i PATH=/usr/bin ./api",
    "env OVC_HTTP_ADDR=127.0.0.1:1 go run ./cmd/api",
    "docker compose ps --format '{{.Service}} {{.Ports}}'",
    "docker compose exec -T -e PGPASSWORD postgres createdb x",
    "docker compose up -d",
    "docker compose down",
    "docker compose config --services",
    "docker image inspect postgres:18",
    "set -euo pipefail",
    "export PATH=/usr/bin:$PATH",
    "echo hello",
    "git status",
    'git commit -m "Fix checks\n\nenv and printenv are refused"',
    "git commit -q -F - <<'EOF'\nSubject\n\nexport and env are refused\nEOF",
    "cat > notes.txt <<EOF\nenv\nEOF",
    "ps aux",
    "ps -ef",
    "ps -p 1234 -o etime=",
    "ps -o pid,user,state",
    "ps -eo pid,comm",
    "git commit -am \"Refuse cat /proc/1/environ\"",
    "python3 - <<'EOF'\nprint('environment notes')\nEOF",
    "docker compose config --quiet",
    "docker compose exec -T postgres pg_isready",
    "git commit -m 'Refuse `env` and `printenv` in the guard'",
    "grep -rn process.env frontend/e2e",
    "rg -n 'import.meta.env' frontend/src",
    "docker compose -f compose.yaml config --services",
    "docker volume inspect ovc_pgdata",
    "docker network inspect ovc_default",
    "command -v env",
    "tools/dev-env.sh pnpm --dir frontend dev",
    "tools/dev-env.sh bash -o pipefail scripts/run.sh",
    "tools/dev-env.sh bash -c 'set -e; go version'",
    "bash -n tools/dev-env.sh",
    "tools/dev-env.sh bash -c 'echo use set -x to debug'",
    "tools/dev-env.sh bash -c 'set -euo pipefail; go version'",
    "pnpm exec prettier --write src",
    "pnpm exec prettier --write src e2e playwright.config.ts",
]
local_checks = [
    "pnpm --dir frontend test",
    "go test ./...",
    "timeout 60 go test ./...",
    "env -C backend go test ./...",
    "tools/dev-env.sh go -C backend test ./...",
    "tools/dev-env.sh go -C backend test -v ./internal/core/",
    "bash tools/dev-env.sh go -C backend test ./...",
    "tools/dev-env.sh bash tools/check-dev-env.sh",
    "pnpm -s exec vitest run src/lib/api",
    "tools/dev-env.sh pnpm exec playwright test e2e/smoke.spec.ts",
    "npx playwright test",
    "npx vitest run",
    "pnpm dlx prettier@3 --check .",
    "pnpm --dir frontend exec playwright test --project chromium",
    "pnpm --dir frontend exec tsc -b",
    "pnpm -s audit --audit-level high",
    "pnpm --filter frontend test",
    "tools/dev-env.sh pnpm e2e",
    "npm exec -- prettier --check .",
    "go vet ./...",
    "go -C backend vet ./...",
    "cd backend && go test -race -count=1 ./internal/api",
    "bash -c 'go test ./...'",
    "../bin/golangci-lint run ./...",
    "../bin/govulncheck ./...",
    "pnpm lint",
    "pnpm typecheck",
    "pnpm --dir frontend e2e --max-failures=10",
    "pnpm -C frontend check:classes",
    "pnpm run format:check",
    "pnpm --dir frontend api:check",
    "pnpm exec prettier --check .",
    "pnpm exec eslint .",
    "playwright test --project chromium",
    "unformatted=x gofmt -l .",
    "../bin/sqlc -f sqlc.yaml diff",
    "tools/docs/check-docs.sh",
    "bash tools/check-merge-gate.sh",
    "tools/check-workflows.sh",
    "tools/ci-settings-test.sh",
    "python3 .claude/hooks/check_guard_secrets.py",
    ".claude/hooks/check_guard_secrets.py",
    "bin/gitleaks git --redact --no-banner --config .gitleaks.toml .",
    "bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml",
    "tools/merge-gate.sh gate --ff-only --change feat/x",
    "bash /private/tmp/scratch/merge-gate.sh gate --squash --change feat/x",
    "./merge-gate.sh --repo . gate --ff-only",
    "tools/merge-gate.sh run-job e2e --shard 1/12 --repo .",
    "echo \"$(go test ./...)\"",
]
file_writers = [
    "gofmt -w internal",
    "gofmt -l -w .",
    "pnpm exec prettier --write src",
    "pnpm --dir frontend format",
    "../bin/sqlc generate",
    "../bin/sqlc -f sqlc.yaml generate",
    "pnpm api:generate",
    "pnpm --dir frontend api:generate",
    "go -C backend get golang.org/x/text@v0.20.0",
    "go mod tidy",
    "go -C backend mod tidy",
    "pnpm install",
    "pnpm --dir frontend install --frozen-lockfile",
    "pnpm add -D vitest",
    "docker compose up -d",
    "tools/dev-env.sh pnpm --dir frontend dev",
    "pnpm build",
    "pnpm --dir frontend preview",
    "tools/merge-gate.sh finish",
    "tools/merge-gate.sh list --scope full",
    "tools/merge-gate.sh claim --ff-only --change feat/x --change-id feat/x",
    "python3 -m py_compile .claude/hooks/check_guard_secrets.py",
    "cat tools/check-workflows.sh",
    "grep -n 'go test' AGENTS.md",
    "git commit -m 'Ask before go test and pnpm lint'",
    "echo pnpm test",
]
lock_scratch = tempfile.TemporaryDirectory()
empty_lock_dir = lock_scratch.name
failed = 0
for command, expected in [(command, 2) for command in refused] + [(command, 0) for command in allowed]:
    payload = json.dumps({"tool_name": "Bash", "tool_input": {"command": command}})
    result = subprocess.run([sys.executable, hook, "--lock-dir", empty_lock_dir], input=payload, text=True, capture_output=True)
    if result.returncode != expected or result.stdout.strip():
        failed += 1
        print("FAIL", result.returncode, repr(command), result.stdout.strip())
malformed = subprocess.run([sys.executable, hook, "--lock-dir", empty_lock_dir], input=json.dumps({"tool_name": "Bash", "tool_input": {"command": 5}}), text=True, capture_output=True).returncode
if malformed != 2:
    failed += 1
    print("FAIL non-string command allowed")
exception_paths = [
    (["--lock-dir", "relative-lock-folder"], {"tool_name": "Bash", "tool_input": {"command": "ls"}}, "Refused: "),
    (["--lock-dir", "relative-lock-folder"], {"tool_name": "Bash", "tool_input": {"command": "git push"}}, "Refused: "),
    (["unexpected"], {"tool_name": "Read", "tool_input": {"file_path": "README.md"}}, "Refused: "),
    (["--lock-dir", empty_lock_dir], {"tool_name": "Bash", "tool_input": "git push"}, "Blocked: "),
    (["--lock-dir", empty_lock_dir], {"tool_name": 5, "tool_input": {"command": "git push"}}, "Blocked: "),
]
for arguments, exception_payload, expected_prefix in exception_paths:
    result = subprocess.run([sys.executable, hook, *arguments], input=json.dumps(exception_payload), text=True, capture_output=True)
    if result.returncode != 2 or result.stdout.strip() or not result.stderr.startswith(expected_prefix):
        failed += 1
        print("FAIL an exception path did not refuse", arguments, exception_payload, result.returncode, result.stdout.strip())

repository = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
scanner = os.path.join(repository, "bin", "gitleaks")
git_environment = {name: value for name, value in os.environ.items() if not name.startswith(("GIT_", "GH_"))}
synthetic_rule = "[[rules]]\nid = \"synthetic-marker\"\ndescription = \"Synthetic marker for the guard self-test\"\nregex = '''synthetic-leak-marker-[0-9]+'''\n"
blind_rule = "[[rules]]\nid = \"blind-marker\"\ndescription = \"A rule that matches nothing in the guard self-test\"\nregex = '''never-present-marker-[0-9]+'''\n"
run_id = "1" * 40
other_sha = "a" * 40


def git(root: str, *arguments: str) -> str:
    identity = ["-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid", "-c", "commit.gpgsign=false"]
    result = subprocess.run(["git", *identity, *arguments], cwd=root, env=git_environment, capture_output=True, text=True, check=True)
    return result.stdout.strip()


def commit_file(root: str, name: str, content: str) -> None:
    path = os.path.join(root, name)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(content)
    git(root, "add", name)
    git(root, "commit", "-q", "-m", f"Change {name}")


def synthetic_repository(root: str, with_scanner: bool) -> None:
    os.makedirs(root)
    git(root, "init", "-q", "-b", "main")
    commit_file(root, ".github/workflows/gate.yml", "name: gate\n")
    commit_file(root, ".gitleaks.toml", synthetic_rule)
    git(root, "update-ref", "refs/remotes/origin/main", "refs/heads/main")
    if with_scanner:
        os.makedirs(os.path.join(root, "bin"))
        os.symlink(scanner, os.path.join(root, "bin", "gitleaks"))


def branch_from_main(root: str, branch: str, changes: list[tuple[str, str]]) -> None:
    git(root, "switch", "-q", "-c", branch, "main")
    for name, content in changes:
        commit_file(root, name, content)
    git(root, "switch", "-q", "main")


def decision(command: str, cwd: str, lock_dir: str = empty_lock_dir) -> tuple[str, str]:
    payload = json.dumps({"tool_name": "Bash", "tool_input": {"command": command}, "cwd": cwd})
    result = subprocess.run([sys.executable, hook, "--lock-dir", lock_dir], input=payload, text=True, capture_output=True)
    if result.stdout.strip():
        return "output on stdout", result.stdout
    if result.returncode == 2:
        return ("refuse" if result.stderr.startswith("Refused: ") else "deny"), result.stderr
    if result.returncode != 0:
        return f"exit {result.returncode}", result.stderr
    return "allow", ""


def file_decision(tool: str, key: str, path: str) -> str:
    payload = json.dumps({"tool_name": tool, "tool_input": {key: path}})
    result = subprocess.run([sys.executable, hook, "--lock-dir", empty_lock_dir], input=payload, text=True, capture_output=True)
    return "deny" if result.returncode == 2 else "allow"


lock_folders = os.path.join(empty_lock_dir, "folders")


def lock_folder(lock: dict[str, str] | None, log: list[str] | None = None) -> str:
    folder = os.path.join(lock_folders, str(len(os.listdir(lock_folders)) if os.path.isdir(lock_folders) else 0))
    os.makedirs(folder, mode=0o700)
    if lock is not None:
        with open(os.path.join(folder, "merge-gate.lock"), "w", encoding="utf-8") as handle:
            handle.write("".join(f"{key}={value}\n" for key, value in lock.items()))
    if log is not None:
        with open(os.path.join(folder, "merge-gate.log"), "w", encoding="utf-8") as handle:
            handle.write("".join(f"{line}\n" for line in log))
    return folder


def claim_lock(root: str, state: str, **fields: str) -> dict[str, str]:
    common_dir = git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
    lock = {"pid": "4242", "started_at": "2026-11-27T00:00:00Z", "checkout": root, "common_dir": common_dir, "state": state}
    lock.update(fields)
    return lock


def dispatched_lock(root: str, head: str, change_id: str, **fields: str) -> str:
    tree = subprocess.run(["git", "rev-parse", "--verify", "--quiet", f"{head}^{{tree}}"], cwd=root, env=git_environment, capture_output=True, text=True).stdout.strip()
    base = git(root, "rev-parse", "refs/heads/main")
    lock = claim_lock(root, "dispatched", kind="gate", change_id=change_id, head=head, tree=tree, base=base)
    lock.update(fields)
    return lock_folder(lock)


def finish_line(head: str, tree: str, record: str, result: str = "passed", extra: str = "") -> str:
    return f"finish: record={record} head={head} tree={tree} result={result}{extra}"


def commit_with(root: str, name: str, content: str, message: str, email: str = "synthetic@example.invalid") -> None:
    path = os.path.join(root, name)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(content)
    git(root, "add", name)
    git(root, "-c", f"user.email={email}", "commit", "-q", "-m", message)


def branch_with_commit(root: str, branch: str, message: str, email: str = "synthetic@example.invalid") -> None:
    git(root, "switch", "-q", "-c", branch, "main")
    commit_with(root, f"notes/{branch.replace('/', '-')}.txt", f"{branch}\n", message, email)
    git(root, "switch", "-q", "main")


github_cases = 1
with tempfile.TemporaryDirectory() as scratch:
    ready = os.path.join(scratch, "ready")
    unscanned = os.path.join(scratch, "unscanned")
    workflow_main = os.path.join(scratch, "workflow-main")
    diverged = os.path.join(scratch, "diverged")
    linked = os.path.join(scratch, "linked")
    blind_working_tree = os.path.join(scratch, "blind-working-tree")
    ignoring_working_tree = os.path.join(scratch, "ignoring-working-tree")
    if not os.access(scanner, os.X_OK):
        failed += 1
        print("FAIL bin/gitleaks is missing; build bin/ as docs/development.md describes before running this check")
    synthetic_repository(ready, with_scanner=True)
    commit_file(ready, "notes/main.txt", "main moves ahead of origin/main\n")
    branch_from_main(ready, "feat/clean-change", [("notes/clean.txt", "clean change\n")])
    branch_from_main(ready, "feat/leaky-change", [("notes/leak.txt", "synthetic-leak-marker-12345\n")])
    branch_from_main(ready, "feat/two-commits", [("notes/one.txt", "one\n"), ("notes/two.txt", "two\n")])
    branch_from_main(ready, "feat/workflow-change", [(".github/workflows/gate.yml", "name: changed\n")])
    branch_from_main(ready, "wip/other-name", [("notes/wip.txt", "wip\n")])
    branch_from_main(ready, "feat/gate-change", [("notes/gate.txt", "gate change\n")])
    branch_from_main(ready, "feat/gate-change-r2", [("notes/gate.txt", "gate change rebuilt\n")])
    branch_from_main(ready, "fix/flaky-before", [("notes/flaky.txt", "flaky\n")])
    branch_from_main(ready, "fix/flaky-after", [("notes/flaky.txt", "flaky fixed\n")])
    branch_from_main(ready, "fix/flaky-r1-after", [("notes/flaky.txt", "flaky fixed again\n")])
    branch_from_main(ready, "fix/flaky-rx-after", [("notes/flaky.txt", "flaky fixed a third way\n")])
    branch_with_commit(ready, "feat/personal-message", "Add a note\n\n- reach the tutor at someone@neutral-mail.ru")
    branch_with_commit(ready, "feat/personal-author", "Add a note by a real address", "someone@neutral-mail.ru")
    branch_with_commit(ready, "feat/noreply-author", "Add a note by a noreply address", "1+someone@users.noreply.github.com")
    branch_from_main(ready, "feat/personal-email", [("notes/contact.txt", "write to someone@neutral-mail.ru\n")])
    branch_from_main(ready, "feat/personal-phone", [("notes/contact.txt", "call +7 (912) 345-67-89\n")])
    branch_from_main(ready, "feat/personal-plain-phone", [("notes/contact.txt", "call 89123456789\n")])
    branch_from_main(
        ready,
        "feat/reserved-addresses",
        [(
            "notes/contact.txt",
            "tutor@example.com student@mail.example.org a@school.test b@x.invalid c@app.localhost d@registry.example "
            "1+someone@users.noreply.github.com run 1234567890123 sha a89123456789b\n",
        )],
    )
    git(ready, "config", "alias.publish", "push")
    git(ready, "worktree", "add", "-q", "--detach", linked, "main")
    synthetic_repository(blind_working_tree, with_scanner=True)
    branch_from_main(blind_working_tree, "feat/leaky-change", [("notes/leak.txt", "synthetic-leak-marker-67890\n")])
    with open(os.path.join(blind_working_tree, ".gitleaks.toml"), "w", encoding="utf-8") as handle:
        handle.write(blind_rule)
    synthetic_repository(ignoring_working_tree, with_scanner=True)
    branch_from_main(ignoring_working_tree, "feat/leaky-change", [("notes/leak.txt", "synthetic-leak-marker-24680\n")])
    leaky_commit = git(ignoring_working_tree, "rev-parse", "refs/heads/feat/leaky-change")
    with open(os.path.join(ignoring_working_tree, ".gitleaksignore"), "w", encoding="utf-8") as handle:
        handle.write(f"{leaky_commit}:notes/leak.txt:synthetic-marker:1\n")
    synthetic_repository(unscanned, with_scanner=False)
    branch_from_main(unscanned, "feat/clean-change", [("notes/clean.txt", "clean change\n")])
    synthetic_repository(workflow_main, with_scanner=True)
    commit_file(workflow_main, ".github/workflows/gate.yml", "name: changed on main\n")
    synthetic_repository(diverged, with_scanner=True)
    branch_from_main(diverged, "side", [("notes/side.txt", "side\n")])
    git(diverged, "update-ref", "refs/remotes/origin/main", "refs/heads/side")
    commit_file(diverged, "notes/main.txt", "main\n")
    two_ahead = os.path.join(scratch, "two-ahead")
    synthetic_repository(two_ahead, with_scanner=True)
    commit_file(two_ahead, "notes/one.txt", "one\n")
    commit_file(two_ahead, "notes/two.txt", "two\n")
    catch_up = os.path.join(scratch, "catch-up")
    synthetic_repository(catch_up, with_scanner=True)
    commit_file(catch_up, "notes/fallback.txt", "a fallback merge\n")
    catch_up_personal = os.path.join(scratch, "catch-up-personal")
    synthetic_repository(catch_up_personal, with_scanner=True)
    commit_file(catch_up_personal, "notes/fallback.txt", "call +7 912 345 67 89\n")
    catch_up_far = os.path.join(scratch, "catch-up-far")
    synthetic_repository(catch_up_far, with_scanner=True)
    commit_file(catch_up_far, "notes/one.txt", "one\n")
    commit_file(catch_up_far, "notes/two.txt", "two\n")
    catch_up_locks: dict[str, str] = {}
    catch_up_heads: dict[str, str] = {}
    for root in (catch_up, catch_up_personal, catch_up_far):
        head = git(root, "rev-parse", "refs/heads/main")
        git(root, "branch", f"catch-up/{head[:12]}", head)
        catch_up_heads[root] = head
        catch_up_locks[root] = dispatched_lock(root, head, "", kind="catch-up", base=git(root, "rev-parse", "refs/remotes/origin/main"))
    git(catch_up, "branch", "catch-up/000000000000", catch_up_heads[catch_up])

    branch_push = "git push origin refs/heads/{0}:refs/heads/{0}"
    main_push = "git push origin refs/heads/main:refs/heads/main"
    sha = git(ready, "rev-parse", "refs/heads/feat/clean-change")
    main_sha = git(ready, "rev-parse", "refs/heads/main")
    two_commits_sha = git(ready, "rev-parse", "refs/heads/feat/two-commits")
    other_name_sha = git(ready, "rev-parse", "refs/heads/wip/other-name")
    cases = [
        (ready, branch_push.format("feat/clean-change"), "allow", ""),
        (ready, branch_push.format("feat/leaky-change"), "refuse", "did not pass"),
        (unscanned, branch_push.format("feat/clean-change"), "refuse", "missing"),
        (ready, branch_push.format("feat/two-commits"), "refuse", "one commit"),
        (ready, branch_push.format("feat/workflow-change"), "refuse", ".github"),
        (ready, branch_push.format("wip/other-name"), "refuse", "names"),
        (ready, main_push, "refuse", "finish line"),
        (linked, branch_push.format("feat/clean-change"), "allow", ""),
        (linked, branch_push.format("feat/leaky-change"), "refuse", "did not pass"),
        (blind_working_tree, branch_push.format("feat/leaky-change"), "refuse", "did not pass"),
        (ignoring_working_tree, branch_push.format("feat/leaky-change"), "refuse", "did not pass"),
        (workflow_main, main_push, "refuse", ".github"),
        (diverged, main_push, "refuse", "ancestor"),
        (ready, "git push", "refuse", ""),
        (ready, "git push origin feat/clean-change", "refuse", ""),
        (ready, "git push origin +refs/heads/feat/clean-change:refs/heads/feat/clean-change", "refuse", ""),
        (ready, "git push origin :refs/heads/feat/clean-change", "refuse", ""),
        (ready, "git push --delete origin feat/clean-change", "refuse", ""),
        (ready, "git push --mirror origin", "refuse", ""),
        (ready, "git push --all origin", "refuse", ""),
        (ready, "git push --tags origin", "refuse", ""),
        (ready, "git push --receive-pack=x origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "git push --exec=x origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "git push -u origin refs/heads/feat/clean-change:refs/heads/feat/clean-change", "refuse", ""),
        (ready, "git push upstream refs/heads/feat/clean-change:refs/heads/feat/clean-change", "refuse", ""),
        (ready, "git send-pack origin main", "refuse", ""),
        (ready, "git -c alias.p=push p origin main", "refuse", ""),
        (ready, "git -C . push origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "git publish origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "env git push origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "command git push origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "/usr/bin/git push origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "bash -c 'git push origin refs/heads/main:refs/heads/main'", "refuse", ""),
        (ready, "eval 'git push origin refs/heads/main:refs/heads/main'", "refuse", ""),
        (ready, "echo origin | xargs git push", "refuse", ""),
        (ready, "git push origin $(git branch --show-current)", "refuse", ""),
        (ready, "GIT_SSH_COMMAND=x git push origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "GH_TOKEN=x gh run list", "refuse", ""),
        (ready, "cd . && git push origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "git status; git push origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "git push origin refs/heads/main:refs/heads/main | cat", "refuse", ""),
        (ready, "python3 -c 'import subprocess; subprocess.run([\"git\", \"push\"])'", "refuse", ""),
        (ready, "git remote", "allow", ""),
        (ready, "git remote -v", "allow", ""),
        (ready, "git remote get-url origin", "allow", ""),
        (ready, "git remote add other https://example.invalid/other.git", "refuse", ""),
        (ready, "git remote set-url origin https://example.invalid/other.git", "refuse", ""),
        (ready, "git config --get user.name", "allow", ""),
        (ready, "git config user.name", "allow", ""),
        (ready, "git config --list --show-origin", "allow", ""),
        (ready, "git config user.name Other", "refuse", ""),
        (ready, "git config --global alias.p push", "refuse", ""),
        (ready, "git config --unset user.name", "refuse", ""),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=full", "refuse", "claim"),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=docs", "refuse", "claim"),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=other", "refuse", ""),
        (ready, "gh workflow run gate.yml --ref main -f sha=abc -f scope=full", "refuse", ""),
        (ready, f"gh workflow run gate.yml --ref main --ref feat/x -f sha={sha} -f scope=full", "refuse", ""),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=full -R other/repo", "refuse", ""),
        (ready, f"gh workflow run other.yml --ref main -f sha={sha} -f scope=full", "refuse", ""),
        (ready, f"gh workflow run gate.yml --ref main -f sha={main_sha} -f scope=full", "refuse", "claim"),
        (ready, f"gh workflow run gate.yml --ref main -f sha={two_commits_sha} -f scope=full", "refuse", "claim"),
        (ready, f"gh workflow run gate.yml --ref main -f sha={other_name_sha} -f scope=full", "refuse", "claim"),
        (ready, f"gh workflow run gate.yml --ref main -f sha={other_sha} -f scope=full", "refuse", "claim"),
        (ready, "gh workflow enable gate.yml", "refuse", ""),
        (ready, "gh workflow disable gate.yml", "refuse", ""),
        (ready, "gh run list --workflow gate.yml --limit 5", "allow", ""),
        (ready, "gh run view 123 --log-failed", "allow", ""),
        (ready, "gh run watch 123 --exit-status", "allow", ""),
        (ready, "gh run list --repo other/repo", "refuse", ""),
        (ready, "gh run rerun 123", "refuse", ""),
        (ready, "gh run cancel 123", "refuse", ""),
        (ready, "gh run delete 123", "refuse", ""),
        (ready, "gh pr view feat/clean-change --json number,state,headRefName,url", "allow", ""),
        (ready, "gh pr list --state open --json number,state,headRefOid", "allow", ""),
        (ready, "gh pr view 3 --json body", "refuse", ""),
        (ready, "gh pr view 3 --json number,comments", "refuse", ""),
        (ready, "gh pr view 3", "refuse", ""),
        (ready, "gh pr view 3 --comments --json number", "refuse", ""),
        (ready, "gh pr create --fill", "refuse", ""),
        (ready, "gh pr create --body 'Refuse `printenv` dumps'", "refuse", "substitution"),
        (ready, "gh pr edit 3 --title x", "refuse", ""),
        (ready, "gh pr merge 3", "refuse", ""),
        (ready, "gh pr close 3", "refuse", ""),
        (ready, "gh pr comment 3 --body x", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs", "allow", ""),
        (ready, f"gh api repos/klukashka/ovc/actions/runs/{run_id[:6]}/jobs --jq .jobs", "allow", ""),
        (ready, "gh api repos/klukashka/ovc/actions/jobs/123 -X GET", "allow", ""),
        (ready, f"gh api repos/klukashka/ovc/commits/{other_sha}/status -H 'Accept: application/vnd.github+json'", "allow", ""),
        (ready, f"gh api repos/klukashka/ovc/commits/{other_sha}/statuses --method GET", "allow", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs -X POST", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --method=DELETE", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --input payload.json", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs -f name=x", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs -F name=x", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --field name=x", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --raw-field name=x", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs -H 'Authorization: x'", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --hostname example.invalid", "refuse", ""),
        (ready, "gh api https://api.github.com/repos/klukashka/ovc/actions/runs", "refuse", ""),
        (ready, "gh api graphql", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/pulls", "refuse", ""),
        (ready, "gh api repos/other/repo/actions/runs", "refuse", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs/../../pulls", "refuse", "allowed repos"),
        (ready, "gh api repos/klukashka/ovc/actions/runs/..", "refuse", "allowed repos"),
        (ready, "gh api repos/klukashka/ovc/actions/jobs/./123", "refuse", "allowed repos"),
        (ready, "gh api 'repos/klukashka/ovc/actions/runs/..?per_page=1'", "refuse", "allowed repos"),
        (ready, "gh api repos/klukashka/ovc/actions/runs/run..1", "allow", ""),
        (ready, "gh alias set p 'run list'", "refuse", ""),
        (ready, "gh extension install x/y", "refuse", ""),
        (ready, "gh gist create notes.txt", "refuse", ""),
        (ready, "gh repo view", "refuse", ""),
        (ready, "gh secret list", "refuse", ""),
        (ready, "gh variable list", "refuse", ""),
        (ready, "gh label list", "refuse", ""),
        (ready, "gh release list", "refuse", ""),
        (ready, "gh ssh-key list", "refuse", ""),
        (ready, "gh gpg-key list", "refuse", ""),
        (ready, "gh cache list", "refuse", ""),
        (ready, "gh auth status", "refuse", ""),
        (ready, "gh auth token", "deny", ""),
        (ready, "gh auth status --show-token", "deny", ""),
        (ready, "gh auth status --show-token=true", "deny", ""),
        (ready, "gh auth status -t", "deny", ""),
        (ready, "gh auth status -at", "deny", ""),
        (ready, "gh auth status --hostname github.com -t", "deny", ""),
        (ready, "gh auth status --active", "refuse", ""),
        (ready, "gh config get oauth_token", "deny", ""),
        (ready, "gh config get -h github.com oauth_token", "deny", ""),
        (ready, "gh config get --host=github.com oauth_token", "deny", ""),
        (ready, "gh config get git_protocol", "refuse", ""),
        (ready, "gh auth git-credential get", "deny", ""),
        (ready, "gh auth login", "deny", ""),
        (ready, "gh auth refresh -s workflow", "deny", ""),
        (ready, "gh auth switch", "deny", ""),
        (ready, "bash -c 'gh auth token'", "deny", ""),
        (ready, "echo \"$(gh auth token)\"", "deny", ""),
        (ready, "git credential fill", "deny", ""),
        (ready, "git credential approve", "deny", ""),
        (ready, "git credential reject", "deny", ""),
        (ready, "git credential-osxkeychain get", "deny", ""),
        (ready, "security find-internet-password -s github.com", "deny", ""),
        (ready, "security find-generic-password -s gh:github.com", "deny", ""),
        (ready, "cat ~/.config/gh/hosts.yml", "deny", ""),
        (ready, "grep -r oauth ~/.config/gh", "deny", ""),
        (ready, "git status", "allow", ""),
        (ready, "git log --oneline --grep push", "allow", ""),
        (ready, "git commit -m 'Explain why gh auth token and git push are guarded'", "allow", ""),
        (ready, "grep -n \"git push\" AGENTS.md", "allow", ""),
        (ready, "rg 'gh workflow run gate.yml' docs", "allow", ""),
        (ready, "echo 'git push origin refs/heads/main:refs/heads/main'", "allow", ""),
        (ready, "grep -rn 'git push' docs | head -5", "allow", ""),
        (ready, "git log --grep 'gh run' --oneline", "allow", ""),
        (ready, "find . -name x -exec git push origin main \\;", "refuse", ""),
        (ready, "env -S 'git push origin refs/heads/main:refs/heads/main'", "deny", ""),
        (ready, "sh -c \"bash -c 'git push origin main'\"", "refuse", ""),
        (ready, "if git push origin main; then true; fi", "refuse", ""),
        (ready, "echo 'unbalanced; git push origin main", "refuse", ""),
        (ready, "G=git; $G push origin refs/heads/main:refs/heads/main", "refuse", ""),
        (ready, "bash <<'EOF'\ngit push origin main\nEOF", "refuse", ""),
        (ready, "bash <<'EOF'\necho git push\nEOF", "allow", ""),
    ]
    dispatch = "gh workflow run gate.yml --ref main -f sha={0} -f scope={1}"
    gate_sha = git(ready, "rev-parse", "refs/heads/feat/gate-change")
    rebuild_sha = git(ready, "rev-parse", "refs/heads/feat/gate-change-r2")
    before_sha = git(ready, "rev-parse", "refs/heads/fix/flaky-before")
    after_sha = git(ready, "rev-parse", "refs/heads/fix/flaky-after")
    main_tree = git(ready, "rev-parse", "refs/heads/main^{tree}")
    before_tree = git(ready, "rev-parse", "refs/heads/fix/flaky-before^{tree}")
    gate_lock = dispatched_lock(ready, gate_sha, "feat/gate-change")
    symlinked_lock = lock_folder(None)
    os.symlink(os.path.join(gate_lock, "merge-gate.lock"), os.path.join(symlinked_lock, "merge-gate.lock"))
    unreadable_lock = lock_folder(None)
    with open(os.path.join(unreadable_lock, "merge-gate.lock"), "w", encoding="utf-8") as handle:
        handle.write("state=dispatched\nnot a lock line\n")
    passed_lock = lock_folder(claim_lock(ready, "passed", head=gate_sha))
    r1_after_sha = git(ready, "rev-parse", "refs/heads/fix/flaky-r1-after")
    rx_after_sha = git(ready, "rev-parse", "refs/heads/fix/flaky-rx-after")
    playwright_title = "pilot-1 › step 2 shows the lesson (desktop)"
    go_selection = "kind:go|package:./internal/api|test:TestChatSearchTimeout|project:|title-sha256:|entry:"
    subtest_selection = "kind:go|package:./internal/api|test:TestChatSearch/Timeout|project:|title-sha256:|entry:"
    playwright_selection = f"kind:playwright|package:|test:|project:chromium|title-sha256:{hashlib.sha256(playwright_title.encode('utf-8')).hexdigest()}|entry:"
    check_selection = "kind:check|package:|test:|project:|title-sha256:|entry:frontend-checks:3"
    evidence_fields = {"change_id": "fix/flaky", "base": main_sha, "before_tree": before_tree, "failed_run": "https://github.com/klukashka/ovc/actions/runs/1", "selection": go_selection}

    def evidence_lock(pending: str, **fields: str) -> str:
        lock = claim_lock(ready, "evidence", pending=pending, **evidence_fields)
        lock.update(fields)
        return lock_folder(lock)

    go_evidence = "gh workflow run gate.yml --ref main -f sha={0} -f scope=evidence -f kind=go -f package=./internal/api -f test=TestChatSearchTimeout -f series={1}"
    playwright_evidence = "gh workflow run gate.yml --ref main -f sha={0} -f scope=evidence -f kind=playwright -f project=chromium -f 'title=" + playwright_title + "' -f series={1}"
    check_evidence = "gh workflow run gate.yml --ref main -f sha={0} -f scope=evidence -f kind=check -f entry=frontend-checks:3 -f series={1}"
    change = "gh workflow run gate.yml --ref main -f sha={0} -f scope=change -f jobs={1}"
    ci_log = lock_folder(None, [finish_line(main_sha, main_tree, "ci")])
    catch_up_log = lock_folder(None, [finish_line(main_sha, main_tree, "ci-catch-up")])
    gate_log = lock_folder(None, [finish_line(main_sha, main_tree, "gate")])
    later_gate_log = lock_folder(None, [finish_line(main_sha, main_tree, "ci"), finish_line(main_sha, main_tree, "gate")])
    stale_log = lock_folder(None, [finish_line(other_sha, main_tree, "ci")])
    failed_log = lock_folder(None, [finish_line(main_sha, main_tree, "ci", "failed")])
    extra_key_log = lock_folder(None, [finish_line(main_sha, main_tree, "ci", extra=" record=ci")])
    unprefixed_log = lock_folder(None, [finish_line(main_sha, main_tree, "ci")[len("finish: ") :]])
    attempt_only_log = lock_folder(None, [f"time=2026-11-27T00:00:00Z base={main_sha} tree={main_tree} checks=full scope=full result=passed"])
    two_ahead_log = lock_folder(None, [finish_line(git(two_ahead, "rev-parse", "refs/heads/main"), git(two_ahead, "rev-parse", "refs/heads/main^{tree}"), "ci")])
    catch_up_push = branch_push.format(f"catch-up/{catch_up_heads[catch_up][:12]}")
    locked_cases = [
        (ready, dispatch.format(gate_sha, "full"), "allow", "", gate_lock),
        (ready, dispatch.format(gate_sha, "docs"), "allow", "", gate_lock),
        (ready, dispatch.format(gate_sha, "full"), "allow", "", dispatched_lock(ready, gate_sha, "feat/gate-change", kind="after-fix")),
        (ready, dispatch.format(rebuild_sha, "full"), "allow", "", dispatched_lock(ready, rebuild_sha, "feat/gate-change")),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "claim", passed_lock),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "claim", dispatched_lock(ready, sha, "feat/clean-change")),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "claim", symlinked_lock),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "claim", unreadable_lock),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "tip of a local", dispatched_lock(ready, gate_sha, "feat/other-change")),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "unknown kind", dispatched_lock(ready, gate_sha, "feat/gate-change", kind="other")),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "tree", dispatched_lock(ready, gate_sha, "feat/gate-change", tree=main_tree)),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "only parent", dispatched_lock(ready, gate_sha, "feat/gate-change", base=other_sha)),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "another repository", dispatched_lock(ready, gate_sha, "feat/gate-change", common_dir="/nonexistent/.git")),
        (ready, dispatch.format(gate_sha, "full"), "refuse", "only parent", dispatched_lock(ready, gate_sha, "feat/gate-change", kind="catch-up")),
        (ready, dispatch.format(main_sha, "full"), "refuse", "only parent", dispatched_lock(ready, main_sha, "feat/gate-change")),
        (ready, dispatch.format(two_commits_sha, "full"), "refuse", "only parent", dispatched_lock(ready, two_commits_sha, "feat/two-commits")),
        (ready, dispatch.format(other_name_sha, "full"), "refuse", "tip of a local", dispatched_lock(ready, other_name_sha, "wip/other-name")),
        (ready, dispatch.format(before_sha, "full"), "refuse", "tip of a local", dispatched_lock(ready, before_sha, "fix/flaky-before")),
        (ready, dispatch.format(other_sha, "full"), "refuse", "tree", dispatched_lock(ready, other_sha, "feat/gate-change")),
        (ready, f"gh workflow run gate.yml --ref main -f sha={gate_sha} -f scope=full -f jobs=e2e", "refuse", "exact", gate_lock),
        (ready, f"gh workflow run gate.yml --ref main -f scope=full -f sha={gate_sha}", "allow", "", gate_lock),
        (ready, f"gh workflow run gate.yml --ref main -f sha={gate_sha} -f sha={gate_sha} -f scope=full", "refuse", "exact", gate_lock),
        (ready, change.format(sha, "backend-checks,e2e:3"), "allow", "", empty_lock_dir),
        (ready, change.format(sha, "backend-tests:2,frontend-checks,docs-checks,e2e"), "allow", "", passed_lock),
        (ready, change.format(rebuild_sha, "e2e:12"), "allow", "", evidence_lock(f"before-first,{before_sha}")),
        (ready, change.format(sha, "backend-checks"), "refuse", "claimed", gate_lock),
        (ready, change.format(sha, "backend-checks"), "refuse", "could not be read", unreadable_lock),
        (ready, change.format(sha, "e2e:13"), "refuse", "jobs", empty_lock_dir),
        (ready, change.format(sha, "backend-checks:1"), "refuse", "jobs", empty_lock_dir),
        (ready, change.format(sha, "e2e:03"), "refuse", "jobs", empty_lock_dir),
        (ready, change.format(sha, "unknown"), "refuse", "jobs", empty_lock_dir),
        (ready, change.format(sha, "e2e:3,e2e:3"), "refuse", "jobs", empty_lock_dir),
        (ready, change.format(sha, "e2e:3,"), "refuse", "jobs", empty_lock_dir),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=change -f jobs=", "refuse", "exact", empty_lock_dir),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=change", "refuse", "jobs", empty_lock_dir),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=change -f jobs=e2e -f title=x", "refuse", "jobs", empty_lock_dir),
        (ready, change.format(before_sha, "e2e:1"), "refuse", "named", empty_lock_dir),
        (ready, change.format(other_name_sha, "e2e:1"), "refuse", "named", empty_lock_dir),
        (ready, change.format(two_commits_sha, "e2e:1"), "refuse", "only parent", empty_lock_dir),
        (ready, change.format(main_sha, "e2e:1"), "refuse", "only parent", empty_lock_dir),
        (ready, go_evidence.format(before_sha, "before-first"), "allow", "", evidence_lock(f"before-first,{before_sha}")),
        (ready, playwright_evidence.format(before_sha, "before-rest"), "allow", "", evidence_lock(f"before-rest,{before_sha}", selection=playwright_selection)),
        (ready, check_evidence.format(after_sha, "after"), "allow", "", evidence_lock(f"after,{after_sha}", selection=check_selection)),
        (ready, check_evidence.format(r1_after_sha, "after"), "allow", "", evidence_lock(f"after,{r1_after_sha}", selection=check_selection)),
        (ready, check_evidence.format(rx_after_sha, "after"), "refuse", "local branch", evidence_lock(f"after,{rx_after_sha}", selection=check_selection)),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "selection inputs differ", evidence_lock(f"before-first,{before_sha}", selection=subtest_selection)),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "selection inputs differ", evidence_lock(f"before-first,{before_sha}", selection="")),
        (ready, playwright_evidence.format(before_sha, "before-rest").replace("step 2", "step 3"), "refuse", "selection inputs differ", evidence_lock(f"before-rest,{before_sha}", selection=playwright_selection)),
        (ready, check_evidence.format(after_sha, "after").replace("frontend-checks:3", "frontend-checks:4"), "refuse", "selection inputs differ", evidence_lock(f"after,{after_sha}", selection=check_selection)),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "pending", evidence_lock(f"before-rest,{before_sha}")),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "pending", evidence_lock("")),
        (ready, go_evidence.format(after_sha, "after"), "refuse", "pending", evidence_lock(f"before-first,{before_sha}")),
        (ready, go_evidence.format(after_sha, "before-first"), "refuse", "before tree", evidence_lock(f"before-first,{after_sha}")),
        (ready, go_evidence.format(before_sha, "after"), "refuse", "local branch", evidence_lock(f"after,{before_sha}")),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "series base", evidence_lock(f"before-first,{before_sha}", base=other_sha)),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "local branch", evidence_lock(f"before-first,{before_sha}", change_id="fix/other")),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "another repository", evidence_lock(f"before-first,{before_sha}", common_dir="/nonexistent/.git")),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "open evidence series", dispatched_lock(ready, before_sha, "fix/flaky", pending=f"before-first,{before_sha}")),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "open evidence series", empty_lock_dir),
        (ready, go_evidence.format(before_sha, "before-first"), "refuse", "open evidence series", passed_lock),
        (ready, go_evidence.format(before_sha, "before-first").replace("test=TestChatSearchTimeout", "test=ChatSearch"), "refuse", "allowed characters", evidence_lock(f"before-first,{before_sha}")),
        (ready, go_evidence.format(before_sha, "before-first").replace("package=./internal/api", "package=-race"), "refuse", "allowed characters", evidence_lock(f"before-first,{before_sha}")),
        (ready, playwright_evidence.format(before_sha, "before-rest").replace("pilot-1", "a" * 200), "refuse", "allowed characters", evidence_lock(f"before-rest,{before_sha}")),
        (ready, playwright_evidence.format(before_sha, "before-rest").replace("project=chromium", "project=-x"), "refuse", "allowed characters", evidence_lock(f"before-rest,{before_sha}")),
        (ready, playwright_evidence.format(before_sha, "before-rest").replace("title=pilot-1", "title=pilot-1 ; rm"), "refuse", "allowed characters", evidence_lock(f"before-rest,{before_sha}")),
        (ready, go_evidence.format(before_sha, "before-first") + " -f title=x", "refuse", "selection", evidence_lock(f"before-first,{before_sha}")),
        (ready, check_evidence.format(after_sha, "after").replace("frontend-checks:3", "e2e:3"), "refuse", "allowed characters", evidence_lock(f"after,{after_sha}")),
        (ready, go_evidence.format(before_sha, "before-first").replace("TestChatSearchTimeout", "TestChatSearch//Timeout"), "refuse", "allowed characters", evidence_lock(f"before-first,{before_sha}")),
        (ready, go_evidence.format(before_sha, "before-first").replace("TestChatSearchTimeout", "TestChatSearch/Timeout"), "allow", "", evidence_lock(f"before-first,{before_sha}", selection=subtest_selection)),
        (ready, playwright_evidence.format(before_sha, "before-rest").replace("title=pilot-1", "title= pilot-1"), "refuse", "allowed characters", evidence_lock(f"before-rest,{before_sha}")),
        (ready, go_evidence.format(before_sha, "before-first").replace(" -f test=TestChatSearchTimeout", ""), "refuse", "selection", evidence_lock(f"before-first,{before_sha}")),
        (ready, go_evidence.format(before_sha, "before-second"), "refuse", "series", evidence_lock(f"before-second,{before_sha}")),
        (ready, go_evidence.format(before_sha, "before-first").replace("kind=go", "kind=shell"), "refuse", "kinds", evidence_lock(f"before-first,{before_sha}")),
        (catch_up, catch_up_push, "allow", "", catch_up_locks[catch_up]),
        (catch_up, dispatch.format(catch_up_heads[catch_up], "full"), "allow", "", catch_up_locks[catch_up]),
        (catch_up, dispatch.format(catch_up_heads[catch_up], "docs"), "refuse", "full run", catch_up_locks[catch_up]),
        (catch_up, catch_up_push, "refuse", "catch-up claim", empty_lock_dir),
        (catch_up, catch_up_push, "refuse", "catch-up claim", dispatched_lock(catch_up, catch_up_heads[catch_up], "feat/x")),
        (catch_up, catch_up_push, "refuse", "first 12", dispatched_lock(catch_up, other_sha, "", kind="catch-up")),
        (catch_up, catch_up_push, "refuse", "another repository", dispatched_lock(catch_up, catch_up_heads[catch_up], "", kind="catch-up", common_dir="/nonexistent/.git")),
        (catch_up, branch_push.format("catch-up/000000000000"), "refuse", "first 12", catch_up_locks[catch_up]),
        (catch_up, branch_push.format("catch-up/ABCDEF012345"), "refuse", "names", catch_up_locks[catch_up]),
        (catch_up, dispatch.format(catch_up_heads[catch_up], "full"), "refuse", "only parent", dispatched_lock(catch_up, catch_up_heads[catch_up], "feat/x")),
        (catch_up_personal, branch_push.format(f"catch-up/{catch_up_heads[catch_up_personal][:12]}"), "refuse", "personal-data", catch_up_locks[catch_up_personal]),
        (catch_up_far, branch_push.format(f"catch-up/{catch_up_heads[catch_up_far][:12]}"), "refuse", "only parent", catch_up_locks[catch_up_far]),
        (catch_up_far, dispatch.format(catch_up_heads[catch_up_far], "full"), "refuse", "only parent", catch_up_locks[catch_up_far]),
        (ready, branch_push.format("feat/personal-email"), "refuse", "personal-data", empty_lock_dir),
        (ready, branch_push.format("feat/personal-phone"), "refuse", "personal-data", empty_lock_dir),
        (ready, branch_push.format("feat/personal-plain-phone"), "refuse", "personal-data", empty_lock_dir),
        (ready, branch_push.format("feat/reserved-addresses"), "allow", "", empty_lock_dir),
        (ready, branch_push.format("feat/personal-message"), "refuse", "personal-data", empty_lock_dir),
        (ready, branch_push.format("feat/personal-author"), "allow", "", empty_lock_dir),
        (ready, branch_push.format("feat/noreply-author"), "allow", "", empty_lock_dir),
        (ready, branch_push.format("feat/gate-change-r2"), "allow", "", empty_lock_dir),
        (ready, branch_push.format("fix/flaky-before"), "allow", "", empty_lock_dir),
        (ready, main_push, "allow", "", ci_log),
        (ready, main_push, "allow", "", catch_up_log),
        (ready, main_push, "refuse", "not a ci", gate_log),
        (ready, main_push, "refuse", "not a ci", later_gate_log),
        (ready, main_push, "refuse", "does not name", stale_log),
        (ready, main_push, "refuse", "result=passed", failed_log),
        (ready, main_push, "refuse", "result=passed", extra_key_log),
        (ready, main_push, "refuse", "finish line", unprefixed_log),
        (ready, main_push, "refuse", "finish line", attempt_only_log),
        (two_ahead, main_push, "refuse", "one finished merge", two_ahead_log),
        (workflow_main, main_push, "refuse", ".github", ci_log),
        (diverged, main_push, "refuse", "ancestor", ci_log),
    ]
    locked_cases.extend((ready, command, "refuse", "runs in CI", empty_lock_dir) for command in local_checks)
    locked_cases.extend((ready, command, "allow", "", empty_lock_dir) for command in file_writers)
    for cwd, command, expected, fragment, lock_dir in [(*case, empty_lock_dir) for case in cases] + locked_cases:
        github_cases += 1
        outcome, reason = decision(command, cwd, lock_dir)
        if outcome != expected or fragment not in reason:
            failed += 1
            print("FAIL", outcome, repr(command), reason.strip())
    for tool, key, path in [("Read", "file_path", "~/.config/gh/hosts.yml"), ("Grep", "path", "~/.config/gh")]:
        github_cases += 1
        if file_decision(tool, key, path) != "deny":
            failed += 1
            print("FAIL credential file not denied", tool, path)

def hook_constants() -> tuple[dict[str, int], str]:
    specification = importlib.util.spec_from_file_location("guard_secrets_constants", hook)
    if specification is None or specification.loader is None:
        raise RuntimeError("the hook could not be loaded")
    module = importlib.util.module_from_spec(specification)
    specification.loader.exec_module(module)
    return dict(module.CI_JOB_SHARDS), module.EVIDENCE_VALUES["entry"].pattern


def merge_gate_job_shards() -> tuple[dict[str, int], set[str]]:
    gate = os.path.join(repository, "tools", "merge-gate.sh")
    shards: dict[str, int] = {}
    for scope in ("full", "docs"):
        matrix = json.loads(subprocess.run(["bash", gate, "ci-matrix", "--scope", scope], capture_output=True, text=True, check=True).stdout)
        for name, value in matrix.items():
            if name == "jobs":
                shards.update({job: 0 for job in value})
            elif value:
                shards[name] = len(value)
    listed = set(subprocess.run(["bash", gate, "ci-jobs"], capture_output=True, text=True, check=True).stdout.split())
    return shards, listed


hook_shards, entry_pattern = hook_constants()
gate_shards, gate_jobs = merge_gate_job_shards()
entry_jobs = set(re.findall(r"[a-z][a-z-]+", entry_pattern.split(")", 1)[0]))
unsharded_jobs = {name for name, count in gate_shards.items() if count == 0}
if hook_shards != gate_shards or not set(hook_shards) <= gate_jobs or entry_jobs != unsharded_jobs:
    failed += 1
    print("FAIL the hook's CI_JOB_SHARDS", hook_shards, "or entry jobs", sorted(entry_jobs), "differ from merge-gate.sh", gate_shards, sorted(gate_jobs))

with open(hook, encoding="utf-8") as hook_file:
    hook_source = hook_file.read()
with open(os.path.join(repository, ".claude", "settings.json"), encoding="utf-8") as settings_file:
    permissions = json.load(settings_file)["permissions"]
required_denials = {"Bash(git *--force*)", "Bash(git *--amend*)", "Bash(git * -f *)", "Bash(git checkout *)", "Bash(git filter-branch*)", "Bash(git update-ref *)", "Bash(git reflog expire*)"}
if "ask" in permissions or not required_denials <= set(permissions.get("deny", [])):
    failed += 1
    print("FAIL the settings hold ask rules or lack a force or history deny rule")
agent_forms = [
    "git push origin refs/heads/feat/clean-change:refs/heads/feat/clean-change",
    "git push origin refs/heads/main:refs/heads/main",
    "git branch -D feat/clean-change",
    "git reset --merge",
    "git restore --staged AGENTS.md",
]
bash_denials = [rule[len("Bash(") : -1] for rule in permissions.get("deny", []) if rule.startswith("Bash(") and rule.endswith(")")]
for form in agent_forms:
    denying = [pattern for pattern in bash_denials if fnmatch.fnmatchcase(form, pattern)]
    if denying:
        failed += 1
        print("FAIL a deny rule refuses an agent form of AGENTS.md", repr(form), denying)
for form in agent_forms[2:]:
    outcome, reason = decision(form, repository)
    if outcome != "allow":
        failed += 1
        print("FAIL the hook does not pass an agent form of AGENTS.md", repr(form), outcome, reason.strip())
if "permissionDecision" in hook_source or re.search(r"[\"']ask[\"']", hook_source):
    failed += 1
    print("FAIL the hook still holds an ask decision")

total = len(refused) + len(allowed) + 2 + len(exception_paths) + 2 + len(agent_forms) + len(agent_forms[2:]) + github_cases
print(f"{total - failed} passed, {failed} failed")
lock_scratch.cleanup()
sys.exit(failed)
