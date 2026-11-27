# Homework library and builder: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R7a `feat/redesign-homework-library` and R7b `feat/redesign-homework-builder` of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

Two screens of the tutor's Homework section:

- **Library** (R7a): the page at `/homework/library` with the tabs Library, Issued and Needs checking, its creation actions, folder tree, search and item table ([pages: Library](../design/web/pages.md#library), [navigation](../design/web/navigation.md)). Today the header holds two actions, "New task" (the one-page task at `/homework/new-task`, [the one-page task](../specs/homework.md#the-one-page-task)) and "New template" (a blank template opened in the builder), plus a menu with "Upload material" and "New folder". Rows show a template's question count, maximum points, "Ready" or "Incomplete" and last edit, with a visible "Issue" or "Open" and a row menu. Code: `frontend/src/features/library/` (`LibraryPage.tsx`, `LibraryTable.tsx`, `LibraryDialogs.tsx`).
- **Builder** (R7b): `/homework/templates/{id}` ([pages: Builder](../design/web/pages.md#builder)). Today: breadcrumbs, a title field under a generic heading, autosave status, readiness or problem count, Preview and, when ready, "Issue assignment"; a vertical list of blocks with drag handle plus Move up, Move down, Duplicate and Delete buttons; "Add block" opens a menu of eight plain-word types; Preview renders the student view in a phone or computer frame. Code: `frontend/src/features/builder/` (`BuilderPage.tsx`, `BlockCard.tsx`, `PreviewDialog.tsx`, `autosave.ts`).
- **Block types** ([assignment blocks](../specs/homework.md#assignment-blocks)): `instruction`, `material`, `single_choice`, `multiple_choice`, `short_text`, `short_number` (checked automatically), `long_text` and `file` (checked by the tutor).
- **Visibility** ([answer visibility](../specs/homework.md#answer-visibility), [ADR-0009](../../docs/decisions/0009-owner-approved-exceptions-and-visibility.md)): keys and explanations reach a student only under the reveal rule of the chosen policy; points, totals and scores never reach a student, and the builder preview never shows a key, explanation or points.

Owner's decisions that bind this study (plan section 3): item 15 (Create choices are presets into the existing mixed-question flow; every builder type is retained), item 16 (destructive actions in confirmed menus, one primary action, internal IDs hidden), item 6 (no underlined action links), item 1 (mockups accepted as drawn, specifications and explicit decisions taking precedence). The benchmark audit at commit `512765d` (`knowledge/research/ui-benchmark-audit-2026-11-27.md`, rows 9 and 10) adds the owner's choice of one "Create" button with "Task with questions" and "Simple task" and the section name "Tasks" ("Задания") everywhere.

Mockups at `8b3a8b0`: `knowledge/design/web/prototype/redesign/tasks.html` (heading "Library" with subtitle, one "Create" menu whose two entries carry a one-line hint each, tabs Library / Issued / Needs checking with a count, an "All folders" filter, rows with title, kind, question count, last used date, folder and a visible "Issue") and `knowledge/design/web/prototype/redesign/builder.html` (breadcrumb "Library / folder", "Saved", the task title as heading with kind and folder under it, Preview and Issue in the header, a numbered block list with "Order can be changed", a block editor with options, a "Correct" mark, points and explanation, and a right-hand "Add block" panel grouped as Materials, Checked automatically and Checked by you, each type with a one-line hint).

## How the compared products solve it

### Google Forms (quiz builder)

- A form becomes a quiz through Settings, "Make this a quiz"; the key is set per question through "Answer key" at the question's bottom left, the point value at its top right, and "Add answer feedback" adds an explanation [1].
- Answer keys exist for short answer, multiple choice, checkboxes, dropdown and the two grid types; points and feedback can be set on all types [1].
- Respondent settings choose whether respondents see missed questions, correct answers and point values; "Release grades" is "Immediately after each submission" (the default) or "Later, after manual review" [1].
- The editor has a floating set of buttons with "Add question", "Add section" and "Add title and description"; Ctrl+Shift+Enter inserts a question after the focused item [2]. The type is chosen from a drop-down at the right of the question title [2, 3].
- Twelve types are listed with a one-line description each: Short answer, Paragraph, Multiple choice, Checkboxes, Dropdown, File upload, Linear scale, Rating, two grids, Date, Time [3].
- Reorder by keyboard: Ctrl+Shift+K up and Ctrl+Shift+J down, with no effect at the ends; Ctrl+Shift+D duplicates; each question has action buttons for delete and required [2].
- Preview opens the form in a new tab (Ctrl+Shift+P) [2]. The creation guide did not confirm editor autosave or drag reorder [4].

### Google Classroom (Classwork and Create menu)

- The Classwork page has one "Create" button at the top; its documented entries include Assignment, Quiz assignment, Question and Material [5, 6]; the screen reader guide opens it with Enter and names Assignment, Quiz assignment and Question [6].
- An Assignment has title and instructions first, then For (classes, up to 100 students at a time, or groups), Points (default 100, or "Ungraded"), an optional Due, one Topic, and Attach; posting is "Assign", with "Schedule" and "Save draft" under the arrow beside it [5].
- "Quiz assignment" is a preset: Classroom creates a blank Google Form, attaches it, and the teacher builds the questions in Forms; question points default to zero [7].
- Students see their grade at once only if the teacher enables it, otherwise after the teacher selects them and chooses "Return" [7].
- The To review page keeps a "Reviewed" list into which checked work moves [8].

### Microsoft Forms (quiz)

- "New Quiz" is a separate entry from "New Form"; "Add new" or "Quick start with" offers four primary types, Choice, Text, Rating and Date, with Ranking, Likert, File upload and Net Promoter Score under "More question types"; "Section" groups questions [9].
- A Choice question has "Correct Answer" check marks, a "Points" box, a per-answer "Message" for feedback, "Multiple answers", "Required", "Add option" and a trash icon per option [9].
- Each question has a "Copy question" icon and Up and Down arrows on its right for reordering [9].
- "Preview" shows the quiz as on a computer or a mobile device, where the author can answer and submit, then "Back" to keep editing; the quiz "is saved automatically" while it is created [9].
- "Show results automatically" lets students see correctness right after submitting and must be on for them to see per-question points [10].

### Moodle (quiz editing)

- The quiz editing page's "Add" offers "a new question", "from question bank" and "a random question"; a new question opens a type chooser where selecting a type on the left shows its description on the right, then "Next" [11].
- Questions reorder by drag with a move icon; page breaks and section headings can be inserted between questions; a "Preview" button previews the whole quiz and a magnifier one question [11].
- Each question has a points box beside it, with a default of one point and a maximum grade of 100.00 at the top right [11].
- Standard types include Description, which is "not actually a question" and only displays content, alongside Essay, Matching, Multiple choice, Short answer, Numerical, True/False and others; administrators can reorder the chooser so common types come first [12].
- Review options are a grid of what a student sees (the attempt, whether correct, marks, feedback, right answer) per timing; teachers with "View hidden grades" are unaffected [13].

## Comparison

| Design question | Google Forms | Google Classroom | Microsoft Forms | Moodle | OVC decision/target |
|---|---|---|---|---|---|
| How creation starts | Blank form, then "Make this a quiz" [1, 4] | One "Create" menu with typed entries [5, 6] | "New Quiz" separate from "New Form" [9] | "Add" menu: new, from bank, random [11] | One "Create" menu: "Task with questions", "Simple task", each a preset of the one mixed-question flow (item 15) |
| Quiz as preset of a general object | Quiz is a setting of a form [1] | Quiz assignment = assignment with an attached blank form [7] | Quiz is a form type [9] | Quiz is its own activity [11] | Presets fill the same template and one-page flow; no new type |
| Where types are chosen | Drop-down beside the question title [2, 3] | Not applicable (Forms) [7] | Four primary types plus "More question types" [9] | Chooser with description pane [11] | Add-block panel grouped Materials / Checked automatically / Checked by you, with a hint per type (mockup) |
| Type descriptions | One line per type in help, not confirmed in the menu [3] | — | Labels only, not confirmed [9] | Description beside the chosen type [11] | One-line hint per type, as drawn |
| Content-only blocks | Title and description, sections [2] | Material post [5] | Section [9] | Description, section heading [11, 12] | Instruction and material blocks retained |
| Reorder | Ctrl+Shift+K / J [2] | — | Up and Down arrows [9] | Drag with move icon [11] | Keep drag plus Move up / Move down buttons; shortcut is an open point |
| Duplicate / delete | Ctrl+Shift+D; delete button [2] | — | Copy icon; trash per option [9] | Not documented here | Keep Duplicate and Delete; Delete confirmed only if the task plan says so (item 16) |
| Preview | New tab [2] | — | Computer or mobile, can submit [9] | Whole quiz or one question [11] | Dialog with Phone / Computer frames, student schema only |
| Save | Not confirmed [4] | "Save draft" explicit [5] | Automatic [9] | Explicit save per question [11] | Autosave after 1 s (unchanged) |
| Points shown to student | Configurable [1] | After Return or at once [7] | With "Show results automatically" [10] | Review options grid [13] | Never (ADR-0009) |
| Key revealed to student | Configurable [1] | Via Forms settings [7] | With "Show results automatically" [10] | Per timing [13] | Only by the reveal rule; never in preview |
| Library tabs / review list | — | To review and Reviewed lists [8] | — | Question bank [11] | Library / Issued / Needs checking, tab names equal headings |

## What OVC adopts

- **One "Create" menu of presets** → Google Classroom's Create menu [5, 6] and its "Quiz assignment" preset over a general object [7]; Microsoft Forms' separate "New Quiz" entry shows the cost of making the quiz a second object, which OVC avoids [9]. Maps to item 15 and R7a's criterion "one Create menu opens presets of the mixed-question flow".
- **A hint under each Create entry and each block type** → Moodle's description beside the selected type [11] and Google Forms' one-line type descriptions [3]; matches both mockups and audit row 9.
- **Grouped add-block panel** → Microsoft Forms' primary versus "More" types [9] and Moodle's ordering of common types first [12], recast as OVC's existing distinction of content, automatic and tutor-checked blocks ([assignment blocks](../specs/homework.md#assignment-blocks)). R7b's criterion "all existing block types remain available" applies to the panel.
- **Title as the page heading** → Google Forms, where the form title is the first edit field of the form [2]; audit row 9.
- **Keyboard reorder and duplicate parity with pointer actions** → Google Forms' shortcuts [2] and Microsoft Forms' visible Up and Down arrows [9]; OVC keeps visible Move up / Move down buttons, which already satisfy WCAG 2.5.7 without a shortcut.
- **Preview in device frames** → Microsoft Forms' computer and mobile preview [9]; OVC keeps its dialog and student-view components.
- **Autosave with a visible state** → Microsoft Forms' automatic saving [9]; OVC keeps its existing contract.
- **Content-only blocks next to questions** → Moodle's Description type [12] and Forms' title and description items [2]; OVC keeps `instruction` and `material`.

## Where OVC differs and why

- **Points and scores never reach students** ([ADR-0009](../../docs/decisions/0009-owner-approved-exceptions-and-visibility.md)). Google Forms [1], Microsoft Forms [10], Classroom [7] and Moodle [13] all offer a setting that shows points or grades to students; OVC offers none, and its preview shows no key, explanation or points. Points stay visible only in the tutor's builder and library.
- **No per-answer verdict for students**: Moodle's "Whether correct" [13] and Microsoft Forms' correctness display [10] have no OVC equivalent; the student schema has no correctness field.
- **Presets, not object types**: unlike Microsoft Forms' "New Quiz" [9] or Moodle's quiz activity [11], both Create entries produce the same template through the same flow (item 15).
- **Types the mockup draws but OVC lacks**: `builder.html` lists "Fill in the gaps" and "Matching" and omits `short_number` and `file`. Item 1 places specifications above drawn details and item 15 retains every existing type; the panel lists exactly the eight specified types, and new types would need a specification change outside R7b.
- **No question bank, random questions, branching, sections, shuffle or grids** as in Moodle [11] and Forms [2, 3]; they are not specified for OVC.
- **Scope**: assignments are issued only to students and groups in the tutor's own relationships ([ADR-0005](../../docs/decisions/0005-tutor-centric-domain.md)); no class-wide or cross-tutor posting as in Classroom's For field [5].
- **No third-party services**: Classroom's Drive, YouTube and Gemini attachments [5] have no counterpart; materials are uploads into OVC's own storage (ADR-0008).
- **Delete** stays in a confirmed menu (item 16) rather than the one-click trash icons of Microsoft Forms [9].

## Open points for the task plan

- What "the existing mixed-question flow" means for each preset: does "Task with questions" open `/homework/new-task` with an empty question list and "Simple task" open it with one tutor-checked question (`long_text` or `file`) prefilled, as the mockup hint "Instruction, materials and student's answer" suggests?
- Where a blank builder template ("New template" today), "Upload material" and "New folder" go once the header has one Create menu: inside the same menu after a separator, or in the existing secondary menu.
- Whether the library rows keep maximum points and "Ready" / "Incomplete" (specified) or show the mockup's kind, last used date and folder columns, and how "All folders" relates to the specified folder tree and breadcrumbs.
- Whether "Library" stays the first tab's name and heading while the section is "Tasks", and the exact Russian and English texts for the tabs, Create entries and hints, so that tab names equal headings (audit row 10).
- Whether the block list becomes a separate navigation column (mockup) or stays one vertical list, and the focus target after selecting a block from it.
- Whether validation problems stay visible on a fresh draft (specified) or appear only after an Issue attempt (audit row 9), given that readiness and issuing come from the server.
- Whether a keyboard shortcut for reorder is added next to the visible buttons, and which one, avoiding browser and screen-reader conflicts.
- Whether the add-block panel is a persistent side column (mockup) or a menu, and how it behaves below 1280 px with the sidebar rail.
- The mockup `builder.html` at `8b3a8b0` draws "Fill in the gaps" and "Matching" blocks, which OVC does not specify, and omits `short_number` and `file`; the plan must confirm that the add-block panel lists exactly the specified types (item 15) and that any new type is a separate homework specification change.

## Unverified

- The complete current Classroom Create menu (Reuse post and Topic entries); only a third-party listing was found in search results, not opened.
- Whether Google Forms' question type menu itself shows icons or descriptions, and whether its editor saves automatically.
- Whether the Moodle type chooser groups types into named categories.
- Edpuzzle and Quizlet behaviors; not researched.
- Edvibe's categorised exercise templates cited by the benchmark audit; not rechecked today.

## Sources

1. Google Forms — Make a quiz with Google Forms — <https://support.google.com/docs/answer/7032287> — FP — checked 2026-11-27 — respondent-setting toggle names not listed on the page.
2. Google Forms — Use a screen reader with Google Forms (editing) — <https://support.google.com/docs/answer/3527854> — FP — checked 2026-11-27 — drag reorder and autosave not mentioned.
3. Google Forms — Choose a type of question for your form — <https://support.google.com/docs/answer/7322334> — FP — checked 2026-11-27.
4. Google Forms — Create your first form in Google Forms — <https://support.google.com/docs/answer/6281888> — FP — checked 2026-11-27 — did not confirm question adding, preview or autosave.
5. Google Classroom — Create an assignment — <https://support.google.com/edu/classroom/answer/6020265> — FP — checked 2026-11-27 — documents Assignment and Material; other entries only named in links.
6. Google Classroom — Use a screen reader with Classroom on your computer (for teachers) — <https://support.google.com/edu/classroom/answer/9849192> — FP — checked 2026-11-27 — names Assignment, Quiz assignment and Question.
7. Google Classroom — Create a quiz assignment — <https://support.google.com/edu/classroom/answer/9095575> — FP — checked 2026-11-27.
8. Google Classroom — To-do and review lists — <https://support.google.com/edu/classroom/answer/9157286> — FP — checked 2026-11-27 — counts and filters not described.
9. Microsoft Forms — Create a quiz with Microsoft Forms — <https://support.microsoft.com/en-us/office/create-a-quiz-with-microsoft-forms-a082a018-24a1-48c1-b176-4b3616cdc83d> — FP — checked 2026-11-27.
10. Microsoft Forms — Display answers and points on quiz questions — <https://support.microsoft.com/en-us/education/display-answers-and-points-on-quiz-questions> — FP — checked 2026-11-27 — default state not stated.
11. Moodle — Building Quiz — <https://docs.moodle.org/en/Building_Quiz> — FP — checked 2026-11-27 — chooser grouping not described.
12. Moodle — Question types — <https://docs.moodle.org/en/Question_types> — FP — checked 2026-11-27.
13. Moodle — Quiz settings (review options) — <https://docs.moodle.org/en/Quiz_settings> — FP — checked 2026-11-27.
