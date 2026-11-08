# Design slice step L: the desktop layout standard

Status: approved by the owner on 2026-11-08. A security-profile planning review returned "approve with changes", and every finding is applied here. It carries out decision 7 of the [interface decisions](ui-ux-decisions-2026-11-06.md#decisions) and step L of the [design slice plan](design-slice.md). Base: `main` at `d7403d9`. No slice 7 branch is open at its start: `fix/lesson-join-access`, `fix/lesson-room-client-residuals` and `fix/chat-download-slot-flake` are already contained in `main`.

## 1. Problem and outcome

**Problem.** Every signed-in page runs down one column. Most pages are capped at the 720 px reading width (`Page` defaults to `max-w-reading`). Lists are stacked rows, Settings are forms that are always open, and dangerous account actions are large buttons in the page flow. Steps D2 to D5 need shared layout components to build on.

**Outcome.** From 1024 px, Students, the group list, My tutors, Notifications and the four Settings pages follow the layout standard of decision 7:

- a one-line header with the title, the main action and a menu for rare actions;
- lists as tables with the filters in one row above them;
- settings as rows, with the label on the left and the value on the right;
- dangerous actions in a menu.

The shared components and their rules are in `tokens.md` and `pages.md`, so steps D2 to D7 reuse them.

## 2. Decisions the owner made for this plan (2026-11-08)

- **Side column below 1280 px.** From 1280 px the main column and the 300 px side column sit side by side, 64 px apart. Below 1280 px the side column follows the main column in one column. This settles the open item "Side column at 1024 px".
- **Sign out everywhere gets a confirmation.** A dialog names the consequence: every session ends, this one included. It opens on Cancel and returns focus to the menu button on close.
- **L is two changes.** L1 holds the shared components, their rules and the pages that do not meet the trigger. L2 holds Settings, which meets it.
- **The stress-only readiness check runs between L and C.** It is carried from slice 7 (section 8).

## 3. Verification tier and security trigger

| Change | Tier | Trigger | Reason |
|---|---|---|---|
| This plan | Task plan | **Met** | It covers L1 and L2. A security-profile reviewer reviewed it before the owner approves it. |
| L1: components, Students, groups, My tutors, Notifications | Standard | **Met** | It rebuilds the rendering of untrusted content: notification labels (T-14), display names, emails, group names and tutor bios in a new shared table. It also edits the token pages' `Page` call sites (T-27). Join-request decisions and Stop studying (T-21, T-33) move on the page, while their handlers, routes, requests and refusals stay as they are. L1 does not touch invitation links, which live in the Add student dialog and the student card, both unchanged. |
| L2: Settings (Profile, Account, Notifications, Teaching) | Standard | **Met** | The forms handle passwords and end sessions, and Teaching holds the tutor code and unblocking (T-1, T-2, T-8, T-33). No route, handler or token handling changes. The password change moves into a dialog. Sign out everywhere and Delete account move into a menu, and the sign-out gets a confirmation. |

Reviews:

- L1 and L2 each get one standard reviewer and one security-profile reviewer, dispatched together.
- Each change has its own change checks and merge gate.

## 4. Threats each change maps to, and what it must hold

**L1:**

| Threat | What L1 must hold |
|---|---|
| T-14 | Display names, emails, group names, tutor bios and notification labels stay React text nodes, with `dir="auto"` where they are today. The notification row link is still built only by `linkAddress(kind, id)`, and a click still marks the row read with `{ids}`. New components style through classes, never `style` attributes, under the `style-src 'self'` content security policy. |
| T-27 | Edits to the token pages (invitation, confirmation, reset, unsubscribe) are limited to the width prop. `pnpm check:bundle` passes. |
| T-18, T-8 | The Students search and group choice still reach the address only on submit, exactly as `listQuery` builds them today, never per keystroke. The empty state's tutor code stays in the query cache only. Existing `students.spec.ts` tests cover both. |
| T-21, T-33 | Join-request decisions (including decline and block in the row menu) and Stop studying keep their handlers, busy states, notices and refusals. Existing specs cover them. |

**L2:**

| Threat | What L2 must hold |
|---|---|
| T-1 | The password change and the delete confirmation still send the current password only in the request body. `autocomplete` values stay `current-password` and `new-password`. **No password field exists in the DOM while its dialog is closed.** Both the password dialog and the delete dialog are built on `ModalDialog`, which renders its children only while open. The delete dialog opens on its password field, as `pages.md` requires. No password value outlives its dialog or the session. No password reaches Web Storage, the address, history state, logs or diagnostics. |
| T-2 | Sign out everywhere still calls the same route and then ends the local session as today. The confirmation only adds a step before it, so Cancel sends nothing. While the request is in flight, the dialog cannot be closed and "Account actions" is unavailable. A refusal shows inside the dialog and takes focus. The threat model row T-2 gains "sign out everywhere asks for confirmation in the interface", and the threat model's status line records the amendment, both marked pending owner review. This is the only `docs/` change in this step, and it lands in L2. |
| T-8, T-33 | The tutor code keeps Copy and Regenerate, and an unconfirmed tutor keeps the confirmation prompt instead of Regenerate. Regenerate, Unblock, the deleted-account row's label and accessible name, and the `not_found` handling of Unblock stay as specified. |
| T-18 | Nothing new reaches logs, addresses or third parties. Dialog open state is not put into the address. |
| T-34 | The delete dialog's consequence text stays true. |

Threats not affected: T-5 and T-6, because no query is added or removed and the frontend makes no authorization decision; T-7, because invitation links are not touched; T-28; T-41, because agent configuration is unchanged.

The negative tests each change adds are in section 6.

## 5. What changes

### L1: components, rules and four pages

**Shared components** (`frontend/src/components/ui/`):

- **`Page` header.**
  - The `<h1>` uses the 28 px `title` size at every width. Today and Home pass `titleSize="display"` and keep 36 px from 1024 px.
  - New `menu` prop: an `ActionMenu` of rare actions, placed after `action` in the one-line header.
  - The width prop `wide` becomes `width: 'page' | 'reading'`. It defaults to `page`, which keeps the 1120 px content width.
  - The lesson page and Schedule already use the page width and keep it. Pages that keep the reading width until their own step pass `reading` explicitly:
    - public and token pages, which keep it permanently;
    - Today and Home, until D2;
    - the student card and the group page, until D5.

    So no page that L does not rework changes width.
- **`PageColumns`:** a main column and a 300 px `aside`, 64 px apart, side by side from a new `wide` breakpoint (1280 px) and stacked below it. The aside comes after the main column in DOM order, so reading order and tab order match the stacked order.
- **`DataTable` styling:** semantic `<table>` with `<th scope="col">` column headers, 1 px `border` hairlines between rows and no zebra striping. Row actions go in the last cell. The row's main link sits in its name cell, not around the whole row, so each row has one link name and separate action buttons. Below 768 px the table stays a table: secondary columns are hidden and their content is shown under the name in the first cell, as the Students list already shows the email, so there is no sideways scroll at 390 px and table semantics hold in every browser (owner's decision of 2026-11-08, replacing stacked block cells, which would need explicit roles that the accessibility lint rejects as redundant).
- **Filter row:** one flex row for the tabs, the search field and its button, and the extra filters, wrapping below 1024 px. As built, Students holds it in its own classes rather than a shared `FilterBar` component, because it is the only page of L with filters; D3 to D5 extract it if they need one.
- **`styles.css`:** add `--breakpoint-wide: 1280px` and `--container-aside: 300px`. No raw values in components, as `tokens.md` requires.

