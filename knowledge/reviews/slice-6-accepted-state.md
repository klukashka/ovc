# Slice 6 accepted state and transition

Decision date: 2026-11-05. The owner accepts the current state and directs work to proceed to slice 7 instead of continuing the slice 6 closure investigation. This is an explicit exception to the completion sequence in the [closure plan](../plans/slice-6-closure.md), not a claim that its remaining checks passed. The accepted implementation baseline is main `e201e16f7dcc3382af0ebb19487f07f1a57f3048`. No deployment approval is implied.

## Delivered baseline

The main branch contains chat and file functionality and the subsequent integrated fixes: direct/group Messages, attachments, unread/read state, presence/typing, digest, quotas and durable daily upload accounting, deletion/erasure and download lifecycle changes, and frontend session/document lifecycle corrections. Specifications remain the behavioral contract; this record does not certify every path anew. Detailed change evidence remains in the slice 6 review records and Git history.

Application change `a3b7d60` passed all 30 gate commands and 979 E2E cases; its 263.6-second duration received a one-time owner exception, recorded by main `e201e16`. That exception does not apply to future changes. The entry bundle plus largest language is 176.4 KB gzip, accepted separately while retaining the 180 KB limit.

## What remains unresolved

| Item | Actual evidence and impact | Disposition |
| --- | --- | --- |
| Intermittent Chromium password-reset E2E | Full runs reported 978/979: Playwright could not obtain the account 401 body, with native ERR_ABORTED. In one instrumented reproduction the application read 124 bytes through EOF, released the reader and did not abort or cancel. This does not establish the browser/DevTools failure cause or prove a production session defect. | Accepted unresolved for transition; track in slice 7 verification and resolve before final deployment hardening in slice 10. Do not weaken JSON/terminal assertions or automatically repeat failures. |
| Missing full-suite reproduction history | Four route/body controls and six streaming/tracing conditions passed. Latest bounded signup group passed 19/19 with six workers. Reset ran in worker 4 after cases 5,8,11,14 with the same browser/deployment; historical failing contexts used slot 0, with no complete predecessor history. | Preserve evidence; do not infer a culprit from list order or treat subset passes as a repaired full suite. |
| Measurement candidate not merged | Clean branch `chore/slice6-backend-measurement-integration`, commit `3ab9e43574c77c7642de33b52f2ee471f63779f2`, failed gate: 29 commands passed, E2E 978/979, wall time 272.5 seconds. | Remains outside main. Acceptance does not merge it. Reassess relevance before separate integration; preserve branch. |
| Full performance/resource acceptance | Smoke checks exist, but the final complete workload matrix, E7/E8 reconciliation and three cold full gates are not established on the final baseline. | Deferred to slice 10 hardening. Reuse and validate relevant tooling then; do not equate smoke with capacity or leak certification. |
| Final cumulative review closure | Individual reviews and fixes exist; a reconciled final all-layer verdict for the frozen final tree is not established. | Accepted as incomplete for transition; reconcile touched boundaries during slice 7 and the remaining cumulative evidence before deployment in slice 10. |
| Check-duration budget | Historical full gates exceeded 260 seconds; the general cause and an enduring improvement are not established. | Optimization paused by owner. No new threshold or future gate exception is created here; choose a sustainable policy from later measurements. |
| Unmerged diagnostic and measurement drafts | Diagnostic worktrees and earlier measurement/frontend drafts contain evidence and work not represented by main. | Preserve without automatic merge, deletion, history rewriting or silent inclusion in slice 7. |

## Latest bounded experiment

One authorized Chromium invocation selected and executed all 19 signup cases with six workers, retries 0, first-failure stop and ordinary retain-on-failure tracing. All passed. Both account and refresh 401 requests had successful native and Playwright terminal events; original JSON and terminal assertions passed. All six deployments recorded stop completion. No owned API/preview listener or binary process remained; run lock absent. Three monitor samples were valid and the frozen 1044-file identity was unchanged. 114 scalar records stayed within the 4096 cap. No trace was retained because every case passed. This does not exercise the first-failure observer path at runtime.

The diagnostic reporter had two major source-review findings before runtime: automatic default-reporter insertion and misclassification of the normal first-failure notification. Both were fixed, static checks passed, and an owner-authorized fresh combined review confirmed closure before the single run. No diagnostic fix is a demonstrated correction of the historical browser failure.

## Improvement priorities

1. Restore deterministic full-suite password-reset evidence by isolating a measured failing context; stop building broader observation infrastructure without a concrete causal question.
2. Keep test failures, observer failures and infrastructure failures distinct; retain exact source identities and bounded private evidence.
3. Consolidate durable execution records so future work does not depend on temporary paths or conversation history.
4. Complete representative capacity, resource-release and cold-run measurements before deployment; optimize general fixture/workload costs only from measurements, preserving coverage.
5. Review current affected contracts and authorization boundaries on each future change; accepted uncertainty is not permission to suppress a new security finding.

## Evidence preservation and limits

Detailed latest artifacts remain locally in `/private/tmp/ovc-worker-history-observed-1` and `/private/tmp/ovc-worker-history-1`; diagnostic source is in branch `chore/slice6-worker-history-isolation` over `3ab9e435`. The frozen manifest SHA-256 is `04a85a7d20b7f37fd89225dae3e6f65cd96613f32da60480c2955f6be5a74284`. Earlier full-run body evidence is under `/private/tmp/ovc-body-lifecycle-diagnostic-1`; candidate gate evidence is under `/private/tmp/ovc-backend-measurement-gate-1`. These temporary paths are locators, not durable evidence guarantees. This committed scalar summary preserves the decision and key observations without credentials, bodies, URLs or raw traces. Private raw retention remains no later than seven days after collection; none is copied into the repository.

Slice 6 is accepted for roadmap progression with these explicit limitations, not fully verified under the original closure definition. The next baseline is main, not the unmerged measurement candidate. Future changes retain normal planning, security review, change checks and exact-tree gate obligations. No further slice 6 diagnostic invocation is authorized by this transition.

## Transition documentation verification

Independent standard and full-security reviews of the transition documentation and proposed slice 7 outline found no blocker or major. Two minor findings were addressed: first-join timestamp wording and explicit threat/negative-check mapping for subsequent implementation plans. These reviews do not replace the deferred cumulative slice 6 review or a live BBB probe. Markdownlint and diff checks passed after the amendments; complete documentation checks and pinned-tool validation passed before the threat-mapping addition and are repeated on the final tree by the documentation gate. Secret scanning and exact-tree gate evidence are retained in the repository merge reports.
