# ADR-0005: A tutor-centric domain with relationships and groups

- Status: accepted, 2026-10-20; relationship states and the student's access rule superseded in part by [ADR-0014](0014-students-join-through-their-tutor.md) on 2026-10-22
- Reviewed: pending line-by-line owner review; the superseded-in-part note of 2026-10-22 pending owner review
- Deciders: project owner
- Related: FR-1, FR-2, FR-3, [accounts specification](../../knowledge/specs/accounts.md), [data model](../../knowledge/specs/data-model.md), [ADR-0014](0014-students-join-through-their-tutor.md)
- Superseded parts: the relationship states and their causes in the second bullet of the decision, and, in the fourth bullet, the rule that a student reaches a tutor's resources only through an active relationship (a provisional relationship also reaches its one-to-one lessons, private chat and the student's own work). [ADR-0014](0014-students-join-through-their-tutor.md) adds the `provisional` state for a link accepted before the student's email is confirmed and the `reclaim_pending` state for a link awaiting the mailbox owner's choice, the silent removal of such links with a new card for the tutor, and ending by the student as well as by the tutor. The text below is kept as written for the record.

## Context

OVC serves private tutors who teach adults, one to one and in small groups. A school model with courses that students join by code does not fit. A tutor's unit of work is a student, sometimes a group of students. The same adult may study with several independent tutors, who must never see each other's work with that student.

## Decision

- An account has exactly one role: `tutor` or `student`. The operator acts through a command-line tool, not a web role.
- The central entity is the **tutor–student relationship**. It is created with a student card (`invited`), becomes `active` when an invitation or join request is accepted, and later `ended`; a withdrawn or merged card is `discarded`. Every lesson, assignment issue, private chat and tutor note that involves a student belongs to one relationship, or to a group of the same tutor.
- A **group** belongs to one tutor and contains students with active relationships to that tutor.
- **Authorization is relationship-scoped.** A tutor reaches a student's data only through their own relationship. A student reaches a tutor's resources only through an active relationship, or read-only through an ended one where the specification says so. Students never reach each other's work.
- One student account may hold relationships with any number of tutors; the student's views aggregate across them and label everything by tutor.

## Alternatives considered

- **Courses with enrolment** (the earlier model): forces a "course" onto one-to-one tutoring and blurs ownership when one student has several tutors.
- **Separate student accounts per tutor:** simpler isolation, but the student juggles several logins and sees no combined schedule.
- **Accounts with several roles:** useful for tutors who also study, but every screen and authorization rule would branch on the active role; deferred.

## Consequences

- Every query that returns student data filters by the requesting tutor's relationships; the authorization guards and their tests are written once around the relationship.
- Ending a relationship is a first-class operation with defined effects on lessons, groups, chats and work.
- The student interface needs a tutor label and filter everywhere.

## Where it is applied

[Accounts](../../knowledge/specs/accounts.md), [data model](../../knowledge/specs/data-model.md), [API](../../knowledge/specs/api.md), [threat model](../threat-model.md).
