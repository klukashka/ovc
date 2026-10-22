# Product

Status: draft written 2026-10-20, visual language revised the same day after the owner chose a direction on the prototype; recordings and analytics marked as after the MVP on 2026-10-22 by the owner's decision; pending line-by-line owner review.

This page explains what OVC is, who it serves and how it feels to use. It is not normative: the binding statements are the [functional requirements](requirements/functional.md), and where this page and a requirement differ, the requirement wins.

## Who it is for

OVC is a workspace for **private tutors** who teach **adult students** online. Each tutor runs their own practice: their own students, their own groups, their own schedule, their own library of exercises. There is no school or organisation above the tutor.

A **student** is a person, not a seat in a class. One student account can study with several tutors (for example a language tutor and a mathematics tutor). Each tutor sees only their own relationship with that student; the student sees everything from all their tutors in one place, labelled by tutor.

The platform operator has no web interface in the first release; blocking and listing users is done with a command-line tool.

## What a tutor does

1. Signs up, confirms their email, sets their timezone and profile, and follows a short first-run checklist.
2. Adds students: either sends a personal invitation link, or shares a tutor code that students enter after signing up, and accepts their requests.
3. Organises students into groups when they teach several people together.
4. Schedules lessons for a student or a group, one-off or weekly, and joins them from the calendar or the Today page.
5. Teaches in a video room with a whiteboard, presentation upload and screen sharing, and, after the MVP, records the lesson when useful. Participants find the recording on the lesson page afterwards.
6. Builds exercises once in a library: tests that check themselves, written questions, and file tasks. Issues them to a student or a group, optionally with a deadline.
7. Reviews submissions on a two-pane screen: automatic scores are already there; the tutor scores written answers, returns corrected files, leaves feedback, or sends work back for revision.
8. After the MVP, sees how a group did on each question, and how each student progresses over time.
9. Talks to each student in a private chat, and to each group in a group chat, with files and pictures.

## What a student does

1. Opens an invitation link (or signs up and enters a tutor code) and lands on their home page.
2. Sees the next lesson with a Join button, homework to do with deadlines, recently checked work with the opening of the feedback and any corrected file, the lessons of the next seven days, and a list of their tutors to message or to add a new one by code, from all tutors.
3. Joins lessons from a phone or a computer, and, after the MVP, rewatches recordings.
4. Completes homework on any device; answers save automatically; submits with one button.
5. Reads feedback, downloads corrected files, and, when the tutor allows it, sees the correct answers and explanations. Scores are the tutor's working tool and are never shown to students.
6. Chats with each tutor and with their groups, and receives email reminders before lessons.

## Principles

- **The tutor's time comes first.** Every repeated task (issuing the same test, scheduling a weekly lesson, finding what needs checking) is one action.
- **The student never wonders what to do next.** The home page always answers: when is my next lesson, what is due, what came back.
- **Everything optional stays optional.** A tutor who only wants video lessons and chat never has to touch homework, and an assignment can be a single file task or a fifty-question test.
- **Nothing leaks across relationships.** A tutor never sees another tutor's work with the same student; students in a group never see each other's submissions.
- **Personal data stays minimal and in Russia.** OVC collects only what a feature needs, and every service that stores user data is hosted in Russia ([ADR-0008](decisions/0008-hosting-and-data-residency.md)).

## Main screens

| Tutor | Student |
|---|---|
| Today: lessons of the day, work to check, unread messages, deadlines, first-run checklist | Home: next lesson, homework to do, recently checked work, next seven days, my tutors |
| Schedule: week and month calendar | Schedule: all lessons from all tutors |
| Students: list, student card, join requests | My tutors: list, enter a tutor code |
| Groups: list and group page | — |
| Homework: library, builder, issued work, review, and analytics after the MVP | Homework: to do, submitted, checked; take an assignment; results |
| Messages | Messages |
| Lesson page with video room, and recordings after the MVP | Lesson page with video room, and recordings after the MVP |
| Settings | Settings |

Element-by-element descriptions are in [the page design](../knowledge/design/web/pages.md).

## Visual language

- **Quiet and warm, like a good book on a desk.** Warm paper background, near-black ink text, book-style serif titles over a calm sans-serif interface, hairline rules instead of boxes, and one muted olive accent: filled only on the main action of each screen, and otherwise used sparingly for links, focus, selection and the few states that ask for action. Light and dark themes ([ADR-0010](decisions/0010-quiet-visual-language.md)).
- **Calm about status.** One vocabulary of states (scheduled, live, needs checking, late, checked…) shown as a word with a small coloured dot, never colour alone and never loud pills.
- **A sidebar like a document tool.** Both tutors and students navigate with a collapsible left sidebar: their name on top, sections with simple icons and counts.
- **Computer first, phone later.** Every screen, including the student's, is designed for a computer first; phone layouts are adapted in a later step, and until then every page still works on a phone in one column.
- **Two languages.** Russian and English throughout, chosen per user; dates, times and plurals follow the chosen language.

## Not in the first release

Payments and lesson balances ([proposal 0001](proposals/0001-payment-ledger.md)), timed tests and multiple attempts, students booking slots themselves, accounts that are both tutor and student, an administration web interface, push notifications, calendar export.
