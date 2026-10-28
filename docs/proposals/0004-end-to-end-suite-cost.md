# Proposal 0004: lower the end-to-end suites' CPU cost without losing coverage

- Status: proposed
- Reviewed: direction chosen by the project owner on 2026-10-29 (lower the end-to-end suites' CPU cost without losing coverage, for example by consolidating tests that share their arrangement and moving checks that do not depend on the browser into Go integration tests, with the Firefox stall investigation pursued only as an input to this proposal; none of the shortening plan's items D(a), E' and F now; D46's threshold at 260 s until the end of slice 6 with the 165 s target kept); the text is pending owner review
- Author: coding agent, for the project owner
- Date: 2026-10-29
- Trigger: the owner's decision of 2026-10-29 on the [end-to-end shortening plan](../../knowledge/plans/e2e-shortening.md), with D46's temporary threshold of 260 s in force until the end of slice 6
- Related: NFR-6.2, NFR-6.4, [ADR-0011](../decisions/0011-tiered-verification.md), [ADR-0018](../decisions/0018-targeted-checks-and-merge-gate.md), D46 in the [slice 5 plan](../../knowledge/plans/slice-5.md), the [end-to-end profile record](../../knowledge/analysis/check-budget-2026-10-29-e2e-profile.md), the [slice 5 gates record](../../knowledge/analysis/check-budget-2026-10-29-slice-5-gates.md)

## Motivation

The merge gate runs the end-to-end suites alone after its three lanes, so the suites set its critical path. The gates since the end of slice 5's planned changes took 187 to 252 s, against D46's target of 165 s and its threshold of 180 s. Until the end of slice 6 the owner has set a temporary threshold of 260 s, and the 165 s target stays (pending owner review). Every slice adds end-to-end tests in three browsers, so without a change the gap grows with each slice, and NFR-6.2's 10 minutes come closer.

The evidence is in the [end-to-end profile record](../../knowledge/analysis/check-budget-2026-10-29-e2e-profile.md), measured on a 12-core machine:

- **Tests are the whole cost.** Three runs on an otherwise idle machine took 119 to 133 s for 897 tests: 326 in Chromium, 285 in Firefox and 286 in WebKit. All but about 2 to 5 s of each run is test execution; global setup with its builds takes at most 3.8 s, and teardown 0.6 s.
- **The suites are CPU-bound.** More Playwright workers did not shorten them: 6, 8 and 9 workers took 144, 136 and 140 s on average, within the spread of the runs, because summed test time grows in step with the worker count.
- **Where the CPU goes.** During the tests the machine averaged about 850 % of its 1200 % CPU:

  | Process family | Share |
  |---|---|
  | Firefox | about 30 % |
  | Docker's virtual machine (PostgreSQL and storage) | about 15 % |
  | Playwright's runner and test workers | about 13 % |
  | WebKit | about 11 % |
  | Chromium | about 8 % |
  | `vite preview` servers | about 6 % |
  | API processes | about 4 % |
  | Everything else on the machine | the rest |

- **The cost is spread across tests.** A test costs 0.71 to 0.79 s of summed time on average across the three engines. Over the three baseline runs, Firefox's median test was 0.70 to 0.82 s, WebKit's 0.66 to 0.69 s and Chromium's 0.43 to 0.45 s.
- **The largest specs.** These five carried 46 % of the summed time in the third baseline run:

  | Spec | Summed time | Tests |
  |---|---|---|
  | `schedule` | 76 s | 92 |
  | `students` | 64 s | 78 |
  | `groups` | 57 s | 72 |
  | `lesson` | 54 s | 81 |
  | `join-requests` | 46 s | 54 |

- **Per-test overheads.** Firefox's page creation (about 0.10 s, as in WebKit, against 0.03 s in Chromium) and its slower test bodies are engine costs that every test pays. Every tenth context close of a Firefox browser stalls, and in the suites the stall grows with the number of contexts the browser has opened (item F of the plan, below).

With 6 workers, removing work worth 1 s of summed test time shortens the suites by about 0.17 s. Removing one test from all three engines saves about 2.1 to 2.4 s of summed time, about 0.35 to 0.4 s of wall time.

## Proposal

Lower the CPU cost of the suites by doing less work per behaviour proven, while every behaviour stays proven in every engine where a browser shows it. The techniques below are candidates. Each is adopted only where its measured gain exceeds the spread of its runs, as in the [critical path plan](../../knowledge/plans/merge-gate-critical-path.md).

### 1. Consolidate tests that share their arrangement

Many tests in one file seed the same actors, open the same page in the same state and then assert one facet each, for example the parts of one dialog, the columns and order of one list, or a page's heading and where its focus lands. Such a group becomes one test with the same arrangement, the same action and every assertion of the group in its assert phase. Where the facets need different actions, the test is split by action, not by assertion.

