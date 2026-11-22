# Proposal 0006: Legal documents and student data

- Status: proposed
- Reviewed: decisions taken by the project owner on 2026-11-22 (the operator, OVC as the only operator, legal wording in the documents) and on 2026-11-23 (the legal bases, the acceptance of the user agreement, the invitation notice and the retention changes); wording pending owner review
- Author: project owner, written up by an agent
- Date: 2026-11-23
- Trigger: a lawyer's review of the legal bases, the acceptance form and the invitation notice before the first deployment ([roadmap: before the pilot](../roadmap.md#before-the-pilot)); then this proposal becomes a decision record with its specification changes
- Related: [ADR-0005](../decisions/0005-tutor-centric-domain.md), [ADR-0008](../decisions/0008-hosting-and-data-residency.md), [ADR-0014](../decisions/0014-students-join-through-their-tutor.md), [proposal 0003](0003-students-aged-14-and-over.md), FR-1.1, FR-1.7, FR-2.7, NFR-1.3, [accounts specification](../../knowledge/specs/accounts.md), [data model](../../knowledge/specs/data-model.md), [notifications specification](../../knowledge/specs/notifications.md), [threat model](../threat-model.md), [legal gap analysis](../../knowledge/analysis/legal-gaps-2026-11-22.md), [outline of the legal documents](../../knowledge/analysis/legal-documents-outline-2026-11-23.md)

## Motivation

OVC collects personal data at sign-up and in every feature, but it has no privacy policy (the `/privacy` page is a placeholder), no consent text behind the consent checkbox of FR-1.1, and no user agreement, while [proposal 0003](0003-students-aged-14-and-over.md) already relies on a user agreement as a legal basis. Federal Law 152-FZ requires a published policy, a consent issued separately from other documents, and notice to a subject whose data comes from someone else; [ADR-0008](../decisions/0008-hosting-and-data-residency.md) leaves these to the owner outside the repository without deciding their content. The [legal gap analysis](../../knowledge/analysis/legal-gaps-2026-11-22.md) also found student data that outlives its purpose: an unaccepted card keeps a prospective student's email without a bound; withdrawn, merged and removed cards become discarded rows that keep a student's name, email or notes, invisible to the tutor, until the tutor's purge; and after a student's purge the tutor's records keep the student's name and notes with no route to delete them. Without this proposal the first deployment has no lawful documents, and FR-1.7's promise of deletion does not reach data held in tutors' records.

The legal interpretations behind it are agent analysis marked `[verify]` there; this proposal records the owner's choices and waits for a lawyer before it becomes a decision record.

## Proposal

1. **Operator:** the operator of personal data in OVC is the owner's individual entrepreneur, the same party for the user agreement, the regulator notification, processor contracts and later payments.
2. **Only operator:** OVC is the only operator of all personal data in the service, including what tutors enter about their students; a tutor is a user, not an operator, and OVC is not a tutor's processor.
3. **Legal bases:** consent (item 1 of part 1 of article 6 of 152-FZ) and the user agreement (item 5); the name and email a tutor enters before a student signs up rest on the legitimate interests of the operator and the tutor (item 7), supported by the tutor's warranty in the user agreement that the student agreed to be invited.
4. **Three documents:** a privacy policy published at `/privacy`; a consent text separate from it, confirmed by the existing consent checkbox; and a user agreement, a public offer of the free service, accepted by pressing the sign-up button with one line under it linking to the agreement, never by the consent checkbox. The same applies on the invitation page.
5. **Legal wording in documents:** explanations, warranties and disclosures live in the documents; the interface carries only what the law requires there, such as the consent checkbox and the acceptance line.
6. **Invitation notice:** the invitation email carries one fixed catalogue line naming the operator and linking to the policy, as the notice of article 18 part 3 to a person whose data came from a tutor. The operator's name and contact are constants or operator settings, never user input; the link is built from `OVC_PUBLIC_URL` with no per-recipient query or token. The rule that the invitation link is the email's only link becomes a rule that the email's only links are the invitation link and the fixed policy link. A send dropped silently (by the per-recipient cap, or for an address whose account is blocked or deleted) carries no notice, and the invitation page links the policy for a card without an email.
7. **Private notes:** stay as they are; the user agreement tells tutors that notes are personal data of the student and may be disclosed to the student on a lawful request.
8. **Retention:**
   - an unaccepted card's email is erased after 90 days;
   - withdrawing or merging a card erases its display name, email and notes at once;
   - removing a provisional or reclaim-pending link, by any path that applies the removal effects of the accounts specification (the tutor's removal, the student stopping studying, a dropped or expired reclaim, the 7-day cleanup, an account's deletion or purge), erases the display name that the discarded row keeps, in the same transaction; the notes and email move to the new invited card as today;
   - when a purged student held a provisional or reclaim-pending link, the replacement invited card stays a live card the tutor sees, with the notes and email moved onto it; its email falls under the 90-day rule counted from the removal;
   - a student's purge, over every relationship row of the student including discarded ones, replaces the card's display name with a fixed label and erases its notes; lesson and issue history stays under the label; the student's direct chat keeps the tutor's own messages, which the tutor may delete. The documents say "replaces the name with a fixed label", never "anonymises": chat, issue content and dates may still identify the student.