**Pages:**

- **Students** (`features/students/StudentsPage.tsx`):
  - the header keeps "Add student" as its action;
  - the tabs, search and group choice form one filter row;
  - the list becomes a table with the columns Name (avatar and display name as the link to the card), Email, State, Groups and Next lesson;
  - pending join requests stay above the filters, as `pages.md` places them, with their handlers unchanged.
- **Groups** (`features/groups/GroupsPage.tsx`):
  - "New group" moves into the header as its action;
  - the list becomes a table with the columns Name (colour dot and name as the link), Members (avatars, "+{n}" and the count), Next lesson and State ("Archived");
  - the empty state keeps its action, as `pages.md` specifies for Groups.
- **My tutors** (`features/tutors/`):
  - `PageColumns` holds the current tutors and Past tutors in the main column, and "Add a tutor" with Sent requests in the aside;
  - current tutors become a table with the columns Tutor (avatar, name, colour dot, and the bio clamped to two lines with its full text available), Subjects and Actions ("Message", and the existing overflow menu with "Stop studying");
  - Past tutors becomes a table with the columns Tutor, Subjects, Ended (the closed-account line when it applies) and "Messages" while the read-only chat exists;
  - the Stop studying dialog, its opener focus and its notices are unchanged.
- **Notifications page** (`features/notifications/NotificationsPage.tsx`):
  - "Mark all as read" moves into the header as its action;
  - the rows become a table with the columns Event (icon, the one-line text and the unread marker with its hidden word), From (the label as text, with the colour dot for students) and Time;
  - "Show more" still focuses the first new row's link, or its first cell when it has no link;
  - the bell panel is unchanged.

