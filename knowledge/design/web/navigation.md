# Navigation and addresses

Every page of the web application, its address, who reaches it and where navigation leads. Page contents are in [pages](pages.md); state words and colours are in [states](states.md).

## Frames

- **Public frame:** logo, language switch (Русский / English), and sign-in or sign-up links. Used for every page reachable without a session.
- **Signed-in frame (both roles)**, styled per [tokens](tokens.md#components) and decided in [ADR-0010](../../../docs/decisions/0010-quiet-visual-language.md):
  - a 248 px left sidebar on the `sidebar` background, full height, scrolling on its own;
  - at the top, the user's avatar initial and name; clicking it opens the account menu (Settings, Sign out); next to it, a "Hide sidebar" icon button;
  - below, the role's sections as items with an outline icon, a label and an optional count, then Notifications (with the unread count) and Settings at the end of the list;
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
| `/signup/tutor`, `/signup/student` | Sign-up form | Consent checkbox, language preselected from the interface language, which follows the browser until the person chooses one; until slice 3 `/signup/student` only explains that students join through their tutor ([proposal 0002](../../../docs/proposals/0002-student-onboarding.md)) |
| `/verify-email?token=` | Email confirmation result | Then continues to the home page; without a token, a signed-in unverified account sees "Check your email" with a resend button |
| `/forgot-password`, `/reset-password?token=` | Password reset | |
| `/invite/{token}` | Accept invitation | Shows the tutor; create an account or sign in |
| `/unsubscribe?token=` | Unsubscribe confirmation | Works without signing in |
| `/privacy` | Privacy policy | Linked from the consent checkbox; a placeholder until the owner supplies the text |

## Tutor addresses

| Address | Page |
|---|---|
| `/today` | Today (home) |
| `/schedule?view=week\|month&date=` | Schedule |
| `/lessons/{id}` | Lesson |
| `/students?state=` | Students |
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
| `/tutors` | My tutors, enter a tutor code |
| `/notifications` | Notifications |
| `/settings/profile`, `/settings/account`, `/settings/notifications` | Settings sections |

## Rules

- **After sign-in:** a tutor goes to `/today`, a student to `/home`. A `next` parameter is honoured only when it starts with exactly one `/` (not `//` or `/\`), and when resolving it against the OVC origin with the URL parser yields the same origin; anything else is ignored.
- **Wrong role:** a tutor address opened by a student (or the reverse) shows Not found inside the user's own frame, never a hint that the address exists. An address both roles share opens in the signed-in user's own frame.
- **Anonymous visitors** opening a private address are redirected to `/?next=<address>`, with the path and query encoded; after an explicit sign-out or a block the redirect goes to `/` without `next`.
- **While the session is loading** at start-up, every address shows a loading status; when the server cannot be reached, a message with a retry button.
- **Unverified accounts:**
  - a tutor sees a confirmation banner on every page;
  - an unverified tutor trying to invite, schedule a lesson or regenerate the tutor code gets the confirmation prompt instead;
  - an unverified student sees only the confirmation page and account settings, which keep sign-out, the language switch and deletion reachable; every other private address redirects to the confirmation page.
- **Other refusals:**
  - `403 forbidden` shows a toast explaining the action is not allowed;
  - `account_blocked` and a WebSocket close with `4401` that a token refresh cannot fix sign the user out with a message.
- **A deep link** to a resource the user cannot see shows Not found (`404` from the API).
- **Signing out** returns to `/` and clears cached data.
- **Notification links** open the related page: lesson, submission, review, channel or student card.
- **Filters and tabs** that change what is shown are reflected in the address, so reload and Back keep the view.

## Token pages

`/invite/{token}`, `/verify-email?token=`, `/reset-password?token=` and `/unsubscribe?token=` follow one rule:

- the static server sends `Referrer-Policy: no-referrer` for them, as for every page;
- the page reads the token, then removes it from the address bar with `history.replaceState`;
- the page loads nothing from other origins (fonts and images are self-hosted; avatars come from the OVC origin);
- the reverse proxy logs them with the token replaced by a placeholder ([observability](../../specs/observability.md#safe-content)).
