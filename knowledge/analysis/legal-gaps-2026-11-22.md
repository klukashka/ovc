# Legal gaps before the first deployment

Written 2026-11-22, amended 2026-11-23 with the gap analysis, the triage of tutor invitations and the owner's decisions of that day. Agent analysis: it maps what the repository already does for Russian personal-data law (Federal Law 152-FZ) and the site's legal texts, lists what is missing with a priority, and holds the owner's decisions until they move into a [proposal](../../docs/proposals/README.md), planned as proposal 0006 on the legal documents and invitation data. It is not legal advice, and the agent is not a lawyer: every interpretation marked `[verify]` and the final texts of the privacy policy, the consent and the user agreement need a lawyer's review. Legal facts are volatile; the dated evidence lives in the [minors consent research](../research/minors-consent-practice-2026-10-28.md) and the 152-FZ section of the [hosting research](../research/hosting-2026-10-22.md#what-152-fz-asks-of-the-hosting), and the legal facts first cited here, including article numbers, amending laws, regulator orders and deadlines, are agent findings from secondary sources until a research note records them with sources; the most uncertain carry `[verify]`.

## What exists

| Area | State | Where |
|---|---|---|
| Data residency | Every component holding user data is hosted in Russia; no foreign analytics, CDNs or fonts | [ADR-0008](../../docs/decisions/0008-hosting-and-data-residency.md), NFR-1.4 |
| Consent at sign-up | A checkbox linking to `/privacy`; the consent version (the constant `CurrentConsentVersion` of type `ConsentVersion`) and time are stored | FR-1.1, [accounts](../specs/accounts.md#registration-and-email-verification), `frontend/src/features/auth/SignUpPages.tsx`, `backend/internal/domain/email.go` |
| Privacy policy | A placeholder page saying the text will be published before launch | `/privacy`, `frontend/src/lib/i18n/en.ts` |
| Account deletion | Self-service; purge 30 days after deletion, unconfirmed accounts removed after 7 days; a blocked account is never cleaned up or purged until the operator unblocks, restores or deletes it (T-22) | FR-1.7, [purge](../specs/data-model.md#purge) |
| Retention | Backups kept for a 30-day window, none older than 31 days; PostgreSQL and conferencing logs rotated within 14 days; recordings at most 365 days; the mail provider must keep message logs at most 30 days, a selection condition not yet confirmed | ADR-0008, [ADR-0013](../../docs/decisions/0013-pilot-deployment.md) |
| Data the conferencing server receives | From OVC's API: display name, opaque participant identifier, role, and the lesson presentation passed unfiltered. From participants' browsers: IP addresses, audio, video and screen streams, and in-room chat and whiteboard content, live and not recorded; its access logs hold IP addresses and per-request join links, which carry the display name and opaque identifier, for 14 days | NFR-1.3, T-11, T-25 |
| Email | One-click unsubscribe for optional emails; service emails cannot be turned off | FR-13.3 |
| Cookies and browser storage | One strictly necessary `__Host-` refresh cookie; preference keys for language, theme and sidebar in `localStorage`; the embedded conferencing client may set its own session cookies on its subdomain `[verify]` on the real host; no trackers | T-15, T-37, `frontend/src/lib/preferences/store.ts` |
| Minors | [Proposal 0003](../../docs/proposals/0003-students-aged-14-and-over.md) (14 and over, age band only, guardian self-assertion), waiting for a lawyer | Minors research, nine questions for a lawyer |
| Web push | Built and off until the legal assessment | [ADR-0015](../../docs/decisions/0015-web-push.md), T-46 |
| Operator steps | Roskomnadzor notification, a responsible person, the protection level, breach reporting, processing instructions to processors are listed | Hosting research |

ADR-0008 puts the privacy policy, consent and the regulator notification outside the repository as owner obligations, so most gaps below are legal acts and texts rather than code.

## Initial gap list, 2026-11-22

The [gap analysis](#gap-analysis-against-152-fz-2026-11-23) table supersedes this list; the list stays as the first orientation.

- **Before any deployment with real users, an alpha included:** the operator (decided below); the tutor's role (decided below); the privacy policy text; a separate consent text; a user agreement, on which proposal 0003 already relies; the Roskomnadzor notification, processing instructions with every processor and a responsible person.
- **Before the pilot:** minors; subject requests; breach response; the protection level and local acts.
- **Later, with their features:** recordings after the MVP (informing participants, the right to one's image under article 152.1 of the Civil Code, not biometric data while not used for identification `[verify]`); web push (ADR-0015); payments ([proposal 0001](../../docs/proposals/0001-payment-ledger.md), [taxes research](../research/taxes-and-acquiring-2026-10-25.md)); copyright terms for uploaded materials; a cookie banner, probably unnecessary with only strictly necessary cookies and preference storage `[verify]`.

## Findings in the specifications

- **Unaccepted invited cards keep the student's email without a bound.** An invitation link expires after 14 days and its ciphertext is cleared, but an `invited` card with its display name and email stays until the tutor withdraws it ([invitations](../specs/accounts.md#invitations)). Withdrawing does not erase it: a withdrawn card, like a card merged into another card of the same student, becomes a discarded row that keeps its display name, email and notes, which the tutor can no longer see, until the tutor's purge; a student's purge clears the email only on cards linked to that student, so a merged card keeps the student's email. Removing a provisional or reclaim-pending link moves the notes and email to the new card and keeps the display name on the discarded row, also invisible to the tutor. Decided below: the email of an unaccepted card goes after 90 days; withdrawn and merged rows need the same treatment.
- **Student data in a tutor's records outlives the student's account without a bound.** A student's purge keeps the tutor's card with its display name and private notes (the card email is cleared), the tutor-owned meeting history, the student's issues and the direct chat channel with the tutor's own messages and files in the tutor's history, until the tutor's purge (the tutor may delete their own messages one by one), since no route lets a tutor delete a purged student's card, notes or issues (T-34, [purge](../specs/data-model.md#purge)). Purged rows also stay in backups for up to 31 days after the purge.
- **The data model and T-34 call the card's display name the tutor's own data** ([data model](../specs/data-model.md), the paragraph on the student's account name after purge; [threat model](../../docs/threat-model.md)), which conflicts with OVC being the only operator of a student's data held by a tutor.
- **Private tutor notes** are never returned to the student ([relationships](../specs/accounts.md#tutorstudent-relationships)). With OVC as the only operator, a student's access request under article 14 may have to cover them `[verify]`.
- **The accounts specification names the consent constant `ConsentVersion`**, which is the type; the constant is `CurrentConsentVersion`.

## Owner decisions of 2026-11-22 and 2026-11-23

Held here until proposal 0006 records them.

1. **The operator is an individual entrepreneur**, the same party for the user agreement, the regulator notification, processor contracts and later payments. The tax regime is the owner's choice with an accountant.
2. **OVC is the only operator** of all personal data in the service, tutors' entries about their students included; a tutor is a user, not an operator, and OVC is not the tutor's processor. Lawyer to confirm.
3. **Data a tutor enters before the student signs up** (the card's name and email, the invitation) rest on a warranty the tutor gives in the user agreement that the student agreed to receive the invitation; the email is used only for the invitation; an unaccepted card loses its email after 90 days while the tutor keeps the card with its name and notes (decided 2026-11-23). The [triage](#triage-of-tutor-invitations-2026-11-23) recommends the legitimate interests of item 7 of part 1 of article 6 as the basis, supported by the warranty; that choice is open for the owner.
4. **Private notes** stay as they are; the user agreement tells tutors that notes are personal data of the student and may be disclosed to the student on a lawful request.
5. **Legal wording lives in the legal documents, not in the interface.** The interface carries only what the law requires there, such as the consent checkbox at sign-up; explanations, warranties and disclosures go into the user agreement and the policy.
6. **The notice of article 18 part 3** to a student whose data a tutor entered is one short line in the invitation email linking to the policy, which holds the details; the email is a legally required message, not interface wording (decided 2026-11-23).
7. **The user agreement is accepted by pressing the sign-up button**, with one line under it linking to the agreement and no second checkbox; the consent keeps its own checkbox (decided 2026-11-23; `[verify]` that this acceptance form holds).

### Constraints on implementing the decisions

Each decision becomes a specification and code change with its own plan and reviews, under these constraints from the threat model.

- **The email erasure covers every row that keeps an email nobody needs:** the specification change also clears the email of withdrawn and merged cards, at once or by the same 90-day job, decides whether their display names and notes, and the display names kept on removed rows, go too, and updates T-34.
- **The 90-day email erasure** keeps the invitation's email binding (T-7, T-23): in the same transaction it revokes the card's open invitation, and the card cannot issue a link until the tutor enters an email again. The specification change decides whether the period runs from the card's creation or from the last time the tutor entered the email, and how a card recreated by removing a provisional link counts. It updates T-7, T-23, T-34 and the [invitations](../specs/accounts.md#invitations) section.
- **The invitation-email line** is fixed catalogue text, with the operator's name and contact as constants and never user input, and links only to OVC's own origin; it changes the [notifications](../specs/notifications.md) rule that the invitation link is the email's only link, and T-31.
- **Retention periods for the reverse proxy's access logs and the `ovc-admin` audit trail** go into the threat model (T-18, T-34, and T-22 for the audit trail) and the deployment plan; the audit period also serves investigations of operator misuse and breaches, and no period relaxes the token redaction of T-27.

## Questions for a lawyer, in addition to the minors research

1. With OVC as the only operator, may a prospective student's name and email be processed to send one invitation on the legitimate interests of item 7 of part 1 of article 6, supported by the tutor's warranty in the user agreement, or on the warranty alone, and how long may an unaccepted card keep them?
2. Does a student's access request under article 14 cover a tutor's private notes about them, and may OVC refuse or limit it?
3. Can an individual entrepreneur be the operator for tutors who are themselves entrepreneurs or self-employed and are paid by students outside OVC, without each tutor becoming an operator?
4. Is a checkbox with links to a separate consent text and to the policy enough under the amended article 9, and is accepting the user agreement by pressing the sign-up button with a link next to it a valid acceptance?
5. Which processor documents suffice with the cloud provider's public data processing terms, the mail provider and the conferencing host?
6. Does an individual entrepreneur answer as a legal entity under article 13.11 of the Code of Administrative Offences, and what are its current amounts?

## Gap analysis against 152-FZ, 2026-11-23

A checklist pass against 152-FZ and its by-laws over the whole personal-data regime before the first deployment. Status: ✅ in place or not applicable, 🟡 partial, 🔴 missing, ⚪ unknown. Currency, from secondary sources only: Federal Law 265-FZ of 2026-07-26 changed the criteria of the list of countries with adequate protection under article 12, which matters only once web push is enabled; no other change to 152-FZ after 2026-08 was found `[verify]`. Fine amounts are those reported for legal entities and are `[verify]`, as the hosting research also marks the 2026 amounts unconfirmed.

Summary: ✅ 10 · 🟡 5 · 🔴 12 · ⚪ 2; the rows on the article 18 notice and the 90-day email bound are decided and wait for their changes.

### Gap table

P0 is before any deployment with real users, P1 before the pilot, P2 with its feature. The owner, as the individual entrepreneur and the person responsible under article 22.1, owns every action.

| Priority | Gap | Status | Norm | Risk marker | Action |
|---|---|---|---|---|---|
| P0 | Privacy policy not issued or published | 🔴 | item 2 of part 1 and part 2 of article 18.1 | 🔴 blocks launch | Write it from the purposes in the [documents outline](legal-documents-outline-2026-11-23.md#privacy-policy) and the specifications; publish at `/privacy` |
| P0 | Regulator notification not filed | 🔴 | article 22 parts 1 and 3 | 100–300 thousand roubles reported for a legal entity `[verify]` | File on the regulator's portal before processing starts, including the database location and protection measures; update by the 15th of the next month after any change |
| P0 | Consent text missing | 🔴 | article 9 part 1; part 4 for written consent | 🔴 | A separate consent text: purposes, data, actions, withdrawal; the processors listed as good practice, since the part 4 list binds written consent |
| P0 | Processing by processors without instructions | 🔴 | article 6 part 3 | 🔴 | Conclude processing instructions with the cloud, mail and conferencing-host providers, and cover processing by them in the consent or another basis `[verify]` |
| P0 | User agreement absent | 🔴 | item 5 of part 1 of article 6; Civil Code | 🟠 | Draft it with the decisions above: tutor warranty, notes, content rules, blocking |
| P0 | Responsible person decided, no order | 🟡 | article 22.1, item 1 of part 1 of article 18.1 | 🟡 | Sign the order appointing the entrepreneur |
| P0 | Consent separateness | 🟡 | article 9 part 1 as amended by 156-FZ | 🔴 if merged | Keep the consent checkbox separate from accepting the user agreement (decision 7) |
| P1 | Data obtained not from the subject: the card and invitation a tutor creates | 🟡 | article 18 parts 3 and 4 | 🟠 | Decided: one line in the invitation email (decision 6); pending its specification and code change |
| P1 | Unaccepted invited cards keep the email without a bound | 🟡 | article 5 parts 4, 5 and 7 | 🟡 | Decided: 90 days (decision 3); pending its specification and code change under the constraints above |
| P1 | Harm assessment not made | 🔴 | item 5 of part 1 of article 18.1, regulator order 178 | 🟡 | Assess per subject category (expected low to medium) and sign the document |
| P1 | Local acts and the list of data and systems | 🔴 | item 2 of part 1 of article 18.1 | 🟡 | A short regulation on processing and the list of processed data |
| P1 | Protection level not determined | 🟡 | article 19, Government Decree 1119, FSTEC order 21 | 🟠 | Determine the level (expected 3 or 4 `[verify]`) and map the threat model's measures to it; information-security work |
| P1 | Subject request procedure | 🔴 | article 14 part 3, article 20 part 1, article 21 | 🟡 | A contact address, a manual procedure and templates; the response term, 10 working days extendable by 5, is `[verify]` |
| P1 | Destruction confirmation | 🔴 | regulator order 179 | 🟡 | An act plus an extract of the system's event log for each purge, covering the backups that keep purged rows up to 31 days; whether the purge job's logs give such an extract is ⚪ |
| P1 | Breach response, both channels | 🔴 | article 21 part 3.1; article 19 part 12 | 1–3 million roubles reported for a legal entity for not notifying `[verify]` | A runbook: the regulator within 24 and 72 hours, and the state system for detecting computer attacks (GosSOPKA); how a small operator connects to the latter `[verify]` |
| P1 | Minors 14–17 for the pilot with school students | 🔴 | articles 9 and 6; Civil Code articles 26 and 28 | 🟠 | Proposal 0003 to a decision record after a specialist's answer; OVC has no age gate, so until then only an owner decision keeps school students out of the pilot |
| P2 | Internal control | 🔴 | item 4 of part 1 of article 18.1 | 🟢 | A yearly self-check against this table |

### In place or not applicable

- Localization in Russia (article 18 part 5 as amended by 23-FZ).
- No special categories collected on purpose (article 10), though chat text may carry any data.
- No biometric identification (article 11); recordings come after the MVP.
- Nothing published for an unlimited audience (article 10.1).
- No decisions based solely on automated processing with legal effect (article 16), since automatic scores reach only the tutor.
- No anonymised data sets (233-FZ).
- Not critical information infrastructure (187-FZ).
- No employees, so no staff contour.
- No staff to familiarise (item 6 of part 1 of article 18.1).
- No transfer abroad now; web push needs, before enabling, a separate notification under article 12 as amended by 265-FZ, 10 working days, the recipients' information and a recheck of the adequacy list.

### Unknown

- Whether a student's access request covers a tutor's private notes.
- Whether the purge job's logs serve as the event-log extract of regulator order 179.

## Triage of tutor invitations, 2026-11-23

A tutor types a prospective student's name and, optionally, email into a student card; OVC emails the invitation link. Result: allowed with conditions.

- **Data:** ordinary data, a name and an email, received from the tutor, not the subject; the tutor's private notes may follow once the card exists. No special categories intended.
- **Basis:** item 7 of part 1 of article 6, the legitimate interests of the operator and of the tutor as a third party, balanced against the student's rights, is a stronger footing than the tutor's warranty alone; the warranty in the user agreement and the 90-day email bound support the balance `[verify]`. Consent is impossible before contact, and item 5 does not apply before the student is a party.
- **Notice of article 18 part 3 before processing:** the invitation email's line names the operator, the purpose and basis, the source (the tutor, already named in the email), and links to the policy for the rights and the users of the data. Whether the operator's address must be in every email or a contact is enough is `[verify]`. A card without an email gets no notice because there is no contact; the invitation page links the policy. Whether notice in the first email meets "before processing starts", given that the email is stored a moment earlier, is `[verify]`.
- **Minors:** an invited person may be under 18; there is no age gate, so keeping school students out of the pilot until proposal 0003 is decided is an owner decision, not a control.
- **Localization, processors, cross-border:** data stays in Russia; the mail provider is a processor under an instruction; no transfer abroad.
- **Policy coverage:** the policy outline covers the purpose, the source and the 90-day bound.
- **Also from this pass:** the content list of article 9 part 4, processors included, is mandatory for consent in written form, not for every confirmable consent; OVC's consent text keeps the list anyway as good practice.
