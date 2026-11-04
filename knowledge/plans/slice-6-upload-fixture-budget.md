# Consolidate upload refusal fixtures without weakening coverage

Status: owner-approved on 2026-11-04 after independent security planning review. Standard tier,
security trigger for upload validation and verification evidence: T-13, T-20,
T-35, T-36 and T-41 in docs/threat-model.md (refusals, limits, cleanup, local
fixtures and effective verification). No production behavior change.

## Problem and scope

Main remains 4d9ba1e; candidate 0880789 and its failed 268.8-second gate are
preserved. Registration consolidation passed the complete backend in 86.128
seconds against 87.293 seconds originally. This single comparison does not
justify recovery of the 8.8-second gate deficit. A bounded Chromium diagnostic
passed and observed the account response drain through EOF, but the historical
ERR_ABORTED remains unexplained. This plan does not resolve that uncertainty.

Consolidate only three existing API tables: TestAvatarRefusesOversizeAndDisguisedFiles
(9 cases), TestADisguisedFileIsRefused (7), and
TestUploadBodiesWithExtraPartsOrOversizedHeadersAreRefused (8). Use one
parent-owned harness per table, keep parent parallelism and make children
sequential. Remove 21 duplicate harness lifecycles. Preserve all 24 names,
inputs, bytes, status/code/field assertions, exact attachment cardinality and
storage/multipart cleanup assertions. Keep assertLeftNothing inside each child,
not only at final parent cleanup. No timeout, concurrency setting outside
these tables, budget, retry, production or harness-internal change is authorized.

Avatar cases may share one tutor/session only after checking account-scoped
limits and verifying that the successful unbound upload cannot affect refusals.
Chat cases must have distinct tutor/student identities, relationships and groups,
with stable case-derived IDs and emails, one upload per student. Introduce a
narrow test helper instead of reusing activeStudentInAGroup's fixed IDs. Keep
existing helpers and their callers unchanged. Preserve child-owned assertions,
parent cleanup and Arrange-Act-Assert. Do not clear shared state between cases.

Files: backend/internal/api/avatars_test.go, chat_uploads_test.go and, only if
needed, a dedicated test helper file in the same package. Record plan/evidence
under knowledge/plans and knowledge/analysis. Existing account-contract and
files specifications remain unchanged; read them and their ADR references first.
Observability changes are limited to verification evidence; no new application
logging or exports. No credentials or raw trace data enter repository artifacts.

## Evidence and acceptance

1. Document a one-to-one mapping for all 24 cases and assertions. Verify per-case
   identity/rate/quota isolation and interaction with worker Tick and S3 cleanup.
2. Before changing source, run the three original tables once with real DB/S3.
   Reuse the just-passed full backend (86.128 seconds) as the baseline; do not
   repeat it for a favorable timing. Check tool provenance before every batch.
3. Run the changed tables once in normal order. In an isolated source copy,
   reverse only case traversal and run once; all original assertions must pass.
   Use deterministic case ordering with preserved names and no fixture reset.
4. In one isolated source copy, change only Service.reencode in
   backend/internal/services/files/files.go: replace
   `if !found || !extensionMatches(kind, filename)` with `if !found`.
   Run the three selected tables once. The avatar cases PNG named JPEG and WebP
   named PNG must fail their retained status assertions with 201 instead of 415;
   other cases must pass. The third successful pending avatar remains within
   existing limits. Setup, quota or cleanup failures do not count as proof.
5. Run gofmt, go vet, golangci-lint, complete backend race tests once, documentation
   checks and staged Gitleaks. No source changes during measured execution.
   Preserve every prior failure. Unexpected assertions stop that stream; no
   automatic repeat or alteration of assertions/limits.
6. Obtain fresh independent standard and security readiness reviews of the whole
   new delta and integration, following the normal after-fix rule. Review must
   confirm that all upload/privacy proof remains effective.

## Resource limits, risks and decision boundary

Only one heavy command at a time, at most three working sessions including other
chats, load check and owned sleep/display prevention with continuous monitoring.
Keep exact commands, source hashes, exit results, CPU/cache conditions and wall
clock measurements. Private controls stay outside served roots.

Serial upload work can erase fixture savings; shared cleanup can affect later
cases. Per-case identities and reversed traversal must detect contamination.
Twenty-one fewer fixtures is not a promised time saving. Compare full command
wall time, never add overlapping case durations. A narrow fixture-only change
may still be insufficient for the 260-second gate.

This approval authorizes implementation and the listed bounded checks only.
It does not authorize a new full acceptance E2E, gate, merge, long measurement
series or acceptance of the unresolved historical reset failure. After the
results, present a readiness decision covering both historical uncertainty and
budget evidence before any new gate. If savings remain insufficient, stop with
the measurements; do not repeat the same tree or raise the threshold.
