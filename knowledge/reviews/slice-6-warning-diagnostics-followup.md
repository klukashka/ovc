# Browser warning diagnostics and development smoke follow-up

Status: implementation in progress; independent reviews and runtime browser verification pending.

Base: `4d9ba1e5467c84b1a09c1737c00ea2c8e4c24271`.
Verification tier: standard. Security trigger: diagnostics/redaction, generated private material,
and the development file-serving boundary (T-19, T-27, T-36 and T-37).

The change completes the warning-observation finding from the slice 6 frontend review under
`knowledge/plans/slice-6-frontend-review-followups.md`. Context-level observers are installed
before application navigation. Explicit observed-context construction covers additional contexts,
including sign-up journeys that use the base fixture, and all pages/popups in each context.
Measurement consumers include browser, context, history-refresh and socket browser observers.
No product behavior assertion is removed by these constructor substitutions.

Safe exports contain categories, bounded counts, engine, stage and route classes. Unknown warnings
fail; warnings cannot consume an exact negative error expectation. Private Playwright traces may
contain generated credentials and token-bearing URLs and are not certified as redacted.

The owned development fixture selects `measurement-dev-tls`, reuses the two existing preview TLS
settings, and generates its disposable pair outside frontend sources. Its preparation mode does
not create an unused preview private key under the source tree. Default preview remains unchanged.
Lexical/canonical/parent-path checks and fixed-category framework logging confine private material.

## Implementation checks

- Frozen dependency installation: two offline attempts failed because the cache lacked tarballs;
  the normal pinned installation then passed without lockfile changes.
- Early `pnpm typecheck` failed on a mechanical constructor type substitution (exit 2), then on an
  undefined socket-measurement engine name (exit 1), then on logger error-union typing (exit 1),
  then on the Vite configuration type export (exit 1). These implementation defects were fixed;
  a subsequent typecheck passed (exit 0).
- `pnpm exec vitest run scripts/development-tls.test.ts --project node` initially failed (exit 1,
  one of four cases): a parent-directory alias could traverse the served tree to an external
  symlink target. The confinement check now includes the canonical parent directory; the same
  command passed all four tests (exit 0). This fix touches the development file-serving boundary.
- Initial targeted ESLint failed (exit 1) on eight explicit-return/async/assertion style findings;
  after the corrections the same targeted command passed (exit 0).
- Real console canary, development HTTPS/file-access probe, development three-engine smoke,
  production journeys, complete default suite, full change checks and exact-tree merge gate:
  pending execution. No readiness or independent review verdict is implied.

Development preparation fails closed when its checkout already contains the preview TLS directory;
it never removes unverified source-tree material. The development smoke therefore runs in a fresh
checkout before production end-to-end tests; repetition after those tests uses another checkout.
A named preparation contract test covers refusal, with ordinary preview behavior unchanged.

The subsequent full frontend lint and format checks passed, as did the production build,
class check and bundle check (175.5 KB first-load entry plus language, unchanged). An initial
API-schema check was blocked by sandbox Go-cache permissions (exit 1), not an assertion failure;
its environment-corrected run passed (exit 0), and the full frontend test suite passed 66 files and 732 tests (exit 0).

The bounded lifecycle control uses a real reserved-port strict-bind failure and a real navigation
to an owned closed local port. Private typed callbacks retain owned PIDs and generated directory
paths only for absence checks; exports contain fixed outcome/count fields. Named-mode readiness
uses the existing health route through HTTPS, and temporary material is removed only after all
owned children actually exit, while stop errors remain failures. These newly added runtime
controls have not yet run. Their final source checks and independent reviews remain pending.

Final source verification after the lifecycle implementation: `pnpm typecheck`, full `pnpm lint`
(including newly staged Git-listed files), `pnpm format:check`, `pnpm build`, `pnpm check:classes`
and `pnpm check:bundle` passed. The complete `pnpm test` passed again: 66 files, 732 tests,
5.31 seconds. `pnpm api:check` passed after correcting the sandbox Go-cache permission failure;
the later changes affect only diagnostics/deployment tooling and do not change API schemas.
The bundle remains 175.5 KB for entry plus the largest language chunk, under the unchanged
180 KB hard threshold. Runtime browser controls, complete production end-to-end tests,
backend test-control refusals, documentation checks, Gitleaks, independent standard/security
reviews and the exact-tree merge gate are still pending; no readiness claim is made.

## Development sign-in Origin correction

The standard review identified a major defect in the development smoke: its API request-context
login omitted the browser-generated Origin required by the existing authentication contract and
would return 403. The smoke now navigates its already-observed page to the existing same-origin
HTTPS health endpoint, then performs the real JSON login with browser `fetch`. That non-application
bootstrap starts no application session check. The browser supplies Origin naturally; the smoke
asserts its exact configured-origin match privately, checks the 200 response, then navigates the
application and retains the Secure/HttpOnly cookie, refresh and zero-diagnostic assertions.
No request header is overridden, no new endpoint is introduced and no failure is exempted.

This fix exercises the existing authentication trust boundary without changing its enforcement.
Its runtime success and independent closure verdict remain pending; static checks alone do not
prove that the three engines complete the journey.

Correction checks passed (exit 0 each): `pnpm typecheck`,
`pnpm exec eslint --flag unstable_native_nodejs_ts_config --max-warnings 0 scripts/check-development-warnings.ts`,
and `pnpm exec prettier --check scripts/check-development-warnings.ts`.
No development deployment or browser run was executed for this correction; those acceptance
checks remain pending coordinated runtime verification.

## Standards-mode console canary correction

The real console canary failed (exit 1) after Chromium reported the expected two warnings and
one error, before Firefox produced its success record. A separate real-browser diagnostic
confirmed that the synthetic HTML lacked a doctype and triggered Firefox's additional quirks-mode
warning. The full security review classified the missing doctype as minor; the synthetic response
now declares HTML standards mode. Expected diagnostic counts and warning/error assertions are
unchanged, and no warning allowlist is added. The rerun remains pending.

The fresh full security review and standard review of the sign-in correction closed the original
major Origin finding at source level and reported no new major finding. This is independent
source-review evidence, not runtime sign-in or slice readiness evidence. The real HTTPS/file-access
confinement control also failed (exit 1); its cause remains under investigation and it is not passed.

The doctype correction's targeted ESLint and Prettier checks passed (exit 0 each). No runtime,
full test suite or commit was performed for this correction.

## Filesystem-path encoding control correction

After the doctype correction, the real console canary passed in Chromium, Firefox and WebKit
(exit 0), with the expected two warnings and one error in every engine; the safety monitor
confirmed unchanged source and safe output. This closes the canary runtime check, not the
remaining development smoke or production verification.

The first real confinement control failed (exit 1) because encoding the entire filesystem path
also encoded its slashes, causing Vite to treat the request as an SPA route and return HTML 200
without private material. The reviewer classified this control defect as minor. The encoded-path
case now percent-encodes one alphabetic character while preserving filesystem separators, so it
continues to address the real filesystem route. Strict 403/404 and private-material-absence
assertions are unchanged. This is a test-input correction and changes no trust boundary or server
permission. A separate real diagnostic returned 403 without private material for all four
certificate/key path variants, but the corrected confinement control itself has not rerun and
is not yet passed. The original failed control remains part of the verification evidence.

The encoded-path correction's targeted ESLint and Prettier checks passed (exit 0 each).
No runtime, full check run or commit was performed for this correction.

## Real browser verification and remaining development findings

The complete initial standard source review identified the missing-Origin major. The subsequent
fresh full-security review also applied the standard checklist to that fix and confirmed it closed
at the source boundary, with runtime still required. It found no new blocker or major. Its minor
canary finding and the later encoded-path fixture clarification are fixed without relaxing their
strict assertions. The reviewed complete source snapshot was tree
`a4636b8b949ac198f7c7f843bc876d78f2fa3d1a`; the two subsequent minor fixture corrections were
verified on candidate tree `3074afc39683f45fccd3ec2b6db23c7bca1cf827`.

Actual command outcomes, all retained rather than replaced by later results:

- The first private runner invocation stopped before launching a browser because the supplied
  manifest used an array where the observer expected a path-to-digest map. A separate converted
  manifest corrected this runner input; no repository code was changed for it.
- `node scripts/check-browser-diagnostics.ts` initially failed in Firefox because the synthetic
  HTML omitted its doctype. After the minor fixture correction it passed in Chromium, Firefox
  and WebKit: each observed exactly two warnings and one unexpected error, consumed its exact
  expected error, and exported neither the synthetic secret marker nor the local origin.
  Runtime: 1.97 seconds, exit 0; both observation samples were safe and source was unchanged.
- `node scripts/check-development-confinement.ts` initially failed because percent-encoding
  every slash selected a SPA fallback instead of the intended filesystem path. Safe diagnosis
  observed normal 403/no material and fully encoded 200/no material; no disclosure was found.
  After correcting the encoded character while preserving path separators, the real command
  passed, exit 0, 0.30 seconds, with unchanged 403/404 and no-material assertions and two safe
  observation samples.
- `node scripts/check-development-lifecycle.ts`, through the sanctioned wrapper, passed both
  actual strict-port startup failure and actual closed-port navigation failure controls. Owned
  processes exited, ports were released and owned TLS directories disappeared. Exit 0,
  23.17 seconds, six safe samples, source unchanged.
- The targeted real backend checks passed in internal/core and internal/api: unsafe test-control
  combinations and database names/hosts are refused, loopback test-control is accepted, and
  test routes remain absent without the setting. The exact five-test selector and output are
  retained with command exit 0, 3.21 seconds, two safe samples and unchanged source.
- `node scripts/check-development-warnings.ts` failed, exit 1, 54.70 seconds. Twelve observation
  samples were safe and source was unchanged. HTTPS readiness, native login 200, natural Origin,
  composer and secure cookie/refresh proceeded; the later visible history assertion timed out
  on an empty zero-height list. This is not a successful development smoke.

Separate narrow diagnostic helpers are not acceptance repetitions. They confirmed the empty
list and identified development console errors as the pinned TanStack missing-query-function
message for the disabled internal channel-scope revision observer. They also observed three
initial aborted GET classes: unread, channel list and current channel. StrictMode teardown is a
source-supported cancellation hypothesis; request-level causality and successful replacement
completion remain unverified. No console/error allowance or cancellation exemption has been
added. A proposed extension requires explicit approval before changing the application observer
or development expectation semantics; production diagnostic rules are unchanged.

The diagnostic helpers retain their exit-1 results, including deliberate diagnostic stops and one
invalid callback that raised ReferenceError before useful cancellation evidence. No listener
remained on either owned API/frontend port after that invalid helper. Its one generated TLS
TEMP directory was identified by the unique recorded creation interval, exact fixture filenames,
current owner/mode and absence of open holders, then removed without reading key contents.
This corrective cleanup is not evidence that the crashed helper performed normal cleanup.

Private evidence is retained outside served output under `/private/tmp/ovc-warning-canary-*`,
`ovc-warning-confinement-*`, `ovc-warning-development-*` and `ovc-warning-backend-refusals-1`.
The record is not a readiness declaration. Corrected complete development smoke, measurement
consumer smokes, full production E2E, final change checks and exact-tree gate/finish remain
pending. No commit or merge of this change has occurred.

Documentation/scanner preparation initially failed with exit 127 because this isolated checkout
had neither the pinned Markdown tools nor its own Gitleaks executable. The prescribed frozen
installation and pinned Go tool build passed without lockfile/module changes; the embedded-tool
verification passed. The complete documentation check and staged Gitleaks then passed, exit 0.
The final evidence-only addition records those setup outcomes; it does not authorize or implement
the proposed development findings extension. Both that extension and the separate measurement
coverage completion await approval before their code changes. The slice remains open.

## Approved development findings extension

The owner approved the security-reviewed development findings plan on 2026-11-02. The earlier
proposal-only statements above describe the state before that approval. The plan is now included
as `knowledge/plans/slice-6-development-smoke-findings.md`; no earlier failed smoke or diagnostic
is reclassified as a passing acceptance sample.

The application correction supplies TanStack's existing `skipToken` to the disabled scope-revision
observer, preserving its key, initial value and cache subscription. A component regression checks
that it cannot fetch, eviction advances the revision and stale detail/history cache is removed;
the existing held-pagination scope-loss regressions remain unchanged. The development fixture
seeds real visible history in both groups and uses actual UI links for the channel switch and
Profile navigation, with exact heading/path checks.

The opt-in initial-read helper freezes exact origin/method/path/query targets and document/account
ownership before the authenticated application document. Its only eligible failures are explicit
engine cancellations of first attempts for the three named GETs. First ready, first complete
successful target response or the unchanged fixture readiness deadline closes the window forever.
A designated later request must complete successfully for every matched cancellation. Per-request
ordinals identify first attempts; replacement start before cancellation delivery is a named failure,
not an expanded event-order allowance. Native user-response identities validate the actual account.
Async observations are retained/awaited and handler failures produce safe failure categories.

The initial-only native entrypoint reports per-target attempts, cancellations, replacements,
complete responses, window/ownership state and a fixed missing-query-function count. The coordinator
must compare the unchanged candidate with an isolated, explicitly patched copy without StrictMode,
and separately confirm that the missing-query-function baseline fails the console requirement.
These comparisons, the complete development smoke and real rejection controls remain unrun for
this extension. Source-supported StrictMode teardown remains a hypothesis until that actual proof.
The shared production collector and default request-failure rules have no new abort allowance.

