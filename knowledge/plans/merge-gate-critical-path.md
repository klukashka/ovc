# Plan: shorten the merge gate's critical path

## 1. Problem

Since the merge gate runs its checks in concurrent lanes, three cold runs took 155.6 to 161.3 s of wall time ([record](../../knowledge/analysis/check-budget-2026-10-27-merge-gate-lanes.md)). The owner set slice 5's D46 target to 165 s with the 180 s threshold kept, and asked for the critical path to be shortened in a separate change beside S2. The critical path is the longer of lane A (`go test -race`, 59 to 61 s) and lane C (61 to 64 s), followed by `pnpm e2e` alone (93 to 96 s). Slice 5's S2 to F3 add backend and end-to-end tests that will grow both.

## 2. Outcome

Each item keeps every check and assertion; none weakens a check.

1. **Parallel `startup` tests.** `backend/cmd/internal/startup` runs 9 of its 28 tests in parallel, and its `TestMain` builds the three binaries one after another. Every test that does not need exclusive use of a shared resource calls `t.Parallel()`, and `TestMain` builds the binaries with one `go build -o <dir>/ ./cmd/api ./cmd/worker ./cmd/ovc-admin`. A test that must stay serial keeps it, with the reason visible in its name or a helper, not a comment.
2. **One `go build` in the end-to-end setup.** `frontend/e2e/deployment.ts` builds the three binaries with one `go build` invocation, as above.
3. **Traces without screenshots.** `trace: 'retain-on-failure'` records a screencast for every test. It becomes `{ mode: 'retain-on-failure', screenshots: false, snapshots: true, sources: true }`, keeping DOM snapshots and sources for failure analysis. It is measured by three `pnpm e2e` runs with and three without it on an otherwise idle machine, and kept only if the mean gain exceeds the spread of the runs.
4. **Lanes balanced.** By the record's per-command figures, lane C (61 to 64 s) outlasts lane A (59 to 61 s) mainly through `tools/check-merge-gate.sh` (23 to 24 s), and lane B takes 36 to 39 s, of which `tools/docs/check-docs.sh` is 11 s. `tools/check-merge-gate.sh` moves from lane C to lane B and `tools/docs/check-docs.sh` with the documentation tools' install from lane B to lane C: lane B becomes about 36 − 11 + 24 ≈ 49 to 52 s and lane C about 64 − 24 + 11 ≈ 51 to 52 s, both below lane A. The tagged list, the self-test that checks the slots, ADR-0018's slot description and the development guide's slot list change accordingly.

Item 1 starts from per-package times: the `ok … Ns` lines of lane A's log in a cold gate run before the change, compared with the same lines after it, show whether `startup` or another package sets lane A's length; the record states both. Item 1's `TestMain` build uses import paths (`ovc/backend/cmd/api`, `ovc/backend/cmd/worker`, `ovc/backend/cmd/ovc-admin`), since it runs in `backend/cmd/internal/startup`; `deployment.ts` runs from `backend/` and uses the relative form.

## 3. Tier and security trigger

- **Tier: standard.**
- **Security trigger: met.** Item 4 changes the merge gate's command list (T-41), and items 1 and 2 change the end-to-end harness and the process tests of the test control surface (T-36). Standard and security-profile reviewers; this plan had a security-profile reviewer before the owner's approval. T-36 and T-41 need no update: the boundaries, the tests' assertions and the review policy are unchanged.

## 4. Files

- `backend/cmd/internal/startup/*_test.go` and `process_test.go`'s `TestMain`.
- `frontend/e2e/deployment.ts`.
- `frontend/playwright.config.ts`; `docs/development.md` (the traces line, the slots).
- `tools/merge-gate.sh` (the tagged list), `tools/check-merge-gate.sh` (the expected list and slot counts).
- `docs/decisions/0018-targeted-checks-and-merge-gate.md` (its lane C description, pending owner review) and `knowledge/reviews/ai-development-setup.md`.
- `knowledge/analysis/check-budget-2026-10-27-gate-critical-path.md`: three cold gate runs.

## 5. Acceptance criteria

- `go test -race -count=1 ./cmd/internal/startup/` passes ten times in a row through `tools/dev-env.sh`, and the count of passed tests including subtests (`--- PASS` lines of `-v` output) is unchanged.
- Every `startup` test made parallel that probes a server's port asserts that its own process exited cleanly, or the child binds `127.0.0.1:0` and reports its address, so a port taken by another test's server can never make a refusal or absence test pass.
- No run of this change, including the gate's, shows `too many clients`; such an error counts as a failure of this change, not as an infrastructure error, and `-parallel` is limited if it appears.
- In all three cold gate runs, lanes B and C each finish before lane A.
- The whole `pnpm e2e` passes, with the same test count (669 at the time of writing).
- `tools/check-merge-gate.sh` passes with the new slot counts and the self-test that every command of the full run is in exactly one slot.
- Three cold gate runs on an idle machine, recorded with the figures D46's amendment requires, each pass and each report a wall time below the last record's 155.6 s; the record states the gain of each item where it can be separated.
- The documentation checks and Gitleaks pass; the merge goes through the gate.

