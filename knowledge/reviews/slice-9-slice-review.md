# Slice 9 slice review

Status: review completed 2026-11-15; resolved 2026-11-15 when its two follow-up changes and the fix of the student card's tab change merged and the owner decided the remaining entries and the gate budget, as listed below. Agent record.

## Scope

- **Base:** `124624cb60ff91160c61885d8ffc15404aea93b1` ("Record the slice 8 slice review and close the slice"), the commit that closed slice 8, as D48 of the [slice 9 plan](../plans/slice-9.md#10-closing-the-slice) requires.
- **Reviewed head:** `52308fa0de5ef9f45563644960dc4a421045f702` ("Add the student's results, Home's Recently checked and the lesson page's linked work"), the last planned change. The review ran over `124624c..52308fa`.
- **Resolved head:** `5cef2e90d1a8ea6891804995dc31a2ddf6b1e6c8` ("Keep a student card tab change from cancelling the card's homework read"), 30 commits and 344 files after the base. The two review follow-up changes, `9ea647a` and `2236b88`, merged after the review, and `5cef2e9` fixed the failure that the two together caused in the first gate of this closing change; this record and the closing change follow them.
- **Changes in the range:**

  | Plan step | Commit | Change |
  | --- | --- | --- |
  | Outside the plan | `6a4d1f0` | Analyse where the end-to-end suite and the merge gate spend their time |
  | Plan | `e130813` | Add the slice 9 implementation plan: issuing, completing and checking |
  | Outside the plan | `be169ce` | Map the public web hygiene checks to the pilot deployment |
  | C0 | `bb31644` | Speed up the end-to-end suite with spec-group projects and stop failing gate runs after 10 failures |
  | S1 | `02483fb` | Specify the homework issuing, completing and checking contract |
  | B1 | `27207a2` | Add the homework domain rules: answer reveal, returns, submission transitions and scoring |
  | B2 | `4cf0401` | Add homework issues: issuing, tutor reads, changes and withdrawal |
  | B3 | `d15b9d0` | Add the answer and corrected upload contexts |
  | F1 | `a5dfb41` | Add the tutor's homework issuing and issued work pages |
  | B4 | `7a3c5b7` | Add homework submissions: student reads, autosave, file answers and submitting |
  | Outside the plan | `bff363e` | Record the rejected local PostgreSQL durability speed-up of the merge gate |
  | Outside the plan | `7ba47fd` | Record the no-go test-database pool prototype of the merge-gate speed-up |
  | Plan revision 6 | `3e978bc` | Amend slice 9's gate target and add the end-to-end consolidation C1 |
  | C1 | `162258f` | Consolidate end-to-end tests whose assertions Go and browserless tests hold |
  | Outside the plan | `c3d3e1b` | Forgive end-to-end request cancellations caused by client-side route changes |
  | C2 | `7b6b1c1` | Record the rejected file-copy clone and deferred-drop speed-ups of the merge gate's lane A |
  | Plan revision 7 | `3171833` | Record slice 9's lane A consolidation C2 and the suspension of the gate's consolidation rule |
  | Outside the plan | `14421a4` | End lock waits in erasure and settlement tests only after observing them |
  | Outside the plan | `36aeadf` | Keep a refetch in flight when Show more loads the next page |
  | B5 | `705a9a2` | Add homework reviews: drafts, corrected files, check, return and the review queue |
  | B6 | `9e9434f` | Erase and close homework through account deletion, relationship discard and issue marking |
  | F2 | `4719510` | Add the student's homework pages: assignment answering, autosave, file answers, submitting and Home's To do |
  | Outside the plan | `91a41ea` | Update go.opentelemetry.io/otel in the tools module to 1.47.0 for GHSA-q7gw-24fr-5p69 |
  | B7 | `c802432` | Add the lesson detail's linked issues for tutors and students |
  | Outside the plan | `089393e` | Wait for the realtime listener and guard process output in startup tests |
  | F3 | `e4d71f7` | Add the tutor's review queue, review screen and Today's homework panels |
  | F4 | `52308fa` | Add the student's results, Home's Recently checked and the lesson page's linked work |
  | Review follow-up | `9ea647a` | Resolve the frontend findings of the slice 9 slice review |
  | Review follow-up | `2236b88` | Close the slice 9 review's backend, test and end-to-end rule findings |
  | Closing gate fix | `5cef2e9` | Keep a student card tab change from cancelling the card's homework read |

- **Tier:** slice, under [ADR-0011](../../docs/decisions/0011-tiered-verification.md). Most changes met the security trigger: answer-key and score visibility, authorization and relationship scope on routes and frames, uploads and downloads with three new signed address kinds, purge, erasure and deletion, email content, rate limits, the amendment of the credentials-in-URLs rule of `AGENTS.md` and the end-to-end request-failure rule, which is verification policy.
- **Deferred to slice 10:** performance measurements wait for the deployment by the owner's decision of 2026-11-08, so the backend and frontend reviewers applied the performance profile by reading code.
- **Unverified:** a real-device check that iOS hands the camera picture over as JPEG, until the pilot, as the slice 9 plan's risks record.

## Reviewers and coverage

Eight reviewers covering four layers ran in fresh contexts. Security was split into five families of trust boundaries, because the slice touched more than one, and the frontend reviewer delegated end-to-end test quality to one fresh sub-review that it re-checked.

| Reviewer | Profiles | Components covered | Why |
| --- | --- | --- | --- |
| Backend | Correctness, performance (code-level) | The homework service (issues, submissions, answers, scoring, reviews, publication, student projections, lifecycle closure, erasure, cleanup, purges); migrations and queries of the new tables; the tutoring service's ending, removal, reclaim, tutor and student deletion and purge paths into homework; the `answer` and `corrected` upload contexts; the homework emails and in-app rows; the lesson detail's linked work; the worker's `homework.cleanup`; the startup test fix | Every backend package the range changes, and the account and relationship paths that reach the new tables |
| Frontend, with an end-to-end sub-review | Correctness, browser (dark theme, Firefox, Safari, by reading), performance | Synchronous route commits and the modal dialog close; Show more and first-new-row focus; the live refetch mapping and the Homework counts; access to Homework for provisional students; the tutor's issue, issued and review pages and Today's panels; the student's homework, assignment and results pages and Home's panels; the lesson page's linked work; notification rows; `check:bundle`; the spec-group projects, the consolidation, the diagnostics fixture and the new specs | Every frontend component the range changes, and the shared primitives reached through them |
| Security: answer keys and scores | Full security | Student schemas, routes and projections, the revealed object, the conventions and contract tests, emails, rows and frames for keys and points | T-9 and T-30 |
| Security: issue, submission and review authorization, frames and notifications | Full security | Tutor and student scope on every homework route, the readability predicate and its SQL copies, frame and notification recipients, email obsolete rules and caps, group issuing | T-5, T-6, T-21, T-29, T-31, T-44 |
| Security: uploads and downloads | Full security | The two upload contexts, their scope, caps and kind allowlist, and the mint and serve rules of `issue_snapshot`, `answer` and `review_item` | T-13, T-20, T-27, T-35 |
| Security: purge, erasure, deletion and quota charges | Full security | Erasure markers on every removal path, the cleanup job, the tutor and student purge batches, charge release and storage deletion | T-20, T-21, T-34 |
| Security: agent configuration, verification policy, dependencies and test control | Full security | The `AGENTS.md` amendment of credentials in URLs, ADR-0009, the request-failure rule, `--max-failures`, the spec groups, the entry budget, the tools module's OpenTelemetry update, the test-control surface | T-19, T-27, T-36, T-38, T-39, T-41 |
| Development tooling and agent configuration | Correctness, security for verification policy and dependencies | The merge gate's end-to-end entry, the change-checks table, C0, C1 and C2, the bundle checks, the gate records and every gate duration since C2, the known-limitations file | Development tooling, verification policy and test rules changed, and section 10 of the plan asks this reviewer for the gate durations |

## Integration boundaries and who reviewed them

| Boundary | Reviewer |
| --- | --- |
| Homework routes, service, repositories and PostgreSQL | Backend; authorization security for scope |
| Homework and the tutoring service (ending, removal, reclaim, deletion, purge) | Backend; purge security |
| Homework and the library (snapshots, references, charges) | Backend; purge security for charges |
| Upload pipeline and download minting and serving for the new kinds | Backend; upload security |
| Homework and notifications (outbox, rows, emails, frames) | Backend; authorization security for recipients; answer-key security for content |
| Student schemas and every student surface | Answer-key security; frontend for rendering |
| Live refetch mapping and the realtime frames | Frontend; authorization security for frame audiences |
| Homework pages over the typed API client | Frontend |
| Merge gate, change checks, bundle checks, end-to-end harness and rules | Tooling; agent-configuration security for verification policy |

## Results

| Reviewer | Blocker | Major |
| --- | --- | --- |
| Backend | 0 | 0 |
| Frontend, with the end-to-end sub-review | 0 | 3 |
| Security: answer keys and scores | 0 | 0 |
| Security: authorization, frames and notifications | 0 | 0 |
| Security: uploads and downloads | 0 | 0 |
| Security: purge, erasure, deletion and charges | 0 | 0 |
| Security: agent configuration and verification policy | 0 | 0 |
| Development tooling and agent configuration | 0 | 2 |

**Majors:**

- **Frontend: the student Homework list's Show more still cancelled an in-flight refetch.** Every other paged list had moved to the shared `loadNextPage` after the owner-approved investigation of 2026-11-13, so the same failure could recur on the student's list.
- **Frontend: Today's request-count assertion was loosened.** Counting reads per path and query string let any number of `/issues` reads pass, because Upcoming deadlines computes `deadline_before` from the clock on each fetch; `AGENTS.md` forbids a loosened assertion unless changing it is the approved task.
- **Frontend: specified focus moves without a test.** Focus moves of the Issue dialog, the issue page, the review queue and screen, the assignment page, Home's To do, Today's panels, the student card and the group page had no `toBeFocused` or `document.activeElement` test.
- **Tooling: gates over 600 s without a recorded owner decision.** Three passing gates broke NFR-6.2's 600 s, and the plan's check-budget risk requires each to go to the owner before the next merge, but no decision was in the repository.
- **Tooling: B7's regate not recorded as a rerun, and "rerun" undefined.** B7's tree `efb42fe` failed a check, and B7 was gated again unchanged as `967c447` on a `main` that held the separate OpenTelemetry change, with no row in the known-limitations file and no record of its approval in the repository; the same kind of gap was found by the slice 7 and slice 8 reviews.

**Minor findings and observations:**

- **Backend:** `LockLifecycleIssues` likely scanned the whole `issues` table in every ending, removal and deletion; the readability agreement test left out the three download mint copies of the predicate; lifecycle closure causes were inline string literals; `homework.md` still listed a signed-in invitation acceptance as a keep path, which the owner excluded on 2026-11-13; the plan's criterion named a test, `TestStudentPurgeSubmissions`, that does not exist.
- **Frontend:** a save sent while leaving the assignment page did not update the cached submission; slice 9 code adds to the next-frame focus pattern; the tutor's live refetch is broad; the student's linked work vanished silently when its texts failed to load; Upcoming deadlines keeps rows whose deadline has passed; three tests acted after asserting outside the pilot-step exception; the results page was outside the points sentinel scan; the request-cancellation forgiveness was wider than its stated purpose; the pilot 2 suite runs every actor in one language, timezone and theme.
- **Security, answer keys and scores:** point sentinels were never looked for in student email bodies; the conventions test missed embedded or reused non-student structs; under `after_check`, work with no manual questions reveals the key at submission, which follows the reveal rule and is not a security gap.
- **Security, authorization:** a points-only republication shows the student that some revision was published, through the revision identifier in the corrected-file address (the residual T-30 records); homework emails ignore the tutor's account state; issuing re-reads a group's members before, not under, the group lock, and `accounts.md`, `homework.md` and T-6 disagree about it; rate-limit slots are reserved before the scope check.
- **Security, uploads and downloads:** no test minted `answer` or `review_item` addresses across parties; the mint copies were outside the agreement test; a tutor gets an `answer` address that serving refuses while the relationship is `reclaim_pending` or discarded.
- **Security, purge:** T-13, T-20 and T-34 cited tests that do not exist; the cleanup released charges without locking the attachments; a probe test changed the database connection of the code under test; revision locks in the student purge are unbounded; an unconfirmed tutor's immediate deletion with library content could fail (outside the slice's diff); `OnAccountPurged` closes an active link without marking its work.
- **Security, agent configuration:** the off-plan change `c3d3e1b` relaxed the request-failure rule with no plan, review or threat mapping; same-document navigations widened the forgiveness; a typo in a WebKit cancellation text; the not-run count of a stopped end-to-end run, already recorded.
- **Tooling:** the request-failure relaxation and its bookkeeping were untested; the development guide and the threat model still cited the smoke for checks C1 moved; the student-reach bundle check could pass vacuously and left out the shared lesson page; the 181,000-byte entry limit was raised inside a feature change; a known-limitations row stood outside its table; ADR-0018 did not mention `--max-failures=10`.