The new real rejection control uses owned native browser fetch/AbortController requests and only
hold-then-continue barriers, covering outside-window/repeated cancellation, absent/failed
replacement, HTTP errors, other target/query, replacement document/account and an actual UI channel
switch. Its account control performs real native login/refresh/user lookup, requiring success and
a private match to the replacement account while the observer checks against the original owner.
No token, raw target, error text or account identifier is exported. These controls are pending,
not passing merely because their state-machine counterpart passes.

Implementation checks: the first typecheck failed on non-erasable constructor parameter properties
(exit 2); explicit typed fields fixed it. The initial targeted lint failed on seven return-style
and nullish-assignment findings (exit 1), then passed after correction. An initial targeted run of
the initial-read state-machine contracts and complete Messages component test file passed 2 files
and 68 tests (exit 0). Final checks and runtime independent review/acceptance remain pending.

The extension's final targeted typecheck, ESLint and Prettier checks passed (exit 0 each). The
last targeted contract/component run passed 2 files and 68 tests (exit 0, 3.79 seconds). No real
development/control deployment, full suite, commit or merge was run for this extension. The
initial-only causal comparison is the next feasibility gate; an incompatible real event order
requires stopping rather than widening the expectation.

## Initial-read review corrections and observed stop boundary

The fresh standard review found one major: the repeated-cancellation, document and account
controls accepted any rejection, allowing their still-unresolved first cancellation to mask a
missing target guard. Its minor found that a designated replacement could complete after the
original deadline. The fresh full security source review reported no new finding. Runtime
acceptance remained stopped; these source verdicts are not a successful development journey.

The real initial-only candidate control failed with three aborted requests: one
`replacement_started_before_cancellation` and two other unexpected failures under the declared
window. The actual StrictMode-off control also failed its readiness wait after 30 seconds,
despite zero observed aborts/unexpected failures, validated owner and complete post-ready target
reads. Neither run passed; in particular, zero observed aborts alone does not establish a valid
StrictMode-off acceptance result. The raw event-order stop is preserved. No request-start or
late-callback eligibility redesign is implemented by this correction.

Every native rejection control now requires its exact intended failure category. The account
case completes the original-token replacement successfully after actual other-account login,
then proves the new account through native refresh and user lookup, so an absent replacement
cannot mask account-boundary detection. The repeated and document cases also issue later real
reads and require their own named guard rather than any remaining error. Matching contract tests
require exact reasons; document/account contracts let the old replacement complete and require
zero unresolved work before asserting the guard's independent failure.

The complete state-machine contract file passed 20 cases. Separate isolated source controls retain
explicit patches and test output: the unchanged source passed all three independent-guard tests;
removing only the repeated, document or account guard made its corresponding named test fail
with an assertion, exit 1 each. Other remaining errors could not make these sensitivity tests
pass. These are pure state-contract controls, not the still-pending real browser negative controls.
A successful designated replacement completing at or after the original deadline now records
`replacement_completed_after_deadline`, remains unresolved and fails; deadline-boundary cases
are included in the passing contract run.

Source inspection also found that readiness waited for all resources, including possible
long-lived development transport requests, rather than only pending REST API requests. The
barrier now counts API requests only, while every failed API or non-API request remains observed
and fails unless covered by the unchanged narrow initial expectation. Fixed-category diagnostics
add API/non-API pending resources, state pending/unresolved/ready/closure/deadline and outstanding
owner-response observations. This is not a claim that the previously failed off-control passes;
its corrected runtime result remains unverified.

The targeted typecheck and ESLint passed (exit 0). Original failed samples and the event-order
stop remain evidence. No browser/deployment run, commit, merge, new review or eligibility expansion
was performed for these fixes. Any redesigned cancellation interpretation requires its separately
approved plan before implementation.

## Native rejection-control setup and barrier correction

The complete real native rejection-control run failed (exit 1), 294.53 seconds. All ten cases
failed; none is a passing guard sample. All sixty safety-monitor samples were valid and source
was unchanged. Both owned API/frontend ports were confirmed to have no listener afterward.
Nine failures had no body-level read-state report, so a shared setup failure before that report's
try/finally remains a possible cause; the failure is not attributed solely to cancellation logic.

Source inspection found a separate barrier cycle: the control awaited Playwright requestfailed
while its route remained held, although the native AbortController promise had already returned
AbortError. The request is now continued unchanged immediately after that native outcome, then
the control waits for the browser failure event. Release is idempotent and the same promise is
awaited in cleanup, preserving continuation errors. Whole-case fixed stages and state/counter
reports now cover setup, owner confirmation, barrier/abort/continuation, expected rejection and
exact-reason verification before cleanup; a second report distinguishes cleanup outcomes.

Native login and GET helpers, and the smoke's native login/explicit refresh, consume complete
real response bodies before returning status, without retaining or exporting those bodies. Safe
pending API diagnostics identify only fixed route classes, state-bound/header-seen booleans and
counts, alongside the existing owner/ready/pending state. An explicit --control=outside_window
selector permits one diagnostic case; all ten remain the default. None of these changes broadens
the original initial-cancellation window, its event-order requirements, HTTP or console rules.

The first targeted lint found one optional-chain style issue (exit 1), which was corrected. Final
targeted typecheck, ESLint and Prettier checks passed (exit 0 each); all twenty initial-read state
contracts passed (exit 0). No browser/deployment rerun, review, commit or cancellation-policy
redesign was executed for this correction. The real setup cause and corrected native controls
remain unverified until coordinated execution; the earlier event-order STOP remains in force.

## Canceled-request continuation diagnostics and deadline checks

The single outside-window real control failed (exit 1), 22.41 seconds; all six monitor samples
were safe and source was unchanged. It observed the intended cancellation-after-window failure
category once, one raw browser failure, zero pending/unresolved requests, validated owner and
zero console diagnostics, but stopped during held-request continuation. This proves a guard
observation, not a passing native control. No other control is inferred to pass.

Pinned Playwright 1.63.0 source shows Chromium continuation sends Fetch.continueRequest through
its cancellation-tolerant protocol helper. The harness instead resumed the handler and removed
interception before awaiting that continuation, creating an avoidable ordering race. Cleanup now
waits for owned continuations before unrouting and drains any remaining handlers afterward. The
real route.continue call still has no overrides. Native cancellation success requires its exact
owned request, native AbortError, explicit net::ERR_ABORTED and open page/context/browser; the
fixed cancelled-request-continued outcome is returned and asserted.

No unobserved thrown error is accepted as canceled continuation. A small classifier exports only
fixed handled/target-closed/Fetch-continuation-protocol/timeout/unknown kinds, with safe native,
request-ownership and liveness booleans. Every thrown continuation still fails. If the ordering
fix is insufficient, the next single-case run can identify its category without exporting raw
error text or a URL; a new allowance is not inferred from cancellation alone.

The existing ownership deny guard also checks pending work of the frozen initial targets, even
when the window has closed and no canceled replacement remains unresolved. A contract verifies
this specific SPA failure; another ensures a new SPA target is not mistaken for old-scope work.
Initial settlement at or after the original deadline now fails even with zero cancellations and
all reads complete. A separate contract preserves later ordinary verification after timely
initial settlement. These tighten existing denial/deadline rules without changing cancellation
eligibility, StrictMode or budgets.

Targeted typecheck, ESLint and Prettier passed (exit 0 each). The continuation classifier and
initial-read contract run passed two files and 29 cases (exit 0). No real browser/deployment run,
new review, commit or lifetime-policy redesign was performed for this correction. The next
coordinated command remains the single outside-window control; every earlier failed sample
and the original event-order STOP remains retained.

## Exact HTTP 404 console expectation correction

The second full real native rejection-control run failed overall (exit 1), 24.41 seconds, with
all six monitor samples valid and unchanged source. Eight controls passed: outside-window,
repeated and missing replacement, different query and target, document and account replacement,
and ordinary SPA switch. Only failed-replacement and HTTP-error controls failed, at console
verification with one unexpected console error and zero warnings each. Their exact HTTP-failure
guard was observed, but neither complete control passed. These results do not turn the whole
command into a passing sample.

The two failed controls previously declared a hardcoded Not Found reason phrase. Their new
synchronous response handler is installed before native fetch and registers one exact expected
resource message only for the known GET, complete URL and 404 status. It uses the real
Playwright response statusText rather than assuming a reason phrase for HTTPS. The control
asserts actual 404, successful response.finished completion, one exact consumed error, no
remaining expectation and zero other errors or warnings. It adds no warning/error allowlist and
changes no shared observer API, HTTP guard or cancellation eligibility. Fixed empty/not-found/
other status-text categories and bounded consumption/error counts make a remaining mismatch
diagnosable without raw text, argument, URL or credential export. The actual mismatch cause and
corrected control outcome remain unverified until the next real run.

The first targeted lint failed on an unnecessary conditional caused by assertion type narrowing
(exit 1); a fresh expectation-count accessor corrected it. Final targeted typecheck, ESLint and
Prettier passed (exit 0 each); the unchanged continuation/state contracts passed two files and
29 cases (exit 0). No browser/deployment run, new review, commit or lifetime-policy redesign was
performed for this correction. The original event-order STOP and every failed sample remain.

## Native rejection controls after fixture corrections

The corrected complete native control command passed all ten cases on source tree
`09a7d5ff86da9763bf5d3fc3cbae64856d1199a5`: exit 0, 24.66 seconds, six safe observation
samples and unchanged source. Each case requires its own named rejection reason. The suite covers
outside-window cancellation, repeated cancellation, missing/failed replacement, HTTP error,
different query/target, document/account replacement and ordinary SPA switch. Both intended
404 responses completed, and each consumed exactly one scoped console expectation using its real
empty status phrase; no other console errors or warnings were accepted. Evidence is retained in
`/private/tmp/ovc-warning-native-rejection-controls-3`.

Earlier native runs remain failures: all ten cases failed in 294.53 seconds; after response-body,
barrier and scoped-observation corrections eight passed and two failed console verification in
24.41 seconds. The first isolated outside-window follow-up failed during continuation cleanup
in 22.41 seconds; the corrected ordering then passed in 22.53 seconds before the full suite.
These results verify the actual rejection controls, not a passing six-journey development smoke.
The original strict callback-order initial control remains failed and its conservative stop applies.
No request-start cancellation eligibility has been implemented or accepted by these results.

The new lifetime-evidence proposal requires owner approval before changing that interpretation.
The pending fresh combined after-fix review must confirm the negative-control major and deadline
minor closed against the eventual final source, including any subsequently approved boundary.
No commit, gate, merge or slice-closure result is claimed by this checkpoint.

## Complete checkpoint change checks

After recording the ten native control outcomes, source checkpoint tree
`d89557550812b174482ac3e4723aeb6d1dc99edc` passed every frontend source-row command:
`pnpm typecheck`, `pnpm lint`, `pnpm format:check`, `pnpm test`, `pnpm build`,
`pnpm check:classes`, `pnpm check:bundle` and `pnpm api:check`, each exit 0.
The full suite passed 68 files and 762 tests. The observed serial run took 23.08 seconds with
six safe samples and unchanged source. Entry plus largest language remains 175.5 KB gzip
against the unchanged 180 KB hard limit; Home and Today remain within their existing limits.
Command results and private logs are retained in `/private/tmp/ovc-warning-frontend-check-evidence`
and `/private/tmp/ovc-warning-frontend-checks-1`.

The five additional startup process tests passed with race detection: unsafe test-control startup
refusal, flagged-route availability, realtime/seeding route absence without test-control, and sink
refusal for a production database name. Exit 0, package 9.808 seconds; monitored wrapper
12.87 seconds, four safe samples and unchanged source. Evidence is retained in
`/private/tmp/ovc-warning-startup-refusals-1`. Together with the earlier five core/API cases,
these cover the named fixture-related refusal/absence checks. No full backend run is implied.

Only this evidence record changed after the executable snapshot used by the native controls;
the later source-row checks do not upgrade the failed initial control or authorize a changed
cancellation boundary. The ordinary production E2E suite, full six-journey development smoke,
final after-fix review and exact-tree merge gate remain pending. The owner-approved original
strict callback-order rule has not been relaxed. The separate request-lifetime clarification
is still awaiting the owner's answer; no new eligibility code has been implemented.

## Owner-approved request-lifetime refinement and producer evidence

The owner explicitly approved the recommended request-lifetime plan on 2026-11-02 after its
independent security planning review. The approved plan is retained as
`knowledge/plans/slice-6-development-cancellation-evidence.md`. This changes the verification
contract prospectively; the original failed candidate/StrictMode-off pair and its conservative
STOP remain failed samples on their exact earlier source, not retroactively passing evidence.

The initial helper now records each exact target's first-request eligibility immutably at its
observed start while the original window is open. Ready, complete target response and original
deadline still close that window irreversibly for new starts. An eligible first cancellation
callback may arrive afterward, but before the original deadline and without an ownership change.
Only the exact second same-target/document/account request can resolve it, with complete 200
before the deadline; its start and completion may precede delivery of the first cancellation.
The completed second identity/time is retained until both facts are known. A successful first,
late callback, wrong owner/query, failed/missing second or successful third grants no allowance.
All exact rejection reasons, deadline checks and raw failure accounting remain enforced.

