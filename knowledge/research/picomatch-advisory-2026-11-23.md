# picomatch advisory recheck (2026-11-23)

Status: agent recheck before the acceptance of GHSA-fc5q-39xc-vjpf expired on 2026-12-01; checked on 2026-11-23 at `main` 7c71824. The owner approved removing the acceptance on 2026-11-23. It supersedes the [triage of 2026-11-17](picomatch-advisory-2026-11-17.md) as the current state, and that note keeps the reachability analysis as a record. Cited from [the development guide](../../docs/development.md#security-scanning) as evidence, not as a decision.

## Advisory state

- The GitHub advisory `https://api.github.com/advisories/GHSA-fc5q-39xc-vjpf` reports `withdrawn_at` 2026-11-18T15:38:56Z and the summary "Withdrawn Advisory: picomatch vulnerable to ReDoS through single-branch repeated extglobs".
- The OSV record `https://api.osv.dev/v1/vulns/GHSA-fc5q-39xc-vjpf`, modified 2026-11-19T16:15Z, carries the same `withdrawn` time. Its details say the advisory was withdrawn because it is not a security issue and cite the picomatch maintainer's comment of 2026-10-21 on micromatch/picomatch#235: glob patterns compile to regular expressions, no general rule predicts every pattern that backtracks badly, and the remedy is not to let untrusted users define patterns.
- The OSV query for npm `picomatch` 4.0.7 (`POST https://api.osv.dev/v1/query`) returns no vulnerability. The CVE record CVE-2026-112366 is still present in OSV, with no npm package range, so lockfile scans do not match it.
- npm `latest` for picomatch is still 4.0.7, published 2026-08-24, and no later release exists. Nothing is updated, so ADR-0017's release age does not come into play.

## Scans without the acceptance

- `bin/osv-scanner scan source --config=<empty file> --lockfile=frontend/pnpm-lock.yaml`: exit 0, "No issues found", 460 packages.
- `bin/osv-scanner scan source --config=<only the katex entry> --lockfile=tools/docs/pnpm-lock.yaml`: exit 0, "No issues found", with GHSA-238p-pmpm-9mq7 filtered by its own entry, 288 packages.
- `pnpm audit --audit-level high` in `frontend/` with `auditConfig` removed: exit 0, "No known vulnerabilities found".

## Change

- `frontend/pnpm-workspace.yaml` drops `auditConfig.ignoreGhsas`; the existing `frontend/scripts/check-ignored-advisories.ts` already accepts a workspace without `auditConfig`, as its test "reads no advisories from a workspace without auditConfig" in `frontend/scripts/ignored-advisories.test.ts` covers.
- `frontend/osv-scanner.toml` is empty again, as before 2026-11-17.
- `tools/docs/osv-scanner.toml` keeps only the katex entry.
- No lockfile, override or package changes.

## Residual

The reachability analysis of the 2026-11-17 triage still holds: picomatch reaches only development tooling, whose patterns come from repository or package configuration, and neither the production bundle nor the Go server holds a copy. A glob that holds a repeated single-branch extglob, written in repository configuration or passed by a dependency, can still backtrack badly in a development tool. That is a property of glob matching, now outside any advisory. The trigger for a recheck is a new advisory against picomatch or a dependency that starts passing untrusted input as a pattern.
