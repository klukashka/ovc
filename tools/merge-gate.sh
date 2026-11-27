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
compose_project=ovc
docker_probe_seconds=5

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
  "e2e|frontend|../tools/dev-env.sh pnpm e2e --max-failures=10 --trace=off"
  "C|.|python3 .claude/hooks/check_guard_secrets.py"
  "C|.|tools/check-dev-env.sh"
  "B|.|tools/check-merge-gate.sh"
  "B|.|tools/check-workflows.sh"
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

ci_full_jobs=(
  "pre|verbatim|.|tools/check-tool-binaries.sh"
  "frontend-checks|verbatim|.|pnpm --dir tools/docs install --frozen-lockfile"
  "frontend-checks|verbatim|.|tools/docs/check-docs.sh"
  "backend-checks|verbatim|.|bin/gitleaks git --redact --no-banner --config .gitleaks.toml ."
  "backend-checks|history-range|.|bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml ."
  "frontend-checks|verbatim|.|bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod"
  "frontend-checks|verbatim|.|bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod"
  "frontend-checks|verbatim|.|bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml"
  "frontend-checks|verbatim|.|bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml"
  "backend-checks|verbatim|backend|go mod tidy -diff"
  'backend-checks|verbatim|backend|unformatted="$(gofmt -l .)" && test -z "$unformatted"'
  "backend-checks|verbatim|backend|go vet ./..."
  "backend-checks|verbatim|backend|../bin/golangci-lint run ./..."
  "backend-checks|verbatim|backend|../bin/govulncheck ./..."
  "backend-tests|go-packages|backend|../tools/dev-env.sh go test -race -count=1 ./..."
  "backend-checks|verbatim|backend|../bin/sqlc -f sqlc.yaml diff"
  "frontend-checks|verbatim|frontend|pnpm install --frozen-lockfile"
  "frontend-checks|verbatim|frontend|pnpm typecheck"
  "frontend-checks|verbatim|frontend|pnpm lint"
  "frontend-checks|verbatim|frontend|pnpm format:check"
  "frontend-checks|verbatim|frontend|pnpm test"
  "frontend-checks|verbatim|frontend|pnpm build"
  "frontend-checks|verbatim|frontend|pnpm check:classes"
  "frontend-checks|verbatim|frontend|pnpm check:bundle"
  "frontend-checks|verbatim|frontend|pnpm api:check"
  "frontend-checks|verbatim|frontend|pnpm audit --audit-level high"
  "e2e|e2e-shard|frontend|../tools/dev-env.sh pnpm e2e --max-failures=10 --trace=off"
  "frontend-checks|verbatim|.|python3 .claude/hooks/check_guard_secrets.py"
  "frontend-checks|verbatim|.|tools/check-dev-env.sh"
  "backend-checks|verbatim|.|tools/check-merge-gate.sh"
  "backend-checks|verbatim|.|tools/check-workflows.sh"
)

ci_docs_jobs=(
  "pre|verbatim|.|tools/check-tool-binaries.sh"
  "docs-checks|verbatim|.|pnpm --dir tools/docs install --frozen-lockfile"
  "docs-checks|verbatim|.|tools/docs/check-docs.sh"
  "docs-checks|verbatim|.|bin/gitleaks git --redact --no-banner --config .gitleaks.toml ."
  "docs-checks|history-range|.|bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml ."
)

ci_bin_jobs=" backend-tests backend-checks frontend-checks docs-checks "
ci_rules=" verbatim go-packages e2e-shard history-range "
ci_unset_arguments=(-u GITHUB_ENV -u GITHUB_PATH -u GITHUB_OUTPUT -u GITHUB_STEP_SUMMARY)
e2e_workers=2

ci_job_table=(
  "validate|structure|0|success|success"
  "prepare|structure|0|success|success"
  "backend-tests|check|3|success|skipped"
  "backend-checks|check|0|success|skipped"
  "frontend-checks|check|0|success|skipped"
  "e2e|check|12|success|skipped"
  "docs-checks|check|0|skipped|success"
  "evidence|evidence|0|skipped|skipped"
  "gate|structure|0|success|success"
)

ci_host=github.com
ci_repository=klukashka/ovc
ci_owner_login=klukashka
ci_workflow_path=.github/workflows/gate.yml
ci_workflow_file=gate.yml
ci_run_url_prefix=https://github.com/klukashka/ovc/actions/runs/
ci_page_limit=50
ci_fetch_refused_variables=(
  GH_TOKEN
  GH_HOST
  GH_REPO
  GH_CONFIG_DIR
  GH_ENTERPRISE_TOKEN
  GH_DEBUG
  GITHUB_TOKEN
  GITHUB_ENTERPRISE_TOKEN
  XDG_CONFIG_HOME
)
evidence_series_runs=" before-first:1 before-rest:9 after:10 "
known_limitations_pattern='^knowledge/reviews/slice-[A-Za-z0-9.-]+-known-limitations[.]md$'

lock_change_id=""
lock_kind=""
lock_pending=""
lock_failed_run=""
lock_before_tree=""
lock_runs=""
lock_reason=""
lock_stalled_runs=""
lock_selection=""
lock_record=""
lock_run=""

