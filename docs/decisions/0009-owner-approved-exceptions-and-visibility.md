# ADR-0009: Owner-approved exceptions to base rules, and tutor-only scores

- Status: accepted, 2026-10-20; superseded in part by [ADR-0014](0014-students-join-through-their-tutor.md) on 2026-10-22 (the first bullet of item 4)
- Reviewed: decided by the project owner on 2026-10-20, item 7 on 2026-10-21, item 8 on 2026-10-25 (its listener-forwarder and goose statement-marker exceptions also on 2026-10-25), and the rewording of item 2 on 2026-10-25; wording pending line-by-line owner review; the additions to item 8 and the rewording of item 2 pending owner review; the superseded-in-part note of 2026-10-22 pending owner review; the end-of-stream relay of the listener forwarder and its package path `internal/testdb/forwarder` in item 8 amended on 2026-10-26, pending owner review
- Deciders: project owner
- Related: [ADR-0004](0004-bigbluebutton-primary-provider.md), [ADR-0005](0005-tutor-centric-domain.md), [ADR-0006](0006-background-worker-and-email.md), [ADR-0014](0014-students-join-through-their-tutor.md), FR-1.2, FR-5.1, FR-5.3, FR-9.3, FR-11.1, [threat model](../threat-model.md)
- Superseded parts: the first bullet of item 4, under which only an invitation sent by email confirmed the invitee's email; since [ADR-0014](0014-students-join-through-their-tutor.md) no invitation confirms an email, and a link accepted before confirmation is provisional. The text below is kept as written for the record.

## Context

The design reviews of the tutor-centric product found places where the specifications relax rules that the first version of `AGENTS.md` stated in absolute terms. They also found product refinements that go beyond the plan the owner approved. The verification policy keeps the stricter base rule until the owner explicitly approves a reduction. This record is that approval, so that future reviews do not reopen it.

## Decision

The project owner approved, on 2026-10-20 (items 1 to 6), 2026-10-21 (item 7) and 2026-10-25 (item 8 and the rewording of item 2, their wording pending owner review):

1. **Credentials in URLs** are allowed for exactly these, each bounded in lifetime and scope, never signing anyone in to OVC, and never logged:
   - invitation links;
   - email-confirmation links;
   - password-reset links;
   - unsubscribe links;
   - per-request conferencing join links;
   - 5-minute signed storage URLs for authorized downloads, recordings and the lesson presentation fetched by the conferencing server.

   Access and refresh tokens never appear in URLs.
2. **Infrastructure adapters may keep their own state in PostgreSQL:** the database mail sink and the shared test clock. Business data is still read and written only by repositories. The broadcaster keeps no state of its own (pending owner review): its signals are written with `pg_notify` inside the committing transaction by the `Signals` repository and by database triggers, and the PostgreSQL listener adapter only holds a listening connection.
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
8. **Four narrow exceptions to the testing and code rules** (pending owner review): a step test of a pilot scenario suite, whose title starts with `pilot-<n> › step <k>` or `pilot-<n> › steps <k>–<m>` and names the scenario steps it proves (a `pilot-<n> › seed` test is not one), may act and assert in turns, because the intermediate states of a scenario step are its acceptance criteria; a test may create, through its own connection to the test database created for it, a database object whose only effect is to raise a real database error for one item, never through application code, migrations or the end-to-end test control surface, only to prove that a worker job isolates that item's failure; only in tests of the database listener's liveness (its heartbeat and the detection of a silent connection), a test may put an in-process TCP forwarder in front of the test PostgreSQL, because a silent connection cannot otherwise be produced without patching the driver: `internal/testdb/forwarder` builds it from the database endpoint the integration-test fixture has already checked, it listens only on `127.0.0.1` on an ephemeral port, dials only that checked host and port and relays every byte unchanged in both directions, passing an end of stream (end of file or a read error) from one side on to the other by half-closing the same direction and doing nothing else to that connection, which is part of a faithful relay (a reset reaches the other side as an ordinary end of stream) (amended 2026-10-26, pending owner review), and its only other behavior is to stop relaying in both directions at once and permanently, after which it passes nothing on, not even an end of stream, and may close both connections, never resuming, delaying or dropping individual bytes; and `backend/migrations/*.sql` may hold `-- +goose StatementBegin` and `-- +goose StatementEnd`, each alone on its line, paired, and only around one `CREATE FUNCTION` or `CREATE OR REPLACE FUNCTION` statement whose dollar-quoted body holds at least one semicolon at which goose would otherwise split it, enforced by the conventions tests. Every other test keeps three phases, changes nothing about how the database behaves and alters nothing about the database connection's behavior, and no other comment-like marker enters a migration.

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
- [Testing](../../AGENTS.md#testing), [Code](../../AGENTS.md#code) and the conventions tests in `backend/internal/conventions/` (item 8)
- [Change workflow](../../AGENTS.md#change-workflow), [AI development verification](../../knowledge/reviews/ai-development-setup.md#minimal-inventory)
- [Secrets](../../AGENTS.md#security) and the [local settings wrapper](../development.md#local-services) (item 7), [threat model](../threat-model.md) (T-19)
