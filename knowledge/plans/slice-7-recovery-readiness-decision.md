# Meeting recovery storage: bounded readiness decision after diagnostics

Status: approved by the project owner on 2026-11-06. Target: the unchanged application and tests of `/private/tmp/ovc-slice7-meeting-recovery-runtime`, branch `feat/meeting-recovery-runtime`, base/main `8664fabbae992844c6be1aef986dcb5b30b9e508`. The sorted SHA-256 manifest of every backend file is `/private/tmp/ovc-slice7-recovery-application-fingerprint.json`, manifest SHA-256 `6023fc7974b2bf2cb4f4068d78828db60b4dd165e68d725f4b5f289d51a7fb21`. This is a one-change readiness decision, not permission to merge a failed gate. The owner approved this bounded verification transition on 2026-11-06.

## Evidence and unresolved limits

The persistence implementation and its negative controls passed targeted testing and independent standard/security review. Final vet/sqlc/lint passed; minor review fixes are resolved. One full backend run passed 2102 named tests/subtests before the normal-priority correction. The final ordinary run then passed 2101 and failed the unchanged presence shutdown test: one connected-account lease remained. Source identity and sleep observations were valid.

The owner approved one isolated diagnostic full backend run with four passive snapshots, identical application source, unchanged assertions, normal parallelism and unchanged 100-millisecond/one-second deadlines. That run passed all 2102 in 96.47 seconds. The blocked admission took 110.91 ms; shutdown took 20.04 ms; both accounts were non-pending afterwards; acquired pool connections were zero; no incomplete-shutdown event or outer context expiry was observed. The 21 monitor samples were safe and the source was unchanged. This proves that observed execution succeeded, not why the previous one failed. Diagnostic observation can perturb timing. No production fix or historical-cause claim is justified by non-reproduction.

Separately, earlier documentation checks passed, but the last run passed Markdown/links and lost its Chromium connection while Mermaid closed a page. The exact process error was `ConnectionClosedError` from `CdpPage.close`; there was no Markdown/link error. Its cause is unknown. No retry has occurred. Neither historical failure is erased or relabelled successful.

## Approved decision

Accept the unresolved historical presence and documentation-browser causes only to permit the following bounded transition for this meeting-storage change. The diagnostic pass is not substituted for acceptance. Production code, original test assertions, parallelism, timeouts, retry settings and the ordinary ten-minute full-run budget remain unchanged. No 260-second optimization is introduced.

1. Record the approved decision in ADR-0018 with a section scoped to this storage change and its base/application fingerprint; update boundary 9/T-41 in the threat model and the verification record. The text below is the approved normative scope. Mark wording pending owner review. Include the already approved diagnostic plan and safe outcome record; no diagnostic helper or observation calls enter the application branch.
2. Verify the backend tree against the fingerprint saved before this decision; the path set and every backend content hash must match, and only these decision/evidence documents may differ:
   - `docs/decisions/0018-targeted-checks-and-merge-gate.md`;
   - `docs/threat-model.md`;
   - `knowledge/plans/slice-7-recovery-readiness-decision.md`;
   - `knowledge/plans/slice-7-presence-shutdown-diagnostic.md`;
   - `knowledge/reviews/slice-7-presence-shutdown-diagnostic.md`;
   - `knowledge/reviews/slice-7-meeting-recovery-storage.md`.
   Persist the manifest digest and its reproducible format in ADR-0018 and the verification record: all 479 backend paths mapped to their content SHA-256, serialized as JSON with sorted keys, two-space indentation and one terminal newline; digest `6023fc7974b2bf2cb4f4068d78828db60b4dd165e68d725f4b5f289d51a7fb21`. Adding or removing a backend path also invalidates this decision. Reconfirm the standard minor correction and unchanged security-reviewed implementation. Obtain one fresh combined standard/security review of the decision/evidence additions; it does not restart the completed application review or waive findings. New blocker/major findings follow the ordinary policy.
3. After checking other sessions/load, perform exactly one complete documentation check under owned sleep/source monitoring, plus final formatting/diff and staged Gitleaks checks. This is the one allowed new documentation execution after the browser failure; no retry on failure.
4. If those checks and the decision review pass, create the ordinary single coherent commit and run exactly one ordinary full main-script merge gate on its exact integration tree, including unmodified full backend and E2E. The gate may run its own documentation check as part of that already specified full list. No failing command, skipped assertion, budget excess, missing tool or unknown diagnostic is accepted by this decision. If a new failure occurs, stop without an automatic repeat or merge.
5. Only if the full gate passes, perform ordinary ff-only merge and finish with exact tested/main tree equality. Preserve all historical failures and the diagnostic status. Then continue the approved local sequence until the server boundary.

No additional acceptance full-backend run outside the gate is requested. No failed-gate exception, future reusable waiver, remote deployment or reopening of slice 6 is authorized. The unresolved historical failure remains tracked for hardening; an uninstrumented successful gate certifies only its own exact run/tree.

## Approved normative scope

ADR-0018 section “One-change readiness decision for meeting recovery storage”:

> For the meeting recovery storage change based on `8664fabbae992844c6be1aef986dcb5b30b9e508`, with backend manifest SHA-256 `6023fc7974b2bf2cb4f4068d78828db60b4dd165e68d725f4b5f289d51a7fb21` covering the exact 479-path set and file contents, the project owner accepts the unresolved cause of one earlier presence-shutdown assertion and one documentation-browser connection loss only for a bounded readiness transition. One isolated diagnostic full-backend run passed all 2102 named cases without changing application code, assertions or deadlines; it is diagnostic evidence, not acceptance or a demonstrated fix. One new complete documentation check and one ordinary full gate are permitted after the decision/evidence review and load checks. Merge and finish require the gate to pass every normal check on the exact final tree. Any new failed check stops integration without an automatic repeat. No failed gate may be merged under this decision, and no later change inherits it. All previous failed results remain recorded.

Threat model boundary 9/T-41: link that exact decision; distinguish observed success, accepted historical uncertainty and normal exact-tree acceptance. No security guarantee is claimed from non-reproduction; no unsafe output or secret is retained in documentation.

Tier: standard, security trigger T-41 verification policy and T-18/T-19 evidence. Security planning review passed before approval; its fingerprint and permitted-document clarification is incorporated. At most three working sessions total, one heavy workload at a time; raw diagnostic outputs stay private outside repository and served directories. Safe counts/times/test identities are durable evidence. No application edits are part of this plan.

## Later deadline change

Later change, recorded 2026-11-07 by the slice 7 slice review: commit `3023459` ("Remove load and race assumptions from presence and capacity tests"), approved by the project owner on 2026-11-07, raised the presence-operation deadline of six presence tests from 100 ms to 2 s: `TestPendingPresenceAccountsHaveATotalCapacity`, `TestPresenceOperationsRespectAdmissionAndShutdownDeadlines`, `TestReadyFailsWhenPresenceCannotCommit`, `TestAFailedPresenceReleaseRemainsPending`, `TestShutdownWithPendingPresenceReportsIncompleteRelease` and `TestChatPresenceFailureRecordsCarryNoBodiesNamesOrAddresses`. The deadline also applied to the connection each test makes during setup, so on a busy machine the socket closed with 1012 before the test began. The admission bound of `TestPresenceOperationsRespectAdmissionAndShutdownDeadlines` scales with it, from 1 s to twice the 2-second deadline. The one-second outer shutdown deadline is unchanged. This change did not establish the cause of the earlier retained-lease failure described here.
