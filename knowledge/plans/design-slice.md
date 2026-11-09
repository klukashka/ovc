# Design slice implementation plan: the desktop interface

Status: approved by the owner on 2026-11-06. It carries out the [interface decisions of 2026-11-06](ui-ux-decisions-2026-11-06.md), which hold the findings, the decisions, the step table with each step's main check and the open items; this plan does not repeat them, and when the two disagree the owner decides. The owner decided on 2026-11-06 how this slice is reviewed at its close (section 5) and where the lesson page and the Join button go (section 4).

## 1. Problem, outcome and scope

**Problem.** The signed-in pages run down one 720 px column, Today and Home are nearly empty, and Messages, the week view and the student card waste a computer's screen. Slice 8 adds materials to these same pages, so the rework is cheaper before it.

**Outcome.** On a computer every signed-in page follows the layout standard of decision 7, Today and Home show a person's day, colour is chosen by swatch alone from a palette whose colours can be told apart, and Messages, the New lesson dialog with the week, the student card with the group page and the lesson page are reworked.

**In scope.** D0, D1, L, C, D2, D3, D4, D5, D6 and a new step D7, the lesson page on the layout standard.

**Out of scope.**

- The video room, and the lesson page's behavior, which stay with slice 7. D7 changes only the page's layout, and applies the join-block proposal unless slice 7 has decided otherwise.
- Phone layouts and the phone navigation frame, which stay in slice 10. Below 1024 px pages keep the interim one-column frame.
- New requirements or roles. A step that finds it needs one, or a new API route or a change to who may do what, stops and asks the owner.

**Already merged and counted in this slice.** The Russian student tab labels (`06f2f3c`) and the lesson form layout (`1cae202`).

