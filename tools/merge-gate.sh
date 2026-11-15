#!/usr/bin/env bash
set -euo pipefail

main_branch=main
lock_name=merge-gate.lock
log_name=merge-gate.log
run_lock_name=run.lock
run_lock_descriptor=7
run_lock_descriptor_variable=E2E_HARNESS_GATE_RUN_LOCK_FD
run_lock_held=no
run_lock_path=""
run_lock_candidate=""

full_checks=(
  "pre|.|tools/check-tool-binaries.sh"
  "C|.|pnpm --dir tools/docs install --frozen-lockfile"
  "C|.|tools/docs/check-docs.sh"
  "B|.|bin/gitleaks git --redact --no-banner --config .gitleaks.toml ."
  "B|.|bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml ."
  "C|.|bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod"
  "C|.|bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod"
  "C|.|bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml"
  "C|.|bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml"
  "B|backend|go mod tidy -diff"
  'B|backend|unformatted="$(gofmt -l .)" && test -z "$unformatted"'
  "B|backend|go vet ./..."
  "B|backend|../bin/golangci-lint run ./..."
  "B|backend|../bin/govulncheck ./..."
  "A|backend|../tools/dev-env.sh go test -race -count=1 ./..."
  "B|backend|../bin/sqlc -f sqlc.yaml diff"
  "C|frontend|pnpm install --frozen-lockfile"
  "C|frontend|pnpm typecheck"
  "C|frontend|pnpm lint"
  "C|frontend|pnpm format:check"
  "C|frontend|pnpm test"
  "C|frontend|pnpm build"
  "C|frontend|pnpm check:classes"
  "C|frontend|pnpm check:bundle"
  "C|frontend|pnpm api:check"
  "C|frontend|pnpm audit --audit-level high"
  "e2e|frontend|../tools/dev-env.sh pnpm e2e --max-failures=10"
  "C|.|python3 .claude/hooks/check_guard_secrets.py"
  "C|.|tools/check-dev-env.sh"
  "B|.|tools/check-merge-gate.sh"
)

lane_self_test_checks=(
  "pre|.|if test -f steps/pre.sh; then bash steps/pre.sh; fi"
  "A|.|if test -f steps/a1.sh; then bash steps/a1.sh; fi"
  "B|.|if test -f steps/b1.sh; then bash steps/b1.sh; fi"
  "C|.|if test -f steps/c1.sh; then bash steps/c1.sh; fi"
  "A|.|if test -f steps/a2.sh; then bash steps/a2.sh; fi"
  "B|.|if test -f steps/b2.sh; then bash steps/b2.sh; fi"
  "e2e|.|if test -f steps/e2e.sh; then bash steps/e2e.sh; fi"
  "C|.|if test -f steps/c2.sh; then bash steps/c2.sh; fi"
)

lane_slots="pre A B C e2e"
lane_start_timeout=60
lane_stop_grace=20
lane_kill_grace=10
lane_end_grace=30
log_retention_minutes=10080
log_tail_lines=40
command_record_pattern='^(pre|A|B|C|e2e)[|](0|[1-9][0-9]{0,3})[|](0|[1-9][0-9]{0,2})[|][0-9]{1,6}([.][0-9])?$'
done_record_pattern='^(pre|A|B|C|e2e)[|]done[|][0-9]{1,6}([.][0-9])?$'
recorded_lanes=""

docs_only_checks=(
  ".|tools/check-tool-binaries.sh"
  ".|pnpm --dir tools/docs install --frozen-lockfile"
  ".|tools/docs/check-docs.sh"
  ".|bin/gitleaks git --redact --no-banner --config .gitleaks.toml ."
  ".|bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml ."
)

self_test_checks=(
  ".|git rev-parse --verify HEAD"
  ".|if test -f gate-self-test.sh; then sh gate-self-test.sh; fi"
)

refused_environment_variables=(
  GOFLAGS
  GOWORK
  GOTOOLCHAIN
  GOPACKAGESDRIVER
  NODE_OPTIONS
  E2E_HARNESS_GATE_RUN_LOCK_FD
  BASH_ENV
  ENV
  SHELLOPTS
  BASHOPTS
  GIT_DIR
  GIT_WORK_TREE
  GIT_INDEX_FILE
  GIT_COMMON_DIR
  GIT_OBJECT_DIRECTORY
  GIT_ALTERNATE_OBJECT_DIRECTORIES
  GIT_NAMESPACE
  GIT_SHALLOW_FILE
  GIT_REPLACE_REF_BASE
  GIT_NO_REPLACE_OBJECTS
)

allowed_changed_go_settings=" GOPATH GOMODCACHE GOCACHE GOTMPDIR GOBIN GOTELEMETRY GOPROXY GOAMD64 GOARM64 "
tool_probes=yes
power_log=yes
go_setting_folders=(. backend tools)
reported_go_settings=(GOVERSION GOTOOLCHAIN GOFLAGS GOWORK GOENV)

influential_ignored_files=(
  docker-compose.override.yml
  docker-compose.override.yaml
  compose.override.yml
  compose.override.yaml
  go.work
  backend/go.work
  tools/go.work
  .npmrc
  frontend/.npmrc
  tools/docs/.npmrc
  .pnpmfile.cjs
  frontend/.pnpmfile.cjs
  tools/docs/.pnpmfile.cjs
)

usage() {
  printf '%s\n' \
    'usage: merge-gate.sh gate --ff-only [--change BRANCH] [--repo PATH] [--lock-dir PATH] [--checks full|self-test|self-test-lanes] [--lane-grace SECONDS] [--skip-tool-probes] [--skip-power-log]' \
    '       merge-gate.sh gate --squash --change BRANCH [--repo PATH] [--lock-dir PATH] [--checks full|self-test|self-test-lanes] [--lane-grace SECONDS] [--skip-tool-probes] [--skip-power-log]' \
    '       merge-gate.sh finish [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh release [--repo PATH] [--lock-dir PATH]' >&2
  exit 2
}

fail() {
  printf 'merge gate: %s\n' "$1" >&2
  exit 1
}

has_control_character() {
  [[ "$1" == *[[:cntrl:]]* ]]
}

account_home() {
  local user home
  user="$(id -un)"
  case "$(uname -s)" in
    Darwin) home="$(dscl . -read "/Users/$user" NFSHomeDirectory | sed -n 's/^NFSHomeDirectory: //p')" ;;
    Linux) home="$(getent passwd "$(id -u)" | cut -d: -f6)" ;;
    *) fail "the merge gate runs only on Linux and macOS" ;;
  esac
  [[ "$home" == /* ]] || fail "cannot find the home folder of $user in the account record"
  printf '%s' "$home"
}

default_lock_dir() {
  local home
  home="$(account_home)"
  case "$(uname -s)" in
    Darwin) printf '%s' "$home/Library/Caches/ovc-e2e" ;;
    *) printf '%s' "$home/.cache/ovc-e2e" ;;
  esac
}

canonical_folder() {
  local folder="$1" suffix=""
  while [[ ! -d "$folder" ]]; do
    suffix="/${folder##*/}$suffix"
    folder="${folder%/*}"
    [[ -n "$folder" ]] || folder=/
  done
  printf '%s%s' "$(cd -P "$folder" && pwd)" "$suffix"
}

script_common_dir() {
  local script_folder
  script_folder="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  git -C "$script_folder" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true
}