- **Expected gain:** each removed test saves its context and page creation, its sign-in, its first navigation and its context close. An estimate puts these at about 0.2 to 0.5 s of summed time per test and engine under the gate's load. The one-worker profile measured the before hooks at 0.05 s (Chromium) and 0.13 s (Firefox) per test. Firefox's after hooks took 0.28 to 0.34 s per test, almost all of it the stall of technique 4. The load inflates each part by about 1.1 to 1.6 times, so the measured parts come to about 0.08 to 0.21 s. The rest of the estimate, a test's sign-in and its first navigation, was not timed and is an assumption. The first step below replaces the estimate with a measurement. Removing 100 tests per engine would save about 60 to 150 s of summed time, about 10 to 25 s of wall time.
- **Coverage risk:**
  - a failing early assertion hides the later ones of the same test;
  - a merged test could drop an assertion that only differed in its arrangement;
  - a focus assertion could become order-dependent;
  - a test that asserts a page's state after a specific action could lose the fresh page that action relied on.
- **How coverage is proven unchanged:** each consolidating change records a table in its check-budget record that maps every removed test title to the assertions in the merged test. That table, checked by the reviewer against the diff, is the proof; the commands below are only guards against an accidental loss:
  - `grep -o 'expect(' frontend/e2e/*.ts | wc -l` and `grep -o 'expect.poll(' frontend/e2e/*.ts | wc -l` print the same numbers before and after;
  - every merged test keeps Arrange–Act–Assert with one arrangement and one action phase, per `AGENTS.md`;
  - every focus move a specification defines still has its own `toBeFocused` assertion;
  - the reviewer checks the mapping against the diff, and a mapping that leaves an assertion out fails the review.

  Pilot scenario step tests are not consolidated, because their titles name the steps they prove.

### 2. Move checks that do not depend on the browser into Go integration tests

Some browser tests prove server effects that the page neither shows nor depends on: the contents of an email in the mail sink, the effect of a worker job, the data a request writes. Only such checks, with no specified rendering, are candidates. Each such check moves to a Go integration test against the real API server, worker and database, which is the bulk of the test suite by `AGENTS.md`, and the browser test is removed only where it proves nothing a browser shows. Every key refusal and every authorization or relationship-scope refusal stays in the browser in every engine, because `AGENTS.md` requires the end-to-end tests to cover the main journeys of both roles and the key refusals; so do the pilot scenario steps.

- **Expected gain:** about 0.35 s of wall time per test removed from all three engines; 30 such tests would save about 10 s. Their Go replacements run in lane A, which ends well before the end-to-end step (lane A took 45 to 48 s in the last gates whose lane times were reported). They add to the critical path only if lane A becomes the longest slot.
- **Coverage risk:** losing the proof that the frontend calls the API as the Go test assumes, or that it renders the result. The frontend's typed API client and the API contract suite limit the first risk; the second is why a test whose rendering is specified is kept.
- **How coverage is proven unchanged:** each moving change lists, per removed browser test, the Go test that proves the same server contract and the browser test, if any, that still proves the rendering. The Go test must fail on the server bug the browser test caught; the change shows this with a throwaway, uncommitted change to the server code in a separate worktree, and records the failing output in its check-budget record. The edit is then reverted by hand and the worktree removed with a plain `git worktree remove`, with no force, restore or checkout. The reviewer checks each pair.

### 3. Lower the database's cost per test

Docker's virtual machine, which runs PostgreSQL and the S3-compatible server together and was measured only as one family, takes about 15 % of the CPU. Candidates, each measured first:

- `synchronous_commit = off` set on each end-to-end database with `ALTER DATABASE` by the harness. It changes durability only, not behaviour, and only on `ovc_e2e_*_test` databases.
- Fewer round trips in seeding, through the existing seeding routes. B0 already shares five worker-scoped actors (`SharedActors` in `frontend/e2e/fixtures.ts`); what remains is the accounts that tests still seed for themselves, test by test, although they only read them, which could become further worker- or file-scoped actors.

A batched seeding route is out of scope. It would add to the test control surface (T-36), and the owner declined it on 2026-10-28.

