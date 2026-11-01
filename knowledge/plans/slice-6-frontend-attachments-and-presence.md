# Chat attachments and presence implementation

Status: owner approved on 2026-10-31. Base: `29fc9090ea3c9ff92b0be15fc45e2289a04d09b8`.

## Outcome and scope

Complete F3 of the [slice plan](slice-6.md): authenticated chat uploads, previews and downloads, presence and typing, file storage usage, and the remaining chat/digest pilot acceptance. Existing server contracts remain authoritative. No new dependencies or verification-policy changes are planned.

Verification tier: **standard**, with the security trigger met by uploads, signed addresses, session ownership and realtime privacy. An independent security planning review examined the current Session, typed client, multipart flow, channel projection, realtime wiring and file/realtime/page contracts; no blocking findings remain. Implementation checks and readiness reviews are pending.

## Implementation order and files

1. Extend `frontend/src/lib/api/transport.ts` and `session.ts` with an explicit typed upload-progress dependency while preserving the existing authenticated typed API path. Use browser XMLHttpRequest only for uploads needing real progress; preserve same-origin checks, response watching, one token-expiry refresh/replay, AbortSignal and account epoch. Cancellation during refresh prevents replay. Never automatically replay an upload after an uncertain network outcome. Bound retained file bodies and release handlers/references; inspect multipart serialization and Request cloning memory with four 25 MB files.
2. Add upload queue/composer and attachment display modules under `frontend/src/features/messages/`; integrate `Conversation.tsx`, `api.ts` and lazy RU/EN texts. Pick/drop/paste uses at most ten files and four active uploads. Inline refusals take focus. Unconfirmed accounts see the confirmation prompt. All attach paths obey the F1 offline guard and writability. Account/channel loss cancels work and rejects stale results. Sending supports attachment-only messages and snapshots both body and attachment identifiers under an idempotency key; editing preserves existing attachments.
3. Render filenames as text with isolated direction, reserve preview dimensions, and use only authorized same-origin file addresses. Coalesce history refresh before expiry and once after picture failure. The failure budget belongs to the attachment and conversation epoch, not its URL or a refetched object. Identical expiry, stale responses and REST refusals cannot create immediate retry loops. Hidden/unmounted pages release timers; fallback remains usable without repeated background retries. Preserve F2 pagination, cache eviction, focus and read-marker behavior.
4. Extend `frontend/src/lib/realtime/`, `frontend/src/app/realtime.ts` and message presence/typing components. Use the existing single socket, ready-only typing at most once per two seconds, and a five-second indicator with reduced-motion support. Render only the fresh authorized channel projection: tutor group member labels/presence, student tutor presence and anonymous other-student typing. Never infer anonymous identity or derive a roster from messages. Bound transient state and clear it on account/scope/disconnect/page lifecycle changes; unknown identifiers do not cause per-frame REST work.
5. Add `GET /tutor/settings` file usage/quota rendering to `TeachingSettingsPage.tsx`. Complete component and Playwright coverage, then documentation and readiness reviews.

## Checkable acceptance

- The seven F3 Playwright cases named in the parent plan pass on every configured browser: pilot 3 steps 6, 7, 8–11 and 14–15; inline file type/quota refusals and confirmation prompt; named document download; teaching storage usage.
- Component/state tests prove four-upload concurrency, ten-file bound, cancellation and late-account rejection, progress and refusal focus, offline pick/drop/paste refusal, attachment-only send and stable retry payload, one image recovery and bounded expiry scheduling, five-second typing expiry and reduced motion, and privacy/read-only lifecycle behavior.
- Browser paths exercise the production frontend and real API/database/local storage. No transport patching or mocked fetch; test doubles are injected adapter implementations. Existing F2 stale-response and account-switch regressions continue to pass.
- Frontend typecheck, lint, formatting, component tests, API generation check, build, class and bundle checks pass. Run the complete E2E suite and associated backend test-control checks. Documentation checks and staged Gitleaks pass. Remaining full checks are deferred to the exact-tree merge gate.
- Entry plus largest language chunk is measured against the 175.2 KB base: above 176 KB requires owner consultation; 180 KB remains the hard limit. Gate wall time must remain within the slice's 260-second threshold, retaining coverage if consolidation is necessary.

## Security, diagnostics and documentation

Map the implementation to T-2, T-13, T-14, T-18, T-20, T-27, T-29 and applicable scope threats in `docs/threat-model.md`, marking changed prose pending owner review. Do not log filenames, bodies, bytes, signed addresses or tokens. Existing structured refusal/request identifiers remain the diagnostic boundary. Update the architecture component map, page/pilot coverage where needed, the F3 verification and budget records, and F2's recorded gate completion from its preserved evidence. Repository artifacts remain in English.

## Coordination and completion

Use an isolated `feat/frontend-chat-attachments-and-presence` worktree. At most three active working sessions total, including the coordinator and other project chats; when the other project is active, only one delegated task runs. Shared files have one writer. Inspect other chats before heavy checks and run builds, browser suites and gates serially; resource measurements require an otherwise idle machine.

After implementation, fresh independent standard and security reviewers examine the complete diff. Follow the after-fix rule without extra review loops or failed-assertion gate retries. Run main's scratch-copied merge gate on the exact committed tree, merge only on success and verify `finish` tree equality. F3 completion does not close slice 6: cumulative layer/security reviews and final performance measurements remain a separate phase.
