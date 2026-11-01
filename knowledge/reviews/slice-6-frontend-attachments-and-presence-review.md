# Chat attachments and presence verification

Base: `29fc9090ea3c9ff92b0be15fc45e2289a04d09b8`. Scope: [approved F3 plan](../plans/slice-6-frontend-attachments-and-presence.md). Status: implementation and independent readiness reviews complete; the first exact-tree merge gate failed a browser fixture assertion; the corrected browser suite passed; owner authorization to amend was granted on 2026-11-01; the new-tree gate remains pending.

## Planning review

An independent security reviewer examined upload/session integration, resource lifetime, bounded signed-address refresh, and the role-specific realtime projection. No blocking findings remained after the plan explicitly retained token-expiry-only replay, cancellation during refresh, multipart memory bounds, non-resetting picture recovery and no anonymous identity inference. No implementation checks were claimed by this review.

## Implementation evidence

The multipart body travels as a typed FormData dependency beside the typed API request. The request cloned for authentication replay has no body stream; the browser upload adapter receives the original File/FormData, without base64, ArrayBuffer or Blob conversion. Four active transfers bound simultaneous browser serialization, while at most ten File references are retained in the queue; no precise browser-native memory measurement is implied. Progress is a callback, never a header. Completion, cancellation and unmount release file references and handlers.

Targeted component checks passed initially (68 tests); upload queue and image recovery tests passed (6 tests). The first full component run inside the sandbox failed six existing run-lock checks because `ps` was refused with `EPERM`; 649 passed. The complete run outside that sandbox passed all 655 tests without a code change for those failures. Initial typecheck, lint, production build, class and bundle checks passed. Subsequent changes require revalidation.

The first targeted browser run passed 14 of 21 tests. Failed evidence is retained: both digest scenarios on all three engines, and the Firefox paste/download scenario. The frozen clock placed a message at exactly the closing presence instant, excluded by the digest's strict message-after-last-seen predicate; a helper treated a token-expired response as final instead of observing Session replay; the synthetic Firefox clipboard event did not carry its transfer. These are being corrected without longer timeouts or weakened assertions. A readiness-wait test edit overlapped this run, so it is diagnostic evidence rather than a frozen readiness snapshot. Another project was active during its launch; its duration is not a performance measurement.

## Verification scope

The historical checkpoints below retain failures and subsequent corrections. Final status is recorded at the end of this record. The exact-tree merge gate and finish remain pending. Slice-level performance and cumulative layer reviews follow the planned changes and are not claimed by this task.

## Independent readiness reviews

Both initial reviewers examined the same complete 42-file snapshot, with SHA-256 of sorted path/NUL/content/NUL records `e25592e578fa67865b6eb67be41e893a3b257097f76b018fbbe47dd69d6fa764`.

The standard review found a major verification gap: attachment-only send and stable body/attachment identifiers/idempotency key after an uncertain response were not exercised. It also found a minor focus loss when removing an upload row or clearing a focused refusal on a read-only transition.

The security review found a major verification gap at T-2/T-29: the new multipart/XHR branch had no shared fake/production contract and insufficient replay/cancellation/account-switch evidence. It also found a minor T-20/T-29 lifecycle defect: the separate one-second history-refresh coalescer survived pagehide and hidden visibility after the expiry timer itself was cancelled. No authorization bypass or credential leak was established.

## Corrections and revalidation

The fake chat adapter now implements scoped pending text uploads and binding, and records typed send payloads. Component tests exercise both attachment-only and text-plus-file sends through the composer and typed client, lose the committed response, repeat with equal body/identifiers/key, and prove one stored message and one binding. Shared row-removal focus restores the next row or composer, including a focused refused upload cleared by read-only state.

The shared upload contract runs against the fake and the actual production browser XHR module: multipart metadata and UTF-8 byte count, nonempty bounded nondecreasing progress, native pre-abort reason and out-of-scope refusal. A browser lifecycle scenario observes a token-expired multipart response followed by one refresh and a successful replay; it holds real requests without changing them and proves actual aborted requests on removal and sign-out, followed by an empty next-account composer. Test modules and build metadata remain outside served output; the browser imports only the existing production upload chunk discovered from the filesystem by the runner. No runtime patch, changed request, fulfilled route or new dependency is introduced.

