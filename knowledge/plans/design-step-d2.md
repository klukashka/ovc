# Design slice step D2: Today and Home show a person's day

Status: approved by the owner on 2026-11-08. A security-profile planning review returned "approve with changes", and every finding is applied here.

## 1. Problem and outcome

**Problem.**

- Today shows only the join requests and the tutor code.
- Home shows only the tutors.
- A tutor does not see their day, and a student does not see their next lesson, although the routes that hold this data exist.

**Outcome.** On a computer both pages follow the layout standard: the page width, a main column, and the 300 px side column (stacked below 1280 px).

| Page | Main column | Side column |
|---|---|---|
| Today (tutor) | The first-run checklist while unfinished, today's lessons | The join requests with the tutor code, unread messages |
| Home (student) | A tutor filter, the next lesson | The next seven days, my tutors |

The homework blocks arrive with slices 8 and 9. The Join button and its states stay with D6. D2 adds no backend route and changes no handler.

## 2. Decisions the owner made for this plan (2026-11-08)

- **The checklist has no dismissal in D2.** It shows while any visible step is undone and disappears by itself when all of them are done. "Hide" arrives with `PATCH /tutor/settings` in slice 10, as the roadmap keeps it.
- **Lessons refetch on the frames that reach a person who lost them.** An `entity.changed` frame of kind `relationship` or `channel` also invalidates the calendar queries. So a removed group member, or a student whose relationship ended, loses the lesson from an open Today, Home or Schedule without a reload. The mapping in `detailed.md` changes with tests.
- **Unread messages on Today show up to five channels with unread messages.** Each shows its name and count, with no message preview. The link "All messages, {n} unread" uses `unread_channels`. The data is the first page of the Messages channel list, through the same query.
- **The Home tutor filter lives in the address as `?tutor=`,** like the Schedule, pushing a history entry. An out-of-scope value (`/calendar` answers `404`) or a malformed one (it answers `422 validation_failed` at `query.tutor`, because the route declares the parameter as a UUID) shows "This tutor is not available." with a link to all tutors and no data; the notice never repeats the address value. The real answers are checked with one integration call before the page is written; no handler changes.

## 3. Verification tier and security trigger

| Change | Tier | Trigger | Reason |
|---|---|---|---|
| This plan | Task plan | **Met** | A security-profile reviewer reviews it before the owner approves it. |
| D2a: Today, and the refetch mapping | Standard | **Met** | Today reads data from several relationships. It renders untrusted lesson titles and audience and channel labels (T-14). It changes which frames refetch which data (T-21, T-29). An unconfirmed tutor reaches it (T-44). |
| D2b: Home | Standard | **Met** | Home shows lessons and tutors from several relationships on one page (T-5, T-6). It holds a client-supplied `tutor` identifier in the address (T-18). Unconfirmed and `reclaim_pending` students reach it (T-44). |

Each change gets one standard reviewer and one security-profile reviewer, dispatched together, its own change checks and its own merge gate. If a part of D2b depends on D2a, D2b starts from the moved `main`.

## 4. What each change must hold

| Threat | Must hold |
|---|---|
| T-5, T-6 | Today and Home read only through existing relationship-scoped routes: `/calendar`, `/tutors`, `/join-requests`, `/join-requests/sent`, `/channels`, `/students`, `/users/me`, `/tutor/code`. The frontend makes no scope decision of its own. One tutor's lessons never appear on another tutor's Today. A student's Home shows only what `/calendar` returns for that student. Query keys hold the `tutor` filter, and an account change clears the whole query cache (`accountCacheOwnership` in `app/queryClient.ts`), as today. The Home `tutor` identifier goes only to the API, never into a scope decision (T-5, as for the group filter). |
| T-14 | Lesson titles, group names, audience labels, tutor names, subjects and channel labels render as React text nodes with `dir="auto"`. The reused components get `dir="auto"` where it is missing: `LessonBlock`'s audience label, `TutorLabel`'s name, and `TutorRow`'s name and subjects. Today shows a channel's label and unread count only, never `last_message.preview`, `sender_label` or an attachment name. Each row links only to an address built from its identifier (`/lessons/{id}`, `/messages/{id}`). Colour dots reuse the palette-key classes and never put a server value into a class or `style`. End-to-end tests use markup in a lesson title, a group name, a student name and a channel label on Today, and in a tutor name on Home, including the filter's button names, and assert that it shows as text. |
| T-21, T-29 | A `relationship` or `channel` frame invalidates `calendarKey`, as well as what it invalidates today. A lesson frame keeps invalidating it. Today's and Home's new queries enter the mapping in `knowledge/architecture/detailed.md`. Refetching never replaces data with frame content. Tests are end to end (the mapping table is data, which AGENTS.md keeps out of unit tests): a removed group member, who gets only the last `channel` frame, and a student whose relationship ended, who gets only a `relationship` frame for a group lesson of that tutor's group, each see the lesson on an open page first and then lose it without a reload. A side effect is recorded as accepted: every card edit sends the tutor a `relationship` frame, which now also refetches an open calendar; D5's notes autosave debounce bounds it. |
| T-44 | An unconfirmed tutor's Today calls no route closed to them. `/students` and `/join-requests` are not requested, which is already true for join requests. The checklist shows "Invite a student" and "Schedule a lesson" as not done, with the confirmation prompt instead of their links. An unconfirmed student's Home shows only what `/calendar` and `/tutors` answer: the provisional link's one-to-one lessons, and no lesson of a `reclaim_pending` link (the server hides them). |
| T-18 | Nothing new reaches logs, storage or third parties; the `tutor` value in the address is an opaque relationship identifier, not personal data. |
| T-20 | Today adds at most three requests (`/calendar`, one `/students?limit=1`, `/channels`) and Home one (`/calendar`), and no request per row. No new polling: data refreshes through the existing frames and on reconnect. An end-to-end test counts the API requests after each page settles. |