usage() {
  printf '%s\n' \
    'usage: merge-gate.sh gate --ff-only [--change BRANCH] [--repo PATH] [--lock-dir PATH] [--checks full|self-test|self-test-lanes] [--lane-grace SECONDS] [--skip-tool-probes] [--skip-power-log]' \
    '       merge-gate.sh gate --squash --change BRANCH [--repo PATH] [--lock-dir PATH] [--checks full|self-test|self-test-lanes] [--lane-grace SECONDS] [--skip-tool-probes] [--skip-power-log]' \
    '       merge-gate.sh gate --ff-only --change BRANCH --change-id ID ...   (takes over a fallback lock)' \
    '       merge-gate.sh finish [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh release [--run RUN_ID | --no-run | --evidence] [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh claim --ff-only --change BRANCH --change-id ID [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh claim --catch-up --change-id ID [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh claim --evidence --change BRANCH --change-id ID --failed-run RUN_ID --kind go --package PACKAGE --test TEST [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh claim --evidence ... --kind playwright --project PROJECT --title TITLE | --kind check --entry JOB:N' \
    '       merge-gate.sh claim --after-fix --change BRANCH --change-id ID [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh ci --ff-only --change BRANCH --runs FOLDER [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh ci --catch-up --runs FOLDER [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh fallback --from-claim --reason TEXT [--stalled-runs ID[,ID...]] [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh series-open --series before-first|before-rest|after --commit SHA [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh evidence-record --run RUN_ID --runs FOLDER --reproduced yes|no [--repo PATH] [--lock-dir PATH]' \
    '       merge-gate.sh ci-fetch --run RUN_ID --commit SHA --out FOLDER [--logs] [--lock-dir PATH]' \
    '       merge-gate.sh ci-matrix --scope full|docs|change [--jobs NAME[:SHARD][,...]]' \
    '       merge-gate.sh list [--scope full|docs] [--job NAME]' \
    '       merge-gate.sh run-job NAME [--shard N/M] [--entry N] --repo PATH [--base REV] [--packages FILE]' \
    '       merge-gate.sh ci-jobs' \
    '       merge-gate.sh classify --repo PATH --base REV' >&2
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
  holder_change_id=""
  holder_kind=""
  holder_pending=""
  holder_failed_run=""
  holder_before_tree=""
  holder_runs=""
  holder_reason=""
  holder_stalled_runs=""
  holder_selection=""
  holder_record=""
  holder_run=""
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
      change_id) holder_change_id="$value" ;;
      kind) holder_kind="$value" ;;
      pending) holder_pending="$value" ;;
      failed_run) holder_failed_run="$value" ;;
      before_tree) holder_before_tree="$value" ;;
      runs) holder_runs="$value" ;;
      reason) holder_reason="$value" ;;
      stalled_runs) holder_stalled_runs="$value" ;;
      selection) holder_selection="$value" ;;
      record) holder_record="$value" ;;
      run) holder_run="$value" ;;
      *) fail "$file is not a merge gate lock; remove it by hand once no gate is active" ;;
    esac
  done <"$file"
  [[ "$holder_pid" =~ ^[1-9][0-9]*$ && -n "$holder_started" && "$holder_checkout" == /* && "$holder_common" == /* ]] ||
    fail "$file is not a merge gate lock; remove it by hand once no gate is active"
  [[ -z "$holder_lanes" || "$holder_lanes" =~ ^[1-9][0-9]*(,[1-9][0-9]*)*$ ]] ||
    fail "$file is not a merge gate lock; remove it by hand once no gate is active"
  case "$holder_state" in
    running | passed | dispatched | evidence | fallback) ;;
    *) fail "$file is not a merge gate lock; remove it by hand once no gate is active" ;;
  esac
  case "$holder_kind" in
    "" | gate | catch-up | after-fix | fallback) ;;
    *) fail "$file is not a merge gate lock; remove it by hand once no gate is active" ;;
  esac
  case "$holder_record" in
    "" | gate | ci | ci-catch-up) ;;
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
  write_lock_extra change_id "$lock_change_id"
  write_lock_extra kind "$lock_kind"
  write_lock_extra pending "$lock_pending"
  write_lock_extra failed_run "$lock_failed_run"
  write_lock_extra before_tree "$lock_before_tree"
  write_lock_extra runs "$lock_runs"
  write_lock_extra stalled_runs "$lock_stalled_runs"
  write_lock_extra selection "$lock_selection"
  write_lock_extra record "$lock_record"
  write_lock_extra run "$lock_run"
  write_lock_extra reason "$lock_reason"
}

write_lock_extra() {
  if [[ -n "$2" ]]; then
    printf '%s=%s\n' "$1" "$2"
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
  printf 'a merge gate from %s (pid %s, lane process groups %s, started %s, %s%s) holds %s' \
    "$holder_checkout" "$holder_pid" "${holder_lanes:-none}" "$holder_started" "$holder_state" \
    "${holder_record:+, record $holder_record}" "$lock_path"
}

try_link_lock() {
  local temporary
  [[ -e "$lock_path" || -L "$lock_path" ]] && return 1
  temporary="$(new_lock_file "${1:-running}")"
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
  local state="${1:-running}"
  try_link_lock "$state" && return 0
  if read_lock && [[ "$holder_state" == running ]] && ! holder_alive; then
    reclaim_stale_lock
    try_link_lock "$state" && return 0
    read_lock || fail "could not take $lock_path"
  fi
  [[ -n "$holder_pid" ]] || fail "could not take $lock_path"
  fail_held_lock
}

fail_held_lock() {
  case "$holder_state" in
    dispatched)
      fail "a claimed CI gate run of $holder_change (change $holder_change_id, kind $holder_kind, head $holder_head) holds $lock_path; it ends only through merge-gate.sh ci, release --run RUN_ID, release --no-run or fallback --from-claim"
      ;;
    evidence)
      fail "an evidence series of change $holder_change_id for the failed run $holder_failed_run holds $lock_path; nothing is claimed or merged until it ends through claim --after-fix, or through release --evidence after a recorded run that ran no test"
      ;;
    fallback)
      fail "a fallback local gate of change $holder_change_id (head $holder_head) holds $lock_path; only merge-gate.sh gate --change-id $holder_change_id takes it over"
      ;;
    passed)
      if [[ "$holder_record" == ci-catch-up ]]; then
        fail "the catch-up CI record of $main_branch at $holder_head holds $lock_path; push $main_branch, then release it with merge-gate.sh release"
      fi
      ;;
  esac
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

gate_wall_seconds() {
  elapsed_seconds "$gate_started" "${gate_finished:-$(now_seconds)}"
}

phase_fields() {
  local phase names=""
  while IFS= read -r phase; do
    [[ -n "$phase" ]] || continue
    names="$names${names:+,}$(phase_label "${phase%%|*}"):${phase#*|}"
  done <<<"$phase_times"
  printf '%s' "$names"
}

phase_label() {
  printf '%s' "${1// /}"
}

attempt_line() {
  local line phases
  line="$(printf 'time=%s base=%s tree=%s checks=%s scope=%s result=%s' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$base" "$tree" "$check_list" "$scope" "$1")"
  if [[ -n "$gate_started" ]]; then
    line="$line wall=$(gate_wall_seconds)"
  fi
  phases="$(phase_fields)"
  if [[ -n "$phases" ]]; then
    line="$line phases=$phases"
  fi
  printf '%s' "$line"
}

write_attempt() {
  prepare_log
  printf '%s\n' "$1" >>"$log_path"
}

append_log() {
  write_attempt "$(attempt_line "$1")"
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
  local line
  line="$(attempt_line e2e-run-lock-busy)"
  attempt_logged=yes
  write_attempt "$line"
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
  if [[ "$mode" == squash ]]; then
    in_repo grep --cached -q -F -e "$1" -- . ':(exclude)*.md' || status=$?
  else
    in_repo grep -q -F -e "$1" "$head" -- . ':(exclude)*.md' || status=$?
  fi
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

compose_project_labels() {
  local docker_pid watchdog_pid status=0
  docker ps --format 'project={{.Label "com.docker.compose.project"}}' </dev/null 2>/dev/null &
  docker_pid=$!
  (sleep "$docker_probe_seconds" && kill -TERM "$docker_pid" && sleep 1 && kill -KILL "$docker_pid") </dev/null >/dev/null 2>&1 &
  watchdog_pid=$!
  wait "$docker_pid" || status=$?
  kill -TERM "$watchdog_pid" 2>/dev/null || true
  wait "$watchdog_pid" 2>/dev/null || true
  return "$status"
}

other_containers() {
  local labels
  if ! command -v docker >/dev/null 2>&1 || ! labels="$(compose_project_labels)"; then
    printf 'unknown'
    return 0
  fi
  if [[ -z "$labels" ]]; then
    printf '0'
    return 0
  fi
  printf '%s\n' "$labels" | awk -v own="project=$compose_project" '
    $0 !~ /^project=/ { unreadable = 1 }
    $0 != own { others++ }
    END { if (unreadable) print "unknown"; else print others + 0 }
  '
}

record_phase_line() {
  local line
  line="gate phase $(phase_label "$1") $2: load: $(load_averages)"
  printf 'merge gate: %s\n' "$line" >&2
  prepare_log
  printf '%s\n' "$line" >>"$log_path"
}

record_gate_start() {
  power_log_started="$(local_stamp)"
  gate_start_line="gate start: power: $(power_source); load: $(load_averages); other containers: $(other_containers)"
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
  if [[ "$holding" == yes && "$attempt_logged" == no && -n "$gate_started" ]]; then
    gate_finished="$(now_seconds)" || gate_finished=""
  fi
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
  record_phase_line "$*" start
  start_lanes "$@"
  wait_lanes
  collect_lane_records
  end_lanes
  phase_times="$phase_times$*|$(elapsed_seconds "$phase_started" "$(now_seconds)")"$'\n'
  record_phase_line "$*" end
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
  printf 'gate wall time: %s s\n' "$(gate_wall_seconds)"
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
  local line
  record_gate_end
  gate_finished="$(now_seconds)"
  line="$(attempt_line failed)"
  print_report "failed: $1"
  attempt_logged=yes
  write_attempt "$line"
  exit 1
}

gate() {
  local changed line
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
  gate_finished=""
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
  fi
  gate_started="$(now_seconds)"
  take_gate_lock
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
  gate_finished="$(now_seconds)"
  line="$(attempt_line passed)"
  attempt_logged=yes
  write_attempt "$line"
  mark_passed
  print_report passed
  case "$mode" in
    ff-only) printf 'next: in the checkout of %s run git merge --ff-only %s, then merge-gate.sh finish\n' "$main_branch" "$change" ;;
    squash) printf 'next: commit the staged squash of %s without changing the index, then merge-gate.sh finish\n' "$change" ;;
  esac
}

take_gate_lock() {
  if [[ -z "$change_id_arg" ]]; then
    take_lock
    return 0
  fi
  [[ "$mode" == ff-only ]] || fail "a fallback local gate runs only with --ff-only"
  read_lock || fail "--change-id is accepted by gate only to take over a fallback lock, and no lock is held"
  [[ "$holder_state" == fallback ]] ||
    fail "--change-id is accepted by gate only to take over a fallback lock; $(holder_description)"
  [[ "$holder_common" == "$common_dir" ]] || fail "$(holder_description) for another repository"
  [[ "$holder_change_id" == "$change_id_arg" ]] ||
    fail "the fallback lock belongs to change $holder_change_id, not to $change_id_arg"
  [[ "$holder_head" == "$head" && "$holder_tree" == "$tree" && "$holder_base" == "$base" ]] ||
    fail "the fallback lock names head $holder_head, tree $holder_tree and base $holder_base, not the checkout's head $head, tree $tree and base $base"
  local taken_pid="$holder_pid" taken_started="$holder_started" temporary
  lock_change_id="$holder_change_id"
  lock_kind=fallback
  lock_reason="$holder_reason"
  lock_stalled_runs="$holder_stalled_runs"
  temporary="$(new_lock_file running)"
  if ! read_lock || [[ "$holder_state" != fallback || "$holder_pid" != "$taken_pid" || "$holder_started" != "$taken_started" ]]; then
    rm -f "$temporary"
    fail "$lock_path changed while the gate took over the fallback lock"
  fi
  mv -f "$temporary" "$lock_path"
  printf 'merge gate: took over the fallback lock of change %s (reason: %s)\n' "$lock_change_id" "$lock_reason" >&2
}

finish_log_line() {
  prepare_log
  printf 'finish: record=%s head=%s tree=%s result=passed\n' "$1" "$2" "$3" >>"$log_path"
}

finish() {
  read_lock || fail "no merge gate holds $lock_path"
  [[ "$holder_state" == passed ]] || fail "$(holder_description); finish only a gate that passed"
  [[ "$holder_common" == "$common_dir" ]] || fail "$(holder_description) for another repository"
  local record="${holder_record:-gate}"
  [[ "$record" != ci-catch-up ]] ||
    fail "the catch-up CI record already wrote its finish-equivalent line; push $main_branch, then release the lock with merge-gate.sh release"
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
  case "$record|$holder_checks|$holder_scope" in
    gate\|full\|full | gate\|full\|docs-only | gate\|docs-only\|docs-only | ci\|ci\|full | ci\|ci\|docs-only) ;;
    *) checks_problem="the $record record ran the $holder_checks check list for a $holder_scope scope, not the full run" ;;
  esac
  printf 'merge gate finish\n'
  printf 'record: %s\n' "$record"
  if [[ -n "$holder_run" ]]; then
    printf 'run: %s%s\n' "$ci_run_url_prefix" "$holder_run"
  fi
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
    printf 'result: failed record=%s\n' "$record"
    printf 'the lock stays held; report this to the owner, then release it with merge-gate.sh release\n'
    exit 1
  fi
  finish_log_line "$record" "$main_commit" "$main_tree"
  rm -f "$lock_path"
  printf 'result: passed; the trees are equal and the lock is released record=%s\n' "$record"
}

release() {
  if ! read_lock; then
    [[ -z "$release_form" ]] || fail "no claim or evidence series holds $lock_path"
    printf 'merge gate: no gate holds %s\n' "$lock_path"
    return 0
  fi
  [[ "$holder_common" == "$common_dir" ]] || fail "$(holder_description) for another repository"
  case "$release_form|$holder_state" in
    run\|dispatched | no-run\|dispatched) release_claim ;;
    evidence\|evidence) release_evidence ;;
    run\|* | no-run\|*) fail "$(holder_description); release --run and --no-run end only a claimed dispatch" ;;
    evidence\|*) fail "$(holder_description); release --evidence ends only an evidence series" ;;
    \|dispatched | \|evidence | \|fallback) fail_held_lock ;;
    *)
      if [[ "$holder_state" == running ]] && holder_alive; then
        fail "$(holder_description) and it or one of its lanes is still running"
      fi
      if [[ "$holder_record" == ci-catch-up ]]; then
        local origin
        origin="$(in_repo rev-parse --verify --quiet "refs/remotes/origin/$main_branch^{commit}")" || origin=""
        [[ "$origin" == "$holder_head" ]] ||
          fail "the catch-up CI record of $main_branch at $holder_head is released only after the push of $main_branch; origin/$main_branch is ${origin:-unknown}"
      fi
      rm -f "$lock_path"
      printf 'merge gate: released %s, held by pid %s from %s (%s)\n' "$lock_path" "$holder_pid" "$holder_checkout" "$holder_state"
      ;;
  esac
}

release_claim() {
  local run_value
  if [[ "$release_form" == run ]]; then
    run_value="$release_run_arg"
  else
    run_value=none
  fi
  end_claim_line "$holder_head" "$holder_base" "$run_value" release
  rm -f "$lock_path"
  printf 'merge gate: released the claim of %s (change %s, head %s, base %s) with run %s\n' \
    "$holder_change" "$holder_change_id" "$holder_head" "$holder_base" "$run_value"
}

release_evidence() {
  evidence_runs_have_outcome no-test ||
    fail "release --evidence ends an evidence series only after a recorded run that ran no test; the series of change $holder_change_id records ${holder_runs:-no run}"
  [[ -z "$holder_pending" ]] || fail "a dispatch of the series is pending ($holder_pending); record it with evidence-record first"
  write_log_line "evidence end: time=$(utc_now) change_id=$holder_change_id failed_run=$holder_failed_run by=release-evidence runs=$holder_runs"
  rm -f "$lock_path"
  printf 'merge gate: ended the evidence series of change %s after a run that ran no test; the change goes back to its branch\n' "$holder_change_id"
}

ci_table_row() {
  local row
  for row in "${ci_job_table[@]}"; do
    if [[ "${row%%|*}" == "$1" ]]; then
      printf '%s' "$row"
      return 0
    fi
  done
  return 1
}

ci_row_field() {
  printf '%s' "$1" | cut -d'|' -f"$2"
}

ci_check_job_names() {
  local row names=""
  for row in "${ci_job_table[@]}"; do
    if [[ "$(ci_row_field "$row" 2)" == check ]]; then
      names="$names${names:+, }${row%%|*}"
    fi
  done
  printf '%s' "$names"
}

ci_scope_of_job() {
  local row
  row="$(ci_table_row "$1")" || return 1
  [[ "$(ci_row_field "$row" 2)" == check ]] || return 1
  if [[ "$(ci_row_field "$row" 4)" == success ]]; then
    printf 'full'
  elif [[ "$(ci_row_field "$row" 5)" == success ]]; then
    printf 'docs'
  else
    return 1
  fi
}

ci_job_shards() {
  local row
  row="$(ci_table_row "$1")" || fail "$1 is not a job of the CI job table"
  ci_row_field "$row" 3
}

ci_job_scope() {
  ci_scope_of_job "$1" ||
    fail "$1 is not a CI job; the jobs are $(ci_check_job_names)"
}

ci_expected_jobs() {
  local scope="$1" row name shards conclusion shard
  for row in "${ci_job_table[@]}"; do
    name="${row%%|*}"
    shards="$(ci_row_field "$row" 3)"
    case "$scope" in
      full) conclusion="$(ci_row_field "$row" 4)" ;;
      docs) conclusion="$(ci_row_field "$row" 5)" ;;
      *) fail "the scope $scope has no expected job set" ;;
    esac
    if [[ "$shards" -gt 0 && "$conclusion" == success ]]; then
      shard=1
      while [[ "$shard" -le "$shards" ]]; do
        printf '%s (%s)|%s\n' "$name" "$shard" "$conclusion"
        shard=$((shard + 1))
      done
    else
      printf '%s|%s\n' "$name" "$conclusion"
    fi
  done
}

json_number_list() {
  local first=yes number
  printf '['
  for number in "$@"; do
    if [[ "$first" == yes ]]; then
      first=no
    else
      printf ','
    fi
    printf '%s' "$number"
  done
  printf ']'
}

ci_matrix_selected_shards() {
  local name="$1" shards="$2" selection="$3" shard entry
  shard=1
  while [[ "$shard" -le "$shards" ]]; do
    for entry in $selection; do
      if [[ "$entry" == "$name" || "$entry" == "$name:$shard" ]]; then
        printf '%s\n' "$shard"
        break
      fi
    done
    shard=$((shard + 1))
  done
}

ci_matrix_selection() {
  local entry name shard row shards seen=" " selection=""
  [[ -n "$jobs_arg" ]] || fail "ci-matrix --scope change needs --jobs with the CI jobs to run"
  [[ "${#jobs_arg}" -le 400 ]] || fail "--jobs is longer than 400 characters"
  [[ "$jobs_arg" =~ ^[a-z0-9:,-]+$ ]] || fail "--jobs holds a character outside lower-case letters, digits, hyphens, colons and commas"
  for entry in ${jobs_arg//,/ }; do
    [[ "$entry" =~ ^([a-z][a-z0-9-]*)(:([1-9][0-9]*))?$ ]] || fail "--jobs entry $entry is not NAME or NAME:SHARD"
    name="${BASH_REMATCH[1]}"
    shard="${BASH_REMATCH[3]}"
    ci_scope_of_job "$name" >/dev/null || fail "--jobs entry $entry names no CI check job; the jobs are $(ci_check_job_names)"
    row="$(ci_table_row "$name")"
    shards="$(ci_row_field "$row" 3)"
    if [[ -n "$shard" ]]; then
      [[ "$shards" -gt 0 ]] || fail "--jobs entry $entry gives a shard to $name, which has no shards"
      [[ "$shard" -le "$shards" ]] || fail "--jobs entry $entry names a shard outside 1 to $shards"
    fi
    case "$seen" in
      *" $name "* | *" $name:$shard "*) fail "--jobs names $entry twice or together with all of $name's shards" ;;
    esac
    if [[ -z "$shard" ]]; then
      case "$seen" in
        *" $name:"*) fail "--jobs names $name together with one of its shards" ;;
      esac
    fi
    seen="$seen$entry "
    selection="$selection $entry"
  done
  [[ -n "$selection" ]] || fail "--jobs names no job"
  printf '%s' "$selection"
}

ci_matrix() {
  local row name kind shards selected selection="" output="{" jobs=() numbers
  case "$scope_arg" in
    full | docs) [[ -z "$jobs_arg" ]] || fail "--jobs is accepted only with --scope change" ;;
    change) selection="$(ci_matrix_selection)" || exit 1 ;;
    *) usage ;;
  esac
  for row in "${ci_job_table[@]}"; do
    name="${row%%|*}"
    kind="$(ci_row_field "$row" 2)"
    shards="$(ci_row_field "$row" 3)"
    [[ "$kind" == check ]] || continue
    case "$scope_arg" in
      full) selected="$(ci_row_field "$row" 4)" ;;
      docs) selected="$(ci_row_field "$row" 5)" ;;
      change) selected=listed ;;
    esac
    if [[ "$shards" -gt 0 ]]; then
      numbers=""
      if [[ "$selected" == success ]]; then
        numbers="$(seq 1 "$shards")"
      elif [[ "$selected" == listed ]]; then
        numbers="$(ci_matrix_selected_shards "$name" "$shards" "$selection")"
      fi
      output="$output\"$name\":$(json_number_list $numbers),"
    elif [[ "$selected" == success ]]; then
      jobs+=("\"$name\"")
    elif [[ "$selected" == listed && " $selection " == *" $name "* ]]; then
      jobs+=("\"$name\"")
    fi
  done
  output="$output\"jobs\":$(json_number_list ${jobs[@]+"${jobs[@]}"})}"
  printf '%s\n' "$output"
}

ci_check_row() {
  local scope="$1" index="$2" job="$3" rule="$4" job_scope
  case "$ci_rules" in
    *" $rule "*) ;;
    *) fail "the CI job table of the $scope scope names the unknown rule $rule" ;;
  esac
  if [[ "$index" == 0 || "$job" == pre ]]; then
    [[ "$index" == 0 && "$job" == pre && "$rule" == verbatim ]] ||
      fail "the CI job table of the $scope scope must map the first entry, and only it, to pre with the verbatim rule"
    return 0
  fi
  job_scope="$(ci_scope_of_job "$job")" || fail "the CI job table of the $scope scope names the unknown job $job"
  [[ "$job_scope" == "$scope" ]] || fail "the CI job table of the $scope scope maps an entry to $job, a job of the $job_scope scope"
  case "$job|$rule" in
    backend-tests\|go-packages | e2e\|e2e-shard) ;;
    backend-tests\|* | e2e\|* | *\|go-packages | *\|e2e-shard)
      fail "the CI job table of the $scope scope maps an entry of $job to the $rule rule"
      ;;
  esac
}

ci_load_scope() {
  local scope="$1" entry row rest job="" rule="" matches index=0
  local entries=() rows=()
  case "$scope" in
    full)
      for entry in "${full_checks[@]}"; do
        entries+=("${entry#*|}")
      done
      rows=("${ci_full_jobs[@]}")
      ;;
    docs)
      entries=("${docs_only_checks[@]}")
      rows=("${ci_docs_jobs[@]}")
      ;;
    *) fail "the scope $scope is neither full nor docs" ;;
  esac
  for row in "${rows[@]}"; do
    rest="${row#*|}"
    rest="${rest#*|}"
    matches=0
    for entry in "${entries[@]}"; do
      [[ "$entry" != "$rest" ]] || matches=$((matches + 1))
    done
    [[ "$matches" == 1 ]] ||
      fail "the CI job table of the $scope scope maps $rest, which the $scope check list holds $matches times"
  done
  ci_entry_jobs=()
  ci_entry_rules=()
  ci_entry_directories=()
  ci_entry_commands=()
  for entry in "${entries[@]}"; do
    matches=0
    for row in "${rows[@]}"; do
      rest="${row#*|}"
      rest="${rest#*|}"
      if [[ "$rest" == "$entry" ]]; then
        matches=$((matches + 1))
        job="${row%%|*}"
        rule="${row#*|}"
        rule="${rule%%|*}"
      fi
    done
    [[ "$matches" == 1 ]] || fail "the CI job table of the $scope scope maps the entry $entry $matches times"
    ci_check_row "$scope" "$index" "$job" "$rule"
    ci_entry_jobs[index]="$job"
    ci_entry_rules[index]="$rule"
    ci_entry_directories[index]="${entry%%|*}"
    ci_entry_commands[index]="${entry#*|}"
    index=$((index + 1))
  done
  ci_entry_count="$index"
}

ci_select_job() {
  local job="$1" index=0 mapped=0
  ci_job_entries=()
  case "$ci_bin_jobs" in
    *" $job "*) ci_job_entries+=(0) ;;
  esac
  while [[ "$index" -lt "$ci_entry_count" ]]; do
    if [[ "${ci_entry_jobs[index]}" == "$job" ]]; then
      ci_job_entries+=("$index")
      mapped=$((mapped + 1))
    fi
    index=$((index + 1))
  done
  [[ "$mapped" -gt 0 ]] || fail "the CI job table maps no entry to $job"
}

ci_print_entry() {
  printf '%s|%s|%s|%s\n' "${ci_entry_jobs[$1]}" "${ci_entry_rules[$1]}" "${ci_entry_directories[$1]}" "${ci_entry_commands[$1]}"
}

ci_rewritten_command() {
  local index="$1" command
  command="${ci_entry_commands[index]}"
  case "${ci_entry_rules[index]}" in
    verbatim) ci_command="$command" ;;
    go-packages)
      [[ "$command" == *" ./..." ]] || fail "the entry $command does not end with ./..., so the shard's packages cannot replace it"
      ci_command="${command% ./...} $shard_packages"
      ;;
    e2e-shard) ci_command="$command --workers=$e2e_workers --shard=$shard_number/$(ci_job_shards e2e)" ;;
    history-range)
      [[ "$command" == *" --staged "* ]] || fail "the entry $command scans no staged changes, so it cannot become a scan of a range"
      ci_command="${command/ --staged / --log-opts=$base_commit..HEAD }"
      ;;
    *) fail "the entry $command has the unknown rule ${ci_entry_rules[index]}" ;;
  esac
}

without_step_files() {
  env "${ci_unset_arguments[@]}" "$@"
}

refuse_unclean_candidate() {
  local changes
  candidate_head="$(in_repo rev-parse --verify --quiet "HEAD^{commit}")" || fail "the candidate checkout $root has no commit"
  changes="$(in_repo status --porcelain --untracked-files=all)" || fail "git status failed in the candidate checkout $root"
  [[ -z "$changes" ]] ||
    fail "the candidate checkout $root has unstaged, staged or untracked changes; it must be a clean checkout of the candidate commit"
  refuse_influential_ignored_files
  candidate_tree="$(in_repo rev-parse --verify "HEAD^{tree}")" || fail "the candidate checkout $root has no tree"
}

resolve_candidate_base() {
  [[ "$base_arg" != -* ]] || fail "the base $base_arg is not a revision"
  has_control_character "$base_arg" && fail "the base holds a control character"
  base_commit="$(in_repo rev-parse --verify --quiet "$base_arg^{commit}")" ||
    fail "the base $base_arg is not a commit of the candidate checkout"
  [[ "$base_commit" != "$candidate_head" ]] || fail "the candidate $candidate_head holds no commit beyond the base"
  in_repo merge-base --is-ancestor "$base_commit" "$candidate_head" ||
    fail "the base $base_commit is not an ancestor of the candidate $candidate_head"
}

refuse_job_tool_settings() {
  local folder line node_version pnpm_version pnpm_settings problem
  environment_lines=()
  if command -v go >/dev/null 2>&1; then
    for folder in "${go_setting_folders[@]}"; do
      [[ -d "$root/$folder" ]] || continue
      line="$(go_environment_line "$folder")" || exit 1
      environment_lines+=("$line")
    done
  else
    environment_lines+=("go environment: go is not on the path of this job")
  fi
  if ! command -v node >/dev/null 2>&1 || ! command -v pnpm >/dev/null 2>&1; then
    environment_lines+=("node environment: node or pnpm is not on the path of this job")
    return 0
  fi
  node_version="$(node --version)" || fail "node --version failed"
  pnpm_version="$(pnpm --dir "$root/frontend" --version)" || fail "pnpm --version failed"
  pnpm_settings="$(pnpm --dir "$root/frontend" config list --json)" || fail "pnpm config list failed"
  problem="$(printf '%s' "$pnpm_settings" | pnpm_settings_problem)" || fail "could not read pnpm's settings"
  [[ -z "$problem" ]] ||
    fail "pnpm's effective settings $problem, from its user, global or project configuration, would change what the checks run; remove them before the job"
  environment_lines+=("node environment: node $node_version, pnpm $pnpm_version, pnpm node-options, script-shell and shell-emulator: defaults")
}

line_listed() {
  case $'\n'"$1"$'\n' in
    *$'\n'"$2"$'\n'*) return 0 ;;
    *) return 1 ;;
  esac
}

read_shard_packages() {
  local directory="$1" listed line
  [[ -f "$packages_arg" && ! -L "$packages_arg" ]] || fail "the package list $packages_arg is not a regular file"
  listed="$(without_step_files go -C "$root/$directory" list ./... </dev/null)" ||
    fail "go list ./... failed in the candidate's $directory"
  shard_packages=""
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[A-Za-z0-9][A-Za-z0-9._/~-]*$ ]] || fail "the package list $packages_arg holds a line that is not a Go package path"
    line_listed "$listed" "$line" ||
      fail "the package list names $line, which go list ./... does not list in the candidate's $directory"
    case " $shard_packages " in
      *" $line "*) fail "the package list names $line twice" ;;
    esac
    shard_packages="$shard_packages${shard_packages:+ }$line"
  done <"$packages_arg"
  [[ -n "$shard_packages" ]] || fail "the package list $packages_arg is empty"
}

ci_check_job_options() {
  local index needs_base=no needs_shard=no needs_packages=no shard_count
  for index in "${ci_job_entries[@]}"; do
    case "${ci_entry_rules[index]}" in
      history-range) needs_base=yes ;;
      go-packages)
        needs_shard=yes
        needs_packages=yes
        ;;
      e2e-shard) needs_shard=yes ;;
    esac
  done
  shard_count="$(ci_job_shards "$ci_job")" || exit 1
  [[ "$shard_count" != 0 && "$needs_shard" == yes ]] || shard_count=""
  shard_number=""
  if [[ -n "$shard_count" ]]; then
    [[ "$shard_arg" =~ ^([1-9][0-9]*)/([1-9][0-9]*)$ ]] || fail "run-job $ci_job needs --shard N/$shard_count"
    shard_number="${BASH_REMATCH[1]}"
    [[ "${BASH_REMATCH[2]}" == "$shard_count" && "$shard_number" -le "$shard_count" ]] ||
      fail "run-job $ci_job needs --shard N/$shard_count with N from 1 to $shard_count"
  else
    [[ -z "$shard_arg" ]] || fail "run-job $ci_job takes no --shard"
  fi
  if [[ "$needs_packages" == yes ]]; then
    [[ -n "$packages_arg" ]] || fail "run-job backend-tests needs --packages with the shard's package list"
  else
    [[ -z "$packages_arg" ]] || fail "run-job $ci_job takes no --packages"
  fi
  if [[ "$needs_base" == yes ]]; then
    [[ -n "$base_arg" ]] || fail "run-job $ci_job needs --base with main's commit, the base of its range scan"
  else
    [[ -z "$base_arg" ]] || fail "run-job $ci_job takes no --base"
  fi
}

ci_select_entry() {
  local chosen
  [[ -n "$entry_arg" ]] || return 0
  [[ "$entry_arg" =~ ^[1-9][0-9]{0,2}$ && "$entry_arg" -le "${#ci_job_entries[@]}" ]] ||
    fail "run-job $ci_job --entry needs N from 1 to ${#ci_job_entries[@]}, the entries merge-gate.sh list --job $ci_job prints"
  chosen="${ci_job_entries[entry_arg - 1]}"
  if [[ "${ci_job_entries[0]}" == 0 && "$chosen" != 0 ]]; then
    ci_job_entries=(0 "$chosen")
  else
    ci_job_entries=("$chosen")
  fi
}

ci_jobs() {
  local row
  for row in "${ci_job_table[@]}"; do
    printf '%s\n' "${row%%|*}"
  done
}

run_job() {
  local job_scope index directory status started finished failed="" after_head after_changes line
  job_scope="$(ci_job_scope "$ci_job")" || exit 1
  ci_load_scope "$job_scope"
  ci_select_job "$ci_job"
  ci_select_entry
  ci_check_job_options
  resolve_repo
  refuse_unclean_candidate
  refuse_job_tool_settings
  base_commit=""
  if [[ -n "$base_arg" ]]; then
    resolve_candidate_base
  fi
  shard_packages=""
  for index in "${ci_job_entries[@]}"; do
    case "${ci_entry_rules[index]}" in
      go-packages) read_shard_packages "${ci_entry_directories[index]}" ;;
    esac
  done
  printf 'merge gate run-job\n'
  printf 'job: %s\n' "$ci_job"
  printf 'scope: %s\n' "$job_scope"
  printf 'shard: %s\n' "${shard_arg:-none}"
  printf 'entry: %s\n' "${entry_arg:-all}"
  printf 'candidate: %s\n' "$candidate_head"
  printf 'candidate tree: %s\n' "$candidate_tree"
  printf 'base: %s\n' "${base_commit:-none}"
  for line in ${environment_lines[@]+"${environment_lines[@]}"}; do
    printf '%s\n' "$line"
  done
  if [[ -n "$shard_packages" ]]; then
    printf 'packages: %s\n' "$shard_packages"
  fi
  for index in "${ci_job_entries[@]}"; do
    ci_rewritten_command "$index"
    directory="${ci_entry_directories[index]}"
    if [[ -n "$failed" ]]; then
      printf 'not run: %s: %s\n' "$directory" "$ci_command"
      continue
    fi
    printf 'merge gate: running in %s: %s\n' "$directory" "$ci_command" >&2
    started="$(now_seconds)"
    status=0
    (cd "$root/$directory" && exec env "${ci_unset_arguments[@]}" bash -c "$ci_command") </dev/null >&2 || status=$?
    finished="$(now_seconds)"
    printf 'exit %s: %s s: %s: %s\n' "$status" "$(elapsed_seconds "$started" "$finished")" "$directory" "$ci_command"
    if [[ "$status" != 0 ]]; then
      failed="$directory: $ci_command"
    fi
  done
  after_head="$(in_repo rev-parse --verify "HEAD^{commit}")" || fail "the candidate checkout $root lost its commit during the job"
  after_changes="$(in_repo status --porcelain --untracked-files=all)" || fail "git status failed in the candidate checkout $root"
  if [[ "$after_head" != "$candidate_head" || -n "$after_changes" ]]; then
    printf 'candidate checkout after the job: changed\n'
    [[ -z "$after_changes" ]] || printf 'changed in the candidate checkout: %s\n' "$after_changes"
    failed="${failed:-the candidate checkout changed during the job}"
  else
    printf 'candidate checkout after the job: unchanged\n'
  fi
  if [[ -n "$failed" ]]; then
    printf 'result: failed: %s\n' "$failed"
    exit 1
  fi
  printf 'result: passed\n'
}

classify() {
  resolve_repo
  refuse_unclean_candidate
  resolve_candidate_base
  mode=ff-only
  base="$base_commit"
  head="$candidate_head"
  if docs_only_change; then
    printf 'docs-only\n'
  else
    printf 'full\n'
  fi
}

list_entries() {
  local scope="$scope_arg" job_scope index=0
  case "$scope" in
    "" | full | docs) ;;
    *) usage ;;
  esac
  if [[ -n "$ci_job" ]]; then
    job_scope="$(ci_job_scope "$ci_job")" || exit 1
    [[ -z "$scope" || "$scope" == "$job_scope" ]] || fail "$ci_job is a job of the $job_scope scope, not of $scope"
    scope="$job_scope"
  fi
  ci_load_scope "${scope:-full}"
  if [[ -z "$ci_job" ]]; then
    while [[ "$index" -lt "$ci_entry_count" ]]; do
      ci_print_entry "$index"
      index=$((index + 1))
    done
    return 0
  fi
  ci_select_job "$ci_job"
  for index in "${ci_job_entries[@]}"; do
    ci_print_entry "$index"
  done
}

ci_python() {
  python3 -I -c '
import hashlib
import json
import os
import re
import subprocess
import sys
import zipfile
from datetime import datetime

GATE_TITLE = re.compile(r"gate (full|docs) ([0-9a-f]{40})")
OTHER_TITLE = re.compile(r"gate (change|evidence) .*")
ROW_KIND = re.compile(r"(?<![A-Za-z0-9-])(rerun|after-fix|fallback)(?![A-Za-z0-9-])")
LOG_TIMESTAMP = re.compile(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:[.][0-9]+)?Z ")
SELECTION_LINE = re.compile(r"evidence selection: (\S+)")
HEADER_LINE = re.compile(r"evidence series (before-first|before-rest|after) of candidate ([0-9a-f]{40}): ([0-9]+) runs of the (go|playwright|check) selection")
NONCE_LINE = re.compile(r"evidence run ([0-9]+)/([0-9]+) nonce: ([0-9a-f]{32})")
RESULT_LINE = re.compile(r"evidence result: run=([0-9]+)/([0-9]+) outcome=(passed|failed|no-test) nonce=([0-9a-f]{32})")
SUMMARY_LINE = re.compile(r"evidence series (before-first|before-rest|after): ([0-9]+) runs, ([0-9]+) passed, ([0-9]+) failed, ([0-9]+) ran no test")
SELECTION_KINDS = {"go": ("package", "test"), "playwright": ("project", "title"), "check": ("entry",)}
SELECTION_FIELDS = ("package", "test", "project", "title", "entry")
SELECTION_VALUES = {
    "package": re.compile(r"[A-Za-z0-9_./][A-Za-z0-9_./-]{0,199}"),
    "test": re.compile(r"Test(?!.*//)(?!.*/$)[A-Za-z0-9_/]{1,196}"),
    "project": re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,63}"),
    "title": re.compile(r"[A-Za-z0-9](?:[A-Za-z0-9 ,:._()/@+?›’×–…»«“”-]{0,198}[A-Za-z0-9,:._()/@+?›’×–…»«“”-])?"),
    "entry": re.compile(r"(backend-checks|frontend-checks|docs-checks):[1-9][0-9]{0,2}"),
}
SHA_PATTERN = re.compile(r"[0-9a-f]{40}")
RUN_LIST = re.compile(r"[0-9]{1,20}(,[0-9]{1,20})*")
LOG_LIMIT = 1073741824


