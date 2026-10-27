# Plan: B0, concurrent lanes in the merge gate

Revision 2, after a security-profile review of revision 1: the golangci-lint lock conflict between lanes is resolved; lanes run in their own process groups and are stopped and waited for before the lock is released; one tagged command list and one pass invariant replace the separate lane arrays; D46 is amended to measure the gate's wall time, with summed command seconds still recorded; the tool-binary check runs alone first; the log folder, the writer and reader inventory, the rerun rule under contention and the missing document updates are specified.

## 1. Problem

Slice 5's decision D46 requires the full local check run, measured cold on an idle machine, to take at most 149 s before change S2 starts. With the template databases, shared seeded actors, prebuilt scanning tools and end-to-end account seeding merged, it takes 178.7 to 186.7 s summed over commands run one after another: `pnpm e2e` 86 s, `go test -race` 42 s, the other 28 commands about 50 s. `go test` is latency-bound (about 30 s of wall time for 53 s of CPU on 12 cores in isolation). Options inside the suites reclaim at most 8 to 25 s together. On 2026-10-27 the owner chose concurrent lanes in the merge gate, with D46's figure measured as the gate's wall time.

## 2. Outcome

- **One tagged list.** `tools/merge-gate.sh` keeps one ordered list of the full run, each entry tagged with its slot: `pre`, `A`, `B`, `C` or `e2e`. The report order is that list's order, and a self-test proves that the slots together hold every command of the full run exactly once.
- **Slots.**
  - **pre:** `tools/check-tool-binaries.sh`, alone, before anything else, so every later use of `bin/` (including the lint rule test inside `go test`) runs verified binaries.
  - **A:** `go test -race -count=1 ./...` through `tools/dev-env.sh`.
  - **B:** Gitleaks over history and staged changes, the four OSV-Scanner scans, the documentation tools' install and `tools/docs/check-docs.sh`, `go mod tidy -diff`, `gofmt`, `go vet`, golangci-lint, govulncheck, `sqlc diff`.
  - **C:** `pnpm install --frozen-lockfile`, `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `check:bundle`, `api:check`, `audit`, the secret guard hook check, `tools/check-dev-env.sh`, `tools/check-merge-gate.sh`.
  - **e2e:** `pnpm e2e` through `tools/dev-env.sh`, alone, only after `pre` and all three lanes passed and every lane process has ended.
- **golangci-lint lock.** `TestProjectLintRulesFire` in lane A runs `bin/golangci-lint` while lane B runs it on `./...`, and golangci-lint refuses a second concurrent run by default. `backend/.golangci.yml` sets `run.allow-serial-runners: true`, so a second run waits for the first instead of failing. This changes no linter, rule or setting that decides findings; the development guide records the reason.
- **Lane runner.** Each lane runs its commands in order and stops at its first failure. It never relies on `set -e`: each command runs as `command || status=$?`, and after each command the lane appends one record `index|exit|seconds` to its own record file (revision 3: the lane's leader writes `slot|index|exit|seconds` to an anonymous pipe instead, see the revision 3 deviations below). Lanes run in their own process groups (`set -m`).
- **Pass invariant.** The gate passes only when all of these hold: every `pre`, `A`, `B` and `C` entry of the list has exactly one record, with a numeric exit of 0, reported by the leader process of the lane its tag names (revision 3 wording: a record file could be rewritten by any command, so "written by the lane" was not something the gate could prove); every lane's `wait "$pid" || status=$?` returned 0; and the `e2e` entry has a record with exit 0. A missing, partial, non-numeric or duplicate record, a record for an index outside its lane, or a non-zero `wait` is a failure. A failing lane lets the other lanes finish; the gate then fails without running `e2e`.
- **Interruption.** On INT, TERM or exit, the gate sends TERM to every lane's process group, waits for all of them, and only then releases the lock. The lock records the lane process groups; a lock counts as stale only when neither the gate's pid nor any recorded lane group is alive.
- **Clean tree.** The clean-tree re-check and the hash re-check run after every lane has been waited for, before `e2e`, and again after `e2e`, as today.
- **Logs.** Each command's output goes to `<index>.log` in a folder made with `mktemp -d` inside the lock folder (folder 0700, files 0600, `umask 077` in the lanes), never in the working tree or a served folder, and not exported to the commands. The report names the folder and prints the last 40 lines of each failed command's log. A gate removes the log folders of its earlier attempts that are older than 7 days. T-19 records that the output of commands run through `tools/dev-env.sh` now stays on disk, inside the owner-only lock folder.
- **Report.** Every command in list order with its slot, exit status and seconds; each lane's wall time; the summed command seconds; and the gate's wall time.
- **Unchanged:** the docs-only list (sequential), the self-test list's rules, the lock, clean tree, hashes, attempt log and `finish`, the commands themselves, NFR-6.2, the change checks (which implementing agents keep running one after another, outside the budget figure).
- **D46 amended** in `knowledge/plans/slice-5.md`, pending owner review: from 2026-10-27 the full run's budget figures (D46's 149 s, the 178 s target and the 180 s threshold) are the merge gate's wall time, cold, on an idle machine; every check-budget record also reports the summed command seconds (comparable with earlier records), the wall time per lane, the machine's core count, and what "cold" covered (the Go build, test and lint caches emptied; the pnpm store and `node_modules` kept, as in earlier records).

## 3. Tier and security trigger

- **Tier: standard** (slice 5 change B0).
- **Security trigger: met** (the merge gate and verification policy, T-41; command logs, T-19; e2e exclusivity, T-36). Standard and security-profile reviewers; this plan had a security-profile reviewer before the owner's approval.

## 4. Writers and readers that overlap

| Writer | Reader running at the same time | Handling |
|---|---|---|
| Lane C `vite build` writes `frontend/dist` and `frontend/build-report` | Lane A's conventions walk of the repository (skips `node_modules`, `frontend/dist`, `test-results`, not `build-report`) | The walk reads only source extensions it checks; a file that appears or disappears fails closed; recorded as residual |
| Lane C `pnpm install` and lane B's documentation tools' install | The shared pnpm store | pnpm locks the store for concurrent installs |
| Lane C `api:check` (`go run ./cmd/api`), lane B `go vet`, golangci-lint, govulncheck | Lane A's builds | The Go build cache is safe for concurrent use |
| Lane B golangci-lint | Lane A's lint rule test | `allow-serial-runners` (section 2) |
| Lane C `check:bundle` (`git status`) | Lane B Gitleaks over staged changes (index) | Gitleaks only reads the index; `git status` may refresh the index's cached file information opportunistically, which never changes the staged content Gitleaks scans (wording corrected in revision 3) |
| Lane B `check-docs.sh` | Untracked Markdown | The clean tree holds none |
| Lane A PostgreSQL | No other lane | — |
| `e2e` ports, run lock and bucket | No lane: `e2e` starts after every lane ended | — |

## 5. Files

1. `tools/merge-gate.sh`: the tagged list, `pre`, lanes in process groups, record files, the pass invariant, interruption handling, lock fields, logs, report.
2. `tools/check-merge-gate.sh`: self-tests, each inducing its failure explicitly and waiting on conditions, never `sleep`: a failing first command in lane A, B and C each fails the gate and names the command; a failure in one lane lets the others finish and be reported, and `e2e` does not run; a lane killed with SIGKILL before its record is written fails the gate; a failing `e2e` after all lanes pass fails the gate; a missing, duplicate or non-numeric record fails it; the slots together equal the full list, each command once; TERM sent to the gate leaves no lane process alive and releases the lock only afterwards; a stale lock whose lane group is alive is not reclaimed; the log folder and files have modes 0700 and 0600 and lie outside the repository; the docs-only and self-test lists produce the same report fields and lock as before.
3. `backend/.golangci.yml`: `run.allow-serial-runners: true`.
4. `docs/development.md`: "Tool setup" (slots and the budget figure), the merge gate's "Checks", "Failures" and "Merge report" bullets, the paragraph listing what `tools/check-merge-gate.sh` proves, the golangci-lint setting's reason, the change-check row note that `tools/check-tool-binaries.sh` runs first; status line; pending owner review.
5. `docs/decisions/0018-targeted-checks-and-merge-gate.md`: the lanes as an amendment and "Where it is applied"; pending owner review.
6. `docs/threat-model.md`: T-41 (the pass invariant and the interruption rule; residuals: lanes share the machine, the conventions walk reads while lane C writes), T-19 (command logs kept 7 days in the owner-only lock folder), T-36 (`e2e` runs only after every lane ended); status line.
7. `knowledge/reviews/ai-development-setup.md`: the gate's run order.
8. `knowledge/plans/slice-5.md`: D46's amendment and B0's measured result.
9. `knowledge/analysis/check-budget-2026-10-27-merge-gate-lanes.md`: three cold gate runs with the figures of section 2's last bullet.

## 6. Acceptance criteria

- `tools/check-merge-gate.sh` exits 0 with every assertion of section 5, item 2, each printed as its own `ok` line.
- Three cold gate runs on an idle machine, with the golangci-lint setting in place, each pass and each report a gate wall time of at most 149 s; the check-budget record shows them with the summed seconds, lane times, core count and cache state.
- No test fails in any of the three runs. A test that fails under contention is a flaky-test bug fixed at its cause; an error caused by contention between lanes (for example `too many clients` or a lock error) is a gate bug, not an allowed infrastructure rerun.
- `tools/docs/check-docs.sh` and Gitleaks over staged changes pass, and `rg -n -F "allow-serial-runners" backend/.golangci.yml docs/development.md` finds both; the merge goes through the gate.

## 7. Order, risks and checks

- Order: golangci-lint setting → script and self-tests → documents → three cold runs, alone on the machine → record and D46 amendment → standard and security-profile reviewers → fixes → gate and merge.
- **Risk: a collection bug passes a failure.** The single invariant and the induced-failure self-tests guard it.
- **Risk: contention makes deadline-based tests flaky** (listener heartbeat, process start timeouts). The three cold runs check it; if `go test` grows past about 55 s, the fallback is more parallel `startup` tests, measured separately.
- **Risk: orphaned lanes** after an interruption. Process groups, the interruption rule and the lock's lane groups guard it, with self-tests.

Approved by the owner on 2026-10-27 at revision 2.

## Revision 3 deviations

Approved by the owner on 2026-10-27. The implementation differs from revision 2 in these points, found while implementing it and in its standard and security-profile reviews:

- **`umask 077` only while creating files.** The gate creates each log under `umask 077`; the commands keep the caller's `umask`, because a 077 mask would change the modes of build outputs such as `frontend/dist`. Logs are still 0600 and the folder 0700.
- **`--checks self-test-lanes`.** A second self-test list of harmless tagged commands run in lanes, so `tools/check-merge-gate.sh` can exercise the lanes; it has the self-test list's refusals, and `finish` refuses it. The sequential self-test list and the Markdown-only list share the earlier sequential path.
- **Lock folder inside the checkout refused.** The gate refuses a lock folder whose canonical path lies inside the checkout or any of whose existing ancestors is the checkout's root (`-ef`), which also catches a case variant on a file system that ignores case.
- **`python3` for timing.** The Bash 3.2 that macOS ships has no `EPOCHREALTIME`, so the gate reads fractional seconds from `python3`, which the development guide already requires.
- **Record pipe instead of record files.** Record files in the log folder could be rewritten by any command of the same user, so the gate could pass a failed command. Each lane's leader writes `slot|index|exit|seconds` after each command and `slot|done|seconds` at its end to an anonymous pipe: a named pipe the gate opens read-write and unlinks at once, inherited by the leaders. Every command runs with that descriptor closed. After the leaders ended, the gate writes an end marker and reads the records into memory. Processes a lane leaves behind get the grace period to end; any that remain fail the gate, and they cannot report records. On Linux a lane command, or any same-user process, can still open the pipe through `/proc/<pid>/fd/` of its leader (for example `/proc/$PPID/fd/9`), read a failed record away and write consistent forged ones, so a failed check can pass; the guarantee holds on macOS, and on Linux the pipe guards only against accidental writes. The gate's validation of the records it receives is tested with crafted records, by sourcing the script, whose dispatch runs only when it is executed.
- **`--lane-grace SECONDS`.** Accepted only with the two self-test lists, it replaces the 30-second wait for processes a lane leaves behind and the 20-second TERM and 10-second KILL waits, so the self-tests prove the wait and KILL paths deterministically in seconds.
- **Report and attempt log.** The report gives each lane's wall time from the start of its phase to its leader's end, each lane's command seconds and each phase's wall time including the wait for its processes; an interrupted or stopped gate prints its log folder and appends its attempt to `merge-gate.log`.
