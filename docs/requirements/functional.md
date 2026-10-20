# Functional requirements

Status: draft written 2026-10-20; pending line-by-line owner review.

What OVC must do, numbered so that specifications, tests and plans can cite them. Each requirement links the specification section that defines its behavior. Roles: **teacher**, **student**, **admin**.

## FR-1 Accounts and profiles

- **FR-1.1** A person registers with email and password as a teacher or a student.
- **FR-1.2** A registered user logs in, stays signed in across page reloads, and logs out from the current device.
- **FR-1.3** A user edits their name, avatar, timezone and, for teachers, a bio and the subjects they teach.
- **FR-1.4** Any signed-in user can view a teacher's public profile (name, avatar, bio, subjects).
- **FR-1.5** An admin can deactivate an account; a deactivated user cannot sign in.

Specified in [auth routes](../../knowledge/specs/api.md#auth), [user and session entities](../../knowledge/specs/data-model.md#user).

## FR-2 Courses and enrolment

- **FR-2.1** A teacher creates a course with a title, description and subject, and can edit or archive it.
- **FR-2.2** A teacher shares a join code; a student enrols in the course by entering it.
- **FR-2.3** A teacher adds a student by email and removes a student from a course.
- **FR-2.4** Every user sees a list of the courses they teach or are enrolled in, and nothing else.
- **FR-2.5** Course members see the course's teacher and students.

Specified in [course routes](../../knowledge/specs/api.md#courses).

## FR-3 Calendar and lessons

- **FR-3.1** A teacher schedules a lesson in a course with a title, description, start and end.
- **FR-3.2** A teacher reschedules, edits or cancels a lesson; members see the change.
- **FR-3.3** Every user sees a calendar of upcoming lessons across all their courses, in their own timezone, by month, week and day.
- **FR-3.4** A lesson shows whether it is scheduled, live, ended or cancelled.

Specified in [lesson routes](../../knowledge/specs/api.md#calendar--lessons), [lesson entity](../../knowledge/specs/data-model.md#lesson).

## FR-4 Video lessons

- **FR-4.1** A course member joins a lesson's video room from the calendar or the course page, from shortly before the start until the lesson ends.
- **FR-4.2** The room supports multiple participants, camera and microphone control, and screen sharing.
- **FR-4.3** The teacher joins as moderator; students join as attendees.
- **FR-4.4** The teacher ends the lesson, which closes the room for everyone.
- **FR-4.5** A person who is not a member of the course cannot join its rooms.

Specified in [video](../../knowledge/specs/realtime-and-video.md#video).

## FR-5 Chat

- **FR-5.1** Every course has a chat channel for its teacher and students.
- **FR-5.2** Any two users who share a course can open a direct conversation.
- **FR-5.3** Messages appear live for every connected member and remain available as history.
- **FR-5.4** An author edits or deletes their own message; a teacher deletes any message in their course channel.
- **FR-5.5** Members see who is online and who is typing.

Specified in [chat](../../knowledge/specs/realtime-and-video.md#chat), [chat routes](../../knowledge/specs/api.md#chat).

## FR-6 Homework

- **FR-6.1** A teacher creates an assignment with instructions, attached files, an optional due date and an optional maximum score, as a draft or published.
- **FR-6.2** Students see only published assignments in their courses.
- **FR-6.3** A student writes an answer and attaches files, saves it, and submits it.
- **FR-6.4** A teacher sees every submission for an assignment, with its status and lateness.
- **FR-6.5** A teacher grades a submission with points and written feedback, or returns it for revision; a returned submission can be resubmitted.
- **FR-6.6** A student sees their grade and feedback.
- **FR-6.7** Only the submitting student and the course teacher can open a submission and its files.

Specified in [assignment routes](../../knowledge/specs/api.md#homework--assignments), [submission routes](../../knowledge/specs/api.md#homework--submissions--grading), [file routes](../../knowledge/specs/api.md#files--attachments).

## FR-7 Notifications

- **FR-7.1** A connected user is notified in the app when an assignment is published in their course, their submission is graded or returned, or one of their lessons is about to start.

Specified in [notifications](../../knowledge/specs/realtime-and-video.md#notifications).
