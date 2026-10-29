# Slice 5 performance measurements

Measured on 2026-10-29 for the [slice 5 review](../reviews/slice-5-review.md), on `main` at `dc32b48`, so after `fix/lesson-jobs-hardening` (horizon extension on its own scheduler loop, the one-hour reminder kind first, the status job's keyset) and `fix/lesson-effects-follow-ups` (the effect queries split by scope, the `tutor.deleted` listing driven from the roster's student index) had merged. The measurements cover four items of the review's measurement list: the reminder burst, tutor deletion at the caps, the first horizon extension after local midnight and the 100-lesson split. The fifth item, the cold full check run, is not part of this record. They also decide the two known limitations accepted pending a measurement. Agent-produced evidence, pending owner review.

## Machine and method

- **Machine:** Apple M4 Pro, 12 cores, 24 GB, macOS 26.2, Go 1.27.3; Docker Desktop running the Compose PostgreSQL 18.6 and storage services. The measurements ran one at a time. No merge gate, test run or lint process was running when each measurement started (checked with `ps`). The one-minute load average was between 2.0 and 4.2 throughout; the value before and after each measurement is given with it. A development deployment (an API with a pool of 20 on the `ovc_dev` database, a worker and the Vite dev server) ran alongside, idle, during the reminder measurements through `Tick`, the status burst and the first three reminder measurements through `Run`. It was stopped before the last two `Run` scenarios and every later measurement.
- **Harness:** temporary `_test.go` files in `backend/internal/api`, behind a `measure` build tag and an `OVC_MEASURE=1` guard. They were run with `/Users/klukashka/Work/ovc/tools/dev-env.sh go test -tags measure -run 'TestMeasure…' -count=1 -v ./internal/api/` and then removed from the tree; they are not committed. The harness is a copy of the integration-test harness of `internal/api` with three differences:
  - the API uses a pool of 20 connections and the worker its own pool of 10, the production defaults of `OVC_DB_POOL_SIZE`;
  - both pools carry a statement tracer that counts and times every statement by its sqlc name;
  - the worker is composed as in `cmd/internal/startup/compose.go`, with the email lane at 10 slots and the status, reminder and extension jobs, the extension on its own loop. The hourly housekeeping jobs and the storage-delete lane were left out.