The supporting FakeChatServer.observeInitialReads capability is passive and explicitly armed.
FakeAuthServer.send synchronously feeds method/address classification and its real request.signal
before the unchanged first await. Unarmed observation returns before parsing an address. Armed
observation retains only the first two real signal references per named target and scalar totals,
recording first.aborted exactly at the second transport invocation. No Request, header, body or
credential is inspected or retained by this observer; snapshots export no address or signal
object. Existing nextTurn, authentication, routing and response handling remain in their order.
Armed/unarmed fake contracts cover response/auth preservation, synchronous entry, bounded retention,
wrong query/origin/method and stopping observation.

Actual SignedInFrame plus MessagesPage run under the real FollowingQueryClient with and without
StrictMode. The three targets are each proved separately. StrictMode yields two observed/retained
lifetimes, firstAborted=true and firstAbortedAtSecondInvocation=true, with secondAborted=false,
for unread, list and current-channel. Without StrictMode each has one lifetime, firstAborted=false,
no second-entry value and secondAborted=false. Fixed scalar records are retained by the targeted
producer-evidence run; they establish producer lifecycle ordering, not browser event timestamps.
The new real browser comparison and complete development smoke remain pending.

Callback-order state contracts include second begin and complete before cancellation delivery,
closure by ready or another target, immutable ineligible starts, late callback/replacement,
wrong-owner second and non-substitution by a successful third. Native outside-window control
already starts its target after a real earlier target completes. A new late-replacement native
case holds the exact second GET to the original absolute readiness deadline, then continues it
unchanged and requires its complete 200 to be rejected. The failed-second native case also
restores owned membership and reads a successful third request, which cannot clear unresolved
second work. Default rejection coverage is now eleven cases; earlier complete ten-case results
remain bound to their tested source, not these unrun changes.

Implementation checks retain one failed new fake-method assertion: it expected 404 for an
unverified POST, but the unchanged audience guard correctly returns 403/email_not_verified.
The expectation was corrected to that existing contract. The first scalar-output attempt used
Node process in the DOM-typed test and failed type/lint; it now uses fixed structured console.info.
A late-control cleanup throw in finally was rejected by lint and moved into aggregated owned
cleanup. These are source/test fixes, not relaxed application behavior or hidden runtime failures.
Final targeted typecheck, ESLint and Prettier passed (exit 0). The full relevant state, fake
transport and Messages component files passed three files and 125 cases (exit 0, 4.01 seconds).
The final run additionally compares complete armed/unarmed response status, headers and parsed
bodies, and native repeated/missing/failed/late/ownership controls assert a successful initial
cancellation classification before exercising their separate rejection guard.
A separate named producer run with console output enabled passed both actual component controls
(exit 0), retaining all six target/mount scalar records. No browser/deployment, full E2E, new
review, commit or merge was run for this refinement; those coordinated checks remain pending.

## Lazy-query enrollment refinement

The owner approved the independently security-reviewed lazy-query enrollment plan on 2026-11-02.
The real initial control on tree 4d1c3eac51fb238799757ee19907e9674406abe6 remains FAILED:
exit 1, 23.7125 seconds, six safe/source-valid monitor samples. It observed three raw cancellations,
one expected and two unexpected cancellation_after_initial_window failures. Unread enrolled before
global closure; lazy list/current-channel first lifetimes did not. All three exact second responses
completed before the original deadline. No failed source is reclassified as passing.

Enrollment now belongs to each target first identity, before the original deadline, unchanged owner
and explicit assembly seal. Ready/unrelated completion remain required settlement evidence, not
cross-target eligibility cutoffs. The driver explicitly seals only after actual visible composer
and seeded history, owner validation, ready, full authoritative reads and request/async quiescence.
Own-first success, exact-second full response, third non-substitution and all strict failure rules
remain. The native outside_window case now establishes actual UI assembly then seals and cancels
an owned held request. This is composite-boundary rejection, not independent seal acceptance
sensitivity; deterministic synthetic reducer contracts separately prove seal classification.
No new fake seam or production scheduling change is introduced. Fresh real browser comparison,
all native controls and six journeys remain pending. Targeted typecheck, ESLint and Prettier passed
exit 0; state/fake-transport/real-Messages contracts passed 132 cases across three files in 4.00s.
A helper edit initially used a wrong relative path and failed before changing source; it was
corrected, and final type/lint/format were rerun successfully. No runtime, review or commit ran.

## Lazy-query enrollment actual verification

The frozen candidate tree 18b2d8e751cbd8ffd782b01948ab2b2508a3173d passed targeted
TypeScript, ESLint and Prettier checks and 132 state/fake-transport/real-Messages tests across
three files (4.00 seconds). These are check timings, not performance measurement samples.
The actual initial Chromium control passed in
`/private/tmp/ovc-warning-lazy-initial-candidate-1`: exit 0, 23.43 seconds, six safe monitor
samples with unchanged source, three raw cancellations, three expected and zero unexpected.
All three exact second lifetimes completed, with owner/UI/quiescence validation and explicit seal.
The complete eleven-case native rejection command passed in
`/private/tmp/ovc-warning-lazy-native-controls-1`: exit 0, 55.23 seconds, twelve safe monitor
samples and unchanged source. This includes the composite sealed-lifetime control and original
deadline rejection; it does not claim independent native sensitivity of a redundant seal guard.

The complete development smoke FAILED in `/private/tmp/ovc-warning-lazy-development-smoke-1`:
exit 1, 22.87 seconds, six safe/source-valid samples. The first Chromium tutor journey failed
at channel_switch with one unexpected_target_cancellation: four raw cancellations, three expected,
one unexpected. The additional request belongs to the fixed other category; existing aggregate
records do not prove its exact endpoint. No six-journey pass or later-mount allowance is claimed.

The first isolated StrictMode-off comparison on tree ca94e6a23ffa45470303c9efa283741864c7ba05
FAILED in `/private/tmp/ovc-warning-lazy-initial-strict-off-1`: exit 1, 25.85 seconds, seven
safe/source-valid samples. Its node_modules was a symlink to another worktree. pnpm's automatic
install explicitly failed because the workspace hoist directory was not a real directory, then
the build continued. It observed eight raw failures, zero expected, nine unexpected (including
scope_boundary_changed), and eight console errors. The fixed server log retains ten errors and
eight warnings but no original messages or denied paths. Cross-root font imports were confirmed
from filesystem realpaths; the existing artifacts do not prove that they caused all eight network
failures or establish their HTTP status. Tracked-source integrity was monitored, but dependency
integrity was not proved: shared executable wrappers had newer timestamps and no pre-run dependency
hash manifest existed. This failed comparison remains failed.

A fresh isolated source-identical comparison was prepared with its own real node_modules.
Offline frozen installation failed with ERR_PNPM_NO_OFFLINE_TARBALL for a pinned package;
the subsequent normal frozen install passed (exit 0), retaining both logs. All 41 source hashes,
package manifest and lockfile remained unchanged. The three known font imports resolved inside
its own node_modules; that directory was real, not a symlink. The owner explicitly approved one
additional corrected comparison while accepting the unresolved exact cause of the earlier resource
failures. That comparison passed in `/private/tmp/ovc-warning-lazy-initial-strict-off-owned-1`:
exit 0, 27.30 seconds, seven safe/source-valid monitor samples, zero raw cancellations, zero expected
and zero unexpected failures. The only source difference from the candidate remained the retained
patch removing the StrictMode wrapper.

The real component producer controls and corrected paired runtime comparison establish the three
INITIAL cancellation lifetimes only. They grant no exception for a channel switch, later mount,
new document, account or scope. Remaining full development journeys, final reviews, exact-tree gate,
merge and full measurement implementation/series are not reported ready or passing by this record.

## Conversation diagnostic implementation

The owner approved the independently security-reviewed conversation lifecycle evidence plan.
Only its first diagnostic phase is implemented: passive destination classification and six fresh
engine/role contexts with preserved assertions, accumulated failures and diagnostic_not_acceptance.
No destination cancellation predicate, fake capability or production behavior was changed.
Previous full-smoke failure remains failed. Actual diagnostic execution remains pending.

Diagnostic targeted checks: initial typecheck/format and all 40 unchanged state contracts passed.
The first ESLint run failed on four assertion closures lacking explicit void returns/braces;
those source forms were corrected without removing assertions. Final static results follow the
retained logs; browser execution remains pending.

Final diagnostic typecheck, ESLint and Prettier passed exit 0. The unchanged initial-read state
contracts passed 40 tests in one file (81ms), not a performance measurement. No browser run,
acceptance pass, destination predicate or producer capability is claimed.

## Conversation diagnostic result and gated navigation implementation

The diagnostic on tree 262bd1d504a54b46ec16312f9fe5516080e285c2 completed in
`/private/tmp/ovc-warning-navigation-diagnostic-1`: exit 1, diagnostic_not_acceptance, 27.94 seconds,
seven safe/source-valid monitor samples. All six fresh engine/role journeys observed exactly one
additional explicit cancellation: destination current_channel ordinal1, account/document matching,
followed by ordinal2 success with complete body and completed destination history. All six recorded
zero console warnings/errors and HTTP failures. Eighteen original assertion failures were retained
(three propagated strict-read assertions per journey). This is endpoint evidence, not acceptance.
No other cancellation family was observed, satisfying the approved implementation gate.

Before adding the predicate, the actual Messages/FollowingQueryClient/Link controls passed:
StrictMode produced two destination lifetimes, firstAborted=true and firstAbortedAtSecondInvocation=true,
secondAborted=false; without StrictMode there was one lifetime and no abort. Two selected tests
passed; 55 were not selected by the name filter. No .skip was added. The first fixture attempt failed
because it set a nonexistent title instead of ChannelDetail.label; the corrected fixture passed.
Full fake transport contracts passed, including armed/unarmed response equivalence and bounded
synchronous signal retention. These results precede predicate implementation, not follow from it.

The exact one-use transition predicate now validates actual source/account/document and quiescence,
requires one observed source-to-destination SPA event, preserves all owner/resource guards and
requires exact-second full response before the original transition deadline. Full UI/history/read
quiescence precedes seal; Settings and unarmed navigation remain strict. Native control source adds
eight named cases while retaining the original eleven. Runtime controls, final six journeys and
source-identical on/off browser proof for this predicate remain pending. No readiness is claimed.

Implementation checks retained a first typecheck failure from a non-erasable constructor parameter
property, replaced by an explicit readonly field. A later lint run failed on void closure syntax and
one impossible tuple guard; corrections retained assertions and transport semantics. Full three-file
contracts then passed 151 tests, with final static checks recorded separately. These durations are
check evidence, not performance measurements.

Final checks passed: typecheck, ESLint and Prettier exit 0; all 151 tests in the three complete
state/fake-transport/real-Messages files passed in 3.94s. Native source explicitly requires exactly
one destination cancellation in its positive boundary and retains unresolved-second evidence after
a real successful third in the failed-second case. Browser/native execution remains pending.

The final destination-only state guard also rejects initial-list/unread target categories at its
boundary. Final typecheck and targeted lint passed again; the complete mandatory three-file run
passed all 151 tests in 4.35s. The native-only assertion additions were type/lint checked and
formatted; no browser/native run was executed by the implementer. Historical plan paragraphs now
link to the current governing refinement while retaining their earlier failed evidence.

## Conversation navigation runtime and independent after-fix review

On executable tree 8b48a296ba056c19001fa29743a31bbae75c48f8, all six default development journeys passed: Chromium, Firefox and WebKit with both roles, zero warnings, console/page errors, HTTP errors or unexpected request failures. The run took 27.50 seconds with seven valid source-unchanged resource observations. The source-identical isolated comparison with only the retained StrictMode wrapper patch and its own real frozen dependencies also passed all six journeys, with zero request cancellations; it took 31.41 seconds with eight valid observations. Component producer evidence and these browser controls support only the named initial and destination-detail lifetimes. They authorize no general navigation or network allowance.

The nineteen native controls run failed overall in 88.10 seconds, with nineteen valid observations: seventeen passed, including all eleven original controls; navigation_repeat and navigation_extra_navigation failed. The independent combined standard/full-security after-fix review found no new blocker or major and identified two minor fail-closed control defects. First, composer/history visibility did not establish complete destination detail settlement before a further native cancellation. Second, a completed navigation invalidates the explicit boundary with navigation_boundary_invalidated, while the control expected spa_scope_changed. The minimal fixes wait for the original exact destination detail/history settlement before either perturbation without sealing or extending its deadline, and require the actual explicit-boundary rejection category. The previous failed command remains failed; checks on the corrected control source are pending.

The review closed the earlier major generic-rejection finding: controls require their intended kinds and isolated ownership cases exclude unresolved-replacement masking. It closed the late-second minor through exact-second full-completion deadline checks and contracts/native evidence. Those fixes concern the verification trust boundary; production authorization is unchanged. The reviewer examined the complete changed security boundary and standard checklist for fixes, including safe diagnostics, opt-in TLS and cleanup, browser-origin/cookie behavior, cache-only observation, request ownership, passive real-signal fixture observation and explicit one-transition state. No further fresh review is required for these two cheap minor control corrections under the repository after-fix rule; all applicable checks must still pass. Full frontend checks, production E2E, exact-tree gate/finish and slice performance/closure remain pending.

The same independent review subsequently clarified a third minor observation-lifecycle gap: the final WebKit off-run snapshot had one pending non-API asset, which alone is not a failure, but final checks preceded context cleanup. Late failures during cleanup could therefore change counters after their last assertion. No lost event was established in that run. The correction waits for finite requests and ownership observations within the existing readiness bound, keeps WebSocket/eventsource transports separately observed, explicitly closes the owned context, drains its pending observations and only then runs final request/HTTP/console assertions and emits the successful journey record. No cancellation allowlist or timeout is widened. The prior successful snapshot remains evidence only for its stated earlier observation boundary; final corrected-source browser checks are pending.

