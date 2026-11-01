# Slice 6 closure plan

Status: approved by the owner on 2026-11-01. The implementation baseline is `f9c7450928a17017bc34654159a5d27b9299147c`; the cumulative review starts at `e2d04c1`, which closed slice 5. This plan supplements [slice 6](slice-6.md) and its approved presence, digest and frontend completion plans. It does not close the slice.

## Outcome and verification

Close the chat and files slice only after its planned work is accounted for, independent reviews cover the cumulative change and its integration boundaries, representative performance and resource-lifetime measurements exist, findings are resolved, and the owner decides the post-slice check budget. Tier: **slice**. The security trigger is met by authorization, files, signed addresses, presence privacy, email, deletion, test control, dependencies and agent configuration.

The closure plan received independent standard and security planning reviews. Neither found a blocker or major issue. Incorporated minor corrections require five separate security contexts, explicit threat-model coverage for measurement tooling, full rather than documentation-only cold gates, workloads that preserve production admission limits, and separate per-job-pass and backlog-drain measurements. These planning reviews are not execution or implementation verdicts.

F3 merged as `f9c7450`: 664 component tests, 944 browser cases and all 30 gate commands passed. Gate wall time was 250.9 seconds. Finish confirmed tree `c37aed4673ccf3f353d45011394b1f8aa4811664`. The local report is `.git/ovc-merge-reports/f9c7450-frontend-attachments.log`; the preceding failed assertion remains separately recorded. This run was not a declared cold idle-machine measurement.

## Work order

1. Map every S, B, F and E item of the original plan to the merged code and execution record. E5 is closed as a record. E2, E3, E4 and E6 are conditional, not silently required or dropped. Search for completed E7 database-cost and E8 Firefox-context investigations; if absent, perform the original approved investigations before treating planned work as complete. Measurement alone does not authorize an unrelated harness optimization.
2. Record a component and integration-boundary coverage matrix. Start read-only reviews while prerequisites are investigated; their final verdicts must account for any later relevant change. Re-derive membership mutation paths from code. Re-verify all eight slice 6 known limitations, including entries marked resolved or partly resolved, and causally affected entries carried from slice 5, including the unverified-account fake transport.
3. Prepare narrowly scoped opt-in measurement tools in separate branches. Register their commands and prerequisites, review their implementation independently and gate them. The existing 200-account presence check, with gateways inside one process and a shared pool, is not the 2,000-socket independent-process workload.
4. Run the measurement matrix serially with a frozen source revision and declared environment. Retain all samples, failures and invalid runs. Investigate findings instead of repeating assertions until green.
5. Fix findings in separate standard-tier changes, with change checks, the required independent reviews and exact-tree gates. Contracts, scope expansions, new budgets and verification reductions need separate owner approval. Reconcile each changed integration boundary against the final code without automatically repeating unaffected checks.
6. Present the measured check-budget decision and any remaining limitations to the owner as quizzes. Only after the closure conditions below hold, update the roadmap and closure records in a final documentation change, gate its exact tree, merge and run finish.

## Independent review ownership

| Layer | Direct and transitive scope | Boundary ownership and profiles |
|---|---|---|
| Backend | Storage multipart/ranges, migrations, file validation and downloads, chat membership/messages/charges, accounts and tutoring effects, schedule channel links, gateway/listener/presence/typing, digest and worker/purge/erasure | Correctness and performance; backend side of HTTP/client, storage/file, membership/message, digest/email and account/purge boundaries |
| Frontend | Session and XHR, shared offline controls, frame parsing/live refetch, Messages cache/history/read state, uploads/previews/presence/typing, settings usage and existing affected navigation | Correctness, browser and performance; frontend side of HTTP/session, frame/cache, signed preview and focus/resource-lifetime boundaries |
| Security | Five fresh contexts: files/downloads; authorization/presence; deletion/erasure/quota/purge; digest/email/unsubscribe; tooling/configuration/dependencies | Full security profile; the security owner combines the separate results and verifies cross-family boundaries and the disposition of each affected threat |
| Development tooling and agent configuration | Environment isolation, test control and deployment fixtures, measurement harness, dependencies and advisory triage, AGENTS rules and preservation of gate obligations | Relevant correctness/performance and configuration checks; test-control/process, environment/secret, build/served-output and check-run boundaries |

