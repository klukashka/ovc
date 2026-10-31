# Restore the local verification baseline

Status: merged on 2026-10-31 as `d23c7fd`; the full exact-tree gate and finish passed. The [merge evidence](local-verification-recovery-merge-2026-10-31.md) records the complete results. Earlier sections below retain their pre-gate evidence states. Base: `e95d27281587699df8286dac335927dd13488631`.

The local check run was blocked by two independently dated fixtures: the successful operator-restore process test deleted accounts at the fixed instant 2026-10-01, outside its 30-day restore window on the check date; and the tools module's temporary x/net advisory exceptions expired on 2026-10-31. Gating either separate fix on the original baseline would retain the other failure. The owner approved one coherent verification-baseline recovery change, one commit and one full gate, with S5 kept separate. Original preparation branches are preserved.

## Scope and acceptance

The restore test captures deletion time one day before the system time used by the real operator process, keeping successful, repeated and unconfirmed-account cases distinct. Production behavior is unchanged. This process test is relative to the system clock, not a deterministic check of the exact retention boundary; the existing injected-clock service test retains the boundary coverage.

The tools update selects x/net 0.62.0 and seven upstream-required transitive versions, removes only the two expired exceptions, rebuilds the five pinned tools and updates the research and T-39 wording. See the [research](../research/tools-x-net-advisories-2026-10-28.md) for origins, release times, module edges and advisory evidence. Human-facing threat-model wording remains pending owner review. The affected threats are T-22/T-34 for preserved operator-test coverage and T-39/T-19/T-41 for tool integrity; no new production authority or trust boundary is introduced.

Acceptance: the real restore process test and startup suite pass; generated tool binary metadata matches the final tools module; the tools OSV scan has no unfiltered issues; the full gate passes on this combined tree and finish confirms main has exactly that tree.

## Verification before the combined gate

Tier: Standard; security trigger met. Each preparation diff received separate independent standard and full-security reviews, with no findings; the complete combined diff also receives those reviews before readiness.

- Five tool builds: exit 0 with Go 1.27.3.
- `tools/check-tool-binaries.sh`: exit 0, `bin/ matches tools/go.mod`.
- Rebuilt OSV-Scanner over `tools/go.mod`: exit 0, no unfiltered issues, only the unchanged accepted triages.
- `go -C tools mod tidy -diff`: exit 0.
- Real `go test -race ./cmd/... -count=1` of the restore fix: exit 0, startup 12.200 seconds.
- `go vet ./...` and golangci-lint over the restore baseline: exit 0. An earlier lint command used the wrong working directory and did not analyze code; the corrected invocation passed.
- Staged Gitleaks scan of the restore fix: exit 0.
- An earlier gate of the separate restore fix failed at the tools OSV scan, naming GO-2026-6860 and GO-2026-6866; its complete backend race suite passed. This is not an infrastructure failure and permits no unchanged-tree retry. The combined change has a different tested tree and resolves the cause.

All other checks remain pending until executed on the combined tree. No earlier result is represented as a passing full gate. Tests, gates and measurements run serially across the machine; no S5 measurement or other check overlaps this gate.

## Additional advisory triage

The first combined gate passed the Go tools scan but failed on GHSA-g3hv-75wg-m249 in the frontend lockfile; an explicit scan confirmed the same advisory in the documentation lockfile. The backend scan passed with its unchanged accepted openpgp triage. The owner approved accepting only this advisory in the two workspace OSV configs until 2026-11-07T00:00:00Z after independent static reachability analysis, with no version, audit or release-age changes. The [minimatch triage](../research/minimatch-advisory-2026-10-31.md) records the inspected consumers and API compatibility constraint. This is a changed tree, not an infrastructure retry. A fresh after-fix reviewer applies the standard checklist to these additions and the full security profile to the complete final recovery diff. Source preparation branches remain intact; no history was amended.

The fresh after-fix review returned passed with no new blocker, major or minor finding after applying the standard checklist to the additions and the full security profile to the complete recovery diff. Both workspace OSV scans then exited 0, applying only their recorded advisory acceptances; documentation checks exited 0 (600 unique links, no errors). The unchanged `pnpm audit --audit-level high` exited 0 while reporting one moderate vulnerability; this result does not mean the dependency inventory contains no vulnerabilities. Tool identity verification again exited 0. Full exact-tree gate and finish are still pending.

## Wrapper self-test isolation

A gate launched with the full sanctioned development environment failed the wrapper self-test because the caller's existing database-password variable overrode its synthetic fixture. The owner authorized one unchanged-tree repeat with fixture-only configuration. That repeat passed the self-test but failed E2E deployment preparation: Compose also requires its development PostgreSQL user/password. No E2E scenario ran successfully in that misconfigured repeat; its failures remain failures and are not called flakes or infrastructure retries.

The owner then approved isolating only `tools/check-dev-env.sh` by removing inherited OVC names in that child process before its own fixtures. The parent gate now keeps all settings loaded by the sanctioned wrapper. Every self-test assertion, including explicit caller-variable precedence, is retained. ADR-0009 item 10 and AGENTS record this exact exception; no application-test environment substitution is authorized. This creates a changed tested tree rather than another unchanged-tree retry. New standard/security reviews and synthetic-ambient self-test checks are required before readiness.

The final complete diff received fresh independent standard and full-security reviews, both passed with no findings. System Bash self-test runs with no OVC variables and with synthetic ambient public-URL/password variables both exited 0 and printed `all checks passed`; the synthetic values were absent from captured output. The secret guard reported 198 passed, zero failed. Documentation checks exited 0 with 600 unique links and no errors. The full gate for the changed isolation tree remains pending; no claim is made that an earlier failed gate passed.
