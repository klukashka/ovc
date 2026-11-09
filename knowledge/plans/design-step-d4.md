# Design slice step D4: the week view and the New lesson dialog

Status: task plan approved by the owner on 2026-11-09, with the chosen audience collapsing to one line and "Scheduled" removed everywhere except the lesson page. On approval it is committed as `knowledge/plans/design-step-d4.md` in this step's change. It carries out step D4 of the [interface decisions](ui-ux-decisions-2026-11-06.md#order-of-work) and the [design slice plan](design-slice.md). Base: `main` after step D3 merges. Open slice 7 branches at writing: `fix/lesson-room-client-residuals` and `fix/lesson-join-access`. They touch `LessonRoom`, `lessonCache`, `roomState` and `roomTab` and the backend, none of the files below.

## 1. Problem and outcome

**Problem.**

- The week always opens at 08:00 in a 576 px window, so lessons at 20:30 and 22:00 are out of view.
- "Scheduled" repeats on every ordinary lesson.
- The New lesson dialog lists every student as a radio button, takes the duration as a bare number, always shows a three-line description field, and clips the date field in Chromium.

**Outcome.**

- **Week view:** from 1024 px, the hours area fills the window's height below the toolbar and the day headings. It opens at the next lesson of the shown week. When no lesson is ahead, it opens at the week's earliest lesson; with no lessons, at 08:00 as today. The opening position is set when the week is first shown and when another week is chosen. A live refetch keeps the reader's position.
- **Status word:** a `scheduled` lesson outside its join window shows no status word in calendar blocks and lesson lists. "Starting soon", "Live", "Ended", "Did not take place" and "Cancelled" stay. The lesson page keeps "Scheduled"; it stays with slice 7.
- **New lesson dialog:**
  - **Audience:** a search field over a short result list (about six rows visible, then the list scrolls; "Show more" stays). Once an audience is chosen, the picker collapses to one line, "Name · Change". Change reopens the search and moves focus to the search field. A preselected audience (from a card, a group or a free slot with an audience) starts collapsed.
  - **Duration:** choices 45, 60 and 90 minutes plus "Other", which shows the number field with today's limits. A prefilled duration outside the presets selects Other with that value.
  - **Description:** collapsed behind "Add description". It opens by itself when the server refuses the description, and focus moves into it when it is opened.
  - **Date:** the date and start fields share a row from 768 px with widths that show the whole date in Chromium, Firefox and WebKit.

## 2. Verification tier and security trigger

Standard tier. The security trigger is **not met**, as the [design slice plan](design-slice.md) classifies D4, on two conditions this plan keeps:

- the title prefill never uses a card's display name (only a group's name, as today);
- the student search never enters the page address (T-18): it stays component state, and the search request is the existing `GET /students?q=`.

The request body of `POST /lessons` is unchanged. One standard reviewer reviews the change, followed by its change checks and its merge gate.

## 3. Questions for the owner, asked with this plan

1. **The chosen audience collapses to one line with Change** (recommended), or the list stays open with the chosen row marked.
2. **"Scheduled" disappears from lesson lists too** (student card, group page, Today, Home), not only from the calendar (recommended, one rule everywhere except the lesson page), or only from the calendar.

## 4. What changes

The audience and duration parts of this section and of sections 1, 5 and 8 are superseded by the owner-approved changes in section 9.

- **`features/schedule/WeekGrid.tsx`:** the hours scroller gets `desktop:max-h-[calc(100dvh-…)]` built from spacing tokens instead of the fixed 36 rem, which stays below 1024 px. The opening effect computes the target hour from the shown week's items: the first item whose end is after now, else the earliest item, else 08:00, scrolled so that its start sits one hour below the top. It runs when the range changes and once when items first arrive for that range.
- **`features/schedule/LessonParts.tsx`:** `LessonStatusWord` renders nothing for `scheduled` outside the join window. The block keeps its accessible name through title, time and audience.
- **`features/schedule/AudiencePicker.tsx`:** the result list gets a shorter height and hairline rows, and a collapsed "chosen" line with a Change button. The radio semantics stay.
- **`features/schedule/NewLessonDialog.tsx`:** a duration fieldset (radios 45, 60, 90, Other, with the number field shown for Other and submitted as `duration_minutes`); the collapsed description; the date and start row.
- **`lib/i18n/en.ts`, `ru.ts`:** "Change" / "Изменить", "Other" / "Другое", "Add description" / "Добавить описание", "{{count}} min" / "{{count}} мин".
- **Tests:**
  - `e2e/schedule.spec.ts` (or the existing week spec): at 1440 × 900 the week's only lesson at 20:30 is inside the viewport without scrolling; with a lesson at 09:00 and one at 20:30 on a later day, the view opens at the next one; ordinary lessons show no "Scheduled".
  - `e2e` for the dialog: choosing a student collapses the picker and Change moves focus to the search; the 90-minute preset creates a 90-minute lesson; Other accepts 75; a refused description opens the field; in Chromium the date field's `scrollWidth` does not exceed its `clientWidth`.
  - Component tests that assert "Scheduled" or the old duration field move with the change; no assertion is loosened.
