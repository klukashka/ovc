# Accounts, relationships and groups specification

Behavior for FR-1, FR-2, FR-3 and FR-16. Decisions: [ADR-0005](../../docs/decisions/0005-tutor-centric-domain.md), [ADR-0014](../../docs/decisions/0014-students-join-through-their-tutor.md). Entities are in the [data model](data-model.md); routes and error codes are in the [API](api.md#accounts).

## Roles and accounts

- An account has one role, `tutor` or `student`, fixed at creation. The operator is not an account role and has no HTTP surface; operator actions exist only in the local `ovc-admin` command.
- **Account states:**
  - `unverified`: email not confirmed;
  - `active`;
  - `blocked` by the operator, remembering the state it was blocked from;
  - `deleted`: pending purge.
- **Routes an `unverified` account may use** are marked `unverified` in the [API](api.md#accounts):
  - sign-in, refresh, sign out and sign out everywhere;
  - confirm email and resend confirmation; password reset and change;
  - `GET /users/me`, `PATCH /users/me` (name, avatar, language, timezone, theme), email preferences, `GET /home`, notifications, avatar uploads;
  - account deletion;
  - for tutors also: tutor profile and settings, reading the tutor code, the library, and library and prompt-image uploads;
  - for students also: My tutors (`GET /tutors`) and ending a relationship there, entering a tutor code and the list of sent requests, and accepting a first invitation while signed in when the account holds no provisional relationship; from the slices that add them, the one-to-one lessons, the private chat and the student's own work of a provisional relationship, never a group's.

  Every other route answers `403 email_not_verified`.
- **Cleanup:** an account that stays unverified for 7 days is removed at once by the cleanup job, applying the [purge rules](data-model.md#purge) immediately; its provisional link gets the [removal effects](#provisional-links-and-reclaim), silently, so its tutor gets a new card, and its held requests are deleted with it. Its email is free again.
- **A student with no active relationship** sees "Enter your tutor's code" on Home, or "Request sent. Waiting for the tutor." while a request waits.
- **On every authenticated request and every WebSocket frame**, the server checks that:
  - the access token's session (`sid`) is not revoked;
  - the account is `active`, or `unverified` for the routes above.

  This is one indexed lookup per request.
- **Closing live connections:** revoking a session, or blocking or deleting an account (including through `ovc-admin`), publishes a revocation signal through the [broadcaster](realtime-and-video.md#broadcaster). The affected WebSocket connections are then closed with code `4401` within 5 seconds.

## Registration and email verification

- **Sign-up form:**
  - name, email, password (10 to 128 characters), language, and a consent checkbox for personal-data processing; the consent version and time are recorded;
  - the browser's IANA timezone is sent automatically and stored as the account timezone;
  - a student sign-up outside an invitation carries a tutor code, which is required.
- **Student sign-up** happens only through a tutor ([ADR-0014](../../docs/decisions/0014-students-join-through-their-tutor.md)): on the invitation page ([invitations](#invitations)), or with a tutor code. `role: student` without `tutor_code` is refused with `422 validation_failed` (`body.tutor_code`: `required`), and an unknown, regenerated or inactive code with `422 tutor_code_invalid`, which creates nothing. The checks run in this order: the per-IP request slot, the code, then validation and password hashing. For a valid code, a new and a taken email both hash the password and get the same `202`; a new account gets a [held join request](#tutor-code-and-join-requests) to the code's tutor, and a taken email changes nothing beyond the duplicate notice. The consent version is the constant `ConsentVersion` in `internal/domain`, changed in code when the privacy policy changes. A browser timezone that is not a known IANA name in canonical form (letters, digits, `_`, `+` and `-`, at most four `/`-separated segments, at most 64 bytes; no `.` segments; not `Local`, `Factory`, `localtime` or `posixrules`, and not under the `posix/` or `right/` trees) is stored as `UTC`, which the person can correct in settings. The server resolves names against the host's zone database, so on a case-insensitive development filesystem a wrongly cased name such as `europe/moscow` may still be accepted; production hosts are case-sensitive. Passwords are compared byte for byte, without Unicode normalisation, and their length is counted in characters.
- **Response:** `POST /auth/register` always answers `202 Accepted` with no session, so a new and an already-used email look identical. The client shows "Check your email"; the person can sign in at once, as an unverified account, or wait for the confirmation link.
- **Duplicate email:**
  - registering with an email that already has an account, including a `deleted` account before purge, does not reveal that fact;
  - the response is the same as a successful sign-up;
  - the existing account's owner receives at most one "someone tried to sign up with your email" message per day.
- **Confirmation tokens:**
  - the worker mints a 32-byte random token when it sends the confirmation email, and commits its hash before sending;
  - valid for 24 hours, single use;
  - each send invalidates earlier unused tokens of the same purpose.
- **Sending limits:**
  - confirmation and reset emails together: at most one per 60 seconds, five per hour and ten per day per account;
  - sign-up, password-reset and resend requests: at most 10 per hour per IP address, counted together; the eleventh answers `429` with `Retry-After`;
  - the per-account limits are counted per normalised email when a request is queued, for an unknown email as for a known one, so both do the same counting work: a sign-up counts as the first confirmation, and a sign-up over a limit (for example right after a reset request for the same email) still creates the account and answers `202`, only without queuing the confirmation, which the person can resend later; on the public routes (forgot, and resend with an email) a request over a per-account limit is silently dropped and answers `202` like any other, and only the signed-in resend answers `429`;
  - resend and forgot send nothing for an unknown email, resend sends nothing to a confirmed account, and forgot sends nothing to a blocked or deleted account.
- **Confirming an email** (`POST /auth/verify-email`):
  - from a browser signed in to that account, it activates the account, revokes every other session, makes its provisional relationship `active` and releases its held join requests;
  - from any other browser, the page asks the person to set a new password. Confirming then sets that password, activates the account, revokes every session, signs this browser in, and is a [reclaim](#provisional-links-and-reclaim). Whoever created a squatting account therefore loses it, because the mailbox owner chooses the password and decides about every link and request it made.
- **Tokens are bound to the account state:** confirming accepts only an `unverified` account and resetting only an `unverified` or `active` one; every other state, like an unknown, expired, used or superseded token, answers the same `422 token_invalid`. A token is consumed by one conditional update while the account row is locked, so the account cannot change state in between. Confirmation, reset and the password change of an active account discard every unused token of both purposes and every queued request that has not used a token, so an email queued before the change is dropped at dispatch. A password change of an unverified account keeps them: whoever signed up with someone else's email knows the password, and must not be able to cancel the mailbox owner's links. The worker mints a token in one transaction that locks the account row and checks again that the account is in a state the token applies to, that the request still exists, has not used a token and is the newest of its purpose.
- **Completing a password reset** also confirms an unverified email, because it proves access to the mailbox; a reset of an `unverified` account is a [reclaim](#provisional-links-and-reclaim). A reset of an `active` account changes no relationship or request, because such an account has no provisional link or held request.
- **The outcome:** confirmation and reset return one `outcome`, chosen by precedence: `choice_required` when any item (a link or a request) waits for the owner's choice after a reclaim; otherwise `linked` when a provisional relationship became `active`; otherwise `request_sent` when a held request was released, visible or suppressed alike; otherwise `code_invalid` when a held request expired at release because its tutor is no longer `active`; otherwise `null`. A reset answers only `choice_required` or `null`.
- **Sign-up through an invitation** is described under [invitations](#invitations).

## Sessions and passwords

- **Access token:** short-lived (15 minutes), carrying `sid`. The frontend keeps it in memory only, never in Web Storage or cookies.
- **Refresh token:**
  - rotating, valid for 30 days;
  - held in a cookie named `__Host-ovc_refresh` with `HttpOnly`, `Secure`, `SameSite=Strict` and `Path=/`;
  - reuse of a rotated refresh token revokes the session;
  - its value is `<session>.<generation>.<random>.<mac>`: the session identifier (16 bytes), the decimal generation (from 1, no sign or leading zero), 32 random bytes and an untruncated HMAC-SHA-256 over the session identifier and generation with a key derived by HKDF from `OVC_JWT_SECRET` (info `refresh-generation`), each binary part in unpadded base64url and parsed strictly;
  - the MAC is verified before the session is looked up, so a forged cookie costs no query and changes nothing; a cookie with a valid MAC and an older generation than the stored one is reuse and revokes the session; a newer generation, or the current generation with another random part, is refused without revoking; a refresh whose rotation loses a race is re-checked against the stored row by the same rules, so the losing request counts as reuse, which the browser's per-tab lock keeps from happening;
  - the session expires 30 days after sign-in; rotation never extends it;
  - a session row is deleted by the hourly `sessions.cleanup` job once it has been expired or revoked for more than a day, after which its cookie is refused as unknown, as a revoked one is; email requests and their tokens are deleted 7 days after the request, and a token never outlives its request.
- **Refresh route checks:**
  - it accepts a request only when its `Origin` header equals the configured OVC origin exactly; a same-site subdomain such as the conferencing host does not qualify;
  - a request without a refresh cookie gets `204` with no token, which the client treats as "signed out" rather than as an error;
  - it is the only route authenticated by a cookie;
  - the conferencing origin is never in the CORS allowlist.
- **Sign out** revokes the current session; **sign out everywhere** revokes all of the user's sessions.
- **Throttling:** more than 5 failed sign-ins for one email within 15 minutes, or more than 20 from one IP address (an IPv6 address counts by its /64), returns `429` with `Retry-After`. The response never says whether the email exists. Throttle counters are rows in PostgreSQL keyed by purpose and subject, with fixed windows; each attempt reserves its per-IP slot and then, only within that limit, its per-email slot, atomically and before any password is hashed; an attempt refused by either limit returns what it reserved at once, and a successful sign-in returns both, so the counters hold failed attempts (a correct password for a blocked account counts as one) and attempts in flight. The email in a subject is trimmed and lower-cased with Unicode case mapping. A rehash after sign-in replaces the stored hash only if it has not changed meanwhile, so it never undoes a concurrent reset or change; when another sign-in replaced it first, the password is verified again against the stored hash. The session starts in a transaction that locks the user row and requires the verified hash and an active or unverified state to be unchanged, so a sign-in that races a reset or a block creates no session; a block found at that point answers `account_blocked`. This guarantee needs the reset, block and deletion flows to change the user row and revoke its sessions in one transaction that holds the same row lock. When hashing is busy or fails, or the request is cancelled while a password is being verified, both reserved slots are returned. The client address is the TCP peer, or, when the peer is inside `OVC_TRUSTED_PROXY_CIDRS`, the right-most `X-Forwarded-For` entry outside those ranges; an unparsable entry falls back to the peer.
- **Sign-in answers:** an unknown email, a wrong password and a deleted account all answer `401 invalid_credentials`, and an unknown email still costs one password verification against a placeholder hash; a blocked account answers `403 account_blocked` only after the correct password. A password hashed with older parameters is rehashed on a successful sign-in. Password hashing runs at most `GOMAXPROCS` hashes at once and answers `503 service_unavailable` after waiting 5 seconds for a slot.
- **Password reset:**
  - requesting a reset always returns the same response, within the sending limits above;
  - the worker mints the emailed token (32 bytes, hashed and committed before sending, valid for 1 hour, single use);
  - a successful reset revokes all sessions.
- **Password change** requires the current password and revokes every other session. Wrong current passwords are counted per account in their own counter with the same numbers as the sign-in failure limit (5 in 15 minutes), separate from the sign-in counters, and an attempt refused by it returns its slot; the sixth attempt answers `429` with `Retry-After`, and a correct password returns its slot.
- **Email change** is not in the MVP.

## Tutor–student relationships

Students join through their tutor ([ADR-0014](../../docs/decisions/0014-students-join-through-their-tutor.md)).

- **States:**

  | State | Meaning |
  |---|---|
  | `invited` | A student card exists; no student account is linked |
  | `provisional` | Linked to an account whose email is not confirmed ([provisional links](#provisional-links-and-reclaim)) |
  | `reclaim_pending` | The mailbox owner reclaimed the account and has not yet kept or dropped this link ([reclaim](#provisional-links-and-reclaim)) |
  | `active` | Linked and working together |
  | `ended` | Ended by the tutor, by the student, or by a deletion (`ended_by`: `tutor`, `student`, `deletion`); a new invitation or request can reactivate it |
  | `discarded` | Withdrawn, merged into another card of the same student, removed while provisional or reclaim-pending, discarded by the tutor's deletion, or kept only as history after the student's purge (`discard_reason`: `withdrawn`, `merged`, `removed`, `tutor_deleted`, `student_purged`) |

- **Transitions:**

  | From | To | Cause |
  |---|---|---|
  | — | `invited` | Card created; the new card that a removal or a dropped link gives the tutor |
  | — | `active` | Join request accepted when the tutor has no relationship with the account and no `invited` card with its confirmed email: a new card whose display name is a copy of the student's account name at accept time, without an email |
  | `invited` | `provisional` | Invitation accepted by a new account, or by a signed-in account whose email is not confirmed and which holds no provisional relationship |
  | `invited` | `active` | Invitation accepted by a signed-in confirmed account; join request accepted with the card's email |
  | `invited` | `discarded` | Withdrawn; merged; tutor deleted |
  | `provisional` | `active` | Email confirmed from the account's own browser |
  | `provisional` | `reclaim_pending` | Reclaim by the mailbox owner |
  | `provisional` | `discarded` (`removed`) with a new `invited` card | The tutor removes it; the student stops studying; the 7-day cleanup; the student account is deleted (and so purged at once) |
  | `provisional` | `discarded` (`tutor_deleted`), no new card | The tutor deletes their account |
  | `reclaim_pending` | `active` | The owner keeps it while the tutor is `active`; or the owner accepts, signed in, an invitation of the same tutor, which counts as a keep and consumes that invitation |
  | `reclaim_pending` | `discarded` (`removed`) with a new `invited` card | The owner drops it; the owner keeps it while the tutor is not `active` (treated as a drop); `reclaim.expire` 7 days after the reclaim; the tutor removes it (`POST /students/{id}/remove` accepts `reclaim_pending` as well as `provisional`, serialised on the student's user row); the student account is deleted or purged |
  | `reclaim_pending` | `discarded` (`tutor_deleted`), no new card | The tutor deletes their account; that tutor's held requests of the account expire |
  | `active` | `ended` | The tutor ends; the student ends; the tutor or the student deletes their account |
  | `ended` | `active` | New invitation accepted by the same student account; join request from the same account accepted |
  | `ended` | `discarded` (`student_purged`) | Purge of the student (the row is kept for the tutor's history, with its `ended_at` and `ended_by`); purge of the tutor deletes the row |

- **Linking:** a relationship belongs to exactly one tutor and, once linked, exactly one student account forever. There is at most one non-discarded relationship per tutor and student account, at most one non-discarded card per tutor and email, compared case-insensitively, and at most one `provisional` relationship per student account, which is why an unconfirmed account cannot hold a second tutor's link. Linking a card without an email never copies the account's email onto the card, so a removed or dropped link leaves no address on the new card. Whenever a student links to a tutor by any path (an invitation, an accepted join request, a reactivation, the owner's keep after a reclaim), every held or pending join request from that student to that tutor becomes `accepted`, and the card's open invitation, if any other than the one being used, is revoked (`linked`) and its ciphertext cleared.
- **The student card** is the tutor's view of the relationship:
  - display name, set by the tutor and private to them; for a card that accepting a join request creates, it starts as a tutor-owned copy of the student's account name taken at accept time, which the tutor can edit and which never follows later changes of the account name;
  - optional email;
  - state, and the linked account (name, email, avatar and whether its email is confirmed), shown neither while the card is `invited` nor while it is `reclaim_pending`, so the tutor never sees a reclaimed account before its owner keeps the link;
  - the open invitation link with its expiry and status (open, expired, or revoked after repeated wrong emails);
  - groups;
  - lesson history (slice 5);
  - issues and scores (slice 9);
  - private notes, never returned to the student.
- **Names the student sees:** the student sees their own account name everywhere. Other participants (group members, the conferencing room) also see the student's account name, never the tutor's private display name.
- **Tutor colour:** each relationship has a colour that labels this tutor's items in the student's views. It is chosen from the fixed palette at linking time: the first palette key that none of the student's other non-discarded relationships uses, otherwise the least used one, in palette order.
- **Isolation:** a tutor lists, reads and acts only on their own relationships, and a student only on their own. Any route that takes a student, relationship, request or group identifier resolves it within the caller's scope first and returns `not_found` otherwise, with the same body as for a random identifier; this resolution runs before any role-specific or state refusal (`409`), so a state refusal never confirms that an out-of-scope identifier exists. A `discarded` card is outside scope, except that repeating the withdrawal of a card this tutor withdrew answers `204`.
- **Listing:** the tutor's list of students is ordered by creation time, newest first, with keyset pagination on creation time and identifier. The `state=active` filter includes `provisional` and `reclaim_pending` cards. The search `q` is a case-insensitive substring of the display name or the card email, with `%`, `_` and `\` taken literally.
- **Duplicate cards:** creating a card with an email that one of the tutor's non-discarded cards already has is refused with `student_exists`, and the response names the existing card.
- **Card limits:** a tutor creates at most 50 cards and changes card emails at most 50 times per day (`429 rate_limited`). The card that a removal or a dropped link creates does not count.
- **Withdrawing a card:** the tutor can withdraw an `invited` card; it becomes `discarded` (`withdrawn`) and its open invitation is revoked. Repeating the withdrawal answers `204`; withdrawing a card that is `provisional`, `reclaim_pending`, `active` or `ended` is refused with `relationship_not_active`; any other `discarded` card is outside scope and answers `not_found`.
- **Changing the email of an `invited` card** revokes its open invitation and issues a fresh link in the same transaction. The email of a card in any other state cannot be changed (`not_editable`).
- **Ending:** ending an `ended` relationship returns its current state (idempotent); ending an `invited`, `provisional` or `reclaim_pending` card is refused with `relationship_not_active` (a provisional student is removed instead); a `discarded` card is outside scope and answers `not_found`.
- **Scheduling and issuing** require an `active` relationship, so invited cards never get lessons or work; from slice 5 one-to-one lessons may also be scheduled for a `provisional` relationship, which passes the [join rule](schedule.md#join-rule) for one-to-one lessons only, and from slice 9 work may also be issued to it directly, never through a group.

## Invitations

- **Creating:** a confirmed tutor creates a student card with a display name and an optional email. The card starts as `invited` and gets its invitation link at once.
- **One open invitation per card**, the same link whether the tutor copies it or OVC emails it:
  - the token is 32 random bytes in base64url, valid for 14 days and single use;
  - it is stored as a SHA-256 hash for lookup and as AES-256-GCM ciphertext under `OVC_FIELD_ENCRYPTION_KEY` (a 12-byte random nonce; as additional data the label `ovc.invitation-token.v1` followed by the invitation identifier, as the [data model](data-model.md) specifies), so the card can show the link again and the worker can email it;
  - the ciphertext is cleared when the invitation is used or revoked, and the hourly `invitations.expire` job clears it once the invitation expires;
  - the key is not rotated in the MVP: a new key makes open links unreadable, and the tutor regenerates them.
- **Regenerating** an `invited` or `ended` card revokes its open invitation and issues a new one; a tutor regenerates at most 50 links per UTC day (`429 rate_limited`).
- **Sending by email** (`POST /students/{id}/invitation/send`) emails the card's open link to the card's email, and refuses a card without an email. An open link that has expired is revoked (`expired_on_send`) and a fresh one is sent; a card with no open link (an `ended` card after the ending revoked it, or a link revoked after repeated wrong emails) gets a fresh one, which counts against the regeneration limit and, once that is used up, is refused with the same `429 rate_limited` as a regeneration; refreshing an expired link does not count against it. Each queued send counts on the invitation, and the count is part of the email's idempotency key, so an earlier send of the same link still waiting at dispatch becomes obsolete. The checks run in this order: scope, state, the card email, the tutor's budget, then the regeneration limit when a fresh link is needed; a refused send changes nothing.
- **Sending limits:**
  - a tutor in their first 7 days after sign-up may send 10 invitation emails per UTC day, and 50 per UTC day after that; only this budget, and the regeneration limit when a send needs a fresh link, answer `429`, never anything about the recipient;
  - at most 3 invitation emails per recipient address per UTC day across all tutors; over that cap, and for an address whose account is `blocked` or `deleted`, the route answers `202` like any other send and the email is dropped; a dropped send still counts toward the tutor's budget, still refreshes an expired link, and still issues a fresh link for a card with no open link, counted as a regeneration, so the card and the counters look exactly as after a queued send; the card never shows delivery status.
- **The invitation email** is written in the language of the account that holds the card's email, otherwise in the tutor's language. The tutor's name appears only in the body, as escaped plain text, never as a link or in the subject.
- **An active tutor only:** a link of a tutor whose account is not `active` (unverified, blocked or deleted) is invalid.
- **An `ended` card** can be given a new invitation. It is **bound to the card's linked student account**: only that account, signed in, can accept it; any other account, and every sign-up on its page, is refused with `invitation_email_mismatch`.
- **The invitation page** (`GET /invitations/{token}`):
  - shows the tutor's public profile (name, avatar, subjects) and never the invitee's email;
  - an unknown, expired, used, revoked or malformed token, and the link of a tutor who is not `active`, all get one identical `404 not_found`;
  - every response that carries an invitation link or accepts one is sent with `Cache-Control: no-store`;
  - follows the [token page rules](../design/web/navigation.md#token-pages); the token stays in the page's memory and never travels in `next` or any other address.
- **Accepting with a new account** (`POST /invitations/{token}/accept` without a bearer token), in this order:
  1. the per-IP request slot, shared with sign-up, reset and resend;
  2. the token is usable and its tutor `active`, otherwise `409 invitation_invalid`;
  3. validation of the sign-up fields;
  4. the email binding: a card email must equal the sign-up email, normalised as for sign-up, and the link of an `ended` card is always a mismatch here; a mismatch is counted and refused with `409 invitation_email_mismatch` before any password is hashed;
  5. the password is hashed, whether or not the email is taken;
  6. for a free email, one transaction creates the account `unverified`, consumes the invitation, makes the relationship `provisional`, chooses the colour and queues the confirmation; for a taken email, nothing changes, the invitation stays usable and the existing owner gets the duplicate sign-up notice.

  Both cases answer the same `202 {}` with no session. The page then signs in with the password just chosen; when that fails it shows "Check your email, or sign in to accept", never a wrong-password error.
- **Accepting as a signed-in account** (`POST /invitations/{token}/accept` with a bearer token and the body `{}`; sign-up fields sent with a bearer are refused with `422 validation_failed`, `not_allowed`), in this order:
  1. a tutor account is refused with `409 role_mismatch`, and an account whose email is not confirmed and which already holds a provisional relationship with `403 email_not_verified`, both before any counter;
  2. the token is usable and its tutor `active`, otherwise `409 invitation_invalid`;
  3. the binding: the card email equals the account's email, or the `ended` card's bound account is the caller; a mismatch is counted and refused with `409 invitation_email_mismatch`;
  4. linking, below. An account whose email is not confirmed links `provisional`; a confirmed account links `active`.

  Until signed-in acceptance lands (S8b in the [slice 3 plan](../plans/slice-3.md)), a bearer with the body `{}` answers `403 forbidden` before any token lookup, counter or hash, whatever the role.
- **The mismatch limit:** every `invitation_email_mismatch` is counted on the invitation in one atomic update, and the fifth revokes the link; the card then shows "Link revoked after repeated wrong emails", and later attempts get the ordinary invalid answer. A link holder can therefore revoke a leaked link on purpose.
- **Consumption is atomic:** a conditional update that succeeds only while the invitation is unused, unrevoked and unexpired, and that clears its ciphertext; a token links only while its card is `invited` or `ended`. Concurrent acceptances of one link, or a concurrent regeneration, lose with `invitation_invalid`; concurrent acceptances by one account are serialised by that account's row lock, so neither uniqueness rule of linking ever surfaces as a server error.
- **Linking:**
  - links the account to the card, sets the relationship `provisional` or `active` as above, chooses the colour, and consumes the invitation used;
  - if the student already has a non-discarded relationship with this tutor through another card, that existing relationship is kept (reactivated if `ended`, and made `active` if `reclaim_pending`, which counts as the owner's keep), its own open invitation is revoked (`linked`), and the invited card becomes `discarded` (`merged`);
  - every held or pending join request from the same student to the same tutor becomes `accepted`;
  - the tutor's in-app notification arrives with slice 4.

## Provisional links and reclaim

- **A provisional link** reaches only the one-to-one relationship: it is listed on the student's My tutors, it cannot be added to a group (`409 relationship_not_active`), and no notification email about it is sent to the unconfirmed address. From slice 5 it reaches the relationship's one-to-one lessons, and from slice 6 its private chat, never a group's. The card shows "Email not confirmed".
- **Removal** (`POST /students/{id}/remove`) is allowed only while the card is `provisional` or `reclaim_pending`, and answers `409 relationship_not_provisional` otherwise. In one transaction, which locks the student's user row and then the relationship and re-checks its state:
  - the relationship becomes `discarded` (`removed`), its open invitation is revoked, and its notes and email are cleared;
  - a new `invited` card with the same display name, email and notes and a fresh link is created and returned as `new_card`; it does not count toward the card limit;
  - nothing is sent: removal is silent; the removed account no longer lists the tutor, the discarded row is invisible to the tutor, and it is never reactivated, even when the same account later accepts the new link;
  - from slice 5 the link's future one-to-one lessons are cancelled without an email.

  Repeating a removal names a discarded card and answers `not_found`; removal is not among the retry-safe routes. The same removal effects apply when the student stops studying on a provisional link, when the 7-day cleanup removes the account, when a deleted unconfirmed account is purged at once, and when a reclaim's link is dropped. A removal racing a confirmation or a reclaim locks the same user row and checks the state, so exactly one of them wins.
- **Confirmation by the account's own browser** (a browser signed in to the account) makes its provisional relationship `active` and releases its held join requests.
- **Reclaim** is a confirmation that sets a new password from a browser not signed in to the account, or a password reset of an `unverified` account. Its transaction, which already holds the account's row, confirms the email, moves the provisional relationship to `reclaim_pending`, marks every held join request as waiting for the owner's choice, and sets the account's reclaim deadline to 7 days later. Nothing becomes permanent, and no request becomes visible to a tutor, without an explicit keep. The outcome is `choice_required`.
- **While `reclaim_pending`** the link grants the account nothing: `GET /tutors` leaves it out, `POST /tutors/{id}/end` answers `not_found`, no group can take it, and the card shows no account and the state "Waiting for the student". `GET /users/me` reports `reclaim_pending: true` while any link or request waits for the choice, derived from those rows.
- **The choice:**
  - `GET /users/me/reclaim` lists each waiting link and request with its tutor's name, avatar and subjects, and the deadline; the choice page also shows the account's current name and avatar with a link to edit them before anything is kept;
  - `POST /users/me/reclaim` names every waiting item exactly once, as kept or dropped. An identifier that is not one of the caller's waiting items answers `not_found` (another account's identifier answers like a random one), checked before `422 validation_failed` for an item that is missing or named twice; on either the client fetches the list again. When nothing waits (a repeat, or after the deadline), it answers `200` with `outcome: null` and changes nothing;
  - keeping a link makes it `active` while its tutor is `active`, and is treated as a drop otherwise; dropping a link applies the removal effects, so its tutor gets a new card; keeping a request releases it as at confirmation; dropping a request expires it silently;
  - the account's deadline is cleared once nothing waits.
- **Signed-in acceptance** of an invitation of the same tutor while the link is `reclaim_pending` counts as a keep and consumes that invitation.
- **Code entry during a pending choice:** entering the code of a tutor whose request waits for the choice counts as a keep of that request; entering the code of a tutor whose link is `reclaim_pending` answers `409 already_linked` and changes nothing.
- **Deadline:** the hourly `reclaim.expire` job takes up to 100 accounts per run through a partial index on the deadline; for each it locks the account's row, re-checks the deadline and drops every item still waiting, whatever the account's state (links with the removal effects, requests expired), then clears the deadline; it also clears a deadline with nothing left to choose. A deleted account never reaches it, because deletion already dropped its items.
- **The deadline is written only by the account's own transactions:** for example the reclaim, the owner's choice, a signed-in acceptance that keeps the last waiting link, a code entry that keeps the last waiting request, the account's deletion, and the job. The list is not exhaustive, because whether a choice is due is derived from the waiting rows and `reclaim.expire` clears a deadline lazily once nothing waits. Tutor paths (removal, tutor deletion) never write the deadline; tutor deletion never locks a student's user row at all.
- **Restoring or unblocking** an account into `unverified` restarts its 7-day clock, including for an account that holds a provisional link; restoring only ever returns an account to `active`, so only unblocking is checked against linked accounts.

## Tutor code and join requests

- **The code:**
  - each tutor has one code of 8 characters from `23456789ABCDEFGHJKMNPQRSTUVWXYZ` (no 0, 1, I, L or O), shown as `XXXX-XXXX` and matched case-insensitively with spaces and hyphens ignored;
  - minted at tutor sign-up, retried on a collision;
  - shared openly by design, shown in Teaching settings and in the join-request panel on Today;
  - can be regenerated at most 10 times per tutor per UTC calendar day (`429 rate_limited`; a refused attempt is not counted), and the old code stops working immediately;
  - inactive while the tutor is unverified (`GET /tutor/code` reports `active: false`), and invalid while the tutor's account is not `active`.
- **Entering a code** (`POST /join-requests`, also open to a student whose email is not confirmed; at sign-up through `POST /auth/register`):
  - a tutor account entering a code is refused with `role_mismatch`;
  - an unknown, regenerated or inactive code is refused with `tutor_code_invalid`;
  - a student who already has a `provisional`, `reclaim_pending` or `active` relationship with that tutor is refused with `already_linked`;
  - a student who already has a held or pending request to that tutor gets the existing request;
  - otherwise a request is created; created, existing, held and suppressed requests all get the same `200` answer with the student's view of the request.
- **Held requests:** a request from an account whose email is not confirmed is `held`: the tutor does not see it, it does not count toward the tutor's limit, and its entry never answers `tutor_unavailable`. A regenerated code does not affect a request already held.
- **Visible and suppressed requests:** a request from a confirmed account is a visible `pending` request when the tutor has fewer than 50 visible pending requests, and is refused with `429 tutor_unavailable` when the limit is full. When the tutor has blocked the student, the request is suppressed instead: the answer is identical to a created request, the student's request list shows it as sent until it expires, and nothing reaches the tutor.
- **Release:** at confirmation from the account's own browser, and when the owner keeps a request after a reclaim, a held request becomes visible `pending` when the tutor is `active` and has fewer than 50 visible pending requests; it is suppressed when the tutor has blocked the student or the limit is full; it expires when the tutor is not `active`.
- **Un-suppression:** a request suppressed for the limit becomes visible, oldest first, in the transaction that frees a slot (an accept, a decline, an expiry, a student's deletion, or an invitation acceptance, signed in or through a merge, that turns a visible pending request into `accepted`). A request suppressed for a block expires when the tutor unblocks the student, so the student can send a fresh one.
- **Limits:** code entries are throttled to 10 per student per hour, held entries included (`429 rate_limited`); a sign-up with a code uses the per-IP request counter; each tutor sees at most 50 pending requests.
- **Accept** (idempotent when already accepted):
  - reactivates the tutor's existing relationship with that student account if one exists (`ended`);
  - otherwise links a non-discarded `invited` card whose email equals the student's confirmed email, if one exists;
  - otherwise creates the relationship with a new card whose display name is a tutor-owned copy of the student's account name at accept time, editable by the tutor, and whose email stays empty;
  - revokes the open invitation of the linked or reactivated card (`linked`) and clears its ciphertext, so that link can no longer be used;
  - and answers with the card. Only a `pending` request can be decided; a decided or expired one is refused with `not_editable`.
- **Decline:** the student learns only that the request was not accepted. "Decline and block" also stops future requests from that student: they are suppressed and shown to the student as sent until they expire. The tutor manages blocked students in Teaching settings and can unblock them.
- **Expiry:** held and pending requests expire 14 days after they were created, on the injected clock, through the hourly `join_requests.expire` job (200 per run). A declined or expired request does not block a new one, unless the student is blocked.
- **The student's list** (`GET /join-requests/sent`) shows each request's state as `pending` (held, visible and suppressed alike), `accepted`, `not_accepted` (declined) or `expired`, with its times only, never the tutor's decision details.

## Ending a relationship

Either party ends a relationship, after confirming in a dialog: the tutor from the student card (`POST /students/{id}/end`), the student from My tutors with "Stop studying" (`POST /tutors/{relationshipId}/end`). The effects are the same. In one transaction:

- the relationship becomes `ended` with `ended_at` and `ended_by` (`tutor` or `student`), the student is removed from all of the tutor's groups, and the card's open invitations are revoked;
- **lessons** (slice 5):
  - one-to-one series of the relationship end with an end date of today in the series timezone, a count being converted into that end date;
  - future one-to-one lessons are cancelled;
  - the student is removed from the audience of future group lessons;
  - the student can no longer join any of the tutor's lessons, including live ones ([join rule](schedule.md#join-rule));
- **work** (slice 9):
  - issued work the student has not submitted (`not_started`, `draft`, `returned`) becomes `closed`;
  - `submitted` work stays checkable by the tutor but can no longer be returned;
  - checked work stays readable by both;
- **chat** (slice 6): the private chat becomes read-only for both;
- **recordings:** recordings of lessons whose roster includes the student stay readable by the student until they expire;
- **notifications:** one `relationship.ended` email to the other party, the student when the tutor ends and the tutor when the student ends, through the outbox, with the key `(relationship.ended, relationship, ended_at)`; it is never sent to an unconfirmed address; from slice 5 it lists the cancelled lessons, and the in-app notification arrives with slice 4. The email to the tutor names the student by the tutor's own card display name, escaped, never by the current account name; for a card created from a join request that is the tutor-owned copy taken at accept time.

Ending an `ended` relationship again returns it unchanged. A student's "Stop studying" on a `provisional` link is a silent [removal](#provisional-links-and-reclaim) answering `204`, so a provisional link never becomes `ended`; on any other of the student's own identifiers that is not `active` or `ended` (invited, discarded or `reclaim_pending`) it answers `not_found`.

**Reactivation:** a new invitation accepted by the same account, or an accepted join request from it, reactivates the same relationship with its history; either party can reactivate it only through these paths. It makes the private chat writable again. It restores no group memberships, no series and no closed work.

## Groups

- **A group** belongs to one tutor and has:
  - a name (1–60 characters);
  - a colour;
  - members who are students with `active` relationships to that tutor (at most 30); a `provisional` or `reclaim_pending` student can never be a member.
- **Limits:** a tutor has at most 200 groups, archived ones included; the 201st is refused with `422 validation_failed`, counted under the tutor's profile lock, so the group list stays unpaginated and bounded.
- **Members:** a member outside the tutor's relationships answers `not_found`, and one whose relationship is not `active` is refused with `409 relationship_not_active`, naming the relationships.
- **Adding a member** gives them:
  - the group chat, including its history;
  - future group lessons;
  - the right to join a group lesson that is already live, which also adds them to that lesson's roster.

  It does not give them work issued to the group earlier ([issuing](homework.md#issuing)). Adding a member to an archived group is refused with `audience_not_active`.
- **Removing a member:**
  - removes them from future group lessons and the group chat, and they can no longer join a live group lesson;
  - they keep their issued work, which they may still submit while their relationship with the tutor is active.
- **Archiving** (idempotent):
  - ends the group's series and cancels its future lessons;
  - makes its chat read-only;
  - refuses new issues to the group (`audience_not_active`);
  - from slice 5, members get one aggregated `group.archived` notification listing the cancelled lessons.
- **Unarchiving** (idempotent) restores issuing and chat, but no series and no cancelled lessons.

## Account deletion

- **Starting deletion:** it requires the current password and an explicit confirmation. The account becomes `deleted` at once: sessions are revoked, sockets are closed and sign-in is refused. A purge job runs 30 days later.
- **From deletion until purge**, the account's messages show their sender as "Deleted user" and keep their bodies.
- **Student deletion:**
  - active relationships end (`ended_by` `deletion`), with the effects above, without sending `relationship.ended`;
  - provisional and reclaim-pending links get the removal effects, so each tutor gets a new card;
  - held and pending join requests expire;
  - the reclaim deadline is cleared.
- **Tutor deletion:**
  - active relationships end (`ended_by` `deletion`), invited cards become `discarded`, and provisional and reclaim-pending links become `discarded` (`tutor_deleted`) without a new card;
  - every open invitation of the tutor is revoked (`tutor_deleted`), including the regenerated links of ended cards;
  - held and pending join requests expire;
  - groups are archived, series end and future lessons are cancelled;
  - every student whose relationship was `active` at deletion and whose email is confirmed receives one `tutor.deleted` email, which from slice 5 lists the cancelled lessons, instead of separate ended or cancellation emails; students of discarded provisional links get no email;
  - the deletion locks only the tutor's own user row and profile, never a student's user row;
  - for 30 days, students keep read-only access to their own submissions, results and recordings from that tutor.
- **An unverified account** is purged at once when its owner deletes it, so its email is free again immediately.
- **In slice 2** deletion and purge covered the account's own data (sessions, email requests and tokens, the tutor profile, uploads and avatar references, the user row); slice 3 adds the effects on relationships, cards, invitations, join requests and groups, and the lesson and message effects arrive with the slices that add those tables. The hourly `accounts.purge` job purges accounts deleted 30 days ago and the hourly `accounts.cleanup_unverified` job removes accounts unverified for 7 days, in batches of 100, each account in its own transaction; an account whose purge fails is logged as `account.purge_failed` and skipped for that run, so it never holds up the others.
- **Purge** is defined per table in [the purge rules](data-model.md#purge). The purge job re-checks the account's state under a row lock, so a restore never races it.
  - Purging a student erases their submissions, answers, uploaded files and message bodies (shown as "message removed"), including the files attached to those messages. Every relationship row of the student, discarded ones included, loses its `student_id` and its card email; the non-discarded ones become `discarded` (`student_purged`), keeping `ended_at` and `ended_by`, and stay in the tutor's history under the card's display name. OVC keeps no copy of the student's name, except that the display name of a card created by accepting a join request began as a copy of the account name at accept time; it is the tutor's own data and is deleted with the tutor's purge.
  - Purging a tutor deletes the tutor's library, lessons, recordings, issues, chats, files, student cards, notes, the emails stored on cards, invitations, join requests, blocked students and groups, in batches of 500 rows per transaction, each re-checking under the user row lock that the account is still `deleted`.
  - Recordings in which a purged student took part are kept until their own expiry (at most 365 days).
- **Backups:** database backups are retained for 30 days, so purged data leaves backups within 30 days after purge ([ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md)).
- **After purge**, deletion is irreversible. Before purge, only the operator can restore an account; restoring does not reactivate ended relationships.

## Locks and the session re-check

- **Lock order:** a transaction that needs user rows locks all of them up front, in one statement that locks the rows in identifier order (`FOR NO KEY UPDATE`), re-checks their states, and never locks another user row later. It then locks every tutor profile row it needs (for the pending-request limit, code regeneration, un-suppression and the group limit) up front, in identifier order, and never another profile later; release at confirmation, student deletion and un-suppression can touch several tutors. Then come relationships in identifier order, then invitations, then join requests, then groups. Every path that frees or fills a pending-request slot follows this order, including the jobs: decline, accept, `join_requests.expire`, student deletion, release at confirmation, un-suppression and invitation acceptance (signed in or through a merge, which can accept a visible pending request) lock the student's user row and then the tutor profiles before touching relationships and requests; `reclaim.expire` locks the user row first, as the owner's choice does, so the two serialise. Tutor deletion is the exception that locks no student's user row: it locks only the tutor's own user row and profile, then relationships, invitations, requests and groups. Its discard and revoke steps are statements issued after those locks are held, so they also see the cards and invitations that a concurrent owner's drop, `reclaim.expire` or removal committed while it waited; no invited card or open invitation of the deleted tutor remains. Tutor removal locks the student's user row and then the relationship. So no two transactions wait on each other's rows in a cycle.
- **Session re-check under the lock:** a request is authenticated once at its start, while confirmation, reset and reclaim revoke sessions inside their own transaction, so a request authenticated before a reclaim could wait on the user row and act after it. One rule therefore applies to every write open to an account whose email is not confirmed, and to every transaction that acts for a signed-in student: all of `PATCH /users/me`, an avatar upload's creation and settling, `PUT /users/me/email-preferences` once it lands in slice 4, an unverified tutor's writes to the tutor profile and settings, the library, and library and prompt-image uploads as they land, signed-in acceptance, code entry, a student's ending of a relationship, confirmation from the account's own browser, the reclaim choice, and any later write the same audience opens. Password change and account deletion are already guarded, because they compare the verified password hash under the same row lock. Each locks the user row first, then reads the session in a separate statement, so it sees a revocation committed while it waited, and re-checks that the session is not revoked and that the account is still in the state the request was authorised for. Otherwise it answers `401 unauthenticated` and changes nothing. A squatter's request, edit or upload authenticated before a reclaim therefore cannot land after it. As implemented so far (S8a in the [slice 3 plan](../plans/slice-3.md)), only confirmation from the account's own browser re-checks the session; `PATCH /users/me`, the avatar upload and its settling gain it with the reclaim choice (S8c), and the other writes with the changes that add them.

## Administration command

`ovc-admin` runs on a production host against the production database with operator credentials. It has no HTTP surface. Commands exit non-zero with a message for an unknown email, a wrong role or an impossible state change. The subcommands:

- `ovc-admin users list [--role tutor|student] [--email <substring>] [--state <state>]` prints identifier, role, state, email and creation time.
- `ovc-admin users block <email>` sets the account `blocked`, remembers its previous state, revokes its sessions and publishes the revocation signal. A `deleted` account cannot be blocked.
- `ovc-admin users unblock <email>` returns the account to the state it was blocked from.
- `ovc-admin users restore <email>` returns a `deleted` account before purge to `active`; an unverified account is never `deleted`, because deleting it purges it at once.
- `ovc-admin tutors set-quota <email> --files <bytes> --recordings <bytes>` changes a tutor's quotas; it refuses student accounts.

Every command writes an audit diagnostic event with the operator's system user name.

As implemented in slice 2:

- **Settings:** the command needs only `OVC_DATABASE_URL` and the logging settings, refuses to run while migrations are pending, and writes its diagnostics to standard error; results go to standard output.
- **Output:** `users list` prints one tab-separated line per account (identifier, role, state, email, creation time in UTC), at most 1000, oldest first; the `--email` filter matches a case-insensitive substring with `%`, `_` and `\` taken literally. The other commands print `done <identifier>`.
- **Exit codes:** `0` on success, `1` for a refusal (`refused: no account has this email`, `the account's state does not allow this change`, `the account does not have the role this command needs`, `a quota must be between 0 and 2^50 bytes`) or a failure, and `2` with the usage text for an unknown command or malformed arguments. Messages never repeat the email.
- **Block** also discards the account's unused email tokens and queued email requests; the revocation signal that closes live connections arrives with the broadcaster in slice 4.
- **Unblock** into `unverified` restarts the 7-day clock, so the cleanup job does not remove the account at once. Blocked accounts are never removed by the cleanup or purge jobs.
- **Failures before the command runs** (an unknown system user, an unreachable database, pending migrations) are audited as `admin.command` with outcome `failed`, at INFO like every other outcome, and a refusal by the test-control database checks, or a database URL the driver cannot parse, prints its reason without any part of the URL. A settings refusal is printed as `configuration: <problem>` and is not audited, because logging is configured from those settings. Under `OVC_CLOCK=test` the command uses the shared test clock, like the API and the worker.
- **Email lookup** is case-insensitive and trims spaces; every change locks the user row.
