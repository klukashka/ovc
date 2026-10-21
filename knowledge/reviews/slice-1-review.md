# Slice 1 review

The slice review of [roadmap slice 1](../../docs/roadmap.md#slice-1-scaffold-and-local-loop), run on 2026-10-21 over everything merged after the commit "Introduce tiered verification" up to the commit "Add the frontend scaffold with frames, typed client and browser smoke", following [ADR-0011](../../docs/decisions/0011-tiered-verification.md). It records what was reviewed, what was fixed and where each remaining finding goes.

## Scope and reviews

Four components, each reviewed by a fresh independent agent per profile, against a shared local deployment of the production frontend build, the API and the worker on a throwaway test database:

| Component | Correctness | Security | Browser | Performance |
|---|---|---|---|---|
| Development tooling, supply chain and agent policy | close after fixes (two major) | close after fixes (one major) | not applicable | not applicable |
| Backend process runtime | close after fixes | close after fixes | not applicable | close after fixes (with the HTTP surface) |
| Backend HTTP surface | close after fixes | can close | not applicable | see the runtime |
| Frontend application | close after fixes (one major) | can close | close after fixes (the same major) | can close |

The major findings were: a lint rule against changing environment variables in tests that never matched; discarded errors not refused by errcheck; documentation checks fetching unpinned, unscanned tools with `npx`; and frontend spacing at 15/16 of the tokens above 768 px, which left targets below 44 px. Every finding is now fixed, carried over to a named slice or closed with a reason, and the re-reviews of the follow-up changes left no blocker or major finding open. Performance found no leaks; the first measured baselines are in [the slice 1 baselines](../analysis/slice-1-baselines-2026-10-21.md).

## Fixed in follow-up changes

Each change had its own standard and security reviews, with fresh reviewers scoped to the fixes after major findings.

- **"Enforce the repository rules the slice 1 review found unchecked":** the lint rules and a test proving they fire, comment checks extended to TOML, Python, ignore files and `.env.example` files (an earlier deferred item), the documentation tools pinned in `tools/docs`, the secret guard hook refusing the common forms of commands that print the environment or credentials, the end-to-end harness, threat-model entries T-19, T-39 and T-41 and the after-fix rule.
- **"Harden the backend runtime and HTTP surface after the slice 1 review":** secret redaction, database URL and driver environment checks, the test-control name check before connecting, timeouts and deadlines, shared readiness checks, logging delivery, structured errors on every handler path, test-control body handling, and the tests for each, including the earlier deferred items on trailing JSON, panic header reset, `core.Run` panics and body read timeouts.
- **"Fix frontend frame sizing, focus and font loading after the slice 1 review":** spacing and target sizes, the current item colour, focus after showing the sidebar, the overlay's name and resize behaviour, the account menu, focus return after a native close of the overlay, readiness status, the favicon and parallel font loading.

## Carried over

| Finding | Where it goes |
|---|---|
| Route guards, the signed-in role replacing the link-state frame role, token pages reading their token and removing it from the address bar, and the `next` parameter rule (the interim behavior, replaced by the [navigation rules](../design/web/navigation.md#rules)) | Slice 2 |
| Lint rules refusing `dangerouslySetInnerHTML`, `innerHTML`, `document.write` and Web Storage outside the preferences store, with a smoke check that stored keys are known preferences | Slice 2, before tokens exist in the browser |
| The error codes published as an enum in the OpenAPI document, so the frontend can type-check their translations | Slice 2 |
| Whether `POST /__test__/tick` requires `OVC_CLOCK=test` or ticks at system time when the worker runs on system time | Slice 2, with the first outbox job; recorded in the development guide and the notifications specification |
| Playwright traces: whether they stay on once test credentials and tokens exist, and that they stay local | Slice 2 |
| The `system` theme following later changes of the colour scheme | Slice 2, with the theme setting |
| A check that rejects Tailwind class names that produce no CSS | Slice 2, with the first real pages |
| Test database fixture errors that show the pgx error text (user, host and database, never the password) and the fixture's lack of a loopback check on the admin URL (T-36) | Slice 2, with the authentication fixtures, applying the end-to-end harness's loopback rule |
| Transport deadlines computed from the injected clock (T-20) | Slice 2, re-checked when session expiry first uses the shared test clock, and whenever a test clock reaches the API handler; today every process passes the system clock |
| Route and vendor code splitting, and loading only the active language | Slice 8, before the builder pages |
| The production reverse proxy: the page headers from `frontend/security-headers.ts`, long-lived caching of hashed assets, compression, answering the Go server's own protocol errors, redacting token segments in its access logs (T-27), and never forwarding `/__test__/` | Before OVC is first deployed, at the latest in slice 10 |
| The readiness indicator on Home polling a database-pinging route | Slice 10, when Home gets its real content |
| Icons at 20 px on phones | Slice 10 |
| Verifying focus return after a native close of the sidebar overlay in Safari | Slice 10, in the accessibility pass |
| The log queue dropping warnings and errors as readily as access records under a flood | Slice 10, with rate limits |

## Closed with a reason

- **The test control routes accept `Host: localhost`:** the header is compared as text and never resolved, a rebinding page sends its own host name, and a page on another local origin is stopped by the JSON preflight; recorded in the [test harness](../../docs/development.md#end-to-end-test-harness).
- **The secret guard hook does not recognise every form:** it matches command text and is documented as protection against accidental disclosure, with its limitations listed in [AI development verification](ai-development-setup.md#minimal-inventory).
