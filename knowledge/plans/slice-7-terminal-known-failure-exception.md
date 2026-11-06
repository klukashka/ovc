# One-change disposition of the meeting cleanup terminal-persistence gate

Status: approved by the project owner on 2026-11-06. Base main `658be0514fca80c5264cf9981547003fc1a4fa71`; change `d5961bfcd7a31c8070debb52c97498ca8d93d21d`; exact tested tree `ade24b9e79e03eda11b52762cac6648fe42dcfb4`.

## Evidence and remaining uncertainty

The approved terminal-persistence implementation passed targeted and full backend race checks, static/generated/documentation/secret checks, five sensitivity controls and independent standard/security reviews. The full gate passed 29 commands; E2E passed 978/979. The one Chromium password-reset case failed at signup.spec.ts:882 while retrieving a 401 body through response.json, with Network.getResponseBody reporting no resource data. It also produced four unknown authentication HTTP diagnostics and two console diagnostics. All remain failures; the primary category and test match prior history, but a common cause and harmlessness are not proven.

Gate wall time was 289.9 seconds; the monitoring wrapper observed 290.88 seconds, 59 safe samples and unchanged source. The existing general ten-minute budget was not exceeded. No rerun, merge or finish has occurred. The immediately preceding tools and storage gates both passed all 979 E2E on unchanged frontend source, which establishes earlier successful executions of that frontend test, not flakiness or the cause on the current application tree. This diff changes shared outbox integration only for meeting cleanup; it activates no provider lane, creation coordinator, public meeting route or UI. No claim of harmlessness follows from that scope. The full safe report is `.git/ovc-merge-reports/d5961bf-meeting-terminal-failed/gate.log`; raw traces remain local/private.

## Proposed exact exception

Ask the owner to accept the stated uncertainty for this exact commit/tree only and permit a single fast-forward from the exact base without rerunning the failed gate. This is a new explicit exception to AGENTS/ADR-0018, not reuse of any prior foundation or ledger exception. The failed gate remains failed; normal finish must not be invoked to certify a failed gate. It grants no deployment, remote BBB acceptance or slice completion, and changes no test, diagnostic, timeout or future gate requirement.

Immediately before integration refresh branch/main/tree/status and verify no tracked changes or unexpected files in the change worktree. Refuse if head/base/tree differs from the identities above. Inspect the destination main index and worktree as well: refuse unexpected tracked/staged modifications or untracked path overlap, preserve the existing owner-owned untracked .agents/ and .codex/ directories untouched, and never stash, discard or clean them. Check that the completed failed gate holds no live lock/process; do not forge or alter gate state. Perform only `git merge --ff-only feat/meeting-recovery-execution` on main, then independently verify main advanced exactly base-to-head and its tree equals the tested tree. Preserve the exception report and remove the merged branch with ordinary branch deletion only after detaching its worktree. Do not claim successful normal gate/finish.

The owner must explicitly also authorize deferring the policy record to an immediately following mandatory documentation change, mirroring the bounded sequencing of the previous local-foundation disposition. No further application implementation begins until that documentation change passes its own independent standard and security reviews, ordinary docs-only gate, merge and finish.

## Mandatory documentation change

In a separate docs branch/commit, amend `docs/decisions/0018-targeted-checks-and-merge-gate.md` with the exact new identities, failure counts and bounded exception; amend `docs/threat-model.md` boundary 9/T-41 with the accepted verification uncertainty and T-18/T-19 safe artifact handling, marking new wording pending owner review. Update `knowledge/reviews/slice-7-meeting-terminal-persistence.md` with every command/exit and exact pre/post identity evidence; update `knowledge/plans/slice-7.md` and persist this approved disposition plan. No other file is needed.

Tier standard, security trigger met because this is a verification-policy exception. Independent full-security planning review precedes owner approval. The later documentation diff receives fresh independent standard and security implementation reviews, documentation checks, staged Gitleaks and an ordinary main-script docs-only gate/finish. Its success certifies only that documentation tree. If any new blocker occurs, retain evidence and stop without automatic repeat or broader authority.

## Alternative and coordination

The owner can instead leave d5961bf unmerged and request a narrowly scoped diagnostic plan for the test. Do not restart the earlier broad slice 6 investigation or weaken the observation assertion without a separately approved plan. At most three active sessions total; no runtime is needed to prepare this disposition. All future heavy checks remain sequential with fresh load/session checks.
