# Tool compression dependency fixes

Checked: 2026-11-06. Scope: tools module only; application dependencies are unchanged.

## Primary evidence and release age

- [GO-2026-6888](https://vuln.go.dev/ID/GO-2026-6888.json), published 2026-11-05, describes excessive allocations while decoding crafted Brotli streams and identifies `github.com/andybalholm/brotli v1.2.7` as fixed. The [official release page](https://github.com/andybalholm/brotli/releases/tag/v1.2.7) exposes release time `2026-10-23T17:03:27Z`; the [Go proxy metadata](https://proxy.golang.org/github.com/andybalholm/brotli/@v/v1.2.7.info) agrees on version, time and commit `33c2a9884269798192b5ecf171288f98939dbe42`. The GitHub release API returned 404; the public release page supplied the date instead.
- [GO-2026-6865](https://vuln.go.dev/ID/GO-2026-6865.json), published 2026-11-05, describes a panic on two-block frames and identifies `github.com/klauspost/compress v1.20.2` as fixed. The [official release API](https://api.github.com/repos/klauspost/compress/releases/tags/v1.20.2) and [release page](https://github.com/klauspost/compress/releases/tag/v1.20.2) identify publication at `2026-10-12T10:08:46Z`. The [Go proxy metadata](https://proxy.golang.org/github.com/klauspost/compress/@v/v1.20.2.info) identifies the earlier commit time `2026-10-12T09:34:06Z` and commit `7f5d8528720c895a6106f201719f87eb030a565d`; commit time is not substituted for publication time.

Both releases exceed ADR-0017's 72-hour minimum on the inspection date. Advisory publication is distinct from package release age. `go get` and `go mod tidy` changed only these two module versions and their checksum pairs; no additional transitive version needs admission. The five tool versions and Go toolchain remain unchanged. Normal Go checksum verification applies; no replacement or checksum bypass is introduced.

## Exposure and correction

`go mod why -m` reaches both modules through Gitleaks, its sources package and `github.com/mholt/archives`. This establishes dependency inclusion, not exploitability of a configured command or an exhaustive consumer inventory. The update removes the vulnerable versions without a reachability exception. Existing OSV exceptions are unchanged. All five tools are rebuilt and their embedded build information checked against the resolved module graph; this detects stale binaries, not malicious binaries forged with matching metadata.

## Evidence boundary

The failed meeting-storage gate remains failed. Successful scans after this update prove the updated tool graph only, not that the previous graph was safe. Raw logs remain local; repository records contain public dependency metadata, revision identities and command outcomes. No application runtime, authentication, provider or test behavior is changed.
