# Meeting recovery ledger: exact failed-gate disposition

Status: owner-approved on 2026-11-06 after independent security planning review. Tier standard; security trigger met by verification policy and boundary 9/T-41. The [ADR-0018 amendment](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#one-change-exception-for-the-meeting-recovery-ledger) governs this one exception.

## Exact scope and acceptance

The owner accepted the [recorded failed gate](../reviews/slice-7-meeting-recovery-ledger.md#owner-approved-exception-integration-2026-11-06): 29 commands passed and E2E passed 978/979; Chromium's password-reset body observation and four unknown HTTP/two console diagnostics remain failures with unverified cause and harmlessness. Integration was authorized only from `01c3f8e1e8b78e8316cf1e09718d7cf89e02e11a` to `be8290afed843561e1b02a6a93e96f54b7101cea`, with tree `98068b9b3d6bb26f4b0c573ed838ba7c8e741202`. No repeat or normal application finish is permitted as false certification. No deployment or slice completion is granted; future gate requirements remain unchanged.

## Integration and mandatory record

1. Refresh clean change head/tree and exact main base; refuse mismatches or a live preceding gate. Fast-forward only, then independently verify exact main head/tree. Preserve the failed evidence and remove the merged branch normally after detaching its worktree. These identity checks have completed; the application gate remains failed.
2. The owner explicitly permitted deferring the policy record to an immediately following separate documentation change. Amend only ADR-0018, threat model, the ledger verification record, slice 7 plan and this disposition plan. Mark changed human documentation pending owner review; preserve every command/exit and safe failure categories without private artifacts.
3. Obtain fresh independent standard and security implementation reviews; apply the ordinary after-fix rule. Run documentation checks and staged Gitleaks. Commit one coherent documentation change and run main's scratch copy of the ordinary docs-only gate on the exact integration tree. Merge only on success and run normal finish for this documentation change. Its successful result certifies only its documentation tree.
4. No further application implementation precedes completion of this mandatory record. Any new failure remains visible and blocks progress under normal policy; no automatic repeat, widened exception or weakened assertion is authorized.

## Coordination and limits

At most three working sessions total including other projects, with sequential reviews while another chat is active. Refresh actual load and other sessions before checks; no application runtime rerun is part of this documentation task. All raw traces remain private and outside served/repository directories. The future bounded recovery coordinator, admission/retention, operator handling, API/UI and live BBB acceptance remain separate work.
