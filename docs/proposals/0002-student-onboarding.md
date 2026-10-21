# Proposal 0002: Students join through their tutor

- Status: draft
- Reviewed: direction and the rules marked "decided" below chosen by the project owner on 2026-10-21, including the scope of the email rule and a new card after removal; wording pending line-by-line owner review, after which the status moves to `proposed`
- Author: project owner, written up by an agent
- Date: 2026-10-21
- Trigger: implemented in roadmap slice 3, together with invitations and the tutor code; to be accepted before slice 3 starts
- Related: FR-1.1, FR-1.2, FR-1.3, FR-2.1, FR-2.2, FR-2.3, [accounts specification](../../knowledge/specs/accounts.md), [API specification](../../knowledge/specs/api.md), [ADR-0005](../decisions/0005-tutor-centric-domain.md), [threat model](../threat-model.md) T-3, T-4, T-6, T-7, T-8, T-21, T-23, T-27, T-28, T-31, T-33, T-34

## Motivation

A student should be able to start working with their tutor in one short, obvious step, without losing the protections OVC already has against account takeover, email squatting and account enumeration.

The current specification gives a student four ways in, with different rules:

- an invitation sent by email, which confirms the email when accepted;
- a copied invitation link, which creates an unverified account and reserves the invitation until the email is confirmed;
- a tutor code entered at sign-up, which is stored and evaluated only after the email is confirmed;
- a sign-up with no invitation and no code, which leads to an empty Home asking for a code.

The differences are invisible to the student but produce many edge cases (reserved invitations, pending codes, two invitation deliveries with different effects), and a student who signs up on their own lands in an empty account with nothing to do.

## Proposal

**A student always arrives through a tutor.**

1. **The invitation link is the main path.** A tutor creates a student card and shares its link, either by copying it into any messenger or by letting OVC email it. Both are the same single-use link with the same behaviour. The student opens it, sees who invites them ("A language tutor invites you to study"), enters a name, email and password, and is linked to the tutor at once, provisionally (rule 3).
2. **The email is confirmed afterwards**, with one click from the confirmation email, as for every account. Until then the student sees a banner asking them to confirm, and the tutor's student card shows "joined, email not confirmed". An account that stays unconfirmed for 7 days is removed, which also discards the provisional relationship it created (rule 1 below).
3. **A student who already has an account** (for example with another tutor) signs in on the invitation page and accepts with one click.
4. **The tutor code is the secondary path**, for tutors who publish a code. A student enters the code at sign-up or later, gets "request sent", and is linked when the tutor accepts.
5. **There is no student sign-up without an invitation or a code.** "I study" on the sign-up page leads to "Ask your tutor for a link, or enter their code".

### Rules that keep it safe

The owner decided the core of each rule labelled "Decided": the provisional link that the tutor can remove, with a new card after removal; the card's email binding the link; code requests held until confirmation, with cards matched only on a confirmed email; and, before confirmation, only the one-to-one relationship and no notification emails, while account and invitation emails are still sent. The details around that core, and every rule not labelled "Decided", are proposed and pending owner review.

- **Decided: a link made before confirmation is provisional.** While the student's email is unconfirmed, the relationship is in a provisional state, and the tutor can remove the student. Removal is a conditional update that succeeds only while the account is still unconfirmed and otherwise answers a named refusal. On removal, or when the 7-day cleanup removes the account:
  - the provisional relationship becomes `discarded`, and its chat, lessons and work stay with the removed account and are never passed to anyone else;
  - the removed account loses access to that tutor at once: chat membership is withdrawn, new meeting join links are refused and, from slice 4, live connections close; someone already connected, or holding an earlier join link, can stay until the meeting ends or the tutor restarts the room, the residual T-21 already accepts;
  - the tutor gets a new `invited` card with the same display name, email and notes, and a new link;
  - a discarded provisional relationship is never reactivated, even when the same account later accepts the new link.

  Ending a provisional relationship the usual way is not offered: "End" on a provisional student performs removal, so the card is never permanently bound to an account whose email is unproven.

  The relationship becomes permanent, bound to that account forever, only when the email is confirmed. Whoever owns the mailbox can always reclaim the address: by confirming it, or through a password reset, which takes over the account.
