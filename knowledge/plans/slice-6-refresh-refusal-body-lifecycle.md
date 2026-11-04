# Refresh refusal response lifecycle and production check follow-ups

Status: approved by the project owner on 2026-11-03, including the bounded lifecycle correction, historical WebKit provenance uncertainty, one further source-changed targeted attempt and the additional independent combined review. Standard tier; security trigger met for session lifecycle, account isolation, response-resource ownership and verification integrity (T-2/T-18/T-19/T-20/T-21/T-23/T-29/T-41). Independent security planning review is required before owner approval.

## Observed failure and bounded scope

The first source-changed targeted production run exercised 156 cases in four files on tree afbc5eac34e148a5590b8169a4c145c0d2f21067: 144 passed, twelve failed, command exit 1 after 64.401 seconds. Observations were valid and source unchanged. All twelve trace archives and the original result are retained privately; several late fixture failures still have only test.trace rather than a complete browser event stream. No full production acceptance run has started.

Read-only triage separates five HTTP/body-phase failures, two ambiguous batches of identical negative responses, four readiness scope errors, and one WebKit page error whose exact producer is unverified. Source and available network evidence identify these concrete problems:

- A status-only test helper does not consume login bodies, and some callers leave the expected phase open until context teardown.
- Five identical DELETE 422 responses were declared in one phase, making native console attribution ambiguous. Each real attempt needs its own finite phase and original field assertion.
- The readiness helper incorrectly requires join requests from an unverified tutor, although that production query is intentionally disabled. A second-account tracker also crosses an intermediate document created by the sign-in helper.
- The token-page reload begins after address stripping without waiting for the real initial verification request/UI state to finish.
- Session.refreshNow returns from its 401/204 branch without consuming the response body. Available Chromium traces show expected refresh 401 headers without complete body evidence in several failures. That source defect is relevant to response ownership; the old artifacts do not establish garbage collection or a unique cancellation producer for every HTTP failure.

The already approved diagnostic plan covers the harness corrections above. Its implementation may proceed without changing production session code or relaxing diagnostics. Those fixes preserve every original assertion, complete-response requirement, exact method/address/status/page/phase/count matching, deadlines and retries. They do not prove the unknown WebKit event's historical cause.

## Proposed production correction

Files: frontend/src/lib/api/session.ts and its real constructor/adapter integration tests; the minimal existing cache-ownership callback extracted into app/queryClient.ts and invoked at the same point in main.tsx so the real composition can be exercised without duplicating production clearing logic; the existing FakeAuthServer/transport contract support only if a precise response-body lifetime control is needed; the already approved account/session/signup/readiness helpers and their evidence; relevant session/navigation contract wording in knowledge and pending-owner wording in docs/development.md and docs/threat-model.md. No backend authorization, route, token policy, dependency, global error exemption or new setting changes.

On the existing refresh-refusal branch, end the old session and clear its token/private-data ownership immediately on the recognized status, before waiting for body completion. Then drain that same real response under the existing 15-second refresh request signal, with an explicit 64-KiB maximum and bounded streaming state rather than an unbounded whole-body allocation. Preserve the 204 empty-body case. Release the reader on every path; overflow cancels the owned body and returns a fixed failure, and read/cancellation errors remain failures of that operation. Never log response content, credentials or raw diagnostic chains.

The later body outcome cannot restore the ended session, set unavailable over a replacement account, accept a token, publish stale account data or replay a request. Check epoch inside the acquired Web Lock callback before sending any cookie-authenticated refresh, not only after its response. Preserve and recheck supersession at each later body/ownership boundary. A queued callback from an ended epoch must return superseded without an outbound request after adopt installs another owner. Body failure must not be converted into a successful refresh or swallowed to make a test pass. The existing refresh deadline is not raised, and holding a Web Lock while the response drains is reported as resource-lifecycle impact rather than an invisible auth-policy change.

