#!/usr/bin/env bash
set +o xtrace +o verbose
if [ -z "${BASH_VERSION:-}" ]; then
  echo "dev-env: run it with bash" >&2
  exit 2
fi
unset -f exec export basename dirname pwd cd read echo printf trap continue return exit local break
trap - DEBUG RETURN ERR EXIT
set +o functrace +o errtrace
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
environment_files=(".env" "backend/.env")
refused_programs=(env printenv set export declare typeset compgen)
loaded_keys=":"

usage() {
  echo "usage: tools/dev-env.sh <command> [arguments...]" >&2
  exit 2
}

refuse_printers() {
  local program
  program="$(basename "$1")"
  for refused in "${refused_programs[@]}"; do
    if [[ "$program" == "$refused" ]]; then
      echo "dev-env: refusing to run $program, which would print the loaded settings" >&2
      exit 2
    fi
  done
}

refuse_line() {
  echo "dev-env: line $2 of $1 $3" >&2
  exit 1
}

load_file() {
  local relative="$1"
  local file="$repository_root/$relative"
  local line_number=0
  local line key value
  [[ -f "$file" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    line="${line%$'\r'}"
    [[ -z "${line//[[:space:]]/}" ]] && continue
    if [[ ! "$line" =~ ^(OVC_[A-Z0-9_]*)=(.*)$ ]]; then
      refuse_line "$relative" "$line_number" "is not OVC_KEY=VALUE"
    fi
    key="${BASH_REMATCH[1]}"
    value="${BASH_REMATCH[2]}"
    if [[ "$value" == \"* || "$value" == \'* ]]; then
      refuse_line "$relative" "$line_number" "quotes its value; write the value without quotes"
    fi
    if [[ "$loaded_keys" == *":$key:"* ]]; then
      refuse_line "$relative" "$line_number" "repeats a key already set in one of the files"
    fi
    loaded_keys="$loaded_keys$key:"
    if [[ -z "${!key+set}" ]]; then
      export "$key=$value"
    fi
  done <"$file"
}

[[ $# -gt 0 ]] || usage
refuse_printers "$1"
for relative in "${environment_files[@]}"; do
  load_file "$relative"
done
unset BASH_ENV ENV ZDOTDIR
export -n BASH_XTRACEFD
exec "$@"
