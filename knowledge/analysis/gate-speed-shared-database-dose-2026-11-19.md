# Lane A of the merge gate: the marginal cost of a test database, measured for proposal 0005

Recorded on 2026-11-19 for the consolidation that the owner inserted into sub-slice 10.2 after the 10.1 slice review: a measured pilot of [proposal 0005](../../docs/proposals/0005-shared-test-database-for-subtests.md), under which the subtests of one integration test would share a test database. The measurement says the target cannot be reached, so this record is the only file the change adds. No code, policy or test changed.

## Why this was measured first

- **The gate is over budget.** At the 10.1 slice review, the last three passing mains-power gates took 614.9, 608.9 and 634.5 s, a median of 614.9 s. That is above ADR-0018's 540 s consolidation threshold and NFR-6.2's 600 s limit.
- **What was already ruled out.** The [lane A checkpoint record](gate-speed-lane-a-checkpoints-2026-11-13.md) and the [lane A profile record](gate-speed-lane-a-profile-2026-11-18.md) found `internal/api` bound by the per-test clone, drop and checkpoint, and ruled out every lever that keeps one database per test.
- **The proposal's premise did not hold in the code.** The task plan for the pilot found that the API harness seeds users with fixed identifiers and addresses: `signedInAs` is called with a fixed account number in about 800 places, and `seedUserAs` builds `domain.UserID{id}`. Every harness also writes state that the whole database shares:
  - the shared fake clock;
  - the single `fake_meeting_state` row of the fake meeting provider, which holds availability and capacity and is taken `FOR UPDATE` by every provider call;
  - the mail sink.

  Converting a test to a shared database would therefore mean rewriting its arrangement, not changing one call. The meeting tests that proposal 0005 named as pilot parents could not share at all.
- **The planning reviews.** The standard and security-profile planning reviews of that plan also found these gaps:
  - harness methods that a conventions test would need to follow;
  - SQL held outside the test body;
  - rate limits keyed by the client address, which all subtests share on loopback.
- **Where the potential stood.** `internal/api` has 904 top-level tests and 1,493 leaf tests in this window's `main` runs (1,422 at the 2026-11-18 profile). A shared database removes at most one clone per subtest beyond the first, which inside today's parents is about 518 clones, and fewer once the excluded families are left out.
- **The owner's order.** On 2026-11-19 the owner decided to measure the cost of a clone first, with no repository change, and to write only this record if the projection missed 540 s.

## Method

- **Machine:** 12 logical cores, macOS, Docker Desktop, PostgreSQL 18.6 with `main`'s settings. Every run was on AC power, shown by `pmset -g batt` at its start and end, under `caffeinate -dis`.
- **Load condition:** each run started only once both the one-minute and the five-minute load average were below 6.
- **Isolation:** the coordinating session held back every merge gate, end-to-end run and backend run for the whole window, 18:05 to 19:28 MSK. Other projects' containers were running, as in the earlier records.
- **Command:** `go test -race -count=1 -json ./...` from each tree's `backend/`, through the main checkout's `tools/dev-env.sh`.
  - Each tree was a detached worktree of a commit with no branch, and the worktrees were removed after the window.
  - The tool binaries of `bin/` were copied into each worktree.
- **Trees:** all built on `main` at `405360f` (tree `3a1a987`). Each candidate added one throwaway test file to `backend/internal/api`, in which every subtest sends one request.

  | Tree | Commit / tree | Added test |
  |---|---|---|
  | `main` | `405360f` / `3a1a987` | none |
  | `own300` | `e75b54e` / `1ddd5f9` | one parallel parent with 300 parallel subtests, each building its own `newAuthHarness`, and so its own database |
  | `own600` | `462b2a6` / `ee0aa03` | the same with 600 subtests |
  | `shared300` | `a6b9da5` / `0c2a390` | one parallel parent that builds one `newAuthHarness` and passes it to 300 parallel subtests, so one database is shared |

- **Series:** three series of three pairs, with the tree that runs first alternating as `A B`, `B A`, `A B`.
  - Adding 300 databases: `main` against `own300`.
  - Adding 600 databases: `main` against `own600`.
  - Removing 299 databases directly: `own300` against `shared300`. Both run the same 300 subtests, so the only difference is the number of databases.
- **Recorded per run:**
  - wall time;
  - the `internal/api` package elapsed from `-json`;
  - leaf count and summed leaf time;
  - failures;
  - the count of passed tests;
  - the `SYN_SENT` peak, sampled every second;
  - the number of `checkpoint complete` lines in the PostgreSQL container's log during the run.

## Results

Every one of the 18 runs passed (rc 0, no failed test), on AC power at start and end.

### Adding 300 databases

| Pair | Order | `main` `internal/api` | `own300` `internal/api` | Difference |
|---|---|---|---|---|
| 1 | `main` first | 144.6 s | 160.7 s | +16.1 s |
| 2 | `own300` first | 164.4 s | 173.2 s | +8.8 s |
| 3 | `main` first | 160.0 s | 162.7 s | +2.7 s |
| Mean | | 156.3 s | 165.5 s | **+9.2 s, 0.031 s per database** |

### Adding 600 databases

| Pair | Order | `main` `internal/api` | `own600` `internal/api` | Difference |
|---|---|---|---|---|
| 1 | `main` first | 169.5 s | 177.6 s | +8.2 s |
| 2 | `own600` first | 158.2 s | 193.5 s | +35.2 s |
| 3 | `main` first | 168.7 s | 196.2 s | +27.5 s |
| Mean | | 165.5 s | 189.1 s | **+23.6 s, 0.039 s per database** |

