# ADR-0014: Students join through their tutor

- Status: accepted, 2026-10-22
- Reviewed: decided by the project owner on 2026-10-22 by accepting [proposal 0002](../proposals/0002-student-onboarding.md), with the owner's choices on reclaiming an unconfirmed account and on students ending a relationship, and the same day on the name of a card created by accepting a join request; wording pending owner review, line by line
- Deciders: project owner
- Supersedes: the first bullet of item 4 of [ADR-0009](0009-owner-approved-exceptions-and-visibility.md) (an invitation sent by email no longer confirms the invitee's email)
- Supersedes in part: [ADR-0005](0005-tutor-centric-domain.md), in its relationship states (adding `provisional` and `reclaim_pending`, removal of a provisional link, and ending by the student) and in its rule that a student reaches a tutor's resources only through an active relationship (a provisional link also reaches the one-to-one relationship)
- Related: FR-1.1, FR-1.2, FR-1.3, FR-1.7, FR-2.1 to FR-2.3, FR-2.7, FR-2.8, [accounts specification](../../knowledge/specs/accounts.md), [API specification](../../knowledge/specs/api.md), [data model](../../knowledge/specs/data-model.md), [threat model](../threat-model.md) T-3, T-7, T-23, T-31, T-34, T-44

## Context

The specification gave a student four ways in with different rules: an emailed invitation that confirmed the email when accepted, a copied link that reserved the invitation until confirmation, a tutor code stored on the account and evaluated at confirmation, and a sign-up with neither, which led to an empty account. The differences were invisible to the student and produced many edge cases. [Proposal 0002](../proposals/0002-student-onboarding.md) replaces them with one model in which a student always arrives through a tutor. While deciding it, the owner also chose how the owner of a mailbox treats links that someone else made in an unconfirmed account with that address, and that a student can end a relationship as a tutor can.

## Decision

- **A student always arrives through a tutor**, by an invitation link or a tutor code. There is no student sign-up without one: `POST /auth/register` accepts `role: student` only with a tutor code, and `/signup/student` is the code form. A tutor account that opens an invitation or enters a code is refused with `role_mismatch`. Invitation links and codes work only while their tutor's account is `active`.
- **One invitation link per card.** A tutor's student card has at most one open invitation: a single-use link valid for 14 days that the tutor copies or has OVC email; both are the same link with the same behaviour. Regenerating, withdrawing the card or changing its email revokes it, and changing the email issues a fresh link at once.
- **An invitation never confirms an email.** Signing up on the invitation page creates an `unverified` account and links it to the card at once, in the state `provisional`. Signing in on the invitation page and accepting links a confirmed account as `active`. An account holds at most one `provisional` relationship, so an unconfirmed account cannot accept a second tutor's invitation until it confirms its email.
- **The card's email binds the link.** When the card has an email, sign-up and signed-in acceptance require that email and refuse any other with `invitation_email_mismatch`; five refusals on one link revoke it, and the tutor sees that on the card. A card without an email is first come, first served. A link of an `ended` card is bound to that card's student account.
- **A provisional link reaches only the one-to-one relationship.** It cannot join a group and receives no notification emails; account and invitation emails are sent as usual. From the slices that add them, it reaches only one-to-one lessons and the private chat. The tutor sees the card as "Email not confirmed" and can remove the student at any time.
- **Removal** ends a provisional link silently: the relationship becomes `discarded`, the removed account no longer lists the tutor, and the tutor gets a new `invited` card with the same display name, email and notes and a fresh link; the discarded row is invisible to the tutor and is never reactivated. "End" on a provisional student performs removal, and so does a student's own "Stop studying" on a provisional link, which therefore never becomes `ended`. The 7-day cleanup of an unconfirmed account and the immediate purge of a deleted unconfirmed account apply the same effects.
- **Confirmation from the account's own browser** (a browser signed in to the account) makes its provisional link `active` and releases its held join requests.
- **Reclaim: the mailbox owner chooses.** A reclaim is a confirmation that sets a new password from a browser not signed in to the account, or a password reset of an `unverified` account. It confirms the email, moves the provisional link to `reclaim_pending` and every held join request to a pending owner choice; nothing becomes permanent, and no request becomes visible to a tutor, without an explicit keep. The owner then sees each item (the tutor's name, avatar and subjects) and keeps or drops it: keeping a link makes it `active` while the tutor is `active`, and is treated as a drop otherwise; dropping it applies the removal effects; keeping a request releases it; dropping it expires it silently. Items the owner never chooses are dropped 7 days after the reclaim. While `reclaim_pending`, the link grants the account nothing and shows the tutor no account.
- **Requests by code from unconfirmed accounts are held.** The tutor does not see them and they do not count toward the tutor's limit of 50 visible pending requests. At confirmation (or when the owner keeps one after a reclaim) a held request becomes visible when the tutor is `active` and has room, is suppressed while the tutor has blocked the student or the limit is full, and expires when the tutor is not `active`. Requests expire 14 days after they were created.
- **A card created by accepting a join request** takes as its display name a tutor-owned copy of the student's account name at accept time, which the tutor can edit, and has no email. Every way of linking revokes the card's other open link and accepts the student's waiting requests to that tutor.
- **Either party ends a relationship.** The tutor ends it from the student card and the student from My tutors ("Stop studying"), with the same effects; the `relationship.ended` email goes to the other party. A new invitation accepted by the same account, or a join request from it accepted by the tutor, reactivates it.
- **Relationship states and transitions** (replacing those of ADR-0005):

  | State | Meaning |
  |---|---|
  | `invited` | A student card exists; no account is linked |
  | `provisional` | Linked to an account whose email is not confirmed |
  | `reclaim_pending` | The mailbox owner reclaimed the account and has not yet kept or dropped this link |
  | `active` | Linked and working together |
  | `ended` | Ended by the tutor, by the student, or by a deletion; can be reactivated |
  | `discarded` | Withdrawn, merged, removed, discarded by the tutor's deletion, or kept only as history after the student's purge |

  | From | To | Cause |
  |---|---|---|
  | — | `invited` | Card created; a new card after a removal or a dropped link |
  | — | `active` | Join request accepted when the tutor has neither a relationship with the account nor an invited card with its confirmed email |
  | `invited` | `provisional` | Invitation accepted by a new account, or by a signed-in account whose email is not confirmed and which holds no provisional link |
  | `invited` | `active` | Invitation accepted by a signed-in confirmed account; join request accepted with the card's email |
  | `invited` | `discarded` | Withdrawn; merged into the student's existing relationship with the tutor; tutor deleted |
  | `provisional` | `active` | Email confirmed from the account's own browser |
  | `provisional` | `reclaim_pending` | Reclaim by the mailbox owner |
  | `provisional` | `discarded` with a new `invited` card | Removed by the tutor; the student stops studying; 7-day cleanup; the student account deleted or purged |
  | `provisional` | `discarded`, no new card | The tutor deletes their account |
  | `reclaim_pending` | `active` | The owner keeps it while the tutor is `active`; or the owner accepts, signed in, an invitation of the same tutor, which counts as a keep |
  | `reclaim_pending` | `discarded` with a new `invited` card | The owner drops it; the owner keeps it while the tutor is not `active`; 7 days pass without a choice; the tutor removes it; the student account is deleted or purged |
  | `reclaim_pending` | `discarded`, no new card | The tutor deletes their account |
  | `active` | `ended` | The tutor ends; the student ends; the tutor or the student deletes their account |
  | `ended` | `active` | New invitation accepted by the same account; join request from the same account accepted |
  | `ended` | `discarded` | Purge of the student (the row is kept for the tutor's history); purge of the tutor deletes the row |

## Alternatives considered

- **Keep the earlier model** with reserved invitations and pending codes: safe, but its extra paths and the empty self-signed-up account make joining harder to explain and to test.
- **Link only after the email is confirmed:** safer against a mistyped email, but it keeps the reservation mechanism and delays the student's first contact with the tutor.
- **On reclaim, discard every provisional link and held request** without asking: protects the mailbox owner, but drops the links a genuine student made before confirming from another device. **On reclaim, keep them all:** convenient, but makes permanent a tutor link that a squatter chose, and exposes the owner's email to that tutor.
- **Only the tutor ends a relationship:** simpler, but a student who stops studying would have to ask the tutor.

## Consequences

- The reserved invitation, the pending tutor code, the two invitation deliveries and the self-sign-up without a tutor leave the specifications.
- The data model gains the `provisional` and `reclaim_pending` states, a held state and an owner-choice flag for join requests, a reclaim deadline on the account, and an encrypted invitation token with its own key (`OVC_FIELD_ENCRYPTION_KEY`).
- Confirmation and password reset now return a link outcome, and a reclaim adds a choice page after sign-in.
- Every transaction that acts for a signed-in student locks the account row and re-checks the session under that lock, so a request authenticated before a reclaim cannot act after it.
- The tutor sees the name an unconfirmed account chose, and can remove it; a squatter reaches at most one tutor's one-to-one relationship until the 7-day cleanup or the owner's reclaim ([T-44](../threat-model.md#threats-and-controls)).

## Where it is applied

- [Accounts](../../knowledge/specs/accounts.md): [registration](../../knowledge/specs/accounts.md#registration-and-email-verification), [relationships](../../knowledge/specs/accounts.md#tutorstudent-relationships), [invitations](../../knowledge/specs/accounts.md#invitations), [tutor code and join requests](../../knowledge/specs/accounts.md#tutor-code-and-join-requests), [ending](../../knowledge/specs/accounts.md#ending-a-relationship), [account deletion](../../knowledge/specs/accounts.md#account-deletion)
- [API](../../knowledge/specs/api.md#students-and-groups), [data model](../../knowledge/specs/data-model.md#relationships-and-groups), [notifications](../../knowledge/specs/notifications.md), [observability](../../knowledge/specs/observability.md#component-coverage)
- [Navigation](../../knowledge/design/web/navigation.md), [pages](../../knowledge/design/web/pages.md), [states](../../knowledge/design/web/states.md#relationships-requests-and-groups), [pilot scenarios](../../knowledge/design/web/pilot-scenarios.md)
- [Functional requirements](../requirements/functional.md) (FR-1, FR-2), [threat model](../threat-model.md)
