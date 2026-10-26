#!/usr/bin/env bash
set -euo pipefail

main_branch=main
lock_name=merge-gate.lock
log_name=merge-gate.log

full_checks=(
  ".|tools/check-tool-binaries.sh"
  ".|pnpm --dir tools/docs install --frozen-lockfile"
  ".|tools/docs/check-docs.sh"
  ".|bin/gitleaks git --redact --no-banner --config .gitleaks.toml ."
  ".|bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml ."
  ".|bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod"
  ".|bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod"
  ".|bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml"
  ".|bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml"
  "backend|go mod tidy -diff"
  'backend|unformatted="$(gofmt -l .)" && test -z "$unformatted"'
  "backend|go vet ./..."
  "backend|../bin/golangci-lint run ./..."
  "backend|../bin/govulncheck ./..."
  "backend|../tools/dev-env.sh go test -race -count=1 ./..."
  "backend|../bin/sqlc -f sqlc.yaml diff"
  "frontend|pnpm install --frozen-lockfile"
  "frontend|pnpm typecheck"
  "frontend|pnpm lint"
  "frontend|pnpm format:check"
  "frontend|pnpm test"
  "frontend|pnpm build"
  "frontend|pnpm check:classes"
  "frontend|pnpm check:bundle"
  "frontend|pnpm api:check"
  "frontend|pnpm audit --audit-level high"
  "frontend|../tools/dev-env.sh pnpm e2e"
  ".|python3 .claude/hooks/check_guard_secrets.py"
  ".|tools/check-dev-env.sh"
  ".|tools/check-merge-gate.sh"
)

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
    'usage: merge-gate.sh gate --ff-only [--change BRANCH] [--repo PATH] [--lock-dir PATH] [--checks full|self-test]' \
    '       merge-gate.sh gate --squash --change BRANCH [--repo PATH] [--lock-dir PATH] [--checks full|self-test]' \
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
  [[ "$requested_checks" == self-test ]] || return 0
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
      *) fail "$file is not a merge gate lock; remove it by hand once no gate is active" ;;
    esac
  done <"$file"
  [[ "$holder_pid" =~ ^[1-9][0-9]*$ && -n "$holder_started" && "$holder_checkout" == /* && "$holder_common" == /* ]] ||
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
}

new_lock_file() {
  local temporary
  temporary="$(umask 077 && mktemp "$lock_dir/.merge-gate.XXXXXX")"
  write_lock_content "$1" >"$temporary"
  chmod 600 "$temporary"
  printf '%s' "$temporary"
}

holder_description() {
  printf 'a merge gate from %s (pid %s, started %s, %s) holds %s' \
    "$holder_checkout" "$holder_pid" "$holder_started" "$holder_state" "$lock_path"
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
    ! kill -0 "$holder_pid" 2>/dev/null; then
    printf 'merge gate: reclaimed %s left by pid %s from %s, which is no longer running\n' \
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
  if read_lock && [[ "$holder_state" == running ]] && ! kill -0 "$holder_pid" 2>/dev/null; then
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
  if [[ -n "$attempts" ]]; then
    printf 'earlier attempts for this tree:\n%s\n' "$attempts"
  else
    printf 'earlier attempts for this tree: none\n'
  fi
  for line in "${results[@]}"; do
    printf '%s\n' "$line"
  done
  printf 'result: %s\n' "$1"
}

gate_failed() {
  print_report "failed: $1"
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
  trap release_if_running EXIT
  trap 'exit 130' INT TERM
  record_state
  if docs_only_change; then
    scope=docs-only
  else
    scope=full
  fi
  case "$requested_checks" in
    self-test) check_list=self-test ;;
    full) check_list="$scope" ;;
  esac
  case "$check_list" in
    full) checks=("${full_checks[@]}") ;;
    docs-only) checks=("${docs_only_checks[@]}") ;;
    self-test) checks=("${self_test_checks[@]}") ;;
  esac
  take_lock
  holding=yes
  prepare_log
  attempts="$(earlier_attempts)"
  run_checks || gate_failed "a check failed"
  if changed="$(state_change_during_gate)"; then
    gate_failed "the checkout changed during the gate: $changed"
  fi
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
  if [[ "$holder_state" == running ]] && kill -0 "$holder_pid" 2>/dev/null; then
    fail "$(holder_description) and is still running"
  fi
  rm -f "$lock_path"
  printf 'merge gate: released %s, held by pid %s from %s (%s)\n' "$lock_path" "$holder_pid" "$holder_checkout" "$holder_state"
}

[[ $# -ge 1 ]] || usage
subcommand="$1"
shift
mode=""
repo_arg=""
lock_dir=""
lock_dir_given=no
change_arg=""
requested_checks=full
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
        full | self-test) requested_checks="$2" ;;
        *) usage ;;
      esac
      shift
      ;;
    *) usage ;;
  esac
  shift
done

resolve_repo
[[ -n "$lock_dir" ]] || lock_dir="$(default_lock_dir)"
refuse_unsafe_self_test
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
