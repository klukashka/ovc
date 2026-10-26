# Pages, element by element

Every page of the web application. Addresses and frames are in [navigation](navigation.md); state words and colours are in [states](states.md); values are in [tokens](tokens.md). Each page lists the requirements it covers. Every page shows the shared loading, empty, error and offline behavior described at the end.

Recordings (FR-5.4 to FR-5.6) and analytics (FR-11.2 to FR-11.4) come [after the MVP](../../../docs/roadmap.md#after-the-mvp) by the owner's decision of 2026-10-22. Until then the lesson page has no Recordings block or empty state, the student card has no Progress tab, the issue page has no Analytics tab, and Settings shows neither recording retention nor recording storage; the parts below that describe them are the later design.

## Public pages

### Sign in

FR-1.4, FR-1.8, FR-17.1.

- Email, password, "Show password", Sign in, links to Forgot password and Sign up.
- A language switch in the frame.
- Throttling shows "Too many attempts, try again in {minutes}".
- A wrong email or password shows one message that does not say which was wrong.

### Sign up

FR-1.1, FR-1.2.

- `/signup` offers two large choices, each a bordered panel: "I teach" and "I study".
- **Form:**
  - name, email, password with a strength hint (minimum 10 characters), language (preselected from the interface language, which follows the browser until the person chooses one), and a consent checkbox linking the privacy policy;
  - the student form (`/signup/student`) also has a required tutor code field, first in the form, with the hint "Got a link from your tutor? Open it instead"; an invalid code (`tutor_code_invalid`) shows "This code is no longer valid — ask your tutor for a new one" under the field, with the form's alert "Check the highlighted fields", and creates nothing; a valid code shows "Check your email" for a new and a taken email alike.
- **Submitting** shows "Check your email" with the address and a resend button, disabled for 60 seconds.
- **After confirmation** in the same browser, a student who signed up with a code sees "Request sent. Waiting for the tutor."

### Accept invitation

FR-2.1, FR-2.2, FR-1.2, FR-1.3.

- **Shows:** tutor avatar, name and subjects, and "{tutor} invites you to study on OVC".
- **Signed out:** "Create account" and "Sign in to accept", both in place, whatever the invitation's email:
  - "Create account" asks for name, email, password, language and the consent checkbox; the page never shows or pre-fills the invited email;
  - after creating the account the page signs in with the chosen password and lands on Home, linked to the tutor with the confirmation banner; when that sign-in fails it shows "Check your email, or sign in to accept" and switches to the sign-in form, never a wrong-password error;
  - when the email was already taken and the password signs in to that account, and the account is confirmed (`active`), the page stays on the invitation and offers "Accept", keeping the link in memory; otherwise it lands on Home;
  - "Sign in to accept" signs in on the same page and then offers "Accept".
- **Signed in as a student:** "You are signed in as {email}", one "Accept" button and "Use another account", which signs out on the page and keeps the invitation; accepting lands on My tutors. An account whose earlier links wait for the [reclaim choice](#keep-or-drop-earlier-links) may still accept, and the page also shows a link "You have links waiting for your choice" to `/reclaim`.
- **Signed in as a tutor:** an explanation that tutor accounts cannot study, with a button that signs out on the page and keeps the invitation (`role_mismatch`).
- **Signed in as an unconfirmed student who already joined a tutor:** "Confirm your email first" with a resend button (`email_not_verified`).
- **Another email:** "This invitation is for another email address" (`invitation_email_mismatch`), for a sign-up and a signed-in acceptance alike.
- **Invalid, expired, used or revoked:** one message, "This invitation is no longer valid — ask your tutor for a new link".
- **The page** never shows the invitee's email, removes the link's secret part from the address bar on its first render (before the session has loaded), and keeps it in memory only, never in `next`. Its address `/invite/{token}` is replaced with `/invite`; opened as `/invite` alone (for example after a reload), it shows "Open the link from your tutor again".

### Confirmation, password reset, unsubscribe

FR-1.1, FR-1.5, FR-13.3. Single-purpose pages with one result message and one next step. The confirmation page refuses an unknown, expired, used, superseded or malformed link at once, before any password is asked for; the reset page learns that the link is invalid only when the new password is sent. The confirmation page, opened in a browser not signed in to that account, first asks for a new password (with the strength hint) and then confirms; it then shows the outcome with a "Continue" link to the home page: "You are now studying with your tutor" (`linked`), "Request sent. Waiting for the tutor." (`request_sent`), "This code is no longer valid — enter a new one" (`code_invalid`); with no outcome it goes straight to the home page, and for `choice_required` it continues to the [reclaim page](#keep-or-drop-earlier-links). The reset page continues to sign-in, after which an account with a pending choice lands on the reclaim page; when the reset answers `choice_required` it adds "After you sign in, choose which earlier links and requests to keep". Unsubscribe names the email group and asks the user to confirm with a button (nothing changes when the page merely loads), then offers a link to all email settings. Token pages remove the token from the address bar after reading it.

- **Unsubscribe** (`/unsubscribe?token=`, slice 4) works without signing in and never signs anyone in. The token handling below is the contract; the page's messages, button and focus were built in slice 4 as proposed here and await the owner's confirmation:
  - it reads the token once on its first render, replaces the address with `/unsubscribe` before any request, and keeps the token only in component state, never in the query cache, storage, history state, `next` or any other address; opened as `/unsubscribe` alone it shows the invalid-link message;
  - it loads `GET /unsubscribe/preview`, showing "Checking the link" meanwhile, and then the group's name with the button "Turn off these emails"; every `token_invalid`, on the preview as on the confirmation, shows one message, "This link is no longer valid", which takes focus, with the link "All email settings" to Settings → Notifications (which asks to sign in when signed out); `rate_limited` shows the throttling message with the minutes to wait; any other failed preview shows the shared load failure with Retry;
  - confirming sends `POST /unsubscribe` with no body; the button stays busy until the answer, and the result "These emails are turned off" replaces the button and takes focus, with the link "All email settings" to Settings → Notifications (which asks to sign in when signed out); a refused confirmation other than `token_invalid` shows its error, which takes focus, and keeps the button;
  - the page's texts are in [states](states.md#notifications).

## Tutor pages

### Today

FR-15.1, FR-15.2, FR-2.3, FR-5.1.

- **Greeting** with date.
- **First-run checklist panel**, until completed or dismissed:
  - four steps (profile and timezone, invite a student, schedule a lesson, create an assignment) with check marks;
  - each step links to the place where it is done.
- **Today's lessons:**
  - a time-ordered list with time, title, audience (student or group with avatars) and status;
  - the next lesson shows a primary "Join lesson" button when its window is open, otherwise "Room opens at {time}";
  - empty: "No lessons today" with "Schedule a lesson".
- **Needs checking:**
  - count and the five oldest items (student, assignment, submitted time, late badge);
  - "Open review queue".
- **Join requests:** student name and email, with Accept, Decline, and "Decline and block" in a menu, oldest first; the panel also shows the tutor code with Copy. In slice 3 this panel is the only part of Today; the rest arrives with slice 10.
  - each row shows the avatar, the account name and email, the state word "Wants to join" and the sending date; the buttons name the student for screen readers ("Accept {name}", "Decline {name}", "More actions for {name}"); "Show more" loads the next page and moves focus to the first new row's Accept;
  - a decision shows a notice in the panel, which takes focus: "Request accepted. The student is now in Students.", "Request declined." or "Request declined. Requests from this student no longer reach you."; no dialog asks first, because a block is undone in Teaching settings;
  - a request that is no longer open when decided (`not_found`, `not_editable`) shows the alert "This request is no longer open. The list is up to date.", which takes focus each time, and the list reloads; a decision refused with `email_not_verified` shows the confirmation prompt "Confirm your email to answer join requests." inside an alert that takes focus;
  - with no visible request the panel says "No new requests."; an unconfirmed tutor sees no request list, and the code with "The code starts working once you confirm your email";
  - until the request list and the tutor code have first loaded, the tutor code and the server status below the list are not shown (a loading state stands in their place once the list has loaded), so nothing moves when the requests arrive; once shown they stay shown, also while a failed list or code is retried.
- **Unread messages:** channels with unread counts.
- **Upcoming deadlines:** issues due in the next 3 days with progress "{submitted}/{total}".
- **Computer layout:** the greeting spans the page; the main column holds the checklist, today's lessons and needs checking; a 300 px side column holds join requests, upcoming deadlines and unread messages.
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
- **Computer layout:** a thin top bar with breadcrumbs; the main column holds the video area (as a join panel until the user is in the room), materials, assignments and recordings; a 300 px side column "About the lesson" holds date and time, format (individual or group), audience, the series rule, a short "Before the lesson" hint (new tab, headphones, camera and microphone permission) and a link to the chat.
- **Phone:** the video area becomes a large Join button that opens the room in a new tab, because phone browsers do not offer full-screen embedding reliably.

### Students

FR-2.1, FR-2.3, FR-2.5.

- **Tabs:** Active (including students whose email is not confirmed and links waiting for the student), Invited, Ended, with search; the tab and the search are in the address.
- **Search:** searching, with the Search button or Enter in the field, keeps focus where it was, and the field and the group choice show what the address holds after every change of the address (including Back); "Clear search" appears while a search or a group is applied, empties both and moves focus to the search field.
- **Group filter:** beside the search, a "Group" choice of every group of the tutor ("All groups" first, archived groups marked "(archived)"), applied with the search and kept in the address as `group=`; it is shown once the tutor has a group. A group the tutor cannot reach, or a `group=` value that is not a group identifier, shows "This group is not available." with a link to all groups; a search the API refuses (longer than 254 characters) shows "This search cannot be used. A search holds at most 254 characters." with the link "Show the list without the search"; neither offers Retry.
- **Rows:** avatar, display name, email, state and groups; next lesson, open assignments and last activity arrive with the slices that add them.
- **"Add student" dialog:**
  - display name and optional email, then "Create invitation";
  - the result shows the link with Copy and, if an email was given, "Send by email"; sending shows "Sent" as a toast and never a delivery status.
- **Pending join requests** appear at the top with Accept, Decline and, in a menu, "Decline and block", in the same section and with the same notices as on Today; the section is shown only while requests wait, and stays after the last decision until the page is left, so its notice keeps focus. Until the join requests have first loaded, the tabs, the search and the list are not shown (a loading state stands in their place), so they do not move when requests arrive; once shown they stay shown, also while a failed request list is retried.
- **Empty:** a heading with "Invite your first student" and "Share your tutor code {code}"; the empty Invited and Ended tabs and a search or group without matches say so in one line ("No open invitations.", "No ended relationships.", "Nobody matches the search.") and keep "Add student".
- **An unconfirmed tutor** gets the confirmation prompt instead of "Add student".

### Student card

FR-2.5, FR-2.7, FR-11.3.

- **Header:** avatar, display name, email, groups, relationship state, and the buttons Message, Schedule a lesson, Issue assignment (each from the slice that adds it), and End working together (in an overflow menu).
- **Tabs:**
  - Overview: next lessons, open assignments, private notes editor (marked "Only you see this");
  - Lessons: history with status and recordings;
  - Homework: every issue with state and score;
  - Progress: a chronological list of checked work with score percentages and a simple bar per item.

  In slice 3 the card shows the header, groups and notes; the tabs arrive with the slices that add lessons and work.
- **Invited card:** the invitation link with Copy, Send and Regenerate, its expiry, "Link revoked after repeated wrong emails" when the mismatch limit revoked it, and "Withdraw invitation".
- **Provisional card:** the badge "Email not confirmed", the account's name and email, and "End working together", which removes the student after a dialog explaining that the link is discarded and a new card with a fresh link replaces it (Cancel focused first; the dialog's button is "End"); the new card then opens with the notice "The link was removed. This new card has a fresh invitation link."
- **Reclaim-pending card:** the badge "Waiting for the student", the line "The owner of this email is deciding whether to keep studying with you", no account details, and the same "End working together", whose dialog says that the owner has not chosen yet and is not told.
- **A dialog action refused because the card changed meanwhile** (for example a removal answering `relationship_not_provisional` after the owner kept the link, or a withdrawal of a card that was linked) closes the dialog and shows the notice "This card changed in the meantime. Its current state is shown below.", which takes focus, above the reloaded card.
- **Live changes (slice 4):** when a live refetch changes the card's state while one of its dialogs is open (the student confirmed, accepted or ended meanwhile), the dialog closes and the same changed-meanwhile notice takes focus. When a live refetch finds the card gone (the provisional student was removed or the invitation withdrawn in another tab or device), the page shows the notice "This student is no longer on your list." and the link "All students"; the notice takes focus when a dialog was open or focus was otherwise lost. While the page's own removal or withdrawal is in flight the card is not fetched again, so the page's own action never meets the frame it causes.
- **Ended card:** "Invite again" creates a new link with Copy and Send; history stays visible. In slice 3 it sits under "Work together again" with the hint that the student comes back to the card and its history but not to the groups; once the link exists, the card shows it as an invited card does (Copy, Send, "Create a new link", its expiry) and the notice "A new invitation link was created." takes focus. An ended card that already holds an open link shows that link instead of "Invite again". When the student's account is deleted, the card shows no account details until the purge: the avatar falls back to the display name's initial and the email line shows only the card's own email.
- **End dialog** for an active student names the consequences: the student leaves the tutor's groups, future lessons are cancelled, the chat becomes read-only, and the student gets an email. It opens on Cancel; its button is "End". After ending, the card shows its ended state and the notice "You no longer work with this student." takes focus. In slice 3 "End working together" is a button below the notes on provisional, reclaim-pending and active cards, not yet in an overflow menu, because the header has no other actions until lessons and chat arrive.

### Groups and group page

FR-3.

- **List:** name, colour, member avatars, next lesson, and "New group".
- **"New group" dialog:** name, colour from the palette, and members picked from active students with search.
- **Group page:**
  - members with Add and Remove;
  - lessons, issued assignments and a link to the group chat;
  - Archive and Unarchive.
- **Adding a member** shows which existing assignments they will not receive.
- **In slice 3** (no lessons, assignments or chat yet):
  - the list shows each group's colour dot, name, up to five member avatars with "+{n}" for the rest, the member count and "Archived" for an archived group; non-archived groups come first; the empty state's action is "New group";
  - the "New group" dialog opens on its name field and offers the eight palette colours (the first one no other unarchived group uses is preselected) and a member choice: a search over the tutor's active students (students whose email is not confirmed and links waiting for the student are left out), checkboxes with name and email, "Show more" for the next page, which moves focus to the first new checkbox, and "Chosen: {count} of 30", beyond which no further student can be chosen; creating opens the new group's page, whose heading takes focus as on every route change;
  - the group page shows the colour, "Archived" when archived, the members with their card name, account name and a "Remove" button each, "Add students", "Name and colour" with Save, and "Archive group" or "Unarchive group" with a sentence on what it does; archiving has no dialog in slice 3, because groups have no lessons, series or chat yet, so archiving removes nothing that unarchiving does not restore; slice 5, where archiving cancels lessons and ends series that unarchiving does not restore, revisits whether it needs a confirmation;
  - "Add students" opens a dialog with the same member choice, limited to the free places, which opens on its search field; adding closes it and the notice "The students were added to the group." takes focus; an archived group offers no "Add students" and says that it takes no new students;
  - removing a member, archiving and unarchiving each show a notice that takes focus ("The student was removed from the group.", "The group is archived.", "The group is active again.");
  - a refusal because chosen students stopped being active (`relationship_not_active`) keeps the dialog open, takes them off the choice and says so; a member outside the tutor's scope (`not_found`) empties the choice and asks to choose again; the 30-member and 200-group limits each have their own message; an addition refused because the group was archived meanwhile (`audience_not_active`) closes the dialog and shows "This group changed in the meantime. Its current state is shown below.", which takes focus above the reloaded group.

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
  - each block is a section under a hairline rule, and the block being edited becomes a panel; each has a type label, a drag handle (keyboard: Move up and Move down buttons), Duplicate and Delete;
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

- **Channel list:** direct chats (student name) and group chats (group name and colour), with last message, time and unread count; search.
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
  - language, timezone (the browser's list of IANA zones), theme (system, light or dark);
  - change password, sign out everywhere, delete account;
  - deleting asks for the password in a dialog that names the consequence, and the sign-in page then says the account was deleted;
  - the consequence names the role's effects on tutoring: for a tutor, that working together ends with every student and the students get an email, the invitation links and the tutor code stop working, join requests expire and the groups are archived; for a student, that studying ends with every tutor, the student leaves their groups and the requests to tutors expire.
- **Notifications** (`/settings/notifications`, slice 4, also for an unconfirmed account; the save-per-switch behaviour and the wording were built in slice 4 as proposed here and await the owner's confirmation): under the heading "Optional emails", one switch per optional email group of the role, in the order the API lists them, each with its name and a one-line description from [states](states.md#notifications); tutors see Lesson reminders, Submitted work and Unread messages, and students Lesson reminders, Lesson changes, New assignments, Checked work and Unread messages. A switch saves at once with `PUT /users/me/email-preferences` naming only that group, stays busy while saving and keeps focus; a failed save puts the switch back, keeps focus on it and shows an alert below the switches that names the group, gives the error and offers Retry, reachable by keyboard (one alert per failed group); Retry moves focus back to that group's switch, which stays busy while it saves again, and the alert goes away; until OVC has a shared toast, this inline alert stands in for the error toast of the shared behavior. A sentence says that required emails (confirmation, password reset, invitations, ended relationships and a tutor's deletion) cannot be turned off.
- **Teaching (tutor):**
  - tutor code with Copy and Regenerate (an unconfirmed tutor gets the confirmation prompt instead of Regenerate);
  - recording retention;
  - storage usage bars for files and recordings, against their quotas;
  - blocked students with Unblock, each row with the avatar, the account name and the blocked date; a student whose account is deleted is shown as "Deleted account" with that label's initial in the avatar circle, never the student's picture, and its Unblock button is named "Unblock the deleted account blocked on {date}" for screen readers, so several such rows stay distinguishable;
  - unblocking shows the notice "The student was unblocked.", which takes focus; an Unblock answered `not_found` (the student was unblocked meanwhile) is not reported as done: the alert "This student is no longer blocked. The list is up to date." takes focus each time, and the list reloads.

## Pages for both roles

### Notifications

FR-13.1, FR-2.6. Address `/notifications`; the Notifications item in the sidebar (the "bell"), and below 1024 px the Notifications button in the top bar, shows the unread count and opens a panel with the latest 10 and a link to the page. Built in slice 4 from `GET /notifications` and `POST /notifications/read` ([notification routes](../../specs/notifications.md#notification-routes)). The panel's focus on opening, "99+", `{through}` for Mark all as read and the polite announcements come from the slice 4 plan; the accessible name with the count, focus returning to the bell after a click outside, the plain-text rows without a link and the details F2 added below (the narrow overlay's link, the name above 99, the unread marker, the relative times, the result of Mark all as read, Tab out of the panel and a row of the current page) are built as written and await the owner's confirmation.

- **The count:** the unread count from the API, shown as "99+" above 99, and hidden at zero; the item's accessible name includes the count ("Notifications, 3 unread"), and above 99 says "Notifications, more than 99 unread".
- **Below 1024 px** the top bar's Notifications button opens the panel under the top bar; in the sidebar overlay, Notifications is a link to the page with the count, because the panel does not fit beside the overlay.
- **The panel:** opening it moves focus to its heading "Notifications"; `Esc`, a click outside or choosing a row closes it, and `Esc` and a click outside return focus to the bell (a click on another control leaves focus there). Leaving the panel with Tab closes it without moving focus. Choosing a row or "Show all notifications" while its page is already open closes the panel and moves focus to the page heading. A route change closes the panel. It lists the latest 10 rows (`limit=10`), "Mark all as read" and "Show all notifications", which opens the page.
- **The page:** the rows newest first, 30 per page with "Show more", which moves focus to the first new row, and "Mark all as read".
- **Rows:** icon, the label when there is one (the tutor label with its colour dot for students; the card's name or the requester's account name for tutors), the event's one-line text from [states](states.md#notifications) (which never interpolates a name), relative time ("now", then minutes, hours and days up to a week, then the date and time, with the full date and time as the time's title), and an unread dot with the visually hidden word "Unread". The label is shown as text only, never as HTML or a link. A row with a link opens the related page (the student card, the Students page with its join requests, or My tutors) and marks the row read with `{ids}`; a row whose link is `null` is plain text, not a link, and "Mark all as read" marks it.
- **Mark all as read** sends `{through}` with the newest row shown, so a row that arrived after the list was shown stays unread; the action keeps focus on its button and announces the result "All notifications are marked as read." politely; a refusal shows the shared refusal alert. The list and the count are then refetched.
- **Empty:** "No notifications yet".
- **Live updates:** a `notification.new` frame refetches the list, the panel and the count, and a polite live region in the signed-in frame (outside the page, not a `status` role, so it never competes with a page's own status) announces "New notification" without moving focus, once for the frames that arrive within one animation frame; an `entity.changed` frame of kind `notification` (a read change in another tab) refetches them silently.
- **An unconfirmed student** sees no bell and no panel, although the routes answer for them ([navigation](navigation.md#rules)); the rows produced meanwhile appear once the email is confirmed.

## Student pages

### Home

FR-15.3, FR-5.1, FR-2.6.

- **Top:** the tutor filter, a segmented control "All tutors / {name}…" (shown when the student has two or more tutors).
- **Next lesson panel:**
  - tutor label, title, and time with a countdown when under an hour;
  - a "Join" button (full width on phones) when live and the room is open, otherwise "The room opens when your tutor joins" or "Starts at {time}".
- **To do:** submissions to do ordered by deadline, with due-soon and overdue states and a Start or Continue action per row.
- **Recently checked:** the last five checked submissions, with the tutor label, the opening of the overall feedback, whether a corrected file came back, and, once the open question in [the architecture](../../architecture/detailed.md#open-implementation-questions) is settled, whether feedback is new. No points.
- **Next seven days** (side column): the student's lessons starting within seven days, cancelled ones left out, one row each with weekday, time and tutor label, and a link to the schedule.
- **My tutors** (side column): each active tutor with label, subject and a Message link, and "Enter a tutor code"; the tutor filter does not hide this list.
- **No tutor yet:** "No tutor yet." and "Enter your tutor's code" with an input; with a pending request, "Request sent. Waiting for the tutor." instead. A code sent from Home replaces the input with that sentence as a notice that takes focus; refusals are those of My tutors. With current tutors, the list ends with the link "Enter a tutor code" to My tutors.
- **An unconfirmed student** sees the confirmation banner "Confirm your email to keep studying with your tutor. We sent a link to {email}." with the button "Send the link again", and reaches Home, My tutors and Settings only; the sidebar shows only those sections, and below 1024 px the top bar has no notifications bell.
- **In slice 3** Home shows the tutor parts (no tutor yet, request sent, My tutors) from `GET /tutors` and `GET /join-requests/sent`; the rest arrives with `GET /home` in slice 10.
- **Computer layout:** greeting and a one-line summary of the day, the tutor filter, then a main column (next lesson panel, to do, recently checked) and a 300 px side column (next seven days, my tutors).
- **Phone:** columns stack with the next lesson first, and the Join button stays within thumb reach.

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

FR-2.6, FR-1.2, FR-2.7, FR-2.8, FR-1.7.

- **Rows:** each tutor's avatar, name, colour, subjects and bio, plus "Message" (from slice 6) and, in an overflow menu, "Stop studying".
- **Stop studying** opens a dialog naming the consequences (the student leaves the tutor's groups, future lessons are cancelled, the chat becomes read-only, the tutor gets an email); for a tutor joined before the email was confirmed it says that the link is removed instead.
  - The dialog opens on Cancel, which returns focus to the row's overflow menu button; its button is "Stop studying".
  - Afterwards the notice "You no longer study with this tutor. The tutor is listed under Past tutors." (or, for a link made before the confirmation, "The link to the tutor was removed.") takes focus above the list; a refusal because the link changed meanwhile closes the dialog, reloads the list and shows "Your tutors changed in the meantime. The list below is up to date.", which takes focus.
  - When a live refetch removes the dialog's tutor (the tutor removed the link) or ends the relationship while the dialog is open and no stop is in flight, the dialog closes and "The link to the tutor was removed." or the changed-meanwhile notice takes focus (slice 4).
- **Add a tutor:** a field that takes a code, with "Send request". A sent code shows the notice "Request sent. Waiting for the tutor.", which takes focus, and empties the field; the answer is the same for a new, repeated, held or suppressed request (T-33). Refusals show an alert that takes focus: "This code is no longer valid — ask your tutor for a new one" (`tutor_code_invalid`), "You already study with this tutor." (`already_linked`), "The tutor is not accepting requests right now." (`tutor_unavailable`), and the throttling message with the minutes to wait (`rate_limited`).
- **Pending requests:** "Request sent. Waiting for the tutor." (no name interpolation, so no grammatical case problems in Russian); a declined request shows "Your request was not accepted".
  - In slice 3 they are listed under "Sent requests", newest first, from `GET /join-requests/sent`: each row shows the state word from [states](states.md) ("Request sent", "Not accepted", "Expired"), the sentence for a pending or declined request and "Sent {date}"; accepted requests are left out, because their tutor is listed; the rows carry no tutor name, because the student-facing request has none; "Show more" loads the next page and moves focus to its first row. The API offers no withdrawal, so a request is never withdrawn from the page.
- **Ended relationships:** listed under "Past tutors" with read-only chat, lessons, recordings until they expire, and results. They are left out of the Home filter by default. In slice 3 each row shows the avatar, name, colour, subjects and "Ended {date}".
- **A tutor who deleted their account:** shown as "Account closed — results available until {date}", with read-only access until then; the date is `account_closed_until`.

### Keep or drop earlier links

FR-1.2. Address `/reclaim`, reached after a confirmation or reset that answers `choice_required`, and after any sign-in while a choice is pending.

- **Explains** that someone used this email before it was confirmed, and that nothing they started continues without the owner's choice.
- **The account:** its current name and avatar with a link to edit them, before anything is kept.
- **Items:** each tutor link and each request with the tutor's avatar, name and subjects, and Keep or Drop; dropped items are removed and their tutors are not told.
- **Deadline:** "Choose by {date}; anything not kept by then is dropped".
- **Saving** sends every choice at once; a refusal reloads the list with "The list has changed. Check your choices and save again". After saving, the page shows the outcome (as on the confirmation page, or "Your choices are saved") with "Continue" to Home; opened when nothing waits, it says "Nothing is waiting for your choice".

### Messages and settings (student)

Same as the tutor's, without tutor-only sections; direct channels are named by tutor, group channels by group with the tutor label.

## Shared behavior

- **Loading:**
  - skeletons in the shape of the content appear after 300 ms; nothing appears before;
  - buttons that start an action show a spinner and stay unavailable until it completes: they carry `aria-disabled="true"` and `aria-busy="true"` and ignore activation, but never the `disabled` attribute, which moves focus to the page body in Chromium, Firefox and WebKit; so a busy button keeps focus, and a button waiting for a cooldown (the confirmation resend) is unavailable the same way;
  - where several controls share one action (the Accept, Decline and menu buttons of every join-request row, the Remove buttons of the group members, the Unblock buttons of the blocked students, Send and "Create a new link" of an invitation link), only the control that started it carries `aria-busy="true"`; the others carry only `aria-disabled="true"` until it completes; a decision chosen from a row's menu marks that row's menu button busy.
- **Empty states:** a heading, one sentence and one primary action (an optional line illustration as in [tokens](tokens.md#illustration-and-iconography)):

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
  - a stale edit (a template, a review draft) shows "This changed elsewhere" with "Reload" and keeps the user's unsaved text visible;
  - the edit forms of a student card (name, email, notes) and a group (name, colour) stay mounted, so focus stays where the page section puts it; when a field's stored value changes, the field shows it at once only while the user has not changed the field since it last showed a stored value, so a background refetch never overwrites typing; after a reload because the card or group changed meanwhile (a refusal that shows the changed-meanwhile notice, or an edit refused as stale), every field whose stored value changed since it last showed one shows the current value, and the other fields keep the unsaved typing.
- **Offline** (slice 6, with chat, whose composer first needs it; until then pages keep working through REST while the realtime client reconnects): a banner "No connection — reconnecting…" in the signed-in frame, above the page's content and below the narrow top bar, outside the page itself, so it never takes focus and the page's heading stays first; a polite live region announces it once when it appears and once when the connection returns.
  - Assignment answers are kept and saved when the connection returns.
  - Other actions are disabled while offline, with the reason in a tooltip.
  - The realtime client reconnects with backoff and refetches visible data on reconnect (slice 4).
- **Live data:** frames from the realtime channel trigger a refetch of the affected item and never replace it directly ([live refetch mapping](../../architecture/detailed.md#live-refetch-mapping)). Lists keep the scroll position.
  - **Focus on live refetch:** a refetch that keeps the focused element leaves focus where it is. When a live refetch removes the focused row of a list, focus moves to the row that took its place, else to the previous row, else to the list's heading, or to the page's heading for a list without one or a list that disappears with its last row. When it removes the subject of an open dialog, the dialog closes and the page's notice that says so takes focus: "This student is no longer on your list.", "This request was already answered." and similar, each defined by its page; for a dialog without such a notice, the page's changed-meanwhile notice is used. Each of these moves has a `toBeFocused` test.
  - **Pages with live refetch in slice 4:** Students (its tabs and pending requests), the student card, groups and the group page, Today's join-request panel, My tutors and student Home.
- **Time:**
  - every time is shown in the viewer's timezone;
  - the lesson page adds the other party's time when it differs, using the `time.other_zone` string from [states](states.md#time-and-scores) with the city of the IANA timezone taken from a bundled, localised table;
  - deadlines show both an absolute time and a relative hint ("in 2 days").
- **Responsive:**
  - "phone" means narrower than 768 px; every page works from 360 px;
  - every page, for both roles, is designed for a 1280 px computer screen first and degrades to a single column ([ADR-0010](../../../docs/decisions/0010-quiet-visual-language.md));
  - dedicated phone layouts are designed in slice 10 of the [roadmap](../../../docs/roadmap.md); until then the rules marked **Phone** above are the minimum.
- **Keyboard and screen readers:**
  - every action is reachable by keyboard in visual order, with a visible focus ring;
  - dialogs trap focus and restore it to the opener on close, unless the page section names another target (a notice) or the action changes the route, as for every route change; a dialog opens with focus on the element its page section names, a destructive confirmation without fields on its safe choice (Cancel), one that asks for a password on that field, and otherwise on its first field;
  - a page's loading, error and refetching states render inside the page, so they never take focus away; after Retry, once the error is gone, focus moves once to the page heading when focus was lost;
  - from slice 4 each route's page code loads on demand: while it loads, the route's page shows its heading with the delayed loading state, and when the loaded page replaces it, focus that was on the loading page's heading moves to the loaded page's heading; when the code cannot be loaded, the page shows its heading and the alert "This page could not be loaded. Check your connection and reload the page." with the button "Reload the page", which reloads the document, because a browser keeps a failed module load for the life of the document;
  - "Show more" moves focus to the first new row; when the loaded page adds no visible row (its rows are all left out, as accepted requests are under Sent requests and students who are not active or already members are in the member choice) and "Show more" disappears with focus on it, focus moves to the last row shown, or, when no row is shown, to the page heading (in the member choice, to its search field);
  - a notice that reports a refusal takes focus when it appears, and again each time it reports a new refusal; a notice that reports a result takes focus when it replaces or follows the control that triggered it and focus was lost or was on that control; otherwise a live region announces it;
  - live regions announce new messages, saving status and toasts;
  - icons that act as buttons have accessible names;
  - charts have a table alternative;
  - every page has a "Skip to content" link, and route changes move focus to the page heading;
  - lesson status changes ("Live now", the Join button appearing) are announced, while countdowns are not;
  - typing dots and the live pulse stop with reduced motion.
