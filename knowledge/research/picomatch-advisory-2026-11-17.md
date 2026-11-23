# picomatch advisory triage (2026-11-17)

Status: agent triage for the temporary acceptance of GHSA-fc5q-39xc-vjpf, decided by the owner on 2026-11-17; checked on 2026-11-17 at `main` 899d50e. Cited from [the development guide](../../docs/development.md#security-scanning) as evidence, not as a decision. Superseded on 2026-11-23: GitHub withdrew the advisory on 2026-11-18 and the acceptance was removed ([recheck](picomatch-advisory-2026-11-23.md)); the reachability analysis below is kept as a record.

## Advisory

- GHSA-fc5q-39xc-vjpf, alias CVE-2026-112366, CWE-1333, severity High (CVSS 3.1 `AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H`).
- OSV record checked through `https://api.osv.dev/v1/vulns/GHSA-fc5q-39xc-vjpf`: published 2026-10-19, modified 2026-11-17T16:00Z, `github_reviewed_at` 2026-11-17T15:56Z. The affected range for npm `picomatch` is introduced `0`, last affected `4.0.7`, so every published version is affected.
- Mechanism: a glob holding a single-branch repeated extglob, such as `+(*)` or `+(*a)`, is skipped by `analyzeRepeatedExtglob` in `lib/parse.js` and compiles into a regular expression that backtracks exponentially against short crafted strings. The attacker has to supply the **pattern**. Controlling only the string matched against a fixed pattern is not enough.
- Fix status: npm `latest` is 4.0.7, published 2026-08-24, and no later release exists. micromatch/picomatch#194, cited by the advisory, fixes a different extglob behaviour. The candidate ReDoS fixes are the open pull requests #238 and #246; #240 was closed unmerged.

## Scanner results before the acceptance

- `bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml`: one finding, picomatch 4.0.7.
- `bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml`: the same finding, next to the already accepted katex advisory.
- `pnpm audit --audit-level high` in `frontend/`: exit 0, "No known vulnerabilities found", because npm's audit feed does not carry the advisory yet. The owner chose to add it to `auditConfig.ignoreGhsas` now, on 2026-11-17, so the gate does not fail again once the feed picks the advisory up. The minimatch acceptance had left `pnpm audit` registered instead. `scripts/check-ignored-advisories.ts` in `pnpm lint` keeps this ignore paired with the dated OSV entry, and it fails once that entry expires.

## Dependency paths

All roots are `devDependencies`. No package under `dependencies` in `frontend/package.json` reaches picomatch.

Frontend (`pnpm why picomatch`, one version, 4.0.7):

- `vite` 8.3.3 → picomatch, and vite → tinyglobby 0.2.17 → fdir 6.5.0 (peer `picomatch: ^3 || ^4`) → picomatch. vite is also pulled in by `@tailwindcss/vite`, `@vitejs/plugin-react` and `@vitest/mocker`.
- `vitest` 5.0.3 → picomatch and → tinyglobby.
- `typescript-eslint` 8.71.1 → `@typescript-eslint/typescript-estree` 8.71.1 → tinyglobby.

Documentation tools (`tools/docs/pnpm-lock.yaml`):

- `markdownlint-cli` 0.49.1 → tinyglobby 0.2.17 → fdir 6.5.0 / picomatch 4.0.7.

## Reachability evidence

Read in the installed packages on 2026-11-17.

- **vite 8.3.3**, `dist/node/chunks/node.js`:
  - `server.fs.deny` defaults to `.env`, `.env.*`, `*.{crt,pem,key,p12,pfx,cer,der}`, `.npmrc`, `.yarnrc.yml` and `**/.git/**` (the `fs` defaults in the config resolution). These are compiled once into `fsDenyGlob` with `pm(server.fs.deny…)`. `frontend/vite.config.ts` sets only `fs: { strict: true }`, so the patterns are vite's own and hold no extglob. A request path reaches picomatch only as the string matched against them.
  - Plugin filters (`createFilter`, `patternToIdFilter`) compile the include and exclude patterns that plugins and the repository configuration pass, joined with the project root (`createFilter` escapes the root; `patternToIdFilter` does not, so the checkout path chosen by the developer becomes part of the pattern). Module ids are only matched.
  - The `import.meta.glob` transform compiles globs written in source. `git grep "import.meta.glob" -- frontend` finds none.
  - Dependency optimisation matches package `exports` keys from installed dependency manifests (supply-chain-authored).
  - The bundled watcher (chokidar's `anymatch` and `readdirp`) compiles its `ignored` and file-filter options from vite's configuration.
  - The development server and the preview server listen on `127.0.0.1` only (`frontend/vite.config.ts`, `server.host` and `preview.host`).
- **vitest 5.0.3**, `dist/chunks/index.*.js`: `pm.isMatch(relativeId, this.config.include)`, `this.config.exclude` and `includeSource`, and the coverage `matchInclude` and `matchExclude` compile the patterns in `frontend/vite.config.ts` (`src/**/*.test.tsx`, `src/**/*.test.ts`, `scripts/**/*.test.ts`, `e2e/**/*.test.ts`) or filters a developer types on the command line.
- **typescript-estree 8.71.1**, `dist/parseSettings/resolveProjectList.js`: tinyglobby's `globSync` runs only on `parserOptions.project` globs. `frontend/eslint.config.ts` uses `projectService: true` and no project globs.
- **markdownlint-cli 0.49.1**, `markdownlint.js`: an argument that `lstatSync` finds as a file is returned unchanged; a directory argument is passed to tinyglobby `globSync` as `path.join(directory, '**', <extensions>)`, so the directory name becomes part of the pattern; an argument that does not exist on disk is passed to `globSync` as a glob. `tools/docs/check-docs.sh` passes the names from `git ls-files --cached --others --exclude-standard -z -- '*.md'`; a listed name missing on disk is treated as a glob.

None of these patterns comes from a request, an upload, a message or any other user content. A `git grep -F` of `+(`, `@(`, `!(` and `?(` over `frontend/vite.config.ts`, `frontend/eslint.config.ts`, `frontend/tsconfig.json`, `frontend/playwright.config.ts`, `tools/docs/check-docs.sh` and `tools/docs/package.json` finds nothing.

## Production bundle and server

- `pnpm build` and `pnpm check:bundle` in `frontend/` exited 0 on 2026-11-17 with this change applied.
- `grep -c -E 'picomatch|tinyglobby|fdir@' build-report/module-graph.json` prints `0`. `grep -o 'node_modules/.pnpm/' build-report/module-graph.json | wc -l` prints `813`, so the graph does record dependency modules.
- The dependency packages in the graph are exactly `@formatjs/fast-memoize`, `@formatjs/icu-messageformat-parser`, `@formatjs/icu-skeleton-parser`, `@tanstack/query-core`, `@tanstack/react-query`, `i18next`, `i18next-icu`, `intl-messageformat`, `openapi-fetch`, `react`, `react-dom`, `react-i18next`, `react-router`, `scheduler` and `use-sync-external-store`.
- The backend and worker are Go (`backend/go.mod`) and have no npm dependency, so no picomatch reaches the server.

## Residual risk

- A Markdown name holding `+(…)` that `git ls-files` lists but that does not exist on disk could stall `tools/docs/check-docs.sh` and the merge gate. An example is a tracked file deleted in the working tree. This is a local denial of service of a check, caused by someone who can already change the repository.
- Copies of picomatch bundled inside packages are invisible to lockfile scanning, and each compiles only repository or package patterns:
  - Prettier 3.9.9 bundles a copy in `index.mjs`; its patterns come from the command line (`prettier --check .`) and `.prettierignore`.
  - Playwright 1.63.0 bundles picomatch 4.0.4 in `lib/matchers/expect.js` (listed in `expect.js.LICENSE`), used by its inlined Jest utilities on configuration patterns such as `testMatch`.
  - In the documentation tools, `@zenuml/core` 3.50.1 vendors a Tailwind CSS browser build holding picomatch (`dist/vendor/tailwindcss/tailwindcss.js`), identified by picomatch's internal identifiers (`POSIX_REGEX_SOURCE`, `QMARK_NO_DOT`) rather than its name, which runs inside the diagram renderer over this repository's own diagrams.
- A future repository glob holding a repeated single-branch extglob, or a dependency that starts passing untrusted input as a pattern, would make the advisory reachable. Either one triggers a recheck.

## Override

There is no patched release to override to. A git or tarball override of an unmerged fix would bypass ADR-0017's release age and the registry integrity path, so none is used.

## Removal

- Remove the acceptance in the change that admits a picomatch release outside the OSV affected range, once it passes ADR-0017's release age.
  - A 4.0.x release satisfies the ranges vite, vitest, tinyglobby and fdir declare, so a lockfile update suffices.
  - A fix released only in a 5.x line needs a scoped override.
- That change deletes both `osv-scanner.toml` entries and the `auditConfig.ignoreGhsas` line together. `check-ignored-advisories` fails only on expiry, not when a fix lands.
- When the entries expire on 2026-12-01 with no fix released, recheck the reachability above and extend with a dated update of this file, as the minimatch acceptance did.
