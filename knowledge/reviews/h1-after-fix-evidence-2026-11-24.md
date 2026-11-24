# H1 after-fix evidence (2026-11-24)

Status: agent record. Evidence for the after-fix gate of change H1 of the [interface rework plan](../plans/ui-rework.md) (`feat/homework-simple-task`), as the Failures bullet of [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#decision) requires; linked from H1's `after-fix` row in the [slice 10 known limitations](slice-10-known-limitations.md).

## Failed gate

Gate of 2026-11-24, 16:01 to 16:11 UTC, on mains power, start load 1.37 5.86, wall time 568.1 s; tested tree `18306507828fd4af177731dbadc6ad137b38fddd` of commit `d401607`, the H1 change squashed onto base `3db6ebc`. The end-to-end run had 3 failed and 2319 passed tests. Command log `/Users/klukashka/Library/Caches/ovc-e2e/logs.8ruc5z/26.log`, SHA-256 `5168c9918b68283fcb3cd87864dffccb5ee2a6e279deb008a26abe39bd2c5799`, lines 2555 to 2614:

- `[chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys`: `Error: locator.click: Error: strict mode violation: locator('main header').getByRole('button', { name: 'Новое задание', exact: true }) resolved to 2 elements` at `builder.spec.ts:1111:99`
- `[firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys`: `Error: locator.click: Error: strict mode violation: locator('main header').getByRole('button', { name: 'Новое задание', exact: true }) resolved to 2 elements` at `builder.spec.ts:1111:99`
- `[webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys`: `Error: locator.click: Error: strict mode violation: locator('main header').getByRole('button', { name: 'Новое задание', exact: true }) resolved to 2 elements` at `builder.spec.ts:1111:99`

## Match key, cause and fix

- **Key:** the title above with each of the projects `chromium`, `firefox-4` and `webkit-4`, and the failing call at `e2e/builder.spec.ts:1111` (the strict mode violation of the click on the Library header's button «Новое задание»).
- **Cause:** H1 added the Library header's "New task" button, «Новое задание» in Russian, beside the existing builder button, whose Russian label was also «Новое задание»; the two entries had the same accessible name.
- **Fix:** by the owner's decision of 2026-11-24 («"Новое задание" + "Новый шаблон" (Recommended)»), the builder button gets its own text key `library.newTemplate`, "New template" and «Новый шаблон», while `library.newAssignment` stays the new template's default title. Hunks: `frontend/src/features/library/texts.en.ts` line 4 and `texts.ru.ts` line 6 (the new key), `frontend/src/features/library/LibraryPage.tsx` line 746 (the button's label), `frontend/e2e/builder.spec.ts` line 1111 (the click names «Новый шаблон»; the test's title and its assertions are unchanged), `frontend/e2e/library.spec.ts` lines 549 and 554 (the same rename in the English library test), `knowledge/design/web/pages.md` lines 309, 320, 323 and 325.

## Command and trees

Each run, from the worktree's root through the main checkout's `tools/dev-env.sh`, one at a time on an otherwise idle machine on mains power, with `frontend/test-results` removed after each run:

```bash
/Users/klukashka/Work/ovc/tools/dev-env.sh pnpm --dir frontend exec playwright test e2e/builder.spec.ts --retries=0 --project=<chromium, firefox-4 or webkit-4> -g "step 5 Maria builds Past simple, week 1 from the library and previews it without keys" --reporter=list --trace=off
```

`main` moved to `27e383b`, so both trees are on it: the before tree `6b8d3339c2741b2fceefdc2dc91e6376cb2b75d9` (commit `a2c047c`) is the failed gate's change (`d401607`) squashed onto `27e383b`, whose `builder.spec.ts` and Library feature are identical to the failed gate's tested tree; the after tree `a1c96ad443f0fa1c6cdc94d12546ef7ec2f2fa18` (commit `42b6634`) is the before tree plus only the fix's six files. Runs from 2026-11-24 17:56 to 18:00 UTC.

## Runs

The first before run of each project reproduced its match key, so it is the only before run; the after tree had 10 runs per project, all passed.

### chromium

- `before` `chromium` run 1, exit 1, log SHA-256 `9adc2e9d1f52abf670b15ed7fee329ded4372d54085406afc513a1b2b168ec73`:
  - `✘  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (1.1s)`
  - `Error: locator.click: Error: strict mode violation: locator('main header').getByRole('button', { name: 'Новое задание', exact: true }) resolved to 2 elements:`
  - `at e2e/builder.spec.ts:1111:99`
  - `1 failed`
- `after` `chromium` run 1, exit 0, log SHA-256 `dae99808d5376bda37f1555d270a1bee54b911d941074a5ef15d817233eb39e2`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.7s)`
  - `1 passed (6.2s)`
- `after` `chromium` run 2, exit 0, log SHA-256 `1c68f524bfcb5ed5ac60158c5fb4907069c05221ae981f69a7122f5f6dc71a18`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.7s)`
  - `1 passed (6.1s)`
- `after` `chromium` run 3, exit 0, log SHA-256 `73fdfc048df553ef56c36cca3eaebcf2002be8bf2fea13f3d4b8769bd887477c`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.6s)`
  - `1 passed (5.8s)`
- `after` `chromium` run 4, exit 0, log SHA-256 `6090fa70d2fa924ab9324cb35126a72afa4e726fd75fa8cc07533b88d534237a`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.6s)`
  - `1 passed (5.6s)`