## Follow-up changes

Each is a standard-tier change under the after-fix rule, names the findings it resolves, and its standard and security-profile reviewers confirmed them closed.

| Change | Findings | Confirmed closed by | Merged |
| --- | --- | --- | --- |
| Resolve the frontend findings of the slice 9 slice review | The three frontend majors: the student list on `loadNextPage` with an ESLint rule against bare `fetchNextPage` calls; Today's per-path bound restored, ignoring only `deadline_before`; tests for every listed focus move, the refused uploads through the fake transport's new refusal queue; and the minors of the cached save after leaving, the issue page's has-submissions alert focused after commit with the next-frame row widened, the linked work's text-load failure, the three Arrange–Act–Assert tests and the results page's sentinel scan | That change's standard and security-profile reviewers | `9ea647a` |
| Close the slice 9 review's backend, test and end-to-end rule findings | `LockLifecycleIssues` rewritten as an indexed union, proven by `TestLockLifecycleIssuesUsesIndexes`; the mint copies in the agreement test and the cross-party mint refusals; typed lifecycle causes; whole-number point sentinels in emails (`TestPointSentinelTextScan`); embedded and reachable structs in the conventions test; the attachment locks in the cleanup; statement counting from the database side through `pg_stat_statements` instead of a traced pool; the unconfirmed tutor's immediate deletion running the tutor purge batches; the stale test names and smoke citations in T-13, T-16, T-20, T-27 and T-34; the request-failure rule tightened to `GET` requests across a route change, with a pure recorder and unit tests, recorded in T-41; the WebKit cancellation text corrected; the bundle check failing on an empty forbidden folder and covering the shared lesson page | That change's standard and security-profile reviewers, and a scoped re-reviewer of its fixes | `2236b88` |
| Keep a student card tab change from cancelling the card's homework read | Not a review finding: the first gate of this closing change failed Chromium and Firefox `students.spec.ts:593` on a read cancelled at a `?tab=` switch, which the two follow-ups caused together on `main`; the owner chose on 2026-11-15 to fix it first as its own change | That change's own reviews | `5cef2e9` |
| This documentation change, which also closes the slice | The two tooling majors, by the owner decisions, the rerun definition and the gate records below; the keep-path sentence, the tutor's issue frame on submission, the `after_check` clarification and the group-member re-read order in `homework.md` and `accounts.md`; the stale CORS citation in T-24 and the development guide; T-6's re-read order; the owner's acceptance date in T-30; T-9's `after_check` residual; the test name in the plan; the row outside the table, the late rows of nine failed gates and every open row of the [slice 9 known limitations](slice-9-known-limitations.md); `--max-failures=10` in ADR-0018; this record | The standard and security-profile reviewers of this change | this commit |

