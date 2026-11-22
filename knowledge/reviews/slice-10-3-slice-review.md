# Sub-slice 10.3 slice review

Status: review completed 2026-11-22; the owner decided its findings the same day, as listed under owner decisions; resolved 2026-11-23, after the review's two follow-up changes merged on 2026-11-22 (UTC), the frontend follow-up as `195d190` and the records follow-up as `24226bd`. This record's commit closes sub-slice 10.3, for the owner to confirm. Agent record.

## Scope

- **Base:** `fbe9456b7c9f66a32fbd52da0503aafd75b5db12` ("Record the sub-slice 10.2 slice review and close the sub-slice"), the commit that closed sub-slice 10.2. The 10.2 record put its follow-ups, which merged after its review started at `50b9338`, and its own closing commit in 10.3's list, so the reviewers' cumulative diff is `50b9338..0dcfe21`.
- **Reviewed head:** `0dcfe21cd00b839f1195eb508834f323f15ee95c` ("Audit phone and computer layouts for WCAG 2.2 AA and fix what the audit found"), the last change of 10.3's list and `main` when the review ran.
- **Sub-slice rule.** Each sub-slice's review covers its own list of changes in section 7 of the [slice 10 plan](../plans/slice-10.md#7-changes-in-order), with any inserted consolidation, its plan revision and the follow-ups of the previous sub-slice's review that merged after that review started (D1, D41, [`AGENTS.md`](../../AGENTS.md#development-verification)). `git rev-list 50b9338..0dcfe21` lists 19 commits; each is in exactly one row below, and none is in another sub-slice's list. Under D49 of the [slice 10 plan](../plans/slice-10.md) (revision 6, 2026-11-23), this record's closing commit, the review's two follow-ups and every other commit that merged after this review started and is in no list go to the list of the interface rework stage between 10.3 and 10.4, not to 10.4's list as D41 said before; on `main` these are `0f241f8` (the legal gap analysis), `15317dc` (the legal documents proposal), `195d190` and `24226bd`, while `352b4a5` (R4, the plan revision for 10.4) is in 10.4's list (D51). None of them is in 10.3's list.
- **10.3's list (reviewed by this review):**

  | Plan step | Commit | Change |
  | --- | --- | --- |
  | 10.2 review follow-up | `4f291ef` | Record the sub-slice 10.2 slice review: gate breaches, reruns and dispositions |
  | 10.2 review follow-up | `da8b43d` | Answer 404 when a student stops an ended link of a tutor whose account closed |
  | 10.2 review follow-up | `d1dda06` | Make Today's Show more wait for a /home refetch and send feedback-seen once per shown review |
  | 10.2 review follow-up | `f347b78` | Record the rerun of the Today Show more fix's gate and the assignment request-failure flake (the second commit of the change of `d1dda06`, accepted by the owner on 2026-11-20) |
  | Inserted consolidation | `02ac903` | Record why merge gate end-to-end runs exceed the suite's time and the lane A upload recovery flake |
  | 10.2 closing record | `fbe9456` | Record the sub-slice 10.2 slice review and close the sub-slice |
  | Inserted consolidation | `85311f6` | Run the merge gate's end-to-end entry without Playwright traces |
  | R3 | `ef9a98d` | Revise the slice 10 plan for sub-slice 10.3: phone layouts and accessibility |
  | X3 | `9c9c1ae` | Continue Today's join-request Show more after a coalesced follow-up home read and keep the member picker's search focus |
  | X4 | `94b8705` | Record unexpected request failures in the redacted end-to-end browser diagnostics |
  | G0 | `2cbacf4` | Design the phone frame and phone layouts of every page |
  | No list (joins 10.3) | `63cb645` | Raise the entry budget of the bundle check from 181 KB to 184 KB for the phone frame (the owner's decision under K21) |
  | G1 | `57319f5` | Build the phone frame below 768 px with 360 px layout sweeps |
  | G3 | `d449e83` | Build the student's pages and the public pages on phones below 768 px |
  | G4 | `d315626` | Build the phone layouts of the homework pages below 768 px |
  | G2 | `82a2ad9` | Lay out the tutor's pages for phones below 768 px |
  | G5 | `32429b4` | Build the phone layout of Messages below 768 px |
  | X5 | `ea97b12` | Wait for lazily loaded pages before end-to-end tests navigate away |
  | A1 | `0dcfe21` | Audit phone and computer layouts for WCAG 2.2 AA and fix what the audit found |

  `63cb645` is in no row of section 7; it merged before 10.3's review started, so it joins 10.3 (D1). D41's table names `fbe9456` a closing record and the 10.2 record names it a review follow-up; both name the same commit.
- **Tier:** slice, under [ADR-0011](../../docs/decisions/0011-tiered-verification.md), with each sub-slice of slice 10 counting as a roadmap slice. Most changes met the security trigger: authorization scope and deletion (`da8b43d`, `d1dda06`), verification policy and agent configuration (`4f291ef`, `fbe9456`, `85311f6`, `63cb645`, `ef9a98d`), test-output redaction (X4), and the phone frame, the token pages, the Lesson page's join, answer visibility, uploads and message rendering (G0 to G5).
- **Deferred:** performance measurements waited for H1, which revision 6 of the slice 10 plan moved from 10.5 to the measurements on the pilot deployment, so reviewers applied the performance profile by reading code and gate output. The machine was shared with another session's merge gates, so reviewers ran no end-to-end suite, no whole backend run, no container and no merge gate; the passing merge gates of each change ran them. Light checks ran behind a load and power gate.

## Reviewers and coverage

Four layer reviewers ran in fresh contexts on 2026-11-22 against `0dcfe21`, read-only, each given the scope above, the plan's 10.3 decisions, its profiles and its layer's rows of the [slice 10 known limitations](slice-10-known-limitations.md) as items to re-verify.

| Reviewer | Profiles | Components covered | Why |
| --- | --- | --- | --- |
| Frontend (subdivided into four fresh contexts: the phone frame; `d1dda06`, X3, X5 and `LoadFailed`; the student's, public and homework pages; the tutor's pages and Messages) | Correctness, browser (three engines, phones, both themes, by reading), performance (by reading), accessibility under D44 and the focus rule | G0's specifications against the implementation; G1 to G5 and A1 in `frontend/src`; the 360, 320 and 780 px sweeps and every other end-to-end test the list adds; `d1dda06`'s Show more and seen marker; X3's `FollowingQueryClient.followUpQueuedFor` and the member picker; X5's `pageSettled` and `pageLoaded`; the bundle budget of `63cb645`; the accessibility audit record | Every frontend change of the list |
| Backend | Correctness, performance (by reading) | `da8b43d`'s stop route at `account_closed_until`, its test, spec and threat-model text; every backend contract the frontend changes call (`schema.ts` is unchanged in the range) | `da8b43d` is the only change that reaches `backend/` |
| Security (subdivided into four families of trust boundaries) | Full security | (1) authorization scope and deletion: `da8b43d`, `d1dda06`, X3; (2) agent configuration and verification policy: `4f291ef`, `fbe9456`, `85311f6`, `63cb645`, `ef9a98d`; (3) test-output redaction: X4, X5, `85311f6`; (4) the frontend boundaries of G0 to G5 and A1, and D42's unchanged content security policy, headers and served files | Changes met the trigger in four families |
| Development tooling and agent configuration | Correctness, ADR-0018's rerun, after-fix and check-budget rules | `tools/merge-gate.sh` and `tools/check-merge-gate.sh` (`85311f6`), the bundle check (`63cb645`), the end-to-end diagnostics (X4) and synchronization (X5), the policy edits of `4f291ef` and `fbe9456`, the plan revision; every 10.3 gate in `merge-gate.log` and its command logs; every `rerun` and `after-fix` row of the known limitations; D43's measured total | The tooling and policy changes of the list and the gates of every change |

## Integration boundaries and who reviewed them

| Boundary | Reviewer |
| --- | --- |
| The stop route's `not_found` and the student's My tutors page | Backend; authorization security |
| The assignment page's `feedback-seen` call and its route | Backend; frontend; authorization security |
| The coalesced `/home` refetch and Today's Show more | Frontend; authorization security |
| Fake transport, its shared contract and the real backend (the fake's new `q` filter) | Backend; frontend |
| The phone frame's sections and the route access rules (`reachable`) | Frontend; frontend-boundaries security |
| The end-to-end diagnostics fixture, its redaction and the gate's kept output | Tooling; redaction security |
| Merge gate, its attempt log and the check-budget, quiet-machine and rerun rules | Tooling; agent-configuration security |

## Results

- **Frontend:** no blocker or major. Every page section of `knowledge/design/web/pages.md` (23) has a **Phone** rule; the 360 px sweeps open every page of every page group in all three engines with 60-character unbroken and markup names, and since A1 also measure 320 × 640 and 780 × 360; one frame is rendered at a time, so there are no duplicate landmarks; the duplicated cells of the homework tables are `display: none` on one side of 768 px each; the frame's 44 × 44 px targets and 20 px icons are asserted; sections are hidden only through `reachable`; the token pages changed only in classes; the Lesson join still opens a tab without an opener at 360 px; X5 changed no application code; `LoadFailed`'s focus rule is tested with `document.activeElement`. The native-close tests use the script's `dialog.close()` rather than `Esc`, a deviation from D44's wording that the owner approved and the audit records; the More sheet's test landed in A1, not G1. Thirteen minor findings.
- **Backend:** no blocker or major. `da8b43d` maps the closed tutor's missing link to the structured `not_found` instead of a `500`, with a body equal to a random identifier's, a test that fails on the old code, and the spec and threat-model text; it also removes a false `tutors.end.failed` error event. One minor finding (the fake's `q` contract).
- **Security:** no blocker or major. The three copies of the rerun and after-fix rule agree; nothing under `.claude/` or `tools/` was loosened; `63cb645` was the approved task under K21, not a weakening; `85311f6` keeps the redacted diagnostics and the tree binding; X4's records are `{method, path, failure}` with the path redacted and its query dropped, fixed failure kinds and a cap of 50; no G change added raw HTML, `window.open` or a new target, or changed the content security policy, headers, `index.html`, the Vite configuration or served files; the G0 mockups use invented data only. Six minor findings.
- **Tooling:** two majors, both about gate records rather than code, and six minor findings. The self-test expects the gate's exact `pnpm e2e --max-failures=10 --trace=off`, and the first gate after `85311f6` (of `9c9c1ae`) ran it; the change-check table lists every changed tool path; X5 adds no expected-error list, retry or timeout; the plan's section 8 greps print nothing over `50b9338..0dcfe21`.

## Findings and their resolution

Severity is the reviewer's. "Follow-up F" is the frontend follow-up change, merged as `195d190` ("Resolve the frontend findings of the sub-slice 10.3 slice review"), and "follow-up R" the records follow-up change, merged as `24226bd` ("Record the sub-slice 10.3 slice review's gate reruns, breach and rule changes"), both decided by the owner on 2026-11-22; each had a standard and a security-profile readiness reviewer and one scoped re-review, which confirmed the findings below closed.

| Layer | Severity | Finding | Resolution |
| --- | --- | --- | --- |
| Tooling | Major (unverified) | Every full gate from `85311f6` to `0dcfe21` logged `other containers: 6`; the quiet-machine rule of the development guide lets only the three named idle containers of the owner's other projects stay without being accounted for. Which containers each gate counted is not in the log. At the review, `docker ps` showed, besides the three, idle PostgreSQL 16.15 containers left by another project's Python test runs (testcontainers, no Compose project), three of them created on 2026-11-20 at 17:54, 18:31 and 21:30 UTC. | A breach: the coordinator's merge reports gave only the count and neither named the containers nor accounted for the rise from 3 to 6. Follow-up R records it in the known limitations, ADR-0018 and the threat model; the owner ratified the twelve merges on 2026-11-22 as a one-time exception that sets no precedent, stops the leftover containers that are not OVC's, and from the next gate on the coordinator names every counted container in the merge report. |
| Tooling | Major | The median of the last three passing full gates, 540.0, 661.9 and 589.2 s, is 589.2 s, above ADR-0018's 540 s, and three passing gates took 600 s or more: the Today Show more fix at 674.8 s (the owner decided its merge on 2026-11-20, recorded in the 10.2 record), `85311f6` at 608.1 s and X5 at 661.9 s. | Consolidation: the owner decided on 2026-11-22 not to consolidate now (see consolidation outcome). Each of the three gates had the owner's answer, merge, by a multiple-choice question before its merge: 2026-11-20 09:58 UTC, 2026-11-21 08:18 UTC and 2026-11-22 11:25 UTC (the coordinator's record of the owner's quiz answers). |
| Tooling | Minor | The rerun row of A1 (2026-11-22) names only the "Nothing found" wait and the one-snapshot geometry read, but `git diff acc60f3 f326df1` also carries the fix of the `lesson-form-layout.spec.ts:220` flake (`openExistingLesson`'s heading and `pageLoaded` wait) and its row; it gives 418.8 s where the log says 418.9 s. | Follow-up R corrects the row; the owner ratified the extra test-only diff on 2026-11-22 as a one-time exception. |
| Tooling | Minor | The resolutions of the rows of the `reclaim.spec.ts:144`, `lesson-room.spec.ts:473` and `desktop-layout.spec.ts:382` flakes run the old carry into the new "fixed by X5" text; the `desktop-layout.spec.ts:382` row still names `chore/load-failure-diagnosis`, which D48 replaced with `fix/page-load-failed`, and says 60 runs per engine where the page-load failure record ran 20 per engine. | Follow-up R. |
| Tooling | Minor | The figures of D43 and the targeted runs of the A1 flake rows existed only in conversations and in gate logs kept for seven days. | Fixed by this record: D43's table and the gate table below. |
| Tooling | Minor | `unexpectedFailures` in `frontend/e2e/request-failures.ts` is now called only by its own test; the fixture calls `isUnexpectedFailure`. | Accepted by the owner on 2026-11-22. |
| Tooling | Observation (recurring) | An owner-approved rerun after the change's own fix, with no after-fix evidence, happened twice in 10.2 and again in A1. | The owner decided on 2026-11-22 on the stricter rule: a gate of a tree that differs from the failed one beyond its known-limitations row is an after-fix gate that needs ADR-0018's evidence, never a rerun. Follow-up R adds it to ADR-0018, `AGENTS.md`, ovc-verify and the development guide, pending owner review. |
| Security | Minor (T-41) | ADR-0018's Failures bullet lacks the 2026-11-20 rule that evidence and measurement runs go through the main checkout's `tools/dev-env.sh` and that a run that ran no test counts as not run; it is only in `AGENTS.md`, ovc-verify and the development guide. | Follow-up R adds it to ADR-0018. |
| Security | Minor (T-41) | The provenance of that rule differs: the development guide says "added by the sub-slice 10.2 slice review", the other copies "by the owner's decision after" it. | Follow-up R aligns the guide. |
| Security | Minor (T-41, coverage) | No committed record names the reviewers of `85311f6` and `63cb645`, and the rerun row of `chore/entry-budget-phone-frame` quotes the gate's end-to-end command without `--trace=off`, which that gate ran. | Fixed by this record: `85311f6` had a standard and a security-profile readiness reviewer, both ready with minor findings that were fixed before its gate; `63cb645` had a standard reviewer (approve, one optional minor on K21's wording, left) and a security-profile reviewer (approve, T-41 examined, no finding) on its first commit `46e324b`; after that commit's failed gate and the owner's approved rerun, `63cb645` rebuilt it with only the two known-limitations rows added, which were not re-reviewed. Their reports are kept outside the repository by the coordinator. Follow-up R corrects the quoted command. |
| Security | Minor (documentation) | The rerun rows of 2026-11-21 and 2026-11-22 quote the owner's quiz answer in Russian; everything committed is in English. | Follow-up R. |
| Security | Minor (T-19) | `frontend/e2e/diagnostic-redaction.test.ts` has no case for userinfo in a URL, another host's presigned query, or `data:` and `blob:` addresses; the code drops all three. | Carried into 10.5 H3 by the owner on 2026-11-22. |
| Security | Minor (T-19, T-27) | A short lowercase token segment under a parent not in `tokenParentSegments` would be kept; no current route has one. | Accepted by the owner on 2026-11-22; a new route with a token in its path meets the security trigger and adds its parent. |
| Backend | Minor (test coverage) | X3 taught the fake student list to filter by `q` (`frontend/src/lib/api/fakeTransport.ts`), but `frontend/src/lib/api/transportContract.ts`, which also runs against the real backend, has no `q` case; the fake filters a `q` over 254 characters that the backend refuses with `422`. | Follow-up F adds the case and the refusal. |
| Frontend | Minor | The entry chunk's headroom after `63cb645` was recorded nowhere after G1. | Fixed by this record: A1's passing gate printed 183.3 KB of 184.0 KB, so 0.7 KB is left for 10.4. |
| Frontend | Minor | Builder Preview on phones opens scrolled to its end: Close follows the student view and takes the initial focus (`frontend/src/features/builder/PreviewDialog.tsx`, `Dialog.tsx`). | Fixed by follow-up F: by the owner's decision of 2026-11-22, below 768 px the preview opens on its heading, at its top, while Close stays at the end, because Safari scrolls a full-screen dialog to a focused Close at its end even with `preventScroll`; `builder.spec.ts` asserts the heading's focus and `scrollTop` 0 with five long blocks, and `pages.md` says so (pending owner review). |
| Frontend | Minor | The feedback-seen mark is lost when an autosave moves the version while the POST is in flight and the POST answers `409` (`frontend/src/features/assignment/seenMarker.ts`); its failures are swallowed. | Follow-up F, with a component test of that order. |
| Frontend | Minor | Messages loses focus to `body` when the width crosses 768 px while the channel loads, failed or is not found, because Back is then rendered outside `Conversation` (`MessagesPage.tsx`). | Follow-up F. |
| Frontend | Minor | Between 767 and 768 px (under zoom) both `(min-width: 768px)` and `(max-width: 767px)` are false in `MessagesPage.tsx`, which then shows the phone conversation in the computer frame. | Follow-up F derives one from the other. |
| Frontend | Minor | Two names on lines G3 touched lack `dir="auto"` (`InvitationPage.tsx`, `ReclaimPage.tsx`). | Follow-up F. |
| Frontend | Minor | The student's 360 px Notifications sweep always measures the empty state. | Follow-up F seeds a student notification; see the row below. |
| Frontend | Minor | The phone-only 44 px targets of Today's homework panels and the group page also apply from 768 to 1023 px. | Accepted by the owner on 2026-11-22 (cosmetic). |
| Frontend | Minor | Untested width-crossing branches: widening with focus on a bar item or More, the bell-to-bell branch of `useFrameSwitchFocus`, narrowing with focus on a channel link. | Accepted by the owner on 2026-11-22. |
| Frontend | Minor | No sweep renders the More sheet with a 60-character name. | Accepted by the owner on 2026-11-22. |
| Frontend | Minor | A failed sign-out's alert shows again when the sheet (or the sidebar) reopens. | Accepted by the owner on 2026-11-22. |
| Frontend | Minor | The student's sweep seeds no lessons, so the student's Schedule agenda is not measured at 360 px; markup names on My tutors, the reclaim choice and the student's Schedule are not tested at 360 px. | Accepted by the owner on 2026-11-22. |
| Frontend | Minor | The audit record did not probe Firefox, measured the token pages only in their invalid-link states, leaves the Issue dialog `unverified` at 1440 and 320 px, and leaves VoiceOver and Safari on macOS and iOS `unverified`. | Accepted by the owner on 2026-11-22 as `unverified`, as D44 and the plan's risks allow. |

## Known-limitations entries and their resolution

Every row of sub-slice 10.3 and every row carried into 10.3 was given to its layer's reviewers. Follow-up R wrote these resolutions into the file.

| Entry | Reviewers | Resolution |
| --- | --- | --- |
| The texts of `states.md` and the 0.7 KB left in the entry chunk (10.2, carried) | Frontend | Fixed: the risk came true and was resolved by the owner's raise in `63cb645`; 0.7 KB is left again at `0dcfe21` |
| `LoadFailed` loses focus when a live refetch resets it (10.2, carried into A1) | Frontend | Fixed by A1 |
| The burst test's early poll | Frontend | Accurate; stays accepted by the owner (2026-11-20) |
| Show more re-checks a few microtasks after the read | Frontend | Fixed by X3 |
| `useFocusOnFirstNewRow` in the member picker | Frontend | Fixed by X3 (`key={search}`) |
| `followUpQueuedFor` resolves after the first batch | Frontend | Limitation accurate; its rationale corrected by follow-up R (the `connected` frame refetches every active query in one batch); accepted by the owner on 2026-11-22 |
| Show more after a failed refetch before the follow-up decides | Frontend | Accurate; accepted by the owner on 2026-11-22 |
| `assignment.spec.ts:594` request-failure flake | Tooling; redaction security | Not seen in the 15 later gates; X4 now names the request if it recurs; follow-up R updates the row; carried into 10.5 H3 (D47) |
| `TestUploadRecoveryBoundsCandidateAndPassDeadlines` flake | Backend | Accurate; carried into 10.5 H3 (D47). The backend reviewer sharpened the cause: the first phase's 200 ms recovery deadline leaves about 100 ms over two 50 ms candidate waits, so under `-race` and load the deadline can fire during the second candidate; H3's plan must name the test change, since raising a budget needs approval |
| The Today Show more fix merged as two commits | Tooling | Accurate; stays accepted by the owner (2026-11-20) |
| `reclaim.spec.ts:144` Firefox diagnostics flake | Tooling | Fixed by X5 (0 failures in 240 runs with the fix); text corrected by follow-up R |
| WebKit browser closure with traces on | Tooling | Not seen in any 10.3 gate, which ran without traces; carried into 10.5 H3 (D47) |
| `lesson-room.spec.ts:473` Firefox flake | Tooling | Fixed by X5; no `page_load_failed` in the gates after X5; text corrected by follow-up R |
| The More sheet's in-sheet sign-out failure has no test | Frontend; frontend-boundaries security | Accurate: jsdom has no `showModal`, and no end-to-end technique the testing rules allow makes the sign-out fail; accepted by the owner on 2026-11-22 |
| No `viewport-fit=cover`, so the safe-area inset is 0 on iOS | Frontend; security | Accurate, no security aspect; accepted by the owner on 2026-11-22; the device check is repeated in 10.4, whose web push on iOS needs the home-screen app |
| Explicitly sized outline icons stay at that size on phones | Frontend | Fixed by A1 (`data-outline-icon` on every icon) |
| The student's Notifications sweep may measure the empty state | Frontend | Still open and always empty; fixed by follow-up F |
| The student's Schedule filter order on phones | Frontend | Fixed by A1 |
| The invitation page's "Use another account" on phones | Frontend | Resolved by the spec change in `pages.md` |
| G3 does not assert full-screen dialogs at 360 px | Frontend | Fixed by A1 (`ending.spec.ts` asserts the 360 × 780 box) |
| Homework tables repeat hidden columns' values | Frontend | Accurate; follow-up R adds `assignment.spec.ts:747` to the affected assertions; accepted by the owner on 2026-11-22 |
| The homework sweeps' unseeded file name and review student | Frontend | Accurate; accepted by the owner on 2026-11-22 |
| The back link above the title on the Lesson and group pages | Frontend | Fixed by A1 |
| G2's 360 px tests do not cover every Today element | Frontend | Accurate; accepted by the owner on 2026-11-22 |
| Two G2 phone rules depend on files other changes own | Frontend | Fixed (`today.spec.ts` asserts both) |
| `desktop-layout.spec.ts:382` WebKit flake | Tooling | Fixed by X5; text corrected by follow-up R |
| Messages hides the frame before the chat texts load | Frontend | Accurate; accepted by the owner on 2026-11-22 |
| `review.spec.ts:149` WebKit flake | Tooling | Fixed by A1 (by inspection; it passed in the final gate); a recurrence goes to 10.5 H3 |
| `lesson-form-layout.spec.ts:220` WebKit flake | Tooling | Fixed by A1 (by inspection; the fix entered after A1's failed gate, see the A1 rerun row); a recurrence goes to 10.5 H3 |
| The builder preview tests at 360 px fail about one run in ten in WebKit (found by follow-up F's change checks) | Follow-up F; this record | Predates the follow-up (3 of 30 on `main`, 4 of 30 on its branch); carried into 10.5 H3 by the owner on 2026-11-22 |
| Rerun rows of 10.1 and 10.2 | Tooling | Unchanged since `fbe9456` and accurate |
| Rerun row of the Today Show more fix (2026-11-20) | Tooling | Accurate: 610.1 s, `[firefox-3] assignment.spec.ts:594`, written in `f347b78`, whose tree is the passing gate's |
| Rerun row of `chore/entry-budget-phone-frame` (2026-11-21) | Tooling; agent-configuration security | Accurate except the quoted command, which lacks `--trace=off`; corrected by follow-up R |
| Rerun row of G2 (2026-11-22) | Tooling | Accurate: 529.4 s, `[webkit-1] desktop-layout.spec.ts:382`, written in `82a2ad9` |
| Rerun row of A1 (2026-11-22) | Tooling | Partly inaccurate (see the tooling finding); corrected by follow-up R and ratified by the owner |

There are no `after-fix` rows in 10.3. Every merged commit's tree equals its passing gate's tested tree.

## Gate record

From `merge-gate.log` (start lines, result lines with wall time and phases). The follow-ups' gates also counted six other containers, after the owner's decision of 2026-11-22 that every merge report names each counted container; this record does not hold those reports, so the naming is the coordinator's record. Power was mains at every start; only the failed gate of `chore/entry-budget-phone-frame` dipped onto battery during its run.

| Gate | Base | Tested tree | Load at start | Other containers | Wall time | Phases pre / lanes / end-to-end | Result |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Review records `4f291ef` | `50b9338` | `0b2cb5f` | 2.79 2.49 2.84 | 3 | 591.8 s | 0.2 / 190.8 / 399.1 | passed |
| End link after account closed `da8b43d` | `4f291ef` | `b2d987f` | 2.19 5.91 8.37 | 3 | 583.0 s | 0.2 / 196.6 / 384.5 | passed |
| Today Show more fix, first | `da8b43d` | `1bff94e` | 1.23 5.92 9.58 | 3 | 610.1 s | 0.2 / 196.4 / 411.8 | failed, end-to-end |
| Today Show more fix `d1dda06`, `f347b78` | `da8b43d` | `62dd6ca` | 3.32 5.94 10.24 | 3 | 674.8 s | 0.2 / 224.2 / 448.7 | passed |
| Gate gap record `02ac903` | `f347b78` | `80a43b4` | 2.89 2.24 1.71 | 3 | 13.1 s (Markdown-only) | | passed |
| 10.2 closing record `fbe9456` | `02ac903` | `8527204` | 2.48 5.85 5.22 | 4 | 16.0 s (Markdown-only) | | passed |
| Traces off `85311f6` | `fbe9456` | `6bc7731` | 1.89 5.78 10.95 | 6 | 608.1 s | 0.2 / 214.5 / 391.7 | passed |
| R3 `ef9a98d` | `85311f6` | `86feebc` | 1.54 2.02 2.31 | 6 | 12.3 s (Markdown-only) | | passed |
| X3 `9c9c1ae` | `ef9a98d` | `98d089c` | 1.55 5.88 6.71 | 6 | 491.6 s | 0.2 / 180.5 / 309.2 | passed |
| X4 `94b8705` | `9c9c1ae` | `8ac6662` | 1.13 5.81 7.80 | 6 | 504.7 s | 0.2 / 191.9 / 310.9 | passed |
| G0 `2cbacf4` | `94b8705` | `7fb0914` | 1.39 5.78 8.32 | 6 | 502.8 s | 0.3 / 185.4 / 315.5 | passed |
| Entry budget, first | `2cbacf4` | `40d3802` | 1.52 5.74 5.17 | 6 | 660.4 s | 0.2 / 278.8 / 379.7 | failed, end-to-end; battery during the run |
| Entry budget `63cb645` | `2cbacf4` | `41552b8` | 3.25 5.71 8.79 | 6 | 499.4 s | 0.2 / 184.7 / 312.9 | passed |
| G1 `57319f5` | `63cb645` | `0f1849f` | 1.49 5.71 8.72 | 6 | 500.6 s | 0.2 / 182.0 / 316.8 | passed |
| G3 `d449e83` | `57319f5` | `f379b28` | 2.19 5.73 7.02 | 6 | 508.7 s | 0.2 / 187.5 / 319.3 | passed |
| G4 `d315626` | `d449e83` | `9e2624b` | 3.93 5.91 5.01 | 6 | 528.9 s | 0.2 / 189.5 / 337.5 | passed |
| G2, first | `d315626` | `aa244f4` | 3.13 5.83 7.54 | 6 | 529.4 s | 0.2 / 195.0 / 332.6 | failed, end-to-end |
| G2 `82a2ad9` | `d315626` | `f54029a` | 1.55 1.70 2.60 | 6 | 534.4 s | 0.2 / 193.1 / 339.4 | passed |
| G5 `32429b4` | `82a2ad9` | `c378b21` | 2.98 5.72 6.95 | 6 | 540.0 s | 0.2 / 190.4 / 347.7 | passed |
| X5 `ea97b12` | `32429b4` | `1843b68` | 3.09 5.07 8.55 | 6 | 661.9 s | 0.2 / 238.0 / 422.0 | passed |
| A1, first | `ea97b12` | `acc60f3` | 2.98 4.28 5.53 | 6 | 669.7 s | 0.3 / 248.7 / 418.9 | failed, end-to-end |
| A1 `0dcfe21` | `ea97b12` | `f326df1` | 4.11 5.25 8.36 | 6 | 589.2 s | 0.2 / 222.5 / 364.7 | passed |
| Follow-up F `195d190` | `15317dc` | `c40b05a` | 2.13 4.67 5.47 | 6 | 607.1 s | 0.6 / 213.6 / 391.2 | passed |
| Follow-up R `24226bd` | `195d190` | `e33082d` | 1.87 5.66 8.85 | 6 | 585.2 s | 0.7 / 207.5 / 375.2 | passed |

## Check cost of the phone tests (D43)

Measured as D43 defines it, from each passing gate's list-reporter output: the summed durations of the end-to-end runs a change added minus those it removed, keyed by engine, file and title, against the previous passing full gate.

| Change | Runs added / removed | Net summed duration |
| --- | --- | --- |
| X3 `9c9c1ae` | 0 / 0 | 0.0 s |
| X4 `94b8705` | 3 / 0 | +1.1 s |
| G1 `57319f5` | 93 / 48 | +27.0 s |
| G3 `d449e83` | 24 / 18 | +11.2 s |
| G4 `d315626` | 18 / 0 | +21.7 s |
| G2 `82a2ad9` | 15 / 6 | +11.4 s |
| G5 `32429b4` | 21 / 15 | +11.5 s |
| X5 `ea97b12` | 3 / 0 | +0.9 s |
| A1 `0dcfe21` | 15 / 6 | +4.6 s |
| **Total** | 2,180 → 2,279 runs | **+89.5 s**, about 17 s of wall time at 5.3 test-seconds per second |

The other changes of the list added or removed no run. The total is inside D43's target of about 130 s. D43's method does not count tests that grew under the same title (the sweeps extended to 320 and 780 px by A1): the gates' end-to-end phase grew from 312.9 s (`63cb645`) to 347.7 s (G5) at comparable lanes phases, about 35 s of wall time, above the target's 25 s; the later phases (422.0 and 364.7 s) ran with lanes phases 30 to 50 s slower, a sign of outside load. The owner saw both figures on 2026-11-22.

## Consolidation outcome

At the review the last three passing full gates were G5 at 540.0 s, X5 at 661.9 s and A1 at 589.2 s, a median of 589.2 s, above ADR-0018's 540 s. With traces off, the nine full gates from X3 to G5 ran 491.6 to 540.0 s with lanes phases of 180.5 to 195.0 s; the two slow ones had lanes phases of 238.0 and 222.5 s, which points to outside load rather than test growth. On 2026-11-22 the owner decided not to consolidate now: the record keeps the median and its cause, the quiet-machine rule is enforced, and the question returns at the 10.4 slice review.

## Threats examined

| Threat | Reviewers | Result |
| --- | --- | --- |
| T-3 Account enumeration | Frontend-boundaries security | Not affected: classes only on the sign-in and sign-up pages |
| T-5 A tutor reaches another tutor's work | Authorization security; backend | Examined: the stop route's refusal equals a random identifier's |
| T-9, T-30 Answer keys, points and scores | Authorization security; frontend-boundaries security | Examined: the seen marker sends server identifiers; the students' phone copies show only the tutor's name and the state |
| T-10, T-11, T-13, T-24, T-35 | Frontend-boundaries security | Not affected: join, signed addresses, uploads and downloads unchanged beyond classes |
| T-14 Script injection | Frontend-boundaries security; frontend | Examined: every name and title is still a React text node; markup names seeded in the sweeps |
| T-16 Content security policy and framing | Frontend-boundaries security | Examined: unchanged (D42) |
| T-18, T-19 Personal data and secrets in output | Redaction security | Examined: X4's records; two minor findings (redaction cases, the token-segment residual) |
| T-20 Resource exhaustion | Authorization security | Examined: at most one seen POST per shown review change, the 600 per hour limit |
| T-21 A removed participant keeps access | Authorization security; backend | Examined: unchanged controls |
| T-25 Lesson access and stale links | Frontend-boundaries security | Examined: the new tab's opener is null at 360 px |
| T-27 Link scope | Redaction security; frontend-boundaries security | Examined: token paths redacted in diagnostics; the token pages unchanged |
| T-28 Open redirects | Frontend-boundaries security | Not affected: `safeNext` unchanged |
| T-34 Data after deletion | Authorization security; backend | Examined: the stop route's `500` became `not_found` at `account_closed_until` |
| T-37 Browser storage | Frontend-boundaries security | Examined: no Web Storage added |
| T-39 Dependencies and tools | Agent-configuration security | Not affected: no tool, pin or lockfile changed |
| T-41 Agent configuration and verification policy | Agent-configuration security; tooling | Examined: four minor findings and the two tooling majors on gate records |
| T-44 Unconfirmed accounts | Frontend-boundaries security | Examined: the phone frame only hides what the API refuses |

## Owner decisions

Each was answered by a multiple-choice question on 2026-11-22.

- **The consolidation question:** no consolidation now; the question returns at the 10.4 review.
- **The frontend and backend minor findings:** one follow-up change fixes Builder Preview's scroll, the seen mark after a `409`, Messages' focus across 768 px, its media-query gap, the two `dir="auto"` names, the student's Notifications sweep and the fake's `q` contract; the other minor findings are accepted.
- **The records and policy findings:** one records follow-up adds the evidence-run rule to ADR-0018, aligns its provenance, renders the quiz answers in English and corrects the known-limitations rows named above; the A1 rerun's extra test-only diff is ratified as a one-time exception; the redaction cases are carried into 10.5 H3; the token-segment residual and the unused `unexpectedFailures` are accepted.
- **The container count of the 10.3 gates:** record the breach and ratify the twelve merges as a one-time exception; the owner stops the leftover containers that are not OVC's; the coordinator names every counted container in each merge report.
- **The recurring rerun after a change's own fix:** the stricter rule, that such a gate is an after-fix gate needing evidence, goes into ADR-0018 and its copies.
- **2026-11-22, Builder Preview on phones:** after the planned `preventScroll` failed in WebKit, the preview opens on its heading below 768 px.
- **2026-11-22, the builder preview flake in WebKit:** carried into 10.5 H3 with a known-limitations row, since its rate on `main` matches the branch's.
- **2026-11-22, a changed diff whose evidence series has a run that ran no test:** the change returns to its branch, and only a further change of its diff with a new, complete evidence series admits a gate.
- **2026-11-22, the follow-ups' gates:** follow-up F's gate took 607.1 s and merged on the owner's answer.

## Carried work

- **Follow-up F** (`195d190`) fixed the six frontend findings and the fake's `q` contract case, each with a test; its change checks found a WebKit flake of the builder preview tests at 360 px that predates it (3 of 30 runs failed on `main`, 4 of 30 on the branch), recorded in the [slice 10 known limitations](slice-10-known-limitations.md) and carried into 10.5 H3 by the owner on 2026-11-22.
- **Follow-up R** (`24226bd`) wrote ADR-0018's evidence-run sentence, the stricter rerun rule and its clause for a broken evidence series (decided by the owner on 2026-11-22), their copies in `AGENTS.md`, ovc-verify and the development guide, the naming of every counted container in merge reports, the sections on the sub-slice 10.3 gate reruns and breach in ADR-0018 and the threat model, and every known-limitations correction and resolution above, all pending owner review.
- **Into 10.4:** the 0.7 KB entry-chunk headroom for push texts; the iOS safe-area device check.
- **To the next slice review:** the consolidation question, with the median of the gates since this review.
- **Into 10.5 H3:** the `assignment.spec.ts:594`, upload recovery and builder preview flakes, the WebKit browser closure under traces, and the redaction test cases.
- The picomatch acceptance, which expires on 2026-12-01, stays the owner's decision outside 10.3.

## Closing commit

Follow-ups F and R have merged (`195d190`, `24226bd`). This record's commit closes sub-slice 10.3; the owner confirms it as 10.3's closing commit.
