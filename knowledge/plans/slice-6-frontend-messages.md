# Text messages implementation

Approved 2026-10-31. Base: `e5ef1a188fc52a3a0afa0d95f4974ee97df4b2e2`. This implements F2 of the [slice 6 plan](slice-6.md), after F1 and S6 merged. Verification tier: standard. The security trigger is met by untrusted text and links, provisional access and account-scoped realtime state. Independent planning security review examined the base and this scope and found no findings; this is design evidence, not runtime verification.

## Scope and order

1. Create the lazy Messages route and RU/EN namespace, provisional navigation and fake transport account gating against the API audience contract.
2. Implement channel search and keyset pages, deep links through channel detail, paginated message history, labels and day separators, edit/delete/erased/reclaim states, responsive list/conversation screens and links from students, tutors, groups and lessons.
3. Send text through REST with a stable idempotency key on uncertain retry; enforce server edit version and time refusals, retain drafts, and allow author and tutor deletion in read-only channels. Apply F1's chat-write guard to every activation, preserving focus and drafts.
4. Connect message/read/channel realtime events to scoped REST refetch. Coalesce sidebar unread and the open list's first page to at most once a second. Report only actually shown messages from visible conversations at most once a second; sends retain the server's existing automatic read semantics. Preserve S6 digest bookkeeping unchanged. Recover histories exceeding one newer-message page without gaps or duplicates.
5. Implement D38 focus behavior and D67 text-only URL rendering, the unconfirmed cue and the pre-reclaim cue for both roles. Cancel timers and ignore late responses across channel/account lifetimes, pagehide and BFCache; remove inaccessible cached history after live loss of access.
6. Run change checks, fresh standard and security reviews, resolve findings under the after-fix rule, then the exact-tree merge gate and finish.

## Files and boundaries

New feature modules and tests live under `frontend/src/features/messages/`. Integration touches router/access, realtime client and liveRefetch, SignedInFrame/Sidebar, i18n, fake transport, and the existing student/tutor/group/lesson pages. Playwright adds `messages.spec.ts` and extends pilot 2 and pilot 4 steps named in the parent plan. The coordinator owns shared realtime and frame changes; each other file has one writer.

Update the applicable navigation, page/state and pilot documentation, frontend module map, review and check-budget records, and threat model in this change. Changes under docs are pending owner review. No backend contract or dependency change is planned. Attachments, previews/downloads, presence/typing UI and digest pilot remain F3.

## Acceptance and checks

- The five new browser tests and two extended pilot steps named in F2 pass against the real API in Chromium, Firefox and WebKit, including a 375px conversation, unread count and header links, read-only deletion, outsider refusal and provisional scope.
- Component tests assert D38 focus, safe links (including userinfo and Unicode format characters), label search, sidebar count/dot and offline guard for every write activation.
- Lifecycle tests cover late responses and timers from an old account/channel, visibility/read throttling, pagehide/resume, reconnect catch-up, version conflicts, stable idempotency retry, live 404 and reclaim_pending/keep cues without a cue on ordinary confirmation.
- Typecheck, lint, formatting, component tests, API generation check, production build, class and bundle checks, affected browser suites, documentation checks and secret scan pass. Other checks are deferred to the full merge gate, never reported passed without execution.
- Entry plus largest language is measured against the 173.4 KB baseline: consult the owner above 176 KB; hard limit 180 KB. The temporary gate budget is 260 seconds. Consolidation must preserve every acceptance assertion.

## Security and observability

T-14: text DOM, isolated direction, no HTML; linkify only parsed http/https URLs without userinfo or format characters, never an unsafe substring of a rejected address. T-44: server-scoped provisional access and role-specific labels, explicit unconfirmed and reclaimed-message cues. T-29: account epoch isolation, no persistent drafts, inaccessible data removal. T-20: bounded coalescing and cleanup, no background read markers or event-driven request storms. T-18/T-27: no bodies, labels, filenames, addresses, tokens or idempotency keys in diagnostics; chat namespace cannot load from a token page before token removal. Reuse safe existing transport errors and bounded failure categories; server diagnostics remain authoritative. No read receipts for another member are introduced.

## Resource coordination

At most three active sessions including the coordinator and other projects. Recheck chat/process activity before delegating and before heavy checks; pause optional work if another session becomes active. Use an isolated worktree. Builds, browser series, race tests and gates are serialized across the device; measurements require an otherwise idle device. Reviewers run read-only inspection concurrently with low-impact work, not competing heavy tests.
