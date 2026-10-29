# Check budget: cost of the end-to-end suites per spec, per test and per arrangement

Measured on 2026-10-29 for step 1 of [proposal 0004](../../docs/proposals/0004-end-to-end-suite-cost.md), change E1 of the [slice 6 plan](../plans/slice-6.md). The record changes no suite, check or budget. It gives, per spec and engine, the summed test time, the tests that share each arrangement, the measured fixed cost of a test under the gate's load, and the browser tests that are candidates for moving to Go integration tests. It replaces the proposal's estimates, ranks its techniques by measured gain and lists, for each of the changes E2 to E6, the consolidations that change can start from. These are not budget measurements: no merge gate ran, because the gate runs only as part of a merge.

## Method

- **Machine:** the 12-core machine of the [previous profile](check-budget-2026-10-29-e2e-profile.md) (8 performance and 4 efficiency cores, macOS), with the PostgreSQL and S3-compatible containers of the development guide in Docker Desktop's virtual machine.
- **Tree:** `main` at `fe932ae`, 897 tests: 326 in Chromium, 285 in Firefox and 286 in WebKit. `tools/dev-env.sh pnpm e2e --list` printed `Total: 897 tests in 23 files`. No test file, fixture or configuration was changed, so the listing is unchanged. As guards for E2 to E6, `grep -o 'expect(' frontend/e2e/*.ts | wc -l` printed 1291 and `grep -o 'expect.poll(' frontend/e2e/*.ts | wc -l` printed 10.
- **Runs:** three runs of `tools/dev-env.sh pnpm e2e --reporter=list,<cost reporter>` from the `frontend/` folder of a git worktree, each started only while the end-to-end run lock was free and no merge gate ran. They used Playwright's default of 6 workers, as the merge gate does, so every figure below is taken under the gate's load: the suites saturate the machine themselves. Every run passed.
- **Cost reporter:** a Playwright reporter kept outside the repository. For every test it writes the start, duration, worker and status; for every hook, fixture, browser call and top-level assertion it writes the start offset within the test and the duration. It writes no page content, request, address or account.
- **Parts of a test:**
  - *context* and *page* are the `context` and `page` fixtures of the before hooks;
  - *sign-in* is the `signedInPage` fixture, the only place where signing in is timed on its own: a request to the test control surface's session route and a call that adds the refresh cookie to the context. The 19 tests per engine that use it are in three specs (`smoke` 14, `session` 4, `signup` 1). This path applies to accounts seeded through the test control surface, which nearly all tests use; an account registered through the API or a page signs in through a page and the login route instead, and that path was not timed;
  - *seeding and sign-in* is the time from the end of the before hooks to the start of the test's first navigation, which holds the test's own seeding through the API and its sign-in;
  - *first navigation* is the first `page.goto` of the test body, until the page's load event; the rendering of the page's data after the load event is counted in the test's first wait or assertion;
  - *close* is the `context` fixture of the after hooks.
