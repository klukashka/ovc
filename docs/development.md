# Development

Status: draft written 2026-10-20, amended the same day with local services; pending line-by-line owner review.

How to set up, check and verify changes locally. Read [AGENTS.md](../AGENTS.md) before contributing. The repository currently contains documentation and agent configuration only; backend and frontend commands are registered here by [roadmap slice 1](roadmap.md#slice-1-scaffold-and-local-loop).

## Tool setup

Use Linux or macOS with Git, Bash, Docker, Node (for `npx`) and the pinned toolchain:

- Python: the version in [`.python-version`](../.python-version), installed with `uv python install`.
- uv: installed with its [official installer](https://docs.astral.sh/uv/getting-started/installation/).
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

## Backend checks

Pending implementation in slice 1. The scaffold registers, at minimum: `uv lock --check`, `uv run ruff format --check .`, `uv run ruff check .`, `uv run pyright`, and `uv run pytest -q`. The pytest run includes the package-layer test and integration tests against PostgreSQL and a local S3-compatible server in Docker, with the worker driven through `tick(now)`.

## Frontend checks

Pending implementation in slice 1. The scaffold registers, at minimum: `pnpm --dir frontend install --frozen-lockfile`, `typecheck`, `lint`, `format:check`, `test` and `build` scripts, and the Playwright smoke run against a production build. Each later slice adds its parts of the pilot scenario suites.

## Local services

Pending implementation in slice 1. The local environment has:

- **Docker Compose** with PostgreSQL and a local S3-compatible server.
- **Two processes**, both reading the same settings: the API (`uvicorn` with reload) and the worker (the `tick(now)` loop).
- **Fakes**:
  - the meeting provider is `fake`, because BigBlueButton runs only on a dedicated Ubuntu server and not on developer machines ([research](../knowledge/research/bigbluebutton-2026-10-20.md#server));
  - mail goes to the database sink. Development runs with `OVC_TEST_CONTROL=1` against a development database, so the sink is readable only through the harness route below.
- **Live conferencing checks**, from slice 7 on, use a remote BigBlueButton test server configured through `OVC_BBB_URL` and `OVC_BBB_SECRET` in `backend/.env.local`, which is git-ignored and unreadable by agents. They are manual: the opt-in command runs the BigBlueButton adapter's contract tests. A readiness report lists the result, or `unverified` when the check was not run. The definition of done does not require them.

Automated tests never call a real BigBlueButton server, SMTP provider or production storage.

## End-to-end test harness

Pending implementation in slice 1, extended by each slice. The [pilot scenarios](../knowledge/design/web/pilot-scenarios.md) run against local test deployments: the real API, worker, database and storage in local containers, configured with `OVC_TEST_CONTROL=1` and `OVC_CLOCK=test`.

- **Safety:**
  - both processes refuse to start with `OVC_TEST_CONTROL=1` unless the meeting provider is `fake`, mail is the database sink, and the database name marks a test or development database;
  - without the setting, none of the routes below exist, and a test proves it.
- **Worker:** with `OVC_CLOCK=test` the worker's own loops are paused, so only harness ticks run jobs. Local development keeps `OVC_CLOCK=system`, so its worker loops run normally while the control routes and the mail viewer stay available.
- **Clock:** the fake clock reads its current instant from a single test-clock row, so the API and the worker of one deployment share one time. Playwright sets the browser clock to the same instant with `page.clock`.
- **Parallelism:** each Playwright worker gets its own deployment (own database, API and worker processes, storage bucket), so suites that move the clock never interfere. Suites assigned to one worker run serially.
- **Control routes:** `/__test__/` routes, absent from every other configuration and from OpenAPI:

  | Route | Effect |
  |---|---|
  | `POST /__test__/clock` | Set or advance the shared clock |
  | `POST /__test__/tick` | Run one worker `tick(now)` to completion |
  | `POST /__test__/mail` with `{to}` in the body | Read messages from the database mail sink, with their links |
  | `POST /__test__/presence` with `{user}` | Report whether the server sees the user online, so tests can wait for a socket close |
  | `POST /__test__/settings/embed` | Override `embed_allowed` for this deployment, until the next override |
  | `POST /__test__/fake-provider/lessons/{lesson}/recording` | Start or stop recording in the lesson's latest fake meeting |
  | `POST /__test__/fake-provider/lessons/{lesson}/ready` | Make the fake provider send a correctly signed recording-ready callback for the lesson's latest fake meeting |

- **Fake room:** the fake meeting provider's join link opens `/__test__/fake-room`, which shows the meeting, participant name, role and recording state, refreshing every second. Under `OVC_TEST_CONTROL=1` it is served with framing allowed from the OVC origin, so both the embedded and the new-tab paths can be asserted.
- **Isolation:** each suite seeds its own tutors and students through the real API with unique emails.

## Local review

Use the [ovc-verify skill](../.claude/skills/ovc-verify/SKILL.md) before planning and before reporting readiness. Claude Code discovers it in `.claude/skills/ovc-verify`. The coordinating agent follows direct and transitive effects, splits affected components, and starts a fresh subagent per component/profile pair; reviewers run the applicable checks above. The [coverage checklist](../knowledge/reviews/ai-development-setup.md) lists component families, workloads and activation.

## Changes and commits

Every nontrivial change starts with an owner-approved plan and lives on its own branch as one commit. Once its checks pass and its readiness reviews are resolved, the agent merges it into `main` with `git merge --ff-only` and deletes the branch, as [AGENTS.md](../AGENTS.md#change-workflow) describes. Agents never push, rebase, reset, clean, force-delete branches, discard work with destructive checkouts, restores or stash drops, or rewrite history (amend, filter-branch, update-ref, reflog expiry) without the owner. A change that alters a contract updates its specification, requirement or decision record in the same change. New dependencies enter only with an implemented consumer and a recorded reason in the [stack table](../knowledge/architecture/detailed.md#stack-per-component).
