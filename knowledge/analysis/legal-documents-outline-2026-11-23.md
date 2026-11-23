# Outline of OVC's legal documents

Written 2026-11-23. Agent analysis, not legal advice and not the final texts: it fixes the structure and the OVC-specific content of the three documents the [legal gaps](legal-gaps-2026-11-22.md) put first, the privacy policy, the consent to processing and the user agreement, so that drafting starts from the real data flows in the specifications. The texts themselves are written in Russian later; `[verify]` marks interpretations to confirm against the current law or with a specialist. The operator is an individual entrepreneur and OVC is the only operator ([owner decisions](legal-gaps-2026-11-22.md#owner-decisions-of-2026-11-22-and-2026-11-23)); [proposal 0006](../../docs/proposals/0006-legal-documents-and-student-data.md) records the decisions and [ADR-0024](../../docs/decisions/0024-legal-documents-and-student-data.md) accepts them; the record governs where this outline differs. The retention bullets below describe current behaviour; ADR-0024 changes the retention of unaccepted, withdrawn, merged and removed cards and of student data after a student's purge.

## Shared facts all three documents state the same way

- **Operator:** the individual entrepreneur, full name, OGRNIP, INN, address, contact email for personal-data requests.
- **Processors by name** (each under a processing instruction): the cloud provider hosting the application, database, backups and object storage; the mail provider; the provider hosting the self-hosted conferencing server. All in Russia.
- **Retention:**
  - account data until deletion plus 30 days, then purged; unconfirmed accounts 7 days; a blocked account until the operator unblocks, restores or deletes it, a period with no bound;
  - expired or revoked sessions deleted after a day; email requests and their tokens 7 days after the request;
  - message bodies shown from a "Deleted user" for the 30 days before the purge;
  - student data in a tutor's records (the card's display name, private notes, the meeting history, the student's issues, the direct chat channel with the tutor's own messages and files; the card email is cleared at the purge) kept after the student's purge until the tutor's purge, since no route lets the tutor delete it, a period with no bound yet;
  - withdrawn and merged cards: discarded rows the tutor cannot see, keeping display name, email and notes until the tutor's purge, a merged card's email even after the student's purge;
  - in-app notifications 90 days;
  - the channel of a discarded link (removal, a dropped or expired reclaim, the 7-day cleanup, a tutor's deletion and the other discards the chat specification lists) until its physical erasure, which has no fixed deadline;
  - removed links: discarded rows keeping the display name, invisible to the tutor, until the tutor's purge;
  - an unaccepted card's email 90 days (decided; specification pending);
  - database backups kept for a 30-day window and never older than 31 days, so purged rows can remain in a backup up to 31 days after the purge;
  - PostgreSQL logs and conferencing logs rotated within 14 days, the latter holding IP addresses and join links that carry display names and opaque identifiers;
  - mail provider logs at most 30 days, a selection condition the provider has yet to confirm;
  - the bounce event stream at its shortest available retention;
  - recordings, once they exist, at most 365 days by the tutor's setting.
- **No transfer abroad** at launch; web push adds the browser vendors' push services only after its own assessment and an update of all three documents.
- One version identifier per document; the consent version is the backend constant `CurrentConsentVersion`, and a changed consent text changes it.

## Privacy policy

Required by item 2 of part 1 and part 2 of article 18.1; published at `/privacy` with unrestricted access.

1. General provisions: scope (the OVC site and service), operator details, date and version, where the current version lives.
2. Terms: personal data, processing, operator, processor, tutor, student, as the documents use them.
3. Legal grounds: items 1 and 5 of part 1 of article 6, the consent and the user agreement; for data a tutor enters before a student signs up, the legitimate interests of item 7, supported by the tutor's warranty (owner decision of 2026-11-23) `[verify]`.
4. Purposes, each with its subject categories, data, actions, methods and retention, derived from the specifications:
   - accounts and sign-in, including the keyed hashes of IP addresses and emails in the throttle counters;
   - student cards and invitations, including data received from a tutor, not from the student (article 18 part 3), and the 90-day email bound;
   - relationships, tutor-code join requests and blocked requesters, groups, schedule and lessons;
   - video lessons on the conferencing server: from OVC, the display name, opaque identifier, role and the presentation file; from browsers, IP addresses, audio, video and screen streams, and in-room chat and whiteboard content, live and not recorded; recordings after the MVP;
   - chats, files and chat presence;
   - homework, answers and reviews published to the student, points and scores visible only to the tutor, all kept readable after a relationship ends;
   - service email, its preferences and the undeliverable marks from the mail provider;
   - administration: account blocking and the operator's audit trail;
   - logs and security, including the minimal records of unfinished file cleanup.
5. Processors by name and what each receives.
6. Localization: all databases in Russia (article 18 part 5).
7. Cookies and similar: one strictly necessary sign-in cookie; language, theme and sidebar preferences in the browser's local storage; the conferencing client's own session cookies on its subdomain, if any `[verify]`; no analytics or trackers.
8. Rights of the subject and how to use them: access, correction, blocking, deletion, withdrawal, complaint to the regulator; the request address and the response term, 10 working days extendable by 5 `[verify]`; deleting the account in settings; today a student cannot have a tutor's card, notes or issues about them deleted through the product, an input for the lawyer.
9. Private notes a tutor keeps about a student: personal data of the student, disclosed on a lawful request `[verify]`.
10. Minors: OVC's position on ages 14–17, pending [proposal 0003](../../docs/proposals/0003-students-aged-14-and-over.md).
11. Security measures in summary, without details useful to an attacker.
12. Breach handling in one paragraph: the regulator is notified as the law requires.
13. Destruction: automatic purge, files deleted with their last reference, backups expiring after their window, and how destruction is confirmed.
14. Changes to the policy and how users learn of them.

## Consent to processing

A text separate from the policy and the agreement, confirmed by its own checkbox at sign-up and on the invitation page (article 9 part 1 as amended by 156-FZ). The checkbox exists; the text behind it does not.

1. Who consents: the person signing up, identified by the account.
2. Operator: name and address.
3. Purposes: by reference to the purposes of the policy, listed briefly.
4. Data: the list.
5. Actions and methods: the list, with and without automation.
6. Processors by name and address: mandatory only for written consent under part 4 of article 9, kept as good practice.
7. Term: until the purpose is reached or the consent withdrawn; how to withdraw (settings or the request address), and that withdrawal ends the account `[verify]`.
8. Version and date.

## User agreement

A public offer of a free service, accepted by pressing the sign-up button with a link to the agreement next to it, never by the consent checkbox (owner decision of 2026-11-23; `[verify]` that this acceptance form holds).

1. Parties: the entrepreneur and the user; the acceptance described above.
2. Subject: free access to OVC; what OVC is and is not (not an educational organization; the tutor teaches) `[verify]`.
3. Accounts: one person per account, accurate data, keeping credentials secret, roles of tutor and student.
4. The tutor's obligations: a warranty that each student agreed to be invited and to the data the tutor enters; keeping students' data within OVC; lawful content; no special categories of data in notes and chats without need; private notes may be disclosed to the student on a lawful request.
5. The student's position: one account across several tutors; each tutor sees only their own relationship; scores are never shown to students.
6. Content rules: prohibited content, harassment, spam; complaint address; blocking and removal by the operator.
7. Uploaded materials: the user keeps rights and grants OVC a licence limited to storing and showing them to the intended participants; responsibility for third-party rights.
8. Lessons and, after the MVP, recordings: who sees them, retention, informing participants.
9. Deleting an account and ending relationships: effects as FR-1.7 and FR-2.7 describe, 30 days of read-only results for students of a deleted tutor.
10. Availability and liability: service as is, limits of liability as far as the law allows `[verify]` consumer-protection limits for a free service.
11. Changes to the agreement: notice and how continuing use or a new acceptance works `[verify]`.
12. Minors: pending proposal 0003.
13. Governing law, disputes, contacts.

## Consequences for the product

Each needs its own specification and code change with a plan and reviews; none is approved here, and the [constraints on implementing the decisions](legal-gaps-2026-11-22.md#constraints-on-implementing-the-decisions) apply.

- Sign-up and the invitation page: the consent checkbox links to the consent text; one line under the sign-up button links to the agreement.
- The invitation email: one fixed line with the operator and a link to the policy on OVC's origin; changes the notifications specification and T-31.
- An unaccepted card loses its email after 90 days, revoking its open invitation in the same transaction, and withdrawn and merged cards lose display name, email and notes at once; changes the invitations specification, T-7, T-23 and T-34.
- Three public pages instead of the `/privacy` placeholder, in Russian and English.

## Facts the policy needs that the repository does not settle

A check of the policy's purposes against the specifications on 2026-11-23 brought in the tutor-code requests and blocked requesters, chat presence, email preferences and undeliverable marks, the keyed hashes in the throttle counters, the operator's blocking and audit trail, the message bodies kept until the purge, work kept readable after a relationship ends, the records of unfinished file cleanup, and what the conferencing server receives from browsers. These periods have no decided value yet:

- the reverse proxy's access logs, which normally hold client IP addresses;
- the `ovc-admin` audit trail, written to the command's standard error;
- the application's own diagnostic logs;
- the lesson presentation on the conferencing server, which T-11 records as kept while the server keeps the meeting, with no numeric bound;
- the channel of a discarded link;
- blocked accounts;
- an invited card's display name and notes.

Student data in a tutor's records after the student's purge, withdrawn and merged cards and removed links' display names are decided in proposal 0006, with their specification pending.

Each needs a decided period recorded in the threat model (T-18, T-34, and T-22 for the audit trail) and the deployment plan before the policy can state it; no period relaxes the token redaction of T-27.
