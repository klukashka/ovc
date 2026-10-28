# Proposal 0003: Students aged 14 and over

- Status: proposed
- Reviewed: direction chosen by the project owner on 2026-10-25 (students can be anyone; the pilot runs with tutors of school students; scores stay hidden from students and parents) and on 2026-10-28 (a 14-year minimum, an age band as the only age data, the user agreement as the basis with a guardian self-assertion for ages 14–17, no guardian verification, risk acceptance with a complaint address for contact between adults and minors, web push for ages 14–17 as for adults); wording pending owner review
- Author: project owner, written up by an agent
- Date: 2026-10-28
- Trigger: a lawyer's assessment confirms or changes the model before the pilot ([roadmap: before the pilot](../roadmap.md#before-the-pilot)); then this proposal becomes a decision record
- Related: [ADR-0005](../decisions/0005-tutor-centric-domain.md), [ADR-0008](../decisions/0008-hosting-and-data-residency.md), [ADR-0009](../decisions/0009-owner-approved-exceptions-and-visibility.md), [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), [ADR-0015](../decisions/0015-web-push.md), FR-1.1, FR-1.2, FR-1.7, FR-12.3, NFR-1.3, [accounts specification](../../knowledge/specs/accounts.md), [threat model](../threat-model.md), [research on consent practice](../../knowledge/research/minors-consent-practice-2026-10-28.md), [open question on who may use OVC](../../knowledge/architecture/detailed.md#open-implementation-questions)

## Motivation

The pilot runs with tutors of school students, but [ADR-0005](../decisions/0005-tutor-centric-domain.md), the [product overview](../product.md), the [technical introduction](../technical-introduction.md) and the [threat model](../threat-model.md) describe adult students only, and OVC asks nobody's age. Without a decision on minors the pilot either deploys without school students or processes minors' personal data that the documents say OVC does not hold. The roadmap makes this decision record and a lawyer's assessment of a legal guardian's consent under 152-FZ conditions of the first deployment.

The [research on consent practice](../../knowledge/research/minors-consent-practice-2026-10-28.md) found that the Russian exam-preparation schools read let a teenager from 14 register alone without verifying a parent; two of the three take a self-warranty that a parent agreed, and the same two base their main processing on their contract rather than on consent. Their contracts are for paid courses, while OVC is free to students, so whether a 14–17-year-old may accept a free user agreement alone is the first question for the lawyer. This proposal follows that model.

## Proposal

### Age threshold and age band

- Student sign-up, on an invitation page or with a tutor code, asks for an age band: "14 to 17", "18 or older" or "under 14". Only the band is stored, as `age_band` (`minor_14_17` or `adult`) on the student account, the only age data OVC keeps about a student (NFR-1.3).
- The band and the guardian assertion are the student's own statement. A tutor never sets or prefills them, on an invitation card or anywhere else.
- **Under 14 is refused by the API**, not only by the page. On both sign-up routes the check runs right after the per-address throttling slot: with a tutor code, before the code check, the email lookup, the duplicate-email notice and the per-account email counters; on an invitation page, before the token's usability check and the email-binding mismatch counter. The refusal is therefore the same for a valid or invalid code or token and for a new or taken email, never counts a mismatch, never revokes a link and never reveals a card's email binding (T-3, T-8, T-31). It creates no account, stores nothing the person typed, sends no email and does not consume the invitation. It answers with its own error code, and its diagnostic event is a count only, with no email, name or code (T-18). The tutor's invitation card, whose name and email the tutor entered, stays with the tutor as it is.
- **An existing account answering "under 14"** (when it states its band after a reclaim, as an account without a band, or after the operator lowered it) is not stored as such. Because it destroys data, it takes effect only with the current password and an explicit confirmation, as account deletion does ([account deletion](../../knowledge/specs/accounts.md#account-deletion)); without them the answer is refused with no effect. Wrong current passwords on this route count in the same per-account counter as a password change and answer `429` beyond its limit ([sessions and passwords](../../knowledge/specs/accounts.md#sessions-and-passwords)). With them, the API refuses to keep the answer and starts the account's deletion. An unconfirmed account is purged at once; otherwise the account is deleted with the deletion effects of FR-1.7 and purged at once rather than after the usual 30 days, unless the lawyer's assessment sets another period, and `ovc-admin users restore` refuses it. A test proves it.
- **A tutor** confirms at sign-up that they are 18 or older. Only the version and time of that confirmation are stored; nothing verifies it. An existing tutor account without a recorded confirmation gives it, on its own confirmation route, before any other route works.

### Legal basis and consent

- The main processing (the account, relationships, lessons, chat, homework, notifications) performs the user agreement that a person accepts with the sign-up button, which links to the agreement and the privacy policy.
- A student who chooses "14 to 17" must also tick an unticked box stating that their parent or legal guardian agrees to their use of OVC. The text version and time are recorded, as consent is today.
- The existing personal-data consent checkbox of sign-up ([accounts](../../knowledge/specs/accounts.md#registration-and-email-verification)) stays until the lawyer says whether the contract basis makes it unnecessary.
- Separate, unticked consent is needed only for processing beyond the agreement, such as lesson recordings after the MVP (T-11). OVC does no marketing processing.
- The agreement, the policy and the wording of the assertions are the owner's obligations outside the repository ([ADR-0008](../decisions/0008-hosting-and-data-residency.md)).

### Changing the band

- A student changes `minor_14_17` to `adult` in settings with one action, which records the time of the change and the student's acceptance of the agreement on their own behalf. The band never moves back from settings; only the operator lowers it.
- A reclaim ([ADR-0014](../decisions/0014-students-join-through-their-tutor.md): confirming the email from another browser, or resetting the password of an unconfirmed account) clears the band and the assertion, and the mailbox owner states them again before any other route works (T-23, T-44).
- An existing student account without a band states it before any other route works; `adult` is never assumed. After the operator lowers a band to `minor_14_17`, the student gives the guardian assertion the same way.
- "Before any other route works" covers every HTTP route and WebSocket frame except refresh, signing out and signing out everywhere, email confirmation and its resend, password reset and change, account deletion, `GET /users/me`, the route that states the band and the assertion, and the tutor's confirmation route.
- Every write of the band or the assertion, at the gate and in settings, takes the account row lock and re-checks the session in the same transaction ([locks and the session re-check](../../knowledge/specs/accounts.md#locks-and-the-session-re-check)), and joins the list of writes open to unconfirmed accounts under T-23. A request authenticated before a reclaim therefore answers `401` and changes nothing; an integration test proves it (T-23, T-44).

### Who sees the band

The student, and a tutor only where the tutor already sees the linked account (`active` and `provisional` relationships); never on `invited`, `reclaim_pending` or deleted-before-purge cards, never in the join-request list, never to other students or group members (T-5, T-6). The band never appears in emails, in-app notifications, push payloads, the conferencing provider or logs ([observability](../../knowledge/specs/observability.md#safe-content), T-18). A provisional account may be a squatter's, so the band a tutor sees on a provisional link is the squatter's statement, a residual risk under T-44. A contract test proves the visibility rules.

### Guardian requests and accounts found to be under 14

- An out-of-band operator process handles a legal guardian's request to see, withdraw consent for or delete a minor's data, and an account found to belong to a child under 14: how the guardian's authority is checked, the deadline for answering, and the outcome. No action is taken and no data is disclosed before the authority is checked, because a person posing as a guardian could otherwise have a student's account deleted or obtain their data; this impostor joins the threat model under T-22. The lawyer's assessment sets the details.
- `ovc-admin` gains two audited subcommands (T-22): one that deletes an account with the deletion effects of FR-1.7 and purges it at once, because blocking keeps data indefinitely and conflicts with the purge rules (T-34), and one that sets a student's band back to `minor_14_17`, after which the student gives the guardian assertion before any other route works.

### Adults with access to a minor

A new actor in the threat model: an adult with access to a minor, either a tutor or an adult member of the same group. The controls in the MVP are the operator's blocking and deletion, a complaint address in the agreement and the policy, and message retention by the existing rules. The owner accepts these residual risks:

- an author deletes their own message at once (FR-12.3), so a complaint may arrive after the evidence is gone;
- one-to-one lessons are not recorded in the MVP;
- adult students see minors' names and avatars in a shared group;
- a teenager can claim 18 or older and sign up as a tutor, and a minor can claim adulthood in one step, leaving any rule that applies only to minors;
- no age is verified.

With the parent role, stronger controls are revisited for the two residuals that matter most: an adult's access lasting between a complaint and the operator's action (T-21), and repeated unwanted contact, which extends T-33 from join requests to chat and groups.

### What does not change

- Points and scores never reach students of any age ([ADR-0009](../decisions/0009-owner-approved-exceptions-and-visibility.md)); by the owner's decision of 2026-10-25 they will not reach parents either, which the decision record adds to ADR-0009's rule.
- Relationship scope ([ADR-0005](../decisions/0005-tutor-centric-domain.md)) and data minimisation (NFR-1.3) stay as they are.
- Web push stays off for everyone until the legal assessment and the decision record that replaces this proposal are recorded ([ADR-0015](../decisions/0015-web-push.md)); afterwards `minor_14_17` accounts get the same opt-in as adults, unless the assessment restricts it. Any such restriction is a server-side refusal both when a browser subscribes and when the worker sends, and it is a policy limit rather than a control, because a student can change the band to `adult` in one step (T-46).
- A parent role is not part of this proposal; it follows after the MVP with its own proposal and decision record.

### Where the decision lands once accepted

One change: a decision record replacing the adults-only wording of ADR-0005; FR-1.1, FR-1.2 and a new age requirement; the accounts specification (sign-up, settings, reclaim, `ovc-admin`); the data model (`age_band` and the assertion records); the product overview's "Who it is for", the technical introduction and the project description in `AGENTS.md`; and the threat model (the asset "personal data of adults" becomes personal data of students from 14, minors included; the new actor, the impostor guardian and the residual risks above; T-3, T-5, T-6, T-8, T-11, T-18, T-21, T-22, T-23, T-31, T-33, T-34, T-44 and T-46 updated). The open question on who may use OVC is then marked resolved.

## Alternatives

- **A birth date:** the age would update itself, but it is more personal data than any feature needs (NFR-1.3).
- **One question, "18 or older?":** it cannot keep children under 14 out, which the threshold needs.
- **A guardian confirming by an emailed link:** none of the services researched does this for a free sign-up; about half a slice of work and an extra step for families. It stays the fallback if the lawyer requires a guardian's consent for ages 14–17.
- **Parent accounts in the MVP:** a third role and a change to FR-1.3, about one and a half slices, ahead of the pilot.
- **Students under 14:** they need a contract with the parent, as services for younger children have, and are out of scope.

## Plan

After the lawyer's answer: the decision record and the document changes in one change, then the implementation, about a quarter of a slice, in slice 10 or earlier in a change that touches sign-up.

The implementation covers the band field and its migration, the sign-up form and API, settings, the reclaim reset, the two `ovc-admin` subcommands, and texts in Russian and English. It is tested with integration tests over the real API and database:

- an under-14 sign-up writes nothing and answers alike for any code, token and email on both sign-up routes, without counting a mismatch or revoking a link;
- an existing account answering "under 14" without the current password changes nothing, wrong passwords beyond the password-change limit answer `429`, and with the password the account is deleted and purged and cannot be restored;
- a band write from a session authenticated before a reclaim answers `401` and changes nothing;
- the assertion is required for "14 to 17";
- the band moves only to `adult` from settings;
- a reclaim clears the band;
- the band is visible only within scope;
- the new subcommands are audited.

A pilot scenario step covers a student signing up as "14 to 17". Until this is implemented OVC has no age gate, so the pilot admits no school students before it.

## Risks and open questions

- How long an account found to belong to a child under 14 is kept before purge: at once in this proposal, unless the lawyer sets another period.
- The lawyer may require a guardian's consent for ages 14–17; the emailed-link fallback then adds about half a slice before the pilot.
- The [questions for a lawyer](../../knowledge/research/minors-consent-practice-2026-10-28.md#questions-for-a-lawyer) are open, chiefly:
  - whether a 14–17-year-old may accept the free agreement alone;
  - whether a self-warranty of a parent's consent is enough;
  - who is the operator of a student's data when the tutor is paid;
  - how a guardian's request is handled.
- False age claims cannot be prevented; the residual risks above are accepted by the owner.

## Decision

Filled in when the status becomes `accepted` or `rejected`.
