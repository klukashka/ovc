# The merge gate's end-to-end run without Playwright traces: paired measurements

Recorded on 2026-11-20 for change `chore/gate-e2e-traces-off`. The change makes the merge gate's end-to-end entry `../tools/dev-env.sh pnpm e2e --max-failures=10 --trace=off`, and leaves local `pnpm e2e` and the Playwright configuration's `trace: 'retain-on-failure'` unchanged. It follows the [gate gap record](e2e-gate-gap-2026-11-20.md), which measured traces at about 40 s of the end-to-end phase in one pair. The owner decided on 2026-11-20 that this change needs its own alternating paired measurements.

In the two complete pairs, the end-to-end run without traces was 106 s and 63 s faster than with them, and the summed test time fell by 577 s and 365 s. A third pair was not completed. Every run had another project's work beside it, so the figures are not from an idle machine.

## What a failed gate keeps without traces

- **The kept evidence does not change.** The gate already kept no trace, screenshot, video, attachment or `error-context.md` (the owner's decision of 2026-11-07, in the [development guide](../../docs/development.md#change-checks-and-the-merge-gate)). It kept only `e2e-results/failed-tests.txt` and each failing test's redacted `browser-diagnostics.json`.
- **Failed tests still leave output, so `failed-tests.txt` still lists them.** The gate lists every test output folder of the run, and until this change the development guide relied on `retain-on-failure` for "only failed tests leave output". That never held for a passing test that writes its own file to its output folder, such as `frontend/e2e/library.spec.ts`'s recording upload test, which is listed in both modes.
  - Playwright 1.63.0 writes `error-context.md` into the test's output folder whenever the test ends with an error, whatever the trace mode.
  - Checked with Playwright 1.63.0 in a scratch project outside the repository: three tests, one failing with a page, one failing without a page and one passing with a page.
  - With `--trace=off`, both failed tests left an output folder holding `error-context.md`, and the passing test left none.
  - With `retain-on-failure`, the same two folders also held `trace.zip`.
- **A failure seen during this series confirms it.** The failed test of run `clean2-off` (traces off) still printed its `browser-diagnostics` attachment and its error context in the list reporter's output, both under its output folder.
- **What is lost:** the raw trace that stayed in the gated worktree's ignored `frontend/test-results/` until the worktree was removed. Debugging a failed gate now needs a local rerun of the named tests, which keeps traces by default.

## Method

- **Tree:** `main` at `fbe9456`, in a new detached worktree for each run, created with `git worktree add` seconds before the run, with the main checkout's `bin/` copied into it, as gates are run. The worktree was removed after each run, and its `frontend/test-results` deleted first.
- **Gate-like run:** the commands of lanes A, B and C from the full check list of that tree's `tools/merge-gate.sh`, in list order and with the lanes in parallel, as the gate runs them, each lane stopping at its first failure. Then the end-to-end run:
  - traces on: `pnpm e2e --max-failures=10`;
  - traces off: `pnpm e2e --max-failures=10 --trace=off`.
- **Differences from the gate:**
  - every `../tools/dev-env.sh` ran as the main checkout's `tools/dev-env.sh`;
  - there was no pre-phase tool check and no gate lock or run lock held by a gate;
  - the end-to-end run used the configuration's list reporter only.
- **Order:** alternating, traces on first in each pair.
- **Admission rule before each run:** AC power, and a one-minute and a five-minute load average both below 6. The series stopped if a run's process samples showed battery power, or if the load did not fall below 6 in time.
- **Samples:** the twelve processes using the most processor time, every 15 s, with the power line.
- **Wall times:** lanes, from the start of the three lanes to the end of the last one; end-to-end, from the start of `pnpm e2e` to its exit, including the global setup that builds the binaries and the frontend.
- **Summed test time:** the sum of the per-test durations of the passed tests that the list reporter prints.
- **Raw data:** the logs, meta files and samples were kept in a temporary folder outside the repository and are not kept.

## Measurements

| Run | Traces | Start (UTC) | Load at start (1, 5, 15 min) | Load at end-to-end start | Lanes | End-to-end | Summed (passed tests) | Result |
|---|---|---|---|---|---|---|---|---|
| `pair1-on` | on | 18:36:12 | 2.99 5.94 6.45 | 11.49 10.66 8.54 | 227 s | 419 s | 2232 s | 2180 passed |
| `clean1-on` | on | 21:56:03 | 1.94 5.90 114.54 | 11.47 10.96 93.15 | 211 s | 423 s | 2259 s | 2180 passed |
| `clean1-off` | off | 22:15:13 | 2.56 5.96 37.68 | 9.85 9.07 32.62 | 192 s | 317 s | 1682 s | 2180 passed |
| `clean2-on` | on | 22:28:59 | 1.12 5.95 19.71 | 15.40 10.84 18.99 | 195 s | 396 s | 2114 s | 2180 passed |
| `clean2-off` | off | 22:45:16 | 1.36 5.85 12.85 | 8.87 8.30 12.44 | 211 s | 333 s | 1749 s | 1 failed, 2179 passed |
| `clean3-on` | on | 23:00:57 | 1.31 5.85 10.34 | 10.10 9.05 10.87 | 198 s | 336 s (stopped early) | 1653 s | 10 failed, 5 interrupted, 478 did not run, 1687 passed; one error outside any test |
| `clean3-off` | off | not run | | | | | | |

- **Every lane command passed in every run.** All runs were on AC power from start to end.
- **`pair1-on`**
  - Another project's Python process ran at about 650 % processor time from about 18:32 UTC, as `ps` read it at 19:23. In the run's samples it reached up to 117 %, above 20 % in all 40 of its samples.
  - The run was meant as the first half of a pair. The series that followed it stopped before its second run because the load did not fall below 6.
  - It is kept as run, and is not part of the comparison.
- **The other project's work during the clean runs**, in process samples:
  - `clean1-on`: a Homebrew Python 3.11 process near one full core (99 %, above 20 % in 33 samples);
  - `clean1-off`: another project's Python up to 52 % (above 20 % in 3 samples);
  - `clean2-on`: up to 99 % (above 20 % in 14 samples);
  - `clean2-off`: up to 94 % (above 20 % in 13 samples);
  - `clean3-on`: up to 45 % (above 20 % in 3 samples).
  - The fifteen-minute load was still falling from that project's earlier load of several hundred.
- **`clean2-off`'s failure:** `[firefox-4] e2e/reclaim.spec.ts:144` ("reclaim › after the choice Continue leads Home to the kept tutor and the choice page has nothing left") failed after 1.5 s with no failed assertion of its body. The browser diagnostics fixture reported two unexpected `console_error` records and one unexpected `console_warning` record, all classified `unknown` on route `unknown`. It is recorded as a flaky test in the [slice 10 known limitations](../reviews/slice-10-known-limitations.md).
- **`clean3-on`'s failures:**
  - The WebKit browsers of workers in `webkit-1` and `webkit-2` closed during the run. Nine tests failed with `browserContext.newPage: Target page, context or browser has been closed`, and one (`account-edge-cases.spec.ts:257`) hit its 30-second timeout on a click.
  - `--max-failures=10` then stopped the run, so its 336 s and its summed time do not compare with the complete runs.
  - macOS recorded `com.apple.WebKit.Networking.Development` user-fault reports at 22:53, 22:54 and 23:09 UTC. Their contents were not read.
  - It is recorded in the [slice 10 known limitations](../reviews/slice-10-known-limitations.md).
- **The series stopped before `clean3-off`.** After `clean3-on` the load did not fall below 6 before the series' limit; at 23:34 UTC it stood at 142.45 156.57 136.24 from several processes of the other project. The owner decided on 2026-11-20 to accept the two complete pairs rather than wait for a third (wording pending owner review).

## Findings

- **Turning traces off shortened the end-to-end run by 106 s and 63 s in the two complete pairs, 84.5 s on average.** The summed test time fell by 577 s and 365 s.
  - The end-to-end runs with traces took 396 to 423 s in the three complete runs, and those without took 317 and 333 s.
  - The difference within each pair is larger than the spread between same-mode runs: 27 s with traces and 16 s without.
  - Both pairs ran beside another project's work, so the size of the saving is uncertain. Its sign is not.
- **The saving is larger than the one pair of the gate gap record (403 s against 363 s).** That pair also ran under outside work.
- **The lanes are unaffected.** The change touches only the end-to-end entry, and the lanes took 192 to 227 s in all runs.
- **One flaky test showed up in each mode, and neither is caused by the trace setting.** One run without traces failed one Firefox test on browser diagnostics. One run with traces lost its WebKit browsers. Two runs per mode cannot show whether either mode is more prone to these failures.
- **A gate with this change would take about 80 s less** than the same gate with traces, if the saving carries over to the gate's own runs. Against the mains gates of the [gate gap record](e2e-gate-gap-2026-11-20.md#starting-point) (583.0 to 674.8 s), that would bring a typical gate near or below the 540 s of ADR-0018. The effect starts with the gate after this change merges, because a change to the gate is gated by `main`'s copy of the script.
