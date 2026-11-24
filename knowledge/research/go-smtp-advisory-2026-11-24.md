# go-smtp advisory GO-2026-7029 (2026-11-24)

Status: agent triage for the update of `github.com/emersion/go-smtp` from 0.25.0 to 0.25.1, approved by the owner on 2026-11-24; checked on 2026-11-24 at `main` 2e78af1. Cited from the [stack table](../architecture/detailed.md#stack-per-component) and from T-39 in [the threat model](../../docs/threat-model.md) as evidence, not as a decision.

## Advisory

- GO-2026-7029, "Unbounded memory allocation during BDAT transaction in github.com/emersion/go-smtp", published 2026-11-23T20:05Z, reviewed in the Go vulnerability database (`https://api.osv.dev/v1/vulns/GO-2026-7029`, modified 2026-11-23T20:30Z).
- Mechanism: the server's `Conn.handleBdat` turns off the line-length limit of the connection reader while it reads a BDAT chunk and restored it only on an error or on the final `BDAT … LAST` chunk. A client that sends another command between two BDAT commands of one transaction bypasses `Server.MaxLineLength`, so the server allocates without bound (denial of service).
- Affected: `github.com/emersion/go-smtp` from 0.15.0 up to, not including, 0.25.1. Symbols: `Conn.handleBdat`, `Server.Serve`, `Server.ListenAndServe`, `Server.ListenAndServeTLS`. Only the server side is affected.
- Fix: the advisory's fix reference, `eb903bcc09670a2b04a7b5f210c29c465289df1a`, is the v0.26.0 tag commit. 0.25.1 carries a cherry-pick of it as its own tag commit `0ed1f057d5110fb8071dcfb7b8d2e72ec05067a6`, branched from v0.25.0, whose commit message says the server restores the line-length limit unconditionally in `Conn.handleBdat`; the v0.25.1 release on GitHub has no notes. OVC takes the 0.25 patch release rather than 0.26.0, released the same day, as the smaller change.
- The other upstream commits between v0.25.0 and v0.25.1 are server-side (discarding a rejected BDAT chunk, consuming the rest of an oversized LMTP body) except one: `Client.Mail` now clears the client's recipient list after a successful `MAIL FROM`. That list is read only for LMTP responses, so OVC's single-recipient SMTP sends on reused connections behave as before.

## Release age

The Go module proxy (`https://proxy.golang.org/github.com/emersion/go-smtp/@v/v0.25.1.info`) records v0.25.1 at 2026-11-05T08:42Z, tag commit `0ed1f057d5110fb8071dcfb7b8d2e72ec05067a6`. It is 19 days old on 2026-11-24, older than the 3 days of ADR-0017 and also older than its later 14 days.

## Reachability in OVC

- Production code uses only the client. `backend/internal/adapters/mail/smtp.go` builds an `smtp.Client` with `smtp.NewClient` or `smtp.NewClientStartTLS` to send the worker's mail to the configured provider. None of the affected symbols are reachable from it.
- `smtp.NewServer` appears only in `backend/internal/adapters/mail/mail_test.go`, the adapter's contract tests, as an in-process SMTP peer listening on `127.0.0.1` on an ephemeral port that only the test's own client reaches.
- depguard in `backend/.golangci.yml` keeps the whole package inside `internal/adapters`, so no other layer can start a server.
- Before the update OVC was therefore not exposed, but OSV-Scanner over `backend/go.mod` reports the module version regardless of the symbols it uses, and it fails the merge gate, so the update is the fix rather than an accepted finding.

## Change

- `go get github.com/emersion/go-smtp@v0.25.1` and `go mod tidy` in `backend/`: `go.mod` moves go-smtp to v0.25.1 and `go.sum` replaces its two hashes. `github.com/emersion/go-sasl` stays at `v0.0.0-20241020182733-b788ff22d5a6`, and no other module moves.
- The stack table's Email row names 0.25.1.
