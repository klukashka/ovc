# Complete owned account-response lifecycles

Status: security planning review completed, no blocker or major; both timing
clarifications incorporated. Owner-approved on 2026-11-04. Standard tier with security trigger:
T-20 resource ownership, T-21/T-29 account isolation, T-18/T-19 safe evidence, T-41 verification.

## Problem and desired result

The full E2E failed one Chromium reset case out of 980. Two isolated diagnostics
preserved every assertion. The latest observed all 124 account-response bytes but
no Chromium or Playwright terminal before timeout. Both route callbacks returned;
refresh completed and signed-out navigation was visible before the account request
was released. These observations do not establish historical causality.

Source independently shows that Session.loadMe abandons an owned Response when its
epoch becomes stale, and also returns on 401/403 without completing that body.
Session.ownerOf similarly abandons its account response when its epoch becomes stale.
The existing normative drain requirement covers refresh401/204 only. Extend it
explicitly to discarded account responses owned by these two methods. Never parse
or publish stale account data. Request transport continues transferring response
ownership to callers; do not drain arbitrary caller-owned API responses.

## Contract and scope

Give each loadMe operation a 15-second signal starting before its HTTP request;
this is a new HTTP/body signal bound, not an increase to a pre-existing timeout.
It does not claim a total 15-second wall-clock bound for authorizedRequest, which
may await an independently bounded refresh. No refresh or Web Lock budget changes.
Reuse ownerOf's existing 15-second signal, rather than starting a new post-response
window. Discarded account bodies have a 64-KiB streamed maximum, reusing the existing
reviewed bounded-drain mechanics through accurate shared naming. Do not buffer,
parse or log their content. Overflow, abort and read/cancel failures remain failures
of the owning operation; release reader ownership and preserve failure evidence.
The existing helper awaits reader.cancel(), so the signal alone does not prove
bounded cleanup settlement. Contract and real transport controls must establish
cancellation settlement and reader release for these paths within their existing
check deadlines. Record cleanup separately from the HTTP/body signal budget; if
settlement cannot be established, stop rather than hide the pending cleanup, raise
a deadline or claim a hard total operation bound.
Immediate refusal/signed-out behavior and cache clearing must precede disposal;
a later body result cannot restore or replace any account, token or cache owner.
A stale success response is discarded, never interpreted as a new account.

Keep successful current account validation unchanged and preserve existing refresh
refusal behavior. No backend authorization, API schema, retry, dependency or general
browser diagnostic allowance changes. This plan does not change stale refresh200
or arbitrary API response ownership. If implementation requires a wider ownership
change, stop for scope approval rather than silently expanding it.

## Files and work order

1. Record the approved plan and failed evidence under knowledge. Update the account
   specification and pending-owner development/threat-model wording with the exact
   account-response bound and isolation rule.
2. Extend the existing constructor-selected fake adapter only as needed for real
   post-headers account-body ownership, with shared transport contract coverage;
   no patched fetch, fabricated browser response or network interception beyond
   the already allowed unchanged holds.
3. Before the fix, exercise real Session.loadMe and ownerOf with delayed responses
   spanning session termination/account replacement. Prove the current unread-body
   defect by checking real reader/consumption lifecycle, not snapshots alone. This
   planned failing control is retained as evidence and not acceptance.
4. Implement the narrow lifecycle change in frontend/src/lib/api/session.ts and
   its existing session/transport integration tests. Cover stale 200/401 for each owner, current
   401/403 refusal, exactly 64 KiB/one byte over, aborted/failed bodies, explicit
   cancellation failure, reader release,
   unchanged replacement account/cache, and no additional refresh/replay. Use the
   existing real composition and injected adapter contract, preserving test budgets.
5. Preserve the previously approved diagnostic reset ordering: signed-out must be
   visible while account remains held, then release account, assert unauthenticated
   JSON and both successful terminals. Run once in Chromium with the same passive
   observation and full private trace on the fixed implementation. Its result can
   support the lifecycle hypothesis only together with the causal controls; no
   claim that it uniquely explains all historical browser errors.
6. Run meaningful mutation controls for the new ownership/epoch guards, changed
   component checks, and one three-browser targeted reset series. Obtain an
   additional fresh independent combined standard/security review of the fixes and
   integration. Retain every old failure and report the exact tested tree.
7. If all criteria and review findings are resolved, run one new full acceptance
   E2E, then the existing exact-tree merge gate at the unchanged 260-second limit.
   Any unexpected failing assertion stops; no automatic repeats. Successful gate
   permits the usual authorized commit/merge/finish workflow and slice closure work.

## Acceptance and operational constraints

The old implementation fails the named lifecycle controls, the fixed implementation
passes them and sensitive negative controls; all existing assertions remain.
The selected native reset completes both exact responses and retains signed-out
state. The full E2E and exact-tree gate pass, and the reviewer confirms the new
ownership behavior and isolation. Source changes and evidence are separately
identified; a passing diagnostic is not a substituted full acceptance result.

Heavy checks remain sequential after checking other sessions and device load, with
owned temporary sleep protection and continuous monitoring; at most three working
sessions overall. Keep private traces outside served roots, mode 0700, at most seven
days. Export only fixed categories and counters. The existing observer's failed
detach after timeout remains explicit cleanup evidence, not a passing cleanup claim.
Long slice measurements are not started by this follow-up approval alone.
