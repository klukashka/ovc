# Local check budget after the in-app notifications

Measured 2026-10-25 on the branch of the in-app notifications change (S5 of the [slice 4 plan](../plans/slice-4.md)). The change adds 26 Go integration tests in `internal/api`, one more denial test for routes that take identifiers in the body, and one more case in the reclaim race table, and no end-to-end test. Section 9 of the plan asks for a record after every change that adds tests. The comparison point is the [record after the WebSocket gateway](check-budget-2026-10-25-websocket-gateway.md), which measured 179.5 seconds and 169.5 seconds in two warm runs.

## Method and its limits

- **Machine:** the same development machine as the comparison record, with warm caches. The machine was not idle. The one-minute load average was 10.8 when the measured run started, because an earlier check run had just finished, and 15.0 at its end. No other session ran checks.
- **Commands:** one script ran the commands of the full local check run one after another and timed each one. The commands are the same as in the comparison record:
  - the documentation checks;
  - Gitleaks over the history and over staged changes;
  - the four OSV-Scanner lockfile scans;
  - the backend checks: `go mod tidy -diff`, `gofmt`, `go vet`, golangci-lint, govulncheck, `go test -race -count=1 ./...` through `tools/dev-env.sh`, and `sqlc diff`;
  - the frontend checks, including `pnpm install --frozen-lockfile` and `pnpm e2e` through `tools/dev-env.sh`;
  - the secret guard hook check and the local settings wrapper check.
- **Result of the measured run:** every command exited 0.
- **Runs:** three warm runs, in this order:
  1. 183 seconds, before consolidation;
  2. 174 seconds, after consolidation; golangci-lint failed in this run, so it is not the measurement;
  3. 178 seconds, the measured run on the final tree.

## Consolidation

The first run took 183 seconds, which is above the 180-second consolidation threshold. Each S5 test harness creates its own database and migrates it, which costs about 0.57 seconds before the test does anything. The three largest subtest tables now share one harness each:

- `TestEveryRelationshipChangePublishesEntityChangedToItsParties`;
- `TestInvitationAcceptanceNotifiesTheTutorWithTheCardName`;
- `TestCodeEntrySendsTheStudentOneIdenticalFrameForEveryOutcome`.

Their subtests run one after another on numbered accounts. The relationship table's purge case runs last, because it moves the shared clock forward. This saves 16 harnesses, and every named case still runs.

## Results

| Measurement | Time (s) |
|---|---|
| Full run as listed above, wall time | 178 |
| `go test -race -count=1 ./...`, wall time | 47 |
| `internal/api`, the longest package | 46.3 |
| `cmd/internal/startup` | 28.0 |
| `internal/worker` | 21.4 |
| `pnpm e2e`, wall time, 559 tests | 90 |
| Documentation checks | 7 |
| Four OSV-Scanner scans and both Gitleaks scans | 7 |
| Frontend checks before `pnpm e2e` | 17 |
| Backend checks other than `go test` | 5 |
| Secret guard hook and local settings wrapper checks | 5 |

- **Go tests:** 47 seconds, against 41.9 seconds in the comparison record. `internal/api` still bounds the Go wall time, at 46.3 seconds against 40.7.
- **End-to-end suites:** 90 seconds, with no test added. This is within the run-to-run spread of the earlier records, which ranged from 84 to 94 seconds.
- **Against the thresholds:** 178 seconds is under the 180-second threshold and equal to the plan's slice target of at most 178 seconds. The plan's target assumes a warm, idle machine, and this run started on a loaded one.
  - F0 to F3 still add Playwright tests, so they have no margin. F1 and F2 should reuse the pilot journeys and shared seeds as section 9 plans.
  - The cold idle-machine run of the slice review remains the deciding measurement.
  - The 10-minute budget of NFR-6.2 is not at risk.