**Specifications** (all new wording marked pending owner review):

- `knowledge/design/web/tokens.md`:
  - the type scale: page titles at 28 px, with 36 px kept for Today and Home;
  - layout widths: the `page` width as the default for signed-in pages, the reading width for public and short-form pages, and the side column side by side from 1280 px and stacked below;
  - a "Tables" and "Page header" entry under Components.
- `knowledge/design/web/pages.md`:
  - a new "Desktop layout" item under Shared behavior, holding decision 7's rules as built: the header, columns, tables, filter row, settings rows and dangerous actions in a menu;
  - the Students, Groups, My tutors and Notifications sections say how they show it.

### L2: Settings

- **`SettingsNav`** stays as the section links under the header.
- **Settings rows.** A shared `SettingsRow` holds the label (and an optional one-line description) on the left in a 240 px column and the value or control on the right. From 768 px they sit on one line. Below 768 px the label sits above the value.
- **Profile.** Name and subjects sit in one row of the grid. The bio spans the width below them. The avatar is one settings row. The form and its Save are unchanged.
- **Account.**
  - Language, timezone and theme become settings rows of the existing preferences form, which keeps its single Save.
  - "Password" is a row whose value is a "Change password" button. The button opens a `ModalDialog` holding the current behavior: current password, new password, and the success notice and refusals as specified. The dialog opens on its first field. A success closes it, and "Your password has been changed." takes focus on the page.
  - The header menu "Account actions" holds "Sign out everywhere" and "Delete account". `ActionMenu` gains a way to expose its button, a forwarded ref, so that dialogs opened from the menu can return focus to it.
  - Sign out everywhere opens a new `ModalDialog` confirmation:
    - it opens on Cancel;
    - its confirm button keeps today's call;
    - while the request is in flight, the confirm button is busy, the dialog cannot be closed and "Account actions" is unavailable;
    - a refusal shows inside the dialog and takes focus.

    `pages.md` gives its heading, consequence and button wording.
  - Delete account moves onto `ModalDialog`, so its form exists only while it is open. It opens on its password field, keeps its consequence, refusals and throttling, and returns focus to the menu button.
- **Notifications** (settings): each email group is a settings row with its name and description on the left and the switch on the right. The save-per-switch behavior, the busy state, the alert and Retry are unchanged.
- **Teaching:**
  - The tutor code is a row with the code, Copy and Regenerate. An unconfirmed tutor sees the confirmation prompt instead of Regenerate.
  - The storage use is a row.
  - Blocked students become a table with the columns Student (avatar and name, or "Deleted account"), Blocked and an Unblock action in the last cell. Unblock keeps its accessible names and notices.
- **Specifications:** `pages.md` Settings rewords Account for the dialog, the menu and the confirmation, and Settings in general for rows. `docs/threat-model.md` row T-2 gets the line in section 4.

## 6. Acceptance criteria

Each criterion is checked by a named test or command. All new end-to-end tests run in Chromium, Firefox and WebKit.

**L1:**

1. `frontend/e2e/desktop-layout.spec.ts` › "Students shows its table columns in one row with the filters above them at 1440 × 900": every `columnheader` of the students table has the same top. The bottom of the filter row's search field and group choice is above the table's top.
2. Same spec › "a page title is 28 px at 1440 × 900" and › "Today keeps the larger title at 1440 × 900": the computed font size of the Students `h1` is `28px` and that of Today is `36px`.
3. Same spec › "/students does not scroll sideways at 1024 px" and the same test for `/groups` and `/notifications` (tutor), with a 60-letter single-word student and group name, and › "My tutors does not scroll sideways at 1024 px" (student): `document.documentElement.scrollWidth <= clientWidth`.
4. Same spec, two tests:
   - › "/students does not scroll sideways at 390 px" and the same test for `/groups` and `/notifications`, and › "at 390 px My tutors shows its side column below the main column": no sideways scroll, and on `/tutors` the aside's top is at or below the main column's bottom;
   - › "at 1440 px My tutors shows its side column beside the main column": the aside's top equals the main column's top.
