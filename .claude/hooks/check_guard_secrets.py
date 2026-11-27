#!/usr/bin/env python3
from __future__ import annotations

import json
import os
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
    "pnpm --dir frontend test",
    "ps aux",
    "ps -ef",
    "go test ./...",
    "ps -p 1234 -o etime=",
    "ps -o pid,user,state",
    "ps -eo pid,comm",
    "git commit -am \"Refuse cat /proc/1/environ\"",
    "python3 - <<'EOF'\nprint('environment notes')\nEOF",
    "timeout 60 go test ./...",
    "env -C backend go test ./...",
    "docker compose config --quiet",
    "docker compose exec -T postgres pg_isready",
    "git commit -m 'Refuse `env` and `printenv` in the guard'",
    "gh pr create --body 'Refuse `printenv` dumps'",
    "grep -rn process.env frontend/e2e",
    "rg -n 'import.meta.env' frontend/src",
    "docker compose -f compose.yaml config --services",
    "docker volume inspect ovc_pgdata",
    "docker network inspect ovc_default",
    "command -v env",
    "tools/dev-env.sh go -C backend test ./...",
    "tools/dev-env.sh pnpm --dir frontend dev",
    "tools/dev-env.sh go -C backend test -v ./internal/core/",
    "bash tools/dev-env.sh go -C backend test ./...",
    "tools/dev-env.sh bash tools/check-dev-env.sh",
    "tools/dev-env.sh bash -o pipefail scripts/run.sh",
    "tools/dev-env.sh bash -c 'set -e; go version'",
    "bash -n tools/dev-env.sh",
    "tools/dev-env.sh bash -c 'echo use set -x to debug'",
    "tools/dev-env.sh bash -c 'set -euo pipefail; go version'",
    "pnpm exec prettier --write src",
    "pnpm -s exec vitest run src/lib/api",
    "tools/dev-env.sh pnpm exec playwright test e2e/smoke.spec.ts",
    "npx playwright test",
    "pnpm --dir frontend exec tsc -b",
    "pnpm -s audit --audit-level high",
    "pnpm --filter frontend test",
    "tools/dev-env.sh pnpm e2e",
    "npm exec -- prettier --check .",
    "pnpm exec prettier --write src e2e playwright.config.ts",
]
failed = 0
for command, expected in [(command, 2) for command in refused] + [(command, 0) for command in allowed]:
    payload = json.dumps({"tool_name": "Bash", "tool_input": {"command": command}})
    code = subprocess.run([sys.executable, hook], input=payload, text=True, capture_output=True).returncode
    if code != expected:
        failed += 1
        print("FAIL", code, repr(command))
malformed = subprocess.run([sys.executable, hook], input=json.dumps({"tool_name": "Bash", "tool_input": {"command": 5}}), text=True, capture_output=True).returncode
if malformed != 2:
    failed += 1
    print("FAIL non-string command allowed")

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


def decision(command: str, cwd: str) -> tuple[str, str]:
    payload = json.dumps({"tool_name": "Bash", "tool_input": {"command": command}, "cwd": cwd})
    result = subprocess.run([sys.executable, hook], input=payload, text=True, capture_output=True)
    if result.returncode == 2:
        return "deny", result.stderr
    if result.returncode != 0:
        return f"exit {result.returncode}", result.stderr
    if not result.stdout.strip():
        return "allow", ""
    output = json.loads(result.stdout)["hookSpecificOutput"]
    return output["permissionDecision"], output["permissionDecisionReason"]


