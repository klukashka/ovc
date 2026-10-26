# Frontend toolchain and library versions

Checked 2026-10-21 against the npm registry (`npm view <package> time`) and each package's declared peer dependencies. For every package the latest release published on or before 2026-10-07 was taken, so nothing younger than two weeks is pinned. Facts here are volatile; recheck them before relying on them after this date.

## Runtime and package manager

| Component | Version | Where it is pinned |
|---|---|---|
| Node | 26.0.0 (Node 26.12.0 bundles undici 8.11.2 and 26.12.1 bundles undici 8.12.2, read from `deps/undici/src/package.json` at each release tag on 2026-10-26) | `frontend/.node-version`, `engines` in `frontend/package.json` |
| pnpm | 12.14.0 | `packageManager` in `frontend/package.json` |

Node 26 runs the TypeScript build scripts in `frontend/scripts/` and `frontend/e2e/` directly through its built-in type stripping, so they use only erasable syntax (`erasableSyntaxOnly` in the TypeScript configuration).

## Libraries

| Package | Version | Note |
|---|---|---|
| `react`, `react-dom` | 19.3.0 | |
| `react-router` | 8.4.0 | |
| `@tanstack/react-query` | 5.104.1 | |
| `i18next` | 26.4.2 | |
| `react-i18next` | 17.0.16 | |
| `i18next-icu` | 2.5.0 | Declares `intl-messageformat` `>=10.3.3 <13.0.0` as a peer |
| `intl-messageformat` | 12.1.3 | Declared explicitly so the peer is not installed implicitly |
| `openapi-fetch` | 0.17.0 | |
| `@fontsource/golos-text`, `@fontsource/literata`, `@fontsource/jetbrains-mono` | 5.3.0 | Per-weight CSS files with a `unicode-range` per subset; the bundle holds Cyrillic, Cyrillic extended, Latin and Latin extended for all three families, plus Greek, Greek extended and Vietnamese for Literata and Greek and Vietnamese for JetBrains Mono, and browsers download only the subsets a page's text uses. The per-subset files lack `unicode-range`, which made browsers fetch Latin and Cyrillic one after the other |

## Build and check tools

| Package | Version | Note |
|---|---|---|
| `vite` | 8.3.3 | |
| `@vitejs/plugin-react` | 6.1.2 | |
| `tailwindcss`, `@tailwindcss/vite` | 4.3.3 | |
| `typescript` | 6.0.3 | 7.0.2 exists, but typescript-eslint 8.71.1 declares the peer `typescript >=4.8.4 <6.1.0` |
| `eslint`, `@eslint/js` | 9.39.5 | 10.12.0 exists, but eslint-plugin-jsx-a11y 6.10.2 declares ESLint up to 9 as its peer; pnpm marks 9.39.5 deprecated in favour of 10; recheck by 2027-01-21, or earlier when eslint-plugin-jsx-a11y declares ESLint 10 support |
| `typescript-eslint` | 8.71.1 | |
| `eslint-plugin-react-hooks` | 7.1.1 | |
| `eslint-plugin-jsx-a11y` | 6.10.2 | Ships no type declarations; `frontend/types/eslint-plugin-jsx-a11y.d.ts` declares the part the configuration uses |
| `@eslint-community/eslint-plugin-eslint-comments` | 4.8.1 | |
| `globals` | 17.13.0 | |
| `prettier` | 3.9.9 | |
| `vitest` | 5.0.3 | |
| `jsdom` | 30.1.2 | |
| `@testing-library/react` | 16.3.3 | With `@testing-library/dom` 10.4.2 |
| `openapi-typescript` | 7.13.0 | Declares the peer `typescript ^5.x`; generation works with 6.0.3, and the peer is accepted in `frontend/pnpm-workspace.yaml` |
| `@playwright/test` | 1.63.0 | Browsers installed with `playwright install chromium firefox webkit` |
| `@types/node` | 26.6.4 | |
| `@types/react`, `@types/react-dom` | 19.3.0 | |

## Checked behaviors

- ESLint 9.39 loads `eslint.config.ts` through Node's type stripping only with `--flag unstable_native_nodejs_ts_config`, which the `lint` script passes.
- `pnpm install` with an empty `allowBuilds` ran no dependency install scripts: the native parts of Rolldown, Lightning CSS and Tailwind's Oxide engine ship as prebuilt optional packages.
- The first resolution pinned only direct dependencies by age; 36 transitive versions (for example `rolldown` 1.2.14 of 2026-10-14) were younger than two weeks. With `minimumReleaseAge: 20160` the lockfile was re-resolved from scratch and those packages fell back to older releases (for example `rolldown` 1.2.12).
- `pnpm audit --audit-level high` and OSV-Scanner over `frontend/pnpm-lock.yaml` reported no known vulnerabilities on 2026-10-21.

### Accepted findings

Accepted findings, listed in `frontend/osv-scanner.toml` and in `auditConfig.ignoreGhsas` of `frontend/pnpm-workspace.yaml` (OSV-Scanner and `pnpm audit` over `frontend/pnpm-lock.yaml`, rechecked 2026-10-24):

| Advisory | Package | Reason |
|---|---|---|
| GHSA-xjj6-f33q-929c (high, CVSS 3.1 7.5), GHSA-4qv8-2p9j-2685 (moderate, CVSS 3.1 5.9), GHSA-f72v-6w45-q472 (moderate, CVSS 3.1 5.9) | `undici` 8.11.2, through `jsdom` 30.1.2 under Vitest | Checked 2026-10-24: jsdom is the DOM of the unit tests, which load no external resources and open no XHR, WebSocket or EventSource through jsdom; nothing in the application build imports undici. The fix 8.12.2 was published 2026-10-15 and is younger than the minimum release age. Accepted until 2026-10-31, then upgraded; the upgrade change also removes the three `auditConfig.ignoreGhsas` entries by hand |
