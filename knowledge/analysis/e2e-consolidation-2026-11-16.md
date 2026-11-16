# End-to-end consolidation C0b: coverage record and measurements

Recorded on 2026-11-16 for change C0b of the [slice 10 plan](../plans/slice-10.md), decisions D8 to D10. The change removes seven end-to-end tests whose assertions Go tests now hold, merges two, advances the 1-second client save delays of 42 tests with Playwright's `page.clock`, and batches four sequential seeds. It drops no coverage. This record is the D9 coverage record and the D8 measurement. Line numbers refer to `main` at `503bb17`, the change's base.

## Owner decisions

The owner decided three things on 2026-11-16, two when approving the task plan and one after the measurement:

- **Implement and measure.** The plan projected a saving of about 8 % against D8's 20 % target. The owner chose to implement the options that keep coverage, run the baseline and three alternating pairs, and apply D10 with the measured pairs if the target is missed.
- **The page clock.** Advancing the client save delays with Playwright's `page.clock`, a fake clock, is an option beyond D8's list. The owner accepted it as a fake clock under the Testing rules of `AGENTS.md`. `docs/development.md` names it in its Clock line (pending owner review).
- **The missed target (D10).** After the paired runs below, the owner decided on 2026-11-16 to merge the saving reached (branch 327.3 s against 368.7 s on `main`, ratio 0.888, 8 to 11 % saved). From this change's merge the ADR-0018 median rule watches the gate. The slice 10 known limitations record the accepted miss.

## Where the time was

From the two latest passing full gates before the change (2162 runs, 395 s and 407 s of end-to-end wall time, 2138 s of summed test time: Chromium 527 s, Firefox 861 s, WebKit 750 s):

- **No Firefox slowdown left.** Since slice 9's spec-group projects, the Firefox/Chromium ratio is flat at 1.3–1.8 across the quarters of every group.
- **Constant waits.** `assignment.spec.ts` averaged 1.39 s per test in Chromium, against a suite median of about 0.47 s. The cause is the 1-second save delay of the answer, review draft, template and notes autosaves: 47 tests waited for 61 delays per engine, about 160 s summed.
- **Small levers elsewhere.**
  - The 22 loop call sites total 145 s summed.
  - Only pilot steps open no page.
  - Safe worker-scoped arrangements would save about 3 s summed.
  - Bulk seeding of the 199 groups of the group-limit test would save about 11 s summed.

## Coverage record

### Removed, held by Go tests