- **Decided: the card's email binds the link.** When the card has an email, sign-up and signed-in acceptance require that email and refuse any other with `invitation_email_mismatch`. The invitation page never shows the email. A card without an email is first come, first served.
- **Decided: requests by code wait for confirmation.** A join request from an unconfirmed account is held: the tutor does not see it and it does not count toward the tutor's pending-request limit until the email is confirmed. Accepting a request links an `invited` card only when the card's email matches the student's confirmed email.
- **Decided: before confirmation a student reaches only the one-to-one relationship**: the tutor's profile, the private chat, one-to-one lessons and their own homework. The tutor cannot add an unconfirmed student to a group, so group chats, group lessons and recordings with other students stay out of reach. No notification email (lessons, reminders, chat, homework, relationships, groups) is sent to an unconfirmed address. Account emails (confirmation, password reset, the duplicate sign-up notice) and invitation emails are sent as specified, under their existing sending limits.
- **Proposed, pending owner review: limits that stop guessing.** Repeated `invitation_email_mismatch` refusals on one link revoke it (for example after 5), counted per link, and the tutor sees that in OVC; the revoked link then gets the same "no longer valid" answer as any other. Entering a code from an unconfirmed account always answers "request sent", never `tutor_unavailable`, as for a blocked requester.
- **Sign-up through an invitation reveals no account.** `POST /invitations/{token}/accept` with a new account answers `202` with no session and the same body whether or not the email already has an account, and hashes the password in both cases so timing does not separate them. For an existing account it neither links nor consumes the invitation, and the existing owner gets the usual duplicate sign-up notice. The page then signs the new student in with the password they just chose; if that sign-in fails, it shows the same "check your email, or sign in to accept" state, never a wrong-password error. That the automatic sign-in works only for a new account is the same residual signal as ordinary sign-up followed by sign-in (T-3).
- **Signing in on the invitation page happens in place.** The token, already removed from the address bar, stays in the page's memory and never travels in `next` or any other URL. A tutor account still gets `role_mismatch`, and an unconfirmed account cannot accept a second tutor's invitation until it confirms its email.

What stays the same:

- invitation links are single use, expire after 14 days, and regenerating one revokes the old one;
- the tutor sees who joined and can end a confirmed relationship;
- confirmation tokens, the identical answers that reveal no account, sending limits and throttles are unchanged;
- one account has one role, and a tutor account cannot accept an invitation or enter a code.

What changes:

- an emailed invitation no longer confirms the email on its own, and nothing is reserved until confirmation: a provisional link replaces both (T-23);
- the unverified-route allowlist of the accounts specification grows by the one-to-one routes listed above, which changes the API contract of those routes for unconfirmed students.

What it removes from the specification:

- the reserved invitation (`invitations.reserved_by_user_id`, `users.reserved_invitation_id`);
- the pending tutor code evaluated at confirmation (`users.pending_tutor_code`), replaced by a held join request;
- the two invitation deliveries with different effects;
- the student branch of `POST /auth/register` and the page `/signup/student`, the sign-up without an invitation or code.

## Alternatives

- **Keep the current model.** It is safe, but the extra paths and the empty self-signed-up account make joining harder to explain and to test.
- **Link only after the email is confirmed.** Safer against a mistyped email, but it keeps the reservation mechanism and delays the student's first contact with the tutor. The provisional link gives the same protection where it matters: the tutor can undo it, and the link holder reaches nothing beyond the one-to-one relationship until the email is proven.
- **Invitation link only, no tutor code.** Simpler still, but a tutor who wants to publish one code (in a profile or an advertisement) could not.

## Plan

