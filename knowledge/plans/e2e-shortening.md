# Plan: shorten the end-to-end suites on the merge gate's critical path

Status: revision 3, written on 2026-10-29 against `main` at `3150810`, and closed by the owner's decision of 2026-10-29 (section 9): items D(a), E' and F are not done now. The suites' CPU cost is worked out in [proposal 0004](../../docs/proposals/0004-end-to-end-suite-cost.md), and D46's threshold is 260 s until the end of slice 6, with the 165 s target kept ([slice 5 plan](slice-5.md), D46, pending owner review). No harness or gate change follows from this plan. Revision 2 was approved by the owner on 2026-10-28 with items A and B (section 9). Its profile and measurements, recorded in the [end-to-end profile record](../analysis/check-budget-2026-10-29-e2e-profile.md), showed that neither item shortens the suites, so neither was adopted and no harness change was made. The owner then chose to reduce the work of the suites rather than add parallelism. This revision records A and B as measured and not adopted, and proposes the migrated template (item D(a)), builds off the critical path (item E') and an optional investigation of a Firefox stall (item F), with the gains the measurements predict for each.

## 1. Problem

The merge gate runs `pnpm e2e` alone after lanes A, B and C, so the suites set the gate's critical path. The gates since the end of slice 5's planned changes took 187 to 252 s, with the end-to-end step at 2.1 to 2.5 minutes, against D46's 165 s target and 180 s threshold ([slice 5 gates record](../analysis/check-budget-2026-10-29-slice-5-gates.md), [slice 5 plan](slice-5.md)). Those gates ran with warm Go caches on a loaded machine; cold gates on an idle machine are still to be measured.

The suites now hold 897 tests: 326 in Chromium, 285 in Firefox and 286 in WebKit. On an otherwise idle machine at `3150810`, three runs with Playwright's default of 6 workers and database pools of 5 took 133, 124 and 119 s (mean 125 s, spread 14 s). Of that:

- from the command's start to the first test, global setup with its builds included, took at most 3.8 s, and at most 1.5 s once the worktree's first build had run;
- the tests ran for 117 to 128 s;
- teardown after the last test took 0.6 s or less.

The machine averaged about 850 % of its 1200 % CPU during the tests: Firefox about 30 %, Docker's virtual machine (PostgreSQL and storage) about 15 %, Playwright's runner and test workers about 13 %, WebKit about 11 %, Chromium about 8 %, `vite preview` about 6 % and the API processes about 4 %. Wall time is set by CPU-bound test execution, not by setup.

## 2. Measured and not adopted

**A. More workers with smaller pools.** Measured at `ba042d8` with `workers` fixed in `frontend/playwright.config.ts` and `OVC_DB_POOL_SIZE` set to 3 in `frontend/e2e/deployment.ts`, two runs per configuration, all passing:

| Configuration | Wall time | Mean | Peak connections |
|---|---|---|---|
| 6 workers, pools of 5 (`main`) | 147 s, 141 s | 144 s | 51 |
| 8 workers, pools of 3 | 144 s, 128 s | 136 s | 41 |
| 9 workers, pools of 3 | 144 s, 136 s | 140 s | 46 |

Summed test time grows in step with the worker count, because the machine is already saturated at 6 workers. The 8-worker gain does not exceed the spread of its runs, so the rule of section 4 drops it. Pools of 3 are safe: no failure, no `too many clients`, and a lower peak. They save no time, so they are not adopted either. Production pool defaults are untouched.

**B. Firefox's per-test overhead.** The one-worker profile without traces split each test's time:

- **Polls:** `expect.poll` waits took at most 4.5 s summed per engine, so shorter intervals could gain at most a fraction of that.
- **Fixtures:** the `diagnostics` fixture took about 2 ms per test, and context creation 13 to 24 ms in Firefox.
- **Firefox's excess:** Firefox creates a page in about 0.10 s, as WebKit does, and its test bodies run about 0.13 s slower per test than Chromium's. These are engine CPU costs that no harness option removes.

No poll interval, context option or fixture was changed. Item F below covers the one removable Firefox cost the profile found.

## 3. Proposed items

**D(a). Migrated template database.** Every deployment runs `api migrate` on a fresh database, 18 times per run. The template is migrated once in the global setup and every deployment's database is cloned from it with `createdb -T`. The constraints of revision 2 stay:

- its name ends in `_test` and can never match the Go fixture's cleanup pattern `ovc_template_<secs>_<rand>_test`;
- it is created only after `readE2ESettings`' loopback checks, migrated by `api migrate` with the deployment's test-control environment, and closed to every connection before it is cloned;
- the global teardown drops it under `assertStillHeld`, and only the run-lock holder removes stale templates of crashed runs;
- it adds no route and no setting.

Measured basis: the 18 migrations of one run took 2.72 s summed (median 0.15 s), spread over 6 workers. Expected gain: about 0.5 s of wall time, less the cost of each clone.

**E'. Builds off the critical path.** The global setup builds the `api`, `worker` and `ovc-admin` binaries with `go build` and the frontend with `pnpm build` before any test starts. Measured alone, `go build` takes 6.3 to 6.8 s from an empty Go build cache and 0.2 s warm; `pnpm build` takes 0.3 s, and 2.5 s on a worktree's first build. In every option the artefacts the suites test must be built from the tested tree, and no option may make it possible to test anything else.

- **Option 1, warm the cache in a lane (recommended if E' goes ahead).** The gate adds one lane B command, run from `backend/`: `go build -o <a folder of the gate's own>/ ./cmd/api ./cmd/worker ./cmd/ovc-admin`. The end-to-end global setup keeps building both from the tree exactly as now, so it still produces every artefact it runs, from the tree under test. The lane only fills Go's build cache, whose entries are keyed by content, so a cache hit cannot yield a binary of other sources. Expected gain: up to about 6.5 s in a cold gate, if lanes A and B do not already leave a non-race build of the three commands in the cache; nothing in a warm gate. Lane B grows by up to 6.8 s, which stays off the critical path while lane B ends before the end-to-end step would start.
- **Option 2, reuse artefacts by content hash.** The harness skips its builds when a recorded hash of their inputs matches. Expected gain: at most about 1.5 s, which Go's own cache already gives. Not recommended: a stale or wrong record would make the suites test artefacts not built from the tested tree, and the harness would need a new file it trusts.
- **Option 3, build in a lane and hand the artefacts over.** The gate builds into a folder of its own and passes it to the end-to-end step through a harness-internal variable, as it passes the run lock. Expected gain: the same as option 1. Not recommended: it adds an input to the end-to-end step that the step must verify against the tested tree (T-41), for no gain over option 1.

**F. Optional: Firefox's growing context-close stall.** Playwright's Firefox build runs `minimizeMemoryUsage` on every tenth context close. In the suites the stall grows with the number of contexts a browser has opened: from 0.5 s to 7.2 s within 111 tests at one worker. A bare page or the production build without an API shows a flat 0.3 s. The item is an investigation first, with no committed change: find what the tests leave in the browser that makes each stall longer. Then propose a change only if a removable trigger is found, through a test that makes the trigger visible rather than through a weaker check. At 6 workers each Firefox worker has about four stalls. Expected gain: up to about 4 to 5 s of wall time, and only if the growth can be removed; the base 0.3 s per stall belongs to Firefox and stays.

**Combined.** The three items together promise at most about 12 s. Each item alone stays below the 14 s spread of the baseline runs, so a single item may be impossible to prove with three runs. Together they cannot bring gates of 187 to 252 s under 165 s. Section 10 asks the owner how to proceed.

## 4. Measurement

- **Before and after:** three cold gate runs at `main` and three after the change, on an idle machine with no other agent running, cold as in the [critical path record](../analysis/check-budget-2026-10-28-gate-critical-path.md). A new check-budget record states the measures that revision 2 named for every run: gate wall time, each lane, the `pnpm e2e` step, summed command and test seconds per engine, the core count, load averages, what the cold run emptied, and the peak of client connections. The sampler runs through `tools/dev-env.sh` and writes only counts and times.
- **Each item's own gain:** three `pnpm e2e` runs with the item and three without it. An item whose mean gain does not exceed the spread of its runs is dropped, as in the [critical path plan](merge-gate-critical-path.md).
- **E':** its gain shows only in a cold gate, so it is measured from the gates' `pnpm e2e` steps.

## 5. Tier and security trigger

- **Tier:** standard. One fresh standard reviewer over the complete diff of each implementing change.
- **Security trigger:** met.
  - T-36: item D(a) changes the end-to-end deployments that run with the test control surface.
  - T-41: item E' changes the gate's command list, and every item changes the evidence of the gate's critical path. E' option 1 adds no input to any check and no new trust boundary; options 2 and 3 would add one.
  - T-20 examined and not affected: production pool defaults are unchanged.
- **Reviews:** a security-profile reviewer reviews this revision before approval and each implementing diff before readiness. No threat-model entry changes unless E' option 2 or 3 is chosen, in which case the change adds a T-41 mitigation for the trusted artefact record or folder.

## 6. Files

- **This revision:** this plan and the [end-to-end profile record](../analysis/check-budget-2026-10-29-e2e-profile.md).
- **D(a):** `frontend/e2e/deployment.ts`, `frontend/e2e/global-setup.ts` and `frontend/e2e/global-teardown.ts`; `docs/development.md`, the end-to-end harness section (pending owner review).
- **E' option 1:** `tools/merge-gate.sh` (its check list); `tools/check-merge-gate.sh`, only if its self-tests name the list; `docs/development.md`, the lane B list under tool setup and the gate's description (pending owner review).
- **F:** a record in `knowledge/analysis/` first; any change it leads to needs its own plan amendment.
- **Each implementing change:** its check-budget record in `knowledge/analysis/`.

## 7. Acceptance criteria

For every implementing change:

- `cd frontend && pnpm exec playwright test --list` prints the same lines before and after, ending in `Total: 897 tests in 23 files`, or the count `main` holds when the change starts.
- `git diff main -- frontend/ | grep -E '^[-+][^-+].*(timeout|retries|grep|testIgnore|grepInvert|workers)'` prints nothing.
- `grep -o 'expect(' frontend/e2e/*.ts | wc -l` and `grep -o 'expect.poll(' frontend/e2e/*.ts | wc -l` print the same numbers before and after.
- Three consecutive `tools/dev-env.sh pnpm e2e` runs pass with no failed test and no `too many clients`. Their sampled connection peak is no higher than the baseline's 43 and at most 80.
- The item's mean gain over three runs with and three without it exceeds the spread of those runs (section 4); otherwise the item is dropped and its branch is not merged.
- `tools/docs/check-docs.sh` passes, and the merge goes through the gate.

For D(a):

- After a full run, `ls frontend/test-results/deployment/migrate-*.log` lists exactly one file newer than the run's start, against 18 today.
- After the global teardown, through `tools/dev-env.sh`, `psql -X -At -c "select count(*) from pg_database where datname like 'ovc\_e2e\_template\_%'"` against `OVC_TEST_DATABASE_ADMIN_URL` prints `0`.
- The template's name matches `^ovc_e2e_template_[a-z0-9]+_test$`, and the Go fixture's cleanup pattern `^ovc_template_[0-9]+_[a-z0-9]+_test$` does not match it. A unit test in `frontend/e2e/` proves both.

For E' option 1:

- `git diff main -- frontend/` prints nothing, and `grep -c "execFileSync('go', \['build'" frontend/e2e/deployment.ts` prints `1` before and after: the end-to-end step still builds what it runs.
- `git diff main -- tools/merge-gate.sh` adds exactly one check-list line, tagged `B` and running `go build -o` with the three commands into a folder under the gate's own temporary folder, and removes none.
- `tools/check-merge-gate.sh` passes.
- In three cold gates, lane B still ends before lane A or C, as the gate's lane wall times show.

For F: a record in `knowledge/analysis/` names the trigger of the growth with a reproduction script, or states that none was found. No file outside `knowledge/` changes.

## 8. Order, risks and checks

- **Order:** owner decisions (section 10); then E' option 1 and D(a) as separate changes, each with its reviews, gain measurement and gate; F only if chosen, starting with its record; then three cold gates and the record.
- **Testing artefacts not built from the tested tree:** excluded by choosing option 1, where the end-to-end step keeps its own builds from the tree, and by the E' criteria above.
- **Template drift (D(a)):** a template migrated by an earlier run's binary. It is created by the run's own `api migrate` after the run lock is taken, and dropped by its teardown.
- **Gains below the noise:** the rule of section 4 drops an item that cannot be shown. The owner question in section 10 addresses that every item is small.
- **Not changed:** NFR-6.2, D46's target (its threshold changes only through the owner's decision recorded in section 9), ADR-0018, the retries of 0, production pool defaults, the worker count and the per-deployment pools.

## 9. Owner decisions

- 2026-10-28: items A and B approved. Item C (concurrent engine phases) not done, because the phases are already packed. Item D limited to the migrated template, and only if A and B miss 165 s. No batched seeding route.
- 2026-10-29: after A and B were measured without a gain, the owner chose to reduce the work of the suites. This revision follows from that.
- 2026-10-29, on this revision's questions:
  - none of D(a), E' and F is done now (question 1, option (d));
  - the suites' CPU cost goes to a proposal, [proposal 0004](../../docs/proposals/0004-end-to-end-suite-cost.md), which takes item F as one of its inputs (question 2, option (a));
  - D46's threshold is 260 s of the gate's wall time until the end of slice 6, and the 165 s target stays;
  - question 3 is moot.

## 10. Questions for the owner

Answered on 2026-10-29; section 9 records the answers.

1. Which items of section 3 go ahead?
   - (a) E' option 1 only.
   - (b) E' option 1 and D(a).
   - (c) D(a), E' option 1 and the investigation F.
   - (d) None for now; the suites are CPU-bound, and the next step is question 2. **Recommended:** each item is below the spread of the runs, and all three together cannot reach 165 s.
2. How is D46's target met for suites that are CPU-bound and growing with every slice?
   - (a) Keep the target and have a proposal in `docs/proposals/` examine the suites' CPU cost per process family, using the CPU table of the [end-to-end profile record](../analysis/check-budget-2026-10-29-e2e-profile.md), with item F as one of its inputs. **Recommended:** it addresses where the time goes, and drops no test, engine or assertion.
   - (b) Revisit D46's target and threshold with the owner, using the cold gate figures once measured.
   - (c) Both (a) and (b).
3. If item E' goes ahead, which option?
   - (a) Option 1, warm the cache in a lane. **Recommended:** no new input to the end-to-end step, and the artefacts are always built from the tested tree.
   - (b) Option 2, reuse by content hash.
   - (c) Option 3, hand over artefacts built in a lane.
