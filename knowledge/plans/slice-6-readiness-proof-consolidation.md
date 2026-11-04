# Coherent browser-readiness ownership and sensitive controls

Status: approved by the project owner on 2026-11-04, including fixes, controls, bounded historical uncertainty and one additional combined readiness reviewer. Standard tier; full security trigger T-18/T-19/T-20/T-21/T-29/T-41. Verification helpers and tests only; production behavior, authentication, CSP, retries and gate limits remain unchanged.

## Findings and evidence

The fresh combined review returned three majors: capture controls do not prove their lifetime claims and omit request-failure verification; readiness uses two separate polls that can pass across different documents; WebSocket page/address matching does not prove frame ownership. It also found minor account-switch cleanup gaps on route setup/removal failure or duplicate handlers. The current native run failed at Firefox supersession with an assertion and warning after 48.070 seconds; Chromium passed its nine bodies, Firefox passed eight, WebKit did not run. Exact failed assertion and warning causality remain unverified. The seven-case execution is still unused. All failed evidence remains preserved.

## Implementation

1. Readiness becomes one synchronous observation of capture completion, current socket ready and zero pending/missing reads, sampled by one existing five-second expect.poll. Do not independently poll pieces or reset the deadline on generation changes. Keep individual observation methods only for controls that explicitly inspect a non-ready state.
2. Expose read-only capture evidence: current generation, captured generation or null, active lifecycle wait count and active handle count. Values derive from actual waiter/handle acquisition and finally cleanup, not from a desired outcome. No paths, identifiers, tokens, arbitrary messages or browser payloads. Keep fixed failure reason and safe assertion stage labels so a new failure identifies its control without dumping private data.
3. Make controlled hard-navigation ordering explicit. Add an initializer barrier for callers that install an observer before initiating a hard navigation. It joins the initial capture before goto, so the old page cannot resolve cached load state and enqueue an evaluator into the new navigation. Add an explicit pre-navigation barrier for subsequent controlled hard navigations: invalidate capture ownership synchronously, join old capture work within the approved five-second cleanup bound, then initiate goto. Network listeners remain installed and begin tracking the new navigation from its actual request/commit. A failed or never-committed goto leaves the observer ineligible, never restores the old ready state, and surfaces its failure. The existing request/commit handlers remain fail-closed for unannounced navigation; they cannot declare a capture ready based on old evidence. Account-switch observation starts synchronously at its already validated confirmation boundary on the same parsed document, with no delayed socket enrollment. SPA identity checks keep the actual Document reference. Audit all trafficAfterReady callers and update only those that initiate controlled hard navigation; no broad routing refactor. Existing readiness deadline and page/action limits remain unchanged.
4. Enforce the application-supported single-main-frame invariant for readiness attribution: reject an existing child frame, fail on frame attachment while the observer is active, and recheck before accepting a socket/ready result. This closes ambiguous page-scoped WebSocket attribution without claiming unavailable frame metadata. Do not change frame-src 'none' or browser policy. Add real iframe-attachment refusal and existing-main-frame success controls, directly observing attachment of an actual child frame under unchanged CSP (an empty about:blank frame where supported), not merely an attempted insertion; no WebSocket stubs. A child frame must never satisfy readiness or replace a main-frame owner. Apply exact origin/method/main-frame ownership to REST as already implemented.
5. Repair account-switch cleanup in one owned scope: registration belongs inside protected cleanup. Enter a synchronous closing state first; no later callback may enroll a new hold or readiness owner. Release barriers, remove only the exact owned route, and drain the tracked set of all operations, including callbacks entering while removal was in flight. Such late callbacks only forward unchanged and remain tracked until terminal. After successful removal establishes no further admission, join until the operation set is empty. Never call unrouteAll on borrowed routes. If removal fails, retain explicit failed/incomplete ownership, attempt all independent cleanup and surface failure; do not claim the handler was released or discard its late tracking. Fixture-owned teardown remains responsible for its page, with no forced closure of a borrowed page. Detach listeners and dispose handles even when another cleanup step fails. Aggregate failures after attempting each owned release. Duplicate requests still fail the boundary; never discard their callback failure. No synthetic responses or mutation of requests.

Affected sources: frontend/e2e/realtime.ts, navigation-readiness.ts, signup.spec.ts, scripts/check-navigation-readiness.ts and its state tests. Mechanical hard-navigation barrier adoption may touch the existing callers in accessibility, account-edge-cases, groups, lesson, message-attachments, messages, realtime, schedule, session, smoke, students, teaching-settings and upload-transport E2E suites. Review the complete resulting diff. Update navigation evidence contracts and pending-owner development/threat-model wording, plus the English review record. No production-module changes.

## Proof and negative controls

