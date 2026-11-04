# Document-owned follow-up query refetch

Status: approved by the project owner on 2026-11-03, including limited historical uncertainty and one additional combined readiness reviewer. Standard tier; security trigger T-20/T-21/T-29/T-41
for request/account ownership and verification integrity, plus T-18/T-19 for safe diagnostic evidence. Implementation and bounded verification are authorized in the sequence below. Existing failures and commits remain preserved.

## Evidence and limits

Main is 4d9ba1e5467c84b1a09c1737c00ea2c8e4c24271. The one authorized WebKit location diagnostic
on the 66-file manifest ovc-navigation-readiness-webkit-location-manifest-2.json failed after
29.856 seconds, exit 1, seven safe samples, unchanged source. All six WebKit body checks and
browser/deployment teardown completed. Three native weberror records occur at begin-main-navigation.
Their exact own-build stack locations reach session-Dl_wg831.js:1:3346 (authorizedRequest sending
HTTP), index-BTnM7Asn.js:2:3112 (the SECOND super.refetchQueries), with join-requests, tutor/code
and health readiness consumers. The exact native message remains unclassified and must not be
assumed to be an access-control refusal or exempted from diagnostics.

FollowingQueryClient's second wave has no document/account generation check after awaiting its
first wave. liveRefetch's suspension controls the chat coalescer, not this pending generic
invalidation continuation. The source gap exists at main, not just in the diagnostic change.
The ordering of beforeunload against second-wave dispatch was not recorded. A guard's ability
to resolve the observed failure is therefore not yet proven.

## Outcome and scope

A follow-up REST wave belonging to a departed or suspended document or an earlier account cannot
start. The existing first-wave requests and their error handling are unchanged. A current active
owner retains its required second-wave refetch after initial loading. A cancelled navigation or
BFCache restoration resumes through the existing realtime reconciliation lifecycle; old pending
continuations never revive merely because the page becomes active again. This is no aborted-request
exemption, new retry, timeout change or global suppression of browser errors.

Use a monotonically increasing owner generation in the real FollowingQueryClient. Capture both
active status and generation when an operation starts. After its first await, the second wave
requires an active entry, the same generation and current activity. Suspension, resumption and
account cache clearing invalidate pending owners; override clear() to advance the generation
before super.clear(). A first wave started while suspended keeps its existing behavior, but cannot
gain a second wave after resume. Preserve the generic accountCacheOwnership interface.

Install one document lifecycle owner synchronously in main.tsx immediately after QueryClient
creation, before Session startup/subscriptions can initiate asynchronous work. Extract the existing
beforeunload/pagehide/pageshow/load state and the 2-second aborted-navigation timer from wireRealtime
into this owner; do not create a second listener/timer system. The owner drives QueryClient
suspend/resume synchronously and exposes a precise activity/loaded snapshot and subscription to
realtime. Its constructor receives the existing EventTarget/loaded predicate/Scheduler interfaces.
A real dispose method removes listeners, cancels the timer and detaches subscribers in tests.
Realtime consumer disposal also unsubscribes and closes its owned transport; snapshot acquisition
and subscription must not miss a state transition.

Pass this already-installed lifecycle owner as a required typed dependency to wireRealtime and
startRealtime. Realtime subscribes to current state, connecting only when loaded and active;
a late dynamic import cannot revive a hidden/departing document. Activity resumption starts new
reconciliation through the existing reconnect logic. Keep liveRefetch's generic QueryClient type
and chat coalescer behavior. Replace the realtime tests' fixture with the real FollowingQueryClient
and real lifecycle owner, using existing fake scheduler/gateway/transport interfaces. Adapt the
native realtime reporter canary's typed imports and constructor wiring to the same real owner.
No private QueryClient access, synthetic browser events or transport patching.

Affected implementation: frontend/src/app/queryClient.ts, frontend/src/app/realtime.ts,
frontend/src/main.tsx and a small frontend/src/app/documentLifecycle.ts extracted owner;
frontend/scripts/check-realtime-error-reporting.ts typed real-owner wiring; their real component
integration tests and existing query ownership tests; frontend/scripts/check-navigation-readiness.ts
for bounded evidence and regression. Session/transport, backend, dependencies, HTTP policy and
ordinary query error propagation remain unchanged. Update knowledge/design/web/navigation.md and
pending-owner wording in docs/development.md and docs/threat-model.md, plus the English verification
record. No deployment environment, credential, remote service or private application export.

