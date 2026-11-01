# Text messages verification

Base: `e5ef1a188fc52a3a0afa0d95f4974ee97df4b2e2`. Scope: [approved F2 plan](../plans/slice-6-frontend-messages.md). Status: merged as `29fc9090ea3c9ff92b0be15fc45e2289a04d09b8`. The exact-tree gate passed in 224.0 seconds and finish confirmed tree `98bfff8dd8b942adc56b3586df09f6fcbda80294`; all 30 gate commands exited zero. The local gate/finish transcript is retained in `.git/ovc-merge-reports/29fc909-frontend-messages.log`. Earlier pending statements below describe their recorded checkpoints.

## Planning review

An independent security reviewer examined the base, contracts and planned boundaries, including untrusted rendering, provisional/reclaim scope, account epoch isolation, bounded event work and diagnostic privacy. The final plan included the pre-reclaim cue for both roles, late-response/timer and live-access-loss tests. No blocker, major or minor findings remained. Runtime verification was not part of planning review.

## Early checks and failures

- Fake transport: 29 tests passed, including six added behavior scenarios. Targeted ESLint initially found three issues; those were corrected and the recheck passed.
- Realtime/refetch: the first added burst test ran invalidation before its initial query had settled. It failed its expected fetch count. The setup now waits for successful initial reads; the three-file run passed 30 tests.
- Frame/read lifecycle: a new component test attempted to open a native dialog unavailable in jsdom. No DOM mocking or production fallback was added. The test inspects the rendered hidden sidebar and its news indicator; browser behavior remains covered by Playwright. An explicit return annotation was also added after lint. The four-file run then passed 45 tests, and targeted ESLint passed.
- Initial integrated typecheck, production build, class check and bundle check passed. Entry plus largest language: 175.2 KB gzip, below the 176 KB owner checkpoint and 180 KB hard cap; 22 page modules load on demand. This is an intermediate build, not the final readiness measurement.

## Initial verification scope

The change requires complete frontend change checks, independent standard and security reviews, browser acceptance, the exact-tree merge gate and finish. The following sections distinguish completed evidence from pending work.

## Complete-diff reviews and follow-up

The independent standard review reported three major findings: reconnect did not retain and recover the entire shown history, first-page channel refresh left inconsistent later-page cursors, and switching edit targets lost the original draft. It also reported minor focus loss on refused/live deletion and late read completion after pagehide. The independent security review reported a major inaccessible-history/preview cache lifetime finding (T-6/T-29/T-44), and the same minor pagehide issue (T-20). Both reviews covered the complete uncommitted implementation against the base; no source edits or runtime checks were delegated to reviewers. The corrections and their regression checks are recorded below. The security cache correction touches a trust boundary. A fresh combined standard and full-security review examined them before readiness.

## Integrated checks before review corrections

`pnpm typecheck`, `pnpm lint`, `pnpm format:check`, `pnpm test`, `pnpm api:check`, `pnpm build`, `pnpm check:classes` and `pnpm check:bundle` all exited 0. The component run passed 628 tests in 58 files. An earlier sandbox run had six run-lock failures because `ps` was denied and three fixture failures: the new centralized token gate bypassed the existing delayed account-read path. Routing account reads through their existing authenticated implementation fixed the fixture regression; the complete run used process-list access and passed without changing assertions.

