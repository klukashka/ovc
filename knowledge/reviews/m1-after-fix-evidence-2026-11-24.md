# M1 after-fix evidence (2026-11-24)

Status: agent record. Evidence for the after-fix gate of change M1 of the [interface rework plan](../plans/ui-rework.md) (`feat/messages-messenger-layout`), as the Failures bullet of [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#decision) requires; linked from M1's `after-fix` row in [the slice 10 known limitations](slice-10-known-limitations.md).

## Failed gate

Gate of 2026-11-24, 15:27 to 15:37 UTC, on mains power, start load 2.07 5.80, wall time 604.6 s; tested tree `ea946df96f66bc134baa90359cbe8e271e55d88e` (commit `1b920fe` on base `bbbf785`). The end-to-end run had 2 failed and 2299 passed tests. Command log `/Users/klukashka/Library/Caches/ovc-e2e/logs.XNIPCO/26.log`, SHA-256 `d1fb5fc7d874dcbe2bc528bc93d023640aeffb8c3a46e1497bae0f9a7d87d903`, quoting:

- `[firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals`: `Error: page.waitForResponse: Test timeout of 30000ms exceeded.` at `messages.spec.ts:690:23`.
- `[webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals`: `Error: expect(received).toEqual(expected) // deep equality` (twelve `false` for twelve `true`) at `messages.spec.ts:741:26`.

## Match keys and causes

- **firefox-2**, key: the title above, project `firefox-2`, error kind timeout at `e2e/messages.spec.ts:690` (`page.waitForResponse` for the send after Enter). Cause: Playwright's Firefox `fill` commits the text through composition events, and the composer's composition latch, set on `compositionstart` and cleared only on a key up, swallowed the next Enter; the same happened to people after dictation or a candidate picked with the mouse. Fixing hunk: `frontend/src/features/messages/Conversation.tsx` lines 49 to 61 (`lineBreakInputs`, `sendsOnEnter` without the latch), 115 (`pendingEnter`), 254 to 267 (the `beforeinput` listener that sends instead of a line break after a qualifying Enter) and 649 to 659 (key down and key up record and clear the pending Enter); the latch and its handlers are removed.
- **webkit-2**, key: the title above, project `webkit-2`, the failing assertion `e2e/messages.spec.ts:741` (`reachedByTab`). Cause: WebKit's Tab does not move focus to buttons by default, as `e2e/accessibility.spec.ts` already handles. Fixing hunk: `frontend/e2e/messages.spec.ts` line 683, which presses `Alt+Tab` in WebKit and `Tab` elsewhere, and the test's `browserName` fixture parameter; the assertion and the title are unchanged.

## Command and trees

Each run, from the worktree's `frontend/` after `pnpm build` of its tree, through the main checkout's `tools/dev-env.sh`, one at a time on an otherwise idle machine:

```bash
/Users/klukashka/Work/ovc/tools/dev-env.sh pnpm e2e e2e/messages.spec.ts --project=<firefox-2 or webkit-2> --retries=0 --reporter=list --trace=off -g "bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals"
```

`main` moved to `3db6ebc`, so both trees are on it: the before tree `87ddc36b5b4329155a6165c5b95697fdf69ab9d4` is the failed gate's change (`1b920fe`) squashed onto `3db6ebc`, whose test file and Messages feature are identical to the failed gate's tested tree; the after tree `231b9ccb884738207b84f4d88285eb9031392e15` is the fixed change squashed onto `3db6ebc`, which differs from the before tree only in `frontend/e2e/messages.spec.ts`, `frontend/src/features/messages/Conversation.tsx` and `frontend/src/features/messages/MessagesPage.test.tsx`. The commit that holds this record adds only this file and the `after-fix` row to the after tree.

## Runs

The first before run of each project reproduced its match key, so it is the only before run; the after tree had 10 runs per project, all passed.

### firefox-2

- `before` `firefox-2` run 1, exit 1, log SHA-256 `fa695c28c26ce9a8067ea9f06ee8b46a68508a9d3d49bece39b4a12f9338fd58`:
  - `✘  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (30.1s)`
  - `Error: page.waitForResponse: Test timeout of 30000ms exceeded.`
  - `at e2e/messages.spec.ts:690:23`
  - `1 failed`
- `after` `firefox-2` run 1, exit 0, log SHA-256 `7655c251a38a9b3144a8685739aea5cc25243fa51bf0f8305e7777150652ae01`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.3s)`
  - `1 passed (7.0s)`
- `after` `firefox-2` run 2, exit 0, log SHA-256 `bf3e6292c8e3f8f2ff10aa1f1ccc6c5d308ad7d89d4a804ec351ddc921102e42`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.3s)`
  - `1 passed (6.0s)`
