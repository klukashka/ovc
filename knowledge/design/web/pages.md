# Pages, element by element

Every page of the web application. Addresses and frames are in [navigation](navigation.md); state words and colours are in [states](states.md); values are in [tokens](tokens.md). Each page lists the requirements it covers. Every page shows the shared loading, empty, error and offline behavior described at the end.

## Public pages

### Sign in

FR-1.4, FR-1.8, FR-17.1.

- Email, password, "Show password", Sign in, links to Forgot password and Sign up.
- A language switch in the frame.
- Throttling shows "Too many attempts, try again in {minutes}".
- A wrong email or password shows one message that does not say which was wrong.

### Sign up

FR-1.1, FR-1.2.

- `/signup` offers two large cards: "I teach" and "I study".
- **Form:**
  - name, email, password with a strength hint (minimum 10 characters), language (preselected from the browser), and a consent checkbox linking the privacy policy;
  - the student form also has an optional tutor code field.
- **Submitting** shows "Check your email" with the address and a resend button, disabled for 60 seconds.
- **After confirmation**, a student with a code sees "Request sent. Waiting for the tutor."; one without a code sees "Enter your tutor's code".

### Accept invitation

FR-2.1, FR-2.2, FR-1.3.

- **Shows:** tutor avatar, name and subjects, and "{tutor} invites you to study on OVC".
- **Signed out, the invited email has no account (or there is none):**
  - "Create account" with name, email (fixed when the invitation has one), password, language and the consent checkbox;
  - an emailed invitation lands on Home with the email confirmed;
  - a copied link or an email-less invitation shows "Check your email to finish joining your tutor".
- **Signed out, the invited email already has an account:** only "Sign in to accept".
- **Signed in as a student:** one "Accept" button. A different email than the invitation's shows "This invitation is for another email address".
- **Signed in as a tutor:** an explanation and a sign-out link.
- **Invalid, expired, used or revoked:** one message, "This invitation is no longer valid — ask your tutor for a new link".
- **The page** never shows the invitee's email, and removes the token from the address bar after loading.

### Confirmation, password reset, unsubscribe

FR-1.1, FR-1.5, FR-13.3. Single-purpose pages with one result message and one next step. The confirmation page, opened in a browser not signed in to that account, first asks for a new password (with the strength hint) and then confirms; it then shows the pending-link outcome ("You are now studying with your tutor", "Request sent. Waiting for the tutor.", or that a code or invitation is no longer valid). Unsubscribe names the email group and asks the user to confirm with a button (nothing changes when the page merely loads), then offers a link to all email settings. Token pages remove the token from the address bar after reading it.

## Tutor pages

### Today

FR-15.1, FR-15.2, FR-2.3, FR-5.1.

- **Greeting** with date.
- **First-run checklist card**, until completed or dismissed:
  - four steps (profile and timezone, invite a student, schedule a lesson, create an assignment) with check marks;
  - each step links to the place where it is done.
- **Today's lessons:**
  - a time-ordered list with time, title, audience (student or group with avatars) and status;
  - the next lesson shows a primary "Join lesson" button when its window is open, otherwise "Room opens at {time}";
  - empty: "No lessons today" with "Schedule a lesson".
- **Needs checking:**
  - count and the five oldest items (student, assignment, submitted time, late badge);
  - "Open review queue".
- **Join requests:** student name and email, with Accept, Decline, and "Decline and block" in a menu.
- **Unread messages:** channels with unread counts.
- **Upcoming deadlines:** issues due in the next 3 days with progress "{submitted}/{total}".
- **Phone:** sections stack in the order above.

### Schedule

FR-4.1–FR-4.5.

- **Toolbar:** Week, Month, Today, previous and next, and "New lesson".
- **Week:** a time grid in the viewer's profile timezone; lesson blocks in the audience colour with title and status. Cancelled lessons are struck through and muted. Clicking a free slot pre-fills "New lesson".
- **Month:** days with up to three lessons and "+{n} more". Lesson blocks and "+{n} more" keep 44 px touch targets.
- **Phone:** an agenda list grouped by day.
- **New lesson dialog:**
  - audience picker: a student or a group, with search;
  - title, date, start time and duration;
  - "Repeat weekly" with weekday toggles and "Ends on {date}" or "After {n} lessons";
  - description and materials from the library.
- **Overlap warning:** an inline list of overlapping lessons with "Schedule anyway".
- **Editing or cancelling a series occurrence** asks: "This lesson" or "This and following lessons". Weekday changes are offered as "End this series and create a new one".

### Lesson