### Removing 299 databases

| Pair | Order | `own300` `internal/api` | `shared300` `internal/api` | Difference |
|---|---|---|---|---|
| 1 | `own300` first | 173.5 s | 160.4 s | −13.2 s |
| 2 | `shared300` first | 169.8 s | 163.5 s | −6.4 s |
| 3 | `own300` first | 172.6 s | 164.2 s | −8.4 s |
| Mean | | 172.0 s | 162.7 s | **−9.3 s, 0.031 s per database removed** |

### Further observations

- **The cost is mildly convex.** It is 0.031 s per database for the first 300 added and 0.039 s per database for 600. A least-squares slope through zero gives 0.038 s.
- **Removing databases saves what adding them costs.** Removing 299 databases saved 0.031 s each, the same as adding the first 300 cost.
- **The leaf times match.** In the removal series, summed leaf time was 1,821.7 to 1,842.3 s on `own300` and 1,723.2 to 1,735.4 s on `shared300`.
- **Checkpoints did not separate the trees.** The checkpoint counts ranged from 338 to 530 and followed the run as much as the tree.
- **Sockets stayed low.** The `SYN_SENT` peaks were 0 to 8 on every tree.
- **The spread between runs of one tree is large.** `main` ranged from 144.6 to 169.5 s within the window.

## Projection

The figures for the gate are restated from the revision of 2026-11-17 of the gate-speed task plan, which was not committed, so this record is their source in the repository. They come from the last three passing gates before that date: in-gate lane A median 202.3 s, end-to-end median 404.0 s, and about 363 s in night gates. Lanes B and C take at most 111 s, so lane A cannot count below that.

The overhead outside the lanes is 8.6 s: the median gate of 614.9 s minus 202.3 + 404.0 s. The in-gate lane A scales by the standalone ratio of `internal/api`, taken against the `main` mean of the window, 160.9 s.

| Clones removed | Standalone saving at 0.031 s (0.038 s) | Projected in-gate lane A | Projected gate, 404 s end-to-end | Projected gate, 363 s end-to-end |
|---|---|---|---|---|
| 250, the planning estimate of qualifying subtests | 7.8 s (9.5 s) | 192.6 s (190.4 s) | 605.2 s (603.0 s) | 564.2 s (562.0 s) |
| 518, every subtest of today's parents | 16.1 s (19.7 s) | 182.1 s (177.6 s) | 594.7 s (590.2 s) | 553.7 s (549.2 s) |

- **Neither projection reaches 540 s, even if every subtest of today's parents shared its parent's database.**
- **The quiet-machine variant** would need lane A at 168.4 s in the gate. That is a standalone saving of about 27 s, or about 870 clones at the measured cost: more than the 518 subtests that today's parents hold.
- **The 404 s variant** would need about 1,900 clones removed: more than the leaf tests that exist.
- **The proposal's own go criterion is missed too.** It asked for 5 % of `internal/api` per 100 clones removed. The measured saving is about 3.1 s per 100, or 2 %.

## Conclusions

- **On this machine a test database costs `internal/api` about 0.03 to 0.04 s of wall time.** Most of the fixed cost of a leaf test overlaps across the twelve parallel slots.
- **The checkpoint-bound picture of the earlier records holds,** but at the margin a database is cheap. Lane A is long because it runs about 1,500 tests, not because each clone is expensive.
- **Proposal 0005 cannot bring the gate to 540 s.** By the owner's decision of 2026-11-19, it stays deferred: no shared fixture, conventions test, decision record, `AGENTS.md` amendment or T-41 amendment is written. The next consolidation comes from the end-to-end run, which is about 400 s of the gate.
- **What remains for lane A:** fewer tests, which would move or merge coverage, and the machine.

## What this record does not cover

- **Lane A was not measured inside a gate.** The in-gate figures are scaled from standalone ratios, which earlier records found to understate lane A inside a gate by a factor of 1.1 to 1.4.
  - If the in-gate saving were 1.4 times the scaled saving, every cell would stay above 540 s except one. At 518 clones and the convex 0.038 s, the quiet-machine variant would project 539.3 s; the 0.031 s cost of that row projects 545.6 s.
  - Even that one cell is an upper bound that cannot be reached. It needs every subtest of today's parents to share, while the excluded families remove many of them. It needs real subtests to save as much as the minimal throwaway ones, and they save less (below). It assumes the night end-to-end time rather than the gate median.
  - The 404 s variant stays above 580 s in every case.
- **The leftover check is unverified.** After the window, the query listing `ovc_%_test` databases that are not templates was not run, because `psql` inside the container asks for a password, and the measurement does not read credentials.
- **The test bodies were minimal.** Each throwaway subtest sends one request that does not touch the database. Real subtests write rows, which a shared database would keep until the parent ends, so their saving per clone could only be smaller.
- **Throwaway commits.** The throwaway commits had no branch or tag, and their worktrees were removed after the window.

## Owner decisions

On 2026-11-19 the owner took the recommended option on every question of the pilot's planning:

- measure first, with no repository change;
- if only the quiet-machine projection reached 540 s, ask before building the pilot;
- if neither reached it, write only this record, keep proposal 0005 deferred, and take the next consolidation from the end-to-end run;
- enforce sharing by an allowlist of harness methods with method traversal;
- approve the policy texts only after this measurement.

The third case applies.
