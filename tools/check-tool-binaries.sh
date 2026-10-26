#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/tool-binaries.XXXXXX")"
trap 'rm -rf "$work"' EXIT
go -C "$root/tools" list -m -f '{{.Path}} {{.Version}}' all >"$work/pinned"
toolchain="$(awk '$1 == "toolchain" { print $2 }' "$root/tools/go.mod")"
if [[ -z "$toolchain" ]]; then
  toolchain="go$(awk '$1 == "go" { print $2 }' "$root/tools/go.mod")"
fi
status=0
check() {
  local binary="$1" module="$2"
  local path="$root/bin/$binary"
  if [[ ! -x "$path" ]]; then
    echo "bin/$binary is missing; build the pinned tools as docs/development.md says" >&2
    status=1
    return
  fi
  go version -m "$path" >"$work/info"
  local problems
  problems="$(awk -v module="$module" -v toolchain="$toolchain" -v pinned="$work/pinned" -v sums="$root/tools/go.sum" '
    BEGIN {
      while ((getline line < pinned) > 0) { split(line, field, " "); version[field[1]] = field[2] }
      while ((getline line < sums) > 0) { split(line, field, " "); if (field[2] !~ /\/go\.mod$/) { sum[field[1] " " field[2]] = field[3] } }
    }
    NR == 1 { if ($2 != toolchain) { print "built with " $2 " instead of the pinned " toolchain } }
    $1 == "mod" {
      seen = 1
      if ($2 != module) { print "its main module is " $2 " instead of " module }
    }
    $1 == "mod" || $1 == "dep" {
      if (!($2 in version)) { print $2 " is not in the tools build list" }
      else if ($3 != version[$2]) { print $2 " is " $3 " instead of the pinned " version[$2] }
      else if ($4 !~ /^h1:/ || $4 != sum[$2 " " $3]) { print $2 " " $3 " has a checksum that differs from tools/go.sum" }
    }
    $1 == "=>" { print "a dependency was replaced by " $2 }
    END { if (!seen) { print "it carries no module information" } }
  ' "$work/info")"
  if [[ -n "$problems" ]]; then
    while IFS= read -r problem; do
      echo "bin/$binary: $problem; rebuild the pinned tools" >&2
    done <<<"$problems"
    status=1
  fi
}
check golangci-lint github.com/golangci/golangci-lint/v2
check govulncheck golang.org/x/vuln
check gitleaks github.com/zricethezav/gitleaks/v8
check osv-scanner github.com/google/osv-scanner/v2
check sqlc github.com/sqlc-dev/sqlc
if [[ "$status" -eq 0 ]]; then
  echo "bin/ matches tools/go.mod"
fi
exit "$status"