### Constraints for the specification change

- The 90-day erasure revokes the card's open invitation in the same transaction and records an erasure marker; regenerating and sending refuse a card with the marker until the tutor enters an email, checked under the card lock, because an email-less card otherwise legitimately holds an unbound link (T-7, T-23). The specification decides whether the period runs from the card's creation or from the last time the tutor entered the email, and how replacement cards count beyond the rule above.
- The erasure job locks each card in the acceptance lock order and re-checks it, skipping a card that is no longer `invited` and never erasing an email the tutor entered concurrently.
- Wherever a label reads an erased display name (`invitation.accepted` rows after a merge or a removal, audience labels of schedules and chats), it shows the surviving or replacement card's display name or a fixed label code, never the account name (T-44); the specification relaxes the required length of `display_name` accordingly.
- If the specification moves a merged card's notes to the surviving card first, the move stays within the same tutor's scope and is never visible to the student.
- Negative tests at the invitation boundary: after erasure the old link answers the same `404` on the invitation page and refuses sign-up and signed-in acceptance whatever the email; regenerating is refused until an email is entered; an invitation email queued before erasure is obsolete at dispatch; a purge test through `Tick(now)` covers a student who held a provisional link.

### What does not change

The 30-day purge after deletion, the 7-day cleanup of unconfirmed accounts and the invitation's email binding stay. Erased values remain in database backups for up to 31 days and in the mail provider's message logs for up to 30 days; the conferencing host keeps its logs for 14 days; recordings in which a purged student took part keep the student's likeness and displayed name until their own expiry, at most 365 days, once recordings exist. Throttle counters keep only keyed hashes and outbox payloads only identifiers, so neither keeps an erased email. Student accounts the operator has blocked are never purged automatically (T-22), so their records are not relabelled until the operator acts. An invited card's display name and notes have no bound beyond the tutor withdrawing the card, and the replacement card a purged student's provisional link leaves keeps that student's name and notes until the tutor withdraws it. Minors stay with [proposal 0003](0003-students-aged-14-and-over.md), web push with [ADR-0015](../decisions/0015-web-push.md). The retention of the reverse proxy's access logs, the `ovc-admin` audit trail and the lesson presentation on the conferencing server is decided in the pilot deployment plan. No rows exist before the first deployment, so no backfill is needed.

The relabelling at purge contradicts the current wording of the data model and of T-34, which call the card's display name the tutor's own data; the specification change replaces that wording.

## Alternatives

- **The tutor as an operator, with OVC as the tutor's processor:** the usual model for business software, but every tutor, often self-employed, would have to notify the regulator and keep their own documents; rejected for the tutors' burden.
- **The tutor's warranty alone as the basis for invitation data:** simpler to state, but a weaker footing than legitimate interests balanced against the student's rights, which the warranty and the 90-day bound support.
- **Erasing withdrawn and merged cards by the 90-day job:** one rule for all unaccepted cards, but it keeps data the tutor can no longer see for months without purpose.
- **A button for tutors to delete a purged student's records, instead of relabelling at the purge:** keeps the tutor in control, but leaves the student's name in place until a tutor acts and leaves the student's right to deletion to the tutor.

## Plan

On acceptance, after the lawyer's review:

1. A decision record recording this proposal's decision, linked from it and amending ADR-0008's statement that the privacy policy and consent live outside the repository, and specification changes in one change: [accounts](../../knowledge/specs/accounts.md) (invitations, relationships, removal, reclaim, deletion), the [data model](../../knowledge/specs/data-model.md) (relationship columns and the erasure marker, the purge table, the paragraph on the tutor's own data), [notifications](../../knowledge/specs/notifications.md) (the invitation email's links, labels and links), [realtime and video](../../knowledge/specs/realtime-and-video.md) (labels of a purged student's direct channel), [schedule](../../knowledge/specs/schedule.md) (labels per role), [homework](../../knowledge/specs/homework.md) (issues kept under the label), FR-1.1 and FR-1.7, and the threat model's T-7, T-23, T-31, T-34 and T-44.
2. The backend: the erasure on withdrawal, merge and removal; the 90-day job; the relabelling in the purge; the invitation email line. Integration tests run the real worker through `Tick(now)` against the real database, with the database mail sink and the fake clock; the negative tests above prove the invitation boundary.
3. The frontend: three public pages in Russian and English replacing the `/privacy` placeholder, the consent checkbox linking to the consent text, and the acceptance line under the sign-up button, with end-to-end coverage in the pilot scenario that signs up.

The texts of the three documents are drafted from the [outline](../../knowledge/analysis/legal-documents-outline-2026-11-23.md) and reviewed by a lawyer before they ship.

## Risks and open questions

- The lawyer may change the basis for invitation data, the acceptance form or the place of the notice.
- A relabelled student may still be identifiable from chat, issue content and dates.
- How an individual entrepreneur answers under article 13.11 of the Code of Administrative Offences.
- Retention periods still undecided: the reverse proxy's logs, the `ovc-admin` audit trail, the application's diagnostic logs, the lesson presentation on the conferencing server, blocked accounts, and an invited card's display name and notes.
- The clock start of the 90-day rule and the counting of replacement cards beyond the rule above.

## Decision

Not yet decided.