- **Firefox's stall:** a context close of 150 ms or more counts as one of Firefox's every-tenth-close stalls ([previous profile](check-budget-2026-10-29-e2e-profile.md#item-b-firefoxs-overhead-profiled-at-ba042d8)). Every other Firefox close took at most 146 ms; every stall but one (162 ms) took at least 0.66 s.
- **CPU and load:** `ps` sampled every 2 s with `%CPU` summed per process family, and `uptime` before and after each run, as in the previous profile.
- **Arrangements:** read from the five largest specs: the seeding helpers and fixtures each test calls, the address it opens first and the action of its act phase.
- **Gain of removing a test:** its measured duration, less the duration of its top-level assertions (which move into the test that absorbs it), less its own context close plus its engine's mean close (so that a Firefox stall that happened to land in it is replaced by the mean cost of one close), averaged over the three runs. A merge that adds a page to the absorbing test also subtracts the engine's median page creation per added page. Totals are sums of the unrounded gains, rounded once. Wall-time gains divide the summed-time gain by 5.5, the measured mean number of tests running at once (summed test time over the span from the first test's start to the last test's end: 5.50, 5.49 and 5.48 in the three runs).

## Runs

| Run | Wall time | Load before → after | Summed test time C / F / W | Median test C / F / W | Firefox stalls (count, summed) | CPU: all, Firefox, other, Docker's VM |
|---|---|---|---|---|---|---|
| 1 | 153 s | 3.25 → 25.27 | 170.2 / 401.5 / 231.1 s | 0.50 / 1.08 / 0.73 s | 28, 78.7 s | 892 %, 26.0 %, 25.8 %, 13.9 % |
| 2 | 134 s | 14.81 → 25.42 | 162.2 / 340.5 / 224.4 s | 0.45 / 0.84 / 0.70 s | 25, 65.6 s | 870 %, 27.5 %, 19.3 %, 14.8 % |
| 3 | 140 s | 3.58 → 29.12 | 161.9 / 362.7 / 223.2 s | 0.46 / 0.93 / 0.70 s | 25, 66.5 s | 891 %, 25.9 %, 25.5 %, 13.7 % |

Mean wall time 142 s, spread 19 s. The span from the first test's start to the last test's end was 146.0, 132.5 and 136.3 s, so about 7 s of run 1 lay outside the tests against 1.5 and 3.7 s in runs 2 and 3: run 1 was the first in a fresh worktree, whose global setup built the binaries and the frontend. Runs 2 and 3 differ by 6 s. The other families took Playwright's runner and workers 10.7 to 11.9 %, WebKit 9.6 to 10.7 %, Chromium 6.5 to 7.1 %, `vite preview` 4.4 to 5.3 % and the API processes 3.0 to 3.4 %. "Other" (everything outside the suites) was 19 to 26 %, against 13 to 16 % in the previous profile: other agent sessions were open, and another project's test run was present but mostly idle during the runs; the run lock was free and no merge gate or other end-to-end run was active.

## Fixed cost of a test under the gate's load

Medians per test over the three runs, in milliseconds (the range is the three runs' medians):

| Part | Chromium | Firefox | WebKit |
|---|---|---|---|
| Context | 4 | 14–18 | 5 |
| Page | 36–40 | 144–171 | 143–145 |
| Sign-in through the API | 8–10 | 10–12 | 11–12 |
| Seeding and sign-in (a test's own arrangement before its first navigation) | 78–82 | 86–98 | 97–107 |
| First navigation | 67–72 | 136–164 | 87–92 |
| Close, median | 6 | 11–14 | 7–8 |
| Close, mean (Firefox's stalls included) | 7–9 | 243–291 | 9–10 |
| **Fixed cost: context, page, sign-in, first navigation and median close** | **about 125** | **about 345** | **about 260** |
| Fixed cost with the mean close | about 125 | about 590 | about 260 |

- Signing in a seeded account costs about 10 ms, because the seeded session is set as a cookie without a page; it is not the untimed part the proposal assumed. The measured fixed cost is 0.13 s in Chromium, 0.26 s in WebKit and 0.35 s in Firefox, or 0.59 s in Firefox when its stalls are spread over its closes. The proposal's estimate of 0.2 to 0.5 s per test and engine holds on average.
- A test's own seeding adds 70 to 95 ms at the median and more where it seeds lessons or several actors.
- In the five largest specs, the before hooks, the seeding and sign-in, the first navigation and the after hooks (without stalls) took 48 to 62 % of a test's time at the median.

## Firefox's context-close stall at 6 workers

Each Firefox worker ran 44 to 51 tests and stalled on every tenth close of its browser. Under the gate's load the stalls start at 0.7 to 1.0 s and grow to 3.7 to 5.8 s by the fourth or fifth stall, for example 0.7, 1.6, 2.8, 4.5 and 5.4 s on one worker of run 3. They summed to 65.6 to 78.7 s per run, 18 to 20 % of Firefox's summed time, against the previous profile's estimate of about 30 s. If the growth were removed and every stall stayed at its first size of about 0.75 s, about 45 to 58 s of summed time, 8 to 11 s of wall time, would return.

## Per spec and engine

Mean over the three runs: summed time / summed time without Firefox's stalls (tests), then the median fixed share of a test (before hooks, seeding and sign-in, first navigation and after hooks, without stalls).

| Spec | Chromium | Firefox | WebKit | All engines |
|---|---|---|---|---|
| `schedule` | 17.3 s (31), 52 % | 41.5 / 36.7 s (30), 59 % | 26.1 s (31), 58 % | 84.9 s |
| `students` | 13.8 s (26), 48 % | 43.2 / 28.5 s (26), 52 % | 18.4 s (26), 52 % | 75.4 s |
| `groups` | 15.9 s (24), 54 % | 28.5 / 25.5 s (24), 57 % | 22.1 s (24), 56 % | 66.5 s |
| `lesson` | 14.4 s (27), 52 % | 27.1 / 23.5 s (27), 57 % | 19.1 s (27), 56 % | 60.6 s |
| `signup` | 10.3 s (17), 36 % | 34.8 / 20.2 s (17), 43 % | 15.3 s (17), 43 % | 60.4 s |
| `join-requests` | 14.3 s (18), 62 % | 25.3 / 21.0 s (18), 60 % | 17.4 s (18), 59 % | 57.0 s |
| `account-edge-cases` | 9.6 s (19), 38 % | 17.0 s (19), 47 % | 13.7 s (19), 48 % | 40.3 s |
| `invitations` | 9.5 s (14), 43 % | 16.2 s (14), 51 % | 13.5 s (14), 50 % | 39.2 s |
| `ending` | 8.5 s (13), 47 % | 17.3 / 15.8 s (13), 45 % | 12.6 s (13), 55 % | 38.4 s |
| `smoke` | 7.2 s (22), 53 % | 19.7 / 16.6 s (22), 61 % | 11.1 s (22), 59 % | 38.0 s |
| `accessibility` | 8.1 s (15), 60 % | 15.6 s (15), 65 % | 10.8 s (15), 60 % | 34.5 s |
| `reclaim` | 5.8 s (8), 35 % | 19.3 / 11.3 s (8), 36 % | 8.6 s (8), 41 % | 33.7 s |
| `session` | 5.6 s (13), 44 % | 15.9 / 11.6 s (13), 49 % | 8.4 s (13), 46 % | 29.9 s |
| `notifications` | 5.1 s (10), 47 % | 11.5 / 9.8 s (10), 53 % | 7.4 s (10), 58 % | 24.0 s |
| `teaching-settings` | 4.5 s (7), 55 % | 10.4 / 7.2 s (7), 60 % | 5.8 s (7), 60 % | 20.7 s |
| `realtime` | 5.0 s (6), 46 % | 9.5 / 7.4 s (6), 45 % | 5.4 s (6), 44 % | 19.9 s |
| `live-updates` | 4.2 s (8), 49 % | 7.1 s (8), 56 % | 5.8 s (8), 51 % | 17.1 s |
| `email-settings` | 2.0 s (4), 54 % | 3.6 s (4), 62 % | 2.8 s (4), 66 % | 8.4 s |
| `language` | 1.0 s (3), 46 % | 4.1 / 2.4 s (3), 61 % | 1.4 s (3), 61 % | 6.5 s |
| `session-cookie` | 0.2 s (1) | 0.8 s (1) | 0.4 s (1) | 1.4 s |
| `session-transport`, `realtime-contract`, `api-contracts` (Chromium only, no browser) | 2.4 s (40) | — | — | 2.4 s |

Where a spec's Firefox figure shows a single number, no stall landed in it in any run. Which spec a stall lands in depends on the order of tests on a worker, so the Firefox figures with stalls vary between runs (`students` took 48.9, 47.5 and 33.1 s).

## Arrangements of the five largest specs

Counts are tests per engine. Most tests that share an arrangement differ in the action of their act phase. Where one test's arrangement and action are the first steps of another test's arrangement, the record calls the pair a prefix pair; the candidates below treat prefix pairs separately from technique 1.

### `schedule` (31; 30 in Firefox, which skips the coarse-pointer test)

| Arrangement | Tests | Actions |
|---|---|---|
| Maria with Anna (`tutorWithStudent`), tomorrow's week, no lessons | 9 | opening New lesson (dialog opens on its first field; Ends on; a series without weekdays; a saved lesson returns focus), a free slot (prefills the dialog; returns focus to New lesson), previous and next (3, one on the month page) |
| The same with a lesson at 15:00, New lesson filled for 15:30 and submitted, `409` expected | 4 | the warning (takes focus), Schedule anyway (saves; keeps focus while it saves), a plain resubmit while pending |
| The same with three short lessons | 3 | the week below 1024 px, on a coarse pointer (own context), a 15-minute lesson at 1024 px |
| The same with other lessons | 5 singletons | next week's scroll, focus across grid and list, month "+n more", a title with markup, a lesson scheduled elsewhere |
| Worker-shared `studentsOfPavel` | 1 | the audience picker's Show more |
| An unconfirmed student with a provisional link | 1 | Schedule shows its lessons |
| Pilot scenario steps (pilot 1 steps 6 and 7; pilot 3 seed and steps 3, 4, 5, 12, 13) | 8 | not consolidated |

### `students` (26)

| Arrangement | Tests | Actions |
|---|---|---|
| Pavel without students, invitation created through the dialog | 2 | Copy link; Open the card |
| Pavel with an invited card, card page | 3 | Send by email and Create a new link; withdrawing after Escape and a second opening; Active, Back and reload after a withdrawal |
| Pavel with active Boris, card page or list | 8 | the card opened from its row; choosing Lessons; choosing Overview; Lessons history with Schedule a lesson (two lessons); Show more in Lessons (31 lessons); the only next lesson cancelled elsewhere (one lesson); a row's next lesson (one lesson); a refused duplicate email |
| Worker-shared `pavelAndAGroup` | 5 | Enter in the search; Search after choosing a group; Clear search; a malformed group in the address; an over-long search in the address |
| Unverified tutor (Russian) | 2 | Students; teaching settings (refusals of an unconfirmed tutor) |
| Singletons | 5 | a withdrawn card's address (not found), Show more for a card removed meanwhile (not found), 31 invited cards, the search in the address with Back, a card changed meanwhile |
| Pilot 1 step 4 | 1 | not consolidated |

### `groups` (24)

| Arrangement | Tests | Actions |
|---|---|---|
| Pavel, Anna in Evening algebra, group page | 6 | removing a student (Boris also a member), archiving without lessons, renaming, a background refetch, Schedule a lesson, adding to a group archived meanwhile (sockets off) |
| The group with two future lessons (`groupWithTwoFutureLessons`) | 3 | Archive group (asks first); then Cancel; then confirm |
| The group page with next lessons (`groupPageWithLessons`) | 3 | a next lesson, the last next lesson and the only next lesson cancelled elsewhere |
| A group archived, or archived and unarchived, through the API | 3 | Unarchive group; Add students (opens on its search); adding a student |
| Refusals | 4 | a chosen student who stopped being active, the 200th group, another tutor's or an unknown group in the student filter, and on the group page (both `seedForeignGroup`) |
| Worker-shared `studentsOfPavel` | 3 | Show more in the member choice; Show more that adds no student; a group that filled up meanwhile |
| Singletons | 1 | the group filter of the student list |
| Pilot 2 step 4 | 1 | not consolidated |

### `lesson` (27)

| Arrangement | Tests | Actions |
|---|---|---|
| Maria with Anna, one one-off lesson tomorrow, lesson page (tutor) | 9 | Edit (opens on its title; dismissed with Escape; saved), Cancel (opens on the dismiss button; dismissed; confirmed), the page alone (no series ending), a stale edit (sockets off), a lesson changed elsewhere |
| A daily series | 3 | Edit (asks for the scope), this and following lessons, ending dismissed |
| A weekly series of four (`weeklySeries`) | 8 | ending (opens its confirmation), ending from a moved occurrence, ending a group series, the new series saved, New lesson dismissed, a new series refused (2), ending refused because the lesson changed |
| A weekly series in Reykjavik time | 2 | prefilled instants; the new series saved |
| Singletons | 2 | a series until its end date; a past lesson |
| Anna's view of the lesson page | 3 | Individual lesson without the card name, no series ending, an open page updated by the tutor |

### `join-requests` (18)

| Arrangement | Tests | Actions |
|---|---|---|
| Anna with an ended relationship, My tutors | 3 | a code no longer valid (refused), a valid code, a valid code after the refusal |
| Anna's request declined with a block by Olga | 2 | My tutors shows it as not accepted; a new request with Olga's code |
| Maria with two requests on Today | 2 | a plain decline; a decline of a request accepted meanwhile (sockets off) |
| Maria with one request | 2 | Students' controls do not move; Today's code and status do not move |
| Anna's sent requests with Show more | 2 | different seeds (31 declined requests; a page of them) |
| Singletons | 6 | a held request of a blocked tutor, past tutors, Olga's decline and block, a blocked student's new request on Olga's Students, Show more on Today (worker-shared `mariaWithJoinRequests`), a code sent from Home |
| Pilot 2 steps 1–3 | 1 | not consolidated |

## Candidates for moving to Go integration tests (technique 2)

All 20 specs that run in a browser were scanned for tests whose assert phase (the last of the three phases) holds no browser assertion, and for reads of server state (the mail sink, `readLesson`, `cardState`, `groupMemberNames`, `visibleJoinRequests`, `invitationIsUsable`, `cardLessonStatuses`, `scheduledCardLessonStarts`, `myTutorNames`, `cardStatesNamed`) in any assert phase.

- **One candidate.** `account-edge-cases` › "email language follows the account" saves English through the account preferences, requests a password reset on the English page and then asserts only the mail sink's subject `Reset your OVC password`. Its server contract, that an email follows a language changed after sign-up, would move to a Go integration test against the real API and worker. The existing `TestAccountEmailsFollowTheLanguageAndCarryNoUserText` in `backend/internal/api/account_flows_test.go` proves only the language given at sign-up. Saving the language through the page stays proven in the browser by `language` › "a saved language survives a reload". The test took 0.60 to 0.67 s in Chromium, 0.87 to 1.47 s in Firefox and 0.81 to 1.13 s in WebKit, about 2.7 s of summed time over the three engines and about 0.5 s of wall time.
- **Server reads beside a specified rendering stay.** Every other test that reads server state asserts a rendering or focus move in the same assert phase, for example `ending`'s stop-studying tests (the tutor's single email, no email before confirmation), `signup`'s code that is not valid (an empty mail list), `students`' Send by email and the server reads of `groups`, `lesson` and `join-requests`. These tests stay; moving only their server reads would save a few API calls per test, far below the spread.
- **Assert phases without a browser assertion that still depend on the browser stay:** `account-edge-cases` › "stored keys are known preferences" (Web Storage and `document.cookie`), `realtime`'s socket tests, `session` › "the access token is refreshed before it expires" (the page's own refresh), and `smoke`'s security-header and size tests (the preview server's responses to the browser and the layout).
- **Tests with no page at all:** the pilot scenario steps pilot 1 step 7, pilot 3 seed and step 13 (`schedule`) and pilot 4 step 2 (`ending`) stay as pilot steps. `smoke` › "the preview server has no build manifest to serve" tests the preview server, not the API, so it has no Go counterpart; it took 0.07 to 0.08 s in Chromium, 0.18 to 1.24 s in Firefox and 0.17 to 0.20 s in WebKit. Running it in one project would be a configuration change outside techniques 1 and 2, worth under 1 s of summed time. The browserless suites `session-transport`, `realtime-contract` and `api-contracts` already run in Chromium only.

The key refusals and the authorization and relationship-scope refusals of the five largest specs stay in the browser in every engine, per proposal 0004 and `AGENTS.md`:

- `students`: the unconfirmed tutor's Students and teaching settings, a withdrawn card's address, Show more for a card removed meanwhile, the duplicate email, the malformed group and the over-long search in the address;
- `groups`: another tutor's group and an unknown group in the student filter and on the group page (T-5), a chosen student who stopped being active, the 200th group, a group that filled up meanwhile, adding to a group archived meanwhile;
- `lesson`: a new series refused because the audience is gone, ending refused because the lesson changed, a stale edit, and Anna's view without the card name, edit or cancel controls or series ending (T-5, T-44);
- `join-requests`: a code no longer valid, a held request of a tutor no longer active, a blocked student's request that never reaches the tutor (T-33), a decline of a request accepted meanwhile.

Where a candidate below absorbs one of these refusals into another test, its assertions stay, in the same engines, as assertions of the absorbing test.

## Kinds of consolidation

Proposal 0004's technique 1 merges tests with the same arrangement and the same action, every assertion in the merged test's assert phase, and splits by action otherwise. No candidate in the five largest specs meets that definition: no two tests there share both their arrangement and their action. The candidates below are of three kinds, and every one needs the owner's agreement in its change's plan:

- **Navigation-only prefix extension:** the removed test's action is its first navigation, which the absorbing test also performs as part of its arrangement before its own, different action (a row click and a notes save, a dialog closed, an edit through the API). The removed test's assertions are added right after that navigation, in the absorbing test's arrange phase. No focus assertion moves, so the risk is lower than for the prefix extension below, but `STU-3` and `LES-8` also change the absorbing test's arrangement (a seeded lesson, another card name), named in their rows.
- **Changed path:** the same arrangement, but the merge changes an action path (Enter instead of a pointer click), adds pages to one test or changes the arrangement (a clock advance); it needs the owner's agreement.
- **Prefix extension:** the removed test's arrangement and action are the first steps of the absorbing test's arrangement. Its action and assertions move into the absorbing test's arrange phase, before that test's own action. This is not technique 1 as the proposal defines it; it extends the technique and needs the owner's agreement in the change's plan. Its risk is order dependence: a focus assertion that moves into the arrange phase then holds only for the focus state the earlier steps leave, and the absorbing test's own action starts from the state the moved steps leave instead of a fresh page. Each row names the focus assertions concerned.

Every merge keeps the assertions that the two tests share in both copies, so that the counts of `expect(` and `expect.poll(` stay unchanged as proposal 0004's guards require, for example the busy-control and address assertions of `STU-1` and the saved-state assertions of `SCH-4`.

## Ranking of the techniques by measured gain

| Technique | Measured basis | Summed-time gain | Wall-time gain |
|---|---|---|---|
| 4: Firefox's growing stall (E8) | stalls of 65.6 to 78.7 s per run, starting at about 0.75 s | up to 45 to 58 s, only if the growth is removable | up to 8 to 11 s |
| 1 as defined: same arrangement and action | no candidate | none | none |
| Extension of 1: prefix merges (E2 to E6) | 17 prefix extensions per engine | 36.7 s | about 6.7 s |
| Extension of 1: navigation-only prefix merges (E3, E5) | 3 merges per engine (`STU-3`, `LES-3`, `LES-8`) | 4.3 s | about 0.8 s |
| Extension of 1: changed path (E2, E4, E6) | 4 merges per engine (`SCH-4`, `GRP-3`, `JR-3`, `JR-4`) | 6.8 s | about 1.2 s |
| 2: server-only checks to Go | 1 test (`account-edge-cases`) | about 2.7 s | about 0.5 s |
| 3: database cost (E7) | Docker's VM took 13.7 to 14.8 % of the CPU; not yet measured with the proposed settings | unknown | unknown |

- The five largest specs hold far fewer merges than the proposal's example of 100 tests per engine. Technique 1 as defined finds none, because the tests that share an arrangement differ in their action. All of the consolidation gain, 47.9 s summed, needs an extension of the technique that the owner agrees to; 36.7 s of it needs the prefix extension that moves an action and its focus assertions into an arrange phase.
- Each of E2 to E6, with every kind of merge, is worth 1.1 to 2.8 s of wall time, well below the spread of these runs (19 s, or 6 s between runs 2 and 3). Three runs with and three without a change cannot resolve such a gain in wall time; the test count and the spec's summed time without Firefox's stalls are the measurable effect. The plan's criterion that each change's gate stays at most 260 s is unaffected.
- All consolidation kinds, technique 2 and technique 4 together, about 17 to 20 s of wall time at most, stay below the 22 to 87 s by which recent gates exceeded D46's 165 s target ([slice 5 gates](check-budget-2026-10-29-slice-5-gates.md)).

## Owner's decision of 2026-10-29

After this measurement the owner decided, for the end-to-end changes of the [slice 6 plan](../plans/slice-6.md):

- of E2 to E8, only E5 (`lesson`) and E8 (Firefox's context-close stall) run now;
- E2 (`schedule`), E3 (`students`), E4 (`groups`) and E6 (`join-requests`) run only when a change's own gate approaches the temporary 260 s threshold;
- E7 (the database cost) stays as planned;
- the decision on D46's target stays at the end of slice 6.

E5's eight candidates are all extensions of technique 1 (six prefix extensions and two navigation-only prefix extensions), so E5's own plan asks the owner to agree to each kind before it merges any of them. The plan records the decision as its revision 6.

## Candidate consolidations for E2 to E6

Each row removes one test from all three engines. "Absorbed by" names the test that takes over its assertions; "How" names every change to the absorbing test's arrangement and the focus assertions concerned. Gains are summed over the three engines and averaged over the three runs; the per-engine durations of the removed test are Chromium / Firefox / WebKit. Row labels are local to this record.

### E2 `schedule`: 1 on a changed path (2.8 s), 4 prefix extensions (8.4 s)

| # | Removed test | Absorbed by | How | Removed test C / F / W | Gain | Kind |
|---|---|---|---|---|---|---|
| SCH-1 | a dialog opens on its first field | a saved lesson returns focus to New lesson | New lesson opened, focus on the search and the default title asserted before filling; the search's focus becomes an arrange-phase assertion | 0.38 / 0.66 / 0.58 s | 1.8 s | prefix extension |
| SCH-2 | a free slot prefills the dialog | a dialog opened from a free slot returns focus to New lesson | the same slot click (14:30 or 09:00, one of the two); date, time and the search's focus asserted before Escape | 0.44 / 2.67 / 0.62 s | 1.9 s | prefix extension |
| SCH-3 | the overlap warning takes focus | a plain resubmit hides the overlap warning while it is pending | the first submission's warning, its focus, text, `dir`, markup, time and Schedule anyway asserted before the resubmit; they then run with the absorbing test's route that holds the second submission installed, and the warning's focus is asserted before `submit.focus()` moves focus away | 0.68 / 1.19 / 0.89 s | 2.6 s | prefix extension |
| SCH-4 | the overlap warning takes focus and Schedule anyway saves | Schedule anyway keeps focus while it saves | same arrangement; the absorbing test presses Enter on Schedule anyway where the removed one clicks it, so no remaining test would activate the override with a pointer; the shared saved-state assertions are kept in both copies and "15:30–16:30" is added | 0.76 / 1.07 / 1.06 s | 2.8 s | changed path |
| SCH-5 | below 1024 px the week lists lessons by day with 44 px targets | a 15-minute lesson at 1024 px shows its time and status icon inside a 24 px link | a resize to 800 px after the absorbing test's assertions; the list would be reached by a resize instead of a fresh load at 800 px | 0.45 / 1.15 / 0.63 s | 2.1 s | prefix extension |

The previous and next tests differ in their pressed control and the remaining dialog tests in their action; they stay.

### E3 `students`: 1 navigation-only prefix extension (1.8 s), 3 prefix extensions (7.1 s)

| # | Removed test | Absorbed by | How | Removed test C / F / W | Gain | Kind |
|---|---|---|---|---|---|---|
| STU-1 | creating an invitation focuses the ready dialog and Copy link copies its link | Open the card from the ready invitation shows the invitation it created | Copy link with its notice, the notice's focus and the clipboard asserted before Open the card; the absorbing test adds the clipboard permission grant in Chromium and the `data-theme` assertion; the shared busy-control and address assertions are kept in both copies; the notice's focus becomes an arrange-phase assertion before the heading's focus | 0.60 / 3.34 / 0.75 s | 2.7 s | prefix extension |
| STU-2 | withdrawing after Escape and a second opening removes the card and keeps no link | after a withdrawal, Active, Back and a reload drop the notice and show no open invitations | the absorbing test withdraws through Escape and a second opening instead of `withdrawThroughTheDialog`, with the removed test's address, notice, storage and usability assertions before Active; the opener's and Cancel's focus become arrange-phase assertions | 0.53 / 1.20 / 0.70 s | 2.4 s | prefix extension |
| STU-3 | a row shows the student's next lesson | the card of an active student opens from its row without invitation controls and keeps saved notes | the absorbing test seeds one lesson tomorrow at 10:00 and asserts "Next lesson … 10:00–11:00" on the row after its first navigation; the card it then opens shows that lesson under Next lessons | 0.33 / 1.17 / 0.54 s | 1.8 s | navigation-only prefix extension |
| STU-4 | choosing Lessons keeps focus on it | choosing Overview keeps focus on it | Lessons chosen from the overview first, then Overview; Overview would follow a client-side navigation instead of a fresh load of `?tab=lessons`, and the Lessons tab's focus becomes an arrange-phase assertion | 0.36 / 0.93 / 0.58 s | 2.0 s | prefix extension |

The five worker-shared `pavelAndAGroup` tests each act differently and stay; the refusals listed above stay.

### E4 `groups`: 1 on a changed path (1.4 s), 2 prefix extensions (4.6 s)

| # | Removed test | Absorbed by | How | Removed test C / F / W | Gain | Kind |
|---|---|---|---|---|---|---|
| GRP-1 | archiving with future lessons asks first, with Cancel focused and the count of lessons it cancels | dismissing the archive confirmation returns focus to Archive group and archives nothing | Cancel's focus and the count asserted before Cancel; Cancel's focus becomes an arrange-phase assertion | 0.50 / 1.25 / 0.72 s | 2.3 s | prefix extension |
| GRP-2 | the Add students dialog of an unarchived group opens on its search with only the students outside it | adding a student to an unarchived group closes the dialog and moves focus to the notice | the search's focus and the single choice asserted before choosing Boris; the search's focus becomes an arrange-phase assertion | 0.57 / 1.13 / 0.75 s | 2.3 s | prefix extension |
| GRP-3 | another tutor's group and an unknown group in the filter are not available and show nothing of the group | another tutor's group and an unknown group are not found and show nothing of the group | same seed; one test with four pages in one context, each refusal kept in every engine (T-5); one more page than each test alone | 0.49 / 1.04 / 0.83 s | 1.4 s | changed path |

Archiving, unarchiving through the page and the two-lesson focus tests differ in their action and stay.

### E5 `lesson`: 2 navigation-only prefix extensions (2.5 s), 6 prefix extensions (12.7 s)

`LES-1` to `LES-3` are absorbed by entries of the `focusReturns` loop, which runs one test body for four entries. Their assertions need a per-entry hook in that loop (an optional assertion step of a `FocusReturn` entry) or the loop split into separate tests.

| # | Removed test | Absorbed by | How | Removed test C / F / W | Gain | Kind |
|---|---|---|---|---|---|---|
| LES-1 | an edit dialog of a one-off lesson opens on its title | a saved edit and a dismissed dialog return focus to the opening control (edit dismissed with Escape) | the title's focus, no scope radios and 18:00 asserted before Escape; the title's focus becomes an arrange-phase assertion | 0.41 / 0.71 / 0.64 s | 1.9 s | prefix extension |
| LES-2 | cancelling opens on the dismiss button | the same (cancellation dismissed) | the dismiss button's focus, no radios and the description asserted before dismissing; the dismiss button's focus becomes an arrange-phase assertion | 0.47 / 0.77 / 0.60 s | 2.0 s | prefix extension |
| LES-3 | a one-off lesson offers no series ending | the same (edit saved) | its two assertions after the shared first navigation | 0.36 / 0.63 / 0.53 s | 1.3 s | navigation-only prefix extension |
| LES-4 | ending a series opens its confirmation on the dismiss button | New lesson dismissed after ending a series closes onto the cancelled notice | inside `endedSeriesWithNewLessonOpen` or after its first steps: the confirmation's focus, radios and description asserted before the confirming `Завершить серию` button; the dismiss button's focus becomes an arrange-phase assertion | 0.49 / 1.36 / 0.56 s | 1.9 s | prefix extension |
| LES-5 | editing a series occurrence asks for the scope | this and following lessons offers no date and moves the following lessons | the scope radios, focus, checked state and date field asserted before choosing this and following; This lesson's focus becomes an arrange-phase assertion | 0.43 / 1.31 / 0.54 s | 1.9 s | prefix extension |
| LES-6 | a new series refused after the ending shows the refusal in the dialog | New lesson dismissed with its Cancel button after a refusal closes onto the cancelled notice | the refusal text and the open dialog asserted before Cancel; the refusal stays in every engine | 0.64 / 0.93 / 0.86 s | 2.5 s | prefix extension |
| LES-7 | ending a series in another timezone prefills the same instants in the profile timezone | a new series saved in another timezone starts at the instants of the cancelled lessons | date, time and weekdays asserted before the `Запланировать` button | 0.59 / 1.01 / 0.82 s | 2.4 s | prefix extension |
| LES-8 | a student sees Individual lesson, never the card name | an open page updates when the tutor moves the lesson | the absorbing test seeds the card name `Аня из вторника` instead of the default and asserts Individual lesson, Maria, no card name, the room time and no edit or cancel controls after its first navigation; the visibility rule stays in every engine (T-5, T-44) | 0.35 / 0.64 / 0.50 s | 1.2 s | navigation-only prefix extension |

### E6 `join-requests`: 2 on a changed path (2.6 s), 2 prefix extensions (4.0 s)

| # | Removed test | Absorbed by | How | Removed test C / F / W | Gain | Kind |
|---|---|---|---|---|---|---|
| JR-1 | a code that is no longer valid is refused with an alert that takes focus | after an invalid code alert the form still sends a valid code | the alert's text and focus and no sent requests asserted before the valid code; the alert's focus becomes an arrange-phase assertion; the refusal stays in every engine | 0.49 / 0.86 / 0.60 s | 2.0 s | prefix extension |
| JR-2 | a valid code sends a request, empties the field and lists the request as sent | after an invalid code alert the form still sends a valid code | the valid code would be proven only after a refusal, not as a first submission; the status's focus is then asserted after an earlier alert held focus | 0.48 / 0.90 / 0.70 s | 2.0 s | prefix extension |
| JR-3 | a request the tutor declined with a block shows as not accepted | the code of a tutor who blocked the student lists a new sent request that the tutor never sees | its assertions on the single sent request (`Не принят`, `Ваш запрос не приняли.`) after the shared first navigation; the absorbing test advances the clock by 60 s before it, which the removed test does not | 0.51 / 0.77 / 0.69 s | 1.6 s | changed path |
| JR-4 | arriving requests do not move the Today code and status | arriving requests do not move the Students controls | same seed; one test with a second page in the same context, each page with its own offset recorder | 0.39 / 1.19 / 0.59 s | 1.0 s | changed path |

### Totals

| Change | Navigation-only prefix extension | Changed path | Prefix extension | All | Tests per engine with all |
|---|---|---|---|---|---|
| E2 `schedule` | — | 2.8 s | 8.4 s | 11.2 s | 31 → 26 |
| E3 `students` | 1.8 s | — | 7.1 s | 8.9 s | 26 → 22 |
| E4 `groups` | — | 1.4 s | 4.6 s | 6.0 s | 24 → 21 |
| E5 `lesson` | 2.5 s | — | 12.7 s | 15.2 s | 27 → 19 |
| E6 `join-requests` | — | 2.6 s | 4.0 s | 6.6 s | 18 → 14 |
| **All** | **4.3 s summed, about 0.8 s wall** | **6.8 s, about 1.2 s wall** | **36.7 s, about 6.7 s wall** | **47.9 s, about 8.7 s wall** | 72 tests fewer over three engines |

Firefox's `schedule` holds one test fewer than the other engines throughout. Each change still needs its own mapping table, its focus assertions and the guard counts above, as proposal 0004 and the slice 6 plan require. Removing tests also removes contexts from each Firefox worker, which delays its stalls; that effect is counted only as the mean close per removed test.
