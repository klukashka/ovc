# Proposal 0007: A tutor CRM in OVC, data export, a familiar messenger and simpler homework

- Status: proposed; the owner approved the plan of its first stage and this proposal's scope on 2026-11-23 (wording pending owner review)
- Reviewed: scope decisions taken by the project owner by multiple-choice questions on 2026-11-23, listed under Decision; wording pending owner review
- Author: agent, from the owner's comments of 2026-11-23
- Date: 2026-11-23
- Trigger: accepted parts are scheduled as the stages named below; the deferred bot is taken up only by a new decision of the owner; the deferred import is taken up when Weekify publishes an official export or a public API, or when tutors moving from another tool ask for a spreadsheet import
- Related: [proposal 0001](0001-payment-ledger.md) (absorbed), [proposal 0003](0003-students-aged-14-and-over.md), [proposal 0006](0006-legal-documents-and-student-data.md), [ADR-0005](../decisions/0005-tutor-centric-domain.md), [ADR-0008](../decisions/0008-hosting-and-data-residency.md), [ADR-0009](../decisions/0009-owner-approved-exceptions-and-visibility.md), [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), [ADR-0015](../decisions/0015-web-push.md), FR-2, FR-4, FR-7, FR-8, FR-12, FR-13, [Weekify research](../../knowledge/research/weekify-2026-11-23.md), [gap map](../../knowledge/analysis/weekify-gap-map-2026-11-23.md), [interface rework stage plan](../../knowledge/plans/ui-rework.md)

## Motivation

On 2026-11-23 the owner named Weekify («Викифай»), a CRM for private tutors, as the product OVC must match: OVC's interface must contain all of Weekify's logic, a new user must be able to bring their data over, every user must be able to download their data as files so as not to lose it, Messages must follow the familiar messenger pattern, and homework must become intuitive, with a file or a photo as a task or as an answer, tests checked automatically and free answers waiting for the tutor.

OVC today has no money records, no students who never sign in, no export, no online booking and no task tracker; Weekify has no video room, chat or homework cycle. The [gap map](../../knowledge/analysis/weekify-gap-map-2026-11-23.md) compares them feature by feature. Without this proposal a tutor moving from Weekify loses their balances, payments and the students who do not use the system, and nobody can take their data out of OVC.

"Financial reports" is a product-scope expansion: OVC records no money at all today, and [proposal 0001](0001-payment-ledger.md), which designs a lesson and payment ledger, has been deferred since 2026-10-20. This proposal takes it up and widens it.

## Proposal

### 1. Export of a user's data (interface rework stage)

- Every user, tutor or student, can request an export of their data from Settings, confirming it with their current password; an email tells them an export was requested. Each user holds at most one export, kept for a few days and deleted earlier when data in it becomes unreadable in OVC, for example after another person's purge. The worker builds one ZIP and the user downloads it through a short-lived authorized download; the user receives an in-app notification when it is ready.
- The ZIP holds: tables as XLSX workbooks (a tutor: students, groups, lessons and series, issued work, submissions with scores, reviews, library index, chat history; a student: their tutors, lessons, homework with feedback, chat history); the user's lessons as one iCalendar (`.ics`) file to import into another calendar; the files the user can download in OVC (library materials, attachments, answers, corrected files); and a machine-readable JSON copy of the same data with a format version, so a later import into OVC or another tool can read it.
- An export contains exactly what the user can see in OVC under the same rules: a student's export never holds answer keys before the visibility policy allows them, and never points or scores ([ADR-0009](../decisions/0009-owner-approved-exceptions-and-visibility.md)); a tutor's export holds only their own relationships.
- When the finance stage lands, its ledger, payments, expenses and monthly report join the export as further sheets.
- The owner chose XLSX on 2026-11-23 on the understanding that it needs one new Go dependency. XLSX is a ZIP of XML parts, so a small writer on the standard library may do the job, which the dependency rule of `AGENTS.md` prefers; the export contract change decides between the two and records the choice in the stack table.

### 2. A familiar messenger (interface rework stage)

The owner chose, on 2026-11-23, an interface like Telegram's inside OVC, not a bridge to Telegram: message content stays in OVC's storage in Russia ([ADR-0008](../decisions/0008-hosting-and-data-residency.md)). The rework keeps every rule of FR-12 and the chat specification about who may read and write, and adds:

- message bubbles, own messages on one side, grouped by sender and minute, date chips, pictures in an album grid, a "jump to the latest" button with the count of new messages, and a visible message menu on hover and focus on computers (the touch menu comes with the phone design, which the owner postponed on 2026-11-23);
- **reply to a message**: a message may quote one earlier message of the same channel, shown above its body and jumping to it; a quoted message that is deleted shows as deleted;
- **read status in direct chats only**: a sent message shows one tick, and two ticks once the other member's read marker has passed it. This amends the chat specification and the API specification, which today say a read-marker move reaches no other member, and FR-12.4 gains the ticks. Group chats keep no read status, so students never learn about each other's reading;
- **pinned message**: the tutor pins one message per channel, shown under the header;
- **search within a conversation** over the bodies the caller can read;
- **reactions** from a fixed small set, visible to the channel's members, with the same student-to-student limits as presence (a student sees reaction counts, not other students' names). The owner may strike reactions in review without affecting the rest;
- voice messages are not part of this proposal: browsers record in formats (WebM, MP4 audio) outside the chat allowlist, and widening the allowlist is a separate files decision.

