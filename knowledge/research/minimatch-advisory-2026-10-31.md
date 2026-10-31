# Minimatch glob-preprocessing advisory triage

Checked on 2026-10-31. The owner approved this narrowly scoped acceptance until 2026-11-07T00:00:00Z. Recheck sooner if a version, caller, pattern source or optimization option changes. This is evidence for the current development tools, not a claim about all minimatch behavior.

## Advisory and affected inventories

[GHSA-g3hv-75wg-m249](https://github.com/advisories/GHSA-g3hv-75wg-m249), CVE-2026-118764, describes denial of service through input-controlled growth of glob parts in `firstPhasePreProcess`, followed by quadratic comparison. The reviewed advisory identifies `optimizationLevel >= 2` as the trigger and reports all versions before 11.0.1 as affected.

`pnpm why minimatch` in each workspace shows only development-tool consumers:

- Frontend 3.1.5: ESLint, its configuration packages and eslint-plugin-jsx-a11y.
- Frontend 5.1.9: openapi-typescript through @redocly/openapi-core.
- Frontend 10.2.6: typescript-eslint through @typescript-eslint/typescript-estree.
- Documentation 10.2.6: markdownlint-cli.

The backend does not use this npm dependency, and it is not an application runtime dependency or an API for processing uploaded/student content.

## Reachability evidence

Installed minimatch 3.1.5 and 5.1.9 contain neither `firstPhasePreProcess` nor the `optimizationLevel` mechanism. The described amplification phase is absent in these older implementations; the broad advisory version range still makes package-only scanners report them.

In minimatch 10.2.6, `dist/commonjs/index.js` selects the described phase only at optimization level 2 or greater. The default is 1. The inspected typescript-estree caller, `dist/useProgramFromProjectService.js`, passes only `{ dot: true }`; the repository's ESLint config enables `projectService` without `allowDefaultProject`. The inspected markdownlint-cli call in `markdownlint.js` passes dot, onlyFiles, expandDirectories and caseSensitiveMatch, without enabling optimization level 2. These patterns come from the local repository configuration and tool invocation, not OVC HTTP/WebSocket requests or user content. No exploit payload was executed on the development device.

A global override to minimatch 11 would change the interface of old consumers: Redocly uses callable `require('minimatch')`, whereas the newer package exposes named exports. Installed 3/5 export the callable function. The npm registry has no patched 3/5 backport in its current lines. Replacing only the two 10.x consumers is a possible later change but does not address scanner reports on 3/5, needs compatibility testing and is unnecessary to disable a phase already not enabled by these callers.

Read-only [npm registry metadata](https://registry.npmjs.org/minimatch) reports 11.0.1 published at 2026-10-27T22:26:03.755Z, with engines `18 || 20 || >=22`. Its three-day age elapsed at 2026-10-30T22:26:03.755Z and its Node requirement fits the project. Release age is not the reason for retaining old consumer-compatible versions; API compatibility and unreachable described paths are.

## Bounded acceptance and checks

Only this advisory is excluded, only in `frontend/osv-scanner.toml` and `tools/docs/osv-scanner.toml`, until 2026-11-07T00:00:00Z. The exclusion is by advisory ID, not a package-wide exclusion, and suppresses none of minimatch's other advisories. Existing documentation-renderer katex acceptance is unchanged. No `auditConfig.ignoreGhsas` addition is made; the registered pnpm audit remains active. No dependency, lockfile, release-age rule or scanner command is changed for this triage.

T-39 owns the vulnerable-tool assessment, with unchanged consequences for T-19 and T-41 if a development dependency or repository is compromised. This acceptance does not make arbitrary repository code safe to execute. The immediate gate failure is a genuine package-scan finding, not infrastructure and not a permitted unchanged-tree retry. The new tree records the explicit acceptance and its expiry. Final scans and full exact-tree gate remain pending until executed; independent review cannot replace those results.
