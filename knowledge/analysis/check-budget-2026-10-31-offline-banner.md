# Offline banner check budget

Working-tree base: `6670bf3fe444c03b889035e1fca7eefb2409d170`.

The production build and check:bundle report 173.4 KB gzip for the entry and largest language chunk: 162.2 KB entry/static imports and 11.2 KB language chunk. The 176 KB owner checkpoint is not reached; the hard limit remains 180 KB. Student Home first load is 188.5 KB of 250 KB and tutor Today 189.1 KB of 400 KB. Twenty-one page modules load on demand. The shared store adds one bounded grace timer and no network polling.

The full component run passed 593 tests in 56 files in 5.13 seconds before the additional route-live-region regression test. The final full gate passed in 235.4 seconds, including 594 component tests and 902 end-to-end tests; see the [merge evidence](../reviews/slice-6-offline-banner-merge-2026-10-31.md). The approved temporary 260-second gate threshold still applies. This is build evidence, not an isolated performance benchmark.

Five consecutive existing realtime/live-updates/notifications browser series passed in all three engines, 72 tests each, in 19.3, 20.4, 19.1, 17.7 and 17.9 seconds. They exercise the production frontend against the real backend. Earlier sign-out failures and their correction are retained in the readiness record; no failed series counts toward these five.
