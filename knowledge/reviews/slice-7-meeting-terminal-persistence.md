# Meeting cleanup terminal persistence verification

Date: 2026-11-06. Base: `658be0514fca80c5264cf9981547003fc1a4fa71`. Status: implemented; independent standard/security reviews passed without findings; change checks passed; ordinary merge gate pending. Tier: standard. Security trigger: provider cleanup evidence, retained uncertainty, capacity accounting and safe outbox diagnostics. The [approved local completion plan](../plans/slice-7-local-completion.md) permits this runtime prerequisite as a separate change.

## Scope

The typed terminal transaction binds the current meeting cleanup lease and cycle, checks successful effect evidence revisions, and atomically persists settlement/capacity release, ledger result, durable receipt and terminal outbox status. Final failure and expired eighth leases preserve exhausted receipts without settlement. Generic outbox terminal paths refuse this kind; other kinds retain their existing behavior. Migration 26 indexes expired meeting leases. Reclamation has a 100-candidate/20-second bound and nonblocking locks in capacity–ledger–outbox order; it continues after candidate errors within its budget.

No provider lane, creation coordinator, minute meeting recovery scanner, operator command, public meeting route, UI or BBB activation is added. The earlier standalone repository transitions are not claimed to be atomic effect-completion APIs. The new transaction still requires the future provider coordinator to supply trustworthy observed-generation evidence; database success alone does not prove remote erasure.

## Integration baseline

The [storage verification history](slice-7-meeting-recovery-storage.md) retains the earlier presence shutdown, documentation-browser and tools vulnerability failures. Their causes and historical outcomes are not erased by later acceptance. After the approved compression dependency correction, the storage integration commit `658be0514fca80c5264cf9981547003fc1a4fa71` passed all 30 ordinary gate commands and 979 E2E tests in 274.9 seconds. Ordinary finish confirmed tested/main tree `5e9ebc59d0e2ea1a81fb2e4ea34b4054c5d2b13c`. That integrated baseline is not a test pass for this new terminal-persistence diff.

## Executed checks

| Check | Result |
| --- | --- |
| `sqlc generate` after query changes | Passed |
| `gofmt` on handwritten changed Go files | Passed |
| Initial compile-only `go test ./internal/repositories ./internal/services/outbox -run '^$'` | Blocked before compilation by sandbox access to the standard Go build cache; no assertion ran |
| Same compile-only command with approved cache access | Passed |
| Initial targeted real PostgreSQL race run | Passed, 3.714 seconds |
| Targeted race run after adding receipt rollback and terminal/reclaim race assertions | Passed, 3.269 seconds |
| `git diff --check` | Passed |
| Full backend, static analysis and documentation change checks | Passed on the corrected source; final backend 91.45 seconds, 20 safe samples, unchanged source; vet/lint/sqlc/docs exit 0 |
| Independent standard and full-security reviews | Passed without findings over the functional diff; later identical-struct conversion is recorded below |
| Remaining full-run checks and ordinary exact-tree gate/finish | Deferred to the merge gate; no pass claimed |
| Remote compatibility, media and remote physical erasure | Unverified; no remote call authorized or performed |

The final targeted command, invoked through the sanctioned development environment wrapper, was:

```sh
go test -race ./internal/repositories -run 'TestMeetingRecovery(Terminal|EighthFailure|ExpiredLeases|LateAcknowledgementFence|MissingReceiptRefused|ReclaimSkipsLockedLedger|ReceiptMismatchRollsBack)' -count=1
```

The tests exercise real PostgreSQL transactions and the actual outbox service. They cover prepared and acknowledged settlement, uncertain/conflicting retained outcomes, lease/revision/generation refusals, generic terminal bypass prevention, receipt retention after outbox deletion, eight failed claims, expired early/final leases, late acknowledgement and final-cycle eligibility, missing/mismatched receipts, cancellation during a real capacity lock, nonblocking progress past a locked ledger, safe diagnostics and a completion/reclaim race. Raw logs remain outside repository and served directories.

## Negative controls and coordinator evidence

Use isolated disposable copies and the unchanged targeted assertions: remove the successful-effect revision comparison; remove exact-generation settlement equality; allow a refused receipt update to commit settlement; permit generic completion/kill of meeting cleanup; or requeue the expired eighth lease. All five isolated mutations failed their corresponding unchanged assertion: revision 3.61 seconds, generation 2.78 seconds, receipt rollback 2.42 seconds, generic terminal bypass 2.21 seconds and expired eighth lease 2.51 seconds. No modified source entered this branch.

The first coordinator full-backend command failed during setup because its monitored working directory was the repository root, not the backend module; no test assertion ran. With explicit `go -C backend`, full `go test -race -count=1 ./...` passed in 94.11 seconds, with 20 safe monitor samples and unchanged source. Vet passed. Staticcheck then required an explicit conversion between two identical generated structs instead of copying their fields (S1016). The one-expression correction changes no field or lifecycle behavior and touches no trust boundary; it requires checks but no additional independent review under the minor-fix rule. Both independent reviews examined the complete functional diff and found no issues; the mechanical conversion is distinguished from that reviewed state. The earlier failed setup and lint outcomes remain preserved. Corrected vet, lint, sqlc diff and documentation checks passed. The final full backend passed in 91.45 seconds with 20 safe monitor samples and unchanged source. The ordinary exact-tree gate/finish remain pending at this record preparation.
