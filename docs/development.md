# Development

Status: draft written 2026-10-20, amended the same day with local services and the Go backend toolchain, and on 2026-10-21 with Docker Compose services, security scanning, the backend scaffold, the frontend scaffold and the pinned documentation tools, and the same day with the test fixture's host check, the sqlc check, the mail settings and test routes of the outbox worker, the local settings wrapper, and the backend OSV triage; pending line-by-line owner review. Amended 2026-10-21 with storage-backed tests run through the local settings wrapper (pending owner review). Amended 2026-10-22 with the local storage bucket, the gosec G101 exclusion, worker-indexed end-to-end logs and the end-to-end bucket cleanup (pending owner review). Amended the same day with the layer-based slice review and the single after-fix re-review, and with the recording test routes moved after the MVP (pending owner review). Amended the same day with the field encryption key and the `check-settings` subcommand (pending owner review). Amended 2026-10-24 with the frontend dependency triage link (pending owner review).

How to set up, check and verify changes locally. Read [AGENTS.md](../AGENTS.md) before contributing. Backend and frontend commands were registered here by [roadmap slice 1](roadmap.md#slice-1-scaffold-and-local-loop); later slices extend them.

## Tool setup

Use Linux or macOS with Git, Bash, Docker with Compose v2 (Docker Desktop, Colima or OrbStack all work), Node and pnpm (see below), a system `python3` (3.9 or newer, for the agent hooks in `.claude/hooks/`), a C compiler (for `go test -race`, which needs cgo) and the pinned toolchain:

- Go: the version in [`tools/go.mod`](../tools/go.mod) (`go` and `toolchain` lines, matched by `backend/go.mod`), installed from [the official downloads](https://go.dev/dl/) or a package manager; the `toolchain` line lets newer Go installations fetch the pinned release.
- Repository tools (Gitleaks and OSV-Scanner now; govulncheck and golangci-lint, built from the pinned versions into the ignored `bin/` directory because they must type-check the backend module; sqlc, which generates `backend/internal/dbgen` from `backend/queries/` and `backend/migrations/`): pinned as Go `tool` dependencies in `tools/go.mod`, with checksums in `tools/go.sum`, and run from the repository root as `go -C tools tool <tool>`. No separate installation is needed.
- Node and pnpm: Node from `frontend/.node-version` and pnpm from the `packageManager` field of `frontend/package.json` ([versions](../knowledge/research/frontend-toolchain-2026-10-21.md)). Playwright's browsers are installed once with `pnpm --dir frontend exec playwright install chromium firefox webkit`.
- lychee for link checks: installed with its [official instructions](https://lychee.cli.rs/installation/).

Exact versions are recorded with their check dates in `knowledge/research/` when the scaffold pins them.

## Documentation checks

Active now and required for every change. Install the pinned tools once per change of `tools/docs/pnpm-lock.yaml`, then run the script from the repository root:

```bash
pnpm --dir tools/docs install --frozen-lockfile
tools/docs/check-docs.sh
```

The script needs the Chromium that `pnpm --dir frontend exec playwright install chromium` installs and the lychee version recorded in [the toolchain research](../knowledge/research/toolchain-2026-10-21.md#documentation-tools), which it checks. It lints every tracked and non-ignored untracked Markdown file with markdownlint, checks relative links and heading anchors offline with lychee, and renders every Mermaid diagram; it exits non-zero on the first failure.

- markdownlint and the Mermaid renderer are pinned in `tools/docs/package.json` and its lockfile, under the same pnpm policy as the frontend (no dependency install scripts, a 14-day minimum release age; `puppeteer` is explicitly denied its browser download). The renderer uses the Chromium that Playwright installed for the frontend, found through `playwright-core` of the same version, so it downloads no browser of its own. `MD060` (table column style, new in markdownlint-cli 0.49) is turned off in `.markdownlint.json` because the repository's tables use the compact `|---|` delimiter that earlier versions accepted.
- lychee is installed with its [official instructions](https://lychee.cli.rs/installation/); the checked version is recorded in [the toolchain research](../knowledge/research/toolchain-2026-10-21.md).

## Security scanning

Run from the repository root; each must pass before a change is ready.

- **Secrets in history:** `go -C tools tool gitleaks git --redact --no-banner ..` reports no leaks.
- **Secrets about to be committed:** `go -C tools tool gitleaks git --staged --redact --no-banner ..` reports no leaks. Gitleaks never scans the working directory, so ignored environment files are never read.
- **Known vulnerabilities in dependencies:** OSV-Scanner over each module and lockfile with an explicit path, starting with `go -C tools tool osv-scanner scan source --no-call-analysis=go --lockfile=go.mod` (the tools module has no packages of its own to call-analyse) and `go -C tools tool osv-scanner scan source --no-call-analysis=go --lockfile=../backend/go.mod` (the backend gets call analysis from `govulncheck` in the backend checks); `go -C tools tool osv-scanner scan source --lockfile=../frontend/pnpm-lock.yaml`, and `go -C tools tool osv-scanner scan source --lockfile=docs/pnpm-lock.yaml`. A pass is exit code 0. Accepted findings are listed in the `osv-scanner.toml` next to the scanned file, each with a reason and an `ignoreUntil` date, and explained in a dated triage table ([Go tools](../knowledge/research/local-s3-2026-10-21.md#repository-tool-scan-triage), [documentation tools](../knowledge/research/toolchain-2026-10-21.md#documentation-tools), [frontend](../knowledge/research/frontend-toolchain-2026-10-21.md#accepted-findings) (pending owner review), [backend](../knowledge/research/backend-libraries-2026-10-21.md#notes)).

## Backend checks

Build the pinned linters once per tools upgrade, from the repository root:

```bash
go -C tools build -o ../bin/ github.com/golangci/golangci-lint/v2/cmd/golangci-lint golang.org/x/vuln/cmd/govulncheck
```

Then, from `backend/`, with Docker Compose running and `OVC_TEST_DATABASE_ADMIN_URL` set to a PostgreSQL URL that may create databases (for example the `ovc` user on `ovc_dev`); the test fixture refuses a URL whose hosts are not loopback addresses or Unix sockets:

```bash
go mod tidy -diff
test -z "$(gofmt -l .)"
go vet ./...
../bin/golangci-lint run ./...
../bin/govulncheck ./...
../tools/dev-env.sh go test -race -count=1 ./...
go -C ../tools tool sqlc -f ../backend/sqlc.yaml diff
```

After changing a query or a migration, regenerate the query package from `backend/` with `go -C ../tools tool sqlc -f ../backend/sqlc.yaml generate` and commit the result; `sqlc diff` fails while `internal/dbgen` is out of date.

Every command must exit 0. The tests include the package-layer architecture test (it parses the [package layers table](../knowledge/architecture/detailed.md#package-layers)), the no-comments and no-`any` conventions test, integration tests that each create and drop their own `ovc_<random>_test` database, and process tests that build the `api` and `worker` binaries and run them with an explicit environment. The storage tests use the local SeaweedFS bucket `ovc-test`, created when missing, with the keys from the root `.env` (`OVC_DEV_STORAGE_ACCESS_KEY`, `OVC_DEV_STORAGE_SECRET_KEY` and `OVC_DEV_STORAGE_PORT`), so `go test` runs through `tools/dev-env.sh`; the storage contract test runs against both SeaweedFS and the in-memory store. A missing or unreachable test database or storage server, or missing storage keys, fails the run; nothing is skipped. (Pending owner review.) The lint configuration excludes gosec's G204 (subprocess with variable arguments) and G304 (file path from a variable) for `_test.go` files only, because the process tests run the binaries they have just built and the convention tests read the repository's own files; production code keeps both rules. It also excludes G101 (hard-coded credentials) only for the lines of `internal/core/settings.go` and `internal/core/storage_settings.go` that assign an `OVC_…` setting name, because the names of secret settings such as `OVC_SMTP_PASSWORD` look like credentials to it, while their values are only ever read from the environment; any other literal in those files is still checked (pending owner review).

## Frontend checks

From `frontend/`:

```bash
pnpm install --frozen-lockfile
pnpm typecheck
pnpm lint
pnpm format:check
pnpm test
pnpm build
pnpm check:classes
pnpm api:check
pnpm audit --audit-level high
../tools/dev-env.sh pnpm e2e
```

Every command must exit 0.

- `lint` runs ESLint and the comment check, which rejects comments in every hand-written frontend file (scripts, styles, markup and configuration); the generated `src/lib/api/schema.ts` is exempt from both and from Prettier. The lint configuration forbids importing `vi` or `vitest` from Vitest at all, the test-double methods on them (`mock`, `doMock`, `importMock`, `mockObject`, `spyOn`, `fn`, `stubGlobal`, `stubEnv`, `useFakeTimers`, `setSystemTime`), importing `vitest` or its subpaths dynamically, replacing `fetch` by assignment, `Object.defineProperty`, `Object.defineProperties`, `Reflect.set` or `Object.assign`, the mocking libraries `msw`, `nock`, `sinon`, `testdouble`, `tinyspy` and any module named `*mock*`, lint and TypeScript directives and explicit `any`; a Vitest suite proves each rule fires. The list names known techniques and is not exhaustive: the rule in AGENTS.md still applies to anything it misses. Shell, YAML, TOML and Python files, ignore files, `.editorconfig` and `.env.example` files anywhere in the repository are covered by the backend conventions test.
- `lint` refuses the `Reflect` API in every hand-written TypeScript and JavaScript file, end-to-end page scripts included, as AGENTS.md's rule on precise types requires (pending owner review).
- `lint` also refuses, in `src/` and `public/`, HTML sinks, code from strings, script-readable cookies, IndexedDB and Web Storage outside the preference store, and any file under `src/` other than `.ts`, `.tsx` and `.css` (pending owner review).
- `check:classes` runs after `build`: it collects the class names written in `className` attributes (string and template literals, conditionals, and constants they name, including imported ones) with the TypeScript compiler and fails on any class that has no selector in the built CSS, so a mistyped or unregistered Tailwind class cannot pass silently. Class names are written as whole literal words; a fragment joined at run time (`` `pl-${size}` ``) and the arguments of a function call inside `className` are skipped, so class names belong in literals and constants, not in call arguments (pending owner review).
- `api:check` regenerates the typed client from `go run ./cmd/api openapi`, which reads no settings, and fails when the committed `src/lib/api/schema.ts` differs; `pnpm api:generate` rewrites it after an API change.
- `pnpm install` runs no dependency install scripts: `allowBuilds` in `pnpm-workspace.yaml` is empty and `strictDepBuilds` fails an install that would need one. `minimumReleaseAge` (14 days) makes pnpm refuse any direct or transitive version published less than two weeks before resolution, and `pnpm install --frozen-lockfile` re-verifies the lockfile against it.
- `e2e` is the Playwright smoke in Chromium, Firefox and WebKit. It needs Docker Compose running with the root `.env` in place (the harness calls `docker compose exec`, which interpolates the Compose file and so needs the `OVC_DEV_*` variables) and `OVC_TEST_DATABASE_ADMIN_URL` set as for the backend checks, and runs through `tools/dev-env.sh` so that the API and worker receive the local storage keys; the harness creates the bucket `ovc-e2e` through the storage container's shell when it is missing (pending owner review), pointing at the Compose PostgreSQL on a loopback host (the harness refuses any other host, because it creates and drops the database inside the Compose container). Its global setup builds the `api` and `worker` binaries and the frontend, creates the preview certificate and the storage bucket; then each Playwright worker creates an `ovc_e2e_<random>_test` database with `createdb` inside the PostgreSQL container (the password travels in the environment, never in arguments), runs `api migrate`, starts the API and the worker with an explicit environment (`OVC_TEST_CONTROL=1`, `OVC_CLOCK=test`, the shared clock set to the current time, and `OVC_TRUSTED_PROXY_CIDRS=127.0.0.1/32`, so a test that seeds accounts directly sends its own `X-Forwarded-For` from the TEST-NET range and never exhausts the per-IP limits) on `127.0.0.1:18800` plus its index, waits for `/api/v1/health/ready` with a deadline, and serves the production build with `vite preview` over HTTPS on `127.0.0.1:4300` plus its index. Teardown stops the processes and drops the database; a process that exited before teardown, or did not exit cleanly after `SIGTERM` (exit code 0 for the API and the worker; the preview server may also end by the signal itself), fails the run. Process output goes to `frontend/test-results/deployment/`, one log per process and Playwright worker, named with the worker's index so that a worker replaced after a failure keeps the failed deployment's logs (pending owner review). The global teardown deletes the shared `ovc-e2e` bucket, so uploaded objects do not accumulate across runs; the next run's setup creates it again (pending owner review). Each later slice adds its parts of the pilot scenario suites.

The development server (`pnpm dev`, `127.0.0.1:5173`) and the preview server (`127.0.0.1:4173`) listen on loopback only and proxy `/api` to `OVC_FRONTEND_API_TARGET` (default `http://127.0.0.1:8000`, documented in `frontend/.env.example`), so the browser talks to one origin as it will behind the production reverse proxy. Vite's own CORS handling is turned off on both servers (`cors: false`; by default Vite answers cross-origin requests from any `localhost` origin), and the smoke asserts on the preview server that neither a preflight nor a simple request from another local origin gets `Access-Control-Allow-Origin` (the development server shares the setting but is not exercised by the smoke). Both send `Referrer-Policy: no-referrer` and `X-Content-Type-Options: nosniff`; the preview server also sends the page Content Security Policy. The headers are defined once in [`frontend/security-headers.ts`](../frontend/security-headers.ts), which the production reverse-proxy configuration must reproduce. The development server sends no CSP because the React refresh runtime injects an inline script.

## Local services

The local environment has:

- **Docker Compose** with PostgreSQL 18 and SeaweedFS as the local S3-compatible server ([research](../knowledge/research/local-s3-2026-10-21.md)), both images pinned by digest in [`docker-compose.yml`](../docker-compose.yml).
  - Every port is published on `127.0.0.1` only; check with `docker compose ps --format '{{.Service}} {{.Ports}}'`, which must show `127.0.0.1:` before every published port.
  - The development credentials are required variables listed in [`.env.example`](../.env.example); copy it to the git-ignored `.env` next to it and fill in your own values. There are no default passwords, and PostgreSQL uses SCRAM authentication for every connection.
  - Host ports default to 55432 (PostgreSQL) and 58333 (storage), away from a PostgreSQL that may already run on 5432; `OVC_DEV_POSTGRES_PORT` and `OVC_DEV_STORAGE_PORT` change them.
  - SeaweedFS also runs its master, volume and filer services inside the container without authentication; they are not published to the host, and on Linux they are reachable from the host and other containers only on the Compose network. Development data only.
  - Agents never run commands that print the interpolated credentials or the environment. The secret guard hook refuses the common forms, including through wrappers, `sh -c` and container `exec`; it guards against accidental disclosure and is not a security boundary ([AI development verification](../knowledge/reviews/ai-development-setup.md#minimal-inventory)). After changing the hook, run `python3 .claude/hooks/check_guard_secrets.py`, which must report no failures.
  - The development database is `ovc_dev`. `docker compose up -d` starts both services; `docker compose ps` shows them `healthy` within a minute; `docker compose down` stops them and keeps the data volumes.
- **Local settings:** the owner keeps the Compose credentials in the root `.env` and the backend settings in `backend/.env`, copied from `backend/.env.example`. `tools/dev-env.sh <command>` runs one command with both loaded ([ADR-0009](decisions/0009-owner-approved-exceptions-and-visibility.md), item 7, until hosting exists). It accepts only plain `OVC_KEY=VALUE` lines and loads them literally, with no shell expansion; it refuses any other key, a quoted value and a key repeated within or across the files, naming only the file and line; a missing file is skipped; it never overrides a variable already set in the environment; it turns shell tracing off before reading, prints no value, and refuses to start `env`, `printenv`, `set`, `export`, `declare`, `typeset` or `compgen`, while the secret guard hook refuses the other printing forms it recognises. A value a command needs in its own arguments must be read inside it, for example with `sh -c '…'`, because the calling shell expands arguments before the wrapper loads anything. After changing the wrapper, run `tools/check-dev-env.sh`, which must exit 0; it checks every rule above on a temporary copy with made-up values. For example, `tools/dev-env.sh go -C backend run ./cmd/api` serves the API with the local settings.
- **Field encryption key:** the API and the worker, `api migrate` included, refuse to start without `OVC_FIELD_ENCRYPTION_KEY`, the AES-256-GCM key for stored invitation links: exactly 32 random bytes in standard base64. Generate it with `openssl rand -base64 32` and write the output into `backend/.env` as the unquoted value; a checkout from before the key existed needs this before its next local start of the API or the worker. Agents never open or edit that file: `tools/dev-env.sh go -C backend run ./cmd/api check-settings` and `tools/dev-env.sh go -C backend run ./cmd/worker check-settings` load the settings without connecting to anything and print only `ok` (exit 0) or the settings error, which names the variable and the problem and never a value. The key is not rotated in the MVP, because a new key makes every open invitation link unreadable and tutors then regenerate them. The process tests and the end-to-end harness generate their own key for every run and pass it to the processes in their environment, never in arguments, so no check depends on `backend/.env` (pending owner review).
- **The administration command** `go run ./cmd/ovc-admin users list` (and the other subcommands in the [accounts specification](../knowledge/specs/accounts.md#administration-command)) needs only `OVC_DATABASE_URL`; locally, run it through `tools/dev-env.sh`. Operators run it under their own personal system accounts, never a shared one, because the audit record names the system user (pending owner review).
- **Three commands**, all reading the same settings from the environment (through `tools/dev-env.sh`, or set by hand): `go run ./cmd/api migrate` applies the migrations with a role that may change the schema; `go run ./cmd/api` serves on `OVC_HTTP_ADDR` and refuses to start while migrations are pending; `go run ./cmd/worker` runs the `Tick(now)` loop. Restart them by hand after a change. The API and the worker need the bucket named in `OVC_STORAGE_BUCKET` (`ovc-dev` in `backend/.env.example`) on the local SeaweedFS server, with `OVC_STORAGE_ACCESS_KEY` and `OVC_STORAGE_SECRET_KEY` equal to the root `.env`'s `OVC_DEV_STORAGE_ACCESS_KEY` and `OVC_DEV_STORAGE_SECRET_KEY`; until the bucket exists, readiness reports storage unavailable. Create it once with `printf 's3.bucket.create -name ovc-dev\n' | docker compose exec -T storage weed shell` (pending owner review).
- **Fakes**:
  - the meeting provider is `fake`, because BigBlueButton runs only on a dedicated Ubuntu server and not on developer machines ([research](../knowledge/research/bigbluebutton-2026-10-20.md#server));
  - mail goes to the database sink. Development runs with `OVC_TEST_CONTROL=1` against a development database, so the sink is readable only through the harness route below.
- **Live conferencing checks**, from slice 7 on, use a remote BigBlueButton test server configured through `OVC_BBB_URL` and `OVC_BBB_SECRET` in `backend/.env.local`, which is git-ignored and unreadable by agents. They are manual: the opt-in command runs the BigBlueButton adapter's contract tests. A readiness report lists the result, or `unverified` when the check was not run. The definition of done does not require them.

Automated tests never call a real BigBlueButton server, SMTP provider or production storage.

## End-to-end test harness

Started in slice 1 (the clock and tick routes), extended by each slice. The [pilot scenarios](../knowledge/design/web/pilot-scenarios.md) run against local test deployments: the real API, worker, database and storage in local containers, configured with `OVC_TEST_CONTROL=1` and `OVC_CLOCK=test`.

- **Safety:**
  - the API, the worker and `ovc-admin` refuse to start with `OVC_TEST_CONTROL=1` unless the meeting provider is `fake`, mail is the database sink, `OVC_HTTP_ADDR` is a loopback IP literal, the storage endpoint of the API and the worker is a loopback IP literal, every database host is a loopback IP literal, `localhost` resolving only to loopback, or a Unix socket (checked before connecting), and `SELECT current_database()` matches `^[a-z0-9_]+_(test|dev)$`; they also refuse `OVC_CLOCK=test` without `OVC_TEST_CONTROL=1`;
  - the control routes require `Content-Type: application/json`, bodies of at most 4 KiB and a loopback `Host` header, so a page in the developer's browser or a rebinding host name cannot drive them; `localhost` is accepted beside the loopback IP literals because the check compares the header as a string and never resolves it, a rebinding page sends its own host name, and a page served from another local origin is stopped by the JSON content-type preflight, which the routes do not answer;
  - without the setting, none of the routes below exist, and a test proves it;
  - the worker also refuses `OVC_MAIL_BACKEND=sink`, the default, unless the database passes the same local host and name checks, so a production database never accumulates emails that carry tokens; a deployment sets `OVC_MAIL_BACKEND=smtp` with the SMTP settings.
- **Worker:** with `OVC_CLOCK=test` the worker's own loops are paused, so only harness ticks run jobs. Local development keeps `OVC_CLOCK=system`, so its worker loops run normally while the mail route stays available; the clock and tick routes exist only with `OVC_CLOCK=test`.
- **Clock:** the fake clock reads its current instant from a single test-clock row, so the API and the worker of one deployment share one time; each deployment starts it at the real current time. Suites that move the server clock and depend on the browser's own time install `page.clock` at the same instant (from the first suite that needs it; pending owner review).
- **Parallelism:** each Playwright worker gets its own deployment (own database, API and worker processes, and `vite preview`, on ports derived from the worker's index), so suites that move the clock never interfere. Suites assigned to one worker run serially. The global setup builds the binaries and the frontend once; every deployment shares the storage bucket `ovc-e2e`, because SeaweedFS allocates volumes per bucket and objects carry random keys anyway (pending owner review).
- **HTTPS preview:** WebKit refuses the `Secure` refresh cookie over plain `http://127.0.0.1`, so each deployment's preview is served over HTTPS with a self-signed certificate for `127.0.0.1` that the global setup creates with `openssl` for every run in the ignored `frontend/test-results/deployment/tls/`. `vite preview` reads it from `OVC_FRONTEND_PREVIEW_CERTIFICATE` and `OVC_FRONTEND_PREVIEW_PRIVATE_KEY` (both or neither; listed in `frontend/.env.example`, empty by default), and the private key is kept at mode 0600 in a 0700 folder. Playwright accepts it with `ignoreHTTPSErrors`; it is never installed into any trust store, and the cookie's `__Host-` prefix and `Secure` flag are never relaxed. `OVC_PUBLIC_URL` is that HTTPS origin, and a probe in every browser checks that confirmation sets the refresh cookie and a refresh then succeeds (pending owner review).
- **Traces:** Playwright keeps a trace only for a failed test (`retain-on-failure`) in the ignored `frontend/test-results/`; traces hold only generated test accounts and are never shared (pending owner review).
- **Control routes:** `/__test__/` routes, absent from every other configuration and from OpenAPI:

  | Route | Effect |
  |---|---|
  | `POST /__test__/clock` | Set or advance the shared clock; only with `OVC_CLOCK=test` |
  | `POST /__test__/tick` | Run one worker `Tick(now)` to completion at the shared clock's instant; only with `OVC_CLOCK=test` |
  | `POST /__test__/mail` with `{to}` in the body | Read messages from the database mail sink, with their links |
  | `POST /__test__/presence` with `{user}` | Report whether the server sees the user online, so tests can wait for a socket close |
  | `POST /__test__/settings/embed` | Override `embed_allowed` for this deployment, until the next override |
  | `POST /__test__/fake-provider/lessons/{lesson}/recording` | After the MVP: start or stop recording in the lesson's latest fake meeting; not registered until recordings land |
  | `POST /__test__/fake-provider/lessons/{lesson}/ready` | After the MVP: make the fake provider send a correctly signed recording-ready callback for the lesson's latest fake meeting; not registered until recordings land |

- **Fake room:** the fake meeting provider's join link opens `/__test__/fake-room`, which shows the meeting, participant name, role and, after the MVP, recording state, refreshing every second. In the MVP the fake meeting provider has no way to start a recording at all (pending owner review). Under `OVC_TEST_CONTROL=1` it is served with framing allowed from the OVC origin, so both the embedded and the new-tab paths can be asserted.
- **Isolation:** each suite seeds its own tutors and students through the real API with unique emails.

## Local review

Use the [ovc-verify skill](../.claude/skills/ovc-verify/SKILL.md) before planning and before reporting readiness. Claude Code discovers it in `.claude/skills/ovc-verify`. The skill picks one of three tiers ([ADR-0011](decisions/0011-tiered-verification.md)): trivial changes run the checks above; standard changes add one fresh reviewer with the standard checklist, plus a security reviewer when the change meets the security trigger defined in [AGENTS.md](../AGENTS.md#development-verification); when a roadmap slice is complete, the coordinating agent follows direct and transitive effects across the slice and starts one fresh reviewer per changed layer (backend, frontend, security, tooling and agent configuration), and reviewers run the applicable checks above; after fixes, at most one reviewer looks at them again (pending owner review). The [coverage checklist](../knowledge/reviews/ai-development-setup.md) lists component families, workloads and activation.

## Changes and commits

Every nontrivial change starts with an owner-approved plan and lives on its own branch as one commit. Once its checks pass and its readiness reviews are resolved, the agent merges it into `main` with `git merge --ff-only` and deletes the branch, as [AGENTS.md](../AGENTS.md#change-workflow) describes. Agents never push, rebase, reset, clean, force-delete branches, discard work with destructive checkouts, restores or stash drops, or rewrite history (amend, filter-branch, update-ref, reflog expiry) without the owner. A change that alters a contract updates its specification, requirement or decision record in the same change. New dependencies enter only with an implemented consumer and a recorded reason in the [stack table](../knowledge/architecture/detailed.md#stack-per-component).
