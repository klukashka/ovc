# Navigation and addresses

Every page of the web application, its address, who reaches it and where navigation leads. Page contents are in [pages](pages.md); state words and colours are in [states](states.md).

The issue page's Analytics tab and the lesson page's recordings come [after the MVP](../../../docs/roadmap.md#after-the-mvp) by the owner's decision of 2026-10-22.

## Frames

- **Public frame:** logo, language switch (Русский / English), and sign-in or sign-up links. Used for every page reachable without a session. For a signed-in person on a public page the language switch also saves the account's language, and says so when that save fails, so the next start-up and the emails keep the choice.
- **Signed-in frame (both roles)**, styled per [tokens](tokens.md#components) and decided in [ADR-0010](../../../docs/decisions/0010-quiet-visual-language.md):
  - a 248 px left sidebar on the `sidebar` background, full height, scrolling on its own;
  - at the top, the user's avatar initial and name; clicking it opens the account menu (Settings, Sign out); next to it, a "Hide sidebar" icon button;
  - below, the role's sections as items with an outline icon, a label and an optional count, then Notifications (with the unread count, "99+" above 99; it opens the notifications panel, [pages](pages.md#notifications)) and Settings at the end of the list;
  - when hidden on a computer, a "Show sidebar" icon button stays in the top-left corner and carries an `accent-text` dot when any section count or the unread-notification count is above zero (its accessible name then ends with "new items" / "есть новое"); the choice is remembered per browser;
  - the page itself may have a thin top bar with breadcrumbs and the page's own actions (the builder, review and lesson pages), never a second navigation.
- **Tutor sections:** Today, Schedule, Students, Groups, Homework (count: needs checking), Messages (count: unread). Homework opens `/homework/library` and has three tabs: Library, Issued and Needs checking.
- **Student sections:** Home, Schedule, Homework (count: to do), Messages (count: unread), My tutors.
- **Below 1024 px (interim frame for both roles, including phones until slice 10):**
  - a top bar shows the "Show sidebar" button (with the same dot and accessible name), the page title and the Notifications button with its unread count as a number, so notifications stay one tap away;
  - the sidebar opens as a modal overlay over a dimmed page: focus moves to the current item, Tab stays inside, `Esc` or a click on the dimmed page closes it and returns focus to the "Show sidebar" button; choosing an item closes it and navigates, and focus then moves to the new page's heading as for every route change;
  - every touch target in the frame is at least 44 × 44 px.
- **Phones (slice 10):** dedicated phone layouts and a possible bottom bar (Home, Schedule, Homework, Messages, More, with the short Russian labels "Главная", "Расписание", "Задания", "Чаты", "Ещё") are designed in slice 10 of the [roadmap](../../../docs/roadmap.md); until then the interim frame above applies.

## Public addresses

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
| `/homework/library?folder=&q=` | Library |
| `/homework/templates/{id}` | Builder |
| `/homework/issued?status=` | Issued work |
| `/homework/issued/{id}` | Issue page with Submissions and Analytics tabs |
| `/homework/review` | Needs checking |
| `/homework/review/{submissionId}` | Review |
| `/messages`, `/messages/{channelId}` | Messages |
| `/notifications` | Notifications |
| `/settings/profile`, `/settings/account`, `/settings/notifications`, `/settings/teaching` | Settings sections |

## Student addresses

| Address | Page |
|---|---|
| `/home?tutor=` | Home |
| `/schedule?tutor=` | Schedule (agenda on phones, week or month wider) |
| `/lessons/{id}` | Lesson |
| `/homework?status=todo\|submitted\|checked&tutor=` | Homework |
| `/homework/{submissionId}` | Assignment: take it, or see results |
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
  - an unverified student sees a confirmation banner on every page, with the email and a button that sends the link again, and reaches Home, My tutors and Settings, which keep sign-out, the language switch and deletion reachable; the sidebar shows only those sections and the narrow top bar has no notifications bell; every other private address redirects to `/verify-email`; this is widened in slices 5, 6 and 9 to the provisional relationship's lessons, chat and work;
  - the rule is the access rule in `src/routes/access.ts`, unit-tested as a permission rule; it hides what the API would refuse and decides nothing the API does not.
- **Other refusals:**
  - `403 forbidden` shows a toast explaining the action is not allowed;
  - `account_blocked` and a WebSocket close with `4401` that a token refresh cannot fix sign the user out with a message. The session's refresh reports its result instead of a boolean: renewed, or ended with a reason. A refresh refused with `403 account_blocked` ends the session as blocked, with the blocked message; any other refusal ends it with the reason `sessionEnded` and the message "You were signed out on this device" ([states](states.md#notifications)), because a revoked and a deleted session cannot, and must not, be told apart.
- **A deep link** to a resource the user cannot see shows Not found (`404` from the API).
- **Signing out** returns to `/` and clears cached data.
- **Notification links** open the related page: in slice 4 the student card (`student_card`), the Students page, whose pending requests list the request (`join_requests`), and My tutors (`my_tutors`); from later slices the lesson, submission, review and channel. A link the API returns as `null`, because its subject is outside the caller's current scope, is not shown as a link.
- **Filters and tabs** that change what is shown are reflected in the address, so reload and Back keep the view.

## Token pages

`/invite/{token}`, `/verify-email?token=`, `/reset-password?token=` and `/unsubscribe?token=` follow one rule:

- the static server sends `Referrer-Policy: no-referrer` for them, as for every page;
- the page reads the token, then removes it from the address bar with `history.replaceState`; the invitation page does so in a layout route above the session check, so the address changes on the first render, even while the session is loading or the server is unavailable;
- the invitation page keeps its token in memory only: signing in or creating an account happens in place, and the token never travels in `next` or any other address;
- the unsubscribe page keeps its token in component state only, like the invitation page, and never in `next`;
- the address replacement runs in the entry chunk, so the token leaves the address before any lazily loaded route code runs (slice 4 loads each route's page module on demand);
- the page loads nothing from other origins (fonts and images are self-hosted; avatars come from the OVC origin);
- the reverse proxy logs them with the token replaced by a placeholder ([observability](../../specs/observability.md#safe-content)).