5. Same spec › "a table row keeps the tutor as its header and its actions apart": on `/tutors` the tutor name is the row header, not a link, because a tutor has no page of their own (an implementation divergence reported to the owner). The "Messages" link and the overflow menu button are separate controls in the last cell.
6. Same spec › "/students shows a name with markup as text in its table" and the same test for `/groups` and `/notifications`, and › "a tutor name with markup is shown as text in My tutors": a student card, a group and a tutor name hold a markup value such as `<img src=x onerror=alert(1)><b>x</b>`; each table shows it as literal text, its first row holds no `img` or `b` element, and the document title is unchanged. `notifications.spec.ts` › "a label with markup is shown as text" keeps covering the bell panel, which stays a list, and `notifications.spec.ts` gains tests for a page row's link marking it read and for "Mark all as read" in the page header; `desktop-layout.spec.ts` › "New group stands in the page header once a group exists and gets focus back from its dialog" covers the moved Groups action.
7. The existing focus and behavior specs pass unchanged in meaning: `students.spec.ts`, `groups.spec.ts`, `join-requests.spec.ts`, `notifications.spec.ts`, `invitations.spec.ts`, `ending.spec.ts`, `live-updates.spec.ts`, `realtime.spec.ts`, `reclaim.spec.ts`, `account-edge-cases.spec.ts`, `accessibility.spec.ts`, `smoke.spec.ts` (including its content-security-policy violation check) and the e2e helper modules. Locators that addressed a `listitem` move to the matching `row`; no assertion is removed or loosened.
8. `tools/docs/check-docs.sh` passes, and `tokens.md` and `pages.md` carry the new rules marked pending owner review.
9. The change checks of `frontend/` source and `frontend/e2e/` pass: `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes`, `check:bundle` and the whole `pnpm e2e`.

**L2:**

1. `desktop-layout.spec.ts` › "a Settings row has its label and value on one line at 1440 × 900", on Account (Time zone) and Teaching (Tutor code): the label's and the control's vertical centres differ by less than half the row height.
2. Same spec › "no Settings page scrolls sideways at 1024 px or 390 px", for the four Settings pages of the right role.
3. Password dialog, in `account-edge-cases.spec.ts` or a new `account-settings.spec.ts`, each a separate test:
   - › "the password dialog leaves no password field behind": type into both fields, then close with Esc. `input[type="password"]` has count 0 on the page. A second test does the same with Cancel.
   - › "the password dialog reopens empty on its first field": both fields are empty, and the first is focused.
   - › "a changed password closes the dialog onto its notice":
     - the notice is focused (`toBeFocused`);
     - the old password answers 401;
     - signing in with the new password works;
     - the page address is exactly `/settings/account` with no query;
     - `JSON.stringify(history.state)` does not contain either password;
     - `localStorage` holds only the known preference keys and `sessionStorage` is empty.
   - › "the password dialog does not outlive the session":
     - open the dialog and type;
     - revoke the session with `callApi(… '/auth/logout-all')`, as `realtime.spec.ts` does;
     - the sign-in page shows with no dialog, and its only password input is the empty sign-in field (as built, the sign-in again was left out to keep the test to one act; a fresh page load cannot hold a dialog).
4. Sign out everywhere, each a separate test:
   - › "sign out everywhere opens on Cancel";
   - › "Cancel on sign out everywhere revokes nothing":
     - focus returns to the "Account actions" button;
     - a passive `page.on('request')` listener counts 0 `POST` requests to the logout-all route;
     - a second browser context stays signed in.
   - The existing test "signing out everywhere ends the sessions of other browsers" goes through the dialog and keeps every assertion.
5. Delete account:
   - › "the delete dialog leaves no password field behind": as criterion 3, for Esc and for Cancel, with the address and storage assertions;
   - › "the delete dialog opens on its empty password field"; the focus return to the menu button is asserted by the Esc and Cancel tests above and by `accessibility.spec.ts` › "Escape on the deletion dialog returns focus to the button that opened it".

   The existing delete tests pass from the menu: the wrong password, the sixth wrong password answering 429, and deletion signing out with the deleted-account message.
6. The existing Settings specs pass:
   - `teaching-settings.spec.ts`, `email-settings.spec.ts`, `language.spec.ts`, `signup.spec.ts` and `reclaim.spec.ts`;
   - the settings parts of `accessibility.spec.ts`, `account-edge-cases.spec.ts` (the password change, the wrong current password, the deletion tests), `join-requests.spec.ts`, `students.spec.ts` and `message-attachments.spec.ts`;
   - the component tests `ProfileSettingsPage.test.tsx`, `StorageUsage.test.tsx` and `TimezoneField.test.tsx`.

   Locators move to the dialog, the menu or the row. No assertion is removed or loosened. The labelled regions the tests address stay, such as the blocked students region and the preferences form's name.
