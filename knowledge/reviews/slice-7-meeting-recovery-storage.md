# Meeting recovery storage verification

Date: 2026-11-06. Base: `8664fabbae992844c6be1aef986dcb5b30b9e508`. Status: implemented, not ready to merge. The [approved local plan](../plans/slice-7-local-completion.md) permits this persistence-first split. No coordinator, provider worker, operator command or public meeting route is activated by this change.

## Scope and review

Migration 25 adds global admission capacity, conditional settlement, durable finite cleanup-cycle receipts and a persistent compare-and-swap cursor. SQL, precise domain types, repository operations, generated mappings and real PostgreSQL tests implement those contracts. ADR-0020 explicitly amends ADR-0008 for retained minimal unresolved evidence.

Independent standard review found no blocker/major. Its two minor findings were fixed: restore ADR-0020 to the contiguous decision table, and use the existing normal outbox priority with an explicit regression assertion. The priority fix touches scheduling/resource ordering, not a new authority grant; full security review inspected the corrected final source and found no issues. These source reviews do not certify runtime or merge readiness.

## Executed evidence

| Check | Result |
| --- | --- |
| Five pinned tools: copied with SHA-256 equality and `tools/check-tool-binaries.sh` | Passed |
| Frozen offline frontend/docs dependencies | Passed using the existing named pnpm store; initial default-store docs install failed with missing offline tarball before checks |
| `sqlc generate`, compilation | Passed |
| Initial targeted `go test -race ./internal/repositories -run '^TestMeeting' -count=1` | Failed: old exact privacy-column list omitted the six approved technical fields; retained-column assertion updated explicitly |
| Corrected targeted run | Passed, 5.60 seconds; immutable source and safe monitor |
| `go vet ./...`, `sqlc diff`, golangci-lint | Passed on final source; earlier exhaustive-enum lint finding fixed without changing refusal behavior |
| First full `go test -race -json ./...` | Passed all 2102 named tests/subtests, 92.79 seconds, before the priority correction |
| Final full backend after priority correction | Failed: 2101 passed, one presence shutdown assertion failed; 74.28 seconds, source unchanged and monitor valid |
| Isolated negative control: remove exact settlement-generation equality | Expected failure at the unchanged returned-orphan generation assertion; 3.52 seconds, monitor valid |
| Isolated negative control: remove automatic-cycle eligibility | Expected failure in the unchanged finite-cycle transition assertions; 2.75 seconds, monitor valid |
| Documentation checks | Earlier corrected runs passed; latest run passed Markdown and all links but failed with Chromium `ConnectionClosedError` during Mermaid page cleanup. Cause unverified; no retry performed |
| Initial staged Gitleaks | Failed on a prose false positive in the BBB signing sentence; clarified the sentence without changing the cryptographic requirement or scanner configuration; corrected staged scan passed |
| Full gate, merge, finish | Pending; no readiness or exception claimed |

The final failed test is `TestPresenceOperationsRespectAdmissionAndShutdownDeadlines`, at `chat_presence_capacity_test.go:120`: one lease survived shutdown and online-until remained in the future. That source is unchanged. The failed test alone does not prove that the new storage caused it, and the earlier passing full run does not resolve the failure. The existing test's 100-millisecond presence deadline and one-second outer shutdown deadline are retained. Static inspection identifies early return from ExtendPresence after the first account error as a hypothesis, not demonstrated historical causality. A separate bounded diagnostic plan requires approval before another run; no automatic retry or inherited ledger exception applies.

Raw outputs and monitoring evidence remain private outside served directories and repository artifacts. This record retains only safe test names, counts, durations and outcomes. Remaining local stage-1 runtime and later API/UI/provider work remain approved but unimplemented; remote compatibility and media remain unverified.

## Approved diagnostic follow-up

The [single approved presence observation](slice-7-presence-shutdown-diagnostic.md) passed all 2102 named backend cases in 96.47 seconds with unchanged application code and original assertions/deadlines. Shutdown released both observed accounts in 20.04 ms. This non-reproduction leaves the previous failure cause unverified; it is not an acceptance substitute. No new gate, merge or documentation retry has occurred.

## Owner-approved bounded readiness transition

On 2026-11-06 the owner approved the [readiness decision](../plans/slice-7-recovery-readiness-decision.md), now recorded in [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#one-change-readiness-decision-for-meeting-recovery-storage). It permits one new documentation check and one ordinary full gate after the combined decision/evidence review. Merge/finish require complete gate success; any new failed check stops integration without an automatic repeat. Earlier failures remain unresolved and no failed-gate exception is granted.

The application fingerprint is SHA-256 `6023fc7974b2bf2cb4f4068d78828db60b4dd165e68d725f4b5f289d51a7fb21`: all 479 backend paths mapped to content SHA-256, serialized as JSON with sorted keys, two-space indentation and one terminal newline. Both the exact path set and every content digest were verified. No diagnostic helper or observation call enters that tree. The combined document review, one new documentation check, full gate and finish remain pending at this record's preparation; subsequent integration evidence must name their actual outcomes.

## Integration after tool dependency correction

The original decision/evidence combined review passed and its authorized new documentation check passed in 7.45 seconds. Storage commit `816aaf0709edbfc7e22a18ae2cb96a64a8579ded`, tree `786610bda42257bb0e0cc22575587423e0cb500c`, then failed its one authorized gate: tools OSV found GO-2026-6888 and GO-2026-6865. Thirteen commands passed, one failed and sixteen did not execute; backend race testing passed in 100.5 seconds, frontend/E2E did not run. Monitoring remained safe and source unchanged. That failed tree was not merged. Historical presence and browser causes remain unverified.

The owner approved the separate [compression dependency update and two-stage integration plan](../plans/slice-7-tools-compression-update.md). Its tools commit `509fd35e3b565daae92da6c0d686d60169660c33` passed all 30 gate commands and 979 E2E in 282.5 seconds; ordinary finish confirmed tested/main tree `c2fda1895d126a507759980d3d865d89f742c115`. Independent standard/security reviews found no issues. This is the integration base, not a pass for the storage tree.

The original storage branch and commit remain intact. A separate integration branch applies its squash onto that base. The only merge conflict was adjacent appended threat-model sections; both approved sections were retained. All 479 backend paths and contents exactly match the original application fingerprint above; the new tools graph is inherited unchanged from the verified base. No original application assertion, timeout, SQL, migration or production code was altered during integration.

The approved next steps are one scoped combined review of the integration/evidence, documentation and staged-secret checks, then one ordinary full exact-tree gate including backend and all E2E. These steps remain pending at record preparation; any new gate failure stops without retry or merge. The earlier gate does not certify this tree, and the existing presence uncertainty is not described as fixed.
