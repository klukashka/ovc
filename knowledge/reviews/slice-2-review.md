# Slice 2 review

The slice review of [roadmap slice 2](../../docs/roadmap.md#slice-2-accounts-sessions-and-email), run on 2026-10-21 and 2026-10-22 over everything merged after the commit "Record the slice 1 review and close the slice" up to the commit "Add the profile and account settings pages", following [ADR-0011](../../docs/decisions/0011-tiered-verification.md). It records what was reviewed, what was fixed, which minor findings were accepted and where each remaining item goes.

## Scope and reviews

Six components, each reviewed by a fresh independent agent per profile against the clean `main` tree, with the Go integration tests on the local PostgreSQL and storage server and the Playwright suites against the per-worker deployments:

| Component | Why it is included | Correctness | Security | Browser | Performance |
|---|---|---|---|---|---|
| Backend process runtime and HTTP surface | New settings, test control routes, trusted proxies, readiness with storage, `ovc-admin` | pass with minor findings | pass with minor findings | not applicable | with the backend as a whole |
| Backend accounts and sessions | Sign-up, sign-in throttling, refresh rotation, passwords, deletion | ready with minor findings | no blocker or major | not applicable | with the backend as a whole |
| Backend background work, email and storage | Outbox, worker jobs, mail adapters, avatars, purge | changes requested (one major) | changes requested (one major) | not applicable | with the backend as a whole |
| Backend as a whole | Load, memory and query plans across the components above | not applicable | not applicable | not applicable | not ready (two major) |
| Frontend application | Session store, route guards, auth, token and settings pages | ready with minor findings | pass with one minor | changes requested (one major) | pass with minor findings |
| Development tooling, supply chain and agent policy | Settings wrapper, secret guard hook, end-to-end harness, new dependencies | pass with minor findings | pass with minor findings | not applicable | not applicable |

Integration boundaries were reviewed by the component on each side: outbox rows written by accounts and files and read by the worker (accounts and background work), the refresh cookie and the error codes between the API and the session store (accounts and frontend), and the harness's test control routes (runtime and tooling).

The major findings were:

- a failing job ended the worker pass and the worker process, so one account whose purge always failed held up every other job and all email dispatch;
- a storage deletion that ended as dead left its object in storage, and a later account purge removed the attachment row without deleting the object (T-34);
- correct sign-ins from one address in parallel filled the per-address counter, so more than 20 at once locked that address out for 15 minutes with no failed password;
- avatars were read whole into memory for every anonymous request, so memory grew with object size times concurrency (T-20);
- the sign-up result and the reset request result were silent for screen readers, because focus fell to the page body with no live region.

Every major finding is now fixed, and the one re-review allowed after fixes ([ADR-0011](../../docs/decisions/0011-tiered-verification.md)) left no blocker or major open. Minor findings were fixed in the same follow-up changes, except those accepted in [slice 2 known limitations](slice-2-known-limitations.md) and placed below.

## Fixed in follow-up changes

Each change had its own standard reviewer, and a security reviewer where it met the security trigger.

- **"Keep the worker and the purge running past individual failures":** a failed job is recorded and logged and the pass continues with the other jobs and the lanes; the purge queues its own deletion row for each upload even when an earlier deletion ended as dead, and runs when the attachment row is already gone; T-34 updated.
- **"Stream avatars instead of holding them in memory":** avatars stream from storage with `Content-Length` from the stored size, at most 32 at once per instance, and a size mismatch aborts the response; T-20 updated.
- **"Stop counting throttled sign-in attempts and store only canonical zones":** a refused attempt releases its slot, so the counters measure failed and in-flight attempts only; timezones are stored only as canonical IANA zone names.
- **"Tighten the process runtime guards around test control and proxies":** test control requires loopback storage, trusted proxy ranges are bounded in width and never IPv4-mapped, `ovc-admin` refuses test control, and process tests prove each refusal; T-20, T-22 and T-36 updated.
- **"Keep the browser session consistent across late answers and failed loads":** answers that arrive after the session changed are dropped, a failed account load ends as unavailable, and the transport fake and its contract cover the new paths.
- **"Fix focus, targets and controls found in the slice 2 browser review":** focus moves to the result after a form is replaced, targets of 44 px, selects of the same height in WebKit, a skip link, focus after closing dialogs, and an accessibility suite in all three browsers.
- **"Harden the development tooling after the slice 2 review":** the settings wrapper and the secret guard hook close the shell features that could print the loaded settings, wrapper checks that pass on bash 3.2 and 5.2, per-worker log names, and the end-to-end bucket removed after each run.
- **"Replace the counting hasher and storage wrapper with checked fakes":** the test doubles that wrapped production code became fakes checked by contract tests, as AGENTS.md requires.
- **"Clean up finished sessions and old email requests hourly":** sessions are deleted a day after they expire or are revoked, email requests and their tokens 7 days after the request, and the outbox cleanup uses one statement per status so both use the partial index.
- **"Keep one cooldown timer and drop the readiness query at once":** the resend cooldown keeps one interval and stops it when the cooldown ends, and the readiness query is dropped when Home unmounts.

## Accepted minor findings

Each entry in [slice 2 known limitations](slice-2-known-limitations.md) was checked again against `main` at the commit "Keep one cooldown timer and drop the readiness query at once" and still holds as described.

| Finding | Where it goes |
|---|---|
| The secret guard hook and the settings wrapper miss some forms (an earlier assignment of an exported variable, an exported `unset` function on bash 4 and later, inner interactive shells reading startup files) and refuse some harmless commands | Closed with a reason, accepted by the owner on 2026-10-22: both guard against accidental disclosure only, and their limits are listed in [AI development verification](ai-development-setup.md#minimal-inventory) |
| A fast series of language choices on a public page can reach the server out of order | Slice 10, with the accessibility and hardening pass |
| `start` ends the session as unavailable after a failed start-up without checking the epoch | Slice 4, when the WebSocket revocation signal can first replace the session while `start` runs |
| Recording and analytics behaviour still described without an after-the-MVP note in the data model, files, accounts and non-functional requirements and in the BigBlueButton research file | After the MVP, corrected when recordings and analytics are planned, as the roadmap's after-the-MVP section states |
| The purge race test's upload goroutine may still be settling when the test database is dropped after a failure | Slice 3, with the purge effects on relationships, which touch the same test |

## Carried over

| Item | Where it goes |
|---|---|
| The worker's per-lane refill loops, so one slow effect no longer holds its whole batch | Slice 4, already in the roadmap |
| Route and vendor code splitting and loading only the active language; the build now warns about one chunk over 500 kB raw | Slice 8, already in the roadmap |
| A backend load test on the pilot deployment, repeating the sign-in and avatar measurements below | Slice 10, with the deployment |
| A real SMTP connection pool under load, and leaks across real process start and stop | Slice 10, with the deployment |

## Closed with a reason

- **`X-Content-Type-Options: nosniff` on API responses:** the API specification requires it on every response, and the outermost middleware sets it before any handler runs; the only open question is the one about signed storage downloads, recorded in [the detailed architecture](../architecture/detailed.md).

## Performance baselines

Measured on the development machine during the review and after the fixes; the machine was not idle, so these are indicative, not budgets.

- **Password hashing:** with 2 CPUs, as on the pilot application VM, about 115 hashes a second; NFR-2.1 holds up to about 50 concurrent sign-ins.
- **Query plans:** every hot query uses an index, each under 1 ms on 50,000 users and 200,000 sessions, and every foreign key used by a cascade is indexed.
- **Frontend bundle:** 161 KB gzip of JavaScript and 6 KB of CSS in one chunk, against 143 KB at the end of slice 1 and the proposed 250 KB student budget in NFR-2.4.
- **Start-up:** a signed-in start-up makes three API round trips one after another (refresh, account, readiness); the signed-in shell renders at about 72 ms locally and 1.6 s on slow 4G with a 4× slower CPU; layout shift stays below 0.001.
- **Leaks:** across 20 sign-in and sign-out cycles in one tab the DOM, listeners, timers and Web Locks returned to their starting counts; in the backend, goroutines, file descriptors and pool connections returned to their starting values.
- **Local checks:** 87 s in total ([check budget at the end of slice 2](../analysis/check-budget-2026-10-22.md)).

## Unverified

- Behaviour behind the production reverse proxy and on the pilot deployment, which do not exist yet.
- Interlaced PNG and lossless WebP decode memory peaks: no generator was available.
- Frontend performance in Firefox and WebKit; the measurements above are Chromium only.
- Delivery through a real SMTP provider; mail ran through the database sink and an in-process SMTP server.