def fail(message):
    sys.stderr.write("merge gate: " + message + "\n")
    sys.exit(1)


def options(arguments):
    result = {}
    for argument in arguments:
        key, separator, value = argument.partition("=")
        if not separator:
            fail("internal error: the helper argument " + argument + " is not KEY=VALUE")
        result[key] = value
    return result


def read_json_file(path, label):
    if os.path.islink(path) or not os.path.isfile(path):
        fail(label + " is missing")
    try:
        with open(path, encoding="utf-8") as handle:
            data = json.load(handle)
    except (OSError, ValueError):
        fail(label + " is not JSON")
    if not isinstance(data, dict):
        fail(label + " is not a JSON object")
    return data


def read_json(folder, name):
    return read_json_file(os.path.join(folder, name), name + " of the fetched runs folder " + folder)


def field(data, path):
    value = data
    for part in path.split("."):
        if not isinstance(value, dict):
            return None
        value = value.get(part)
    return value


def first_attempt(value):
    return type(value) is int and value == 1


def sha256_of(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        while True:
            block = handle.read(1048576)
            if not block:
                break
            digest.update(block)
    return digest.hexdigest()


def folder_files(folder):
    found = []
    for directory, subdirectories, files in os.walk(folder):
        for name in subdirectories + files:
            path = os.path.join(directory, name)
            if os.path.islink(path):
                fail("the fetched runs folder " + folder + " holds the link " + os.path.relpath(path, folder))
            if "\n" in name or "\r" in name:
                fail("the fetched runs folder " + folder + " holds a name with a line break")
        for name in files:
            path = os.path.join(directory, name)
            if not os.path.isfile(path):
                fail("the fetched runs folder " + folder + " holds " + os.path.relpath(path, folder) + ", which is not a regular file")
            found.append(os.path.relpath(path, folder))
    return sorted(found)


def write_sums(o):
    folder = o["folder"]
    lines = []
    for relative in folder_files(folder):
        if relative == "SHA256SUMS":
            continue
        lines.append(sha256_of(os.path.join(folder, relative)) + "  " + relative)
    with open(os.path.join(folder, "SHA256SUMS"), "w", encoding="utf-8") as handle:
        handle.write("".join(line + "\n" for line in lines))
    os.chmod(os.path.join(folder, "SHA256SUMS"), 0o600)
    for line in lines:
        print(line)


def verify_sums(folder):
    path = os.path.join(folder, "SHA256SUMS")
    if os.path.islink(path) or not os.path.isfile(path):
        fail("the fetched runs folder " + folder + " holds no SHA256SUMS; fetch it with merge-gate.sh ci-fetch")
    listed = {}
    with open(path, encoding="utf-8") as handle:
        for line in handle.read().splitlines():
            match = re.fullmatch(r"([0-9a-f]{64})  (\S.*)", line)
            if match is None or match.group(2) in listed:
                fail("SHA256SUMS of the fetched runs folder " + folder + " holds a malformed or repeated line")
            listed[match.group(2)] = match.group(1)
    present = [relative for relative in folder_files(folder) if relative != "SHA256SUMS"]
    if sorted(listed) != present:
        fail("the files of the fetched runs folder " + folder + " differ from those its SHA256SUMS lists")
    for relative in present:
        if sha256_of(os.path.join(folder, relative)) != listed[relative]:
            fail("the SHA-256 of " + relative + " in " + folder + " differs from its SHA256SUMS line")
    return sha256_of(path), listed


def fetch_record(folder):
    path = os.path.join(folder, "fetch.txt")
    if os.path.islink(path) or not os.path.isfile(path):
        fail("the fetched runs folder " + folder + " holds no fetch.txt; fetch it with merge-gate.sh ci-fetch")
    values = {}
    with open(path, encoding="utf-8") as handle:
        for line in handle.read().splitlines():
            key, separator, value = line.partition("=")
            if separator:
                values[key] = value
    return values


def complete_list(data, key, label):
    total = data.get("total_count")
    items = data.get(key)
    if type(total) is not int or not isinstance(items, list):
        fail(label + " holds no total_count and " + key + " list")
    if len(items) != total:
        fail(label + " holds " + str(len(items)) + " of its " + str(total) + " entries, so the list is not completely read; fetch it again")
    for item in items:
        if not isinstance(item, dict) or type(item.get("id")) is not int:
            fail(label + " holds an entry without a numeric id")
    return items


def claims_from_log(log):
    claims = []
    if not log or not os.path.exists(log):
        return claims
    if os.path.islink(log) or not os.path.isfile(log):
        fail(log + " is not the merge gate log")
    with open(log, encoding="utf-8", errors="replace") as handle:
        for line in handle:
            if not line.startswith("claim: "):
                continue
            values = {}
            for token in line.rstrip("\n")[len("claim: "):].split(" "):
                key, separator, value = token.partition("=")
                if separator:
                    values[key] = value
            claims.append(values)
    return claims


def related_claims(claims, candidate, base, require_open):
    current_index = None
    for index, claim in enumerate(claims):
        if claim.get("head") == candidate and claim.get("base") == base and (claim.get("run") == "open" or not require_open):
            current_index = index
    if current_index is None:
        if require_open:
            fail("the merge gate log holds no open claim of " + candidate + " on " + base + "; run merge-gate.sh claim before the dispatch")
        return None, []
    current = claims[current_index]
    change_id = current.get("change_id") or ""
    patch_id = current.get("patch_id") or "none"
    related = []
    for index, claim in enumerate(claims):
        if index == current_index:
            continue
        same_change = change_id != "" and claim.get("change_id") == change_id
        same_patch = patch_id != "none" and claim.get("patch_id") == patch_id
        if same_change or same_patch:
            related.append(claim)
    return current, related


def claim_runs(claim):
    value = claim.get("run", "")
    if value == "none":
        return [], True
    if value == "open":
        return None, False
    if RUN_LIST.fullmatch(value):
        return value.split(","), False
    fail("the claim line of " + str(claim.get("head")) + " in the merge gate log holds the malformed run " + value)


def claimed(o):
    claims = claims_from_log(o.get("log", ""))
    current, related = related_claims(claims, o["candidate"], o["base"], False)
    printed = set()
    for claim in related:
        ids, without_run = claim_runs(claim)
        for run_id in ids or []:
            if ("run", run_id) not in printed:
                printed.add(("run", run_id))
                print("run " + run_id)
        old_base = claim.get("base", "")
        if old_base != o["base"]:
            if not SHA_PATTERN.fullmatch(old_base):
                fail("the claim line of " + str(claim.get("head")) + " holds the malformed base " + old_base)
            if ("base", old_base) not in printed:
                printed.add(("base", old_base))
                print("base " + old_base)


def pages(o):
    key = o["key"]
    files = [path for path in o["pages"].split(":") if path]
    items = []
    total = None
    last_count = 0
    for path in files:
        data = read_json_file(path, "the page " + os.path.basename(path))
        page_total = data.get("total_count")
        page_items = data.get(key)
        if type(page_total) is not int or not isinstance(page_items, list):
            fail("the page " + os.path.basename(path) + " holds no total_count and " + key + " list")
        if total is None:
            total = page_total
        elif total != page_total:
            fail("the pages of " + key + " disagree on total_count, so the list changed while it was read; fetch it again")
        items.extend(page_items)
        last_count = len(page_items)
    if total is None:
        fail("internal error: no page was read")
    if len(items) == total:
        with open(o["out"], "w", encoding="utf-8") as handle:
            json.dump({"total_count": total, key: items}, handle, sort_keys=True)
        os.chmod(o["out"], 0o600)
        print("complete")
    elif len(items) > total or last_count == 0:
        print("short " + str(len(items)) + " " + str(total))
    else:
        print("incomplete")


def json_field(o):
    data = read_json_file(o["file"], os.path.basename(o["file"]))
    value = field(data, o["path"])
    if type(value) is int or isinstance(value, str):
        print(value)
    else:
        fail(os.path.basename(o["file"]) + " holds no " + o["path"])


def extract(o):
    destination = o["destination"]
    os.mkdir(destination, 0o700)
    total = 0
    try:
        archive = zipfile.ZipFile(o["zip"])
    except (OSError, zipfile.BadZipFile):
        fail("the logs of the run are not a zip archive")
    with archive:
        for info in archive.infolist():
            name = info.filename
            if name.startswith("/") or "\\" in name or any(ord(character) < 32 or ord(character) == 127 for character in name):
                fail("the logs archive holds the unsafe path " + repr(name))
            parts = name.split("/")
            directory = name.endswith("/")
            if directory:
                parts = parts[:-1]
            if not parts or any(part in ("", ".", "..") for part in parts):
                fail("the logs archive holds the unsafe path " + repr(name))
            kind = (info.external_attr >> 16) & 0o170000
            if kind not in (0, 0o100000, 0o040000):
                fail("the logs archive holds " + repr(name) + ", which is not a regular file or folder")
            target = os.path.join(destination, *parts)
            if directory or kind == 0o040000:
                os.makedirs(target, mode=0o700, exist_ok=True)
                continue
            total += info.file_size
            if total > LOG_LIMIT:
                fail("the logs archive expands beyond " + str(LOG_LIMIT) + " bytes")
            os.makedirs(os.path.dirname(target), mode=0o700, exist_ok=True)
            with archive.open(info) as source, open(target, "xb") as sink:
                while True:
                    block = source.read(1048576)
                    if not block:
                        break
                    sink.write(block)
            os.chmod(target, 0o600)


def check_identity(run, url, o, title, head_sha):
    expected = [
        ("repository.full_name", o["repository"]),
        ("head_repository.full_name", o["repository"]),
        ("path", o["workflow"]),
        ("event", "workflow_dispatch"),
        ("head_branch", "main"),
        ("head_sha", head_sha),
        ("display_title", title),
        ("triggering_actor.login", o["owner"]),
    ]
    for path, wanted in expected:
        actual = field(run, path)
        if actual != wanted:
            fail("the run " + url + " has " + path + " " + json.dumps(actual) + ", not " + json.dumps(wanted))
        print("report: identity: " + path + ": " + wanted)


def stamp(value, label):
    if not isinstance(value, str):
        fail(label + " has no timestamp")
    try:
        return datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ")
    except ValueError:
        fail(label + " has the unreadable timestamp " + value)


def tree_of(root, commit):
    result = subprocess.run(["git", "-C", root, "rev-parse", "--verify", "--quiet", commit + "^{tree}"], capture_output=True, text=True)
    if result.returncode != 0:
        return None
    return result.stdout.strip()


def row_names(root, tree, url):
    result = subprocess.run(
        ["git", "-C", root, "grep", "-h", "-I", "-F", "-e", url, tree, "--", ":(glob)knowledge/reviews/*known-limitations.md"],
        capture_output=True,
        text=True,
    )
    if result.returncode == 1:
        return False
    if result.returncode != 0:
        fail("git grep failed while looking for a known-limitations row naming " + url)
    exact = re.compile(re.escape(url) + r"(?![0-9])")
    return any(exact.search(line) and ROW_KIND.search(line) for line in result.stdout.splitlines())


def title_commit(entry):
    title = entry.get("display_title")
    match = GATE_TITLE.fullmatch(title) if isinstance(title, str) else None
    return match.group(2) if match else None


def check_claimed_run(entry, claim, o, url):
    head = claim.get("head")
    expected = [
        ("repository.full_name", o["repository"]),
        ("path", o["workflow"]),
        ("event", "workflow_dispatch"),
        ("head_sha", claim.get("base")),
    ]
    for path, wanted in expected:
        actual = field(entry, path)
        if actual != wanted:
            fail("the run " + url + " recorded for the claim of " + str(head) + " on " + str(claim.get("base")) + " has " + path + " " + json.dumps(actual) + ", not " + json.dumps(wanted) + "; a mistyped release --run or --stalled-runs is corrected by the owner")
    if title_commit(entry) != head:
        fail("the run " + url + " recorded for the claim of " + str(head) + " on " + str(claim.get("base")) + " is titled " + json.dumps(entry.get("display_title")) + ", not gate full or gate docs " + str(head) + "; a mistyped release --run or --stalled-runs is corrected by the owner")


def selection_text(kind, values):
    title = values.get("title", "")
    digest = hashlib.sha256(title.encode("utf-8")).hexdigest() if title else ""
    parts = ["kind:" + kind, "package:" + values.get("package", ""), "test:" + values.get("test", ""), "project:" + values.get("project", ""), "title-sha256:" + digest, "entry:" + values.get("entry", "")]
    return "|".join(parts)


def selection(o):
    kind = o.get("kind", "")
    used = SELECTION_KINDS.get(kind)
    if used is None:
        fail("--kind must be go, playwright or check")
    for name in SELECTION_FIELDS:
        value = o.get(name, "")
        if name in used:
            if not SELECTION_VALUES[name].fullmatch(value):
                fail("--" + name + " " + json.dumps(value) + " is missing or outside the characters gate.yml accepts for a " + kind + " selection")
        elif value:
            fail("--" + name + " is not part of a " + kind + " selection")
    print(selection_text(kind, o))


def check_jobs(folder, url, run_id, scope, expected):
    jobs = complete_list(read_json(folder, "jobs.json"), "jobs", "jobs.json of " + folder)
    seen = {}
    for job in jobs:
        name = job.get("name")
        if not isinstance(name, str):
            fail("the run " + url + " lists a job without a name")
        if name in seen:
            fail("the run " + url + " lists the job " + name + " twice")
        if job.get("run_id") is not None and job.get("run_id") != run_id:
            fail("jobs.json lists the job " + name + " of another run")
        seen[name] = job
    missing = [name for name, conclusion in expected if name not in seen]
    if missing:
        fail("the run " + url + " lacks the expected jobs " + ", ".join(missing) + " of the " + scope + " scope")
    names = set(name for name, conclusion in expected)
    extra = [name for name in seen if name not in names]
    if extra:
        fail("the run " + url + " lists jobs the " + scope + " scope does not expect: " + ", ".join(sorted(extra)))
    for name, wanted in expected:
        job = seen[name]
        if not first_attempt(job.get("run_attempt")):
            fail("the job " + name + " of the run " + url + " ran at attempt " + json.dumps(job.get("run_attempt")) + ", not 1: it was retried")
        conclusion = job.get("conclusion")
        if job.get("status") != "completed":
            fail("the job " + name + " of the run " + url + " is " + json.dumps(job.get("status")) + ", not completed")
        if conclusion != wanted:
            if wanted == "skipped":
                fail("the job " + name + " of the run " + url + " should be skipped in the " + scope + " scope but concluded " + json.dumps(conclusion))
            fail("the job " + name + " of the run " + url + " concluded " + json.dumps(conclusion) + ", not success")
        print("report: job: " + name + ": " + conclusion + " at attempt 1")
    return seen


def run_times(run, seen, url):
    ran = [job for job in seen.values() if job.get("conclusion") != "skipped"]
    starts = [stamp(job.get("started_at"), "the job " + job["name"] + " of " + url) for job in ran]
    earliest = min(starts)
    gate_end = stamp(seen["gate"].get("completed_at"), "the gate job of " + url)
    execution = int((gate_end - earliest).total_seconds())
    queue = int((earliest - stamp(run.get("created_at"), "the run " + url)).total_seconds())
    job_queue = 0
    for job in ran:
        if job.get("created_at") is not None:
            waited = int((stamp(job.get("started_at"), job["name"]) - stamp(job.get("created_at"), job["name"])).total_seconds())
            job_queue = max(job_queue, waited)
    return execution, queue, job_queue


def verify(o):
    folder = o["folder"]
    root = o["root"]
    candidate = o["candidate"]
    base = o["base"]
    tree = o["tree"]
    prefix = o["prefix"]
    scopes = o["scopes"].split(",")
    expected_by_scope = {}
    for line in sys.stdin.read().splitlines():
        scope, name, conclusion = line.split("|")
        expected_by_scope.setdefault(scope, []).append((name, conclusion))
    sums_digest, listed = verify_sums(folder)
    fetched = fetch_record(folder)
    run = read_json(folder, "run.json")
    run_id = run.get("id")
    if type(run_id) is not int or fetched.get("run") != str(run_id):
        fail("run.json of " + folder + " is not the run its fetch.txt names")
    url = prefix + str(run_id)
    if fetched.get("commit") != candidate:
        fail("the folder " + folder + " was fetched for the commit " + str(fetched.get("commit")) + ", not for the candidate " + candidate)
    title = run.get("display_title")
    if isinstance(title, str) and OTHER_TITLE.fullmatch(title):
        fail("the run " + url + " is titled " + json.dumps(title) + ": a change or evidence run is never a gate")
    match = GATE_TITLE.fullmatch(title) if isinstance(title, str) else None
    title_scope = match.group(1) if match and match.group(2) == candidate and match.group(1) in scopes else scopes[0]
    print("report: run: " + url)
    check_identity(run, url, o, "gate " + title_scope + " " + candidate, base)
    if run.get("status") != "completed":
        fail("the run " + url + " is " + json.dumps(run.get("status")) + ", not completed")
    if not first_attempt(run.get("run_attempt")):
        fail("the run " + url + " is at attempt " + json.dumps(run.get("run_attempt")) + ", not 1: it was re-run")
    if run.get("conclusion") != "success":
        fail("the run " + url + " concluded " + json.dumps(run.get("conclusion")) + ", not success")
    print("report: completion: completed, success, attempt 1")
    seen = check_jobs(folder, url, run_id, title_scope, expected_by_scope[title_scope])
    execution, queue, job_queue = run_times(run, seen, url)
    claims = claims_from_log(o["log"])
    current, related = related_claims(claims, candidate, base, True)
    related_heads = set(claim.get("head") for claim in related if claim.get("head"))
    attempts = {}
    history = complete_list(read_json(folder, "history.json"), "workflow_runs", "history.json of " + folder)
    for entry in history:
        if entry.get("head_sha") != base:
            fail("history.json lists the run " + prefix + str(entry["id"]) + " with head_sha " + json.dumps(entry.get("head_sha")) + ", not " + base)
        if entry["id"] == run_id:
            continue
        named = title_commit(entry)
        if named is None:
            continue
        if named == candidate or named in related_heads:
            attempts[entry["id"]] = entry
            continue
        named_tree = tree_of(root, named)
        if named_tree is None or named_tree == tree:
            attempts[entry["id"]] = entry
    for claim in related:
        ids, without_run = claim_runs(claim)
        if ids is None:
            fail("the earlier claim of " + str(claim.get("head")) + " on " + str(claim.get("base")) + " (change " + str(claim.get("change_id")) + ") never ended in the merge gate log")
        for text in ids:
            entry = read_json(folder, "claims/run-" + text + ".json")
            if entry.get("id") != int(text):
                fail("claims/run-" + text + ".json of " + folder + " is not the run " + text)
            check_claimed_run(entry, claim, o, prefix + text)
            if entry["id"] != run_id:
                attempts[entry["id"]] = entry
        if claim.get("base") != base:
            old = complete_list(read_json(folder, "bases/" + claim["base"] + ".json"), "workflow_runs", "bases/" + claim["base"] + ".json of " + folder)
            for entry in old:
                if entry.get("head_sha") != claim.get("base"):
                    fail("bases/" + claim["base"] + ".json of " + folder + " lists the run " + prefix + str(entry["id"]) + " with head_sha " + json.dumps(entry.get("head_sha")))
                if title_commit(entry) == claim.get("head") and entry["id"] != run_id:
                    attempts[entry["id"]] = entry
    for attempt_id in sorted(attempts):
        entry = attempts[attempt_id]
        attempt_url = prefix + str(attempt_id)
        status = entry.get("status")
        conclusion = entry.get("conclusion")
        attempt = entry.get("run_attempt")
        description = json.dumps(status) + ", " + json.dumps(conclusion) + ", attempt " + json.dumps(attempt)
        if status == "completed" and conclusion == "success" and first_attempt(attempt):
            print("report: earlier attempt: " + attempt_url + ": " + description)
            continue
        if row_names(root, tree, attempt_url):
            print("report: earlier attempt: " + attempt_url + ": " + description + ", named by a row of a known-limitations file")
            continue
        if status != "completed":
            fail("the earlier attempt " + attempt_url + " is still " + json.dumps(status) + "; wait for it or cancel it, and name it in a rerun, after-fix or fallback row of the slice known-limitations file")
        fail("the earlier attempt " + attempt_url + " ended " + description + ", and the candidate tree holds no rerun, after-fix or fallback row of a known-limitations file naming it")
    print("value: run_id=" + str(run_id))
    print("value: run_url=" + url)
    print("value: title_scope=" + title_scope)
    print("value: execution=" + str(execution))
    print("value: queue=" + str(queue))
    print("value: job_queue=" + str(job_queue))
    print("value: sums=" + sums_digest)
    print("value: attempts=" + str(len(attempts)))


def harness_text(line):
    text = line.rstrip("\r\n")
    match = LOG_TIMESTAMP.match(text)
    return text[match.end():] if match else text


def nearest(texts, before, pattern, accept):
    index = before - 1
    while index >= 0:
        match = pattern.fullmatch(texts[index])
        if match is not None and accept(match):
            return index, match
        index -= 1
    return None, None


def harness_series(texts, series, count, commit, selection_wanted, url):
    summary_index, summary = nearest(texts, len(texts), SUMMARY_LINE, lambda match: True)
    if summary is None:
        return None
    label = "the evidence log of " + url
    if summary.group(1) != series or int(summary.group(2)) != count:
        fail(label + " ends with the summary of the " + summary.group(1) + " series of " + summary.group(2) + " runs, not of the " + series + " series of " + str(count) + " runs")
    outcomes = {}
    position = summary_index
    for number in range(count, 0, -1):
        result_index, result = nearest(texts, position, RESULT_LINE, lambda match: True)
        if result is None or int(result.group(1)) != number or int(result.group(2)) != count:
            fail(label + " holds no harness result line of run " + str(number) + " of " + str(count) + " before the summary; the run is not recorded, report it to the owner")
        nonce = result.group(4)
        nonce_index, opened = nearest(texts, result_index, NONCE_LINE, lambda match: match.group(3) == nonce)
        if opened is None or int(opened.group(1)) != number or int(opened.group(2)) != count:
            fail(label + " holds a result line of run " + str(number) + " whose nonce the harness did not print before the output of that run; the run is not recorded, report it to the owner")
        outcomes[number] = result.group(3)
        position = nonce_index
    header_index, header = nearest(texts, position, HEADER_LINE, lambda match: True)
    if header is None or header.group(1) != series or header.group(2) != commit or int(header.group(3)) != count:
        fail(label + " holds no harness header of the " + series + " series of " + commit + " with " + str(count) + " runs before its first run")
    selection_index, selected = nearest(texts, header_index, SELECTION_LINE, lambda match: True)
    if selected is None:
        fail(label + " holds no evidence selection line before the series header")
    if selected.group(1) != selection_wanted:
        fail(label + " ran the selection " + selected.group(1) + ", not the claimed selection " + selection_wanted + "; the run is not recorded, report it to the owner")
    if not selection_wanted.startswith("kind:" + header.group(4) + "|"):
        fail(label + " names the " + header.group(4) + " kind in its header, not the kind of the claimed selection")
    ordered = [outcomes[number] for number in range(1, count + 1)]
    counted = (ordered.count("passed"), ordered.count("failed"), ordered.count("no-test"))
    if counted != (int(summary.group(3)), int(summary.group(4)), int(summary.group(5))):
        fail(label + " sums its runs as " + summary.group(3) + " passed, " + summary.group(4) + " failed and " + summary.group(5) + " ran no test, which its result lines contradict; the run is not recorded, report it to the owner")
    return ordered


def check_evidence_conclusions(run, folder, url, outcomes):
    wanted = "success" if all(outcome == "passed" for outcome in outcomes) else "failure"
    jobs = complete_list(read_json(folder, "jobs.json"), "jobs", "jobs.json of " + folder)
    evidence_jobs = [job for job in jobs if job.get("name") == "evidence"]
    if len(evidence_jobs) != 1:
        fail("the run " + url + " lists " + str(len(evidence_jobs)) + " evidence jobs instead of one")
    job = evidence_jobs[0]
    if job.get("status") != "completed" or not first_attempt(job.get("run_attempt")):
        fail("the evidence job of " + url + " is " + json.dumps(job.get("status")) + " at attempt " + json.dumps(job.get("run_attempt")) + ", not completed at attempt 1")
    for label, conclusion in (("the evidence run " + url, run.get("conclusion")), ("the evidence job of " + url, job.get("conclusion"))):
        if conclusion in ("cancelled", "timed_out"):
            fail(label + " concluded " + json.dumps(conclusion) + "; the run is not recorded, report it to the owner")
        if conclusion != wanted:
            fail(label + " concluded " + json.dumps(conclusion) + ", which contradicts the recorded outcomes " + ",".join(outcomes) + "; the run is not recorded, report it to the owner")
    print("report: evidence conclusions: run " + json.dumps(run.get("conclusion")) + ", evidence job " + json.dumps(job.get("conclusion")))


def evidence(o):
    folder = o["folder"]
    prefix = o["prefix"]
    run_text = o["run"]
    commit = o["commit"]
    series = o["series"]
    count = int(o["count"])
    sums_digest, listed = verify_sums(folder)
    fetched = fetch_record(folder)
    if fetched.get("run") != run_text or fetched.get("commit") != commit or fetched.get("logs") != "yes":
        fail("the folder " + folder + " was not fetched with --logs for the run " + run_text + " and the pending commit " + commit)
    run = read_json(folder, "run.json")
    if run.get("id") != int(run_text):
        fail("run.json of " + folder + " is not the run " + run_text)
    url = prefix + run_text
    print("report: run: " + url)
    check_identity(run, url, o, "gate evidence " + commit + " " + series, o["base"])
    if run.get("status") != "completed":
        fail("the evidence run " + url + " is " + json.dumps(run.get("status")) + ", not completed")
    if not first_attempt(run.get("run_attempt")):
        fail("the evidence run " + url + " is at attempt " + json.dumps(run.get("run_attempt")) + ", not 1: it was re-run")
    archive = "logs/run-" + run_text + ".zip"
    if archive not in listed:
        fail("the folder " + folder + " holds no logs archive of the run " + run_text)
    log_root = os.path.join(folder, "logs", "run-" + run_text)
    recorded = None
    log_files = 0
    for relative in sorted(folder_files(log_root)) if os.path.isdir(log_root) else []:
        parts = relative.split(os.sep)
        top = len(parts) == 1 and "evidence" in parts[0] and parts[0].endswith(".txt")
        nested = len(parts) > 1 and parts[0] == "evidence"
        if not top and not nested:
            continue
        log_files += 1
        with open(os.path.join(log_root, relative), encoding="utf-8", errors="replace") as handle:
            texts = [harness_text(line) for line in handle]
        found = harness_series(texts, series, count, commit, o["selection"], url)
        if found is None:
            continue
        if recorded is not None and recorded != found:
            fail("the evidence logs of " + url + " record different outcomes in different files; the run is not recorded, report it to the owner")
        recorded = found
    if log_files == 0:
        fail("the logs of " + url + " hold no log of the evidence job")
    outcomes = recorded if recorded is not None else ["no-test"] * count
    for number in range(1, count + 1):
        print("report: evidence run " + str(number) + " of " + str(count) + ": " + outcomes[number - 1] + ("" if recorded is not None else " (no harness summary line, so it ran no test)"))
    if recorded is not None:
        print("report: evidence selection: " + o["selection"])
    check_evidence_conclusions(run, folder, url, outcomes)
    reproduced = "yes" if "failed" in outcomes else "no"
    if "no-test" in outcomes:
        outcome = "no-test"
    elif series == "after":
        outcome = "failed" if reproduced == "yes" else "passed"
    else:
        outcome = "reproduced" if reproduced == "yes" else "not-reproduced"
    if o["stated"] != reproduced:
        fail("the coordinator states reproduced=" + o["stated"] + ", but the recorded lines of " + url + " give reproduced=" + reproduced + "; the run is not recorded, report it to the owner")
    print("value: outcome=" + outcome)
    print("value: reproduced=" + reproduced)
    print("value: logs_sha256=" + listed[archive])
    print("value: sums=" + sums_digest)


modes = {"verify": verify, "evidence": evidence, "selection": selection, "claimed": claimed, "pages": pages, "extract": extract, "sums": write_sums, "field": json_field}
if len(sys.argv) < 2 or sys.argv[1] not in modes:
    fail("internal error: unknown helper mode")
modes[sys.argv[1]](options(sys.argv[2:]))
' "$@"
}

