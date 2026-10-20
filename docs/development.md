# Development

Status: draft written 2026-10-20; pending line-by-line owner review.

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

Pending implementation in slice 1. The scaffold registers, at minimum: `uv lock --check`, `uv run ruff format --check .`, `uv run ruff check .`, `uv run pyright`, and `uv run pytest -q` including the package-layer test and the integration tests against PostgreSQL in Docker.

## Frontend checks

Pending implementation in slice 1. The scaffold registers, at minimum: `pnpm --dir frontend install --frozen-lockfile`, `typecheck`, `lint`, `format:check`, `test` and `build` scripts, and the Playwright smoke run against a production build.

## Local review

Use the [ovc-verify skill](../.claude/skills/ovc-verify/SKILL.md) before planning and before reporting readiness. Claude Code discovers it in `.claude/skills/ovc-verify`. The coordinating agent follows direct and transitive effects, splits affected components, and starts a fresh subagent per component/profile pair; reviewers run the applicable checks above. The [coverage checklist](../knowledge/reviews/ai-development-setup.md) lists component families, workloads and activation.

## Changes and commits

Every nontrivial change starts with an owner-approved plan, lives on its own branch, and is merged into `main` as one squash commit by the owner. Agents commit their own work once its checks pass and never push, merge, rebase or reset. A change that alters a contract updates its specification, requirement or decision record in the same change. New dependencies enter only with an implemented consumer and a recorded reason in the [stack table](../knowledge/architecture/detailed.md#stack-per-component).
