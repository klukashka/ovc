# Weekify against OVC: a gap map, 2026-11-23

Status: agent analysis written 2026-11-23 for the interface rework stage of slice 10 (D49 of the [slice 10 plan](../plans/slice-10.md)); it decides nothing. The owner's decisions it cites were taken by multiple-choice questions on 2026-11-23 and are recorded in [proposal 0007](../../docs/proposals/0007-tutor-crm-scope.md) (pending owner review). Weekify's facts come from the [Weekify research](../research/weekify-2026-11-23.md), which also says how much to trust each; OVC's facts come from the [functional requirements](../../docs/requirements/functional.md), the specifications in `knowledge/specs/` and the page design in `knowledge/design/web/` at commit `15317dc`.

## Why this map exists

On 2026-11-23 the owner asked that OVC contain all the logic Weekify («Викифай») has, that a Weekify user can move their data into OVC, that every user can download their tables, schedule and financial reports as a file, that Messages follow the familiar messenger pattern, and that homework become intuitive, with files and photos as tasks and as answers. This map compares the two products feature by feature so that each owner comment lands in a named place.

## The two models in one paragraph each

- **Weekify** is one tutor's CRM. A student is a record the tutor keeps: name, phone, birthday, parents' contacts, notes and a balance. The student need never sign in. The tutor's weekly template fills the calendar, a held lesson debits the balance, and payments raise it. Optionally a student or a parent opens a shared cabinet by an invitation code and sees the schedule, the homework text, attendance and payments. There is no video room, no chat, no homework submission and no export.
- **OVC** is a shared workspace of a tutor and their adult students. A student is a person with an account ([ADR-0014](../../docs/decisions/0014-students-join-through-their-tutor.md)); a student card without an account (`invited`) exists, but lessons, chats and homework attach only once the student has accepted (`provisional` or `active`). OVC hosts the lesson itself (BigBlueButton), the chats and the whole homework cycle with automatic and tutor checking. It keeps no money records.

## Verdicts

- **already in OVC**: OVC has the capability at least as fully; no work in this stage.
- **in OVC, UI rework**: OVC has it, and the owner's comments or the [UI guidelines research](../research/ui-guidelines-2026-11-23.md) ask for a better interface; planned in the [interface rework stage plan](../plans/ui-rework.md).
- **new scope**: neither the requirements nor a planned slice cover it; it widens the product and goes through proposal 0007 before any requirement or decision record changes.
- **out of scope**: not planned, with the reason.

## Students and groups