The wording added to `docs/` and `AGENTS.md` is pending owner review.

The review's remaining items have these resolutions. The rate-limit slot reserved before the scope check is accepted: it follows the specified refusal order (`429` before `404`), and the per-caller hourly slot bounds it. The typo in the WebKit cancellation text was fixed in `2236b88`. The tutor's `answer` address that serving refuses during a reclaim window and `OnAccountPurged`'s active-link case are accepted rows of the known limitations. Issuing's re-read of a group's members after the relationship locks and before the group lock is safe, because every writer of group membership either holds the tutor's profile lock, which issuing holds first, or takes the relationship `FOR UPDATE`, which conflicts with issuing's `FOR SHARE`; `accounts.md`, `homework.md` and T-6 now say so and name that dependency.

## Gate reruns and ratifications

The failures rule of [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md) allows one rerun only for a named infrastructure error with no failed assertion. Every slice 9 gate that failed and was gated again is in the reruns table of the [slice 9 known limitations](slice-9-known-limitations.md#gate-reruns-and-failed-gates-before-an-after-fix-gate), which now gives each row's kind under the definition the owner decided on 2026-11-15; this change adds the missing rows, recorded late: the after-fix predecessors B2 `e469561`, F1 `c8964dc` (second gate), B6 `41b313e` and F2 `f37e592` and `a1cfda4`, and the reruns of F1 `c8964dc` and `4392287`, of B5 `1a061ef` and of B7 `efb42fe`:

- **Rerun:** a new gate of a change whose own diff is unchanged since its failed gate, apart from its rows in the known-limitations file, even when `main` moved and even when the new `main` holds a fix for the failure. It needs the owner's approval, except the one infrastructure rerun.
- **After-fix gate:** a gate counts as after-fix only when the change's own diff changed and the readiness report names the failure it fixes; in doubt it is a rerun. It follows the after-fix rule and needs a recorded row for its failed predecessor, but no approval.

Every rerun of slice 9 was approved by the owner by a multiple-choice question before it ran, except one. Three approved reruns had no row recorded before their gate, although `AGENTS.md` has required that record since 2026-11-11; their rows are now recorded late, with the approvals the coordinator's record of the owner's answers gives:

- **F1's regate of tree `c8964dc`,** approved on 2026-11-12: the laptop had slept on battery while Chromium tests hung, and the owner chose to count it as the one infrastructure rerun, without rebuilding F1.
- **B5's regate from tree `1a061ef` to `4a6fd07`,** approved on 2026-11-12: fix the cause of the aborted requests behind `notifications.spec.ts:64` and `builder.spec.ts:792` as its own change first, then gate B5 on the new `main`.
- **B7's regate from tree `efb42fe` to `967c447`,** approved on 2026-11-13: update the tools module's OpenTelemetry to 1.47.0 as its own change (`91a41ea`), then gate B7 again. B7's tree `efb42fe` had failed in lane C on OSV-Scanner's advisory GHSA-q7gw-24fr-5p69 for `tools/go.mod`, a failed check, not an infrastructure error, and B7's own diff was unchanged, so the regate was a rerun.

The owner ratified three items on 2026-11-15, each as a one-time exception for its change that sets no precedent:

1. **F1's regate of tree `4392287`.** That tree failed only because OSV-Scanner exited 127, an error ADR-0018 does not name as an infrastructure error, and F1's infrastructure rerun had already been used; the same tree was gated again and passed, without the owner's approval and without a row. The slice review's re-check of the gate log found it, and its row is now recorded late.
2. **The backend review follow-up's merge.** The owner's decision of 2026-11-15 to merge both review follow-ups and consolidate first in slice 10 was taken after the frontend follow-up's 639.9 s gate and before the backend follow-up's gate ran; that gate then took 716.1 s, and the change merged on the strength of the earlier decision, not of a decision on that duration.
3. **The entry limit of 181,000 bytes.** The owner raised `check:bundle`'s entry limit from 180,000 bytes by quiz on 2026-11-14, but the raise landed inside F3, the tutor review interface, rather than as its own change with its own reviews. From now on a check-budget change lands as its own change.

This is the third slice in a row whose review found regates without a record in the repository (slices 7, 8 and 9), and the gate cannot detect it, so the rerun definition is added to `AGENTS.md` as the rule that prevents it, as `AGENTS.md` asks for a recurring finding.

The backend review follow-up's first gate, tree `02ee873`, failed `check:bundle` because the frontend follow-up, which merged first, had moved the Issue dialog's refusal mapping into `issueProblems.ts`, newly reached from the shared lesson page; the change pinned that module and was gated again as tree `32a3240`, an after-fix gate the owner approved on 2026-11-15.

The first gate of this closing change, tree `5d98388` on 2026-11-15, failed two end-to-end tests outside its diff: Chromium `students.spec.ts:593` and the same test for Homework in Firefox, each a `request_failure` from a read cancelled on a `?tab=` switch, which the two review follow-ups cause together on `main`. The owner chose by quiz the same day to fix the cause first as its own change and to gate this change again on the `main` that holds the fix, a rerun under the definition above; its row is recorded in this change's own commit before that gate.

## Owner decisions

Each was answered by a multiple-choice question.

- **2026-11-14, the OpenTelemetry change's gate at 615.4 s:** merge and keep watching the gate's duration.
- **2026-11-14, B7's gate at 769.3 s:** merge; the owner stopped another project's load on the machine.
- **2026-11-15, F4's gate at 700.3 s:** merge and record the time.
- **2026-11-15, the frontend review follow-up's gate at 639.9 s:** merge both review follow-ups, then consolidate first in slice 10. The backend follow-up's 716.1 s gate merged under this decision, which the owner ratified the same day, as the ratifications above record.
- **2026-11-15, closing the review's findings:** three changes, the frontend follow-up, the backend, test and end-to-end rule follow-up, and this documentation change.
- **2026-11-15, the off-plan change `c3d3e1b`:** tighten its request-cancellation forgiveness rather than ratify it as it was, and record it under T-41; `2236b88` did both.
- **2026-11-15, counting database statements in tests:** no traced connection pool; count from the database side. The owner also approved recreating the shared development PostgreSQL container with `pg_stat_statements` preloaded, which the coordinator did; `2236b88` implements the counting.
- **2026-11-15, the rerun definition** above, written into `AGENTS.md`, ADR-0018 and the development guide (pending owner review), with a "Kind" column in the reruns table.
- **2026-11-15, the backend review follow-up's regate** after the bundle-check failure on tree `02ee873`: approved.
- **2026-11-15, the ratifications** above: F1's regate of tree `4392287`, the backend review follow-up's merge and the entry limit.
- **2026-11-15, the check budget:** a standing rule in ADR-0018 (pending owner review). The hard limit stays NFR-6.2's 600 s. A consolidation is due when the median of the last three passing full-run gates on mains power exceeds 540 s, a gate whose power source is unknown counting as on mains. A passing gate of 600 s or more goes to the owner before that same change merges. The last three passing gates, all on mains, are F4 at 700.3 s, the frontend review follow-up at 639.9 s and the backend review follow-up at 716.1 s, with a median of 700.3 s, so slice 10 starts with a consolidation, which also makes the gate print its power source and its load at start and end.
- **2026-11-15, the homework emails' tutor-state rule** (an email queued before a tutor's block or deletion can name that tutor): fixed in slice 10, with the specification rule in `notifications.md`.
- **2026-11-15, a blocked tutor's students keep starting `answer` uploads charged to that tutor:** accepted as intended behaviour, as D36 and `files.md` specify.
- **2026-11-15, the tutor's extra issue frame on submission and the excluded keep path:** align the specification with the code.
- **2026-11-15, residuals accepted:** a points-only republication reveals that a republication happened, through the revision identifier in the corrected-file address (T-30); under `after_check`, work with no manual questions shows the key right after submission, which `homework.md` and T-9 now state, and a hint for tutors at issuing is carried into slice 10.
- **2026-11-15, the remaining known-limitation rows:** every open row is accepted, or carried into slice 10, as its re-verifying reviewer recommended; the measurement rows join slice 10's performance measurements; the major row on the assignment page's refusal focus moves is carried into a named slice 10 change, with the parts the frontend follow-up closed marked fixed.