## Verification after all three minor corrections

Executable tree 2c7cf59493d3d767457486157dbac2ead5faf98c passed all nineteen native browser controls, including the corrected repeat and extra-navigation cases, in 88.62 seconds with nineteen valid source-unchanged observations. The final default six development journeys passed with post-context-cleanup assertions in 27.00 seconds with seven observations. The exact source-identical StrictMode-off comparison passed all six journeys in 26.61 seconds with seven observations, zero raw/expected/unexpected cancellations and zero pending API or non-API resources in the final snapshots. The fixed first/second lifetimes, native negative controls and post-cleanup diagnostics are exercised on real browser/API paths. Earlier failed commands remain failures at their stated snapshots.

The complete eight-command frontend change-check run also passed on this exact executable tree: pnpm typecheck, pnpm lint, pnpm format:check, pnpm test, pnpm build, pnpm check:classes, pnpm check:bundle and pnpm api:check, every exit zero. There were 803 tests in 68 files. The command group took 25.40 seconds with seven valid observations. Entry JavaScript plus its largest language chunk is 175.5 KB gzip against the unchanged 180 KB limit; Home is 191.0 KB and Today 191.5 KB. This is bundle evidence, not a full slice performance sample. No final code changes followed these checks; subsequent evidence-only changes require documentation/scanner checks.

All three minor after-fix findings are resolved by the named corrections and passing affected checks under the no-further-review rule for cheap minors. The independent review covered executable tree 8b48a296ba056c19001fa29743a31bbae75c48f8; the coordinator subsequently changed only the identified native-control barriers/categories and observation-completion boundary, then ran the stated checks. This distinguishes the reviewed snapshot, corrected executable snapshot and later evidence-only index. Security fixes affect verification trust boundaries, not production authorization. Production E2E, documentation/scanners, the exact committed-tree merge gate/finish and final slice measurements remain pending until separately recorded.

The first final documentation run failed only Markdown MD012 at newly appended section boundaries. Consecutive blank lines were normalized in the five named changed Markdown files; wording, contracts and executable source were preserved. The failure is retained and the corrected documentation check remains pending.

The retained second documentation run still reported two MD012 boundaries in docs/development.md that were omitted from the first tail-only inspection. All changed Markdown files were then checked for consecutive blank lines and normalized, without wording or executable changes.

## Approved production diagnostic correction

The owner approved the security-reviewed production diagnostic plan and its historical-evidence
uncertainty decision on 2026-11-03. The original full production E2E remains FAILED (933 passed,
38 failed). The single authorized trace-on diagnostic remains diagnostic evidence: 35 failed,
3 passed, 21.37 seconds, six safe/source-valid monitor samples on unchanged tree
869bf13ef0114daa1bef96e5af33e3d0da856c4c. No failure was retrospectively passed.

All 38 complete traces were privately inspected. Thirty-one cases contain intended negative HTTP;
thirty emit native resource console errors. Exact families are refresh401, blocked login403,
login401 twice, login401 five times then429, expired logout401, verify-email422, resend429,
register422, account_me401 plus refresh401 after reset, and password/reset422. Chromium connecting
socket closure and Firefox font interruption correspond to navigation/reload before full readiness.
The Firefox language abort emits a distinct native module-load warning after the original expected
application error. Firefox proactive refresh proves a Window.reportError receiver TypeError at the
actual RealtimeClient server-error-frame reporting call, during twelve-minute runFor, correlated to
the real gateway frame-rate rejection. Unknown historical extra events remain limited as recorded
in the known-limitations file; no unrelated error exemption follows.

Implementation introduces finite request/status/body-owned page phases and removes the blanket
HTTP-console flag. Status-shaped console text alone cannot consume a grant: only one corresponding
completed declared response can. Explicit phase end revokes grants; deferred candidates and request
records are bounded. Legacy diagnosed declarations retain their HTTP/UI assertions under the
stronger collector. Session/signup negatives add explicit action phases; secondary confirmation and
signout pages are watched before navigation and declare their own responses. The existing owned
native404 helper adds a finite real HTTP grant and retires its duplicate raw-console grant while
preserving response/body/exact-error assertions. This is a necessary transitive consumer change.

The reporter is a named receiver-safe production function. Its native canary loads the actual source
modules in owned development TLS mode, uses real session/API/query/realtime/socket implementations,
and sends one invalid frame through its retained real socket. Correct and Firefox original-alias
paths report only fixed booleans/counts; the collector must still reject the intentional page error.
No browser, console, fetch, socket or timer is monkeypatched. A separate owned local module-failure
canary validates the real Firefox warning baseline before duplicate/extra-warning rejection.

The two twelve-minute tests now fastForward after actual socket/authoritative read/font readiness,
then observe the relevant completed refresh response. Original UI/focus/session/storage outcomes,
timeouts, retries and server limits remain. No new dependency, backend route or session policy is
introduced. Documentation wording is pending owner review; runtime acceptance is pending.

Implementation checks retained the first canary typecheck failure: importing full application module
types crossed the existing node-project file boundary. Narrow typed real-constructor projections
replace those imports without changing compiler configuration. Initial lint findings concerned void
closures, optional chaining and callback-flow narrowing; their source corrections preserve behavior.
The bounded state plus existing real-wiring integration command passed 43 tests in two files in
212ms, with matching pre/post source hashes. These are targeted checks, not performance samples.
Native canaries, affected E2E and full acceptance have not yet run on this implementation.

Final targeted typecheck, ESLint and Prettier passed exit 0. The final state/real-wiring integration
run passed all 43 tests in two files (198ms). The retained first 43-test run has identical source
hashes before/after; subsequent source formatting and canary-only typing refinements are covered
by final static checks. Existing realtime integration tests remain unchanged; jsdom does not expose
the native Window reporter, so actual receiver-branding evidence is assigned to the mandatory
Firefox native canary rather than a patched global or a glue-only unit test. All native and
production acceptance results for this source remain pending.

Native canary verdicts are checked after actual context closure and observer.finish. The reporter
requires exactly one intentional page error, one total diagnostic error and zero warnings after
closure; the collector must reject it. Language baseline requires zero unexpected warnings/errors;
duplicate and extra-warning controls require one unexpected warning and no unrelated error after
closure. Light post-close type/lint/format checks passed. No native result is claimed yet.

## Production correction native evidence and independent readiness review

The additional owner-approved combined standard/full-security review inspected all 51 files on tree 28f75c8139ed07e29860132617f6cc769c57eb16, independently matching every file digest and the complete staged patch. It found no blocker or major and one minor: browser/history measurement consumers and development read controls asserted before context cleanup, leaving later diagnostics outside the last assertion. These consumers now close their owned context and await collector.finish during registered cleanup, preserving all earlier read-state and browser assertions. This is a cheap minor correction at the verification lifecycle boundary (T-18/T-19/T-41), with applicable checks required and no additional review round under AGENTS. No diagnostic allowance, deadline or retry changes.

Native controls on the reviewed tree ran in an exact clean isolated copy with all 51 file hashes and equal Git tree, its own offline frozen dependency installation, and no copied environment files, keys or previous TLS output. The collector command passed in 5.778 seconds: Chromium, Firefox and WebKit rejected the original unknown warning/error canaries; Firefox observed an actual owned failed module request and native warning, accepted only the expected baseline, and rejected duplicate and unrelated warning controls. Safe records excluded the synthetic secret.

The real API/WebSocket reporter command passed in 29.917 seconds. All three engines reported the original server-frame Error exactly once through the corrected callback without a receiver TypeError. The Firefox original-alias comparison reproduced the receiver TypeError and failed the reporting contract as intended. The collector still rejected every intentional page error. Both command observations were valid, unchanged-source and safe; these are correctness controls rather than performance samples. Private coordinator evidence is retained in ovc-warning-native-diagnostics-1 and ovc-warning-native-realtime-reporting-1. Targeted production E2E, the single complete source-changed acceptance run and the exact-tree gate remain pending; the historical failed production and diagnostic runs remain failed.

The isolated Firefox callback-alias control also produced its intended exit 1 with exactly one observed receiver TypeError and no original Error report; the collector rejected that page error. Private evidence: ovc-warning-native-realtime-alias-control-1, 23.325 seconds, valid unchanged-source observation. The generic runner records commandExit=1 and passed=false; a separate expected-negative verdict records the intended oracle outcome. This is an expected negative control, never a successful acceptance run.

## Failed held-navigation HTTP control and declaration ordering correction

The first post-review nineteen-control run on tree cb9e164f9e8dff275d8ef7517711b1f9d812bc26 failed, exit 1, after 88.820 seconds with nineteen safe observation samples and unchanged source. Eighteen controls completed their intended outcome; navigation_failed_second failed. Its real held destination GET returned a complete 404 and one exact native console error. Source ordering establishes the defect: expectResource404 registered the finite HTTP declaration only after navigation had already started and held that request, so the collector had no declared request identity and correctly rejected its response. This violated the approved before-action rule; it was not a reason to widen matching or accept undeclared responses.

The control now declares the exact GET/status before actual destination navigation. The two direct-fetch 404 controls declare before invoking their own request. The helper observes and verifies the previously declared response; it asserts collector errors/warnings after closing the HTTP phase. Its exported expectedErrorConsumed boolean now requires actual successful finite-phase verification, rather than merely an empty legacy exact-console list. Original response, count, body-completion, console, read-state and cleanup assertions remain. Private failed-run evidence: ovc-warning-production-native-read-controls-1. Changed-source rechecks remain pending; the failure is preserved.

### Targeted production harness correction

The first targeted four-file production attempt remains FAILED: 144 passed and 12 failed in
64.401 seconds, with unchanged source and valid monitors. Harness corrections consume the actual
API-sign-in response body, explicitly end its negative phases, and give each wrong deletion-password
attempt its own finite phase without changing matcher rules. The known unverified-tutor Today set
excludes only the deliberately disabled join-requests query; active-account requirements remain.
Readiness observations begin after API sign-in and before the actual target-document navigation.
Token-page reload now waits for the real declared verify-email422, invalid-link UI and phase/body
completion before the original reload assertions. The historical WebKit page-error producer remains
UNPROVEN because that artifact has no browser stream; this adds no error allowance or runtime pass.
Production Session response draining and any new acceptance attempt require their separate decision.

Source recheck confirms Gateway.recheck marks ready sockets suspended and starts recheckAccount immediately; the account contract requires revocation within five seconds while the listener lives. The affected other-browser tests now establish actual ready/refetch first, observe real sign-out and complete the expected 401 phase before the original hard reload. This avoids using navigation to terminate that response. The blocked-account test likewise separates the revoked refresh phase from the later blocked-login phase. No ping interval or deadline was changed and no unproved 25-second wait was introduced. The source contract, rather than a new delivery promise, justifies these barriers.

### Refresh refusal lifecycle implementation

The owner approved the independently reviewed refresh-body plan on 2026-11-03. The correction
ends the old session immediately on 401/204 headers, drains its same response with bounded streaming
state (64 KiB, unchanged 15-second signal), releases the reader and propagates fixed body failures.
The Web Lock callback checks epoch before cookie-authenticated send. Stale refusal bodies drain
without ending another owner; stale200 and startup error-to-state behavior remain unchanged.

The explicitly selected SignalAwareRefreshBodies fake mode supplies real signal-aware post-header
streams with headers/read/body barriers, completion/failure/cancel/abort and reader-lock evidence.
The shared transport contract adds post-header abort, exercised by both selected fake and production
adapter. Actual Session controls cover complete/failing/oversized bodies, exact64KiB, empty204,
queued callback plus adopt and no old-request replay. Real Gate/router plus FollowingQueryClient
uses the extracted production accountCacheOwnership binding to assert actual private-data release
and replacement ownership; main invokes the binding in its unchanged callback order, with preference
and lazy-realtime handling retained. The existing startup wrapper is separately covered by that
composition, preserving new-owner state after an old body failure.

The opt-in check-refresh-response-lifetimes command exercises exact-limit, overflow and actual
15-second signal abort without increasing ordinary test timeouts. Native API/browser body completion,
new targeted acceptance and final reviews remain pending. Earlier 144/12 and all historical failures
remain failures; no garbage-collection cause is claimed. Initial light typechecks passed; the first
lint found readonly-signal flow narrowing and a constant loop condition, corrected with an explicit
abort-state read and completion predicate. No runtime checks have run on this implementation yet.

The first targeted session/transport/Gate/query-client run failed four cases (76 passed, 10.18s),
with identical before/after source hashes. Two Gate assertions exposed an unwired fake read-failure
counter; two protected-request controls had revoked the token too early, yielding unauthenticated
without the token_expired refresh path and timing out at the unchanged five-second case limit.
The corrected causal setup uses the existing fake account-read barrier to capture expired401, then
revokes the cookie session before refresh. The second run failed two assertions (78 passed, 590ms):
the replay counter had included adopt's legitimate new-owner account read. Separate pre-adopt and
post-adopt/body-completion marks now require one original read and zero replay, rather than relaxing
an expected count. The corrected four complete files passed all 80 cases in 581ms with unchanged
source hashes. These are fixture/evidence corrections; production drain/epoch behavior was not
weakened, and all failed attempts remain retained. Final fixture cleanup also releases owned body
barriers and disposes subscriptions/cache/router on assertion failure.

