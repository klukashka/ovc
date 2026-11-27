# Task plan: make the CI run the merge gate (phase 2 of the CI adoption)

- Status: draft for owner approval, 2026-11-27. Revision 2 resolves the standard and security planning reviews of revision 1; revision 3 resolves the re-review of revision 2; revision 4 resolves the last re-review an agent may run on its own; revision 5 resolves the owner-requested review of revision 4 (section 12). A further review is again the owner's decision (Q10).
- Builds on: phase 1, change `chore/github-ci-adoption` (commit `74375b3`), its ADR-0026, and its approved plan `knowledge/reviews/github-ci-adoption-task-plan.md`, section 3.9.
- Branch: `chore/github-ci-gate-switch`, one commit, on `main` after phase 1 has merged. On `main` it joins the list of the earliest slice-10 sub-slice whose slice review has not started, as `AGENTS.md` requires for a commit in no list.
- Tier: **standard**.
- Security trigger: **met**. The change:
  - replaces the verification policy (ADR-0018's merge gate);
  - changes agent configuration;
  - relies on GitHub's run records;
  - changes what becomes public and when.

  It therefore gets a standard reviewer and a fresh security-profile reviewer, for this plan and at readiness.
- Pushes: this change edits `.github/workflows/gate.yml`, so the owner pushes its candidate branch and, after the merge, `main` (ADR-0026: the agents' token has no Workflows permission).

## 1. Problem

Phase 1 runs `main`'s check list on GitHub-hosted runners in shadow mode, and the local merge gate on the laptop is the only gate. The owner decided on 2026-11-27:

- **Q6-B.** CI becomes the gate as soon as this change merges. There is no series of shadow pairs first; one pair is a smoke check in this change's own verification.
- **Q3-A.** The laptop then runs only file-writing tools.
- **Q7-A.** When GitHub is unavailable, the coordinator falls back to the local gate, with a record.
- **Q2-A.** NFR-6.2 is restated for CI from the median of the first runs.

ADR-0026 leaves phase 2 three open requirements:

- stop resting acceptance on the forgeable `needs` results alone;
- solve the fixed Playwright shard set;
- define what happens to candidates that change the CI harness.

## 2. Desired outcome

- **`ci-fetch` and `ci` modes of `tools/merge-gate.sh`.** They verify one CI run for the exact candidate commit, and `finish` accepts their record.
- **Change checks and after-fix evidence run in CI,** through the `change` and `evidence` scopes of `gate.yml`.
- **The e2e job shards all projects with Playwright's own `--shard`,** so a candidate that adds a project or spec group is still fully covered.
- **Documents.** `AGENTS.md`, ADR-0018, ADR-0026, ovc-verify, the development guide, NFR-6 and the threat model describe:
  - CI as the gate;
  - the recorded fallback;
  - the CI check budget;
  - what stays on the laptop.

## 3. Design

### 3.1 The CI job set, now fixed

| Job | Shards | Change |
|---|---|---|
| `validate`, `prepare`, `gate` | — | none |
| `backend-tests` | 3 | none |
| `backend-checks`, `frontend-checks` | — | none |
| `e2e` | 12 | `pnpm e2e --max-failures=10 --trace=off --workers=2 --shard=k/12`, over every project of the candidate's configuration |
| `docs-checks` | — | none |

- **Job counts, stated separately.** Phase 1 already runs 12 e2e shards. A full run executes 20 jobs and lists 21, because the skipped `docs-checks` also appears; about 17 run at peak. This change adds an `evidence` job, skipped in full and docs runs, and the counts change accordingly. ADR-0026 states executing, listed and peak counts.
- **Shard coverage.** Playwright partitions the whole suite, across every project, into 12 shards. A candidate that adds a project or spec group needs no harness change, and `run-job`'s project-set refusal is removed.
  - `--max-failures=10` applies per shard, so the ADR-0018 amendment says so, and the not-run rule applies per shard.
  - A shard with no tests ("No tests found") fails, which is the safe outcome; the suite is far larger than 12 tests.
  - How Playwright balances tests across shards and projects is recorded in step 1.
- **Expected names per scope.** These are written once in `merge-gate.sh` and used by both `run-job` and `ci`. They list every job key of the new `gate.yml`, `evidence` included, with its expected conclusion per scope, and a `check-workflows-test.sh` case asserts that the expected sets equal `gate.yml`'s job keys. The sets are:
  - `full`: `validate`, `prepare`, `backend-tests (1)`…`(3)`, `backend-checks`, `frontend-checks`, `e2e (1)`…`(12)`, `gate`, all `success`, and `docs-checks` and `evidence` `skipped`;
  - `docs`: `validate`, `prepare`, `docs-checks`, `gate`, all `success`, and every other job `skipped`.

- **Counts.** A full run executes 20 jobs and lists 22: the skipped `docs-checks` and `evidence` jobs included. A docs run executes 4 and lists whatever the real fixture shows.
- **Fixtures.** The new `gate.yml` cannot run before it merges, so the fixtures are derived: phase 1's real runs, plus the new jobs as the documentation and step 1's checks predict. Skipped `fromJSON` matrix jobs may appear as one entry, and the earliest-job timestamp excludes skipped jobs.
- **PM-0.** Before any merge relies on `ci`, the first post-merge full and docs runs confirm the names. A mismatch takes the `harness` fallback path, decided in advance.
- **Empty matrices.** Every matrix job has a job-level `if` on a non-empty validated list, so an empty list never reaches `strategy.matrix`.
- **One source of shard counts.** `prepare` takes its shard lists from a `merge-gate.sh ci-matrix` subcommand that reads the same constants as the expected sets, and a test ties them.

### 3.2 `ci-fetch` and `ci`

Both run from `main`'s scratch copy, like `gate`.

**`ci-fetch --run RUN_ID --commit C --out FOLDER [--logs]`**

- **Validation.** `RUN_ID` must match `^[0-9]{1,20}$` and `C` must match `^[0-9a-f]{40}$`. `FOLDER` must not exist, or be empty and not a symbolic link.
- **Refusals.** Before resolving `gh`, it refuses `GH_TOKEN`, `GH_HOST`, `GH_REPO`, `GH_CONFIG_DIR`, `GH_ENTERPRISE_TOKEN`, `GH_DEBUG`, `GITHUB_TOKEN`, `GITHUB_ENTERPRISE_TOKEN` and `XDG_CONFIG_HOME`.
- **Requests.** Read-only `gh api --hostname github.com --method GET` on `repos/klukashka/ovc/`:
  - `actions/runs/RUN_ID`;
  - `actions/runs/RUN_ID/jobs?filter=all&per_page=100`, which must return its whole `total_count`;
  - `actions/workflows/gate.yml/runs?event=workflow_dispatch&head_sha=M&per_page=100`, paginated until its whole `total_count` is read, refusing otherwise. This is the attempt history (section 3.4).

    A dispatch is accepted only when the candidate's parent is `main`'s commit at dispatch time, so every attempt for a candidate on `M` has `head_sha` equal to `M`. The set is therefore complete and bounded. Step 1 verifies that `head_sha` filtering works for `workflow_dispatch` runs.
- **Logs.** With `--logs`, it downloads into `FOLDER/logs`: the logs of failed jobs, the evidence job, or every job of a `change` run.
- **Output.** It writes the JSON answers and the SHA-256 of each file.
- **Visibility.** The guard hook does not see these `gh` calls inside the script. The GET-only contract is held by tests and review, and T-49 records that.

**`ci --ff-only --change BRANCH --runs FOLDER`**

1. **Bookkeeping.** It takes the merge-gate lock, applies the same refusals as `gate`, and records `M` (local `main`) and `T = BRANCH^{tree}`.
2. **Candidate shape.** `BRANCH` is one commit `C` whose parent is `M`, and `origin/main` (as last fetched) equals `M`.
3. **The run's identity.** Exact string comparison on the parsed JSON only:
   - repository and head repository `klukashka/ovc`;
   - path `.github/workflows/gate.yml`;
   - event `workflow_dispatch`;
   - `head_branch` `main` and `head_sha` `M`;
   - `display_title` exactly `gate full C`, or `gate docs C` for a Markdown-only candidate under the existing classification;
   - `triggering_actor.login` is the owner.
4. **The run's completion.**
   - `status` is `completed`, `run_attempt` is 1 and `conclusion` is `success`.
   - The jobs, read independently of the `gate` job and of the commit status, equal the expected set of section 3.1, with `success` and `skipped` exactly as listed and every job at attempt 1.
5. **Attempt history** (section 3.4).
6. **The record.** It writes a record of kind `ci` with:
   - the run URL;
   - the fetched files' SHA-256;
   - the execution time and queue time (section 3.6);
   - the scope, `M` and `T`.

   The lock stays `passed` for `finish`, as with `gate`.

**`finish`** accepts a `ci` record like a passed gate record. It still checks that `main` advanced from `M` only by this merge and that `main^{tree}` equals `T`.

### 3.3 Trust in a CI result: an owner decision (Q1)

ADR-0026 requires that acceptance not rest on the `needs` results alone. `ci` mode reads each job's conclusion from the REST API, not from the `gate` job or the commit status, and catches missing, skipped, extra and retried jobs.

That is still not tamper-proof. A job that runs candidate code can forge its own conclusion, because the code has `sudo` on the runner and can write the job's step files. No setting of a hosted runner prevents this.

The local gate has the comparable limit: candidate code runs as the owner's user and can rewrite the scratch script and its records. ADR-0018, however, does not say that the gate guards only against accidents.

This change therefore asks the owner (Q1) to decide, and records the answer:

- in an ADR-0018 amendment;
- in ADR-0026, replacing its "must not rest on them alone" sentence;
- in T-41 and T-49.

The decision to record: the merge gate, local or CI, protects against accidental breakage by the owner and the owner's agents, and malicious code is the readiness reviews' job.

### 3.4 One run per attempt: the rerun rule kept

- **Who dispatches what.** Only the coordinator dispatches `full` and `docs`. Implementing agents dispatch only `change` and `evidence`, and `AGENTS.md` states the rule.
- **The claim.** Before dispatching `full` or `docs`, the coordinator runs `merge-gate.sh claim --ff-only --change BRANCH`.
  - It takes the merge-gate lock in a durable state `dispatched`, with `C`, `T` and `M`, which is not judged stale by process.
  - `ci` continues from that claimed lock.
  - A failed or cancelled run ends with an explicit `release`, and its failure follows the rerun and after-fix rules.
- **The hook's check.** The hook allows the exact `full` or `docs` dispatch only when the lock file's fields `state=dispatched` and `head=C` match the dispatched commit. Any other `full` or `docs` dispatch asks. While the lock holds `state=dispatched`, the hook also asks before `change` and `evidence` dispatches.
- **Lock code: states, keys and transitions.** Phase 1's `read_lock_file` fails on unknown keys and accepts only `running` and `passed`. It learns the following, and the hook uses the same parser:

| State | Keys added | Entered by | Left by | Who may rewrite |
|---|---|---|---|---|
| `dispatched` | `change_id`, `kind` (`gate`, `catch-up`, `after-fix`), `head`, `tree`, `base` | `claim`, `claim --catch-up`, `claim --after-fix` | `ci`/`ci --catch-up` (to `passed`), `release --run` / `--no-run`, `fallback --from-claim` | any process whose arguments match `head`, `tree` and `base` |
| `evidence` | `change_id`, `failed_run`, `before_tree`, `base`, `pending`, `runs` (series, run ID, outcome) | `claim --evidence` | `claim --after-fix`, `release --evidence` (only as above) | `series-open` and `evidence-record` matching `change_id` and `base` |
| `fallback` | `change_id`, `reason`, `stalled_runs` | `fallback --from-claim` | the `gate` mode takes it over, then `passed` and `finish` | `gate` with the same `change_id` |
| `passed` (`ci-catch-up`) | `kind=catch-up` | `ci --catch-up` | a finish-equivalent line, then the push of `main`, then release | the same process, or `release` after the push |

Each transition gets an AC-1 case.

- **A stable change identity.** `claim` takes a mandatory `--change-id`, the change's original branch name. Every rebuild is named `<change-id>-r<k>`, and the hook checks that derivation for branch pushes and dispatches.
- **Earlier attempts across bases.**
  - Every `claim` appends a claim line to the merge-gate log with:
    - the change ID and branch;
    - `C`, `T` and `M`;
    - `git patch-id --stable` of `C` against its parent.
  - A claim ends only through `ci`, through `release --run ID`, or through `release --no-run`. `--no-run` records that no run exists, and `ci-fetch` checks that against the `head_sha` history by title. Each way of ending appends the run ID, or `none`, to the claim line.
  - `ci-fetch` reads the claim log and also fetches every earlier claimed run of the same change ID or patch-id by its ID. A claim without an ID is resolved through its own old-base `head_sha` history by title. `ci` reads only what `ci-fetch` stored, so the GET-only split holds.
  - A failed gate on an earlier base is therefore seen, whether the rebuild kept the diff (a rerun) or changed it (which needs an `after-fix` row).
- **Stray dispatches.** The remote `head_sha = M` query stays as the check for stray dispatches on the current base. Every `full` or `docs` run there whose title names `C`, or names a local commit whose tree is `T`, counts, and a commit no longer known locally counts within that bounded set.
- **Attempts still open.** `ci` also refuses while any other counted attempt is not completed, or is completed with anything but first-attempt `success`, unless a row names it.
- **The attempt check.** `ci` refuses when an earlier attempt failed, was cancelled, or was not first-attempt, unless the candidate's tree holds a `rerun` or `after-fix` row in the slice's known-limitations file that names that run's URL. The ADR-0018 failure rules are then applied as today.
- **Reruns.** The one infrastructure rerun is a new dispatch, never "re-run jobs". It is allowed only after a named infrastructure error with no failed assertion: every entry line that `run-job` printed has exit 0, and the failing job ended in a quoted infrastructure error.
- **Named infrastructure errors** (ADR-0018 amendment): runner lost, service start failure, image, package or browser download failure, a GitHub outage, plus the local list (Docker or PostgreSQL unreachable, `too many clients`).

### 3.5 The `change` and `evidence` scopes

**`change`**

- **Input.** `jobs` is a comma-separated list of CI job names, with shards for sharded jobs (`e2e:3`).
- **Validation.** `prepare` validates `jobs` against `main`'s table and outputs a JSON array, never letting the raw input through.
  - Unsharded jobs select themselves with `contains(fromJSON(needs.prepare.outputs.jobs), '<name>')`.
  - The `backend-tests` and `e2e` matrices are built from `prepare`'s validated shard lists (`fromJSON` in `strategy.matrix`), because a job-level `if` cannot read the `matrix` context (unverified, checked in step 1).
  - A full run still produces exactly the expected names, and a fixture case covers it.
- **Change-check table.** The development guide's table gains a CI-jobs column; jobs are coarser than rows, so a row maps to every job holding its commands. Any doubt means `full`.
- **Title and results.** `display_title` is `gate change C`. The run posts no status, and `ci` refuses it.
- **Repeats.** Repeated `change` dispatches for one commit are allowed and reported; they are not gates.

**`evidence`** (ADR-0018's after-fix rule)

- **Inputs.**
  - `kind`: `go`, `playwright` or `check`.
  - For `go`, `package` and `test`. The harness builds `go test` with the gate's flags for that package, `-count=1 -v` and the anchored `-run ^Test…$`.
  - For `playwright`, `project` and `title`. The harness builds `--retries=0`, the project, `-g` with the escaped title and the `list` reporter.
  - For `check`, `entry`: the name of a list entry of `main`'s gate. The harness runs the gate's own command for it.

  Titles are limited to a character allowlist and 200 characters, and reach the command as one argument through `env:`, never through `eval` or `bash -c`. They never appear in `run-name`.
- **Series.** `series` is one of these:
  - `before-first`: 1 run;
  - `before-rest`: 9 runs, allowed only when the attempt history shows a `before-first` run of the same selection that did not reproduce;
  - `after`: 10 runs.

  Any other count is not supported in CI. It goes to the owner and runs locally as a recorded fallback.
- **Trees.** Both trees are one commit on the same `M`, so the hook and `prepare` accept them unchanged:
  - **before** is the failed candidate's diff rebuilt on `M`. Where `main` has not moved, its tree equals the failed gate's tested tree, and the row records both.
  - **after** is that diff plus only the fix.
- **One base for a whole evidence series,** held by the merge-gate lock rather than by a file the series cannot see yet.
  - **Opening.** `claim --evidence --change BRANCH` writes `state=evidence` with the before tree, `M` and the failed run's URL.
  - **While open.** `claim` and `ci` refuse, so nothing merges and both series stay on one base, as ADR-0018 requires.
  - **Recording.** After each evidence run, `evidence-record --run ID` fetches its log through `ci-fetch --logs` and stores the run with the coordinator's recorded outcome: for `before-first`, whether it reproduced the match key.
  - **Gating each dispatch.** The hook allows a `before-first` dispatch only when the lock holds `state=evidence` with that before tree and no run yet. It allows `before-rest` only when the recorded `before-first` did not reproduce and its before tree matches. It allows `after` only once the before series is complete. Otherwise it asks.
  - **No silent end.** A plain `release` refuses `state=evidence`, and the refusal messages of durable states no longer suggest `release`. `release --evidence` is allowed only with a recorded run that ran no test, the case ADR-0018's evidence rules handle as not run.
  - **No second series.** `claim --evidence` refuses when the merge-gate log already holds an evidence claim for the same failed run URL. The one exception is ADR-0018's: the earlier series recorded a run that ran no test, and the diff has changed since.
  - **One dispatch per opening.** Before each evidence dispatch, `series-open --series <name> --commit <sha>` writes `pending=<series>,<sha>` into the lock. The hook allows exactly one dispatch matching it, and `evidence-record` clears it. The `after` commit's parent must be the lock's `M`.
  - **Checked records.** `evidence-record` checks the run's identity:
    - the title `gate evidence C <series>`;
    - `head_sha` equal to the lock's `M`;
    - the commit equal to the pending one.

    It derives "reproduced" from the log's recorded lines against the match key, and stores the coordinator's statement next to it.
  - **No gap before the after-fix gate.** The series does not end with a release. `claim --after-fix --change BRANCH` takes the lock directly from `state=evidence` to `dispatched`, and refuses unless all of these hold:
    - the after series is complete and passing;
    - the candidate's parent is the lock's `M`;
    - the candidate's tree equals the after tree plus only the known-limitations rows, checked by a path diff.

    So no other change can merge between the evidence and its gate.
  - **Job limit.** The evidence job's `timeout-minutes` covers 10 runs of the longest allowed entry, so a series is never cut and recorded as not run.
- **One dispatch per series.** Each series is one dispatch whose single job runs its runs one after another.
- **Branches.** The trees are the tips of local branches named `<branch>-before` and `<branch>-after`, pushed in the exact form. They are deleted on the remote after the after-fix gate merges, which asks the owner.
- **Validation of the other inputs:**
  - `package` matches `^[A-Za-z0-9_./-]+$`, does not start with `-`, and is a member of the candidate's `go list ./...`;
  - `test` matches `^Test[A-Za-z0-9_/]+$`;
  - `project` is a member of the candidate's Playwright projects;
  - `entry` is a member of `main`'s list;
  - each is passed after `--` where the tool accepts it.

  Step 1 checks GitHub's limit on the number of dispatch inputs.
- **Title and results.** `display_title` is `gate evidence C <series>`, and the run posts no status. The job reuses `ci-settings.sh` masking and redaction, and uploads no traces.
- **Records.** Every evidence dispatch is found by its title in the attempt history, so none is repeated, discarded or replaced unseen by accident.
  - The agents' token can also cancel or delete runs and logs, and the hook does not see `gh` calls inside scripts. So the guarantee holds only under the accident model of Q1.
  - T-41 and T-49 record this.

### 3.6 The CI check budget (Q5)

- **Timestamps.** For each run:
  - execution time = the `gate` job's `completed_at` minus the earliest job's `started_at`;
  - queue time = the earliest `started_at` minus the run's `created_at`.

  Queue time is reported separately.
- **The limit.** NFR-6.2 for CI applies to execution time.
  - It is provisional at first: 1.25 times the median execution time of at least three full runs of a pre-final candidate of this change, rounded up to the next minute. Those runs use phase 1's harness on `main`, not this change's sharding.
  - It is re-derived from the first three full gate runs after the merge, and ADR-0026 records both values.
  - Per-job queue time inside a run is reported too, because more than 20 jobs can compete.
- **ADR-0018's budget rules, in CI terms:**
  - a passing run at or above the limit goes to the owner before the merge;
  - when the median of the last three passing gate runs exceeds 90 % of the limit, the consolidation question goes to the owner;
  - battery and idle-machine rules no longer apply to CI runs.
- **One gate run at a time.** The coordinator dispatches `full` and `docs` one at a time, and agents do not dispatch `change` or `evidence` runs while a gate run is queued or running. That keeps queue time out of the gate and the fallback trigger honest.
- **Local fallback.** A fallback local gate keeps 600 s and all local rules.

### 3.7 Evidence that outlives GitHub's 3-day retention

The coordinator runs `ci-fetch --logs` within retention:

- for every failed gate run;
- for every evidence run.

The logs go to `~/Library/Caches/ovc-e2e/ci-logs/<run>/`, readable only by the owner, beside the local gate's log folders. The row in the known-limitations file then records, in the committed tree:

- the run URL;
- each log's SHA-256;
- the required recorded lines, quoted verbatim.

Slice reviewers re-verify against those lines and the local files, never against GitHub.

- **Readiness reports** quote the lines of their `change` runs the same way, and those runs' logs are fetched too.
- **The logs** are stored as the raw zip with its SHA-256, and extracted only after a path check: no absolute paths and no `..`.

### 3.8 How a change flows

1. The implementing agent finishes one commit on a branch built on `main`'s tip.
2. The agent pushes it. The hook allows the exact form after Gitleaks with `main`'s configuration and ignore list, and push protection is the second line.
3. The agent dispatches `change` with its jobs and quotes the result lines.
4. The readiness reviews run.
5. The coordinator rebuilds the one-commit candidate on the current `M` if `main` moved, always under a new branch name so that the exact non-force push form applies, runs `claim`, pushes it, and dispatches `full` or `docs`.
6. The coordinator runs `ci-fetch` and `ci`, then `git merge --ff-only`, `finish` and `git push origin main`, and deletes the remote branch, which asks the owner.

**Public before review (Q6).** Step 2 makes code public before its readiness reviews, which T-49's phase 1 control ("pushed only after its readiness reviews") forbids. That sentence is amended. These residuals are recorded:

- A pushed commit stays retrievable by its hash after the branch is deleted, so a leaked secret is rotated, never just deleted.
- Gitleaks and push protection detect secrets, not personal data. The push precondition also runs the heuristic scan of the pre-publication audit (email domains outside reserved test domains, phone patterns) on `main..<branch>`, and asks when it finds anything.
- Once OVC is deployed, pushing a security fix before its review would disclose the vulnerability before it is deployed. From the first deployment, a fix for a vulnerability goes through a private route: a GitHub security advisory's private fork. Otherwise it is pushed only after review. T-49 records this.

### 3.9 Fallback to the local gate (Q7-A)

- **Triggers.** Only GitHub-side unavailability, quoted in the record:
  - a dispatch answered with HTTP 5xx or an outage message;
  - a run that does not start within 30 minutes while no other run of ours is queued;
  - an Actions incident on GitHub's status page.

  A 4xx, a hook refusal, a failed run and an infrastructure error inside a run are not unavailability.
- **Harness changes (Q8).** A candidate that changes the CI harness cannot be verified by `main`'s harness. The harness here means `.github/workflows/gate.yml`, `tools/merge-gate.sh`'s CI table, `tools/ci-settings.sh`'s generation rules or `tools/ci-tools.sha256`. Such a candidate is gated locally as a fallback of kind `harness`, with the owner's approval, as ADR-0018 gates a gate change by the gate it replaces.
- **Fewer harness fallbacks.** `ci-settings.sh` now reads the candidate's example files as data. A value written as a placeholder is generated at random, and any other value is copied. So a candidate that adds an `OVC_*` setting needs no harness change.
  - Every generated value is masked with `::add-mask::` and added to `CI_GENERATED_SETTING_NAMES` for redaction, like the fixed ones.
  - A placeholder of an unknown format gets 32 random bytes in hex.
  - A denylist of harness-reserved names refuses the candidate's run: the run-lock, run-output and gate-adoption variables of the end-to-end harness, and `OVC_TEST_CONTROL`, which keeps `main`'s rules.
- **Recording.** A fallback local gate always goes through the owner. The guard hook asks on any `merge-gate.sh` with the `gate` argument, by basename, wherever the script runs. The owner's approval is recorded in a row of kind `fallback` in the slice's known-limitations file, and the merge report repeats it.
- **At most one unpushed fallback merge.** A fallback merge is never pushed by an agent, and further merges wait until it is resolved.
- **From claim to fallback.** When a claimed run stalls or the trigger fires, `fallback --from-claim` moves the lock from `dispatched` to a local fallback gate with the quoted reason. The `fallback` row must name every stalled or cancelled run, because `ci --catch-up` otherwise counts it as a failed attempt.
- **Catch-up when GitHub returns:**
  1. `claim --catch-up` writes `state=dispatched` of kind `catch-up`, with `C` (local `main`'s tip), its parent `origin/main`, and `T`.
  2. The coordinator pushes `C` as the branch `catch-up/<first 12 characters of C>`. The hook allows exactly that form when the lock's catch-up claim names `C` and `C`'s parent is `origin/main`, after Gitleaks and the heuristic personal-data scan of `origin/main..C`. It never needs a push of `main`.
  3. The coordinator dispatches a `full` run of `C`, even if it was Markdown-only. The hook allows that dispatch under the same catch-up claim.
  4. `ci --catch-up` applies the same identity, job and attempt checks, with `head_sha` equal to `origin/main`. It writes a record of kind `ci-catch-up` and a finish-equivalent line naming `main`'s tip and its tree, which the hook's push of `main` requires.
- **The `main-ci` ruleset.** Under Q2-A the required `ovc-gate` check sits in its own ruleset, `main-ci`. The `main` ruleset (deletion, force push, linear history) keeps no bypass.
  - **Agents' token.** Whether a push made with the agents' fine-grained token, which belongs to the owner's account, would use an owner bypass is unverified, and step 1 checks it.
  - **If it would:** `main-ci` gets no bypass list. For a harness candidate (Q8) or a fallback resolution, the owner temporarily sets `main-ci` to disabled, pushes, and re-enables it. The `fallback` row records this, and `tools/check-github-settings.sh` confirms afterwards that it is active again.
  - **If it would not:** the owner-only bypass is used, recorded the same way.
- **If the catch-up run fails,** the owner resets local `main` to `origin/main`, which only the owner may do, and the change goes back to its branch under the after-fix rule. The decision goes into the `fallback` row, and no merge happens until it is resolved.
- **Stalled runs.** Before `fallback --from-claim`, the coordinator cancels the stalled run, so it cannot start later and post `ovc-gate` on `C`.
- **The local gate is kept working.** At each slice review, one recorded local full gate runs on `main`'s tip, so the fallback still works when it is needed.
- **Slice review.** The slice review re-verifies `fallback` rows as it does `rerun` and `after-fix` rows (AGENTS.md slice-10 text and Slice review bullet).

### 3.10 GitHub-side enforcement (Q2)

The `main` ruleset can require the status context `ovc-gate`. Every workflow posts as the same GitHub Actions app, so the app restriction shuts out only other apps, not other workflows.

What actually limits forging that status is:

- the agents' token lacks the Workflows permission;
- the hook's `.github/` equality check on pushes;
- `ci` mode reads the run itself.

The status is computed from forgeable `needs` results and is never evidence. It adds a GitHub-side backstop against a push of an unverified `main`.

Changes needed:

- `gate.yml` posts one context, `ovc-gate`, with the scope in its description. This deviates from phase 1's `ovc-gate/<scope>`.
- `change` and `evidence` runs post none.
- A harness candidate (Q8) is pushed by the owner, so the token and hook controls do not apply to it. Its branch is pushed only after `check-workflows.sh` passes on it locally, or it is not pushed at all and only `main` is pushed after the merge. T-49 records this.
- Before enabling the rule, step 1 verifies:
  - that a required check applies to direct pushes;
  - whether `GITHUB_TOKEN` is issued per job with that job's permissions, so candidate jobs never hold statuses write.

### 3.11 The owner's push prompts (Q3)

`.claude/settings.json` keeps its `git push` ask rules, because an agent may not weaken its own permission settings.

If the owner chooses to remove them, that is a recorded change of the owner's under the security trigger. Before it, this change makes the hook allow the exact push of `main` only when all of these hold:

- the merge-gate log's last `finish` line names local `main`'s tip and its tree;
- that line belongs to a `ci` record or to the finish-equivalent line of a `ci-catch-up` record, never to a local fallback gate;
- exactly one finished merge lies between `origin/main` and local `main`.

Otherwise it asks. T-41 records the residual.

### 3.12 The laptop after the switch (Q4, Q7)

- **Allowed:**
  - file-writing tools: formatters with `--write`, `sqlc generate`, the API client generator;
  - the dependency tools AGENTS.md requires: `go get`, `go mod tidy`, `pnpm install` and `pnpm add`;
  - the local stack, a production build and the preview server for a rendered-screen review;
  - the hook's own pre-push Gitleaks and heuristic scans.
- **Measurements stay on the idle laptop (Q7),** as an explicit exception: comparable hardware and the idle-machine rule apply only there. That covers the measurement procedures of the development guide and slice-review performance measurements. The hook also asks before each measurement command, and the owner's approval is the slot.
- **Everything else runs in CI:** tests, lints, type checks, scans, gates and evidence. Reviewers do not run checks locally.
- **The guard hook asks before** these commands (accident protection; the list is illustrative, not complete):
  - `go test`, `go vet`;
  - `golangci-lint`, `govulncheck`;
  - `pnpm test|e2e|lint|typecheck|check:…|format:check`, including through `--dir`;
  - `playwright test`;
  - `tools/docs/check-docs.sh`, `tools/check-*.sh`, the hook's checker;
  - Gitleaks and OSV scans;
  - `merge-gate.sh gate`.

## 4. Files

- **Code:**
  - `tools/merge-gate.sh`: `ci-fetch`, `ci`, `finish`, e2e sharding, the expected job sets, attempt history, the removal of the project-set check
  - `tools/check-merge-gate.sh`
  - `tools/testdata/ci-runs/`: fixtures derived from a real phase 1 run, then varied
  - `.github/workflows/gate.yml`: the `change` and `evidence` scopes, the `ovc-gate` context, e2e `--shard=k/12`
  - `tools/ci-settings.sh` and `tools/ci-settings-test.sh`: reads the candidate's example files
  - `tools/check-workflows.sh` and `tools/check-workflows-test.sh`
  - `.claude/hooks/guard_secrets.py` and `.claude/hooks/check_guard_secrets.py`: the new dispatch forms, coordinator-only `full`/`docs`, the `finish`-backed `main` push, the personal-data heuristic, asking for local checks and for `merge-gate.sh gate`
  - `tools/check-github-settings.sh` and `tools/check-github-settings-test.sh`: answers a read-limited token may not read (HTTP 403 on collaborators, deploy keys, webhooks, secrets, variables, environments) are reported as `settings match where readable; unverified (no read permission): <names>` with exit 0 when nothing else differs, never as a full match (requested 2026-11-27 for the owner's read-limited token)
- **ADR-0026 sections to amend:**
  - "What the second change must keep";
  - "Only the coordinator dispatches";
  - the shadow-pair preconditions;
  - "One run at a time";
  - "Expected failures";
  - the job count;
  - the status context;
  - Consequences;
  - Open owner actions;
  - the CI time limit.
- **ADR-0018:** the CI gate, the trust statement, the CI check budget, named infrastructure errors, evidence in CI, fallback kinds.
- **`AGENTS.md`:**
  - change workflow: agents push and dispatch `change`/`evidence`;
  - Development verification: the merge gate summary, the evidence and measurement sentence ("every evidence run goes through the main checkout's `tools/dev-env.sh`" becomes CI for evidence, local for measurements), fallback rows in the slice-10 and Slice review text;
  - Testing: the run-budget bullet.
- **`.claude/skills/ovc-verify/SKILL.md`:** the "Merge gate" and "Shadow CI" sections, including the sentence that a readiness report never cites a CI run.
- **`docs/development.md`:** the change-check table's CI column, the merge gate section, the evidence and measurement sections, the laptop tools.
- **Other documents:**
  - `docs/requirements/non-functional.md`: NFR-6.1, NFR-6.2;
  - `docs/threat-model.md`: T-41 and T-49, with every residual of sections 3.3, 3.8, 3.10 and 3.11;
  - `knowledge/research/hosted-ci-2026-11-27.md`: the facts of step 1;
  - `knowledge/analysis/ci-shadow-runs.md`;
  - this plan.

Every changed sentence in `docs/` is marked pending owner review, and the identical-summary rule of ADR-0018 is kept across AGENTS.md, ovc-verify and the development guide.

## 5. Acceptance criteria (before the merge)

- **AC-1.** `bash tools/check-merge-gate.sh` passes, on fixtures derived from a real phase 1 run (section 3.1), with these cases. The fixtures keep the real run's field names, job names and structure; their run and job identifiers, commits, titles, timestamps, node identifiers and check-suite identifiers are synthetic, the commits filled in per case, and the evidence and docs fixtures are synthetic variations of the same shape:
  - **Accepts:**
    - `ci` accepts a valid full run and a valid docs run.
  - **Refuses:**
    - every identity mismatch of section 3.2, step 3;
    - a run that is not completed;
    - `run_attempt` above 1;
    - a conclusion other than `success`;
    - a missing job;
    - an extra job;
    - a job that should be skipped but ran;
    - a retried job;
    - a job that did not succeed;
    - a `change` run;
    - an `evidence` run;
    - an earlier failed attempt without a row;
    - a two-commit branch;
    - a parent other than `M`;
    - `origin/main` other than `M`;
    - a `docs` run for a full-scope candidate.
  - **Also:**
    - an earlier failed attempt with a row naming its URL is accepted;
    - `finish` accepts a `ci` record and refuses one whose tree differs;
    - `ci-fetch` refuses each listed variable, a bad run ID or commit, and a non-empty or linked output folder, all before `gh` is resolved, which the test shows from the refusal message and exit status;
    - the expected job sets match the committed real full-run and docs-run fixtures;
    - attempt history that is not completely read is refused;
    - an attempt matched only by patch-id is counted;
    - `claim` writes `state=dispatched` and a claim line, and `ci` continues from it in another process;
    - a failed run on an earlier base, found through its claim line, is counted;
    - an attempt still in progress is refused;
    - `claim --evidence` writes `state=evidence`; `ci` and `claim` are refused while it is held; `evidence-record` stores the outcome; `release --evidence` ends it;
    - `fallback --from-claim` moves a `dispatched` lock to the fallback;
    - `ci --catch-up` accepts a valid catch-up run, writes its finish-equivalent line, and refuses a candidate whose parent is not `origin/main` or whose stalled run no row names;
    - `ci` refuses a run whose job set lacks the skipped `evidence` job;
    - a plain `release` refuses `state=evidence`;
    - a second `claim --evidence` for the same failed run is refused, except the ADR-0018 case;
    - `claim --after-fix` takes the lock from `evidence` and refuses an incomplete after series, a wrong parent, or a tree beyond the after tree and known-limitations rows;
    - a `claim` of another change while `evidence` is held is refused;
    - a changed-diff rebuild under a new name on a new base is found by its change ID;
    - a released claim without a run ID is resolved through its old-base history;
    - `release --run` and `--no-run` append to the claim line;
    - `series-open` allows one dispatch, and `evidence-record` checks identity and derives reproduction;
    - `ci-matrix` and the expected sets share their constants;
    - each lock transition in the table of section 3.4 works.
- **AC-2.** `tools/check-workflows-test.sh` covers the new inputs and rules:
  - exact-membership job selection;
  - no status from `change`/`evidence`;
  - title limits;
  - `--shard=k/12`;
  - the expected job sets equal `gate.yml`'s job keys;
  - matrices built from `prepare`'s outputs.

  `tools/check-workflows.sh` passes on the new `gate.yml`.
- **AC-3.** `python3 .claude/hooks/check_guard_secrets.py` passes, with cases for:
  - each new dispatch form, including the evidence series gating by lock state and before tree;
  - the catch-up branch push and dispatch;
  - `change` and `evidence` dispatches asked while a gate run is claimed;
  - a coordinator-only `full` dispatch, allowed with `state=dispatched` and `head=C`, and asked otherwise;
  - the `finish`-backed `main` push, allowed after a `ci` or `ci-catch-up` record and asked after a fallback `finish`;
  - the personal-data heuristic;
  - the local check commands that ask;
  - the file-writing and dependency tools that pass.
- **AC-4.** `tools/ci-settings-test.sh` passes, including:
  - a candidate example file with a new placeholder setting, masked and listed for redaction;
  - a refused reserved name.
- **AC-5.** These pass:
  - `tools/docs/check-docs.sh`;
  - Gitleaks over staged changes and over history;
  - the backend conventions tests.
- **AC-6.** Three full CI runs of a pre-final candidate give the median, and the limit is written into ADR-0026.
- **AC-7.** The local merge gate of this change passes. This is the last default local gate. Its merge report includes the smoke pair: a full CI run of the exact final commit beside that gate (Q6-B).

## 6. After the merge

- **PM-0.** The owner pushes `main`. A full and a docs run of the merged `gate.yml` confirm the expected job names. A mismatch is fixed through the `harness` fallback before any merge relies on `ci`.
- **PM-1.** The next change merges through `ci-fetch` and `ci`. The merge report quotes the run, the record, `finish` and the push. Under ADR-0018's gate-change rule, a fault reopens this change under the after-fix rule.
- **PM-2.** The next change's change checks run as a `change` dispatch.
- **PM-3.** The first `evidence` dispatch, whenever a gate first fails, is quoted in its row as the first exercise of that path.
- **PM-4.** If Q2 is A, the owner adds the required `ovc-gate` check to the `main` ruleset after step 1's verification, and `tools/check-github-settings.sh` confirms it.

## 7. Order of work

1. **Verify on GitHub** and record in the research file:
   - whether a ruleset's required status applies to direct pushes;
   - whether `GITHUB_TOKEN` is issued per job;
   - the jobs endpoint's `run_attempt` per job;
   - `filter=all`;
   - the matrix job naming, expanded and skipped;
   - the logs endpoint;
   - `head_sha` filtering of `workflow_dispatch` runs;
   - the dispatch input limit;
   - Playwright's shard balancing;
   - empty `fromJSON` matrix behaviour and skipped-matrix naming;
   - whether a push with the agents' token uses an owner ruleset bypass.

   Capture a real phase 1 full run and docs run as the fixture base.
2. Sharding, the job sets, `ci-fetch`, `ci`, the attempt history and `finish` (AC-1).
3. The scopes, settings reading, and the workflow check (AC-2, AC-4).
4. The hook (AC-3).
5. Documents (AC-5).
6. Three full runs on a pre-final candidate, which the owner pushes (AC-6).
7. Readiness reviews, one commit, the owner's push of the final candidate, the smoke run and the local gate (AC-7).

## 8. Risks

- **Forgeable job results** (section 3.3): an owner decision, recorded.
- **Public before review** (section 3.8): the residuals are recorded, and secrets are rotated, never just deleted.
- **GitHub outages** stop CI merges until the fallback applies, with at most one unpushed fallback merge.
- **Deadline-sensitive tests and WebKit on Linux:** failures are fixed or recorded, never waived, and timeouts are never raised.
- **Job queueing beyond Free's 20 concurrent jobs** (section 3.6).
- **Unverified GitHub behaviour** (section 7, step 1): if it differs, implementation stops and the owner is asked.

## 9. Observability

No runtime component. `ci-fetch` and `ci` print report lines in the gate's format, with each checked field and the run URL.

## 10. Questions for the owner

Each question is multiple choice, with a recommended option.

- **Q1. Trust in CI results.**
  - **A (recommended):** record that the gate, local or CI, protects against accidental breakage by the owner and the owner's agents, and that malicious code is the readiness reviews' job. `ci` reads every job's conclusion from the API, and ADR-0026's sentence is amended.
  - **B:** keep the local gate authoritative and CI advisory. This contradicts Q6-B.
- **Q2. Required `ovc-gate` status on `main`.**
  - **A (recommended):** add it after step 1's verification, in its own ruleset `main-ci` with only the owner able to bypass it, for harness changes and fallback resolutions, with the one-unpushed-fallback rule.
  - **B:** rely on `ci` mode alone.
- **Q3. Push prompts.**
  - **A (recommended):** the owner may later remove the four `git push` ask rules as a recorded change of his own; the hook then still asks for every non-exact form and for any `main` push without a `finish` record.
  - **B:** keep a prompt for every push.
- **Q4. Laptop enforcement.**
  - **A (recommended):** the hook asks before local checks and `merge-gate.sh gate`.
  - **B:** policy text only.
- **Q5. The CI time limit.**
  - **A (recommended):** 1.25 times the median execution time of three full runs of a pre-final candidate, re-measured at the next slice review.
  - **B:** a fixed 20 minutes.
- **Q6. Public before review.**
  - **A (recommended):** accept, with Gitleaks, push protection and the heuristic personal-data scan before each push.
  - **B:** reviews before any push, which keeps change checks local and contradicts Q3-A.
- **Q7. Measurements.**
  - **A (recommended):** measurements stay on the idle laptop as an explicit exception.
  - **B:** design a CI measurement scope later and stop measurements until then.
- **Q8. Harness-changing candidates.**
  - **A (recommended):** gated locally as a `harness` fallback with the owner's approval.
  - **B:** split each harness change into two merges.

## 11. Earlier decisions this plan depends on

Phase 1's plan, section 14, and ADR-0026: the public repository, dispatch from `main`, two checkouts, no caches, no stored secrets, rulesets, fork approval, and the agent rules.

## 12. Planning review findings of revision 1 and their resolutions

Standard reviewer:

| Finding | Severity | Resolution |
|---|---|---|
| M1: contradicts ADR-0026 on `needs` | major | Section 3.3, Q1, ADR amendments |
| M2: fixed Playwright shard set | major | `--shard=k/12` over all projects (section 3.1) |
| M3: the job-set rule would refuse real runs | major | Exact names, `skipped`, `filter=all`, real-run fixture (sections 3.1, 3.2) |
| M4: any green run could be the gate | major | Attempt history, coordinator-only `full`/`docs` (section 3.4) |
| M5: evidence not faithful to ADR-0018 | major | Fixed series, trees rebuilt on `M`, check kind, built commands, documents listed (section 3.5) |
| M6: evidence logs expire | major | `ci-fetch --logs`, quoted lines and hashes committed (section 3.7) |
| M7: check budget undefined | major | Section 3.6 |
| M8: measurements | major | Local exception, Q7 (section 3.12) |
| m1: files list | minor | Section 4 |
| m2: status context and app claim | minor | Section 3.10 |
| m3: AC ordering and tree | minor | AC-6, AC-7 |
| m4: owner pushes this change | minor | Header, section 7 |
| m5: fallback rules | minor | Section 3.9 |
| m6: named infrastructure errors | minor | Section 3.4 |
| m7: `change` scope mechanism | minor | Section 3.5 |
| m8: laptop tool list | minor | Section 3.12 |
| m9: post-merge and test method | minor | PM-3, AC-1, header |
| m10: job count | minor | Section 3.1 |

Security reviewer:

| Finding | Severity | Resolution |
|---|---|---|
| M1: the required status can be faked | major | Section 3.10 corrected; the per-job token check in step 1 |
| M2: fallback and ruleset deadlock | major | At most one unpushed fallback merge; then a `full` run before the push (section 3.9) |
| M3: before-tree evidence cannot pass | major | Both trees rebuilt as one commit on `M` (section 3.5) |
| M4: evidence lost to retention | major | Section 3.7 |
| M5: the trust statement was presented as settled | major | Owner decision Q1 (section 3.3) |
| M6: shard set, settings and pins | major | `--shard`, settings read from the candidate as data, `harness` fallback Q8 (sections 3.1, 3.9) |
| m1: an agent could trigger the fallback | minor | GitHub-side triggers only; owner prompt; recorded approval (section 3.9) |
| m2: `ci-fetch` inputs | minor | Section 3.2 |
| m3: the new inputs | minor | Section 3.5 |
| m4: public-before-review residuals | minor | Section 3.8 |
| m5: removing the push prompts | minor | Section 3.11 |
| m6: hook list | minor | Illustrative, extended (section 3.12) |

Re-review of revision 2:

| Finding | Severity | Resolution |
|---|---|---|
| N1: attempt history completeness | major | `head_sha=M` filter, full pagination, bounded unknown commits (sections 3.2, 3.4) |
| N2: coordinator-only dispatch not buildable | major | `claim` with durable `state=dispatched`; the hook reads `state` and `head` (section 3.4) |
| N3: evidence base drift | major | No merges while a series is open, enforced by `ci` and `claim`; one dispatch per series; same before tree (section 3.5) |
| N4: catch-up of the unpushed fallback merge | major | `claim`/`ci --catch-up`, `ci-catch-up` record, failure path to the owner (section 3.9) |
| N5: `main` push after a fallback | minor | Only after `ci` or `ci-catch-up`, one merge ahead (section 3.11) |
| N6: actions write and unseen script calls | minor | Wording and residuals (section 3.5) |
| N7: rerun after `main` moves | minor | Patch-id matching (section 3.4) |
| N8: `--max-failures` per shard | minor | Section 3.1 |
| N9: empty shards | minor | Section 3.1 |
| N10: skipped matrix naming | minor | Real full and docs fixtures (section 3.1) |
| N11: evidence input validation | minor | Section 3.5 |
| N12: `change` logs | minor | Section 3.7 |
| N13: provisional limit, evidence during gates | minor | Section 3.6 |
| N14: owner-pushed harness branches | minor | Section 3.10 |
| N15: generated settings | minor | Masking, format, reserved names (section 3.9), AC-4 |
| N16: security fixes before deployment | minor | Private route (section 3.8) |
| N17: evidence branches | minor | Section 3.5 |
| N18: local gate rot | minor | A local gate at each slice review (section 3.9) |

Last re-review an agent may run, of revision 3:

| Finding | Severity | Resolution |
|---|---|---|
| N3: the open series was not visible to `ci` and `claim` | major | Lock state `evidence`, `evidence-record`, hook gating by state and before tree, `release --evidence` (section 3.5) |
| N4: catch-up commit not on GitHub; transitions | major | `claim --catch-up`, catch-up branch push and dispatch forms, `fallback --from-claim`, stalled runs named, owner-only bypass ruleset (section 3.9) |
| N7: reruns across bases | major | Claim lines in the merge-gate log, lookup by change name and patch-id across bases (section 3.4) |
| New 1: new jobs missing from the expected sets | major | Every job key listed per scope; a test ties them to `gate.yml` (section 3.1) |
| N12: `change` logs | minor | `--logs` fetches every job of a `change` run (section 3.2) |
| N2 notes: lock states and takeover | minor | Section 3.4 |
| New 2: per-shard selection | minor | Matrices from `prepare`'s outputs (section 3.5) |
| New 3: open or later attempts | minor | Refused unless a row names them (section 3.4) |
| New 4: catch-up and the push rule | minor | Finish-equivalent line (section 3.9) |
| New 5: token variables | minor | Section 3.2 |
| New 6: rebuilt candidates and force pushes | minor | New branch name on every rebuild (section 3.8) |
| New 7: job counts | minor | Section 3.1 |
| New 8: dispatch during a gate | minor | Hook asks while `state=dispatched` (section 3.4) |

That reviewer was the last an agent runs on its own under `AGENTS.md`. Its four major findings are fixed, and a further review is the owner's decision (Q9).

Owner-requested review of revision 4 (Q9-A):

| Finding | Severity | Resolution |
|---|---|---|
| R4-M1: a plain `release` ends an evidence series | major | `release` refuses `evidence`; no second series for one failed run (section 3.5) |
| R4-M2: gap before the after-fix gate | major | Atomic `claim --after-fix` from `evidence` (section 3.5) |
| R4-M3: no stable change identity; no run IDs | major | `--change-id` with derived rebuild names; `release --run`/`--no-run`; `ci-fetch` reads the claim log (section 3.4) |
| R4-M4: job names unobservable before the merge | major | Derived fixtures, PM-0 confirmation, empty-matrix guard (section 3.1) |
| R4-M5: owner bypass may cover the agents' token | major | Step 1 check; disable and re-enable instead of a bypass if so (section 3.9) |
| m1: counts and the skipped `evidence` job | minor | Section 3.1 |
| m2: shard counts tied by a test | minor | `ci-matrix` (section 3.1) |
| m3: lock schema and transitions | minor | State table (section 3.4) |
| m4: repeated dispatch before a record | minor | `series-open` (section 3.5) |
| m5: unchecked recorded outcome | minor | `evidence-record` checks identity and derives reproduction (section 3.5) |
| m6: `XDG_CONFIG_HOME` and wording | minor | Section 3.2, AC-1 |
| m7: catch-up scan range, stalled run, revert | minor | Section 3.9 |
| m8: evidence job timeout | minor | Section 3.5 |

- **Q9. A further plan review.** The last reviewer an agent may run on its own found four major findings, fixed in revision 4: the evidence series held by lock state, the catch-up push and dispatch, attempts across bases, and the new job in the expected sets.
  - **A (recommended):** one more fresh reviewer, standard and security, scoped to revision 4's changes, before approval.
  - **B:** no further review; the readiness reviews of the implementation will cover it.
- **Q10. A further plan review.** The owner-requested review of revision 4 found five major findings, fixed in revision 5:
  - lock states that a plain `release` could end;
  - an atomic `claim --after-fix`;
  - a stable change ID and run IDs on claims;
  - derived fixtures confirmed after the merge;
  - the ruleset bypass and the agents' token.

  The answers:
  - **A (recommended):** approve revision 5 now. The implementation's readiness reviews (standard and security) check the state machine against real code, which a further review of the text cannot.
  - **B:** one more fresh reviewer over revision 5's changes.

## 13. Implementation deviations from revision 5

Recorded at readiness, 2026-11-27, for the owner's approval with the change:

- **Settings check with a read-limited token.** `tools/check-github-settings.sh` reports unreadable answers as unverified instead of failing or claiming a full match, as section 4 lists; requested 2026-11-27 for the owner's read-limited token. `docs/development.md` and T-49 say so.
- **Evidence selection bound to the claim.** `claim --evidence` takes the selection (`--kind`, `--package`, `--test`, `--project`, `--title`, `--entry`) and writes it into the lock as `selection=kind:…|package:…|test:…|project:…|title-sha256:…|entry:…`; the hook allows only an evidence dispatch whose inputs encode to it; the evidence job prints one `evidence selection: …` line after its validation; `evidence-record` refuses a log whose selection line differs, and the evidence claim, open and record lines carry it.
- **Failed run tied to its claim.** `claim --evidence` requires an ended claim line in the merge gate log whose run is `--failed-run`, of the same change ID, and, when that claim was on the current `main`, with the before tree as its tree.
- **Harness result lines.** Each evidence run prints `evidence run k/n nonce: <hex>` before its output and `evidence result: run=k/n outcome=… nonce=<hex>` after commands resume, the nonce drawn after the run ends; `evidence-record` reads the log backwards from the final `evidence series …` summary line, accepts only result lines whose nonce the harness printed before that run's output, checks the summary's counts, and refuses to record, leaving the run to the owner, when the run or its evidence job concluded cancelled or timed out or contradicts the outcomes. A log without the summary line records every run as one that ran no test.
- **Recorded runs of earlier claims checked.** `ci` refuses a run recorded on an earlier claim unless its title names that claim's head, its `head_sha` is that claim's base and its path, event and repository match; for every related claim on another base, with or without a recorded run, `ci-fetch` fetches that base's history and `ci` counts every gate run titled with the claim's head.
- **Go and Playwright evidence matching.** A Go run counts as one that ran a test only with a `--- PASS` or `--- FAIL` line naming the selected test, so `--- SKIP` is no test. The Playwright `-g` pattern is anchored as `(^| )<escaped title>( @tag)*$`, because Playwright matches it against the title path joined with spaces and followed by the tags (checked in Playwright 1.63's runner), not joined with `›`; the project is escaped in the recorded-line patterns.
- **pnpm through corepack.** Every job that runs `corepack enable` then installs the candidate's pinned `packageManager` (`pnpm@<version>+sha512.<hex>`, refused in any other form) with `corepack install --global`, because corepack otherwise resolves its own default pnpm from the workspace root, which has no `package.json`, and pnpm then refuses the project's pinned version; `tools/check-workflows.sh` requires the install step in every such job.
- **Smaller fixes.** `ci-matrix`'s `--jobs` length check works on macOS bash 3.2; the `change` scope's `jobs` input accepts digits in job names; an evidence `check` selection passes `--base` only for an entry that scans a range; the hook reads only `finish:` lines with exactly `record`, `head`, `tree` and `result=passed`, accepts `<change-id>-r<k>-before` and `-after` evidence branches, scans commit messages and the diff separately and checks author and committer emails, and follows `pnpm exec`, `pnpm dlx` and `npx` into the program they run; `fallback --reason` refuses an equals sign.

Readiness re-review, 2026-11-27: the hook's check of author and committer emails, added during this change's readiness fixes, is dropped. Every commit in the history carries the owner's real address, which is already public, so the check made the hook ask on every push without protecting anything new. The push scan covers commit messages and added lines only.