## Gate durations since C2

From the tooling reviewer's table, checked against the gate log, and the coordinator's figures for the gates after the review. Power comes from the machine's power log; "mains" for the last three is the coordinator's record.

| Gate | Base | Tested tree | Load at start | Wall time | Result | Power |
| --- | --- | --- | --- | --- | --- | --- |
| C2 record | `c3d3e1b` | `f01e25c` | 3.16 | Markdown-only | passed | |
| Plan revision 7 | `7b6b1c1` | `025bd8d` | 10.45 | Markdown-only | passed | |
| B5, first | `3171833` | `4a6fd07` | 4.67 | 10,586.6 s | failed in lane A | battery, slept |
| B5, second | `3171833` | `84755d8` | 6.15 | 219.3 s | failed in lane A | |
| Lock-wait test fix `14421a4` | `3171833` | `88687ca` | 5.24 | 588.3 s | passed | mains |
| B5, third | `14421a4` | `f57dc0d` | 18.83 | 600.9 s | failed, end-to-end | mains |
| Show more fix `36aeadf` | `14421a4` | `79608b4` | 6.54 | 561.5 s | passed | not logged |
| B5 `705a9a2` | `36aeadf` | `75ead40` | 12.80 | 537.1 s | passed | mains |
| B6, first | `705a9a2` | `41b313e` | 10.24 | 169.3 s | failed, documentation checks | |
| B6 `9e9434f` | `705a9a2` | `a6d641f` | 4.85 | 497.8 s | passed | mains |
| F2, first | `9e9434f` | `f37e592` | 10.25 | 511.0 s | failed, end-to-end | not logged |
| F2, second | `9e9434f` | `a1cfda4` | 5.24 | 551.6 s | failed, end-to-end | mains |
| F2 `4719510` | `9e9434f` | `d36f82c` | 1.38 | 550.3 s | passed | battery |
| B7, first | `4719510` | `efb42fe` | 2.42 | 181.3 s | failed, OSV-Scanner | |
| OpenTelemetry change `91a41ea` | `4719510` | `6863798` | 2.73 | 615.4 s | passed | battery |
| B7 `c802432` | `91a41ea` | `967c447` | 4.08 | 769.3 s | passed | battery, woke from sleep 90 s before |
| F3, first | `c802432` | `dec762c` | 5.88 | 176.8 s | failed in lane A | |
| Realtime test fix `089393e` | `c802432` | `03a6188` | 13.55 | 578.4 s | passed | not logged |
| F3, second | `089393e` | `7b23e4f` | 15.87 | 624.3 s | failed, end-to-end | mains |
| F3, third | `089393e` | `c8ccad1` | 2.37 | 670.6 s | failed, end-to-end | battery, then mains |
| F3, fourth | `089393e` | `6bacef4` | 7.55 | 620.4 s | failed, end-to-end | battery from a minute after the start |
| F3 `e4d71f7` | `089393e` | `80c6007` | 1.76 | 521.7 s | passed | mains |
| F4 `52308fa` | `e4d71f7` | `663be8b` | 3.24 | 700.3 s | passed | mains; another project's load started mid-run |
| Frontend review follow-up `9ea647a` | `52308fa` | `80ba1f9` | 1.9 | 639.9 s | passed | mains |
| Backend review follow-up, first | `9ea647a` | `02ee873` | | 200.2 s | failed, `check:bundle` | |
| Backend review follow-up `2236b88` | `9ea647a` | `32a3240` | | 716.1 s | passed | mains |
| This closing change, first | `2236b88` | `5d98388` | 1.75 | 664.4 s | failed, end-to-end | mains |
| Student card tab fix `5cef2e9` | `2236b88` | | | 586.9 s | passed | |