utc_now() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

write_log_line() {
  prepare_log
  printf '%s\n' "$1" >>"$log_path"
}

helper_value() {
  printf '%s\n' "$1" | sed -n "s/^value: $2=//p"
}

print_helper_report() {
  printf '%s\n' "$1" | sed -n 's/^report: //p'
}

validate_change_id() {
  [[ "$1" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$ && "$1" != *..* && "$1" != *//* && "$1" != */ && "$1" != *.lock ]] ||
    fail "the change ID $1 is not a branch name of letters, digits, dots, hyphens, underscores and slashes"
}

derived_change_name() {
  local id="$1" name="$2" suffix="$3" rest
  [[ "$name" == "$id$suffix" ]] && return 0
  [[ "$name" == "$id-r"* ]] || return 1
  rest="${name#"$id"-r}"
  [[ "$rest" == *"$suffix" ]] || return 1
  rest="${rest%"$suffix"}"
  [[ "$rest" =~ ^[1-9][0-9]{0,3}$ ]]
}

branch_commit() {
  [[ "$1" != -* ]] || fail "$1 is not a branch name"
  has_control_character "$1" && fail "the branch name holds a control character"
  git check-ref-format --branch "$1" >/dev/null 2>&1 || fail "$1 is not a branch name"
  in_repo rev-parse --verify --quiet "refs/heads/$1^{commit}" || fail "$1 is not a local branch"
}

main_commit_now() {
  in_repo rev-parse --verify "$main_branch^{commit}"
}

origin_main_commit() {
  in_repo rev-parse --verify --quiet "refs/remotes/origin/$main_branch^{commit}" ||
    fail "the repository has no origin/$main_branch; fetch it first"
}

require_origin_main() {
  local origin
  origin="$(origin_main_commit)" || exit 1
  [[ "$origin" == "$1" ]] ||
    fail "origin/$main_branch (as last fetched) is $origin, not $main_branch's $1; push or fetch $main_branch first"
}

require_one_commit_on() {
  local commit="$1" parent="$2" label="$3" parents
  [[ "$commit" != "$parent" ]] || fail "$label holds no commit beyond $parent"
  parents="$(in_repo rev-list --parents -n 1 "$commit")" || fail "could not read the parents of $label"
  [[ "$parents" == "$commit $parent" ]] && return 0
  if in_repo merge-base --is-ancestor "$parent" "$commit"; then
    fail "$label holds more than one commit beyond $parent, or a merge commit; it must be exactly one commit whose only parent is $parent"
  fi
  fail "$label is not based on $parent; rebuild it as one commit on $parent under a new name"
}

patch_id_of() {
  local diff identifier
  diff="$(in_repo diff-tree -p --no-color --no-renames "$1^" "$1")" || fail "could not read the diff of $1"
  identifier="$(printf '%s\n' "$diff" | in_repo patch-id --stable | awk 'NR == 1 { print $1 }')" ||
    fail "could not compute the patch-id of $1"
  printf '%s' "${identifier:-none}"
}

write_claim_line() {
  write_log_line "claim: time=$(utc_now) change_id=$lock_change_id change=$change kind=$1 head=$head tree=$tree base=$base patch_id=$2 run=open"
}

open_claim_line_exists() {
  prepare_log
  awk -v head="$1" -v base="$2" '
    $0 ~ /^claim: / && index($0, " head=" head " ") && index($0, " base=" base " ") && $0 ~ / run=open$/ { found = 1 }
    END { exit found ? 0 : 1 }
  ' "$log_path"
}

end_claim_line() {
  local temporary
  prepare_log
  temporary="$(umask 077 && mktemp "$lock_dir/.merge-gate-log.XXXXXX")"
  if ! awk -v head="$1" -v base="$2" -v value="$3" -v ended="$4" '
    { lines[NR] = $0 }
    $0 ~ /^claim: / && index($0, " head=" head " ") && index($0, " base=" base " ") && $0 ~ / run=open$/ { last = NR }
    END {
      if (!last) exit 3
      for (n = 1; n <= NR; n++) {
        line = lines[n]
        if (n == last) sub(/ run=open$/, " run=" value " ended=" ended, line)
        print line
      }
    }
  ' "$log_path" >"$temporary"; then
    rm -f "$temporary"
    fail "the merge gate log $log_path holds no open claim of $1 on $2"
  fi
  chmod 600 "$temporary"
  mv -f "$temporary" "$log_path"
}

adopt_holder() {
  mode="$holder_mode"
  change="$holder_change"
  base="$holder_base"
  head="$holder_head"
  tree="$holder_tree"
  check_list="$holder_checks"
  scope="$holder_scope"
  recorded_lanes=""
  lock_change_id="$holder_change_id"
  lock_kind="$holder_kind"
  lock_pending="$holder_pending"
  lock_failed_run="$holder_failed_run"
  lock_before_tree="$holder_before_tree"
  lock_runs="$holder_runs"
  lock_reason="$holder_reason"
  lock_stalled_runs="$holder_stalled_runs"
  lock_selection="$holder_selection"
  lock_record="$holder_record"
  lock_run="$holder_run"
}

replace_lock() {
  local expected_pid="$holder_pid" expected_started="$holder_started" expected_state="$holder_state" temporary
  temporary="$(new_lock_file "$1")"
  if ! read_lock || [[ "$holder_pid" != "$expected_pid" || "$holder_started" != "$expected_started" || "$holder_state" != "$expected_state" ]]; then
    rm -f "$temporary"
    fail "$lock_path changed while this command rewrote it"
  fi
  mv -f "$temporary" "$lock_path"
}

read_held_lock() {
  read_lock || fail "no claim or evidence series holds $lock_path; $1"
  [[ "$holder_common" == "$common_dir" ]] || fail "$(holder_description) for another repository"
}

classify_scope() {
  if docs_only_change; then
    scope=docs-only
  else
    scope=full
  fi
}

print_claim_report() {
  printf 'merge gate claim\n'
  printf 'kind: %s\n' "$1"
  printf 'change ID: %s\n' "$lock_change_id"
  printf 'change: %s\n' "$change"
  printf 'candidate: %s\n' "$head"
  printf 'candidate tree: %s\n' "$tree"
  printf 'base: %s\n' "$base"
  printf 'scope: %s\n' "$scope"
  printf 'lock: %s, state %s\n' "$lock_path" "$2"
}

claim() {
  started_at="$(utc_now)"
  validate_change_id "$change_id_arg"
  case "$claim_form" in
    gate) claim_gate ;;
    catch-up) claim_catch_up ;;
    evidence) claim_evidence ;;
    after-fix) claim_after_fix ;;
    *) usage ;;
  esac
}

