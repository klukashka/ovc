# Slice 5 review

The slice review of [roadmap slice 5](../../docs/roadmap.md#slice-5-schedule-and-reminders), run on 2026-10-28 over everything merged after the commit "Record the slice 4 review and close the slice" (`f5b80fa`) up to "End a series on the lesson page and open New lesson prefilled with the same audience, time and weekdays" (`afd6d48`), against the approved [slice 5 plan](../plans/slice-5.md).

**Status: open.** The slice review is resolved only when each follow-up change below has merged and its reviewer has confirmed that the finding it names is closed.

## Reviewed range

`git diff f5b80fa afd6d48`, 19 commits:

| Area | Files | Lines |
|---|---|---|
| Backend (`backend/`) | 124 | +16,939 / −420 |
| Frontend (`frontend/`) | 101 | +10,103 / −308 |
| `docs/` | 8 | |
| `knowledge/` | 24 | |
| `tools/` | 3 | |
| `.claude/` | 1 | |

## Scope and reviews

One fresh independent reviewer per scope, against the clean `main` tree at `afd6d48` ([ADR-0011](../../docs/decisions/0011-tiered-verification.md)). The reviewers ran read-only and at the same time, so none of them ran the full suites, the end-to-end suites, the merge gate or a performance measurement; performance was assessed by reading queries, indexes, locks and bounds, and the coordinator runs the measurements one at a time on an otherwise idle machine.

| Scope | Components | Profiles | Why this split | Result |
|---|---|---|---|---|
| Backend: schedule core and jobs | Lesson creation, edits, splits and cancellation; series materialisation and horizon extension; the calendar and lesson reads; the status, reminder and extension jobs and the worker's job loop; the lesson migrations and queries | Correctness, performance | The backend changed in two families with separate data flows: lesson writes with the per-minute jobs, and the effects of tutoring actions with their notifications | No blocker or major; six minor findings |
| Backend: lesson effects and notifications | Lesson effects of relationship endings, group archiving and unarchiving and tutor deletion; lessons on the tutoring routes; lesson notifications and optional emails and their texts | Correctness, performance | See the row above | No blocker or major; four minor findings and one performance risk to measure |
| Frontend | The Schedule pages, the New lesson dialog and audience picker, the lesson page and its clock, end series and create new, lessons on the tutoring pages, live refetch, the timezone city table, the end-to-end specs of the slice | Correctness, browser, performance | One layer, one reviewer | One major; eight minor findings |
| Security: authorization and relationship scope | Lesson routes and frames by role and scope, student visibility of lessons and series, the roster, the idempotency and rate limits of lesson writes | Security | The slice touches more than one family of trust boundaries, so security is subdivided into fresh contexts, one per family | No blocker or major; two minor findings |
| Security: email, unsubscribe, worker jobs and observability | Lesson email texts and their encoding, unsubscribe links of the new kinds, the daily lesson email cap, the jobs' failure handling and diagnostics | Security | See the row above | No blocker or major; three minor findings |
| Security: frontend rendering and navigation, tooling, dependencies and agent configuration | Rendering of lesson titles and descriptions, navigation and focus of the new routes, the served-directory check, the merge gate and change checks, dependencies, `.claude/` | Security | See the row above | No blocker or major; three minor findings |
| Development tooling and agent configuration | `tools/merge-gate.sh`, the change checks, the end-to-end harness and run lock, the backend convention and process tests, the check-budget records, ADR-0018 and `docs/development.md` | Correctness, security | One layer, one reviewer | One major; seven minor findings and one information item |

Integration boundaries:

- **Schedule core and the tutoring effects**: both backend reviewers; the lesson effects reviewer owns the call side (what the tutoring services call in the schedule service, inside which transaction and under which locks).
- **API and frontend**: the frontend reviewer and the authorization security reviewer (what each role's schemas carry and what the pages derive from them).
- **Merge gate and end-to-end harness**: the development tooling reviewer.

## Findings

Severity is the reviewer's. Each finding names the follow-up change that resolves it; each follow-up is a standard-tier change with its own reviews, and its reviewer confirms the finding closed.

### Major

1. **Live refetch bursts are neither coalesced nor aborted** (frontend; `frontend/src/app/liveRefetch.ts`, the schedule query functions in `frontend/src/features/schedule/api.ts`, `frontend/src/lib/realtime/client.ts`). The reviewer reasoned that the realtime client forwards frames one by one, that each `lesson` frame invalidates its query keys at once and that the query functions take no abort signal, so a split or a "cancel following", which sends about 100 `lesson` frames to the tutor and up to 30 members, would make each open tab issue about 100 `GET /calendar` requests and refetch the card's Lessons pages about 100 times, around 3,000 requests within seconds. Closed as not confirmed (amended 2026-10-28, pending owner review): the premise did not hold. An end-to-end probe on `main` at `afd6d48` cancelled the following lessons of a 100-lesson series while the tutor's week Schedule was open, waited for all 100 `lesson` frames and counted the page's `GET /calendar` requests: 2 in each of 6 runs (Chromium, Firefox and WebKit, twice each). The application's query client, `FollowingQueryClient` in `frontend/src/app/queryClient.ts`, forces `cancelRefetch: false`, so a frame that arrives during a fetch joins it and adds one follow-up fetch afterwards; it never restarts or aborts the running fetch, and a burst costs about one fetch per fetch duration, not one per frame. Two ways of batching the frames' invalidations were tried and measured with the same probe. Batching for 100 milliseconds gave 1 `GET /calendar` in each of 9 runs, but a refetch that started 100 milliseconds after a live update was cancelled when a test navigated right after the visible change, and WebKit reported the cancelled fetch as a page error: the pilot 2 join-request journey failed 2 of 15 repeated WebKit runs, against 0 of 30 on `main`. Flushing on the next task passed 30 of 30 such runs but gave 2 `GET /calendar` in each of 9 runs, as on `main`, so it saved no requests. On 2026-10-28 the owner decided on no code change: no batching and no abort signal in the query functions, and the documented no-abort design of the query client stays ([live refetch mapping](../architecture/detailed.md#live-refetch-mapping)). The known limitation on bursts of lesson frames is removed as not confirmed for the same reason.
2. **The merge gate does not coordinate with the per-user end-to-end run lock** (tooling; `tools/merge-gate.sh` around its end-to-end step, `frontend/e2e/run-lock.ts`). A parallel session's `pnpm e2e` makes the gate's end-to-end step refuse with `RunLockRefusedError` after about 50 seconds of lanes; one tree failed the gate twice this way before passing on a third run, and run-lock refusal is not named as a rerunnable infrastructure error anywhere. A session's end-to-end run during the gate's lanes also adds about 70 database connections and can exhaust them. Resolved by `fix/merge-gate-heavy-checks-lock`: the gate takes the run lock or a machine-wide heavy-checks lock before its first lane and hands it to the end-to-end run, a change-check end-to-end run waits or refuses while the gate runs, and the failure rule names the refusal.

### Minor: backend, schedule core and jobs

1. **The extension job blocks the per-minute jobs after local midnight** (`backend/internal/worker/worker.go`, the job budgets in `backend/internal/services/schedule/jobs.go` and `compose.go`). Due jobs run one after another; horizon extension has a 2-minute budget, and every open series becomes a candidate once per local day, so the first run after midnight touches all series while status, reminders and outbox reclaim wait. A one-hour reminder can then be about 3 minutes late, outside NFR-4.5's 2 minutes. Resolved by `fix/lesson-jobs-hardening` (extension outside the per-minute loop or with a small resumable budget, and a series becomes a candidate only when its next occurrence after `materialised_until` falls within the horizon), after the coordinator's post-midnight extension measurement.
2. **The series-to-extend query does not match its index** (`backend/queries/lesson_jobs.sql`, `ListSeriesToExtend`; migration 00009). The query orders by `(materialised_until, id)` but the partial index covers only `materialised_until`, so paging reads about N²/400 rows. Resolved by `fix/lesson-jobs-hardening` with an index on `(materialised_until, id) WHERE ended_at IS NULL`.
3. **One-hour reminders can starve behind the 24-hour kind** (`jobs.go`, `QueueReminders`). Both kinds share one 20-second deadline and the 24-hour kind runs first. Resolved by `fix/lesson-jobs-hardening`: the one-hour kind first or its own budget, with `TestABurstOfLessonsIsWorkedThroughWithinTheBudget` adjusted to the order.
4. **A purged series turns into a job failure** (`jobs.go`, the lesson ending and reminder paths). `ErrNotFound` from `LockSeriesShared` after a purge is treated as an error, so the job logs a spurious ERROR and `job.run` reports a failure. Resolved by `fix/lesson-jobs-hardening`: treated like a missing lesson.
5. **Lesson writes read the clock before their lock wait** (`creation.go`, `edits.go`, `cancellation.go` in the schedule service). A write queued behind the acting tutor's lock may edit or cancel a lesson that started while it waited. Resolved by `fix/lesson-jobs-hardening`: the clock is read inside the transaction after `lockActingTutor`.
6. **Untested job paths** (backend tests). No test covers the jobs' retry-then-defer through `withAudienceRetry`, failure isolation of the reminder and extension jobs, or a calendar lesson ending exactly at `from`. Resolved by `fix/lesson-jobs-hardening`.

### Minor: backend, lesson effects and notifications

1. **Repeated archiving exhausts a shared student's daily lesson email cap** (group archiving and unarchiving). The routes have no rate limit, and each archiving sends `group.archived` in-app and by email to every member even when nothing is cancelled; a loop of archive and unarchive uses up a student's 20 lesson emails a day, so another tutor's reminders to that student are dropped as `daily_cap`. Resolved by `fix/lesson-effects-follow-ups`: archiving and unarchiving under a tutor write limit, or `group.archived` sent only when the archiving cancelled at least one lesson on the recipient's roster.
2. **The `group.archived` email overstates and drops its heading** (`lesson_effect_emails.go`, `core/lesson_effect_text.go`). The email says the group's future lessons are cancelled even when none were, omits the list heading, and `GroupArchivedText.Group` is never read. Resolved by `fix/lesson-effects-follow-ups`.
3. **A lesson under way at an ending or a tutor deletion loses its audience** (lesson effects). Such a lesson stays scheduled and is later ended by the status job with an empty current audience, so the student loses it from their history; archiving keeps the members and is not affected. Resolved by `fix/lesson-effects-follow-ups`, by the owner's decision: the roster of a scheduled lesson whose start is at or before the effect's instant is frozen in the effect's transaction.
4. **Effect queries cannot use their indexes** (`backend/queries/lesson_effects.sql`, `ListGroupMemberStudentsOfTutor`). The `OR` predicates over relationship and group scope defeat the start indexes under generic plans, the series walk includes ended series, and the member query scans `group_members`, which has no tutor index. Resolved by `fix/lesson-effects-follow-ups`: queries split by scope and the member query joined through `groups_tutor_id_idx`, after measurement.
5. **Performance risk: tutor deletion at the bound of the per-tutor caps** (lesson effects in `DELETE /users/me`). At the bound the request cancels about 12,100 lessons, writes about 360,000 roster rows and 12,100 signals, and holds every relationship `FOR UPDATE`. Unmeasured at the review; the coordinator measures the latency and the lock hold, and `fix/lesson-effects-follow-ups` acts on the result.

### Minor: frontend

1. **Date formatters are rebuilt per call** (`frontend/src/features/schedule/calendarFormat.ts` and similar helpers). Resolved by `fix/schedule-frontend-follow-ups`: one `Intl.DateTimeFormat` cached per locale and zone.
2. **Previous and next drop the calendar to its loading state** (`SchedulePage.tsx`). The scroll position jumps on each step. Resolved by `fix/schedule-frontend-follow-ups`: the previous data stays shown with a refetching indicator.
3. **"Today" is frozen at mount** (`SchedulePage.tsx`). A Schedule left open past local midnight highlights the previous day. Resolved by `fix/schedule-frontend-follow-ups`: a tick at local midnight.
4. **The card tabs' keep-focus rule has no focus test** ([shared behavior](../design/web/pages.md#shared-behavior)). Resolved by `fix/schedule-frontend-follow-ups` with a test asserting the focused element.
5. **End-to-end specs hold and abort requests through `page.route`** (`frontend/e2e/schedule.spec.ts` holds `POST /api/v1/lessons`; `frontend/e2e/language.spec.ts` holds and aborts language chunks). `AGENTS.md`'s rule against mocks names only the storage forwarder and the listener-test forwarder as request-holding exceptions. Resolved by `docs/e2e-request-holding-exception`, by the owner's decision: a narrow exception in `AGENTS.md` and ADR-0009 that lets an end-to-end spec hold a request and forward it unchanged, and abort one only for the language-chunk load failure test, never fulfilling or altering a response.
6. **The audience picker searches on every deferred keystroke** (`AudiencePicker.tsx`). Resolved by `fix/schedule-frontend-follow-ups`: the search is debounced.
7. **New lesson defaults its end date from the prefill, not from the date field** (`NewLessonDialog.tsx`). After the tutor moves the first date later, the default end date can fall before it and the server refuses the creation. Resolved by `fix/schedule-frontend-follow-ups`.
8. **The lesson clock does not refresh when the page becomes visible again** (`lessonTiming.ts`, `useLessonClock`). A tab that slept shows a stale timing state until the next tick. Resolved by `fix/schedule-frontend-follow-ups`; the known limitation on the browser clock names it.
9. **A Schedule end-to-end test depends on the weekday of the shared fake clock** (`frontend/e2e/schedule.spec.ts`, "schedule › on a coarse pointer the week lists lessons by day with 44 px targets"). It schedules lessons on `moscowTomorrow` and the day after and expects both days in one week; when earlier tests in the same Playwright worker have moved the shared fake clock so that `moscowTomorrow` is a Sunday, the second day falls in the next week and the test finds one day heading instead of two (seen once in Chromium, on the week of 2026-10-26 to 2026-11-01; amended 2026-10-28, pending owner review). Resolved by `fix/schedule-frontend-follow-ups`: the test picks a first date whose next day falls in the same week.

### Minor: security

1. **Students see a series' current pattern on lessons they hold only by roster** (authorization and relationship scope; T-6, T-21; the student branch of the lesson detail's series attachment in the schedule service's views). For a roster-only lesson, `GET /lessons/{id}` attaches the live series row, so a former member of a group sees the group's current pattern after later splits or a "cancel following", which also moves cancelled rows into the new series. Not confirmed at runtime. Resolved by `fix/student-lesson-series-visibility`: no series on a roster-only lesson for a student, with a test case.
2. **The student visibility rule exists in about five copies with a narrow agreement test** (authorization and relationship scope; T-5, T-6, T-44). `ListStudentCalendar`, `GetStudentLessonView`, `ListStudentNotificationLessons`, `IsOnLessonRoster` and the Go audience checks each restate it; `lesson_visibility_copies_test.go` compares three of them over seven cases and has no provisional, reclaim-pending or removed-link case. Resolved by `fix/student-lesson-series-visibility`: the agreement test covers every copy and those cases.
3. **Persistently failing lessons block the status job** (email, unsubscribe, worker jobs and observability; T-20; `ListLessonsDueToEnd` in `lesson_jobs.sql`, the status job in `jobs.go`). The query takes 500 lessons with no cursor, so failing lessons are retried and logged at ERROR every minute, and 500 of them at the head block every later lesson. Resolved by `fix/lesson-jobs-hardening`: a keyset cursor on `(end, id)` with a budget, as the reminder job has.
4. **Email-safe text misses some invisible characters** (T-31, T-14; `core/lesson_email_text.go`, `EmailSafeText`). The fixed strip list misses U+2061 to U+2064, U+180E, U+00AD, U+034F, the tag characters U+E0000 to U+E007F and the Hangul fillers U+115F and U+3164, so a lesson title can carry invisible text into a subject. Resolved by `fix/lesson-effects-follow-ups`: every `unicode.Cf` character and the Hangul fillers are stripped, and `TestTitlesInSubjectsAndBodiesAreStrippedAndEncoded` covers them.
5. **A deferred job item leaves no event** (observability; `jobs.go` and `horizon.go` discard `withAudienceRetry`'s result). Resolved by `fix/lesson-jobs-hardening`: a DEBUG `lesson.job_deferred` event in the code and in the [observability specification](../specs/observability.md).
6. **The merge gate passes the caller's Go environment** (T-41; `tools/merge-gate.sh`). `GOFLAGS`, `GOWORK`, `GOENV`, `GOTOOLCHAIN` or a `go env -w` setting can make `go test` run nothing and pass; the reviewer proved it with `GOFLAGS='-run=^$'`. Resolved by `fix/merge-gate-heavy-checks-lock`: the gate refuses those variables and changed Go settings beyond an allowlist and prints the effective values, or the residual is recorded in T-41 and ADR-0018.
7. **T-37 says `src/` holds only `.ts`, `.tsx` and `.css` files** (T-37). The source-types check now allows `src/lib/time/timezone-cities.json`. Resolved by `chore/tooling-follow-ups`, which amends T-37.
8. **T-36 still describes the removed `/__test__/unsubscribe-token` route** (T-36). Resolved by `chore/tooling-follow-ups`, which amends T-36.

### Minor: development tooling and agent configuration

1. **The hash-comment convention test walks other worktrees** (`backend/internal/conventions/conventions_test.go`, `hashCommentFiles`). Its walk from the repository root enters `.claude/worktrees`, where 731 of 759 matched files were found in 27 worktrees; it can only fail falsely. Resolved by `fix/merge-gate-heavy-checks-lock`: the walk lists files through Git or skips `.claude/worktrees`.
2. **The rerunnable error rule disagrees between documents**. The ovc-verify skill and ADR-0018 list `too many clients` as rerunnable, while `docs/development.md` calls lane-contention `too many clients` a gate bug with no rerun, and none names the run-lock refusal. Resolved by `fix/merge-gate-heavy-checks-lock`, which aligns the three and names the refusal.
3. **The process test's pool constant does not follow the test database's pool size** (`backend/cmd/internal/startup/process_test.go`: 2 connections per process against `testdb`'s default of 4). `TestProcessesRefuseUnsafeTestControl` holds one slot for about 31 children. Resolved by `chore/tooling-follow-ups`: the constant is derived from `testdb`.
4. **The deployment's stop throws after SIGKILL without awaiting exit** (`frontend/e2e/deployment.ts`, `stop`). The next worker's address check may find the port still held, a plausible cause of the WebKit failures on one worker's port. Resolved by `chore/tooling-follow-ups`: stop awaits the exit and logs which process missed its deadline.
5. **Playwright browsers run in their own process groups** outside the gate lanes' TERM and KILL coverage. Resolved by `chore/tooling-follow-ups`: documented or checked.
6. **Check-budget records are missing** for most slice 5 changes, the 173.3-second gate figure of the last frontend change (lanes A 48.4 s, B 39.9 s, C 49.8 s, end-to-end 122.4 s) is not recorded, the end-to-end shortening plan does not exist, and ADR-0018's review of `tools/check-changed.sh` at the close of slice 5 has not happened. Resolved by `chore/tooling-follow-ups` with the records and the ADR-0018 review, and by the plan `knowledge/plans/e2e-shortening.md`.
7. **Lane C is the longest lane** (47.6 s against lane A's 45.5 s). Resolved by `fix/merge-gate-heavy-checks-lock`: `check_guard_secrets.py` (5.6 s) moves from lane C to lane B.

### Information and observations

- **`golang.org/x/net` advisories**: the ignores expire on 2026-10-31 at 00:00 UTC, and the release-age rule allows the upgrade after 2026-10-30 at 19:00 UTC. The upgrade lands in that window, as its own dependency change.
- **golangci-lint's `allow-serial-runners`** waits without a bound on a shared lock, so a hung lint in another worktree stalls the gate's static lane. Resolved by `chore/tooling-follow-ups`, which bounds the wait or documents why it cannot.
- **The listener heartbeat test flakes** occasionally. The flake predates slice 5 and passed eight times in a row with `-race`; scheduler starvation is the likely cause. `chore/tooling-follow-ups` diagnoses it by logging the heartbeat instants; widening the loss margin would count as a raised timeout and needs the owner's decision.
- **End-to-end budget**: D46's 165-second target for the end-to-end run has been missed since the slice's schedule core change; the 178-second ceiling was met narrowly. 881 tests run on 6 workers, at 0.96 s per test in Firefox against 0.55 s in Chromium. The plan `knowledge/plans/e2e-shortening.md`, by the owner's decision, covers more workers with database pools of 3 and profiling of the Firefox overhead; it runs after `fix/merge-gate-heavy-checks-lock` and before slice 6.
- **First load** is 172.8 KB of the 180 KB entry budget, 3.2 KB more than at the slice 4 review; the timezone city table, about 7.9 KB in the lesson page's chunk, could be split per language.
- **Unsubscribe link parsing** accepts `http` in the mail package; production requires `https` through the public URL setting, so no change is needed.

## Measurements

The coordinator runs each measurement one at a time on an otherwise idle machine and records it in `knowledge/analysis/` before the follow-up it informs is planned:

- NFR-4.5 with a burst of reminders, which decides whether the dispatcher batches its per-row reads.
- Tutor deletion at the bound of the per-tutor caps: request latency and lock hold, and the dispatch of its `tutor.deleted` emails.
- The first horizon extension run after local midnight, and the delay it causes to the per-minute jobs.
- A split of a 100-lesson series, which decides whether the split's per-row statements are batched.
- The cold full local check run through the merge gate on an idle machine.

All five are pending at the time of this record.

## Threats

| Security family | Examined | Not affected | Unverified |
|---|---|---|---|
| Authorization and relationship scope | T-5, T-6, T-20 (its rate-limit part), T-21, T-29, T-34, T-44 | T-9, T-10, T-30 | |
| Email, unsubscribe, worker jobs and observability | T-17, T-18, T-20, T-21, T-27, T-31, T-32, T-36, T-44 | T-43 | T-34 in this family, examined by the authorization family |
| Frontend rendering and navigation, tooling, dependencies and agent configuration | T-14, T-19, T-36, T-37, T-41, T-44 (its interface) | T-27, T-40 | |

## Known limitations

The reviewers re-verified the [slice 5 known limitations](slice-5-known-limitations.md) as items to check. Verdicts:

- **Closed as resolved**, rows removed: the timezone city table test, the `too_many` field code, the card and group write answers without the lesson fields, the racing tutor deletion test that covered only creations, the untested `409 conflict` after members changed twice (for creations and edits; the job side is the untested job paths finding above), and the reminder and status jobs on archived groups.
- **Accepted**: the stale-template cleanup tests, the student purge's roster rows in one statement, the idempotency slot of a running request, the two-transaction card and group reads, the sign-out-everywhere wait list, the presence-count wait, the browser clock of the lesson page (now also naming the visibility refresh), the untested focus when a dialog closes by the browser's clock, and the fake transport's unverified accounts.
- **Accepted pending a measurement**: the dispatcher's per-row reads (the reminder burst; otherwise carried into slice 10) and the split's per-row statements (the 100-lesson split).
- **Carried into slice 7**: `live_at` on ended lessons, with the joining change.
- **Closed as not confirmed**: bursts of lesson frames, measured at 2 `GET /calendar` per open week Schedule for a 100-lesson "cancel following" (see the live refetch bursts finding; amended 2026-10-28, pending owner review).
- **Fixed now**: the `tutor.deleted` email query, which the lesson effects reviewer found understated, by `fix/lesson-effects-follow-ups` with a query driven from the roster's student index, after the tutor deletion measurement; the two end-series occurrence-date rows by `feat/lesson-occurrence-date`, which adds the tutor-only `occurrence_date` to the lesson detail and makes the lesson page judge the ending and prefill the new series from it, so an occurrence moved earlier opens New lesson after its series ended and one moved beyond the series' end and cancelled meanwhile closes onto the changed-meanwhile notice; both rows are removed (pending owner review).
- **No verdict at the review**: the NUL byte in a card's notes answering `500`; no layer report covered it. The owner decided on 2026-10-28 to fix it now in `fix/lesson-effects-follow-ups`: card notes refuse control characters other than line feed and tab with `422`.

## Owner decisions of 2026-10-28

- Every finding is fixed now, in the follow-up changes named above, at most three in parallel, with the performance risks measured before their fixes are planned.
- End-to-end specs may hold a request and forward it unchanged, and abort a request only for the language-chunk load failure test; they never fulfil or alter a response. The exception is recorded in `AGENTS.md` and ADR-0009 by `docs/e2e-request-holding-exception`.
- The tutor's lesson detail carries the occurrence date (`occurrence_date`) now, by `feat/lesson-occurrence-date`, instead of carrying it into slice 7.
- The roster of a lesson under way at a relationship ending or a tutor deletion is frozen now, by `fix/lesson-effects-follow-ups`.
- The end-to-end shortening plan is written now and executed after `fix/merge-gate-heavy-checks-lock`, before slice 6.

## Unverified

- The student series visibility finding at runtime.
- Every measurement listed above, until it is recorded.
- Behaviour behind a production reverse proxy and on the pilot deployment, which do not exist yet.
- Live conferencing, which slice 5 does not touch.
