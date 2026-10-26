# Plan: targeted checks per change, one full check run at merge

Revision 2, after a security-profile review of revision 1: the gate is bound to tree and base hashes and to a per-user lock; the rerun rule is limited to named infrastructure errors; the docs-only exception excludes Markdown read by tests; ADR-0009, the roadmap and the security scanning section are added; this change keeps today's rule for itself; the change-check table is widened; the statuses, the merge report and exact-phrase criteria are defined; T-41's residual is completed.

## 1. Problem

Every agent that implements a change runs the full local check run (`docs/development.md`, "Tool setup"; 186.7 s cold on an idle machine, several minutes under load) before it reports readiness, and again after each fix. When several agents work at once, their runs compete for the shared PostgreSQL, Docker, the per-user end-to-end run lock and CPU, so runs fail for environmental reasons or take far longer than measured. The full run's value is a green `main`; running it in every implementing context does not add to that.

## 2. Outcome

- An implementing agent runs the **change checks**: the checks of what its diff touches, from a table in the development guide, and reports them as its readiness evidence. Checks it did not run carry the new status `deferred to the merge gate`, never `passed`.
- The coordinating agent runs the **merge gate** before every merge into `main`: the full local check run on the exact tree that `main` will hold, bound to hashes as in section 5. A merge happens only when the gate passes; a failure sends the change back to its branch, and its fix follows the existing after-fix rule.
- `main` receives only commits on which the full run passed, except Markdown-only changes under the exception of section 5, if the owner approves it.
- Tiers, reviewers, the security trigger, the after-fix rule, the slice review, NFR-6.2's budget and every check's content are unchanged.

## 3. Tier and security trigger

- **Tier: standard.** One change to verification policy and agent configuration.
- **Security trigger: met** (verification policy, `AGENTS.md`, T-41). This plan has had a security-profile reviewer; the change gets a standard and a security-profile reviewer and the owner's explicit review.
- **This change keeps today's rule for itself:** until the owner approves the reduction, it runs the full run at readiness and again at the merge gate.

## 4. Change checks (the table that lands in `docs/development.md`)

Before any row that runs a scanner from `bin/`, `tools/check-tool-binaries.sh` runs. Any path not listed, or any doubt, means the full run.

