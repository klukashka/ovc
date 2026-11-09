# Design slice step D7: the lesson page on the layout standard

Status: task plan approved by the owner on 2026-11-09 after its security-profile planning review, with Edit in the header and the other tutor actions in a Lesson actions menu. On approval it is committed as `knowledge/plans/design-step-d7.md` in this step's change. It carries out step D7 of the [interface decisions](ui-ux-decisions-2026-11-06.md#order-of-work) (decision 5, the join-block proposal) and the [design slice plan](design-slice.md). Base: `main` after step D6 merges, which adds `roomEntry.ts` and `JoinControl.tsx` and amends the threat model. Precondition met: stage 4 of the [slice 7 local plan](slice-7-local-completion.md#4-lesson-ui-and-local-pilot-scenarios) merged as `3a996bb`, and slice 7 has not decided against the join-block proposal. Open slice 7 branches that touch `features/schedule/` are listed when the work starts. None are open at writing.

## 1. Problem and outcome

**Problem.**

- The lesson page shows the date, time and audience twice: in the main column above the room and again in "About the lesson".
- The join block sits below them, so the page's most important action is not the first thing in its column.
- The tutor's Edit, Cancel and "End this series" buttons, and the chat link, sit in the page body.

**Outcome.** The page follows the layout standard of step L: a page-width header, and two columns from 1280 px, stacked below that by `PageColumns`' CSS grid.

- **Header:** the lesson title as the page heading. On the right:
  - "Messages" when the lesson has a chat, for both roles;
  - for the tutor, while the lesson is editable as today, "Edit" and a menu "Lesson actions" with "Cancel lesson" and, for a series occurrence, "End this series and create a new one".

  The back link "Back to the schedule" stays above the header.
- **Main column, first:** the join block, which is the live phase word (`role="status"`, unchanged) followed by the lesson room (`LessonRoom`, unchanged). The description follows.
- **Side column:** "About the lesson" is the one place for the date and time (with the second timezone), the format, the audience or members, the tutor and the series. The duplicate lines above the room are removed.
- **Behavior unchanged:** every handler, dialog, refusal, notice, focus rule, live update, tab and frame rule of the page and the room stays. Only placement and the openers of the tutor's actions change.

## 2. Verification tier and security trigger

Standard tier. The security trigger is **met**: the change rearranges the page that holds the join action (T-10, T-25). This plan had a security-profile planning review, whose findings are folded in. The change gets one standard reviewer and one security-profile reviewer.

How the obligations are kept:

- **T-10.** The room and its handlers are untouched, and the offers come from the same lesson data. The tutor's header actions are hidden in the interface only; the server's authorization is unchanged.
- **T-25.**
  - The tab, frame and join-address rules live in files this step does not change (criterion 6).
  - The room is never remounted by the layout: its element keeps `key={lesson.id}` and its props verbatim, and its position and ancestors in the view do not depend on the phase, the controls, notices, refetch state or role state.
  - The breakpoint switch is CSS only, never a JavaScript media query that renders different trees.
  - Tests prove that a held join survives the moment the lesson goes live and the header controls leave (criterion 4).
- **Threat model** (pending owner review), as the design slice plan requires D7 to map itself:
  - T-25 gains the evidence that the lesson page keeps the room mounted across its header and layout changes, citing the new tests;
  - T-10 notes that the tutor's header actions are hidden in the interface only.

## 3. Question for the owner, asked with this plan

1. **The tutor's actions.** Recommended: "Edit" in the header, with "Cancel lesson" and "End this series and create a new one" in a "Lesson actions" menu. This matches the student card and the group page, where dangerous actions sit in a menu. The alternative is the three buttons in the header row.

## 4. What changes

- **`features/schedule/LessonPage.tsx`:**
  - **Dialogs stay at page level.** `EditLessonDialog`, both `CancelLessonDialog`s and the `NewLessonDialog` after an ending stay where they are, mounted whether or not the controls are shown. The header slots render only the openers. So a live change still closes an open dialog onto the changed-meanwhile notice, and the New lesson dialog after an ending still closes onto "Lesson cancelled".
  - **Header slots never `undefined`.** The page passes `Page`'s `action` slot (Messages, Edit) and `menu` slot ("Lesson actions") in every role and state, including not found and inactive tutors. A slot with nothing to show passes `null`, so the heading element is never swapped.
  - **Focus when the controls leave.** `controlsShown = tutorWrites && isEditable(lesson, now)` is computed in `LessonPage`, and `useHeadingWhenControlsLeave` is called there, where it stays mounted.
  - **Refs.** One `menuButton` ref, through `ActionMenu`'s `buttonRef`, is the `returnFocus` of both Cancel dialogs and the New lesson dialog. The `cancelButton` and `endSeriesButton` refs are removed, and `editButton` stays on the header Edit.
  - **`LessonView`.** `PageColumns` puts the join block (phase word, then the unchanged `room` element) and the description in the main column, and `AboutLesson` in the side column.
