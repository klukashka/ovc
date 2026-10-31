#!/usr/bin/env bash
set -euo pipefail
for dev_env_check_key in "${!OVC_@}"; do
  unset "$dev_env_check_key"
done
work="$(mktemp -d "${TMPDIR:-/tmp}/dev-env-check.XXXXXX")"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/tools" "$work/backend"
cp "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/dev-env.sh" "$work/tools/dev-env.sh"
wrapper="$work/tools/dev-env.sh"
secret="fake-root-value-7f3a"

write_files() {
  printf 'OVC_DEV_POSTGRES_PASSWORD=%s\nOVC_SHARED_ROOT=root\n\n' "$secret" >"$work/.env"
  printf 'OVC_PUBLIC_URL=http://127.0.0.1:5173\r\nOVC_INJECTED=$(touch %s/pwned)\nOVC_GLOB=*\nOVC_LAST=no-final-newline' "$work" >"$work/backend/.env"
}

check() {
  if [[ "$1" != "$2" ]]; then
    echo "FAIL: $3: got [$1], want [$2]"
    exit 1
  fi
  echo "ok: $3"
}

refusal() {
  set +e
  local message
  message="$("$wrapper" true 2>&1)"
  local status=$?
  set -e
  check "$status" "1" "$1 is refused"
  check "$message" "$2" "$1 names only the file and line"
}

write_files
loaded="$("$wrapper" bash -c 'printf "%s|%s|%s|%s|%s" "$OVC_PUBLIC_URL" "${#OVC_DEV_POSTGRES_PASSWORD}" "$OVC_GLOB" "$OVC_LAST" "$OVC_INJECTED"')"
check "$loaded" "http://127.0.0.1:5173|${#secret}|*|no-final-newline|\$(touch $work/pwned)" "values load literally, CR is stripped, the last line needs no newline"
check "$([[ -e "$work/pwned" ]] && echo yes || echo no)" "no" "values are never executed"
check "$("$wrapper" true 2>&1)" "" "a successful run prints nothing"
check "$(bash -x "$wrapper" true 2>&1 | grep -c "$secret" || true)" "0" "bash -x does not trace the loaded values"
check "$(env SHELLOPTS=xtrace "$wrapper" true 2>&1 | grep -c "$secret" || true)" "0" "SHELLOPTS=xtrace does not trace the loaded values"
printf 'set -x\n' >"$work/trace-startup"
traced="$(env "BASH_ENV=$work/trace-startup" "$wrapper" bash -c ': "$OVC_DEV_POSTGRES_PASSWORD"' 2>&1 | grep -c "$secret" || true)"
check "$traced" "0" "BASH_ENV does not reach the command's shell to trace the loaded values"
traced="$(env "ENV=$work/trace-startup" "$wrapper" sh -i -c ': "$OVC_DEV_POSTGRES_PASSWORD"' </dev/null 2>&1 | grep -c "$secret" || true)"
check "$traced" "0" "ENV does not reach an interactive sh to trace the loaded values"
printf 'trap '"'"'printf "%%s\\n" "${OVC_DEV_POSTGRES_PASSWORD:-}" >&2'"'"' DEBUG\nset -o functrace\n' >"$work/trap-startup"
trapped="$(env "BASH_ENV=$work/trap-startup" "$wrapper" true 2>&1 | grep -c "$secret" || true)"
check "$trapped" "0" "a DEBUG trap set before the wrapper starts does not print the loaded values"
mkdir -p "$work/zdotdir"
printf 'setopt xtrace\n' >"$work/zdotdir/.zshenv"
if command -v zsh >/dev/null 2>&1; then
  traced="$(env "ZDOTDIR=$work/zdotdir" "$wrapper" zsh -c ': "$OVC_DEV_POSTGRES_PASSWORD"' 2>&1 | grep -c "$secret" || true)"
  check "$traced" "0" "ZDOTDIR does not reach the command's zsh to trace the loaded values"
else
  echo "skipped: zsh is not installed"
fi
check "$(BASH_XTRACEFD=1 "$wrapper" bash -c 'printf "%s" "${BASH_XTRACEFD:-unset}"')" "unset" "BASH_XTRACEFD does not reach the command"
check "$(BASH_XTRACEFD=2 "$wrapper" bash -c 'printf "out"; printf "err" >&2' 2>&1)" "outerr" "the command keeps its standard output and error when BASH_XTRACEFD was set"

kept="$(OVC_PUBLIC_URL=https://kept.example "$wrapper" bash -c 'printf "%s" "$OVC_PUBLIC_URL"')"
check "$kept" "https://kept.example" "an existing variable is kept"

set +e
printed="$("$wrapper" /usr/bin/env 2>/dev/null)"
status=$?
set -e
check "$status|$printed" "2|" "env is refused even by its full path and prints nothing"

for printer in printenv set export declare typeset compgen; do
  set +e
  printed="$("$wrapper" "$printer" 2>/dev/null)"
  status=$?
  set -e
  check "$status|$printed" "2|" "$printer is refused and prints nothing"
done

rm -f "$work/backend/.env"
check "$("$wrapper" bash -c 'printf "%s|%s" "${OVC_SHARED_ROOT:-}" "${OVC_PUBLIC_URL:-unset}"')" "root|unset" "a missing file is skipped"

write_files
printf 'not a setting\n' >>"$work/.env"
refusal "a malformed line" "dev-env: line 4 of .env is not OVC_KEY=VALUE"

write_files
printf 'BASH_ENV=/tmp/x\n' >>"$work/.env"
refusal "a key outside OVC_" "dev-env: line 4 of .env is not OVC_KEY=VALUE"

write_files
printf 'export OVC_X=1\n' >>"$work/.env"
refusal "an export line" "dev-env: line 4 of .env is not OVC_KEY=VALUE"

write_files
printf '\nOVC_QUOTED="value"' >>"$work/backend/.env"
refusal "a quoted value" "dev-env: line 5 of backend/.env quotes its value; write the value without quotes"

write_files
printf 'OVC_DEV_POSTGRES_PASSWORD=again\n' >>"$work/.env"
refusal "a key repeated in one file" "dev-env: line 4 of .env repeats a key already set in one of the files"

write_files
printf '\nOVC_SHARED_ROOT=backend' >>"$work/backend/.env"
refusal "a key repeated across the files" "dev-env: line 5 of backend/.env repeats a key already set in one of the files"

for other_shell in zsh dash; do
  if command -v "$other_shell" >/dev/null 2>&1; then
    set +e
    message="$("$other_shell" "$wrapper" true 2>&1)"
    status=$?
    set -e
    check "$status|$message" "2|dev-env: run it with bash" "$other_shell is refused"
  else
    echo "skipped: $other_shell is not installed"
  fi
done

write_files
overridden="$(
  echo() { printf 'leaked-through-a-function'; }
  export -f echo
  "$wrapper" 2>&1 || true
)"
check "$overridden" "usage: tools/dev-env.sh <command> [arguments...]" "an exported echo function does not replace the wrapper's echo"

write_files
leaked="$(
  continue() { printf '%s\n' "${OVC_DEV_POSTGRES_PASSWORD:-}" >&2; }
  export -f continue
  "$wrapper" true 2>&1 | grep -c "$secret" || true
)"
check "$leaked" "0" "an exported continue function does not run while the files load"

echo "all checks passed"