| The diff touches | Change checks |
|---|---|
| Anything | Gitleaks over staged changes; `tools/docs/check-docs.sh` when any Markdown file, `.markdownlint.json` or the link-checker configuration changes |
| `backend/` Go code | `gofmt`, `go vet ./...`, golangci-lint `./...`, and `go test -race` (through `tools/dev-env.sh`) of the changed packages, every package that imports them directly or transitively (from `go list`), `internal/conventions` and every `cmd/` package's tests; `api:check` and the affected end-to-end specs when routes, schemas or behaviour a page uses change |
| `backend/migrations/`, `backend/queries/`, `backend/sqlc.yaml`, `internal/testdb` | `sqlc diff` and the whole `go test -race ./...` |
| `backend/go.mod`, `go.sum`, `backend/osv-scanner.toml` | `go mod tidy -diff`, `govulncheck`, OSV over `backend/go.mod` |
| Shell, YAML, TOML or Python files anywhere, ignore files, `.env.example`, `knowledge/architecture/detailed.md` | `internal/conventions` tests, plus the rows of the folder they live in |
| `frontend/` source | `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `check:bundle`, and the end-to-end specs the change affects, named in its plan |
| The generated OpenAPI document or the generated client | `api:check` and the whole `pnpm e2e` |
| `frontend/pnpm-lock.yaml`, `pnpm-workspace.yaml`, `package.json`, `auditConfig.ignoreGhsas` | `pnpm install --frozen-lockfile`, `audit`, `lint` (release-age and ignored-advisory checks), OSV over the lockfile |
| `tools/docs/` lockfile, workspace or `package.json` | the documentation tools' install, `tools/docs/check-docs.sh`, frontend `lint` (release-age check), OSV over `tools/docs/pnpm-lock.yaml` |
| `frontend/e2e/`, the Playwright configuration, `/__test__/` routes and the test-control settings, the Compose file, `frontend/security-headers.ts`, the Vite configuration, `frontend/public/` | `build`, `check:bundle`, the whole `pnpm e2e`, and the backend tests of the test-control start-up refusals and of the routes' absence without `OVC_TEST_CONTROL=1` |
| `tools/`, `tools/go.mod`, `tools/osv-scanner.toml`, the `bin/` build | `tools/check-tool-binaries.sh`, OSV over `tools/go.mod`, and the checks that use any rebuilt tool |
| `.claude/`, `tools/dev-env.sh`, `.gitleaks.toml` | the secret guard hook check, `tools/check-dev-env.sh`, Gitleaks over history |

## 5. Merge gate

- **Lock.** The gate takes a per-user merge-gate lock, built like the end-to-end run lock (exclusive create, pid, start time, checkout path), so coordinators of parallel sessions never run two gates at once; a second gate fails at once naming the holder.
- **Clean tree.** `git status --porcelain` must print nothing: no unstaged and no untracked files, so the working tree the build and the docs check read is the tested tree.
- **Hashes.** The gate records `main`'s commit at its start and the tested tree: `git rev-parse HEAD^{tree}` on the branch for `--ff-only`, or `git write-tree` of the squashed index for `--squash`.
- **Merge.** Immediately before the merge or the squash commit, `main` must still be the recorded commit, and after it `git rev-parse main^{tree}` must equal the tested tree; otherwise the gate runs again on the new tree. Nothing is edited, restaged or committed between the gate and the merge.
- **Failures.** A failed command blocks the merge. One rerun is allowed only when the output shows a named infrastructure error with no test assertion failing (Docker or PostgreSQL unreachable, `too many clients`), quoted in the merge report; a test that fails and then passes is reported as failed and handled as a flaky-test bug; a second infrastructure failure goes to the owner. A check never turns green by weakening it.
- **One gate per merge,** no batching, so a failure points at one change.
- **Merge report** to the owner after each merge: the change, the recorded base commit, the tested tree hash and `main^{tree}` after the merge (equal), and every command with its exit result.
- **Docs-only exception (owner's choice, section 9).** A change whose diff holds only Markdown under `docs/` and `knowledge/` that no test, script or generator reads gates on `tools/docs/check-docs.sh`, Gitleaks over staged changes and over history, with the same lock, clean tree and hashes. `knowledge/architecture/detailed.md` is excluded, because `backend/internal/conventions/architecture_test.go` parses its package layers table; the development guide names how to find any other such file (`git grep` for the path in tests and scripts), and any file found is excluded too.

## 6. Files to change

1. `docs/decisions/0018-targeted-checks-and-merge-gate.md` (new): context, decision (sections 2, 4 and 5), alternatives (full run in every agent; batched gate; checks only at slice close; a script that derives the change checks, deferred), consequences. Row in `docs/decisions/README.md`. ADR-0011's "Related" and "Where it is applied" link ADR-0018.
2. `docs/decisions/0009-owner-approved-exceptions-and-visibility.md`, item 5: agents merge once change checks pass, readiness reviews are resolved and the merge gate passes.
3. `AGENTS.md`: change workflow, a development-verification bullet for change checks and the merge gate, the report status `deferred to the merge gate`, and the definition of done (done means merged through the gate).
4. `.claude/skills/ovc-verify/SKILL.md`: evidence rules, the new status and the merge report.
5. `docs/development.md`: the section "Change checks and the merge gate" (table, lock, hashes, failures, report, docs-only exception), "Security scanning" (scans pass at the merge gate; their change-check rows), "Local review", "Changes and commits", status line.
6. `docs/requirements/non-functional.md`: NFR-6.1 reads "Strict static checks and tests as defined in AGENTS.md pass in the merge gate before any change merges."
7. `docs/roadmap.md`, line 9: a slice's changes have merged through the merge gate.
8. `docs/threat-model.md`: T-41 records the merge gate; residual: a wrongly chosen change-check set is caught only at the gate, after review; the gate's evidence is reported by the coordinator itself; parallel coordinators are serialised only by the merge-gate lock and the hash check. Status line.
9. The merge-gate lock: `tools/merge-gate.sh` (shell, no comments), which takes the lock, checks the clean tree, records the hashes, runs the full run's commands and prints the merge report; and `tools/merge-gate-finish.sh` or a subcommand that checks `main` and `main^{tree}` after the merge and releases the lock. Covered by a shell test in the style of `tools/check-dev-env.sh`.
10. `knowledge/reviews/ai-development-setup.md`: the active policy summary.
11. `knowledge/plans/slice-5.md`: a note that `tools/check-changed.sh` (deriving the change checks from the diff) is deferred and revisited at slice 5's close.
12. `knowledge/plans/targeted-checks-and-merge-gate.md`: this plan, with the owner's approval.

All new and changed `docs/` wording is marked pending owner review.

## 7. Acceptance criteria

- `tools/docs/check-docs.sh` exits 0; Gitleaks over history and staged changes report no leaks.
- `rg -n -F "merge gate"` finds the rule in `AGENTS.md`, `.claude/skills/ovc-verify/SKILL.md`, `docs/development.md`, ADR-0009, ADR-0018, `docs/roadmap.md`, `docs/threat-model.md` and `docs/requirements/non-functional.md`.
- `rg -n -F "deferred to the merge gate"` finds the status in `AGENTS.md` and the skill.
- `rg -n -F "pass in the merge gate before any change merges"` finds NFR-6.1; `rg -n "0018-targeted-checks-and-merge-gate" docs/decisions/README.md docs/decisions/0011-tiered-verification.md` finds the index row and both ADR-0011 links.
- `git diff --name-only main` lists only files of section 6, and each changed `docs/` file contains "pending owner review" in its changed text.
- The shell test of the merge gate shows: a second gate refused while one holds the lock; a dirty or untracked tree refused; a moved `main` or a differing `main^{tree}` reported as failure.
- This change's readiness report shows a full run that passed, and its merge report shows a second full run through `tools/merge-gate.sh` with equal tested and merged tree hashes.

## 8. Order, risks and checks

- Order: ADR-0018 and ADR-0009 → `AGENTS.md` → skill → development guide → NFR-6.1, roadmap → T-41 → merge-gate scripts and their test → setup summary and slice 5 note; full run; standard and security-profile reviewers in one message; fixes under the after-fix rule; full run; commit on `chore/targeted-checks-and-merge-gate`; the merge through the new gate.
- **Risk: a wrong change-check set** lets a review run on a diff that later fails the gate. The gate catches it before `main`; the table errs wide and any doubt means the full run.
- **Risk: the gate becomes a queue.** Serial gates of about three minutes bound merges to roughly 15 to 20 an hour, far above the current pace; slice 5's B0 shortens the run.
- **Risk: self-reported evidence.** The merge report carries hashes and command results produced by the script, which reviewers and the owner can rerun; recorded in T-41's residual.

## 9. Questions for the owner at approval

- Markdown-only changes: the full run, or the documentation and secret checks under the exception of section 5 (recommended: the exception, with `knowledge/architecture/detailed.md` and any file a test reads excluded)?
- `tools/check-changed.sh` deriving the change checks from the diff: now, or deferred to slice 5's close (recommended: deferred, recorded in the slice 5 plan; the gate backstops a misjudged table)?

Approved by the owner on 2026-10-27 at revision 2, with these answers to section 9: Markdown-only changes gate under the exception of section 5, with `knowledge/architecture/detailed.md` and any Markdown file a test or script reads excluded; `tools/check-changed.sh` is deferred to slice 5's close and recorded in the [slice 5 plan](slice-5.md).

After the standard and the security-profile reviews of the implementation, the merge gate gained, as review fixes within this plan: the check list and the scope recorded in the lock, with the finishing step refusing anything but the full run (or the Markdown-only list for a Markdown-only scope) and the self-test list refused outside a test lock folder and repository; a Markdown-only scope limited to regular files of mode 100644 and a repository-wide search for files that name them; `main`'s copy of the script run from a scratch folder through `--repo`; a stale-lock reclaim that moves the lock aside and confirms it before removing it; refusal of influential ignored files by name; a log of attempts per tested tree, with a held lock no longer counted as an infrastructure error for a rerun; a squash gate that names its branch and checks the staged tree against `git merge-tree --write-tree`; the report printed when the tree changes during the checks; and the Markdown-only exception stated in the definition of done and NFR-6.1.

The change that introduces the gate cannot run `main`'s copy of `tools/merge-gate.sh`, because `main` holds none before it merges. It is therefore gated once by its own copy of the script, after a full run at readiness under the earlier rule, and its merge report records that its gate ran the change's own copy.