- `after` `firefox-2` run 3, exit 0, log SHA-256 `84942cbdcc00bf8e3377f83f1fe65c242dccf8a17485d323554779d09f6335c7`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.7s)`
  - `1 passed (6.6s)`
- `after` `firefox-2` run 4, exit 0, log SHA-256 `2b12d51970f83f17421dd4fc4a8f60315af2cec61624eb383869ce7e20b55b18`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.3s)`
  - `1 passed (6.6s)`
- `after` `firefox-2` run 5, exit 0, log SHA-256 `4e991e4bc9a40d89e54fa5c371ab7ece38c6d3b02d910b7827c10f721678b29c`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.3s)`
  - `1 passed (6.3s)`
- `after` `firefox-2` run 6, exit 0, log SHA-256 `d4ddbeecf7cac9f2c2c6f6f19045a88a9350ae54f3324f09fef1201003b66ff0`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.7s)`
  - `1 passed (6.7s)`
- `after` `firefox-2` run 7, exit 0, log SHA-256 `5c1f976a6ce8f3a100ca8399849d980dc7ed10851f4f611e24d7077c9bc44fa3`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.3s)`
  - `1 passed (6.2s)`
- `after` `firefox-2` run 8, exit 0, log SHA-256 `63e38432abd0e2348a773f7236120b3f8fe12fe571c702637e7e2cfa4b43912b`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.6s)`
  - `1 passed (6.5s)`
- `after` `firefox-2` run 9, exit 0, log SHA-256 `29c964e6079974c48e73055f2dcc5defb76269f89cef037c6aee5f4c2e79000a`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.6s)`
  - `1 passed (7.0s)`
- `after` `firefox-2` run 10, exit 0, log SHA-256 `6fff579aafbfe0102e461cb8f363c80a0e7cfce058df740bed5f3024432b31d6`:
  - `✓  1 [firefox-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (4.4s)`
  - `1 passed (8.9s)`

### webkit-2

- `before` `webkit-2` run 1, exit 1, log SHA-256 `2e52da421fd85651cbc3ce81c232f50c9fb2d49d63e9f387338c8a2fd68e9e55`:
  - `✘  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.6s)`
  - `Error: expect(received).toEqual(expected) // deep equality`
  - `at e2e/messages.spec.ts:741:26`
  - `1 failed`
- `after` `webkit-2` run 1, exit 0, log SHA-256 `52bf5c4803fda6e145942f36b833172cb46930b8ac359e0158b21fe65352be19`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.4s)`
  - `1 passed (7.1s)`
- `after` `webkit-2` run 2, exit 0, log SHA-256 `432609132d921bda1d7bfc3b97c060be228815e9e07685a73bbbe46b0d7e76fd`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.3s)`
  - `1 passed (6.1s)`
- `after` `webkit-2` run 3, exit 0, log SHA-256 `d0ef15263f7c31029e8bb7e0a38c89a8c36b51b58473a11ad3a33f20a3e0cb23`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.2s)`
  - `1 passed (5.9s)`
- `after` `webkit-2` run 4, exit 0, log SHA-256 `acb1dda4ebd58571f6a63eed3e7415550cdf386f9cc24ae5849d23cc889840b5`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.2s)`
  - `1 passed (6.0s)`
- `after` `webkit-2` run 5, exit 0, log SHA-256 `a5cce24e5261c5c7e72192bfb9a8087f6a92a309d38a6a92c3b85e21690514aa`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.3s)`
  - `1 passed (6.4s)`
- `after` `webkit-2` run 6, exit 0, log SHA-256 `1ab076fde5334390d67e78a1a84a06b2a4272a8e3ee242c41b2a8f5f359eb045`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.3s)`
  - `1 passed (6.0s)`
- `after` `webkit-2` run 7, exit 0, log SHA-256 `4671c18802bad2706117afb2a2e5a6a845b96c111469a2cb12cc84d12871931a`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.2s)`
  - `1 passed (6.1s)`
- `after` `webkit-2` run 8, exit 0, log SHA-256 `fb21ac358c23ce19ce647806458d7eaeb36af5c8b447fa2c6ff5049fabe548c3`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.2s)`
  - `1 passed (6.2s)`
- `after` `webkit-2` run 9, exit 0, log SHA-256 `2ad50430f2ee78870c5886af43cedd7649676ca4c9d3890ebe5f11fbd64e4e14`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.5s)`
  - `1 passed (6.2s)`
- `after` `webkit-2` run 10, exit 0, log SHA-256 `7fa7f40653f75b6f79323540b42ceccd07960004a334ac94d9394a63e1444203`:
  - `✓  1 [webkit-2] › e2e/messages.spec.ts:665:3 › the messenger layout on a computer › bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals (3.3s)`
  - `1 passed (6.1s)`
