# CI shadow runs

Started 2026-11-27 under [ADR-0026](../../docs/decisions/0026-github-hosting-and-ci.md). Each row is one shadow pair: a dispatch of `.github/workflows/gate.yml` on GitHub-hosted runners beside the local merge gate of the same one-commit candidate on `main`'s tip, as the [development guide](../../docs/development.md#continuous-integration-in-shadow-mode) describes. A shadow run decides no merge.

- A run that was not dispatched, because `origin/main` differed from local `main`, or that was cancelled, is recorded as missing shadow data with its reason.
- A failure that the shadow notes of ADR-0026 expect (a new required `OVC_*` setting, a changed `tools/ci-tools.sha256`, a new Playwright project or spec group) is marked `expected`, not counted as a disagreement.
- A failure that only one side shows is a bug to fix, never waived; timeouts are never raised.
- Each row notes the known differences: CI scans the fetched refs with Gitleaks and scans `main..<candidate>` instead of the staged changes, and runs on Linux.

| Date | Candidate commit | Tree | Local tree equals candidate tree | Scope | Local gate result and failing tests | Run address | Run result and failing tests | Wall time | Summed job minutes | Runner image version | Notes |
|---|---|---|---|---|---|---|---|---|---|---|---|