- **Expected gain:** unknown until measured. If the database's share fell by a third, about 5 % of the CPU would return to the browsers, about 5 to 6 s of wall time.
- **Coverage risk:** `synchronous_commit = off` changes when a commit is flushed to disk, which no test observes, but it also shortens commit latency below production's, so it can change the timing and interleaving of concurrent requests, jobs and listener notifications; a race that production's slower commits expose could then pass in the suites. The change keeps the Go integration tests, which run with the default setting, as the proof of concurrency behaviour. Seeding read-only actors once per worker or file risks tests that depend on each other's data; a shared actor is used only where no test mutates it, as the existing shared actors are.
- **How coverage is proven unchanged:** `playwright test --list` prints the same lines before and after, and the assertion counts above are unchanged.
- **Confinement of `synchronous_commit = off`, checkable:** the harness sets it only after `readE2ESettings`' loopback checks have passed, and only on a database whose name matches `^ovc_e2e_[a-z0-9]+_test$`; a harness test proves that it refuses any other name. During a run, `SHOW synchronous_commit` prints `off` on an end-to-end database and `on` on the administrative database and on a Go test database. The change updates T-36 in the threat model, because it changes the deployments that run with the test control surface.

### 4. Firefox's growing context-close stall

This is item F of the [end-to-end shortening plan](../../knowledge/plans/e2e-shortening.md). Playwright's Firefox build minimises memory on every tenth context close. In the suites the stall grows with the number of contexts a browser has opened, from 0.5 s to 7.2 s within 111 tests at one worker; on a bare page it stays at 0.3 s.

- **Investigation:** find what the tests leave in the browser that makes the stall grow, for example pages, sockets or listeners kept alive after a context closes.
- **Expected gain:** up to about 4 to 5 s of wall time with 6 workers, only if the growth is removable.
- **Coverage risk:** none, if the fix releases what the tests leave behind rather than changing what they assert.
- **How coverage is proven unchanged:** as for technique 3.

### What does not change

- Every engine runs every test that uses a browser.
- No engine is dropped. No test or assertion is dropped without a mapping that proves its behaviour elsewhere.
- No timeout or retry changes.
- The pilot scenario suites keep their steps.
- NFR-6.2, ADR-0018 and the gate's command list stay as they are.

## Alternatives

- **More Playwright workers:** measured without a gain, because the suites are CPU-bound ([profile record](../../knowledge/analysis/check-budget-2026-10-29-e2e-profile.md)).
- **Running some specs in Chromium only:** saves up to about 480 s of summed time but drops engine coverage of the supported browsers. The owner excluded it, and the definition of done forbids it.
- **A migrated template database for the deployments, or moving the builds off the critical path:** about 0.5 s and at most 6.5 s. The owner decided on 2026-10-29 not to do them now.
- **A lighter static server instead of `vite preview`:** about 6 % of the CPU, but the suites would then no longer test the preview server whose headers, proxy and Content Security Policy the smoke proves.
- **Raising D46's threshold for good:** the owner set a temporary threshold until the end of slice 6 instead, so that the cost is reduced rather than accepted.

## Plan

1. **Measure.** No suite change. A record in `knowledge/analysis/` gives per spec and engine:
   - the summed time;
   - the number of tests that share each arrangement, from the specs' fixtures and first actions;
   - the measured fixed cost of a test in each engine, as context, page, sign-in, first navigation and close under the gate's load;
   - the candidate browser tests whose assertions concern only server effects with no specified rendering.

   It replaces this proposal's estimates and ranks the techniques by measured gain.
2. **Consolidate** in the five largest specs (technique 1), one spec per change. Each change is a standard-tier change with its mapping table, three runs with and three without it, and its gate.
3. **Move server-only checks** to Go (technique 2), one feature area per change, each with its pairs of Go and browser tests.
4. **Database and Firefox items** (techniques 3 and 4), each measured before any committed change. Technique 3's harness change meets the security trigger (T-36, the deployments that run with the test control surface).
5. **Close.** Three cold gates on an idle machine are recorded, and D46's threshold after slice 6 is the owner's to set.

**Sequencing relative to slice 6.** Slice 6 (chat and files) will add end-to-end tests in three browsers. Its plan applies techniques 1 and 2 to its own new tests from the start: one test per shared arrangement, and Go tests for every server-only check. Steps 1 and 2 above run before slice 6's first frontend change, so that the slice adds tests to suites that already cost less. Steps 3 and 4 run beside the slice's backend changes, which do not touch the end-to-end suites. Every change that adds tests measures the gate, and above the temporary 260 s threshold the suites are consolidated in the same change before more tests are added.

## Risks and open questions

- **The gains are estimates** until step 1 measures the fixed cost of a test and the number of tests that share an arrangement. If together they promise less than the gap to 165 s, the owner decides on D46's target with that measurement.
- **Consolidated tests report less precisely** which facet failed. The failing assertion's message and the retained trace still name it.
- **Moving checks to Go relies on the frontend calling the API as specified.** The API contract suite and the typed client cover that, but a page that silently ignores an error would no longer be caught by a removed browser test. This is why tests whose rendering is specified are kept.
- **Open:** whether `synchronous_commit = off` reduces the virtual machine's CPU at all on this machine, and what makes Firefox's stall grow.

## Decision

Filled in when the status becomes `accepted` or `rejected`.