Final resource-cleanup controls passed all 80 tests in the four complete selected files in 744ms,
with identical before/after source hashes. The pre-send epoch check is located only in the acquired
Web Lock callback, so the queued-request causal assertions exercise that guard without a redundant
private-entry check masking its removal. Final type/lint/format checks pass; actual fifteen-second
control, production-adapter shared contract, browser/native body evidence and fresh combined review
remain pending. No timeout/configuration/dependency changes or commits were made.

## Refresh response lifecycle verification

The frozen 61-file tree 6aff70724cfd5e71b3faa58b1667fe0f3f31cc9f passed the explicit
check-refresh-response-lifetimes command: exact 65536-byte completion, cancellation at 65537 bytes,
and native AbortSignal timeout after 15000 milliseconds with the body still held. Every case
finished with zero active bodies and an unlocked reader. The observed command exited 0 after
15.262 seconds; five monitoring samples were safe and before/after source hashes were identical.
This exercises the configured actual signal; the script reports elapsed time rather than adding
a scheduling-tolerance assertion to the normal five-second Vitest selection.

All eight frontend checks then exited 0 on that same frozen tree: typecheck, lint, format, tests,
build, classes, bundle and generated API comparison. The 69 test files passed 841 cases. Entry
and largest language chunk totalled 175.8 KB gzip against the existing 180 KB limit. The observed
sequence took 23.858 seconds, with six safe monitoring samples and unchanged source.

The first targeted command did not start any test: it invoked the worktree-local wrapper, which
had no sanctioned storage settings. Global setup exited with the named missing-storage-settings
error in 0.291 seconds. That failed launch is retained; it is neither an assertion failure nor an
acceptance result. The corrected command uses the primary repository wrapper without reading or
copying environment files. The five selected files (session, signup, account-edge-cases, language
and session-transport) then passed all 180 cases in 32.5 seconds, command exit 0 after 32.811 seconds.
The browserless transport contract runs once in Chromium's project selection; affected browser
journeys run in all three engines. Eight monitoring samples were safe, source hashes were unchanged,
and all native trace archives remain private. The named historical WebKit producer remains
unproven; its event did not recur in this bounded source-changed attempt.

The additional independent combined standard/full-security review verified all 61 source hashes,
the complete indexed patch and tree 6aff70724cfd5e71b3faa58b1667fe0f3f31cc9f. It examined refresh,
transport/body ownership, epoch/lock serialization, account cache/main/Gate composition, finite
HTTP/native diagnostics and development confinement. No blocker or major was found. Its minor
corrects the development guide's obsolete eleven-control count to nineteen and describes finite
HTTP declarations before the action, with response facts verified afterward. These wording fixes
change no executable predicate. This is source-review evidence; the full production acceptance,
remaining checks and exact committed-tree gate are still pending.

The first final documentation check failed Markdown style rules only: repeated blank lines and an
ordered-list restart after an intervening paragraph. Tool-binary verification passed; later docs
checks and staged Gitleaks did not run in that fail-fast command. The formatting-only correction
removes the repeated blanks and restarts the list at one without changing requirements.

## Full production acceptance and finite-phase corrections

The complete production acceptance on tree b8156472228fbe72af3580b5bdd6e2735fb1c9f4 failed:
969 passed and three failed, command exit 1 after 157.502 seconds. All 33 observation samples were
safe and source hashes were unchanged. The failed run and all three trace archives are retained
privately; there is no infrastructure or sleep explanation and no retrospective acceptance.

Two failures are the same schedule resubmit scenario in Chromium and WebKit. Its two declarations
for identical POST lessons 409 responses share a phase, and both exact native console events remain
ambiguous under the required unique-attribution rule. Their archives contain test.trace only; this
source-level ambiguity is definite, while no absent browser stream is invented. The correction ends
the first finite phase after its existing overlap alert, declares the second response before the
unchanged keyboard action and ends that phase after its original focus assertion. It retains both
responses and all original pending, focus and warning assertions; the matcher is unchanged.

The signup resend case fails the request-lifecycle fixture in Chromium. Its retained browser stream
shows GET channels/unread with native ERR_ABORTED during the initial connection/navigation interval;
POST verify-email/resend completes with the declared 429 and its UI assertions pass. Source shows
separate tutor/public Gate mounts own the unread query, while initial realtime connected handling
invalidates active queries. The test navigates from Today before observing readiness. The correction
uses the existing unverified-tutor Today query set and existing socket/query/font barrier before the
original verification-link navigation. This prevents the test from intentionally racing that startup
wave; it adds no cancellation allowance or general aborted-request exemption. Exact historical
scheduling causality is an inference from that trace and source, not a new browser-private export.

Both corrections implement the already approved finite phase and pre-navigation readiness rules.
No production code, matcher, original assertion, timeout, retry or budget changes. Changed-source
targeted verification and any further acceptance result remain pending; the failed full run remains
a failure. No unchanged repeat is authorized or started.

On changed tree 5f37d4cb463544f6d51d88ff2795c25a2c9d1a58, the two corrected scenarios passed
all six cases in Chromium, Firefox and WebKit, with trace enabled. The observed command exited 0
after 6.021 seconds with three safe monitoring samples and identical source hashes; the six traces
remain private. This bounded check is not the previously failed full acceptance. A separate owner
decision is pending for one further full changed-source E2E and explicit acceptance of the stated
historical signup scheduling uncertainty. No further full acceptance or merge has started.

The after-fix sequence passed all eight frontend commands again, including 841 tests. Its later
documentation check failed two repeated blank lines introduced while appending this evidence;
staged Gitleaks was not reached. Only Markdown spacing is corrected afterward; executable hashes
remain those of the six-case passed source. The failed command remains retained.

The project owner approved one further full E2E on the corrected source on 2026-11-03 and explicitly
accepted the remaining historical signup scheduling uncertainty. The prior 969/972 result remains
a failure. The corrected six-case browser set and mandatory frontend/documentation checks have
passed as recorded; new unexplained failures block merge and no automatic unchanged retry is
granted. Existing assertions, operation timeouts, retries=0 and the 260-second gate ceiling remain.

The explicitly approved further full E2E passed all 972 cases on tree
c3fd2aa8f29fe7b8ff3f62d3fdb40ee78ce33cbd. Documentation and staged Gitleaks also passed before
that E2E in the same observed invocation. Command exit 0 after 158.621 seconds; all 33 monitoring
samples were safe, source hashes were unchanged and no automatic retry ran. The previous 969/972
result and all other failures remain retained. This proves the corrected full browser suite on the
stated tree, not the cause of every historical event and not the still-pending merge gate. Only
English evidence and budget records are added after that passing executable state.

## First exact-tree gate and document ownership correction

The first gate of committed candidate 9adddce90904e8517a02a11fa2c8732a14befba4, tree
eba49f406d1cce18aa69b755796380e616bcd926, failed one Chromium pilot signup readiness assertion.
All 29 other commands passed; E2E passed 971 and failed one. Gate wall time was 252.1 seconds,
observed command time 252.905 seconds, summed command seconds 366.1. Lane wall times were pre 0.2,
A 97.7, B 56.2, C 58.7 and E2E 153.3 seconds on 12 cores with existing caches. All 52 monitoring
samples were safe and the committed source stayed unchanged. This was below 260 seconds but failed
its assertion, so it did not authorize merge or an unchanged repeat. Main remains 4d9ba1e. The
complete gate report, command logs and failed native trace remain private and retained.

Independent trace analysis confirms four requests from an intermediate document without terminal
events in the readiness observer. The actual second Today document's post-ready requests start
around 29996 ms and finish around 30010–30013 ms. The page-wide pending Set retained the old four
requests, causing the exact five-second assertion failure. No API timeout, sleep or unknown browser
error is claimed as its cause.

The independently security-reviewed document-ownership plan was approved by the project owner's
instruction to continue on 2026-11-03. Candidate 9adddce and its branch remain unchanged. A new branch
from main squashes that candidate without rewriting history and adds the approved observer fix.
The implementation uses a committed main-navigation generation and request/socket ownership.
Same-document navigation is checked with an owned public DOM handle, without a window export or
private application data. Required post-ready completion, strict diagnostics and deadlines remain.
Fifteen direct state contracts pass; initial targeted lint identified asynchronous narrowing of
mutable document state and was corrected with an explicit current-document predicate. Typecheck
and the subsequent targeted lint passed. Browser controls, fresh combined review, full E2E and the
new exact committed-tree gate remain pending.

The first native navigation control on frozen tree 1856afb51b06ce0eb340560db54081fc8d3ca890
failed before its acceptance stages: the old intercepted request was continued only after its
document had been destroyed. Playwright rejected route.continue with the already-handled category,
and a separate handler Promise escaped as an unhandled rejection. Observed exit 1 after 4.770 seconds,
two safe samples, unchanged source. No native acceptance is inferred. Subsequent process inspection
found no live API/worker/preview process belonging to this control; graceful stop outcomes were not
recorded, so successful fixture teardown is not claimed. The failed evidence is retained.

Independent combined review classified this control lifecycle defect as major, T-20/T-41. Its
explicit document-scope source review found no additional blocker/major in the observer itself;
parts of the broader governing/evidence reads were truncated, so it did not grant a complete
passed verdict over all 66 files. The severity is preserved. The fix touches the verification and
resource-lifecycle boundary. A further owner-authorized complete security-profile/standard review
is required after the fixed controls/checks; no merge or full E2E proceeds without it.

The corrected control holds the next real main-navigation request before commit. It observes old
work at navigation start, checks the new navigating generation and zero current pending work,
continues and joins the old GET unchanged while its document remains live, then continues and
joins the navigation and verifies current ready/queries. Evidence states release before commit,
not an impossible continued hold after document destruction. The historical missing-terminal
sequence remains supported by the original trace and direct state contracts, not a claimed forced
browser omission. Every route operation and goto rejection is observed immediately; the owned join
propagates fixed aggregate failures rather than leaving callback rejections unhandled. Safe fixed
engine/control/stage diagnostics identify failures without exporting raw exception content. Initial
lint found unsafe-finally masking in the control cleanup; it was corrected and targeted lint,
types and formatting passed. The changed real-path control remains pending execution.

The changed control on tree 31c54b0c5c824c9a0e207a2f764698f108abcd4e failed after the main-navigation
release stage: exit 1 after 22.782 seconds, six safe monitoring samples and unchanged source. It
reached old-request release before commit without an unhandled rejection. The fixed exports did
not retain the underlying error class, so that stage alone is not a unique producer attribution.
No native acceptance, extra full E2E or gate was inferred from this run.

Read-only inspection of the pinned Playwright 1.63.0 implementation identifies a definite second
cleanup race: Page unroute removes the last request interceptor without waiting for active user
callbacks; removal notifies in-flight routes and continues the current one. The control removed
that handler before joining its own continue. The correction releases and joins all owned route
operations first, removes the control-owned handlers with unrouteAll behavior wait, then joins any
late observed operations and propagates every failure. It uses no ignoreErrors, changed request,
fabricated response or continuation-error exemption. The two failed controls remain retained, and
the exact historical producer of the second failure is not claimed solely from its last stage.

The original independent combined review subsequently completed its missing governing reads on
immutable tree 1856, including the whole threat model and applicable normative sections. Its major
verdict is unchanged and no further findings were added. This completion did not review either
fix. The additional owner decision for a fresh after-fix reviewer is still required before merge.

The next source-changed native command on tree c0fca613cf28bbbae3a60fc73459c348fb8a51a4
completed Chromium's hard-document, SPA/current-held deadline, route-cleanup and three disposal
checks. Firefox then stopped making progress after begin-main-navigation and a held-operation
failure of unknown fixed class. The coordinator requested SIGTERM only for the process tree
proven by this command's own run-lock and PPIDs. All known owned descendants and the coordinator
were confirmed absent afterward; the unrelated old worker was not signalled. Node ended with
exit 13 and an unsettled-top-level-await warning after 386.462 seconds. All 78 observations were
safe and source unchanged. This is a failed control, not a three-engine pass; successful ordinary
fixture teardown is not claimed.

Source inspection identifies a callback-join defect: a custom terminal-request throw before any
Route operation leaves the SDK's handling Promise unresolved even though the wrapper observes
its own rejection. The correction finalizes a natively ended old request through unchanged
fallback, then requires the exact old request's native cancellation to satisfy the existing
request-failures navigation classifier using actual Page event order. Only the explicitly retiring
old request can take that path; unexpected current terminal requests and every thrown continuation
remain failures. Other failed requests also enter the unchanged classifier. The control distinguishes
old work held when goto starts from old work still held at the observed main request, and records
actual native retirement without claiming successful HTTP completion. A bounded activity record
rejects overflow and removes its handlers. This changes no production or shared diagnostic allowance.
The exact original unknown error class is not retroactively inferred from its last stage.

The next native control on tree 8968e032b5ae10738336c1db4a2e12c5c77ae823 completed all 18
body controls across Chromium, Firefox and WebKit, then failed during finalization with exit 1
after 51.010 seconds. All 12 monitoring samples were safe and source was unchanged. Each engine
proved hard-document readiness, SPA preservation with the original held-current deadline, joined
route cleanup and three observer disposal cycles. Firefox and WebKit reported actual old-request
native cancellation; Chromium did not. These differences are observations, not waived failures.

The final aggregate retained an unknown leaf because the control replaced cleanup diagnostics with
a fixed error but did not classify that fixed error or export the collector's safe records. The
exact final producer is unverified; 18 completed bodies do not constitute a passed control. The
retained output and failure remain immutable. Pending diagnostic instrumentation emits only fixed
category/route/count fields and lifecycle completion stages, preserving every assertion and error.
A further diagnostic execution requires an explicit owner decision; it cannot count as acceptance.