- `after` `chromium` run 5, exit 0, log SHA-256 `bf579101960d9210fcc14c179c2eacf41142f444347e878835c2f4590b2ddbdf`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.6s)`
  - `1 passed (5.8s)`
- `after` `chromium` run 6, exit 0, log SHA-256 `78440f361562695a6b99da9143c37c82186edaa7e40ad6abe3f39b7c64a35a63`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.6s)`
  - `1 passed (6.0s)`
- `after` `chromium` run 7, exit 0, log SHA-256 `d2d1cb5430863fe5cb6424e7a3a2d6ef4137cf13106c0fc13ad9e804c3e2cf3b`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.5s)`
  - `1 passed (5.7s)`
- `after` `chromium` run 8, exit 0, log SHA-256 `43fe3143b8eb17afd08f5dd724dc8e68cb0153e523a151571c2e45817406471d`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.6s)`
  - `1 passed (5.7s)`
- `after` `chromium` run 9, exit 0, log SHA-256 `56a95d502d41af63246bfd4dee0fc86c086a0cc7ac1249745f23f972cb4e01fd`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.6s)`
  - `1 passed (5.7s)`
- `after` `chromium` run 10, exit 0, log SHA-256 `5ff8f66c65a9bf0508e874bf52216fda794045a39eeeb1e30a22c5c6461b6c62`:
  - `✓  1 [chromium] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.5s)`
  - `1 passed (5.7s)`

### firefox-4

- `before` `firefox-4` run 1, exit 1, log SHA-256 `ebc20fd01a3006719902db6ddc0776df4fe9c6da36cb0aebc028bf103347a302`:
  - `✘  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (1.6s)`
  - `Error: locator.click: Error: strict mode violation: locator('main header').getByRole('button', { name: 'Новое задание', exact: true }) resolved to 2 elements:`
  - `at e2e/builder.spec.ts:1111:99`
  - `1 failed`
- `after` `firefox-4` run 1, exit 0, log SHA-256 `56caea47b6f3795e80fc35a82d861dfe64d83c8631b7d0247dfe7b15e2e1c0c4`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.2s)`
  - `1 passed (6.9s)`
- `after` `firefox-4` run 2, exit 0, log SHA-256 `3a3d362c235e01d794d16a266cbee6b497fa79dfa5ea96a8dc4b26111b84fde8`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.1s)`
  - `1 passed (6.8s)`
- `after` `firefox-4` run 3, exit 0, log SHA-256 `7a3b3a50753a4fbdae0dd945b81eb4283e1d6a1c4fecfded93f6c49f862c95be`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.2s)`
  - `1 passed (8.1s)`
- `after` `firefox-4` run 4, exit 0, log SHA-256 `7f0fa320c296bad234ff939e0a364fd81c549525818d4caa5b072f604f865493`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.3s)`
  - `1 passed (6.8s)`
- `after` `firefox-4` run 5, exit 0, log SHA-256 `75236cc4a8fc11e1d4e0b36736e62bcaf2ad98f6550040940cac23387d203f90`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.3s)`
  - `1 passed (7.2s)`
