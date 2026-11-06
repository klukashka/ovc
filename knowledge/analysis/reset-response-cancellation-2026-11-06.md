# Trigger of the intermittent Chromium password-reset end-to-end failure

Measured on 2026-11-06 at main `57bf16d`. Chromium as pinned by Playwright 1.63.0. Threats: T-20 (the memory limit of the drain) and T-41 (verification integrity).

## Finding

`frontend/e2e/signup.spec.ts`, "resetting the password in a browser signed in to that account ends its session there", failed in Chromium because the application drained its 401 responses with a stream reader. When that reader reached the end of the body, Chromium sometimes reported the request as cancelled at the same moment. The test then could not read the body or see the request finish through DevTools. The application itself had received the whole body and showed the correct signed-out state.

The failure took two forms, depending on which of the two parallel 401 responses was cancelled:

- `/users/me`: `response.json()` failed with `Network.getResponseBody: No data found for resource with given identifier`.
- `/auth/refresh`: `response.finished()` never settled, and the test ran into its 30-second timeout. Route cleanup errors that followed were a consequence of the closed page.

The stream reader in `drainOwnedResponse` arrived with commit `a3b7d60` on 2026-11-04, and the recorded full-gate failures of this test start around the same date.

## Evidence

A full-suite run was not needed. Repeating this one test in Chromium reproduces it, from the repository root:

```bash
tools/dev-env.sh pnpm --dir frontend e2e --project=chromium --repeat-each=200 -g "resetting the password in a browser signed in" signup.spec.ts
```

Raw logs were kept only for the duration of the investigation. Machine load was not recorded for the first four runs.

| Run | Application drain | Instrumentation | Repeats | Failed |
|---|---|---|---|---|
| Unchanged test, main | stream reader | none | 200 | 15 |
| Raw DevTools event log | stream reader | DevTools session | 120 | 19 |
| Raw log and in-page log | stream reader | DevTools session, wrapped page APIs | 100 | 13 |
| Experiment | `arrayBuffer()` for every body | as above | 300 | 0 |
| This change, unchanged test | `arrayBuffer()` for a declared body | none | 300 | 0 |

In each failing repeat inspected, the raw DevTools log shows `responseReceived`, `dataReceived` with the full length, then `loadingFailed` with `net::ERR_ABORTED` and `canceled=true`, all within about a millisecond. There is no `loadingFinished`. In the same repeats the page log shows the reader returning all bytes and then `done=true`, and it shows no call to `AbortController.abort`, `ReadableStream.cancel` or `ReadableStreamDefaultReader.cancel`, and no abort event on the request signal.

## What is inferred, not measured

No Chromium source or network log was examined. The likely mechanism is an order race inside Chromium: the end of the body becomes visible to the stream before the loader's completion arrives, and closing the stream cancels a loader that has not completed. This explanation fits the evidence above but is not proven. The repeat counts show the changed drain removes the failure for this test. They do not show that no other cause of browser diagnostics exists.

The run-to-run failure rate depends on machine load. The last run of this change ran at a load average of about 13.

## Change

`drainOwnedResponse` reads a body whole when the response declares a `Content-Length` within 64 KiB and neither `Content-Encoding` nor `Transfer-Encoding`. Every other body keeps the streamed drain with its hard limit. The test and its assertions are unchanged.

The memory limit changes in kind on the whole-read path. A probe over raw HTTP/1.1 in Chromium, Firefox and WebKit found that a small `Content-Length` truncates a longer body, a body with no `Content-Encoding` is not decompressed, and a small `Content-Length` with `Transfer-Encoding: chunked` delivers the whole body, which is why that header routes to streaming. The same probe saw `Transfer-Encoding` in the response headers in all three browsers. The client is same-origin; a cross-origin response would hide the encoding headers but still expose `Content-Length`, so such a deployment would need this rule changed. A mismatch the headers do not reveal is detected as overflow after the read, not prevented, and HTTP/2 mismatches were not probed.

## Not covered

A body with no declared length or with an encoding header keeps the stream reader and can still show the same cancellation; no such 401 occurs behind the current local proxy, and a deployed proxy that compresses or chunks these responses would bring it back. The join-request console warning that also failed the local gate of the unmerged meeting recovery activation has a separate, still unknown cause. Slice 6's other deferred acceptance items in [the accepted-state record](../reviews/slice-6-accepted-state.md) are unaffected.