Acceptance requires an actual Session with the real constructor boundary and the real query-cache/navigation composition, not snapshot-only assertions: refusal immediately yields signed-out state/token/cache ownership release while the body is still held; normal body release completes without retained reader/request resources; a replacement account installed during that interval remains the owner after success/failure; incomplete, failed and oversized bodies cannot become successful renewals; 204 remains anonymous as before. Extend the existing fake adapter with an explicitly constructor-selected signal-aware post-headers stream mode. Separate headers/body barriers support exact completion, read failure, overflow and AbortSignal cancellation; the existing pauseRefreshes pre-headers promise and its ignored signal do not prove these cases. Check abort and body ownership through the shared transport contract against both the fake and production adapter in the permitted local configuration, with no patched fetch, mocks, custom RoundTripper or browser response interception. Test exactly 64 KiB and one byte above it, reader/lock release, and no extra refresh or replay. A causal queue control holds the first refusal body, queues another refresh, adopts another account, then releases or fails the old body: the stale callback must send no request, return superseded and preserve the new cookie/token/private cache. Add the explicit opt-in scripts/check-refresh-response-lifetimes.ts command for the actual existing 15-second deadline; ordinary Vitest case timeouts and default heavy-test selection remain unchanged. Actual native HTTP completion and existing auth journeys are then verified against the real API in all three browsers.

## Historical uncertainty and check authority

The WebKit token-page event from the targeted run lacks its browser stream and remains unverified. The actual reload ordering error can be corrected and verified, but source inspection alone does not prove it produced that particular event. Approval must explicitly accept this named historical provenance uncertainty for one further source-changed targeted attempt after the concrete corrections, with unknown errors remaining strict and no retrospective pass. If it recurs, stop for diagnosis; no automatic unchanged repeat is granted.

The previously authorized single full source-changed production acceptance remains unused. Run it only after targeted checks, real native controls and required reviews pass. The earlier production 933/38, diagnostic 35/3, targeted 144/12 and all preparation failures remain in the evidence. No failed gate is retried by this plan.

Approval also authorizes one additional fresh independent combined standard/full-security review for the new session resource-lifecycle scope and its integration with the complete diagnostic boundary. The prior review covered the earlier snapshot and its cheap minor fixes, not this production session change. Apply AGENTS after-fix rules to any new findings; never downgrade a review severity.

Run the appropriate session/transport integration and negative controls, affected three-browser E2E, complete frontend checks, documentation/scanners and existing test-control refusal/absence checks, then the unchanged exact committed-tree gate at 260 seconds and ff merge/finish. Heavy commands remain sequential and total active working sessions across chats at most three. This is not performance acceptance or permission to change the frozen closure matrix.

## Planning findings incorporated

The initial independent security planning review identified two major design gaps before approval: a queued callback needed an epoch guard before its cookie-authenticated send, and the existing fake did not model signal-aware post-headers body ownership. Both are explicit implementation and causal-control requirements above; neither is treated as a completed code fix. The review also requires actual cache/navigation composition evidence. The same independent reviewer completed the required governing reads and confirmed the clarified plan has no remaining blocker/major findings. These are mandatory future implementation requirements, not code-fix or runtime evidence. Implementation is owner-approved; readiness is not granted by planning evidence.

The existing start() error-to-state wrapper remains unchanged: a failed refresh operation rejects, while start preserves an already ended or replacement-owned state rather than overwriting it with unavailable. That wrapper behavior is not a successful refresh or proof of HTTP completion. Its causal control separately verifies signed-out/private-cache state and the observed body failure.

## Implementation boundary clarification

The owner-authorized implementation extracts the existing account-id cache-clear callback into
`frontend/src/app/queryClient.ts` and invokes it at the same position in `frontend/src/main.tsx`:
after requesting lazy realtime and before preference handling. It introduces no new subscription,
cache policy or authentication decision. The actual Gate/router integration uses that same binding.
`start()` retains its existing error-to-state wrapper: direct refresh/authorized requests reject on
body failure, while startup does not replace a signed-out or newer owner with unavailable. This is
state handling, not proof that an incomplete native response body completed. Stale 401 responses
are drained without ending a replacement owner; stale 200 authentication behavior is unchanged.