Pagehide/hidden visibility now cancel the pending history-refresh coalescer, and cancellation prevents a replacement image request. The regression proves no history request during hide and expiry refresh after return. These corrections touch the browser resource-lifetime boundary (T-20/T-29); the upload contract and fake corrections verify the authentication boundary (T-2/T-29) without weakening production authentication.

The corrected original 21 browser cases passed on Chromium, Firefox and WebKit in 16.9 seconds. A subsequent 27-case run passed all six new upload-contract/lifecycle cases but remained failed overall (25 passed, two failures): Firefox's digest fixture assumed the expired token would first be observed by message POST, while background GETs won the race; the fixture now signs the sender out before advancing and signs in afresh, leaving expiry testing in the dedicated upload test. WebKit's typing case exceeded its deadline during a confirmed system sleep from 00:23:05 to 00:38:06 on 2026-11-01; `pmset` recorded Maintenance Sleep. No timeout or assertion was weakened. The 15.4-minute duration is failed infrastructure evidence, not a performance measurement.

On 2026-11-01 the complete frontend checks passed after those corrections: typecheck, ESLint and source/security policy checks, formatting, API generation consistency, 664 component tests in 63 files (5.98 seconds), production build, class check and bundle check. Backend test-control isolation checks passed in 7.796 seconds. Documentation checks passed before this evidence update and require a final recheck. Final full E2E, staged Gitleaks, independent after-fix review and merge gate/finish remain pending. Another project resumed CPU-heavy work, so the full browser run waits while the read-only after-fix reviewer works.

## Independent after-fix review

A fresh reviewer applied the standard checklist to the fixes and the full security profile to the security fixes, tracing upload authentication/refresh/cancellation through queue and composer, and history refresh through signed-image recovery. The reviewed 46-file snapshot fingerprint was `95ef2ed24b36616844a34a759bbd5bf08e5b8c6b3bf03a1561770bf60d2eeeec`.

The reviewer closed both major verification findings and both minor findings: attachment-only/stable retry coverage, shared fake/production upload contract with real XHR lifecycle evidence, row-removal/read-only focus, and hidden-page coalescer cancellation. No new blocker, major or minor finding was reported. The review made no claim that the then-pending full E2E or merge gate had passed. No production source changed after this review; subsequent changes correct browser fixture synchronization and record execution evidence.

## Final browser fixture corrections

The first complete browser run passed 940 of 944 cases and failed four. Three failures came from a new exact account-menu locator omitting the account name; the accessible name is `Boris, Account menu`. The fourth was the upload expiry fixture in Firefox: the WebSocket could renew the token before a new upload started, legitimately producing only `201`. The fixture now holds the already-created real upload request, advances business time, and continues that request unchanged; the captured old token must receive `401`, and the existing Session then performs the successful replay. The contract still requires `[401, 201]`, one refresh and the original metadata. No application behavior, interception permission, assertion or timeout changed.

All 27 targeted browser cases then passed in 15.9 seconds on all three engines. Typecheck, targeted ESLint and formatting passed for these fixture changes. The complete browser rerun is pending at this checkpoint. These changes only correct fixtures; the independently reviewed production source is unchanged.

The next complete run passed 943 of 944 cases. WebKit reported an intentionally cancelled channel-list request in the existing inaccessible-channel case. Its trace shows the list request began immediately before the channel-detail `404`; F2's `evictChannel` deliberately cancels every list query before filtering cached rows. The first fixture correction declared `/api/v1/channels` cancellation on both pages and added assertions that neither page contains the private group label. That attempt was incorrect: the diagnostic helper requires a declared cancellation to occur, and the narrow student page does not mount a list. The following complete run remained failed (940 passed, four failures): three missing-cancellation assertions and a Chromium sign-out-everywhere case that raced the initial unread request. No production cancellation behavior or diagnostic policy changed.

The inaccessible-channel fixture now holds the desktop list request until channel denial, ensuring the intended cancellation, while the narrow student page retains its exact refusal expectation without declaring an absent list request. Both pages retain the private-label absence assertions. A targeted run initially failed three cases (66 passed) because expiring one-shot route registrations let Playwright handle a held route before its explicit continuation; retaining registrations until the held requests are released fixes that helper lifecycle. The sign-out-everywhere fixture now waits for the existing ready/refetch completion signal before acting. All 69 cases in `messages.spec.ts` and `account-edge-cases.spec.ts` then passed in 22.8 seconds across Chromium, Firefox and WebKit. Typecheck, targeted ESLint and formatting passed. No retry, longer timeout, changed HTTP request/response, diagnostic exemption or removed assertion was introduced.