### 3. Simpler homework (interface rework stage)

The owner chose, on 2026-11-23, a simple task path beside the builder:

- "New task" asks for the instructions, lets the tutor attach files and photos at once (choosing files or pictures; taking a photo with the phone's camera comes with the postponed phone design), and offers one of two answer kinds: a **test** (choice, short text and number questions, checked automatically) or a **free answer** (text and files or photos, checked by the tutor). It issues to a student or a group in the same flow, with an optional deadline.
- Underneath it uses the existing model: the attached files become library materials in a folder for task attachments, the task becomes a template of instruction, material and question blocks, and issuing takes the usual snapshot (FR-6 to FR-8). The full builder stays for longer work and opens any simple task.
- A student answers a free-answer question with text and files or pictures; the existing file answer rules apply ([files](../../knowledge/specs/files.md#contexts-and-limits)).
- FR-7 and FR-8 change only in wording: a task may be created and issued in one flow, and its attachments may be uploaded while writing it.

### 4. A tutor CRM: finances and students without an account (finance stage)

- **Students without an account** (owner, 2026-11-23): a student card that has no account (`invited`) may have lessons and ledger entries, visible only to the tutor: no email, no room join and no chat for that student. When the student accepts the card's invitation, the future lessons and the balance become theirs. This amends [ADR-0014](../decisions/0014-students-join-through-their-tutor.md) and the audience rules of FR-4 through a new decision record, and it touches authorization on every route that lists lessons and ledger entries.
- **Ledger** (from proposal 0001, widened to Weekify's logic): a price per relationship and per group, overridable per lesson; payments and packages; each held lesson debits its participants, a cancelled lesson debits nothing unless the tutor applies a cancellation fine; prepaid and owed totals; other income and expenses; attendance, from the conferencing provider's join events and marked by hand for students without an account; a receipt picture on a payment, as a new upload context; a student may mark their own payment for the tutor to confirm or reject.
- **Reports and statistics**: a monthly report and Weekify-like metrics (earnings, load, cancellations, net income, a month comparison and a forecast), downloadable through the export.
- **Reminders** through OVC's channels (email, in-app and, once enabled, web push): low balance and payment received, to the tutor and the student; a morning summary of the day for the tutor; a birthday reminder for the tutor, from a new optional birthday field on the card.
- **Small schedule additions**: tutor-only lesson notes and plans, and personal events in the tutor's calendar (a webinar, time for marking), which the owner may strike in review.
- No money moves through OVC: no online payments, no acquiring, no fiscal receipts.

### 5. Online booking (booking stage)

A tutor publishes a booking page with free weekly slots; a visitor or a signed-in student requests a slot; the tutor accepts or declines, and acceptance creates the lesson (and, for a new person, a card without an account). This removes "students booking slots themselves" from the product's list of things outside the first release. A public page that accepts requests from anyone is a new trust boundary: rate limits, abuse and the personal data of people who are not yet users.

### 6. A task tracker (task tracker stage)

A tutor's own board of tasks in three columns (to do, in progress, done), optionally linked to a student, a group or a lesson. Tutor-only.

### Deferred: notifications through a bot in MAX or VK

The owner first chose, on 2026-11-23, to add notifications through a bot in the Russian messengers MAX or VK, beside email, in-app and web push. Each user links their own messenger account and chooses the kinds. A bot is a new recipient of personal data (at least the messenger account identifier and the notification text), so this part needs a decision record against [ADR-0008](../decisions/0008-hosting-and-data-residency.md) and a lawyer's assessment before it is built. Telegram is not included: its servers are outside Russia. Later the same day the owner decided to leave the bot out for now; it is taken up only by a new decision of the owner.

### Order (owner, 2026-11-23)

The interface rework stage, then the finance stage, the booking stage, and the task tracker stage, then a phone stage for the pages these stages rework (the owner postponed phone design on 2026-11-23 and placed it here the same day), then sub-slice 10.4 (web push) and the partial 10.5, then the pilot deployment. Every CRM stage is planned separately, with its own plan, reviews and slice review, as the interface rework stage is (D49 of the [slice 10 plan](../../knowledge/plans/slice-10.md)). This changes D49, under which 10.4 follows the interface rework directly.

### Deferred: moving data from Weekify

Weekify publishes no export, no file import and no public API; its private API answers only its own site, and its offer forbids circumventing technical protection (§4.1.7) ([research](../../knowledge/research/weekify-2026-11-23.md#3-export-import-api-integrations)). An automated transfer would have to read that private API with the tutor's Weekify session, which OVC will not do. On 2026-11-23 the owner decided to drop the import for now. When the trigger above fires, the import is designed as a spreadsheet import with column mapping and a preview, which would also read a file obtained from Weekify's support and the paste-a-list-of-names convenience Weekify offers; the fields carried are the student's name and email, with other columns offered to the tutor to add to the card's private notes, off by default (owner, 2026-11-23).

### What does not change

The tutor-centric domain and relationship scope ([ADR-0005](../decisions/0005-tutor-centric-domain.md)), the answer visibility and the tutor-only scores ([ADR-0009](../decisions/0009-owner-approved-exceptions-and-visibility.md)), data residency ([ADR-0008](../decisions/0008-hosting-and-data-residency.md)), the adults-only audience and no parents' access, the operator model of [proposal 0006](0006-legal-documents-and-student-data.md), and the absence of online payments.

## Alternatives

- **A bridge to Telegram for chats or notifications:** message content and metadata would leave Russia; rejected by the owner on 2026-11-23 in favour of the interface only.
- **Finances after the pilot, or not at all:** keeps the pilot sooner, but a tutor moving from Weekify loses the money side, which is Weekify's core; the owner chose to build finances first.
- **Students without an account only after they sign up:** keeps ADR-0014 unchanged, but a tutor whose students do not use the system cannot move; the owner chose to allow them.
- **CSV instead of XLSX in the export:** no dependency at all, but no sheets and no typed dates; the owner chose XLSX.
- **An automated Weekify transfer through its private API or a browser extension:** against Weekify's offer, fragile, and it handles another service's session; rejected.
- **Only reworking the builder:** keeps one way to set homework, but the owner's complaint is that the path is not intuitive; the owner chose the simple path beside the builder.

## Plan

1. **Interface rework stage** (slice 10, between 10.3 and the finance stage): export, the messenger, the simple task path and the UI guideline gaps, as the [stage plan](../../knowledge/plans/ui-rework.md) orders them. Its contract changes are specified in its own changes after this proposal is accepted for those parts.
2. **Finance stage:** a decision record on students without an account; the ledger, reports, reminders and schedule additions; the export's finance sheets. Tested with the fake meeting provider's join events and the fake clock.
3. **Booking stage:** the public booking page and requests. Tested end to end with the fake clock.
4. **Task tracker stage.**
5. **Phone stage:** the phone design of every page the stages above rework, with camera capture and the touch message menu.
6. Then sub-slice 10.4, the partial 10.5 and the pilot deployment, as planned.

Each stage changes the functional requirements, the specifications and, where it needs, a decision record in its own first change, once the owner accepts the corresponding part of this proposal.

## Risks and open questions

- **The pilot moves later.** Four stages come before 10.4; the owner chose this order knowing it.
- **Legal bases.** Students without an account keep data on the legitimate-interest basis indefinitely, with lesson and money records; the birthday is a new category; the export puts student data into files outside OVC's protection; a MAX or VK bot, if the owner takes it up later, would add a recipient. Each goes to the lawyer with [proposal 0006](0006-legal-documents-and-student-data.md) `[verify]`.
- **Read status changes a privacy rule.** The chat specification's "no read receipts" changes for direct chats, and FR-12.4 gains the ticks; a student who does not want the tutor to see that they read a message has no switch in this proposal.
- **Export size and cost.** A tutor's export with every file may be large; the export needs limits on frequency, retention and size, decided in its contract change.
- **Spreadsheet content.** XLSX cells holding user text must never become formulas, and file names inside the ZIP must be safe to extract.
- **Ledger correctness.** Entries are append-only with corrections as new entries, rounding and currency rules are open, as proposal 0001 recorded.
- **Weekify may change.** If it publishes an export later, the deferred import is taken up; its facts are dated in the research file.

## Decision

Partly decided by the owner on 2026-11-23, by multiple-choice questions, each answered with the option shown:

| Question | Answer |
|---|---|
| Finances | A ledger as a separate stage before the pilot; no money moves through OVC |
| Messages like Telegram | An interface like Telegram's inside OVC; data stays in OVC |
| Export format | One ZIP: XLSX tables, an `.ics` calendar, the files and a JSON copy |
| Homework | A simple task path beside the builder |
| Students without an account | Allowed to have lessons and a balance, as in Weekify |
| Fields carried by an import | Name and email, other columns optionally into private notes |
| Import from Weekify | Dropped for now, after learning that Weekify has no export and no public API |
| Other Weekify features | All except parents' access: online booking, a task tracker, a MAX or VK bot, reminders; later the same day: no bot for now |
| Phone and computer | Concentrate on the computer version; the phone version is postponed (after a page of mockups shown outside the repository was approved) |
| Order | All CRM stages before the pilot: the interface rework, finances, booking, the task tracker, the bot, then 10.4 and 10.5; later the same day the bot was left out and a phone stage placed before 10.4 |

The resulting requirement, specification and decision record changes are made by the stages above, each after the owner accepts the wording of this proposal for its part.
