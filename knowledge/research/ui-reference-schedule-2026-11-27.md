# Schedule: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R5 `feat/redesign-schedule` (possibly split into R5a, week grid, toolbar, live marker and List mode, and R5b, the quick-create popover) of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

The tutor's Schedule page and the step from a free slot into lesson creation:

- the toolbar (Today, previous and next, Week, Month and a new List view) and the period heading;
- the week grid's typography, block content and a current-time marker;
- the marker of a lesson that is live now;
- List (agenda) mode on desktop and the agenda at 800 and 360 px;
- a quick-create popover on a free slot (audience, title, duration, "More options" carrying the values into the full New lesson page);
- keyboard access and focus when the popover opens and closes.

What OVC has today ([pages](../design/web/pages.md#schedule), [schedule specification](../specs/schedule.md), code in `frontend/src/features/schedule/`: `SchedulePage.tsx`, `WeekGrid.tsx`, `MonthGrid.tsx`, `Agenda.tsx`, `NewLessonPage.tsx`, `AudiencePicker.tsx`, `newLesson.ts`):

- Toolbar: Week, Month, Today, previous and next; "New lesson" is the page header's main action. Previous and next keep focus; a loading period stays dimmed with `aria-busy`.
- Week: a time grid in the profile timezone, shown only from 1024 px with a fine pointer; it opens one hour before the first lesson that has not ended, or at 08:00. Blocks are at least 24 px high, lanes at least 64 px wide; a lesson shorter than 45 minutes shows only its start time and status icon, with the full time, title, audience and status in the accessible name and tooltip. The benchmark audit (`knowledge/research/ui-benchmark-audit-2026-11-27.md`, commit `512765d`, top 15 item 5) measured about 48 px per hour, no current-time line, two-line day headers and blocks that all read "Урок".
- Status: a block shows the word of its status from [states](../design/web/states.md#lessons) ("Live now" / "Идёт сейчас" for `live`), none for `scheduled`; the calendar derives no state from the clock.
- Free slots: clicking one opens the full New lesson page with date and time prefilled; free slots are kept out of the Tab order and the accessibility tree, and leaving the page returns focus to "New lesson" (decided in change U2 of the interface rework stage).
- New lesson page: audience picker with server search after 300 ms, title prefilled "Lesson" / "Урок" (plus the group's name for a group lesson), never the card's display name, with the hint "Students see the title"; date, start, duration with 45/60/90 min buttons; "Repeat weekly"; description; materials; error summary; overlap warning with "Schedule anyway". The prefill lives in page memory, not in the address (except `date`).
- Below 1024 px and on coarse pointers the week lists its lessons day by day; the phone agenda lists only days with lessons and "No lessons in this period" when empty.

Owner's decisions that bind this study (plan section 3 at `8b3a8b0`):

- Item 12: add List next to Week and Month and a quick-create popover on a free slot with audience, title and duration, and "More options" carrying the values to the existing New lesson page; this amends U2's slot-click decision; New lesson remains the full form; R5 tests full validation, server authority, focus and refusal behavior.
- Item 10: sidebar defaults 232/340 px, automatic icon rail at 1024–1279 px, existing interim and phone frames below 1024 px, tutor phone bottom bar kept until the PHONE stage. The grid must therefore work in the width left by a 64 px rail at 1024 px and by a 232 px sidebar from 1280 px.
- Item 16: one primary action per view, sentence-case status text.
- The audit's owner's choices: the default lesson title stays "Урок"; grid legibility improves without changing it (showing the audience's name on the block is one candidate, not a decision).

Mockup `knowledge/design/web/prototype/redesign/schedule.html` at `8b3a8b0` (static, non-normative): a 1280 × 800 board with a 232 px sidebar; toolbar row "Сегодня", previous and next icon buttons, period heading "23–29 ноября 2026", and a segmented group "Неделя / Месяц / Список"; timezone "Москва · UTC+3" in the header; one-line day headers ("пт 27", today in accent); 84 px per hour; blocks with the audience name on the first line and the time on the second, a compact one-line "name · 17:00" block, a struck-through cancelled block, a live block with "Идёт сейчас" and a "Войти" button; a red now-line with the time "18:26" in the hours column and a dot on today's column; a dashed slot selection "20:00–21:00" and a 324 px popover "Новый урок" with the slot's date and time, a close button, "Ученик или группа", "Название" (drawn prefilled with the student's name), "Длительность" (60 or 45 minutes), "Ещё параметры" and "Создать"; a hint "Нажмите на свободное время, чтобы назначить урок." The drawn title prefill conflicts with the owner's choice and with the label rules of the schedule specification, so it is not adopted.

## How the compared products solve it

### Google Calendar (web, with the mobile app for the agenda)

- Creating: clicking an empty time slot opens a pop-up box where the title is entered and "Save" commits [1]. Since 2020 the pop-up also edits guests, description, attachments, colour, notifications and visibility "without having to click into 'More options'", which keeps the full-page path behind that button [7].
- Keyboard: `c` creates an event, and with a screen reader "A page opens to create a regular event" with initial focus on the title field; Escape leaves an unsaved event [2, 5]. The keyboard path therefore reaches the full page, while the pointer path reaches the pop-up.
- Navigation and views: views are chosen in the top right (Day, Week, Month, Year, Schedule, 4 days), the chosen view becomes the default until changed, and "Today" sits top left [3]. Shortcuts: `t` today, `j`/`n` next range, `g` go to date, `1`/`d` day, `2`/`w` week, `3`/`m` month, `5`/`a` agenda (Schedule), `4`/`x` custom, `e` event details, `Esc` back to the grid, `z` undo, `?` the shortcut list [2, 8]. The previous-range key is not listed on the page read [2].
- Agenda: Google recommends "schedule view, also called agenda view" for screen-reader use [5]. On Android the views are Month and Schedule, and Schedule lists "all your events or tasks by day" [4].
- Series scope wording: "This event", "This and following events", "All events" (documented for deletion) [6].

### Microsoft Outlook (web, new Outlook for Windows and mobile)

- Creating: in Outlook on the web the event is created by clicking the calendar at the day and time; "More details" (or the view-event icon) expands to the full form, and the reminder defaults to 15 minutes [9].
- Keyboard (Outlook on the web): Alt+Shift+Y today, Shift+Left and Shift+Right previous and next period, Shift+Alt+1–4 Day, Work week, Week and Month, `N` new item, Shift+? the shortcut list [10]. New Outlook for Windows uses Ctrl+Alt combinations and Ctrl+N [10].
- Views: new Outlook offers Day, Work Week, Week and Month; "List view" and Schedule view are "not available in new Outlook at this time" [11]. Outlook mobile offers Agenda, Day, 3-Day and Month through a view switcher at the top right [12]. The desktop product thus dropped its list view while the phone app keeps an agenda as a primary view.

### Preply (tutor calendar)

- Calendar: weeks are navigated with arrows at the top left, the day is scrolled vertically, and "the red line" marks the current time [13].
- Scheduling: the tutor clicks an open time slot (or "Schedule new lesson"), confirms or adjusts date and time, chooses the student, chooses the duration (options depend on the student's remaining lessons) and clicks "Schedule lesson"; only students with lessons available appear, and slots inside time off cannot be used [14]. The student is notified and the lesson appears in both schedules [14].
- Rescheduling: click the lesson or drag it to another day or time; for a weekly lesson a popup asks "Single" or "Weekly" [15].
- App: tutors schedule from a "Schedule tab" by choosing date, time and duration; tapping a lesson opens a pop-up; the article notes that only a limited set of functions is available in the app [16].

### Teachworks (tutoring-business calendar, comparator)

- More than six views, including month, week agenda, daily agenda, teacher and location views; lessons colour-coded by teacher, student or location; filters by student, teacher and location [17].
- Clicking a time slot starts a new lesson; dragging moves a lesson, resizing changes its duration; clicking a lesson opens a menu with View, Edit, Delete, Complete, Remind and Copy [17]. The help-centre article on adding lessons did not load (HTTP 405), so whether the slot click opens a pop-up or the full form is unverified.

## Comparison

| Design question | Google Calendar | Outlook | Preply | Teachworks | OVC decision/target |
|---|---|---|---|---|---|
| Slot click | Pop-up box, title then Save [1] | Click creates; "More details" for full form [9] | Scheduling flow with date, student, duration [14] | Starts a new lesson [17] | Popover: audience, title, duration (item 12) |
| Path to full form | "More options" [7]; `c` opens the full page [5] | "More details" [9] | Same flow is the full form [14] | Unverified | "More options" carries values to New lesson; header "New lesson" stays |
| Initial focus | Title field [5] | Not documented | Not documented | Not documented | Open point: audience picker vs title |
| Required who-field | Guests optional [7] | Attendees optional [9] | Student required, only eligible ones listed [14] | Not documented | Audience required; server-scoped search |
| Views | Day, Week, Month, Year, Schedule, 4 days [3] | Day, Work Week, Week, Month; no List in new Outlook [11]; Agenda on mobile [12] | Week with arrows [13] | 6+ incl. week and daily agenda [17] | Week, Month, List |
| Remembered view | Chosen view becomes default [3] | Not documented | — | Not documented | Week/Month/List and date survive reflow (R5 criteria) |
| Current time | Not documented on pages read | Not documented | Red line [13] | Not documented | Now-line in the live colour (mockup) |
| Agenda for accessibility | Recommended for screen readers [5] | — | — | — | List gives an accessible alternative to the grid |
| Shortcuts | `t`, `j`/`n`, `w`, `m`, `a`, `c`, `?` [2] | Alt+Shift+Y, Shift+arrows, `N`, Shift+? [10] | Not documented | Not documented | Open point (audit suggests `t`, arrows, `w`, `m`, `?`) |
| Series question | This / following / all [6] | Not read | Single / Weekly [15] | Not documented | Existing "This lesson / This and following lessons" |
| Phone | Month and Schedule on Android [4] | Agenda, Day, 3-Day, Month [12] | Schedule tab, limited [16] | Not documented | Agenda at 800/360 px; phone design deferred to PHONE |

## What OVC adopts

- Popover on a free slot with a small, fixed field set and a link-out to the full form → Google Calendar's pop-up and "More options" [1, 7], Outlook's "More details" [9] → item 12; acceptance: a free slot opens the popover, More options transfers audience, title, duration, date and start into New lesson.
- A required, server-scoped "who" choice in the quick flow, listing only audiences that can receive lessons → Preply's student step [14] → reuse the existing `AudiencePicker` search rather than the mockup's static select; the server still refuses out-of-scope (`404`) and inactive (`409 audience_not_active`) audiences, shown inside the popover (T-5/T-6).
- Two creation paths: pointer through the popover, keyboard through the full page → Google's `c` opening a page with focus on the title [5] → keep header "New lesson" as the keyboard and screen-reader path, which also keeps free slots out of the accessibility tree as U2 decided.
- List view as the accessible linear alternative to the grid → Google's Schedule view, recommended for screen readers [5], and Outlook mobile's Agenda [12] → item 12 List; reuse `Agenda.tsx` so desktop List and the 800/360 px agenda share one component and one empty sentence.
- A current-time line in a distinct colour → Preply's red line [13] and the mockup's now-line → use the selected palette's `live` token, label the time in the hours column, keep it decorative (`aria-hidden`), since the grid stays clock-free for status.
- A view switch that the product remembers → Google [3] → item 12 and the R5 criterion that Week/Month/List and the date survive reflow; the remembered view is a per-device preference, not server state.
- One-line day headers and block text that names who the lesson is with → mockup and audit item 5; the block may show the tutor-side audience label beside the title "Урок" without changing the default title, since the label table already lets the tutor see the card's name.

## Where OVC differs and why

- Default title stays "Урок": the owner's choice; the specification's label rules keep the card's display name out of the student-visible title (T-34). The mockup's name-prefilled title is not adopted; legibility comes from layout and from showing the audience label on the tutor's block.
- No drag to create or move: not in the owner's item 12; moves keep the lesson page's edit dialog with its series scope question, unlike Preply [15] and Teachworks [17].
- No guests, colour or notification fields in the popover, unlike Google [7]: the popover holds exactly audience, title and duration; repeat, description and materials stay on New lesson.
- No time-off, availability or lesson-balance filtering of the audience as in Preply [14]: OVC has no booking or balance yet (finance and booking stages come later); eligibility is the relationship and group state the server checks.
- The server stays authoritative: the popover makes no permission or timezone decision; creation goes through `POST /lessons` with its full refusal order, including `lesson_overlap`, `lesson_in_past`, caps and `429`.
- Single-letter shortcuts, if any, must satisfy WCAG 2.2 success criterion 2.1.4 (turn off, remap or active only on focus); Google gates them behind a setting [2].
- No third-party calendar sync, unlike Preply's Google Calendar connection [13] (ADR-0008, no services outside Russia receiving user data).
- The phone layout is not redesigned here: the owner deferred phone design to the PHONE stage, so 800/360 px keep an accessible agenda only.

## Open points for the task plan

- Initial focus in the popover: the audience search (required, first field) or the title (Google [5]); and whether the popover is a non-modal dialog or a modal one with a focus trap.
- Focus on Escape, outside click and the close button: free slots are not focusable, so the plan must name the target (the "New lesson" header button, the toolbar, or the nearest lesson block) and the target after a successful creation (the new lesson's block with a polite "Lesson scheduled").
- How a `409 lesson_overlap` answer behaves in the popover: show the overlap list and "Schedule anyway" inside it, or hand over to New lesson with the warning.
- Whether the title field in the popover prefills "Урок" (plus the group's name) exactly as New lesson does, and how More options carries an edited title through the page-memory prefill without putting names in the address.
- The duration control: a select (mockup) or the existing 45/60/90 buttons with a free field; the default duration of a slot click.
- Hour height and minimum block content at 1024 px with the rail and at 1280 px with the 232 px sidebar; whether the "shorter than 45 minutes shows only time and icon" rule changes to "name · time" on one line.
- The live block: whether it carries a Join button in the grid (mockup) given ADR-0010's one focal action per view and the 24 px block minimum; Join's accessible name must include the lesson.
- Where the now-line sits in Month and List (a "now" divider in List or none), and whether the week opens scrolled to now when the shown week contains today, replacing or extending the current opening rule.
- Keyboard shortcuts: adopt none, or a set such as `t`, arrows, `w`, `m`, `l`, `?`, with a way to turn them off (WCAG 2.2 SC 2.1.4).
- Split decision: R5a (toolbar, grid, now-line, live marker, List) and R5b (popover, security trigger met), and which tests each owns (cross-tutor audience refusal, live access change while the popover is open).
- The mockup `schedule.html` at `8b3a8b0` prefills the popover's title with the student's name; this diverges from the owner's choice to keep the default title "Урок" and from the label rules behind T-34, so the plan must state that the popover's title default is "Урок" and where, if anywhere, the audience label appears instead.

## Unverified

- Google Calendar scrolling the week to the current time on open, and whether its web week view draws a current-time line: no first-party page read today documents either.
- Google Calendar's previous-range shortcut (`k`/`p`): not on the page read [2].
- Google Calendar's default event duration and the exact fields of the slot pop-up today: the help page names only title and Save [1]; the field list comes from a 2020 announcement [7].
- Outlook on the web's "Quick compose" name and its fields when a slot is selected: seen only in a search snippet of "Schedule events in Outlook on the web", which returned HTTP 404 when fetched.
- Outlook on the web's current List or Agenda view: the page read covers new Outlook for Windows [11].
- Preply's calendar views other than week, and the layout of its slot dialog (pop-up or page) [13, 14].
- Teachworks: whether a slot click opens a pop-up or the full Add Lesson form; the help-centre articles returned HTTP 405 and the tutorial page held no steps.
- TutorBird: no first-party help page found; not compared.
- Google Calendar drag to move or extend events: no first-party page read today confirms it.

## Sources

1. Google Calendar — Create an event — <https://support.google.com/calendar/answer/72143> — FP — checked 2026-11-27 — confirms slot click, title, Save, Shift+C; does not name "More options" or a default duration.
2. Google Calendar — Keyboard shortcuts for Google Calendar — <https://support.google.com/calendar/answer/37034> — FP — checked 2026-11-27 — previous-range key not listed.
3. Google Calendar — Change your calendar view — <https://support.google.com/calendar/answer/37189> — FP — checked 2026-11-27.
4. Google Calendar — Android views (Month, Schedule) — <https://support.google.com/calendar/answer/6110849> — FP — checked 2026-11-27 — no current-time or past-event details.
5. Google Calendar — Use Google Calendar with a screen reader — <https://support.google.com/calendar/answer/6101541?hl=en> — FP — checked 2026-11-27.
6. Google Calendar — Delete recurring events — <https://support.google.com/calendar/answer/37113> — FP — checked 2026-11-27 — scope wording for deletion only.
7. Google Workspace Updates — Improved Google Calendar event creation (June 2020) — <https://workspaceupdates.googleblog.com/2020/06/improved-google-calendar-event-creation.html> — FP — checked 2026-11-27 — dated announcement; rooms and conferencing not mentioned.
8. Google Calendar — Change Google Calendar settings — <https://support.google.com/calendar/answer/6084644> — FP — checked 2026-11-27 — confirms `?` shortcut list only.
9. Microsoft Outlook — Create, modify, or delete a meeting request or appointment in Outlook on the web — <https://support.microsoft.com/en-us/outlook/create-modify-or-delete-a-meeting-request-or-appointment-in-outlook-on-the-web> — FP — checked 2026-11-27 — no "Quick compose" or double-click wording.
10. Microsoft Outlook — Keyboard shortcuts for Outlook — <https://support.microsoft.com/en-us/office/keyboard-shortcuts-for-outlook-3cdeb221-7ae5-4c1d-8c1d-9e63216c1efd> — FP — checked 2026-11-27.
11. Microsoft Outlook — Change how you view your Outlook calendar — <https://support.microsoft.com/en-us/office/change-how-you-view-your-calendar-a4e0dfd2-89a1-4770-9197-a3e786f4cd8f> — FP — checked 2026-11-27 — covers new and classic Outlook for Windows, not the web.
12. Microsoft Outlook — How do I switch to Month view? (Outlook for Android and iOS) — <https://support.microsoft.com/en-US/Outlook/how-do-i-switch-to-month-view> — FP — checked 2026-11-27.
13. Preply — How to use Preply calendar and connect Google Calendar — <https://help.preply.com/en/articles/4175340-how-to-use-preply-calendar-and-connect-google-calendar> — FP — checked 2026-11-27.
14. Preply — How to schedule a single lesson as a tutor — <https://help.preply.com/en/articles/4178923-how-to-schedule-a-single-lesson-as-a-tutor> — FP — checked 2026-11-27 — reached through a redirect from the older article address.
15. Preply — How to reschedule a lesson as a tutor — <https://help.preply.com/en/articles/16554772-how-to-reschedule-a-lesson-as-a-tutor> — FP — checked 2026-11-27.
16. Preply — How to manage your calendar in Preply App — <https://help.preply.com/en/articles/6080094-how-to-manage-your-calendar-in-preply-app> — FP — checked 2026-11-27 — no view or time-marker details.
17. Teachworks — Calendar and scheduling features — <https://www.teachworks.com/features/calendar> — FP — checked 2026-11-27 — marketing page; help-centre articles failed to load (HTTP 405).
