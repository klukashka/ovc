# Proposal 0005: let the subtests of one integration test share a test database

- Status: deferred
- Reviewed: direction chosen by the project owner on 2026-11-18 (write a deferred proposal for subtests sharing a database); the text is pending owner review
- Author: coding agent, for the project owner
- Date: 2026-11-18
- Trigger: the median of the last three passing merge gates on mains power still exceeds ADR-0018's 540 s once gates run only on an idle machine as the development guide requires, or the owner schedules the change
- Related: NFR-6.2, the Testing section of `AGENTS.md`, [ADR-0018](../decisions/0018-targeted-checks-and-merge-gate.md), [ADR-0009](../decisions/0009-owner-approved-exceptions-and-visibility.md), [threat model: T-41](../threat-model.md), the [lane A checkpoint record](../../knowledge/analysis/gate-speed-lane-a-checkpoints-2026-11-13.md), the [test-database pool record](../../knowledge/analysis/gate-speed-test-database-pool-2026-11-12.md), the [PostgreSQL durability record](../../knowledge/analysis/gate-speed-postgres-2026-11-12.md)

## Motivation

- **The gate is over budget.** Its length is lane A plus the end-to-end run. Lane A is the `go test -race` run of `internal/api`, which took 146.5 to 200.2 s in the merge gates of sub-slice 10.1. The median of the last three passing gates stood at 607.3 s in the 10.1 slice review, above ADR-0018's 540 s consolidation threshold and above NFR-6.2's 600 s limit.
- **The tests already run as parallel as they can.** A profile of lane A on 2026-11-18, recorded in `knowledge/analysis/gate-speed-lane-a-profile-2026-11-18.md`, found the package at its `-parallel` cap of 12 for about 85 % of its run. The time is set by a fixed cost per test: 1,022 of the package's 1,422 leaf tests took 0.5 to 1.5 s, mostly that fixed cost. It includes:
  - cloning a fresh database from the migrated template;
  - building the harness;
  - dropping the database.
- **Every drop costs a checkpoint.** Each drop requests an immediate checkpoint and waits for it, and that checkpoint must write and sync the files of every database alive at the time. The [lane A checkpoint record](../../knowledge/analysis/gate-speed-lane-a-checkpoints-2026-11-13.md) measured this.
- **What has already been measured as no-go:**
  - more concurrency: a higher `-parallel`, more packages or deferred drops;
  - cloning by file copy;
  - durability off, rejected;
  - reusing databases from a pool, also no-go;
  - letting the 19 top-level tests that run serially run in parallel, slower in three paired runs on 2026-11-18 and not merged.

  The checkpoint record lists fewer test databases, by letting tests share one, among the remaining levers, and notes that this changes test isolation and needs its own decision. The profile record finds it the one lane A lever left that it did not rule out.

## Proposal

- **The fixture.** A top-level test may create one database and hand it to its subtests. A new constructor in `internal/testdb` names this, for example `testdb.NewShared(t)` called by the parent. Subtests receive the parent's harness or a harness built on the shared database. The database is dropped once, when the parent ends, after all its parallel subtests.
- **Which tests may share.** Each subtest arranges its own accounts, relationships and resources, all with fresh random identifiers, and asserts only on rows reachable from them. That is the same scoping the API itself enforces.
- **Which tests never share.** They keep their own database:
  - tests that count or scan rows across the whole database: purge, erasure and expiry jobs, worker `Tick` passes over all due rows, outbox totals, and audit, outbox or mail-sink reads not filtered to the test's own parties, such as "no mail was sent" or "the latest mail";
  - tests whose evidence covers the whole database's statements or connections: `testdb.WatchStatements`, `testdb.NewQueryMeasurement`, `Database.HoldAllConnectionsBut`, and `pg_stat_activity` or `pg_locks` lock-waiter counts;
  - tests that hold row locks or cancel statements under ADR-0009 item 12, including every test built with `testdb.NewForLockContention`;
  - tests that change database-wide state: migrations, settings, triggers created under ADR-0009 item 8, the shared fake clock;
  - tests that subscribe to the real PostgreSQL listener or assert on WebSocket frames or live refetches, because `LISTEN` and `NOTIFY` channels span the whole database and siblings would receive each other's notifications;
  - tests that use the listener's connection forwarder.
