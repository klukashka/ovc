# End-to-end readiness and cancelled writes

Status: implemented on 2026-11-07, approved by the owner as a narrow fix. Tier: standard. Security trigger: met through T-41, because it narrows what the end-to-end readiness observer treats as a failure. It changes no route, control surface or application code.

## Failure

A merge gate failed on `session.spec.ts:52` in Chromium: the 5-second readiness wait expired although the trace shows every request answered. The readiness state failed for the rest of the document on any failed request it tracks. A temporary diagnostic in a stress run identified `POST /api/v1/auth/refresh` failing with `net::ERR_ABORTED` during sign-in, while the phase was `checking`. A stress failure with this reason has the same trace as the gate failure. Why this request is cancelled is not established: refresh carries only a timeout signal, and an [earlier analysis](../analysis/reset-response-cancellation-2026-11-06.md) measured the same text with no application abort. The gate itself ran without readiness diagnostics, so the link to that one failure is strong but not proven.

## Change

A failure with one of the cancellation texts already listed in `frontend/e2e/request-failures.ts` no longer fails readiness when the request is not a read: it only leaves the pending set. A transport failure of any request and a cancelled `GET` stay fatal. The existing page diagnostics are unchanged. [The development guide](../../docs/development.md) and T-41 record this exception; its [accepted limitation](slice-7-known-limitations.md#end-to-end-readiness-and-cancelled-writes) covers unanswered cancellations on pages without watched diagnostics.

## Evidence

`frontend/scripts/navigation-readiness.test.ts` proves the state logic. The mapping from a browser failure text to that state is covered only by the stress run and the shared exact-text tests of `frontend/e2e/request-failures.test.ts`. Repeating the test 400 times with 12 workers before the fix gave `request_failed` once in each of two runs, with five and three failed tests. With the fix it gave no `request_failed` and seven failed tests, all with other reasons. The cancellation occurs rarely, so one clean stress run is supporting evidence, not proof. These runs were local, and their logs were not kept in the repository. [Remaining failure kinds](slice-7-known-limitations.md#end-to-end-readiness-failures-seen-only-under-a-stress-run) stay unresolved.
