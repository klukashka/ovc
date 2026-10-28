# Request failure texts of the browser engines

Checked on 2026-10-28 on macOS only, with Playwright 1.63.0 and its bundled Chromium, Firefox and WebKit, by a probe page on a local HTTP server that never answers one path and resets the connection on another. The texts are what `request.failure().errorText` reports in a `requestfailed` event.

| Case | Chromium | Firefox | WebKit (macOS) |
|---|---|---|---|
| `fetch` cancelled by an `AbortController` | `net::ERR_ABORTED` | `NS_BINDING_ABORTED` | `cancelled` |
| A pending `fetch` when the page navigates away | no event | `NS_BINDING_ABORTED` | `cancelled` |
| A pending `fetch` when its browser context closes | no event | no event | no event |
| The server resets the connection | `net::ERR_EMPTY_RESPONSE` | `NS_ERROR_NET_RESET` | `The network connection was lost.` |

A navigation that Playwright itself cancels in WebKit reports `Provisiolal navigation canceled.`, with Playwright's own misspelling; this text is verified: it is emitted by Playwright's bundled code (`playwright-core/lib`, version 1.63.0), where it appears exactly so, and the diagnostics match it exactly. Not verified, because Linux is not available here: WebKit on Linux (WebKitGTK) reports a cancellation as `Load request cancelled`, as the review of the diagnostics stated. The diagnostics treat it as a cancellation too. A wrong spelling only fails closed: such a cancellation is then reported as a failed request and fails the test, so a Linux run that reports one shows the exact text to add.

The end-to-end diagnostics (`frontend/e2e/request-failures.ts`) treat exactly these five texts as cancellations, compared whole, never as a pattern, so a transport failure whose text contains "abort" or "cancel" (for example Chromium's `net::ERR_CONNECTION_ABORTED`) still fails a test. A cancellation is forgiven only when the page left: a navigation between the request's start and its failure, or right after the failure with no request in between, or a close of its context that a test announced through `diagnostics.closeContext` between the request's start and its failure. These texts may change with a browser release; recheck them when Playwright is upgraded.
