# Slice 1 performance baselines

Measured on 2026-10-21 during the [slice 1 review](../reviews/slice-1-review.md), before the follow-up fixes, on one developer machine: Apple M4 Pro, 12 cores, 24 GB, macOS, Go 1.27.3, Node 26.0.0, Playwright 1.63.0 browsers. Other desktop processes were running, so the numbers are a first reference, not budgets. Budgets are set in slice 10 of the [roadmap](../../docs/roadmap.md).

## Backend

Against the API with debug logging (each health request wrote an access record) and the PostgreSQL container on the same machine, keep-alive connections, three samples of 3000 requests each:

| Route | Concurrency | Throughput | p50 | p95 | p99 |
|---|---|---|---|---|---|
| `/api/v1/health/live` | 10 | 65,000–75,000 requests/s | 0.12–0.14 ms | 0.22–0.25 ms | 0.42–0.44 ms |
| `/api/v1/health/live` | 50 | about 100,000 requests/s | 0.38 ms | 1.13–1.17 ms | 1.39–1.53 ms |
| `/api/v1/health/ready` | 10 | 12,300–12,900 requests/s | 0.65–0.69 ms | 1.44 ms | 1.68–1.76 ms |
| `/api/v1/health/ready` | 50 | 20,400–22,300 requests/s | 2.1–2.3 ms | 3.1–4.3 ms | 3.4–6.5 ms |

- Memory: the API used 18 MB idle and at most 33 MB under load, returning to 15–23 MB; the worker stayed at 12 MB.
- Leaks: over five cycles of 12,000 requests goroutines returned to the baseline after each cycle and after shutdown, heap in use stayed between 5.3 and 5.5 MB, and file descriptors and connections returned to their idle values.
- Handler cost without output: about 1.3 µs and 29 allocations per health request at info level, 2.8 µs for a structured `not_found`; one info log record cost the caller about 240 ns.
- Logging under a flood: 200,000 records from 50 goroutines into a slow sink never blocked a caller, and every record was either written or counted as dropped.
- Before the fixes, 50 concurrent readiness requests held all 20 pool connections; readiness checks are now shared and reused for 1 s.

## Frontend

Production build served by `vite preview`, cold loads in fresh browser contexts at 1280 × 800 with a Russian locale (medians, seven samples; five when throttled):

| Browser | Page | First and largest contentful paint | Layout shift | Transfer |
|---|---|---|---|---|
| Chromium | `/` | 56 ms | under 0.001 | 221 KB, 10 requests |
| Chromium | `/today` | 64 ms | under 0.001 | 241 KB, 13 requests |
| Firefox | `/` and `/today` | 48 and 50 ms | 0 | 221 and 241 KB |
| WebKit | `/` and `/today` | 46 and 55 ms | 0 | 221 and 241 KB |
| Chromium, slow 4G (150 ms, 1.6 Mbit/s) and 4× CPU, 375 × 812 | `/` and `/today` | 1.18 and 1.19 s | under 0.001 | — |

- Bundle: one JavaScript chunk of 143 KB gzip (React DOM 45 %, React Router 20 %, i18next 9 %, TanStack Query 7 %, ICU formatting about 12 KB gzip, application code under 5 %), CSS 4.6 KB gzip. The proposed student budget in [NFR-2.4](../../docs/requirements/non-functional.md) is 250 KB compressed.
- Render-blocking resources: only `/theme-init.js` (under 1 KB) and the CSS, loaded in parallel.
- Runtime: route changes and sidebar toggles completed in 16–40 ms (56 ms at 4× CPU); over 1200 route changes DOM nodes and listeners stayed flat and the heap levelled off.
- Fonts: before the follow-up fix, Russian pages downloaded the Latin face first and the Cyrillic face only afterwards, about 300 ms later on slow 4G; the faces now carry their unicode ranges and load in parallel.
