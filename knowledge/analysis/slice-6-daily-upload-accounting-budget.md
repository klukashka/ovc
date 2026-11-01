# Daily upload accounting verification budget

Status: consolidation source prepared on 2026-11-01; runtime sensitivity, correctness, review and gate margin remain unverified for the consolidated tests.

## Preserved measurement

The unmerged accounting commit is `5b531bda073a55a2b594cc1442d4bafd32ec438a`, tree `c4a2b362ff1dcefc889e679b0f6957e9199785bf`, over main `d0b91d44ade076cef116f15897e3499d9d8a443a`. Its gate completed all 30 commands successfully, including 944 E2E cases, but wall time was **265.4 seconds**, exceeding the unchanged **260-second** ceiling by 5.4 seconds. This is a failed budget outcome and the change was not merged, despite the script's command-success `result: passed` line.

| Measurement | Seconds |
|---|---:|
| Lane A wall time | 97.3 |
| Lane B wall time | 54.5 |
| Lane C wall time | 56.5 |
| E2E wall time | 166.3 |
| Summed command time | 374.9 |
| Gate wall time | 265.4 |

The report recorded 12 cores. The preserved report is `ovc-merge-reports/5b531bd-daily-accounting-budget-exceeded.log` in the common Git directory; its command logs are `/Users/klukashka/Library/Caches/ovc-e2e/logs.VFqEIW`. The coordinator used supported release of the completed gate lock, preserved the branch and report, and did not merge or repeat the unchanged tree. Noticeable competing-session load was not established, so no automatic load exception applies.

This was not a cold measurement. Lane times, summed command time and gate wall time describe different quantities; their differences do not establish avoidable work or predict savings. No speedup or sufficient gate margin is claimed from source consolidation, and a later faster run alone would not establish a causal performance improvement.

## Approved consolidation

The [approved plan](../plans/slice-6-daily-accounting-consolidation.md) replaces thirteen expanded unused group arrangements and removes exactly two duplicate harness lifecycles. The direct fixture retains real users, sessions, profile/name updates and the accepted tutor/student relationship. The skipped-student case retains its real provisional/unverified student. The combined cleanup fixture retains 101 distinct historical student aggregates and 1001 distinct historical days for a separate locked student. The combined pool fixture retains both independent cancellation observations and all original deadlines.

No production service, query, migration, worker composition, E2E journey, public contract, dependency, policy or deadline changes. Every physical-cleanup observation and all eight 25,000,000-byte lifecycle uploads remain. The [verification record](../reviews/slice-6-durable-daily-upload-accounting.md#consolidated-source-and-assertion-mapping) maps old assertions to saved observations. Positive runtime checks, four isolated sensitivity controls, applicable static/frontend/E2E checks and a fresh combined standard/full-security review remain pending. One new exact-tree gate is required; another budget excess requires a separately reviewed broader plan, not an unchanged retry. Slice performance measurements and three cold gates remain separate obligations.