7. `tools/docs/check-docs.sh` passes. `pages.md`, the T-2 row and the threat model's status line are updated and marked pending owner review.
8. The same frontend change checks as L1 pass.

## 7. Order of work and files

1. Worktree `../ovc-worktrees/design-l1` (outside `/private/tmp`) on `feat/design-step-l1-layout` from `main`. Build its `bin/` tools and run `tools/check-tool-binaries.sh`.
2. L1:
   - `styles.css` tokens;
   - `Page`, `PageColumns`, the table and filter components;
   - the `width="reading"` call sites;
   - the Today and Home `titleSize`;
   - Students, Groups, My tutors, Notifications;
   - i18n keys for the column headers and the menu label, in `en.ts` and `ru.ts`;
   - `desktop-layout.spec.ts` and the locator moves;
   - `tokens.md` and `pages.md`;
   - this plan as `knowledge/plans/design-step-l.md`.
3. L1 change checks, then the standard reviewer, fixes under the after-fix rule, readiness report, merge gate, merge and finish.
4. L2 on `feat/design-step-l2-settings` from the moved `main`:
   - `SettingsRow`;
   - the four Settings pages;
   - the password and sign-out dialogs;
   - the header menu;
   - the tests;
   - `pages.md` Settings and the threat-model row T-2.
5. L2 change checks, then the standard and security-profile reviewers together, fixes, readiness report, gate, merge and finish.
6. The stress-only readiness check (section 8), then step C.

## 8. The two items carried from slice 7

- **WebKit failures under heavy load.** On any WebKit failure of a design slice gate, from L1 onward, the gate counts as failed as usual. Before any decision on a rerun, I read the redacted `browser-diagnostics.json` that the failed gate keeps under `~/Library/Caches/ovc-e2e/logs.*/e2e-results/`. I record the diagnosis (test, browser, phase, request and console evidence, machine load) in `knowledge/reviews/design-slice-webkit-failures.md`. The record quotes only fields of the already redacted file, never raw request bodies, cookies or addresses carrying tokens (T-18, T-27). The owner then decides on the rerun or the fix through a multiple-choice question. If no WebKit failure occurs by the slice review, the record says so, and the item is carried on by the owner's decision.
- **Readiness failures seen only under a stress run.** This runs after L2 merges and before C starts, with a 2-hour time box, on an idle machine (no gate, `pnpm e2e` or `go test`, and port 55432 free, checked first). The hypothesis comes from the slice 7 record, and the stress run is the one that showed the failures. I run it once as recorded there and once idle. The outcome is either a named cause with its evidence or "not reproduced within the time box". Any run that needs the owner's settings goes only through `tools/dev-env.sh`. The outcome is recorded under `knowledge/analysis/`, and any fix becomes its own change with its own plan. No dev or demo server is left running.

## 9. Observability

No backend change and no new route. The frontend adds no log or diagnostic event. Requests per page stay the same, because no query is added or removed: tables render the same queries.

## 10. Risks

- **Many end-to-end locators move from list items to rows.** Among others, `join-requests`, `students`, `groups`, `ending`, `invitations`, `notifications`, `live-updates`, `realtime`, `reclaim` and the helper modules address `listitem`. Only the locators of the lists L turns into tables move, each to the matching row with the same assertion.
- **Hidden columns below 768 px.** Their content must appear in the first cell at that width, so nothing is lost on a phone; the 390 px test runs in WebKit too.
- **Tables and focus rules.** "Show more" and live-refetch focus moves ("the row that took its place, else the previous row") now target a row's link or first cell. Existing `toBeFocused` tests cover them and must pass unchanged.
- **Dialogs opened from a menu.** Focus must return to the menu button, not the removed menu item. The new tests in L2 criteria 4 and 5 check this. The menu component's focus handling is reused, not rewritten.
- **Width changes on pages L does not rework.** The explicit `reading` call sites keep them unchanged. The 1024 px test covers only L's pages, and D2 to D7 add theirs.
- **Slice 7 work on shared files** (`i18n`, `pages.md`, the e2e helpers). Each change rechecks open slice 7 branches at its start and takes a moved `main` under the squash rule.
- **WebKit flakiness can stop a gate.** Section 8 handles it. A failed assertion is never rerun without the owner.