## Sequence and proof

1. Establish the independent lifecycle defect with a real QueryClient/fake-adapter integration
   regression on unchanged production source: start an initial query, start connected invalidation,
   hold first-wave completion, deliver departure through the existing injected EventTarget, then
   release the first wave and observe a forbidden second wave on the same Query object. This
   approved negative control must fail its no-follow-up assertion on old source, not a setup
   failure or timeout. Keep the observer mounted; prove the first response succeeds, actual
   departure effects occurred and Query identity is unchanged. Await the real invalidation
   settlement or its deterministic operation barrier before checking the final dispatch count. No mocked query
   methods, fetch replacement, browser synthetic events, retries or relaxed assertions. If the
   real regression does not demonstrate the claimed defect, stop and revise this plan.
   The browser error's exact message and native beforeunload timing remain unverified; owner
   approval explicitly accepts this remaining historical uncertainty for fixing the independently
   demonstrated defect and one subsequent strict native comparison. This does not accept any
   remaining browser error or waive the final acceptance criteria. No additional pre-fix browser
   diagnostic is proposed.
2. Implement the generation guard and typed lifecycle wiring only after that real-path negative control demonstrates the
   stale continuation. Preserve original required current-query refetch behavior and all diagnostic refusals.
3. Exercise real FollowingQueryClient with the existing injected fake API adapter, real Session
   and real cache: pending first wave across departure; suspend then resume before completion;
   a new operation born suspended then resumed; departure before dynamic realtime installation;
   account replacement/clear before completion; current owner still dispatches its required second
   wave; failure in first wave preserves both default handling and explicit throwOnError behavior.
   Prove account replacement keeps only the new account data; do not claim that this alone is a
   mutation-sensitive generation test because cache.clear already removes old query identities. No mocked fetch or
   replaced QueryClient methods. Negative controls remove the guard in an isolated source copy
   and must fail the named departure assertion with the same retained Query object; account-clear
   behavior is verified independently. Do not change assertions or deadlines.
4. Native navigation checks in all three engines keep the old held notification request, actual
   new-document readiness, current-held 5-second refusal, SPA ownership and repeated disposal.
   Existing realtime tests cover cancelled-navigation resumption and BFCache recovery. Verify the
   combined actual lifecycle boundary, not only a pure state machine or compile success.
5. Run all affected frontend checks, targeted signup/schedule browser journeys, docs and scanners.
   A fresh combined standard/full-security reviewer checks the complete fixes and integration
   boundary, including the previously recorded major control-lifecycle finding. Because the prior
   re-review found a major, owner approval of this plan must explicitly authorize that reviewer.
6. Only after these checks/review pass, run one complete E2E on the corrected source and one
   exact-tree merge gate within 260 seconds. No unknown failure, automatic retry, deadline increase,
   test removal or diagnostic allowance. Preserve 9adddce and all failed evidence; use the existing
   uncommitted replacement branch, one coherent commit, ordinary gate/ff/finish on current main.

## Resource and observability contract

At most three active working sessions across chats. Browser/backend/heavy checks are sequential;
ordinary checks require preflight load/service checks, owned caffeinate and continuous monitoring.
No long performance series is authorized by this follow-up; the original slice closure matrix
remains mandatory afterward. Native diagnostics expose at most 128 fixed-category records, own
script indexes/coordinates and ordered lifecycle events, never raw error text, stack, URL, payload,
account or credential. Unknown events remain failures. No production per-query logs are added.

## Risks

A late generation check cannot undo first-wave work that already started. Clearing the cache must
invalidate owners even while queries are running. A page returning from BFCache or a cancelled
navigation needs new reconciliation, not a permanently suspended client. A purely test-local wait
would hide the real lifecycle gap and is not an acceptable substitute. Additional frontend bytes
must remain within the existing entry/language budget, and runtime/gate budgets stay unchanged.

## Planning review

Independent security planning review initially found a major pre-lazy-install lifecycle gap.
The synchronous shared owner resolves it in this plan. Final review found no blocking issue after
replacing ambiguous native timing inference with a real-path baseline negative control. Its final
criteria are incorporated above. This is planning evidence only; implementation, regressions, strict native comparison, readiness review, full E2E and gate remain pending.
