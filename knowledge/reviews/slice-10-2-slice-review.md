# Sub-slice 10.2 slice review

Status: review completed 2026-11-20; resolved 2026-11-20 when its follow-up changes had merged and the owner decided the remaining entries, the two gate breaches and the consolidation question, as listed below. Agent record.

## Scope

- **Base:** `c28c0e83f172579b41e99094179893701771379b` ("Record the sub-slice 10.1 slice review and close the sub-slice"), the commit that closed sub-slice 10.1. The reviewers' cumulative diff started at `2ad5c2e`, the last planned change of 10.1, because the 10.1 follow-ups in 10.2's list merged before `c28c0e8`.
- **Reviewed head:** `50b93380fb180ecc705d3b6ad26446348292dfc8` ("Move the issued notice and the removed-list fallback focus after commit"), the last change of 10.2's list. Reviewers read each change with `git show` and the cumulative view with `git diff 2ad5c2e..50b9338`, ignoring the 10.1 record inside that range except where it stated what 10.2 must cover.
- **Resolved head:** `main` at `f347b78` ("Record the rerun of the Today Show more fix's gate and the assignment request-failure flake") when this record was written; this record's own commit closes the sub-slice.
- **Sub-slice rule.** Each sub-slice's review covers its own list of changes in section 7 of the [slice 10 plan](../plans/slice-10.md#7-changes-in-order), with any inserted consolidation, its plan revision and the follow-ups of the previous sub-slice's review that merged after that review started (D1, [`AGENTS.md`](../../AGENTS.md#development-verification)). Every commit on `main` since `c28c0e8` falls in exactly one list below. The [10.1 record](slice-10-1-slice-review.md#scope) put its own commit, `c28c0e8`, in 10.2's list. It merged inside the reviewed range `2ad5c2e..50b9338`, but the five layer reviewers were told to ignore the 10.1 record there, so they did not cover it; by the owner's decision of 2026-11-20 a sixth, scoped reviewer reviewed it. One rule holds for closing records: a sub-slice's closing record is in the next sub-slice's list and is reviewed by that sub-slice's review, as this record is in 10.3's list.
- **10.2's list (reviewed by this review):**

  | Plan step | Commit | Change |
  | --- | --- | --- |
  | 10.1 review follow-up | `38eb0c9` | Log the merge gate's wall time, phase loads and other containers |
  | 10.1 review follow-up | `8f112a0` | Tighten after-fix evidence and record the idle-machine gate rule |
  | 10.1 review follow-up | `65a9289` | Resolve the sub-slice 10.1 slice review findings |
  | S2 | `528db96` | Specify the results and home pages contract of slice 10.2 |
  | B1 | `cb73714` | Serve the tutor's Today and the student's Home from one GET /home |
  | B2 | `bc400a9` | Dismiss the tutor's onboarding checklist and count open assignments on the student list |
  | B4 | `4da7cba` | End a student's reads of a deleted tutor's work at account_closed_until |
  | 10.1 review follow-up | `f1855cf` | Record the lane A profile and the slower parallel parent tests |
  | 10.1 review follow-up | `1a4fe67` | Propose letting the subtests of one integration test share a test database |
  | B3 | `bfff5c0` | Mark checked feedback as seen with feedback_new and POST /submissions/{id}/feedback-seen |
  | F2 | `e11f01f` | Show the issuing hint for after-checking work with no manual question |
  | 10.1 review follow-up (scoped reviewer) | `c28c0e8` | Record the sub-slice 10.1 slice review and close the sub-slice |
  | F1 | `405360f` | Serve Today and Home from GET /home with Dismiss, the New marker and open assignments |
  | Inserted consolidation | `608030a` | Record the marginal cost of a test database and stop the proposal 0005 pilot at its measurement |
  | No list (joins 10.2) | `11b1019` | Wait for Today's catch-up refetch before Show more in the join-requests test |
  | No list (joins 10.2) | `99120a4` | Require yes or no power-log fields in the merge gate self-test when the macOS power log is readable |
  | No list (joins 10.2) | `50b9338` | Move the issued notice and the removed-list fallback focus after commit |

  `11b1019` fixed a flake that failed the first gate of `99120a4`; `99120a4` and `50b9338` were carried into 10.2 by the 10.1 review. None of the three is in section 7's list of 10.2, so each joins 10.2, the earliest sub-slice whose review had not started when it merged.
- **10.3's list, as far as it has merged (reviewed by the 10.3 slice review):** the follow-ups of this review merged after it started, so they are in 10.3's list, and this record names, for each finding, the follow-up that resolved it.

  | Plan step | Commit | Change |
  | --- | --- | --- |
  | 10.2 review follow-up | `4f291ef` | Record the sub-slice 10.2 slice review: gate breaches, reruns and dispositions |
  | 10.2 review follow-up | `da8b43d` | Answer 404 when a student stops an ended link of a tutor whose account closed |
  | 10.2 review follow-up | `d1dda06` | Make Today's Show more wait for a /home refetch and send feedback-seen once per shown review |
  | 10.2 review follow-up | `f347b78` | Record the rerun of the Today Show more fix's gate and the assignment request-failure flake |
  | 10.2 review follow-up | this commit | Record the sub-slice 10.2 slice review and close the sub-slice |

  `d1dda06` and `f347b78` are one change, `fix/today-show-more-after-refetch`, merged together after its rerun gate as two commits instead of one, a deviation the owner accepted on 2026-11-20 and recorded as a known limitation of sub-slice 10.3; `f347b78` adds only two known-limitations rows: that gate's rerun row and the flaky-test row of `e2e/assignment.spec.ts:594`.
- **Tier:** slice, under [ADR-0011](../../docs/decisions/0011-tiered-verification.md), with each sub-slice of slice 10 counting as a roadmap slice. Most changes met the security trigger: authorization scope and relationship scope (B1, B2, B3, F1), deletion and purge (B4), the threat model (S2), and verification policy and agent configuration (`38eb0c9`, `8f112a0`, `65a9289`'s lint guard, `99120a4`).
- **Deferred:** performance measurements wait for 10.5 H1, so reviewers applied the performance profile by reading code. The laptop could be on battery and shared with another project, so reviewers ran no end-to-end suite, no whole backend run, no container and no merge gate; the passing merge gates of each change ran them.

## Reviewers and coverage

Five layer reviewers ran in fresh contexts on 2026-11-20 against `50b9338`, read-only. Security was split into two families of trust boundaries, because the sub-slice touched more than one. No reviewer subdivided its scope. A sixth, scoped reviewer covered the 10.1 closing record `c28c0e8` the same day.

| Reviewer | Profiles | Components covered | Why |
| --- | --- | --- | --- |
| Backend | Correctness, performance (code-level) | `GET /home` and its services for both roles, the access counts, `PATCH /tutor/settings` (B2), `open_assignments`, the seen marker and migration 00043 (B3), the read boundary at `account_closed_until` in every homework read, mint, serving, notification subject and `GET /tutors` (B4), the T-21 relationship lock and its lock order (`65a9289`), and their integration tests | Only `65a9289`, B1, B2, B3 and B4 change `backend/` |
| Frontend | Correctness, browser (dark theme, Firefox, Safari, phones, by reading), performance (by reading) | Today and Home on `GET /home`, Dismiss, the New marker and its `feedback-seen` call, the Students column, the coalesced live refetch, Today's join requests and their Show more, the fake transport's `/home` and its shared contract, F2's hint, `50b9338`'s focus moves, the test fix `11b1019`, and the end-to-end diffs scanned for forbidden interception, clock, sleep and mock forms | Every frontend file the range changes |
| Security: authorization scope, visibility and purge | Full security | The `/home` scope and unconfirmed-caller parts, role key sets and the student schema conventions, `open_assignments`' double tutor predicate, the dismissal's session re-check and limit, the seen marker's order of locks and readability, the B4 predicate in Go and SQL, the T-21 lock, the frontend never granting, logging of the new events | T-5, T-6, T-9, T-14, T-18, T-20, T-21, T-27, T-29, T-30, T-34, T-44 |
| Security: agent configuration and verification policy | Full security | `8f112a0`'s rerun and after-fix wording in every copy, the idle-machine rule, ADR-0009 item 13; the gate lines and self-test of `38eb0c9` and `99120a4`; `65a9289`'s lint guard; proposal 0005 and its pilot record; the `rerun` and `after-fix` rows; the start lines of the attempt log against the merged trees | T-19, T-25, T-36, T-38, T-39, T-41 |
| Scoped: the 10.1 closing record | Standard checklist, security for verification policy | `c28c0e8`: the 10.1 record against `git log` and `merge-gate.log`, its ADR-0018, threat-model, known-limitations and roadmap edits against the later state of `main` | The commit is in 10.2's list but is the base of the layer reviewers' diff; T-18, T-19, T-39, T-41 |
| Development tooling and agent configuration | Correctness, security for verification policy | `tools/merge-gate.sh` and `tools/check-merge-gate.sh` as `38eb0c9` and `99120a4` change them; `8f112a0`'s policy text; every 10.2 gate in `merge-gate.log` and its command logs; the lane A and proposal 0005 records; the change-check table; every `rerun` and `after-fix` row and every tooling and budget row of the known limitations | Development tooling and verification policy changed, and the sub-slice rule asks this reviewer to re-verify every `rerun` and `after-fix` row |

## Integration boundaries and who reviewed them

| Boundary | Reviewer |
| --- | --- |
| `GET /home`, the generated typed client and the pages that read it | Backend; frontend |
| Fake transport, its shared contract and the real backend | Frontend; authorization security |
| The assignment page's `feedback-seen` call and the seen marker's locks and limit | Backend; frontend; authorization security |
| The `account_closed_until` cutoff across homework reads, attachment authorizations and notification subjects | Backend; authorization security |
| Live frames and the coalesced `/home` refetch | Frontend; authorization security (T-29) |
| Merge gate, its attempt log and the check-budget, idle-machine and rerun rules | Tooling; agent-configuration security |

## Results

| Reviewer | Blocker | Major | Minor |
| --- | --- | --- | --- |
| Backend | 0 | 0 | 3 |
| Frontend | 0 | 1 | 3 |
| Security: authorization scope, visibility and purge | 0 | 0 | 0 (2 observations) |
| Security: agent configuration and verification policy | 0 | 1 | 2 |
| Development tooling and agent configuration | 0 | 2 | 4 |
| Scoped: the 10.1 closing record | 0 | 0 | 3 |

**Majors:**

- **Frontend: Today's Show more did not wait for an in-flight `/home` refetch.** `HomeJoinRequestList.loadMore` (`405360f`) fetched from the current cursor at once, so a `/home` answer landing during that fetch dropped the appended rows and moved focus back, against the shared rule "Show more during a refetch" of `pages.md`. `11b1019` had changed the end-to-end test to wait for the catch-up refetch rather than the code, and a component test asserted the deviation.
- **Agent-configuration security and tooling (the same finding): `11b1019` merged from a gate that started at a one-minute load of 19.30**, at or above the 6 from which the idle-machine rule of ADR-0018 does not use a gate to merge, in 952.8 s, with no breach row and no recorded answer to its 600 s question.
- **Tooling: `50b9338`'s 1007.9 s gate showed the signature of concurrent load.** It started at 4.20, but its lanes' phase ended at a one-minute load of 31.47 and its end-to-end phase took 723.3 s against about 410 s; whether something else ran, and its 600 s answer, were not recorded. That gate carried the median the review computed.

**Minor findings and observations:**

- **Backend:** `POST /tutors/{relationshipId}/end` answered `500` for an ended link of a deleted tutor from `account_closed_until` until the purge, because the re-read of the link excludes that tutor and `repositories.ErrNotFound` was not mapped; the assignment page sent `feedback-seen` on every `version` change while a review was shown, so each autosave cost two more write transactions and a rate slot; lesson detail and lesson-material and template downloads also stay reachable after `account_closed_until`, which known-limitations row 32 did not name. Observations for H1: the student's `/home` reads a 63-day calendar and computes assignment counts twice, and `next_seven_days` includes a lesson in progress while `api.md` said "starting from now".
- **Frontend:** the Retry of a failed Show more page on Today moved focus to the page heading, not to the first new row, with no test; Home's next lesson disappeared at its end time instead of moving to the following lesson, because only one candidate arrives and nothing refetched at `ends_at`; the end-to-end burst test in `today.spec.ts` could end its poll before a third read.
- **Authorization security (observations):** the `PATCH /tutor/settings` row of `api.md` said the rate slot is kept when the write is refused later, while it shares the write's transaction; known-limitations row 31 still read as open although B2 had fixed it.
- **Agent-configuration security:** the attempt log is now policy evidence for the idle-machine rule and the median, but any process of the same user can rewrite it, and T-41 did not say so; the other-containers count can be understated by a foreign container labelled with the `ovc` Compose project or another OVC checkout's stack.
- **Scoped, the 10.1 closing record:** the 10.1 record calls other projects' work during the picomatch and F-b gates of 2026-11-17 unverified but also reports a view that such work explained the afternoon gates' growth; the rule that a gate change's criterion never asks its own gate to show that change's behaviour lived only in the 10.1 record; the roadmap paraphrased ADR-0018's "earliest slice review that is not yet resolved" as "the next slice review".
- **Tooling:** the proposal 0005 record projected the gate with an overhead constant taken from other gates (8.6 s instead of the 1.7 to 1.9 s of the same gates), overstating the margin of its upper-bound cells; the change-check row for the Compose file omitted `tools/check-merge-gate.sh`, which now reads `docker-compose.yml`; the quiet-machine step did not define an active container, while every full-run gate logged three other containers; after-fix and measurement runs in fresh worktrees repeatedly ran no test, because those worktrees hold no environment files or no `bin/` tools.

## Findings and their resolution

Each follow-up is a standard-tier change under the after-fix rule that names the findings it resolves, and its readiness reviewers confirmed them closed. The follow-ups are in 10.3's list, whose slice review reviews them again.

| Resolution | Findings | Confirmed closed by | Merged |
| --- | --- | --- | --- |
| Record the sub-slice 10.2 slice review: gate breaches, reruns and dispositions | Both breach majors: rows for `11b1019` and `50b9338` in the known limitations and their ratification by the owner in [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#gate-reruns-and-ratifications-of-sub-slice-102) and in the threat model's [section on the sub-slice 10.2 gate breaches](../../docs/threat-model.md#sub-slice-102-gate-breaches-and-ratification), with every passing gate of 600 s or more and its answer; the attempt log's integrity and the understated container count as T-41 residuals; the rule that evidence and measurement runs go through the main checkout's `tools/dev-env.sh` with `bin/` present and count only when their output names the selected test, in `AGENTS.md`, ovc-verify and the development guide; an active container defined in the quiet-machine step; `tools/check-merge-gate.sh` in the Compose file's change checks; the proposal 0005 projection recomputed from the same gates, its conclusion kept; row 31 resolved as fixed, row 32 extended to lesson detail and material downloads, and the review's other dispositions | That change's standard and security-profile readiness reviewers | `4f291ef` |
| Answer 404 when a student stops an ended link of a tutor whose account closed | The `500` of `POST /tutors/{relationshipId}/end`: it now answers `not_found` with the body of a random identifier, and `TestResultsReadableUntilAccountClosedUntil` stops the ended link a minute before, at the instant and after the purge; `api.md` and the threat model state the boundary; `api.md` also corrects the `PATCH /tutor/settings` slot wording and the `next_seven_days` wording | That change's standard and security-profile readiness reviewers | `da8b43d` |
| Make Today's Show more wait for a /home refetch and send feedback-seen once per shown review | The frontend major: Show more waits for the in-flight `/home` read, continues from the refreshed cursor and focuses the first new row, and loads nothing when that read fails, as the owner decided; the Retry of a failed page focuses the first new row; Home keeps the next lesson past its end and reads `/home` again at its `ends_at`; the seen mark is keyed on the submission's status and the shown review's content instead of `version`, so autosaves no longer resend it | That change's standard readiness reviewer and a scoped re-review of its fixes | `d1dda06`, with its rerun row `f347b78` |

The remaining findings have these resolutions:

- **The end-to-end burst test's early poll:** accepted by the owner on 2026-11-20 as a known limitation of sub-slice 10.3, the follow-up's change; the component test in `app/liveRefetch.test.ts` remains the deterministic proof of F1's criterion.
- **The 10.1 closing record's minor findings:** this commit adds the rule on the criteria of gate changes to [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#decision) and replaces the roadmap's paraphrase with ADR-0018's wording, both by the owner's decision of 2026-11-20 and pending owner review; the other projects' work during the two gates of 2026-11-17 is recorded as a known limitation and accepted by the owner as unverified, not as a breach.
- **The `/home` read costs observed by the backend reviewer:** carried into 10.5 H1 with row 30, which measures `/home` under a burst of frames.

## Known-limitations entries and their resolution

Every row of the [slice 10 known limitations](slice-10-known-limitations.md) in sub-slice 10.2, and every row carried into 10.2, was given to the reviewers of its layer as an item to re-verify. The resolutions below are written into the file by `4f291ef`, except the row on the gates of 2026-11-17, which this commit adds.

| Entry | Re-verified by | Resolution |
| --- | --- | --- |
| C0a: the self-test accepts `unknown` power fields (10.1, carried) | Tooling; agent-configuration security | Fixed by `99120a4` for the passing and the failing self-test gate that read the power log |
| F-a: `IssuedNotice` and the removed-row fallback on the next frame (10.1, carried) | Frontend | Fixed by `50b9338` |
| F-a: the fake submissions routes' missing refusals (10.1, carried) | Frontend; authorization security | Partly fixed by F1; accepted by the owner under the row's rule that each missing refusal is added by the first change whose page test needs it |
| F-b: the fake `deadline_before` filter outside the contract (10.1, carried) | Frontend | Fixed by F1 |
| The 10.1 median of 571.0 s (10.1) | Tooling | Accepted by the owner on 2026-11-20 as superseded by the consolidation decision below |
| `38eb0c9`: the container probe's bound and untested branches | Tooling; agent-configuration security | Accepted by the owner on 2026-11-20 as a diagnostic that never decides a result, with the understated count as a T-41 residual |
| `8f112a0`'s gate: two browser diagnostics failures under load | Tooling | Carried into 10.5 H3, confirmed by the owner on 2026-11-20 |
| `65a9289`: `TestGroupRemovalSerialisesWithASubmit` counts the second wait | Backend; authorization security | Accepted by the owner on 2026-11-20 |
| S2: started beyond the concurrency limit | Tooling | Accepted by the owner on 2026-11-18 |
| S2: placement of "Dismiss" and "Open assignments" | Frontend | Fixed by F1 (global chunk, 180.3 of 181.0 KB); the 0.7 KB left is a risk for 10.3's phone texts |
| B1: the caps of 100 and exact boundaries untested | Backend | Accepted by the owner on 2026-11-20 |
| B1: separate read transactions in `/home` | Backend; authorization security | Carried into 10.5 H1 by the owner on 2026-11-20 |
| B1: the dismissed checklist case untested | Backend; authorization security | Fixed by B2 |
| B4: a deleted tutor's names, lesson detail and material downloads until the purge | Backend; authorization security | Extended by this review; accepted by the owner on 2026-11-20 until the purge, which removes the lessons, materials and chat |
| B4: recordings not tied to `account_closed_until` | Backend; authorization security | Carried to the change that registers the recordings routes |
| Lane A target missed (`f1855cf`) | Tooling; agent-configuration security | Accepted by the owner on 2026-11-18 |
| B3: `lockStudent` reads the instant before the lock wait | Backend; authorization security | Accepted by the owner on 2026-11-19 as a residual risk |
| B3: no test produces `submissions.feedback_seen.failed` | Backend; authorization security | Accepted by the owner on 2026-11-20 |
| F1: fake `GET /home` fidelity | Frontend; authorization security | Accepted by the owner on 2026-11-20; the backend's home tests are the authority |
| F1: the tutor's nested home items not compared by the contract | Backend; frontend; authorization security | Accepted by the owner on 2026-11-20 |
| F1: `LoadFailed` loses focus when a live refetch resets it | Frontend | Carried into 10.3's accessibility pass, A1, by the owner on 2026-11-20 |
| Proposal 0005 pilot stopped at its measurement (`608030a`) | Tooling; agent-configuration security | Accepted by the owner on 2026-11-20 with the corrected projection; proposal 0005 stays deferred |
| `11b1019` merged in breach of the idle-machine rule (new row) | Tooling; agent-configuration security | Ratified by the owner on 2026-11-20 as a one-time exception that sets no precedent |
| `50b9338` merged beside another project's job (new row) | Tooling | Ratified by the owner on 2026-11-20 as a one-time exception that sets no precedent |
| Other projects' work during the picomatch and F-b gates of 2026-11-17 (new row) | Scoped reviewer of `c28c0e8` | Accepted by the owner on 2026-11-20 as unverified, not as a breach |
| The reruns of `8f112a0` (`7c3b858`), F2 (`61e13d4`), F1 (`f231519`) and `99120a4` (`779b049`) | Tooling; agent-configuration security | Consistent with the rerun definition: base and tree match `merge-gate.log`, each row was in the change's own commit before the next gate, and the owner approved each rerun before it ran; F2's and F1's own diffs changed without after-fix evidence, so the owner's approval, not the fix, admitted those gates. No `after-fix` row exists in 10.2 |

The rows of the follow-ups' change `fix/today-show-more-after-refetch`, including its gate's rerun row, are in sub-slice 10.3 under the file's header rule, for the 10.3 slice review.

## Gate record

From `merge-gate.log`, except where noted. The gate of `38eb0c9` ran a copy of `tools/merge-gate.sh` from before that change, so its log line has no `wall=` and its time comes from its gate report.

| Gate | Base | Tested tree | Load at start | Wall time | Phases pre / lanes / end-to-end | Result | Power |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Gate log visibility `38eb0c9` | `2ad5c2e` | `f5ae2fc` | 3.40 3.08 2.41 | 692.2 s (gate report) | | passed | mains |
| Verification-policy follow-up, first | `38eb0c9` | `7c3b858` | 5.15 16.43 16.12 | 610.1 s | 0.3 / 174.6 / 433.7 | failed, end-to-end (2) | mains |
| Verification-policy follow-up `8f112a0` | `38eb0c9` | `f14bea2` | 5.41 3.52 2.63 | 768.3 s | 0.4 / 227.2 / 538.7 | passed | battery during the run |
| Review findings `65a9289` | `8f112a0` | `4d31c8a` | 2.25 5.86 10.36 | 601.2 s | 0.2 / 192.5 / 406.9 | passed | mains |
| S2 `528db96` | `65a9289` | `cf3bee4` | 3.42 5.88 8.26 | 628.0 s | 0.2 / 197.7 / 428.2 | passed | mains |
| B1 `cb73714` | `528db96` | `caafa59` | 1.67 5.91 6.50 | 552.5 s | 0.2 / 183.1 / 367.5 | passed | mains |
| B2 `bc400a9` | `cb73714` | `88bd9bc` | 3.24 2.89 2.39 | 614.9 s | 0.3 / 195.9 / 417.0 | passed | mains |
| B4 `4da7cba` | `bc400a9` | `49cee51` | 3.02 4.24 6.60 | 608.9 s | 0.2 / 194.6 / 412.3 | passed | mains |
| Lane A record `f1855cf` | `4da7cba` | `cdffdac` | 2.27 | Markdown-only, 13.4 s | | passed | mains |
| Proposal 0005 `1a4fe67` | `f1855cf` | `d20fbc6` | 4.39 | Markdown-only, 13.0 s | | passed | mains |
| F2, first | `1a4fe67` | `61e13d4` | 1.55 5.84 7.80 | 608.0 s | 0.3 / 262.2 / 343.8 | failed, end-to-end (10, stopped) | mains |
| B3 `bfff5c0` | `1a4fe67` | `4d5ea74` | 2.72 5.87 8.78 | 634.5 s | 0.2 / 216.6 / 416.0 | passed | mains |
| F2 `e11f01f` | `bfff5c0` | `f3885c1` | 2.86 5.97 9.91 | 552.3 s | 0.2 / 174.4 / 376.0 | passed | battery during the run |
| F1, first | `e11f01f` | `f231519` | 3.04 4.10 5.64 | 700.1 s | 0.3 / 219.7 / 478.4 | failed, end-to-end (9) | battery during the run |
| 10.1 record `c28c0e8` | `e11f01f` | `42f5e1e` | 1.86 | Markdown-only, 14.4 s | | passed | mains |
| F1 `405360f` | `c28c0e8` | `3a1a987` | 3.13 5.06 7.09 | 667.8 s | 0.3 / 240.4 / 425.5 | passed | mains |
| Power-log assertion, first | `405360f` | `779b049` | 4.18 5.92 6.34 | 657.6 s | 0.3 / 207.9 / 447.8 | failed, end-to-end (1) | mains |
| Proposal 0005 record `608030a` | `405360f` | `29f0ae5` | 21.87 21.44 21.14 | Markdown-only, 24.9 s | | passed | mains |
| Join-requests test fix `11b1019` | `608030a` | `bddb39b` | 19.30 22.48 21.78 | 952.8 s | 0.6 / 541.3 / 408.8 | passed, merged in breach | battery during the run |
| Power-log assertion `99120a4` | `11b1019` | `4a86e62` | 1.11 5.30 14.07 | 569.9 s | 0.2 / 181.7 / 386.3 | passed | battery during the run |
| Issued notice focus `50b9338` | `99120a4` | `8c30b91` | 4.20 5.64 9.49 | 1007.9 s | 0.3 / 281.2 / 723.3 | passed, merged in breach | mains |

Every full-run gate from `8f112a0`'s first gate on logged three other containers, four at F1's first gate; `38eb0c9`'s gate ran the older script, which logged no count. The idle-machine rule covers full-run gates only, so the Markdown-only gate of `608030a` at 21.87 broke no rule.

The follow-ups are in 10.3's list; their gates are recorded here because they decide the median this record reports, and the 10.3 slice review re-verifies them.

| Gate | Base | Tested tree | Load at start | Wall time | Phases pre / lanes / end-to-end | Result | Power |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Review records `4f291ef` | `50b9338` | `0b2cb5f` | 2.79 2.49 2.84 | 591.8 s | 0.2 / 190.8 / 399.1 | passed | mains |
| End link after account closed `da8b43d` | `4f291ef` | `b2d987f` | 2.19 5.91 8.37 | 583.0 s | 0.2 / 196.6 / 384.5 | passed | mains |
| Today Show more fix, first | `da8b43d` | `1bff94e` | 1.23 5.92 9.58 | 610.1 s | 0.2 / 196.4 / 411.8 | failed, end-to-end (1) | mains |
| Today Show more fix `d1dda06` and `f347b78` | `da8b43d` | `62dd6ca` | 3.32 5.94 10.24 | 674.8 s | 0.2 / 224.2 / 448.7 | passed | mains |

**Gates of 600 s or more and the owner's answers.** Each went to the owner as a multiple-choice question and was answered before its change merged, according to the coordinator's record of the owner's quiz answers; [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#gate-reruns-and-ratifications-of-sub-slice-102) lists them too. The answers to the first three are also in the [10.1 record](slice-10-1-slice-review.md#gate-record).

| Gate | Wall time | Date | Owner's answer | Before the merge |
| --- | --- | --- | --- | --- |
| Gate log visibility `38eb0c9` | 692.2 s | 2026-11-18 | Merge | before |
| Verification-policy follow-up `8f112a0` | 768.3 s | 2026-11-18 | Merge | before |
| Review findings `65a9289` | 601.2 s | 2026-11-18 | Merge | before |
| S2 `528db96` | 628.0 s | 2026-11-18 | Merge | before |
| B2 `bc400a9` | 614.9 s | 2026-11-19 | Merge | before |
| B4 `4da7cba` | 608.9 s | 2026-11-19 | Merge | before |
| B3 `bfff5c0` | 634.5 s | 2026-11-19 | Merge | before |
| F1 `405360f` | 667.8 s | 2026-11-19 | Merge | before |
| Join-requests test fix `11b1019` | 952.8 s | 2026-11-19 | Merge; the breach of the idle-machine rule ratified on 2026-11-20 | before |
| Issued notice focus `50b9338` | 1007.9 s | 2026-11-19 | Merge; the run beside another project's job ratified on 2026-11-20 | before |
| Today Show more fix (10.3's list) | 674.8 s | 2026-11-20 | Merge, after the owner-approved rerun | before |

**Failed gates.** 10.2's list has four failed gates, each followed by an owner-approved rerun whose row is re-verified in the table above: the verification-policy follow-up's `7c3b858`, F2's `61e13d4`, F1's `f231519` and the power-log assertion's `779b049`. The follow-up `fix/today-show-more-after-refetch` failed once on `1bff94e`; its rerun row is in sub-slice 10.3.

**Idle-machine rule.** Every full-run gate of 10.2's list started at a one-minute load below 6 except `11b1019`'s at 19.30. `50b9338`'s gate started at 4.20, and its five-minute load rose to 45.14; by the coordinator's account it ran beside an external job of another of the owner's projects. Both merges are ratified by the owner as one-time exceptions. The follow-ups' gates started between 1.23 and 3.32. Whether another project's work overlapped the other gates cannot be checked from the log, which counts containers but not other processes; the other containers stayed at three, except four at F1's first gate, and the quiet-machine step now defines when such a container counts as active.

**Battery.** When `99120a4` merged, two of the last three passing gates, `11b1019`'s and `99120a4`'s, had run partly on battery power, which the check-budget rule has the coordinator report to the owner; ADR-0018's section on sub-slice 10.2 records the condition, and the report itself rests on the coordinator's record.

## Consolidation outcome

At the review the last three passing full-run gates on mains power for the whole run were B3 at 634.5 s, F1 at 667.8 s and `50b9338` at 1007.9 s, a median of 667.8 s, above the 540 s of ADR-0018 and above the 600 s of NFR-6.2; 10 of 13 passing full-run gates of 10.2's list took 600 s or more. At this record the last three are the follow-ups' gates, `4f291ef` at 591.8 s, `da8b43d` at 583.0 s and the Today Show more fix at 674.8 s, a median of 591.8 s, still above 540 s.

The records support three conclusions:

- The lane A levers that keep one database per test are exhausted (`f1855cf`), and shared test databases cannot bring the gate to 540 s for any reachable sharing ([proposal 0005 measurement](../analysis/gate-speed-shared-database-dose-2026-11-19.md), its projection corrected by `4f291ef`).
- The end-to-end phase is about two thirds of a gate: the median end-to-end phase of the twelve passing full-run 10.2 gates with phase data was 414.2 s, against 196.8 s for the lanes.
- Outside load added about 340 to 400 s to the gates of `11b1019` and `50b9338`, so enforcing the idle-machine rule matters as much as any test change.

On 2026-11-20 the owner inserted a measured end-to-end consolidation as its own change before sub-slice 10.3's feature work. It is the next change after this record: it uses the C0b method of alternating pairs, with the guidance the 10.1 record kept (alternate which tree runs first, measure the committed tree, project from in-gate times), and every gate keeps to the idle-machine rule. The 10.3 plan revision, R3, is to give it its row in 10.3's list before G0.

## Threats examined

| Threat | Reviewers | Result |
| --- | --- | --- |
| T-5 A tutor reaches another tutor's work | Authorization security | Examined, no finding: `/home` keyed on the caller, `homeFilter`, the double tutor predicate of `open_assignments` |
| T-6 A student reaches another student's work | Authorization security | Examined, no finding: `SubmissionParties` scoped by student; the classmate case of `TestHomeScope` |
| T-9, T-30 Answer keys, points and scores | Authorization security | Examined, no finding: student-prefixed schemas under the conventions test; `feedback_version` moves only on visible publications |
| T-14 Script injection | Authorization security; frontend | Not affected in the backend; titles, excerpts and names render as text, with markup-as-text tests in `home.spec.ts` and `today.spec.ts` |
| T-18 Personal data in logs | Authorization security | Examined, no finding: access counts only, the route template only, identifiers only in the new events |
| T-19 Secret disclosure | Agent-configuration security | Examined, not affected: the gate lines print no settings; the abandoned evidence runs and the pilot read no environment files |
| T-20 Resource exhaustion | Authorization security; backend | Examined: bounded lists and capped counts, the 60 and 600 per hour limits; the seen marker's repeated calls fixed by `d1dda06`; `/home` measured in 10.5 H1 |
| T-21 A removed participant keeps access | Authorization security; backend | Examined: `65a9289`'s lock and its lock order; `/home` reads relationship state on every read |
| T-25 Lesson access and stale links | Agent-configuration security | Not affected |
| T-27 Link scope in notification rows | Authorization security | Examined only as B4 cites it: `inbox.go` drops a deleted tutor's names and links after the instant |
| T-29 Stale socket access | Authorization security | Not affected: no new frame kind or field |
| T-34 Data after deletion | Authorization security; backend | Examined: the cutoff in every homework read, mint and serving; the stop route's `500` fixed by `da8b43d`; the window until the purge for lessons, materials and chat accepted by the owner and recorded in the threat model by `4f291ef` |
| T-36 Test-control surface | Agent-configuration security | Not affected: proposal 0005 stays deferred; the lint guard for fakes widened by `65a9289` |
| T-38 Local infrastructure | Agent-configuration security | Not affected |
| T-39 Dependencies and tools | Agent-configuration security | Examined, not affected: `docker` in the shadowing residual |
| T-41 Agent configuration and verification policy | Agent-configuration security; tooling; scoped reviewer of `c28c0e8` | Examined: the copies of the rerun rule agree; the two breaches ratified and the attempt log's integrity and the container count recorded as residuals by `4f291ef`; the rule on the criteria of gate changes moved from the 10.1 record into ADR-0018, with its residual in T-41, by this commit |
| T-44 Unconfirmed accounts | Authorization security | Examined, no finding: the null tutor parts mirror the `AudienceTutor` routes; both writes re-check the session under their locks |

## Owner decisions

Each was answered by a multiple-choice question.

- **2026-11-18, the rerun of the verification-policy follow-up** after its failure on `7c3b858`: approved before it ran, as tree `f14bea2`.
- **2026-11-18, the gates of 692.2 s, 768.3 s, 601.2 s and 628.0 s:** merge each.
- **2026-11-19, the gates of B2, B4, B3, F1, `11b1019` and `50b9338`:** merge each.
- **2026-11-19, the reruns of F2, F1 and `99120a4`:** approved before each ran; `11b1019` fixed the flake first as its own change.
- **2026-11-20, the frontend major:** fix Today's Show more in code, not by amending `pages.md`; the follow-up `fix/today-show-more-after-refetch` also fixes the Retry focus, Home's next lesson and the seen marker's repeated calls.
- **2026-11-20, the two breaches:** record them and ratify each merge as a one-time exception that sets no precedent.
- **2026-11-20, the consolidation question:** insert a measured end-to-end consolidation as its own change before 10.3's feature work.
- **2026-11-20, the minor findings:** applied as the review proposed: the backend's stop route fixed by `da8b43d`; the records, residuals, rules and dispositions of `4f291ef`; the end-to-end burst test's early poll accepted.
- **2026-11-20, the Today Show more fix's gate:** rerun approved after its failure on `1bff94e`, and merge after the 674.8 s rerun.
- **2026-11-20, the 10.1 closing record `c28c0e8`:** reviewed by a scoped reviewer in this review; its findings resolved by the rule on the criteria of gate changes in ADR-0018, the roadmap wording and the acceptance of the two gates of 2026-11-17 as unverified.
- **2026-11-20, that change's two commits on `main`:** accepted as a deviation that sets no precedent, without rewriting history.

## Carried

- into 10.3: the measured end-to-end consolidation, first; A1 with `LoadFailed`'s focus; the 10.3 slice review of the follow-ups and their known-limitations rows; the 0.7 KB left in the entry chunk for the phone texts;
- into 10.5: H1 with `/home` under a burst of frames and the read costs the backend reviewer observed; H3 with the browser-diagnostics failures under load;
- the recordings boundary at `account_closed_until`, to the change that registers the recordings routes;
- the picomatch acceptance, which expires on 2026-12-01 and is removed or renewed by the owner before then.

## Resolution

The review was resolved on 2026-11-20. The frontend major is closed by `d1dda06`, confirmed by its readiness reviewer and scoped re-review. The two breach majors are closed by `4f291ef`: both merges are recorded and ratified by the owner, and every passing gate of 600 s or more of 10.2's list had the owner's answer before its merge, according to the coordinator's record of the owner's quiz answers. Every minor finding is fixed by `4f291ef`, `da8b43d`, `d1dda06` or this commit, accepted by the owner, or carried into a named sub-slice change, as the tables above and the [slice 10 known limitations](slice-10-known-limitations.md) list.
