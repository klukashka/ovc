# ADR-0009: Owner-approved exceptions to base rules, and tutor-only scores

- Status: accepted, 2026-10-20; superseded in part by [ADR-0014](0014-students-join-through-their-tutor.md) on 2026-10-22 (the first bullet of item 4)
- Reviewed: decided by the project owner on 2026-10-20, item 7 on 2026-10-21; wording pending line-by-line owner review; the superseded-in-part note of 2026-10-22 pending owner review
- Deciders: project owner
- Related: [ADR-0004](0004-bigbluebutton-primary-provider.md), [ADR-0005](0005-tutor-centric-domain.md), [ADR-0006](0006-background-worker-and-email.md), [ADR-0014](0014-students-join-through-their-tutor.md), FR-1.2, FR-5.1, FR-5.3, FR-9.3, FR-11.1, [threat model](../threat-model.md)
- Superseded parts: the first bullet of item 4, under which only an invitation sent by email confirmed the invitee's email; since [ADR-0014](0014-students-join-through-their-tutor.md) no invitation confirms an email, and a link accepted before confirmation is provisional. The text below is kept as written for the record.

## Context

The design reviews of the tutor-centric product found places where the specifications relax rules that the first version of `AGENTS.md` stated in absolute terms. They also found product refinements that go beyond the plan the owner approved. The verification policy keeps the stricter base rule until the owner explicitly approves a reduction. This record is that approval, so that future reviews do not reopen it.

## Decision

The project owner approved, on 2026-10-20 (items 1 to 6) and 2026-10-21 (item 7):

1. **Credentials in URLs** are allowed for exactly these, each bounded in lifetime and scope, never signing anyone in to OVC, and never logged:
   - invitation links;
   - email-confirmation links;
   - password-reset links;
   - unsubscribe links;
   - per-request conferencing join links;
   - 5-minute signed storage URLs for authorized downloads, recordings and the lesson presentation fetched by the conferencing server.

   Access and refresh tokens never appear in URLs.
2. **Infrastructure adapters may keep their own state in PostgreSQL:** the `NOTIFY` broadcaster, the database mail sink and the shared test clock. Business data is still read and written only by repositories.
3. **Slow WebSocket consumers are closed** (`4429`) and recover by reconnecting and refetching through REST. This replaces any guarantee that every frame reaches every connected client; the database stays the source of truth.
4. **Product refinements:**
   - only an invitation sent by email from OVC confirms the invitee's email; a copied link, or an invitation without an email, requires confirming the email first;
   - the video room opens in a new tab until the slice 7 probe confirms that embedding works on the supported browsers;
   - students join a lesson's room only after the tutor has joined.
5. **Agents commit and merge their own work** into `main` once its checks pass and its readiness reviews are resolved, one commit per branch with fast-forward merges. Push, rebase, reset, clean, forced branch deletion, destructive checkouts and history rewriting stay with the owner and require confirmation.
6. **Scores are tutor-only.**
   - Students never see points, totals, overrides, scores or percentages, before or after checking.
   - They see the status of their work, the tutor's comments, feedback and corrected files, and, when the answer policy allows, the correct answers with explanations.
   - Scores and analytics are the tutor's working tools.
7. **Agents may run local commands with the owner's development settings** until OVC has hosting. The owner keeps the root `.env` (Compose credentials) and `backend/.env` (backend settings) with development-only values, and edits and later moves them. Agents never open, print, copy or edit these or any other environment file; the only sanctioned use is `tools/dev-env.sh <command>`, which loads those two files into that one command's environment without printing them. Other environment files, such as `backend/.env.local` with real provider secrets for live checks, are never loaded by the wrapper and stay owner-only. The exception ends when hosting exists; then the wrapper, its check `tools/check-dev-env.sh`, the hook's support for it with its cases, and their documentation in `AGENTS.md`, the threat model, the development guide and the AI development inventory are removed.

## Alternatives considered

- **Keeping the absolute rules:** emails could carry no working links, files and recordings would need an authenticated streaming proxy, and live delivery would need unbounded per-socket buffers.
- **Showing students a total or a final score:** familiar from schools, but it can reveal answer keys before release, and the owner prefers that adult students focus on feedback.

## Consequences

- `AGENTS.md`, the threat model and the specifications state these exceptions as approved, and reviews treat them as the base.
- The answer-key contract test also asserts that no points or score field reaches any student-facing response, frame, notification or email.
- Pages and pilot scenarios show no scores on student surfaces.
- Commands run through `tools/dev-env.sh` hold the development settings in their environment; the owner keeps only development values in the two files it loads.

## Where it is applied

- [AGENTS.md](../../AGENTS.md#security)
- [Answer visibility](../../knowledge/specs/homework.md#answer-visibility)
- [Invitations](../../knowledge/specs/accounts.md#invitations)
- [Join rule](../../knowledge/specs/schedule.md#join-rule)
- [Embedding](../../knowledge/specs/realtime-and-video.md#embedding)
- [Broadcaster](../../knowledge/specs/realtime-and-video.md#broadcaster)
- [Package layers](../../knowledge/architecture/detailed.md#package-layers)
- [Functional requirements](../requirements/functional.md) (FR-1.2, FR-5.1, FR-5.3, FR-9.3, FR-11.1), [product overview](../product.md)
- [Pages](../../knowledge/design/web/pages.md), [states](../../knowledge/design/web/states.md), [pilot scenarios](../../knowledge/design/web/pilot-scenarios.md)
- [Email content](../../knowledge/specs/notifications.md#email), [threat model](../threat-model.md) (T-19, T-30)
- [Change workflow](../../AGENTS.md#change-workflow), [AI development verification](../../knowledge/reviews/ai-development-setup.md#minimal-inventory)
- [Secrets](../../AGENTS.md#security) and the [local settings wrapper](../development.md#local-services) (item 7), [threat model](../threat-model.md) (T-19)
