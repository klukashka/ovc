# Pilot scenarios

End-to-end journeys that define what "the product works" means. Each scenario becomes a Playwright suite that drives the production frontend build against a local test deployment of the real API, worker, database and storage.

- **Harness:** the [end-to-end test harness](../../../docs/development.md#end-to-end-test-harness) provides the fake meeting provider and its test room, the database mail sink, and the shared fake clock.
- **Steps:** every step names the page from [pages](pages.md) and the API operation from [the API specification](../../specs/api.md) it relies on. `harness:` marks a test control route.
- **Mail and worker effects:** mail assertions read the sink through `harness: POST /mail`, and are made after a `harness: POST /tick`. Steps that expect an email or a worker effect include that tick implicitly.
- **Pass condition:**
  - every expected result holds;
  - no unexpected console error, failed request or HTTP error occurs (a startup refresh without a cookie answers `204` and is not an error);
  - automated accessibility checks report no violations on every page visited, once the checker is adopted.
- **Isolation:** each suite seeds its own accounts with unique emails through the real API, using the same operations the steps use.
- **Strings:** expected interface texts are asserted by their keys in [states](states.md) or the i18n resources, rendered in the actor's language, never by hard-coded English.

## Actors

| Actor | Role | Language | Timezone | Theme | Viewport |
|---|---|---|---|---|---|
| Maria | tutor (languages) | Russian | Europe/Moscow | light | 1280 × 800 |
| Pavel | tutor (mathematics) | English | Asia/Yekaterinburg | dark | 1280 × 800 |
| Anna | student | Russian | Europe/Berlin | system (light) | 390 × 844 |
| Boris | student | English | Europe/Moscow | dark | 1280 × 800 |

Browser locale and timezone match each actor's settings, and the browser timezone becomes the account timezone at sign-up. Dates in English use the `en-GB` format. Every suite starts with the clock on a fixed date in November, so no daylight-saving change interferes.

Recordings (FR-5.4 to FR-5.6) and analytics (FR-11.2 to FR-11.4) are outside the MVP by the owner's decision of 2026-10-22: scenario 1 steps 12, 14 and 15, scenario 2 step 15, the Recordings suite, the FR-11.3 part of the Progress and onboarding suite, and the FR-5.4 to FR-5.6 and FR-11.2 to FR-11.4 parts of the coverage rows wait until those features are built; the MVP suites end the lesson without recording (step 13 still ends it) and skip the analytics step.

## Scenario 1: first lesson (with a recording after the MVP)

| # | Actor | Page | Operation | Expected |
|---|---|---|---|---|
| 1 | Maria | Sign up | `POST /auth/register` | `202`; the page shows "Check your email"; the confirmation email is in the sink |
| 2 | Maria | Confirm email | Opens the emailed link in a fresh browser, sets a new password: `POST /auth/verify-email` with `{token, new_password}` | Signed in; Today shows the first-run checklist |
| 3 | Maria | Settings | `PATCH /users/me` | Bio saved; checklist step done |
| 4 | Maria | Students | `POST /students` with Anna's email, then `POST /students/{id}/invitation/send` | Card for Anna; invitation email in the sink |
| 5 | Anna | Accept invitation, Home, Confirm email | `GET /invitations/{token}`, `POST /invitations/{token}/accept` from the emailed link, then the confirmation link in the same browser: `POST /auth/verify-email` with `{token}` | Anna creates her account on the invitation page and is signed in; Home shows Maria as her tutor with the confirmation banner, and Maria's card shows "Email not confirmed" without a reload; Maria's bell shows 1, and the row names the card by its display name with "Invitation accepted" (`GET /notifications`); after confirming, the banner is gone, the outcome is `linked`, the relationship is active, and no second row arrives |
| 6 | Maria | Schedule | `POST /lessons` (series, Tuesdays and Thursdays at 18:00, 8 lessons) | Eight lessons in the calendar; Anna's Schedule shows them at 16:00; her lesson page shows `time.other_zone` with 18:00 and Moscow |
| 7 | Clock, Worker | — | `harness: POST /clock` to 60 minutes before the first lesson, `harness: POST /tick` | The 1-hour reminder emails for Maria and Anna are in the sink, in Russian, with Anna's in Berlin time |
| 8 | Clock | — | `harness: POST /clock` to 5 minutes before the first lesson | — |
| 9 | Anna | Home | — | "The room opens when your tutor joins"; no Join button |
| 10 | Maria | Lesson | `POST /lessons/{id}/join` | The fake room opens in a new tab as moderator, showing her name; the lesson becomes live |
| 11 | Anna | Home | `POST /lessons/{id}/join` | The Join button appears without reload; the fake room opens as viewer, showing her name |
| 12 | Maria | — | `harness: POST /fake-provider/lessons/{lesson}/recording` start, then stop | The fake room shows its recording state as on, then off, to both |
| 13 | Maria | Lesson | "End lesson", `POST /lessons/{id}/end` | The lesson shows "Ended" for both; in the MVP the lesson page shows no Recordings block for Maria or Anna |
| 14 | Worker | — | `harness: POST /fake-provider/lessons/{lesson}/ready`, `harness: POST /tick` | The recording appears on the lesson page for Maria and Anna, available until 90 days later |
| 15 | Anna | Lesson | `GET /recordings/{id}/playback` | The player loads the returned signed URL; the same API request without a session gets 401 |

## Scenario 2: group homework from the library

Seed: Maria (verified) and Anna (linked through an emailed invitation), as in scenario 1, steps 1–5.

**The template** "Past simple, week 1" is worth 10 points. It has these blocks:

- an instruction;
- a material "Irregular verbs.pdf" from the folder "Grammar";
- a single-choice question (2 points, options "goed" and "went", correct "went");
- a short-text question (2 points, accepted "went");
- a file question (6 points).

Each question has an explanation.

| # | Actor | Page | Operation | Expected |
|---|---|---|---|---|
| 1 | Maria | Settings | `GET /tutor/code` | The tutor code is shown |
| 2 | Boris | Sign up, Confirm email | `POST /auth/register` (student, with the code) on `/signup/student`, signs in, then opens the confirmation link in the same browser: `POST /auth/verify-email` with `{token}` | Before confirming, Maria's Today shows no request and her bell no row; after confirming, the outcome is `request_sent` and Boris sees "Request sent. Waiting for the tutor."; the request appears on Maria's open Today without a reload, with a `join_request.created` row in her notifications |
| 3 | Maria | Today | `POST /join-requests/{id}/accept` | The request disappears; Boris appears in Students; Boris's bell shows a `join_request.accepted` row labelled with Maria's name |
| 4 | Maria | Groups | "New group", `POST /groups` | Group "B1 evening" with Anna and Boris; its chat exists |
| 5 | Maria | Library, Builder | `POST /library/folders`, `POST /uploads?context=library`, `POST /library/materials`, `POST /library/templates`, `PUT /library/templates/{id}` | The template above is in the library; the preview shows its five blocks in order without answer keys |
| 6 | Maria | Issue dialog | `POST /issues` (group, deadline in 2 days, answers after checking) | Both students get the assignment and an email |
| 7 | Anna | Assignment (phone) | `POST /uploads?context=answer&scope=submission:…`, `PUT /submissions/{id}/answers` ("went", "went", one photo), `POST /submissions/{id}/submit` | The draft saves while she types; after submitting she sees "Submitted", with no points of any kind and no correct answers |
| 8 | Clock | — | `harness: POST /clock` past the deadline | — |
| 9 | Boris | Assignment | Answers "goed", "Went ", one PDF; `POST /submissions/{id}/submit` | The submission is marked late; Maria's view shows `homework.auto_points` with 2, Boris's view shows no points |
| 10 | Maria | Today, Review | `GET /review-queue`, `POST /uploads?context=corrected&scope=submission:…`, `PUT /submissions/{id}/review` (file question 6, overall feedback "**Great work.** Check the word order in sentence 2."), `POST /submissions/{id}/check` | Anna's work is checked with a corrected file attached; the email to Anna is in the sink |
| 11 | Maria | Review | `PUT /submissions/{id}/review` with a comment on the file question, `POST /submissions/{id}/return` | Boris's work is returned; only the file question is editable for him; the comment is visible |
| 12 | Boris | Assignment | New upload, `POST /submissions/{id}/submit` | Resubmitted; back in Maria's queue; the resubmission email to Maria is in the sink |
| 13 | Maria | Review | `PUT /submissions/{id}/review` (file question 4), `POST /submissions/{id}/check` | Boris's work is checked at 6 of 10 |
| 14 | Anna | Results, Home | `GET /submissions/{id}`, `GET /attachments/{id}`, `GET /home` | Comments, the corrected file downloads, and correct answers with explanations; Home lists the work under recently checked with the feedback excerpt "Great work. Check the word order in sentence 2." (Markdown removed) and the corrected-file note; no points, total or percentage anywhere on either page or in the submission and home responses |
| 15 | Maria | Issue analytics | `GET /issues/{id}/analytics` | 2 of 2 submitted, 2 checked, 1 late; average 80%; single choice 50% correct, short text 100% correct; options "goed" 1 and "went" 1; top short answer "went" (2) |
| 16 | Anna | Results | Opens Boris's submission address | Not found |

## Scenario 3: two tutors, chat and email

Seed:

- Maria and Anna are linked, with a weekly series of 4 one-to-one lessons on Wednesdays at 19:00 Moscow time.
- Maria and Boris are linked.
- Anna and Boris are both in Maria's group "B1 evening".
- Maria has issued an assignment to Anna.

| # | Actor | Page | Operation | Expected |
|---|---|---|---|---|
| 1 | Pavel | Sign up, confirm, Students | `POST /auth/register`, `POST /auth/verify-email`, `POST /students` with Anna's email, `POST /students/{id}/invitation/send` | The invitation email to Anna is in the sink |
| 2 | Anna | Accept invitation (signed in) | `POST /invitations/{token}/accept` | Pavel is added; Anna's My tutors shows both tutors with their colours |
| 3 | Pavel | Schedule, Builder, Issue dialog | `POST /lessons` (Monday at 10:00 his time), `POST /library/templates`, `POST /issues` | — |
| 4 | Anna | Schedule, Home | `GET /calendar`, `GET /home` | Schedule shows lessons from both tutors, each with its tutor label; Home lists work and the next seven days' lessons from both, and both tutors in My tutors; filtering by Pavel shows only his items in every section except My tutors |
| 5 | Pavel | Student card | `GET /students/{id}` | Only Pavel's lesson and work with Anna; none of Maria's |
| 6 | Anna | Messages | `POST /uploads?context=chat&scope=channel:…`, `POST /channels/{id}/messages` with a picture | Maria sees the message live with a preview; the unread badge on Maria's Messages; Anna shows as online in Maria's chat header |
| 7 | Boris | Messages | Types in the group chat composer and sends (`message.send`) | Anna and Maria see the typing indicator while he types, then the message live |
| 8 | Anna | — | Closes all her tabs; the test waits until `harness: POST /presence` reports her offline | — |
| 9 | Boris | Messages | Another group message | — |
| 10 | Clock, Worker | — | `harness: POST /clock` +11 minutes, `harness: POST /tick` | One digest email to Anna, naming the group chat with 1 unread message |
| 11 | Clock, Worker | — | `harness: POST /clock` +31 minutes, `harness: POST /tick` | No second digest for the same message |
| 12 | Maria | Lesson | `PATCH /lessons/{id}` with scope `this` (the next Wednesday lesson moves to 20:00) | A "lesson moved" email to Anna in Russian, with 18:00 her time |
| 13 | Clock, Worker | — | `harness: POST /clock` to 60 minutes before the moved lesson, `harness: POST /tick` | The reminder uses the new time; none for the old time |
| 14 | Anna | Unsubscribe | (`pending implementation` until the digest of slice 6 carries the link) Opens the unsubscribe link from the digest, presses the button: `POST /unsubscribe?token=`, then closes the tab; the test waits until `harness: POST /presence` reports her offline | The preference is off |
| 15 | Boris, Clock, Worker | — | Another group message, `harness: POST /clock` +11 minutes, `harness: POST /tick` | No digest to Anna, under the same conditions in which step 10 produced one |

## Scenario 4: ending a relationship and refusals

Seed:

- Maria creates a card for Boris; he signs up on the invitation page of the copied link, which links him provisionally, and confirms his email in the same browser, which makes the relationship active.
- Maria adds him to group "B1 evening" with Anna.
- Maria schedules a weekly one-to-one series for Boris and a group lesson next week.
- Maria issues an assignment to the group, which Boris has not started.

| # | Actor | Page | Operation | Expected |
|---|---|---|---|---|
| 1 | Maria | Student card | `POST /students/{id}/end` for Boris | Boris's future one-to-one lessons are cancelled and his series ends; one aggregated email to Boris; he leaves "B1 evening"; Boris gets a `relationship.ended` row in his notifications |
| 2 | Clock, Worker | — | `harness: POST /clock` +2 days, `harness: POST /tick` | Horizon extension creates no new lessons for the ended series |
| 3 | Boris | Messages | — | The private chat with Maria is read-only, with its composer disabled; the group chat is no longer listed |
| 4 | Boris | Lesson | Opens the group lesson's address | Not found |
| 5 | Boris | Homework | — | The Closed tab shows the group assignment, which cannot be submitted |
| 6 | Boris | Accept invitation | Opens the copied link from the seed again | "This invitation is no longer valid" |
| 7 | Maria | Student card (Ended) | "Invite again", `POST /students/{id}/invitation`; Boris accepts the new link | The relationship is active again with its history; the chat is writable; Boris is not back in the group; Maria gets an `invitation.accepted` row naming the card |
| 8 | Anonymous | Any private page | Any private operation | Redirect to `/?next=…`; the API returns 401 |

## Supplementary suites

Smaller Playwright or integration suites cover what the pilots do not walk through. Each is seeded independently.

| Suite | Kind | Covers |
|---|---|---|
| Account edge cases | Playwright + integration | FR-1.3 role mismatch, FR-1.4 sign out everywhere, FR-1.5 reset and change, FR-1.7 deletion and purge, FR-1.8 throttling, refresh reuse, squatting and cleanup of unverified accounts |
| Requests and invitations | integration + Playwright | FR-2.3 decline, block and expiry, regeneration, email-bound and student-bound refusal, provisional links, reclaim and races, FR-2.5 notes, FR-2.8 a student stops studying |
| Groups | Playwright | FR-3.3 member removal with work still submittable, FR-3.4 archive and unarchive |
| Schedule edits | integration + Playwright | FR-4.2 daylight-saving gap and overlap, FR-4.3 "this and following" splits (including the first occurrence and concurrent extension), FR-4.4 overlap confirmation beyond the horizon, automatic end and "did not take place", joins after a relationship ends |
| Video embed | Playwright | FR-5.3 with `harness: POST /settings/embed` on: embedded room, the 15-second hint, switching to a new tab, focus mode, disposal on leave |
| Recordings | integration | FR-5.6 retention and deletion, quota hold and retry, reconciliation without callbacks, callback replays |
| Homework policies | integration + Playwright | FR-6.3 template edits after issue, FR-8.4 withdrawal and its races, FR-10.3 return blocked under "after submission", overrides, updated reviews, draft corrected files refused, the answer-key contract test, number-parsing and normalisation units |
| Chat edits and presence | Playwright | FR-12.3 edit and delete, FR-12.4 presence, FR-12.5 read-only channels |
| Files | integration | FR-14.1 size and type refusals, FR-14.2 quota refusal |
| Notifications | integration | FR-13.1 list and unread count, every email event, obsolete-at-dispatch rules, idempotency of in-app rows and emails, RFC 8058 unsubscribe; active from slice 4 for FR-13.1, the idempotency of in-app rows and RFC 8058 unsubscribe, with the email events of later slices pending |
| Progress and onboarding | Playwright | FR-11.3 student progress, FR-15.2 checklist dismissal, FR-17 language switch across every page |
| Realtime resilience | Playwright | WebSocket disconnect and reconnect with refetch, `4401` after sign-out everywhere, `4429` handling; active from slice 4, with the server-side closes driven by `harness: POST /realtime/close` and socket closes awaited through `harness: POST /presence` |
| Operator command | integration | FR-16.1 list, block, unblock, restore, set quota |

## Coverage

| Requirement | Pilot scenarios | Supplementary suites |
|---|---|---|
| FR-1.1, FR-1.2, FR-1.6 | 1, 2, 3, 4 | Account edge cases |
| FR-1.3 to FR-1.5, FR-1.7, FR-1.8 | — | Account edge cases |
| FR-2.1, FR-2.2, FR-2.4, FR-2.6, FR-2.7 | 1, 3, 4 | Requests and invitations |
| FR-2.3 | 2 | Requests and invitations |
| FR-2.5 | 3 | Requests and invitations |
| FR-2.8 | — | Requests and invitations |
| FR-3.1, FR-3.2 | 2, 3 | Groups |
| FR-3.3, FR-3.4 | — | Groups |
| FR-4.1, FR-4.5 to FR-4.7 | 1, 3 | Schedule edits |
| FR-4.2 to FR-4.4 | 1 (FR-4.2), 3 (FR-4.3) | Schedule edits |
| FR-5.1, FR-5.2, FR-5.4, FR-5.5 | 1, 4 | Recordings |
| FR-5.3 | 1 (new tab) | Video embed |
| FR-5.6 | — | Recordings |
| FR-6.1, FR-6.2, FR-7 | 2 | Homework policies |
| FR-6.3 | — | Homework policies |
| FR-8.1 to FR-8.3, FR-9, FR-10.1, FR-10.2, FR-10.4, FR-11.1, FR-11.2, FR-11.4 | 2 | Homework policies |
| FR-8.4, FR-10.3 | 2 (return) | Homework policies |
| FR-11.3 | — | Progress and onboarding |
| FR-12.1, FR-12.2, FR-12.5 | 3, 4 | Chat edits and presence |
| FR-12.3, FR-12.4 | 3 (FR-12.4) | Chat edits and presence |
| FR-13.1 to FR-13.4 | 1, 2, 3, 4 | Notifications |
| FR-14.1, FR-14.2 | 2, 3 (accepted uploads) | Files |
| FR-14.3 | 1, 2 | — |
| FR-15.1, FR-15.3 | 1, 2, 3 | — |
| FR-15.2 | 1 | Progress and onboarding |
| FR-16.1 | — | Operator command |
| FR-17 | 1, 3 (Russian email) | Progress and onboarding |