## Readiness checkpoint

The final complete browser run passed all 944 cases in 2.5 minutes on 2026-11-01. The production code remains the independently reviewed snapshot; the subsequent fixture synchronization changes passed the full frontend lint/security-policy and formatting commands, and the complete browser run. The final classification remains standard with the security trigger met. Both major review findings and both minor findings are closed by the independent after-fix review above.

Commands below exited zero. Environment-dependent commands use the sanctioned `tools/dev-env.sh` from the main checkout without opening or copying environment files; browser commands run with temporary `caffeinate -is` protection against automatic idle sleep.

| Command | Evidence |
|---|---|
| `pnpm typecheck` | All TypeScript projects pass |
| `pnpm lint` | ESLint and source/security policy checks pass, including final fixtures |
| `pnpm format:check` | Complete frontend formatting passes |
| `pnpm api:check` | Generated API is consistent |
| `pnpm test` | 664 tests, 63 files, 5.98 seconds |
| `pnpm build` | Production build passes |
| `pnpm check:classes` | Class check passes |
| `pnpm check:bundle` | 175.5 KB entry plus largest language; within all budgets |
| `tools/dev-env.sh pnpm e2e` | 944 passed, 2.5 minutes; earlier failed attempts retained above |
| `tools/check-tool-binaries.sh` | Pinned binary build metadata matches |
| `tools/dev-env.sh go test ./internal/conventions -count=1` | Pass, 2.328 seconds |
| `tools/dev-env.sh go test ./cmd/internal/startup -run 'TestProcessesRefuseUnsafeTestControl\|TestTestControlRoutesExistOnlyWithTheFlag\|TestRealtimeTestRoutesAreAbsentWithoutTestControl\|TestSeedingRoutesAreAbsentWithoutTestControl' -count=1` | Test-control start-up and route isolation pass, 7.796 seconds |

Final documentation checks and staged Gitleaks run after this evidence update. Full backend race/static checks, dependency audits, guard/wrapper/gate self-tests and history Gitleaks are deferred to the exact-tree merge gate, not claimed as passed at this checkpoint. The gate and finish transcripts, including the tested and merged trees and every command status, are retained under `.git/ovc-merge-reports/` after completion; this record is not amended after its tested commit. The 260-second gate limit remains mandatory. Slice-level measurements and cumulative layer reviews remain a separate phase.

Observability impact: uploads preserve the existing structured API failure and correlation path; no filename, body, byte content, signed address or token is added to diagnostics. Realtime signals remain transient authorized UI state, without identity inference or content logging. Browser diagnostics and refusal/lifecycle tests exercise these paths. Precise native multipart memory measurements remain unverified until slice-close performance review; the bounded serialization/reference analysis above is not substituted for that measurement.

## Failed gate and browser event synchronization

The first exact-tree gate tested commit `3122126de2a4557b7eb8d885833d6043ecf92c6e`, tree `6b1bb07ffd584b8efd01052ad7684c6e5ffc6d00`, against base `29fc9090ea3c9ff92b0be15fc45e2289a04d09b8`. It failed in 241.9 seconds: all 29 other commands passed, but the browser command passed 943 cases and failed the WebKit inaccessible-channel fixture because its declared aborted list request had not been observed. This is a failed assertion, not a passing gate or an infrastructure retry. The complete command report is retained at `.git/ovc-merge-reports/3122126-frontend-attachments-failed.log`; main was not advanced.

The fixture previously released its held list request after observing the visible refusal text, before explicitly waiting for the focus effect and network cancellation event. It now asserts the specified focus on both refusal notices before release, then awaits the actual `requestfailed` event before diagnostic verification. This adds observable lifecycle barriers without changing production code, requests, responses, timeout limits, retries or diagnostic policy. All 12 Messages browser cases passed across the three engines in 13.5 seconds, with typecheck, targeted ESLint and formatting passing. The full browser run of the correction then passed all 944 cases in 2.4 minutes. The failed gate has not been rerun on its unchanged tree. On 2026-11-01 the owner authorized amending the local F3 commit with the six added test lines and evidence updates, preserving the one-commit branch rule. Final lint, formatting, documentation checks and staged Gitleaks passed before that approval. A new exact-tree gate is required before merge.
