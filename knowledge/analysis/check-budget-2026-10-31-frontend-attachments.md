# Chat attachments and presence check budget

Status: component and browser change checks passed before gating; the first gate failed a browser fixture assertion; the corrected browser suite passed; the amended commit and new-tree gate remain pending. Base: `29fc9090ea3c9ff92b0be15fc45e2289a04d09b8`.

The initial build reports 175.4 KB gzip for entry plus the largest language chunk (164.1 KB static imports and 11.3 KB language), compared with 175.2 KB before F3. This remains below the 176 KB owner-consultation checkpoint and 180 KB hard limit. Student Home is 190.7 KB of 250 KB; tutor Today is 191.2 KB of 400 KB. Messages, RU/EN chat texts and the browser upload transport load on demand; 22 page modules remain lazy.

The complete component run passes 655 tests in 62 files in 6.08 seconds outside the process-inspection sandbox. Seven new browser tests per engine cover the parent plan's F3 scenarios; initial browser failures and their causes are recorded in the [verification record](../reviews/slice-6-frontend-attachments-and-presence-review.md). No assertion, browser or timeout has been removed to meet the budget.

The slice gate limit remains 260 seconds. Heavy runs are serialized within this change; another project can become active independently, so checks inspect both chat status and process load. Final gate evidence is required before merge, and slice-close performance measurements require an otherwise idle machine.

## Review corrections

The 2026-11-01 corrected build reports 175.5 KB entry plus largest language (164.1 + 11.3 KB), 190.8 KB Home and 191.3 KB Today. All remain within their limits and below the owner's 176 KB checkpoint. The complete component suite passes 664 tests in 63 files in 5.98 seconds.

The seven planned F3 browser cases gained two mandatory security-boundary cases per engine after independent review: the shared fake/production upload contract and multipart refresh/cancellation/account-switch lifecycle. They consolidate several assertions in two scenarios, preserving all original coverage. The corrected 21-case run passed in 16.9 seconds. The later 27-case run's 15.4-minute duration includes a confirmed 15-minute system sleep and two failed tests and is not budget evidence; see the verification record. The final full E2E and gate remain pending.

The final complete browser run passed all 944 cases in 2.5 minutes after deterministic fixture synchronization corrections. The final targeted 69-case run passed in 22.8 seconds. Another project was idle during the final full run; these execution durations do not replace the separate slice-close performance measurements. No production source changed after independent after-fix review. The exact-tree gate transcript under `.git/ovc-merge-reports/` supplies the final wall time and command statuses after this commit.

The first exact-tree gate took 241.9 seconds, within the 260-second ceiling, but failed one WebKit assertion (943 browser cases passed). This is failed-gate evidence, not readiness. All other 29 commands passed. The failed report and the event-synchronization correction are recorded in the verification record; no same-tree gate retry is permitted.

After adding explicit refusal-focus and request-cancellation event waits, all 944 browser cases passed in 2.4 minutes. The preceding 12-case targeted Messages run passed in 13.5 seconds. The earlier gate failure remains recorded; this passing change check does not authorize merging without a new-tree gate.
