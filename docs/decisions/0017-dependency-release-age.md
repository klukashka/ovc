# ADR-0017: A three-day release age for dependencies until the first deployment

- Status: accepted, 2026-10-26
- Reviewed: decided by the project owner on 2026-10-26; wording pending owner review, line by line
- Deciders: project owner
- Related: [ADR-0002](0002-implementation-stack.md), [ADR-0013](0013-pilot-deployment.md), [threat model: T-19, T-39, T-41](../threat-model.md), [development guide](../development.md#frontend-checks), [roadmap: before the pilot](../roadmap.md#before-the-pilot)

## Context

The frontend and the documentation tools set pnpm's `minimumReleaseAge` to 14 days, so pnpm refuses any direct or transitive version published less than two weeks before resolution, patch releases of packages already in use included. The same 14 days were applied by hand to the Node runtime, which pnpm does not check, and to Go modules and tools. The delay guards against a compromised release of a popular package, which is usually found and withdrawn within hours or days.

OVC is developed locally and is not deployed, yet the developer machine is worth protecting: package code runs with the owner's privileges on every check, not only at install time (ESLint and its plugins, Vite, Vitest and its DOM, Playwright, the diagram renderer), and the end-to-end suites run through `tools/dev-env.sh` with the owner's development settings in their environment. Code in a compromised release can therefore read the environment files, including the provider secrets of `backend/.env.local` (T-19), local credentials such as SSH keys and Git access, and change the repository and the agent configuration that agents then commit (T-41). At the same time the 14-day delay held fixes for accepted advisories back by up to two weeks, needed a version-specific exclusion for one package and dated changes in the slice plans.

## Decision

- **Until the first deployment** of OVC (the pilot deployment of ADR-0013, a lessons-only alpha or any earlier one), `minimumReleaseAge` is 3 days (4320 minutes) in `frontend/pnpm-workspace.yaml` and `tools/docs/pnpm-workspace.yaml`, and the same 3 days apply by hand to the Node runtime pinned by `frontend/.node-version`, to Go modules and to the tools pinned in `tools/go.mod`. Most compromised releases are withdrawn within that window, while a fix waits days instead of weeks. No release-age exclusion is kept. Every other control of T-39 stays: lockfiles, no dependency install scripts, the OSV-Scanner and `pnpm audit` checks with dated triage, pinned tools.
- **Before the first deployment**, the delay returns to 14 days (20160 minutes) in both workspaces and for Node, Go modules and tools, and the dependencies resolved while it was 3 days are reviewed: list every lockfile, `go.mod` and `go.sum` entry and every `frontend/.node-version` change made since this decision from the Git history, scan them with OSV-Scanner (whose data includes malicious-package entries), reinstall both workspaces with `pnpm install --frozen-lockfile` against an empty store so a withdrawn or replaced tarball fails its integrity check, and review the cumulative diff of the window. Restoring the delay alone proves nothing about those entries, because they are older than 14 days by then. The roadmap lists this as an item before the pilot, and `pnpm lint` enforces the values (below).
- **From then on**, dependencies are updated only when there is a reason: a security fix for an advisory that affects OVC, a fix for a defect OVC hits, or a feature a planned change needs. The change that updates a dependency names the reason. A version younger than two weeks is admitted only by the owner, through an exclusion of exactly that version with the reason recorded in the development guide and the threat model.
- **Enforcement:** `frontend/scripts/check-release-age.ts`, part of `pnpm lint`, parses both workspace files with a YAML parser and fails when either is not one YAML mapping, repeats a key, or sets the release age under another spelling or below the top level, when the two workspaces disagree, when either sets a value other than 4320 or 20160, or when a `deploy/` folder exists at the repository root and either value is below 20160; it also refuses any `minimumReleaseAgeExclude` while the delay is 3 days, any exclusion that does not name exactly one package version once it is 14 days, the setting in a `.npmrc` or in the `pnpm` field of `package.json` of either folder, and a pnpmfile or the `pnpmfile`, `globalPnpmfile` and `configDependencies` settings, whose hooks could change the value (enforcement wording amended 2026-10-26, pending owner review). The first deployment configuration lives in that folder ([deployment shapes](../../knowledge/architecture/detailed.md#deployment-shapes)), so the change that adds it trips the check. Settings outside the repository (pnpm's environment variables, command-line flags and global configuration) can override the workspace value and are out of the check's reach.

## Alternatives considered

- **Keep 14 days during development:** the fullest protection of the developer machine, at the cost of waits of up to two weeks for fixes and exclusions for urgent ones.
- **No delay until the first deployment:** fixes land at once, but the developer machine, its secrets and the repository are exposed to a compromised release from its first minute; rejected after the security review showed what package code can reach on that machine.
- **Drop the delay for good:** leaves a deployed OVC without the window that catches most compromised releases; rejected.

## Consequences

- A compromised release that stays published for more than 3 days can run on the developer machine during the checks, read the environment files, the provider secrets and the local credentials, and change the repository and the agent configuration (T-19, T-39, T-41). The owner accepts this residual until the first deployment; the lockfiles, the absence of install scripts and the scans remain the other controls.
- The slice plans no longer wait two weeks for fixes.
- The deployment preparation restores the 14-day delay and reviews the window as described above; `pnpm lint` fails once a `deploy/` folder exists while the delay is still 3 days.

## Where it is applied

- `frontend/pnpm-workspace.yaml` and `tools/docs/pnpm-workspace.yaml` (`minimumReleaseAge`), `frontend/scripts/check-release-age.ts` in `pnpm lint`.
- [Development guide](../development.md#frontend-checks), [T-39](../threat-model.md) and the [roadmap item before the pilot](../roadmap.md#before-the-pilot).
