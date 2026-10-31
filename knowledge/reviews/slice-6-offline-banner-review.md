# Shared offline banner readiness

Base: `6670bf3fe444c03b889035e1fca7eefb2409d170`.
Status: merged as `41bb7957dd0ffaf3c17c0ed6094b9a405c2814e5`; [full gate and finish evidence](slice-6-offline-banner-merge-2026-10-31.md) records all command results and tree equality.

## Scope and independent reviews

The [approved plan](../plans/slice-6-offline-banner.md) is a standard-tier change with the security trigger met. The implementation changes readiness notification, shared connection state, signed-in frame rendering, opt-in chat-write guarding, localized text and component/lifecycle tests. No backend, authentication protocol, dependency or test-control surface changes.

A fresh standard reviewer examined the complete tracked and untracked diff and the affected navigation/realtime contracts: passed, no blocker, major or minor finding. The reviewer noted missing direct evidence of persistent live-region DOM on route navigation; an additional frame test now asserts the same node, no text/child mutations and the route heading's expected focus.

A separate fresh security reviewer examined the complete diff: passed, no blocker, major or minor security finding. T-29 account epochs and stale callbacks and T-20 bounded timers were examined. Fixed translations and React rendering introduce no unsafe sinks or new diagnostics. Server authorization is unaffected. Uploads, conferencing, single-purpose links, answer visibility, CSP, test-control, dependencies and agent configuration are not affected. Neither reviewer ran execution checks; the coordinator owns that evidence. No security finding was fixed and no trust-boundary fix re-review is required.

## Execution evidence

- Initial typecheck failed on a TypeScript parameter property forbidden by erasableSyntaxOnly and DOM matchers absent from this repository. Replaced with a normal typed member and native DOM assertions; typecheck passed.
- Initial targeted tests found an ambiguous status query (fixture output elements also have status role) and a missing wait for the fake gateway's asynchronous close. The test now selects the unnamed banner and waits for scheduled grace before advancing time. The five targeted files then passed all 37 tests.
- Lint found missing explicit return types and confusing void expressions in new code/tests; fixed without configuration changes. Full lint and format check passed.
- First full component run failed six existing run-lock tests because the sandbox denied ps with EPERM. The same complete suite with permitted process inspection passed: 56 files, 593 tests, 5.13 seconds. No run-lock code or assertion changed. This was a change-check launch limitation, not a merge-gate attempt.
- Production build passed. Initial class checking interpreted the condition's offline state literal as a CSS class. Replaced conditional class selection with an exhaustive typed state-to-class mapping; no scanner exemption or configuration change. Class checking passed.
- After the navigation coverage addition and state-class mapping, typecheck and the four affected component/lifecycle test files passed: 37 tests. Production build, class checking and bundle checking passed.
- Entry plus largest language chunk: 173.4 KB gzip, below the 176 KB owner checkpoint and 180 KB hard limit. Student Home 188.5 KB; tutor Today 189.1 KB. Twenty-one page modules remain lazy.
- The first browser series passed 71 tests and failed one WebKit sign-out test: the new empty status role made the existing status locator ambiguous before routing finished. The banner now uses an explicit polite/atomic live region without an additional status role, preserving its announcements and existing status semantics. The first attempted edit did not match the formatter-compressed JSX, so another browser run repeated the same failure (71 passed, one failed); the source was then inspected and the exact attribute removed and verified before rebuilding. Both failures are retained. Assertions and browser timeouts are unchanged. Five consecutive three-engine series after the verified fix passed: 72 tests each, 360 executions total, in 19.3, 20.4, 19.1, 17.7 and 17.9 seconds. Raw logs are retained at /private/tmp/ovc-f1-e2e-final-1.log through -5.log.
- Final lint and format check passed. Tool-binary verification, full documentation checks (Markdown, links and diagrams), staged diff check and staged Gitleaks passed.
- All remaining full-run checks subsequently passed in the full merge gate, including 594 component tests and 902 E2E tests; see the merge evidence.

The initial local offline install selected an empty temporary pnpm store and failed on a missing tarball. Installation from the existing user store with frozen lockfiles succeeded offline; no dependency files changed.

## Limits

F1 fixtures exercise the shared guard and composer draft behavior. Real Messages Send/Edit/Delete/Attach paths belong to F2/F3 and remain pending there. Actual assistive-technology speech is unverified; DOM live-region and focus contracts are tested. This change does not close slice 6. Gate and finish confirmed the exact tested tree as recorded in the merge evidence.