## 6. Order, risks and checks

- Order: items 2, 1, 4, then 3 with one separate measurement; documents; three cold runs; reviews; the gate.
- **Risk: parallel `startup` tests contend** for ports, the database or the run lock. Each test has its own test database and a from-scratch child environment, but `freeAddress` releases its port before the child binds it, a gap that grows with parallelism; the acceptance criterion on probing tests closes it. More parallel tests also open more PostgreSQL connections against the default limit of 100.
- **Connection budget of `go test -race ./...`.** The development PostgreSQL keeps its default `max_connections` of 100; `docker-compose.yml` does not change it. Packages run side by side (`-p` is the core count, 12 here). With every `startup` test parallel and each child process keeping the default pool of up to 20 connections (10 for the worker) beside the test's own pool, a cold gate run failed in `internal/repositories` with `too many clients`. The startup tests therefore cap each child's pool at 5 (`OVC_DB_POOL_SIZE=5`, as the end-to-end deployment did when this plan was written) and run at most `processTestSlots` tests that create a test database at a time, through a package-level semaphore held from the database's creation until the test ends. A slot holds at most 5 pooled connections of the child, 1 listener connection and about 2 connections of the test's own pool, 8 in all, and the budget of 24 connections gives 3 slots, so the package holds at most about 24 connections plus brief database creations on the shared administrative pool, less than the seven concurrent API servers with pools of 20 that the package started before this change. Sampled every 0.2 to 0.3 s during runs on 2026-10-27 on a machine shared with other sessions: all other packages without `startup` peaked at 76 client connections; `startup` alone peaked at 14 with the slots and at 34 without them; three full runs with the slots showed no `too many clients` and peaked at 98. The headroom of the whole suite against 100 was thin regardless of `startup`.
- **Bounded test database pools.** The owner decided on 2026-10-27 to give the suite headroom by bounding the pools of the integration-test fixture `internal/testdb`, not by raising `max_connections`. Each test database opens at most 4 connections instead of pgxpool's default of one per core (12 here); the API tests' authentication harness, whose lock tests hold one row lock while up to three requests wait on it and a fourth connection counts the waiters, asks for the only larger pool, of 6 connections, through `testdb.NewForLockContention`; the pool size is an unexported type, so no test can ask for any other size. The template database is migrated through a pool of 2. The arithmetic: up to 12 packages run side by side and each runs up to 12 tests at a time, so the per-test pools alone are bounded by 12 × 12 × 4 = 576 connections with the bound and 12 × 12 × 12 = 1728 without it; neither bound is reached, because few packages run 12 database tests at once and pools open connections only on demand, so the sampled peaks decide. The shared administrative pool of each test process (pgxpool's default size, one per core) kept every connection it had opened idle for up to 30 minutes; it now closes a connection after about one second idle (`MaxConnIdleTime` 1 s, health check every 0.5 s). Capping its size instead serialized the clones and drops: the API package alone took 31 to 46 s with its default size, 44 to 52 s with 8 and 61 to 69 s with 4, so its size is unchanged. Peaks of client connections in `pg_stat_activity`, sampled every 0.2 to 0.3 s over three full `go test -race -count=1 ./...` runs each on a machine shared with other sessions: 98 before the bound; 81 with the bounded test database pools alone (test databases at most 44, other databases at most 54); 75 with the administrative idle release as well (test databases at most 42, other databases at most 47, mostly administrative connections cloning and dropping test databases). All six runs passed with no `too many clients`. The owner accepted the peak of 75 on 2026-10-27.
- **Risk: losing failure evidence.** Traces keep DOM snapshots, network and sources; only the screencast goes.
- **Risk: lane B grows past lane A.** The three runs show the lane times; if B becomes the longest, one command moves back.
- **Not changed:** the commands themselves, NFR-6.2, D46's target and threshold, the docs-only list.

Approved by the owner on 2026-10-27.

Outcome of 2026-10-28: three cold gate runs took 157.3 to 181.1 s ([record](../analysis/check-budget-2026-10-28-gate-critical-path.md)); traces without screenshots were dropped by the measurement rule of item 3; the owner decided to merge and to shorten the end-to-end suites next under their own plan. The tools module's `golang.org/x/net` upgrade to 0.62.0 was reverted because it was younger than the release age of [ADR-0017](../../docs/decisions/0017-dependency-release-age.md), and GO-2026-6860 and GO-2026-6866 are ignored until 0.62.0 is three days old, by the owner's decision of 2026-10-28 ([triage](../research/tools-x-net-advisories-2026-10-28.md)).
