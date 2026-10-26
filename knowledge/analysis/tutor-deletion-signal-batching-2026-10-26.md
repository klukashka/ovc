# Tutor deletion with batched signals

Measured on 2026-10-26 for the change that sends a tutor deletion's relationship signals in one `pg_notify` statement per 1,000 signals, the first follow-up of the [slice 4 review](../plans/slice-4-review-follow-ups.md). It repeats the failed tutor deletion measurement of the [slice 4 review measurements](slice-4-review-measurements-2026-10-26.md#tutor-deletion-at-50000-relationships) on the changed tree.

## Machine and method

- **Machine:** the same as the review's measurements: Apple M4 Pro, 12 cores, 24 GB, macOS 26.2, Go 1.27.3, Docker Desktop running the Compose PostgreSQL 18 and storage services. No other session ran checks during the measurement.
- **Load:** the one-minute load average was 0.94 before and 1.69 after.
- **Harness:** the review's temporary harness files for the API and for the deletion, unchanged, copied into `internal/api` of the changed working tree for the run and removed afterwards; they are not committed. They build the API as the integration-test harness does, with a database pool of 10 connections carrying a statement-counting tracer, and seed the dataset of the review: 30,000 active, 10,000 ended and 10,000 invited relationships with open invitations, 200 groups with 10,000 memberships and 40,000 confirmed students, on a fresh database for every sample.
- **Command:** `tools/dev-env.sh go test -count=3 -run TestPerfTutorDeletion -v ./internal/api/` from `backend/`, three samples without a listener and three with the real gateway and listener running.

## Results

| Sample | Without listener (s) | With listener (s) |
|---|---|---|
| 1 | 1.099 | 0.997 |
| 2 | 0.982 | 0.980 |
| 3 | 1.004 | 1.002 |

- **Median:** 1.00 s without a listener and 1.00 s with it, against 7.9 s before the change and the 0.95 s baseline of slice 3.
- **Statements:** 53 per request without a listener, against 30,023 before. The 30,000 signals now take 30 `NotifySignals` statements, 48 to 56 ms in total, where the 30,000 single `NotifySignal` statements took 6.79 s. The runs with a listener count 55 statements, the two more being the listener's own heartbeats. The other statements are unchanged; the largest are `InsertOutboxRows` (0.38 to 0.44 s) and `EndRelationshipsForTutorDeletion` (0.26 to 0.31 s).
- **Other outcomes:** every sample queued 30,000 `tutor.deleted` outbox rows, ended 30,000 relationships by deletion and discarded 10,000.
- **Not measured here:** the harness does not count the signals the listener received. Delivery of every signal to its student is asserted by the committed test `TestTutorDeletionStatementsDoNotGrowWithItsRelationships` in `internal/services/tutoring`, which listens on the channel for 3, 150 and 1,001 relationships, and by `TestTutorDeletionSplitsItsSignalsAndLocksNoStudentRow` through the gateway.

## Verdict

Passed against the review's finding: the deletion no longer grows by one round trip per relationship, and at 1.00 s it is about 5 % above the 0.95 s baseline, the remaining time being the set-based statements that the baseline also ran.
