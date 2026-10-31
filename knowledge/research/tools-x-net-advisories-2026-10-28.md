# The tools module's golang.org/x/net advisories

Checked 2026-10-28; update metadata rechecked 2026-10-31. Facts here are volatile; recheck them before relying on them after this date.

## Finding on 2026-10-28

OSV-Scanner over `tools/go.mod` reports two advisories in `golang.org/x/net` 0.60.0, an indirect dependency of the pinned tools only; `backend/go.mod` does not require `golang.org/x/net`. Both were published on 2026-10-27T21:16Z and are fixed in 0.62.0, which the Go module proxy lists as published on 2026-10-27T19:00Z. Under the release age of [ADR-0017](../../docs/decisions/0017-dependency-release-age.md), 0.62.0 may be adopted only after 2026-10-30T19:00Z, so the tools module keeps 0.60.0 until then, by the owner's decision of 2026-10-28.

| Advisory | Alias | Affected package and symbols | Content |
|---|---|---|---|
| [GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860) | CVE-2026-78661 | `golang.org/x/net/html`: `Parse`, `ParseFragment`, `ParseFragmentWithOptions`, `ParseWithOptions`, `parser.addText`, `parser.fosterParent`, `parser.parse` | The HTML tree parser rebuilds the accumulated string each time it merges adjacent text nodes, so a document with many short text runs between ignored end tags costs quadratic time and memory (denial of service) |
| [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866) | CVE-2026-97045 | `golang.org/x/net/html`: `Render`, `childTextNodesAreLiteral` | When rendering a tree, the check for raw text stops at the nearest foreign-namespace ancestor at an HTML integration point, so a foster-parented subtree can be rendered as literal text that re-parses into another namespace (sanitiser bypass in code that parses, edits and re-renders untrusted HTML) |

Both entries cover every version before 0.62.0. Sources: the OSV entries at `api.osv.dev/v1/vulns/<id>` and the Go vulnerability database entries at `vuln.go.dev/ID/<id>.json`, which list the same packages and symbols.

## Reachability on 2026-10-28

- **Packages compiled in.** `go -C tools list -deps` over each tool's main package (`GOTOOLCHAIN` pinned to `go1.27.3`): golangci-lint, govulncheck, Gitleaks and sqlc compile in no `golang.org/x/net/html` package. OSV-Scanner compiles in `golang.org/x/net/html`, `html/atom` and `html/charset`.
- **Importers in OSV-Scanner.** `golang.org/x/net/html` is imported by `github.com/microcosm-cc/bluemonday` 1.0.27, which uses only `html.NewTokenizer`, and by `html/charset`; bluemonday is reached only through `charm.land/glamour/v2/ansi`, which only the guided-remediation subcommand `osv-scanner fix` imports. `html/charset` is imported by `github.com/google/osv-scalibr/clients/datasource` for decoding Maven registry metadata, which OSV-Scanner fetches only for Maven manifests; the repository has none. None of these call `Parse`, `ParseFragment*`, `ParseWithOptions` or `Render`.
- **Symbol analysis.** `bin/govulncheck -C tools -show verbose` over the five tool main packages reports no symbol results ("Your code is affected by 0 vulnerabilities"); both advisories appear only under package results, as imported but not called. `bin/govulncheck -mode=binary` over each binary in `bin/` reports no affected symbols for any of the five.
- **Inputs.** The registered commands run `osv-scanner scan source` over the repository's own `go.mod` files and pnpm lockfiles and send the resolved package versions to the OSV API; they never run `osv-scanner fix` and never parse HTML from the repository or from the network through the affected functions.

## Temporary acceptance on 2026-10-28

Neither advisory is reachable from the checks: no tool calls an affected symbol, and only OSV-Scanner compiles the package in, through code paths the registered commands do not use or that call only the unaffected tokenizer. Both are accepted in [`tools/osv-scanner.toml`](../../tools/osv-scanner.toml) until 2026-10-31, the first day after 0.62.0 turns three days old; the change that upgrades `golang.org/x/net` to 0.62.0 or later removes both entries. If the upgrade has not landed by then, the scan fails and the ignores are not extended without a new triage.