## 5. What changes

### D2a: Today and the refetch mapping

**Layout.**

- `<Page titleSize="display">` takes the page width. The title stays "Today", with the local date in one line beside it (the "greeting with date").
- `PageColumns` holds the checklist and today's lessons in the main column, and the join requests and unread messages in the side column.

**First-run checklist** (`features/home/FirstRunChecklist.tsx`). It shows three visible steps, each with a check mark or an empty circle and a link to where it is done:

| Step | Done when | Link |
|---|---|---|
| Profile | `bio` or `subjects` in `/users/me` is not empty | Settings › Profile |
| Invite a student | `GET /students?limit=1`, which lists every card that is not discarded, holds a card | Students |
| Schedule a lesson | the tutor's calendar from the start of today to 62 days ahead holds a lesson | Schedule |

- The step "Create an assignment" stays hidden until the library exists.
- The panel is not shown when every visible step is done.
- For an unconfirmed tutor the second and third steps show as not done with the confirmation prompt, and `/students` is not requested.

**Today's lessons** (`features/home/TodayLessons.tsx`).

- The day comes from the same calendar query as the checklist step: one range, from local midnight today (`useLocalToday`) to 62 days ahead.
- It shows the lessons that start today in the viewer's timezone, ordered by start. Each row has the time range, the title, the audience label and the status word, and links to the lesson page.
- Empty: "No lessons today" with a link "Open the schedule".
- The Join button and "Room opens at" stay with D6.

**Join requests.** The existing panel is unchanged and moves to the side column with the tutor code. Decisions keep their no-dialog rule (`pages.md`).

**Unread messages** (`features/home/UnreadMessages.tsx`).

- It shows up to five channels with `unread_count > 0` from the first page of the Messages channel list. The data comes through the query hook extracted from `ChannelList`, so the key and the cached value stay the same and the existing coalescer and eviction keep it fresh.
- Each row shows the channel's label and colour dot and the count ("99+" above 99). It links to `/messages/{id}`.
- The footer link "All messages, {n} unread" opens Messages.
- With no unread channel the panel says "No unread messages".

**Refetch mapping.** In `app/liveRefetch.ts`, `relationship` and `channel` add `calendarKey`. In `knowledge/architecture/detailed.md` the live refetch rows name Today's and Home's calendar, Today's unread messages (through the channel list's first page) and the checklist's student query (through the `relationship` row's student lists).

**Requests on Today.**

- Already made: notifications, the tutor code and join requests.
- Added: `/calendar` (one), `/students?limit=1` (one, only while the tutor is confirmed) and `/channels` (one, shared with Messages).
- For an unconfirmed tutor: `/calendar` and `/channels` only.

**Specifications** (pending owner review):

- `pages.md` Today: the parts as built; "In slice 3 this panel is the only part of Today" removed.
- `knowledge/design/web/pilot-scenarios.md` scenario 1 step 2: the checklist on Today.
- `detailed.md`: the mapping above.
- `docs/threat-model.md`: T-14 (Today among the listed pages), T-21 (the calendar refetch on `relationship` and `channel` frames), T-6 (a former group member loses the group's lessons from an open page on the last `channel` frame) and T-44 (an unconfirmed tutor's Today calls no closed route).
- `knowledge/specs/schedule.md`: where a student who lost a lesson "loses it on their next fetch", note that the client refetches on `relationship` and `channel` frames.

### D2b: Home

**Layout.** `<Page titleSize="display">` takes the page width. The title stays "Home". A one-line summary of the day sits under it: "{n} lessons today", or "No lessons today". `PageColumns` holds the filter and the next lesson in the main column, and the next seven days and my tutors in the side column.

