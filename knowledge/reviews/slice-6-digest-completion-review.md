# Message digest completion review

Reviewed implementation base: `6670bf3fe444c03b889035e1fca7eefb2409d170`.
Final integration base: `41bb7957dd0ffaf3c17c0ed6094b9a405c2814e5` (F1). The reviewed S6 source and specifications were copied byte-for-byte to this base without rewriting history. The only integration conflict was the threat-model append; both independent sections were retained. The prepared source branch remains preserved.
Status: change checks and independent readiness reviews passed; full merge gate pending.

## Scope and planning

The [approved completion plan](../plans/slice-6-digest-completion.md) is standard tier with the security trigger met. Independent security planning review identified lost pending work when new membership appears after channel enumeration and overlapping jobs not serialized by JobDue. The approved revision protocol addresses both. Implementation uses sequence-backed revisions so delete/recreate cannot produce an ABA match, and an existing-user key-share lock before channel locks so purge cannot be followed by orphan state creation. These preserve the approved guarantees without adding a new product behavior.

## Initial independent reviews

The fresh standard reviewer examined the complete tracked/untracked backend and documentation diff. Major: the approved real-code acceptance coverage lacked both remove/re-add race orders and queued digest dispatch after suspension/reclaim/keep. Minor: dispatch performed up to two SQL queries per payload channel and retained all labels before truncating to twenty. Initial verdict: failed, pending fixes and evidence.

A separate fresh security reviewer examined the complete diff. Major: an optional digest queued for an unconfirmed account could survive reclaim/keep and include old provisional messages after the account becomes active. Keep resets last_read_seq for visible history but advances digested_through_seq to exclude past digests; checking only current last_read_seq and the payload bounds missed this lifecycle boundary. Simply using current digested_through_seq at dispatch would suppress ordinary queued digests, so the fix must distinguish the reclaim boundary. This finding concerns T-21/T-44 and its fix touches a trust boundary. Minor: the required INFO digest.queued event with member/channel counts per run was missing. Initial verdict: failed, pending fixes and evidence.

The reviewers found no additional confirmed defects in pending revision ordering, conditional read clear, existing-user/purge ordering, safe email rendering or unsubscribe binding. This is static evidence only, not a passing runtime result. The after-fix review must use one fresh reviewer with the standard checklist for fixes and the full security profile, and confirm these findings closed.

## Execution before review fixes

- sqlc generation and git diff --check passed.
- Tool-binary verification passed; golangci-lint passed before the final predicate adjustments.
- Targeted digest API checks passed in 2.503 seconds; migration up/down/up passed in 1.668 seconds.
- Full go test -race ./... failed only in TestEveryPathReadsMembersThroughTheActivePredicate; API passed in 65.030 seconds and the other packages passed. The two new lock/write SQL paths were then made explicit active_channel_members consumers, added to the existing named lock/write registry and covered by TestDigestMemberWritesUseTheActivePredicate for active, suspended, discarded and erasing states. This fix is pending execution recheck. No test was disabled or loosened.
- Earlier compile attempts encountered a denied Go cache path and a startup TestMain missing sanctioned configuration when invoked without dev-env. A new purge assertion initially used UserID where a string was required; corrected before the successful targeted run. These attempts are not passing evidence.
- Raw full-race output: /private/tmp/ovc-s6-backend-race.log, retained locally.

## Pending

Run the remaining change checks (including the whole backend race suite after SQL changes), documentation checks and Gitleaks. Review fixes and the fresh after-fix review are complete as recorded below. Other full-run checks are deferred to the merge gate. Each gate and finish must identify the exact tested tree and command results. No slice-close claim follows from this task. Bounce intake remains deferred to slice 10.

## After-fix verification

One fresh independent reviewer applied the standard checklist to the fixes and the full security profile to the complete S6 diff: passed with no new blocker, major or minor findings. The real remove/re-add tests cover both pending-lock orders; queued provisional digest tests cover suspended, keep and drop, plus allowed new post-keep messages and the tutor's unchanged history. The student-only provisional-through dispatch floor closes the major lifecycle finding (T-21/T-44); this fix touches a trust boundary. Batched dispatch returns at most twenty labels and the total count in one SQL query, closing the query-fanout finding. The INFO digest.queued event counts only committed queued members/channels and has a safe-content test, closing the diagnostics finding. Explicit active-predicate lock/write SQL and the new behavioral registry test were examined as well.

Targeted post-fix command through the main checkout's sanctioned dev-env wrapper: go test ./internal/api ./internal/repositories with the digest, read-clear, pending-race and active-predicate test patterns, count=1. Exit 0: API 4.004 seconds, repositories 1.457 seconds. Output retained at /private/tmp/ovc-s6-afterfix-targeted.log. The reviewer independently ran git diff --check (exit 0), but no runtime checks; the targeted results are coordinator-owned evidence.

The full post-fix backend race run subsequently passed as recorded below; the merge gate remains pending. Normal at-least-once SMTP delivery and dispatch snapshot semantics remain: sending an external email is not atomic with later membership changes. Bounce intake remains a slice 10 capability.

## Final backend change checks

On integration base 41bb795, tool-binary verification passed. The sanctioned main dev-env wrapper ran go test -race ./... with exit 0: API 67.568 seconds, repositories 3.742 seconds and startup/migrations 13.056 seconds; all packages passed. Raw output is retained at /private/tmp/ovc-s6-final-backend-race.log. Then go vet ./..., golangci-lint run ./... (zero issues), sqlc diff and the empty gofmt -l assertion passed. No backend source changed afterward. Two documentation-check attempts found extra blank separators in this evolving record; corrected without changing checks. Full Markdown, link and diagram checks then passed. Staged Gitleaks and staged diff checks passed. The earlier architectural-test failure remains recorded above.