A 20-second trailing interval does not prove a forced process stop: deployment's existing
Promise.race timeout retains a Node timer even when the child exit wins. That observation does not
identify the actual final failure, and the unrelated timer implementation is unchanged.

The owner-authorized finalization diagnostic ran once on the 66-file manifest
`ovc-navigation-readiness-finalization-diagnostics-manifest.json`. It failed with exit 1 after
44.147 seconds; all ten monitoring samples were safe and source unchanged. All 18 body controls
completed. Browser close and deployment stop completed in every engine. WebKit's collector
reported exactly two page_error records, route unknown; the terminal fixed error was diagnostics,
not process stop. This is diagnostic evidence only, not acceptance.

The collector has two producers for page_error: a real browser weberror and its own failed
request.frame lookup. The existing export does not distinguish them, so JavaScript failure and
collector attribution remain unresolved. The pinned Playwright request.frame implementation
explicitly throws for a navigation request before its frame exists and for service-worker requests.
This is a candidate explanation, not a proven attribution of either record.

Prepared control-local provenance observation distinguishes native weberror from request-frame
lookup failure, with fixed lifecycle stage, allowlisted error classes and fixed error categories.
It exports no error message, stack, URL, body, identity or credential. Shared collectors, assertions,
timeouts and all refusal rules remain unchanged. A further single diagnostic execution needs an
owner decision; no automatic repeat, acceptance, suppression or merge follows from the prior run.

The owner-authorized provenance diagnostic ran once on the 66-file
`ovc-navigation-readiness-provenance-diagnostics-manifest.json`. Exit 1 after 44.506 seconds, ten
safe monitoring samples and unchanged source. All 18 body controls and all browser/deployment
stops completed. Both WebKit failures are actual native weberror events with a Page, emitted
during begin-main-navigation; no request-frame lookup failure was observed. The native error name
is outside the initial five-class allowlist and its message does not match the initial three
fixed categories. This proves the producer, not its exact cause. The result remains diagnostic
only and failed; no repeated run or merge is authorized by it.

Pinned Playwright's WebKit adapter turns JavaScript-source console errors into weberror and splits
the text at its first colon to obtain Error.name. Consequently an unrecognized name alone does
not establish the JavaScript exception class or a production bug. Further investigation must use
public source locations and bounded fixed classifications, preserving strict failure handling.

Read-only independent source investigation confirmed the WebKit serialization boundary and did
not establish a unique cause. Prepared diagnostic mode --diagnostic-webkit keeps the complete
WebKit body assertions and cleanup but makes no three-engine acceptance claim. The local observer
exports up to 128 fixed-category events, rejecting overflow; allowlisted name families, independent
cancellation/socket/network/module flags, exact-own-socket-address equality as a boolean, and
location/stack coordinates only for exact own-origin JavaScript assets enumerated from the build.
Asset indexes refer to the sorted local build list. No raw message, stack, URL or browser payload
is exported. Ordered navigation, notifications failure and socket lifecycle observations supplement
the prior attribution. This is proposed diagnostic execution, pending owner authorization; all
shared diagnostic matchers and acceptance rules remain unchanged.

The authorized WebKit-only location diagnostic failed after 29.856 seconds, exit 1, seven safe
monitoring samples and unchanged source. All six WebKit body controls and browser/deployment
stops completed. Three native weberror events were recorded at begin-main-navigation. Exact
own-build coordinates identify Session.authorizedRequest sending HTTP and the second
super.refetchQueries in FollowingQueryClient, with join-requests, tutor-code and health-readiness
consumers. Native text classification and exact departure/dispatch ordering remain unverified.

Read-only source investigation found that the second refetch wave has no lifetime guard after
awaiting the first, while realtime's existing suspension does not control that pending generic
continuation. A proposed document-refetch-lifetime plan first requires an old-code real-path
negative control to establish this independent defect, rather than claiming the original WebKit
message is already explained. Security planning review closed a major design gap by requiring
a single synchronous lifecycle owner before lazy realtime installation. The final proposed plan
has no blocking security planning finding; it still requires owner approval before implementation.
No further native run, production change, acceptance or merge followed from this diagnostic.

The owner approved document-refetch-lifetime implementation, limited historical uncertainty and
one additional combined readiness reviewer on 2026-11-03. The unchanged-production real-path
regression failed exactly at its no-second-request assertion (two requests instead of one). The
first response succeeded, the original Query remained mounted and departure closed the socket.
After the generation guard and synchronous shared lifecycle owner, 28 targeted tests passed,
including that assertion, resumed/born-suspended ownership, delayed realtime installation, BFCache,
account replacement, both rejection policies and disposal. Full project TypeScript build passed.
Earlier root-tsconfig --noEmit invocations contain no build of referenced projects and are not
claimed as complete TypeScript evidence.

The first isolated negative-control preparation failed before mutation because its source matcher
expected multiline formatting. Its subsequent command ran the unmodified copied source and passed;
that result is invalid negative-control evidence. pnpm also refused auto-install into its linked
node_modules directory. Both outputs are retained. After an exact single-match mutation check,
the same Vitest CLI ran directly on the isolated copy: removal of only the lifetime eligibility
guard restored the exact two-versus-one request-count failure. No production source was altered
by that control, and the negative result is expected evidence rather than a waived regression.

The corrected lifetime tree passed all eight frontend commands (865 tests in 70 files), observed
25.252 seconds, seven safe samples and unchanged source. Entry plus largest language is 176.4 KB
gzip against hard limit 180 KB; the 176-KB owner consultation is pending. Its strict native
three-engine control passed all 18 bodies, diagnostics and teardown in 43.620 seconds, ten safe
samples and unchanged source. The independent combined review closed the prior major control
lifecycle finding and found no production lifetime issue, but returned a new major T-41: REST
readiness matched pathname without origin/frame/method ownership. A minor T-18/T-19 reported raw
parameterized paths in failure output. Severity is retained and merge is blocked pending fixes,
checks and an owner decision authorizing another fresh security-profile reviewer.

The readiness fix accepts only HTTP(S) requests belonging to the current main frame and its
committed origin, excludes service-worker traffic, and only GET can satisfy required reads. Other
owned API methods remain pending work until completion. Unknown frame ownership fails closed.
Internal exact paths are retained for matching, but exported pending/missing states are fixed
categories with no path identifiers. Twenty-two state/ownership contracts and full TypeScript
build passed; new full frontend/native checks are pending.

The reporter command's initial 0.602-second preflight refusal is retained. Re-executing the pure
preparation guard confirmed development_fixture_refuses_existing_source_tls_material against the
preview fixture's existing directory; no key was read, copied or removed. A separate source-only
copy omitted runtime TLS artifacts and received its own frozen dependency install. Offline install
failed on a missing cached tarball; the ordinary frozen install succeeded without lockfile changes.
The real reporter control on that copy passed in 30.568 seconds, eight safe samples. Chromium,
Firefox and WebKit each reported exactly the original error; the original Firefox callback alias
failed the reporting contract with a receiver TypeError as the intended negative control. All 1013
copied source hashes were checked after the run with no differences; the origin tree also stayed
unchanged. This does not erase the earlier preflight refusal.

The origin/frame/method ownership fix passed all eight frontend commands with 872 tests in 70
files, observed 23.355 seconds, six safe samples and unchanged source. The strict three-engine
native controls passed all 18 bodies and cleanup in 43.805 seconds, ten safe samples. Tool binary
probes, documentation checks and staged secret scanning completed; the scanner found no leaks.
These results do not resolve the subsequent targeted browser failure.

The targeted signup/schedule run on staged tree 7f7a3c14820ebbe058cfe8330135ca1cbb6e23a3 failed:
135 passed and eight failed, exit 1 in 32.830 seconds, eight safe samples and unchanged source.
Chromium's account-confirmation navigation did not satisfy readiness; six Firefox scenarios
reported warnings; WebKit's password-reset HTTP phase expected two refusals but observed one.
All eight retained traces were preserved locally before another harness run. No full E2E, gate,
commit or merge followed. The further fresh readiness-review decision is still outstanding.

Read-only trace/source analysis found two Firefox warnings around injected document evaluation
and stylesheet loading; the other four Firefox artifacts lack browser warning events. This does
not establish their cause or justify an exemption. Chromium's recorded socket handshake and
successful account/page reads do not establish the observer's ready-frame state. The WebKit
trace contains an account-read 401 but no refresh 401; its response body is unavailable, and
Session only refreshes a refused authorized request for token_expired while its epoch remains
current. The exact missing-request cause is unverified. A separate bounded eight-case diagnostic
plan is proposed; no additional browser execution is authorized by these observations.

On 2026-11-04 the owner accepted 176.4 KB gzip for the initial load, preserving the hard 180-KB
limit, and approved the bounded eight-case diagnostic plan and one diagnostic execution. The
previous 135/143 failure remains. This approval does not authorize acceptance, merge or a repeat.

The approved diagnostic implementation passed TypeScript, ESLint, formatting and 49 focused
state tests. Initial lint rejected bracket notation for the opt-in variable; dot notation fixed
it before the passing checks. List-only preflight initially selected five tests because the
installed test-list parser splits embedded title separators, then a temporary module setup
failed before test discovery. The final local config preserved the original browser/setup/
teardown settings and applied project-specific exact title filters together with test-list
file constraints. List output was compared to all eight original project/file/line/title
identities and matched exactly before execution. These preparation failures ran no browsers.

The single approved eight-case diagnostic ran on 2026-11-04 Moscow time, exit 1 after 9.832
seconds: all eight failed. Three monitoring samples were safe and all source hashes remained
unchanged. Raw traces and full reporter output were routed from process creation to private
local storage outside the checkout; the recorded raw deletion deadline is 2026-11-11 Moscow.
No further execution, acceptance or merge followed. Initial warning classification returned
other because Firefox wraps the warning text; offline inspection establishes that all eight
warning events in the six Firefox cases contain the forced-layout/stylesheet warning and
originate in debugger evaluation. This does not yet establish the causal operation.

Chromium's second-account readiness observer recorded request_failed before opening its new
socket. The matching own POST logout request has HTTP 204 headers followed by ERR_ABORTED;
this is not an unanswered logout. The observer remained failed despite later successful
navigation. The exact browser cause of that terminal event remains unverified. WebKit's
phase recorded two declarations, one matched and completed refusal, zero phase faults. The
retained account-read 401 body contains unauthenticated, not token_expired. The current Session
only attempts refresh for token_expired on the same epoch; this explains why a mandatory
refresh refusal is not established by that account-read refusal. Fix proposals must preserve
strict unknown-error rejection and require separate scope approval where acceptance rules change.

The owner approved the targeted follow-up plan, limited historical uncertainty and one additional
combined readiness reviewer on 2026-11-04. The implementation wraps the actual Document reference,
starts new-account readiness only after the paired logout 204 and exact confirmation start, and
holds that real request to prove readiness cannot pass before release. The reset scenario now
selects an unexpired revoked-session branch and requires unauthenticated with no refresh.

TypeScript and lint initially found local implementation errors (unsupported Promise.withResolvers
and cleanup typing); these were fixed before the passing checks. Sixty-six targeted tests passed.
The full component run initially failed six run-lock cases because sandbox denied ps with EPERM;
the same command with the required process-inspection permission passed all 883 tests in 70 files.
The sandbox failure is retained, not counted as a pass. Typecheck, lint, format, build, classes,
bundle and API checks passed; initial bundle remains 176.4 KB. Strict three-engine native controls
passed all 18 bodies and cleanup in 43.756 seconds, ten safe samples, unchanged source.

The single authorized corrected eight-case browser run failed seven and passed one, exit 1 after
12.810 seconds, four safe samples, unchanged source. Chromium passed the account-switch scenario
including the real confirmation hold. All six Firefox cases retained warnings, so a plain Document
wrapper did not resolve their producer. No warning exemption or repeat was applied. WebKit failed
at the newly added named-account menu wait before reaching reset: this was an implementation
mistake in the test because PublicFrame exposes the signed-in logout button, not the private-frame
account menu. That one-line locator was corrected to the actual signed-in indicator, preserving
the approved initial-account completion condition; it has not been browser-reverified. The prior
run remains failed. Additional readiness review, full E2E and merge gate remain blocked.

Read-only inspection after the failed follow-up identifies a stronger Firefox hypothesis:
pinned Playwright's frameSnapshotStreamer performs synchronous scrollTop/scrollLeft reads at
coordinates matching the injected-evaluator warning, around early document-handle capture.
The Document wrapper alone was insufficient. Clock/selector source coordinates do not match
that read. Causality remains inferred until a controlled lifecycle correction passes. Disabling
DOM snapshots would also remove trace network evidence, so no tracing reduction is proposed.
A separate parsed-document capture plan preserves enrollment from commit and gates capture on
DOMContentLoaded with explicit generation/disposal requirements. It is not implemented or approved.

The owner approved parsed-document capture on 2026-11-04. TypeScript, lint, formatting and 31
state checks passed. The first corrected native series failed after 48.070 seconds, exit 1,
11 safe monitoring samples, unchanged source. Chromium passed its nine bodies. Firefox passed
six previous bodies and released/disposed parsing controls, then failed supersession with an
assertion and one warning. All processes completed cleanup. WebKit did not start. Exact failed
assertion and warning cause remain unverified; no seven-case E2E or repeat followed.