- Roadmap slice 2 ships tutor sign-up only; student sign-up arrives in slice 3 with invitations and the tutor code.
- Slice 3 changes, in the same change that implements them:
  - FR-1.2 and FR-2 wording;
  - the [accounts specification](../../knowledge/specs/accounts.md): registration, the unverified-route allowlist (including the upload scope checks once one-to-one chats and submissions are reachable), invitations, the tutor code, the provisional relationship state in the relationship state table, including whether it passes the join rule for one-to-one lessons, and its removal (whether removal applies the ending effects to future lessons, how long the discarded relationship's content is kept and whether the tutor keeps read-only history, what the removed student sees, and a removal that races a confirmation, both serialised on the same row), and the cleanup of a linked unconfirmed account;
  - held join requests: what happens at confirmation when the tutor's pending limit is full, the tutor has blocked the student, the code was regenerated or the tutor is unverified or deleted, and which clock the 14-day expiry runs on;
  - the [API specification](../../knowledge/specs/api.md): `POST /invitations/{token}/accept`, `POST /join-requests`, the removal route for a provisional link, `invitation_email_mismatch`, and the `role: student` refusal on `POST /auth/register`;
  - the data model: no reservation columns, no pending code, a held state for join requests;
  - the pages `/invite/{token}` and `/signup`, and the navigation rule that an unverified student sees only the confirmation page;
  - the pilot scenarios: scenario 2 step 2, which today signs Boris up with a code and expects "Request sent" after confirming; scenario 1 step 5, where the emailed link confirms the email; scenario 4's seed; and the "reservations and races" entry of the test matrix;
  - the threat model: T-3 (identical invitation sign-up, the automatic sign-in and the card state as residual signals, the email mismatch limit if it is accepted), T-7 (forwarded links, email binding, provisional link, and the mismatch limit if it is accepted, including that a link holder can then revoke a leaked link on purpose), T-8 and T-33 (held requests), T-23 (rewritten for the provisional link and reclaim), T-31 (no notification email to unconfirmed addresses), T-6 (no group membership before confirmation; a removed provisional relationship's data never passes to another account), T-21 (a removed provisional student loses access at once), T-27 and T-28 (sign-in on the invitation page), T-34 (purge of a linked unconfirmed account and of a discarded provisional relationship), and a new threat: an unconfirmed, possibly squatting, student reaching a tutor's practice.
- Tests use the database mail sink and the shared test clock:
  - joining through a copied link and through an emailed link behave identically;
  - an invitation sign-up with an existing email gives the same response and links nothing;
  - a card with an email refuses another email;
  - removal racing a confirmation leaves exactly one outcome;
  - the tutor removes a provisional student, the removed account loses access, and the tutor gets a new card with a new link that carries none of the old chat or work;
  - if the mismatch limit is accepted, a link refused for email mismatches too many times is revoked;
  - an unconfirmed student reaches the one-to-one routes and is refused group membership;
  - a request by code stays invisible to the tutor until confirmation;
  - an unconfirmed student is removed after 7 days with their relationship, silently;
  - a forwarded link used once cannot be used again.

## Risks and open questions

- **A forwarded or leaked link** for a card without an email lets whoever opens it first join. The tutor sees the joined name and email, can remove the provisional student, and the intended student gets a new link.
- **A squatter on a card with an email** holds that email in their unconfirmed account even after removal, so the intended student's sign-up takes the duplicate path. The intended student reclaims the address through a password reset and then accepts the new link while signed in. The squatter could accept again only if the new link leaks too.
- **Content written during the provisional period** (chat messages, submissions) stays in the account when its mailbox owner reclaims it through a reset, now attributed to the owner. For a card without an email, the reclaiming owner would also inherit a provisional link to a tutor they never chose. Open question for slice 3, recommended answer: when the mailbox owner reclaims an unconfirmed account (through a password reset, or a confirmation that sets a new password), erase the provisional-period content and discard its provisional relationships with the removal effects, including a new `invited` card for each tutor, before the email counts as confirmed.
- **A mistyped email** leaves a joined student who cannot confirm. Open question for slice 3, recommended answer: allow correcting an unconfirmed email under the existing confirmation sending limits, invalidating earlier confirmation tokens, and refused when the card binds a different email (the tutor then issues a new link).
- **The 7-day cleanup** of a linked student erases what they did in those days. It applies the removal effects of a provisional relationship silently: the relationship becomes `discarded`, the student loses access and the tutor gets a new `invited` card, with no `relationship.ended` email. Restoring or unblocking an account into `unverified` restarts its 7-day clock, and deleting an unverified account purges it at once, as decided for slice 2; slice 3 checks both against linked unconfirmed accounts.

## Decision

To be filled in when the owner accepts the wording.