**Carried from slice 7** by the owner on 2026-11-08 ([dispositions](../reviews/slice-7-known-limitations.md#owner-dispositions-2026-11-08)):

- WebKit end-to-end failures under heavy load: the next WebKit gate failure is diagnosed from its kept redacted evidence.
- End-to-end readiness failures seen only under a stress run: a time-boxed causal check on an idle machine.
- Join on Home and Today, already step D6.

The owner decided on 2026-11-08 that the stress readiness check, the diagnosis of WebKit failures under load and performance measurements wait until OVC is deployed on a server, and moved the two items above into Slice 10 (results, home pages and hardening), which prepares the pilot deployment of [ADR-0013](../../docs/decisions/0013-pilot-deployment.md) that first puts OVC on a server; this slice concentrates on the interface. Merge gates and change checks stay as they are. A WebKit failure of a gate is still recorded from its redacted evidence in [the WebKit failure record](../reviews/design-slice-webkit-failures.md) and goes to the owner, without a load diagnosis. The slice review below keeps its performance profile as a review of performance risks in the code; only the measurements on the machine wait for the deployment (owner's decision of the same day).

## 2. Verification tier and security trigger

- **This plan and the roadmap change (D0):** the plan meets the [security trigger](../../AGENTS.md#development-verification), because D2, D6, D7 and part of L touch trust boundaries. It gets one standard reviewer and one security-profile reviewer before the owner approves it. It changes no behavior and no verification policy.
- **Each later step:** standard tier with its own task plan, change checks, reviewer and merge gate. Each task plan restates the tier and the trigger with reasons, and the readiness report rechecks them against the final diff. A step that splits into several changes says so in its task plan, and each change is standard tier.
- **Closing:** the slice tier, described in section 5.

| Step | Trigger at the start | Condition or reason |
|---|---|---|
| D1 | Not met | Removes a readiness poll and changes no route. `/health/ready` stays. The test replacing the `smoke.spec.ts` readiness assertion keeps that test's frame assertions. "Add student" keeps the unconfirmed tutor's prompt. |
| L, account and teaching settings | **Met** | The forms handle passwords and end sessions, and Teaching holds the tutor code and unblocking (T-1, T-2, T-8, T-33). No handler, route or token handling changes. Moving the password change into a dialog must clear its fields on close and remount them on open, as the delete dialog already does. Whether sign out everywhere gets a confirmation is decided in its task plan. |
| L, Students, group list, My tutors, Notifications | Not met, unless handling changes | Invitation links, join-request decisions and Stop studying move on the page and their handlers stay as they are (T-7, T-21, T-27, T-33). |
| C | Not met | Stored keys stay. A key renamed reopens the classification. Swatches must not violate the style content security policy. |
| D2 | **Met** | Shows data from several relationships on landing pages. See section 3. |
| D3 | Not met while restyling | Reclassified if it changes message rendering or link parsing, the upload client, the prompt to confirm an email before sending files, the access-loss notice or the student group header (T-6, T-13, T-14, T-21). |
| D4 | Not met | The title prefill never uses a card's display name. The student search never enters the page address (T-18). |
| D5 | Not met, on conditions | Autosave sends `notes` alone, never a stale email, because that could revoke an invitation link; no draft in Web Storage, addresses or logs; a pending save is cancelled on sign-out or account change; a gone card's `404` is handled; "Only you see this" stays; a debounce bounds writes. Reclassified if invitation link handling changes (T-7, T-18, T-20, T-27, T-37). |
| D6 | **Met** | Drives join links and the meeting provider. See section 3. |
| D7 | **Met** | Rearranges the page that holds the join action; its handlers stay (T-10, T-25). |

## 3. Threats the expected-trigger steps map to

The D2, D6 and D7 task plans and L's settings change map themselves to [the threat model](../../docs/threat-model.md) and update it in the same change.

| Step | Threat | What the step must hold |
|---|---|---|
| D2 | T-5, T-6 | Today and Home read through existing relationship-scoped routes only. A student's Home never shows another student's work, and one tutor's data never appears under another. |
| D2 | T-14 | Lesson titles, audience labels, card names and tutor names render as text nodes with `dir="auto"`; an end-to-end test uses a label with markup. Today shows channels with unread counts and no message preview, as `pages.md` specifies. |
| D2 | T-21, T-29 | Today's specified refetch rules do **not** cover this: a removed member receives no lesson frame, and `liveRefetch.ts` does not refresh the calendar on a `relationship` frame. So a lesson stays on an open page until the next fetch, though the API already refuses it. A former group member gets no `relationship` frame, only a last `channel` frame, which `liveRefetch.ts` maps to nothing. D2 either invalidates its lesson queries on the frames that reach each of these people, extends the mapping in `knowledge/architecture/detailed.md` and tests it, or records this as an accepted residual with the owner. Its new queries enter that mapping either way. |
| D2 | T-44 | Home is one of the few pages an unconfirmed student reaches. Lessons of a `reclaim_pending` link stay hidden. An unconfirmed tutor's checklist does not depend on routes closed to them (`GET /students`, `GET /join-requests`), which would answer `403`. |
| D2 | T-18 | Nothing new reaches logs, addresses or third parties. |
| D6 | T-10, T-21, T-44 | The API alone decides who joins. The page may hide Join and never grants it, and each refusal (`403`, `404`, `409`, `503`) is handled as on the lesson page. `room_open` is per lesson, so a person who sees a live lesson only through a frozen roster can see it true and still be refused. |
| D6 | T-20 | `/calendar` has no `room_open`. The task plan names the data source and must not issue one `GET /lessons/{id}` per row without a bound. |
| D6 | T-25, T-27 | Join stays a `POST` on an explicit action, never on page load or through a link that joins on opening, because a tutor's join creates a meeting. The link never enters the query or mutation cache, Web Storage, history state, an OVC address or diagnostics. The opener rule of `pages.md` holds, and a failed join closes the tab opened for it. The lesson page's join lifecycle from slice 7 holds here too, including closing the tab on an account change and rejecting a late response from another account or lesson. |

**Negative tests D2 names in its task plan:** a student shared by two tutors, a former group member, an ended relationship, a provisional account, a `reclaim_pending` account, a label with markup, an account change clearing data, and an out-of-scope `?tutor=` value answered `404` and handled by the page. D6 evidence comes from the fake provider and is reported as `unverified` for a live server.

**Threat model updates to propose in those steps:** T-14 (Today and Home among the listed pages), T-44 (Home's new panels), and T-21 (the refetch or the accepted residual). None lands in this change.

**Decided here for D2.** Existing routes cover its parts: `/calendar`, `/tutors`, `/channels`, `/join-requests`, with `/users/me` and `/students` for the checklist. `/channels` pages by last activity and has no unread filter, so the task plan states how Today bounds its unread list and links to Messages. D2 does not register `GET /home`, which would add another copy of the visibility rule and carry homework data that does not exist yet. It stays in slice 10. D2 updates `pages.md` and scenario 3 step 4 of `pilot-scenarios.md`, which still place these panels in slice 10, and D6 updates scenario 1 steps 9 and 11.

**Left to the owner before D6.** The specifications disagree on whether an unconfirmed provisional student can join: the join rule admits `provisional` students (`schedule.md`), only unconfirmed accounts hold provisional links, and the join route is not marked `unverified ok` (`api.md`). This reaches Home directly. Unless slice 7 has settled it, the D6 task plan asks the owner a multiple-choice question. **Resolved** (design slice step D6, pending owner review): slice 7 settled it; the join route is `unverified ok` and the join rule admits a `provisional` student to a one-to-one lesson, as the backend test `testJoinProvisional` and the end-to-end test of an unconfirmed student joining from Home prove ([D6 task plan](design-step-d6.md)).

**Homework data.** The feedback excerpt and corrected-file flag of "recently checked work" are specified only in `GET /home`. The slice 9 plan extends `/assignments` or brings that part forward, keeping the rule that students never receive points.

**First-run dismissal.** `PATCH /tutor/settings` stays in slice 10 unless the D2 task plan brings it forward and the owner agrees. If brought forward, it is a separate change meeting the trigger: a strict schema accepting one dismissal field, because `tutor_profiles` also holds the quota columns, a refusal of unexpected properties, audience tests, and a decision on `unverified ok`.

## 4. Order, dependencies and running beside slice 7

- **Order.** D0, D1, L, C, D2, D3, D4, D5, then D6 and D7. L comes before D2 to D5 because they build on its components. C and L both edit `tokens.md`, so C follows L. D1 adds "Add student" with the existing header, and L moves it onto the new one.
- **D6 and D7 start on one local event.** They start once [stage 4 of the slice 7 local plan](slice-7-local-completion.md#4-lesson-ui-and-local-pilot-scenarios) has merged, which is when Join works from the lesson page. They need no remote server. Stages 1 to 3 of that plan come first, so the date depends on slice 7.
- **Slice 8 waits for this slice,** and so for stage 4 of slice 7's local plan, unless the owner decides otherwise.
- **Pilot scenario 1 steps 9 and 11,** which expect Join on Home, belong to D6, not to slice 7's additions to the pilot suite.
- **Shared files.** Slice 7 and this slice both touch `frontend/src/lib/i18n/`, `knowledge/design/web/pages.md`, `frontend/src/features/schedule/` and the end-to-end helpers. Each task plan lists the slice 7 branches open at its start. Whichever merges second takes the moved `main` under the normal squash rule; neither reformats the other's lines. D4's changes to `features/schedule/` are reviewed against slice 7's lesson work.
- **Heavy runs** go one at a time across all worktrees, because the end-to-end run lock is shared.

## 5. Closing the slice

The owner decided on 2026-11-06 that the existing rule applies unchanged: the slice review covers everything merged since the previous slice closed, as `AGENTS.md`, [ADR-0011](../../docs/decisions/0011-tiered-verification.md) and the roadmap say. No verification policy changes.

- **Base:** `e201e16`, the baseline of the [accepted slice 6 state](../reviews/slice-6-accepted-state.md). So this review also covers slice 7's local work merged by then, and the fixes between the two slices.
- **Reviewers:** one fresh reviewer per changed layer, which means the backend, the frontend with the correctness, browser and performance profiles, security, and tooling with agent configuration if anything there changed. The dark theme, Firefox and Safari, which the original interface review did not examine, are part of the browser profile here.
- **Slice 7 afterwards:** this slice's closing commit becomes the base of the next slice review, as ADR-0011 says, so slice 7's remote work is reviewed from there and nothing falls between them.
- **Known limitations** go to `knowledge/reviews/slice-design-known-limitations.md`, in the format the numbered files use. The closing commit is recorded under the slice's roadmap entry.

The review will be larger than a design slice alone, because it includes slice 7's backend. It starts after D6 and D7 have merged, so slice 7's stages 1 to 4 are in it, and stage 5 too if it has merged by then. It also satisfies the cumulative review that [slice 7's local plan](slice-7-local-completion.md) asks for local readiness, for whatever it covers, and it re-verifies the entries of `slice-7-known-limitations.md` and the slice 6 boundaries that [the accepted state](../reviews/slice-6-accepted-state.md) left to be reconciled. Anything merged after this slice closes falls to the next review.

## 6. Observability

D1 removes the readiness polling from Today and Home, which removes those requests from access records and changes no event. No step is expected to add a backend event; a step that adds or changes a route states its observability impact in its task plan, following [the observability specification](../specs/observability.md).

## 7. Acceptance criteria of this plan

- `tools/docs/check-docs.sh` passes.
- `docs/roadmap.md` has a section named "Design slice" between slices 7 and 8, marked pending owner review, that links this plan.
- Each item taken from slice 10 names where it went, and slices 8 and 9 name the homework blocks they take.
- `git status --short` shows only `docs/roadmap.md`, this file, `knowledge/plans/ui-ux-decisions-2026-11-06.md` and `knowledge/reviews/slice-1-review.md`.

The acceptance criteria of each step start from the [main check in the order of work](ui-ux-decisions-2026-11-06.md#order-of-work) and are completed in its task plan.

## 8. Risks

- **Removing the 720 px default touches every page and many end-to-end specs.** L's plan names them, and its pages move in groups, with the settings that meet the trigger in a change of their own.
- **D2 can grow.** It reads several relationships at once from several routes; request counts on Today and Home are checked against the existing budgets.
- **Slice 7's date sets this slice's end.** If stage 4 is late, D6 and D7 and the slice close are late, and slice 8 with them. The owner can reopen this.
- **The closing review is large.** Section 5 explains why. The security reviewer subdivides by family of trust boundaries, as the rules require, and the others may.
- **Parallel work with slice 7 collides.** Section 4 keeps each collision to a normal squash on a moved `main`. A conflict in meaning, as opposed to text, goes to the owner.
- **Flaky tests delay every gate.** At this plan's writing `TestPresenceOperationsRespectAdmissionAndShutdownDeadlines` and `TestAFailedPresenceReleaseRemainsPending` are open as flaky ([record](../reviews/lesson-form-layout.md#gate-rerun-exception)). A failed assertion is never rerun without the owner's decision, so a flaky gate can stop a step until its cause is fixed.
- **No real user has seen the interface.** The decisions rest on one review and the owner's remarks. Pilot use can reopen them.