- **Specifications** (pending owner review):
  - `knowledge/design/web/states.md`: the `scheduled` row shows no word in rows and blocks, with the lesson page as the exception, and the principle "the word is always present" names this exception;
  - `knowledge/design/web/pages.md`: Schedule (window height and opening position) and the New lesson dialog (audience, duration, description, date);
  - `docs/` is not changed.

## 5. Acceptance criteria

1. At 1440 × 900, with the shown week's only lesson at 20:30, its block is inside the viewport without scrolling, in Chromium, Firefox and WebKit.
2. With lessons at 09:00 (already past) and at 20:30 later in the week, the view opens with the 20:30 block visible.
3. No calendar block or lesson list row of a `scheduled` lesson outside its join window contains "Scheduled"; the lesson page still shows it.
4. Choosing an audience collapses the picker to the chosen name and a Change button; Change focuses the search field (`toBeFocused`).
5. Choosing 90 creates a lesson of 90 minutes, and Other with 75 creates one of 75 minutes (read back from the calendar).
6. A description refused by the server opens the description field with its error.
7. In Chromium at 1440 × 900 and at 768 px, the date field's `scrollWidth` is at most its `clientWidth`.
8. The existing schedule, lesson and pilot tests pass with no assertion removed or loosened; locators move only where a text moved.
9. `tools/docs/check-docs.sh` and the frontend change checks pass: `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `check:bundle` and the whole `pnpm e2e`.

## 6. Order of work

1. Worktree on `feat/design-step-d4-schedule` from `main`.
2. The week height and opening position, with its test.
3. The status word, with the specifications.
4. The dialog: audience, duration, description, date, with their tests.
5. Change checks, the standard reviewer, fixes under the after-fix rule, the readiness report, the gate, the merge and finish.

## 7. Observability

None. No backend change and no new request.

## 8. Risks

- **The opening position fights the reader.** It runs only on a new range or the first arrival of items, never on a refetch, so a live update does not jump the view.
- **Focus in the dialog.** Collapsing the picker removes the focused radio; focus moves to the Change button, and the dialog's focus tests cover the opening focus on the search field.
- **The status word's absence.** Status is never colour alone: an ordinary lesson shows no colour of status either, only the audience colour, so nothing relies on colour.
- **Slice 7.** The lesson page is not touched; `LessonStatusWord` is not used there.

## 9. As built

The owner approved three changes to this plan on 2026-11-09 while it was built:

- **Audience choices are buttons, not radios.** A radio group changes its choice with the arrow keys, so a picker that collapses on a change would collapse at the first arrow press. Each result is a button; activating it chooses and collapses, and Tab moves through the list.
- **The duration keeps its number field.** The lesson form layout from slice 7 gives the full date its own row, which already ends the date clipping, and aligns the start time with the duration field, a layout `lesson-form-layout.spec.ts` proves. Radios with "Other" would break that pair, so the field stays beside the start time and the buttons "45 min", "60 min" and "90 min" under it put their value into it. Acceptance criterion 7 is therefore met by the existing layout test, and the new dialog test also checks that the date's `scrollWidth` does not exceed its `clientWidth`.
- **The date and start fields keep their rows** for the same reason.

Other notes:

- Calendar blocks never showed "Starting soon"; with "Scheduled" gone, an ordinary lesson inside its join window shows no status word in blocks and lists either, as before for "Starting soon", which stays on the lesson page.
- The description opens itself on a server refusal, but no refusal can reach a collapsed field, because a collapsed field sends no description; acceptance criterion 6 is replaced by the test that "Add description" opens the field with focus and that the description is saved.
- The opening position is proved by unit tests of `openingHour` in `WeekGrid.test.tsx` (next lesson, all ended, empty week, a lesson starting before the week), because component tests run without the stylesheet and cannot scroll, and by the end-to-end test at 1440 × 900 with the week's only lesson at 20:30. An end-to-end test of a past lesson would need the browser clock ahead of the server clock, which breaks the session.
- The existing test "next week keeps the shown week and its scroll position while the next week loads" now holds the next week's calendar request, checks the position while it is held, and then checks that the week opened at its lesson.
- A scheduled 15-minute block shows only its time; the 24 px check of a status icon moved to a cancelled 15-minute lesson.
- Change clears the chosen audience, so a submit from the search before a new choice asks for one instead of scheduling the previous audience; a test proves it.
- The choice buttons are named by the name alone, with "Email not confirmed" as their description.