FR-4.6, FR-4.7, FR-5.1–FR-5.6.

- **Header:**
  - title, audience, date and time (with the second timezone when it differs), status badge, and description;
  - for the tutor: Edit and Cancel while the lesson is scheduled, and "End lesson" while it is live, with a confirmation.
- **Video area**, driven by these join states:

  | State | Tutor sees | Student sees |
  |---|---|---|
  | Window not open | "Room opens at {time}" | "Room opens at {time}" |
  | Window open, lesson scheduled | "Join lesson" | "The room opens when your tutor joins", updating live |
  | Live, room open | "Join lesson" or "Rejoin" | "Join" |
  | Live, room closed (the tutor dropped) | "Rejoin" | "The room opens when your tutor joins", updating live |
  | Join refused | The message for the error code (`outside_join_window`, `provider_unavailable` "The video service is unavailable, try again"); the lesson reloads | "You can't join this lesson" for `not_found`; the live-room-closed state for `lesson_not_live`; the same `provider_unavailable` message |
  | Joining | Spinner; the button stays disabled until the join response | Same |
  | In room, embedded | The room in an iframe titled with the lesson name, at 16:9 or full viewport height in focus mode (with an "Exit focus" button and `Esc`), and an always-visible "Open in new tab" | Same |
  | In room, new tab | "The lesson is open in another tab" and "Open again" | Same |
  | Ended | "Ended" or "Did not take place" | Same |
  | Cancelled | "Cancelled" with the cancellation date | Same |

  - **Embed or new tab:** the page knows `embed_allowed` from the lesson data before the click.
    - **New tab:** it is opened in the click handler before the join request completes, so pop-up blockers allow it; its `opener` is cleared before it is navigated to the link. If the join fails, the pre-opened tab is closed.
    - **Embedded:** the page cannot see inside the cross-origin client, so "Open in new tab" is always visible. If the iframe has not fired its `load` event within 15 seconds, the page highlights that button with "Having trouble? Open in a new tab". A `load` event does not prove the client works (error pages load too), which is why the button never disappears.
    - **Switching** from the embed to a new tab removes the iframe first, so the user is never connected twice.
    - **"Exit focus"** sits outside the iframe, because key presses inside the client never reach the page.
  - **Leaving:** leaving the page, ending the lesson, or a `lesson` change to ended removes the iframe.
  - **Reloading** asks for a new join link; links are never kept.
- **Materials:** attached library files with download; the tutor has "Attach material" at any status, opening the library picker.
- **Room state** comes from `room_open` and `embed_allowed` in the lesson data, refreshed on `entity.changed`.
- **Restart the room** appears for the tutor while the lesson is live, with a confirmation explaining that everyone must rejoin.
- **Assignments for this lesson:** linked issues. Tutor: "Issue assignment". Student: their submission state.
- **Recordings:**
  - a list with date, duration, state and "available until";
  - Play opens a player dialog;
  - the tutor has Delete, and Retry on a recording held because storage is full.
- **Phone:** the video area becomes a large Join button that opens the room in a new tab, because phone browsers do not offer full-screen embedding reliably.

### Students

FR-2.1, FR-2.3, FR-2.5.

- **Tabs:** Active, Invited, Ended, with search.
- **Rows:** avatar, display name, groups, next lesson, open assignments and last activity.
- **"Add student" dialog:**
  - display name and optional email, then "Create invitation";
  - the result shows the link with Copy and, if an email was given, "Send by email".
- **Pending join requests** appear at the top with Accept, Decline and, in a menu, "Decline and block".
- **Empty:** an illustration with "Invite your first student" and "Share your tutor code {code}".

### Student card

FR-2.5, FR-2.7, FR-11.3.

- **Header:** avatar, name, email, groups, relationship state, and the buttons Message, Schedule a lesson, Issue assignment, and End working together (in an overflow menu).
- **Tabs:**
  - Overview: next lessons, open assignments, private notes editor (marked "Only you see this");
  - Lessons: history with status and recordings;
  - Homework: every issue with state and score;
  - Progress: a chronological list of checked work with score percentages and a simple bar per item.
- **Invited card:** the invitation link with Copy, Send and Regenerate, its expiry, and "Withdraw invitation".
- **Ended card:** "Invite again" creates a new link with Copy and Send; history stays visible.

### Groups and group page

FR-3.

- **List:** name, colour, member avatars, next lesson, and "New group".
- **"New group" dialog:** name, colour from the palette, and members picked from active students with search.
- **Group page:**
  - members with Add and Remove;
  - lessons, issued assignments and a link to the group chat;
  - Archive and Unarchive.
