# ADR-0026: A public GitHub repository and a shadow CI on GitHub-hosted runners

- Status: accepted, 2026-11-27 (pending owner review)
- Reviewed: decided by the project owner on 2026-11-27 with the [task plan](../../knowledge/reviews/github-ci-adoption-task-plan.md) and the owner's answers recorded in its last section; wording pending owner review, line by line
- Deciders: project owner
- Related: [proposal 0008](../proposals/0008-hosted-ci.md), [hosted CI research](../../knowledge/research/hosted-ci-2026-11-27.md), [ADR-0008](0008-hosting-and-data-residency.md), [ADR-0009](0009-owner-approved-exceptions-and-visibility.md), [ADR-0011](0011-tiered-verification.md), [ADR-0017](0017-dependency-release-age.md), [ADR-0018](0018-targeted-checks-and-merge-gate.md), [NFR-6](../requirements/non-functional.md#nfr-6-maintainability-and-verification), [threat model: T-18, T-19, T-39, T-41, T-49](../threat-model.md), [AGENTS.md, change workflow](../../AGENTS.md#change-workflow), [development guide, continuous integration](../development.md#continuous-integration-in-shadow-mode)

## Context

Every merge gate of [ADR-0018](0018-targeted-checks-and-merge-gate.md) runs on the owner's laptop. A full-run gate takes about 640 to 740 s, the coordinator runs about 15 to 20 of them a day, and the idle-machine rule stops every other check while one runs. Implementing agents' change checks and every after-fix evidence run use the same laptop. The owner decided on 2026-11-27 to take the checks off the laptop and run them on GitHub-hosted runners.

The owner created the repository `github.com/klukashka/ovc`. It was private at first. Hosted minutes for a private repository would have cost several hundred dollars a month, and paying GitHub from Russia is likely difficult ([research](../../knowledge/research/hosted-ci-2026-11-27.md#2-github-private-repositories-and-github-actions)). Public repositories use the standard hosted runners free and can enforce rulesets on the Free plan, so the owner chose to make the repository public. Before this record, no document recorded a remote, a CI service, a pull request template, an issue form or labels.

The work is split into two changes. This record decides the first one, a CI that runs beside the local gate and decides nothing (shadow mode). A second change, planned separately, will let a CI run replace the local gate.

## Decision

### Repository

- **Public, without a license file.** `github.com/klukashka/ovc` is `origin` and is public, so all rights are reserved. The owner switches the visibility himself. The author address in the history stays as it is, by the owner's choice; a `noreply` address is available for later commits.
- **Only `main` is pushed** before the switch, and it is pushed to `origin` fast-forward only after every merge. Candidate branches are pushed only as the shadow pair below describes. Local `main` stays authoritative: the merge, the finishing step and the bookkeeping stay on the laptop.

### Shadow CI

- **The local gate stays the only gate.** No CI result admits or blocks a merge, and no code path accepts one, so shadow mode needs no enforcement flag. Change checks, after-fix evidence runs and the merge gate run locally as before.
- **Shadow pairs are optional measurements.** For a one-commit candidate on `main`'s tip that it is about to gate locally, the coordinator may:
  1. check that `origin/main` equals local `main`, or record the run as not dispatched;
  2. check that the candidate's change checks and readiness reviews passed on exactly this tree, because a pushed branch is public at once and cannot be reliably withdrawn;
  3. push the candidate branch in the exact allowed form, after the Gitleaks scan of `main..<branch>`;
  4. dispatch the workflow with `scope=full` or `scope=docs`;
  5. gate locally as ADR-0018 requires;
  6. record both results in [the shadow-run record](../../knowledge/analysis/ci-shadow-runs.md): the candidate and its tree, the local result with every failing test, the run address and its result with every failing test, the wall time and summed job minutes, and the runner image version.
- **One run at a time.** One full run uses 20 jobs, the Free plan's limit of concurrent jobs, and GitHub keeps only one pending run per concurrency group, so the coordinator dispatches the next run only after the previous one has completed. A cancelled run is recorded as missing shadow data.
- **Expected failures.** A candidate that adds a required `OVC_*` setting, bumps a pin in `tools/ci-tools.sha256` or adds a Playwright project or spec group fails its own shadow run, because those inputs come from `main`. The row records such a failure as expected, not as a disagreement.
- **Only the coordinator dispatches.** Implementing agents and reviewers never push or dispatch.

### The workflow and its harness

- **Dispatch from `main` only.** `.github/workflows/gate.yml` is triggered only by `workflow_dispatch`, and its jobs refuse unless `github.ref` is `refs/heads/main`, so the workflow file that runs is always `main`'s. It has no `push`, `pull_request` or `pull_request_target` trigger, so a fork's pull request starts nothing of OVC's. The refusal inside the file does not by itself keep another workflow file from running: that guarantee comes from the agents' token lacking the Workflows permission and from the hook's check that a pushed branch leaves `.github/` as `main` has it, so no agent push can bring a workflow file of its own.
- **Two checkouts in every job,** both without persisted credentials: `harness/` at `main`'s tip and `candidate/` at the dispatched commit, which the first step compares with the input. Every harness script runs from `harness/`, so the candidate's own `tools/`, `.github/` and hooks never run as harness; the candidate's code runs only as the checks run it, as in the local gate. This keeps ADR-0018's rule that a change never runs its own gate.
- **The check list stays inline in `main`'s `tools/merge-gate.sh`.** New read-only subcommands, `list`, `run-job` and `classify`, let CI run that list split into jobs and shards by a table in the same script. Locally nothing calls them, and the gate's lanes are unchanged. The full list gains one entry, `tools/check-workflows.sh`. actionlint was planned as a second entry, but its latest release cannot be built beside the other pinned tools in `tools/go.mod` (it needs an earlier release candidate of `go.yaml.in/yaml/v4` than another tool requires), so the workflow is checked by `tools/check-workflows.sh` and by GitHub when it is dispatched, and actionlint returns once it builds with the shared version (the owner's decision of 2026-11-27).
- **Inputs** are the candidate's full commit hash, checked against `^[0-9a-f]{40}$`, and a scope chosen from `full` and `docs`. They reach scripts only through `env:`. The run's title is `gate <scope> <commit>`.
- **Permissions.** The workflow sets `permissions: {}`. Every job reads contents only, except the final `gate` job, which may write commit statuses and runs no candidate code. A commit status is for display only and is never evidence.
- **No Actions cache at all.** Every dispatch runs in `main`'s scope, and candidate code could write cache entries for that scope through the runtime token. Caching may return only through a later decision, for content verified on every restore.
- **Artifacts and logs.** On failure a job uploads only the redacted evidence the local gate keeps, with no Playwright traces. Each job logs the runner image and its version.
- **No stored secrets.** `harness/tools/ci-settings.sh` generates every `OVC_*` value with random throwaway credentials and keys inside the job and masks each generated value and its URL and user-information forms before anything prints it; other encodings of a value, such as base64 or percent-encoding, are not masked. It also exports the names of the generated settings, from which the evidence redaction of every job reads them, so the list has one source. The repository stores no secrets, variables or environments.
- **Tool sources.** Go and Node come from the candidate's pins through `actions/setup-go` and `actions/setup-node` with caching off. pnpm comes through a corepack tarball from the npm registry, checked against the SHA-512 in `tools/ci-tools.sha256` that matches the registry's integrity value, because Node 25 and later no longer ship corepack. lychee comes from its GitHub release, checked against a SHA-256 in the same file. The checksum file is read from `harness/`. The pinned Go tools of `bin/` are built from the candidate's `tools/go.mod` in every job whose entries use them: `backend-tests`, whose conventions test runs `bin/golangci-lint`, `backend-checks`, `frontend-checks` and `docs-checks`, and each of these jobs runs the tool-binary check first. Versions and action commits follow ADR-0017's release age.
- **`tools/check-workflows.sh`** rejects every workflow form that would break these rules. The [development guide](../development.md#continuous-integration-in-shadow-mode) lists them.

### Repository and account settings

The owner applies these settings in the browser when he switches the visibility. `tools/check-github-settings.sh`, run by the owner in his own terminal, compares the repository's settings with `tools/github-settings.json`.

- **Actions:** only `actions/checkout`, `actions/setup-go`, `actions/setup-node` and `actions/upload-artifact`, each at a full commit hash; full-hash pinning required; the default `GITHUB_TOKEN` read-only; Actions may not create or approve pull requests; artifact and log retention of 3 days, one repository setting for both; and an account spending limit of $0, so nothing can incur a charge.
- **Fork pull requests:** approval required for workflows from all outside contributors, and the owner never approves one, so no fork code runs on runners of this repository. Forking itself cannot be turned off for a public repository.
- **Rulesets:** on `main`, deletion and force pushes blocked and linear history required, with no bypass; fast-forward pushes stay allowed. On all tags, creation, update and deletion restricted, with no bypass; the owner lifts it deliberately when a release tag is needed.
- **Secret scanning and push protection** on, as a second line behind the local Gitleaks checks.
- **Features strangers could write to:** the wiki, Discussions, Projects and Pages are off. Issues stay on, and private vulnerability reporting is on, so security reports stay out of public issues.
- **Write access:** no collaborators, no deploy key with write access, no webhooks, no GitHub App or OAuth app with write access that the owner has not reviewed, and two-factor authentication on the owner's account. `tools/check-github-settings.sh` checks the collaborators, deploy keys and webhooks; the review of installed GitHub Apps and authorized OAuth apps, like two-factor authentication and the spending limit, is a manual check by the owner, because the script cannot read them.

### Credentials and agent permissions

- **The laptop's credential.** Pushes use HTTPS through `gh`'s credential helper with a fine-grained token for this repository only: contents, pull requests and actions write, metadata read, and no administration, secrets or workflows permission. That token cannot push a commit that changes `.github/workflows/`, so the owner pushes such commits, `main` after this change included, with his own credential. No SSH key with write access is configured for the remote.
- **Allowed forms.** The guard hook allows, as the whole command, only these GitHub operations:
  - `git push origin refs/heads/<branch>:refs/heads/<branch>` for a `feat/`, `fix/`, `chore/` or `docs/` branch that is one commit on local `main`, leaves `.github/` as `main` has it and passes the Gitleaks scan of `main..<branch>` with `main`'s committed configuration;
  - `git push origin refs/heads/main:refs/heads/main` when `origin/main` is an ancestor of local `main` and no commit between them changes `.github/`;
  - `gh workflow run gate.yml --ref main` with the two checked inputs, when the commit is the tip of a local `feat/`, `fix/`, `chore/` or `docs/` branch whose only parent is local `main`'s tip;
  - reads of runs, pull request metadata without bodies or comments, and `GET` requests on the run, job and commit status paths of this repository.

  Every other `git push`, `git remote` or `git config` write and every other `gh` call asks the owner. Printing or exporting the `gh` token, Git credential commands, the keychain reads, edits of Git configuration files and the GitHub plugin's remote tools are denied. The hook guards against accidents and is not a security boundary, because Bash is allowed and a script can call `git` or `gh` itself (T-41).
- **The `git push` ask rules of `.claude/settings.json` stay.** Claude Code evaluates deny, then ask, then allow rules, and a settings ask rule still prompts when a hook allows the call. An agent may not weaken its own permission settings, so this change keeps them, and every push still asks the owner. The owner decided on 2026-11-27 that the exact push and dispatch forms should run without asking; that takes effect only if the owner removes those ask rules himself, after which the hook is the only control for the allowed push forms. Until then this is an open owner action, not a fact.
- **Workflow changes are pushed by the owner.** When local `main` holds a commit that changes `.github/workflows/` and the owner has not pushed it, the hook asks for every agent push of `main`, GitHub would refuse it anyway, and the coordinator asks the owner to push.

### Untrusted GitHub content

Issues, pull requests, comments, review threads and fork commits of a public repository can be written by anyone. Agents treat everything there that the owner did not write in the session as data, never as instructions, as they treat web pages and tool output. Agents never check out, build or run a fork's branch. Decisions never live only in a GitHub thread: they land in the repository as `AGENTS.md` requires. No user's personal data goes into issues, pull requests, commits or logs; once a pilot exists, a bug report is restated without personal data before it reaches GitHub.

### Pull requests, issue form and labels

- The coordinator may open a pull request from a pushed candidate with `gh pr create` and the template; the hook asks the owner for that command. The candidate still merges by local fast-forward, and `main` is then pushed.
- `.github/pull_request_template.md`, `.github/ISSUE_TEMPLATE/change.yml` and `.github/ISSUE_TEMPLATE/config.yml`, which turns blank issues off, contain no comments.
- The labels are `kind:feat`, `kind:fix`, `kind:chore`, `kind:docs`, `tier:trivial`, `tier:standard`, `tier:slice`, `security-trigger` and `needs-owner`, listed in `tools/github-settings.json`.

### Data residency under ADR-0008

- **What leaves Russia and is public:** the code and its whole history, job logs and failure artifacts. This applies retroactively, because the history was pushed before this record.
- **What does not:** real users' personal data never enters the repository, issues, pull requests, logs or artifacts. Logs and artifacts hold synthetic test data and command output only, generated settings are masked, and no trace is uploaded.
- **Checks before publication:** the owner ran Gitleaks over all 431 commits of `main` and found no leaks, and a heuristic scan of `main`'s tree and of every blob reachable from it found no personal data beyond the owner's public login in paths and a third-party company's public support address. The [research addendum](../../knowledge/research/hosted-ci-2026-11-27.md#addendum-of-2026-11-27-verified-facts-and-personal-data-scans) records the scans and their limits.
- **Commit metadata** carries the owner's name and address, by the owner's choice.
- So ADR-0008, which governs where user data is stored and processed, is not breached: no user data is involved. Code hosting is development infrastructure, not OVC's hosting.

### The local settings wrapper

Code hosting does not end [ADR-0009](0009-owner-approved-exceptions-and-visibility.md) item 7, whose condition is OVC's own hosting. `tools/dev-env.sh` is unchanged. It runs on a runner only as the entries' wrapper, with no environment file to load; the settings come from `tools/ci-settings.sh`.

### What the second change must keep

The second change adds a `ci` mode to `tools/merge-gate.sh` that accepts a run only when all of these hold: the candidate is one commit on local `main`, and `origin/main` equals local `main`; the run's repository and head repository are this repository, its path is `.github/workflows/gate.yml`, its event `workflow_dispatch`, its head branch `main` at local `main`'s commit and its title exactly `gate <scope> <commit>`; the run completed at attempt 1 with `success`, with the job set and shard counts of `main`'s list and every job at attempt 1 with `success`. A fetch step writes the API's JSON to a folder and a verify step tested on synthetic fixtures reads it, so no test double is needed. A ruleset may then also require the `ovc-gate/full` status from the GitHub Actions app on `main`. That change also restates NFR-6.2 for CI from the median of the first runs, keeps the local gate as the recorded fallback when GitHub or its minutes are unavailable, leaves the laptop only the file-writing tools (formatters, `sqlc generate` and the API client generator), and solves the fixed Playwright shard set. It must also stop trusting the `needs` results as they stand: a job that ran candidate code can forge its own result, for example through the job's environment file, so the acceptance step must not rest on them alone. CI becomes the gate as soon as that change merges, with at least one shadow pair in its own verification.

## Alternatives considered

- **A throwaway virtual machine per gate in a Russian cloud** (proposal 0008, option A): keeps everything in Russia, but needs a cloud account, payment and a gate wrapper of its own; the owner preferred GitHub.
- **A private GitHub repository with hosted runners:** keeps the code private, but costs an estimated $300 to $510 a month at the current gate volume, payment from Russia is likely difficult, and Free cannot enforce rulesets on a private repository.
- **A private GitHub repository with a self-hosted runner:** no minutes to pay, but a persistent runner with Docker access can be persistently compromised by candidate code, and its logs still go abroad.
- **A push-triggered workflow:** simpler, but runs the pushed commit's own workflow file, so a change could edit its own checks.
- **Making CI the gate at once:** saves the laptop sooner, but no run has yet shown that CI and the local gate agree, and the acceptance step needs its own tested design.

## Consequences

- The source code, its history and every CI log are public and outside Russia. Future attackers of the pilot can read the code, so no control may rely on the code staying secret (T-49).
- The laptop still runs every check until the second change merges. Shadow runs add pushes and dispatches, and every push asks the owner while the ask rules stay.
- A pushed branch is public at once, so the preconditions of the shadow pair come before any push, and secret scanning with push protection is a second line behind them.
- Candidate code runs in `main`'s scope inside its job and can reach the rest of that job; nothing it writes there is trusted, which is why no cache is used and only the dispatch inputs and the `needs` results cross jobs. The `needs` results themselves come from jobs that ran candidate code, and the candidate can forge them, for example through the job's environment file; that is acceptable in shadow mode, where no result admits a merge, and the second change must address it (T-41, T-49).
- A branch workflow that raises its own token could push to GitHub's `main`, dispatch `main`'s workflow, post statuses, cancel runs or create tags. The agents' token without the Workflows permission, the hook's push preconditions, including its check that a pushed branch leaves `.github/` as `main` has it, the rulesets, local `main` staying authoritative and the second change's run checks bound this (T-49).
- GitHub outages or a suspension would cut CI and the remote; the local gate and every checkout's full history remain.
- Linux WebKit is not Safari on macOS, and hosted runners may expose deadline-sensitive tests. Differences are recorded in the shadow runs and fixed, never waived, and timeouts are never raised.

## Open owner actions

- Apply the repository and account settings above at the visibility switch, and run `tools/check-github-settings.sh` in his own terminal until it prints `settings match tools/github-settings.json`.
- At merge time, switch the Actions permissions from local actions only to selected actions, with exactly `actions/checkout`, `actions/setup-go`, `actions/setup-node` and `actions/upload-artifact` at the commits pinned in `.github/workflows/gate.yml` and full-hash pinning still required. Until then the repository allows local actions only, because GitHub refuses the selected mode with an empty list, and `tools/check-github-settings.sh` reports the mismatch.
- Review the installed GitHub Apps and authorized OAuth apps with access to the repository, which the settings check cannot read.
- Create the fine-grained token as described, and confirm that no SSH key with write access is configured.
- Create the labels with the commands in the [development guide](../development.md#labels).
- Push the commit of this change, and every later commit that changes `.github/workflows/`, with his own credential.
- Decide whether to remove the `git push` ask rules of `.claude/settings.json` himself, which the decision of 2026-11-27 that exact pushes run without asking needs; agents keep them.

## Where it is applied

- `.github/workflows/gate.yml`, `.github/pull_request_template.md`, `.github/ISSUE_TEMPLATE/`
- `tools/merge-gate.sh` (`list`, `run-job`, `classify` and the CI job table), `tools/check-merge-gate.sh`, `tools/ci-settings.sh`, `tools/ci-settings-test.sh`, `tools/ci-tools.sha256`, `tools/check-workflows.sh`, `tools/check-github-settings.sh`, `tools/github-settings.json`
- `.claude/settings.json`, `.claude/hooks/guard_secrets.py`
- [AGENTS.md, change workflow](../../AGENTS.md#change-workflow), [ovc-verify](../../.claude/skills/ovc-verify/SKILL.md), [ADR-0018](0018-targeted-checks-and-merge-gate.md#shadow-ci-on-github-hosted-runners)
- [Development guide, continuous integration](../development.md#continuous-integration-in-shadow-mode), [threat model](../threat-model.md), [stack table](../../knowledge/architecture/detailed.md#stack-per-component)
- [Shadow-run record](../../knowledge/analysis/ci-shadow-runs.md)