The targeted real-browser run (`pnpm e2e messages.spec.ts ending.spec.ts groups.spec.ts`, via the main checkout's sanctioned environment wrapper) exited 0: 126 passed in 34.6 seconds, across Chromium, Firefox and WebKit.

The first complete browser run exited 1: 910 passed and 7 failed in 6.0 minutes. Chromium diagnostics captured simultaneous `ERR_NETWORK_IO_SUSPENDED` failures for notifications, unread channels, join requests and groups, followed by successful reconnect/refetch. Six Firefox traces paused together for approximately 214 seconds, including context creation before application startup. Read-only system power logs confirm sleep at 22:53:27 and wake at 22:57:01 on 2026-10-31 (Europe/Moscow). This is failed execution evidence, not a pass; no timeout or assertion was weakened. Subsequent complete runs of corrected implementations are recorded below. Raw logs: `/private/tmp/ovc-f2-targeted-e2e.log`, `/private/tmp/ovc-f2-full-e2e.log`; browser traces remain in ignored test-results for run 23467.

## Corrections under verification

History refresh now retains its oldest shown sequence and reads successive REST pages until that boundary is reached, including more than 100 missed messages. Channel-list continuation pages detach when the refreshed first-page cursor changes; explicit continuation uses the new boundary. A fresh detail check gates remounts, channel events remove inactive history/detail and all cached list previews, and confirmed scope loss clears all channel data without resetting a still-authorized read-only conversation. Switching edit targets retains the original compose draft; refused and remote deletes restore focus to the message. The shared coalescer suspends on pagehide/beforeunload and resumes on page restoration, so a delayed read reply cannot restart hidden-page requests. Regression tests exercise each path.

The cache correction affects a trust boundary (T-6/T-29/T-44). The page lifecycle correction affects resource isolation (T-20), without changing server authorization. No API contract, dependency, assertion, timeout or verification policy was weakened.

`tools/docs/check-docs.sh` exited 0 (2,094 links checked, zero errors, all six Mermaid sources parsed). The startup test-control refusal/absence checks exited 0 in 8.860 seconds. These checks precede the final correction review and do not replace final checks of changed files.

## Fresh combined review

The permitted fresh combined review applied the standard checklist to corrections and the full security profile to F2. It closed the reconnect, first-page boundary, draft, deletion-focus and pagehide findings, and the stale detail/history portion of scope eviction. It retained one security major: a channel-list continuation response already in flight could restore a removed channel and preview after cache filtering, especially when the first-page cursor did not change. This is a continuation of the original T-6/T-29/T-44 finding, not a severity downgrade.

The additional correction cancels in-flight list requests on scope changes, advances a cache scope revision, detaches continuation queries from the old revision and permits continuation only after a first page fetched under the current revision. Confirmed 404 schedules the same bounded shared refresh. A held-response regression checks that late continuation data cannot restore the removed preview, and that fresh continuation still works. Conversation mutation callbacks also check the captured account epoch and channel-query identity before applying results; a held response during account adoption tests the interval before React unmounts the old conversation. These corrections touch the same trust boundary.

The prior corrected snapshot passed the full browser run: 917 tests across all three engines, exit 0, in 3.0 minutes (`/private/tmp/ovc-f2-afterfix-e2e.log`). Another project started checks during that run, so its duration is not an idle-device measurement. That run predates the final held-continuation correction and does not prove the final tree. Final correction checks and the owner decision on further independent review are recorded below.

## Final held-response correction checks

The final source passes `pnpm typecheck`, `pnpm lint`, `pnpm format:check`, `pnpm test` (639 tests, 58 files, 5.04 seconds), `pnpm api:check`, `pnpm build`, `pnpm check:classes` and `pnpm check:bundle`, all exit 0. The entry/language total remains 175.2 KB. The implementation-specific regression run passed 101 tests before this complete run. `git diff --cached --check` and staged Gitleaks exited 0, with no detected leaks. The final complete E2E run exited 0: 917 passed across Chromium, Firefox and WebKit in 2.2 minutes (`/private/tmp/ovc-f2-final-e2e.log`).

On 2026-10-31 the owner approved one additional independent security-profile review, also applying the standard checklist to the final corrections, because the combined re-review retained a major finding. That review must close the finding before the merge gate and merge.

Final frontend evidence identifies the staged frontend diff against the base: SHA-256 `89a652b18050f2b2c6c5cd96a5c09e24c687703076a9d9cb996722725fbee219`, computed by `git diff --cached -- frontend | shasum -a 256`. Documentation status updates do not change this source fingerprint.

## Authorized readiness review

The additional independent reviewer applied the full security profile to F2 and the standard checklist to the final corrections. The review passed without new blocker, major or minor findings and closed the remaining T-6/T-29/T-44 major. It confirmed cancellation before cache filtering, revision-bound continuation, fresh first-page and detail gates, the confirmed-404 refresh, and account-epoch/query-identity guards before asynchronous results are applied. The held-continuation and account-adoption regressions exercise real UI/query-cache behavior through the configured fake transport. The correction touches the membership and account-isolation trust boundaries; server authorization is unchanged.

The reviewed frontend fingerprint matches the one above. The reviewer also confirmed the prior history, pagination, draft, focus and pagehide corrections. Read-only inspection and diff checks passed; runtime and scanner evidence remains owned by the coordinator. A dedicated browser test of the held-continuation race was not added: the deterministic component regression covers it, while real-browser suites exercise chat and scope refusals. Attachments, downloads, presence and typing UI remain F3. No known readiness findings remain.

The final tier remains standard and the security trigger remains met. All change checks passed; the full repository checks not separately run for this frontend change are deferred to the merge gate. The exact-tree gate and finish remain required before reporting a merge.