- **Passing full runs:** 12, from 497.8 s to 769.3 s; none reached D2's 480 s target, and five passed 600 s (615.4, 639.9, 700.3, 716.1 and 769.3 s), each taken to the owner before its merge or, for 716.1 s, ratified afterwards, as listed above.
- **Patterns, from the tooling reviewer:** the end-to-end suite grew from 1,859 to 2,102 runs through F2 to F4, and its test phase from about 6 to about 8 minutes, while the rest of the gate stayed near 160 to 280 s; several of the slowest gates ran on battery; the load at start predicts little, because load that starts during a run is not recorded; gates of similar content vary by about 100 s, so a single-gate threshold is noisy, which is why the check-budget rule uses a median of three.

## Threats examined

| Threat | Reviewers | Result |
| --- | --- | --- |
| T-5 A tutor reaches another tutor's work | Authorization, uploads, purge | Examined; cross-party mint refusals added in `2236b88` |
| T-6 A student reaches another student's work or a group they left | Authorization, uploads, purge | Examined; the group re-read order aligned in the documents by this change |
| T-9 Answer keys leak before release | Answer keys, authorization | Examined; test gaps closed in `2236b88`; the `after_check` behaviour clarified in `homework.md` and accepted in T-9 |
| T-13 Malicious uploads | Uploads | Examined; stale test name corrected in `2236b88` |
| T-18 Personal data in logs or emails | Answer keys, authorization, uploads, purge | Examined; the queued-email tutor-name residual stays as recorded |
| T-19 Secret disclosure | Agent configuration, uploads | Not affected |
| T-20 Resource exhaustion | Authorization, uploads, purge | Examined; the unbounded revision locks of the student purge carried into slice 10 |
| T-21 A removed participant keeps access | Answer keys, authorization, uploads, purge | Examined |
| T-27 Tokens in URLs | Agent configuration, uploads, authorization, purge | Examined; not changed |
| T-29 Stale WebSocket or client access | Answer keys, authorization, purge | Examined |
| T-30 Inferring the key from scores | Answer keys, authorization | Examined; the points-only republication residual accepted by the owner on 2026-11-15 |
| T-31 Email caps | Authorization | Examined; the tutor-state rule for homework emails carried into slice 10 |
| T-34 Data retained after deletion | Uploads, purge | Examined; attachment locks and the unconfirmed tutor's purge added in `2236b88` |
| T-35 Header injection and sniffing | Uploads | Examined |
| T-36 Test-control surface | Agent configuration, purge | Not affected |
| T-38 Local infrastructure | Agent configuration | Not affected in the reviewed range; `pg_stat_statements` added and recorded in `2236b88` |
| T-39 Dependencies and tools | Agent configuration | Examined; the tools module's OpenTelemetry update verified for release age and checksums, the advisory text unverified offline |
| T-41 Agent configuration and verification policy | Agent configuration, tooling | Examined; the request-failure relaxation tightened in `2236b88`; the rerun definition, the check-budget rule and the ratifications recorded by this change |
| T-44 Unconfirmed accounts and reclaim | Answer keys, authorization, purge | Examined |

