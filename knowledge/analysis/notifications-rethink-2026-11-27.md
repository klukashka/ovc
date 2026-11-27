# Rethinking in-app notifications

Status: agent analysis, written 2026-11-27, pending owner review. It informs the owner's decision and decides nothing. The current behaviour is specified in the [notifications specification](../specs/notifications.md), the [notifications page](../design/web/pages.md#notifications), the [navigation](../design/web/navigation.md) and FR-13 of the [functional requirements](../../docs/requirements/functional.md#fr-13-notifications). Code was read at `main` `41af16b` (read only; no checks were run). External facts were checked on 2026-11-27.

## Summary

- **Why this analysis exists.** After a demo on 2026-11-27 the project owner said the Notifications tab needs rethinking as a whole, and that it may not be needed or should work differently. Today it is a chronological feed behind a "bell" (the Notifications item in the sidebar and the Notifications button in the top bar), with an unread dot per row and an unread counter. "Mark all as read" only removes the dots. Rows that were already handled, such as an accepted join request, stay in the feed for 90 days.
- **What OVC sends today.** 18 in-app kinds and 11 email-only kinds. Of the 18 in-app kinds, only 4 ask the recipient to do something: a new join request, submitted work to check, a new assignment, and work returned for revision. Each of the 4 already has a better home that clears itself once the action is done: Today's join requests and Needs checking for the tutor, Home's To do for the student, and the Homework count in the navigation. 13 kinds only tell the recipient about a change, and 1 is a reminder that is also emailed. No row ever resolves itself, and nothing on the server marks a row read when its action is completed elsewhere (`MarkRead` in `backend/internal/services/notifications/inbox.go` is the only writer of `read_at`).
- **What comparable products do.** Products built around tasks (Google Classroom's To-do and To review, Linear's Inbox with a Priority tab, GitHub's Done) separate "what needs me" from "what happened". Chat products (Telegram, Slack) put unread state on each conversation, not in one global feed. Russian tutor tools (Weekify, Finch) have no in-app feed that the research found: they send reminders through messengers or as reminders inside the product. None of them relies on a "mark all read" button to keep a feed useful.
- **Recommendation (alternative A).** Remove the bell, the panel, the `/notifications` page and the global unread counter. The open actions stay where they already live: the Today and Home panels and the section counts. Changes with no action are shown where they happen, for example a cancelled lesson marked in Home's next seven days. The few changes that have no such place are shown in a short "Recent updates" list on Today and Home, which keeps 14 days and has no read state. Emails stay as they are. Alternative B keeps one place, an inbox with "To do" items that clear themselves and "Updates" the user can clear. Alternative C, the cheapest, keeps the feed but resolves handled items automatically.

## 1. Inventory

### 1.1 In-app kinds (each also signals `notification.new`)

Sources: the event table and the in-app producer tables of the [notifications specification](../specs/notifications.md#events), `backend/internal/domain/notifications.go` and `backend/internal/domain/exports.go` (the 18 values of `NotificationEvents()`), the producers in `backend/internal/services/tutoring` (`join_requests.go`, `realtime.go`, `ending.go`, `archiving.go`), `services/schedule` (`jobs.go`, `edits.go`, `cancellation.go`), `services/homework` (`events.go`, `submitting.go`, `publishing.go`, `changes.go`) and `services/export` (`lane.go`, `jobs.go`, `invalidation.go`), which all write through `repositories.Notifications` (`InsertNotification` in `backend/queries/notifications.sql`). The row texts come from `frontend/src/lib/i18n/en.ts` (`notifications.event`) and `frontend/src/features/notifications/texts.en.ts`.

| Kind | Recipient | Trigger | Row text, label and link | Implied action | Already possible elsewhere | Becomes stale or handled | Email |
|---|---|---|---|---|---|---|---|
| `join_request.created` | tutor | a student's request becomes visible to the tutor | "New request to study with you", the requester's name, link to the Students page's requests | **decide**: accept, decline, block | yes: Today's join requests panel (from `/home`) and the Students page; both clear themselves | yes: accepted, declined or expired, but the row stays (its label and link turn `null` outside scope) | no |
| `invitation.accepted` | tutor | an invitation links its card | "Invitation accepted", the card's name, link to the student card | none (awareness) | the card's state on Students and the student card | no; purely historical | no |
| `relationship.ended` | tutor or student (the other party) | either side ends the relationship | "Working together has ended", the card's or tutor's name, link to the card or My tutors | none | the card's status (tutor); My tutors no longer lists the tutor as current (student) | no | yes, required |
| `join_request.accepted` | student | the tutor accepts | "Your request was accepted", the tutor's name and colour, link to My tutors | none (now able to study) | Home and My tutors list the new tutor | no | no |
| `join_request.declined` | student | the tutor declines (with or without a block) | "Your request was not accepted", no label, link to the sent requests | none, or enter another code | My tutors' sent requests | no | no |
| `lesson.reminder_1h` | tutor and each student of the audience | 1 hour before the start | "The lesson starts in an hour", the lesson title, link to the lesson | **join** soon | Today's lessons and Home's next lesson, with the Join control | yes: after the lesson, the row is useless | yes (`lesson_reminders`); the 24-hour reminder is email only |
| `lesson.moved` | each student of the audience | one lesson's start or duration changes | "The lesson was moved", title, link | none (note the new time) | Home and Schedule show the new time, but nothing marks it as changed | after the lesson | yes (`lesson_changes`) |
| `lesson.cancelled` | each student of the frozen roster | the tutor cancels one lesson | "The lesson was cancelled", title, link | none | Home's next seven days **leaves cancelled lessons out**, so in-app the cancellation is visible only in the feed, the schedule and the lesson page | after the lesson's time | yes (`lesson_changes`) |
| `lesson.series_changed` | each student of an affected lesson | "this and following" edit or cancellation | "Lessons of a series changed", title, link to the occurrence acted on | none | Schedule, without a change marker | after the lessons | yes (`lesson_changes`) |
| `group.archived` | each member, when the archiving cancelled lessons | the tutor archives a group | "The group was archived", the group's name, no link | none | nowhere: the archived group has no page for a student | no | yes (`lesson_changes`) |
| `issue.created` | each student of the issue | the tutor issues work | "New assignment", title, link to the assignment | **do the work** | Home's To do, the Homework page and its "to do" count in the navigation | yes: submitted, but the row stays | yes (`homework`) |
| `issue.withdrawn` | each student who could read the work | the tutor withdraws it | "An assignment was withdrawn", no title, no link | none | the work disappears from To do | no | no |
| `submission.submitted` | the issue's tutor (work with manual questions) | a student submits or resubmits | "Work submitted for checking", the card's name, link to the review screen | **check** | Today's Needs checking, the review queue, the "to check" count in the navigation; all clear themselves | yes: published, but the row stays | yes (`submissions`) |
| `submission.checked` | the student | automatic check on submission, or the tutor publishes a check | "Your work was checked", title, link | read the feedback | Home's Recently checked with the "New" badge (the seen marker, cleared when the results are opened) | yes: the seen marker clears, the row stays | yes (`results`) |
| `submission.returned` | the student | the tutor returns the work | "Your work was returned for revision", title, link | **revise and resubmit** | Home's To do ("Returned for revision") and the to-do count | yes: resubmitted, but the row stays | yes (`results`) |
| `submission.review_updated` | the student | a later revision changes what the student can see | "Your tutor updated the review", title, link | read | Recently checked "New" badge | yes, like `submission.checked` | yes (`results`) |
| `export.ready` | the export's owner, either role | the build completes | "Your data export is ready to download", link to My data | download within 3 days | Settings → My data shows the state | yes: downloaded, expired or deleted | no (the `export.requested` email is sent at request time) |
| `export.failed` | the export's owner | the build fails or is withdrawn | "Your data export could not be prepared", link to My data | request again | My data shows the reason | no | no |

`recording.available`, `recording.held_quota` and `recording.failed` are specified for after the MVP and are not in the code.

### 1.2 Email-only kinds

| Kind | Recipient | Preference group |
|---|---|---|
| `invitation.created` | the card's address | required |
| `email.confirmation`, `password.reset`, `signup.duplicate_attempt` | the account | required |
| `tutor.deleted` | the students of a deleted tutor | required |
| `export.requested` | the requesting account | required |
| `lesson.reminder_24h` | audience and tutor | `lesson_reminders` |
| `messages.digest` | an offline member with unread messages, at most every 30 minutes | `message_digest` |

Email rules relevant to any redesign: optional groups per role (tutors `lesson_reminders`, `submissions`, `message_digest`; students also `lesson_changes`, `homework`, `results`), all on by default; one-click unsubscribe per group (RFC 8058, T-17, T-32); a daily cap of 20 lesson emails and 20 homework emails per student (T-31); no email to an unconfirmed address (T-44); email preferences never suppress in-app rows.

### 1.3 Mechanics today

- **Storage:** one `notifications` row per recipient with identifiers only; labels and links resolved at read time under the caller's current scope; `read_at`; idempotency key; no foreign keys, deleted by the purge, by homework erasure and after 90 days ([data model](../specs/data-model.md#notification)).
- **Routes:** `GET /notifications` (keyset pages, `unread_count` capped at 100) and `POST /notifications/read` with `{ids}` or `{through}` ([api](../specs/api.md#notifications-and-home-pages)).
- **Realtime:** `notification.new {id}` to the recipient on insert; `entity.changed` of kind `notification` after a read; the frontend refetches the list, the panel and the badge, and from E2 also My data's export ([live refetch mapping](../architecture/detailed.md#live-refetch-mapping)).
- **Interface:** the bell (sidebar item, top bar button on narrow screens and phones, the More sheet's row), a panel of the latest 10, the `/notifications` page, a polite "New notification" announcement, the "Show sidebar" button's dot. Code: `frontend/src/features/notifications` (`Bell.tsx`, `NotificationsPanel.tsx`, `NotificationsPage.tsx`, `NotificationRows.tsx`, about 850 lines without tests).
- **Tests that name it:** pilot scenario steps expect a bell row (for example Boris's `join_request.accepted` row and `relationship.ended` row in [pilot scenarios](../design/web/pilot-scenarios.md)); `notifications.spec.ts` for phones; component tests of the panel, page and rows; backend integration tests of every producer.

### 1.4 What the inventory shows

1. **Actions are duplicated.** Every kind that asks for an action (`join_request.created`, `submission.submitted`, `issue.created`, `submission.returned`) is also on Today or Home, where it disappears once it is done. In the feed it stays, and that is the "handled items remain" problem.
2. **Reads are duplicated.** `submission.checked` and `submission.review_updated` already have the "New" badge on Recently checked. That badge is an unread marker on the object itself, and it clears itself.
3. **Some changes have no home besides the feed.** `lesson.cancelled`, `lesson.moved` and `lesson.series_changed` (Home hides cancelled lessons and marks no change), `group.archived`, `issue.withdrawn`, `join_request.declined` and `relationship.ended` are seen in-app only in the feed. All but the last two are also emailed, unless the student turned the emails off or hit the daily cap.
4. **The read state carries little meaning.** For actions, the right signal is whether the action is open, not whether the row was read. For changes, it is whether the change is recent. "Mark all as read" changes only the dots and the counter, which is why it looks like it does nothing.
5. **Global counter versus section counts.** The navigation already shows Homework counts (to check, to do) and Chats unread counts, which are counts of open things. The bell's counter adds a third, cumulative number that mostly repeats them.

## 2. How comparable products handle it

Checked 2026-11-27.

| Product | Pattern | What clears an item | Sources |
|---|---|---|---|
| Google Classroom | Work lists by role: students' **To-do** (work not yet submitted) and teachers' **To review**, where each row can be marked "Reviewed". Notifications go out by email and as device notifications, with switches per type and per class. The Android app also lists the notifications of the past 30 days. | submitting the work; reviewing it | [Manage notifications](https://support.google.com/edu/classroom/answer/6141557?hl=en); [View all your students' work](https://support.google.com/edu/classroom/answer/9157286); student To-do described in a school guide, [Classroom tip: the To Do page](https://sites.google.com/carmischools.org/cwchs-gafe-tips/google-classroom/classroom-tip-3) |
| Slack | The **Activity** view gathers mentions, threads, reactions, invitations and reminders, and offers filters such as Unreads and saved views. A numbered badge counts unread activity. Items can be **cleared** (C, or swipe on mobile) into a Cleared filter. Replying to a message marks it read. Each channel and DM keeps its own unread state. | replying; clearing | [Introducing the new Activity view](https://slack.com/help/articles/46751260742035-Introducing-the-new-Activity-view-in-Slack); [Triage notifications with Activity](https://slack.com/help/articles/19693583638803-Triage-notifications-with-Activity) |
| Linear | **Inbox** of events on issues the user created, was assigned or was mentioned on. A **Priority** tab separates what needs attention from other updates. Items can be read or unread, deleted (one at a time or every read item), and **snoozed** until a chosen time. Reminders also land there. Linear keeps at most 2,000 open notifications. The page describes no automatic clearing when an issue is done. | the user deletes or snoozes | [Linear Inbox docs](https://linear.app/docs/inbox) |
| GitHub | **Inbox** with **Done** (separate from read; `is:done`), **Saved** (kept indefinitely; other items kept 3 months), unsubscribe from a thread (which also removes it from the inbox), custom filters and a "reason" label. Email and web are synchronised. There is no built-in automatic Done when a pull request merges; users rely on saved searches or third-party automation. | the user marks Done or unsubscribes | [About notifications](https://docs.github.com/en/subscriptions-and-notifications/concepts/about-notifications); [Configuring notifications](https://docs.github.com/en/subscriptions-and-notifications/get-started/configuring-notifications); [community discussion on archiving](https://github.com/orgs/community/discussions/50224) |
| Telegram | No global feed. Unread state is per chat, with folders that can filter to unread chats. A muted group still notifies on mentions and replies. | opening the chat | [Telegram FAQ](https://telegram.org/faq); [Folders](https://telegram.org/blog/folders); the per-chat counters and the mention badge come from the apps themselves, not from a first-party page |
| Weekify (Russian tutor CRM) | Separate switches for reminders and events (two pre-lesson reminders, after the lesson, a morning daily summary, a payment, a low balance, a booking request, a birthday), delivered **only through VK, Telegram or MAX**. The research found no in-app feed. | not applicable | [Weekify research](../research/weekify-2026-11-23.md), checked 2026-11-23 |
| Finch (Russian tutor platform) | The home page lists "reminders about lessons and payments" and an automatic payment reminder to the student. It mentions no notification centre or feed. | not applicable | [finch.study](https://www.finch.study/); [tutor tools market](../research/tutor-tools-market-2026-11-23.md) |

### Patterns and takeaways

- **Chronological feed with a global unread badge** (OVC today; also Classroom's 30-day list and Slack's Activity view). It works where volume is high and every item is a conversation to look at. For a tutor with a few dozen students it mostly repeats what the dashboard shows.
- **Action inbox with Done or Archive** (GitHub, Linear). It keeps "needs me" separate from "read". It works for people who triage every day, and it costs explicit triage clicks. Even GitHub does not mark items done automatically.
- **"Needs attention" lists that clear when handled** (Classroom's To-do and To review, OVC's own Today and Home panels). Nobody has to triage, because the list is computed from state.
- **Email, push or messenger only** (Weekify, Finch). No in-app history at all. What reaches the user is reminders and time-critical changes.
- **Unread state on each object** (Telegram, Slack channels, OVC's Chats counts and the "New" feedback badge). The marker sits on the object and clears when the object is opened.

The takeaway for OVC: a small practice needs little in-app history. The dashboard already lists the open actions, and the missing piece is surfacing *changes* (mostly lesson changes) on the objects they concern.

## 3. Alternatives

The three alternatives share these premises:

- Emails and their preference groups are unchanged unless stated otherwise.
- Web push ([ADR-0015](../../docs/decisions/0015-web-push.md)) is unaffected: it pushes lesson reminders and lesson changes, and only its click target is affected (the lesson page).
- No alternative adds a service outside Russia or new personal data.

### A. Needs attention on Today and Home, changes shown where they happen, no bell (recommended)

**Experience.**

- *Tutor, Today:* Today's top panels already list join requests and Needs checking, and they clear themselves. A small **Recent updates** list in the side column shows the changes of the last 14 days that have no other place: a student ended working together, an invitation was accepted, an export is ready or failed. Each update is one line with its time and a link while the subject is in scope. It has no unread dot and no "mark read", and it disappears after 14 days. The navigation keeps its section counts: Homework "to check" and Chats unread. The Students item may gain a count of pending join requests, so a request is noticed from any page (an owner question).
- *Student, Home:* To do (new and returned work) and Recently checked with its "New" badge already exist. The **Next seven days** list keeps cancelled lessons with a "Cancelled" mark, and marks moved lessons "Moved" until they have passed, so lesson changes show on the lesson itself. **Recent updates** shows the request accepted or declined, a tutor ending the relationship, an archived group, a withdrawn assignment and the export outcome, for 14 days.
- *Both:* no bell, no panel, no `/notifications` page, no global counter, no "New notification" announcement. The phone top bar loses its Notifications button, and the More sheet loses its Notifications row.

**Email.** Unchanged. Emails remain the out-of-app channel. Optionally, `join_request.created` gains an email to the tutor in a new optional group, since the bell no longer catches the tutor's eye outside Today (an owner question).

**Rows, counter, frames, read state.**

- Action kinds stop producing rows: `join_request.created`, `submission.submitted`, `issue.created`, `submission.returned`, `submission.checked` and `submission.review_updated`. The derived panels and the seen marker cover them. Their emails are queued as before.
- `lesson.reminder_1h` stops producing a row. The email reminder, push and the Join control on Today and Home remain.
- `lesson.moved`, `lesson.cancelled` and `lesson.series_changed` stop producing rows. Home reads the change from the lesson itself: status `cancelled`, plus a moved marker taken from the lesson's schedule revision or a `moved_at` column (to be specified).
- The remaining informational kinds keep writing rows into the same table with the same identifiers-only rule, purge and erasure. Retention drops from 90 to 14 days, and `read_at` is dropped. `GET /notifications` becomes the source of `/home`'s `recent_updates` (at most 10). Its lifecycle filters stay, so labels and links still resolve under the current scope.
- `notification.new` is kept as the frame that refetches `/home`, and My data keeps its refetch on it. Alternatively it is renamed `update.new`. `entity.changed` of kind `notification` and `POST /notifications/read` go away.

**Documents that change.**

- `notifications.md`: the in-app channel, the producer tables, labels and links, the routes and retention.
- `pages.md`: the Notifications section is removed; Today, Home (next seven days, recent updates) and the More sheet change.
- `navigation.md`: the sidebar item, the top bar button, the phone top bar, the More sheet, notification links and the "Show sidebar" dot.
- `api.md`: `/notifications` routes removed, `/home` gains `recent_updates`, and lesson items gain a moved marker.
- `realtime-and-video.md`: the `notification.new` meaning, and the `entity.changed notification` kind is removed.
- `data-model.md`: the `read_at` column, the event check and retention.
- `detailed.md`: the live refetch mapping.
- `states.md`: the texts.
- `pilot-scenarios.md`: steps that expect a bell row now expect the update on Today or Home.
- In `docs/`: FR-13.1 ("in-app notifications in a list with an unread badge") and FR-15.1 and FR-15.3, which need the owner's wording. No ADR decides the bell. ADR-0015 is untouched apart from its click target.

**Threats.** T-14 (labels stay text only), T-33 (join requests stay held and suppressed under the existing Today rules, and the count must obey them too), T-44 (card display names only), T-21 and T-29 (fewer frame kinds; recent updates resolve scope at read time as today), T-20 (one write route fewer; `/home` grows by one bounded list), T-48 (the export outcome row stays, and My data's refetch must keep working). The threat model's in-app notification controls are rewritten in the same change. The change touches realtime frames and authorization of reads, so it meets the security trigger.

**Migration.** One migration deletes the rows of the kinds that no longer produce rows, and every row older than 14 days. It drops `read_at` and narrows the event check. Nothing has to be preserved, because OVC has no production users yet and the pilot has not started.

**Effort, as change rows.**

1. `docs/notifications-attention` (specs, threat model, FR wording, pilot scenarios)
2. `feat/home-recent-updates` (backend: producers trimmed, the `/home` field, the lesson moved marker, migration, tests)
3. `feat/today-home-updates-ui` (frontend: the panels, next seven days marks, the bell and page removed, e2e updated)
4. optionally `feat/join-request-email` and the Students count

That is about 3 to 4 standard-tier changes, roughly the size of a slice 10 sub-stage. It mostly deletes code.

**Risks.**

- A tutor who works from Students or Messages and rarely opens Today may miss a request until the email or the count shows it.
- A student who turned off `lesson_changes` emails sees a cancellation only on Home or the schedule. The cancelled mark addresses this.
- Removing the global counter removes the one place where "something happened" shows on every page.
- FR-13.1 changes in `docs/`, which needs the owner's wording.
- Pilot e2e scenarios need rework, which costs check-budget time.

### B. One inbox: "To do" items that clear themselves, "Updates" the user can clear

**Experience.** The bell stays, but opens an inbox with two tabs.

- **To do** holds open actions only: join requests, work to check, work to do, returned work and an export to download. Each item disappears by itself when its action is done anywhere in OVC.
- **Updates** holds the informational kinds, with **Clear** per row and **Clear all**. This works like GitHub's Done and Slack's Clear.

The counter counts open To do items plus uncleared updates. "Mark all as read" becomes "Clear all", which visibly empties the tab. The phone keeps its top bar button.

**Email.** Unchanged.

**Rows, counter, frames, read state.**

- A new `resolved_at` column is set by each resolving path in its own transaction: accept or decline, publish, submit, download or expiry, mirroring today's producers. Alternatively it is computed at read time from the subject's state, as labels and links already are.
- `read_at` becomes `cleared_at`.
- `POST /notifications/read` becomes `POST /notifications/clear`, under the same rate and lock rules.
- `notification.new` stays. Resolving signals `entity.changed notification` to the recipient.

**Documents.** The same as A, without the Today and Home changes but with the inbox. `data-model.md` gets the new columns, and FR-13.1 is reworded. The threats are T-20 (one write route), T-21 and T-29, T-33, T-44 and T-14, as in A. The security trigger is met.

**Migration.** Set `cleared_at = read_at`, and compute `resolved_at` for existing action rows once.

**Effort.** About 3 changes. The resolving paths touch every producer's counterpart (accept, decline, publish, submit, export), so B touches more transactions than A.

**Risks.**

- The dashboard and the inbox still say the same thing twice.
- Resolving writes, if stored, add locking work to hot paths such as publishing and submitting. Computed resolution makes `GET /notifications` heavier.
- Triage remains a chore the owner already found pointless.

### C. Keep the feed, resolve handled items automatically

**Experience.** The bell, panel and page stay.

- A row whose action has been completed shows as handled, greyed with "Done" or "Accepted", and does not count as unread.
- Action rows are hidden from the panel once handled, and kept on the page as history.
- "Mark all as read" also hides handled rows.
- Lesson rows older than their lesson are hidden.

**Email.** Unchanged.

**Rows, counter, frames.** No new write path. `GET /notifications` computes each row's handled state at read time from the subject (request state, submission state, seen marker, lesson time, export state), as it already does for labels and links, and the unread count skips handled rows. Frames are unchanged, except that the resolving `entity.changed` frames (`join_request`, `submission`, `lesson`) must also refetch the badge.

**Documents.** `notifications.md` (labels and links, a `handled` field), `api.md`, `pages.md`, `detailed.md` and `states.md`. FR-13.1 is unchanged. Threats: T-21, T-29 and T-14 are unchanged, with scope resolution as today. The trigger is likely met because read-time state crosses relationship scope.

**Migration.** None.

**Effort.** 1 to 2 changes.

**Risks.**

- It does not answer the owner's question of whether the tab is needed at all, and only makes it less wrong.
- The unread count query grows joins over several tables.

## 4. Questions for the owner

1. **Which direction for in-app notifications?**
   - (a) **Recommended:** alternative A. No bell; open actions on Today and Home; changes marked on the lessons themselves; a 14-day "Recent updates" list.
   - (b) Alternative B. An inbox with self-clearing "To do" items and clearable "Updates".
   - (c) Alternative C. Keep the feed and resolve handled items automatically.
   - (d) Remove in-app notifications entirely and rely on email and the dashboards, with no Recent updates.
2. **Does the student keep any page listing past notifications?**
   - (a) **Recommended:** no. Home's Recent updates (14 days) and the lesson marks are enough.
   - (b) Yes, a simple history page reachable from Settings.
   - (c) Yes, the current `/notifications` page as it is.
3. **How long is history kept in-app?**
   - (a) **Recommended:** 14 days, informational kinds only.
   - (b) 90 days, as today.
   - (c) None: only open actions are shown.
4. **What changes for email?**
   - (a) **Recommended:** nothing; every email and preference group stays.
   - (b) Add a `join_request.created` email to the tutor in a new optional group, because the bell no longer flags requests.
   - (c) Replace per-event optional emails with one daily summary per user.
5. **How does a tutor notice a new join request away from Today?**
   - (a) **Recommended:** a count of pending requests on the Students item in the navigation, under the existing hold and suppression rules.
   - (b) Only Today's panel and an email.
   - (c) Only Today's panel.
6. **How do lesson changes show to a student in-app?**
   - (a) **Recommended:** "Cancelled" and "Moved" marks on the lesson in Home's next seven days and the schedule, until the lesson has passed.
   - (b) Entries in Recent updates only.
   - (c) Email and push only.

## Owner decision (2026-11-27)

The owner decided by multiple-choice question on 2026-11-27, through the coordinating agent:
- direction **A**: the bell, the notifications panel, the `/notifications` page and the global counter are removed;
- no history page for students;
- a 14-day "Recent updates" list on Today and Home, with no read state;
- email notifications unchanged;
- a pending join-request count on the Students navigation item;
- "Cancelled" and "Moved" marks on lessons until they have passed.

It is planned as its own change after the gate speed-up and the interface rework stage's backend follow-up, and before the finance stage. The rewording of FR-13.1 in `docs/requirements/functional.md` will be marked pending owner review.
