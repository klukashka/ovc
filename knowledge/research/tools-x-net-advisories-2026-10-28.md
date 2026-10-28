# The tools module's golang.org/x/net advisories

Checked 2026-10-28. Facts here are volatile; recheck them before relying on them after this date.

## Finding

OSV-Scanner over `tools/go.mod` reports two advisories in `golang.org/x/net` 0.60.0, an indirect dependency of the pinned tools only; `backend/go.mod` does not require `golang.org/x/net`. Both were published on 2026-10-27T21:16Z and are fixed in 0.62.0, which the Go module proxy lists as published on 2026-10-27T19:00Z. Under the release age of [ADR-0017](../../docs/decisions/0017-dependency-release-age.md), 0.62.0 may be adopted only after 2026-10-30T19:00Z, so the tools module keeps 0.60.0 until then, by the owner's decision of 2026-10-28.

| Advisory | Alias | Affected package and symbols | Content |
|---|---|---|---|
| [GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860) | CVE-2026-78661 | `golang.org/x/net/html`: `Parse`, `ParseFragment`, `ParseFragmentWithOptions`, `ParseWithOptions`, `parser.addText`, `parser.fosterParent`, `parser.parse` | The HTML tree parser rebuilds the accumulated string each time it merges adjacent text nodes, so a document with many short text runs between ignored end tags costs quadratic time and memory (denial of service) |
| [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866) | CVE-2026-97045 | `golang.org/x/net/html`: `Render`, `childTextNodesAreLiteral` | When rendering a tree, the check for raw text stops at the nearest foreign-namespace ancestor at an HTML integration point, so a foster-parented subtree can be rendered as literal text that re-parses into another namespace (sanitiser bypass in code that parses, edits and re-renders untrusted HTML) |

Both entries cover every version before 0.62.0. Sources: the OSV entries at `api.osv.dev/v1/vulns/<id>` and the Go vulnerability database entries at `vuln.go.dev/ID/<id>.json`, which list the same packages and symbols.

## Reachability

- **Packages compiled in.** `go -C tools list -deps` over each tool's main package (`GOTOOLCHAIN` pinned to `go1.27.3`): golangci-lint, govulncheck, Gitleaks and sqlc compile in no `golang.org/x/net/html` package. OSV-Scanner compiles in `golang.org/x/net/html`, `html/atom` and `html/charset`.
- **Importers in OSV-Scanner.** `golang.org/x/net/html` is imported by `github.com/microcosm-cc/bluemonday` 1.0.27, which uses only `html.NewTokenizer`, and by `html/charset`; bluemonday is reached only through `charm.land/glamour/v2/ansi`, which only the guided-remediation subcommand `osv-scanner fix` imports. `html/charset` is imported by `github.com/google/osv-scalibr/clients/datasource` for decoding Maven registry metadata, which OSV-Scanner fetches only for Maven manifests; the repository has none. None of these call `Parse`, `ParseFragment*`, `ParseWithOptions` or `Render`.
- **Symbol analysis.** `bin/govulncheck -C tools -show verbose` over the five tool main packages reports no symbol results ("Your code is affected by 0 vulnerabilities"); both advisories appear only under package results, as imported but not called. `bin/govulncheck -mode=binary` over each binary in `bin/` reports no affected symbols for any of the five.
- **Inputs.** The registered commands run `osv-scanner scan source` over the repository's own `go.mod` files and pnpm lockfiles and send the resolved package versions to the OSV API; they never run `osv-scanner fix` and never parse HTML from the repository or from the network through the affected functions.

## Conclusion

Neither advisory is reachable from the checks: no tool calls an affected symbol, and only OSV-Scanner compiles the package in, through code paths the registered commands do not use or that call only the unaffected tokenizer. Both are accepted in [`tools/osv-scanner.toml`](../../tools/osv-scanner.toml) until 2026-10-31, the first day after 0.62.0 turns three days old; the change that upgrades `golang.org/x/net` to 0.62.0 or later removes both entries. If the upgrade has not landed by then, the scan fails and the ignores are not extended without a new triage.
