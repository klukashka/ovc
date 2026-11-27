# Proposals

Status: draft written 2026-10-20, proposal 0002 added 2026-10-21 and accepted 2026-10-22 (its row pending owner review); proposal 0003 added 2026-10-28 (its row pending owner review); proposal 0004 added 2026-10-29 (its row pending owner review); proposal 0005 added 2026-11-18 (its row pending owner review); proposal 0006 added 2026-11-23 and accepted the same day as ADR-0024 (its row pending owner review); proposal 0007 added 2026-11-23 (its row pending owner review); proposal 0008 added 2026-11-27 and deferred the same day with the owner's response (its row pending owner review); pending line-by-line owner review.

A proposal describes a change to OVC that is designed but not decided or not scheduled: a new provider, several backend instances, recordings, a mobile client. It is where a change is worked out before it becomes a decision record and a specification change.

## Lifecycle

| Status | Meaning |
|---|---|
| `draft` | being written; may be incomplete |
| `proposed` | complete and ready for the owner's decision |
| `deferred` | complete, agreed in principle, waiting for a trigger stated in the proposal |
| `accepted` | decided; the resulting decision record and specification changes are linked from the proposal |
| `rejected` | decided against; the reason is recorded in the proposal |

A proposal is never deleted and never renumbered. When it is accepted, the decision becomes a record in [decisions](../decisions/README.md) and the proposal links to it.

## Writing one

Copy [template.md](template.md) to `NNNN-short-title.md` with the next free number and add a row below.

| Proposal | Status | Trigger |
|---|---|---|
| [0001 Lesson and payment ledger](0001-payment-ledger.md) | deferred | tutors using the MVP ask to track what students owe, or the slice is scheduled |
| [0002 Students join through their tutor](0002-student-onboarding.md) | accepted ([ADR-0014](../decisions/0014-students-join-through-their-tutor.md)) | roadmap slice 3, with invitations and the tutor code |
| [0003 Students aged 14 and over](0003-students-aged-14-and-over.md) | proposed (pending owner review) | a lawyer's assessment of the model before the pilot |
| [0004 Lower the end-to-end suites' CPU cost](0004-end-to-end-suite-cost.md) | proposed (pending owner review) | gates above D46's 165 s target; the temporary 260 s threshold ends with slice 6 |
| [0005 Let the subtests of one integration test share a test database](0005-shared-test-database-for-subtests.md) | deferred (pending owner review) | the gate median still above 540 s with gates run only on an idle machine, or the owner schedules it |
| [0006 Legal documents and student data](0006-legal-documents-and-student-data.md) | accepted ([ADR-0024](../decisions/0024-legal-documents-and-student-data.md), pending owner review) | history: a lawyer's review before the first deployment, which now reviews the three texts instead |
| [0007 A tutor CRM in OVC, data export, a familiar messenger and simpler homework](0007-tutor-crm-scope.md) | proposed (pending owner review) | its parts are scheduled as the interface rework stage and the CRM stages of slice 10; the deferred Weekify import waits for an official Weekify export or public API, or for tutors asking for a spreadsheet import |
| [0008 Run the merge gate on hosted machines in Russia](0008-hosted-ci.md) | deferred: accepted in principle on 2026-11-27, GitHub preferred (pending owner review) | the owner schedules the work |
