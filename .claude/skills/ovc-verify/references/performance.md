# Performance review

Act as an independent reviewer. Read `AGENTS.md`, the affected specifications and the [non-functional requirements](../../../../docs/requirements/non-functional.md). Review only the assigned component/profile without source edits. You may delegate narrower work in fresh contexts with inherited scope and permissions; own the combined conclusion and report descendant assignments. Return measurement evidence and limitations using the parent skill's format.

Before implementation, review complexity, bounded work, query patterns, payload sizes and performance-sensitive architecture. After frontend or backend changes, measure the relevant executable components. Use the commands registered in `docs/development.md`. Missing tooling for an implemented capability is `blocked`; an unimplemented capability names its roadmap slice. Neither is a passing benchmark.

Declare datasets, workload, sample count, metrics and regression decisions before running. Compare base and head with the same build configuration, tool versions, machine and database state. Retain all samples and environment details. Run serially on an otherwise idle machine; a noisy or invalid run needs a documented cause and a complete replacement measurement, not reruns until green.

- Backend: API latency and payload size per route, database query count per request (no per-row queries in loops), keyset pagination cost at large offsets, chat fan-out latency to N connected members, upload and download streaming memory independent of file size.
- WebSocket gateway: memory and task count per connection, cleanup of the connection registry on disconnect, error and abrupt-close paths, behavior with many idle connections.
- Frontend: loading milestones, JavaScript and asset size, layout shifts, interaction latency for chat input and calendar navigation, bounded query caches, long message lists.
- Establish baselines from measured representative runs when a component first becomes executable, and record justified budgets in the implementing change and in `docs/requirements/non-functional.md`. Do not invent a zero baseline or an arbitrary threshold. Budget increases or reduced coverage need explicit owner approval.

## Resource lifetime measurements

Use the [component checklist](../../../../knowledge/reviews/ai-development-setup.md#component-checklist) to select the workload and resource counters for the assigned component. Scope these checks to the changed lifecycle and its transitive consumers; a fast happy path does not establish leak freedom.

1. Inventory acquired resources, their owner, maximum retained size and release condition: database sessions and pool checkouts, WebSocket connections and registry entries, async tasks, file handles and temporary upload files, conferencing embed instances, event listeners, subscriptions and timers in the browser.
2. Declare warm-up, input sizes, concurrency, cycle and sample counts, measurement points and budgets before running. Repeat complete acquire, use and release cycles for success, error, cancellation, disconnect and reconnect, and shutdown.
3. Wait for explicit cleanup conditions with deadlines before post-cycle samples; do not use sleeps to hide unfinished cleanup. Run repeated cycles in one long-lived process to expose accumulation.
4. Record peak and post-quiescence memory and resource counts. Use allocation snapshots or heap snapshots where supported, and descriptor, task and connection counts for what heaps omit. A single flat snapshot does not establish cleanup.
5. Compare growth per cycle against the declared budget. Distinguish bounded caches from retained live resources, and attribute unexplained growth to its retainer.
6. Preserve samples, workload, commands, working-tree state and environment in the readiness report.
