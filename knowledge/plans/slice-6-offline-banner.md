# Shared offline state and banner

Status: implementation approved 2026-10-31; execution evidence pending.
Base: `6670bf3fe444c03b889035e1fca7eefb2409d170`.

Implement F1 of [slice 6](slice-6.md), D39/D75. Standard tier, security trigger met because shared realtime lifecycle crosses every signed-in page and account epoch. Independent security planning review accepted the design; fresh standard and security reviews of the complete implementation remain required.

## Behavior and files

The realtime client publishes readiness and loss of readiness. A small shared store with an injected scheduler starts the five-second grace at signed-in lifecycle start, including the first-ready wait. Failed reconnects never restart that grace. Only a ready frame restores the connection. Account epoch changes and sign-out reset state and ignore stale callbacks; logout is not announced as recovery. Page lifecycle stops timers on pagehide and restarts on BFCache restore. Preserve existing authentication, reconnect backoff and live refetch.

Render the banner in SignedInFrame below the narrow top bar and outside Page. A persistent polite live region announces appearance and recovery once without taking focus or repeating on page navigation. Shared opt-in chat-write controls combine offline with existing busy/unavailable states, expose aria-disabled and the reason in a tooltip and accessible description, and suppress all activation paths including form submission without native disabled. Ordinary REST actions, navigation and typing continue. Composer drafts remain intact.

Files: realtime client and tests; shared state and React hook; app realtime wiring and main initialization; SignedInFrame and OfflineBanner; UI action controls and chat-write guard; RU/EN entry texts and component tests. Actual Messages controls/composer arrive in F2/F3; F1 tests fixtures using the production shared mechanism. No dependency, backend API or authorization change is planned. Update lifecycle design and T-20/T-29 threat documentation, with human-facing wording pending owner review; record check evidence and bundle size.

## Acceptance

Prove 4999/5000 ms boundaries, first-ready waiting, fast reconnect, repeated failures/ready events, stale epoch callbacks, sign-out and page lifecycle cleanup using injected socket and scheduler implementations. Prove one appearance/recovery announcement, no focus movement, preserved composer text, disabled click/submit with reason, preserved independent busy/unavailable reasons after reconnect and functioning non-chat actions. New offline interception E2E is forbidden; use component fixtures. Run realtime.spec.ts, live-updates.spec.ts and notifications.spec.ts five consecutive times in Chromium, Firefox and WebKit.

## Checks and coordination

Run frontend typecheck, lint, format:check, test, build, check:classes and check:bundle, documentation checks and Gitleaks. Other full-run checks are deferred to the independent merge gate. Entry load above 176 KB goes back to the owner; the hard limit remains 180 KB. Gate wall time must not exceed 260 seconds. Preserve failures and follow the repository after-fix policy.

S6 and F1 own separate worktrees; the coordinator edits shared documentation and owns checks/reviews/merges. At most three active sessions including coordination, taking other sessions on the device into account. Recheck chats/processes/locks before heavy work; backend race tests, builds, E2E series and gates run sequentially machine-wide. Measurements run without other checks. Each change has a separate gate and finish; neither closes slice 6.
