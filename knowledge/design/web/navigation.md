# Navigation and addresses

Every page of the web application, its address, who reaches it and where navigation leads. Page contents are in [pages](pages.md); state words and colours are in [states](states.md).

The issue page's Analytics tab and the lesson page's recordings come [after the MVP](../../../docs/roadmap.md#after-the-mvp) by the owner's decision of 2026-10-22.

## Frames

- **Public frame:** logo, language switch (Русский / English), and sign-in or sign-up links. Used for every page reachable without a session. For a signed-in person on a public page the language switch also saves the account's language, and says so when that save fails, so the next start-up and the emails keep the choice. These saves run one after another in the order the choices were made, a choice overtaken by a later one, or by a sign-out or a change of account, before its turn is not sent, and only the latest choice's answer updates the account shown or the failure message, so the latest choice is the one the account keeps (from slice 10).
- **Signed-in frame (both roles)**, styled per [tokens](tokens.md#components) and decided in [ADR-0010](../../../docs/decisions/0010-quiet-visual-language.md):
  - a 248 px left sidebar on the `sidebar` background, full height, scrolling on its own;
  - at the top, the user's avatar initial and name; clicking it opens the account menu (Settings, Sign out); next to it, a "Hide sidebar" icon button;
  - below, the role's sections as items with an outline icon, a label and an optional count, then Notifications (with the unread count, "99+" above 99; it opens the notifications panel, [pages](pages.md#notifications); in the sidebar overlay below 1024 px it is a link to the page, because the top bar's Notifications button opens the panel) and Settings at the end of the list;
  - when hidden on a computer, a "Show sidebar" icon button stays in the top-left corner and carries an `accent-text` dot when any section count or the unread-notification count is above zero (its accessible name then ends with "new items" / "есть новое"); the choice is remembered per browser;
  - the page itself may have a thin top bar with breadcrumbs and the page's own actions (the builder, review and lesson pages), never a second navigation.
- **Tutor sections:** Today, Schedule, Students, Groups, Homework (count: needs checking), Messages (count: unread). Homework opens `/homework/library` and has three tabs: Library, Issued and Needs checking. In slice 8 it showed the Library tab only and no count (pending owner review). From slice 9:
  - the tab list (Library, Issued, Needs checking) stands under each Homework page's header as links to `/homework/library`, `/homework/issued` and `/homework/review`, the current one marked with `aria-current="page"`; the builder, the issue page and the review screen keep their back links instead of the tab list;
  - the count is `count` of `GET /review-queue?limit=1`, which the API caps at 100, shown as "99+" above 99 and hidden at zero; the item's accessible name includes it ("Homework, 3 to check"; above 99 "Homework, more than 99 to check"); it refetches on `entity.changed` of kind `submission` and on reconnect;
  - an unconfirmed tutor, whose issue and queue reads the API refuses, sees no count, and the Issued and Needs checking tabs show the confirmation prompt "Confirm your email to issue and check assignments." without requesting either route.
- **Student sections:** Home, Schedule, Homework (count: to do), Messages (count: unread), My tutors. From slice 9 the Homework count is `counts.todo` of `GET /assignments?status=todo&limit=1`, capped at 100 by the API, shown as "99+" above 99 and hidden at zero, with the accessible name "Homework, {count} to do" ("Homework, more than 99 to do" above 99); it refetches on `entity.changed` of kind `submission` and on reconnect.
- **Text loading:** the section names and the counts' accessible names are in the global language chunk, and so are the phone frame's new texts "Chats" with its two counted names, "More" and "Close" (slice 10 change G0, [texts](states.md#phone-frame)); the tutor's Homework tab names and the unconfirmed-tutor prompt load with the `homework` feature section, which every tutor Homework page loads, and the student's Homework tab names with the `assignment` feature section, which the student's Homework list loads (agent changes pending owner review); every Homework page's own texts load with its feature section ([localisation](states.md#localisation)).
- **From 768 to 1023 px (the interim frame for both roles; it applied below 768 px too until the phone frame below replaces it there with slice 10 change G1):**
  - a top bar shows the "Show sidebar" button (with the same dot and accessible name), the page title and the Notifications button with its unread count as a number, so notifications stay one tap away;
  - the sidebar opens as a modal overlay over a dimmed page: focus moves to the current item, Tab stays inside, `Esc` or a click on the dimmed page closes it and returns focus to the "Show sidebar" button; choosing an item closes it and navigates, and focus then moves to the new page's heading as for every route change;
  - every touch target in the frame is at least 44 × 44 px.
- **Phone frame, below 768 px** (slice 10 change G0, owner's decisions of 2026-11-21, pending owner review; built in change G1; mockups in [the phone prototype](prototype/phone.html)). It replaces the interim frame on phones for both roles; from 768 px the interim frame above and the sidebar from 1024 px are unchanged, and a window that crosses 768 px (for example a phone turned to landscape) switches frames without remounting the page: an open More sheet or notifications panel closes without moving focus back to its opener, focus inside the page stays where it is, and focus on a frame control that the other frame does not have (a bottom bar item, More, a row of the sheet, the overlay's "Show sidebar") moves to the page heading:
  - **Order in the document:** the skip link "Skip to content" first, then the top bar, the banners (confirmation, offline), the page's `main`, and the bottom bar last, so Tab reaches the page before the bottom bar and the bottom bar's `nav` is the frame's navigation landmark, named "Main navigation".
  - **Top bar:** 48 px high on the `bg` background with a 1 px `border` rule below it, scrolling away with the page; it holds the OVC logo, linking to the role's home page, and at its right end the Notifications button with its unread count as a number ("99+" above 99), which opens the notifications panel ([pages](pages.md#notifications)). The page's own heading stays the first element of `main` and keeps every route-focus rule; the top bar shows no title, so the page title is never shown twice. An unconfirmed student's top bar has no Notifications button.
  - **Bottom bar:** fixed to the bottom of the window, 56 px high plus the device's bottom safe area (`env(safe-area-inset-bottom)`), on the `sidebar` background with a 1 px `border` rule above it. It holds five items of equal width, each a 20 px outline icon above a short label in `text-caption`, at least 44 × 44 px: for a tutor Today, Schedule, Homework, Chats and More ("Сегодня", "Расписание", "Задания", "Чаты", "Ещё"); for a student Home, Schedule, Homework, Chats and More ("Главная", "Расписание", "Задания", "Чаты", "Ещё"). The first four are links; the current one carries `aria-current="page"`, its icon and label in `text`, the label at weight 600, on a `hover` pill behind the icon. Every Homework page, every Messages page and every lesson page (`/lessons/{id}`, whatever page it was opened from) marks its item current; a page that the More sheet lists marks More with the same visual state but without `aria-current`, because More is a button, and the sheet's row of that page carries `aria-current="page"`. Labels never truncate: at 360 px each item is 72 px wide, and the longest label, "Расписание", fits in `text-caption`; where a label does not fit (at 320 px, or with WCAG 1.4.12 text spacing) it wraps onto a second line and the bar grows in height, and the page's bottom padding follows the bar's real height.
  - **Counts in the bottom bar:** Homework and Chats show the counts of the sidebar (to check or to do; unread channels) as a badge on the icon's top right corner in `text-caption`, "99+" above 99, hidden at zero, and each link's accessible name starts with its visible label (WCAG 2.5.3): Homework keeps the sidebar item's names ("Homework, 3 to check", "Homework, more than 99 to do"), and Chats is named "Chats" at zero, otherwise "Chats, {count} unread" ("Чаты, непрочитанных: {count}") and above 99 "Chats, more than 99 unread" ("Чаты, непрочитанных: больше 99"). They refetch as the sidebar's counts do. More carries no badge or dot: of the sheet's rows only Notifications shows a count, and the unread notifications already show on the top bar's button.
  - **More sheet:** "More" is a button that opens a modal sheet from the bottom edge over the dimmed page (a native modal dialog, named "More"), at most 80 % of the window's height and scrolling on its own. It holds, in order: the account's avatar initial, name and role word as text; the role's remaining sections (tutor: Students, Groups; student: My tutors); Notifications, a link to the page with the unread count, because the panel does not fit beside the sheet; Settings; Sign out; and a "Close" button. Each row is at least 48 px high with a 20 px icon. On opening, focus moves to the current page's row when the sheet lists it, otherwise to the first section row (the account's name and role are text, not a control); Tab stays inside; `Esc` (closed natively by the browser), a tap on the dimmed page or "Close" closes it and returns focus to "More". Choosing a row closes the sheet and navigates, and focus moves to the new page's heading as for every route change. Sign out in the sheet keeps the account menu's behaviour: "Signing out…" while busy, the sign-out rules below, and on failure "Could not sign out. Check your connection and try again." inside the sheet, which keeps focus on Sign out; when the sheet was closed while the sign-out was in flight, the failure shows in an alert above the page instead and focus moves to "More". The sheet closes when the account changes, because the signed-out state unmounts the frame. The account menu of the sidebar is not shown on phones; the sheet is its phone form.
  - **Unconfirmed accounts:** an unconfirmed student's bottom bar shows Home, then Schedule, Homework and Chats only as the unverified rule below widens access to them (a provisional relationship), then More, whose sheet lists My tutors, Settings and Sign out and no Notifications; items are never shown greyed out, and with fewer items each item stays a fifth of the width, centred as a group. An unconfirmed tutor sees the full bar with no Homework count, as in the sidebar. The bar, like the sidebar, only hides what the API refuses and decides nothing ([`src/routes/access.ts`](#rules)).
  - **The page above the bar:** `main` ends with bottom padding equal to the bar's height and safe area, and the document sets `scroll-padding-bottom` to the same value, so no content and no focused element is ever hidden under the bar (WCAG 2.4.11); toasts and menus that open near the bottom edge are placed above the bar. The conversation screen of Messages, which has its own Back to the list and a composer at the bottom, hides the top bar and the bottom bar while it is shown; full-screen dialogs cover it.
  - **Targets and icons:** every target in the phone frame is at least 44 × 44 px; every outline icon of the frame and of the pages is 20 px below 768 px ([tokens](tokens.md#illustration-and-iconography)).
  - **Public frame on phones:** the logo, the language switch and the sign-in or sign-up link on one row that wraps when the language's labels need it; no bottom bar.

## Public addresses

Pending the change that follows [ADR-0024](../../../docs/decisions/0024-legal-documents-and-student-data.md) (accepted 2026-11-23), the consent checkbox links a separate consent text, the sign-up and invitation pages gain a line accepting the user agreement, and the invitation page always links the privacy policy (pending owner review).

| Address | Page | Notes |
|---|---|---|
| `/` | Sign in | Signed-in users are redirected to their home page |
| `/signup` | Choose: "I teach" or "I study" | |
| `/signup/tutor`, `/signup/student` | Sign-up form | Consent checkbox, language preselected from the interface language, which follows the browser until the person chooses one; `/signup/student` is the sign-up with a required tutor code, with a hint to open the tutor's link instead ([ADR-0014](../../../docs/decisions/0014-students-join-through-their-tutor.md)) |
| `/verify-email?token=` | Email confirmation result | Then continues to the home page, or to `/reclaim` when the outcome is `choice_required`; without a token, a signed-in unverified account sees "Check your email" with a resend button |
| `/forgot-password`, `/reset-password?token=` | Password reset | After a reset answering `choice_required`, the next sign-in lands on `/reclaim` |
| `/invite/{token}` | Accept invitation | Reachable signed in or out, also while a reclaim choice is pending; shows the tutor; create an account or sign in in place, or accept while signed in; the address becomes `/invite` on the first render, and `/invite` alone asks to open the link again |
| `/unsubscribe?token=` | Unsubscribe confirmation | Works without signing in and never signs in; the address becomes `/unsubscribe` on the first render, and `/unsubscribe` alone shows the invalid-link message |
| `/privacy` | Privacy policy | Linked from the consent checkbox; a placeholder until the owner supplies the text |

## Tutor addresses

| Address | Page |
|---|---|
| `/today` | Today (home) |
| `/schedule?view=week\|month&date=` | Schedule |
| `/lessons/{id}` | Lesson |
| `/students?state=&group=&q=` | Students |
| `/students/{id}` | Student card |
| `/groups`, `/groups/{id}` | Groups, group page |
| `/homework/library?folder=&q=` | Library; from slice 8. `folder` and `q` are not sent together: a search ignores the folder, and an unknown folder shows Not found |
| `/homework/templates/{id}` | Builder; from slice 8. "New assignment" in the library creates a template titled with the localised "New assignment" and opens it here; a deleted or foreign template shows Not found |
| `/homework/issued?status=active\|withdrawn` | Issued work; from slice 9, `active` when no status is given |
| `/homework/issued/{id}` | Issue page with its submissions; from slice 9. The Analytics tab comes after the MVP; a foreign or unknown issue shows Not found |
| `/homework/review?issue=&group=&student=` | Needs checking; from slice 9. A filter the API refuses shows "This filter is not available." |
| `/homework/review/{submissionId}?issue=&group=&student=` | Review; from slice 9. The filters are the queue's, kept for "Back to the queue" and "Next submission"; a submission of another tutor shows Not found |
| `/messages`, `/messages/{channelId}` | Messages |
| `/notifications` | Notifications |
| `/settings/profile`, `/settings/account`, `/settings/notifications`, `/settings/teaching` | Settings sections |

## Student addresses

| Address | Page |
|---|---|
| `/home?tutor=` | Home |
| `/schedule?view=week\|month&date=&tutor=` | Schedule (agenda on phones, week or month wider; `view` and `date` as for tutors, owner's decision of 2026-10-28, pending owner review) |
| `/lessons/{id}` | Lesson |
| `/homework?status=todo\|submitted\|checked\|closed&tutor=` | Homework; from slice 9, `todo` when no status is given |
| `/homework/{submissionId}` | Assignment: take it, or see results; from slice 9. Another student's submission, a withdrawn or erased one, and work of a `reclaim_pending` link show Not found |
| `/messages`, `/messages/{channelId}` | Messages |
| `/tutors` | My tutors, enter a tutor code, stop studying |
| `/reclaim` | Keep or drop the links and requests made before the email was confirmed; sign-in lands on it while `GET /users/me` reports `reclaim_pending: true`; opened when nothing waits, it says so; a tutor gets Not found |
| `/notifications` | Notifications |
| `/settings/profile`, `/settings/account`, `/settings/notifications` | Settings sections |

## Rules

- **After sign-in:** a tutor goes to `/today`, a student to `/home`, and an account whose `GET /users/me` reports `reclaim_pending: true` to `/reclaim`, even when a `next` parameter names another address. Such an account may still open an invitation and accept it; the invitation page then links to `/reclaim` ("You have links waiting for your choice"). A `next` parameter is honoured only when it starts with exactly one `/` (not `//` or `/\`), and when resolving it against the OVC origin with the URL parser yields the same origin; anything else is ignored.
- **Wrong role:** a tutor address opened by a student (or the reverse) shows Not found inside the user's own frame, never a hint that the address exists. An address both roles share opens in the signed-in user's own frame.
- **Anonymous visitors** opening a private address are redirected to `/?next=<address>`, with the path and query encoded; after an explicit sign-out or a block the redirect goes to `/` without `next`.
- **While the session is loading** at start-up, every address shows a loading status; when the server cannot be reached, a message with a retry button. Retry unmounts the message, so once the session check settles (the page, a redirect or Not found), focus moves once to the page heading when it was lost; a redirect moves it as every route change does.
- **Account change:** when the signed-in account of a tab changes (a sign-out, a block, a sign-in as another account), the loaded data is cleared at once, and every change passes through the signed-out state, which unmounts the private pages, so no page shows data or field values of the previous account. The one deliberate exception is the invitation page, which keeps its token in component state across a sign-out and a sign-in so the person can accept with the other account ([threat model](../../../docs/threat-model.md), T-27).
- **Unverified accounts:**
  - a tutor sees a confirmation banner on every page;
  - an unverified tutor trying to invite, schedule a lesson or regenerate the tutor code gets the confirmation prompt instead;
  - an unverified student sees a confirmation banner on every page, with the email and a button that sends the link again, and reaches Home, My tutors and Settings, which keep sign-out, the language switch and deletion reachable; the sidebar shows only those sections, the narrow top bar has no notifications bell, and on phones the bottom bar and the More sheet show only those sections (from slice 10 change G1); every other private address redirects to `/verify-email`; this is widened in slices 5, 6 and 9 to the provisional relationship's lessons, chat and work: from slice 5 an unconfirmed student whose `GET /tutors` lists a provisional relationship also sees Schedule in the sidebar and reaches `/schedule` and `/lessons/{id}`, which show only that relationship's one-to-one lessons ([visibility](../../specs/schedule.md#visibility)), and from slice 6 also Messages, reaching `/messages` and `/messages/{channelId}`, which show only that relationship's direct chat, with text messages only; from slice 9 also Homework, with its count, reaching `/homework` and `/homework/{submissionId}`, which show only that relationship's work, answerable except for file questions until the email is confirmed; every other address still redirects;
  - the rule is the access rule in `src/routes/access.ts`, unit-tested as a permission rule; it hides what the API would refuse and decides nothing the API does not.
- **Other refusals:**
  - `403 forbidden` shows a toast explaining the action is not allowed;
  - `account_blocked` and a WebSocket close with `4401` that a token refresh cannot fix sign the user out with a message. The session's refresh reports its result instead of a boolean: renewed, or ended with a reason. A refresh refused with `403 account_blocked` ends the session as blocked, with the blocked message; any other refusal ends it with the reason `sessionEnded` and the message "You were signed out on this device" ([states](states.md#notifications)), because a revoked and a deleted session cannot, and must not, be told apart. A refused refresh ends a signed-in session as `sessionEnded` whatever started the refresh (a `4401` close or an expired token on a request), and so does a `401` on any request of a signed-in session, and, like a sign-out or a block, returns to `/` without `next`; a refresh at start-up that finds no session leaves the visitor anonymous, with `next`.
- **Route changes commit synchronously** (slice 9 change F3, agent change pending owner review): every client-side navigation (links, `navigate`, redirects, and Back and Forward) renders the new route, unmounts the page it leaves and ends that page's query subscriptions in the same task as the address change (`withSynchronousNavigation` in `src/routes/router.tsx`, with `flushSync` passed to `RouterProvider`), so no read of the left page can start after the address changed, for example a live refetch's follow-up; `src/routes/router.test.tsx` proves it. Back and Forward (`popstate`) reach the router without a synchronous option, so the router's subscribers receive every update that changes the location with `flushSync` set (slice 10 change X2, owner decision of 2026-11-16, pending owner review). In a browser React 19 already renders a transition scheduled during a `popstate` event eagerly, in the microtask that runs when the router's listener returns, but only once per event and only for a real `popstate` event; the wrapper makes the synchronous commit a guarantee independent of that heuristic. `src/routes/router.test.tsx` proves the wrapper on a Back navigation without a browser event, and the students test of Back while the card is still loading guards the behaviour in each browser engine. A redirect made from an effect (`<Navigate>`, the token holders) now calls `flushSync` while React runs effects, which React's development build reports as a console error; the production build does not, and the update still commits at the end of that effects pass. A modal dialog that a navigation removes while it is open (a link or a completed action inside it that opens another page) closes itself before it leaves the document (`ModalDialog` in `src/components/ui/Dialog.tsx`): WebKit refuses to focus an element outside a modal dialog removed while open in the same task, so without it the new page's heading would not receive focus; the students test of Open the card and step 4 of pilot scenario 2 prove it in WebKit.
- **A deep link** to a resource the user cannot see shows Not found (`404` from the API).
- **Signing out** returns to `/` and clears cached data. The tab's own sign-out, sign out everywhere or account deletion first waits until the tab's reads already in flight (`GET` requests) are answered, for at most 15 seconds, and only then sends its own request, so the server does not check a read of the tab after the action revoked its session unless that read hung past the bound; a write in flight, such as a join or an upload, is not waited for and its late answer is dropped as the page's rules say. While the action is in flight, every new API request of the tab waits for it, as the socket's refresh after `4401` does ([realtime](../../specs/realtime-and-video.md)): when the action ended the session, the request is never sent and fails as aborted; when the action failed, it is sent as usual. Only the action's own request is not held, and the session ends before the held requests resume, so no request sent after the action began carries a token that the action revoked.
- **Notification links** open the related page: in slice 4 the student card (`student_card`), the Students page, whose pending requests list the request (`join_requests`), and My tutors (`my_tutors`); from slice 5 the lesson (`lesson`) and from slice 9 the assignment page `/homework/{submissionId}` (`assignment`, a student's link) and the review screen `/homework/review/{submissionId}` (`review`, a tutor's link). A `group.archived` row has no link. A link the API returns as `null`, because its subject is outside the caller's current scope, is not shown as a link.
- **Filters and tabs** that change what is shown are reflected in the address, so reload and Back keep the view.

## Token pages

`/invite/{token}`, `/verify-email?token=`, `/reset-password?token=` and `/unsubscribe?token=` follow one rule:

- the static server sends `Referrer-Policy: no-referrer` for them, as for every page;
- the page reads the token, then removes it from the address bar with `history.replaceState`; every token page does so in a layout route above the session check, which renders nothing below it while the address still carries a token, so the address changes on the first render, even while the session is loading or the server is unavailable, and before the page's own code is requested;
- the invitation page keeps its token in memory only: signing in or creating an account happens in place, and the token never travels in `next` or any other address;
- the unsubscribe page keeps its token in component state only, like the invitation page, and never in `next`;
- the address replacement runs in the entry chunk's static imports, so the token leaves the address before any lazily loaded route code is requested or runs (from slice 4 each route's page module loads on demand); the active language's chunk, which holds only interface texts, is requested before the first render and may be requested while the address still carries the token ([localisation](states.md#localisation));
- the page loads nothing from other origins (fonts and images are self-hosted; avatars come from the OVC origin);
- the reverse proxy logs them with the token replaced by a placeholder ([observability](../../specs/observability.md#safe-content)).

### Refused refresh response ownership

A recognized refresh401/204 ends the old session and releases its token/private-cache ownership
before its response body finishes. The same existing 15-second request signal bounds a drain with a 64-KiB
limit, read whole for a declared length and otherwise streamed ([accounts](../../specs/accounts.md));
a streamed overflow cancels the owned body and releases its reader, and body failures reject refresh/authorized-request operations. The startup error-to-state wrapper keeps its
existing signed-out/new-owner behavior and does not reinterpret body failure as HTTP completion.
The Web Lock covers the drain. A queued callback checks its captured session epoch after acquiring
the lock and before sending a cookie-authenticated refresh; replacement-account adoption makes it
superseded without an outbound request. Completion or failure of an old body cannot restore tokens,
clear a newer account's private cache or replay the original request. The production cache-ownership
binding remains ordered before preference updates and is shared with the real Gate/router controls.

### Document-owned follow-up reads

The page installs one document lifecycle owner synchronously after QueryClient creation and before
Session startup. Query follow-ups and the lazily installed realtime consumer share this owner.
A pending second refetch wave retains its entry activity and generation; departure, resumption or
account cache clearing invalidates that ownership before it may issue another request. A wave
born suspended cannot adopt a later active document. First-wave handling and explicit throwOnError
semantics remain unchanged.

The owner holds the existing load/beforeunload/pagehide/pageshow listeners and one two-second
cancelled-navigation resume timer. Hidden or disposed documents do not connect when realtime
loads late. Cancelled navigation and BFCache restoration establish new active ownership and perform
normal realtime reconciliation. Disposal removes listeners, cancels the timer, unsubscribes
realtime and closes its owned transport. Existing request and browser failures remain failures.