## Security update on 2026-10-31

The tools module now selects `golang.org/x/net` v0.62.0 through `go get golang.org/x/net@v0.62.0` followed by `go mod tidy`. Only the temporary ignores for GO-2026-6860 and GO-2026-6866 are removed. The existing GO-2025-4020 and GO-2026-5932 triages and deadlines are unchanged. The earlier reachability findings above describe the pre-update tools; they were not rerun for the new build list.

`go list -m -json <module>@<version>` confirmed these release times and origins on 2026-10-31. Every version is older than ADR-0017's three days even at 00:00 UTC on that date; the newest, x/net, crossed the threshold at 2026-10-30T19:00:16Z. The source of each module is its `https://go.googlesource.com/<name>` repository, with the named version tag except for telemetry, whose pseudo-version identifies commit `ed294f9431572147af3f0aaa3fe932a93b698664`. Version metadata is also available from `https://proxy.golang.org/golang.org/x/<name>/@v/<version>.info`; requirements are in the matching `.mod` file.

| Module under golang.org/x/ | Previous version | Selected version | Published (UTC) | Required edge |
|---|---|---|---|---|
| net | v0.60.0 | v0.62.0 | 2026-10-27T19:00:16Z | Security fix for GO-2026-6860 and GO-2026-6866 |
| crypto | v0.57.0 | v0.58.0 | 2026-10-09T16:24:17Z | net v0.62.0 requires crypto v0.58.0 |
| sys | v0.48.0 | v0.49.0 | 2026-10-06T18:35:47Z | net v0.62.0 requires sys v0.49.0 |
| term | v0.46.0 | v0.47.0 | 2026-10-09T15:22:17Z | net v0.62.0 requires term v0.47.0 |
| text | v0.42.0 | v0.43.0 | 2026-10-09T15:18:32Z | net v0.62.0 requires text v0.43.0 |
| tools | v0.50.0 | v0.51.0 | 2026-10-02T18:49:45Z | text v0.43.0 requires tools v0.51.0 |
| sync | v0.23.0 | v0.24.0 | 2026-09-23T16:28:17Z | text v0.43.0 requires sync v0.24.0 |
| telemetry | v0.0.0-20260908163034-4bcc4b2ee518 | v0.0.0-20260924152758-ed294f943157 | 2026-09-24T15:27:58Z | tools v0.51.0 requires this telemetry version |

These seven accompanying upgrades are mandatory under Go minimum version selection; retaining any previous version would violate an upstream requirement. No tool main-module pin, Go toolchain, backend dependency, dependency purpose or decided stack row changes.

The official Go vulnerability database JSON for [GO-2026-6860](https://vuln.go.dev/ID/GO-2026-6860.json) and [GO-2026-6866](https://vuln.go.dev/ID/GO-2026-6866.json), fetched on 2026-10-31, still records v0.62.0 as the fixed version. An OSV API `POST https://api.osv.dev/v1/querybatch` for the eight selected Go module versions on that date returned no entries for net, sys, term, text, tools, sync or telemetry, and only the already accepted GO-2026-5932 for crypto v0.58.0. This is advisory metadata evidence, not a replacement for the required repository scanner checks or rebuilt-binary verification.

The security scope is T-39 (vulnerable tools and dependency integrity), with the unchanged consequences T-19 (developer secrets) and T-41 (repository and agent configuration). The update introduces no application capability or new trust boundary. Existing tool diagnostics and observability contracts are unchanged. On 2026-10-31 all five tools were rebuilt with Go 1.27.3 against this build list. `tools/check-tool-binaries.sh` exited 0 with `bin/ matches tools/go.mod`; the rebuilt OSV-Scanner over `tools/go.mod` exited 0 with no unfiltered issues and only the two unchanged accepted triages; `go -C tools mod tidy -diff` exited 0. The full merge gate remains pending.