- **Adding a member** shows which existing assignments they will not receive.

### Library

FR-6.

- **Layout:** a folder tree on the left (a drop-down on phones) and search.
- **Items:** a grid or list of materials (file icon, title, size) and templates (title, question count, maximum points, last edited).
- **Actions:**
  - "New assignment", "Upload material", "New folder";
  - per item: Open, Duplicate, Move, Issue (templates), Delete.
- **Empty:** "Build your first assignment" plus an upload drop zone.

### Builder

FR-7, FR-8.1.

- **Header:** a title field, autosave status ("Saved" or "Saving…"), Preview, and Issue.
- **Block list:**
  - each block is a card with a type label, a drag handle (keyboard: Move up and Move down buttons), Duplicate and Delete;
  - "Add block" opens a menu of the eight block types.
- **Question editors:**
  - choice options with a correct toggle;
  - accepted answers as chips for short text;
  - value and tolerance for numbers;
  - points;
  - an explanation, collapsed by default;
  - allowed file kinds and count.
- **Preview** renders the student view in a phone-width or desktop-width frame.
- **The width** is optimised for desktop; on phones the builder stacks and stays fully usable.

### Issue dialog

FR-8.

- **Fields:**
  - audience (a student or a group);
  - optional linked lesson;
  - optional deadline (date and time in the tutor's timezone);
  - answer visibility (Never / After submission / After checking), with a note that "After submission" turns off returning for revision.
- **Summary:** "{n} students will receive this".

### Issued work and issue page

FR-8.4, FR-10.1, FR-11.2.

- **The list:** issues with title, audience, deadline, progress (submitted, checked, late), and a "needs checking" badge.
- **Issue page:**
  - Submissions tab: a row per student with state, score and open;
  - Analytics tab:
    - summary numbers;
    - a per-question table with correct share or average points;
    - option distribution bars and the top short answers;
  - Change deadline and Withdraw (only while nothing is submitted).

### Review queue and review

FR-10.

- **Queue:** oldest first, filters by issue, group and student; each row has student, assignment, submitted time and late badge.
- **Review screen:**
  - two panes on desktop: the student's answers on the left, scoring on the right, aligned per question;
  - per question: points input (manual), automatic points with "Override" (points and reason), comment, and "Attach corrected file" for file answers with previews of the student's files;
  - bottom: overall feedback, total score and percentage, "Save draft", "Return for revision" (hidden when not allowed), and "Mark checked";
  - "Next submission" moves through the queue.
- **Phone:** single column, with scoring under each answer.

### Messages

FR-12.

- **Channel list:** direct chats (student name) and group chats (group name and colour), with last message, time and unread badge; search.
- **Conversation:**
  - header with name, presence and a link to the student card or group;
  - a message list with day separators, attachments with image previews, edited and deleted states;
  - on the user's own messages, a menu (hover or focus on desktop, long press on touch) with Edit (within 24 hours) and Delete; the tutor also gets Delete on others' messages in group chats;
  - a composer with text, an attach button with drag and drop and paste of images, upload progress, and Send.
- **Read-only channels** show the banner and a disabled composer.
- **Phone:** the list and the conversation are separate screens.

### Settings

FR-1.4–FR-1.7, FR-5.6, FR-13.3, FR-14.2, FR-17.

- **Profile:** name, avatar, bio and subjects (tutor).
- **Account:**
  - language, timezone, theme;
  - change password, sign out everywhere, delete account.
- **Notifications:** email switches per group.
- **Teaching (tutor):**
  - tutor code with Copy and Regenerate;
  - recording retention;
  - storage usage bars for files and recordings, against their quotas;
  - blocked students with Unblock.

## Pages for both roles

### Notifications

FR-13.1, FR-2.6. Address `/notifications`; the bell in the frame opens a panel with the latest 10 and a link to the page.

- **Rows:** icon, one-line text (for students with the tutor label), relative time, unread dot; clicking opens the related page and marks the row read.
- **Actions:** "Mark all as read".
- **Empty:** "No notifications yet".
- **Live updates:** new rows arrive live and are announced politely to screen readers.

## Student pages

### Home

FR-15.3, FR-5.1, FR-2.6.

- **Top:** the tutor filter (shown when the student has two or more tutors).
- **Next lesson card:**
  - tutor label, title, and time with a countdown when under an hour;
  - a full-width "Join" button when live and the room is open, otherwise "The room opens when your tutor joins" or "Starts at {time}".
- **To do:** submissions to do ordered by deadline, with due-soon and overdue badges.
- **Recently checked:** the last five checked submissions, with the tutor label and whether feedback is new.
- **No tutor yet:** "Enter your tutor's code" with an input; with a pending request, "Request sent. Waiting for the tutor." instead.
- **Phone first:** cards stack and the Join button stays within thumb reach.

### Schedule (student)

FR-4.5. Agenda by day on phones; week and month from 768 px. Each item has its tutor label, the tutor filter sits at the top, and there are no editing controls.

### Homework (student)

FR-9, FR-11.1.

- **Tabs:** To do, Submitted, Checked, and Closed (shown only when not empty); plus the tutor filter.
- **Assignment page:**
  - instructions and materials, then questions in order, with a progress indicator "{answered}/{total} answered" and a saving status;
  - large tap targets for options; native file picker and camera capture for file answers;
  - Submit confirms and lists any unanswered questions.
- **After submission:**
  - the status "Submitted", with no points at any time;
  - correct answers and explanations next to their own answers, only when revealed.
- **Save conflict:** if another device saved newer answers, a banner says "Your answers changed on another device". It offers "Load the latest" and keeps the unsaved local answers shown, so they can be reapplied.
- **Returned work:** the tutor's comments are highlighted, and only the reopened questions are editable.
- **Checked results:** per-question comments, corrected files to download, overall feedback, and correct answers with explanations when allowed. No points or scores.

### My tutors

FR-2.6, FR-1.2, FR-2.7, FR-1.7.

- **Cards:** each tutor's avatar, name, colour, subjects and bio, plus "Message".
- **Add a tutor:** a field that takes a code.
- **Pending requests:** "Request sent. Waiting for the tutor." (no name interpolation, so no grammatical case problems in Russian).
- **Ended relationships:** listed under "Past tutors" with read-only chat, lessons, recordings until they expire, and results. They are left out of the Home filter by default.
- **A tutor who deleted their account:** shown as "Account closed — results available until {date}", with read-only access until then.

### Messages and settings (student)

Same as the tutor's, without tutor-only sections; direct channels are named by tutor, group channels by group with the tutor label.

## Shared behavior

- **Loading:**
  - skeletons in the shape of the content appear after 300 ms; nothing appears before;
  - buttons that start an action show a spinner and stay disabled until it completes.
- **Empty states:** an illustration, a heading, one sentence and one primary action:

  | Page | Message | Action |
  |---|---|---|
  | Messages | "No conversations yet" | Open a student or group |
  | Review queue | "Nothing to check" | — |
  | Issued work | "Nothing issued yet" | Open the library |
  | Groups | "No groups yet" | Create a group |
  | Student homework tabs | "Nothing to do" / "Nothing submitted" / "Nothing checked yet" | — |
  | Recordings | "No recordings for this lesson" | — |

- **Errors:**
  - a failed page load shows "Something went wrong" with Retry and the request identifier in small text;
  - a forbidden or missing resource shows Not found;
  - failed actions show an error toast with Retry, reachable by keyboard;
  - uploads show the refusal inline: "File is larger than {limit}", "This file type is not allowed here", "Storage is full";
  - a stale edit (a template, a review draft) shows "This changed elsewhere" with "Reload" and keeps the user's unsaved text visible.
- **Offline:** a banner "No connection — reconnecting…".
  - Assignment answers are kept and saved when the connection returns.
  - Other actions are disabled while offline, with the reason in a tooltip.
  - The realtime client reconnects with backoff and refetches visible data on reconnect.
- **Live data:** frames from the realtime channel trigger a refetch of the affected item and never replace it directly. Lists keep the scroll position.
- **Time:**
  - every time is shown in the viewer's timezone;
  - the lesson page adds the other party's time when it differs, using the `time.other_zone` string from [states](states.md#time-and-scores) with the city of the IANA timezone taken from a bundled, localised table;
  - deadlines show both an absolute time and a relative hint ("in 2 days").
- **Responsive:**
  - "phone" means narrower than 768 px; every page works from 360 px;
  - tutor pages are designed for 1280 px and degrade to a single column;
  - student pages are designed for 390 px and expand to two columns from 1024 px.
- **Keyboard and screen readers:**
  - every action is reachable by keyboard in visual order, with a visible focus ring;
  - dialogs trap focus and restore it on close;
  - live regions announce new messages, saving status and toasts;
  - icons that act as buttons have accessible names;
  - charts have a table alternative;
  - every page has a "Skip to content" link, and route changes move focus to the page heading;
  - lesson status changes ("Live now", the Join button appearing) are announced, while countdowns are not;
  - typing dots and the live pulse stop with reduced motion.