The additionally authorized fresh combined standard/full-security review returned FAILED. Major
T-41: entry-module controls are not mutation-sensitive proof of capture waiting/generation, and
new branches omit nativeActivity.verify, so some failed resources could escape the evidence.
Major T-20/T-41: separate ready and settled polls can pass across different documents; capture
and socket readiness are absent from the second predicate. Major T-21/T-29/T-41: page/address
WebSocket attribution does not establish frame ownership; current frame-src none limits the
scenario but does not make the verifier's attribution claim true. Minor T-41: account-switch
route setup/removal failure can skip owned cleanup, and one completion promise does not join
duplicate handlers. Severities are retained. The review found no new content/token/URL export
in the examined lifecycle/session/diagnostic integration, but did not establish full earlier
tooling/runtime coverage. Main remains unchanged; readiness and merge remain blocked.

Pinned Playwright's public lifecycle waiter can use a cached load state or a later frame event;
it does not itself carry document identity. Initial capture immediately before goto can enqueue
evaluation before the navigation request updates the local generation. Publication guards and
execution ordering are different guarantees. A proposed proof-consolidation plan addresses the
review findings, explicit controlled navigation ordering and real mutation-sensitive evidence;
it is not implemented and awaits planning review and owner approval. Another fresh readiness
reviewer after fixing these majors requires an explicit owner decision.

The owner approved readiness-proof consolidation on 2026-11-04. Implementation adds coherent
sampling, actual waiter/handle/publication evidence, initial capture barriers at callers, explicit
hard-navigation retirement, single-frame refusal and tracked route cleanup. Static checks and
33 state tests passed before the single corrected native series. That series failed after
39.049 seconds, exit 1, nine safe samples, unchanged source. Chromium passed prior controls plus
held/released and disposed resource checks; supersession reached a new capture with zero waiters,
one handle and one observed parsing event, then failed. There was no recorded console/HTTP
failure. Exact failing assertion is not recoverable from the safe failure-kind record. Other
engines, browser negative controls and the seven-case E2E did not run.

Read-only source diagnosis established an independent error in the new control: it waited for
old-request terminal before releasing the route hold and unconditionally required nativeRetired,
which only describes a request already terminal before forwarding. This is stronger than the
approved conditional native-cancellation proof and can wait on its own barrier. The corrected
control first releases unchanged, then uses separately retained listeners to await terminal for
that exact Request. A failed old request must have no response, an allowlisted cancellation and
start < explicit navigation action < failure; a finished old request is valid. Other failures
still pass through nativeActivity.verify. The old log does not prove this was its exact failing
assertion. No second native execution occurred after this correction.

The isolated coherent-readiness negative control removed only the combined captured/socket
condition. Its named test failed at expect(state.coherent(false)).toBe(false), observing true,
which is the intended sensitivity result rather than a preparation failure. Four browser mutation
copies were prepared but remain unexecuted. The additional fresh readiness reviewer authorized
for this consolidation has not run. Seven-case/full E2E and merge gate remain blocked.

The one additional terminal-order native execution was approved and ran once on 2026-11-04.
It failed after 38.990 seconds, exit 1, nine safe samples, unchanged source. Chromium again reached
new_capture with zero waits, one handle and one parsing event, then failed without a recorded
console/HTTP finding. Exact failing location remains unverified. No browser mutation, seven-case
E2E, full E2E or gate followed.

Inspection found that the supersession control itself started goto before joining prepareNavigation,
contrary to the approved controlled-navigation contract. The revised control invalidates the owner,
releases the real entry module while the old document remains alive, requires that exact request
to finish, joins retirement, then asserts no old publication and zero waiters/handles before goto.
The existing hard-navigation control retains cancellation proof, now requiring an unanswered,
allowlisted cancellation after the explicit navigation action for the exact old request when its
native cancellation is observed. This separates a stale-capture guard proof from abandoned-document
request terminal behavior; it does not waive failed resources or establish the previous assertion's
cause. It has not been browser-executed.

Failure records now include only bounded numeric line/column coordinates from this exact native
control's real stack frames. Messages, stack text, URLs and other source paths remain unexported.
This closes the missing-location evidence gap without another broad diagnostic mode. A new native
execution still requires owner authorization because the previous single additional allowance was
consumed. All browser mutation copies must be refreshed before use on the corrected source.

The owner-approved ordered-supersession native execution ran once on 2026-11-04. It failed
after 60.837 seconds, exit 1, with 14 safe monitoring samples and unchanged source. Chromium
and Firefox completed hard-navigation, SPA, three observer-disposal, three parsed-document
and child-frame-refusal controls. Both engines proved zero old waits/handles before the new
navigation and successful new-owner capture afterward. WebKit completed hard-navigation, SPA
and three observer-disposal controls, then failed at check-navigation-readiness.ts:730:56: the
entry-module hold had not observed its request within the existing polling deadline. It did
not reach the first held-capture assertion. All three deployment-stop stages completed; no
browser diagnostic finding was recorded. The failure location is established, but its cause
is not: browser resource reuse and interception behavior remain hypotheses. No repeated run,
browser mutation, seven-case execution, full E2E, gate or merge followed. The private evidence
is /private/tmp/ovc-readiness-ordered-native-1; the source manifest is
/private/tmp/ovc-readiness-ordered-manifest.json. Earlier failures remain retained.

The owner approved bounded entry-module observation and one diagnostic WebKit native series
on 2026-11-04. The implementation records separate page/context request and actual route
entry/selection counters, exact-identity mismatches and unavailable frame attribution, commit
and parsing counts, and capture resource state before cleanup. Counts saturate at 128 with
an explicit overflow flag. Added listeners detach even on setup or body failure. Existing
matching, assertions, deadlines and forwarding remain unchanged. The run has no acceptance
or merge credit; no automatic repeat is authorized.

The approved entry-module diagnostic ran once, exit 1 after 33.954 seconds, with eight safe
monitor samples and unchanged source. WebKit reported one exact entry-module request at both
page and context, zero route callback entries and selections, zero attribution mismatches or
unknowns, one commit and one DOMContentLoaded. At failure the capture had no waiter and one
handle owned by generation 2. Thus no held-module precondition was established; the result
does not prove why interception was bypassed. No browser diagnostic finding was recorded,
and owned deployment shutdown completed. Evidence: /private/tmp/ovc-entry-module-observation-1;
source manifest: /private/tmp/ovc-entry-module-observation-manifest.json. TypeScript, lint,
formatting and 61 targeted tests passed before execution after correcting diagnostic field
types and lint findings. No acceptance run, automatic retry or merge followed.

The owner approved fresh-context capture fixtures and one strict three-engine native execution
on 2026-11-04, accepting only the stated historical interception uncertainty. Each capture mode
now owns a fresh context under the same deployment and validates seeded-account origin/API
before cookie-only sign-in. Strict diagnostics and policy observation precede page creation.
The unchanged hold is installed before first application navigation. Resource cleanup precedes
context closure; diagnostics finalize after closure even on failure. Existing navigation, SPA,
disposal and child-frame controls remain. Production behavior and timing limits are unchanged.

The strict fresh-context series passed on 2026-11-04 in 61.991 seconds, exit 0, 14 safe
monitor samples, unchanged source. All three engines completed hard-navigation, SPA, three
disposal, three parsed-document and child-frame controls. Evidence is retained at
/private/tmp/ovc-fresh-capture-native-1 with /private/tmp/ovc-fresh-capture-manifest.json.
This establishes the corrected fixture, not a historical browser-internal cause.

Four isolated WebKit mutation controls each failed at its intended assertion: removing the
parsing wait published capture while the module remained held (line 880); omitting disposal
join left one active waiter (line 895); removing generation guards published the old owner
(line 923); removing frame refusal left the state outside failed after child attachment
(line 987). Their exit codes were 1 as expected, with respective durations 31.308, 31.730,
37.101 and 38.478 seconds and safe monitor counts 8, 8, 9 and 9. Primary and all four copied
source manifests remained unchanged. Evidence lives in /private/tmp/ovc-fresh-negative-<guard>-1,
with copied manifests under /private/tmp/ovc-fresh-capture-negatives. The earlier coherent-state
negative control remains applicable. These are sensitivity passes, not application failures.
The account-switch late/duplicate cleanup runtime proof, seven targeted E2E, additional combined
review, full E2E and exact-tree gate remain outstanding; no readiness or merge is claimed.

The previously authorized seven-case E2E ran once after all native sensitivity controls. Six
Firefox cases passed; the WebKit same-browser password-reset case failed, overall exit 1 in
9.509 seconds with three safe samples and unchanged source. It observed one refresh request
where signup.spec.ts expected zero. The retained network evidence shows users/me 401 followed
by refresh 401 starting approximately one millisecond later; it does not itself prove the
refresh initiator. Real realtime code separately refreshes after close 4401, while REST only
refreshes token_expired. Password-reset revocation closes sockets by contract. Consequently
the broad zero-refresh assertion does not isolate the REST behavior exercised by the existing
real Session regression. Raw evidence remains private at /private/tmp/ovc-fresh-capture-seven-evidence
with /private/tmp/ovc-fresh-seven-manifest.json. No repeat or merge occurred. A deterministic
ordering plan must account for reset submission skipping reloadAccount if refresh has already
ended the session; holding only users/me cannot guarantee that request exists.

The owner approved the two-request reset-refusal ordering correction and one WebKit execution
on 2026-11-04. Both real requests are held before forwarding; refresh is released first and
its exact refusal/sign-out observed, then the original users/me request is released and its
unauthenticated refusal and final reset UI verified. Exact method/origin/main-frame identities,
one request each, unchanged forwarding and joined owned cleanup are enforced. Existing
real Session and realtime tests retain separate REST/no-refresh and 4401/refresh causality.
TypeScript, lint, formatting, 69 session/readiness tests and 12 realtime integration tests passed.
The remaining original account-switch cleanup proof and combined review are not yet complete.

The single corrected WebKit reset E2E passed, exit 0, in 4.366 seconds with two safe monitoring
samples and unchanged source. Evidence: /private/tmp/ovc-reset-refusal-webkit-evidence/observed,
manifest /private/tmp/ovc-reset-refusal-manifest.json. This proves the ordered real refusals and
final signed-out UI on that source; it does not establish the historical refresh initiator.
The separate 12 realtime integration checks passed, including real 4401-triggered refresh and
revoked-session termination. The browser execution preceded completion of the additional
route-helper sensitivity evidence; that proof remains outstanding, so this result is not a
readiness verdict and no further positive repetition or merge is authorized by it.

The new real wrong-method cleanup control passed its body assertions (two actual 405 responses,
one named guard failure, exact owned-route removal and surviving borrowed route), but the
strict diagnostic fixture failed with two unknown console_error records. Overall exit 1 in
3.835 seconds, two safe samples, unchanged source; evidence is /private/tmp/ovc-reset-route-control.
Its retained trace contains test actions only, not browser console events, so the messages and
causality cannot be reconstructed. The fixture incorrectly opened /login instead of the actual
sign-in route /; this independent setup error is corrected without claiming it explains both
console errors. No retry or mutation execution occurred. Strict diagnostics remain unchanged.

The owner approved one corrected route-control execution with full private tracing and limited
historical uncertainty on 2026-11-04. It failed strict diagnostics in 5.085 seconds, exit 1,
two safe samples, unchanged source, while all body assertions passed. The retained browser
trace under /private/tmp/ovc-reset-route-trace-1 identifies both console records as standard
405 resource failures with empty status text. HttpNegativePhase.finish requires a unique
matching completed request; declaring both identical requests in one phase leaves two candidates
for each console event and records two ambiguity failures. A local execution of the real
HttpNegativePhase reproduced exactly zero HTTP failures and two console failures for the
combined phase, versus zero failures for each separately completed phase. No matcher, assertion
or browser policy was relaxed. The earlier /login setup correction was not the cause of this
reproduced ambiguity. Proposed next work separates phases in the control; no browser retry ran.

The owner approved separate HTTP phases and one corrected WebKit route-control execution
on 2026-11-04 after independent security planning review. The first exact 405 phase now
finishes before declaring or issuing the second probe; all response, ownership, named guard
failure and borrowed-route assertions remain. Production and diagnostic matcher are unchanged.

The separated-phase WebKit control passed in 3.935 seconds, exit 0, two safe samples, unchanged
source; /private/tmp/ovc-reset-route-separated-1 retains full private evidence. TypeScript,
lint, formatting and 27 diagnostic state tests passed first. The isolated method-guard-removal
control then failed at the intended assertion requiring AggregateError from cleanup (outcome
was no error), proving guard sensitivity. It ran in 7.128 seconds with three safe samples;
primary and copied manifests remained unchanged. The initial negative-control launcher failed
before any tests because a shell shim was passed to Node (SyntaxError); the corrected launcher
invoked the same shim as an executable. Both preparation failure and intended assertion failure
remain preserved under /private/tmp/ovc-reset-method-negative-1. No acceptance retry occurred.

The approved account-switch cleanup proof now uses real invalid-token requests during the
closing window and after exact unroute, each with its own strict 422 phase. Actual late entry,
forwarding and request terminal are observed; confirmation admissions, hold count and observer
identity cannot change after closing. A separate real duplicate scenario requires both the
body boundary error and exact duplicate-route cleanup cause; unrelated cleanup errors cannot
satisfy it. Body and cleanup errors are aggregated rather than masking each other. The normal
transition uses React Router and does not imply a new Document. An unroute transport failure
is not manufactured or claimed tested. TypeScript, lint, formatting and 61 state tests passed.

