# Local check budget after the test database template

Measured on 2026-10-26 on the branch of the test database template (B0 of the [slice 5 plan](../plans/slice-5.md)), based on `main` at `3b110a4`, which includes the slice 4 frontend realtime client. D46 of the plan asks B0 to bring the full local check run, measured on an idle machine at the slice 4 closing commit, to at most 149 seconds before S2 starts. The comparison point is the [record after the frontend realtime client](check-budget-2026-10-26-realtime-client.md), which measured 201.8 seconds on a loaded machine.

## What the change does

- **Go integration tests:** each test process migrates one template database and every migrated test database is cloned from it, instead of running all migrations once per test database ([development guide](../../docs/development.md#backend-checks)). The change adds five tests of the fixture itself.
- **End-to-end suites:** read-only tests share seeded actors per worker instead of seeding their own:
  - three new worker-scoped fixtures on the base test in `frontend/e2e/fixtures.ts`: Maria with her active student Anna, Pavel with one empty group, and the existing Pavel with 31 active students, moved there from `groups.spec.ts`;
  - fixtures that are not automatic are built only for tests that ask for them, so the shared actors add no worker of their own;
  - the shared tests run in one worker per group (`mode: 'default'`), as the existing groups precedent did;
  - three accessibility tests use the existing worker-scoped tutor;
  - tests that change an account, a card, a group, a session or the clock still seed their own actors.

## Method and its limits

- **Machine:** the same development machine as the comparison records, with warm caches.
- **Load:** the machine was not idle, because other sessions were running their own checks. The one-minute load average was 15.7 at the start and 19.6 at the end, and the 15-minute load average was 12.8 and 14.0. This run is indicative only.
- **Commands:** one script ran the commands of the full local check run one after another and timed each one, as in the comparison records.
- **Discarded run:** an earlier run on this tree, under a load average that rose to 30, failed in `go test` with PostgreSQL's `too many clients` while other sessions' tests used the same server. The rerun below passed.
- **Result of the measured run:** every command exited 0.

## Results

| Measurement | Time (s) |
|---|---|
| Full run, wall time | 214.5 |
| `go test -race -count=1 ./...`, wall time | 42.0 |
| `pnpm e2e`, wall time, 619 tests | 118.4 |
| Documentation checks | 16.8 |
| Four OSV-Scanner scans and both Gitleaks scans | 8.8 |
| Frontend checks before `pnpm e2e` | 17.5 |
| Backend checks other than `go test` | 5.2 |
| Secret guard hook and local settings wrapper checks | 4.6 |

## Go tests in isolation

Two runs of `go test -race -count=1 ./...` alone, a few minutes apart under a one-minute load average of about 4 to 6:

- before the change, at `9fc1fdc`: 48.5 seconds wall time and 163 seconds of processor time;
- after the change: 29.9 to 30.8 seconds wall time and 52 to 53 seconds of processor time.

The template removes most of the processor time the integration tests spent on migrations, about a third of the wall time.

## End-to-end tests

- On `main` before this change, a run under similar load took 122 seconds, with four Firefox failures in tests this change does not touch; the replacement workers lengthened its Firefox phase.
- Most seeding is spent on registration, confirmation and sign-in. Across one full run the API logs record about 1,900 sign-ins at 57 ms each and 940 registrations at 107 ms each.
- Sharing actors removes about 30 seedings per run. That is a few seconds of summed test time and well under a second of wall time with six workers, smaller than the noise of a loaded machine.

## Against the target

- **Not yet decided.** D46's target is measured on an idle machine at the slice 4 closing commit. This loaded run cannot show whether the full run is at most 149 seconds.
- **The idle measurement is pending.** The coordinating session owns it and runs it once the remaining slice 4 frontend changes have merged and the machine is quiet. That run also serves as the slice 4 cold-run measurement.
- **What the numbers suggest.** The Go tests now take about 30 seconds alone instead of about 48. The end-to-end suites remain the largest part of the run and gain little from shared actors, so reaching 149 seconds depends mostly on how the end-to-end suites measure on an idle machine.
- **If the target is missed.** D46 says the owner is asked again with the measurement before S2 starts.
- **The 10-minute budget of NFR-6.2 is not at risk.**
