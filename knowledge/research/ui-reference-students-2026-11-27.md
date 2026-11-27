# Students list and student card: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R6a `feat/redesign-students` and R6b `feat/redesign-student-card` of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

Two screens of the tutor role:

- **Students list** (R6a): pending join requests on top, the Active / Invited / Ended tabs, search and filters, one row per student, the invited, ended, provisional and reclaim-pending states.
- **Student card** (R6b): the header with its actions, the tabs, the private autosaved notes, the summaries, and the destructive actions behind confirmations.

OVC today, as specified in [pages.md, Students](../design/web/pages.md#students) and [pages.md, Student card](../design/web/pages.md#student-card), with texts in [states.md, Relationships, requests and groups](../design/web/states.md#relationships-requests-and-groups):

- The list is a table (Name with avatar, Email, State, Groups, Next lesson, Open assignments) under one filter row of tabs, a search applied with a Search button or Enter, and a Group choice, all kept in the address. Pending join requests stand on top with Accept, Decline and "Decline and block" in a menu. "Add student" is the only creation button, in the page header.
- The card has the display name as heading, "Messages", "Schedule a lesson" as the primary action, "Issue assignment" as a secondary one, and the menu "Card actions" holding "Name and email…" and the state's dangerous action as the last item after a separator, each opening a dialog that opens on Cancel. Tabs Overview, Lessons and Homework are address links. Notes sit in a side column, marked "Only you see this", saved 1 second after typing stops and on blur through `PATCH /students/{id}` with only `notes`, never stored in Web Storage, addresses or logs.
- Components live in `frontend/src/features/students/` (`StudentsPage.tsx`, `StudentCardPage.tsx`, `JoinRequests.tsx`, `StateBadge.tsx`, `GroupDots.tsx`, `InvitationLink.tsx`, `notesAutosave.ts`).

The benchmark audit (`knowledge/research/ui-benchmark-audit-2026-11-27.md`, commit `512765d`) found for these screens: table cells break words (its top-15 item 2), a live lesson reads as "No lesson scheduled" (item 3), phone headers spend two rows on actions (item 7), search needs a button and there are no filters (item 11), and the card repeats the email and the lesson action (item 12).

Owner's decisions in section 3 of the plan that bind this study:

- **Item 14:** initials plus the existing authorized avatar fields, with no expanded audience; Files is deferred; private notes stay; only scoped summaries and filters are added, with explicit paginated API contracts where missing.
- **Item 16:** destructive actions go in confirmed menus, one action is primary, internal identifiers are hidden, ADR-0010's one focal panel and sentence-case status text stay.
- **Item 6:** no underlined action links; Back is a muted arrow link.
- **Item 8** (pending notifications change): pending join requests count on the Students navigation item.
- Section 7, R6a/R6b acceptance: all relationship states remain usable, summary values are scoped, notes never reach students, browser storage or logs, tabs preserve focus and note drafts, and refusal and live-change notices keep their focus rules.

Mockups (static, non-normative) at `8b3a8b0`: `knowledge/design/web/prototype/redesign/students.html` shows a "New request · 1" block with Accept and Decline above the tabs (Active 4 / Invited / Archive), a field labelled as searching while typing, three filter chips (Group, Has unfinished work, No lesson), and rows with initials avatar, name, a group and weekday subtitle, a Next lesson cell ("Live now 18:00–19:00" with Join, or "Not scheduled, last on 20 November"), an open-work count and a last-message time with an unread count; a footer reads "Showing all 4 students". `student-card.html` shows the name and email, the state line, the buttons Message, Schedule a lesson and Issue assignment, a menu with "Name and contact…" and "Stop working together" marked as needing confirmation, three summary tiles (Next lesson with Join, Open assignments "2 works, 1 awaiting review", Groups), tabs Lessons, Homework, Notes and Files, and a note panel marked as visible only to the tutor.

## How the compared products solve it

### TutorBird (tutor and studio CRM)

- The Student List switches its visible set with a toolbar item on the right offering Active, Inactive, Waiting, Leads and Trial; waiting students and leads used to sit at the bottom of the page and were moved into that toolbar [1].
- Trial works like Active but separates students who have not committed long term [1]. Statuses separate leads, trials, waiting lists and active students [2].
- Search exists on the Student List [3]; students can carry custom tags and be sorted or searched by tag [2]; the list can be sorted by the school column when that column is shown [8].
- The student record holds contact details, family members, notes, attachments and other optional details [2]. A Notes column in the list shows a button when notes exist, which opens a pop-up with them [5].
- The student profile window has tabs; "Student Overview" was renamed "Student Details" (time zone, group tags, notification preferences, extra fields), and the Student Portal tab's Disable Access and Reset Password moved under a three-dot menu at the top right [4]. A student switcher in the profile lists only active students [4]. The profile shows the student's age [7].
- Private notes exist for lesson notes, with a separate heading for private notes in lesson-notes emails [6]; whether profile notes are private was not stated in the pages found.
- The support centre is client-rendered and gave no readable content [9].

### My Music Staff (sister product of TutorBird, music-teacher CRM)

- The same Student Details redesign as TutorBird: the overview tab renamed, portal actions moved under a three-dot menu at the top right, and Family Contacts actions moved to the top of its tab [10].
- Lead students were added to a clickable student count at the top of the Students page; the mobile Students list was redesigned to fit more rows and tapping a student opens the full details [11].

### Teachworks (tutoring-business management)

- Notes are internal: they "can't be seen by families or students", and each note can be limited to administrators or administrators and teachers [12]. Communication logs open from student or family profiles, and a "Profile Notes" link on the student and family lists shows all notes, filterable by student [12].
- The student profile lists the 10 most recent lessons with their notes, under a "Scheduling" tab for a child student or a "Learning" tab for an independent student; teachers see notes only of their own lessons by default [13, excerpt only].
- The student list has a status filter at the top right of the table; profiles created by forms appear under "Prospective" and can be activated one by one or in bulk with checkboxes [14, excerpt only].
- Deleting a student is possible only after its lessons (and, for independent students, invoices and payments) are deleted, from a Delete button on the list; keeping the records means setting the status to Inactive [15, excerpt only]. An add-on sets students Inactive after a period with no lessons [16, excerpt only].
- Profiles created by booking forms must be set to "Active" by staff; customers see their own lesson history and billing [17].

### Weekify («Викифай», Russian tutor CRM)

- The earlier repository research ([Weekify research of 2026-11-23](weekify-2026-11-23.md)) records a student card with contacts, goals, parent contacts, a balance limit, archive and unarchive, and delete, and bulk adding by pasting names; those facts came from the public client bundle and are medium confidence there.
- Rechecked today: the site's robots rules still disallow everything except the home page [18], so the bundle and the legal files were not fetched again. The App Store description says the tutor keeps a student database with "all the necessary information", tracks payments and leaves lesson notes; the current version is 1.0.21 [19]. The RuStore listing answered with an error page and confirmed nothing [20].
- No layout of Weekify's student list or card could be verified from a public page today.

### Google Classroom People page (roster comparator with a scope model)

- The teacher invites from People with "Invite students", entering an email or group alias with autocomplete; the class list then shows the invited students' names [21]. The invite link and class code live on the class card and in class settings, not on People [21].
- Removing students: tick their boxes, then Actions, Remove, and confirm with Remove again; their grades leave the gradebook while their work and stream posts stay [22].
- Clicking a student's name on People opens that student's page: assigned work with due dates, a status per item (assigned, turned in, late, missing), grades, attachment and private-comment icons, View Details, and filters "Turned in", "Returned with grade" and "Missing"; only a student who has joined has such a page [23].
- Classmates can see each other's name and photo and can find each other's email address [24]. OVC's scope rules forbid this for students (see below).

## Comparison

| Design question | TutorBird / My Music Staff | Teachworks | Weekify | Google Classroom | OVC decision or target |
|---|---|---|---|---|---|
| Relationship states on the list | Status toolbar: Active, Inactive, Waiting, Leads, Trial [1] | Status filter, Prospective and Inactive [14, 15] | Archive and unarchive (earlier research) | Invited students shown in the list [21] | Keep tabs Active / Invited / Ended; provisional and reclaim-pending stay in Active with badges |
| Counts on the list | Clickable student count at the top [11] | Not found | Not found | Not found | Tab counts drawn; pending requests counted on the navigation item (item 8) |
| Incoming requests | Leads as a status [1] | Prospective filter, bulk activate [14] | Booking request inbox (earlier research) | Not applicable | Requests block on top with Accept / Decline, as today |
| Search and filters | Search, tag sort and search [2, 3] | Status filter [14] | Not verified | Status filters on a student's page [23] | Instant search; chips Group, Has open work, No lesson (audit item 11, mockup) |
| Row summary | Optional columns, Notes button [5, 8] | Not verified | Not verified | Name and photo | Initials or permitted avatar, next or live lesson, open work, last message: scoped, paginated sources only |
| Profile structure | Tabs, overview renamed Student Details [4] | Tabs by profile type, recent 10 lessons [13] | Card with lessons, homework, finances (earlier research) | Per-student work page with filters [23] | Overview / Lessons / Homework tabs as links; summary tiles; Files deferred |
| Secondary and portal actions | Three-dot menu at top right [4] | Edit form | Not verified | Actions menu with checkboxes [22] | "Card actions" menu; one primary action (item 16) |
| Destructive actions | Not verified | Delete only after records go; Inactive keeps them [15] | Archive, delete (earlier research) | Remove, confirmed twice [22] | End / Withdraw last in the menu, after a separator, confirmed dialog opening on Cancel |
| Private notes | Notes on the record; private lesson notes [2, 6] | Internal only, never seen by students [12] | Lesson notes [19] | Private comments on work [23] | Autosaved private notes, never to students, storage or logs |
| Peer visibility for students | Not applicable | Child students see only their contact data [13, excerpt only] | Not applicable | Classmates see name, photo, email [24] | Students never see other students (ADR-0005) |

## What OVC adopts

- **Status segmentation as tabs with counts, requests on top.** TutorBird's single status switch [1] and My Music Staff's clickable count [11] support keeping one control for relationship state; OVC keeps its three address-held tabs and adds counts as drawn, with pending requests in their own block, matching item 8's Students count.
- **Instant search and a few filters.** TutorBird's list search and tags [2, 3] and Classroom's status filters [23] support the audit's instant filter (item 11) with the mockup's chips Group, Has open work and No lesson; each filter value stays in the address like `group=`, and the R6a contract defines them server-side so results never come from filtering one loaded page.
- **Notes stay private and inline.** Teachworks states that notes never reach families or students [12]; TutorBird puts a notes affordance on the list [5]. OVC keeps the private autosaved notes on the card (item 14) and does not add notes to list rows.
- **Secondary actions in a three-dot menu, destructive last.** TutorBird moved portal actions under a three-dot menu [4]; Classroom confirms Remove a second time [22]; Teachworks prefers Inactive over delete to keep history [15]. OVC keeps "Card actions" with End or Withdraw last, confirmed, and ending keeps history, which matches item 16.
- **One per-student summary of work with filters.** Classroom's per-student work page with status filters [23] and Teachworks' recent-lessons list [13] support the card's summary tiles (next lesson, open work, groups) and the existing Homework tab; the tiles reuse the existing `next_lessons` and issue reads.
- **Live lesson vocabulary.** The mockup's "Live now" with Join in the Next lesson cell and tile resolves audit item 3; no compared product documented this, so it rests on the audit and the plan, not on this study.

## Where OVC differs and why

- **Relationship scope and peer privacy.** Classroom lets classmates see each other's name, photo and email [24]; OVC shows a student to a tutor only through that tutor's own relationship and never shows students to each other ([ADR-0005](../../docs/decisions/0005-tutor-centric-domain.md)). Summaries and avatars on the list come only from the tutor's own relationship data and the existing authorized avatar fields (item 14).
- **No statuses beyond the relationship model.** TutorBird's Leads, Waiting and Trial [1] and Teachworks' Prospective [14] are sales pipeline states; OVC's states are the relationship lifecycle of [states.md](../design/web/states.md#relationships-requests-and-groups). The BOOK and FIN stages, not R6, would add anything like them.
- **No hard delete from the list.** Teachworks offers Delete after records are removed [15] and Weekify a delete (earlier research); OVC ends a relationship, keeps history and erases only through the paths of [ADR-0024](../../docs/decisions/0024-legal-documents-and-student-data.md), so no delete action is added.
- **No bulk actions.** Classroom's checkboxes [22] and Teachworks' bulk status change [14] are not adopted; every destructive OVC action names its consequences for one relationship in its own dialog.
- **No money on the card.** Weekify, TutorBird and Teachworks put balances and billing on the profile; OVC's finances belong to the FIN stage, after UI (plan section 3 item 9).
- **No Files tab.** The mockup draws Files; item 14 defers it.
- **Notes placement.** The mockup draws Notes as a tab; the current specification keeps Notes in a side column visible beside every tab. Moving notes into a tab would hide them from Overview and change the 1440 × 900 in-view requirement; see the open points.
- **Accessibility.** Instant search must announce result counts politely and keep focus in the field (WCAG 2.2 status messages); chips need names and pressed states; row actions such as Join must not wrap (audit item 2); 44 px coarse-pointer targets stay.

## Open points for the task plan

- Which API reads supply each row summary (next or live lesson, open work count, last message time and unread count), with pagination, scope tests across two tutors and refusals, and whether the message summary needs a new field in [api.md](../specs/api.md).
- Whether instant search debounces requests (and by how long, on the injected scheduler) or filters only a server page, and how the address and Back keep the field and chips in sync as today.
- The exact chips and their address keys, and whether "No lesson" includes provisional and invited cards.
- Whether tab counts are drawn for all three tabs or only Active, and where they come from without loading every page.
- Notes as a side column (current specification) or as a tab (mockup), and how either keeps drafts across tab changes and the existing autosave rules.
- Whether Overview stays as a tab, given the mockup's summary tiles above the tabs and its tab set without Overview.
- The card's phone header: one primary action and the rest in a menu (audit item 7), while the phone redesign stays deferred to PHONE.
- Whether the Join button on a live row is shown only to the tutor of that lesson under the existing join gates, and what it shows when the window has not opened.

## Unverified

- TutorBird showing a Trial count in the Students page header: a search excerpt suggested it, but no page with that text was found.
- Whether TutorBird's or My Music Staff's profile notes (as opposed to lesson notes) are private to staff.
- Teachworks' exact list columns, tab names and filter set: its help centre refused automated fetches (HTTP 405), so its claims rest on search-result excerpts of its own articles [13]–[16].
- Wise and other tutor CRMs' student pages: no first-party help page describing them was found.
- Weekify's actual list and card layout: no public page shows them.

## Sources

1. TutorBird — "Sign Me Up!" (2018-10-05) — <https://www.tutorbird.com/?p=22677> — FP — checked 2026-11-27
2. TutorBird — Student Management — <https://tutorbird.com/student-management> — FP — checked 2026-11-27
3. TutorBird — "Here A Search, There A Search" (2014-11-24) — <https://www.tutorbird.com/here-a-search-there-a-search/> — FP — checked 2026-11-27
4. TutorBird — "It's Makeov-ember!" (2024-12-03) — <https://tutorbird.com/?p=71057> — FP — checked 2026-11-27
5. TutorBird — "February Fixes" (2023-02-10) — <https://www.tutorbird.com/?p=28650> — FP — checked 2026-11-27
6. TutorBird — "Grow, grow, grow your notes" (2023-03-22) — <https://www.tutorbird.com/grow-grow-grow-your-notes> — FP — checked 2026-11-27 — lesson notes only
7. TutorBird — "Just Call It Spring Cleaning…" (2026-05-06) — <https://www.tutorbird.com/?p=78818> — FP — checked 2026-11-27
8. TutorBird — "May this Month Bring You New Updates!" (2024-05-29) — <https://www.tutorbird.com/?p=68185> — FP — checked 2026-11-27
9. TutorBird — support centre — <https://support.tutorbird.com/> — FP — checked 2026-11-27 — client-rendered, no readable content
10. My Music Staff — "It's Makov-ember!" (2024-12-03) — <https://www.mymusicstaff.com/?p=75127> — FP — checked 2026-11-27
11. My Music Staff — "Gather 'round the (mobile) table" (2023-07-28) — <https://www.mymusicstaff.com/?p=64459> — FP — checked 2026-11-27
12. Teachworks — Notes & Communication Logs add-on — <https://teachworks.com/addons/notes-communications> — FP — checked 2026-11-27
13. Teachworks — Viewing lesson notes — <https://intercom.help/teachworks-e2d272c6e669/en/articles/15556454-viewing-lesson-notes> — FP — checked 2026-11-27 — page refused automated fetch (HTTP 405); search-result excerpt only; the child-student visibility claim is from the excerpt of <https://intercom.help/teachworks-e2d272c6e669/en/articles/11472281-viewable-profile-fields-for-each-profile-type>
14. Teachworks — Activating prospective client profiles — <https://intercom.help/teachworks-e2d272c6e669/en/articles/11472423-activating-prospective-client-profiles> — FP — checked 2026-11-27 — search-result excerpt only
15. Teachworks — deleting a student and setting it Inactive — <https://intercom.help/teachworks-e2d272c6e669/en/articles/11472291> — FP — checked 2026-11-27 — search-result excerpt only
16. Teachworks — Student auto inactive add-on — <https://intercom.help/teachworks-e2d272c6e669/en/articles/11472330-student-auto-inactive-add-on> — FP — checked 2026-11-27 — search-result excerpt only
17. Teachworks — "The Customer Journey Explained" (2024-12-18) — <https://blog.teachworks.com/2024/12/teachworks-tutoring-management-software-the-customer-journey-explained/> — FP — checked 2026-11-27
18. Weekify — robots rules — <https://weekify.ru/robots.txt> — FP — checked 2026-11-27 — still `Disallow: /` and `Allow: /$`
19. Weekify — App Store listing «Викифай» — <https://apps.apple.com/ge/app/%D0%B2%D0%B8%D0%BA%D0%B8%D1%84%D0%B0%D0%B9/id6762042825> — description FP, metadata TP — checked 2026-11-27
20. Weekify — RuStore listing — <https://www.rustore.ru/catalog/app/ru.weekify.weekify> — description FP — checked 2026-11-27 — answered HTTP 429, confirmed nothing
21. Google Classroom — Invite students to your class — <https://support.google.com/edu/classroom/answer/6020282?hl=en> — FP — checked 2026-11-27
22. Google Classroom — Remove a student or co-teacher from a class — <https://support.google.com/edu/classroom/answer/6069576?hl=en> — FP — checked 2026-11-27
23. Google Classroom — View all of a student's work in one place — <https://support.google.com/edu/classroom/answer/7451599> — FP — checked 2026-11-27
24. Google Classroom — About your personal information in Classroom by your role — <https://support.google.com/edu/classroom/answer/6386395?hl=en> — FP — checked 2026-11-27