The first six-case account-cleanup proof run failed all cases in 35.103 seconds, exit 1, eight
safe samples and unchanged source. Full private evidence is retained under
/private/tmp/ovc-account-cleanup-controls-1. Source inspection confirmed a circular wait in
the new control: its route callback awaited the late response after fallback, while pinned
Playwright waits for the callback to return before passing fallback to the next handler.
The owned callback now records its exact Request and returns after fallback; the closing
controller observes that Request's response and terminal outside the callback. Assertions,
strict diagnostic phases, duplicate error preservation and deadlines are unchanged. No
corrected browser execution has occurred; the earlier failed result remains preserved.

The owner approved one corrected six-case execution on 2026-11-04. All six controls passed
in 7.056 seconds, exit 0, three safe samples, unchanged source; evidence is retained under
/private/tmp/ovc-account-cleanup-terminal-1. Two isolated guard removals then failed at the
intended named assertions recorded in full traces: account_switch_closing_confirmation_not_enrolled
for removed closing admission protection (7.896 seconds), and account_switch_duplicate_failure_retained
for removed duplicate admission protection (6.592 seconds). Both had three safe samples and
unchanged primary/copy manifests. These intended failures prove sensitivity, not application
failures. Copy preparation initially rejected an incorrect indentation anchor before execution;
no partial mutation was executed. Evidence: /private/tmp/ovc-account-guard-negatives.
The transport-error branch of Playwright unroute itself is not artificially reproduced.

All eight frontend checks passed on the final cleanup-control source: TypeScript, lint,
formatting, 887 tests in 70 files, build, class validation, bundle and generated API check.
Logs: /private/tmp/ovc-warning-readiness-frontend-final-1. Initial JavaScript plus largest
language remains 176.4 KB gzip, within the owner-accepted result and hard 180 KB limit.
Documentation checks passed after removing one extra blank line flagged by MD012; both logs
remain retained as /private/tmp/ovc-warning-readiness-docs-final-{1,2}.log. Tool provenance
matched tools/go.mod. Required startup/test-control refusal and absent-route tests passed
with race detection in internal/core and cmd/internal/startup; output is retained at
/private/tmp/ovc-warning-readiness-startup-final-1.log. The fresh combined standard/security
review is in progress. Full final E2E, staged Gitleaks and exact-tree merge gate remain pending.

Fresh independent combined standard/full-security readiness review covered all 83 changed
files against 4d9ba1e, including staged/unstaged changes and four untracked plans. It found no
new blocker or major and confirmed closure of prior capture sensitivity, split readiness polls,
frame attribution and account-switch cleanup findings. Reviewed integration included session
epoch/body/cache, DocumentLifecycle/QueryClient/realtime, diagnostics, TLS and local harness
ownership. T-2/T-23, T-18/T-19/T-27, T-20/T-21/T-29 and T-36/T-37/T-41 were examined.
Verification fixes touch T-41; production session/body/cache fixes also touch ownership of
sessions/private data. Reviewed changed-file-set SHA-256 was
2ca96185e8d4eaf9fda5e29fc6950957244baf68d221ab40f2f48e981afa5e4c.

The reviewer found one minor: development and threat-model wording still described the reset
regression as globally without refresh. Both pending-owner paragraphs now distinguish standalone
REST/no-refresh proof from socket-triggered refresh and the ordered two-request browser control,
without claiming the historical initiator is known. This documentation correction changes no
trust boundary or production policy and needs checks, not another review round. Full final E2E,
staged Gitleaks and exact-tree gate remain pending; unroute transport failure is unverified,
not simulated or reported as tested.

The single authorized final full E2E completed with 979 passed and one failed out of 980,
exit 1 in 182.800 seconds, 38 safe monitoring samples and unchanged source. The Chromium
same-browser password-reset case reached both held requests and the selected refresh response
401, then waited at response.finished before releasing the held account request. The test
reached its existing deadline. The exact reason for that missing terminal is under diagnosis;
no acceptance rerun, commit or gate occurred. Full private evidence and manifest are retained
under /private/tmp/ovc-warning-final-full-e2e-1. Earlier targeted passes are not substituted
for this final-tree failure.

During local metadata extraction, an action title containing a generated synthetic fixture
password was accidentally printed to the tool output. No value is reproduced here. Subsequent
trace extraction is restricted to fixed allowlisted fields and source coordinates, never
arbitrary action titles or parameters. This is a diagnostic-handling error, not evidence of
production credential exposure; raw traces remain private.

The owner approved one isolated Chromium terminal-order diagnostic on 2026-11-04.
Only the refresh terminal assertion moved after the account response terminal; all assertions
and deadlines remained. The run failed in 36.889 seconds, exit 1, nine safe monitoring samples,
with unchanged source. Signed-out navigation was observed while users/me remained held.
After release, users/me returned 401, but reading its JSON body at signup.spec.ts:882 did not
complete before the deadline. Moving the refresh terminal wait therefore did not resolve the
failure. This diagnostic is not acceptance and authorizes no further execution or merge.
Primary production and test source remain unchanged. Private traces, exact source manifests
and observation results are retained under /private/tmp/ovc-reset-terminal-diagnostic-1;
the isolated source is /private/tmp/ovc-reset-terminal-diagnostic-copy. The cause of the missing
response terminal remains unverified; analysis continues without additional browser runs.

The owner approved one further isolated Chromium observation on 2026-11-04 under
/private/tmp/ovc-slice6-reset-network-terminal-plan.md. Independent security planning
review found no blocker or major, with explicit limits on CDP/public-request correlation
and no claim that a second session observes Playwright-owned authentication events.
The observer enables only Network events and records at most 128 fixed enum/numeric
entries alongside public request terminals and owned callback counts. No raw protocol
logging, response changes, application changes, relaxed assertions or retries are authorized.
The original failed run, first isolated diagnostic and primary sources are preserved.

The single passive Network diagnostic failed in 36.294 seconds, exit 1, nine safe samples;
primary and isolated source manifests remained unchanged. Private evidence is retained under
/private/tmp/ovc-reset-network-terminal-1. The allowlisted observation contained 51 records,
verified request correlation, no overflow, redirects, foreign-frame request or identity
ambiguity. Both owned route callbacks returned. Refresh401 received successful terminal events
in Chromium and Playwright. Account401 received headers and a Chromium data event containing
124 bytes, matching its declared length, but neither observer recorded a terminal before the
deadline. This excludes a claim that only the public Playwright terminal was missing; it does
not prove a particular Chromium, server or authentication cause. The observer detach failed
after the timeout/target closure and is explicitly recorded as cleanupFailed, not suppressed.
The selected body read at signup.spec.ts:889 failed at the existing timeout. No retry occurred.
Static TypeScript, ESLint and formatting passed before execution; the test list contained only
the selected Chromium case. The primary application and regression remain unchanged.

The account-response lifecycle plan was owner-approved on 2026-11-04 and recorded as
knowledge/plans/slice-6-account-response-lifecycle.md. Documentation checks passed
(/private/tmp/ovc-account-response-docs-1.log). Before the production fix, TypeScript passed
and the six selected real Session lifecycle controls failed at their intended assertions:
four stale loadMe/ownerOf 200/401 cases returned with consumed=false, active=1 and
completed=0; current loadMe401/403 returned with the original response consumed=false.
Command: pnpm exec vitest run src/lib/api/session.test.ts -t 'owned account lifecycle'.
Exit 1 was the planned causal negative control, not acceptance; 35 unrelated tests were
filtered. Evidence: /private/tmp/ovc-account-response-causal-original.log.
The current-refusal fixture initially observed the pre-clone stream lock rather than the
original Response's current body lock. That observation was corrected after this execution;
consumed/active/completed assertions and production source were unchanged. The original
readerLocked value is not treated as proof of a leaked original-response reader. The four
stale cases independently prove unfinished body ownership. No duplicate causal run was used.

The first fixed-source targeted execution ran session.test.ts and fakeTransport.test.ts:
84 passed, one failed, exit 1. The existing concurrent refresh contract expected 200/401
and received 401/401. Evidence: /private/tmp/ovc-account-response-targeted-1.log. All selected
Session lifecycle cases passed, but this does not make the targeted suite acceptable.
Source inspection established an introduced adapter race: SignalAwareRefreshBodies.response
assigned an observational lastResponse field, awaited a header barrier, then returned that
shared field. Another concurrent invocation could replace it before return. The controller
now retains and returns a local Response per invocation; lastResponse remains observation
only. No new test is needed to expose that race because the existing shared concurrency
contract already failed precisely. TypeScript passed after this correction. No runtime
repeat occurred; continuation requires the owner decision specified by the approved plan's
unexpected-failure stop rule. Browser, real-deadline, mutation, full-E2E and gate allowances
remain unused for this account-response change.

The owner authorized one corrected targeted execution and continuation on 2026-11-04.
The corrected session/adapter suite passed all 85 cases. All eight frontend checks then
passed, including 898 tests in 70 files; entry plus largest language remains 176.4 KB gzip.
Evidence: /private/tmp/ovc-account-response-targeted-2.log and
/private/tmp/ovc-account-response-frontend-1/results.json. The standalone real-time fake
transport controls passed at 15001-15002 ms for refresh, loadMe and ownerOf with no active
bodies or retained readers (/private/tmp/ovc-account-response-deadlines-1.log). These are not
claims of a hard total operation bound or held native-network cancellation proof.

Independent combined standard/full-security review of the account-response delta and
integration found no blocker/major. Its minor normative wording concern was corrected:
no-parse and 64-KiB refer to disposal, while existing refusal classification stays unchanged.
The reviewer reread the correction and confirmed no open findings. Fixed production ownership
crosses session/private-data lifecycle boundaries T-20/T-21/T-29; diagnostics retain T-18/T-19/T-41.
The remaining explicitly unverified paths include held native-body cancellation failure and
signal expiry before response ownership transfer; review did not assert these were runtime-passed.

Isolated deletion of stale loadMe drain failed both intended consumed/active/completed
assertions. Isolated deletion of ownerOf's initial epoch/disposal branch failed both original
reader-release assertions. The latter demonstrates sensitivity to that ownership branch,
not a separately observed replacement-account overwrite. Logs are
/private/tmp/ovc-account-response-negative-drain.log and
/private/tmp/ovc-account-response-negative-epoch.log; copies/manifests are retained.

The authorized observed Chromium control on the fixed production build passed in 7.146
seconds, three safe samples, unchanged primary and isolated sources. Both refresh and account
responses received successful Chromium and Playwright terminals; observer correlation was
verified and cleanupFailed=false. Evidence: /private/tmp/ovc-account-response-native-1.
The following targeted series passed all three browser reset cases but failed its new real
transport contract, exit 1 in 4.884 seconds, two safe samples and unchanged source. The
contract required the first reader.read after abort to reject; that assertion failed.
Evidence: /private/tmp/ovc-account-response-browser-targeted-1. The result remains failed;
no full E2E, gate or automatic repeat occurred. Body-completion observation is being examined
without inferring that a single read establishes the terminal outcome of the entire stream.

The owner approved the full-read abort observation correction on 2026-11-04 under
knowledge/plans/slice-6-account-abort-terminal-control.md. Only the new shared contract
changed: it reads until rejection or EOF, counts at most 64 KiB, and keeps EOF/overflow
separate from a read failure. Reader release and the independent awaited cancellation
check remain. An initial TypeScript narrowing error was corrected before runtime.
The selected fake contract passed; an isolated removal of abort failed precisely at
expected account body abort to reject reading. Logs: /private/tmp/ovc-account-abort-fake-1.log
and /private/tmp/ovc-account-abort-negative-1.log. The single authorized real-backend contract
then passed in 2.609 seconds, two safe samples and unchanged source; evidence and manifests
are retained under /private/tmp/ovc-account-abort-terminal-1. This passing full-read outcome
does not claim the exact historical cause of the first successful read. The earlier failed
result remains retained. Full E2E and exact-tree gate are still pending, not substituted by
this transport result.

Final account-response frontend checks all passed again after the full-read contract change
(/private/tmp/ovc-account-response-frontend-final/results.json), including 898 tests in 70
files. Documentation checks, tool provenance and staged Gitleaks passed. The one authorized
full acceptance E2E passed in 157.463 seconds with 33 safe samples and unchanged source;
its manifest and private output are retained under /private/tmp/ovc-account-response-full-e2e-1.
All earlier failed attempts remain failed and retained. Standard tier with the security trigger
remains applicable to the final diff. Independent review coverage combines the prior complete
browser-diagnostics review with the account-response delta review and corrected minor wording;
the narrow abort-observation correction follows its approved security-assessed plan and its
positive and negative runtime controls. The exact-tree merge gate is the remaining integration
check, not yet passed. Slice performance measurements and closure are separate outstanding work.

Count reconciliation before commit: the authoritative full-run logs record 977 passed and
one failed out of 978 in /private/tmp/ovc-warning-final-full-e2e-1, followed by all 979 passing
in /private/tmp/ovc-account-response-full-e2e-1 after adding one shared transport contract.
Earlier narrative totals of 979/980 in this record and diagnostic plans were transcription
errors, not additional executed cases. No test was removed to obtain the final result; the
full-run configuration is unchanged. The source manifest identifies signup.spec.ts as the
only changed E2E file, and the transport contract gained one case. Use the logged totals
and exact manifests for acceptance rather than the erroneous earlier narrative counts.
