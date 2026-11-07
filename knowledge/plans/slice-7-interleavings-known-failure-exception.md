# One-change disposition of the cleanup interleaving test gate

Status: owner-approved on 2026-11-06; exact exceptional integration completed. The mandatory documentation change merged as `57bf16d4be98b6e7e367747a2f35b3a2943d9366` through the merge gate (status line updated 2026-11-07 by the slice 7 slice review records). Standard tier; security trigger: verification policy boundary 9/T-41 and private diagnostic artifacts T-18/T-19.

## Evidence

Base main: `5d3bfe31965a61cd861c3a10704c06dce41ea41b`. Change `fix/meeting-cleanup-interleavings`: `05a79d2207c1573617f9ba4838ae7325b0f2280a`. Tested tree: `c9b54f8ca08bf2135702f85e001f4a96e57e7c5e`.

The change adds real shared-provider interleaving tests and review evidence; no production or frontend source changes. Targeted race checks, a sensitivity negative control, full backend checks and independent standard/security reviews passed. Full gate: 29 commands passed, E2E failed with 978/979 passed. The Chromium password-reset test exceeded 30000 ms; cleanup then reported closed page/context on unroute. Two unknown authentication HTTP diagnostics and one console diagnostic remain. The test identity matches earlier failures, but this is a timeout rather than the previous explicit DevTools response-body failure. Read-only inspection of the existing trace shows successful JSON and terminal assertions for the account response (signup.spec.ts:882–883), followed by no next assertion before timeout. Together with the code this points to the subsequent await of refresh.finished() at line 884; it does not establish why that terminal was missing. Common cause and harmlessness are not established. No assertion is waived implicitly by source scope. Gate 287.9 s is within the current ten-minute budget; monitoring was valid, 58 safe samples, source unchanged. At gate completion main remained the base and no rerun or merge had occurred; the subsequently authorized integration is recorded below.

Full command/exit evidence, monitoring and reviews are preserved in `.git/ovc-merge-reports/05a79d2-cleanup-interleavings-failed/`. Raw traces stay private and are not committed.

## Approved exact owner exception

Permit one fast-forward of this exact commit/tree without repeating the failed gate, accepting only the uncertainty described above. This does not mark the gate passed, authorize deployment or close the slice, and changes no future checks, assertions, deadlines or policy. No ordinary finish certifies this failed gate.

Before integration refresh both worktrees, branch head, base, tree, index and tracked changes. Reject any identity mismatch, unexpected tracked/staged edits or untracked path overlap. Preserve owner-owned `.agents/` and `.codex/`; never stash, reset or clean. Confirm the completed gate has released its lock and no owned runner remains. Run only `git merge --ff-only fix/meeting-cleanup-interleavings`, independently verify exact base-to-head advance and matching tree, and record the exception result. Detach the worktree then delete the merged branch with ordinary `git branch -d` only.

Explicitly permit deferring the durable policy record solely until the immediately following documentation change. No further application implementation starts until that change passes independent standard/security reviews and ordinary documentation gate/merge/finish.

## Mandatory documentation

One separate docs branch and commit updates `docs/decisions/0018-targeted-checks-and-merge-gate.md` and `docs/threat-model.md` boundary 9/T-41 (changed human-facing text pending owner review), `knowledge/reviews/slice-7-cleanup-interleavings.md`, `knowledge/reviews/slice-7-known-limitations.md`, `knowledge/plans/slice-7.md`, and persists this disposition in `knowledge/plans/`. Record exact identities, all command exits, timeout and unknown diagnostics, accepted uncertainty and no general waiver. Replace pending ordinary-integration wording in the known-limitations entry with the exact exceptional integration status and disposition link; retain its unverified BBB-specific limitation unchanged. Documentation checks, staged Gitleaks, fresh independent standard/security review, main-script docs-only gate and finish are mandatory. A new failure blocks further integration without automatic retry.

## Alternative

Leave this commit unmerged and prepare a separate narrow diagnostic/fix plan from the existing trace. No new runtime is authorized by this alternative until its plan is approved. At most three active sessions globally; heavy checks sequential. Read-only preparation may continue.

## Completed exceptional integration

The owner approved the bounded exception and immediate documentation sequence on 2026-11-06 after independent security planning review. Both clean tracked states and exact identities were refreshed; only the existing owner-owned `.agents/` and `.codex/` were untracked on main. The run lock was absent. Fast-forward integration advanced main from the recorded base to the exact change head; `main^{tree}` equals the tested tree. No gate was repeated and no ordinary finish ran. The merged branch was deleted normally after detaching its worktree. `.git/ovc-merge-reports/05a79d2-cleanup-interleavings-failed/exception-integration.json` preserves the scalar identity record. Mandatory documentation checks, independent reviews and ordinary documentation gate/finish are pending; no further application implementation is authorized before they pass.
