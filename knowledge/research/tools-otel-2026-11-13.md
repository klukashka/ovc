# Tool OpenTelemetry dependency fix

Checked: 2026-11-13. Scope: the tools module only. Application dependencies are unchanged; no other Go module in the repository requires `go.opentelemetry.io`.

## Advisory and fixed version

The OSV-Scanner scan of `tools/go.mod` reported [GHSA-q7gw-24fr-5p69](https://osv.dev/GHSA-q7gw-24fr-5p69):
- CVSS 5.3, medium;
- affects `go.opentelemetry.io/otel/trace` 1.45.0;
- fixed in 1.47.0.

## Release age

The Go module proxy's `.info` records give the same release time for each of the four modules: `go.opentelemetry.io/otel`, `otel/trace`, `otel/metric` and `otel/log`, all v1.47.0.
- **Release time:** 2026-10-02T08:10:46Z.
- **Source:** upstream commit `66cfc9520e205b7d450183532772401bc2b6674c` of `github.com/open-telemetry/opentelemetry-go`.
- **Tags:** `v1.47.0`, `trace/v1.47.0`, `metric/v1.47.0` and `log/v1.47.0`.

On the inspection date the release was 42 days old, which exceeds ADR-0017's minimum of 3 days (4320 minutes). Advisory publication is distinct from package release age.

## Change and exposure

`go get go.opentelemetry.io/otel/trace@v1.47.0` and `go mod tidy` raised `otel`, `otel/metric` and `otel/trace` from 1.45.0 to 1.47.0 as indirect requirements. They also added `otel/log` 1.47.0, which `otel` 1.47.0 itself requires. The otel SDK modules stay at 1.45.0, a combination that Go's minimum version selection allows.

`go mod why -m` reaches the modules through OSV-Scanner and its image layer scanning, through the moby client and otelhttp. This establishes inclusion, not the exploitability of a configured command.

## Verification

- `go -C tools mod verify` reported all modules verified.
- `go -C tools mod tidy -diff` printed no difference.
- The five pinned tools were rebuilt with the command in the development guide, and `tools/check-tool-binaries.sh` printed `bin/ matches tools/go.mod`.
- `bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod` reported no issues. Only the two existing dated filters apply, and no suppression was added for this advisory.
