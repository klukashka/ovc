# Slice 8 slice review

Status: review completed 2026-11-11; resolved 2026-11-11 when its follow-up changes merged and the owner decided the remaining entries, as listed below. Agent record.

## Scope

- **Base:** `94ab6f784477c0904f36ea18534ae48a8e4f335a` ("Record the design slice review and close the slice"), the commit that closed the design slice, as the [slice 8 plan](../plans/slice-8.md#10-closing-the-slice) requires.
- **Head:** `a1b5df9b922e15cd8f1528b59cc2f0f5b33a9051` ("Fix the backend findings of the slice 8 slice review"), 19 commits and 276 files. The two review follow-up changes, `6959442` and `a1b5df9`, merged after the review; the head includes them.
- **Changes in the range:**

  | Plan step | Commit | Change |
  | --- | --- | --- |
  | Plan | `02fd46a` | Plan slice 8: library and builder |
  | S1 | `dcb08a2` | Specify the slice 8 library and builder contract |
  | B2a | `7c8446e` | Make the upload admission, scope checks and pending sums context-driven |
  | F1 | `9066b53` | Render restricted Markdown with OVC's own parser |
  | B1 | `0bfd36f` | Add text normalisation and number parsing version 1 |
  | Outside the plan | `c372969` | Record the owner's acceptance of the presentation URL residual |
  | B2 | `fe015c8` | Add the library upload contexts and download token version 2 |
  | B3 | `8b37b2f` | Add the library folders and materials |
  | B4 | `c2f7c7e` | Add the library templates |
  | Outside the plan | `4f8ebf4` | Move the minimatch 10 tool consumers to minimatch 11 and drop the advisory acceptance |
  | B5 | `d4b8179` | Add the lesson and series materials |
  | B6 | `216e3a7` | Send the lesson presentation to the meeting provider |
  | F2 | `a3c761a` | Add the tutor's library page and the checklist step for assignments |
  | F4 | `e8069fb` | Add the lesson materials to the lesson page and the lesson dialogs |
  | F3 | `9c063a8` | Add the assignment builder with autosave, reordering, prompt pictures and the student-view preview |
  | Outside the plan | `c2bb228` | Fix the causes of the flaky tests of the slice 8 merge gates |
  | Outside the plan | `2dad4ee` | Raise the Go toolchain to go1.27.4 for GO-2026-6859 |
  | Review follow-up | `6959442` | Resolve the frontend findings of the slice 8 slice review |
  | Review follow-up | `a1b5df9` | Fix the backend findings of the slice 8 slice review |

- **Tier:** slice, under [ADR-0011](../../docs/decisions/0011-tiered-verification.md). Many changes met the security trigger (uploads, downloads and signed download addresses, answer-key visibility, rendering of untrusted content, purge, the meeting provider, dependencies and agent configuration).
- **Deferred to slice 10:** performance measurements wait for the deployment by the owner's decision of 2026-11-08, so the backend reviewer applied the performance profile by reading code.
- **Unverified:** the presentation's live fetch by a real BigBlueButton server, because there is no remote server; it moved to the remote BigBlueButton probe and activation.

## Reviewers and coverage

Six fresh reviewers took one layer each, with security split into three families of trust boundaries because the slice touched more than one.

| Reviewer | Profiles | Components covered | Why |
| --- | --- | --- | --- |
| Backend | Correctness, performance (code-level) | Library folders, materials and templates; text normalisation and number parsing; the context-driven upload pipeline and library uploads; download token version 2 and its reference kinds; lesson and series materials, the series-material retirement and the upload expiry in the worker; the lesson presentation in the meetings service and the provider adapters; migrations and queries | Every backend package the range changes, and the purge, deletion and worker paths that reach the new tables |
| Frontend | Correctness, browser (dark theme, Firefox, Safari), performance | The restricted Markdown renderer; the Library page and its dialogs; the builder with autosave, reordering, prompt pictures and the student-view preview; lesson materials on the lesson page and in the lesson dialogs; the checklist step; the sign-out wait of the stabilization change; the per-feature texts and `check:bundle` | Every frontend component the range changes, and the shared primitives reached through them |
| Security: uploads, downloads, signed tokens and purge | Full security | Upload admission and limits for the `library` and `prompt_image` contexts, the content-type allowlist and picture metadata filter, download token version 2 and its `ref` refusal order, download slots and the stall rule, quota charges, the upload expiry and the purge of library rows | The upload and download trust boundaries and their deletion paths |
| Security: authorization, answer-key visibility and untrusted content | Full security | Relationship scope of every library and lesson-material route, the student view without keys, explanations or points, the student-view components' import guard, restricted Markdown and text rendering, the sign-out wait | Authorization, answer visibility and rendering trust boundaries; T-5, T-6, T-9, T-30, T-14, T-21, T-44 and T-2 with T-29 examined |
| Security: meeting provider, settings, dependencies, test control and agent policy | Full security | The presentation sent to the provider and its settings, the shared fake's presentation fetch, the test-control surface, the minimatch override and the Go toolchain upgrade, `AGENTS.md` and `.claude/` | The provider, configuration, dependency and agent-policy trust boundaries; T-11, T-18, T-19, T-36, T-39 and T-41 examined |
| Development tooling and agent configuration | Correctness, security for verification policy and dependencies | The merge gate records of the slice, the change-checks table, the new ESLint guards, the `check:bundle` builder check, the `pg_cancel_backend` test helper against ADR-0009 item 12, the full check run's duration, the slice 8 known limitations | Development tooling, verification policy and test rules changed |

## Integration boundaries and who reviewed them

| Boundary | Reviewer |
| --- | --- |
| Library and lesson-material routes, services, repositories and PostgreSQL | Backend; authorization security for scope |
| Upload pipeline, storage adapter and quota charges | Backend; upload security |
| Download token minting, reference authorization and serving | Backend; upload security |
| Worker: upload expiry, series-material retirement, purge | Backend; upload security for data loss and deletion |
| Meetings service and the provider adapters for the presentation | Backend; provider security |
| Student view and its preview in the builder | Frontend; authorization security for answer keys |
| Library, builder and lesson pages over the typed API client | Frontend |
| Restricted Markdown renderer and text rendering | Frontend; authorization security for untrusted content |
| Merge gate evidence, change checks, lint guards and test helpers | Tooling; provider security for agent policy |

## Results

| Reviewer | Blocker | Major |
| --- | --- | --- |
| Backend | 0 | 0 |
| Frontend | 0 | 1 |
| Security: uploads, downloads, signed tokens and purge | 0 | 0 |
| Security: authorization, answer-key visibility and untrusted content | 0 | 0 |
| Security: meeting provider, settings, dependencies, test control and agent policy | 0 | 0 |
| Development tooling and agent configuration | 0 | 1 |

**Majors:**

- **Frontend: focus moves without `toBeFocused` tests.** The Library's breadcrumb and tree-item focus moves and the builder's Change material cancel had no test asserting the focused element, which `AGENTS.md` requires for every focus move a specification defines.
- **Tooling: gate reruns without a recorded approval.** Three trees were merged after reruns of failing tests, and the owner's approval of those reruns lived only in the conversation, not in the repository.

**Minor findings and nits:**

- **Backend:** download records lacked `context` and `ref_kind`; alternative UUID spellings in lesson `material_ids` answered `500`; library migrations could drop rows on the way down; the series-material retirement query scanned the table; the `ClaimExpiredUploads` race between expiry and binding; a nit on the order of the presentation lookup.
- **Frontend:** dragging a block handle could drop its text into the title field; the Library list stayed stale after leaving the builder; the student view's prompt was not tied to its question group; the bundle headroom is 1.4 KB.
- **Security, uploads and downloads:** one token holder could hold all 64 download slots for 10 minutes with a 100 MB library file; the `ClaimExpiredUploads` expiry and bind race could lose a bound file inside the tutor's own scope.
- **Security, authorization and content:** sign-out could wait up to 15 seconds with no busy feedback (a T-2 residual); the Markdown link residual (a deceptive label, look-alike IDN hosts, private hosts, bidirectional characters in labels) was not recorded in T-14.
- **Security, provider and policy:** `AGENTS.md` and NFR-1.3 said the conferencing provider receives only a display name, an opaque identifier and a role, but from slice 8 it also receives the tutor's lesson PDF; nits.
- **Tooling:** the `pg_cancel_backend` test helper was wider than ADR-0009 item 12; the change-checks table missed the two workspace `osv-scanner.toml` files; the new ESLint guards had no regression tests; the full check run is near its 10-minute budget (peak 581 s); several rows of the known limitations were unresolved.

## Follow-up changes

Each is a standard-tier change under the after-fix rule, names the findings it resolves, and its reviewer confirmed them closed.

| Change | Findings | Merged |
| --- | --- | --- |
| Resolve the frontend findings of the slice 8 slice review | Frontend major: the three focus tests, and the `isFocused` check in `library.spec.ts` rewritten to `toBeFocused`; the sign-out busy state; the drag MIME type; the Library list invalidation; the student prompt's `aria-describedby`; regression tests for the ESLint rules; nits | `6959442` |
| Fix the backend findings of the slice 8 slice review | The expiry race, with a re-check under the row lock and a two-connection test; per-address download limits, as the owner chose on 2026-11-11: at most 8 downloads per client address that have run longer than 5 seconds and 24 in flight per address, so quick picture loads do not count; alternative UUID spellings answer `422`; download records carry `context` and `ref_kind`; guards in the migrations' down steps; the `pg_cancel_backend` helper narrowed to direct waiters; the retirement query rewritten; the presentation lookup moved before the dispatch check | `a1b5df9` |
| This documentation change, which also closes the slice | The tooling major, by the rerun approvals below; the two `osv-scanner.toml` files in the change-checks table of the [development guide](../../docs/development.md#change-checks-and-the-merge-gate); the approval dates of items 10 to 12 in [ADR-0009](../../docs/decisions/0009-owner-approved-exceptions-and-visibility.md); the T-2, T-14 and T-20 residuals and the slice 8 gate reruns in T-41 of the [threat model](../../docs/threat-model.md); the slice 8 gate reruns in [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#owner-approved-gate-reruns-of-slice-8); the rule recording owner-approved reruns in `AGENTS.md`; NFR-1.3, `AGENTS.md` and the realtime and video specification on the lesson presentation; the slice 9 budget item and the slice 10 flaky-test item in the [roadmap](../../docs/roadmap.md); every row of the [slice 8 known limitations](slice-8-known-limitations.md) resolved; this record | this commit |

The wording added to `docs/` and `AGENTS.md` is pending owner review.

## Owner-approved gate reruns

The failures rule of [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md) allows one rerun only for a named infrastructure error with no failed assertion. Every other rerun below was approved by the owner by a multiple-choice question on the date given, for that change only, and sets no precedent. Each change merged only after a later run of its gate passed every command and its finish confirmed that `main^{tree}` equalled the tested tree. Counts like "95 of 95" are repeated runs of the failing test alone on the same tree.

| Date | Change | Tested tree | Run | Failure | Owner's decision |
| --- | --- | --- | --- | --- | --- |
| 2026-11-10 | B3, library folders and materials | `3208d34` | 1 | Backend SQLSTATE 53300 "too many clients" in six packages | The one rerun ADR-0018 allows for a named infrastructure error with no failed assertion |
| 2026-11-10 | B3 | `3208d34` | 2 | WebKit `messages.spec.ts:164`: `locator.evaluate` timed out on a visible row; 95 of 95 alone (WebKit, five repetitions) | Rerun approved |
| 2026-11-10 | B2, library upload contexts and download token version 2 | `a6f3f47` | 1 | Six Firefox tests (meeting room, message attachments) failed with "browser has been closed"; 78 of 78 alone | Rerun approved by the owner; carried into slice 10 as a load failure, because machine contention is never an infrastructure error under T-41 |
| 2026-11-10 | B2 | `f8d6558` | 2 | Firefox `upload-transport.spec.ts:170`: `replayRefreshes` was 2 instead of 1; repeated alone it failed 1 of 300 on `main` and 4 of 300 on B2 | Rerun approved on `fe015c8`; the cause was fixed later in `c2bb228` |
| 2026-11-10 | F2, the library page | `716b40f` | 1 | Chromium `session.spec.ts:89`: an unexpected `401` after sign-out, and WebKit `schedule.spec.ts:870` timed out; 10 of 10 and 5 of 5 alone | Rerun approved |
| 2026-11-10 | F2 | `716b40f` | 2 | WebKit `signup.spec.ts:343`: `goto /today` timed out | Investigation first; a comparison of 1,880 tests on each tree found 4 failures on F2 and 5 on `main`, so the owner then approved a rerun, which passed |
| 2026-11-10 | F4, lesson materials on the lesson page | `7cf94c3` | 1 | WebKit `schedule.spec.ts:1064` timed out; 30 of 30 alone | Rerun approved |
| 2026-11-10 | F4 | `7cf94c3` | 2 | Firefox `smoke.spec.ts:196`: the coherent refetch poll timed out; 10 of 10 alone | Rerun approved |
| 2026-11-10 | F4 | `7cf94c3` | 3 | Backend `TestReconnectBeforePendingReleaseRecoveryKeepsTheNewLease`: close `1012` | Rerun approved, which passed; the cause was fixed later in `c2bb228` |
| 2026-11-11 | Go toolchain go1.27.4 | `06d092c` | 1 | WebKit `reclaim.spec.ts:200`: two unexpected `console_warning` diagnostics; 10 of 10 alone | Rerun approved |
| 2026-11-11 | Backend review follow-up | `3dd8768` | 1 | 19 Firefox failures (lesson room, lesson form, students) in a 1,614-second end-to-end run while Spotlight indexing (`mds_stores`) used about 96% of a CPU at a load of about 21; 73 of 73 alone | Rerun approved once indexing had ended |
| 2026-11-11 | Backend review follow-up | `3dd8768` | 2 | On an idle machine, WebKit `message-attachments` steps 8 to 11: unexpected `401` on channels after sign-out; WebKit `messages.spec.ts:180`: the edit and delete `evaluate` timed out | Rerun approved, and both failure families carried into slice 10's load diagnosis |
| 2026-11-11 | Slice 8 review record and closure | `bab4da9` | 1 | WebKit `messages.spec.ts:125` (live exchange at 375 px): the Send button was not found within the 30-second test timeout; the same WebKit messages family carried into slice 10; the change edits only documentation and `AGENTS.md` | Rerun approved, recorded in this commit before the rerun as `AGENTS.md` requires |
| 2026-11-11 | Slice 8 review record and closure | `cc29ed7` | 2 | WebKit `session.spec.ts:142` (sixth failed sign-in): the Email field was not found within the 30-second test timeout; WebKit slows down about 2.4 times by the end of a full run (measured in the end-to-end timing analysis of 2026-11-11) | Rerun approved, recorded in this commit before the rerun |

Two further failures needed no rerun approval. The first gate of F3 (tree `925a8ca`, 2026-11-11) failed 22 tests of the lesson material picker, a real regression that went back to the branch and was fixed before F3 merged as `9c063a8`. During F2's own checks on 2026-11-10, Firefox `account-edge-cases.spec.ts:152` failed when a reload aborted a font download (3 of 15 on `main`, 4 of 10 on F2); the cause was fixed in `c2bb228`.

## Owner decisions on 2026-11-11

Each was answered by a multiple-choice question.

- **Remaining minor findings:** accepted as known limitations of slice 8. Every flaky test without a fixed cause is carried into slice 10's diagnosis of failures under load, with its failure count, as the [flaky tests section](slice-8-known-limitations.md#flaky-tests-of-the-slice-8-merge-gates) of the known limitations lists: unexpected `401`s after sign-out, the WebKit message edit and delete stall, the 10-second test-control fixture timeout, Firefox `smoke.spec.ts` readiness, WebKit schedule timeouts, WebKit `signup.spec.ts:343` (`goto /today` timed out), WebKit `reclaim.spec.ts:200` (unexpected `console_warning` diagnostics), the six Firefox "browser has been closed" failures of one gate, the 19 Firefox failures under Spotlight indexing load and SQLSTATE 53300 in the full race run.
- **Residuals recorded in `docs/`**, each pending owner review: T-2, the sign-out that may wait up to 15 seconds for reads in flight, now with a busy state; T-14, the Markdown link residual; T-20, the per-address download limits, a slow reader within them, and library uploads sharing the upload slots of 4 per user and 32 per instance with chat; NFR-1.3 and the matching sentence of `AGENTS.md`, that the conferencing provider receives a display name, an opaque participant identifier and a role as participant data, plus the lesson presentation, a PDF the tutor chose as lesson material, passed unfiltered and possibly holding personal data such as student names or its author's metadata, fetched through a signed storage URL only by the self-hosted conferencing server in Russia. T-2 also names that only reads are waited for, that a tab closed during the wait never revokes its session, that a second ending action is not serialised, and that unexpected `401`s after sign-out still recur under load.
- **Budgets:** the end-to-end suite's speed-up and consolidation becomes the first change of slice 9, and new interface texts load lazily from then on, because the full check run peaked at 581 seconds of its 10-minute budget and the bundle has 1.4 KB of headroom.
- **Per-address download limits:** at most 8 long downloads (running longer than 5 seconds) and 24 in flight per client address, implemented in `a1b5df9`.
- **A major residual accepted:** the re-review of `a1b5df9` rated the residual of the per-address limits Major: a client trickling just above the stall minimum keeps up to 8 long downloads per address, so a few addresses can still hold every download slot of an instance, and users behind one shared address share the 8. The owner accepted it on 2026-11-11 as the consequence of choosing the per-address limits, with the defence against several addresses left to the deployment's reverse proxy; it is recorded in T-20 and in the [slice 8 known limitations](slice-8-known-limitations.md), and is the one major finding that is accepted rather than fixed.
- **Rerun records:** because gate reruns without a recorded approval were found in both the slice 7 and slice 8 reviews, `AGENTS.md` now requires every rerun the owner approves after a failed assertion to be recorded in the repository before the merge, with its date, tested tree, failing test and decision, in the slice's known-limitations file and in the merge report (pending owner review); the slice 8 reruns are recorded in ADR-0018 and T-41.

## Known limitations re-verified

- **Slice 8 entries:** every entry of [the slice 8 known limitations](slice-8-known-limitations.md) was re-verified by the reviewer of its layer and now carries its resolution: fixed, accepted by the owner, or carried into slice 9 or slice 10.
- **Earlier entries:** the entries carried into slice 10 from the [design slice known limitations](slice-design-known-limitations.md) and the [WebKit failure record](design-slice-webkit-failures.md) stay carried there.

## Resolution

The review was resolved on 2026-11-11. Both majors of the review are closed: the focus tests by `6959442`, and the rerun approvals by this record, ADR-0018, T-41 and the new rule in `AGENTS.md`. The major residual of the per-address download limits found by the re-review of `a1b5df9` is accepted by the owner, as the owner decisions above state. The minor findings the owner asked to fix are fixed by `6959442` and `a1b5df9`, each confirmed closed by its reviewer. Every other finding is accepted by the owner, recorded as a residual in `docs/` pending owner review, or carried into slice 9 (the end-to-end budget and lazily loaded interface texts) or slice 10 (flaky tests without a fixed cause, performance measurements), as the [slice 8 known limitations](slice-8-known-limitations.md) list. The presentation's live fetch by a real BigBlueButton server stays `unverified` until the remote probe and activation.