- **`lib/i18n`:** "Lesson actions" / "Действия с уроком".
- **Tests:**
  - **`e2e/lesson.spec.ts`:**
    - At 1440 × 900 the join block (the phase word and the room's control) is the first content of the main column and inside the viewport without scrolling, for a tutor in the window and for a student while live.
    - The date and time text appears once.
    - Edit, Cancel and "End this series" open from their new places and return focus there.
    - A Cancel dialog opened from the menu closes onto the changed-meanwhile notice when a live change makes the lesson not editable.
  - **Absence assertions rewritten.**
    - Where the page must offer no tutor actions (a student, a cancelled lesson, a past lesson), the test asserts that the "Lesson actions" menu button is absent.
    - Where the menu exists but an item must be absent (a one-off lesson has no "End this series"), the test opens the menu, asserts the item is absent and asserts "Cancel lesson" is present.
    - This covers the tests at `lesson.spec.ts` lines 401, 492, 591, 732 and 743 on `main`.
  - **`e2e/lesson-room.spec.ts`:**
    - **Embedded path:** from the click on Join until the header no longer shows Edit and the status reads live, exactly one room document loads and the frame keeps focus.
    - **New-tab path:** with the existing slow-join setup, the shared clock passes the start while the join is in flight; the blank tab stays open and the answer enters it.
  - **`LessonPage.test.tsx`:**
    - the harness renders the controls through `Page`'s slots with the page's gating, and the two tests of focus reaching the heading when the controls leave keep their assertions;
    - a new case: focus on an open "Lesson actions" item when the start passes moves to the heading;
    - the heading element is the same node before and after the controls leave.
  - **Other files:** the tests in `schedule.spec.ts` and `lesson-form-layout.spec.ts` that open these actions change only their opener.
- **Specifications** (pending owner review):
  - **`knowledge/design/web/pages.md` Lesson:**
    - the layout and the actions' places, including the chat link in the header;
    - the "Position" line ("before the description and the Edit and Cancel buttons");
    - the end-series sentences on returning focus to the button and on the button standing after Edit and Cancel.
  - **The interface decisions' join-block proposal** is marked as applied.
  - **`docs/threat-model.md`:** the T-25 and T-10 notes above.

## 5. Acceptance criteria

1. In Chromium, Firefox and WebKit, `lesson.spec.ts` › at 1440 × 900 the join block is the first content of the main column and is visible without scrolling, for a tutor in the join window and for a student in a live lesson.
2. The lesson's date and time text appears once on the page.
3. **The tutor's actions.**
   - Edit opens its dialog, and a dismissal returns focus to Edit.
   - Cancel and "End this series" open from "Lesson actions", and a dismissal returns focus to the menu button.
   - A Cancel dialog opened from the menu closes onto the changed-meanwhile notice on a live change.
   - The rewritten absence tests assert the menu button's absence, or open the menu and assert the item's absence beside "Cancel lesson". Rewriting them this way keeps the assertions; it does not loosen them.
4. **The room is never remounted.** `lesson-room.spec.ts` proves it:
   - **Embedded:** one room document, with focus kept, while the lesson goes live and the header controls leave.
   - **New tab:** a join in flight across the start enters its blank tab.
5. `LessonPage.test.tsx` proves that focus reaches the heading when the controls leave, including from an open menu item, and that the heading node is kept. `LessonRoom.test.tsx`, `JoinControl.test.tsx`, `lesson-room.spec.ts` and `lesson.spec.ts` pass, with no assertion removed or loosened.
6. This command exits 0: `git diff --exit-code <base> -- frontend/src/features/schedule/LessonRoom.tsx frontend/src/features/schedule/roomEntry.ts frontend/src/features/schedule/roomTab.ts frontend/src/features/schedule/roomState.ts frontend/src/features/schedule/JoinControl.tsx frontend/src/features/schedule/LessonDialogs.tsx frontend/src/features/schedule/lessonTiming.ts frontend/src/features/schedule/NewLessonDialog.tsx`. The `<LessonRoom …/>` element in `LessonPage.tsx` keeps its key and props verbatim.
7. The change checks pass, including the whole `pnpm e2e` and `tools/docs/check-docs.sh`. Live-server evidence is reported as `unverified`.

## 6. Order of work

1. Worktree on `feat/design-step-d7-lesson-page` from `main` after D6.
2. The header slots, refs and page-level focus hook.
3. The columns.
4. Tests, then the specifications and the threat model.
5. Change checks, the standard and security reviewers, fixes under the after-fix rule, the readiness report, the gate, the merge and finish.

## 7. Observability

None. No backend change and no new request.

## 8. Risks

- **A remount of the room** would close a held tab or reset the frame. It fails closed, but it breaks the join. The invariants of section 2 and criterion 4 guard it.
- **Absence tests becoming vacuous** behind a closed menu. Criterion 3 fixes how they are written.
- **The header heading swap.** Slots are never `undefined`, and the node-identity assertion of criterion 5 guards it.

## 9. As built

- The page computes `controlsShown` through the exported `lessonControlsShown(me, lesson, now)` (the active tutor and `isEditable`), and the header openers are the exported `LessonEditButton` and `LessonActionsMenu`, so the unit test harness renders them through `Page`'s slots with exactly the page's gating instead of a stand-in control.
- "About the lesson" is no longer its own `aside`: `PageColumns` already wraps the side column in a complementary landmark labelled "About the lesson", so the section keeps its heading inside a plain container.
- The audience line above the room was the only place where a student saw "Individual lesson" for a one-to-one lesson, which the [label table](../specs/schedule.md#labels-per-role) and an end-to-end test require. With it removed, the student's "Format" detail reads that label; the tutor's and group lessons keep "Individual" and "Group".
- The back link "Back to the schedule" stays where it was, the first line under the header: `Page` renders its header first and has no slot above it.
- `schedule.spec.ts` and `lesson-form-layout.spec.ts` open only Edit, which keeps its name and role in the header, so their openers needed no change.
- The join-block proposal of the [interface decisions](ui-ux-decisions-2026-11-06.md) is applied by this step. That file records no per-step status, so the mark lives here rather than there.
