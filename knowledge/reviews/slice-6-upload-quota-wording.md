# Chat-upload quota wording verification

Status: implementation complete; independent standard and full-security source reviews passed; documentation checks and gate pending. Base: `5962f0466d56fbe33a541afb331fe679d73c969c`. The owner approved the documentation-only plan on 2026-11-01 after security planning review. Standard tier; security trigger met by resource admission and quota lifecycle (T-20/T-34).

The quotas paragraph now agrees with upload-flow item 7 and the existing implementation: admission rejects existing used plus pending at or above quota, completion checks final size excluding its own row, and binding is the exact atomic charged bound. The threat model states no stronger pre-streaming guarantee. Application code, schema, generated artifacts, tests, limits and dependencies are unchanged.

Source inspection covered `files.admitChatUpload`, `files.exactChecks`, `GetChatQuotaUse`, the message-binding charge path and the existing racing-completions scenario in `TestPendingUploadsCountTowardTheQuota`. The full gate on the unchanged application revision above passed all 30 commands and 944 E2E cases in 254.6 seconds; this is evidence for that revision, not a newly executed behavior test of this documentation change. The daily-byte accounting finding S6-SEC-02 is distinct and remains open under its own proposal; this change makes no claim to fixing it.

Expected change checks: Markdown/link/diagram validation, staged secret scanning and separate independent standard/full-security reviews against the complete diff. A Markdown-only exact-tree gate is eligible only if no test or script consumes the changed prose. Gate and finish evidence will be retained outside the tested tree. No executable checks are reported as independently rerun by a reviewer.

## Independent review results

A fresh standard reviewer and a separate fresh full-security reviewer inspected the complete four-document diff against the base above and reported no blocker, major or minor findings. Both traced admission, exact completion, quota SQL and transactional binding; the security reviewer also traced charge removal, pending reservations, safe quota diagnostics and the retained ADR-0019 cleanup responsibility. T-20 and T-34 were examined; unrelated authentication, token, rendering/CSP, conferencing, test-control, dependency and agent-policy implementations are unchanged. Neither reviewer ran runtime checks or represented existing test source as a new pass.

The standard reviewer confirmed Markdown-only gate eligibility: no executable consumer of the changed prose was found. Markdown validation itself is the intended gate consumer. Final classification remains standard with the security trigger met. No review finding required a fix. Actual documentation/scanner and gate/finish results remain pending.

## Change-check results

The offline frozen documentation install passed, exit 0, reusing the pinned local packages without downloads or lockfile changes. The changed-document markdownlint command and `tools/docs/check-docs.sh` passed, exit 0, including relative links/anchors and all six Mermaid documents. Logs are `/private/tmp/ovc-quota-docs-install.log` and `/private/tmp/ovc-quota-docs.log`. `git diff --check` passed. No new backend/browser behavior run is required for this prose-only correction; historical behavior evidence retains its stated revision. Staged secret scanning and the exact Markdown-only gate/finish are recorded separately; the report does not predeclare them successful.
