# Minimatch glob-preprocessing advisory triage

Checked on 2026-10-31. The owner approved this narrowly scoped acceptance until 2026-11-07T00:00:00Z and extended it on 2026-11-07 until 2026-11-21T00:00:00Z. On 2026-11-10 the version-10 consumers were moved to minimatch 11.0.5 and the acceptance was removed; see [the last section](#removal-of-the-version-10-consumers-2026-11-10). The sections before it are the history of the acceptance. Recheck sooner if a version, caller, pattern source or optimization option changes. This is evidence for the current development tools, not a claim about all minimatch behavior.

## Advisory and affected inventories

[GHSA-g3hv-75wg-m249](https://github.com/advisories/GHSA-g3hv-75wg-m249), CVE-2026-118764, describes denial of service through input-controlled growth of glob parts in `firstPhasePreProcess`, followed by quadratic comparison. The reviewed advisory identifies `optimizationLevel >= 2` as the trigger and, as reviewed on 2026-10-31, reported all versions before 11.0.1 as affected; the recheck below records the later narrowing.

`pnpm why minimatch` in each workspace shows only development-tool consumers:

- Frontend 3.1.5: ESLint, its configuration packages and eslint-plugin-jsx-a11y.
- Frontend 5.1.9: openapi-typescript through @redocly/openapi-core.
- Frontend 10.2.6: typescript-eslint through @typescript-eslint/typescript-estree.
- Documentation 10.2.6: markdownlint-cli.

The backend does not use this npm dependency, and it is not an application runtime dependency or an API for processing uploaded/student content.

## Reachability evidence

Installed minimatch 3.1.5 and 5.1.9 contain neither `firstPhasePreProcess` nor the `optimizationLevel` mechanism. The described amplification phase is absent in these older implementations; the advisory's version range as of 2026-10-31 still made package-only scanners report them.

In minimatch 10.2.6, `dist/commonjs/index.js` selects the described phase only at optimization level 2 or greater. The default is 1. The inspected typescript-estree caller, `dist/useProgramFromProjectService.js`, passes only `{ dot: true }`; the repository's ESLint config enables `projectService` without `allowDefaultProject`. The inspected markdownlint-cli call in `markdownlint.js` passes dot, onlyFiles, expandDirectories and caseSensitiveMatch, without enabling optimization level 2. These patterns come from the local repository configuration and tool invocation, not OVC HTTP/WebSocket requests or user content. No exploit payload was executed on the development device.

A global override to minimatch 11 would change the interface of old consumers: Redocly uses callable `require('minimatch')`, whereas the newer package exposes named exports. Installed 3/5 export the callable function. The npm registry has no patched 3/5 backport in its current lines. Replacing only the two 10.x consumers is a possible later change but, under the range as of 2026-10-31, did not address scanner reports on 3/5; it needs compatibility testing and is unnecessary to disable a phase already not enabled by these callers.

Read-only [npm registry metadata](https://registry.npmjs.org/minimatch) reports 11.0.1 published at 2026-10-27T22:26:03.755Z, with engines `18 || 20 || >=22`. Its three-day age elapsed at 2026-10-30T22:26:03.755Z and its Node requirement fits the project. Release age is not the reason for retaining old consumer-compatible versions; API compatibility and unreachable described paths are.

## Bounded acceptance and checks

Only this advisory is excluded, only in `frontend/osv-scanner.toml` and `tools/docs/osv-scanner.toml`, until 2026-11-21T00:00:00Z. The exclusion is by advisory ID, not a package-wide exclusion, and suppresses none of minimatch's other advisories. Existing documentation-renderer katex acceptance is unchanged. No `auditConfig.ignoreGhsas` addition is made; the registered pnpm audit remains active. No dependency, lockfile, release-age rule or scanner command is changed for this triage.

T-39 owns the vulnerable-tool assessment, with unchanged consequences for T-19 and T-41 if a development dependency or repository is compromised. This acceptance does not make arbitrary repository code safe to execute. The immediate gate failure is a genuine package-scan finding, not infrastructure and not a permitted unchanged-tree retry. The new tree records the explicit acceptance and its expiry. Final scans and full exact-tree gate remain pending until executed; independent review cannot replace those results.

## Recheck at expiry, 2026-11-07

The acceptance expired and failed the OSV scan of every merge gate. The owner chose a 14-day extension over forcing minimatch 11 onto the version-10 consumers. Only the two `ignoreUntil` dates changed; no dependency, lockfile, scanner command or other advisory entry changed.

The installed tree was read again, without executing any payload. The inventory is unchanged: frontend 3.1.5, 5.1.9 and 10.2.6, documentation 10.2.6. Versions 3.1.5 and 5.1.9 still contain neither `firstPhasePreProcess` nor `optimizationLevel`. typescript-estree still passes only `{ dot: true }` in `useProgramFromProjectService.js`, and the ESLint configuration still enables `projectService` without `allowDefaultProject`. markdownlint-cli 0.49.1 still builds its options from `dot`, `onlyFiles`, `expandDirectories` and `caseSensitiveMatch`.

The [OSV record](https://api.osv.dev/v1/vulns/GHSA-g3hv-75wg-m249), modified 2026-11-04, now gives the affected range as introduced in 7.0.0 and fixed in 11.0.1. A scan with an empty configuration reports only 10.2.6 in both workspaces, not 3.1.5 or 5.1.9. Replacing the two version-10 consumers would therefore clear the finding, and Redocly's callable import no longer stands in the way. Both consumers declare only the version-10 line (`^10.2.2` and `~10.2.5`), so this needs a scoped override or upstream releases, and its compatibility is unverified. The owner kept the extension knowing this; no replacement is scheduled by this change.

Prettier 3.9.9 and Playwright 1.63.0 bundle copies of minimatch 10 with this phase, which lockfile scans do not see. They stay at the default level, and the traced callers' options do not set it. Replacing the two lockfile consumers clears the scanner finding, not these copies.

The registry now lists minimatch up to 11.0.5, published 2026-11-05. At the new expiry, replace the version-10 consumers if upstream allows it, or request another explicit decision; the acceptance does not renew by itself.

On 2026-11-09 the owner decided, in the design slice review, that the minimatch 10.x consumers are removed in a separate change before this acceptance ends on 2026-11-21.

## Removal of the version-10 consumers, 2026-11-10

Checked on 2026-11-10 against the npm registry. No upstream release leaves the version-10 line: typescript-eslint up to 8.72.2 (2026-11-09) has typescript-estree declare `^10.2.2`, and markdownlint-cli declares `~10.2.5` in the pinned 0.49.1 and `~10.2.6` in the newest, 0.50.0 (2026-10-24). minimatch publishes no fixed 10.x release; the newest is 10.2.6.

Both workspaces therefore add the override `minimatch@10: 11.0.5` to `pnpm-workspace.yaml`, applied with `pnpm install`. It is scoped by the `@10` selector to the version-10 requests, so minimatch 3.1.5 and 5.1.9 of ESLint and Redocly, outside the advisory's range, stay as they were. `pnpm why minimatch` now lists 3.1.5, 5.1.9 and 11.0.5 in the frontend and only 11.0.5 in the documentation tools. minimatch 11.0.5 was published at 2026-11-05T23:30:59Z and its new dependency `@isaacs/nonrecursive-glob-match` 1.1.0 at 2026-11-05T21:38:25Z; both are older than the three-day minimum release age. `@isaacs/nonrecursive-glob-match` is new (first published 2026-10-26, six releases by 1.1.0), comes from minimatch's own publisher under the same BlueOak-1.0.0 licence, has no install scripts and, like minimatch, no npm provenance attestation; it runs only in local development tools, never in served output, and minimatch 11.0.5 requires Node `20 || >=22`, which fits the project's Node 26. brace-expansion stays at 5.0.12.

Version 11 removes `makeRe`, the `AST` export, the `debug` option and `maxExtglobRecursion`, and keeps the named `minimatch` function, the `Minimatch` class with `match`, and the options the two callers pass. typescript-estree calls only `minimatch(filePath, pattern, { dot: true })` in `useProgramFromProjectService.js`, and only for `allowDefaultProject`, which the repository's ESLint configuration does not set. markdownlint-cli imports the named `minimatch` and uses `new minimatch.Minimatch(pattern, globOptions)` with `match` when it applies `--ignore` patterns to an earlier file list; its other globbing goes through tinyglobby. A resolution check through each caller's own module path loaded 11.0.5 and returned the expected matches, and markdownlint run on a glob with an `--ignore` pattern, which takes the `Minimatch` path, exited 0.

The `GHSA-g3hv-75wg-m249` entries are removed from `frontend/osv-scanner.toml`, which is empty again, and from `tools/docs/osv-scanner.toml`, which keeps only the katex entry. OSV-Scanner over both lockfiles passes without them.

The copies of minimatch 10 bundled inside Prettier 3.9.9 and Playwright 1.63.0 are unchanged and remain invisible to lockfile scans; their callers leave the optimization level at its default, as recorded above. Remove the override when typescript-eslint and markdownlint-cli declare a fixed minimatch range themselves.
