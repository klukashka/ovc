# Accounts, relationships and groups specification

Behavior for FR-1, FR-2, FR-3 and FR-16. Decisions: [ADR-0005](../../docs/decisions/0005-tutor-centric-domain.md). Entities are in the [data model](data-model.md); routes and error codes are in the [API](api.md#accounts).

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
  - for tutors also: tutor profile and settings, the library, and library and prompt-image uploads.

  Every other route answers `403 email_not_verified`.
- **Cleanup:** an account that stays unverified for 7 days is removed at once by the cleanup job, applying the [purge rules](data-model.md#purge) immediately and releasing any reserved invitation. Its email is free again.
- **A student with no active relationship** sees the "enter a tutor code" state on Home.
- **On every authenticated request and every WebSocket frame**, the server checks that:
  - the access token's session (`sid`) is not revoked;
  - the account is `active`, or `unverified` for the routes above.

  This is one indexed lookup per request.
- **Closing live connections:** revoking a session, or blocking or deleting an account (including through `ovc-admin`), publishes a revocation signal through the [broadcaster](realtime-and-video.md#broadcaster). The affected WebSocket connections are then closed with code `4401` within 5 seconds.

## Registration and email verification

- **Sign-up form:**
  - name, email, password (10 to 128 characters), language, and a consent checkbox for personal-data processing; the consent version and time are recorded;
  - the browser's IANA timezone is sent automatically and stored as the account timezone;
  - a student sign-up may carry a tutor code.
- **Slice 2** accepts tutor sign-ups only; students join through their tutor from slice 3 ([proposal 0002](../../docs/proposals/0002-student-onboarding.md)), and `role: student` is refused with `validation_failed`. The consent version is the constant `ConsentVersion` in `internal/domain`, changed in code when the privacy policy changes. A browser timezone that is not a known IANA name is stored as `UTC`, which the person can correct in settings. Passwords are compared byte for byte, without Unicode normalisation, and their length is counted in characters.
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
  - from a browser signed in to that account, it activates the account and revokes every other session;
  - from any other browser, the page asks the person to set a new password. Confirming then sets that password, activates the account, revokes every session, and signs this browser in. Whoever created a squatting account therefore loses it, because the mailbox owner chooses the password.
- **Tokens are bound to the account state:** confirming accepts only an `unverified` account and resetting only an `unverified` or `active` one; every other state, like an unknown, expired, used or superseded token, answers the same `422 token_invalid`. A token is consumed by one conditional update while the account row is locked, so the account cannot change state in between. Confirmation, reset and the password change of an active account discard every unused token of both purposes and every queued request that has not used a token, so an email queued before the change is dropped at dispatch. A password change of an unverified account keeps them: whoever signed up with someone else's email knows the password, and must not be able to cancel the mailbox owner's links. The worker mints a token in one transaction that locks the account row and checks again that the account is in a state the token applies to, that the request still exists, has not used a token and is the newest of its purpose.
- **Completing a password reset** also confirms an unverified email, because it proves access to the mailbox.
- **Both** then evaluate the pending tutor code and consume the reserved invitation, and return the outcome (`linked`, `request_sent`, `code_invalid`, `invitation_invalid` or nothing).
- **A tutor code given at sign-up** is kept on the account as a pending code and evaluated when the email is confirmed, as if the student entered it then:
  - an invalid or regenerated code shows "This code is no longer valid — enter a new one";
  - a tutor with too many pending requests shows `tutor_unavailable`;
  - a blocked requester sees the normal "request sent" state.
- **Sign-up through an invitation** is described under [invitations](#invitations).

## Sessions and passwords

- **Access token:** short-lived (15 minutes), carrying `sid`. The frontend keeps it in memory only, never in Web Storage or cookies.
- **Refresh token:**
  - rotating, valid for 30 days;
  - held in a cookie named `__Host-ovc_refresh` with `HttpOnly`, `Secure`, `SameSite=Strict` and `Path=/`;
  - reuse of a rotated refresh token revokes the session;
  - its value is `<session>.<generation>.<random>.<mac>`: the session identifier (16 bytes), the decimal generation (from 1, no sign or leading zero), 32 random bytes and an untruncated HMAC-SHA-256 over the session identifier and generation with a key derived by HKDF from `OVC_JWT_SECRET` (info `refresh-generation`), each binary part in unpadded base64url and parsed strictly;
  - the MAC is verified before the session is looked up, so a forged cookie costs no query and changes nothing; a cookie with a valid MAC and an older generation than the stored one is reuse and revokes the session; a newer generation, or the current generation with another random part, is refused without revoking; a refresh whose rotation loses a race is re-checked against the stored row by the same rules, so the losing request counts as reuse, which the browser's per-tab lock keeps from happening;
  - the session expires 30 days after sign-in; rotation never extends it.
- **Refresh route checks:**
  - it accepts a request only when its `Origin` header equals the configured OVC origin exactly; a same-site subdomain such as the conferencing host does not qualify;
  - a request without a refresh cookie gets `204` with no token, which the client treats as "signed out" rather than as an error;
  - it is the only route authenticated by a cookie;
  - the conferencing origin is never in the CORS allowlist.
- **Sign out** revokes the current session; **sign out everywhere** revokes all of the user's sessions.
- **Throttling:** more than 5 failed sign-ins for one email within 15 minutes, or more than 20 from one IP address (an IPv6 address counts by its /64), returns `429` with `Retry-After`. The response never says whether the email exists. Throttle counters are rows in PostgreSQL keyed by purpose and subject, with fixed windows; each attempt reserves its per-IP slot and then, only within that limit, its per-email slot, atomically and before any password is hashed, and a successful sign-in returns both. The email in a subject is trimmed and lower-cased with Unicode case mapping. A rehash after sign-in replaces the stored hash only if it has not changed meanwhile, so it never undoes a concurrent reset or change; when another sign-in replaced it first, the password is verified again against the stored hash. The session starts in a transaction that locks the user row and requires the verified hash and an active or unverified state to be unchanged, so a sign-in that races a reset or a block creates no session; a block found at that point answers `account_blocked`. This guarantee needs the reset, block and deletion flows to change the user row and revoke its sessions in one transaction that holds the same row lock. When hashing is busy or fails, or the request is cancelled while a password is being verified, both reserved slots are returned. The client address is the TCP peer, or, when the peer is inside `OVC_TRUSTED_PROXY_CIDRS`, the right-most `X-Forwarded-For` entry outside those ranges; an unparsable entry falls back to the peer.
- **Sign-in answers:** an unknown email, a wrong password and a deleted account all answer `401 invalid_credentials`, and an unknown email still costs one password verification against a placeholder hash; a blocked account answers `403 account_blocked` only after the correct password. A password hashed with older parameters is rehashed on a successful sign-in. Password hashing runs at most `GOMAXPROCS` hashes at once and answers `503 service_unavailable` after waiting 5 seconds for a slot.
- **Password reset:**
  - requesting a reset always returns the same response, within the sending limits above;
  - the worker mints the emailed token (32 bytes, hashed and committed before sending, valid for 1 hour, single use);
  - a successful reset revokes all sessions.
- **Password change** requires the current password and revokes every other session. Wrong current passwords are counted per account under the sign-in failure limit (5 in 15 minutes); the sixth attempt answers `429` with `Retry-After`, and a correct password returns its slot.
- **Email change** is not in the MVP.

## Tutor–student relationships

- **States:**

  | State | Meaning |
  |---|---|
  | `invited` | A student card exists; no student account is linked |
  | `active` | Linked and working together |
  | `ended` | The tutor ended the relationship; a new invitation or request can reactivate it |
  | `discarded` | An invited card withdrawn by the tutor, merged into another card of the same student, or cleaned up by account deletion |

- **Transitions:**

  | From | To | Cause |
  |---|---|---|
  | — | `invited` | Card created |
  | `invited` | `active` | Invitation accepted; join request accepted with the card's email |
  | `invited` | `discarded` | Withdrawn; merged; tutor deleted |
  | `active` | `ended` | Tutor ends; tutor or student deleted |
  | `ended` | `active` | New invitation accepted by the same student account; join request from the same account accepted |
  | `ended` | `discarded` | Purge of the student (the row is kept for the tutor's history); purge of the tutor deletes the row |

- **Linking:** a relationship belongs to exactly one tutor and, once linked, exactly one student account forever. There is at most one non-discarded relationship per tutor and student account, and at most one non-discarded card per tutor and email, compared case-insensitively.
- **The student card** is the tutor's view of the relationship:
  - display name, set by the tutor and private to them;
  - optional email;
  - groups;
  - lesson history;
  - issues and scores;
  - private notes, never returned to the student.
- **Names the student sees:** the student sees their own account name everywhere. Other participants (group members, the conferencing room) also see the student's account name, never the tutor's private display name.
- **Tutor colour:** each relationship has a colour that labels this tutor's items in the student's views. It is chosen from a fixed palette at linking time, unique among the student's relationships where possible.
- **Isolation:** a tutor lists, reads and acts only on their own relationships. Any route that takes a student or relationship identifier resolves it within the requesting tutor's relationships and returns `not_found` otherwise.
- **Duplicate cards:** creating a card with an email that one of the tutor's non-discarded cards already has is refused with `student_exists`, and the response names the existing card.
- **Card limits:** a tutor creates at most 50 cards and changes card emails at most 50 times per day.
- **Withdrawing a card:** the tutor can withdraw an `invited` card; it becomes `discarded` and its invitations are revoked.
- **Changing the email of an `invited` card** revokes its outstanding invitations.
- **Ending:** ending an `ended` relationship returns its current state (idempotent); ending an `invited` or `discarded` card is refused with `relationship_not_active`.
- **Scheduling and issuing** require an `active` relationship, so invited cards never get lessons or work.

## Invitations

- **Creating:** a verified tutor creates a student card with a display name and an optional email. The card starts as `invited`.
- **Invitations come in two deliveries:**
  - **link**: the tutor copies it;
  - **email**: OVC sends it to the card's email.

  Each delivery has its own token (32 random bytes), valid for 14 days and single use. The token is stored as a hash for lookup and encrypted for re-display. The card shows the current link token; the worker renders the email token.
- **Regenerating** revokes all of the card's unused invitations.
- **An `ended` card** can be given new invitations. They are **bound to the card's linked student account**: only that account can accept them, and any other account is refused with `invitation_email_mismatch`.
- **Sending limits:**
  - a tutor in their first 7 days may send 10 invitation emails per day, and 50 per day after that;
  - at most 3 invitation emails per recipient address per day across all tutors.
- **The invitation page** (`GET /invitations/{token}`):
  - shows the tutor's public profile (name, avatar, subjects) and never the invitee's email;
  - an invalid, expired, used or revoked token gets one identical "no longer valid" response;
  - follows the [token page rules](../design/web/navigation.md#token-pages).
- **Accepting with a new account:**
  - **The invitation has an email and no account holds it:** the sign-up form fixes that email.
    - For an **email** delivery, completing sign-up confirms the email and consumes the invitation at once.
    - For a **link** delivery, the account is created `unverified` and the invitation is **reserved** for it: held on the account and not usable by anyone else. It is consumed when the email is confirmed.
  - **The invitation has an email that an account already holds:** the page still offers both "Create account" and "Sign in to accept", so it reveals nothing. A sign-up attempt follows the duplicate-email path, and the existing owner gets the notice and can sign in to accept.
  - **The invitation has no email:** the new account is `unverified` and the invitation is reserved for it as above.
- **Reserved invitations:**
  - if a reserved invitation expires or is revoked before confirmation, the confirmed account shows "This invitation is no longer valid — ask your tutor for a new link" and stays unlinked;
  - an invitation can be reserved by only one account at a time; another account trying it gets `invitation_invalid`.
- **Accepting as a signed-in account:**
  - an invitation with an email can be accepted only by an account with that email (`invitation_email_mismatch` otherwise);
  - an invitation on a linked card only by that card's student account;
  - a tutor account is refused with `role_mismatch`.
- **Consumption is atomic:** a conditional update that succeeds only while the token is unused, unrevoked and unreserved by another account. Concurrent acceptances or a concurrent regeneration lose with `invitation_invalid`.
- **Linking:**
  - links the account to the card, sets the relationship `active`, and revokes the card's other unused invitations and reservations;
  - if the student already has a relationship with this tutor through another card, that existing relationship is kept (reactivated if `ended`), and the invited card becomes `discarded`;
  - a pending join request from the same student to the same tutor becomes `accepted`;
  - the tutor gets an in-app notification.

## Tutor code and join requests

- **The code:**
  - each tutor has one code of 8 characters from an unambiguous alphabet (no 0/O, 1/I/L);
  - shown in Settings and on Today;
  - can be regenerated, and the old code stops working immediately;
  - inactive while the tutor is unverified.
- **Entering a code:** a verified student enters a code.
  - A tutor account entering a code is refused with `role_mismatch`.
  - An unknown or inactive code is refused with `tutor_code_invalid`.
  - Otherwise it creates a `pending` join request, unless:
    - they already have an `active` relationship with that tutor (refused with `already_linked`);
    - they have a pending request to that tutor (the existing request is returned);
    - the tutor has blocked their requests: the response is identical to a created request, and the student's request list shows it as pending until it expires; nothing reaches the tutor.
- **Limits:** codes are throttled to 10 attempts per student per hour, and each tutor holds at most 50 pending requests (further requests get `tutor_unavailable`).
- **Accept:**
  - reactivates the tutor's existing relationship with that student account if one exists (`ended`);
  - otherwise links a non-discarded `invited` card with the student's email if one exists;
  - otherwise creates the relationship.
- **Decline:** the student learns only that the request was not accepted. "Decline and block" also stops future requests from that student; the tutor manages blocked students in Settings.
- **Expiry:** pending requests expire after 14 days. A declined or expired request does not block a new one, unless blocked.

## Ending a relationship

The tutor ends a relationship from the student card, after confirming. In one transaction:

- the relationship becomes `ended` and the student is removed from all of the tutor's groups;
- **lessons:**
  - one-to-one series of the relationship end with an end date of today in the series timezone, a count being converted into that end date;
  - future one-to-one lessons are cancelled;
  - the student is removed from the audience of future group lessons;
  - the student can no longer join any of the tutor's lessons, including live ones ([join rule](schedule.md#join-rule));
- **work:**
  - issued work the student has not submitted (`not_started`, `draft`, `returned`) becomes `closed`;
  - `submitted` work stays checkable by the tutor but can no longer be returned;
  - checked work stays readable by both;
- **chat:** the private chat becomes read-only for both;
- **recordings:** recordings of lessons whose roster includes the student stay readable by the student until they expire;
- **notifications:** one aggregated `relationship.ended` email lists the cancelled lessons, together with the in-app notification, through the outbox.

**Reactivation:** a new invitation accepted by the same account, or an accepted join request from it, reactivates the same relationship with its history. It makes the private chat writable again. It restores no group memberships, no series and no closed work.

## Groups

- **A group** belongs to one tutor and has:
  - a name (1–60 characters);
  - a colour;
  - members who are students with `active` relationships to that tutor (at most 30).
- **Adding a member** gives them:
  - the group chat, including its history;
  - future group lessons;
  - the right to join a group lesson that is already live, which also adds them to that lesson's roster.

  It does not give them work issued to the group earlier ([issuing](homework.md#issuing)).
- **Removing a member:**
  - removes them from future group lessons and the group chat, and they can no longer join a live group lesson;
  - they keep their issued work, which they may still submit while their relationship with the tutor is active.
- **Archiving:**
  - ends the group's series and cancels its future lessons;
  - makes its chat read-only;
  - refuses new issues to the group (`audience_not_active`);
  - members get one aggregated `group.archived` notification listing the cancelled lessons.
- **Unarchiving** restores issuing and chat, but no series and no cancelled lessons.

## Account deletion

- **Starting deletion:** it requires the current password and an explicit confirmation. The account becomes `deleted` at once: sessions are revoked, sockets are closed and sign-in is refused. A purge job runs 30 days later.
- **From deletion until purge**, the account's messages show their sender as "Deleted user" and keep their bodies.
- **Student deletion:**
  - active relationships end, with the effects above, without sending `relationship.ended`;
  - pending join requests expire;
  - reserved invitations and pending codes are cleared.
- **Tutor deletion:**
  - active relationships end and invited cards become `discarded` with their invitations revoked;
  - pending join requests expire;
  - groups are archived, series end and future lessons are cancelled;
  - every student receives one `tutor.deleted` email that lists the cancelled lessons, instead of separate ended or cancellation emails;
  - for 30 days, students keep read-only access to their own submissions, results and recordings from that tutor.
- **An unverified account** is purged at once when its owner deletes it, so its email is free again immediately.
- **In slice 2** deletion and purge cover the account's own data (sessions, email requests and tokens, the tutor profile, uploads and avatar references, the user row); the relationship, group, lesson and message effects above arrive with the slices that add those tables. The hourly `accounts.purge` job purges accounts deleted 30 days ago and the hourly `accounts.cleanup_unverified` job removes accounts unverified for 7 days, in batches of 100, each account in its own transaction; an account whose purge fails is logged as `account.purge_failed` and skipped for that run, so it never holds up the others.
- **Purge** is defined per table in [the purge rules](data-model.md#purge). The purge job re-checks the account's state under a row lock, so a restore never races it.
  - Purging a student erases their submissions, answers, uploaded files and message bodies (shown as "message removed"), including the files attached to those messages.
  - Purging a tutor deletes the tutor's library, lessons, recordings, issues, chats, files, student cards, notes and the emails stored on cards.
  - Recordings in which a purged student took part are kept until their own expiry (at most 365 days).
- **Backups:** database backups are retained for 30 days, so purged data leaves backups within 30 days after purge ([ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md)).
- **After purge**, deletion is irreversible. Before purge, only the operator can restore an account; restoring does not reactivate ended relationships.

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
- **Failures before the command runs** (an unknown system user, an unreachable database, pending migrations) are audited as `admin.command` with outcome `failed`. Under `OVC_CLOCK=test` the command uses the shared test clock, like the API and the worker.
- **Email lookup** is case-insensitive and trims spaces; every change locks the user row.
