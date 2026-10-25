# Functional requirements

Status: rewritten 2026-10-20 for the tutor-centric product and amended the same day after design review and for the student Home content (FR-15.3); FR-5.4 to FR-5.6, FR-11.2 to FR-11.4 and the recording storage email of FR-13.2 moved after the MVP on 2026-10-22 by the owner's decision; FR-1.1 to FR-1.3, FR-1.7, FR-2.2, FR-2.3, FR-2.7 and FR-13.2 amended and FR-2.8 added on 2026-10-22 by ADR-0014; FR-2.6 amended on 2026-10-25 with an exception for notifications about join requests that never formed a relationship, pending owner review; pending line-by-line owner review.

What OVC must do, numbered so that specifications, tests, pages and plans can cite them. Each section links the specification that defines the exact behavior. Roles are **tutor**, **student** and **operator** (command line only). The [product overview](../product.md) explains the intent; this file is normative.

## FR-1 Accounts

- **FR-1.1** A person signs up as a tutor with name, email and password, consents to the processing of their personal data, and must confirm the email address before creating student cards, sending invitations or regenerating the tutor code. Every sign-up, including a student's, records that consent (amended 2026-10-22 by [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), pending owner review).
- **FR-1.2** A person becomes a student only through a tutor: by signing up on a tutor's invitation page, or by signing up with a tutor code; there is no student sign-up without one. No invitation confirms the email: every student confirms it from the confirmation email. Signing up through an invitation links the student to the tutor at once, provisionally until the email is confirmed; a request sent with a code reaches the tutor only once the email is confirmed. When the owner of the mailbox confirms the email or resets the password from a browser not signed in to the account, they choose whether to keep or drop each provisional link and each waiting request (amended 2026-10-22 by [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), pending owner review).
- **FR-1.3** An account has exactly one role. An invitation accepted or a tutor code entered while signed in as a tutor is refused with an explanation; every other route of the other role answers as if it did not exist (amended 2026-10-22 by [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), pending owner review).
- **FR-1.4** A user signs in with email and password, stays signed in across reloads, signs out of the current device, and can sign out of all devices.
- **FR-1.5** A user resets a forgotten password by email and changes their password while signed in.
- **FR-1.6** A user sets their name, avatar, language (Russian or English), timezone (taken from the browser at sign-up) and theme (light, dark or system). A tutor also sets a short bio and the subjects they teach.
- **FR-1.7** A user deletes their account. Their uploads are deleted and their messages show as from a deleted user. When a student deletes their account, their relationships end without an email, their provisional links are removed and their join requests expire. When a tutor deletes their account, their relationships end, their student cards, invitations and provisional links are discarded, their join requests expire, their groups are archived, each student with a confirmed email and an active relationship receives one email, and their students keep read-only access to their own results for 30 days (amended 2026-10-22 by [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), pending owner review).
- **FR-1.8** Repeated failed sign-in attempts are throttled.