- **A conventions test enforces what it can.** A test that uses the shared constructor may not call, in the same test function or its subtests, any of these: `testdb.NewForLockContention`, the migration helpers `NewMigratedTo`, `MigrateUpTo` and `MigrateDownTo`, the forwarder, the statement and connection watchers above, worker `Tick`, the fake clock's `Advance` or `Set`, mail-sink reads, or a WebSocket or listener subscription, and its source may not contain the SQL `pg_cancel_backend`, `pg_locks`, `pg_stat_activity` or `CREATE TRIGGER`, which today's lock, cancellation and trigger tests run inline. Calls made through other package helper functions are not caught by the check and are left to review. The remaining rule, that a subtest asserts only on rows reachable from its own parties, cannot be checked mechanically and rests on review of every moved test.
- **What does not change:**
  - the template, its migrations and the clone strategy;
  - the per-process template sweep, the host check and the drop rules;
  - every assertion, timeout and test name.

  For the tests that qualify, a subtest's behaviour is unchanged except that other subtests' rows exist beside its own; the exclusions above cover the database-wide channels, such as notifications, through which siblings could otherwise reach each other.

## Alternatives

- **Keep one database per test:** today's isolation. Lane A stays at about 150 s on an idle machine and 1.1 to 1.4 times that inside a gate, and the budget relies on the end-to-end run and a quiet machine.
- **Reset and reuse databases from a pool:** measured as no-go on 2026-11-12. It was not faster, and runs failed.
- **Fewer, larger tests:** merging subtests into one test body saves the same clones, but it loses the failure locality and the names that the subtests give.
- **A tmpfs tablespace, or checkpointer and `shared_buffers` settings:** not measured. Each changes the container's configuration, which is close to the rejected durability change. Each would need its own proposal.

## Plan

1. **Measure the potential.** Count the databases each package creates per run, by adding a clone count to the fixture or by counting the fixture's constructor calls per run, and the leaves per parent. Estimate the saving as the clones removed multiplied by the measured per-clone cost.
2. **Pilot on two parents with many subtests that qualify,** for example the meeting join authorization and concurrent join tests. Run three alternating pairs of `internal/api` and `./...` against `main` on an idle machine. Each pilot test runs `-count=10 -race` to look for coupling.
3. **Go criterion:**
   - every run passes;
   - the `internal/api` mean is lower by at least 5 % per 100 clones removed;
   - no test fails under `-count=10`.

   On go, extend the change parent by parent in reviewed changes. Each change lists the tests it moves and why each qualifies.
4. **Tests.** The conventions test above, and a fixture test proving that the shared database is dropped once, after the last subtest.

## Risks and open questions

- **Hidden coupling.** A subtest that reads "the latest" or "all" rows of a table passes alone and fails beside its siblings, or worse, passes because of them. The exclusion list and the `-count=10` runs reduce this risk but cannot prove its absence. Review must check every moved test. Test isolation is part of the verification policy, so a change to it maps to T-41 and meets the security trigger.
- **Weaker evidence for scope tests.** A refusal test (`not_found` outside the caller's scope) becomes stronger when other parties' rows exist. A test that asserts an empty list becomes invalid unless it filters to its own parties. A test that shows a student cannot see another student's work, or that a purge removed a person's data, can pass vacuously if the rows it checks came from a sibling instead of its own arrangement. Answer-key and score visibility tests check the response body and are not weakened, but they too must keep their own fixtures. Each such test must be read, not converted mechanically.
- **Cross-delivered notifications.** A test that subscribes to the listener would receive its siblings' notifications on a shared database. The exclusion list keeps such tests on their own database; a missed one would see frames or refetches it did not cause.
- **The saving is uncertain.** A shared database grows with its subtests' rows, so its drop and the checkpoints it shares write more. The net gain per clone removed is unknown until the pilot measures it.
- **Failure locality.** A leaked row from one failing subtest can make its siblings fail too, which makes failures harder to read.

## Decision

Not decided. Deferred by the owner on 2026-11-18 until the trigger above. On the same day the owner decided not to merge the parallel parent tests, which leaves this proposal as the one lane A lever that the profile record did not rule out.

Adopting it needs, together: a decision record, an amendment to the Testing section of `AGENTS.md`, an amendment to T-41 in the threat model, and a security-profile review of the plan. ADR-0009 items 8 and 12 speak of the test database created for a test; they stay satisfied without change only because the tests they cover are excluded and keep their own database.
