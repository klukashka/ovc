# Explicit recovery from a stale message edit

Status: approved by the owner on 2026-11-01 after independent security planning review; one minor lifetime clarification is incorporated. Implementation is authorized within this plan. Baseline inspected: `ecd74d8e165b54f5cde41f45884a48cb122ea8b2`, 2026-11-01.

## Problem and intended behavior

The frontend slice review identified a minor recovery defect. Conversation submit handles `conflict` by refetching history but retains the original editing message version. Save therefore repeats the rejected version indefinitely; cancelling restores the earlier new-message draft and discards the current edit. The existing test proves the initial refusal and retained text, not successful continuation.

Keep the unsaved edit visible and introduce an explicit choice after a successful fresh read of that same message: show its current server text alongside the retained editor, with **Use latest text** and **Keep my text**. Neither action sends a PATCH. Both deliberately adopt the displayed message version; the first also replaces the editor text, the second keeps it. Focus moves to the composer. The subsequent explicit Save uses that version; another concurrent update produces a fresh conflict again. Normal Save must not silently adopt a refetched version while conflict resolution is pending.

If refresh fails, retain the user's edit and offer the existing retry behavior; do not enable resolution with stale data. If the message is deleted, erased, no longer owned/editable, or its channel becomes unavailable/read-only, preserve the existing refusal and focus rules and offer no resolution that can restore permissions. Cancellation keeps the existing documented new-message draft behavior. New texts ship in both English and Russian route chunks.

## Scope, contracts and risks

Standard tier; security trigger met by additional rendering/lifetime of message content and stale response handling (T-14/T-18/T-20/T-21/T-29/T-44). No backend authorization or version semantics change. The server remains authoritative. Use the existing text renderer and account/channel lifetime guards; never restore removed history or preview from a late response, and clear conflict snapshots with existing channel/account invalidation. Guard each asynchronous resolution with an edit-generation identity as well as account/channel lifetime and message identity. Cancel, starting a different edit, and cancelling then reopening the same message invalidate the previous edit generation. A late conflict refresh must not change the new editor, its version, snapshot or focus. The conflict snapshot is bounded to one message and does not add polling or a second permanent cache.

Expected files: `frontend/src/features/messages/Conversation.tsx`, its route texts and shared precise types if required, the Messages component tests, the existing production browser Messages suite, `knowledge/design/web/pages.md` (Messages conflict and focus behavior), `docs/threat-model.md` with pending-owner-review wording, and a verification record/finding disposition. No new dependency or API route is needed. Any broader contract or interaction change returns to approval.

## Acceptance criteria

A real configured-adapter component test edits version 1 while a second client publishes version 2, observes the refusal and fresh current text, retains the local edit, chooses Keep my text, and successfully saves version 3 without an automatic overwrite. A separate path chooses Use latest text, verifies the text and focused composer, and cancels/saves under the existing semantics. Refetch failure cannot enable adoption of a stale version; a further concurrent change after the choice causes another conflict. Existing guards and tests must prove no content revival after account/channel change, deletion or access loss. A held-refresh regression covers conflict A → Cancel → Edit B → late A completion and Cancel → re-edit A; neither old result modifies the new editor or shows old conflict controls. Tests assert each specified focus move and retain the existing preserved-draft regression.

A production-build browser continuation scenario uses two real authenticated API clients to create the conflict, resolves it through the UI and confirms the stored result and focus. Run across supported browser projects, with translated texts covered through existing locale checks. Relevant type/lint/format/component/build/client checks, independent standard and security reviews and an exact-tree merge gate are mandatory. No retry, timeout increase or weakened diagnostic check is allowed.

## Order and observability

Review this plan, obtain owner approval, implement the bounded conflict state and explicit actions, add real-path continuation checks, update contracts/threat mapping, run affected checks, obtain independent readiness reviews and apply the after-fix rule, then merge through a separate gate. Existing safe request/error diagnostics suffice; no message text, versions paired with personal data or draft contents enter logs. Runtime performance remains part of the cumulative slice measurements.

## Security planning review

A fresh independent security reviewer found no blocker/major. Its one minor requirement, an edit-generation guard and late-response regressions for same-channel cancellation/reopening, is incorporated above. The proposed explicit version adoption, safe renderer, bounded snapshot and separate Save preserve server authority. Runtime evidence remains pending implementation.
