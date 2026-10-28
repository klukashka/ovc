# Review of proposal 0003, students aged 14 and over

Reviewed 2026-10-28. The change adds [proposal 0003](../../docs/proposals/0003-students-aged-14-and-over.md), the [research on consent practice](../research/minors-consent-practice-2026-10-28.md) and a row in the [proposals index](../../docs/proposals/README.md). It is a Markdown-only change at the standard tier that meets the security trigger: it designs how OVC handles personal data of minors aged 14–17, the legal basis and consent, an age gate at sign-up and operator tooling.

## Reviews

| Review | Scope | Profiles | Result |
|---|---|---|---|
| Plan review | the task plan | security | 3 major, 7 minor, 2 nit; all folded into the plan before the owner approved it |
| Diff review | the complete diff | standard | 12 minor and nit; all fixed |
| Diff review | the complete diff | security | 2 major, 6 minor, 1 nit; all fixed |
| Re-review of the fixes | the fixes | security and standard | every earlier finding closed; 1 new major, 2 minor, 2 nit; all fixed |
| Re-review after the owner's choice | the last fixes | security and standard | every earlier finding closed; 1 new minor; fixed |

After the first re-review found a major finding, the owner chose another security-profile re-review before merging.

## Findings

| Finding | Severity | Resolution | Status |
|---|---|---|---|
| A squatter's band and guardian assertion survive a reclaim (T-23, T-44) | major | a reclaim clears both and the mailbox owner states them again; a tutor never sets or prefills them | fixed |
| No path for a guardian's withdrawal or deletion request, or an account found to be under 14; blocking keeps data indefinitely (T-22, T-34) | major | out-of-band operator process; audited `ovc-admin` subcommands to delete with purge and to lower the band; a ninth question for the lawyer | fixed |
| Adult–minor contact controls overstated (T-21, T-33, T-6) | major | new actor; residual risks stated and accepted by the owner, with a complaint address | fixed |
| An existing account answering "under 14" had no outcome | major | refused unless confirmed; then deleted and purged at once, not restorable | fixed |
| Band writes not under the account row lock and session re-check (T-23, T-44) | major | both writes lock the account row and re-check the session; a request authenticated before a reclaim answers `401` | fixed |
| Answering "under 14" on an existing account destroyed data with a session alone | major | requires the current password and an explicit confirmation, as account deletion does | fixed |
| Under-14 refusal ordering on both sign-up routes, enumeration and diagnostics (T-3, T-8, T-31, T-18) | minor | ordering after the per-address slot on both routes; no write, email, mismatch count or revocation; count-only event | fixed |
| Band visibility surfaces (T-5, T-6, T-18) | minor | only the student and tutors who already see the linked account; never in emails, notifications, push, the provider or logs; contract test | fixed |
| Band change records, residual of claiming adulthood, tutor 18+ confirmation records | minor | times and versions recorded; residuals stated | fixed |
| Web push for ages 14–17 (T-46) | minor | same opt-in as adults after the gate; any restriction a server-side refusal and a policy limit | fixed |
| Existing accounts without a band; no age gate until implemented | minor | stated before any other route; the pilot admits no school students before the implementation | fixed |
| Operator subcommand details and the impostor guardian (T-22) | minor | purge timing, restore refusal, assertion after lowering, legacy tutors; no action or disclosure before the guardian's authority is checked | fixed |
| Exemption list of the gate | minor | refresh, signing out everywhere, email confirmation and resend, password reset and change, deletion, `GET /users/me`, the band and tutor confirmation routes | fixed |
| No limit on wrong current passwords on the under-14 confirmation | minor | counted in the password-change counter, `429` beyond it | fixed |
| Accuracy: scores and parents attributed to ADR-0009, the web push gate, the documents to update, the research summarised fairly | minor | wording corrected | fixed |
| Research labels carrying agent judgements; unreadable pages; the VK table cell | minor and nit | inferences marked as the agent's, relabelled `[unconfirmed]` where no source states them | fixed |
| Acceptance criteria not checkable by a command | minor | replaced with commands | fixed |
| Status `proposed` before the owner's line-by-line review | nit | the owner chose `proposed`, wording pending owner review | fixed |

No finding is open.

## Outside this change

Account deletion (`knowledge/specs/accounts.md`, account deletion) checks the current password without a limit on wrong guesses, unlike a password change. The re-review noted it as a separate change; it is not part of this proposal.