refuse_unsafe_self_test() {
  [[ "$requested_checks" == self-test || "$requested_checks" == self-test-lanes ]] || return 0
  [[ "$lock_dir_given" == yes ]] || fail "--checks self-test needs an explicit --lock-dir outside the default lock folder"
  [[ "$lock_dir" == /* ]] || fail "the lock folder $lock_dir is not an absolute path"
  [[ "$(canonical_folder "$lock_dir")" != "$(canonical_folder "$(default_lock_dir)")" ]] ||
    fail "--checks self-test refuses the default lock folder"
  [[ "$common_dir" != "$(script_common_dir)" ]] ||
    fail "--checks self-test refuses the repository the gate script belongs to"
  if in_repo cat-file -e "HEAD:tools/merge-gate.sh" 2>/dev/null; then
    fail "--checks self-test refuses a repository that holds tools/merge-gate.sh"
  fi
}

prepare_lock_dir() {
  [[ "$lock_dir" == /* ]] || fail "the lock folder $lock_dir is not an absolute path"
  has_control_character "$lock_dir" && fail "the lock folder path holds a control character"
  [[ -L "$lock_dir" ]] && fail "$lock_dir is a link, not a lock folder"
  case "$(canonical_folder "$lock_dir")/" in
    "$(canonical_folder "$root")"/*) fail "the lock folder $lock_dir lies inside the checkout $root" ;;
  esac
  local ancestor="$lock_dir"
  while [[ -n "$ancestor" ]]; do
    if [[ -e "$ancestor" && "$ancestor" -ef "$root" ]]; then
      fail "the lock folder $lock_dir lies inside the checkout $root"
    fi
    ancestor="${ancestor%/*}"
  done
  if [[ ! -e "$lock_dir" ]]; then
    (umask 077 && mkdir -p "$lock_dir")
  fi
  [[ -d "$lock_dir" && ! -L "$lock_dir" && -O "$lock_dir" ]] || fail "$lock_dir must be a folder owned by this user"
  chmod 700 "$lock_dir"
  lock_path="$lock_dir/$lock_name"
  log_path="$lock_dir/$log_name"
}

resolve_repo() {
  local start="${repo_arg:-$PWD}"
  root="$(git -C "$start" rev-parse --show-toplevel)" || fail "$start is not inside a git checkout"
  has_control_character "$root" && fail "the checkout path holds a control character"
  common_dir="$(git -C "$root" rev-parse --path-format=absolute --git-common-dir)"
}

in_repo() {
  git -C "$root" "$@"
}

clear_holder() {
  holder_pid=""
  holder_started=""
  holder_checkout=""
  holder_common=""
  holder_state=""
  holder_mode=""
  holder_change=""
  holder_base=""
  holder_head=""
  holder_tree=""
  holder_checks=""
  holder_scope=""
  holder_lanes=""
}

read_lock_file() {
  local file="$1"
  clear_holder
  [[ -L "$file" ]] && fail "$file is a link, not a merge gate lock; remove it by hand once no gate is active"
  [[ -e "$file" ]] || return 1
  [[ -f "$file" && -O "$file" ]] || fail "$file must be a regular file owned by this user to be a merge gate lock; remove it by hand once no gate is active"
  local key value
  while IFS='=' read -r key value; do
    case "$key" in
      pid) holder_pid="$value" ;;
      started_at) holder_started="$value" ;;
      checkout) holder_checkout="$value" ;;
      common_dir) holder_common="$value" ;;
      state) holder_state="$value" ;;
      mode) holder_mode="$value" ;;
      change) holder_change="$value" ;;
      base) holder_base="$value" ;;
      head) holder_head="$value" ;;
      tree) holder_tree="$value" ;;
      checks) holder_checks="$value" ;;
      scope) holder_scope="$value" ;;
      lanes) holder_lanes="$value" ;;
      *) fail "$file is not a merge gate lock; remove it by hand once no gate is active" ;;
    esac
  done <"$file"
  [[ "$holder_pid" =~ ^[1-9][0-9]*$ && -n "$holder_started" && "$holder_checkout" == /* && "$holder_common" == /* ]] ||
    fail "$file is not a merge gate lock; remove it by hand once no gate is active"
  [[ -z "$holder_lanes" || "$holder_lanes" =~ ^[1-9][0-9]*(,[1-9][0-9]*)*$ ]] ||
    fail "$file is not a merge gate lock; remove it by hand once no gate is active"
  case "$holder_state" in
    running | passed) ;;
    *) fail "$file is not a merge gate lock; remove it by hand once no gate is active" ;;
  esac
  return 0
}

read_lock() {
  read_lock_file "$lock_path"
}

write_lock_content() {
  printf 'pid=%s\nstarted_at=%s\ncheckout=%s\ncommon_dir=%s\nstate=%s\nmode=%s\nchange=%s\nbase=%s\nhead=%s\ntree=%s\nchecks=%s\nscope=%s\n' \
    "$$" "$started_at" "$root" "$common_dir" "$1" "$mode" "$change" "$base" "$head" "$tree" "$check_list" "$scope"
  if [[ -n "$recorded_lanes" ]]; then
    printf 'lanes=%s\n' "$recorded_lanes"
  fi
}

group_alive() {
  kill -0 -- "-$1" 2>/dev/null
}

holder_alive() {
  local group
  kill -0 "$holder_pid" 2>/dev/null && return 0
  for group in ${holder_lanes//,/ }; do
    group_alive "$group" && return 0
  done
  return 1
}

new_lock_file() {
  local temporary
  temporary="$(umask 077 && mktemp "$lock_dir/.merge-gate.XXXXXX")"
  write_lock_content "$1" >"$temporary"
  chmod 600 "$temporary"
  printf '%s' "$temporary"
}

holder_description() {
  printf 'a merge gate from %s (pid %s, lane process groups %s, started %s, %s) holds %s' \
    "$holder_checkout" "$holder_pid" "${holder_lanes:-none}" "$holder_started" "$holder_state" "$lock_path"
}

try_link_lock() {
  local temporary
  [[ -e "$lock_path" || -L "$lock_path" ]] && return 1
  temporary="$(new_lock_file running)"
  if ln "$temporary" "$lock_path" 2>/dev/null; then
    if [[ ! -L "$lock_path" && "$lock_path" -ef "$temporary" ]]; then
      rm -f "$temporary"
      return 0
    fi
    rm -f "$temporary"
    fail "$lock_path changed while the gate took it; remove it by hand once no gate is active"
  fi
  rm -f "$temporary"
  return 1
}

reclaim_stale_lock() {
  local stale_pid="$holder_pid" stale_started="$holder_started" stale_checkout="$holder_checkout" moved
  moved="$lock_dir/.merge-gate.stale.$$.$RANDOM$RANDOM"
  mv "$lock_path" "$moved" 2>/dev/null || fail "could not move the stale lock $lock_path aside"
  if read_lock_file "$moved" && [[ "$holder_state" == running && "$holder_pid" == "$stale_pid" && "$holder_started" == "$stale_started" ]] &&
    ! holder_alive; then
    printf 'merge gate: reclaimed %s left by pid %s from %s, which is no longer running and has no running lane\n' \
      "$lock_path" "$stale_pid" "$stale_checkout" >&2
    rm -f "$moved"
    return 0
  fi
  if ln "$moved" "$lock_path" 2>/dev/null; then
    rm -f "$moved"
    fail "$lock_path changed while the gate reclaimed it and was put back; start the gate again"
  fi
  fail "$lock_path changed while the gate reclaimed it; the moved lock is $moved, remove it by hand once no gate is active"
}

take_lock() {
  try_link_lock && return 0
  if read_lock && [[ "$holder_state" == running ]] && ! holder_alive; then
    reclaim_stale_lock
    try_link_lock && return 0
    read_lock || fail "could not take $lock_path"
  fi
  [[ -n "$holder_pid" ]] || fail "could not take $lock_path"
  fail "$(holder_description); wait for it to finish, or once it is abandoned release it with merge-gate.sh release"
}

lock_is_ours() {
  read_lock && [[ "$holder_pid" == "$$" && "$holder_started" == "$started_at" && "$holder_checkout" == "$root" ]]
}

release_if_running() {
  if [[ "$holding" == yes ]] && lock_is_ours && [[ "$holder_state" == running ]]; then
    rm -f "$lock_path"
  fi
}

rewrite_running_lock() {
  lock_is_ours || fail "$lock_path no longer holds this gate"
  local temporary
  temporary="$(new_lock_file running)"
  mv -f "$temporary" "$lock_path"
}

mark_passed() {
  lock_is_ours || fail "$lock_path no longer holds this gate"
  local temporary
  temporary="$(new_lock_file passed)"
  mv -f "$temporary" "$lock_path"
  holding=passed
}

prepare_log() {
  [[ -L "$log_path" ]] && fail "$log_path is a link, not the merge gate log"
  if [[ ! -e "$log_path" ]]; then
    (umask 077 && : >>"$log_path")
  fi
  [[ -f "$log_path" && -O "$log_path" ]] || fail "$log_path must be a regular file owned by this user"
  chmod 600 "$log_path"
}

append_log() {
  prepare_log
  printf 'time=%s base=%s tree=%s checks=%s scope=%s result=%s\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$base" "$tree" "$check_list" "$scope" "$1" >>"$log_path"
}

earlier_attempts() {
  local status=0
  grep -F " tree=$tree " "$log_path" || status=$?
  case "$status" in
    0 | 1) ;;
    *) fail "could not read $log_path" ;;
  esac
}

unstaged_or_untracked() {
  in_repo status --porcelain --untracked-files=all | awk 'substr($0, 2, 1) != " " { found = 1 } END { exit found ? 0 : 1 }'
}

tracked() {
  in_repo ls-files --error-unmatch -- "$1" >/dev/null 2>&1
}

refuse_influential_ignored_files() {
  local name
  for name in "${influential_ignored_files[@]}"; do
    if [[ -e "$root/$name" || -L "$root/$name" ]] && ! tracked "$name"; then
      fail "$name exists in the checkout outside the tested tree and would change what the checks run; move it away before the gate"
    fi
  done
}

refused_go_settings() {
  local setting refused=""
  while IFS= read -r setting; do
    [[ -n "$setting" ]] || continue
    [[ "$setting" =~ ^([A-Z][A-Z0-9_]*)= ]] || fail "go env -changed printed a line the gate cannot read"
    case "$allowed_changed_go_settings" in
      *" ${BASH_REMATCH[1]} "*) ;;
      *) refused="$refused${refused:+ }${BASH_REMATCH[1]}" ;;
    esac
  done <<<"$1"
  printf '%s' "$refused"
}

changed_go_setting_names() {
  local setting names=""
  while IFS= read -r setting; do
    [[ -n "$setting" ]] || continue
    names="$names${names:+ }${setting%%=*}"
  done <<<"$1"
  printf '%s' "${names:-none}"
}

go_environment_line() {
  local folder="$1" changed refused values name value line
  changed="$(go -C "$root/$folder" env -changed)" || fail "go env -changed failed in $folder"
  refused="$(refused_go_settings "$changed")" || exit 1
  [[ -z "$refused" ]] ||
    fail "go env -changed in $folder reports $refused, set in the environment or with go env -w, which would change what the checks run; run the gate with GOENV=off to ignore the user-level Go configuration, or unset the variables for the gate only with env -u"
  values="$(go -C "$root/$folder" env "${reported_go_settings[@]}")" || fail "go env failed in $folder"
  line="go environment in $folder:"
  for name in "${reported_go_settings[@]}"; do
    IFS= read -r value
    line="$line $name=$value"
  done <<<"$values"
  if [[ "${GOENV:-}" == off ]]; then
    line="$line (GOENV=off, so the user-level Go configuration is ignored)"
  fi
  printf '%s changed: %s' "$line" "$(changed_go_setting_names "$changed")"
}

exported_names() {
  python3 -I -c '
import os

for name in sorted(os.environ):
    print(name.replace("\n", " "))
'
}

exported() {
  case $'\n'"$1"$'\n' in
    *$'\n'"$2"$'\n'*) return 0 ;;
    *) return 1 ;;
  esac
}

refuse_shell_environment() {
  local functions names name refused="" prefixed="" unset_forms=""
  functions="$(declare -Fx)"
  functions="${functions//declare -fx /}"
  if [[ -n "$functions" ]]; then
    for name in $functions; do
      unset_forms="$unset_forms -u 'BASH_FUNC_$name%%'"
    done
    fail "the exported shell functions ${functions//$'\n'/ } would shadow commands in every check; unset them for the gate only, for example with env$unset_forms bash merge-gate.sh …, or start the gate from a shell that exports no function"
  fi
  names="$(exported_names)" || fail "could not list the names of the gate's environment"
  for name in "${refused_environment_variables[@]}"; do
    if exported "$names" "$name"; then
      refused="$refused${refused:+ }$name"
    fi
  done
  if exported "$names" GOENV && [[ "${GOENV:-}" != off ]]; then
    refused="$refused${refused:+ }GOENV"
  fi
  [[ -z "$refused" ]] ||
    fail "$refused set in the gate's environment would change what the checks run; unset it for the gate only, for example with env -u $refused, or start the gate from a clean shell"
  prefixed="$(printf '%s\n' "$names" | awk '
    tolower($0) ~ /^(npm|pnpm)_config_/ || $0 ~ /^GIT_CONFIG/ || $0 ~ /BASH_FUNC/ || $0 ~ /[%()]/ { printf "%s ", $0 }
  ')"
  [[ -z "$prefixed" ]] ||
    fail "${prefixed% } set in the gate's environment would change what the checks run; unset them for the gate only, for example with env -u, or start the gate from a clean shell"
}

pnpm_settings_problem() {
  python3 -I -c '
import json
import sys

try:
    settings = json.load(sys.stdin)
except ValueError:
    print("unreadable")
    sys.exit(0)
if not isinstance(settings, dict):
    print("unreadable")
    sys.exit(0)
problems = [name for name, key in (("node-options", "nodeOptions"), ("script-shell", "scriptShell")) if settings.get(key) not in (None, "")]
if settings.get("shellEmulator") not in (None, False, "false"):
    problems.append("shell-emulator")
print(" ".join(problems))
'
}

refuse_influential_tool_settings() {
  local folder line node_version pnpm_version pnpm_settings problem
  environment_lines=()
  if [[ "$tool_probes" == no ]]; then
    environment_lines+=("tool settings: not probed by the self-test check list")
    return 0
  fi
  for folder in "${go_setting_folders[@]}"; do
    [[ -d "$root/$folder" ]] || continue
    line="$(go_environment_line "$folder")" || exit 1
    environment_lines+=("$line")
  done
  node_version="$(node --version)" || fail "node --version failed"
  pnpm_version="$(pnpm --dir "$root" --version)" || fail "pnpm --version failed"
  pnpm_settings="$(pnpm --dir "$root" config list --json)" || fail "pnpm config list failed"
  problem="$(printf '%s' "$pnpm_settings" | pnpm_settings_problem)" || fail "could not read pnpm's settings"
  [[ -z "$problem" ]] ||
    fail "pnpm's effective settings $problem, from its user, global or project configuration, would change what the checks run; remove them before the gate"
  environment_lines+=("node environment: node $node_version, pnpm $pnpm_version, pnpm node-options, script-shell and shell-emulator: defaults")
}

process_alive() {
  ps -p "$1" >/dev/null 2>&1
}

run_lock_holder_of() {
  local file="$1"
  [[ -L "$file" ]] && fail "$file is a link, not an end-to-end run lock; remove it by hand once no run is active"
  [[ -e "$file" ]] || return 3
  [[ -f "$file" && -O "$file" ]] ||
    fail "$file must be a regular file owned by this user to be an end-to-end run lock; remove it by hand once no run is active"
  python3 -I -c '
import json
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as lock:
        holder = json.load(lock)
except (OSError, ValueError):
    sys.exit(1)
if not isinstance(holder, dict):
    sys.exit(1)
pid = holder.get("pid")
started = holder.get("startedAt")
checkout = holder.get("checkout")
kind = holder.get("kind", "end-to-end")
if "kind" in holder and kind != "merge-gate":
    sys.exit(1)
if type(pid) is not int or pid <= 0 or not isinstance(started, str) or not isinstance(checkout, str):
    sys.exit(1)
if not checkout.startswith("/") or "|" in started or any(ord(character) < 32 or ord(character) == 127 for character in started + checkout):
    sys.exit(1)
print(f"{pid}|{kind}|{started}|{checkout}")
' "$file" || fail "$file is not an end-to-end run lock; remove it by hand once no run is active"
}

run_lock_record() {
  python3 -I -c '
import json
import sys

print(json.dumps({"pid": int(sys.argv[1]), "startedAt": sys.argv[2], "checkout": sys.argv[3], "kind": "merge-gate"}))
' "$$" "$started_at" "$root"
}

run_lock_holder_description() {
  local kind="$2"
  case "$kind" in
    merge-gate) kind="another merge gate" ;;
    *) kind="an end-to-end run" ;;
  esac
  printf '%s from %s (pid %s, started %s)' "$kind" "$4" "$1" "$3"
}

fail_run_lock_busy() {
  attempt_logged=yes
  append_log e2e-run-lock-busy
  fail "the end-to-end run lock $run_lock_path is busy: $(run_lock_holder_description "$@") holds it; the gate ran no check; wait for it to end, then start the gate again"
}

reclaim_stale_run_lock() {
  local stale_pid="$1" stale_started="$2" moved line status=0 pid kind started checkout
  moved="$lock_dir/.run-lock.stale.$$.$RANDOM$RANDOM"
  if ! mv "$run_lock_path" "$moved" 2>/dev/null; then
    [[ ! -e "$run_lock_path" && ! -L "$run_lock_path" ]] || fail "could not move the stale end-to-end run lock $run_lock_path aside"
    return 0
  fi
  line="$(run_lock_holder_of "$moved")" || status=$?
  [[ "$status" == 0 ]] ||
    fail "$run_lock_path changed while the gate reclaimed it; the moved lock is $moved, remove it by hand once no run is active"
  IFS='|' read -r pid kind started checkout <<<"$line"
  if [[ "$pid" == "$stale_pid" && "$started" == "$stale_started" ]] && ! process_alive "$pid"; then
    rm -f "$moved"
    printf 'merge gate: reclaimed the stale end-to-end run lock %s of pid %s from %s, which is no longer running\n' \
      "$run_lock_path" "$pid" "$checkout" >&2
    return 0
  fi
  if ln "$moved" "$run_lock_path" 2>/dev/null; then
    rm -f "$moved"
    fail_run_lock_busy "$pid" "$kind" "$started" "$checkout"
  fi
  rm -f "$moved"
  fail "$run_lock_path changed hands while the gate reclaimed a stale end-to-end run lock; start the gate again"
}

link_run_lock() {
  local candidate="$1"
  [[ -e "$run_lock_path" || -L "$run_lock_path" ]] && return 1
  ln "$candidate" "$run_lock_path" 2>/dev/null || return 1
  [[ ! -L "$run_lock_path" && "$run_lock_path" -ef "$candidate" ]] ||
    fail "$run_lock_path changed while the gate took it; remove it by hand once no run is active"
}

take_run_lock() {
  local attempt=0 line status pid kind started checkout
  run_lock_path="$lock_dir/$run_lock_name"
  run_lock_candidate="$(umask 077 && mktemp "$lock_dir/.run-lock.XXXXXX")"
  run_lock_record >"$run_lock_candidate"
  chmod 600 "$run_lock_candidate"
  while [[ "$attempt" -lt 3 ]]; do
    attempt=$((attempt + 1))
    if link_run_lock "$run_lock_candidate"; then
      remove_run_lock_candidate
      run_lock_held=yes
      exec 7<"$run_lock_path"
      return 0
    fi
    status=0
    line="$(run_lock_holder_of "$run_lock_path")" || status=$?
    case "$status" in
      0) ;;
      3) continue ;;
      *) exit 1 ;;
    esac
    IFS='|' read -r pid kind started checkout <<<"$line"
    if process_alive "$pid"; then
      fail_run_lock_busy "$pid" "$kind" "$started" "$checkout"
    fi
    reclaim_stale_run_lock "$pid" "$started"
  done
  fail "$run_lock_path could not be taken; start the gate again"
}

remove_run_lock_candidate() {
  if [[ -n "$run_lock_candidate" ]]; then
    rm -f "$run_lock_candidate"
    run_lock_candidate=""
  fi
}

run_lock_is_ours() {
  local line status=0 pid kind started checkout
  line="$(run_lock_holder_of "$run_lock_path")" || status=$?
  [[ "$status" == 0 ]] || return 1
  IFS='|' read -r pid kind started checkout <<<"$line"
  [[ "$pid" == "$$" && "$kind" == merge-gate && "$started" == "$started_at" && "$checkout" == "$root" ]]
}

release_run_lock() {
  [[ "$run_lock_held" == yes ]] || return 0
  run_lock_held=no
  exec 7<&-
  if run_lock_is_ours; then
    rm -f "$run_lock_path"
    return 0
  fi
  printf 'merge gate: %s no longer holds this gate and was left alone\n' "$run_lock_path" >&2
  return 1
}

refuse_other_lock_dir_for_full_run() {
  [[ "$subcommand" == gate && "$requested_checks" == full && "$lock_dir_given" == yes ]] || return 0
  [[ "$lock_dir" == /* ]] || fail "the lock folder $lock_dir is not an absolute path"
  [[ "$(canonical_folder "$lock_dir")" == "$(canonical_folder "$(default_lock_dir)")" ]] ||
    fail "the full check list runs only with the default lock folder $(default_lock_dir), which holds the end-to-end run lock the harness takes"
}

record_ff_state() {
  local current
  current="$(in_repo symbolic-ref --quiet --short HEAD)" || fail "HEAD is detached; run the gate on the change's branch"
  [[ "$current" != "$main_branch" ]] || fail "run a fast-forward gate on the change's branch, not on $main_branch"
  [[ -z "$change_arg" || "$change_arg" == "$current" ]] || fail "the checkout is on $current, not on $change_arg"
  change="$current"
  [[ -z "$(in_repo status --porcelain --untracked-files=all)" ]] ||
    fail "the working tree has unstaged, staged or untracked changes; commit or remove them before the gate"
  refuse_influential_ignored_files
  base="$(in_repo rev-parse --verify "$main_branch^{commit}")"
  head="$(in_repo rev-parse --verify "HEAD^{commit}")"
  [[ "$head" != "$base" ]] || fail "$change holds no commit beyond $main_branch"
  in_repo merge-base --is-ancestor "$base" "$head" ||
    fail "$main_branch has moved past the base of $change; gate a squash merge instead"
  tree="$(in_repo rev-parse --verify "HEAD^{tree}")"
}

record_squash_state() {
  local current merged
  [[ -n "$change_arg" ]] || fail "a squash gate needs --change with the branch whose squash is staged"
  current="$(in_repo symbolic-ref --quiet --short HEAD)" || fail "HEAD is detached; run a squash gate on $main_branch"
  [[ "$current" == "$main_branch" ]] || fail "run a squash gate on $main_branch with the squashed change staged"
  unstaged_or_untracked && fail "the working tree has unstaged or untracked changes; the squashed change must be staged only"
  in_repo diff --cached --quiet && fail "nothing is staged; run git merge --squash with the change's branch first"
  refuse_influential_ignored_files
  change="$change_arg"
  base="$(in_repo rev-parse --verify "$main_branch^{commit}")"
  head="$(in_repo rev-parse --verify "refs/heads/$change^{commit}")" || fail "$change is not a branch"
  tree="$(in_repo write-tree)"
  merged="$(in_repo merge-tree --write-tree "$main_branch" "refs/heads/$change" | head -n 1)" ||
    fail "$change does not merge into $main_branch without conflicts"
  [[ "$tree" == "$merged" ]] ||
    fail "the staged tree $tree is not the merge of $change into $main_branch ($merged)"
}

record_state() {
  case "$mode" in
    ff-only) record_ff_state ;;
    squash) record_squash_state ;;
  esac
}

raw_changes() {
  case "$mode" in
    ff-only) in_repo diff --raw --no-renames --no-abbrev "$base...$head" ;;
    squash) in_repo diff --cached --raw --no-renames --no-abbrev ;;
  esac
}

read_by_test_or_script() {
  local status=0
  in_repo grep --cached -q -F -e "$1" -- . ':(exclude)*.md' || status=$?
  case "$status" in
    0) return 0 ;;
    1) return 1 ;;
    *) fail "git grep failed while looking for $1" ;;
  esac
}

plain_file_mode() {
  [[ "$1" == 100644 || "$1" == 000000 ]]
}

docs_only_change() {
  local changes meta path old_mode new_mode rest
  changes="$(raw_changes)"
  [[ -n "$changes" ]] || return 1
  while IFS=$'\t' read -r meta path; do
    read -r old_mode new_mode rest <<<"${meta#:}"
    plain_file_mode "$old_mode" && plain_file_mode "$new_mode" || return 1
    [[ "$old_mode" != 000000 || "$new_mode" != 000000 ]] || return 1
    case "$path" in
      knowledge/architecture/detailed.md) return 1 ;;
      docs/*.md | knowledge/*.md) ;;
      *) return 1 ;;
    esac
    read_by_test_or_script "${path##*/}" && return 1
  done <<<"$changes"
  return 0
}

laned_list() {
  [[ "$check_list" == full || "$check_list" == self-test-lanes ]]
}

now_seconds() {
  python3 -c 'import time; print("%.3f" % time.time())'
}

elapsed_seconds() {
  awk -v start="$1" -v end="$2" 'BEGIN { printf "%.1f", end - start }'
}

power_from_pmset_batt() {
  local first_line=""
  IFS= read -r first_line || true
  case "$first_line" in
    "Now drawing from 'AC Power'"*) printf 'mains' ;;
    "Now drawing from 'Battery Power'"*) printf 'battery' ;;
    *) printf 'unknown' ;;
  esac
}

