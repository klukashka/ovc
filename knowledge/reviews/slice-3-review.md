# Slice 3 review

The slice review of [roadmap slice 3](../../docs/roadmap.md#slice-3-tutors-students-and-groups), run on 2026-10-24 and 2026-10-25 over everything merged after the commit "Record the slice 2 review and close the slice" (`c2d5c15`) up to the commit "Add the groups pages, ending and Stop studying, and the deletion consequences" (`0d9fc60`): 23 commits, 240 files.

## Scope and reviews

One fresh independent reviewer per changed layer, against the clean `main` tree, with the Go integration tests on the local PostgreSQL and storage server and the Playwright suites against the per-worker deployments ([ADR-0011](../../docs/decisions/0011-tiered-verification.md)):

| Layer | Components | Profiles | Result |
|---|---|---|---|
| Backend | Migrations 00004 and 00005, the tutoring and notification services, the field cipher, the tutoring routes and schemas, the worker jobs, `ovc-admin` | Correctness, performance, with two descendant correctness reviews (join requests, reclaim and acceptance; deletion, purge, ending, groups and the outbox) | No blocker or major; nine minor findings |
| Frontend | The shared interface components, the frame and access rules, the auth, students, groups, tutors, home and settings features, the typed client and the transport fake with its contract | Correctness, browser, performance, with a separate measurement pass | One major (search on the Students page lost focus); eleven minor findings |
| Security across the slice | Four trust-boundary families: tokens in links and email; authentication and account state; authorization and relationship scope; dependencies, secrets, the test control surface and diagnostics | Security | No blocker or major; minor findings in every family |
| Development tooling and agent configuration | `AGENTS.md`, `.claude/`, `tools/`, lint and test configuration, the end-to-end harness, dependency triage, the check-budget records | Correctness, security | No blocker or major; eight minor findings |

Integration boundaries were reviewed by the layer on each side: the generated OpenAPI client by the backend (API side) and the frontend (client side); the outbox rows, the worker jobs and the tutor profile lock by the backend; the transport fake against the real backend by the frontend; the agent configuration's security impact by the security reviewer.

Every affected threat was reported as examined, not affected or unverified: T-1, T-3, T-4, T-5, T-6, T-7, T-8, T-18, T-19, T-20, T-21, T-22, T-23, T-27, T-29 (pending slice 4), T-31, T-33, T-34, T-36, T-37, T-39, T-41 and T-44. The threat model's amendments pending owner review were checked against the code; the mismatches found are fixed below.

## Fixed in follow-up changes

The follow-ups were approved by the owner as [the slice 3 review follow-up plan](../plans/slice-3-review-follow-ups.md). Each change had a standard reviewer, and a security-profile reviewer where it met the security trigger; a fixed blocker or major from a re-review waited for the owner's decision on another reviewer.

- **"Re-check the tutor's state and code under the profile lock on every tutor write":** accepting and declining join requests, regenerating the code, removing a provisional student, ending, withdrawing a card and the tutor's unblock re-check under the profile lock that the tutor is still active; code entry and code sign-up compare the code read under the lock; operator unblock and restore take the profile lock; a lost invitation link mid-transaction answers the ordinary `202`; the pending-request limit is checked before the block (removing a T-33 residual); overdue held requests expire at their own expiry; the denial suite covers a student shared with the intruding tutor; new race tests.
- **"Queue tutor deletion emails with one statement and return only the ended identifiers":** `DELETE /users/me` for a tutor with 50,000 relationships went from 10.2 s to 0.95 s (indicative, loaded machine), with identical outbox rows.
- **"Hide a deleted student's account from the tutor until the purge":** the owner's decision; T-34 amended.
- **"Fix focus, busy markers, refusals and stale fields found in the slice 3 frontend review":** the major focus loss on the Students search, busy markers on the control that started an action, explained refusals for malformed address values and stale unblocks, forms that keep unsaved typing and show current values after a reload, content held back until join requests settle instead of shifting the layout, and the transport fake's new routes checked against the real backend.
- **"Tighten the agent configuration and testing exceptions after the slice 3 review":** two narrow testing exceptions in `AGENTS.md` and ADR-0009 item 8, the reflection ban extended to TypeScript and JavaScript with an ESLint rule, confirmation for every `git checkout` with branches switched only through `git switch`, and wider confirmation rules for forced, amending and discarding options; T-41 amended.
- **"Serialise end-to-end runs and enforce the pairing of ignored advisories":** an exclusive run lock with a Playwright output folder per run, a shared ESLint instance in the lint rule tests, the security scans in the full check run, and a lint check that every `pnpm audit` ignore has an unexpired OSV entry; T-39 amended.

## Accepted minor findings

Accepted by the owner on 2026-10-24 with the follow-up plan and on 2026-10-25 at the close of the slice; each is recorded with its reason in [slice 3 known limitations](slice-3-known-limitations.md):

- the student list reads and sorts all matching cards (3.5 ms at 50,000 cards);
- signed-in acceptance reads the account email outside the row lock, which an email-change feature must move under the lock ([accounts](../specs/accounts.md));
- two endings of one relationship in the same microsecond share an idempotency key, and the ending services read the clock before their transaction;
- `join_requests.expire` expires at most 1,000 requests per hourly run;
- several writes rely on a scoped lock taken earlier in the same transaction instead of repeating `tutor_id`;
- the single bundle grew 16% in slice 3 and stays inside the student budget;
- tutor deletion still runs in one unbounded transaction, now 0.95 s at 50,000 relationships;
- the group dialogs' `not_found` refusal has no test, because only a student purge produces it;
- tutor deletion builds partly filled `Relationship` values for its logging;
- the `Reflect` lint rule does not catch a lookup through `Object.getOwnPropertyDescriptor`;
- the Git confirmation rules match the command text only (T-41 residual);
- three end-to-end runs starting at once over a stale run lock can race, which is detected rather than prevented.

## Carried over

| Item | Where it goes |
|---|---|
| zod 4.6.6, puppeteer 25.13.0 and dompurify 3.4.17 before their triage entries expire on 2026-10-27; undici 8.12.2 after 2026-10-29 with the `pnpm audit` ignores removed; Node 26.12.1 or later from 2026-11-04; a dated research entry for the DOMPurify copies bundled inside mermaid and ZenUML | The dependency upgrade change of the follow-up plan, before slice 4 adds code |
| The full local check run above the slice plan's 180 s threshold, and the journey-style end-to-end tests outside the pilot scenarios that interleave act and assert | The suite consolidation change of the follow-up plan, before slice 4 adds tests, measured on an idle machine |
| One large tutor purge delaying the purge of other accounts by whole hourly runs | Slice 5, where lessons and series make tutor purges larger |
| Route-level splitting of the bundle | Before slice 5, with the slice 8 code-splitting item |

## Performance baselines

Measured during the review on the development machine (Apple M4 Pro, 12 cores); the machine was not idle, so these are indicative, not budgets.

- **Backend:** the student list at 50,000 cards answers in about 6 ms (first page) and 10 ms (search); join-request lists at 100,000 requests in about 2 ms; `GET /groups` with 200 groups in about 10 ms and 185 KB; tutor deletion as above.
- **Frontend bundle:** 185 KB gzip of JavaScript and 7.3 KB of CSS in one chunk, against 159 KB and 6.3 KB at the end of slice 2 and the 250 KB student budget in NFR-2.4.
- **Loading:** data-ready pages in 88 to 150 ms in Chromium at desktop and phone widths; a throttled phone profile reached Home in about 0.7 s, inside NFR-2.5.
- **Interactions:** under 140 ms in every browser for Students "Show more" to 1,000 cards, search, tab switches, join-request decisions and the member picker.
- **Leaks:** 100 open-and-close cycles of every new dialog and menu and of card navigation left listeners, DOM nodes and query-cache entries flat; the heap levelled off over 800 cycles.
- **Local checks:** 195 s in total during the review (see the carried-over consolidation).

## Unverified

- Behaviour behind the production reverse proxy and on the pilot deployment, which do not exist yet.
- Layout shift, input latency and heap metrics in Firefox and WebKit, which lack the measuring APIs; throttled profiles in Firefox and WebKit.
- A cold run of the full check suite on an idle machine.
- Live WebSocket closing on ending, removal and reclaim (T-29), pending slice 4.
