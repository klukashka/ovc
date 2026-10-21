# Backend libraries for roadmap slice 2

Checked 2026-10-21 against the Go module proxy (`go list -m -versions` and `go list -m -json <module>@<version>` for the publication time) and OSV-Scanner. Releases younger than two weeks were skipped. Facts here are volatile; recheck them before relying on them after this date. Each library is added to this file in the change that adopts it.

## Versions

| Component | Version | Published | Licence | Where it is pinned | Adopted in |
|---|---|---|---|---|---|
| sqlc (`github.com/sqlc-dev/sqlc/cmd/sqlc`) | 1.31.1 | 2026-04-22 | MIT | Go `tool` in `tools/go.mod` | the accounts schema change |
| cel-go (`github.com/google/cel-go`, indirect through sqlc) | 0.30.0 | 2026-07-25 | Apache-2.0 | `tools/go.mod` | the accounts schema change |
| x/crypto (`golang.org/x/crypto`, for `argon2`) | 0.57.0 | 2026-09-08 | BSD-3-Clause | `backend/go.mod` | the sign-in and sessions change |
| golang-jwt (`github.com/golang-jwt/jwt/v5`) | 5.3.1 | 2026-01-28 | MIT | `backend/go.mod` | the sign-in and sessions change |
| go-smtp (`github.com/emersion/go-smtp`) | 0.25.0 | 2026-08-13 | MIT | `backend/go.mod` | the outbox worker and mail change |
| go-sasl (`github.com/emersion/go-sasl`, used for SMTP PLAIN authentication) | v0.0.0-20241020182733-b788ff22d5a6 | 2024-10-20 | MIT | `backend/go.mod` | the outbox worker and mail change |
| AWS SDK for Go v2, S3 (`github.com/aws/aws-sdk-go-v2/service/s3`) | 1.114.1 | 2026-10-06 | Apache-2.0 | `backend/go.mod` | the storage and avatars change |
| AWS SDK for Go v2, static credentials (`github.com/aws/aws-sdk-go-v2/credentials`) | 1.20.7 | 2026-10-06 | Apache-2.0 | `backend/go.mod` | the storage and avatars change |
| AWS SDK for Go v2 core (`github.com/aws/aws-sdk-go-v2`, pulled in by the S3 client) | 1.47.1 | 2026-09-24 | Apache-2.0 | `backend/go.mod` | the storage and avatars change |
| smithy-go (`github.com/aws/smithy-go`, pulled in by the S3 client) | 1.28.1 | 2026-08-26 | Apache-2.0 | `backend/go.mod` | the storage and avatars change |
| x/image (`golang.org/x/image`, for the WebP decoder) | 0.46.0 | 2026-09-08 | BSD-3-Clause | `backend/go.mod` | the storage and avatars change |

## Notes

- The S3 client was checked on 2026-10-21: `service/s3` 1.114.2 and 1.114.3, `credentials` 1.20.8 and 1.20.9 and the core 1.47.2 and 1.47.3 were published on 2026-10-08 or later, younger than two weeks, so 1.114.1, 1.20.7 and 1.47.1 are pinned; every internal module the S3 client pulled in (`internal/configsources`, `internal/endpoints/v2`, `internal/v4a`, `service/internal/accept-encoding`, `service/internal/checksum`, `service/internal/presigned-url`, `service/internal/s3shared`, `aws/protocol/eventstream`) was published on 2026-09-24 or earlier. The client is built with `s3.New` from settings only; `aws-sdk-go-v2/config`, `credentials/ec2rolecreds` and `feature/ec2/imds`, which read environment variables, shared files or instance metadata, are banned by depguard. Path-style addressing is on, and request and response checksums are computed only when an operation requires them, because the CRC32 defaults break many S3-compatible providers. x/image 0.47.0 (2026-10-09) was too young, so 0.46.0 is pinned; only its `webp` decoder is imported. OSV-Scanner reported no new finding for `backend/go.mod`.
- sqlc 1.31.1 chooses its PostgreSQL parser by build tag: with cgo enabled (the default wherever a C compiler is installed, which `go test -race` already requires) it compiles the bundled libpg_query C sources through `pg_query_go`; with `CGO_ENABLED=0` it uses `github.com/wasilibs/go-pgquery`, the same parser compiled to WebAssembly. The committed code was generated and checked with cgo enabled. It parsed the slice 2 schema, including `UNIQUE NULLS NOT DISTINCT` and partial indexes, and `sqlc diff` exits 0 against the committed `internal/dbgen`.
- sqlc pulled in cel-go 0.28.0, which OSV-Scanner reports as GO-2026-6094 (GHSA-gcjh-h69q-9w9g, CVSS 6.3, fixed in 0.30.0). The tools module requires cel-go 0.30.0 instead; sqlc builds with it and generates identical code. After the upgrade, OSV-Scanner reports no issues for `tools/go.mod` and `backend/go.mod`, apart from the findings already accepted in `tools/osv-scanner.toml`.
- go-smtp 0.25.0 is the newest release older than two weeks. Its client and its server share one package, so depguard restricts the whole package to `internal/adapters`; the server is used only by the adapter's contract tests, as an in-process SMTP peer with STARTTLS and PLAIN authentication. go-sasl has no tagged releases; go-smtp requires the pseudo-version above. OSV-Scanner reports no issues for `backend/go.mod` with both added.
- x/crypto 0.58.0 (2026-10-09) was younger than two weeks, so 0.57.0 is pinned. OSV-Scanner reports GO-2026-5932, which covers `x/crypto/openpgp`; the backend imports only `argon2` and `blake2b` from x/crypto (`go list -deps ./...`) and govulncheck finds no reachable call, so `backend/osv-scanner.toml` accepts it until 2027-01-21. golang-jwt is used only for HS256: its key function returns an empty interface, which the project's type rules forbid in hand-written code, so OVC parses unverified, compares the method with HS256, verifies the signature with `SigningMethodHS256.Verify` and then validates the claims with `jwt.NewValidator`.
- The 1.30.0 release (2025-09-01) was also available; 1.31.1 was chosen as the newest release older than two weeks.
