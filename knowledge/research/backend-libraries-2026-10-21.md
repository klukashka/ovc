# Backend libraries for roadmap slice 2

Checked 2026-10-21 against the Go module proxy (`go list -m -versions` and `go list -m -json <module>@<version>` for the publication time) and OSV-Scanner. Releases younger than two weeks were skipped. Facts here are volatile; recheck them before relying on them after this date. Each library is added to this file in the change that adopts it.

## Versions

| Component | Version | Published | Licence | Where it is pinned | Adopted in |
|---|---|---|---|---|---|
| sqlc (`github.com/sqlc-dev/sqlc/cmd/sqlc`) | 1.31.1 | 2026-04-22 | MIT | Go `tool` in `tools/go.mod` | the accounts schema change |
| cel-go (`github.com/google/cel-go`, indirect through sqlc) | 0.30.0 | 2026-07-25 | Apache-2.0 | `tools/go.mod` | the accounts schema change |

## Notes

- sqlc 1.31.1 chooses its PostgreSQL parser by build tag: with cgo enabled (the default wherever a C compiler is installed, which `go test -race` already requires) it compiles the bundled libpg_query C sources through `pg_query_go`; with `CGO_ENABLED=0` it uses `github.com/wasilibs/go-pgquery`, the same parser compiled to WebAssembly. The committed code was generated and checked with cgo enabled. It parsed the slice 2 schema, including `UNIQUE NULLS NOT DISTINCT` and partial indexes, and `sqlc diff` exits 0 against the committed `internal/dbgen`.
- sqlc pulled in cel-go 0.28.0, which OSV-Scanner reports as GO-2026-6094 (GHSA-gcjh-h69q-9w9g, CVSS 6.3, fixed in 0.30.0). The tools module requires cel-go 0.30.0 instead; sqlc builds with it and generates identical code. After the upgrade, OSV-Scanner reports no issues for `tools/go.mod` and `backend/go.mod`, apart from the findings already accepted in `tools/osv-scanner.toml`.
- The 1.30.0 release (2025-09-01) was also available; 1.31.1 was chosen as the newest release older than two weeks.
