# Development

Status: draft written 2026-10-20, amended the same day with local services and the Go backend toolchain, and on 2026-10-21 with Docker Compose services, security scanning and the backend scaffold; pending line-by-line owner review.

How to set up, check and verify changes locally. Read [AGENTS.md](../AGENTS.md) before contributing. The repository currently contains documentation and agent configuration only; backend and frontend commands are registered here by [roadmap slice 1](roadmap.md#slice-1-scaffold-and-local-loop).

## Tool setup

Use Linux or macOS with Git, Bash, Docker with Compose v2 (Docker Desktop, Colima or OrbStack all work), Node (for `npx`), a system `python3` (3.9 or newer, for the agent hooks in `.claude/hooks/`), a C compiler (for `go test -race`, which needs cgo) and the pinned toolchain:

- Go: the version in [`tools/go.mod`](../tools/go.mod) (`go` and `toolchain` lines, matched by `backend/go.mod`), installed from [the official downloads](https://go.dev/dl/) or a package manager; the `toolchain` line lets newer Go installations fetch the pinned release.
- Repository tools (Gitleaks and OSV-Scanner now; govulncheck and golangci-lint, built from the pinned versions into the ignored `bin/` directory because they must type-check the backend module; sqlc from slice 2): pinned as Go `tool` dependencies in `tools/go.mod`, with checksums in `tools/go.sum`, and run from the repository root as `go -C tools tool <tool>`. No separate installation is needed.
- Node and pnpm: pinned by the frontend scaffold in `frontend/.node-version` and the `packageManager` field of `frontend/package.json`.
- lychee for link checks: installed with its [official instructions](https://lychee.cli.rs/installation/).

Exact versions are recorded with their check dates in `knowledge/research/` when the scaffold pins them.

## Documentation checks

Active now and required for every change. Run from the repository root in Bash. Git lists tracked and non-ignored untracked Markdown, so dependencies and ignored scratch files are excluded.

```bash
set -euo pipefail
OVC_DOC_FILES="$(mktemp)"
git ls-files --cached --others --exclude-standard -z -- '*.md' > "$OVC_DOC_FILES"
test -s "$OVC_DOC_FILES"
xargs -0 npx --yes markdownlint-cli@0.45.0 -- < "$OVC_DOC_FILES"
xargs -0 lychee --config lychee.toml -- < "$OVC_DOC_FILES"

out="$(mktemp -d)"
parsed=0
while IFS= read -r -d '' file; do
  grep -q '^```mermaid' "$file" || continue
  echo "parsing $file"
  npx --yes @mermaid-js/mermaid-cli@12.0.0 --quiet \
    --input "$file" --output "$out/${file//\//_}" --artefacts "$out"
  parsed=$((parsed + 1))
done < "$OVC_DOC_FILES"
test "$parsed" -gt 0
```

The link check is offline and validates relative links and heading anchors. Mermaid rendering downloads a headless browser on first use.

## Security scanning

Run from the repository root; each must pass before a change is ready.

- **Secrets in history:** `go -C tools tool gitleaks git --redact --no-banner ..` reports no leaks.
- **Secrets about to be committed:** `go -C tools tool gitleaks git --staged --redact --no-banner ..` reports no leaks. Gitleaks never scans the working directory, so ignored environment files are never read.
- **Known vulnerabilities in dependencies:** OSV-Scanner over each module and lockfile with an explicit path, starting with `go -C tools tool osv-scanner scan source --no-call-analysis=go --lockfile=go.mod` (the tools module has no packages of its own to call-analyse) and `go -C tools tool osv-scanner scan source --no-call-analysis=go --lockfile=../backend/go.mod` (the backend gets call analysis from `govulncheck` in the backend checks); `frontend/pnpm-lock.yaml` joins with the frontend scaffold. A pass is exit code 0. Accepted findings are listed in the `osv-scanner.toml` next to the scanned file, each with a reason and an `ignoreUntil` date, and explained in a dated triage table ([current triage](../knowledge/research/local-s3-2026-10-21.md#repository-tool-scan-triage)).

## Backend checks

Build the pinned linters once per tools upgrade, from the repository root:

```bash
go -C tools build -o ../bin/ github.com/golangci/golangci-lint/v2/cmd/golangci-lint golang.org/x/vuln/cmd/govulncheck
```

Then, from `backend/`, with Docker Compose running and `OVC_TEST_DATABASE_ADMIN_URL` set to a PostgreSQL URL that may create databases (for example the `ovc` user on `ovc_dev`):

```bash
go mod tidy -diff
test -z "$(gofmt -l .)"
go vet ./...
../bin/golangci-lint run ./...
../bin/govulncheck ./...
go test -race -count=1 ./...
```

Every command must exit 0. The tests include the package-layer architecture test (it parses the [package layers table](../knowledge/architecture/detailed.md#package-layers)), the no-comments and no-`any` conventions test, integration tests that each create and drop their own `ovc_<random>_test` database, and process tests that build the `api` and `worker` binaries and run them with an explicit environment. A missing or unreachable test database fails the run; nothing is skipped. `sqlc diff` joins this list in slice 2. The lint configuration excludes gosec's G204 (subprocess with variable arguments) and G304 (file path from a variable) for `_test.go` files only, because the process tests run the binaries they have just built and the convention tests read the repository's own files; production code keeps both rules.

## Frontend checks

Pending implementation in slice 1. The scaffold registers, at minimum: `pnpm --dir frontend install --frozen-lockfile`, `typecheck`, `lint`, `format:check`, `test` and `build` scripts, and the Playwright smoke run against a production build. Each later slice adds its parts of the pilot scenario suites.

## Local services

The local environment has:

- **Docker Compose** with PostgreSQL 18 and SeaweedFS as the local S3-compatible server ([research](../knowledge/research/local-s3-2026-10-21.md)), both images pinned by digest in [`docker-compose.yml`](../docker-compose.yml).
  - Every port is published on `127.0.0.1` only; check with `docker compose config | grep host_ip`, which must print `host_ip: 127.0.0.1` for every port.
  - The development credentials are required variables listed in [`.env.example`](../.env.example); copy it to the git-ignored `.env` next to it and fill in your own values. There are no default passwords, and PostgreSQL uses SCRAM authentication for every connection.
  - Host ports default to 55432 (PostgreSQL) and 58333 (storage), away from a PostgreSQL that may already run on 5432; `OVC_DEV_POSTGRES_PORT` and `OVC_DEV_STORAGE_PORT` change them.
  - SeaweedFS also runs its master, volume and filer services inside the container without authentication; they are not published to the host, and on Linux they are reachable from the host and other containers only on the Compose network. Development data only.
  - Agents never run commands that print the interpolated credentials (`docker compose config` without a filter, `docker inspect`, `env` inside a container); the filtered `grep host_ip` form above is the only allowed use of `docker compose config`.
  - The development database is `ovc_dev`. `docker compose up -d` starts both services; `docker compose ps` shows them `healthy` within a minute; `docker compose down` stops them and keeps the data volumes.
- **Three commands**, all reading the same settings from the environment (copy `backend/.env.example`; nothing loads it automatically): `go run ./cmd/api migrate` applies the migrations with a role that may change the schema; `go run ./cmd/api` serves on `OVC_HTTP_ADDR` and refuses to start while migrations are pending; `go run ./cmd/worker` runs the `Tick(now)` loop. Restart them by hand after a change.
- **Fakes**:
  - the meeting provider is `fake`, because BigBlueButton runs only on a dedicated Ubuntu server and not on developer machines ([research](../knowledge/research/bigbluebutton-2026-10-20.md#server));
  - mail goes to the database sink. Development runs with `OVC_TEST_CONTROL=1` against a development database, so the sink is readable only through the harness route below.
- **Live conferencing checks**, from slice 7 on, use a remote BigBlueButton test server configured through `OVC_BBB_URL` and `OVC_BBB_SECRET` in `backend/.env.local`, which is git-ignored and unreadable by agents. They are manual: the opt-in command runs the BigBlueButton adapter's contract tests. A readiness report lists the result, or `unverified` when the check was not run. The definition of done does not require them.

Automated tests never call a real BigBlueButton server, SMTP provider or production storage.

## End-to-end test harness

Started in slice 1 (the clock and tick routes), extended by each slice. The [pilot scenarios](../knowledge/design/web/pilot-scenarios.md) run against local test deployments: the real API, worker, database and storage in local containers, configured with `OVC_TEST_CONTROL=1` and `OVC_CLOCK=test`.

- **Safety:**
  - both processes refuse to start with `OVC_TEST_CONTROL=1` unless the meeting provider is `fake`, mail is the database sink, `OVC_HTTP_ADDR` is a loopback IP literal, every database host is a loopback IP literal, `localhost` resolving only to loopback, or a Unix socket (checked before connecting), and `SELECT current_database()` matches `^[a-z0-9_]+_(test|dev)$`; they also refuse `OVC_CLOCK=test` without `OVC_TEST_CONTROL=1`;
  - the control routes require `Content-Type: application/json`, bodies of at most 4 KiB and a loopback `Host` header, so a page in the developer's browser or a rebinding host name cannot drive them;
  - without the setting, none of the routes below exist, and a test proves it.
- **Worker:** with `OVC_CLOCK=test` the worker's own loops are paused, so only harness ticks run jobs. Local development keeps `OVC_CLOCK=system`, so its worker loops run normally while the control routes and the mail viewer stay available.
- **Clock:** the fake clock reads its current instant from a single test-clock row, so the API and the worker of one deployment share one time. Playwright sets the browser clock to the same instant with `page.clock`.
- **Parallelism:** each Playwright worker gets its own deployment (own database, API and worker processes, storage bucket), so suites that move the clock never interfere. Suites assigned to one worker run serially.
- **Control routes:** `/__test__/` routes, absent from every other configuration and from OpenAPI:

  | Route | Effect |
  |---|---|
  | `POST /__test__/clock` | Set or advance the shared clock |
  | `POST /__test__/tick` | Run one worker `Tick(now)` to completion |
  | `POST /__test__/mail` with `{to}` in the body | Read messages from the database mail sink, with their links |
  | `POST /__test__/presence` with `{user}` | Report whether the server sees the user online, so tests can wait for a socket close |
  | `POST /__test__/settings/embed` | Override `embed_allowed` for this deployment, until the next override |
  | `POST /__test__/fake-provider/lessons/{lesson}/recording` | Start or stop recording in the lesson's latest fake meeting |
  | `POST /__test__/fake-provider/lessons/{lesson}/ready` | Make the fake provider send a correctly signed recording-ready callback for the lesson's latest fake meeting |

- **Fake room:** the fake meeting provider's join link opens `/__test__/fake-room`, which shows the meeting, participant name, role and recording state, refreshing every second. Under `OVC_TEST_CONTROL=1` it is served with framing allowed from the OVC origin, so both the embedded and the new-tab paths can be asserted.
- **Isolation:** each suite seeds its own tutors and students through the real API with unique emails.

## Local review

Use the [ovc-verify skill](../.claude/skills/ovc-verify/SKILL.md) before planning and before reporting readiness. Claude Code discovers it in `.claude/skills/ovc-verify`. The skill picks one of three tiers ([ADR-0011](decisions/0011-tiered-verification.md)): trivial changes run the checks above; standard changes add one fresh reviewer with the standard checklist, plus a security reviewer when the change meets the security trigger defined in [AGENTS.md](../AGENTS.md#development-verification); when a roadmap slice is complete, the coordinating agent follows direct and transitive effects across the slice, splits affected components, and starts a fresh subagent per component/profile pair, and reviewers run the applicable checks above. The [coverage checklist](../knowledge/reviews/ai-development-setup.md) lists component families, workloads and activation.

## Changes and commits

Every nontrivial change starts with an owner-approved plan and lives on its own branch as one commit. Once its checks pass and its readiness reviews are resolved, the agent merges it into `main` with `git merge --ff-only` and deletes the branch, as [AGENTS.md](../AGENTS.md#change-workflow) describes. Agents never push, rebase, reset, clean, force-delete branches, discard work with destructive checkouts, restores or stash drops, or rewrite history (amend, filter-branch, update-ref, reflog expiry) without the owner. A change that alters a contract updates its specification, requirement or decision record in the same change. New dependencies enter only with an implemented consumer and a recorded reason in the [stack table](../knowledge/architecture/detailed.md#stack-per-component).
