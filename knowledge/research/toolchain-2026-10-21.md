# Backend toolchain and library versions

Checked 2026-10-21 against the Go module proxy (`go list -m -versions -json`) and the libraries' source at the pinned versions. Releases younger than two weeks were skipped. Facts here are volatile; recheck them before relying on them after this date. The documentation tools' accepted findings were rechecked on 2026-10-22.

## Versions

| Component | Version | Published | Where it is pinned |
|---|---|---|---|
| Go toolchain | 1.27.3 | — | `toolchain` lines in `tools/go.mod` and `backend/go.mod` |
| huma | 2.39.1 | 2026-07-29 | `backend/go.mod` |
| pgx | 5.10.0 | 2026-06-03 | `backend/go.mod` (goose 3.28.0 requires at least 5.10.0) |
| goose | 3.28.0 | 2026-09-02 | `backend/go.mod`; 3.29.0 of 2026-10-11 skipped |
| golangci-lint | 2.14.0 | 2026-09-24 | `tools/go.mod`; 2.15.0 of 2026-10-16 skipped |
| govulncheck (`golang.org/x/vuln`) | 1.8.0 | 2026-09-08 | `tools/go.mod`; 1.9.0 of 2026-10-11 skipped |

golangci-lint and govulncheck must type-check the backend module, which `go -C tools tool` cannot do from the tools module, so they are built from the pinned versions into the ignored `bin/` directory and run from `backend/` ([development guide](../../docs/development.md#backend-checks)).

## huma defaults confirmed at 2.39.1

- `huma.DefaultConfig` serves the OpenAPI document at `/openapi.json` and `/openapi.yaml`, documentation at `/docs` rendered with Stoplight Elements loaded from a third-party CDN, and schemas under `/schemas`, and adds a `$schema` link through a create hook. OVC clears all three paths and the hooks, and serves only the JSON document at `/api/docs/openapi.json` when `OVC_API_DOCS_ENABLED=true`.
- An operation with no explicit limits gets `MaxBodyBytes` of 1 MiB and `BodyReadTimeout` of 5 seconds (`huma.go`, `Register`). OVC sets 1 MiB and 30 seconds on every operation explicitly.
- The default error model is RFC 9457 problem details whose `errors[].value` echoes client input. OVC replaces `huma.NewError` and `huma.NewErrorWithContext` with the error shape of the [API conventions](../specs/api.md#conventions).
- The default formats are JSON only; CBOR needs an explicit import, which OVC does not add.

## Documentation tools

Pinned 2026-10-21 in `tools/docs` (package manifest, lockfile and pnpm policy) instead of being fetched with `npx` on every run.

| Tool | Version | Note |
|---|---|---|
| markdownlint-cli | 0.49.1 (2026-07-14) | 0.45.0 pulled `glob`, `js-yaml`, `markdown-it` and `minimatch` versions with high-severity advisories; 0.49.1 adds rule `MD060`, turned off in `.markdownlint.json` |
| @mermaid-js/mermaid-cli | 12.0.0 (2026-09-24) | Declares `puppeteer` ^25 as a peer |
| puppeteer | 25.13.0 (2026-10-08; 25.12.0 of 2026-09-23 until 2026-10-25) | Its install script, which downloads a browser, is denied in `tools/docs/pnpm-workspace.yaml` |
| playwright-core | 1.63.0 | Same version as the frontend's Playwright, used only to locate its Chromium for the renderer |
| lychee | 0.24.2 | Installed outside the repository with the official instructions; no checksum is recorded |

Overrides in `tools/docs/pnpm-workspace.yaml` raise `smol-toml` to 1.9.0 and `js-yaml` 5 to 5.4.1, both older than two weeks, to clear their advisories. Accepted findings, listed in `tools/docs/osv-scanner.toml` (OSV-Scanner over `tools/docs/pnpm-lock.yaml`, rechecked 2026-10-24):

| Advisory | Package | Reason |
|---|---|---|
| GHSA-gfr2-rv44-r23c (high, CVSS 7.5), GHSA-vfgh-jcmw-g37q (low, CVSS 3.7) | `markdown-it` 14.3.2 | The fix 14.3.3 was published 2026-10-16 and is younger than the minimum release age; the linter parses only this repository's Markdown. Accepted until 2026-11-15 |
| GHSA-238p-pmpm-9mq7 (low, CVSS 2.1) | `katex` 0.16.47 | Fixed only in the 0.18 line, outside mermaid's 0.16 range; the renderer processes only this repository's diagrams offline. Accepted until 2027-01-21 |