| End-to-end test (3 engines each) | What it asserted | Now held by |
|---|---|---|
| `lesson-room.spec.ts:744`, a restart ends the old room of a student removed from the group | After the tutor restarts, the removed student's old room shows `ended` and no longer shows the student's name. | **New** `TestMeetingRestartRevocation/a_member_removed_from_the_group_loses_the_old_room_and_gets_none_of_the_replacement` (`backend/internal/api/meeting_lifecycle_test.go`). After the member is removed and the room restarted: their old link is refused with `FailureNotFound`; it does not enter the replacement; they get no lesson frame; their join answers `404 not_found`; and the fake provider keeps no participant row, and so no display name, of theirs. The ended room's rendering stays in `meeting-room.spec.ts` ("a restart ends the room that is open"). The old tab ending after a restart from the page stays in `lesson-room.spec.ts` ("lesson room › a restart opens a fresh tab, ends the old room and returns focus to its button"). |
| `meeting-room.spec.ts:236`, after a restart the old link is refused and the new link opens in the same page | The old link reaches `refused`, and the new link enters as moderator. | `testRestartRevokes` (`meeting_lifecycle_test.go`): the old link gives `FailureNotFound`, the fresh link enters. `testRoomStaleGeneration` (`backend/internal/testcontrol/fake_room_test.go`): a stale capability is absent, and the replacement shows the current name. "Refused" rendering: `meeting-room.spec.ts` "a changed capability opens no room and stops". The new link opening in the same page after a restart: `lesson-room.spec.ts` "lesson room › a restart replaces the frame and returns focus to its button", whose new frame address shows the room entered. |
| `meeting-room.spec.ts:260`, ending the lesson through the API ends the room | `End` answers 200; after a tick the room shows `ended` and no participant name. | `testEndClosesAndCleans` (`meeting_lifecycle_test.go`): the lesson is ended, cleanup runs through the worker tick, no provider room remains and the link is refused. `TestMeetingRecoveryRuntime` (`backend/cmd/internal/startup/meeting_recovery_process_test.go`) runs the production cleanup lane in a built server process. The ended room's rendering stays in `meeting-room.spec.ts`. |
| `review.spec.ts:442`, a saved draft comment does not reach the student until it is published | The stored draft holds the comment; the student's submission body does not. | **Extended** `contractReview` in `TestAnswerKeyContract` (`backend/internal/api/review_contract_test.go`). At the draft state, the student body is checked against the exact key allowlist (`check.submission`); it holds `"review":null`; and it contains none of the draft's comments, feedback or override reason. The Comment field and the stored draft stay in `review.spec.ts` ("shows each answer beside its scoring and saves the draft…"). |
| `assignment.spec.ts:1040`, a points-only publication changes nothing the student sees, and a later draft stays hidden | After a points-only publication and a later draft with feedback, a comment and a draft-only corrected file, the student's page and body are unchanged and show no score. | **Extended** `TestPointsOnlyRevisionInvisible` (`backend/internal/api/reviews_test.go`). After the points-only publication, a draft is saved with feedback, a comment and a draft-only corrected file. The student body, with signed addresses removed, stays equal to the body before, and contains none of the draft's texts or the file's name. The copied-file order check is now scoped to the revision it copies from, because the draft also copies those files. The published review's rendering without points stays in `assignment.spec.ts` ("checked work shows the comment, the corrected file, the feedback and the revealed answers, and no points", and its no-points sentinel test). |
| `account-edge-cases.spec.ts:459`, email language follows the account | After the interface language is switched in the settings, the reset email's subject is in the new language. | **New** `TestTheResetEmailFollowsTheLanguageSavedInTheSettings` (`backend/internal/api/account_flows_test.go`): an English account saves `ru` through `PATCH /users/me`, and the reset email that follows is in Russian. The server reads no request language (no route reads `Accept-Language`), so no conflicting request language exists to test. The settings form's language switch stays in `language.spec.ts`. The forgot-password form stays in `signup.spec.ts`. |
| `assignment.spec.ts:638`, a student removed from the group still submits the group's work | A student removed from the group submits the group's work. | `TestRemovedMemberKeepsWork` (`backend/internal/api/submissions_test.go`), which T-6 already cites. The focused Submitted notice stays in `assignment.spec.ts` ("submitting shows the focused Submitted notice…"). `knowledge/design/web/pilot-scenarios.md` now names both. |

### Merged

`lesson.spec.ts:1299`, "a student without materials sees no Materials block", is merged into `lesson.spec.ts:848`, "a student sees Individual lesson, never the card name".

- Both seed a tutor and a student, schedule one lesson tomorrow, sign the student in and open the lesson page, under the same `test.use`.
- They differ only in `:848`'s card name, which none of `:1299`'s assertions reads.
- `:848` now also asserts `:1299`'s heading, "Разговорная практика", and the absent Materials block.
- The kept title does not name the Materials assertion.

### Save delays advanced with the page clock

Each listed test installs `page.clock` without a fixed time, through an explicit option of its spec's opener (`openAssignment`, `openReview`, `openBuilder`) or `installPageClock` before its first navigation. The page's time keeps flowing. After an edit, `frontend/e2e/save-delay.ts` advances the page clock by the 1000-millisecond save delay:

- for answers and review drafts, once the status shows "Saving…" ("Сохраняется…"), the state in which the save timer is armed, or "Saved" ("Сохранено"), when the save already landed while an earlier step ran, so the wait can never outlast a successful save that has passed (a save that ends in a conflict leaves no status, so the tests that expect a conflict wait right after their edit);
- for templates, at once, since the builder arms its timer in the edit's event handler.

A fast-forward that lands before the timer is armed, or after the save, changes nothing, and the test then waits for the save as before. No other test installs a clock.

The tests:

- `assignment.spec.ts`: :322, :338, :351, :365, :378, :398, :415, :426, :444, :455, :464, :553, :572, :591, :658, :683, :1020, and pilot-2 steps 7, 8–9, 10, 11, 12 and 13;
- `review.spec.ts`: :258, :277, :297, :317, :339, :415, :429, :460, :484;
- `builder.spec.ts`: :334, :417, :435, :520, :548, and pilot-2 step 5;
- `homework.spec.ts:444`.

What was left out:

- The student notes tests (`students.spec.ts` :444, :520, :557) keep real time. :557 is the Firefox flake change X1 owns.
- `assignment.spec.ts:310` keeps real time, because it asserts "Saving…" without holding the request, and the fast-forward would shorten that state into a race.
- `builder.spec.ts:403` keeps real time, because "Saving…" there is the held request. It also never asserts the delay.
- `builder.spec.ts:810` keeps real time, because the moment its upload arms the save is not visible.

No assertion changed. The product's delay stays 1000 milliseconds in `features/assignment/autosave.ts`, `features/review/draftSave.ts` and `features/builder/autosave.ts`. New component tests pin it at exactly 1000 milliseconds, with no save at 999 and one at 1000:

- `autosave.test.ts` "the delay before a save is exactly 1000 milliseconds";
- `draftSave.test.ts` "the delay before a draft save is exactly 1000 milliseconds";
- `builder/autosave.test.ts` "the delay before a save is exactly 1000 milliseconds".

Before this change, no test pinned the literal value.

### Seeds sent in batches

`frontend/e2e/tutoring.ts` gains `inSeedBatches`, which runs at most 5 seeding calls at once. It is used by:

| Seed | Limit checked | Order the test asserts |
|---|---|---|
| `seedBlockedStudents` declines (`teaching-settings.spec.ts`, Show more) | No rate or concurrency limit on decisions in the specifications; the writes serialise on the tutor's profile lock | Count and focus only; the blocked list orders by `created_at` |
| `seedInvitedCards` (`students.spec.ts`) | No concurrency limit on `POST /students` | Names compared as a set |
| 31 templates (`library.spec.ts`, Show more) and 30 templates (`builder.spec.ts`, the picker's Show more) | 600 library writes per tutor per hour, not concurrent | Lists order by `normalised_title` |
| 20 issues and 21 submissions (`review.spec.ts`, Show more) | Issuing and submitting take their locks in the specified order; homework emails per student per day are counted, not concurrent | Counts only |

Uploads stay sequential: `knowledge/specs/files.md` allows 4 concurrent uploads per user and 32 per API instance, so the 31 materials of `lesson.spec.ts` keep their sequential seed.

### Proof that the new and extended Go tests catch a regression

For each case, the product was broken temporarily, the test was run, and the file was restored by copy. Afterwards, `git diff -- backend/internal/services backend/internal/dbgen backend/cmd` was empty, and every test passed on the restored product.

| Temporary break | Test | Result |
|---|---|---|
| `internal/services/meetings/lifecycle.go`: the group membership check of a student's join accepts a removed member | `TestMeetingRestartRevocation/a_member_removed…` | failed: "a join of the removed member after the restart answered 200…, expected 404 not_found" |
| `internal/dbgen`: the student's submission read takes the latest revision of any state, and the published revision read drops its state filter | `TestPointsOnlyRevisionInvisible`, `TestAnswerKeyContract` | both failed: "a points-only publication or a later draft changed the student's view", and "a draft review reached the student" for each policy |
| `internal/services/notifications/account_emails.go`: account emails always use English | `TestTheResetEmailFollowsTheLanguageSavedInTheSettings` | failed: "subjects [Reset your OVC password]" |
| `cmd/internal/startup/compose.go`: the provider-call lane gets no meeting recovery function | `TestMeetingRecoveryRuntime` | failed: "runtime recovery result=pending cycle=1 status=created" |

### Inventory

`pnpm exec playwright test --list` lists 2162 runs in 37 files on `main` and 2138 on the branch. With the projects normalised (`firefox-1` to `firefox-4` as `firefox`, and the same for WebKit) and the line numbers dropped, the difference is exactly these eight titles, each once per engine, and nothing added:

- `account-edge-cases.spec.ts › account edge cases › email language follows the account`
- `assignment.spec.ts › answering an assignment › a student removed from the group still submits the group’s work`
- `assignment.spec.ts › results of checked and returned work › a points-only publication changes nothing the student sees, and a later draft stays hidden`
- `lesson-room.spec.ts › ending the lesson and restarting the room › lesson room › a restart ends the old room of a student removed from the group`
- `lesson.spec.ts › lesson materials for a student in Russian › lesson › a student without materials sees no Materials block`
- `meeting-room.spec.ts › the fake meeting room › after a restart the old link is refused and the new link opens in the same page`
- `meeting-room.spec.ts › the fake meeting room › ending the lesson through the API ends the room`
- `review.spec.ts › the review screen › a saved draft comment does not reach the student until it is published`

No `pilot-` title differs.

### Citations

- **The threat model.** No removed or merged title is cited in `docs/threat-model.md`. The files the threat model cites (`home.spec.ts`, `today.spec.ts`, `lesson-room.spec.ts`, `api-contracts.spec.ts`, `language.spec.ts` and the others) keep their tests and engines. T-41 gains this change's line.
- **The specifications and design.** `knowledge/specs` cites none of these titles. `knowledge/design/web/pilot-scenarios.md` cited `assignment.spec.ts` for member removal with work still submittable, and now names `TestRemovedMemberKeepsWork` beside it.
- **Review records.** `knowledge/reviews/slice-7-fake-room.md` cited the two removed `meeting-room.spec.ts` titles and gains a dated note with their holders.

## Divergences from the approved plan

- **The email language test.** `account-edge-cases.spec.ts:459` is held by a new test, `TestTheResetEmailFollowsTheLanguageSavedInTheSettings`, rather than by an extension of `TestAccountEmailsFollowTheLanguageAndCarryNoUserText`. The existing test sets the language at sign-up, and three phases did not allow a settings change to be added to it. The new test saves `ru` on an English account; the removed test saved `en` on a Russian one. The property is the same: the email follows the language saved in the settings.
- **Tests left in real time.** `assignment.spec.ts:310`, `builder.spec.ts:403` and `builder.spec.ts:810` keep real time, for the reasons given above. The plan had `builder.spec.ts:403` fast-forward after its release.
- **Batched seeds.** The plan named four seed sites after `lesson.spec.ts:1068` left O4. Five functions batch:
  - `seedBlockedStudents`;
  - `seedInvitedCards`;
  - the library templates;
  - the builder templates;
  - the review issues and submissions.

## Measurements

Method: `tools/dev-env.sh pnpm e2e --trace=off` under `caffeinate -dis`, on `main` at `503bb17` (a detached worktree) and on the branch (the same base with this change uncommitted), with `test-results` deleted after each run. Each run started on AC power (`pmset -g batt`) only once the one-minute load average was below 6, with no merge gate, other end-to-end run or full backend run active on the machine. Wall time is from the start of `pnpm e2e` to its exit, including the global setup.

### Baseline (`main`, three runs)

| Run | Start (UTC) | Load (1, 5, 15 min) | Wall | Result |
|---|---|---|---|---|
| 1 | 16:55:53 | 2.13, 11.88, 13.10 | 370 s | 2162 passed |
| 2 | 17:04:03 | 5.34, 15.60, 15.52 | 350 s | 2162 passed |
| 3 | 17:11:08 | 5.59, 14.43, 15.53 | 347 s | 2162 passed |
| Mean | | | 355.7 s | |

### Alternating pairs

| Pair | `main` start, load | `main` | Branch start, load | Branch |
|---|---|---|---|---|
| 1 | 17:29:34, 5.11 | 365 s, 2162 passed | 17:37:24, 5.32 | 329 s, 2138 passed |
| 2 | 17:44:39, 5.79 | 354 s, 2162 passed | 17:52:18, 5.20 | 328 s, 2138 passed |
| 3 | 17:59:47, 5.19 | 387 s, 1 failed, 2161 passed | 18:07:59, 5.19 | 325 s, 2138 passed |
| Mean | | 368.7 s | | 327.3 s |

- **The target is missed.** The branch mean is 0.888 of `main`'s pair mean, against D8's 0.80. Leaving out pair 3, whose `main` run failed, it is 0.914 (328.5 s against 359.5 s). Against the three-run baseline it is 0.920. The change saves about 28 to 41 s of end-to-end wall time, 8 to 11 %, as the plan projected (about 30 s).
- **The failure.** The failure in pair 3's `main` run is `lesson-room.spec.ts:473` ("signed out, the lesson address asks to sign in and shows nothing private") in Firefox, an unexpected console error. This is the flake the slice 7 known limitations record for WebKit, seen here in Firefox. It occurred on `main`, whose tree this change does not alter there. The plan's rule (AC2) replaces a pair only for that test in WebKit. Because the target is missed in every reading, no further pair was run and D10 applied; the owner's decision is recorded above.
- **Every branch run passed** all 2138 tests in three engines, including every test that installs the page clock.

### Lane A and `internal/api`

One `go test -race -count=1 ./...` per side, under the same conditions:

| | `main` | Branch |
|---|---|---|
| Start (UTC), load | 17:22:52, 2.81 | 17:26:23, 5.34 |
| Lane A wall | 166 s | 146 s |
| `internal/api` | 150.3 s | 140.0 s |

- **The single runs are within the run-to-run spread.** The three new or extended Go cases add about 1 s of test time. The branch was not slower.
- **`main`'s run failed one package for a reason outside the comparison.** `internal/conventions`' `TestProjectLintRulesFire` needs `bin/golangci-lint`, which the detached `main` worktree had not built. Every other package passed. The branch, with `bin/` built, passed every package.

### Projected gate

With lane A at about 146–166 s and the end-to-end run at 327 s on the branch against 356–369 s on `main`, a quiet-machine gate is projected at about 475–495 s on the branch against 505–535 s on `main`. Gates have run 600 to 700 s, because the gate runs its end-to-end suite after lanes that load the machine.

## Threat model mapping

- **T-9 and T-30 (answer keys and scores):** examined. The draft hiding is held by the extended contract test with the key allowlist and the absent texts. The points-only rule and the later draft are held by the extended `TestPointsOnlyRevisionInvisible`. Both are proven against a broken read. The answer-key end-to-end tests that read the pages and their responses stay.
- **T-21 (meeting links):** examined. The removed member's revocation gets a new Go case, proven against a broken membership check.
- **T-6 (group membership):** examined. `TestRemovedMemberKeepsWork` is the exact holder.
- **T-20 (uploads and resource limits):** examined. No upload seed runs concurrently.
- **T-31 (email content):** examined. The language rule is held by the new Go test.
- **T-36 (the test control surface):** not affected. No route changes and no bulk seeding was added.
- **T-41 (verification policy):** examined. No test is skipped, no assertion is loosened, and no timeout, retry or worker setting changes. Every moved assertion is mapped above, and the page clock is recorded in `docs/development.md` by the owner's decision.
