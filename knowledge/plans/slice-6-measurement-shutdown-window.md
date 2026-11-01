# Measurement peer shutdown observation window

Status: approved by the owner on 2026-11-01 after independent security planning review. Implementation and smoke verification of this narrow scope are authorized; full measurement series are not authorized. This narrowly supplements the [slice 6 closure plan](slice-6-closure.md).

## Problem and contracts

The upload-process smoke based on `baabaf179973fdfab1ee5f5fb6280a553219ec6d` failed after its cancellation case passed and while its shutdown case held an admitted streaming request open. Evidence is retained in `/private/tmp/ovc-measurements-cli-upload-process.log` and `.status`, with history in [the tooling verification record](../reviews/slice-6-measurement-tooling-verification.md). Its generic failure does not establish the exact shutdown outcome. Safe event-only inspection of the last owned peer log found process.started but no process.stopped; the fixture currently reuses the peer log path across cycles.

Source inspection identifies an incompatible observation window: `frontend/e2e/deployment.ts` allows twenty seconds after SIGTERM before forced termination, while `backend/cmd/api/main.go` gives HTTP shutdown forty seconds. The [file specification](../specs/files.md#upload-flow) requires a thirty-second stalled-body window and one detached thirty-second settlement budget, bounded by the earlier process-drain deadline. A held request can therefore legitimately remain in flight beyond the harness's twenty seconds. The existing backend process shutdown test releases its held server-side barrier immediately after SIGTERM and requires a normal exit below forty seconds; it does not prove behavior for an indefinitely held client stream.

This is a source-derived explanation to test, not a production bug verdict. Forty-five seconds would allow the existing forty-second drain plus five seconds for observation. It cannot make a nonzero exit, stalled cleanup or forced termination correct. The production drain may still fail or the process may still require more time; those outcomes remain failures requiring diagnosis, not another timeout increase.

## Desired result and scope

Provide one fixed measurement-only forty-five-second observation window for an owned API peer in the shutdown workload. Preserve the default end-to-end twenty-second stop deadline and five-second post-kill observation deadline, all production timeouts, admission limits, process clean-exit policy and workload counts. Do not release or cancel the held client body cooperatively before stop completes merely to make the scenario pass.

Tier: **standard**. The security trigger is met by owned process lifecycle, test-control fixtures, diagnostics and a verification timeout increase. A fresh security-profile planning review precedes the owner's multiple-choice approval. Implementation then requires the independent standard and full-security review obligations of the overall measurement change, including its pending combined after-fix review. Observability adds only fixed stage labels, numeric duration/exit outcome and bounded process termination classification; no raw errors, process arguments, environments, tokens or request content.

Planned files:

- `frontend/e2e/deployment.ts`: expose a named measurement-peer shutdown option with exactly the fixed forty-five-second window, available only on the opt-in measurement peer path; ordinary deployment and ordinary peer stop keep twenty seconds. Preserve strict clean-exit checks and the five-second kill observation. Use distinct owned peer log paths so cycle evidence is retained.
- `frontend/scripts/measure-slice6-upload-process.ts`: select that named option only for shutdown-in-flight cases; record stage and safe process outcome, retaining every failure and ensuring request cancellation, response observation and deployment cleanup are all attempted on failure.
- `docs/development.md`: document the option and its limited reason in the measurement section, pending owner review.
- `docs/threat-model.md`: update the measurement lifecycle mapping for T-18/T-19/T-20/T-36/T-41, pending owner review; no production threat control changes.
- `knowledge/reviews/slice-6-measurement-tooling-verification.md`: preserve earlier failures and record exact source scope, commands, outcomes, process cleanup and limitations.

No production code, application socket factory, API contract, dependency, agent policy or general check budget changes. Stop and seek a revised plan if implementation requires them.

## Acceptance criteria and work order

1. Inspect the current repository tree and cited contracts; implement the narrow option only after planning review and owner approval. A source diff must show the ordinary twenty-second stop and five-second kill windows unchanged and the measurement option fixed at forty-five seconds, without arbitrary caller-supplied durations.
2. Exercise the unchanged twenty-second path as an explicit negative control on the same held-body scenario, at most once and only after its termination is separately authorized under the existing owned-child cleanup policy. It must retain its failed classification when stop exceeds twenty seconds. The already retained original failure remains evidence, but its generic diagnostic alone does not establish a specific forced-termination verdict. No forced termination is ever counted as workload success. If the negative control cannot be safely authorized, record it blocked and do not claim this criterion passed.
3. Run the changed upload-process smoke once. It must observe the admitted streaming phase, preserve the held body during stop, accept only a normal permitted peer exit, reject a successful upload response, and pass the original durable intent/capacity/receipt/uploading cleanup assertions. After cancellation, awaiting the upload outcome has an explicit deadline within the existing cleanup observation budget; an unresolved outcome is a failed cleanup, never an unbounded await or a fabricated status. A failure retains its stage/outcome and stops execution; no unchanged assertion retry.
4. Prove option confinement through meaningful fixture checks: default callers retain the twenty-second deadline, the named opt-in path selects forty-five seconds, nonzero outcomes and forced termination remain rejected, and owned process/request cleanup still runs after an operation failure. All cleanup attempts run and aggregate their errors with the original operation error rather than replacing it. The negative control verifies failure sensitivity rather than weakening it.
5. Run applicable change checks and complete the independent reviews. Frontend typecheck, lint, formatting, tests, production build, class/bundle/API checks and the complete default browser suite apply because the shared deployment fixture changes; preserve tagged backend verification and full backend checks required by the wider change. Documentation checks and Gitleaks cover the final documentation. Exact-tree merge-gate obligations remain unchanged; no commit or merge is part of this proposal's execution authorization.

## Runtime cost and limits

Keep one hundred shutdown cycles per full sample and three samples, alongside one hundred cancellation cycles per sample. The thirty-second stall window suggests about fifty minutes of shutdown waiting per sample if each held request reaches it; the fixed forty-five-second observation ceiling allows up to seventy-five minutes per sample before setup, cleanup and cancellation work. Three full samples therefore imply roughly 150 to 225 minutes of shutdown observation alone. These are source-derived planning estimates, not measurements or guaranteed bounds on the whole command. Process setup, settlement, storage cleanup and diagnostics add time; a failed first cycle stops rather than spending the remaining count.

The coordinator must reserve an otherwise idle machine for any full measurement and present this cost before scheduling it. Do not silently reduce one hundred cycles, increase any other budget or represent smoke as full lifecycle evidence. This proposal authorizes no full measurement by itself. If the preserved contracts cannot yield a normal shutdown within the fixed observation window, record the failed observation and request a separate decision; do not accept nonzero exits or extend the window again.

## Planning review

An independent security-profile planning review found no blocker or major. Its minor lifecycle clarification requires bounded observation of the cancelled upload outcome and preservation of the operation error together with all cleanup errors; criteria 3 and 4 incorporate it. This planning verdict is not runtime, implementation-readiness or full-tooling review evidence.