- `after` `firefox-4` run 6, exit 0, log SHA-256 `f18d88bdf787e5a3b0b81ce493b45674083468719401c8f660f6cf3a8d5b4197`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.2s)`
  - `1 passed (6.9s)`
- `after` `firefox-4` run 7, exit 0, log SHA-256 `f89966ba812cbe45878ad0b9593eff2102dd320b490a69f5ad433f6d54e6181b`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.2s)`
  - `1 passed (6.9s)`
- `after` `firefox-4` run 8, exit 0, log SHA-256 `15a724f59847e1937ae05e142c05d8ef649db77368bb1ad958ae21f5b9895c9f`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.3s)`
  - `1 passed (6.8s)`
- `after` `firefox-4` run 9, exit 0, log SHA-256 `bcecbe4c8d1d5ef482e9638fc2aa312275317bcfef55e807c2d7002123121d9b`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.2s)`
  - `1 passed (6.8s)`
- `after` `firefox-4` run 10, exit 0, log SHA-256 `99b63580d21cf986bb177ff80f75289d2791c1efe861e6ced107b4e8edb5e05e`:
  - `✓  1 [firefox-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.2s)`
  - `1 passed (6.8s)`

### webkit-4

- `before` `webkit-4` run 1, exit 1, log SHA-256 `f2e22e6db51b3159a2ca83db4085bec77830d9d95e82938e126c8c12f13eec88`:
  - `✘  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (1.5s)`
  - `Error: locator.click: Error: strict mode violation: locator('main header').getByRole('button', { name: 'Новое задание', exact: true }) resolved to 2 elements:`
  - `at e2e/builder.spec.ts:1111:99`
  - `1 failed`
- `after` `webkit-4` run 1, exit 0, log SHA-256 `03149df716f636845aa54bf235e1f47529511059042ca79626bbe12dbd9b12d6`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.3s)`
  - `1 passed (7.1s)`
- `after` `webkit-4` run 2, exit 0, log SHA-256 `655b85bfbeb939e934b405378e8a3bf1ea4755024a5a4f81fbef8c314e22d4fc`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (3.5s)`
  - `1 passed (7.8s)`
- `after` `webkit-4` run 3, exit 0, log SHA-256 `6addf97eeacb473a555674d5230e02caa8c041b3b9914c9d246727309de332ad`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.9s)`
  - `1 passed (6.3s)`
- `after` `webkit-4` run 4, exit 0, log SHA-256 `ceb6cbe32a8221064391097b087a848c0c5ff8f7adf25c6f38933f342ebaa505`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.8s)`
  - `1 passed (6.8s)`
- `after` `webkit-4` run 5, exit 0, log SHA-256 `4ba27152ea6d84abea77f96bf3f599137d12bd35ee592d5eb975e059bb356374`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.8s)`
  - `1 passed (6.3s)`
- `after` `webkit-4` run 6, exit 0, log SHA-256 `13d393da148204313e0a5093339c837199752873296bc1220ea3bff082be2a84`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.8s)`
  - `1 passed (6.2s)`
- `after` `webkit-4` run 7, exit 0, log SHA-256 `62b2bfca29954e5ac13cc8f66076e710825b406668d2d8abc0d3ba2304ef5322`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.8s)`
  - `1 passed (6.5s)`
- `after` `webkit-4` run 8, exit 0, log SHA-256 `871fcddab0b237957e1b2d9834c48fd633b31fc0f8ec6c0dc4f6d06b82c39f44`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.8s)`
  - `1 passed (6.5s)`
- `after` `webkit-4` run 9, exit 0, log SHA-256 `88a2429d830b96e8db10b0a4e762c63c624a872784416587758ae4c55da9d2f5`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.8s)`
  - `1 passed (6.3s)`
- `after` `webkit-4` run 10, exit 0, log SHA-256 `bc3459aa53c89301b2a0c6e55ff23a1a708c8b7fea3bd15e651be29253e4d82f`:
  - `✓  1 [webkit-4] › e2e/builder.spec.ts:1087:3 › Maria in Russian › pilot-2 › step 5 Maria builds Past simple, week 1 from the library and previews it without keys (2.9s)`
  - `1 passed (6.7s)`
