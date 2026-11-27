# Task plan: adopt GitHub hosting and a shadow CI on GitHub-hosted runners

- Status: approved by the owner on 2026-11-27 (revision 8); implementation in progress. Revision 7 applied the owner's answers of 2026-11-27 (section 14), the main one being that the repository becomes public. Revision 8 resolves the review that followed those answers (section 13).
  - Revision 3 followed the owner's decision of 2026-11-27 to take checks off the laptop and run them on GitHub-hosted runners. Revision 4 resolves the standard and security planning reviews of revision 3, revision 5 resolves the re-review of revision 4, and revision 6 resolves the last re-review (section 13).
  - Revisions 1 and 2, with a self-hosted runner on the laptop, are superseded.
- Implementation deviation (the owner's decision of 2026-11-27): actionlint was dropped. Its latest release cannot be built beside the other pinned tools in `tools/go.mod`, so every mention of actionlint below (the lane B entry, the `backend-checks` entry, the merge-gate edit, the order of work and AC-3) describes the original design. `tools/check-workflows.sh` and GitHub's validation at dispatch check the workflow, and a known-limitations row tracks its return.
- Branch: `chore/github-ci-adoption`, one commit. On `main` it joins the list of the earliest sub-slice whose slice review has not started, as `AGENTS.md` requires for a commit in no list.
- Tier: **standard**.
- Security trigger: **met**, for these reasons:
  - agent configuration: `AGENTS.md`, `.claude/settings.json`, the guard hook and ovc-verify;
  - the verification policy: ADR-0018, and the gate script's new subcommands;
  - dependencies, including new CI tool sources;
  - an outbound network path;
  - code and test logs leave Russia.

  So it gets a standard reviewer and a fresh reviewer with the full security profile, both for this plan and at readiness.
- Related:
  - [proposal 0008](../../docs/proposals/0008-hosted-ci.md) and the [hosted CI research](../research/hosted-ci-2026-11-27.md);
  - [ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md), [ADR-0009](../../docs/decisions/0009-owner-approved-exceptions-and-visibility.md), [ADR-0011](../../docs/decisions/0011-tiered-verification.md), [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md);
  - [NFR-6](../../docs/requirements/non-functional.md#nfr-6-maintainability-and-verification);
  - [threat model](../../docs/threat-model.md): T-18, T-19, T-39, T-41;
  - the `AGENTS.md` bullet "When a hosted remote and CI are adopted, add the issue form, pull request template, labels and CI workflow in one change and amend this section."

## 1. Problem

The owner created a GitHub repository, `github.com/klukashka/ovc`. It was private at first, and the owner makes it public himself (Q1-B), without a license file, so all rights are reserved.

- **Settings first.** The owner applies the settings of section 3.6 when he switches visibility: fork approval, secret scanning and push protection, wiki, Discussions, Projects and Pages off, and the rulesets. He does not wait for this change to merge.
- **Before the switch,** `main`'s full history was scanned for personal data (section 5). Only `main` has been pushed, so no other branch becomes public. It is `origin`, and `main` is pushed there fast-forward only after every merge.

The owner wants the laptop to stop running checks: "разгрузим ноутбук и не будем на нем проверки запускать, будем делать всё через CI". Today the laptop runs:

- about 15 to 20 merge gates a day:
  - each full run takes about 640 to 740 s; the median is about 690 s in the gate log of 2026-11-26 and 2026-11-27;
  - each Markdown-only run takes about 15 s;
- every implementing agent's change checks;
- every after-fix evidence run.

ADR-0018's idle-machine rule stops all other work while a gate runs.

Nothing in the repository records the remote, a CI service, a pull request template, an issue form or labels. Until CI is live, local checks continue as now (the owner's choice).

## 2. Scope: three changes

The reviewers of revision 3 asked to split the work, and this plan does that.

1. **This change: shadow CI.** It covers:
   - ADR-0026;
   - the workflow running on GitHub-hosted runners;
   - two new read-only subcommands of the gate script that let CI run `main`'s check list;
   - the CI tool setup;
   - the agent permission rules for GitHub;
   - the templates, issue form and labels;
   - the threat model and documents.

   The local gate stays the only gate. The coordinator may dispatch CI beside the local gate on the same candidates and records both results.
2. **Phase 2, a separate plan, written right after this change:** the `ci` mode of `tools/merge-gate.sh`, which accepts a CI run for the exact candidate commit, the `finish` support for it, and the switch of the gate, change checks and evidence runs to CI.
   - The switch happens as soon as phase 2 merges (Q6-B). Phase 2's own verification includes at least one shadow pair as a smoke check.
   - Phase 2 also covers:
     - the laptop afterwards: only file-writing tools, meaning formatters, `sqlc generate` and the API client generator (Q3-A);
     - NFR-6.2 restated for CI from the median of the first runs (Q2-A);
     - the fallback to the local gate, recorded in each merge report (Q7-A). Its design constraints are fixed now, in section 3.9, so that the shadow runs already produce what phase 2 will check.
3. **The check list stays where it is.** The full and Markdown-only lists stay inline in `main`'s `tools/merge-gate.sh`, so the coordinator's procedure does not change. The only additions to the full list are two lane B entries: actionlint and `tools/check-workflows.sh`, which also runs its own self-test and that of `tools/check-github-settings.sh` (section 3.10).

## 3. Design

### 3.1 The harness always comes from `main`

ADR-0018's rule is that a change never runs its own gate. In CI it is kept like this:

- **Trigger.** The workflow is triggered only by `workflow_dispatch`, and its jobs refuse unless `github.ref == 'refs/heads/main'`. So the workflow file that runs is `main`'s.
- **Two checkouts in every job**, both with `actions/checkout` and `persist-credentials: false`:
  - `harness/` at `github.sha`, which is `main`'s tip `M`;
  - `candidate/` at `inputs.sha`, the candidate commit `C`.

  The first step checks that `git -C candidate rev-parse HEAD` equals `inputs.sha`.
- **Only harness code runs as harness.** Every harness script runs from `harness/`, with `--repo candidate`. The candidate's own `tools/`, `.github/` and hooks are never executed as harness. The candidate's code runs only as the checks run it, as in the local gate.
- **New subcommands of `harness/tools/merge-gate.sh`:**
  - `list [--scope full|docs] [--job NAME]` prints the inline list's entries with a CI job name for each.
  - `run-job NAME [--shard N/M] --repo PATH` runs one job's entries in list order and stops at the first failure.
    - It applies the gate's environment refusals and its clean-tree and ignored-file refusals to the candidate checkout.
    - It prints one result line per entry: entry, exit status and seconds.
    - It takes no lock. A hosted VM runs one job.
    - It runs its entries without `GITHUB_ENV`, `GITHUB_PATH`, `GITHUB_OUTPUT` and `GITHUB_STEP_SUMMARY` in their environment, so that a command cannot alter later steps by accident.
  - `classify --repo PATH --base REV` prints `full` or `docs-only`, by the gate's existing Markdown-only rule.

  Locally, nothing calls them. The gate's own lanes are unchanged.
- **The CI job table is data in the harness.** A table in `merge-gate.sh` gives each entry of the inline list:
  - its CI job;
  - its split rule: none, Go packages, or end-to-end projects.

  Rules for that table:
  - Every entry runs verbatim, `../tools/dev-env.sh` included. That is the candidate's copy, as in the local gate; on a runner it runs only as the entries' wrapper, with no environment file to load, and the settings come from `tools/ci-settings.sh`.
  - Only three rewrites exist:
    - the Go test entry gets the shard's package list in place of `./...`;
    - the end-to-end entry gets `--workers=2` and either `--project <p>` or `--project chromium --shard=k/4`;
    - the staged Gitleaks scan becomes a scan of `M..C` (section 3.2).
  - The `pre` entry, `tools/check-tool-binaries.sh`, runs first in every job that builds `bin/`: `backend-tests`, whose conventions test runs `bin/golangci-lint`, `backend-checks`, `frontend-checks` and `docs-checks`.
  - The table's test (AC-1) proves that, for each scope, the rewritten commands cover every entry of the inline list exactly once. `pre` is the one exception: it runs in each job that builds `bin/`.
- **Inside a job, candidate code can reach the rest of that job** (`harness/`, the step files), as it can reach the scratch copy of the script in the local gate. Only `prepare`'s outputs and the `needs` results cross jobs as trusted. T-41 and T-49 state this.
- **Each shard checks its own set.**
  - A Go shard fails if its package list differs from `prepare`'s assignment.
  - An end-to-end shard fails if the candidate's project list differs from the harness's shard set.

  The `gate` job only displays `prepare`'s assignment. Shard outputs are never treated as proof.
- **The push precondition uses `main`'s copy too** (section 3.7).

### 3.2 Workflow `.github/workflows/gate.yml`

- **Inputs, all validated in a first `validate` job before anything else runs:**
  - `sha` must match `^[0-9a-f]{40}$`;
  - `scope` is `type: choice` over `full` and `docs`.

  The `change` and `evidence` scopes belong to phase 2.

  Inputs reach scripts only through `env:`, never as `${{ }}` inside `run:`.
- **`run-name`** is `gate <scope> <sha>`, so the run's `display_title` names the candidate (section 3.9).
- **`prepare` job.** It runs from `harness/`. It reads `candidate/` with `git`, and runs `go list ./...` with `GOFLAGS=-mod=readonly` and `GOTOOLCHAIN=local` after `setup-go`. It outputs:
  - `C` and `T = C^{tree}`;
  - `M`, which is `github.sha`;
  - the classification;
  - the job set the scope needs;
  - the Go package assignment for the shards, which the shards take from it rather than computing their own lists.

  It fails with "candidate is not on main's tip", and nothing else runs, when `C`'s parent is not `M` or `C` is not exactly one commit on `M`.
- **Check jobs** run on `ubuntu-24.04`, the free standard runner of a public repository:

| Job | Entries from the inline list | Shards |
|---|---|---|
| `backend-tests` | `go test -race -count=1`, after `pre`, because the conventions test runs `bin/golangci-lint`, so this job builds `bin/` first. Packages are assigned at run time from `go list ./...`: `internal/api` gets its own shard and the rest are balanced deterministically by name. `prepare` checks that the union of its assignment equals `go list` exactly, with no package twice. | 3 |
| `backend-checks` | lane B's entries: the backend static checks, both Gitleaks scans, `tools/check-merge-gate.sh`, actionlint and `tools/check-workflows.sh`, which also runs its own self-test, that of `tools/check-github-settings.sh` and `tools/ci-settings-test.sh` | 1 |
| `frontend-checks` | lane C's entries: the frontend checks other than the end-to-end suites, the documentation checks, the OSV-Scanner scans, and the hook and wrapper self-tests | 1 |
| `e2e` | `pnpm e2e --max-failures=10 --trace=off --workers=2`, one shard each for the four Firefox groups and the four WebKit groups, plus four shards of the Chromium project with `--shard=k/4`, because it matches every spec. Each shard first lists the candidate's Playwright projects (`playwright test --list --reporter=json`) and fails if they differ from the harness's shard set, so a candidate that adds a project or group fails rather than going untested. | 12 |
| `docs-checks` | the Markdown-only list, for the `docs` scope. The list starts with `tools/check-tool-binaries.sh` and runs `bin/gitleaks`, so this job builds `bin/` first. | 1 |

- **The `gate` job.**
  - It depends on every job of the scope, checks out nothing and runs no candidate code.
  - It requires every `needs` result to be `success`, so no job may be skipped.
  - It writes `C`, `T`, `M`, the scope, the job set and `prepare`'s package assignment to the step summary.
  - It posts the commit status `ovc-gate/<scope>` on `C` for display. That status is never evidence (section 3.9).
- **Permissions.** `permissions: {}` at workflow level. Every job has `contents: read`, except `gate`, which has `statuses: write`. No job sets `continue-on-error`, and only `gate` uses `if:` result functions.
- **No Actions cache at all** (finding B1). Every dispatch runs under `refs/heads/main`, and the candidate's code can write `main`-scoped cache entries itself through the runtime token. So nothing is cached:
  - `setup-go` gets `cache: false`, and `setup-node` gets no cache option and its package-manager cache turned off;
  - `tools/check-workflows.sh` rejects any `actions/cache*` and any `setup-*` step without its cache explicitly off.

  Caching may return only through a later decision, for content that is verified on every restore.
- **Artifacts.**
  - On failure a job uploads only the redacted evidence the local gate keeps: the failing entry's log tail and `browser-diagnostics.json`, with no traces. The redaction reads the generated setting names from `CI_GENERATED_SETTING_NAMES`, which `tools/ci-settings.sh` exports, so the list has one source.
  - Artifact retention is 3 days, and log retention is the repository minimum.
  - Each job logs `ImageOS` and `ImageVersion`.
- **Gitleaks.** Jobs that run Gitleaks use `fetch-depth: 0` on `candidate/`, and `tools/check-workflows.sh` requires it.
  - CI scans the fetched refs, while the local gate scans every local branch.
  - The staged scan finds nothing on a clean checkout, so CI scans `M..C` with `--log-opts` instead.
  - The shadow notes record both differences.
- **Concurrency.** One full run uses 20 jobs, which is Free's limit of concurrent jobs.
  - The coordinator dispatches the next run only after the previous one has completed.
  - GitHub keeps only one pending run per concurrency group, and a newer one cancels it, so a group alone would not queue them.
  - A cancelled run is recorded as missing shadow data.
  - The research addendum checks this behaviour.

### 3.3 Tools on the runner (finding: CI setup)

Each tool has a source, a pin and a check, recorded in the stack table and under T-39:

| Tool | Source and pin |
|---|---|
| Go | `actions/setup-go` with `go-version-file` set to the candidate's `tools/go.mod` (its `toolchain` line) and `cache: false`. `tools/check-tool-binaries.sh` then compares it, as locally. |
| Node | `actions/setup-node` with `node-version-file` set to the candidate's `frontend/.node-version`, the pin ADR-0017 names, without caching. The candidate's pins are used, as in the local gate, where the change's own toolchain files apply. |
| pnpm | The corepack tarball of a pinned version is downloaded from the npm registry (`registry.npmjs.org/corepack/-/corepack-<version>.tgz`), checked against a SHA-512 in `tools/ci-tools.sha256`, which matches the registry's integrity value, and installed with `npm install -g <tarball>`. Then `corepack enable`, so pnpm's version and SHA-512 in `packageManager` are enforced. Whether Node 26 bundles corepack is checked in the addendum. |
| `bin/` tools | Built from `tools/go.mod` only in `backend-tests` (its conventions test runs `bin/golangci-lint`), `backend-checks`, `frontend-checks` and `docs-checks`, the jobs that run them. The `e2e` job needs none. |
| lychee 0.24.2 | Downloaded from its GitHub release (`github.com/lycheeverse/lychee/releases`), checked against a SHA-256 committed in `tools/ci-tools.sha256`. |
| Playwright browsers | `pnpm exec playwright install --with-deps` from the pinned `@playwright/test`. The Chromium of `tools/docs` comes from its pinned `playwright-core`. `--with-deps` runs `apt-get` with unpinned Ubuntu packages, a residual recorded under T-39. |
| Compose images | The digest-pinned images of `docker-compose.yml`, pulled per job. Docker Hub's pull limits on hosted runners are checked in the addendum. |

The `setup-*` actions must not export a setting that the gate refuses, such as `GOTOOLCHAIN` or `NPM_CONFIG_*`. `run-job` refuses them, so a mismatch fails visibly. The addendum checks this.

Three further points:

- **Which copy of the checksum file.** `tools/ci-tools.sha256` is read from `harness/`. A candidate that bumps a CI tool therefore fails its own run, as expected (section 3.5).
- **Release age.** The corepack and lychee versions and the action SHAs follow ADR-0017's release-age delay, like the other pinned tools.
- **The Go version.** The addendum checks whether `setup-go`'s `go-version-file` honours the `toolchain` line. If it does not, the step names the version from that line explicitly, so `GOTOOLCHAIN=local` in `prepare` does not fail and nothing downloads a toolchain silently.

### 3.4 Services and settings: no stored secrets

- **Services.** Jobs that need PostgreSQL or the S3-compatible server start the Compose file's own services with `docker compose up --wait`. GitHub's `services:` key cannot pass PostgreSQL its `command`.
- **Settings.** `harness/tools/ci-settings.sh` generates every `OVC_*` value the tests need, from both `.env.example` and `backend/.env.example`:
  - the database and storage credentials, and the signing and encryption keys, come from `openssl rand`;
  - the operator settings get synthetic values;
  - every value passes the start-up settings checks, including the `OVC_TEST_CONTROL` preconditions.
- **Masking.**
  - Each generated value and its URL and user-information forms (the database URL and `user:password`) are masked with `::add-mask::` before anything prints it; other encodings of a value are not masked.
  - The script exports `CI_GENERATED_SETTING_NAMES`, which both redaction steps read, and `tools/ci-settings-test.sh` checks the generator against the real example files.
  - The values are written to the job's environment file.
  - The script never uses `set -x`, and the settings check confirms that no repository variable turns on step debugging.
- **No stored secrets.** These are throwaway test values that exist only in that VM, so the repository stores no secrets, variables or environments.
- **Unchanged.** `tools/dev-env.sh` and ADR-0009 item 7 are unchanged. On a runner `tools/dev-env.sh` runs only as the entries' wrapper, with no environment file to load. ADR-0026 states that code hosting does not end item 7, whose condition is OVC's own hosting.

### 3.5 Shadow mode

- **Who dispatches.** Only the coordinator dispatches. `AGENTS.md` says so, and the guard hook asks for any dispatch outside the exact allowed form (section 3.7).
- **The candidate is always one commit on `M`.** When `main` has moved, the coordinator first builds a one-commit branch on `M` with `git switch -c` and `git merge --squash`, then commits, as the squash practice already does. The row records that the local tested tree equals `C^{tree}`. Phase 2 keeps the same rule.
- **Expected failures.** The row records each of these as expected, not as a disagreement:
  - `ci-settings.sh` comes from `main`, so a candidate that adds a required `OVC_*` setting fails its own shadow run, and that change updates the generator;
  - a candidate that bumps a pin in `tools/ci-tools.sha256` fails the same way;
  - a candidate that adds a Playwright project or spec group fails against `main`'s shard set, while the local gate runs every project. Phase 2 must solve this (section 3.9).
- **The shadow pair.** For the one-commit candidate it is about to gate locally, the coordinator:
  1. checks that `origin/main` equals local `main`; otherwise it records the run as not dispatched;
  2. checks that the candidate's change checks and readiness reviews passed on exactly this tree, because a pushed branch is public at once and cannot be reliably withdrawn;
  3. pushes the candidate branch, which the hook allows only after a Gitleaks scan of `main..<b>` (section 3.7);
  4. dispatches `scope=full` (or `docs`);
  5. gates locally with `--ff-only` as today;
  6. records both results in `knowledge/analysis/ci-shadow-runs.md`.
- **Each row records:**
  - the candidate and its tree;
  - the local result and every failing test;
  - the run address and its result, with every failing test;
  - the wall time and summed job minutes;
  - the image version.
- **No minimum count of pairs in this phase.** Under Q6-B, phase 1 needs no run of consecutive agreeing pairs. Shadow runs are optional measurements, and phase 2's verification includes at least one.
- **No CI result admits or blocks a merge in this phase.** Phase 2 adds the record kind that `finish` accepts. Until then no code path accepts one, so shadow mode needs no enforcement flag (finding M5).
- **Cost control.** The public repository's standard runners are free, and the account's spending limit is $0 (section 3.6), so no run can incur a charge. The coordinator still dispatches one run at a time.

### 3.6 Repository and account settings, applied by the owner in the browser

The repository is public (Q1-B). That brings these consequences:

- **Free standard runners.** Public repositories use GitHub-hosted standard runners free. They are larger than the private 2-core runner; the size is checked in the addendum.
- **Public logs.** Logs and artifacts are readable by anyone (artifacts by any signed-in user).
- **Forks always allowed.** Anyone can fork the repository and open pull requests. For a public repository, forking cannot be turned off.
- **Rulesets.** Rulesets and required status checks are available on Free for public repositories.

`tools/check-github-settings.sh` compares the repository's settings with the committed `tools/github-settings.json`, which also holds the label set and the allowed action SHAs.

- The script reads only through `gh api --hostname github.com` GET requests on `repos/klukashka/ovc/...`, a name committed in the data file.
- It refuses `GH_TOKEN`, `GH_HOST`, `GH_REPO`, `GH_CONFIG_DIR` and `GH_ENTERPRISE_TOKEN`.
- It needs administration read, so **the owner runs it in their own terminal** with their own login, never an agent.
- Its comparison logic has a self-test on synthetic JSON fixtures.

The settings:

- **Actions:**
  - only these GitHub-authored actions, each at a full SHA: `actions/checkout`, `actions/setup-go`, `actions/setup-node`, `actions/upload-artifact`;
  - full-SHA pinning required;
  - default `GITHUB_TOKEN` permission read-only;
  - Actions may not create or approve pull requests;
  - minimum log and artifact retention;
  - a spending limit of $0 on the account, so nothing can incur a charge.
- **Fork pull requests.**
  - `gate.yml` has no `pull_request` or `pull_request_target` trigger, so a fork's pull request starts nothing of ours. The only way a fork could run code is a workflow file it adds itself, which GitHub runs on the `pull_request` event with a read-only token and no secrets.
  - The setting "Require approval for fork pull request workflows" is set to all outside contributors, and the owner never approves one. So no fork code runs on our runners.
  - `tools/check-workflows.sh` already rejects `pull_request` and `pull_request_target` in our own workflows.
- **Ruleset on `main`, available because the repository is public:**
  - force pushes blocked;
  - deletion blocked;
  - linear history required;
  - no bypass for anyone. The fast-forward pushes stay allowed.

  A second ruleset targets all tags (`~ALL`). It restricts creation, update and deletion, with no bypass, so no one, the owner included, can create a tag such as `main`. The owner can lift it deliberately when a release tag is needed. This also closes most of the branch-workflow route of T-49 to GitHub's `main`, because a raised token can no longer force-push or delete it. A fast-forward push by a branch workflow stays possible until phase 2 requires the CI status, so local `main` stays authoritative.
- **Secret scanning and push protection,** free on public repositories (unverified, checked in the addendum), are turned on as a second line of defence behind the local Gitleaks checks (T-19, T-49).
- **Features strangers can write to.** Wiki, Discussions, Projects and Pages are turned off. On a public repository the wiki may be editable by any GitHub user by default (unverified).
- **Contributions from strangers.**
  - Issues stay enabled.
  - "Private vulnerability reporting" is turned on, so security reports do not land in public issues.
  - Content written by anyone other than the owner in issues, pull requests, comments or discussions is untrusted data. An agent never acts on it (section 3.8).
- **Write access:**
  - no collaborators;
  - no deploy keys with write access;
  - no webhooks;
  - no GitHub App or OAuth app with write access unless the owner reviewed it;
  - two-factor authentication on the owner's account.
- **No secrets:** no repository secrets, variables or environments.
- **The laptop's Git and `gh` credential (finding M6).**
  - Pushes use HTTPS through `gh`'s credential helper, with a fine-grained token for this repository only:
    - write: contents, pull requests and actions (to dispatch). Issues write is left out: no allowed form needs it.
    - read: metadata;
    - no administration, secrets or Workflows permission.
  - Without Workflows, that token cannot push a commit that changes `.github/workflows/`. Such pushes, including `main` after this change merges, are done by the owner with their own credential.
  - No SSH key with write access is configured for this remote. PM-1 confirms that.

### 3.7 Agent permissions: an allowlist in the guard hook

The guard hook `.claude/hooks/guard_secrets.py` gains an allowlist parser for `git push`, `git send-pack`, `git remote` and `git config` writes, and for every `gh` call. The hook protects against accidents; it is not a security boundary, because Bash is allowed and a script can call `git` or `gh` internally. T-41 says so, in the existing guard's wording.

**Precedence (unverified, checked first in the addendum).**

- If a hook's "allow" overrides a `settings.json` ask rule, the ask rules stay, and the hook allows exactly the forms below.
- If it does not, but a hook can return "ask", the `git push` ask rules are replaced by the hook alone, which asks for everything but the allowed forms. The hook is then the only control for those forms, and T-41 records that.
- If neither holds, the owner's Q5-A answer cannot be implemented: every push and dispatch asks the owner, and the readiness report says so.

- **Allowed without asking (Q5-A).** An allowed form must be the whole command, with no `cd`, `&&`, `;` or `|`:
  - exactly `git push origin refs/heads/<b>:refs/heads/<b>`, where `<b>` matches `^(feat|fix|chore|docs)/[a-z0-9-]+$`. All of these must hold:
    - `<b>` is one commit on local `main`;
    - `git rev-parse <b>:.github` equals `git rev-parse main:.github`.

    - `bin/gitleaks git --redact --no-banner --config <main's .gitleaks.toml> --log-opts=main..<b> .` passes, with the configuration written from `git show refs/heads/main:.gitleaks.toml` to a temporary file and the main checkout's `bin/gitleaks` when the hook finds the main worktree, else the current checkout's. If it fails, times out after 45 seconds, or `bin/gitleaks` is missing, the hook asks the owner.

    Apart from that scan, which takes seconds and is read-only, the hook runs only `git rev-parse`, `rev-list`, `for-each-ref` and `show` reads.
  - exactly `git push origin refs/heads/main:refs/heads/main`. Both of these must hold:
    - `origin/main` is an ancestor of local `main`;
    - no commit in `origin/main..main` changes `.github/`.

    Today this push asks the owner every time. Allowing it reduces the "never push" rule, so `AGENTS.md` records it explicitly, as part of Q5-A.
  - exactly `gh workflow run gate.yml --ref main` with `-f sha=<40 hex>`, `-f scope=<choice>` and the pattern-checked inputs, when `sha` is the tip of a local `refs/heads/(feat|fix|chore|docs)/…` branch whose only parent is local `main`'s tip;
  - `gh run view|list|watch`;
  - `gh pr view` and `gh pr list` only with `--json` fields that exclude bodies and comments (number, state, head, URL);
  - `gh api` only with a path under `repos/klukashka/ovc/` from the set phase 1 needs: `actions/runs…`, `actions/jobs…` and `commits/<sha>/status` or `statuses`. These are excluded:
    - `--input`, `-f`, `-F`, `--field` and `--raw-field`;
    - `-X` and `--method` other than `GET`;
    - a full URL, `graphql` and `--hostname`;
    - `-H` and `--header`, other than `Accept`.

    `--input` alone switches `gh api` to POST, which is why it is excluded.
- **Asked of the owner:** every other form, and any parse failure or indirection. That includes:
  - `git -c`, aliases, `env`, `command`, absolute paths to `git` or `gh`, `bash -c`, `eval`, `xargs` and `$( )`;
  - `GIT_*` and `GH_*` prefixes, `-R` and `--repo`;
  - repeated `--ref`;
  - `-X` or `--method` with any value other than `GET`, and `--field`/`--raw-field` forms;
  - `+refspec`, deletes, `--mirror`, `--all`, `--tags`, `--receive-pack` and `--exec`;
  - `git remote` and `git config` writes;
  - `gh alias`, `gh extension`, `gh gist`, `gh repo`, `gh secret`, `gh variable`, `gh label`, `gh release`, `gh ssh-key`, `gh gpg-key` and `gh cache`;
  - `gh workflow enable|disable`, `gh run rerun|cancel|delete`, and `gh pr create|edit|merge|close|comment`.

  If the hook cannot return "ask" (unverified, checked in the addendum), it blocks instead, and the owner runs the command.
- **Which calls the hook decides.** A call is relevant only when `git` with a push-family subcommand or alias, a `git-push`-family program or `gh` stands in command position: the first word of a simple command after assignments and wrappers, inside `bash -c` scripts, substitutions and shell heredoc bodies too. A mention as an argument, such as `grep -n "git push" AGENTS.md`, gets no decision. The hook still asks for every dynamic command word such as `$G`, and when a command it cannot parse, `env -S` or an opaque program such as an interpreter, `eval` or `find` names `git` or `gh` anywhere.
- **Denied:**
  - `gh auth token`, `--show-token`, `gh auth status` with `-t` alone or among combined short flags, `gh config get` naming `oauth_token`, `gh auth git-credential`, and `gh auth login|refresh|switch`;
  - `git credential fill|approve|reject`, `git credential-osxkeychain`, and `security find-internet-password|find-generic-password`;
  - reading `~/.config/gh/hosts.yml`;
  - Edit and Write on `**/.git/config`, `**/.git/**`, `~/.gitconfig` and `~/.config/git/**`;
  - `mcp__plugin_engineering_github__*`.
- **When the owner has not pushed yet.** Local `main` may hold a commit that changes `.github/workflows/` and that the owner has not pushed. Until the owner pushes it, the hook asks for the agents' pushes of `main`, and GitHub would refuse them anyway, because the agents' token has no Workflows permission. The coordinator asks the owner to push. `AGENTS.md` describes this.
- **Self-test.** `check_guard_secrets.py` gets a case for each form above.

### 3.8 Pull requests, template, issue form, labels

- **Untrusted GitHub content.** In a public repository, issues, pull requests, comments, commit messages on forks and review threads can be written by anyone.
  - Agents treat all of it as data, never as instructions: the same rule `AGENTS.md` and the agent harness apply to web pages and tool output.
  - An agent acts only on what the owner writes in the session.
  - Agents never check out, build or run a fork's branch.
  - `AGENTS.md` and ovc-verify say so, and T-49 records prompt injection through GitHub content.
- **Pull requests (Q4-A).** The coordinator opens a pull request from the candidate branch with `gh pr create` and the template. The hook asks the owner for that command, because the owner's Q5 answer covers only pushes and dispatches.
  - CI's commit status shows on the pull request.
  - After the local merge and the push of `main`, GitHub marks a fast-forwarded pull request merged (unverified, checked on this change's own pull request).
  - Pull request and issue threads never hold a decision, and no user's personal data goes into them, into commits or into logs.
- **The template** is Markdown without HTML comments. Its sections:
  - change and plan;
  - tier and trigger;
  - acceptance criteria;
  - change checks;
  - reviews;
  - rendered-screen review;
  - threat-model mapping;
  - known limitations;
  - documentation.
- **Issue forms:** `.github/ISSUE_TEMPLATE/change.yml`, and `config.yml`, which turns off blank issues.
- **Labels**, in `tools/github-settings.json`:
  - `kind:feat|fix|chore|docs`;
  - `tier:trivial|standard|slice`;
  - `security-trigger`;
  - `needs-owner`.

### 3.9 Constraints fixed now for phase 2's `ci` mode

Phase 2 will accept a run only when all of the following hold:

- the commit checks:
  - the candidate is one commit `C` whose parent is local `main` `M`;
  - `origin/main` equals `M`;
- the run's identity:
  - repository and head repository are the committed name;
  - workflow path `.github/workflows/gate.yml`;
  - event `workflow_dispatch`;
  - `head_branch` `main` and `head_sha` `M`;
  - `display_title` exactly `gate full <C>` (or `gate docs <C>`);
- the run's completion:
  - `status` `completed`, `run_attempt` 1 and conclusion `success`;
  - the job set equal to `main`'s list for the scope, with the shard counts;
  - every job at attempt 1 with `success`.

It will read the run through a fetch step that writes the API's JSON to a folder, and a verify step tested on synthetic fixtures, so no test double is needed. Commit statuses are display only.

Because the repository is public, phase 2 can also enforce the result on GitHub with a ruleset on `main`:

- the required status check is `ovc-gate/full` from the GitHub Actions app (not any app), so a status posted by a branch workflow with a raised token does not count;
- `C` is pushed and its run completes before the fast-forward push of `main` to `C`.

Whether a ruleset's required check applies to direct pushes, and how it binds to the posting app, is checked in phase 2's research.

Phase 2 must also solve one more problem: a candidate that adds a Playwright project or spec group fails against `main`'s fixed shard set, while the local gate runs every project. For example, the shard set could be derived from the candidate's configuration, with a bound on the number of shards.

The shadow runs record these fields, so phase 2 can be built against real data.

### 3.10 Checks of the configuration

- **`tools/check-workflows.sh`** is one entry in lane B and in `backend-checks`. It first runs its own self-test, `tools/check-workflows-test.sh`, and the self-test of `tools/check-github-settings.sh`. It fails on any of these:
  - a workflow file other than `gate.yml`, or a `*.yaml` file;
  - a `uses:` that is not pinned to a full SHA, or that names an action outside the allowlist;
  - a job-level `uses:` (a reusable workflow) or a `docker://` action;
  - a trigger other than `workflow_dispatch`;
  - a missing top-level `permissions`;
  - a write permission on any job other than `gate`, or `id-token`;
  - a `runs-on` other than `ubuntu-24.04`;
  - `container:` or `services:`;
  - `continue-on-error`;
  - `if:` with `always()` or `failure()` outside `gate`, or a `gate` that does not require every `needs` result to be `success`;
  - `${{ inputs.* }}` or `${{ github.event.* }}` inside `run:`;
  - `secrets.` or `pull_request_target`;
  - any cache use, or a `setup-*` step without its cache off;
  - a Gitleaks job without `fetch-depth: 0`;
  - a job that runs a script from `candidate/tools/` rather than `harness/`;
  - a comment in any file under `.github/`, the issue forms and `config.yml` included;
  - `.github/dependabot.yml`.
- **actionlint was dropped during implementation** (the owner's decision of 2026-11-27): its latest release, 1.7.12, needs `go.yaml.in/yaml/v4` v4.0.0-rc.3, while another pinned tool requires rc.6, and the shared module cannot build it. `tools/check-workflows.sh` and GitHub's own validation at dispatch cover the workflow, and a known-limitations row tracks its return. The text below describes the original design.
- **actionlint** runs with `-shellcheck=`, so local and CI results do not depend on whether shellcheck is installed. It enters through `go get -tool` and `go mod tidy` in `tools/`, with three related edits:
  - `check actionlint …` in `tools/check-tool-binaries.sh`;
  - the development guide's build command;
  - the stack table.

  After the merge, every worktree and the gating checkout rebuild `bin/` before the next gate.
- **`gh` and lychee's CI source** are recorded in the stack table.

## 4. Minute budget (estimate; measured by the shadow runs)

Under Q1-B the repository is public, so standard GitHub-hosted runners cost nothing and the figures below no longer decide anything. They are kept as the estimate of how much runner time one gate uses, and to compare with the shadow runs. The public standard runner is larger than the 2-core private one, so the wall-time estimate below is an upper bound. Free's limit of 20 concurrent jobs still applies.

These facts come from the [research](../research/hosted-ci-2026-11-27.md#2-github-private-repositories-and-github-actions), checked 2026-11-27:

- Free includes $12 of Actions compute a month for private repositories, about 2,000 minutes on the 2-core Linux runner at $0.006 a minute.
- Pro includes $18.
- Jobs are billed per started minute.
- Public repositories use standard runners free.

**Assumptions:**

- A runner vCPU is 2 to 3 times slower than a laptop core under the end-to-end stack's contention.
- The laptop's end-to-end phase, about 410 s on 6 workers, is about 2,200 worker-seconds.
- There is no cache, so every job builds cold.

| Job group | Billed minutes per full gate (estimate) |
|---|---|
| `e2e`: 12 shards; tests about 37 to 55 min, plus 12 × about 6 to 9 min of cold setup (checkout, Go modules and build, toolchains, pnpm install, `--with-deps` packages, browsers, image pulls, backend binaries, frontend build) | 109 to 163 |
| `backend-tests`: 3 shards, each building `bin/` cold (about 5 min), then compiling and running under `-race` cold | 55 to 70 |
| `backend-checks`: building `bin/` cold, about 5 min, then lint, vet, govulncheck and sqlc | 15 to 20 |
| `frontend-checks` | 10 to 15 |
| `validate`, `prepare`, `gate` | 3 |
| **Full gate** | **about 195 to 275** |
| Markdown-only gate: `docs-checks`, with about 5 minutes to build `bin/` cold, plus the documentation checks and Gitleaks | about 8 to 12 |

**Per month.** With 10 to 12 full gates a day over 22 working days, plus change-check and evidence runs in phase 2 (roughly +30 %), the runner time is about **50,000 to 85,000 job minutes**. That is free on a public repository.

**Wall time** is set by the slowest shard. The `internal/api` package alone, under `-race` on 2 vCPUs, may take 15 to 25 minutes, and less on the public runner. So a full run is estimated at **at most 15 to 25 minutes**. Under Q2-A, phase 2 restates NFR-6.2 for CI from the median of the first runs.

**For the record:** on a private repository this would have cost about $300 to $510 a month, and payment from Russia was likely difficult. That led to Q1-B.

## 5. ADR-0008 analysis, recorded in ADR-0026

- **What leaves Russia, and is public:** the code and its history, job logs and failure artifacts. This applies retroactively, because the history was pushed before this record. The repository has no license file, so all rights are reserved.
- **Pre-publication check.** Before the repository was made public, the owner ran Gitleaks over every commit of `main` and found no leaks. A heuristic scan of `main`'s files on 2026-11-27 found no personal data beyond:
  - the owner's macOS user name in absolute paths in review records, which is the same as the public GitHub login;
  - a third-party company's public support address in a research record.

  The research addendum records the scan and its limits.
- **History scan.** On 2026-11-27 the same heuristic ran over every blob reachable from `main`: 7,595 text blobs. It looked for email domains outside reserved test domains, Russian mobile numbers, document numbers and home-folder paths. It found:
  - one public third-party support address;
  - no phone or document numbers;
  - the home folder `klukashka`, the public GitHub login, in 16 paths;
  - a placeholder `someone` in 1 path.

  Only `main` has been pushed. A heuristic misses free-form personal data such as names in prose, and the addendum says so.
- **Logs and artifacts** hold synthetic test data and command output only. There are no traces.
- **Commit metadata** carries the owner's name and address, by the owner's choice. A `noreply` address is available.
- **Real users' personal data** never enters the repository, issues, pull requests, logs or artifacts. Once a pilot exists, a bug report is restated without personal data before it reaches GitHub.
- **A dated audit** in the research addendum checks that the fixtures are synthetic. It greps for email domains other than reserved test domains, and for phone-number patterns.
- **Public logs.** Generated settings are masked before anything prints them. The job uploads no traces, and our own failure artifacts hold only the gate's redacted evidence with synthetic data, kept 3 days. Candidate code could upload its own artifacts through the runtime token, as it could write caches. A candidate is pushed only after its change checks, readiness reviews and Gitleaks (section 3.5), and T-49 records that residual.

## 6. Threat model

- **New trust boundary 11,** "GitHub ↔ repository, CI runners and the coordinator's laptop".
- **New T-49, "CI and code hosting".** It covers:
  - account or token compromise;
  - candidate code that nobody has reviewed running in `main`'s scope, so nothing it writes there is trusted, and why no cache is used;
  - branch workflows that raise their own token and can:
    - push to GitHub's `main`;
    - dispatch `main`'s workflow on it;
    - post statuses;
    - cancel runs;
    - create tags.

    The controls are: no Workflows permission on the agents' token, the hook's push precondition, local `main` authoritative, and phase 2's checks on the run;
  - commit statuses that are display only;
  - pinned actions;
  - logs and artifacts outside Russia, and public;
  - suspension;
  - public visibility:
    - fork pull requests, whose workflows need approval that is never given;
    - abuse of free runners through such workflows;
    - spam;
    - prompt injection through issues, pull requests and comments, which agents treat as data only;
    - reconnaissance of the source code by future attackers of the pilot, which nothing relies on staying secret;
  - the `main` and tag rulesets.
- **T-18** is amended for logs and artifacts.
- **T-19** is amended for the generated, masked settings, the fine-grained token, the denial of `gh auth token` and the absence of stored secrets.
- **T-39** is amended for the actions, the runner image, the unpinned `apt` packages of `--with-deps`, Playwright's browser downloads, lychee's checksum, the corepack route and actionlint.
- **T-41** is amended for:
  - the harness checkout from `main`;
  - dispatch only from `main`;
  - the allowlist hook and the `.git` denials;
  - the residual that the coordinator's laptop holds the bookkeeping, as today.

## 7. Files

- **New:**
  - `docs/decisions/0026-github-hosting-and-ci.md`
  - `.github/workflows/gate.yml`
  - `.github/pull_request_template.md`
  - `.github/ISSUE_TEMPLATE/change.yml` and `config.yml`
  - `tools/ci-settings.sh` and `tools/ci-settings-test.sh`
  - `tools/ci-tools.sha256`
  - `tools/check-workflows.sh` and `tools/check-workflows-test.sh`
  - `tools/check-github-settings.sh`, `tools/github-settings.json` and their fixtures under `tools/testdata/github/`
  - `knowledge/analysis/ci-shadow-runs.md`, with its table header
- **Changed:**
  - `tools/merge-gate.sh`: the `list`, `run-job` and `classify` subcommands, the CI job table, and the two lane B entries (actionlint and `tools/check-workflows.sh`)
  - `tools/check-merge-gate.sh`
  - `tools/check-tool-binaries.sh`
  - `tools/go.mod` and `tools/go.sum`
  - `.claude/settings.json`
  - `.claude/hooks/guard_secrets.py` and `check_guard_secrets.py`
  - `.claude/skills/ovc-verify/SKILL.md`: shadow mode, and that only the coordinator dispatches
  - `AGENTS.md`: the change workflow, the remote, pushes and dispatch, decisions never in GitHub threads, and the last bullet replaced
  - ADR-0018: a section on shadow CI pointing to ADR-0026
  - `docs/decisions/README.md`
  - `docs/proposals/0008-hosted-ci.md` and `docs/proposals/README.md`
  - `docs/development.md`: the CI section, tool sources, labels, change-check rows for `.github/` and the new scripts
  - `docs/threat-model.md`
  - `knowledge/architecture/detailed.md`: the stack table
  - `knowledge/research/hosted-ci-2026-11-27.md`: a dated addendum. Each fact gets a source and a date:
    - the `workflow_dispatch` rules (default branch, `github.ref`, `run-name`, `display_title`);
    - the run and job API fields;
    - whether a workflow's `permissions:` can exceed the default setting;
    - the runtime token and cache writes;
    - whether `setup-*` caches by default and what it exports;
    - whether Node 26 bundles corepack;
    - the Docker Hub limits;
    - retention limits;
    - spending-limit behaviour;
    - fork settings;
    - whether a pull request is marked merged after a fast-forward push;
    - the fine-grained token's Workflows permission;
    - whether a Claude Code hook can return "ask";
    - the synthetic-data audit.
- **Unchanged:** no backend or frontend code, no Compose file, `tools/dev-env.sh`, and ADR-0009.

Every changed sentence in `docs/` is marked pending owner review.

## 8. Acceptance criteria (before the merge)

- **AC-1.** `bash tools/check-merge-gate.sh` passes. It shows these things, with synthetic repositories created by the test:
  - `list --scope full` prints exactly the entries of the gate's full list, and `--scope docs` those of the Markdown-only list;
  - `classify` agrees with the gate's Markdown-only rule on the existing cases;
  - `run-job` on a candidate that edits `tools/merge-gate.sh` still runs the harness's entries, not the candidate's;
  - `run-job` refuses an unclean candidate and each refused setting;
  - for each scope, the CI job table's rewritten commands cover every entry of the inline list exactly once, except `pre`, which runs in each job that builds `bin/`;
  - the end-to-end shard set covers every project of the current configuration.
- **AC-2.** `bash tools/check-workflows-test.sh` fails every forbidden form of section 3.10 on a synthetic file and passes the committed workflow. `bash tools/check-workflows.sh` exits 0.
- **AC-3.** `tools/check-tool-binaries.sh` prints `bin/ matches tools/go.mod` (actionlint was dropped, section 3.10).
- **AC-4.** `python3 .claude/hooks/check_guard_secrets.py` passes with a case for every form of section 3.7, including a branch push refused when Gitleaks fails and when `bin/gitleaks` is missing.
- **AC-5.** The self-test of `tools/check-github-settings.sh` passes on its fixtures: one match and one mismatch per setting group.
- **AC-6.** `tools/docs/check-docs.sh` exits 0. Gitleaks over staged changes and over history reports no leaks. OSV-Scanner over `tools/go.mod` exits 0.
- **AC-7.** These greps return their counts:
  - `grep -c 'local only' AGENTS.md` prints 0;
  - `grep -c 'shadow' AGENTS.md .claude/skills/ovc-verify/SKILL.md docs/development.md` prints at least 1 per file.
- **AC-8.** The local merge gate of this change passes.
- **AC-9.** These structural checks pass:
  - `grep -c '^| T-49 ' docs/threat-model.md` prints 1, and the threat model names trust boundary 11;
  - `docs/decisions/0026-github-hosting-and-ci.md` exists and `docs/decisions/README.md` lists it;
  - a `jq` query on `tools/github-settings.json` finds the fork-approval, secret-scanning, push-protection, wiki, Discussions, Projects, Pages and both ruleset fields;
  - the research addendum records the history scan of section 5 and its limits;
  - `.gitignore` lists `.agents/` and `.codex/`, so an accidental `git add -A` cannot stage and publish them.

AC-8 is run by `main`'s current script. Under ADR-0018's gate-change rule, the new entries and subcommands are shown first by later runs (section 9).

## 9. After the merge

These are recorded in `knowledge/reviews/github-ci-adoption-first-run.md` and `knowledge/analysis/ci-shadow-runs.md`. A fault reopens this change under the after-fix rule.

- **PM-1.** The owner has applied section 3.6 at the visibility switch, including the rulesets, fork approval, secret scanning and push protection, and the disabled features. In the owner's terminal, `tools/check-github-settings.sh` prints `settings match tools/github-settings.json`, and the token type and the absence of a write SSH key are confirmed. At merge time the owner switches the Actions permissions from local actions only, which GitHub requires while the selected list would be empty, to selected actions with exactly `actions/checkout`, `actions/setup-go`, `actions/setup-node` and `actions/upload-artifact` at the commits pinned in `gate.yml`, with full-hash pinning still required; the settings check then matches. The owner also reviews installed GitHub Apps and authorized OAuth apps by hand, which the settings check cannot read.
- **PM-2.** The first later full local gate quotes the result line of `tools/check-workflows.sh`.
- **PM-3.** The first dispatched full run names every job and its result. Its summary shows `C`, `T` and `M`, and the package-union proof.
- **PM-4.** A controlled test, done by the owner because it pushes a workflow:
  - a branch carries a `push`-triggered workflow that asks for `contents: write` and `statuses: write`;
  - it posts a forged `ovc-gate/full` status on a candidate, with a `target_url` that points at a real run;
  - the record shows that phase 2's constraints of section 3.9 would refuse it;
  - afterwards the push of `main` still fast-forwards, and the branch is deleted.
- **PM-5.** At least one shadow pair is recorded, with minutes, wall time and image version. Under Q6-B no longer series is required before phase 2.
- **PM-6.** In a live session, one allowed push and one asked form are run, showing that Claude Code honours the hook's decisions. AC-4 proves only what the hook decides.

## 10. Order of work

1. The research addendum, verified first. If the dispatch, run fields, token behaviour or hook "ask" output differ from this plan, stop and ask.
2. ADR-0026 and the threat model.
3. `list`, `run-job` and `classify`, with their tests (AC-1).
4. `tools/check-workflows.sh` and its test (AC-2), and actionlint (AC-3).
5. The workflow, `tools/ci-settings.sh` and `tools/ci-tools.sha256`.
6. The hook allowlist and the settings (AC-4).
7. The settings data, its check and fixtures (AC-5); the template and the issue form.
8. The amendments to `AGENTS.md`, ovc-verify, ADR-0018, the development guide, the proposal and the indexes (AC-6, AC-7).
9. The change checks, as the full local run in a coordinator slot, because `tools/merge-gate.sh`, `tools/go.mod` and `.claude/` change. Then the standard and security readiness reviews, and one commit.
10. Local gate, merge (AC-8), and the owner's push of `main`, because the commit adds `.github/workflows/`. Then PM-1 to PM-6.

## 11. Risks

- **Public by mistake.** A pushed branch is public at once. The preconditions of sections 3.5 and 3.7 come first, and push protection adds a second line of defence behind them.
- **WebKit on Linux is not Safari on macOS.** Engine-only differences are recorded in the shadow runs, never waived.
- **Deadline-sensitive tests on hosted runners.** Backend and end-to-end tests with deadlines may fail under contention. Timeouts are never raised, so a shadow failure is a bug to fix. This may delay phase 2.
- **Wall time** of up to 15 to 25 minutes, set by `internal/api`. Phase 2 restates NFR-6.2 from the measured median.
- **Docker Hub limits and image or apt drift** on the runners.
- **The laptop's bookkeeping stays trusted** (T-41).
- **GitHub outages or suspension** would cut CI. The local gate stays available.

## 12. Observability

Not applicable: no runtime component changes. The CI jobs print the gate's own result lines, and `run-job` adds only its per-entry result line.

## 13. Planning review findings of revision 3 and their resolutions

Security reviewer:

| Finding | Severity | Resolution |
|---|---|---|
| B1: cache poisoning through `main`'s scope | blocker | No cache at all; `setup-*` caches off; check rule (sections 3.2, 3.10), T-49 |
| M1: the harness came from the candidate | major | Two checkouts; harness scripts only from `main`; AC-1 case (section 3.1) |
| M2: the run was not tied to `C` | major | `display_title`, input validation, `env:` only, phase 2 constraints (sections 3.2, 3.9), PM-4 |
| M3: the permission rules could not be implemented | major | Hook allowlist with ask or block, `.git` denials, token-print denials (section 3.7) |
| M4: the branch-workflow chain | major | No Workflows permission on the agents' token; mechanical push precondition; T-49; PM-4 |
| M5: shadow mode was not enforced | major | `ci` mode moved to phase 2; no accepting code path exists now |
| M6: the push credential | major | HTTPS with a fine-grained token, no Workflows permission, the owner pushes workflow changes, PM-1 |
| m1: missed workflow forms | minor | Section 3.10 |
| m2: masking | minor | Section 3.4 |
| m3: runner residuals | minor | Section 3.3, T-39, image version per row |
| m4: `gh` environment | minor | Section 3.6; phase 2 inherits it |
| m5: AC-8 | minor | AC-8 and section 9 |
| m6: retroactive ADR-0008, fixture audit | minor | Section 5 |

Standard reviewer:

| Finding | Severity | Resolution |
|---|---|---|
| 1: the candidate's own list could run | major | Section 3.1; the list stays inline in `main`'s script |
| 2: AC-8 against the gate-change rule | major | AC-8 and section 9 |
| 3: the fake `gh` | major | `ci` mode moved to phase 2, with fetch and verify split (section 3.9) |
| 4: lychee, pnpm, Go and docs Chromium setup | major | Section 3.3 |
| 5: Gitleaks fetch depth | major | Section 3.2 |
| 6: static package lists | major | Run-time assignment and the union check |
| 7: the cache reasoning | major | No cache (security B1) |
| 8: token permissions | major | No Workflows permission; the owner runs the settings check |
| 9: the minute estimate under-counted | major | Section 4 redone |
| 10: scope | major | Split (section 2) |
| Minors: `finish` claims, refusal cases, the source of `T`, setup exports, the frozen list, the settings inputs, deadline risk, HTML comments, missing statements, `bin/` rebuild, dispatch cap and Docker Hub, the platform charge in Q1-C, the settings self-test | minor | Sections 2, 3.2–3.6, 4, 8, 10–12, 14 |

Re-review of revision 4:

| Finding | Severity | Resolution |
|---|---|---|
| N1: the hook's allow against the settings ask rules; the main push changed silently | major | Precedence checked first, with three outcomes; main push preconditions, recorded as a reduction in Q5 and `AGENTS.md`; unpushed workflow commits described (section 3.7) |
| N2: `gh api --input` got around "GET only" | major | Strict `gh api` form, with self-test cases (section 3.7) |
| N3: what `run-job` runs was not specified | major | The CI job table as data, three rewrites only, `pre` assigned, step files dropped, AC-1 cases (section 3.1) |
| N4: end-to-end coverage could drop silently | major | Each shard compares the candidate's projects, and `gate` proves it; `prepare` runs `go list` (section 3.2) |
| N5: Node not pinned; corepack unchecked | major | `.node-version` and `go.mod` from the candidate; corepack tarball checksum (section 3.3) |
| N6: squash candidates had no commit | major | A one-commit branch on `M` first (section 3.5) |
| N7: scope inconsistencies | minor | Section 2 reworded; `change` and `evidence` scopes dropped |
| N8: credential stores readable | minor | Denials and accident-protection wording (section 3.7) |
| N9: a redundant script in the hook | minor | Dropped |
| N10: comments in other `.github` files | minor | Every file under `.github/` |
| N11: the staged Gitleaks scan is empty in CI | minor | `M..C` scan in CI |
| N12: actionlint's shellcheck; the hook honoured live | minor | `-shellcheck=`; PM-6 |
| N13: setup estimate, 20 concurrent jobs, settings from `main` | minor | Section 4 redone; expected failure noted (section 3.5); the concurrency fix replaced in revision 6 |

Last re-review, of revision 5:

| Finding | Severity | Resolution |
|---|---|---|
| M-a: the `docs` scope could not pass without `bin/` | major | `docs-checks` builds `bin/` and runs `pre`; Markdown-only estimate redone (sections 3.1, 3.2, 3.3, 4) |
| m1: a concurrency group cancels pending runs | minor | The coordinator dispatches one run at a time; cancelled runs recorded (section 3.2) |
| m2: an editing slip in section 3.1 | minor | Moved back |
| m3: trust of the proofs | minor | Each shard checks its own set; `gate` only displays (section 3.1) |
| m4: ordering and `origin` lag | minor | One order; dispatch only when `origin/main` equals local `main` (section 3.5) |
| m5: `-X` wording; whole-command forms | minor | Section 3.7 |
| m6: entry count; the settings self-test in no list | minor | `check-workflows.sh` runs both self-tests; two entries named (sections 2, 3.10) |
| m7: `gh auth git-credential` | minor | Denied (section 3.7) |
| m8: the checksum file's copy, sources, release age | minor | Read from `harness/`; URLs; ADR-0017 delay (section 3.3) |
| m9: `setup-go` and the `toolchain` line | minor | Addendum check and explicit version (section 3.3) |
| m10: PM-6 in the order of work | minor | Section 10 |
| m11: `pre` against "exactly once" | minor | Exception in AC-1 |
| m12: a fixed project set blocks phase 2 | minor | Expected failure (section 3.5) and a phase 2 constraint (section 3.9) |

The last reviewer allowed under `AGENTS.md` found one major finding, fixed in revision 6. The owner answered Q8 with one more review (section 14).

Review after the owner's answers, of revision 7:

| Finding | Severity | Resolution |
|---|---|---|
| MAJOR-1: candidate branches became public before any secret or personal-data check | major | Change checks and readiness reviews first; the hook runs Gitleaks on `main..<b>` before allowing the push; secret scanning and push protection; AC-4 and AC-9 (sections 3.5, 3.6, 3.7) |
| MAJOR-2: the history was checked for secrets only | major | Full-history heuristic scan run on 2026-11-27 and recorded, with its limits (section 5); AC-9 |
| m1: stale references | minor | Sections 3.2, 3.5, 3.7, 3.8, 4, 11 and 13 updated |
| m2: features strangers can write to | minor | Wiki, Discussions, Projects and Pages off (section 3.6) |
| m3: settings applied too late | minor | Applied at the visibility switch (section 1, PM-1) |
| m4: untrusted text through allowed commands | minor | Narrowed `gh pr` and `gh api` forms; header restriction (section 3.7) |
| m5: the token was wider than needed | minor | Issues write dropped (section 3.6) |
| m6: the tag ruleset was ambiguous | minor | Target, rules and no bypass stated (section 3.6) |
| m7: candidate-uploaded artifacts | minor | Section 5, T-49 |
| m8: untracked folders could be published | minor | `.gitignore` entries, AC-9 |
| m9: outcomes without criteria | minor | AC-9 |

This review found two major findings, now fixed. Under the owner's Q8 answer, the plan therefore goes back to the owner, who decides on a further review.

## 14. Owner's answers of 2026-11-27

The owner answered the questions of revision 6 on 2026-11-27. They are recorded here, and ADR-0026 and the phase 2 plan carry them:

- **Q1 = B:** the repository becomes public, for free GitHub-hosted runners and rulesets on Free.
  - There is no license file, so all rights are reserved.
  - The owner switches visibility himself, after a Gitleaks scan over all 431 commits of `main` with no leaks.
  - The author address in history stays as it is.
- **Q2 = A:** phase 2 restates NFR-6.2 for CI from the median of the first runs.
- **Q3 = A:** after phase 2 the laptop runs only file-writing tools: formatters, `sqlc generate` and the API client generator.
- **Q4 = A:** a pull request per candidate, merged by local fast-forward, then pushed.
- **Q5 = A:** the exact push and dispatch forms run without asking. Changes to `.github/workflows/` are pushed by the owner.
- **Q6 = B:** CI becomes the gate as soon as phase 2 merges, with no series of shadow pairs required. At least one shadow pair is kept as a smoke check in phase 2's own verification.
- **Q7 = A:** if minutes or GitHub are unavailable, fall back to the local gate, with a record.
- **Q8 = A:** one more fresh reviewer, standard and security, over revision 6 together with these answers, before approval. If it finds no blocker or major, the plan counts as approved.
