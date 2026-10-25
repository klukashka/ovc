# Slice 3 review follow-ups

Written on 2026-10-24, after the slice review of [roadmap slice 3](../../docs/roadmap.md) over everything merged after the commit "Record the slice 2 review and close the slice" (`c2d5c15`) up to "Add the groups pages, ending and Stop studying, and the deletion consequences" (`0d9fc60`). The owner approved this plan on 2026-10-24. Each change below is a standard-tier change with its own reviewers under [ADR-0011](../../docs/decisions/0011-tiered-verification.md), names the slice review findings it resolves, and has its reviewer confirm them closed. The review record `knowledge/reviews/slice-3-review.md` closes the slice once changes 1 to 6 have merged; changes 7 and 8 are bound to dates and to the start of slice 4.

## Owner decisions

- A deleted student's account (name and email) is hidden from the tutor as soon as the account is deleted, on ended cards and in the blocked students list; the tutor keeps the card's own name, email and notes until the purge. `knowledge/specs/accounts.md` and T-34 change with it.
- AGENTS.md gains a narrow testing exception: a test may create database objects in its own test database that raise a real database error, only to prove failure isolation in worker jobs.
- AGENTS.md gains a narrow exception to the three-phase test layout for the steps of the pilot scenario journeys, whose intermediate states are the acceptance criteria; other journey-style end-to-end tests are split into tests with three phases in the consolidation change.
- The AGENTS.md ban on reflection covers TypeScript too: no `Reflect` API in hand-written TypeScript, tests and end-to-end page scripts included, enforced by ESLint (decided on 2026-10-25 while the agent configuration change was prepared).
- `.claude/settings.json` asks for confirmation before every `git checkout`, and agents create and switch branches only with `git switch`; the confirmation rules also cover `git checkout <ref> -- <path>`, `git checkout -B`, `git switch -C`, `git worktree remove --force` and the forced, amending and discarding options wherever they appear in a Git command (widened on 2026-10-25 after the agent configuration review).

## Changes in order

1. **`fix/tutor-state-rechecks`.** Accepting and declining a join request re-check under the tutor profile lock that the tutor is still active; code entry and sign-up with a code compare the entered code with the code read under the profile lock; code regeneration re-checks the tutor's state; overdue held and owner-choice requests are expired with `decided_at = LEAST(expires_at, now)`; unblock and restore of a tutor take the profile lock; an invitation sign-up whose link stops being usable inside the transaction answers `202` for a new email as for a taken one; the 50-request cap is checked before the block, which removes the T-33 residual; `FieldCipher.Format` handles a failed write as the fail-fast rule requires; the denial suite gains cases where the intruding tutor shares the student; tests race removal and Stop studying against tutor deletion and assert the `decided_at` values tutor deletion writes; `waitForLockWaiters` waits on the contested lock in `pg_locks`; the slice 3 plan's acceptance list records the renamed tests; the observability specification matches the logged tutoring fields.
2. **`fix/tutor-deletion-batching`.** Tutor deletion queues its `tutor.deleted` rows with one statement and returns only the columns it needs.
3. **`fix/hide-deleted-student-account`.** The owner decision above.
4. **`fix/frontend-slice-3-review`.** Search and clear search on the Students page keep focus; the row control that started an action announces that it is busy; a malformed `group=` value gets its own message; an unblock answered `not_found` is not reported as done; edit forms show current values after a changed-meanwhile reload; empty Students tabs and searches keep "Add student"; "Show more" keeps focus when a fetched page is filtered out; the transport contract covers the fake's refusal of a student on card routes; the account edge-case and accessibility specs use the diagnostics fixture; join requests no longer shift the Students and Today layouts as they load.
5. **`chore/agent-config-after-slice-3`.** The two AGENTS.md exceptions (also item 8 of ADR-0009), the reflection ban extended to TypeScript and JavaScript with an ESLint rule, the confirmation rules above widened to every `git checkout` with branches switched only through `git switch`, the T-41 text-matching residual, and the focus rule's account-change clause backed by the navigation design.
6. **`chore/e2e-harness-after-slice-3`.** An exclusive run lock in the end-to-end harness, with a Playwright output folder per run, because Playwright clears its output folder before the global setup can take the lock (approved by the owner on 2026-10-25); the advisory pairing check runs in `pnpm lint` against the real date, so lint fails on the day an ignore expires, as the OSV scan does; one shared ESLint instance in the lint rule tests; the check-budget records include the security scans; a check that each `auditConfig.ignoreGhsas` entry has a dated OSV entry.
7. **`chore/dependency-upgrades`.** zod 4.6.6, puppeteer 25.13.0 and dompurify 3.4.17 once they are two weeks old (2026-10-25 and 2026-10-26), before their triage entries expire on 2026-10-27; undici 8.12.2 after 2026-10-29 with the `ignoreGhsas` entries removed by hand; Node 26.12.1 or later from 2026-11-04; a dated research entry for the DOMPurify copies bundled inside mermaid and ZenUML.
8. **`chore/consolidate-suites`.** Before slice 4 adds tests: the end-to-end and Go suites are consolidated back under the 180 s threshold of the slice 3 plan, the journey-style tests outside the pilot scenarios are split, and the full check run, including the security scans, is measured on an idle machine.

## Accepted

- The student list reads and sorts all matching cards: 3.5 ms at 50,000 cards.
- Two endings of one relationship in the same microsecond share an idempotency key.
- The ending services read the clock before their transaction.
- Signed-in acceptance reads the account email outside the row lock; `knowledge/specs/accounts.md` records that an email-change feature must move the binding check under the lock.
- `join_requests.expire` expires at most 1,000 requests per hourly run.
- Several writes rely on a scoped lock taken earlier in the same transaction instead of repeating `tutor_id`.
- The single bundle grew 16% in slice 3 and stays inside the student budget; route-level splitting lands before slice 5.

## Carried into slice 5

- One large tutor purge can delay the purge of other accounts by whole hourly runs; each account gets its own share of the run's budget.