- **Worker modes:** `Tick(now)` runs the jobs one after another and then drains the lanes, which gives deterministic job and dispatch figures. `Run` with a clock shifted by a fixed offset from the system clock runs the production loops: the shared per-minute scheduler, the extension's own loop and the lanes polling every second. Both modes use the real worker against a fresh migrated database. For `Run`, the `job_runs` rows were preset so that the status, reminder and extension jobs all fell due at the same instant.
- **Email:** the database mail sink, wrapped in a fake mailer that waits a fixed time before each send to simulate the provider. Runs used no wait (the dispatcher's own cost) or 200 ms per send, the simulated provider of the [slice 4 measurements](slice-4-review-measurements-2026-10-26.md).
- **Data:** seeded with SQL. Every seeded person uses English and `Europe/Moscow`, and every relationship is `active`. Measurement code waited on explicit conditions with deadlines; the only timed waits are the simulated send time and the 50 ms polling of the `Run` scenarios' completion condition.

## Summary

| Measurement | Result | Bound | Verdict |
|---|---|---|---|
| One-hour reminders, 100 lessons in one minute (780 recipients), 200 ms per send | all sent 16.4 to 17.4 s after the reminder run starts; at most about 78 s after the due time at the worst run phase | NFR-4.5: 2 minutes | passed |
| One-hour reminders, 500 lessons in one minute (3,900 recipients), 200 ms per send | all sent 80 to 87 s after the run starts; up to about 145 s after the due time at the worst run phase | NFR-4.5: 2 minutes | failed beyond NFR-3.1's volume (see below) |
| The same bursts with no provider wait | 1.3 s (100 lessons) and 6.4 s (500 lessons) from the run's start to the last email | none set | passed |
| The bursts while horizon extension runs on its own loop (5,000 and 50,000 series) | the reminder run starts within 15 ms of the due time; the last email is no later than without extension | NFR-4.5 | passed |
| Status job, 500 lessons ending in one minute | 0.98 s | 20 s budget | passed |
| First horizon extension after local midnight, 5,000 daily series | 10.7 s alone, 11.9 to 12.4 s beside a reminder burst | 2-minute budget | passed |
| The same at 50,000 daily series (the cap of every tutor) | 110.1 s alone, 120.0 s beside a burst, all 50,000 extended in the first run | 2-minute budget | passed, at the edge |
| Tutor deletion at the caps, half group lessons (11,400 lessons, 176,700 roster rows) | 1.10 s median (1.09 to 1.13 s); transaction and lock hold 1.08 to 1.13 s | 5 s (slice 5 plan) | passed |
| Tutor deletion at the caps, all group lessons of 30 (11,400 lessons, 342,000 roster rows) | 1.98 s median (1.96 to 1.99 s); transaction and lock hold 1.95 to 1.98 s | 5 s | passed |
| `tutor.deleted` listing per email at the cap | mean 1.7 to 2.6 ms, at most 32.6 ms; 17.2 ms and 37,475 buffers under `EXPLAIN ANALYZE` for a student on 11,400 cancelled lessons | none set | passed |
| Split of a 100-lesson series | p95 46 ms (one-to-one), 75 ms (group of 30); at most 80 ms | NFR-2.1: 500 ms for writes | passed |
| "Cancel following" of a 100-lesson series | p95 34 ms (one-to-one), 83 ms (group of 30); at most 88 ms | NFR-2.1: 500 ms | passed |
| Dispatcher's per-row reads | about 1.9 ms of statements per email without contention, about 3.2 ms with a 200 ms provider; about 7 % of the lane's throughput | none set | batching not needed |

## NFR-4.5: a burst of one-hour reminders

### Data and timing

- **Burst:** lessons all starting at 2030-03-11 22:00 UTC, each of its own tutor. A fifth of them are group lessons of 30 members, as the slice 5 plan sets the burst.
  - 100 lessons (NFR-3.1's concurrent lessons): 80 one-to-one lessons and 20 group lessons, 780 recipients (the tutor and the students of each lesson).
  - 500 lessons: 400 one-to-one lessons and 100 group lessons, 3,900 recipients.
- **Due time:** the one-hour kind's due time is 21:00 UTC. The run at that instant selects every lesson starting in the next 45 to 60 minutes.
- **Run phase:** in production the reminder job runs once a minute. Its next run is due one minute after the `now` of its last run, checked by a scheduler that ticks every second. A lesson is therefore picked up between 0 and about 61 s after its due time. The measurements start the run at the due time (phase 0), and the worst case adds 61 s.

### Through `Tick`

Each row is a fresh database. The load average was 2.00 to 2.59 before and 2.00 to 2.28 after.

| Lessons | Send wait | Reminder job | From the run's start to each email |
|---|---|---|---|
| 100 | none | 0.71 s, 780 outbox rows and 780 in-app rows | p50 1.03 s, p95 1.28 s, last 1.31 s |
| 100 | 200 ms | 0.65 s | p50 9.1 s, p95 16.8 s, last 17.4 s |
| 500 | none | 3.54 s, 3,900 outbox rows | p50 4.98 s, p95 6.25 s, last 6.41 s |
| 500 | 200 ms | 3.34 s | p50 45.2 s, p95 83.0 s, last 87.1 s |

`Tick` dispatches only after the jobs return, so these figures include the whole job time before the first send.

### Through `Run`

These runs used the production loops, 200 ms per send, and 1,000 normal-priority `tutor.deleted` emails queued before the due time, as NFR-4.5 requires. The extension candidates are daily series whose next occurrence enters the horizon at local midnight in Moscow, which is 21:00 UTC, the reminders' due time.

| Lessons | Extension candidates | Reminder run starts after due | Reminder run | From the due time to each email | Normal emails sent meanwhile | Load before / after |
|---|---|---|---|---|---|---|
| 100 | none | 7 ms | 0.82 s | p50 8.5 s, p95 16.2 s, last 16.9 s | 160 | 2.29 / 2.24 |
| 500 | none | 4 ms | 3.36 s | p50 41.8 s, p95 79.4 s, last 83.4 s | 160 | 2.24 / 2.09 |
| 100 | 5,000 | 13 ms | 1.03 s | p50 8.2 s, p95 15.6 s, last 16.4 s | 160 | 2.01 / 2.86 |
| 500 | 5,000 | 15 ms | 5.11 s | p50 41.8 s, p95 79.5 s, last 83.6 s | 150 | 2.87 / 3.36 |
| 500 | 50,000 | 12 ms | 4.95 s | p50 40.1 s, p95 76.3 s, last 80.2 s | 1,000 | 3.82 / 3.71 |

The extension ran 11.9 s, 12.4 s and 120.0 s in the last three rows, overlapping the whole reminder run and dispatch. In the last row all 1,000 normal emails went out because the scenario waited for the extension to finish. The status job ran in 2 to 6 ms each minute, with nothing to end.

### Findings

- **Throughput:** the email lane ran at about 46.5 emails per second with a 200 ms provider. That is 7 % under the ceiling of 10 slots × 5 sends per second. Reminders always went before the queued normal emails.
- **Extension:** running horizon extension on its own loop removed the delay it caused to the per-minute jobs. The one-hour run started within 15 ms of its due time while extension worked on 50,000 series. The reminder job itself took about 1.5 times as long beside extension (5.0 s against 3.4 s for 500 lessons), but the last email did not move. The finding "the extension job blocks the per-minute jobs after local midnight" is closed by measurement.
- **100 lessons (NFR-3.1's volume):** the last reminder went out 17 s after the run started, so at most about 78 s after its due time at the worst phase. This passes NFR-4.5 with a 42 s margin.
- **500 lessons, five times NFR-3.1's concurrent lessons:**
  - the last reminder went out 83 to 87 s after the run started;
  - it is late whenever the run's phase exceeds about 36 s;
  - at the worst phase, about 29 % of the burst's reminders arrive after 2 minutes, at most about 145 s after their due time;
  - averaged over a uniformly distributed phase, about 6 % do.
- **Limit:** the limit is the provider's send time × the lane's 10 slots, not the jobs and not the dispatcher's reads.
- **Status job:** 500 lessons ending in the same minute, 100 of them group lessons of 30, were ended in 0.98 s with 3,400 roster rows written. The load average was 2.39 before and after.

## The dispatcher's per-row reads

- **Statements per email:** 7.6 in both bursts. Each email claims its row and completes it. In between it reads:
  - the lesson (`GetLessonForEmail`);
  - the recipient (`GetUserByID`);
  - the preferences (`ListEmailPreferences`);
  - group membership, for group lessons only (`IsCurrentGroupMember`);
  - the daily cap slot, for students only (`ReserveRateLimitSlotBelow`).

  It then writes the sink.
- **Without contention:** in the 500-lesson burst with no provider wait, these five reads took 7.5 s of statement time for 3,900 emails, about 1.9 ms per email. Spread over 10 slots, that is 0.19 ms of wall time per email, and the whole burst was dispatched in 2.9 s, about 1,350 emails per second.
- **With a 200 ms provider:** the same reads took about 3.2 ms per email. That is roughly 1.5 % of each slot's time per email; with the claim, the completion and the sink write it amounts to the 7 % shortfall above.
- **Conclusion:** batching the reads per claimed batch would save at most a few milliseconds per email. It would not bring a 500-lesson burst under NFR-4.5, because the bound is set by provider latency and lane concurrency.
- **The reminder job:** 3.9 statements per recipient, plus one transaction per lesson of about 10 statements. 500 lessons took 3.3 to 3.5 s against the job's 20 s budget.

## The first horizon extension after local midnight

- **Data:**
  - 500 tutors with 10 active students each;
  - daily series in `Europe/Moscow`, either 5,000 (one per relationship) or 50,000 (ten per relationship, 100 per tutor, the per-tutor series cap of every tutor);
  - each series materialised through the day before the horizon of midnight and holding its 84 future lessons: 420,000 or 4.2 million lessons.

  Seeding took 8 s and 2 min 13 s.
- **Method:** through `Tick`, a run one second before local midnight, then the first run at midnight, then the next hourly run.
- **Machine state:** load average 3.70 before and 4.18 after for 5,000 series; 2.44 before and 3.24 after for 50,000.

| Series | Run before midnight | First run at midnight | Next hourly run | Statements |
|---|---|---|---|---|
| 5,000 | 9 ms, no candidate | 10.65 s, 5,000 series, 5,000 lessons added | 9 ms, no candidate | 8 per series, about 2.1 ms |
| 50,000 | 121 ms, no candidate | 110.1 s, 50,000 series, 50,000 lessons added | 80 ms, no candidate | 8 per series, about 2.2 ms |

- **Candidate query:** 2.3 to 2.5 ms per batch of 200 on average, with one slow first batch of 121 ms at 50,000 series.
- **Transaction cost:** the time is spread evenly over the transaction's statements: insert, set the horizon, the series lock, the relationship lock, the audience read, the signal, begin and commit, each about 0.2 to 0.4 ms.
- **Budget:** at 50,000 every-day series the first run uses 92 % of its 2-minute budget alone and all of it beside a reminder burst. At about 55,000 such series the remainder would wait for the next hourly run. The spec allows that, because the horizon is 12 weeks ahead and `materialised_until` may trail it.
- **Effect on the per-minute jobs:** none beyond the slower reminder run noted above, because extension runs on its own loop.
- **Old sequential order:** under `Tick`, which still runs the jobs one after another, the first run would delay the status and reminder jobs by the full 110 s.

## Tutor deletion at the per-tutor caps

- **Data:** the bound of the slice 5 plan:
  - 100 daily series materialised 12 weeks ahead (84 lessons each, 8,400 lessons) and 3,000 one-off lessons, so 11,400 future lessons;
  - 300 active students and 10 groups of 30 members.

  Three shapes:
  - **half group:** 50 group series and 1,500 group one-offs spread over the 10 groups;
  - **all group:** every lesson a group lesson, spread over the 10 groups;
  - **one group:** every group lesson in one group of 30, once, for the worst listing.

  The bound of about 12,100 lessons (91 occurrences per series) is about 6 % above this dataset; the figures scale roughly linearly with the roster rows.
- **Method:** `DELETE /api/v1/users/me` through the real API, three samples per shape on a fresh database each. The tracer recorded the request's statements and the transaction from its first statement to its commit.
- **Machine state:** load average 3.18 to 4.13 before and 3.18 to 4.13 after.

| Shape | Request | Transaction (lock hold) | Roster rows written | `FreezeEffectRosters` | `CancelLessonsOfTutorForEffect` | Signals (13 statements) |
|---|---|---|---|---|---|---|
| Half group | 1.134, 1.103, 1.085 s | 1.081 to 1.129 s | 176,700 | 0.80 to 0.85 s | 0.12 s | 0.08 s |
| All group | 1.955, 1.985, 1.976 s | 1.950 to 1.981 s | 342,000 | 1.59 s | 0.12 s | 0.13 s |
| One group | 1.959 s | 1.953 s | 342,000 | 1.60 s | 0.12 s | 0.13 s |

- **Statements:** 40 in the request and 34 in its one transaction. Every other statement took under 4 ms; queuing the 300 `tutor.deleted` emails took 3.7 to 4.8 ms.
- **Lock hold:** the transaction locks the tutor's relationships (`LockTutorRelationships`) right after the account and profile locks. The relationships stay locked `FOR UPDATE` for practically the whole transaction, 1.1 s to 2.0 s. What waits meanwhile is the jobs and student actions on that tutor's relationships, each for at most that time.
- **Comparison:** about 2.5 times under the plan's 5 s budget at the worst shape. The time is dominated by the single roster statement, which grows with the roster rows. The fallback the plan names (freezing rosters in the purge) is not needed. The review's performance risk on tutor deletion is closed by measurement.
- **`tutor.deleted` dispatch:** 300 emails were sent in 0.25 to 0.28 s with no provider wait.
  - `ListLessonsCancelledOnRoster` took a mean of 1.66 ms per email (half group), 2.56 ms (all group) and 2.31 ms (one group, at most 32.6 ms for the 30 members on all 11,400 cancelled lessons).
  - The plan reads the recipient's later roster rows through `lesson_roster_student_idx` and looks each lesson up by its key, memoized. Under `EXPLAIN (ANALYZE, BUFFERS)` it took 2.2 ms and 4,431 buffers for a member on 1,140 cancelled lessons, and 17.2 ms and 37,475 buffers for a member on 11,400.
  - The window count over every matched row is the cost that grows with the roster; at the cap it stays in milliseconds.

## Split and "cancel following" of a 100-lesson series

- **Method:** through the real API, one tutor, five samples per audience.
  - Each sample created a 100-lesson daily series by count (`POST /lessons` with `allow_overlap`), split it at its second lesson with a new start time (`PATCH` with scope `following`), created another such series and cancelled its following lessons from the second lesson (`POST …/cancel` with scope `following`).
  - Audiences: a one-to-one card, and a group of 30 active members.
- **Machine state:** load average 4.19 before and 3.94 after.

| Request | One-to-one | Group of 30 |
|---|---|---|
| Create 100 lessons | p50 9 ms, at most 15 ms | p50 14 ms, at most 15 ms |
| Split, 99 lessons moved | p50 43 ms, p95 46 ms, at most 49 ms | p50 72 ms, p95 75 ms, at most 80 ms |
| Cancel following, 99 lessons | p50 33 ms, p95 34 ms, at most 36 ms | p50 82 ms, p95 83 ms, at most 88 ms |

- **Per-row statements:** the split issues one `UpdateEditedLesson` per moved lesson (99 statements, about 0.34 ms each, about 33 ms per split). The cancellation issues one `FreezeLessonRoster` per cancelled lesson (99 statements, about 0.31 ms each, 30 rows each for the group).
- **Comparison:** at most about a fifth of NFR-2.1's 500 ms for writes.

## Recommendations for the known limitations and findings

| Known limitation or finding | Decided by | Recommendation |
|---|---|---|
| The dispatcher reads each claimed email's recipient, preferences and subject row by row ([slice 5 known limitations](../reviews/slice-5-known-limitations.md)), accepted pending the NFR-4.5 measurement with a burst of reminders | the dispatcher's per-row reads | **Accept.** Batching is not needed: the reads cost 2 to 3 ms per email against a 200 ms send. NFR-4.5 holds at NFR-3.1's volume. Remove "pending the measurement" and keep the row as accepted; nothing is carried into slice 10 for it. |
| A split or a "cancel following" of up to 100 lessons issues one statement per changed lesson, accepted pending the 100-lesson split measurement | the split and "cancel following" | **Accept.** At most 88 ms, under a fifth of NFR-2.1's bound; batched statements are not needed. |
| The lesson jobs check their budget only before each batch and set no lock or statement timeout, left to a measurement of the jobs under load | the reminder and status bursts and the extension at midnight | **Accept.** No run overran its budget except extension at 50,000 series, which reached it as designed. The reminder job used 3.3 to 5.1 s of its 20 s with 500 lessons, and the status job 1.0 s. |
| Performance risk: tutor deletion at the bound of the per-tutor caps (review finding) | tutor deletion | **Accept, no fix.** 2.0 s at worst against the 5 s budget, with the lock hold equal to the transaction. The fallback that freezes rosters in the purge is not needed. |
| The `tutor.deleted` email query, fixed now by `fix/lesson-effects-follow-ups` after the tutor deletion measurement | the `tutor.deleted` listing | **Confirmed fixed.** The listing is driven from the roster's student index, at most 32.6 ms per email at the cap. |
| The extension job blocks the per-minute jobs after local midnight (review finding, resolved by `fix/lesson-jobs-hardening`) | the bursts beside extension | **Confirmed closed.** At 50,000 every-day series the first run fills its 2-minute budget, which the spec allows. |
| New: a 500-lesson reminder burst, five times NFR-3.1's concurrent lessons, misses NFR-4.5 when the run's phase is late, up to about 145 s with a 200 ms provider | the 500-lesson burst | **Carry into slice 10**, as a new known limitation of minor severity, or ask the owner. It lies beyond NFR-3.1's scale. Either fix shortens the path without touching the per-row reads: more email slots or SMTP connections for high-priority mail, or a reminder run aligned to the start of each minute so that the run phase stops adding up to 61 s. |

## Unverified

- **Provider timing:** a real SMTP provider's send time and its concurrency limits; the 200 ms send is a simulation.
- **Reference deployment:** NFR-2's reference deployment (separate hosts for the API, the worker and PostgreSQL); everything here ran on one development machine.
- **Housekeeping jobs:** the hourly housekeeping jobs and the storage-delete lane were not part of the `Run` scenarios. They share the per-minute scheduler loop with the status and reminder jobs.
- **The full deletion bound:** tutor deletion at the full bound of about 12,100 lessons. The measured 11,400 is extrapolated linearly.