**Tutor filter** (`features/home/TutorFilter.tsx`).

- It is a segmented control: a labelled `group` of `aria-pressed` buttons, "All tutors / {name}…".
- It is shown when two or more current (not ended) tutors are listed.
- The choice sets `?tutor={relationship_id}` with a new history entry, so Back restores the previous choice.
- The filter applies to the next lesson and the next seven days, never to My tutors.
- A value that `/calendar` answers with `404` or with `422` at `query.tutor`, or that no current tutor holds, shows the notice "This tutor is not available." with the link "All tutors", and no lesson. It offers no Retry and never repeats the value.

**Next lesson** (`features/home/NextLesson.tsx`; as built, it and the next seven days share `features/home/HomeLessons.tsx`).

- One calendar query covers the next lesson and the next seven days: from local midnight today to 62 days ahead, with `tutor` when filtered.
- The next lesson is the first lesson that is not cancelled and has not ended. It shows the tutor label, the title, the date and time, and a link to the lesson page.
- No lesson within the range: "No lesson scheduled".
- The countdown, Join and the room states stay with D6.

**Next seven days** (`features/home/NextSevenDays.tsx`).

- It lists the lessons that start within seven days of now and are not cancelled.
- Each row shows the weekday, the time, the title and the tutor label, and links to the lesson page.
- A link "Open the schedule" keeps the filter; it is hidden for an unconfirmed student without a provisional link, for whom the Schedule redirects to email confirmation.

**My tutors.** The existing list moves to the side column, unchanged except for `dir="auto"`. The "No tutor yet" and "Request sent" states keep their behavior and focus rules.

**Unconfirmed student.**

- The banner and the reachable sections stay as specified.
- The calendar query runs: `/calendar` is open to unverified accounts and shows the provisional link's lessons.
- The filter is not shown, because such a student has at most one link.

**Requests on Home.**

- Already made: `/tutors` and `/join-requests/sent`.
- Added: `/calendar` (one per filter value).

**Specifications** (pending owner review):

