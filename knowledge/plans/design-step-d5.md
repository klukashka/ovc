# Design slice step D5: the student card and the group page

Status: task plan approved by the owner on 2026-11-09, with the members in the group page's main column and autosave 1 second after typing stops and on leaving the field. On approval it is committed as `knowledge/plans/design-step-d5.md` in this step's change. It carries out step D5 of the [interface decisions](ui-ux-decisions-2026-11-06.md#order-of-work) and the [design slice plan](design-slice.md). Base: `main` after step D4 merges. Open slice 7 branches at writing: none that touch `features/students/` or `features/groups/`.

## 1. Problem and outcome

**Problem.** The student card and the group page are long single columns of always-open forms:

- the card stacks the summary, the invitation, a name and email form with Save, a notes form with Save and the ending button;
- the group page stacks the members, a name and colour form with Save, the next lessons and the archive section;
- at 1440 × 900 the card's notes sit below the fold, and rare or dangerous actions are full-width sections.

**Outcome.** Both pages follow the layout standard of step L: a page-width header with the name and actions in one row, and two columns from 1280 px, stacked below that as `PageColumns` already does.

- **Student card:**
  - **Header:** avatar and name, with the email, state badge and group dots in one line under it. On the right: "Schedule a lesson" as the primary action, where offered today, and a menu "Card actions" with "Name and email…", which opens a dialog with today's fields, and the state's dangerous action ("Withdraw invitation", "End working together"), which opens today's confirmation.
  - **Main column:** the invitation block for invited and ended cards, as today, then the Overview and Lessons tabs, as today.
  - **Side column:** "Notes", marked "Only you see this", with autosave and no Save button.
- **Group page:**
  - **Header:** colour dot and name, "Archived" when archived. On the right: "Schedule a lesson" as the primary action for an unarchived group, and a menu "Group actions" with "Name and colour…", which opens a dialog with today's fields and swatches, and "Archive group" or "Unarchive group", with today's confirmation rules.
  - **Main column:** the members with Add and Remove, as today.
  - **Side column:** the next lessons with "Open the schedule".
- **Autosave of notes:**
  - It saves 1 second after typing stops and when the field loses focus.
  - A quiet status under the field says "Saving…", then "Saved", in a polite live region that never takes focus.
  - A failed save keeps the text and shows "Notes were not saved" with Retry.
  - It sends `{ "notes": … }` alone, never the name or email.

## 2. Verification tier and security trigger

Standard tier. The security trigger is **not met**, on the conditions the [design slice plan](design-slice.md) names for D5, which this plan keeps:

- autosave sends `notes` alone, never a stale email, because an email change can revoke an invitation link;
- no draft goes to Web Storage, addresses or logs;
- a pending save is cancelled on sign-out or an account change: the timer runs on the injected scheduler and is cancelled when the card unmounts, and an account change unmounts it;
- a gone card's `404` is handled as today ("This student is no longer on your list.");
- "Only you see this" stays;
- the debounce bounds the writes.

Invitation link handling does not change: the name and email dialog sends today's patch, and the email change rules and their daily limit stay. The change is reclassified if the final diff changes either (T-7, T-18, T-20, T-27, T-37). One standard reviewer reviews it, followed by its change checks and its merge gate.

## 3. Questions for the owner, asked with this plan

1. **Group page columns.** Members in the main column and the next lessons beside them (recommended: members are the wider table and the page's main work), or the lessons in the main column.
2. **Autosave timing.** 1 second after typing stops and on leaving the field, with "Saving…" and "Saved" (recommended), or on leaving the field only.

## 4. What changes

- **`features/students/StudentCardPage.tsx`:**
  - the header row with an `ActionMenu`;
  - `DetailsForm` moves into a `ModalDialog` opened from the menu, and closes on a saved edit with the existing "Saved" notice taking focus; it keeps the stale-edit rule;
  - `NotesForm` becomes `NotesAutosave` in the side column of `PageColumns`;
  - the dangerous actions open from the menu, with their dialogs unchanged.
- **`features/groups/GroupPage.tsx`:** the header row with an `ActionMenu`, `DetailsForm` in a dialog, the archive toggle in the menu with its confirmation rules, and `PageColumns`.
- **New `features/students/notesAutosave.ts`:** the debounce on `useScheduler`, cancelled on unmount; one request at a time, so a change made while a save is in flight saves after it.
- **`lib/i18n/en.ts`, `ru.ts`:** "Card actions", "Group actions", "Name and email…", "Name and colour…", "Saving…", "Saved", "Notes were not saved", "Retry".
- **Tests:**
  - `e2e/students.spec.ts`: notes typed at 1440 × 900 are visible without scrolling and saved without a button, read back after a reload; a reload in the middle of typing keeps nothing in storage; editing the name through the menu dialog; ending through the menu; the existing focus tests move to the menu entry points.
  - `e2e/groups.spec.ts`: name and colour through the menu dialog; archive and unarchive through the menu, with and without the confirmation; members and lessons in their columns.
  - A unit test of the autosave scheduler: debounce, save on blur, one request at a time, cancelled on unmount.
- **Specifications** (pending owner review):
  - `knowledge/design/web/pages.md`: the Student card and Group page layouts, and the shared stale-edit rule restated for autosave: a refetch shows a stored value only while the field has not changed since it last showed one, and an autosave that answers after newer typing does not overwrite it;
  - `docs/` is not changed.

## 5. Acceptance criteria

1. At 1440 × 900 the notes field of an active card is inside the viewport without scrolling, in Chromium, Firefox and WebKit.
2. Notes typed and left for 1 second are saved without a button (a `PATCH /students/{id}` whose body has only `notes`), and are shown after a reload.
3. A failed save keeps the text and shows "Notes were not saved" with Retry; Retry saves.
4. "Name and email…" and "Name and colour…" open dialogs on their first field; a saved edit closes the dialog and the "Saved" notice takes focus; a dismissed dialog returns focus to the menu button.
5. The dangerous actions open from the menu with today's dialogs and focus rules; the existing ending, removal, withdrawal and archive tests pass with their entry point moved.
6. The existing stale-edit, live-change and refetch-failure tests of both pages pass with no assertion removed or loosened.
7. `tools/docs/check-docs.sh` and the frontend change checks pass: `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `check:bundle` and the whole `pnpm e2e`.

## 6. Order of work

1. Worktree on `feat/design-step-d5-card-and-group` from `main`.
2. The autosave hook with its unit test.
3. The card layout, menu and dialog, then the group page.
4. Tests, then the specifications.
5. Change checks, the standard reviewer, fixes under the after-fix rule, the readiness report, the gate, the merge and finish.

## 7. Observability

None. No backend change; autosave uses the existing `PATCH` route and its logging.

## 8. Risks

- **Autosave against a refetch.** The stored-value rule keeps typing; a save that answers after newer typing does not overwrite the field, because the field keeps what the user typed and the next debounce saves it.
- **Autosave against a change made elsewhere.** Notes have no version, so the last write wins, as with today's Save button.
- **Focus.** Moving actions into menus changes the entry points of many focus tests; each keeps its assertion and changes only how the dialog opens.
- **Size.** Both pages change at once; the plan keeps every dialog and refusal unchanged so the diff is layout and entry points.

## 9. As built

- The Russian menu item is "Имя и email…", matching the dialog heading and field; "Try again" follows the application's wording instead of "Retry".
- "Try again" moves focus back to the notes field, and leaving the field while a save is refused does not resend it, so the button never disappears under focus or the pointer.
- The group page no longer shows the archive sentences; a stale archive refusal without a dialog shows only the changed notice; the name and colour dialog closes on a stale refusal. `pages.md` lists these.
- The end-to-end tests check stored notes through the API, because Chromium hides `fetch` bodies from Playwright; the component test checks that the body holds only `notes`. The refused-save test sets a control character with `setRangeText` and an input event, because typing in Firefox drops it. The stale-card test holds the autosave request with `page.route` and releases it unchanged before its assertions.
- Known limitations of this step are in `knowledge/reviews/slice-design-known-limitations.md`.
