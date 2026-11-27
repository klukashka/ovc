# Today and Home: reference study, checked 2026-11-27

Status: agent research, checked 2026-11-27 from public first-party sources; decides nothing. Product behavior is volatile; recheck before relying on it. Supplies the reference study that the AGENTS.md UI work rule requires for R4a `feat/redesign-today` and R4b `feat/redesign-student-home` of the umbrella redesign plan `knowledge/plans/redesign.md` (revision 3 at commit `8b3a8b0`, approved by the owner on 2026-11-27 as a sequencing plan). "FP" marks a first-party source, "TP" a third-party one; numbers refer to the source list at the end.

## Scope and OVC baseline

The screens are the tutor's Today (`/today`) and the student's Home (`/home`): the first page after sign-in for each role. The study covers five design questions: how the live or next lesson and its Join control lead the page; how open actions are gathered in place of a notification bell; which counts the navigation carries; how a short history of changes is shown without a read state; and how the coming days are listed.

OVC today, per [pages.md, Today](../design/web/pages.md#today) and [pages.md, Home](../design/web/pages.md#home):

- Today reads one `GET /home` and shows the first-run checklist, today's lessons with Join controls (new tab only, "Room opens at {time}" before the window), Needs checking (five oldest, "Open review queue"), join requests with Accept, Decline and a confirmed "Decline and block", the tutor code with Copy, Upcoming deadlines (three days, "{submitted}/{total} submitted") and up to five unread channels. Main column plus a 300 px side column.
- Home shows the tutor filter (two or more tutors), the next lesson with Join only while `live` with `room_open`, To do (five rows, Start or Continue), Recently checked with the "New" feedback badge, the next seven days (cancelled lessons left out), My tutors and the no-tutor and provisional states.
- The bell, its panel and the `/notifications` page are specified in [pages.md, Notifications](../design/web/pages.md#notifications); section counts (Homework to check or to do, Messages unread) in [navigation.md, Frames](../design/web/navigation.md#frames).
- Components: `frontend/src/features/home/` (`TodayPage.tsx`, `HomePage.tsx`, `TodayLessons.tsx`, `HomeLessons.tsx`, `TutorHomeworkPanels.tsx`, `TodoPanel.tsx`, `CheckedPanel.tsx`, `UnreadMessages.tsx`, `FirstRunChecklist.tsx`, `TutorFilter.tsx`) and `frontend/src/features/notifications/`.

Owner's decisions that bind this study (plan section 3 at `8b3a8b0`):

- Item 8: no bell, panel, `/notifications` page or global counter; actionable items live in a needs-attention block on Today and Home; Cancelled and Moved lesson marks and a 14-day Recent updates list have no read state; pending join requests count on Students. The direction and its owner decision are recorded in `knowledge/analysis/notifications-rethink-2026-11-27.md` at `3f7eec8` (direction A).
- Item 14: initials and existing authorized avatar fields only. Item 15: Recently checked, My tutors and every other omitted Home feature stay. Item 16: one primary action, destructive actions in confirmed menus, no internal identifiers, the tutor code stays. Item 13 limits room state to what the client already holds (live, starting soon, ended), never named participant presence.

Mockups (static, non-normative): `knowledge/design/web/prototype/redesign/today.html` and `student-home.html` at `8b3a8b0`. Today draws a greeting with "3 lessons today", a "Live now, 34 min left" panel with the student's initials and "Join lesson", a "Needs attention" block (a join request with Decline and Accept, "3 works await checking, oldest 2 days", "3 unread messages", "Due tomorrow: {title}, 2 of 5 submitted"), "Recent updates" (Cancelled, Moved with times), a today agenda with "Schedule" link and an "Invite a student" card with the code and Copy. Home draws the tutor filter, a "Live now" panel with "Join", "To do" (three rows with state word, deadline and Continue, Fix or Start), "Unread messages" with an avatar row and a message preview, "Recent updates" (Moved, Cancelled), "Next 7 days" and the timezone note.

## How the compared products solve it

### Google Classroom

- Since a redesign rolling out from 2026-07-27 the homepage has role views (Teaching, Enrolled, Admin), reopens to the last view used, and all its modules are collapsible [2]. The help page says the homepage defaults to the role used most, remembers collapsed modules, and hides a module with no current content [1].
- Teaching view: "Recently due" lists up to 5 assignments due in the last 7 days that need review; "Class insights" cards flag missing work, low grades and inactive students; then a feature spotlight and class cards [1]. From Recently due, "View work in progress" opens the complete list, filterable by class; a "To review" module holds assignment cards with a checkmark that moves them to a Reviewed list [3]. Teacher statuses "To review" and "Assigned" are defined on the grading page [4].
- Enrolled view: "Due soon" lists up to 5 assignments from all classes due within 7 days [1, 5]. The To-do page, reached from "View to-dos" or the Due soon module, filters Assigned, Missing and Done; work not turned in is marked Missing or Done late as soon as the due time arrives [5].
- Notifications go out by email and device notifications with per-type and per-class switches; students get a reminder for unsubmitted work due within 24 hours; an in-app list of the past 30 days exists in the Android app [6]. The page mentions no web bell [6].

### Linear

- The Inbox collects notifications on subscribed issues (created, assigned, mentioned) [7]. A Priority tab separates notifications that need attention from other updates; Linear selects them by default and users can change the filter [7, 8].
- Items are read or unread (`U`, `Alt+U` for all), deleted (`Backspace`, `Shift+Backspace` for every read item) or snoozed (`H`) until a chosen time; at most 2,000 open notifications are kept [7, 10]. The page does not describe automatic clearing when an issue is done [7].
- My Issues is a separate computed view: Assigned grouped in a focus order from urgent to completed, plus Created, Subscribed and Activity tabs; `G` then `M` opens it [9].
- Email digests are sent only if the inbox notification has not already been read, delayed by urgency and issue status [10].

### Preply

- A student joins from My lessons or Home with "Join lesson", which is inactive and becomes clickable when the lesson starts soon or has started; the lesson page does not say how many minutes before [11]. The same personal classroom link also appears in Messages ("Enter classroom" in the sidebar), on the student homepage, in calendar events and in lesson emails and reminders [11].
- Joining passes through a lobby where the student tests audio and video, then "Enter classroom" [11]. The tutor can preview video and "see if the student is already there or not" before joining [13].
- The tutor enters the classroom from a "Classroom" button, from the Students list ("···" then a camera icon) or from the conversation's "Enter classroom" [12].
- The student's remaining lesson count stands at the top of the screen on desktop, totalled across tutors with a per-tutor breakdown on selection [14].

### Slack (re-check of the inbox model)

- Activity gathers messages and notifications from channels, DMs and canvases, with a numbered badge of unread notifications; filters include Unreads, Mentions, Threads, Invitations, Reminders and Cleared [15].
- Items can be cleared (`C`, swipe left on mobile) into the Cleared filter; replying marks a message read; clearing an unread item also marks it read [15].

### GitHub notifications (contrasting inbox model)

- Done removes a notification from the inbox and is separate from read; Saved items are kept indefinitely and others for 3 months; each row carries a reason label such as `review requested`; unsubscribing removes the thread [16]. The page describes no automatic Done [16].

### Microsoft Teams Activity (contrasting feed model)

- Activity lists mentions, replies, reactions, team additions and meeting updates with type symbols, an Unread filter and "Mark all as read"; items expire after 30 days [17].
- The Activity badge counts new items since the last visit and clears when the Activity app is opened; Chat and Teams counts add 1 per unread chat or channel mention, muted chats excluded [18].

### Telegram (re-check of per-object unread state)

- Chat folders can include or exclude chat types, including "Unread"; an archived chat that receives a notification returns to the main list [19]. The page describes no global feed [19].

## Comparison

| Design question | Google Classroom | Linear | Preply | Slack / GitHub / Teams | OVC decision or target |
|---|---|---|---|---|---|
| What leads the page | Role view; Recently due or Due soon first [1] | Inbox or My Issues, user's choice [7, 9] | Join lesson on Home and My lessons [11] | Activity feed [15, 17] | Live or next lesson with Join first (mockups; plan section 4 "Today/Home") |
| Where open actions live | Computed modules capped at 5, hidden when empty [1] | Inbox Priority tab, triaged by hand [7, 8] | Not documented beyond Join | Inbox with Clear or Done by hand [15, 16] | Needs-attention block computed from state, clears when handled (item 8) |
| Join timing and path | Not applicable | Not applicable | Inactive until soon, then lobby [11] | Not applicable | Existing join window and gates; new tab; lobby is R9 (item 13) |
| Global unread counter | None on web documented [6] | Dock badge only documented [10] | Not documented | Numbered badge on Activity [15, 18] | None (item 8) |
| Counts on navigation | Not documented | Not documented for the sidebar [9] | Lesson balance at the top [14] | Per chat and channel [18] | Homework, Messages, and Students for pending requests (item 8) |
| History of changes | Android 30-day list [6] | Inbox up to 2,000 [7] | Not documented | 3 months (GitHub) [16]; 30 days (Teams) [17] | Recent updates, 14 days, no read state (item 8) |
| Read state | Not applicable | Read, unread, delete, snooze [7] | Not documented | Read plus Clear or Done [15, 16] | None; marks on objects (Cancelled, Moved, "New" feedback) |
| Coming days | Due soon, 7 days, 5 items [1] | Not applicable | Upcoming lessons in the classroom Info tab [13] | Not applicable | Today's agenda (tutor), next seven days (student) |
| Empty modules | Hidden [1] | Not applicable | Not documented | Not applicable | Open point (see below) |

## What OVC adopts

- **A needs-attention block computed from state, capped and self-clearing** → Classroom's Recently due and Due soon (at most 5, hidden when empty) [1] and To review [3]. It maps to item 8 and to R4's acceptance criteria "attention actions use the existing scoped routes": each row opens the existing review queue, join requests, conversation or issue page, and leaves the block when the underlying state changes, with the expiry and removal focus tests R4 requires.
- **The lesson and Join as the page's lead** → Preply's Join on Home that activates only near the lesson [11]. OVC keeps its own join window, `room_open` gate and new-tab rule, so R4's criterion "Join gates survive" holds; only the main action is filled.
- **Counts on the object, not a global badge** → Teams' per-chat counts [18] and Telegram's per-chat unread model [19]; Classroom documents no web bell [6]. Maps to item 8's Students count and the existing Homework and Messages counts.
- **A short, expiring history without triage** → the retention ideas of Teams (30 days) [17] and GitHub (3 months) [16], shortened to 14 days and stripped of read state, because the triage verbs of Linear [7], Slack [15] and GitHub [16] were the cost the owner rejected (item 8; the benchmark audit at `512765d` records "an actionable inbox page" as rejected).
- **Separate "what needs me" from "what happened"** → Linear's Priority tab versus other updates [8] and My Issues versus Inbox [9]; in OVC the split is two blocks on one page (needs attention, Recent updates) rather than two tabs.
- **One agenda per role** → Classroom's 7-day Due soon for students [1]; OVC keeps today's agenda for the tutor and the next seven days for the student, with Cancelled and Moved marks on the rows (item 8).

## Where OVC differs and why

- **No triage actions at all.** Linear, Slack, GitHub and Teams all keep read, clear, done or mark-all verbs [7, 15, 16, 17]. Item 8 removes read state, so Recent updates has no row action other than its link and R4 adds no mark-read write.
- **No message previews on Today.** The Home mockup shows a message excerpt under the unread row; the current Today panel shows "never a message preview" ([pages.md, Today](../design/web/pages.md#today)). Message bodies are personal content; keeping previews out is the safer default unless the task plan explicitly changes it.
- **No named presence.** The mockups' "Ivan is already in the room" and Preply's "see if the student is already there" [13] need a participant roster. Item 13 allows only client-held room state; Home's existing "The room opens when your tutor joins" comes from `room_open`, not a roster, and is the closest permitted wording.
- **No lobby on Today or Home.** Preply routes Join through a lobby [11]; in OVC the lobby is R9's lesson page, and BigBlueButton owns the room, so R4 keeps the existing new-tab Join.
- **Relationship scope.** Classroom aggregates across classes [1]; OVC aggregates only within the caller's relationships (ADR-0005), and the student's attention rows never name other students. Join request rows and the Students count obey the existing hold and suppression rules (T-33).
- **No AI or insight cards.** Classroom's Class insights and Gemini modules [1] have no counterpart; OVC adds no third-party service (ADR-0008).
- **Retained features.** The mockups omit the first-run checklist, Recently checked, My tutors, the no-tutor state and the tutor filter's refusal states; item 15 and R4's acceptance criteria retain them.
- **Accessibility.** Counts on navigation items stay in the accessible name ("Students, 2 requests"), not colour alone (WCAG 2.2 1.4.1); removal of a focused attention row follows the shared focus rule (WCAG 2.2 2.4.3).

## Open points for the task plan

- Order and cap of the needs-attention block: one combined list (the mockup) or the existing per-kind panels restyled; whether rows are capped at five as Classroom does [1]; whether an empty block hides or says "Nothing needs your attention".
- Join request rows inside the block: the mockup shows Decline and Accept side by side; item 16 allows one filled action. Decide whether Accept stays filled with Decline as a secondary button, and where "Decline and block" lives.
- Aggregated rows ("3 works await checking, oldest 2 days") versus per-item rows; the aggregate needs `oldest submitted_at` from `/home`, which may be a contract change.
- Which kinds appear in Recent updates per role: the mockups put Cancelled and Moved lessons there, while the notifications direction puts lesson changes as marks on lesson rows and keeps Recent updates for kinds with no other home. Whether a tutor sees their own cancellations at all.
- Where the tutor code card goes: the mockup keeps an "Invite a student" card; the benchmark audit at `512765d` suggests moving it to the add-student flow and Settings.
- Live-lesson wording and countdown ("34 min left") without named presence; whether the countdown updates live and how it is announced.
- Home's duplication of the next lesson in the next seven days (benchmark audit at `512765d`): start the list from the second lesson or keep it.
- Whether the first-run checklist moves below the lesson and attention blocks, so the live lesson stays in the first screen.
- The `/home` contract fields R4 needs (recent updates, moved marker, Students count source) and which notifications change supplies them.
- The mockups `today.html` and `student-home.html` at `8b3a8b0` show named presence ("… is already in the room"), which item 13 forbids, and a message excerpt under an unread chat row, which the Today specification forbids; the plan must state the presence-free live wording and whether unread rows show only the chat label and count.

## Unverified

- Preply's exact join window (minutes before start) and the layout of the tutor's and student's home pages beyond the Join button and the lesson balance.
- Whether Linear shows an Inbox count in its sidebar; the pages found document a configurable badge count only for the priority inbox and the macOS dock badge [7, 10].
- Whether Classroom's web interface has any notification indicator; the page found covers Android only [6].
- Telegram's unread counters on folder tabs and the mention badge; no first-party page found describes them [19].
- Slack's per-channel unread rules beyond the Activity view.

## Sources

1. Google Classroom — Use the Classroom homepage — <https://support.google.com/edu/classroom/answer/17231999> — FP — checked 2026-11-27 — no "To review" module named on this page
2. Google Workspace Updates — Redesigned Google Classroom homepage with tailored views based on user's role — <https://workspaceupdates.googleblog.com/2026/07/redesigned-google-classroom-homepage-with-tailored-views-based-on-users-role.html> — FP — checked 2026-11-27
3. Google Classroom — View all your students' work — <https://support.google.com/edu/classroom/answer/9157286> — FP — checked 2026-11-27 — per-assignment counts not confirmed
4. Google Classroom — Grade, assess and provide feedback — <https://support.google.com/edu/classroom/answer/16643267> — FP — checked 2026-11-27 — teacher statuses only
5. Google Classroom — Find your classwork (computer) — <https://support.google.com/edu/classroom/answer/6020284?hl=en&co=GENIE.Platform%3DDesktop> — FP — checked 2026-11-27
6. Google Classroom — Manage notifications — <https://support.google.com/edu/classroom/answer/6141557?hl=en> — FP — checked 2026-11-27 — Android steps only
7. Linear — Inbox — <https://linear.app/docs/inbox> — FP — checked 2026-11-27
8. Linear — Changelog, Priority inbox (2026-09-03) — <https://linear.app/changelog/2026-09-03-priority-inbox> — FP — checked 2026-11-27 — criteria and default tab not stated
9. Linear — My Issues — <https://linear.app/docs/my-issues> — FP — checked 2026-11-27
10. Linear — Notifications — <https://linear.app/docs/notifications> — FP — checked 2026-11-27
11. Preply — Preply Classroom: a student's guide — <https://help.preply.com/en/articles/4182666-preply-classroom-a-student-s-guide> — FP — checked 2026-11-27 — join timing not quantified
12. Preply — Starting and teaching lessons — <https://help.preply.com/en/articles/4984773-starting-and-teaching-lessons> — FP — checked 2026-11-27
13. Preply — Preply Classroom features for tutors: a complete guide — <https://help.preply.com/en/articles/4182374-preply-classroom-features-for-tutors-a-complete-guide> — FP — checked 2026-11-27
14. Preply — How many lessons do I have and why does the number change? — <https://help.preply.com/en/articles/15432715-how-many-lessons-do-i-have-and-why-does-the-number-change> — FP — checked 2026-11-27
15. Slack — Introducing the new Activity view in Slack — <https://slack.com/help/articles/46751260742035-Introducing-the-new-Activity-view-in-Slack> — FP — checked 2026-11-27
16. GitHub Docs — About notifications — <https://docs.github.com/en/subscriptions-and-notifications/concepts/about-notifications> — FP — checked 2026-11-27
17. Microsoft Support — Explore the Activity feed in Microsoft Teams — <https://support.microsoft.com/en-us/teams/notifications-settings/explore-the-activity-feed-in-microsoft-teams> — FP — checked 2026-11-27 — page holds two layout versions
18. Microsoft Support — Catch up with and manage badge count activity in Microsoft Teams — <https://support.microsoft.com/en-us/office/catch-up-with-and-manage-badge-count-activity-in-microsoft-teams-d55a5249-8f72-4f99-9d2a-04fe4b9d68ce> — FP — checked 2026-11-27
19. Telegram — Chat Folders — <https://telegram.org/blog/folders> — FP — checked 2026-11-27 — no folder-tab counters described