Each review records the exact revision, inspected components, contract and causal edges, commands and results, findings with severity and evidence, and missing coverage. An affected threat is examined, not affected with a reason, or unverified; self-review is not an independent review. Security contexts are sequential when the shared session limit requires it.

## Measurement protocol

Before running a workload, record its command, seed, dataset, participants, admission limits, warm-up, sample count, measurement points, timeout/cleanup conditions and acceptance budget. Defaults are three independent samples, a 30-second warm-up and 60-second steady load for sustained workloads, and 100 acquire/use/release cycles for lifecycle checks; finite job datasets run to the stated completion condition. These sampling choices are not new performance thresholds.

Use the real API, worker, PostgreSQL listener/database and local S3-compatible storage. Use only sanctioned injected external adapters. Use synthetic data and local services; load secret settings only through the sanctioned dev-env wrapper, never open or copy environment files. Do not add production debug routes or serve instrumentation, reports, fixtures or source maps. No new dependency is presumed necessary.

Compare base and head with the same tool versions, build configuration and dataset where the capability existed at the base. New chat functionality receives a measured baseline, never a fabricated zero baseline. The local machine is not the proposed 2-vCPU/4-GB regional reference deployment; local results must state that limitation. Proposed targets remain distinguishable from accepted contracts and established budgets.

| Workload | Required observations |
|---|---|
| 2,000 sockets, groups of 30, 20 messages/second, typing from 10% of accounts and reconnect churn | Delivery, unread and first-page refetch, notification rate, send and sign-out commit latency, listener/writer queues. For each message take the maximum latency across intended recipients before calculating p95. Measure refreshed content and actual browser rendering separately from frame arrival. The proposed delivery target is below one second. |
| Independent API processes | Account-wide presence when one instance disconnects, lease expiry, release backlog, renewal/cleanup and shutdown. Do not substitute two gateways sharing one process and pool. |
| 32 concurrent 25-MB uploads and 64 downloads, plus smaller legal sizes and an office document at inspection bounds | Peak and post-quiescence RSS/heap, goroutines/tasks, descriptors, pool checkouts, temporary resources, incomplete multipart uploads and objects; success, refusal, cancellation, disconnect and shutdown. Keep concurrency fixed while varying file size. |
| Tutor with 500 channels; student with five tutors and three groups; channel with 100,000 messages | Read p95, response size, query count and query plans; keyset cost and absence of per-row queries. Compare applicable NFR-2.1 targets without claiming reference-deployment certification. |
| 5,000 digest recipients, 300,000-message erasure, presence for 2,000 accounts | Per-pass wall time and work completed, deadline/cancellation behavior, forward progress and total drain time. Digest, erasure and presence cleanup have 20-second pass budgets, not an assertion that every backlog must disappear in one pass. |
| Tutor purge with 100,000 messages and 5,000 files; student purge with 50,000 messages across five tutors | Batching, lock waits, duration, object/charge cleanup and effects on concurrent worker lanes. Include reminder/outbox regression under the changed load. |
| 1,000-message conversation; 100 channel switches; channel with 200 pictures under no-store | Render and input latency, layout shifts, downloaded bytes, active download slots, time to display pictures, DOM/listener/timer/cache retention after explicit quiescence. |
| Four concurrent 25-MB browser uploads | Native multipart/cloning memory, file and handler release, progress, cancellation, page lifecycle and account-epoch changes. Static bounds alone are not native-memory evidence. |
| Production frontend in Chromium, Firefox and WebKit | Applicable tutor/student journeys, both languages and themes, phone/tablet/desktop, focus, offline/recovery, diagnostics, bundle and initial rendering milestones. Include a small development-mode warning smoke. |
| Three cold full check runs | Every command status, lane and total wall times, summed command seconds, core count and precise cache state. Retain all three samples. |

