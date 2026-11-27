# Interface benchmark audit: where OVC's screens fall behind comparable products

> **Status:** agent research, checked on 2026-11-27. It decides nothing. Each pattern it proposes becomes a planned change only after the project owner chooses it, and each change then updates the specifications it names. Nothing here changes a specification, a decision record or `docs/`.
>
> **Scope:** the main screens of both roles and the public sign-in and sign-up pages, walked in the local demo on 2026-11-27 (frontend at `127.0.0.1:5173`, built from `main` at `db28015`). Each screen is compared with two or three products that solve the same job, and each gap gets a pattern to adopt, a severity, an effort and the files or specifications it touches.

## Method

- **Walk.** Every screen was opened in a browser at a 1280 × 800 viewport and at 360 × 780, as the tutor and as a linked student of the demo. Screenshots and page text were read only locally; no screenshot or application data went to an external service. The 1024 px width of the browser pane was also checked, because several layout faults appear only there.
- **Automated page probe.** On each screen a short script run in the page reported horizontal overflow, buttons whose text wraps into a narrow column, interactive targets under 24 × 24 px (WCAG 2.2 success criterion 2.5.8), clipped text and glyphs from symbol or private-use ranges that may render as a missing-glyph box. Findings from the probe are marked "(probe)".
- **Heuristics.** Without a product library connection, gaps are judged against Nielsen's ten usability heuristics (named "H1" to "H10" below: visibility of status, match with the real world, user control, consistency, error prevention, recognition over recall, flexibility, minimalist design, error recovery, help) and WCAG 2.2 AA, plus the [layout guidance of 2026-11-23](ui-guidelines-2026-11-23.md).
- **Comparisons.** Comparable products were researched from public sources on 2026-11-27; the list is at the end. "FP" means a first-party source (the vendor's own help, docs or blog), "TP" a third-party one. No account was created anywhere.
- **Limits.**
  - The demo holds one tutor, one linked student, one draft library item, one live lesson and no messages, issued work or submissions. The review queue, the student's work page, a filled chat and the issue page of issued work could therefore not be seen with content; their gaps are judged from [pages.md](../design/web/pages.md) and marked **unverified in the demo**.
  - Weekify publishes no help centre and its site asks crawlers to stay away ([Weekify research](weekify-2026-11-23.md)), so it is compared only through that earlier research. Finch's help centre shows article titles only.
  - Already known before this audit and not repeated as new findings: the library row button that wraps letter by letter (re-confirmed below, because it is still present at 1280 px), the week grid hiding night lessons (the grid now spans 00:00 to 23:00), the lesson status glyph and the blocked-lesson pop-up (the probe found no missing glyph on the lesson page or Today in this build), and the owner's decision to drop the notifications bell for a "needs attention" model.

### Scales

- **Severity:** *high* blocks or misleads a frequent task, or looks broken; *medium* costs extra clicks, misreading or polish on a frequent screen; *low* is polish on a rare screen.
- **Effort:** *S* is a frontend change on one page with no contract change; *M* touches several pages or a shared component, or needs a small specification change; *L* needs a new contract, backend work or a new interaction model.

## Top 15, ranked

Ranked by severity first, then by how many sessions the screen appears in, then by effort.

| # | Gap | Screen | Pattern to adopt (source) | Severity | Effort | Files and specifications |
|---|---|---|---|---|---|---|
| 1 | The tutor's Today and the student's Home do not lead with what needs attention. On a phone the setup checklist pushes the live lesson below the fold; the tutor code card takes the right column; the Notifications page is a read-only table whose item ("New lesson request") is already handled while Today says "No new requests". | Today, Home, Notifications | Replace the bell with a **needs-attention block** at the top of Today and Home: the live or next lesson with its Join button first, then up to five actionable rows (work to review, unread chats, pending requests, deadlines within 3 days), each opening its target; empty modules hide themselves; the setup checklist moves below or collapses. Google Classroom's homepage "Recently due" (at most 5, hidden when empty) and "To review"; Linear's Priority inbox. | high | M | `features/home`, `features/notifications`, the frame's sidebar and phone header; [pages.md](../design/web/pages.md) Today, Home, Notifications; [states.md](../design/web/states.md) Notifications; [navigation.md](../design/web/navigation.md) Frames |
| 2 | Table cells break words. Library's "Open" button wraps into a 45 × 156 px column at 1024 and 1280 px (probe); the Students header "Open work" breaks mid-word ("Открыты / е задания") and emails wrap onto two lines at 1280 px. | Library, Students | **List rows with a fixed action column**: actions never wrap (`white-space: nowrap`, a minimum column width), the whole row opens the item, long values truncate with an ellipsis and a tooltip, and headers stay one line or abbreviate. Linear and Notion list views. | high | S | `features/library`, `features/students`, the shared table component under `frontend/src/components/ui/`; [tokens.md](../design/web/tokens.md) table rules |
| 3 | A live lesson reads as "nothing planned" elsewhere. While the lesson is live, the student card says "No lessons scheduled", the Students list says "No lesson scheduled" and the student's Home heads it "Next lesson". | Student card, Students, Home | **One lesson-status vocabulary everywhere**: a lesson in progress counts as the current lesson, labelled "Live now" with Join, in every list that shows "next lesson". Preply shows Join on Home and My lessons as soon as the lesson starts. | high | S | `features/students`, `features/home`; [states.md](../design/web/states.md) Lessons and Home pages; the API field behind "next lesson" in [api.md](../specs/api.md) if it excludes live lessons |
| 4 | Joining a lesson has no lobby. The lesson page goes straight to the room with no device check, no "who is already in" and no countdown in the join block. | Lesson | **Pre-join lobby**: camera and microphone preview with device pickers, a microphone level meter and a speaker test, then one "Join" button; before the window opens, a disabled Join with "Opens at 18:50, in 12 min" instead of a bare state line. Google Meet's green room; Preply's lobby before "Enter classroom". | high | M | lesson feature under `frontend/src/features/schedule`; [pages.md](../design/web/pages.md) Lesson (video area and join states); [realtime-and-video.md](../specs/realtime-and-video.md) |
| 5 | The week grid is hard to scan. Each hour is about 48 px, so a one-hour block shows only its time and the title is cut; there is no current-time line; day headers wrap onto two lines; and because New lesson defaults the title to "Lesson", every block reads "Урок". | Schedule | **Calendar blocks that name the person**: default the lesson title to the student's or group's name, show "name · time" in one line on short blocks, add a red now-line and scroll the grid to it on open, keep day headers on one line ("пт 27"). Preply's calendar ("the red line indicates the current time"); Google Calendar's compact event chips. | high | M | `features/schedule`; [pages.md](../design/web/pages.md) Schedule and New lesson; [states.md](../design/web/states.md) Lessons |
| 6 | Messages at 1024 px leaves the conversation about 400 px wide beside a 240 px sidebar and a 300 px channel list; on a phone the text "Send" button takes a quarter of the composer row and the student card link takes a row of its own under the header. | Messages | **Messenger proportions**: collapse the app sidebar to icons while Messages is open below 1280 px; on phones an icon send button that replaces the attach button once text is typed; the header itself opens the card or group page. Telegram Web and Telegram's mobile apps. | medium | M | `features/messages`, the frame; [pages.md](../design/web/pages.md) Messages (computer layout, phone) |
| 7 | Page headers on phones spend two rows on actions. The student card, Library and the lesson page wrap their header buttons onto a second line, mix a text link ("Messages") with buttons, and put the back link under the actions. | Student card, Library, Lesson, phone | **Phone top bar**: back arrow and title in the bar, one primary action, the rest in a kebab menu; on computers the back link above the heading. Telegram and Android top app bars; GOV.UK back link placement. | medium | M | the page header component; [navigation.md](../design/web/navigation.md) Frames; [pages.md](../design/web/pages.md) Shared behavior |
| 8 | Creating a lesson from the grid leaves the grid. Clicking a free slot keeps its date and time but opens the full New lesson page; lessons cannot be dragged to create or move. | Schedule | **Quick-create popover** on a slot (audience, title, duration, "More options" for the full page) and drag to move with the series scope question. Google Calendar quick create and drag; Preply drag-to-reschedule. | medium | L | `features/schedule`; [pages.md](../design/web/pages.md) Schedule; [schedule.md](../specs/schedule.md) for move semantics |
| 9 | The builder warns before the tutor has done anything ("1 issue", "Add at least one block"), its heading is the generic "Task builder" while the real name sits in a field, and the block menu lists eight types as plain words with no icons, hints or grouping. | Builder | **Title as heading and a typed block menu**: the editable task name is the page heading; validation shows when the tutor tries to issue, not on an empty draft; the menu groups "Content" (instruction, material) and "Questions checked automatically" vs "Questions you check", each with an icon and a one-line hint. Google Forms (title field as heading, icon per question type); Edvibe's categorised exercise templates. | medium | M | `features/builder`; [pages.md](../design/web/pages.md) Builder; [states.md](../design/web/states.md) Library and builder |
| 10 | Homework labels disagree. The nav says "Tasks", the first tab and its heading say "Library", a folder control repeats "Library"; the top buttons "New task" and "New template" do not say how they differ; the third tab "Under review" opens a page headed "Need to review". | Library, Issued, Review | **One noun per object and one Create menu**: a single "Create" button with "Task with questions" and "Simple task" entries and a hint each; tab names equal page headings. Google Classroom's Classwork page with one Create menu. | medium | S | `features/library`, `features/review`, Russian texts; [pages.md](../design/web/pages.md) Library, Review queue; [states.md](../design/web/states.md) Homework texts |
| 11 | Students search needs a "Find" button, the Status column is empty for an active student, and there are no filters. | Students | **Instant filter** while typing, with filter chips (group, has open work, no lesson booked). Linear and Notion list filters. | medium | S | `features/students`; [pages.md](../design/web/pages.md) Students |
| 12 | The student card repeats itself: the email twice, "Schedule lesson" in the header and "New lesson" under "Upcoming lessons"; there is no money view. | Student card | **Card summary row** (name, contact, next lesson, open work, balance) and one action per intent; the payment status per lesson arrives with the finance stage. Finch's student card with price and payment status (prepaid, partial, full). | medium | S (M with finance) | `features/students`; [pages.md](../design/web/pages.md) Student card; the finance stage plan |
| 13 | The phone schedule spends three rows (title, view toggle, navigation) before the first lesson, hides empty days and offers no day strip. | Schedule, phone | **Day strip with dots plus an agenda list**, and the view switch in an overflow. Google Calendar's mobile Schedule and 3-day views. | medium | M | `features/schedule`; [pages.md](../design/web/pages.md) Schedule (phone) |
| 14 | Reviewing work, judged from the specification, has no next-student flow, comment bank or bulk return. **Unverified in the demo** (no submissions). | Review queue | **Grading navigation**: status groups (to review, assigned, reviewed, returned), "Next" and "Previous" between submissions, a "#" comment bank of recent feedback, a bulk return. Google Classroom's grading tool; Edvibe's per-exercise accept or redo. | medium | L | `features/review`; [pages.md](../design/web/pages.md) Review queue and review; [homework.md](../specs/homework.md) |
| 15 | Sign-in is a narrow form on an otherwise empty page, "Show password" is a separate checkbox, and sign-up asks for the interface language although the page header already has a language switch. | Sign in, Sign up | **Eye toggle inside the password field**, a short statement of what OVC is beside the form on computers, the language taken from the header switch. Email-code or passkey sign-in (Linear, Notion) is an option for later. | low | S (L for passwordless) | `features/auth`; [pages.md](../design/web/pages.md) Sign in, Sign up |

## Owner's choices of 2026-11-27

After reading the top 15, the project owner chose the following on 2026-11-27. They decide direction only; each becomes a planned change with its own plan, and the specifications named in the top 15 change in that change (pending owner review there). Section 3 of the umbrella redesign plan, `knowledge/plans/redesign.md` (revision 3, approved by the owner on 2026-11-27 as a sequencing plan), records the same decisions with further constraints; where the two differ, the plan and the notifications analysis it cites govern.

- **Notifications (top 15 #1):** the model of Google Classroom and Telegram, recorded as direction A in `knowledge/analysis/notifications-rethink-2026-11-27.md` and item 8 of the redesign plan. The bell, the notifications panel, the `/notifications` page and the global unread counter are removed. Open actions live in a needs-attention block on the tutor's Today and the student's Home (the lesson with Join, work to check or to do, unread chats, pending join requests, deadlines), counts of open things stay on the navigation items (Homework, Chats) and pending join requests gain a count on Students; Cancelled and Moved lesson marks and a 14-day Recent updates list have no read state. Rejected: an actionable inbox page as in Linear and Notion, and both together.
- **Default lesson title (top 15 #5):** stays "Урок" ("Lesson"). The week grid's legibility is to be improved without changing the default; how is left to the schedule change's own plan (showing the student's or group's name on the block is one candidate, not a decision).
- **Creating homework (top 15 #10):** one "Create" button with "Task with questions" and "Simple task" entries, and the section is called "Tasks" everywhere. Per item 15 of the redesign plan the entries are presets into the existing mixed-question flow, and every builder block type is retained.
- **Joining a lesson (top 15 #4) and sign-in (top 15 #15):** a pre-join lobby with a camera, microphone and speaker check before Join; per item 13 of the redesign plan the preview stays local, joining keeps the current server gates, and the lobby shows only lesson and room state the client already holds (live, starting soon, ended), never named participant presence. Passwordless sign-in is deferred, and sign-in gets only an eye toggle inside the password field for now.

## Per-screen detail

Each screen lists what was seen, how two or three comparable products solve it, and the gaps with their proposals. Gaps already in the top 15 are referenced by number.

### Tutor: Today

**Seen (1280, 1024, 360 px):** heading and date; the setup checklist ("Where to start") with three crossed-out steps and one open step; "Lessons today" with the live lesson card and "Join again" under the card; "Under review 0" with a link to the queue; a right column with "Lesson requests" and the tutor code card ("Copy code"), "Upcoming deadlines" and "Unread messages". On a phone the checklist fills the first screen and the live lesson starts at about 540 px.

**Others:** Google Classroom's 2026 teacher homepage shows "Recently due" (up to 5 assignments due in the last 7 days that still need review), "Class insights" cards and classes, and hides empty modules (FP). Its To-do page has "To review" with per-assignment counts (FP). Linear replaces a bell with an Inbox and a default "Priority" tab (FP). Finch promises lesson reminders and a monthly income and debt summary (FP, layout not verifiable).

**Gaps:**
- Top 15 #1 (needs-attention block, checklist below the lesson).
- The tutor code card is a once-a-month action in a permanent column; Classroom keeps the class code on the class page. Move it into the "Add student" flow and Settings → Teaching, keeping a small link. *Medium, S.* `features/home`; pages.md Today.
- The checklist counts "Create a task" as done while the only library item is "Not ready". Tick it only when a task is ready or issued (H1). *Low, S.* pages.md Today; states.md Home pages.
- The lesson card repeats today's date ("пт, 27 нояб.") under the heading "Lessons today". Show time only. *Low, S.*

### Tutor: Schedule and New lesson

**Seen:** week view from 00:00 to 23:00 with 30-minute click targets (hidden from assistive technology, so not a WCAG issue), "Week" and "Month" toggles, Today, previous and next. The live lesson block shows "00:35–01:35" and a cut title. Clicking a slot opens `/schedule/new` with the date and time filled. New lesson is a full page; its title defaults to "Урок". At 360 px the week becomes an agenda list of days that have lessons.

**Others:** Google Calendar: click a slot for a quick-create box, drag to extend or move, "This event / This and following events / All events" for series, keyboard views 1–5 and `c` to create, mobile Schedule, Day, 3-day, Week and Month views (FP). Preply: a red current-time line, drag to reschedule with the student notified, weekly slots locked as a series and a one-off reschedule detaching one lesson (FP). Finch: day, week and month load views and a public self-booking link that shows only free slots (FP).

**Gaps:** top 15 #5, #8, #13. Also:
- No keyboard shortcuts for views and today. Add `t`, `←`/`→`, `w`, `m` with a shortcut list on `?`. Google Calendar. *Low, S.* `features/schedule`; pages.md Shared behavior.
- Self-booking of free slots is the booking stage's scope; the benchmark confirms Finch offers it. *No new gap; stays with that stage.*

### Tutor: Lesson page

**Seen:** heading "Урок", "Messages" link and an actions menu; back link "To the schedule"; status "Live now" and "Join again"; Materials with "Attach material"; Tasks with both "Issue task" in the section header and "Issue for this lesson" in the empty state; "About the lesson" on the right (date, format, student). At 360 px "About the lesson", which carries the date and time, falls to the bottom.

**Others:** Google Meet's green room (camera, microphone and speaker preview with pickers and a test sound; "Join now") and host controls such as "host must join first" (FP). Preply: Join disabled until the lesson is near, a lobby to check audio and video, then "Enter classroom"; the classroom link also in chat, calendar events and reminder emails (FP).

**Gaps:** top 15 #4. Also:
- Two buttons for one action ("Issue task" and "Issue for this lesson"). Keep the section header button and make the empty state text only. *Medium, S.* pages.md Lesson.
- On phones put the date, time and audience under the heading, before Join. *Medium, S.* pages.md Lesson (phone).
- The generic title "Урок" as the page heading (see top 15 #5 for the default title).

### Tutor: Students and the student card

**Seen:** heading, "Add student", tabs "Active / Invited / Archive", a labelled search field with a "Find" button, a table (name, email, status, groups, next lesson, open work). The card: name, "Messages" link, "Schedule lesson", "Issue task", a menu; "All students" back link under the actions; avatar and email, then "Student account" with name and email again; tabs "Overview / Lessons / Tasks"; "Upcoming lessons" (empty while the lesson is live) with "New lesson"; "Open tasks"; private notes.

**Others:** Finch stores name, contacts, price and progress per student, payment status per payment and automatic payment reminders, and links the card to the student's chat (FP, article titles only). Preply's Students page lists scheduled lessons and gives a "···" menu with a camera icon that enters the classroom with that student (FP). Weekify's student card holds lessons, homework and finances ([Weekify research](weekify-2026-11-23.md)).

**Gaps:** top 15 #2, #3, #7, #11, #12. Also:
- The back link sits under the action buttons (H4, layout guidance 1.3). Move it above the heading. *Medium, S.* pages.md Shared behavior.

### Tutor: Groups

**Seen:** empty state "No groups yet", one sentence and "New group".

**Others:** Google Classroom student groups are created from People with a name and checkboxes and are offered in the assign flow's "Groups" tab (FP). Finch has group lessons, a group calendar and group income and debt (FP).

**Gaps:**
- The empty state says what a group is but not what it gives (one lesson for several students, a group chat, homework issued to all). Add the three benefits and a second action "Create from students". Classroom's assign-to-group flow. *Low, S.* `features/groups`; pages.md Groups.

### Tutor: Library, Builder and Issue

**Seen:** Library heading with "New task", "New template" and a menu; tabs "Library / Issued / Under review"; a folder link or select "Library"; search; a table (name, details "0 questions / 0 points", state "Not ready", changed, "Open" and a row menu). The Open button wraps at 1024 and 1280 px (probe). At 360 px the header buttons wrap to two rows. Builder: heading "Task builder", back link "Library", name field, "Saved", "1 issue", "Preview", a warning "Add at least one block", "Add block" with eight plain entries. Issue page: task search, student or group, optional deadline, answer visibility (never, after submission with a warning, after review), "Issue" and "Cancel".

**Others:** Google Forms quizzes: an answer key and points on each question type, "Release grades" immediately or after manual review, preview in a new window, a drag handle and keyboard reorder for questions (FP). Edvibe's lesson builder: 31 exercise templates in categories and a preview "exactly as your students will experience them" (FP). ProgressMe: "···" → "To homework" on an exercise (FP, snippet).

**Gaps:** top 15 #2, #9, #10. Also:
- Keyboard reorder of blocks (Ctrl/Cmd+Shift+↑/↓) next to drag. Google Forms. *Low, S.* pages.md Builder.
- The Issued empty state links only to the Library; offer "Issue task" directly. *Low, S.* pages.md Issued work.

### Tutor: Issued and Review

**Seen:** Issued with "Active / Withdrawn" and an empty state; Review with a group filter, "Under review 0" and an empty state. No submissions exist in the demo.

**Others:** Google Classroom: students grouped "To review / Assigned / Graded / Returned", "Back" and "Next" between submissions, a "#" comment bank, private comments with recordings, draft grades until "Return", bulk "Return", "Mark as missing", "Excuse" (FP). Edvibe: "New → Pending → Completed", the tutor accepts correct exercises and returns the rest until all are correct (FP).

**Gaps:** top 15 #14 (unverified in the demo).

### Tutor and student: Messages

**Seen:** a channel list with a labelled search field and one row showing only the name (the chat has no messages, so the specified preview, time and unread count could not be seen); "Choose a chat" on the right. The conversation shows the name, a "Student card" link on its own line, a search button, "No messages yet" and a composer with attach, text and "Send". At 1024 px the conversation is about 400 px wide. At 360 px list and conversation are separate screens with Back.

**Others:** Telegram: one check for sent and two for read, pinned chats, folders as tabs over the list, long-press reactions, quote replies, universal search (FP). Finch: chat for materials, homework and reschedules (FP). Preply: "Enter classroom" in the chat sidebar (FP).

**Gaps:** top 15 #6. Also:
- The empty conversation gives no starter. Offer the next lesson's time and "Send the lesson link" or "Send a task" as chips. Preply's classroom entry in chat. *Low, S.* pages.md Messages.
- Pinned chats and a group/direct filter over the list are missing. Telegram pins and folders. *Low, M.* pages.md Messages; [realtime-and-video.md](../specs/realtime-and-video.md) if pins are stored.
- Naming: the phone tab bar says "Chats", the sidebar and heading say "Messages" (H4). Use one word. *Low, S.* [navigation.md](../design/web/navigation.md) Frames.

### Tutor and student: Settings

**Seen:** tabs "Profile / Account / Notifications / Teaching / My data" (the student has no Teaching). Account: language, a time zone search field above a separate select of about 400 zones, theme, one "Save" for all three, "Change password"; account deletion sits in a "···" menu at the top right. Notifications: three optional email switches with no reminder lead time. Teaching: tutor code, storage use, blocked students. My data: the export request.

**Others:** Notion: account section with "Delete my account" at the bottom of account security, preferences with language, "Start week on Monday", time zone with "Automatically update time zone" and appearance; deletion asks for the email and suggests an export first (FP). Linear: notifications per channel, digest email skipped when the item was read in the Inbox, settings search with Cmd/Ctrl+K (FP).

**Gaps:**
- One combobox for the time zone with the detected zone suggested, instead of a search field plus a separate select (H6, H8). Notion's automatic time zone. *Medium, S.* `features/settings`; pages.md Settings.
- Theme applies at once without "Save"; Notion and Linear apply appearance immediately. *Low, S.*
- Account deletion is hidden behind "···". Put a "Delete account" section at the bottom of Account with the export suggestion. Notion. *Medium, S.* pages.md Settings.
- Lesson reminder emails have no lead time choice (for example 15 minutes, 1 hour, 1 day). Finch's reminders; Google Calendar's event notifications. *Low, M.* [notifications.md](../specs/notifications.md).

### Student: Home

**Seen:** heading and "1 lesson today"; "Next lesson" with the live lesson and Join; "To do" and "Recently reviewed" empty states; on the right "Next seven days" repeating the same lesson and "My tutors" with "Enter tutor code".

**Others:** Google Classroom's student "Due soon" (up to 5 within 7 days, hidden when empty) and To-do filters "Assigned / Missing / Done" (FP). Preply shows the lesson balance at the top and Join on Home (FP).

**Gaps:** top 15 #1 and #3. Also:
- The same lesson appears twice on the first screen (left and right). Show the week list from the second lesson on. *Low, S.* pages.md Home.

### Student: Homework and the work page

**Seen:** tabs "To do / Submitted / Reviewed" with an empty state. No work was issued in the demo, so the work page was not seen.

**Others:** Google Classroom: "Your work" with attachments, an optional private comment, "Turn in" with a confirmation, "Unsubmit", "Missing" and "Done late" states (FP). Edvibe: accepted exercises on green, ones to redo on white (FP).

**Gaps (unverified in the demo):**
- Check that the work page shows a "Missing" or "Late" state after the deadline and lets the student take a submission back before review, as Classroom does; pages.md Homework (student) defines the states, so this is a check, not yet a gap.

### Student: My tutors

**Seen:** a table with the tutor's avatar, a coloured dot, the name, "Messages" and a menu; beside it "Add tutor" with the code field and a filled "Send request" button. At 360 px the add form fills the screen under the only tutor.

**Others:** Preply keeps a balance per tutor and lets the student pause a subscription (FP). No product with a page literally named "My tutors" was found.

**Gaps:**
- The coloured dot next to the tutor's name has no legend (H6, WCAG 1.4.1 if colour is the only cue). Label it or drop it here. *Low, S.* pages.md My tutors.
- "Add tutor" with a filled primary button outweighs the tutors themselves. Make it a secondary "Add tutor" button that opens the form. *Low, S.*

### Public: Sign in and Sign up

**Seen:** sign-in with email, password, a "Show password" checkbox, "Sign in", "Forgot password?" and "Sign up", left-aligned on an empty page; sign-up first asks "I teach / I study", then name, email, password (at least 10 characters), the interface language, the consent checkbox and the terms line.

**Others:** Linear: Google, an email with a link and a code, passkeys, no password (FP). Notion: email code, email and password, Google, Apple, passkeys, and a plan to phase out password-only login without two-step verification (FP). Preply registers at the moment of booking, with Google or Facebook beside email (FP).

**Gaps:** top 15 #15. Passwordless sign-in and third-party sign-in providers are product and threat-model decisions (external identity providers would have to keep data in Russia, ADR-0008), not interface fixes.

## Cross-cutting observations

- **Consistency of status words (H4):** "Under review" and "Need to review", "Messages" and "Chats", "Next lesson" for a live lesson. A short glossary of user-facing nouns in [states.md](../design/web/states.md) would stop these recurring.
- **Duplicate actions:** the student card, the lesson page and the library each offer one action twice. The layout guidance's "one primary action per view" applies.
- **Touch targets:** the probe found no target under 24 × 24 px except the visually hidden skip link and a native checkbox whose label is clickable; 2.5.8 holds on every walked screen.
- **Overflow:** no screen scrolls horizontally at 360, 1024 or 1280 px.

## Sources

All checked on 2026-11-27.

- Google Classroom homepage (2026): https://support.google.com/edu/classroom/answer/17231999 (FP); https://workspaceupdates.googleblog.com/2026/07/redesigned-google-classroom-homepage-with-tailored-views-based-on-users-role.html (FP)
- Google Classroom To-do and grading: https://support.google.com/edu/classroom/answer/9157286, https://support.google.com/edu/classroom/answer/16643267, https://support.google.com/edu/classroom/answer/6020294, https://support.google.com/edu/classroom/answer/6020284, https://support.google.com/edu/classroom/answer/6020285 (FP); student groups https://support.google.com/edu/classroom/answer/15263790 (FP)
- Google Forms quizzes and editing: https://support.google.com/docs/answer/7032287, https://support.google.com/docs/answer/3527854 (FP)
- Google Calendar: https://support.google.com/calendar/answer/72143, https://support.google.com/calendar/answer/37113, https://support.google.com/calendar/answer/37034, https://support.google.com/calendar/answer/6110849 (FP)
- Google Meet: https://support.google.com/meet/answer/10409699, https://support.google.com/meet/answer/9302870 (FP)
- Preply: https://help.preply.com/en/articles/4175340-how-to-use-preply-calendar-and-connect-google-calendar, https://help.preply.com/en/articles/16598169-how-weekly-lessons-work-for-tutors, https://help.preply.com/en/articles/4984773-starting-and-teaching-lessons, https://help.preply.com/en/articles/4182666-preply-classroom-a-student-s-guide, https://help.preply.com/en/articles/13924649-how-to-schedule-a-lesson (FP)
- Linear: https://linear.app/docs/inbox, https://linear.app/changelog/2026-09-03-priority-inbox, https://linear.app/docs/notifications, https://linear.app/docs/login-methods (FP)
- Notion: https://www.notion.com/help/account-settings, https://www.notion.com/help/delete-your-account, https://www.notion.com/help/log-in-and-out, https://www.notion.com/help/notification-settings (FP)
- Telegram: https://telegram.org/faq, https://telegram.org/blog/reply-revolution, https://telegram.org/blog/folders (FP)
- Finch: https://finch.study, https://www.finch.study/finchhelp (FP; the help centre shows titles only)
- Edvibe: https://intercom.help/edvibe/ru/articles/10293936, https://intercom.help/edvibe/ru/articles/8001337 (FP)
- Weekify: [Weekify research of 2026-11-23](weekify-2026-11-23.md); nothing new could be verified on 2026-11-27.

## Not verified

- The review queue, the student's work page, a chat with messages and the issue page of issued work, because the demo had no such data.
- Whether Google Calendar scrolls to the current time and how it lays out overlapping events; no first-party page documents either.
- Weekify's and Finch's actual screens.