- `pages.md` Home: the parts as built, and the slice 3 sourcing note updated.
- `pilot-scenarios.md` scenario 3 step 4: the Home part proven through `/calendar` and `/tutors` instead of `GET /home`, with the To do part still pending slices 8 and 9.
- `docs/threat-model.md`: T-14 (Home), T-44 (an unconfirmed or `reclaim_pending` student's Home) and T-5 (the Home `tutor` identifier goes only to the API, and its refusal shows "This tutor is not available." with nothing of the tutor).

## 6. Acceptance criteria

All end-to-end tests run in Chromium, Firefox and WebKit, except where a test needs a coarse pointer.

**D2a:**

1. `frontend/e2e/today.spec.ts` (new) › "a tutor with a lesson today sees it on Today": the row shows the time, the title, the audience label and the status, and links to the lesson page. A lesson tomorrow is not listed.
2. Checklist:
   - › "a new tutor sees the three undone checklist steps";
   - › "a tutor with a bio, a student and a lesson sees no checklist";
   - `pilot-1 › steps 2–3 Maria sees the first-run checklist and saving her bio marks its step done`, a pilot scenario step test that acts and asserts in turns, as AGENTS.md allows for that title form.
3. › "an unconfirmed tutor's Today asks for confirmation in the checklist and requests no closed route": no request to `/api/v1/students` or `/api/v1/join-requests` is made, checked by a passive `page.on('request')` listener and by `unverifiedTutorTodayQueryPaths`.
4. › "unread messages show the channel name and count without a preview": after a student sends a message, Today lists the channel with "1". The message text appears nowhere on Today. "All messages, 1 unread" links to Messages.
4a. › "Today makes only its listed requests": a passive `page.on('request')` listener counts the API requests after Today settles: notifications, the tutor code, join requests, `/calendar`, one `/students` and `/channels`, each at most twice (the first read and the one refetch every query gets when the realtime socket becomes ready, plus the frame's `/channels/unread`), and none per row (as built: the ready refetch is the existing behaviour of every page).
5. › "a lesson title, group name, student name and channel label with markup show as text on Today": no `img` or `b` element comes from them, and the document title is unchanged.
6. › "one tutor's Today never shows another tutor's lessons, for a student they share": Maria and Pavel both teach Anna; each first sees their own lesson listed, then the other's is absent.
7. › "an account change clears Today's lessons": after signing out and in as another tutor, that tutor's own empty state "No lessons today" shows first, then the earlier tutor's lesson is absent.
8. (The mapping is proved end to end by 9, not by a unit test of the table.)
9. `frontend/e2e/realtime.spec.ts` › "a removed group member's open Schedule drops the group lesson without reloading": the student first sees the group lesson, the tutor removes them from the group, and the lesson disappears with no new document load (the page's navigation count stays the same). It uses the Schedule because it exists before D2b; D2b adds the Home cases.
10. Existing specs pass unchanged in meaning:
    - `join-requests.spec.ts`: its `main h3` locator moves to the code section's own heading;
    - `live-updates.spec.ts`, `realtime.spec.ts`, `smoke.spec.ts`, `session.spec.ts`, `signup.spec.ts`, `desktop-layout.spec.ts`;
    - `todayQueryPaths` and `unverifiedTutorTodayQueryPaths` gain the new paths.

    No assertion is removed or loosened.
11. `tools/docs/check-docs.sh` passes, and the documents above are updated and marked pending owner review.
12. The frontend change checks pass:
    - `typecheck`, `lint`, `format:check`, `test`, `build`, `check:classes` and `check:bundle`, with Today within its 400 KB first-load budget;
    - the whole `pnpm e2e`.

**D2b:**

1. `frontend/e2e/home.spec.ts` (new) › "a student with two tutors sees each lesson on Home with its tutor label": the next lesson and the next seven days show both tutors' lessons, each with its tutor's name and colour dot.
2. › "filtering by one tutor shows only that tutor's lessons and keeps My tutors whole". `?tutor=` is in the address, and Back restores All tutors.
3. › "a tutor value no current tutor holds shows that the tutor is not available and never reaches the calendar": a relationship of another student and a malformed value each show the notice with "All tutors", no lesson and not the value, and a passive request listener proves no calendar request carries either value (as built: the page refuses a value no current tutor holds before asking the calendar, so the server's `404`/`422` path is reached only in a race and is recorded as a known limitation).
4. › "a cancelled lesson is left out of the next seven days and the next lesson".
5. › "an unconfirmed student's Home shows the provisional link's lesson".
6. › "a reclaim_pending link's lessons are not shown on Home": the test goes to `/home` explicitly (sign-in lands on `/reclaim` while a choice is pending), waits for the other link's lesson or the settled empty state, then asserts the reclaim-pending link's lesson is absent; the reclaim-pending relationship's own identifier as `?tutor=` shows the notice.
7. › "a removed group member's open Home drops the group lesson without reloading", and › "an ended relationship's group lesson leaves an open Home": the ending tutor's group has a scheduled group lesson; the student sees it, the tutor ends the relationship, and it disappears with no new document load. (A one-to-one lesson would only turn cancelled and reach the student through a `lesson` frame, which proves nothing new.)
7a. › "an account change inside the app clears Home": student A's Home, unfiltered and then filtered, then sign out and in as student B in the same document; a passive observer proves A's lessons never appear while B's Home settles, B's own state shows, and the document never reloads. A previous account's `tutor` value is covered by criterion 3, which uses another student's relationship.
7c. Focus: › "focus moves to the lesson that takes the place of a cancelled focused next lesson", the removed-member test asserting the page heading is focused, and › "focus moves to the page heading when the filter disappears with focus on it".
7b. › "Home makes only its listed requests", counted after it settles as on Today.
8. › "a lesson title, group name and tutor name with markup show as text on Home", including the filter's button names.
9. Existing specs that visit Home pass unchanged in meaning: `join-requests.spec.ts`, `invitations.spec.ts`, `notifications.spec.ts`, `session.spec.ts` and `reclaim.spec.ts`.
10. Documents and change checks as in D2a, with Home within its 250 KB first-load budget.

## 7. Order of work

1. D2a in a worktree outside `/private/tmp`, on `feat/design-step-d2a-today`:
   - the `dir="auto"` fixes in the reused components;
   - the extracted channel list query hook;
   - the refetch mapping;
   - the checklist, today's lessons and unread messages;
   - the layout;
   - the tests and documents.

   Then the change checks, the two reviewers, the fixes, the readiness report, the gate and the merge.
2. D2b on `feat/design-step-d2b-home` from the moved `main`:
   - the filter, the next lesson and the next seven days;
   - the layout;
   - the tests and documents.

   Then the same checks, reviews, gate and merge.

## 8. Observability

No backend change. The new requests are ordinary API reads, recorded as every request is. The frontend adds no log or diagnostic event.

## 9. Risks

- **The calendar range is 62 days.** A lesson more than 62 days ahead does not count for the checklist step and is not the next lesson. Such a lesson shows "No lesson scheduled" on Home, a rare case accepted here.
- **The shared channel list query.** Today reuses the Messages first page, so a change to that hook affects both pages. The hook is extracted, not copied.
- **End-to-end readiness lists.** `todayQueryPaths` gain paths, and every spec that waits for Today's readiness now waits for the calendar and the channels too.
- **Pages from other steps.** The Schedule, Messages and the lesson page stay as they are, and D6 adds Join to Today and Home later.
