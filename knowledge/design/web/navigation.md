# Navigation and addresses

Every page of the web application, its address, who reaches it and where navigation leads. Page contents are in [pages](pages.md); state words and colours are in [states](states.md).

## Frames

- **Public frame:** logo, language switch (Русский / English), and sign-in or sign-up links. Used for every page reachable without a session.
- **Tutor frame:**
  - desktop and tablet: a left sidebar with Today, Schedule, Students, Groups, Homework and Messages, plus Settings at the bottom; a top bar with the notification bell and the avatar menu (Settings, Sign out);
  - Homework opens `/homework/library` and has three tabs: Library, Issued and Needs checking;
  - below 768 px: the sidebar collapses into a menu button.
- **Student frame:**
  - phone: a bottom bar with Home, Schedule, Homework, Messages and More (My tutors, Settings, Sign out); the notification bell sits in the top bar;
  - 768 px and wider: the same items in a left sidebar.
- Russian navigation labels use short forms at phone width ("Главная", "Расписание", "Задания", "Чаты", "Ещё").
- Unread badges appear on Messages and on the bell; a needs-checking badge on Homework (tutor); a to-do badge on Homework (student).

## Public addresses

| Address | Page | Notes |
|---|---|---|
| `/` | Sign in | Signed-in users are redirected to their home page |
| `/signup` | Choose: "I teach" or "I study" | |
| `/signup/tutor`, `/signup/student` | Sign-up form | Consent checkbox, language preselected from the browser |
| `/verify-email?token=` | Email confirmation result | Then continues to the home page |
| `/forgot-password`, `/reset-password?token=` | Password reset | |
| `/invite/{token}` | Accept invitation | Shows the tutor; create an account or sign in |
| `/unsubscribe?token=` | Unsubscribe confirmation | Works without signing in |

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
- **Wrong role:** a tutor address opened by a student (or the reverse) shows Not found, never a hint that the address exists.
- **Anonymous visitors** opening a private address are redirected to `/?next=<address>`.
- **Unverified accounts:**
  - a tutor sees a confirmation banner on every page;
  - an unverified tutor trying to invite, schedule a lesson or regenerate the tutor code gets the confirmation prompt instead;
  - an unverified student sees only the confirmation page.
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