| Feature | Weekify | OVC today | Verdict | Where |
|---|---|---|---|---|
| Student card | Name, phone, email, birthday, goals, notes, messenger links, parents' contacts, low-balance limit, archive | Display name, optional email, colour, private notes; end and restore a relationship (FR-2) | already in OVC for name, email and notes | — |
| Birthday on the card and a birthday reminder | Yes | No | new scope (owner, 2026-11-23: every Weekify feature except parents' access) | Proposal 0007, finance stage; widens the personal data collected, so the policy and consent texts of [proposal 0006](../../docs/proposals/0006-legal-documents-and-student-data.md) change with it |
| Phone and messenger links on the card | Yes | No | out of scope: OVC collects only what a feature needs (AGENTS.md, Security); no OVC feature uses a phone number | — |
| Parents' contacts and a parents' cabinet | Yes | No; OVC serves adults ([proposal 0003](../../docs/proposals/0003-students-aged-14-and-over.md) is pending) | out of scope (owner, 2026-11-23) | — |
| Students who never sign in, with lessons and a balance | Yes, the default | No: lessons need an accepted relationship | new scope (owner, 2026-11-23: allow it, as Weekify does) | Proposal 0007, finance stage; needs a decision record amending ADR-0014 and the scope rules of FR-4 |
| Adding many students by pasting a list of names | Yes | One card at a time | new scope, deferred with the Weekify import (owner, 2026-11-23) | Proposal 0007, deferred part |
| Groups | Up to 200 students, group lessons | Groups with colour, chat, lessons, issued work, archive (FR-3) | already in OVC | — |

## Schedule and lessons

| Feature | Weekify | OVC today | Verdict | Where |
|---|---|---|---|---|
| Weekly schedule | A week template generates the calendar | Weekly series on one or more weekdays with an end rule, kept in the tutor's wall-clock time (FR-4.2, ADR-0007) | already in OVC | — |
| Move, cancel, add a one-off lesson | Yes | Yes, a single lesson or this and following (FR-4.3) | already in OVC | — |
| Week and month views | With load and earnings | Week and month, agenda on phones (FR-4.5) | already in OVC; earnings come with finances | Finance stage |
| Other events in the calendar (a webinar, time for marking) | Yes | No | new scope, small | Proposal 0007, finance stage, as the owner may strike it in review |
| Per-lesson plan and notes for the tutor | Yes | A lesson description and materials, visible to participants; no private lesson notes | new scope, small | Proposal 0007, finance stage |
| Attendance | Shown to the student | Join events exist on the provider side; not recorded as attendance | new scope | Proposal 0007, finance stage (from proposal 0001) |
| Calendar export | Not found | Not in the first release ([product](../../docs/product.md)) | new scope, as a one-time `.ics` file in the export | Interface rework stage, export change |
| A subscribed calendar feed with a secret address | Not found | No | out of scope for now: a long-lived credential in a URL, which AGENTS.md allows only where a specification and ADR-0009 define it | — |

## Money

| Feature | Weekify | OVC today | Verdict | Where |
|---|---|---|---|---|
| Lesson price, per student or per lesson | Yes | No | new scope | Finance stage (proposal 0007, absorbing [proposal 0001](../../docs/proposals/0001-payment-ledger.md)) |
| Payments, balances, automatic debit for a held lesson | Yes | No | new scope | Finance stage |
| Prepaid and owed totals ("deposits", "credits") | Yes | No | new scope | Finance stage |
| Cancellation fine | Yes | No | new scope | Finance stage |
| Packages of lessons | Not found | No | new scope (proposal 0001 already has them) | Finance stage |
| Other income and expenses, net income | Yes | No | new scope | Finance stage |
| Statistics: 12 metrics, forecast, month comparison | Yes | Homework analytics after the MVP (FR-11.2 to FR-11.4); no money statistics | new scope | Finance stage |
| A receipt picture on a payment | Yes, JPEG, PNG, HEIC converted, 3 MB | No | new scope; a new upload context | Finance stage |
| A student marks their own payment for the tutor to confirm | Yes (by parents or students) | No | new scope | Finance stage |
| Low-balance and payment-received reminders | Yes | No | new scope | Finance stage, through OVC's channels |
| Online payment by students, fiscal receipts | Not found | No | out of scope: a regulated flow; proposal 0001 excludes it | — |
| Monthly financial report as a file | Not found | No | new scope | Finance stage adds its sheets to the export of the interface rework stage |

## Homework and materials

| Feature | Weekify | OVC today | Verdict | Where |
|---|---|---|---|---|
| Homework | A text field per lesson | Templates of blocks, issuing with a deadline and an answer policy, autosaved answers, submission, review and feedback (FR-6 to FR-11) | already in OVC, far beyond Weekify | — |
| Auto-checked tests | Not found | Single and multiple choice, short text, short number (FR-7.2) | already in OVC | — |
| Free answers checked by the tutor | Not found | Long text and file answers (FR-7.3), the review queue (FR-10) | already in OVC | — |
| A photo or file as the student's answer | Not found | `file` questions take pictures (JPEG, PNG, WebP, GIF), PDF, office documents and audio, 1 to 5 files ([files](../specs/files.md#contexts-and-limits)) | in OVC, UI rework: a plainer path on computers; a camera-first answer on phones comes with the phone design the owner postponed on 2026-11-23 | Interface rework stage; the camera with the deferred phone design |
| A file or photo attached to the task itself | Not found | Only through a library material block, or up to three prompt pictures | in OVC, UI rework: attach directly while writing the task (owner, 2026-11-23: a simple task path beside the builder) | Interface rework stage |
| An intuitive way to set a task | — | The builder of eight block types, then the Issue dialog | in OVC, UI rework | Interface rework stage |
| Materials library | Not found | Folders, materials, search (FR-6) | already in OVC | — |

## Communication and notifications

| Feature | Weekify | OVC today | Verdict | Where |
|---|---|---|---|---|
| Chat between tutor and student | Not found; messenger links only | Direct and group chats with files, edit, delete, unread, presence, typing (FR-12) | in OVC, UI rework: the familiar messenger pattern (owner, 2026-11-23: an interface like Telegram's inside OVC, no bridge) | Interface rework stage |
| Chat carried over Telegram | — | — | out of scope: message content would leave Russia ([ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md)); the owner chose the interface only | — |
| Lesson reminders | Two, at times the tutor chooses | Email 24 hours and 1 hour before (FR-13.2); web push planned in 10.4 | already in OVC; configurable times are not planned | — |
| Morning summary of the day | Yes | No | new scope, small | Finance stage, with the other reminders |
| Notifications through a bot in VK or MAX | Yes (and Telegram) | Email, in-app, web push planned | deferred: chosen by the owner on 2026-11-23 and left out for now later the same day; Telegram out of scope as above | Proposal 0007, deferred part; if taken up, it needs a decision record against ADR-0008 and a lawyer's assessment first |
| Video lessons and a whiteboard | Not found | BigBlueButton rooms (FR-5) | already in OVC | — |

## Other tools

| Feature | Weekify | OVC today | Verdict | Where |
|---|---|---|---|---|
| Online booking: a public link with free slots, requests to accept | Yes | Not in the first release ([product](../../docs/product.md)) | new scope (owner, 2026-11-23) | Booking stage: a new public page and a new trust boundary |
| Task tracker for the tutor | Kanban columns | No | new scope (owner, 2026-11-23) | Task tracker stage |
| Link shortener with QR codes | Yes | No | out of scope: unrelated to tutoring, and a redirect service is an abuse surface; the owner may raise it in review | — |
| Sign-in through VK, Telegram or MAX | Optional | Email and password | out of scope: another identity provider and its personal data | — |
| Session list, sign out everywhere | Yes | Sign out of all devices (FR-1.4) | already in OVC | — |
| Mobile apps | iOS, RuStore, APK | Phone layouts (10.3), installable web app (10.4); store apps after the MVP | already planned | — |

## Data portability

| Feature | Weekify | OVC today | Verdict | Where |
|---|---|---|---|---|
| Export of the user's data as files | Not found | No | new scope (owner, 2026-11-23: one ZIP with XLSX tables, an `.ics` calendar, the files and a machine-readable JSON copy) | Interface rework stage |
| Import of a Weekify user's data | Weekify has no export, no file import and no public API; its private API answers only its own site, and its offer forbids circumventing technical protection (§4.1.7) | — | deferred (owner, 2026-11-23: drop the idea for now) | Proposal 0007, deferred part, with a trigger |
| Import from a spreadsheet | Not found | No | deferred with the Weekify import | Proposal 0007, deferred part |

## What OVC has that Weekify lacks

Video lessons with a whiteboard and presentation, direct and group chats, the whole homework cycle with automatic and tutor checking, corrected files and feedback, a materials library, students who study with several tutors in one account, email notifications with one-click unsubscribe, two interface languages, and an operator command-line tool. Weekify's strength is the money side and the "students who never sign in" model; proposal 0007 brings both.

## Legal notes for the new scope

- **Export.** A tutor's export holds what the tutor already sees about their students, a student's export holds the student's own work. With OVC as the only operator ([proposal 0006](../../docs/proposals/0006-legal-documents-and-student-data.md), decision 2), a tutor's download of student data is a use of data the tutor already has access to, but the file then leaves OVC's protection; the user agreement should say the tutor is responsible for the files they download `[verify with a lawyer]`. A student's export supports their right of access under article 14 of 152-FZ, but does not by itself answer such a request, because private tutor notes are not in it ([legal gaps](legal-gaps-2026-11-22.md)).
- **Students without an account.** Their data rests on the same basis as the name and email a tutor enters before an invitation: the legitimate interests of item 7 of part 1 of article 6, supported by the tutor's warranty (proposal 0006, decision 3), but held indefinitely rather than until an invitation expires, and with lesson and money records added; this is a new question for the lawyer `[verify]`.
- **Birthday.** A new category of a student's personal data; the policy and consent texts list it.
- **Weekify import.** Deferred. Any later import from a file the user obtained lawfully, for example from Weekify's support, is the user's own action; automated reading of Weekify's private API is not planned, because of the offer's §4.1.7 and because OVC would handle the user's Weekify session.
- **MAX or VK bot, deferred.** If taken up, a new recipient of personal data (at least a messenger account identifier and the notification text); Russian services, but a new processor or an independent operator, so it needs the lawyer and a decision record before it is built.
