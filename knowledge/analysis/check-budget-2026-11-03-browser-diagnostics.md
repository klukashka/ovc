# Browser diagnostic and refresh-response check budget

Status: pre-merge evidence; the first exact committed-tree gate failed one E2E assertion. This record does not
close slice 6 or replace its three cold idle-machine gates. The closure record must reconcile the
actual merge-gate report, base, tested tree, lane times and every command result after integration.

## Scope and machine

Base: 4d9ba1e5467c84b1a09c1737c00ea2c8e4c24271. The coherent change adds finite browser diagnostics,
development lifecycle controls and bounded refresh-refusal response ownership. Standard tier with
security trigger; the independent combined standard/full-security review and its corrected minor
wording findings are recorded in the [verification record](../reviews/slice-6-warning-diagnostics-followup.md).

The observed machine is macOS on arm64 with 12 logical cores and 24 GiB RAM. These change checks
use existing dependencies, binaries, browser installations, Go and application build caches;
no cache was emptied. They are correctness evidence, not cold performance measurements. Heavy
commands were sequential with owned sleep/display assertions, lid and source-integrity monitoring.
Other active chats were included in coordination; full performance measurements still require
otherwise idle hardware. The current gate ceiling is 260 seconds and target 165 seconds.

## Passed change checks

- `pnpm typecheck`: exit 0, 0.710 seconds.
- `pnpm lint`: exit 0, 13.963 seconds.
- `pnpm format:check`: exit 0, 1.893 seconds.
- `pnpm test`: exit 0, 5.902 seconds.
- `pnpm build`: exit 0, 0.289 seconds.
- `pnpm check:classes`: exit 0, 0.670 seconds.
- `pnpm check:bundle`: exit 0, 0.105 seconds.
- `pnpm api:check`: exit 0, 0.516 seconds.

All 841 frontend cases in 69 files passed. Entry plus largest language chunk: 175.8 KB gzip of
JavaScript, below the 176 KB consultation threshold and 180 KB hard limit. The eight frontend
commands above total 24.048 command-seconds; this is not the full-run wall time.

The final source-changed full E2E passed 972 cases. Its wrapper also ran documentation and staged
Gitleaks: exit 0, 158.621 seconds observed wall time, 33 safe samples and unchanged source tree
c3fd2aa8f29fe7b8ff3f62d3fdb40ee78ce33cbd. The direct actual 15-second body-deadline opt-in and
180 affected transport/browser cases passed on the earlier identical executable lifecycle snapshot;
the later schedule/signup corrections passed six targeted cases in all three browsers and the
final full suite. The exact snapshots and every failed attempt remain in the verification record.

The backend test-control refusal/route-absence subset passed on unchanged backend base 4d9ba1e.
No backend, dependency or generated-client behavior changes are introduced. Other full-run commands
not selected by the change-check table are deferred to the merge gate, not reported passed.

## Merge gate

The first gate of 9adddce, tree eba49f406d1cce18aa69b755796380e616bcd926, ran in 252.1 seconds
but failed one E2E assertion (971 passed). All 29 other commands passed. Summed command seconds:
366.1; lanes pre 0.2, A 97.7, B 56.2, C 58.7 and E2E 153.3 seconds. This is not an acceptance
or a cold measurement. The document-scoped readiness correction receives its own exact-tree gate.
No command result or elapsed time is inferred from the change checks. The coordinator will retain the complete gate/finish report; slice closure will
record its exact tested and merged trees, summed command seconds, per-lane wall times, total wall
time and warm-cache qualification. A failed assertion blocks merge; exceeding 260 seconds requires
the approved consolidation workflow rather than an automatic unchanged retry.
