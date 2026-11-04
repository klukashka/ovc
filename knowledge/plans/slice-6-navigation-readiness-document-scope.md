# Document ownership for browser navigation readiness

Status: approved by the project owner on 2026-11-03 by the instruction to continue the presented plan. Standard tier; security trigger
met for verification integrity and request/session ownership (T-18/T-19/T-20/T-21/T-29/T-41).
Independent security planning review and owner approval precede code. Existing assertions, retries,
operation deadlines and the 260-second merge-gate ceiling remain unchanged.

## Observed failure and desired outcome

The diagnostics candidate commit 9adddce90904e8517a02a11fa2c8732a14befba4, tree
eba49f406d1cce18aa69b755796380e616bcd926, passed a separately approved complete 972-case E2E.
Its first merge gate then failed one Chromium pilot signup readiness assertion: 971 cases passed,
one failed. The observed command took 252.905 seconds with 52 safe monitoring samples and unchanged
source. All non-E2E gate commands passed. Main remains 4d9ba1e5467c84b1a09c1737c00ea2c8e4c24271.
The failed commit, branch, gate logs and native trace remain preserved; no unchanged repeat or
history rewrite is proposed.

The retained browser trace establishes four GET requests (notifications, channels/unread,
join-requests and tutor/code) from the intermediate signed-in root document at approximately
29920 ms. After the actual hard navigation to the second Today document, its own post-ready requests start around 29996 ms and complete successfully around 30010–30013 ms. The old requests have no terminal
observer event before the assertion, and remain in trafficAfterReady's page-wide pending Set.
The unchanged five-second readiness poll therefore sees old-document entries after the current
page is ready. This is not evidence of an API timeout or an absent current response. The helper
has no document generation or main-document reset.

Make readiness own the current committed main document. An observation from an earlier document
cannot satisfy current readiness, consume a required current query completion or remain current
pending work. Current-document failures, incomplete queries, missing ready and missing post-ready
queries still fail. A SPA navigation retains the same document/socket scope rather than clearing
required work. No aborted-request exemption, diagnostic matcher fallback or production change.

## Files and state boundaries

Change frontend/e2e/realtime.ts and, if needed for the explicit state machine,
frontend/e2e/navigation-readiness.ts; named state contracts under frontend/scripts and one explicitly
invoked scripts/check-navigation-readiness.ts native control entrypoint; the existing affected
signup scenarios only where attachment/disposal of the observer requires it; pending-owner wording
in docs/development.md and docs/threat-model.md; this plan and English verification/budget records.
No route, authentication policy, Session, QueryClient invalidation policy, backend, dependency,
new environment setting or ordinary test timeout changes.

Model an explicit main-navigation generation with navigation-start and committed-document states.
Use the actual main-frame navigation request and frame commit from Playwright, preserving redirects
as one pending main navigation and verifying the current main frame. Reset old pending/query/ready
ownership at a real new-document boundary. Requests emitted by the departing document while the
main navigation is pending cannot enroll in the next committed document. An old WebSocket's ready
callback carries its original generation and cannot mark the new document ready. A WebSocket created
while the main document has not committed is not adopted as the next document's socket. Frames,
requests, completions and failures are accepted only with their enrolled generation. Same-document
SPA navigation is distinguished from an actual main navigation and does not reset the generation.
A real failed main navigation, ambiguous commit or incomplete current owner is failed/unverified,
never an inferred successful document.

Keep the existing rule that each required query must start after the current socket's ready and
complete, plus no current pending API request. Do not silently accept a pre-ready query as its
replacement. Discard references to departed-document work without claiming those native requests
completed, suppressing a diagnostic, or changing the actual application cancellation behavior.
The production/browser diagnostic collectors keep every existing failure rule. Observer bookkeeping
exports only fixed route classes/counts and safe generation/state categories; no raw URL, query,
account identifier, cookie, body, exception text or browser-private state export.

## Checkable acceptance

State contracts exercise old pending requests with no terminal event, late old ready and completion,
new navigation before commit, redirects, failed navigation, same-document changes, repeated document
changes, missing current ready, missing post-ready query and held current request. The old-event
negative cases must not make a new document pass or mutate its counts. A current requestfailed,
a non-success response for a required query and absence of a current request terminal event must
not produce readiness. Explicit idempotent dispose removes Page and WebSocket handlers and all
retained generation references; repeated attach/dispose has no listener growth. Existing diagnostics
still own intentionally declared negative HTTP outcomes outside the required readiness query set. These are direct tests of an
intricate state machine, not patched Page/WebSocket/fetch behavior or synthetic browser events.

The explicitly invoked native control uses the real production frontend/API with existing accounts
and the permitted unchanged request hold/continue barrier. It crosses an actual hard main-document
navigation with observed old work, then verifies actual new ready/query completion; a current-query
hold cannot pass readiness and must fail at the original deadline when left unreleased. Release and
join all held continuations unchanged during cleanup; thrown continuation errors remain failures.
A separate SPA transition retains the current document generation and does not waive pending work.
No sleeps, networkidle, browser offline emulation, response fabrication, emitted fake Page events or
raised timeout. State contracts prove the no-terminal-old-event case without falsely claiming the
browser is forced to omit its terminal notification. The original failed trace remains the native
evidence for that historical missing-event sequence.

Run the three-browser affected signup/schedule journeys, complete frontend checks, documentation,
staged Gitleaks and existing backend test-control refusals/absence as applicable. A fresh independent
combined standard/full-security review examines the observer fixes and complete integration boundary.
Run one new full E2E on the corrected source and one new exact-tree gate only after targeted/native
controls and review pass. A new unexplained failure blocks merge; no automatic unchanged retry.

## Branch and integration order

Preserve 9adddce and its branch and immutable failed evidence. Do not amend or rebase. Prepare a new
branch/worktree from current main and squash the existing committed diagnostics change into its
staged tree, then implement this approved follow-up and keep one coherent commit. Refresh main and
all relevant worktrees before staging, review, commit and gate. The final tree receives its own gate;
only a passed gate within 260 seconds allows ff merge and finish. Record exact trees, every command
exit, lane times, aggregate time and hardware/cache qualification in the final merge/closure evidence.

This approval authorizes the named implementation and bounded correctness checks, no change in the
durable measurement matrix and no long performance run. At most three active sessions across chats; heavy checks
are sequential, full performance still requires otherwise idle hardware. The native upload stand
may continue independent approved source preparation while this plan is reviewed.

## Planning review

Independent security planning review found no blocker or major. Its minor clarifications are
incorporated: distinguish request start/completion times; reject current failures or absent terminal
evidence as false readiness; and make observer disposal and repeated attachment resource checks
explicit. This is planning evidence, not executable readiness. Owner approval was recorded on 2026-11-03; implementation and runtime evidence remain pending.
