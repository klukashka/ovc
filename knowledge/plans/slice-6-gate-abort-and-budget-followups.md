# Diagnose the reset abort and consolidate registration fixtures

Status: independent security planning review completed without blockers; owner-approved on 2026-11-04. Standard tier with security trigger
T-18/T-19/T-20/T-21/T-29/T-41. Two independent workstreams; heavy checks serial.

## Preserved state and blockers

Candidate 0880789c7482d1dfa6babbb893726ece5ded171b is not merged. Main remains
4d9ba1e5467c84b1a09c1737c00ea2c8e4c24271. Gate tree
3702707df4e3775414f2f28ef10efeb487b656d3 failed: 29 commands passed, E2E had
978 passed and one failed; wall time 268.8 seconds exceeds 260. Both failures
remain blockers. The separate earlier full E2E passed 979/979. No result replaces
another and no unchanged gate retry is proposed.

The failed Chromium reset observed account401 followed by net::ERR_ABORTED and
an unavailable body in Network.getResponseBody. Its short interval does not prove
the 15-second signal expired. Existing traces do not establish whether the abort
preceded body lookup failure and teardown or why it happened.

## A. One isolated owner-lifecycle diagnostic

Add passive, bounded observation only in an isolated source copy of 0880789.
The copy's Session constructor may receive an optional typed observer; wire it only
in that copy. Record loadMe request start, response receipt, stale/current epoch,
disposal entry, read completion/EOF/error, cancellation start/settlement, reader
release and signal abort. Export no body, URL, headers, token, account identifier,
raw error or source preview. Local operation labels, fixed stages, elapsed times,
byte counts and boolean are sufficient. Observer callbacks must be synchronous and non-throwing and must not change
application control flow. Observation errors mark the diagnostic incomplete separately;
do not retain Response objects, chunks or exception objects. Collect at most 128 records; overflow or unmatched identity
invalidates completeness rather than discarding inconvenient events.

Retain the existing passive Chromium Network observer and record the exact reset
control's body-lookup start/failure and cleanup start. Correlate internally by exact
request identity/method/endpoint; exported labels remain local numbers. Different
clock domains and event delivery order are not interchangeable: do not claim a
cross-source causal order unless alignment is established. Preserve existing limits
on CDP frame attribution and lack of authentication-event visibility.

Remove every owned listener and preserve cleanup failures. Keep production behavior,
all JSON/status/terminal/UI assertions and existing timeouts unchanged. Instrumentation
may affect timing; a passing run does not explain the historical failure. Run exactly
one Chromium reset diagnostic after static checks and exact test-list verification,
on an otherwise available device with owned sleep protection and monitoring. Private
traces stay outside served roots, mode 0700, at most seven days; only allowlisted
records are exported. Do not run another acceptance E2E or gate from this permission.
If the cause remains unknown, report it as unknown and seek a readiness decision;
no browser-error allowance or additional production fix is implied by this plan.

## B. Registration fixture consolidation

First record why the change's new browser controls cannot be mechanically merged:
normal versus duplicate account-switch outcomes, GET405 versus POST401 ownership,
and closest signup prefix mergers preserve different actions and intermediate
assertions. Do not remove their browser evidence or silently introduce an AAA exception.

Change only backend/internal/api/account_flows_test.go in a new integration branch.
Keep all nine named TestRegisterRefusesInvalidInput cases and all six named
TestRegisterStoresUTCForAZoneNameThatIsNotCanonical cases and their assertions.
Use one parent-owned newFlowHarness per table; subtests become sequential, parents
may remain parallel. Each timezone case must use a distinct account email stably derived from its case,
and repository lookup must use that same email, so duplicate registration cannot
accidentally validate a previous row. Keep the tables separate,
and verify rate-limit isolation and order independence. Remove only 13 duplicate
fixture lifecycles. Do not change production, harness internals, pool sizes, clocks,
timeouts, concurrency settings outside these tables, retries or the 260-second limit.

Record one targeted timing and one full-backend timing on original code, then one
corresponding execution on the consolidated code, with real DB/S3 fixtures and unchanged
conditions. The changed full-backend execution also serves as its required test check.
Compare wall time of the backend command, not summed subtest or fixture times; standalone
measurements still do not guarantee the same gain under concurrent gate lanes. Timing is evidence, not an assertion
that must be rerun until favorable. A canonicalization negative control must fail
at the corresponding timezone assertions, never only at a rate-limit or fixture failure; demonstrate all original named cases map
to the preserved assertions. Run required backend checks and an additional fresh
independent combined standard/security review of the consolidation and its integration.
Savings are not yet measured and may be insufficient for the 8.8-second deficit.
If evidence does not justify sufficient savings, prepare a broader plan before a gate.

Preserve 0880789 and its branch/history. After approval, create a new branch from main
and squash the candidate into it, then apply the approved consolidation; do not amend
or rebase the old commit. Record the failed gate and decisions under knowledge in the
new change. Do not merge or run a new gate while the reset failure is unresolved.
A new implementation scope or acceptance/gate attempt requires a separate decision
based on the diagnostic and consolidation results. Long slice measurements remain
outside this approval. At most three working sessions overall; no parallel heavy runs.