claim_gate() {
  local patch
  derived_change_name "$change_id_arg" "$change_arg" "" ||
    fail "the branch $change_arg is neither the change ID $change_id_arg nor a rebuild of it named $change_id_arg-r<k>"
  mode=ff-only
  change="$change_arg"
  base="$(main_commit_now)"
  head="$(branch_commit "$change")" || exit 1
  tree="$(in_repo rev-parse --verify "$head^{tree}")"
  require_one_commit_on "$head" "$base" "$change"
  require_origin_main "$base"
  classify_scope
  check_list=ci
  lock_change_id="$change_id_arg"
  lock_kind=gate
  patch="$(patch_id_of "$head")" || exit 1
  take_lock dispatched
  write_claim_line gate "$patch"
  print_claim_report gate dispatched
  printf 'patch-id: %s\n' "$patch"
  if [[ "$scope" == docs-only ]]; then
    printf 'next: push %s, dispatch gate.yml from %s with scope docs (or full) and sha %s, then run merge-gate.sh ci-fetch and merge-gate.sh ci\n' "$change" "$main_branch" "$head"
  else
    printf 'next: push %s, dispatch gate.yml from %s with scope full and sha %s, then run merge-gate.sh ci-fetch and merge-gate.sh ci\n' "$change" "$main_branch" "$head"
  fi
}