The load generator respects 30 message writes per minute per account and upload concurrency/hourly/pending/quota bounds; seed and distribute enough independent participants for the whole warm-up and measurement. Count admission failures, errors and missing events explicitly rather than excluding them from latency statistics. Do not raise production limits to make a benchmark pass.

Two thousand sockets do not require two thousand browser processes. Use lightweight real protocol clients, faithfully exercising the production refetch/coalescing path where supported, and a declared bounded set of actual browser observers. Report protocol, REST and rendered-UI results separately. A custom immediate GET is not proof of the production client's latency; unsupported observations remain unverified. The current one-second refetch coalescer is a measurement risk, not evidence that the delivery target already passes.

For resource lifetimes, retain peak and post-cleanup samples in one long-lived process across repeated complete cycles. Wait for explicit cleanup conditions with deadlines. Attribute bounded caches separately from live retained resources. Where an engine cannot provide a particular heap counter, state the missing observation and use supported process/resource counters without claiming equivalence.

Cold full gates run in three isolated disposable repositories, each with main at the immediate parent of the last code-change commit and the change branch at that commit. Each tests one change, verifies `scope: full` and `check list: full`, and performs the local fast-forward and finish to release the gate lock. They never advance the real main branch. Declare isolated application/compiler caches and retained dependency/browser downloads precisely; do not clear global or owner caches or claim a cold OS page cache. A failed assertion is not eligible for an unchanged-tree gate retry. The final docs-only closure has its own gate and must not alter the measured production code.

## Device and session coordination

At most three working sessions total, including the coordinator and active work in other projects. Recheck chat status and local process load before dispatching work and before each heavy command. When another project is active, only one OVC reviewer works alongside the coordinator. Read-only reviews may overlap only within that limit; shared files have one writer.

Builds, browser suites, gates and measurements run one at a time across the machine. Performance samples require otherwise idle hardware; record CPU, memory pressure, active relevant processes without their arguments/environment, tool versions, service configuration and test-run locks before and through the run. Do not terminate other sessions or owner applications. Retain a noisy or interrupted sample with its cause and replace the complete measurement only once the cause is removed. A missing tool, busy device or unsupported observation is blocked/unverified, not passed.

## Files and completion criteria

Planned records: this plan, `knowledge/reviews/slice-6-review.md`, per-layer review records, `knowledge/reviews/slice-6-known-limitations.md`, dated performance and cold-check-budget records under `knowledge/analysis/`, and the original plan's execution status. Measurement code is opt-in under `backend/internal/api/*_measurement_test.go` and frontend measurement scripts outside served output; any harness change also updates `docs/development.md` and `docs/threat-model.md` with pending owner review, mapping T-18/T-19/T-20/T-36/T-41 and checking observability-contract impact. Exact commands are documented before execution. Final human-facing closure updates `docs/roadmap.md`, with owner-review status preserved.

The slice closes only when every mandatory planned item has a recorded disposition, all applicable review profiles have evidence for the final code, no unresolved blocker or major remains, and each known limitation is fixed, accepted by the owner or assigned to a named later slice. New baseline-only or unverified measurements cannot be represented as passing targets. Existing unrelated future-slice exclusions remain in force.

The current 260-second merge ceiling and 165-second target are unchanged. Entry plus largest language above 176 KB requires owner consultation; 180 KB is the hard limit, with the existing Home/Today budgets unchanged. After the three cold full runs, the owner chooses the post-slice threshold from evidence. Neither the closure plan nor a passing F3 gate pre-approves a budget increase, a retry, a contract change or acceptance of a limitation.
