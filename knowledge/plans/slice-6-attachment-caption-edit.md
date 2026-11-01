# Editing an attachment caption to an empty body

Status: approved; independent security planning review completed with two minor documentation corrections incorporated; owner approved implementation on 2026-11-01. Implementation is authorized within this plan. Baseline inspected: `ecd74d8e165b54f5cde41f45884a48cb122ea8b2`, 2026-11-01; the upload change is merged and its full gate and finish passed.

## Problem and outcome

The slice backend review's minor empty attachment-caption finding is present in `backend/internal/services/chat/messages.go`: Edit calls `validBody(body, 0)` even when its immutable attachment set is non-empty. The Messages contract permits an empty or whitespace-only body when an attachment remains and editing changes no attachments, but the API PATCH table currently says 1–4,000 characters. Approval explicitly resolves this discrepancy in favor of attachment-aware empty captions and updates both specifications. The request schema has no minimum-length constraint; no generated schema relaxation is needed. The frontend already includes existing attachment IDs in its empty-body validation. Correct the backend to apply that existing contract after authorization, writability, edit-window and version checks under the existing locks.

Tier: standard; security trigger assumed met because untrusted message validation and attachment ownership are involved (T-13/T-14/T-21). A separate security planning review precedes owner approval. Fresh standard and full security readiness reviews follow implementation, with an exact-tree gate and finish. This is a separate coherent change from durable upload cleanup.

## Implementation and files

Use the already loaded message scope's attachment presence when validating the edit after its channel/message locks. The scope is read before those locks: safety depends on immutable attachments of a live message and on refusing a message/channel deleted or erased while the request waits, not on describing the scope as a locked snapshot. Preserve the 4,000-code-point bound, forbidden controls, the refusal precedence, rate accounting, version increment, edit timestamp, safe diagnostics and authorization. Attachments are immutable during edit; deletion/erasure is serialized by the channel/message locks. No attachment rebinding, quota or download authorization change is intended.

Expected files: `backend/internal/services/chat/messages.go`; existing real API message regression family `backend/internal/api/chat_messages_test.go` or a small companion test; `knowledge/specs/realtime-and-video.md` and `knowledge/specs/api.md` to make applicability to editing explicit and resolve the PATCH discrepancy; a narrow `docs/threat-model.md` update marked pending owner review; a verification record and the slice finding disposition. The threat update records preserved author/membership checks, text bounds, deletion and attachment-serving authorization. Any broader contract change returns to approval.

## Acceptance and checks

A named real API test uploads a legal attachment to local S3, sends a captioned message, edits it to an empty body and to whitespace, and observes successful versions and edit timestamps. Its existing attachment identifier, live reference, quota charge and downloadable bytes remain unchanged. Editing a message without attachments to empty/whitespace stays refused. Oversized text and forbidden controls stay refused even when an attachment exists; stale version, another author, read-only, deleted/erased, deleted tutor/channel and edit-window refusals remain authoritative. Existing suites cover these rules and the new test must exercise the formerly failing path, not duplicate only pure validation logic.

Run affected backend race/static and generation checks and the relevant production-browser attachment/edit journey; other full-run checks are deferred to the merge gate. Independent reviewers explicitly confirm closure of the finding and authorization/validation preservation. The implementation records exact commands, failures and final tested revision. No assertion, timeout, rate cap or gate budget is weakened.

## Risks and observability

Using attachment state before stabilization could admit a truly empty message after a race; verify the existing channel/message lock protection and immutable-attachment premise rather than inferring it from the UI. Existing `chat.message.edited` remains the sole business outcome record and contains no message body or filename. Add regression assertions on resulting state and safe diagnostics where the existing family provides capture. A passing unit helper alone is insufficient evidence.

## Security planning review

The independent reviewer inspected the clean upload implementation revision above and found no blocker/major. Both minor findings are incorporated: the explicit API PATCH discrepancy is part of the approval scope, and the threat model is updated in the same change. The reviewer verified the frontend attachment-aware validation and the backend refusal/rate ordering. Relevant threats are T-14 (untrusted text), T-5/T-21 (author/membership), T-20 (rate/size), T-18 (safe diagnostics) and T-34 (erasure/reference lifecycle). T-13 upload format/content inspection is unchanged. Runtime checks were not performed during planning.