claim_catch_up() {
  local patch
  [[ -z "$change_arg" ]] || fail "claim --catch-up takes no --change; it claims the tip of local $main_branch"
  mode=ff-only
  head="$(main_commit_now)"
  base="$(origin_main_commit)" || exit 1
  [[ "$head" != "$base" ]] || fail "local $main_branch equals origin/$main_branch; there is no fallback merge to catch up"
  require_one_commit_on "$head" "$base" "local $main_branch"
  tree="$(in_repo rev-parse --verify "$head^{tree}")"
  change="catch-up/${head:0:12}"
  scope=full
  check_list=ci
  lock_change_id="$change_id_arg"
  lock_kind=catch-up
  patch="$(patch_id_of "$head")" || exit 1
  take_lock dispatched
  write_claim_line catch-up "$patch"
  print_claim_report catch-up dispatched
  printf 'next: push %s as the branch %s, dispatch gate.yml from %s with scope full and sha %s, then run merge-gate.sh ci-fetch and merge-gate.sh ci --catch-up\n' \
    "$head" "$change" "$main_branch" "$head"
}

evidence_series_entry() {
  local entry
  for entry in ${holder_runs//,/ }; do
    if [[ "${entry%%:*}" == "$1" ]]; then
      printf '%s' "$entry"
      return 0
    fi
  done
  return 1
}

evidence_series_outcome() {
  local entry
  entry="$(evidence_series_entry "$1")" || return 1
  printf '%s' "${entry##*:}"
}

evidence_runs_have_outcome() {
  local entry
  for entry in ${holder_runs//,/ }; do
    [[ "${entry##*:}" != "$1" ]] || return 0
  done
  return 1
}

evidence_series_count() {
  case "$evidence_series_runs" in
    *" $1:"*)
      local rest="${evidence_series_runs#* "$1":}"
      printf '%s' "${rest%% *}"
      ;;
    *) fail "the series $1 is not one of before-first, before-rest and after" ;;
  esac
}

refuse_second_evidence_series() {
  local counts
  prepare_log
  counts="$(awk -v url="$1" '
    $0 ~ /^evidence claim: / && index($0 " ", " failed_run=" url " ") { claims++ }
    $0 ~ /^evidence end: / && index($0 " ", " failed_run=" url " ") && index($0, " by=release-evidence ") { ended++ }
    END { printf "%d %d", claims, ended }
  ' "$log_path")"
  [[ "${counts% *}" == 0 || "${counts% *}" == "${counts#* }" ]] ||
    fail "the merge gate log already holds an evidence series for $1; a second series is allowed only after every earlier one recorded a run that ran no test and ended with release --evidence"
}

earlier_after_trees() {
  prepare_log
  awk -v url="$1" '
    $0 ~ /^evidence open: / && index($0 " ", " failed_run=" url " ") && index($0, " series=after ") {
      for (n = 1; n <= NF; n++) if ($n ~ /^tree=/) print substr($n, 6)
    }
  ' "$log_path"
}

failed_run_claim() {
  prepare_log
  awk -v run="$1" '
    $0 ~ /^claim: / && $0 ~ / ended=[a-z-]+$/ {
      for (n = 1; n <= NF; n++) {
        if ($n !~ /^run=/) continue
        count = split(substr($n, 5), ids, ",")
        for (i = 1; i <= count; i++) if (ids[i] == run) found = $0
      }
    }
    END {
      if (found == "") exit 1
      print found
    }
  ' "$log_path"
}

claim_line_field() {
  printf '%s\n' "$1" | tr ' ' '\n' | sed -n "s/^$2=//p" | head -n 1
}

claim_evidence() {
  local selection failed_claim failed_change_id failed_base failed_tree
  [[ "$failed_run_arg" =~ ^[0-9]{1,20}$ ]] || fail "--failed-run $failed_run_arg is not a run ID of 1 to 20 digits"
  derived_change_name "$change_id_arg" "$change_arg" -before ||
    fail "the before branch $change_arg is not named $change_id_arg-before or $change_id_arg-r<k>-before"
  selection="$(ci_python selection kind="$kind_arg" package="$package_arg" test="$test_arg" project="$project_arg" \
    title="$title_arg" entry="$entry_arg")" || exit 1
  mode=ff-only
  change="$change_arg"
  base="$(main_commit_now)"
  head="$(branch_commit "$change")" || exit 1
  tree="$(in_repo rev-parse --verify "$head^{tree}")"
  require_one_commit_on "$head" "$base" "$change"
  require_origin_main "$base"
  failed_claim="$(failed_run_claim "$failed_run_arg")" ||
    fail "the merge gate log holds no ended claim line naming the run $failed_run_arg; --failed-run names the failed run of a claimed gate"
  failed_change_id="$(claim_line_field "$failed_claim" change_id)"
  failed_base="$(claim_line_field "$failed_claim" base)"
  failed_tree="$(claim_line_field "$failed_claim" tree)"
  [[ "$failed_change_id" == "$change_id_arg" ]] ||
    fail "the run $failed_run_arg ended the claim of change $failed_change_id, not of $change_id_arg"
  [[ "$failed_base" != "$base" || "$failed_tree" == "$tree" ]] ||
    fail "the run $failed_run_arg tested the tree $failed_tree on the current $main_branch, not the before tree $tree"
  refuse_second_evidence_series "$ci_run_url_prefix$failed_run_arg"
  check_list=evidence
  scope=evidence
  lock_change_id="$change_id_arg"
  lock_failed_run="$ci_run_url_prefix$failed_run_arg"
  lock_before_tree="$tree"
  lock_selection="$selection"
  take_lock evidence
  write_log_line "evidence claim: time=$(utc_now) change_id=$lock_change_id change=$change failed_run=$lock_failed_run before_tree=$tree base=$base head=$head selection=$selection"
  print_claim_report evidence evidence
  printf 'failed run: %s\n' "$lock_failed_run"
  printf 'selection: %s\n' "$selection"
  printf 'next: run merge-gate.sh series-open --series before-first --commit %s, then dispatch the before-first evidence run\n' "$head"
}