The real held entry-module must block DOMContentLoaded. Before release, assert actual capture remains absent and a real lifecycle waiter is active; after disposal while still held, require zero actual waits and handles. On supersession, tie successful capture to the new generation and prove no old generation publishes a handle. Do not require exactly one page-level DOMContentLoaded count across cancellation of an old document: distinguish old and current document lifecycle instead of assuming a global count is document identity. Every control installs nativeActivity and verifies it. If the old held request is canceled, require that exact request identity, observed terminal event and the existing navigation cancellation rule; an unrelated/current failed resource is a failure. Preserve strict console/HTTP/page errors.

Use isolated source-copy negative controls, each changing only one named guard: remove the actual parsing wait, omit joining it on disposal, permit stale-generation publication, split the coherent readiness predicate, and omit the child-frame refusal. Each must fail its corresponding named state/resource assertion, not fail preparation or timeout unrelated to that assertion. Controls may use existing pure state machines for coherent snapshot and generation interleavings, but browser resource lifetime and frame attachment require real browser paths. Do not replace runtime functions, mock fetch, fabricate frames or alter responses. Prove account-switch cleanup restores listener/handler ownership on normal paths and identifies incomplete ownership on real error paths, including a late/duplicate request during closing, using the real route hold lifecycle. No runtime patching of route removal is permitted.

## Verification and decisions requested

After implementation and all targeted/static checks, run one strict native series on the corrected source (including bounded negative controls above). Unknown failure or a control insensitive to its mutation blocks continuation; there is no automatic retry. The owner explicitly accepts historical native warning/assertion uncertainty only for fixing these independently source-proven review findings and this one corrected series. If it passes, consume the previously approved single seven-case E2E execution. Chromium's prior switch result remains historical evidence; the full E2E will cover the final integration.

Authorize one additional fresh combined standard/full-security reviewer after checks to recheck the three fixed majors, the minor cleanup fix and the complete integration boundary. This is required because the preceding re-review returned majors. Only when checks and review pass, perform the already planned full E2E and exact-tree gate <=260 seconds. No pass is claimed for unrun checks; no old result is substituted for a new tree. Initial bundle acceptance remains 176.4 KB with hard limit 180 KB.

## Resources and privacy

Maximum three active working sessions including other chats. Native/E2E/heavy checks run sequentially after device/service checks with owned sleep protection and continuous monitoring. No long measurement matrix is started by this approval. Full tracing remains enabled; raw traces and reporter output stay private outside served paths with at most seven-day retention. Safe durable output is bounded fixed categories/counters only. Preserve prior commits and failed results; no history rewriting.

## Risks

Explicit navigation barriers must not miss the first request or socket; all listeners stay installed. A barrier may add up to the previously approved five-second cleanup for a held old document, but cannot restart readiness deadlines. Resource counters are evidence only and must not replace real lifecycle assertions. Single-frame enforcement intentionally rejects unsupported observation contexts and must not be represented as production authorization. If the design cannot prove these conditions through public APIs, stop before accepting the result.

## Planning review

The initial security planning review required an explicit route closing/admission boundary and clarified failed-navigation and actual iframe-attachment controls. These conditions are included; security planning recheck closed the finding with no remaining planning blocker. No implementation or execution followed from the proposed plan.

## Proposed additional execution after terminal-order correction

The single corrected native series failed on 2026-11-04 after 39.049 seconds. Chromium reached
a successful new-document capture with zero waiters and one handle, but the subsequent old-request
terminal control failed; the exact assertion is unverified. Source inspection independently
confirmed that waiting for terminal before releasing a held route and requiring cancellation
unconditionally exceeded this plan's intended conditional proof. The implementation now forwards
unchanged first, retains separate terminal listeners, and distinguishes successful completion
from exact unanswered native cancellation after the observed navigation action. All other failed
resources remain fatal through the existing verifier.

One additional native series on this corrected source is proposed, with existing negative
controls afterward only if the positive series passes. This needs an explicit owner decision
accepting the limited uncertainty of the previous assertion; it is not authorized by the initial
single-execution allowance. No automatic repeat, timeout increase or assertion waiver. The earlier
seven-case allowance and additional readiness reviewer remain unused, conditional on passing
controls. Previous source manifests and failed outcomes remain preserved.

The owner approved the one additional corrected native execution and its limited historical
assertion uncertainty on 2026-11-04. No further repeat is authorized.

## Proposed execution with ordered supersession control

The additionally authorized run failed and remains retained. The supersession control has now
been aligned with the plan: invalidate ownership, release and finish the real old module, join
retirement and prove no old capture/remaining resource, then navigate and prove the new owner.
This avoids starting goto before the approved preparation barrier has settled. The separate hard
navigation control continues to prove exact unanswered native cancellation when observed. Safe
failure output includes only this control's numeric source coordinates, not messages or stacks.
One native series on this corrected implementation is proposed, conditional on an explicit owner
acceptance of the still-unverified historical assertion location. The other execution/review limits
remain unchanged; no automatic repetition or seven-case execution precedes a passing native series.

The owner approved one ordered-supersession native execution and limited historical uncertainty
on 2026-11-04. No automatic repeat is authorized.
