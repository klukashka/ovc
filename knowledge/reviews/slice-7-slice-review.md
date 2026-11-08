# Slice 7 slice review

Status: review completed 2026-11-07; the follow-up code changes have merged, as listed below. The review was resolved for local closure on 2026-11-08; see [Resolution](#resolution). Remote BigBlueButton behaviour stays `unverified` throughout.

## Scope

- Base: `e201e16f7dcc3382af0ebb19487f07f1a57f3048`, the accepted slice 6 baseline ([slice 6 accepted state](slice-6-accepted-state.md)).
- Head: `2950e1c489e5bc0739d4e997d37be7e2a6c4fe3a` ("Settle uncertain meeting creation after a one-hour quiet window and confirmed provider absence"), 34 commits.
- Tier: slice, under [ADR-0011](../../docs/decisions/0011-tiered-verification.md). The slice met the security trigger (meeting authorization, credential-bearing join links, outbound provider calls, an unauthenticated callback, framing, test control and verification policy), so security reviewers ran.
- Every reviewer worked read-only on `main` at the head commit. No reviewer ran a test, build, scanner or gate, so every check result in their reports is deferred to the merge gate, never passed. Runtime claims come from reading code, tests and documents.

## Reviewers and coverage

Five fresh, independent reviewers each took one layer or one family of trust boundaries. The security review was split in two because the slice touched two families of trust boundaries: the server-side meeting provider, and the browser room with the test-control surface.

| Reviewer | Profiles | Components covered | Why |
| --- | --- | --- | --- |
| Backend | Correctness, performance | `internal/adapters/meeting` (provider interface, in-memory and shared fakes, navigable fake, BigBlueButton adapter); `internal/services/meetings` (room coordinator, lifecycle routes' service, cleanup executor, quiet-window settlement, recovery scanner, operator); meeting repositories, queries and migrations 00022 to 00031; `internal/services/outbox` and `internal/worker` for typed recovery; the schedule status job, tutoring deletion and account purge additions; meeting API routes and `ovc-admin meetings`; process composition and settings; the integration tests, read for rules and coverage | The slice added the meeting backend end to end; every package was directly changed. |
| Frontend | Correctness, browser, performance | `LessonRoom.tsx`, `roomState.ts`, `roomTab.ts` and their tests; the lesson page, list and form changes; the bounded body drain in `session.ts` and the transport contract; the realtime socket close-after-open; strings; preview security headers; the lesson-room, smoke and harness end-to-end files | The slice added the lesson room interface and touched the session, realtime and harness code it depends on. The fake room page was left to the backend and room security reviewers. |
| Security: meeting provider side | Full security | BigBlueButton adapter and fakes; coordinator, lifecycle, recovery and operator services; meeting routes and access log; `ovc-admin meetings`; provider selection and fake room origin settings; the threat model meeting rows | Trust boundary 2 (API and BigBlueButton), boundary 4 as far as join links go, and boundary 6 (`ovc-admin`). |
| Security: room, browser and test control | Full security | Preview security headers and their test; the lesson room, room tab and room state; realtime socket, client and session adoption; the end-to-end room origin forwarder and room specs; the test-control fake room routes, page and guard; settings and composition for test control | Trust boundaries of framing, join links in the browser, realtime sessions and the end-to-end test control surface. |
| Development tooling and agent configuration | Correctness, security | `tools/` modules and OSV configurations, gate and wrapper scripts (checked unchanged), `.claude/`, `AGENTS.md`, lint configurations, every manifest and lockfile, `.env.example` files, the new settings, the end-to-end harness, `sqlc.yaml`, the test database fixture, the architecture test against the package layers table, ADR-0018 exceptions checked against git, the development guide and the tooling threat rows | The slice changed tool dependencies, scanner exceptions, gate exception records, settings and the harness. |

## Integration boundaries

| Boundary | Reviewed by |
| --- | --- |
| API routes and the meetings service (error mapping, join link only in the `POST` body with `no-store`) | Backend; provider security for authorization and link handling |
| Meetings service and provider adapter (calls outside transactions, per-call budgets, generation binding) | Backend; provider security |
| Worker and recovery repositories (typed completion, reclaim, lock order, scanner cursor) | Backend |
| Lifecycle service and repositories (guards, closure, capacity) | Backend |
| OVC and BigBlueButton (checksums, transport, untrusted XML, what the provider receives) | Provider security |
| `ovc-admin` and the meetings service | Provider security; backend for correctness |
| Web client and the lesson API contract (`join`, `end`, `restart-room`, `room_open`, `embed_allowed`, `took_place`) | Frontend; room security for link handling |
| Web client and the room origin (new tab, frame, CSP `frame-src`) | Frontend; room security |
| Test control and the processes (flag-gated routes, fake room, loopback checks) | Room security; tooling for settings and wrappers |
| Package layers table and the architecture test | Tooling |
| Gate exceptions and git identities | Tooling |

## Counts

| Reviewer | Blocker | Major | Minor | Informational |
| --- | --- | --- | --- | --- |
| Backend | 0 | 0 | 7 | 0 |
| Frontend | 0 | 0 | 9 | 0 |
| Security: meeting provider side | 0 | 0 | 5 | 0 |
| Security: room, browser and test control | 0 | 0 | 3 | 3 |
| Development tooling and agent configuration | 0 | 0 | 5 | 1 observation |
| Total | 0 | 0 | 29 | 3 and 1 observation |

The owner directed that the important findings be fixed in code by separate changes and that everything else be recorded.

## Findings and dispositions

Known limitations below means the [slice 7 known limitations](slice-7-known-limitations.md#slice-review-residuals); remote activation means the [remote-activation checklist](../plans/slice-7-remote-activation-checklist.md). The access-rules change merged as `0a16bd1` ("Refuse lesson room restarts past the join window and student joins while the tutor is not active, and prove a foreign tutor cannot restart the room"): it requires the join window for restart-room, refuses student joins while the tutor's account is not `active`, an operator block being one example (owner decision of 2026-11-07), and tests restart-room by an unrelated tutor. Its own T-22, student join rule and restart-room wording landed with it; this record does not repeat it.

### Backend

| Finding | Disposition |
| --- | --- |
| One failed meeting-cleanup lease reclaim returned an error that stopped every worker lane and job | Fixed by `a4e783c` ("Report failed meeting cleanup lease reclaims per item instead of stopping the worker, and log provider deadlines at WARNING"); its own residuals are in the known limitations under meeting cleanup reclaim residuals |
| Restart could open a room after the tutor's join window closed, while join answered `outside_join_window` | Fixed by `0a16bd1` |
| Provider deadline timeouts were logged at DEBUG, invisible at INFO | Fixed by `a4e783c` |
| Every cleanup completion and reclaim takes the global capacity row lock | Known limitation |
| The recovery scanner writes its singleton cursor once per candidate | Known limitation |
| A Create failure followed by two failed closures retries the join and adds a 10-second wait before `503` | Known limitation |
| The quiet window and global capacity are SQL literals duplicating Go constants | Known limitation |

### Frontend

| Finding | Disposition |
| --- | --- |
| A pre-opened tab the user closed, or one whose navigation throws, left the session showing a tab that does not exist or stuck in joining | Fixed by `a87666e` ("Guard the lesson room against closed tabs, blocked switches, stale lesson reads and non-loopback http links, and check the session epoch in every realtime callback"); the untested real-tab branches are a known limitation under lesson room client residual tests |
| "Open in new tab" with a blocked pop-up removed the embedded room first | Fixed by `a87666e` |
| Ending wrote the ended lesson into the cache without cancelling an in-flight lesson read, which could restore End and Restart | Fixed by `a87666e`; the End path calling the helper is proven only through the helper, a known limitation under lesson room client residual tests |
| On iPhone Safari the pre-opened tab's navigation depends on the backgrounded opener page | Known limitation; remote activation |
| The device clock decides whether Join shows for a scheduled lesson | Known limitation |
| One component test acts again after its assertions | Known limitation |
| The room-tab fake has no contract test shared with the browser implementation | Known limitation |
| The pilot scenario 1 steps 8–11 test does not assert that the lesson becomes live | Known limitation |
| The frame's `allow` list holds only for the frame's own origin, so a redirecting join address loses camera and microphone | Known limitation; remote activation |

### Security: meeting provider side

| Finding | Disposition |
| --- | --- |
| Students could still join an operator-blocked tutor's open room until the lesson ended | Fixed by `0a16bd1` (owner decision of 2026-11-07: refuse student joins while the tutor's account is not `active`, of which an operator block is one example) |
| Anonymous requests can starve the ended-hint permit pool | Known limitation; remote activation; T-12 residual in the threat model |
| No negative test for a foreign tutor on `restart-room` | Fixed by `0a16bd1` |
| Admission caps are not tied to provider host capacity | Known limitation; remote activation |
| The quiet window's no-replay premise covers only OVC's transport; a proxy may retry the GET `create` | Known limitation; remote activation; residual in the threat model |

### Security: room, browser and test control

| Finding | Disposition |
| --- | --- |
| The client accepted `http:` join addresses for any host | Fixed by `a87666e`: `http:` only for a loopback address; T-25 amended in that change |
| More realtime callbacks than `onOpen` lacked the session epoch check during an account adoption | Fixed by `a87666e`; the known-limitations entry on account adoption is marked resolved and T-29 amended in that change |
| T-25 and the known limitations described fake presence only as a test issue, though a future presence-based decision would rely on wrong data | Known limitation; residual in the threat model |
| No frame sandbox and no `Permissions-Policy` header (informational) | Known limitation; remote activation |
| The test room origin shares cookies with the preview origin on `127.0.0.1` (informational) | Known limitation, test-only |
| The default fake room origin is the API origin (informational) | Known limitation, test-only |

### Development tooling and agent configuration

| Finding | Disposition |
| --- | --- |
| The owner-approved presence test deadline change from 100 ms to 2 s (`3023459`) was recorded only in its commit message | Recorded by this change in the known limitations and in the records that still called the deadline unchanged or the tests flaky |
| The owner-approved gate rerun of `1cae202` was recorded outside ADR-0018 | Recorded by this change in [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#one-change-gate-rerun-for-the-lesson-form-layout) and in the threat model's [slice 7 owner-approved gate reruns](../../docs/threat-model.md#slice-7-owner-approved-gate-reruns) section, linked from T-41, pending owner review |
| The minimatch acceptance rationale lagged its recheck | Corrected by this change in the development guide and both OSV reason strings: only 10.2.6 is in range, a fix path exists in 11.x, bundled copies are invisible to the scanner; `ignoreUntil` unchanged |
| The package-layer qualifier "exported typed recovery outcome only" is not enforced by the architecture test | Known limitation |
| Stale status lines in the meeting recovery storage record and the cleanup interleaving exception plan | Corrected by this change |
| Observation: the minimatch extension `0552c92` has no review record or merge report in the repository | Unverified; no change |

## Threat verdicts

Verdicts are from the two security reports and the tooling report, at the head commit. "Fixed after review" names a follow-up that changed the verdict's gap.

| Threat | Verdict | Reviewer |
| --- | --- | --- |
| T-5 tutor reaches another tutor's work | Examined, not affected; restart-room lacked a direct negative test, fixed after review by `0a16bd1` | Provider |
| T-6 student reaches another student's work | Examined, not affected | Provider |
| T-10 uninvited join | Examined, minor gap (blocked tutor's open room), fixed after review by `0a16bd1`; browser side examined | Provider, room |
| T-11 recordings exposed | Examined, not affected; host settings unverified remotely | Provider |
| T-12 forged callbacks | Examined, minor residual (hint permit exhaustion), recorded | Provider |
| T-13 malicious uploads | Not affected | Provider |
| T-14 script injection through names | Examined, not affected in OVC; provider rendering unverified remotely | Provider, room |
| T-16 framing | Examined, sound; sandbox and `Permissions-Policy` informational | Room |
| T-18 personal data to logs or provider | Examined, not affected | Provider, room |
| T-19 secret disclosure | Examined, not affected | Provider, room, tooling |
| T-20 resource exhaustion | Examined, minor residuals (caps against host capacity, hint permit exhaustion), recorded | Provider, room |
| T-21 removed participant keeps access | Examined, not affected; fake presence residual recorded | Provider, room |
| T-22 operator misuse | Examined, minor gap (a block leaves live rooms joinable), fixed after review by `0a16bd1` | Provider |
| T-24 same-site attacks from the conferencing host | Examined, test-only residual (shared cookies on the loopback address) | Room |
| T-25 leaked join link | Examined, not affected on the provider side; `http:` gap on the browser side fixed after review by `a87666e` | Provider, room |
| T-26 routing keys from `meta_*` | Examined, not affected | Provider |
| T-27 tokens in Referer, history or logs | Examined, not affected on the OVC side; the production reverse proxy unverified | Provider, room |
| T-28 open redirect | Not affected | Provider |
| T-29 stale WebSocket access | Examined, residual (adoption window) fixed after review by `a87666e` | Room |
| T-34 data retained after deletion | Examined, not affected | Provider |
| T-36 test control surface | Examined, sound | Provider, room, tooling |
| T-37 script injection stealing the token | Not affected | Room |
| T-38 development services | Examined, sound | Room, tooling |
| T-39 dependencies | Examined; compression update correct, minimatch extension bounded and expiring | Provider, room, tooling |
| T-41 verification policy | Examined; exceptions bounded to exact identities, with the recording gaps closed by this change | Tooling |
| T-44 unconfirmed account in a one-to-one relationship | Examined, not affected | Provider |
| T-45 cross-site WebSocket | Not affected | Room |

The threat model now records the hint permit exhaustion, the proxy retry of `create` against the quiet-window premise and the untrustworthy fake presence flag ([cumulative review residuals](../../docs/threat-model.md#slice-7-cumulative-review-residuals)).

## Known-limitations rechecks

Each reviewer rechecked the existing entries of its layer as items, not verdicts:

- The cleanup unchanged-generation branch and the revocation conflict branch are still unreachable with the shared fake and, by reading, with the BigBlueButton adapter; both stay open for remote activation.
- The meeting lifecycle, clock step, navigable fake room, lesson room interface and cancelled-write readiness entries were confirmed accurate.
- The resolution of uncertain obligations by the quiet-window settlement was verified in code, with its recorded residuals.
- The account adoption entry was widened by the room security review and then resolved by `a87666e`.
- The "end-to-end readiness failures seen only under a stress run" entry stays open and unverified; no reviewer ran a stress repetition. It needs the owner's acceptance or a carry into a named later slice.

## Merge-gate evidence for the cleanup reclaim change

| Field | Value |
| --- | --- |
| Change | `a4e783c60f51c31cc783918eb174ae208ab6f086` ("Report failed meeting cleanup lease reclaims per item instead of stopping the worker, and log provider deadlines at WARNING") |
| Base | `2950e1c489e5bc0739d4e997d37be7e2a6c4fe3a` |
| Tested tree | `fb56cfa4c7c3a9aa47ff80f2f6d9f64b05932f31` |
| First attempt | Failed: six WebKit end-to-end tests failed under a machine load average near 66 (listed below) |
| Second attempt | Owner-approved rerun; failed: five backend tests got PostgreSQL `too many clients` |
| Third attempt | Owner-approved rerun after a stray long-running local API process of another session, holding 9 database connections, was stopped; passed every command |
| Finish | `main^{tree}` after the merge equalled the tested tree |

The six WebKit tests that failed in the first attempt:

- `[webkit] e2e/message-attachments.spec.ts:209` "pilot-3 › steps 8–11 Anna closes her tabs and receives one digest for one unread group message without a duplicate";
- `[webkit] e2e/smoke.spec.ts:255` "showing the sidebar moves focus to the hide button";
- `[webkit] e2e/students.spec.ts:669` "Clear search empties the filters and moves focus to the search field";
- `[webkit] e2e/students.spec.ts:709` "an over-long search in the address is explained and links to the list without it";
- `[webkit] e2e/teaching-settings.spec.ts:81` "a student blocked meanwhile appears after a reload and unblocking empties the list";
- `[webkit] e2e/teaching-settings.spec.ts:103` "an unblock of a student unblocked meanwhile is refused with an alert that takes focus".

The logs showed `browserContext.newPage: Target page, context or browser has been closed` and unknown WebKit console warnings and HTTP diagnostics. Under ADR-0018's rule that a test which fails and then passes is a flaky-test bug, they are tracked as an open entry in the [known limitations](slice-7-known-limitations.md#webkit-end-to-end-failures-under-heavy-load).

The failures rule of ADR-0018 allows at most one rerun, and only for a named infrastructure error with no failed assertion. The first failure had failed assertions, and a second failure goes to the owner, so the repeated rerun was an owner exception for this change only. It is recorded in [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#repeated-gate-reruns-for-the-meeting-cleanup-reclaim-change) and in the threat model's [slice 7 owner-approved gate reruns](../../docs/threat-model.md#slice-7-owner-approved-gate-reruns) section, linked from T-41, pending owner review; it changes no test, deadline, budget or gate behaviour.

## Database connection exhaustion in gates

Two gate attempts failed with PostgreSQL `too many clients` (SQLSTATE 53300): the second attempt of the cleanup reclaim change above and the third attempt of the access-rules change below. Root-cause analysis: an uncapped full backend run peaks at about 67 client connections, another development run's end-to-end processes held about 44, and a demo API process left running held 9, which together exceed PostgreSQL's limit of 100.

The change `dd04bc3` ("Give the API test harness pools of 4 and bound the lock-waiter polls, and keep heavy runs apart") resolves these failures: the API authentication harness opens the default test pool of 4, and only the two tests in which three or four requests wait on one held row lock use a pool of 6; each lock-waiter poll query is bounded by the pause deadline, so an exhausted pool fails the test instead of hanging it; and the [development guide](../../docs/development.md) forbids running the merge gate, full backend runs, end-to-end suites and measurements concurrently on one machine. A cap on the administrative pool was measured again: uncapped, a full `go test -race` took 105.8 s with a peak of 67 connections, against about 200 s with peaks of 55 and 46 when capped at 4 and 3. The owner rejected the cap again on 2026-11-07 for its cost of about +90 s. The two minor findings of that change's re-review are in the [known limitations](slice-7-known-limitations.md#lock-waiter-poll-bounds).

## Merge-gate evidence for the test harness pool change

| Field | Value |
| --- | --- |
| Change | `dd04bc359309f70884bd33347bd8e45982703fd9` ("Give the API test harness pools of 4 and bound the lock-waiter polls, and keep heavy runs apart") |
| Base | `a87666e7a199b75b2e6cd62320ef8e91c07126c0` |
| Tested tree | `a202fd876dc842fa2062a4d2d54b8026c8d44dee` |
| First attempt | Failed: two WebKit end-to-end tests timed out, `students.spec.ts:87` "pilot-1 › step 4 Maria creates a card for Anna and sends the invitation by email" and `upload-transport.spec.ts:170` "an expired multipart upload refreshes once, removal aborts, and changing accounts clears a held upload", with swap at 7.4 of 8 GB |
| Second attempt | Owner-approved rerun after the owner freed memory and restarted Docker; passed every command |
| Finish | `main^{tree}` after the merge equalled the tested tree |

The first failure had failed assertions, so the rerun was a one-time owner exception for this change only and sets no precedent. The two WebKit tests are added to the open [WebKit entry](slice-7-known-limitations.md#webkit-end-to-end-failures-under-heavy-load). The exception is recorded in [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#gate-reruns-for-the-test-harness-pool-and-access-rules-changes), pending owner review.

## Merge-gate evidence for the access-rules change

| Field | Value |
| --- | --- |
| Change | `0a16bd1fea165e590008e324fe58f0c45cec6b74` ("Refuse lesson room restarts past the join window and student joins while the tutor is not active, and prove a foreign tutor cannot restart the room") |
| Base | `dd04bc359309f70884bd33347bd8e45982703fd9` |
| Tested tree | `fb5184c9dc9a2455c75ec5e7724ac4e969f1c71f` |
| First attempt | Failed: WebKit `lesson-room.spec.ts:428` "signed out, the lesson address asks to sign in and shows nothing private" reported one unknown console error, whose text the trace does not contain |
| Second attempt | Owner-approved rerun; failed: the Mac slept during the gate, giving a 2413-second run, a DNS failure in `pnpm audit` and upload `500` answers in `TestStudentDeletionAndPurgeSerializeWithOtherMemberDigest` |
| Third attempt | Owner-approved rerun; failed: SQLSTATE 53300 in `TestSignalsArriveInCommitOrder`, before the connection change `dd04bc3` was on its base |
| Fourth attempt | Owner-approved rerun; failed: `TestChatDownloadFinalProbeHonorsCancellationAndConcurrentClose` and `TestChatDownloadOpeningDeadlineIncludesDatabaseWaits` with "a released download slot was unavailable" |
| Fifth attempt | Owner-approved rerun; passed every command |
| Finish | `main^{tree}` after the merge equalled the tested tree |

Each rerun was a one-time owner exception for this change only and sets no precedent. The WebKit test is added to the open [WebKit entry](slice-7-known-limitations.md#webkit-end-to-end-failures-under-heavy-load), and the download slot failures are tracked in the open [chat download slot entry](slice-7-known-limitations.md#chat-download-slot-availability-under-load). The exception is recorded in [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#gate-reruns-for-the-test-harness-pool-and-access-rules-changes), pending owner review.

## Open items for slice closure

All resolved on 2026-11-08 as the [Resolution](#resolution) records; the list below is kept as the original record.

- The owner accepts or carries each known limitation added by this review, and the stress-run readiness entry.
- The WebKit end-to-end failures seen in the gates of the cleanup reclaim, test harness pool and access-rules changes are investigated as a flaky-test bug, or carried by the owner into a named later slice.
- The chat download slot failures are fixed by the separate change being investigated, or carried by the owner.
- Remote activation stays blocked on the [remote-activation checklist](../plans/slice-7-remote-activation-checklist.md) and the probe of the [slice 7 plan](../plans/slice-7.md#remote-prerequisites-and-probe-before-live-provider-acceptance).

## Resolution

Resolved for local closure on 2026-11-08. The owner gave every open entry of the [slice 7 known limitations](slice-7-known-limitations.md#owner-dispositions-2026-11-08) a disposition: the remaining minor test, code and performance items are accepted as known limitations of slice 7; the provider-specific items are carried into the [remote BigBlueButton probe and activation](../plans/slice-7.md#remote-prerequisites-and-probe-before-live-provider-acceptance) and held by its [remote-activation checklist](../plans/slice-7-remote-activation-checklist.md); the WebKit failures under heavy load, the readiness failures seen only under a stress run and Join on Home and Today are carried into the [design slice](../plans/design-slice.md). The chat download slot failures were resolved by their own change. The remote probe is still pending a host, and remote BigBlueButton behaviour stays `unverified`; this resolution does not approve activation of a real provider.