claim_after_fix() {
  local after_entry after_commit paths path patch evidence_runs
  read_held_lock "claim --after-fix continues an open evidence series"
  [[ "$holder_state" == evidence ]] || fail "$(holder_description); claim --after-fix continues only an open evidence series"
  [[ "$holder_change_id" == "$change_id_arg" ]] || fail "the evidence series belongs to change $holder_change_id, not to $change_id_arg"
  [[ -z "$holder_pending" ]] || fail "a dispatch of the series is pending ($holder_pending); record it with evidence-record first"
  evidence_runs_have_outcome no-test && fail "a run of the series ran no test, so it admits no after-fix gate; end it with release --evidence"
  [[ "$(evidence_series_outcome after || true)" == passed ]] ||
    fail "the after series of change $holder_change_id is not complete and passing (recorded: ${holder_runs:-no run})"
  derived_change_name "$change_id_arg" "$change_arg" "" ||
    fail "the branch $change_arg is neither the change ID $change_id_arg nor a rebuild of it named $change_id_arg-r<k>"
  evidence_runs="$holder_runs"
  after_entry="$(evidence_series_entry after)"
  after_commit="$(printf '%s' "$after_entry" | cut -d: -f3)"
  head="$(branch_commit "$change_arg")" || exit 1
  base="$holder_base"
  [[ "$(main_commit_now)" == "$base" ]] || fail "$main_branch moved from the series base $base during the evidence series"
  require_origin_main "$base"
  require_one_commit_on "$head" "$base" "$change_arg"
  paths="$(in_repo diff --name-only --no-renames "$after_commit" "$head")" || fail "could not compare $change_arg with the after commit $after_commit"
  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    [[ "$path" =~ $known_limitations_pattern ]] ||
      fail "the candidate's tree differs from the after tree beyond the known-limitations rows, in $path"
  done <<<"$paths"
  tree="$(in_repo rev-parse --verify "$head^{tree}")"
  mode=ff-only
  change="$change_arg"
  classify_scope
  check_list=ci
  patch="$(patch_id_of "$head")" || exit 1
  lock_change_id="$holder_change_id"
  lock_kind=after-fix
  lock_failed_run="$holder_failed_run"
  lock_before_tree=""
  lock_runs=""
  lock_pending=""
  lock_reason=""
  lock_stalled_runs=""
  lock_selection=""
  lock_record=""
  lock_run=""
  replace_lock dispatched
  write_log_line "evidence end: time=$(utc_now) change_id=$lock_change_id failed_run=$lock_failed_run by=claim-after-fix runs=$evidence_runs after_commit=$after_commit"
  write_claim_line after-fix "$patch"
  print_claim_report after-fix dispatched
  printf 'after commit: %s\n' "$after_commit"
  printf 'next: push %s, dispatch gate.yml from %s with scope %s and sha %s, then run merge-gate.sh ci-fetch and merge-gate.sh ci\n' \
    "$change" "$main_branch" "${scope%-only}" "$head"
}

series_open() {
  local commit_tree earlier
  read_held_lock "series-open needs an open evidence series (claim --evidence)"
  [[ "$holder_state" == evidence ]] || fail "$(holder_description); series-open needs an open evidence series (claim --evidence)"
  [[ -z "$holder_pending" ]] ||
    fail "a dispatch of the series is already pending ($holder_pending); record it with evidence-record before opening another"
  evidence_series_count "$series_arg" >/dev/null
  [[ "$commit_arg" =~ ^[0-9a-f]{40}$ ]] || fail "--commit $commit_arg is not a full lower-case commit hash"
  [[ "$(main_commit_now)" == "$holder_base" ]] || fail "$main_branch moved from the series base $holder_base during the evidence series"
  require_origin_main "$holder_base"
  in_repo cat-file -e "$commit_arg^{commit}" 2>/dev/null || fail "$commit_arg is not a commit of this repository"
  require_one_commit_on "$commit_arg" "$holder_base" "the $series_arg commit $commit_arg"
  commit_tree="$(in_repo rev-parse --verify "$commit_arg^{tree}")"
  evidence_runs_have_outcome no-test && fail "a run of this series ran no test; end it with release --evidence"
  case "$series_arg" in
    before-first)
      [[ -z "$holder_runs" ]] || fail "the before-first run of this series is already recorded"
      [[ "$commit_tree" == "$holder_before_tree" ]] || fail "the before-first commit's tree $commit_tree is not the series' before tree $holder_before_tree"
      ;;
    before-rest)
      [[ "$(evidence_series_outcome before-first || true)" == not-reproduced ]] ||
        fail "before-rest is allowed only after a recorded before-first run that did not reproduce (recorded: ${holder_runs:-no run})"
      evidence_series_entry before-rest >/dev/null && fail "the before-rest runs of this series are already recorded"
      [[ "$commit_tree" == "$holder_before_tree" ]] || fail "the before-rest commit's tree $commit_tree is not the series' before tree $holder_before_tree"
      ;;
    after)
      if [[ "$(evidence_series_outcome before-first || true)" != reproduced ]] && ! evidence_series_entry before-rest >/dev/null; then
        fail "the after series starts only once the before series is complete (recorded: ${holder_runs:-no run})"
      fi
      evidence_series_entry after >/dev/null && fail "the after runs of this series are already recorded"
      [[ "$commit_tree" != "$holder_before_tree" ]] || fail "the after commit's tree equals the before tree; it holds no fix"
      while IFS= read -r earlier; do
        [[ -z "$earlier" || "$earlier" != "$commit_tree" ]] ||
          fail "the after tree $commit_tree was already used by an earlier series for $holder_failed_run; only a further change of the diff admits a new series"
      done <<<"$(earlier_after_trees "$holder_failed_run")"
      ;;
  esac
  adopt_holder
  started_at="$(utc_now)"
  lock_pending="$series_arg,$commit_arg"
  replace_lock evidence
  write_log_line "evidence open: time=$(utc_now) change_id=$lock_change_id failed_run=$lock_failed_run series=$series_arg commit=$commit_arg tree=$commit_tree selection=$lock_selection"
  printf 'merge gate series-open\n'
  printf 'change ID: %s\n' "$lock_change_id"
  printf 'series: %s (%s runs)\n' "$series_arg" "$(evidence_series_count "$series_arg")"
  printf 'commit: %s\n' "$commit_arg"
  printf 'pending: %s\n' "$lock_pending"
  printf 'selection: %s\n' "$lock_selection"
  printf 'next: push the commit, dispatch exactly one gate.yml evidence run titled gate evidence %s %s with the inputs of this selection, then run merge-gate.sh ci-fetch --logs and merge-gate.sh evidence-record\n' \
    "$commit_arg" "$series_arg"
}

evidence_record() {
  local series pending_commit count output outcome reproduced logs_sha
  read_held_lock "evidence-record needs an open evidence series"
  [[ "$holder_state" == evidence ]] || fail "$(holder_description); evidence-record needs an open evidence series"
  [[ -n "$holder_pending" ]] || fail "no evidence dispatch is pending; open one with series-open first"
  [[ "$run_arg" =~ ^[0-9]{1,20}$ ]] || fail "--run $run_arg is not a run ID of 1 to 20 digits"
  case "$reproduced_arg" in
    yes | no) ;;
    *) usage ;;
  esac
  series="${holder_pending%%,*}"
  pending_commit="${holder_pending#*,}"
  count="$(evidence_series_count "$series")" || exit 1
  [[ "$pending_commit" =~ ^[0-9a-f]{40}$ ]] || fail "$lock_path holds the malformed pending dispatch $holder_pending"
  [[ -n "$holder_selection" ]] || fail "$lock_path names no evidence selection; open the series again with claim --evidence"
  output="$(ci_python evidence folder="$runs_arg" run="$run_arg" commit="$pending_commit" series="$series" count="$count" \
    base="$holder_base" stated="$reproduced_arg" selection="$holder_selection" repository="$ci_repository" workflow="$ci_workflow_path" owner="$ci_owner_login" \
    prefix="$ci_run_url_prefix")" || exit 1
  outcome="$(helper_value "$output" outcome)"
  reproduced="$(helper_value "$output" reproduced)"
  logs_sha="$(helper_value "$output" logs_sha256)"
  adopt_holder
  started_at="$(utc_now)"
  lock_pending=""
  lock_runs="${lock_runs:+$lock_runs,}$series:$run_arg:$pending_commit:$outcome"
  replace_lock evidence
  write_log_line "evidence record: time=$(utc_now) change_id=$lock_change_id failed_run=$lock_failed_run series=$series run=$run_arg commit=$pending_commit outcome=$outcome reproduced=$reproduced stated=$reproduced_arg logs_sha256=$logs_sha selection=$lock_selection"
  printf 'merge gate evidence-record\n'
  print_helper_report "$output"
  printf 'series: %s\n' "$series"
  printf 'outcome: %s\n' "$outcome"
  printf 'reproduced: %s (stated %s)\n' "$reproduced" "$reproduced_arg"
  printf 'logs archive SHA-256: %s\n' "$logs_sha"
  printf 'recorded runs: %s\n' "$lock_runs"
}

fallback_from_claim() {
  read_held_lock "fallback --from-claim moves a claimed dispatch to a local gate"
  [[ "$holder_state" == dispatched ]] || fail "$(holder_description); fallback --from-claim moves only a claimed dispatch"
  case "$holder_kind" in
    gate | after-fix) ;;
    *) fail "a $holder_kind claim does not fall back to a local gate; release it with release --run or --no-run" ;;
  esac
  [[ -n "$reason_arg" && ${#reason_arg} -le 300 ]] || fail "--reason must quote the GitHub-side unavailability in 1 to 300 characters"
  has_control_character "$reason_arg" && fail "--reason holds a control character"
  [[ "$reason_arg" != *=* ]] || fail "--reason holds an equals sign, which the merge gate log reads as a field; quote the unavailability without it"
  [[ -z "$stalled_arg" || "$stalled_arg" =~ ^[0-9]{1,20}(,[0-9]{1,20})*$ ]] || fail "--stalled-runs is not a comma-separated list of run IDs"
  open_claim_line_exists "$holder_head" "$holder_base" || fail "the merge gate log $log_path holds no open claim of $holder_head on $holder_base"
  adopt_holder
  started_at="$(utc_now)"
  lock_reason="$reason_arg"
  lock_stalled_runs="$stalled_arg"
  replace_lock fallback
  end_claim_line "$head" "$base" "${stalled_arg:-none}" fallback
  write_log_line "fallback: time=$(utc_now) change_id=$lock_change_id change=$change head=$head tree=$tree base=$base stalled_runs=${stalled_arg:-none} reason=$reason_arg"
  printf 'merge gate fallback\n'
  printf 'change ID: %s\n' "$lock_change_id"
  printf 'change: %s\n' "$change"
  printf 'candidate: %s\n' "$head"
  printf 'stalled runs: %s\n' "${stalled_arg:-none}"
  printf 'reason: %s\n' "$reason_arg"
  printf 'next: with the approval of the owner recorded in a fallback row naming every stalled run, run merge-gate.sh gate --ff-only --change %s --change-id %s in the checkout of the change\n' \
    "$change" "$lock_change_id"
}

ci_record() {
  local main_now origin scopes expected output run_id execution queue job_queue sums attempts title_scope record label
  read_held_lock "run merge-gate.sh claim before dispatching a full or docs run"
  case "$holder_state" in
    dispatched) ;;
    evidence) fail_held_lock ;;
    *) fail "$(holder_description); ci continues only from a claimed dispatch" ;;
  esac
  main_now="$(main_commit_now)"
  if [[ "$catch_up" == yes ]]; then
    [[ "$holder_kind" == catch-up ]] || fail "the claim is of kind $holder_kind; ci --catch-up continues only a claim --catch-up"
    [[ -z "$change_arg" ]] || fail "ci --catch-up takes no --change"
    head="$main_now"
    base="$(origin_main_commit)" || exit 1
    label="local $main_branch"
    record=ci-catch-up
  else
    [[ "$holder_kind" == gate || "$holder_kind" == after-fix ]] ||
      fail "the claim is of kind $holder_kind; continue it with ci --catch-up"
    [[ "$change_arg" == "$holder_change" ]] || fail "the claim is for $holder_change, not for $change_arg"
    head="$(branch_commit "$change_arg")" || exit 1
    base="$main_now"
    label="$change_arg"
    record=ci
  fi
  tree="$(in_repo rev-parse --verify "$head^{tree}")"
  [[ "$head" == "$holder_head" && "$tree" == "$holder_tree" && "$base" == "$holder_base" ]] ||
    fail "the claim names head $holder_head, tree $holder_tree and base $holder_base, not head $head, tree $tree and base $base"
  require_one_commit_on "$head" "$base" "$label"
  if [[ "$catch_up" == yes ]]; then
    scope=full
    scopes=full
  else
    require_origin_main "$base"
    mode=ff-only
    classify_scope
    if [[ "$scope" == docs-only ]]; then
      scopes=docs,full
    else
      scopes=full
    fi
  fi
  [[ -n "$runs_arg" ]] || usage
  open_claim_line_exists "$head" "$base" || fail "the merge gate log $log_path holds no open claim of $head on $base"
  expected="$(ci_expected_jobs full | sed 's/^/full|/')"$'\n'"$(ci_expected_jobs docs | sed 's/^/docs|/')"
  output="$(printf '%s\n' "$expected" | ci_python verify folder="$runs_arg" root="$root" candidate="$head" base="$base" tree="$tree" \
    scopes="$scopes" log="$log_path" repository="$ci_repository" workflow="$ci_workflow_path" owner="$ci_owner_login" \
    prefix="$ci_run_url_prefix")" || exit 1
  run_id="$(helper_value "$output" run_id)"
  title_scope="$(helper_value "$output" title_scope)"
  execution="$(helper_value "$output" execution)"
  queue="$(helper_value "$output" queue)"
  job_queue="$(helper_value "$output" job_queue)"
  sums="$(helper_value "$output" sums)"
  attempts="$(helper_value "$output" attempts)"
  adopt_holder
  started_at="$(utc_now)"
  lock_record="$record"
  lock_run="$run_id"
  check_list=ci
  replace_lock passed
  end_claim_line "$head" "$base" "$run_id" "$record"
  write_log_line "ci: time=$(utc_now) type=$record change_id=$lock_change_id change=$change head=$head tree=$tree base=$base scope=$title_scope run=$run_id execution=$execution queue=$queue job_queue=$job_queue sums=$sums attempts=$attempts"
  if [[ "$record" == ci-catch-up ]]; then
    finish_log_line ci-catch-up "$head" "$tree"
  fi
  printf 'merge gate ci\n'
  printf 'record: %s\n' "$record"
  printf 'change ID: %s\n' "$lock_change_id"
  printf 'change: %s\n' "$change"
  printf 'candidate: %s\n' "$head"
  printf 'base: %s\n' "$base"
  printf 'tested tree: %s\n' "$tree"
  print_helper_report "$output"
  printf 'scope: %s (candidate classified %s)\n' "$title_scope" "$scope"
  printf 'execution time: %s s\n' "$execution"
  printf 'queue time: %s s (longest job queue %s s)\n' "$queue" "$job_queue"
  printf 'fetched files SHA256SUMS: %s\n' "$sums"
  printf 'counted earlier attempts: %s\n' "$attempts"
  printf 'result: passed record=%s\n' "$record"
  if [[ "$record" == ci-catch-up ]]; then
    printf 'finish: record=ci-catch-up head=%s tree=%s result=passed\n' "$head" "$tree"
    printf 'next: push %s to origin, then run merge-gate.sh release\n' "$main_branch"
  else
    printf 'next: in the checkout of %s run git merge --ff-only %s, then merge-gate.sh finish\n' "$main_branch" "$change"
  fi
}