power_from_online_values() {
  local value offline=no
  while IFS= read -r value; do
    case "$value" in
      1)
        printf 'mains'
        return 0
        ;;
      0) offline=yes ;;
    esac
  done
  if [[ "$offline" == yes ]]; then
    printf 'battery'
  else
    printf 'unknown'
  fi
}

load_from_fields() {
  local text one="" five="" fifteen="" rest="" value
  text="$(tr -d '{}')"
  read -r one five fifteen rest <<<"$text" || true
  for value in "$one" "$five" "$fifteen"; do
    if [[ ! "$value" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
      printf 'unknown'
      return 0
    fi
  done
  printf '%s %s %s' "$one" "$five" "$fifteen"
}

run_fields_from_power_log() {
  awk -v start="$1" -v end="$2" '
    $1 ~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/ && $2 ~ /^[0-9][0-9]:[0-9][0-9]:[0-9][0-9]$/ {
      stamp = $1 " " $2
      if (stamp < start || stamp > end) next
      if ($0 ~ /Using B(att|ATT)/) battery = 1
      if ($4 == "Sleep") slept = 1
    }
    END { printf "battery-during-run: %s; sleep: %s", battery ? "yes" : "no", slept ? "yes" : "no" }
  '
}

linux_online_values() {
  local file
  for file in /sys/class/power_supply/*/online; do
    [[ -f "$file" && -r "$file" ]] || continue
    head -n 1 "$file" 2>/dev/null || true
  done
}

power_source() {
  local text
  case "$(uname -s)" in
    Darwin)
      text="$(LC_ALL=C pmset -g batt 2>/dev/null)" || text=""
      power_from_pmset_batt <<<"$text"
      ;;
    Linux) linux_online_values | power_from_online_values ;;
    *) printf 'unknown' ;;
  esac
}

load_averages() {
  local text
  case "$(uname -s)" in
    Darwin) text="$(LC_ALL=C sysctl -n vm.loadavg 2>/dev/null)" || text="" ;;
    Linux) text="$(head -n 1 /proc/loadavg 2>/dev/null)" || text="" ;;
    *) text="" ;;
  esac
  load_from_fields <<<"$text"
}

run_power_fields() {
  local fields
  if [[ "$power_log" == yes && "$(uname -s)" == Darwin ]] &&
    fields="$(LC_ALL=C env -u TZ pmset -g log 2>/dev/null | run_fields_from_power_log "$1" "$2")"; then
    printf '%s' "$fields"
  else
    printf 'battery-during-run: unknown; sleep: unknown'
  fi
}

local_stamp() {
  env -u TZ date '+%Y-%m-%d %H:%M:%S'
}

record_gate_start() {
  power_log_started="$(local_stamp)"
  gate_start_line="gate start: power: $(power_source); load: $(load_averages)"
  printf 'merge gate: %s\n' "$gate_start_line" >&2
  prepare_log
  printf '%s\n' "$gate_start_line" >>"$log_path"
}

record_gate_end() {
  [[ -n "$gate_start_line" && -z "$gate_end_line" ]] || return 0
  gate_end_line="gate end: power: $(power_source); load: $(load_averages); $(run_power_fields "$power_log_started" "$(local_stamp)")"
  prepare_log
  printf '%s\n' "$gate_end_line" >>"$log_path"
}

split_checks() {
  local entry rest tag earlier index=0
  entry_tags=()
  entry_directories=()
  entry_commands=()
  for entry in "${checks[@]}"; do
    tag="${entry%%|*}"
    rest="${entry#*|}"
    case " $lane_slots " in
      *" $tag "*) ;;
      *) fail "entry $index of the $check_list check list names the unknown slot $tag" ;;
    esac
    [[ "$rest" == *"|"* ]] || fail "entry $index of the $check_list check list names no folder"
    earlier=0
    while [[ "$earlier" -lt "$index" ]]; do
      [[ "${entry_directories[earlier]}|${entry_commands[earlier]}" != "$rest" ]] ||
        fail "entry $index of the $check_list check list repeats entry $earlier"
      earlier=$((earlier + 1))
    done
    entry_tags[index]="$tag"
    entry_directories[index]="${rest%%|*}"
    entry_commands[index]="${rest#*|}"
    index=$((index + 1))
  done
  entry_count="$index"
}

prepare_log_folder() {
  find "$lock_dir" -maxdepth 1 -type d -name 'logs.*' -mmin "+$log_retention_minutes" -exec rm -rf {} + ||
    fail "could not remove the command logs older than seven days from $lock_dir"
  log_folder="$(umask 077 && mktemp -d "$lock_dir/logs.XXXXXX")"
  chmod 700 "$log_folder"
}

keep_end_to_end_evidence() {
  local run="$1" kept="$2" test_folder name evidence
  while IFS= read -r test_folder; do
    name="$(basename "$run")/$(basename "$test_folder")"
    printf '%s\n' "$name" >>"$kept/failed-tests.txt" || add_problem "could not list $name in $kept"
    evidence="$test_folder/browser-diagnostics.json"
    if [[ -f "$evidence" && ! -L "$evidence" ]]; then
      if [[ -n "$(find "$evidence" -links 1)" ]]; then
        if ! { mkdir -p "$kept/$name" && cat "$evidence" >"$kept/$name/browser-diagnostics.json"; }; then
          add_problem "could not keep the browser diagnostics of $name in $kept"
        fi
      else
        add_problem "did not keep the browser diagnostics of $name, which has more than one hard link"
      fi
    fi
  done < <(find "$run" -mindepth 1 -maxdepth 1 -type d ! -name attachments)
}

keep_end_to_end_results() {
  local marker="$1" results="$root/frontend/test-results" outputs="$root/frontend/test-results/playwright" run
  local kept="$log_folder/e2e-results" previous_umask
  if [[ -n "$laned_failure" && -d "$outputs" && ! -L "$results" && ! -L "$outputs" ]]; then
    previous_umask="$(umask)"
    umask 077
    while IFS= read -r run; do
      if mkdir -p "$kept" && : >>"$kept/failed-tests.txt"; then
        keep_end_to_end_evidence "$run" "$kept"
        kept_results="$kept"
      else
        add_problem "could not create $kept for the failed end-to-end evidence"
      fi
    done < <(find "$outputs" -mindepth 1 -maxdepth 1 -type d -name 'run-*' -newer "$marker")
    umask "$previous_umask"
  fi
  rm -f "$marker"
}

run_entry() {
  cd "$root/${entry_directories[$1]}"
  if [[ "${entry_tags[$1]}" == e2e ]]; then
    export "$run_lock_descriptor_variable=$run_lock_descriptor"
  fi
  exec bash -c "${entry_commands[$1]}"
}

run_lane() {
  local slot="$1" token="" index=0 status started finished log
  trap - EXIT INT TERM
  if [[ "$slot" != e2e ]]; then
    exec 7<&-
  fi
  read -r -n 1 -t "$lane_start_timeout" -u 8 token || exit 1
  exec 8<&-
  [[ "$token" == g ]] || exit 1
  started="$(now_seconds 9>&-)"
  while [[ "$index" -lt "$entry_count" ]]; do
    if [[ "${entry_tags[index]}" == "$slot" ]]; then
      log="$log_folder/$index.log"
      (umask 077 && : >"$log") 9>&-
      status=0
      (run_entry "$index") </dev/null >>"$log" 2>&1 9>&- || status=$?
      finished="$(now_seconds 9>&-)"
      printf '%s|%s|%s|%s\n' "$slot" "$index" "$status" "$(elapsed_seconds "$started" "$finished" 9>&-)" >&9
      started="$finished"
      [[ "$status" == 0 ]] || break
    fi
    index=$((index + 1))
  done
  finished="$(now_seconds 9>&-)"
  printf '%s|done|%s\n' "$slot" "$(elapsed_seconds "$phase_started" "$finished" 9>&-)" >&9
}

lane_groups_list() {
  local pair groups=""
  for pair in $active_lanes; do
    groups="$groups${groups:+,}${pair#*:}"
  done
  printf '%s' "$groups"
}

lanes_with_live_groups() {
  local pair live=""
  for pair in $active_lanes; do
    if group_alive "${pair#*:}"; then
      live="$live${live:+ }$pair"
    fi
  done
  printf '%s' "$live"
}

wait_for_lane_groups() {
  local deadline=$((SECONDS + $1))
  while [[ -n "$(lanes_with_live_groups)" ]]; do
    [[ "$SECONDS" -lt "$deadline" ]] || return 1
    sleep 0.1
  done
}

open_fifo() {
  local fifo="$log_folder/$2.fifo"
  mkfifo -m 600 "$fifo"
  case "$1" in
    8) exec 8<>"$fifo" ;;
    9) exec 9<>"$fifo" ;;
  esac
  rm -f "$fifo"
}

start_lanes() {
  local slot
  open_fifo 8 start
  open_fifo 9 records
  phase_started="$(now_seconds 9>&-)"
  set -m
  for slot in "$@"; do
    run_lane "$slot" &
    active_lanes="$active_lanes${active_lanes:+ }$slot:$!"
  done
  set +m
  recorded_lanes="$(lane_groups_list)"
  rewrite_running_lock
  for slot in "$@"; do
    printf 'g' >&8
  done
  exec 8<&-
}

add_problem() {
  problems="$problems$1"$'\n'
}

wait_lanes() {
  local pair status
  for pair in $active_lanes; do
    status=0
    wait "${pair#*:}" || status=$?
    [[ "$status" == 0 ]] || add_problem "lane ${pair%%:*} ended with status $status"
  done
}

collect_lane_records() {
  local line
  phase_records=""
  printf 'end\n' >&9
  while :; do
    if ! read -r -t 10 -u 9 line; then
      add_problem "the lane record pipe ended without its end marker"
      break
    fi
    [[ "$line" != end ]] || break
    phase_records="$phase_records$line"$'\n'
  done
  exec 9<&-
  record_lines="$record_lines$phase_records"
}

end_lanes() {
  local pair
  if [[ -n "$(lanes_with_live_groups)" ]]; then
    printf 'merge gate: waiting up to %s seconds for the remaining processes of lane:process-group %s to end\n' \
      "$lane_end_grace" "$(lanes_with_live_groups)" >&2
  fi
  if ! wait_for_lane_groups "$lane_end_grace"; then
    for pair in $(lanes_with_live_groups); do
      add_problem "processes of lane ${pair%%:*} (process group ${pair#*:}) were still running $lane_end_grace seconds after it ended"
    done
    return 0
  fi
  active_lanes=""
  recorded_lanes=""
  rewrite_running_lock
}

signal_lane_group() {
  kill -"$1" -- "-$2" 2>/dev/null || ! group_alive "$2" ||
    printf 'merge gate: could not send %s to lane process group %s\n' "$1" "$2" >&2
}

stop_lanes() {
  local pair status
  [[ -n "$active_lanes" ]] || return 0
  for pair in $active_lanes; do
    signal_lane_group TERM "${pair#*:}"
  done
  for pair in $active_lanes; do
    status=0
    wait "${pair#*:}" 2>/dev/null || status=$?
  done
  if [[ -n "$(lanes_with_live_groups)" ]]; then
    printf 'merge gate: waiting up to %s seconds for the processes of lane:process-group %s to end\n' \
      "$lane_stop_grace" "$(lanes_with_live_groups)" >&2
  fi
  if ! wait_for_lane_groups "$lane_stop_grace"; then
    for pair in $(lanes_with_live_groups); do
      signal_lane_group KILL "${pair#*:}"
    done
    if ! wait_for_lane_groups "$lane_kill_grace"; then
      printf 'merge gate: lane process groups %s still run after TERM and KILL; the lock %s stays held\n' \
        "$(lanes_with_live_groups)" "$lock_path" >&2
      return 1
    fi
  fi
  active_lanes=""
}

on_exit() {
  local exit_status=$? stopped=yes
  remove_run_lock_candidate
  stop_lanes || stopped=no
  if [[ "$holding" == yes && "$attempt_logged" == no ]]; then
    attempt_logged=yes
    printf 'merge gate: stopped with status %s before a result; command logs: %s\n' "$exit_status" "${log_folder:-none}" >&2
    if [[ "$exit_status" == 130 ]]; then
      (append_log interrupted) || printf 'merge gate: the interrupted attempt is missing from %s\n' "$log_path" >&2
    else
      (append_log stopped) || printf 'merge gate: the stopped attempt is missing from %s\n' "$log_path" >&2
    fi
  fi
  if [[ "$stopped" == yes ]]; then
    release_if_running
    release_run_lock || true
  fi
}

records_of() {
  local line
  while IFS= read -r line; do
    if [[ "$line" =~ $command_record_pattern && "$line" == "$1|$2|"* ]]; then
      printf '%s\n' "$line"
    fi
  done <<<"$record_lines"
}

done_records_of() {
  local line
  while IFS= read -r line; do
    if [[ "$line" =~ $done_record_pattern && "$line" == "$1|done|"* ]]; then
      printf '%s\n' "$line"
    fi
  done <<<"$record_lines"
}

line_count() {
  if [[ -z "$1" ]]; then
    printf '0'
  else
    printf '%s\n' "$1" | awk 'END { printf "%d", NR }'
  fi
}

record_field() {
  printf '%s' "$1" | cut -d'|' -f"$2"
}

check_lane_records() {
  local phase_slots=" $* " slot line line_number index lines count failed_before
  line_number=0
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    line_number=$((line_number + 1))
    slot="${line%%|*}"
    if [[ "$line" =~ $command_record_pattern ]]; then
      index="$(record_field "$line" 2)"
      if [[ "$phase_slots" != *" $slot "* || "$index" -ge "$entry_count" || "${entry_tags[index]}" != "$slot" ]]; then
        add_problem "lane $slot reported a record for entry $index, which is not in lane $slot"
      fi
    elif [[ "$line" =~ $done_record_pattern ]]; then
      [[ "$phase_slots" == *" $slot "* ]] || add_problem "lane $slot reported its end outside its phase"
    else
      add_problem "line $line_number of the lane records of $* is malformed"
    fi
  done <<<"$phase_records"
  for slot in "$@"; do
    count="$(line_count "$(done_records_of "$slot")")"
    [[ "$count" == 1 ]] || add_problem "lane $slot reported its end $count times"
    failed_before=no
    index=0
    while [[ "$index" -lt "$entry_count" ]]; do
      if [[ "${entry_tags[index]}" == "$slot" ]]; then
        lines="$(records_of "$slot" "$index")"
        count="$(line_count "$lines")"
        if [[ "$count" == 0 ]]; then
          [[ "$failed_before" == yes ]] || add_problem "entry $index of lane $slot has no record"
          failed_before=yes
        elif [[ "$count" != 1 ]]; then
          add_problem "entry $index of lane $slot has $count records"
          failed_before=yes
        elif [[ "$(record_field "$lines" 3)" != 0 ]]; then
          failed_check=yes
          failed_before=yes
        fi
      fi
      index=$((index + 1))
    done
  done
}

run_phase() {
  printf 'merge gate: running %s, command logs in %s\n' "$*" "$log_folder" >&2
  start_lanes "$@"
  wait_lanes
  collect_lane_records
  end_lanes
  phase_times="$phase_times$*|$(elapsed_seconds "$phase_started" "$(now_seconds)")"$'\n'
  check_lane_records "$@"
  if [[ "$failed_check" == yes ]]; then
    laned_failure="a check failed"
  elif [[ -n "$problems" ]]; then
    laned_failure="${problems%%$'\n'*}"
  fi
}

run_laned_checks() {
  local changed marker
  prepare_log_folder
  run_phase pre
  if [[ -n "$laned_failure" ]]; then
    return 0
  fi
  run_phase A B C
  if [[ -n "$laned_failure" ]]; then
    return 0
  fi
  if changed="$(state_change_during_gate)"; then
    laned_failure="the checkout changed during the gate: $changed"
    return 0
  fi
  if ! run_lock_is_ours; then
    laned_failure="the end-to-end run lock $run_lock_path no longer holds this gate"
    return 0
  fi
  marker="$(umask 077 && mktemp "$log_folder/e2e-started.XXXXXX")"
  run_phase e2e
  keep_end_to_end_results "$marker"
}

print_laned_results() {
  local index tag lines count exit_status seconds slot log phase timings=""
  index=0
  while [[ "$index" -lt "$entry_count" ]]; do
    tag="${entry_tags[index]}"
    lines="$(records_of "$tag" "$index")"
    count="$(line_count "$lines")"
    case "$count" in
      0)
        if [[ -n "$log_folder" && -f "$log_folder/$index.log" ]]; then
          printf 'no record: %s: %s: %s\n' "$tag" "${entry_directories[index]}" "${entry_commands[index]}"
        else
          printf 'not run: %s: %s: %s\n' "$tag" "${entry_directories[index]}" "${entry_commands[index]}"
        fi
        ;;
      1)
        exit_status="$(record_field "$lines" 3)"
        seconds="$(record_field "$lines" 4)"
        timings="$timings$tag $seconds"$'\n'
        printf 'exit %s: %s: %s s: %s: %s\n' "$exit_status" "$tag" "$seconds" "${entry_directories[index]}" "${entry_commands[index]}"
        ;;
      *) printf '%s records: %s: %s: %s\n' "$count" "$tag" "${entry_directories[index]}" "${entry_commands[index]}" ;;
    esac
    index=$((index + 1))
  done
  for slot in $lane_slots; do
    lines="$(done_records_of "$slot")"
    if [[ "$(line_count "$lines")" == 1 ]]; then
      printf 'lane %s wall time: %s s\n' "$slot" "$(record_field "$lines" 3)"
    else
      printf 'lane %s wall time: not run\n' "$slot"
    fi
    printf 'lane %s command seconds: %s\n' "$slot" \
      "$(printf '%s' "$timings" | awk -v slot="$slot" '$1 == slot { total += $2 } END { printf "%.1f", total }')"
  done
  while IFS= read -r phase; do
    [[ -n "$phase" ]] || continue
    printf 'phase %s wall time with the wait for its processes: %s s\n' "${phase%%|*}" "${phase#*|}"
  done <<<"$phase_times"
  printf 'summed command seconds: %s\n' "$(printf '%s' "$timings" | awk '{ total += $2 } END { printf "%.1f", total }')"
  printf 'gate wall time: %s s\n' "$(elapsed_seconds "$gate_started" "$(now_seconds)")"
  printf 'cores: %s\n' "$(getconf _NPROCESSORS_ONLN)"
  printf 'command logs: %s\n' "${log_folder:-none}"
  if [[ -n "$kept_results" ]]; then
    printf 'kept end-to-end evidence of the failed run: %s\n' "$kept_results"
  fi
  if [[ -n "$problems" ]]; then
    printf 'lane problems:\n%s' "$problems"
  fi
  index=0
  while [[ "$index" -lt "$entry_count" ]]; do
    log="$log_folder/$index.log"
    if [[ -n "$log_folder" && -f "$log" ]]; then
      lines="$(records_of "${entry_tags[index]}" "$index")"
      if [[ "$(line_count "$lines")" != 1 || "$(record_field "$lines" 3)" != 0 ]]; then
        printf 'last %s lines of the log of %s: %s:\n' "$log_tail_lines" "${entry_tags[index]}" "${entry_commands[index]}"
        tail -n "$log_tail_lines" "$log"
      fi
    fi
    index=$((index + 1))
  done
}

run_checks() {
  local entry directory command status
  local failed=no
  for entry in "${checks[@]}"; do
    directory="${entry%%|*}"
    command="${entry#*|}"
    if [[ "$failed" == yes ]]; then
      results+=("not run: $directory: $command")
      continue
    fi
    printf 'merge gate: running in %s: %s\n' "$directory" "$command" >&2
    status=0
    (cd "$root/$directory" && bash -c "$command") >&2 || status=$?
    results+=("exit $status: $directory: $command")
    [[ "$status" == 0 ]] || failed=yes
  done
  [[ "$failed" == no ]]
}

state_change_during_gate() {
  local recorded="$base|$head|$tree|$change" after status=0
  after="$( { record_state && printf '%s|%s|%s|%s' "$base" "$head" "$tree" "$change"; } 2>&1)" || status=$?
  if [[ "$status" != 0 ]]; then
    printf '%s' "${after#merge gate: }"
    return 0
  fi
  if [[ "$after" != "$recorded" ]]; then
    printf '%s, the head or the tested tree changed' "$main_branch"
    return 0
  fi
  return 1
}

print_report() {
  local line
  printf 'merge gate report\n'
  printf 'mode: %s\n' "$mode"
  printf 'change: %s\n' "$change"
  printf 'change head: %s\n' "$head"
  printf 'base %s: %s\n' "$main_branch" "$base"
  printf 'tested tree: %s\n' "$tree"
  printf 'scope: %s\n' "$scope"
  printf 'check list: %s\n' "$check_list"
  for line in "$gate_start_line" "$gate_end_line"; do
    [[ -z "$line" ]] || printf '%s\n' "$line"
  done
  for line in ${environment_lines[@]+"${environment_lines[@]}"}; do
    printf '%s\n' "$line"
  done
  if [[ -n "$run_lock_path" ]]; then
    printf 'end-to-end run lock: %s, taken by this gate (pid %s) before its first check and released after its last\n' "$run_lock_path" "$$"
  else
    printf 'end-to-end run lock: not taken by the %s check list\n' "$check_list"
  fi
  if [[ -n "$attempts" ]]; then
    printf 'earlier attempts for this tree:\n%s\n' "$attempts"
  else
    printf 'earlier attempts for this tree: none\n'
  fi
  if laned_list; then
    print_laned_results
  else
    for line in "${results[@]}"; do
      printf '%s\n' "$line"
    done
  fi
  printf 'result: %s\n' "$1"
}

gate_failed() {
  record_gate_end
  print_report "failed: $1"
  attempt_logged=yes
  append_log failed
  exit 1
}

gate() {
  local changed
  holding=no
  started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  change=""
  base=""
  head=""
  tree=""
  scope=""
  check_list=""
  results=()
  attempts=""
  active_lanes=""
  recorded_lanes=""
  log_folder=""
  kept_results=""
  problems=""
  failed_check=no
  laned_failure=""
  entry_count=0
  gate_started=""
  record_lines=""
  phase_records=""
  phase_times=""
  phase_started=""
  attempt_logged=no
  power_log_started=""
  gate_start_line=""
  gate_end_line=""
  environment_lines=()
  run_lock_held=no
  run_lock_path=""
  run_lock_candidate=""
  trap on_exit EXIT
  trap 'exit 130' INT TERM
  refuse_influential_tool_settings
  record_state
  if docs_only_change; then
    scope=docs-only
  else
    scope=full
  fi
  case "$requested_checks" in
    self-test | self-test-lanes) check_list="$requested_checks" ;;
    full) check_list="$scope" ;;
  esac
  case "$check_list" in
    full) checks=("${full_checks[@]}") ;;
    docs-only) checks=("${docs_only_checks[@]}") ;;
    self-test) checks=("${self_test_checks[@]}") ;;
    self-test-lanes) checks=("${lane_self_test_checks[@]}") ;;
  esac
  if laned_list; then
    split_checks
    gate_started="$(now_seconds)"
  fi
  take_lock
  holding=yes
  prepare_log
  attempts="$(earlier_attempts)"
  record_gate_start
  if laned_list; then
    take_run_lock
    run_laned_checks
    [[ -z "$laned_failure" ]] || gate_failed "$laned_failure"
  else
    run_checks || gate_failed "a check failed"
  fi
  if changed="$(state_change_during_gate)"; then
    gate_failed "the checkout changed during the gate: $changed"
  fi
  release_run_lock || gate_failed "the end-to-end run lock $run_lock_path no longer held this gate at its end"
  record_gate_end
  attempt_logged=yes
  append_log passed
  mark_passed
  print_report passed
  case "$mode" in
    ff-only) printf 'next: in the checkout of %s run git merge --ff-only %s, then merge-gate.sh finish\n' "$main_branch" "$change" ;;
    squash) printf 'next: commit the staged squash of %s without changing the index, then merge-gate.sh finish\n' "$change" ;;
  esac
}

finish() {
  read_lock || fail "no merge gate holds $lock_path"
  [[ "$holder_state" == passed ]] || fail "$(holder_description); finish only a gate that passed"
  [[ "$holder_common" == "$common_dir" ]] || fail "$(holder_description) for another repository"
  local main_commit main_tree hash_problem="" checks_problem=""
  main_commit="$(in_repo rev-parse --verify "$main_branch^{commit}")"
  main_tree="$(in_repo rev-parse --verify "$main_branch^{tree}")"
  case "$holder_mode" in
    ff-only)
      [[ "$main_commit" == "$holder_head" ]] ||
        hash_problem="$main_branch is $main_commit, not the gated head $holder_head"
      ;;
    squash)
      if [[ "$(in_repo rev-parse --verify "$main_branch^1" 2>/dev/null || true)" != "$holder_base" ]]; then
        hash_problem="the parent of $main_branch is not the recorded base $holder_base"
      elif in_repo rev-parse --quiet --verify "$main_branch^2" >/dev/null; then
        hash_problem="$main_branch is a merge commit, not the squash commit"
      fi
      ;;
    *) fail "$lock_path records an unknown mode" ;;
  esac
  if [[ -z "$hash_problem" && "$main_tree" != "$holder_tree" ]]; then
    hash_problem="the tree of $main_branch $main_tree differs from the tested tree $holder_tree"
  fi
  case "$holder_checks|$holder_scope" in
    full\|full | full\|docs-only | docs-only\|docs-only) ;;
    *) checks_problem="the gate ran the $holder_checks check list for a $holder_scope scope, not the full run" ;;
  esac
  printf 'merge gate finish\n'
  printf 'mode: %s\n' "$holder_mode"
  printf 'change: %s\n' "$holder_change"
  printf 'base %s: %s\n' "$main_branch" "$holder_base"
  printf '%s after the merge: %s\n' "$main_branch" "$main_commit"
  printf 'tested tree: %s\n' "$holder_tree"
  printf '%s tree: %s\n' "$main_branch" "$main_tree"
  printf 'scope: %s\n' "$holder_scope"
  printf 'check list: %s\n' "$holder_checks"
  if [[ -n "$hash_problem" ]]; then
    printf 'hash check: failed: %s\n' "$hash_problem"
  else
    printf 'hash check: passed\n'
  fi
  if [[ -n "$checks_problem" ]]; then
    printf 'check list check: failed: %s\n' "$checks_problem"
  else
    printf 'check list check: passed\n'
  fi
  if [[ -n "$hash_problem" || -n "$checks_problem" ]]; then
    printf 'result: failed\n'
    printf 'the lock stays held; report this to the owner, then release it with merge-gate.sh release\n'
    exit 1
  fi
  rm -f "$lock_path"
  printf 'result: passed; the trees are equal and the lock is released\n'
}

release() {
  if ! read_lock; then
    printf 'merge gate: no gate holds %s\n' "$lock_path"
    return 0
  fi
  [[ "$holder_common" == "$common_dir" ]] || fail "$(holder_description) for another repository"
  if [[ "$holder_state" == running ]] && holder_alive; then
    fail "$(holder_description) and it or one of its lanes is still running"
  fi
  rm -f "$lock_path"
  printf 'merge gate: released %s, held by pid %s from %s (%s)\n' "$lock_path" "$holder_pid" "$holder_checkout" "$holder_state"
}

main() {
  refuse_shell_environment
  [[ $# -ge 1 ]] || usage
  subcommand="$1"
  shift
  mode=""
  repo_arg=""
  lock_dir=""
  lock_dir_given=no
  change_arg=""
  requested_checks=full
  lane_grace=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --ff-only) mode=ff-only ;;
      --squash) mode=squash ;;
      --repo)
        [[ $# -ge 2 ]] || usage
        repo_arg="$2"
        shift
        ;;
      --lock-dir)
        [[ $# -ge 2 ]] || usage
        lock_dir="$2"
        lock_dir_given=yes
        shift
        ;;
      --change)
        [[ $# -ge 2 && -n "$2" ]] || usage
        change_arg="$2"
        shift
        ;;
      --checks)
        [[ $# -ge 2 ]] || usage
        case "$2" in
          full | self-test | self-test-lanes) requested_checks="$2" ;;
          *) usage ;;
        esac
        shift
        ;;
      --skip-tool-probes) tool_probes=no ;;
      --skip-power-log) power_log=no ;;
      --lane-grace)
        [[ $# -ge 2 && "$2" =~ ^[1-9][0-9]?$ ]] || usage
        lane_grace="$2"
        shift
        ;;
      *) usage ;;
    esac
    shift
  done

  resolve_repo
  [[ -n "$lock_dir" ]] || lock_dir="$(default_lock_dir)"
  refuse_unsafe_self_test
  if [[ "$tool_probes" == no ]]; then
    [[ "$subcommand" == gate && ( "$requested_checks" == self-test || "$requested_checks" == self-test-lanes ) ]] ||
      fail "--skip-tool-probes is accepted only by a gate with --checks self-test or self-test-lanes"
  fi
  if [[ "$power_log" == no ]]; then
    [[ "$subcommand" == gate && ( "$requested_checks" == self-test || "$requested_checks" == self-test-lanes ) ]] ||
      fail "--skip-power-log is accepted only by a gate with --checks self-test or self-test-lanes"
  fi
  if [[ -n "$lane_grace" ]]; then
    [[ "$subcommand" == gate && ( "$requested_checks" == self-test || "$requested_checks" == self-test-lanes ) ]] ||
      fail "--lane-grace is accepted only by a gate with --checks self-test or self-test-lanes"
    lane_end_grace="$lane_grace"
    lane_stop_grace="$lane_grace"
    lane_kill_grace="$lane_grace"
  fi
  refuse_other_lock_dir_for_full_run
  prepare_lock_dir

  case "$subcommand" in
    gate)
      [[ -n "$mode" ]] || usage
      gate
      ;;
    finish)
      [[ -z "$mode" && -z "$change_arg" && "$requested_checks" == full ]] || usage
      finish
      ;;
    release)
      [[ -z "$mode" && -z "$change_arg" && "$requested_checks" == full ]] || usage
      release
      ;;
    *) usage ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
