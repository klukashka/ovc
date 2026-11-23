# U2 after-fix evidence: `pnpm check:bundle` entry budget (2026-11-23)

Status: agent record, written 2026-11-23 for the after-fix gate of change U2 of the interface rework stage (`feat/ui-shared-rules`), as the Failures bullet of [ADR-0018](../../docs/decisions/0018-targeted-checks-and-merge-gate.md#decision) requires; linked from its `after-fix` row in [the slice 10 known limitations](slice-10-known-limitations.md). Re-verified by the interface rework stage's slice review.

## Failed gate

- Gate: 2026-11-23 14:11:41 UTC, `--squash`, base `cc4494cb94b3705aecac857a2062e48c33d0986f`, tested tree `32f44ba6d616715756a3a4da47a9fbef8c514ae4`, result `failed`, wall 234.1 s (only `pre` and ABC ran).
- Command log: `~/Library/Caches/ovc-e2e/logs.j0k1Fw/23.log` (the lane C entry `pnpm check:bundle`), SHA-256 `f46e170a7af28c08acaad6592decd63f2a3dfc91299621006021053a13d38f9a`; its lines 2 and 13 to 14:

```text
entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 12.5 KB: 184.1 KB (limit 184.0 KB)
The bundle is outside its budget:
the entry and its static imports with the largest language chunk are 184.1 KB gzip of JavaScript, above 184.0 KB
```

- Match key: the check `pnpm check:bundle`, finding "the entry and its static imports with the largest language chunk are … above 184.0 KB" (`frontend/scripts/bundle-budget.ts`, `entryLimit`).
- Hunks that address it: the `studentCard` and `groups` sections moved out of `frontend/src/lib/i18n/en.ts` and `ru.ts` into `frontend/src/features/students/cardTexts.{ts,en.ts,ru.ts}` and `frontend/src/features/groups/texts.{ts,en.ts,ru.ts}`, loaded by `StudentCardPage.tsx`, `GroupsPage.tsx` and `GroupPage.tsx` through `useFeatureTexts`, with `frontend/src/lib/i18n/resources.ts`, `frontend/eslint.config.ts` and `newLesson.materialsChosen` in `MaterialPicker.tsx`. The check, its limit and its configuration are unchanged.

## Command and trees

Each run, in `frontend/` of the change's worktree, through the main checkout's wrapper: `/Users/klukashka/Work/ovc/tools/dev-env.sh pnpm build`, then `/Users/klukashka/Work/ovc/tools/dev-env.sh pnpm check:bundle` (the gate's own commands for the check, which reads the build). `main` had not moved: both trees are on base `cc4494c`.

- Before tree: `32f44ba6d616715756a3a4da47a9fbef8c514ae4` (the squash of `cf4f8cf` onto `cc4494c` with both threat-model status additions, identical to the failed gate's tested tree).
- After tree: `2f829650debb719c69a744681a406dbc1df8e3f7` (the before tree plus only the change's new diff, before this record and its row were added).

## Recorded lines

Before run 1 (the only before run, because it reproduced the finding): build exit 0, `check:bundle` exit 1:

```text
entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 12.5 KB: 184.1 KB (limit 184.0 KB)
The bundle is outside its budget:
the entry and its static imports with the largest language chunk are 184.1 KB gzip of JavaScript, above 184.0 KB
```

After runs 1 to 10 (all passed; no `outside its budget` line in any run):

```text
run 1 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 2 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 3 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 4 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 5 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 6 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 7 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 8 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 9 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 10 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
```

## Second series on the new base `7b961ff`

The series above ran on base `cc4494c`. Before the next gate `main` moved to `7b961ff` (a change of test files only), and the owner decided on 2026-11-23, by multiple-choice question: "Новая серия на 7b961ff (Recommended)", that a fresh series runs on the new base as ADR-0018's Trees item says ("when `main` has moved, both run on the new base"), while the first series stays in this record unchanged.

- Before tree: `24f450f0b97bb2bf500dfca10336187cb44d21d1` (the squash of `cf4f8cf`, U2 without its new diff, onto `7b961ff`, with both threat-model status additions and both sides' known-limitation rows), built in a temporary worktree of `7b961ff`.
- After tree: `f0cc675f466d98acf50b1440df3dfeccf6e3565a` (commit `21fb53d` on `7b961ff`: the before tree plus only the change's new diff, before this second series and its row were added).
- Command: as above, `/Users/klukashka/Work/ovc/tools/dev-env.sh pnpm build`, then `/Users/klukashka/Work/ovc/tools/dev-env.sh pnpm check:bundle`, in `frontend/` of each tree.

Before run 1 (the only before run, because it reproduced the finding): build exit 0, `check:bundle` exit 1:

```text
entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 12.5 KB: 184.1 KB (limit 184.0 KB)
The bundle is outside its budget:
the entry and its static imports with the largest language chunk are 184.1 KB gzip of JavaScript, above 184.0 KB
```

After runs 1 to 10 (all passed; no `outside its budget` line in any run):

```text
run 1 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 2 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 3 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 4 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 5 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 6 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 7 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 8 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 9 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
run 10 build=0 check=0 entry and its static imports: 171.6 KB gzip of JavaScript, with the largest language chunk 11.1 KB: 182.6 KB (limit 184.0 KB)
```