ci_gh_get() {
  "$gh_path" api --hostname "$ci_host" --method GET "repos/$ci_repository/$1" </dev/null >"$2" ||
    fail "gh api GET repos/$ci_repository/$1 failed"
  chmod 600 "$2"
}

ci_gh_pages() {
  local endpoint="$1" key="$2" out="$3" stem="$4" page=1 files="" state
  mkdir -p "$(dirname "$stem")"
  while [[ "$page" -le "$ci_page_limit" ]]; do
    ci_gh_get "$endpoint&page=$page" "$stem-$page.json"
    files="$files${files:+:}$stem-$page.json"
    state="$(ci_python pages key="$key" out="$out" pages="$files")" || exit 1
    case "$state" in
      complete) return 0 ;;
      short*) fail "GET repos/$ci_repository/$endpoint returned ${state#short } entries instead of total_count, so the list is not completely read" ;;
    esac
    page=$((page + 1))
  done
  fail "GET repos/$ci_repository/$endpoint needs more than $ci_page_limit pages"
}

ci_fetch_main() {
  local names name refused="" kind value claimed_lines head_sha listing
  run_arg=""
  commit_arg=""
  out_arg=""
  fetch_logs=no
  lock_dir=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --logs) fetch_logs=yes ;;
      --run | --commit | --out | --lock-dir)
        [[ $# -ge 2 && -n "$2" ]] || usage
        case "$1" in
          --run) run_arg="$2" ;;
          --commit) commit_arg="$2" ;;
          --out) out_arg="$2" ;;
          --lock-dir) lock_dir="$2" ;;
        esac
        shift
        ;;
      *) usage ;;
    esac
    shift
  done
  names="$(exported_names)" || fail "could not list the names of the environment"
  for name in "${ci_fetch_refused_variables[@]}"; do
    if exported "$names" "$name"; then
      refused="$refused${refused:+ }$name"
    fi
  done
  [[ -z "$refused" ]] ||
    fail "$refused set in the environment would change which host, account, token or configuration gh uses; unset it for ci-fetch only, for example with env -u ${refused%% *}"
  [[ "$run_arg" =~ ^[0-9]{1,20}$ ]] || fail "--run ${run_arg:-(missing)} is not a run ID of 1 to 20 digits"
  [[ "$commit_arg" =~ ^[0-9a-f]{40}$ ]] || fail "--commit ${commit_arg:-(missing)} is not a full lower-case commit hash of 40 characters"
  [[ -n "$out_arg" ]] || usage
  has_control_character "$out_arg" && fail "the output folder path holds a control character"
  [[ ! -L "$out_arg" ]] || fail "the output folder $out_arg is a link"
  if [[ -e "$out_arg" ]]; then
    [[ -d "$out_arg" ]] || fail "the output folder $out_arg is not a folder"
    [[ -z "$(ls -A "$out_arg")" ]] || fail "the output folder $out_arg is not empty"
  fi
  [[ -n "$lock_dir" ]] || lock_dir="$(default_lock_dir)"
  log_path="$lock_dir/$log_name"
  [[ ! -L "$log_path" ]] || fail "$log_path is a link, not the merge gate log"
  gh_path="$(command -v gh)" || fail "gh is not on the path"
  [[ "$gh_path" == /* && -x "$gh_path" ]] || fail "gh resolves to $gh_path, not to an executable file"
  if [[ ! -e "$out_arg" ]]; then
    (umask 077 && mkdir "$out_arg") || fail "could not create the output folder $out_arg"
  fi
  chmod 700 "$out_arg"
  out_arg="$(cd -P "$out_arg" && pwd)"
  ci_gh_get "actions/runs/$run_arg" "$out_arg/run.json"
  head_sha="$(ci_python field file="$out_arg/run.json" path=head_sha)" || exit 1
  [[ "$head_sha" =~ ^[0-9a-f]{40}$ ]] || fail "the run $run_arg has the malformed head_sha $head_sha"
  ci_gh_pages "actions/runs/$run_arg/jobs?filter=all&per_page=100" jobs "$out_arg/jobs.json" "$out_arg/pages/jobs"
  ci_gh_pages "actions/workflows/$ci_workflow_file/runs?event=workflow_dispatch&head_sha=$head_sha&per_page=100" workflow_runs \
    "$out_arg/history.json" "$out_arg/pages/history"
  claimed_lines="$(ci_python claimed log="$log_path" candidate="$commit_arg" base="$head_sha")" || exit 1
  while read -r kind value; do
    case "$kind" in
      run)
        mkdir -p "$out_arg/claims"
        ci_gh_get "actions/runs/$value" "$out_arg/claims/run-$value.json"
        ;;
      base)
        mkdir -p "$out_arg/bases"
        ci_gh_pages "actions/workflows/$ci_workflow_file/runs?event=workflow_dispatch&head_sha=$value&per_page=100" workflow_runs \
          "$out_arg/bases/$value.json" "$out_arg/pages/base-$value"
        ;;
      "") ;;
      *) fail "internal error: the claim lookup printed $kind" ;;
    esac
  done <<<"$claimed_lines"
  if [[ "$fetch_logs" == yes ]]; then
    mkdir -p "$out_arg/logs"
    ci_gh_get "actions/runs/$run_arg/logs" "$out_arg/logs/run-$run_arg.zip"
    ci_python extract zip="$out_arg/logs/run-$run_arg.zip" destination="$out_arg/logs/run-$run_arg" || exit 1
  fi
  printf 'run=%s\ncommit=%s\nhead_sha=%s\nlogs=%s\ntime=%s\n' "$run_arg" "$commit_arg" "$head_sha" "$fetch_logs" "$(utc_now)" >"$out_arg/fetch.txt"
  chmod 600 "$out_arg/fetch.txt"
  listing="$(ci_python sums folder="$out_arg")" || exit 1
  printf 'merge gate ci-fetch\n'
  printf 'run: %s%s\n' "$ci_run_url_prefix" "$run_arg"
  printf 'commit: %s\n' "$commit_arg"
  printf 'head_sha: %s\n' "$head_sha"
  printf 'folder: %s\n' "$out_arg"
  printf 'earlier claimed runs and bases: %s\n' "$(printf '%s' "$claimed_lines" | tr '\n' ' ')"
  printf 'files and SHA-256:\n%s\n' "$listing"
  printf 'SHA256SUMS: %s\n' "$(shasum_of_sums "$out_arg")"
}

shasum_of_sums() {
  python3 -I -c '
import hashlib
import sys

with open(sys.argv[1], "rb") as handle:
    print(hashlib.sha256(handle.read()).hexdigest())
' "$1/SHA256SUMS"
}

ci_main() {
  local subcommand="$1"
  shift
  ci_job=""
  scope_arg=""
  shard_arg=""
  repo_arg=""
  base_arg=""
  packages_arg=""
  jobs_arg=""
  entry_arg=""
  if [[ "$subcommand" == run-job ]]; then
    [[ $# -ge 1 && -n "$1" && "$1" != -* ]] || usage
    ci_job="$1"
    shift
  fi
  while [[ $# -gt 0 ]]; do
    [[ $# -ge 2 && -n "$2" ]] || usage
    case "$subcommand|$1" in
      list\|--scope | ci-matrix\|--scope) scope_arg="$2" ;;
      list\|--job) ci_job="$2" ;;
      ci-matrix\|--jobs) jobs_arg="$2" ;;
      run-job\|--shard) shard_arg="$2" ;;
      run-job\|--packages) packages_arg="$2" ;;
      run-job\|--entry) entry_arg="$2" ;;
      run-job\|--repo | classify\|--repo) repo_arg="$2" ;;
      run-job\|--base | classify\|--base) base_arg="$2" ;;
      *) usage ;;
    esac
    shift 2
  done
  case "$subcommand" in
    list) list_entries ;;
    ci-matrix) ci_matrix ;;
    ci-jobs) ci_jobs ;;
    run-job)
      [[ -n "$repo_arg" ]] || usage
      run_job
      ;;
    classify)
      [[ -n "$repo_arg" && -n "$base_arg" ]] || usage
      classify
      ;;
  esac
}

allow_options() {
  local option
  for option in $given_options; do
    [[ " $1 " == *" $option "* ]] || usage
  done
}

require_options() {
  local option
  for option in $1; do
    [[ " $given_options " == *" $option "* ]] || usage
  done
}

main() {
  refuse_shell_environment
  [[ $# -ge 1 ]] || usage
  subcommand="$1"
  shift
  case "$subcommand" in
    list | run-job | classify | ci-matrix | ci-jobs)
      ci_main "$subcommand" "$@"
      return 0
      ;;
    ci-fetch)
      ci_fetch_main "$@"
      return 0
      ;;
  esac
  mode=""
  repo_arg=""
  lock_dir=""
  lock_dir_given=no
  change_arg=""
  change_id_arg=""
  requested_checks=full
  lane_grace=""
  given_options=""
  catch_up=no
  claim_form=""
  failed_run_arg=""
  runs_arg=""
  run_arg=""
  release_form=""
  release_run_arg=""
  series_arg=""
  commit_arg=""
  reproduced_arg=""
  reason_arg=""
  stalled_arg=""
  kind_arg=""
  package_arg=""
  test_arg=""
  project_arg=""
  title_arg=""
  entry_arg=""
  while [[ $# -gt 0 ]]; do
    given_options="$given_options $1"
    case "$1" in
      --ff-only) mode=ff-only ;;
      --squash) mode=squash ;;
      --catch-up) catch_up=yes ;;
      --evidence | --after-fix | --no-run | --from-claim) ;;
      --skip-tool-probes) tool_probes=no ;;
      --skip-power-log) power_log=no ;;
      --repo | --lock-dir | --change | --change-id | --checks | --lane-grace | --failed-run | --runs | --run | --series | --commit | --reproduced | --reason | --stalled-runs | --kind | --package | --test | --project | --title | --entry)
        [[ $# -ge 2 ]] || usage
        case "$1" in
          --repo) repo_arg="$2" ;;
          --lock-dir)
            lock_dir="$2"
            lock_dir_given=yes
            ;;
          --change)
            [[ -n "$2" ]] || usage
            change_arg="$2"
            ;;
          --change-id)
            [[ -n "$2" ]] || usage
            change_id_arg="$2"
            ;;
          --checks)
            case "$2" in
              full | self-test | self-test-lanes) requested_checks="$2" ;;
              *) usage ;;
            esac
            ;;
          --lane-grace)
            [[ "$2" =~ ^[1-9][0-9]?$ ]] || usage
            lane_grace="$2"
            ;;
          --failed-run) failed_run_arg="$2" ;;
          --runs) runs_arg="$2" ;;
          --run) run_arg="$2" ;;
          --series) series_arg="$2" ;;
          --commit) commit_arg="$2" ;;
          --reproduced) reproduced_arg="$2" ;;
          --reason) reason_arg="$2" ;;
          --stalled-runs) stalled_arg="$2" ;;
          --kind) kind_arg="$2" ;;
          --package) package_arg="$2" ;;
          --test) test_arg="$2" ;;
          --project) project_arg="$2" ;;
          --title) title_arg="$2" ;;
          --entry) entry_arg="$2" ;;
        esac
        shift
        ;;
      *) usage ;;
    esac
    shift
  done

  case "$subcommand" in
    gate)
      allow_options "--ff-only --squash --repo --lock-dir --change --change-id --checks --lane-grace --skip-tool-probes --skip-power-log"
      [[ -n "$mode" ]] || usage
      ;;
    finish) allow_options "--repo --lock-dir" ;;
    release)
      allow_options "--repo --lock-dir --run --no-run --evidence"
      case " $given_options " in
        *" --run "*" --no-run "* | *" --no-run "*" --run "* | *" --run "*" --evidence "* | *" --evidence "*" --run "* | *" --no-run "*" --evidence "* | *" --evidence "*" --no-run "*) usage ;;
        *" --run "*)
          [[ "$run_arg" =~ ^[0-9]{1,20}$ ]] || fail "--run $run_arg is not a run ID of 1 to 20 digits"
          release_form=run
          release_run_arg="$run_arg"
          ;;
        *" --no-run "*) release_form=no-run ;;
        *" --evidence "*) release_form=evidence ;;
      esac
      ;;
    claim)
      case " $given_options " in
        *" --catch-up "*)
          allow_options "--catch-up --change-id --repo --lock-dir"
          claim_form=catch-up
          ;;
        *" --evidence "*)
          allow_options "--evidence --change --change-id --failed-run --kind --package --test --project --title --entry --repo --lock-dir"
          require_options "--change --failed-run --kind"
          claim_form=evidence
          ;;
        *" --after-fix "*)
          allow_options "--after-fix --change --change-id --repo --lock-dir"
          require_options "--change"
          claim_form=after-fix
          ;;
        *)
          allow_options "--ff-only --change --change-id --repo --lock-dir"
          require_options "--ff-only --change"
          claim_form=gate
          ;;
      esac
      require_options "--change-id"
      ;;
    ci)
      if [[ "$catch_up" == yes ]]; then
        allow_options "--catch-up --runs --repo --lock-dir"
      else
        allow_options "--ff-only --change --runs --repo --lock-dir"
        require_options "--ff-only --change"
      fi
      require_options "--runs"
      ;;
    fallback)
      allow_options "--from-claim --reason --stalled-runs --repo --lock-dir"
      require_options "--from-claim --reason"
      ;;
    series-open)
      allow_options "--series --commit --repo --lock-dir"
      require_options "--series --commit"
      ;;
    evidence-record)
      allow_options "--run --runs --reproduced --repo --lock-dir"
      require_options "--run --runs --reproduced"
      ;;
    *) usage ;;
  esac

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
    gate) gate ;;
    finish) finish ;;
    release) release ;;
    claim) claim ;;
    ci) ci_record ;;
    fallback) fallback_from_claim ;;
    series-open) series_open ;;
    evidence-record) evidence_record ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
