# End-to-end flakiness from loopback connection stalls (2026-10-26)

Status: agent analysis, 2026-10-26. Evidence for the change that makes the development and preview servers reuse their connections to the API.

## Symptom

Full runs of `tools/dev-env.sh pnpm e2e` (646 tests in Chromium, Firefox and WebKit, six Playwright workers, each with its own deployment) failed 2 to 13 different tests per run, mostly in Firefox and WebKit. The failures looked unrelated: 5-second `toBeVisible`, `toHaveText` and `toHaveCount` timeouts with buttons still busy, `toHaveURL` still on `/invite`, `page.goto: NS_ERROR_NET_TIMEOUT`, request-failure lists in the console and network verification of the diagnostics fixture, `TimeoutError: The operation was aborted due to timeout` in seeding helpers, and `api migrate` exiting with `process.failed kind=timeout` while a deployment started.

## Root cause

For several seconds at a time, no new TCP connection over the host's loopback interface completed, for any process and any destination. The browsers' requests, the preview proxy's requests to the API, the seeding requests of the Playwright workers and `api migrate` all stalled together and resumed together.

Evidence, from runs on `main` at `fb89585` with diagnostic probes outside the repository: a loop that opened a new HTTP connection every 200 ms to each target and logged attempts slower than 500 ms, `pg_isready` in the same loop, `netstat -an -p tcp` counts of socket states once per second, `vm_stat 1`, and `sample` of the API processes:

- A probe outside the Playwright process tree opened a new connection every 200 ms to a standalone Python HTTP server, to two API processes and, with `pg_isready`, to the PostgreSQL port that Docker forwards. All four stalled at the same moments, for 2, 4, 6 or 8 seconds (the SYN retransmission schedule), roughly every 13 seconds while the suite ran; some attempts ended in `ETIMEDOUT` after 7.8 seconds. The probe's own event loop never lagged.
- During every stall, `netstat` showed 17 to 21 sockets stuck in `SYN_SENT`; outside stalls there were none. The run held 3,000 to 4,000 sockets in `TIME_WAIT` (with the 30-second `TIME_WAIT` of macOS, more than 100 new loopback connections per second), far below the 16,384 ephemeral ports, and the mbuf statistics showed no exhaustion.
- `sample` of an API process taken during a stall showed it idle, its network poller waiting in `kevent`: the connections never reached it. The API logs of all six deployments of a browser project show the same silent gap of 4 to 8 seconds, and a PostgreSQL probe over an already open connection, inside the container and from the host, saw no gap at all.
- In every failing trace, the requests that never got an answer were those sent during a stall; the request that started the page's wait (a sign-in, a token refresh, a decline) is proxied by `vite preview` but never logged by the API.

The machine runs a VPN client whose macOS kill switch uses the packet filter. A packet filter that keeps state for loopback traffic holds an entry per connection for tens of seconds after it closes, and drops the SYN of new connections while its state table is full, which matches every observation above. Confirming the ruleset and its state counters needs `sudo pfctl -s info`, which was not run, so this mechanism is unverified; the stalls, their correlation with the connection rate and their removal by the change are measured.

## Contributing cause in the repository

The development and preview servers proxy `/api` through Vite's bundled `http-proxy-3`. Without an `agent`, it sends every proxied request with `Connection: close` on a new connection, so each API call of every page opened and closed one loopback connection. The realtime client and the notification bell add a socket and refetches per page, which most likely raised the rate further; that increase was not measured on its own.

## Change and result

The `/api` proxy of both servers now uses a keep-alive `http.Agent` with a 60-second socket timeout. The agent destroys a pooled socket once it has been idle that long, shorter than the API's 120-second idle timeout, so the proxy never reuses a connection the API closed for idleness; on a socket carrying a request the timeout has no effect, because the proxy sets no proxy timeout and the agent only destroys free sockets, so slow requests and uploads are not cut. A connection the API closes for another reason is announced with `Connection: close` or dropped from the pool when its close arrives, which leaves only a narrow race in which a reused socket yields a `502`. The WebSocket route is unchanged.

| Measurement during a full run | Before | After |
|---|---|---|
| Peak sockets in `TIME_WAIT` | 4,021 | 1,887 (including the probe's own connections) |
| Seconds with a socket stuck in `SYN_SENT` | 54 | 0 |
| Probe connections slower than 500 ms | 51 | 0 |
| Failed tests per full run | 2 to 13 in each of seven runs, 0 in two | 0 in each of three consecutive runs, and 0 in each of three runs with the shared end-to-end actors of the test database template change merged |

The change is a mitigation: the host condition remains, and the repository now stays below the connection rate that triggers it on this machine. Heavier load, more workers or another session testing at the same time can bring the stalls back; the after-figures come from six runs.

## Hypotheses ruled out

- The realtime client or the notification bell slowing pages or leaving requests in flight at navigation: the hung requests reached no server, in pages with and without a socket, and the stalls hit processes the browsers never touch.
- A stalled preview server or API process: both were idle during stalls, and stalls hit all six deployments of a browser project at once.
- PostgreSQL commit or checkpoint stalls, Docker port forwarding and host memory pressure: probes over open connections saw no gap, and `vm_stat` showed no paging burst aligned with the stalls.
- Firefox opening a second HTTP/2 session when the server does not offer extended `CONNECT` for WebSockets: it does, and then uses HTTP/1.1 for the socket, but the extra session stays idle in passing runs and plays no part in the stalls.