Specified in [accounts](../../knowledge/specs/accounts.md#registration-and-email-verification), [sessions and passwords](../../knowledge/specs/accounts.md#sessions-and-passwords), [account deletion](../../knowledge/specs/accounts.md#account-deletion).

## FR-2 Tutors and students

- **FR-2.1** A tutor creates a student card with a name and an optional email, and gets a single-use invitation link that expires after 14 days. The tutor sends it by email from OVC or copies it.
- **FR-2.2** A person who opens an invitation link while already holding a student account signs in on the invitation page and accepts with one action. When the card has an email, only the account with that email can accept it, and repeated attempts with another email revoke the link. An account whose email is not confirmed cannot accept a second tutor's invitation until it confirms (amended 2026-10-22 by [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), pending owner review).
- **FR-2.3** A tutor has a tutor code they can share and regenerate. A student who enters it, at sign-up or later, creates a join request; the tutor accepts, declines, or declines and blocks further requests, and unanswered requests expire 14 days after they were sent. A request from a student whose email is not confirmed is held, invisible to the tutor, until the email is confirmed (amended 2026-10-22 by [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), pending owner review).
- **FR-2.4** One student account can have relationships with several tutors. Each tutor sees only their own relationship: their lessons, assignments, chat and notes with that student.
- **FR-2.5** The student card shows the student's contact details, groups, lesson history, assignments and scores, and private notes that only the tutor sees.
- **FR-2.6** A student sees the profile (name, avatar, bio, subjects) of each of their tutors, and every lesson, assignment, chat and notification is labelled with its tutor's name and colour, except notifications about join requests that never formed a relationship, such as a declined request, which carry no tutor label or colour (amended 2026-10-25 with the slice 4 notifications, pending owner review).
- **FR-2.7** A tutor ends a relationship. Future lessons with that student are cancelled, the student leaves the tutor's groups, the chat becomes read-only, recordings (after the MVP) stay viewable until they expire, and submitted work and results stay visible to both; the student receives an email. A new invitation or an accepted join request restores the relationship. While the student's email is not confirmed, ending removes the student instead: the link is discarded silently and the tutor gets a new student card with a fresh link (amended 2026-10-22 by [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), pending owner review).
- **FR-2.8** A student ends a relationship with a tutor from their list of tutors, after confirming in a dialog. The effects are those of FR-2.7, and the tutor receives an email. A new invitation accepted by the same student, or a join request accepted by the tutor, restores it. A provisional link ended this way is removed silently, and the tutor gets a new student card (added 2026-10-22 by [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), pending owner review).

Specified in [tutor–student relationships](../../knowledge/specs/accounts.md#tutorstudent-relationships), [invitations](../../knowledge/specs/accounts.md#invitations), [tutor code and join requests](../../knowledge/specs/accounts.md#tutor-code-and-join-requests), [ending a relationship](../../knowledge/specs/accounts.md#ending-a-relationship).

## FR-3 Groups

- **FR-3.1** A tutor creates a group with a name and a colour from their active students, and adds or removes members.
- **FR-3.2** A group has its own chat, lessons and issued assignments.
- **FR-3.3** A student removed from a group keeps their issued work and results but loses the group chat and future group lessons.
- **FR-3.4** A tutor archives a group; an archived group has no future lessons and its chat is read-only.

Specified in [groups](../../knowledge/specs/accounts.md#groups).

## FR-4 Schedule and lessons

- **FR-4.1** A tutor schedules a one-off lesson for one student or one group with a title, start, duration and optional description and materials.
- **FR-4.2** A tutor schedules a weekly series on one or more weekdays with an end date or a number of lessons. Lessons keep their local wall-clock time across daylight-saving changes in the timezone the tutor had when creating the series.
- **FR-4.3** A tutor edits or cancels a single lesson, or this and all following lessons of a series. The time and audience of past lessons are never changed; materials and assignments can still be linked to them.
- **FR-4.4** The tutor is warned when a new or moved lesson overlaps another of their lessons.
- **FR-4.5** Every user sees a calendar of their lessons (week and month on wide screens, an agenda list on narrow screens) in their own timezone. When the other party's timezone differs, the lesson page also shows the lesson in that timezone: the student's for the tutor of a one-to-one lesson, the tutor's for students.
- **FR-4.6** A lesson is scheduled, live, ended or cancelled. It becomes live when the tutor first joins and ends when the tutor ends it or 30 minutes after its scheduled end. If the video room closes earlier, for example because the tutor's connection dropped, the tutor can rejoin and the lesson continues.
- **FR-4.7** The lesson page shows time, participants, description, attached materials, linked assignments, the join button and, after the MVP, recordings.

Specified in [schedule](../../knowledge/specs/schedule.md).

## FR-5 Video lessons and recordings

- **FR-5.1** The tutor joins the lesson's video room from 10 minutes before its start until it ends, as moderator. Students join as viewers once the tutor has joined, while they still study with the tutor and, for a group lesson, are still in the group. Anyone else is refused.
- **FR-5.2** The room provides video, audio, screen sharing, a shared whiteboard, presentation upload and in-room chat.
- **FR-5.3** The room opens embedded in the lesson page once embedding is confirmed to work on the supported browsers, and in a new tab until then; opening it in a new tab is always possible.
- **FR-5.4** (after the MVP, by the owner's decision of 2026-10-22) The tutor starts and stops recording during the lesson, and participants see that recording is on.
- **FR-5.5** (after the MVP, by the owner's decision of 2026-10-22) After processing, the recording appears on the lesson page for the tutor and the students in the lesson's roster: its audience when the lesson started, plus group members who joined later. No recording is reachable without signing in.
- **FR-5.6** (after the MVP, by the owner's decision of 2026-10-22) A tutor sets how long recordings are kept (default 90 days, chosen from 7, 30, 90, 180 or 365 days). The period applies to recordings made after the change. The tutor can delete a recording earlier.

Specified in [video](../../knowledge/specs/realtime-and-video.md#video), [recordings](../../knowledge/specs/realtime-and-video.md#recordings).

## FR-6 Library

- **FR-6.1** A tutor keeps a library of materials (uploaded files) and assignment templates, organised in folders, with search by title.
- **FR-6.2** A tutor attaches library materials to lessons and to assignment content blocks.
- **FR-6.3** A tutor duplicates, edits and deletes templates; editing or deleting a template never changes work already issued.

Specified in [library](../../knowledge/specs/homework.md#library).

## FR-7 Assignment builder

- **FR-7.1** An assignment is an ordered list of blocks with a title. Content blocks are instruction text and attached materials. Question blocks are single choice, multiple choice, short text answer, short numeric answer, long text answer and file answer.
- **FR-7.2** Single choice, multiple choice, short text and short numeric answers are checked automatically: short text against a list of accepted answers after normalisation, short numeric against a value and tolerance.
- **FR-7.3** Long text and file answers are checked by the tutor.
- **FR-7.4** Every question has points and an optional explanation shown with the correct answer.
- **FR-7.5** The builder previews the assignment as a student will see it.

Specified in [assignment blocks](../../knowledge/specs/homework.md#assignment-blocks), [short-answer normalisation](../../knowledge/specs/homework.md#short-answer-normalisation).

## FR-8 Issuing assignments

- **FR-8.1** A tutor issues a template to one student or one group, optionally linked to a lesson, with an optional deadline and an answer visibility policy: never, after submission, or after checking.
- **FR-8.2** Issuing takes a snapshot; later template edits do not change it.
- **FR-8.3** Issuing to a group creates separate work for each member at that moment; members added later receive it only if the tutor issues it again.
- **FR-8.4** A tutor withdraws issued work that no student has submitted; the students are notified and it disappears from their lists.

Specified in [issuing](../../knowledge/specs/homework.md#issuing), [answer visibility](../../knowledge/specs/homework.md#answer-visibility).

## FR-9 Completing assignments

- **FR-9.1** A student opens issued work, answers in any order, and their answers save automatically as a draft.
- **FR-9.2** A student submits with one action after confirming. Work submitted after the deadline is marked late; the deadline does not block submission.
- **FR-9.3** Automatically checked questions are scored on submission. Points and scores are for the tutor only: the student never sees points, totals, overrides, scores or percentages, before or after checking.
- **FR-9.4** The student never receives correct answers or explanations before the visibility policy allows it.

Specified in [submission lifecycle](../../knowledge/specs/homework.md#submission-lifecycle).

## FR-10 Checking work

- **FR-10.1** Submitted work with manual questions appears in the tutor's needs-checking list, oldest first.
- **FR-10.2** On the review screen the tutor scores each manual question, comments on any question, overrides an automatic score with a reason, attaches corrected files to a file answer, and writes overall feedback.
- **FR-10.3** The tutor marks work checked, or returns it for revision. Returning reopens only the manual questions; automatic answers stay as submitted. Returning is unavailable when the policy is "after submission".
- **FR-10.4** The student resubmits returned work; it goes back to the needs-checking list.

Specified in [scoring and review](../../knowledge/specs/homework.md#scoring-and-review).

## FR-11 Results and analytics

- **FR-11.1** A student sees the status of their work, the tutor's comments, overall feedback and corrected files, and, once answers are revealed, the correct answers with explanations next to their own answers. They never see points or scores.
- **FR-11.2** (after the MVP, by the owner's decision of 2026-10-22) For each issue the tutor sees how many students submitted, were checked and are late, the average score, the share of correct answers per question and the distribution of answers for choice questions.
- **FR-11.3** (after the MVP, by the owner's decision of 2026-10-22) For each student the tutor sees their scores across assignments over time.
- **FR-11.4** (after the MVP, by the owner's decision of 2026-10-22) Analytics use final scores after any tutor overrides.

Specified in [analytics](../../knowledge/specs/homework.md#analytics).

## FR-12 Chat

- **FR-12.1** Each tutor–student relationship has a private chat; each group has a group chat for the tutor and the members. Students cannot chat privately with each other.
- **FR-12.2** Messages appear live and stay as history. Members can send files and pictures; pictures show a preview.
- **FR-12.3** An author edits or deletes their own message; the tutor deletes any message in their group chat.
- **FR-12.4** Users see unread counts, who is online and who is typing.
- **FR-12.5** Chats of ended relationships and archived groups are read-only.

Specified in [chat](../../knowledge/specs/realtime-and-video.md#chat).

## FR-13 Notifications

- **FR-13.1** Users see in-app notifications in a list with an unread badge.
- **FR-13.2** Users receive email for:
  - lesson reminders 24 hours and 1 hour before;
  - a lesson moved or cancelled;
  - a new assignment;
  - work submitted (to the tutor);
  - work checked or returned (to the student);
  - unread messages, as a digest;
  - invitations, email confirmation and password reset, and a notice when someone tries to sign up with their email;
  - a relationship ended (to the other party: the student when the tutor ends it, the tutor when the student ends it) or a tutor's account deleted (to students) (amended 2026-10-22 by [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), pending owner review);
  - recording storage full (to the tutor) (after the MVP, by the owner's decision of 2026-10-22).
- **FR-13.3** Users turn off each optional email group in settings or with one click from the email. Invitation, confirmation, password-reset, duplicate sign-up notice, relationship-ended, tutor-deleted and (after the MVP) storage-full emails cannot be turned off.
- **FR-13.4** Emails use the recipient's language and timezone and reflect the lesson as it is when the email is sent.

Specified in [notifications](../../knowledge/specs/notifications.md).

## FR-14 Files

- **FR-14.1** Files are accepted within per-context size limits and type allowlists: chat, answers, corrected files, library and avatars.
- **FR-14.2** Each tutor has a storage quota covering their library, the files in their chats, and the work of their students. The tutor sees their usage, and uploads beyond the quota are refused.
- **FR-14.3** A file is downloadable only by users who can see the item it belongs to.

Specified in [files](../../knowledge/specs/files.md).

## FR-15 Home pages and onboarding

- **FR-15.1** The tutor's Today page shows today's lessons with join buttons, work needing checking, unread chats, upcoming deadlines and pending join requests.
- **FR-15.2** A new tutor sees a first-run checklist (profile and timezone, invite a student, schedule a lesson, create an assignment) until it is completed or dismissed.
- **FR-15.3** The student's Home page shows the next lesson with a join button, assignments to do ordered by deadline, recently checked work with the opening of the overall feedback and whether a corrected file came back, the lessons of the next seven days, and the student's tutors with a way to message them or enter a new tutor code, from all tutors, with a filter by tutor (amended 2026-10-20 by [ADR-0010](../decisions/0010-quiet-visual-language.md), pending owner review).

Specified in [pages](../../knowledge/design/web/pages.md).

## FR-16 Operator administration

- **FR-16.1** An operator lists users, blocks, unblocks and restores accounts, and changes a tutor's quotas with a command-line tool. A blocked user cannot sign in, and their sessions and live connections end.

Specified in [administration command](../../knowledge/specs/accounts.md#administration-command).

## FR-17 Languages

- **FR-17.1** The whole interface and every email are available in Russian and English. A new user's language defaults to the browser's language when it is Russian or English, and to English otherwise.
- **FR-17.2** Dates, times, numbers and plurals follow the user's language. Content written by users is never translated.

Specified in [localisation](../../knowledge/design/web/states.md#localisation).