def file_decision(tool: str, key: str, path: str) -> str:
    payload = json.dumps({"tool_name": tool, "tool_input": {key: path}})
    result = subprocess.run([sys.executable, hook], input=payload, text=True, capture_output=True)
    return "deny" if result.returncode == 2 else "allow"


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

    branch_push = "git push origin refs/heads/{0}:refs/heads/{0}"
    main_push = "git push origin refs/heads/main:refs/heads/main"
    sha = git(ready, "rev-parse", "refs/heads/feat/clean-change")
    main_sha = git(ready, "rev-parse", "refs/heads/main")
    two_commits_sha = git(ready, "rev-parse", "refs/heads/feat/two-commits")
    other_name_sha = git(ready, "rev-parse", "refs/heads/wip/other-name")
    cases = [
        (ready, branch_push.format("feat/clean-change"), "allow", ""),
        (ready, branch_push.format("feat/leaky-change"), "ask", "did not pass"),
        (unscanned, branch_push.format("feat/clean-change"), "ask", "missing"),
        (ready, branch_push.format("feat/two-commits"), "ask", "one commit"),
        (ready, branch_push.format("feat/workflow-change"), "ask", ".github"),
        (ready, branch_push.format("wip/other-name"), "ask", "names"),
        (ready, main_push, "allow", ""),
        (linked, branch_push.format("feat/clean-change"), "allow", ""),
        (linked, branch_push.format("feat/leaky-change"), "ask", "did not pass"),
        (blind_working_tree, branch_push.format("feat/leaky-change"), "ask", "did not pass"),
        (ignoring_working_tree, branch_push.format("feat/leaky-change"), "ask", "did not pass"),
        (workflow_main, main_push, "ask", ".github"),
        (diverged, main_push, "ask", "ancestor"),
        (ready, "git push", "ask", ""),
        (ready, "git push origin feat/clean-change", "ask", ""),
        (ready, "git push origin +refs/heads/feat/clean-change:refs/heads/feat/clean-change", "ask", ""),
        (ready, "git push origin :refs/heads/feat/clean-change", "ask", ""),
        (ready, "git push --delete origin feat/clean-change", "ask", ""),
        (ready, "git push --mirror origin", "ask", ""),
        (ready, "git push --all origin", "ask", ""),
        (ready, "git push --tags origin", "ask", ""),
        (ready, "git push --receive-pack=x origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "git push --exec=x origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "git push -u origin refs/heads/feat/clean-change:refs/heads/feat/clean-change", "ask", ""),
        (ready, "git push upstream refs/heads/feat/clean-change:refs/heads/feat/clean-change", "ask", ""),
        (ready, "git send-pack origin main", "ask", ""),
        (ready, "git -c alias.p=push p origin main", "ask", ""),
        (ready, "git -C . push origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "git publish origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "env git push origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "command git push origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "/usr/bin/git push origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "bash -c 'git push origin refs/heads/main:refs/heads/main'", "ask", ""),
        (ready, "eval 'git push origin refs/heads/main:refs/heads/main'", "ask", ""),
        (ready, "echo origin | xargs git push", "ask", ""),
        (ready, "git push origin $(git branch --show-current)", "ask", ""),
        (ready, "GIT_SSH_COMMAND=x git push origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "GH_TOKEN=x gh run list", "ask", ""),
        (ready, "cd . && git push origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "git status; git push origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "git push origin refs/heads/main:refs/heads/main | cat", "ask", ""),
        (ready, "python3 -c 'import subprocess; subprocess.run([\"git\", \"push\"])'", "ask", ""),
        (ready, "git remote", "allow", ""),
        (ready, "git remote -v", "allow", ""),
        (ready, "git remote get-url origin", "allow", ""),
        (ready, "git remote add other https://example.invalid/other.git", "ask", ""),
        (ready, "git remote set-url origin https://example.invalid/other.git", "ask", ""),
        (ready, "git config --get user.name", "allow", ""),
        (ready, "git config user.name", "allow", ""),
        (ready, "git config --list --show-origin", "allow", ""),
        (ready, "git config user.name Other", "ask", ""),
        (ready, "git config --global alias.p push", "ask", ""),
        (ready, "git config --unset user.name", "ask", ""),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=full", "allow", ""),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=docs", "allow", ""),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=other", "ask", ""),
        (ready, "gh workflow run gate.yml --ref main -f sha=abc -f scope=full", "ask", ""),
        (ready, f"gh workflow run gate.yml --ref main --ref feat/x -f sha={sha} -f scope=full", "ask", ""),
        (ready, f"gh workflow run gate.yml --ref main -f sha={sha} -f scope=full -R other/repo", "ask", ""),
        (ready, f"gh workflow run other.yml --ref main -f sha={sha} -f scope=full", "ask", ""),
        (ready, f"gh workflow run gate.yml --ref main -f sha={main_sha} -f scope=full", "ask", "only parent"),
        (ready, f"gh workflow run gate.yml --ref main -f sha={two_commits_sha} -f scope=full", "ask", "only parent"),
        (ready, f"gh workflow run gate.yml --ref main -f sha={other_name_sha} -f scope=full", "ask", "tip of a local"),
        (ready, f"gh workflow run gate.yml --ref main -f sha={other_sha} -f scope=full", "ask", "only parent"),
        (ready, "gh workflow enable gate.yml", "ask", ""),
        (ready, "gh workflow disable gate.yml", "ask", ""),
        (ready, "gh run list --workflow gate.yml --limit 5", "allow", ""),
        (ready, "gh run view 123 --log-failed", "allow", ""),
        (ready, "gh run watch 123 --exit-status", "allow", ""),
        (ready, "gh run list --repo other/repo", "ask", ""),
        (ready, "gh run rerun 123", "ask", ""),
        (ready, "gh run cancel 123", "ask", ""),
        (ready, "gh run delete 123", "ask", ""),
        (ready, "gh pr view feat/clean-change --json number,state,headRefName,url", "allow", ""),
        (ready, "gh pr list --state open --json number,state,headRefOid", "allow", ""),
        (ready, "gh pr view 3 --json body", "ask", ""),
        (ready, "gh pr view 3 --json number,comments", "ask", ""),
        (ready, "gh pr view 3", "ask", ""),
        (ready, "gh pr view 3 --comments --json number", "ask", ""),
        (ready, "gh pr create --fill", "ask", ""),
        (ready, "gh pr edit 3 --title x", "ask", ""),
        (ready, "gh pr merge 3", "ask", ""),
        (ready, "gh pr close 3", "ask", ""),
        (ready, "gh pr comment 3 --body x", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs", "allow", ""),
        (ready, f"gh api repos/klukashka/ovc/actions/runs/{run_id[:6]}/jobs --jq .jobs", "allow", ""),
        (ready, "gh api repos/klukashka/ovc/actions/jobs/123 -X GET", "allow", ""),
        (ready, f"gh api repos/klukashka/ovc/commits/{other_sha}/status -H 'Accept: application/vnd.github+json'", "allow", ""),
        (ready, f"gh api repos/klukashka/ovc/commits/{other_sha}/statuses --method GET", "allow", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs -X POST", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --method=DELETE", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --input payload.json", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs -f name=x", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs -F name=x", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --field name=x", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --raw-field name=x", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs -H 'Authorization: x'", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs --hostname example.invalid", "ask", ""),
        (ready, "gh api https://api.github.com/repos/klukashka/ovc/actions/runs", "ask", ""),
        (ready, "gh api graphql", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/pulls", "ask", ""),
        (ready, "gh api repos/other/repo/actions/runs", "ask", ""),
        (ready, "gh api repos/klukashka/ovc/actions/runs/../../pulls", "ask", "allowed repos"),
        (ready, "gh api repos/klukashka/ovc/actions/runs/..", "ask", "allowed repos"),
        (ready, "gh api repos/klukashka/ovc/actions/jobs/./123", "ask", "allowed repos"),
        (ready, "gh api 'repos/klukashka/ovc/actions/runs/..?per_page=1'", "ask", "allowed repos"),
        (ready, "gh api repos/klukashka/ovc/actions/runs/run..1", "allow", ""),
        (ready, "gh alias set p 'run list'", "ask", ""),
        (ready, "gh extension install x/y", "ask", ""),
        (ready, "gh gist create notes.txt", "ask", ""),
        (ready, "gh repo view", "ask", ""),
        (ready, "gh secret list", "ask", ""),
        (ready, "gh variable list", "ask", ""),
        (ready, "gh label list", "ask", ""),
        (ready, "gh release list", "ask", ""),
        (ready, "gh ssh-key list", "ask", ""),
        (ready, "gh gpg-key list", "ask", ""),
        (ready, "gh cache list", "ask", ""),
        (ready, "gh auth status", "ask", ""),
        (ready, "gh auth token", "deny", ""),
        (ready, "gh auth status --show-token", "deny", ""),
        (ready, "gh auth status --show-token=true", "deny", ""),
        (ready, "gh auth status -t", "deny", ""),
        (ready, "gh auth status -at", "deny", ""),
        (ready, "gh auth status --hostname github.com -t", "deny", ""),
        (ready, "gh auth status --active", "ask", ""),
        (ready, "gh config get oauth_token", "deny", ""),
        (ready, "gh config get -h github.com oauth_token", "deny", ""),
        (ready, "gh config get --host=github.com oauth_token", "deny", ""),
        (ready, "gh config get git_protocol", "ask", ""),
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
        (ready, "find . -name x -exec git push origin main \\;", "ask", ""),
        (ready, "env -S 'git push origin refs/heads/main:refs/heads/main'", "deny", ""),
        (ready, "sh -c \"bash -c 'git push origin main'\"", "ask", ""),
        (ready, "if git push origin main; then true; fi", "ask", ""),
        (ready, "echo 'unbalanced; git push origin main", "ask", ""),
        (ready, "G=git; $G push origin refs/heads/main:refs/heads/main", "ask", ""),
        (ready, "bash <<'EOF'\ngit push origin main\nEOF", "ask", ""),
        (ready, "bash <<'EOF'\necho git push\nEOF", "allow", ""),
    ]
    for cwd, command, expected, fragment in cases:
        github_cases += 1
        outcome, reason = decision(command, cwd)
        if outcome != expected or fragment not in reason:
            failed += 1
            print("FAIL", outcome, repr(command), reason.strip())
    for tool, key, path in [("Read", "file_path", "~/.config/gh/hosts.yml"), ("Grep", "path", "~/.config/gh")]:
        github_cases += 1
        if file_decision(tool, key, path) != "deny":
            failed += 1
            print("FAIL credential file not denied", tool, path)

total = len(refused) + len(allowed) + 1 + github_cases
print(f"{total - failed} passed, {failed} failed")
sys.exit(failed)