## Known limitations re-verified

- **Slice 9 entries:** every entry of the [slice 9 known limitations](slice-9-known-limitations.md) was given to the reviewers of its layer as an item to re-verify and now carries its resolution: fixed, accepted by the owner, or carried into slice 10.
- **Earlier entries:** the entries carried into slice 10 from the slice 8 and design slice reviews stay carried there.

## Closing evidence

- **Pilot segments:** the passing full gates of `9ea647a` (tree `80ba1f9`) and `2236b88` (tree `32a3240`) ran the whole end-to-end suite in Chromium, Firefox and WebKit, including the pilot segments slice 9 adds or extends: pilot 2 steps 6 to 14 and 16, the pilot 3 seed and steps 3 to 5, and pilot 4 step 5, as section 10 of the plan requires.
- **Findings:** every finding of the review is fixed and confirmed closed by the reviewers of `9ea647a` or `2236b88`, recorded by this change, accepted by the owner, or carried into slice 10, as the table above and the known limitations list.

## Carried into slice 10

- the end-to-end consolidation that the check-budget rule requires before further features, with the gate printing its power source and its load at start and end;
- the tutor-state rule for homework emails;
- the hint for tutors at issuing that `after_check` work with no manual questions shows the key at submission;
- the Firefox `students.spec.ts:508` flaky test, as its own fix;
- the `popstate` residual, in which Back and Forward still commit in a transition;
- an end-to-end or page test of the assignment page's `validation_failed` and `507` focus moves;
- the owner question whether an after-fix gate needs a reviewer to confirm that the changed diff addresses the named failure;
- the next-frame focus of the shared notices, the narrower live refetch mapping, the refresh of Upcoming deadlines when a listed deadline passes, the pilot 2 actors' own language, timezone and theme, and the notification rows' text-load state;
- the performance measurements: the per-issue `EXISTS` of the tutor's filters, the autosave's three transactions, the review save's full schema, the student purge's revision locks, the slow `POST /auth/refresh` under gate load and the slice 6 measurement harness's traced connection pool.

## Resolution

The review was resolved on 2026-11-15. The three frontend majors are closed by `9ea647a`, each confirmed by its reviewers. The two tooling majors are closed by the owner decisions, the rerun definition in `AGENTS.md`, ADR-0018 and the development guide, the late rows of the failed gates and the ratifications this record and ADR-0018 hold. Every minor finding is fixed by `9ea647a` or `2236b88`, recorded by this change, accepted by the owner, or carried into slice 10, as the [slice 9 known limitations](slice-9-known-limitations.md) list.
