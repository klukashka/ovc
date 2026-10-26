# Local check budget after end-to-end account seeding

Measured on 2026-10-26 for the account seeding change of the [slice 4 review follow-ups](../plans/slice-4-review-follow-ups.md) (the change that adds the `/__test__/accounts` and `/__test__/session` routes), on its branch based on `main` at `7795199`. The targets are a cold full local check run on an idle machine, the security scans included, at 180 seconds or less, and slice 5's D46 target of 149 seconds ([slice 5 plan](../plans/slice-5.md)). The comparison point is the cold run of the [record after the slice 4 review](check-budget-2026-10-26-after-slice-4-review.md): 237.1 seconds, of which 108.9 in `pnpm e2e`.

## Method

- **Machine:** Apple M4 Pro, 12 cores, 24 GB, macOS 26.2, Go 1.27.3, Node 26.12.1; Docker Desktop running the Compose PostgreSQL 18 and storage services. No other session ran checks during the run.
- **Commands:** as in the comparison record, one script ran every command of the [development guide](../../docs/development.md) one after another and timed each with Python's `time.monotonic()`: the documentation checks, Gitleaks over history and over staged changes, the four OSV-Scanner scans through `go -C tools tool`, the build of the pinned linters, the backend checks with `go test -race` through `tools/dev-env.sh`, the frontend checks with `pnpm e2e` through `tools/dev-env.sh` (with the `dot` and `json` reporters), and the secret guard hook and local settings wrapper checks. Every command exited 0; the end-to-end suites passed 660 tests.
- **Cold:** Go and golangci-lint started from empty caches (`GOCACHE` and `GOLANGCI_LINT_CACHE` set to new empty folders); the pnpm store, `node_modules` and the Vite and TypeScript caches were left as they were, as in the comparison record.
- **Machine state:** the run started once the one-minute load average was below 2.5; the load averages at the start were 2.29, 4.86 and 5.21, lower in the 15-minute figure than before the comparison record's cold run (30.49).
- **Not covered:** the change that prebuilds Gitleaks, OSV-Scanner and sqlc into `bin/` was not merged when this run was taken. Its script `tools/check-tool-binaries.sh` is not part of this run, and the scanners and sqlc still ran through `go -C tools tool`, so its saving of about 31 seconds of cold compilation is not in these numbers; both are measured with that change's merge.

## Results

| Command | Cold (s) | Comparison cold (s) |
|---|---|---|
| Documentation checks (install and `check-docs.sh`) | 6.1 | 6.0 |
| Gitleaks over history, over staged changes | 5.7, 0.6 | 6.0, 0.6 |
| OSV-Scanner: tools, backend, frontend, documentation tools | 13.1, 2.1, 1.7, 1.7 | 13.0, 2.0, 1.6, 1.7 |
| Build of the pinned linters | 5.3 | 5.2 |
| `go mod tidy -diff`, `gofmt`, `go vet` | 0.5, 0.0, 4.2 | 0.4, 0.0, 4.3 |
| golangci-lint, govulncheck | 4.4, 2.7 | 4.3, 2.8 |
| `go test -race -count=1 ./...` | 45.5 | 44.2 |
| `sqlc diff` | 14.3 | 13.7 |
| `pnpm install`, `typecheck`, `lint`, `format:check`, `test` | 0.1, 0.1, 10.2, 1.4, 4.5 | 0.0, 0.1, 9.1, 1.3, 4.1 |
| `build`, `check:classes`, `check:bundle`, `api:check`, `audit` | 0.3, 0.6, 0.1, 1.8, 0.6 | 0.3, 0.6, 0.1, 1.2, 0.5 |
| `pnpm e2e` (660 tests) | 88.2 | 108.9 |
| Secret guard hook checks, local settings wrapper check | 4.4, 0.8 | 4.1, 0.9 |
| **Total** | **221.0** | **237.1** |

- **End-to-end suites:** 87.9 seconds of wall time in the reporter and 462 seconds of summed test time (113 in Chromium, 198 in Firefox, 151 in WebKit), against 108.6 to 114.9 seconds and 578 to 611 seconds in the comparison record's runs.
- **Everything else** took 132.8 seconds against 128.2; part of the one-second rise of `go test` comes from the three new process tests of the seeding routes, and the rest is within the run-to-run spread.
- **A warm run** of the same tree before the review fixes took 173.1 seconds in all, 85.3 of them in `pnpm e2e`; it is not the measured run.

## Where the end-to-end time goes now

From the API logs of the measured run (request durations as the API logs them; the WebSocket route is left out because its duration is the socket's lifetime):

| Route | Requests | Total (s) | Mean (ms) |
|---|---|---|---|
| `POST /api/v1/join-requests` | 681 | 96.7 | 142 |
| `POST /api/v1/groups` | 663 | 53.6 | 81 |
| `POST /__test__/accounts` | 1,105 | 34.4 | 31 |
| `POST /__test__/session` | 1,821 | 22.9 | 13 |
| `POST /api/v1/auth/login` | 289 | 10.3 | 36 |
| `POST /api/v1/auth/register` | 87 | 3.5 | 40 |
| `POST /api/v1/auth/verify-email` | 123 | 3.1 | 25 |
| every other route except the socket upgrade | 7,820 | 65.9 | 8 |

- **Hashing left:** 87 registrations, 289 sign-ins and 123 confirmations, against 1,246, 2,302 and 1,419 in the comparison record's profile; they belong to the sign-up, confirmation, invitation, sign-in, password and deletion journeys and to the pilot scenario step and seed tests, which keep the real flows. The seeding routes hash once per API process.
- **The remaining API time** is dominated by `POST /api/v1/join-requests`, whose code entries now also seed the requests that the sign-up with a tutor code used to create, and by group creation; the suites' wall time is bound by the three browsers' processor time, as before.

## Against the targets

- **180 seconds, cold:** 221.0 seconds, 41 seconds over. **Failed** on this branch alone. Adding the expected saving of about 31 seconds from the prebuilt scanning tools would give about 190 seconds, still about 10 seconds over; that estimate is not a measurement.
- **149 seconds of slice 5 D46:** failed by 72 seconds, and by about 41 seconds with the expected saving of the prebuilt tools.
- **NFR-6.2:** the 10-minute budget is not at risk.
- **Saving of this change:** 20.7 seconds of `pnpm e2e` wall time and 116 to 149 seconds of summed test time in the measured cold runs, less than the 576 seconds of hashing removed from the API, because the hashing ran in parallel with browser work on the other cores.
