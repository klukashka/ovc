#!/usr/bin/env bash
set -euo pipefail
work="$(cd "$(mktemp -d "${TMPDIR:-/tmp}/merge-gate-check.XXXXXX")" && pwd -P)"
trap 'rm -rf "$work"' EXIT
source_gate="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/merge-gate.sh"
own_repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$work/gate"
cp "$source_gate" "$work/gate/merge-gate.sh"
gate_script="$work/gate/merge-gate.sh"
repo="$work/repo"
lock_dir="$work/lock"
lock="$lock_dir/merge-gate.lock"
log="$lock_dir/merge-gate.log"

check() {
  if [[ "$1" != "$2" ]]; then
    echo "FAIL: $3: got [$1], want [$2]"
    printf 'last gate output:\n%s\n' "${output:-}"
    exit 1
  fi
  echo "ok: $3"
}

contains() {
  if [[ "$1" != *"$2"* ]]; then
    echo "FAIL: $3: [$1] does not contain [$2]"
    exit 1
  fi
  echo "ok: $3"
}

in_repo() {
  git -C "$repo" "$@"
}

commit_file() {
  printf '%s\n' "$2" >"$repo/$1"
  in_repo add -- "$1"
  in_repo commit --quiet -m "$3"
}

run_gate() {
  set +e
  output="$(bash "$gate_script" "$@" --repo "$repo" --lock-dir "$lock_dir" 2>&1)"
  status=$?
  set -e
}

run_unchecked() {
  set +e
  output="$("$@" 2>&1)"
  status=$?
  set -e
}

lock_state() {
  if [[ -e "$lock" ]]; then
    echo held
  else
    echo released
  fi
}

file_mode() {
  if stat -f '%Lp' "$1" >/dev/null 2>&1; then
    stat -f '%Lp' "$1"
  else
    stat -c '%a' "$1"
  fi
}

account_lock_dir() {
  case "$(uname -s)" in
    Darwin) printf '%s/Library/Caches/ovc-e2e' "$(dscl . -read "/Users/$(id -un)" NFSHomeDirectory | sed -n 's/^NFSHomeDirectory: //p')" ;;
    *) printf '%s/.cache/ovc-e2e' "$(getent passwd "$(id -u)" | cut -d: -f6)" ;;
  esac
}

record_lock_as_full_run() {
  sed 's/^checks=self-test$/checks=full/' "$lock" >"$lock_dir/rewritten"
  chmod 600 "$lock_dir/rewritten"
  mv "$lock_dir/rewritten" "$lock"
}

start_change() {
  in_repo switch --quiet main
  in_repo switch --quiet -c "$1"
}

mkdir -p "$repo"
in_repo init --quiet -b main
in_repo config user.name "Merge gate check"
in_repo config user.email "merge-gate-check@example.invalid"
in_repo config commit.gpgsign false
mkdir -p "$repo/docs" "$repo/tools" "$repo/knowledge/architecture"
commit_file docs/guide.md "first" "Add the guide"
commit_file tools/reader.sh "cat docs/listed.md" "Add a reader"
commit_file .gitignore "go.work" "Ignore the workspace file"
start_change docs/guide-wording
commit_file docs/guide.md "second" "Reword the guide"

run_unchecked bash "$gate_script" gate --ff-only --checks self-test --repo "$repo"
check "$status" "1" "the self-test list is refused without an explicit lock folder"
contains "$output" "explicit --lock-dir" "the refusal names the missing lock folder"
run_unchecked bash "$gate_script" gate --ff-only --checks self-test --repo "$repo" --lock-dir "$(account_lock_dir)"
check "$status" "1" "the self-test list is refused in the default lock folder"
contains "$output" "refuses the default lock folder" "the refusal names the default lock folder"
run_unchecked bash "$source_gate" gate --ff-only --checks self-test --repo "$own_repo" --lock-dir "$lock_dir"
check "$status" "1" "the self-test list is refused on the gate script's own repository"
contains "$output" "refuses the repository the gate script belongs to" "the refusal names the script's repository"
check "$(lock_state)" "released" "a refused self-test takes no lock"

run_gate gate --ff-only --checks self-test
check "$status" "0" "a clean fast-forward gate passes"
contains "$output" "scope: docs-only" "a Markdown-only change under docs/ is docs-only"
contains "$output" "check list: self-test" "the report names the self-test check list"
contains "$output" "tested tree: $(in_repo rev-parse 'HEAD^{tree}')" "the report records the tested tree"
contains "$output" "base main: $(in_repo rev-parse main)" "the report records the base commit"
contains "$output" "exit 0: .: git rev-parse --verify HEAD" "the report lists each command with its exit code"
contains "$(cat "$lock")" "checks=self-test" "the lock records the check list"
contains "$(cat "$lock")" "scope=docs-only" "the lock records the scope"
check "$(file_mode "$lock_dir")|$(file_mode "$lock")|$(file_mode "$log")" "700|600|600" "the lock folder, lock and log are readable only by the owner"

held="$(cat "$lock")"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a second gate is refused while the lock is held"
contains "$output" "a merge gate from $repo" "the refusal names the holding checkout"
check "$(cat "$lock")" "$held" "a refused gate leaves the holder's lock alone"

in_repo switch --quiet main
in_repo merge --quiet --ff-only docs/guide-wording
run_gate finish
check "$status" "1" "finish refuses a gate that ran the self-test list"
contains "$output" "hash check: passed" "finish accepts the fast-forward hashes"
contains "$output" "check list check: failed" "finish names the self-test check list"
check "$(lock_state)" "held" "a refused finish keeps the lock"
record_lock_as_full_run
run_gate finish
check "$status" "0" "finish passes after a fast-forward merge of a full gate"
contains "$output" "result: passed; the trees are equal and the lock is released" "finish reports equal trees"
check "$(lock_state)" "released" "finish releases the lock"

start_change docs/read-by-script
commit_file docs/listed.md "listed" "Add a document a script reads"
run_gate gate --ff-only --checks self-test
contains "$output" "scope: full" "a document a script reads is gated with the full run"
run_gate release
check "$status" "0" "an abandoned passed gate is released"

start_change docs/layers
commit_file knowledge/architecture/detailed.md "layers" "Change the package layers"
run_gate gate --ff-only --checks self-test
contains "$output" "scope: full" "the package layers document is gated with the full run"
run_gate release

start_change docs/link
ln -s guide.md "$repo/docs/link.md"
in_repo add -- docs/link.md
in_repo commit --quiet -m "Link a document"
run_gate gate --ff-only --checks self-test
contains "$output" "scope: full" "a symbolic link named like Markdown is gated with the full run"
run_gate release

start_change fix/dirty
commit_file tools/reader.sh "cat docs/guide.md" "Read the guide"
printf 'dirty\n' >"$repo/tools/reader.sh"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a tree with an unstaged change is refused"
check "$(lock_state)" "released" "a refused gate takes no lock"
printf 'cat docs/guide.md\n' >"$repo/tools/reader.sh"
printf 'untracked\n' >"$repo/untracked.txt"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a tree with an untracked file is refused"
rm "$repo/untracked.txt"
printf 'go 1.25\n' >"$repo/go.work"
run_gate gate --ff-only --checks self-test
check "$status" "1" "an ignored workspace file is refused"
contains "$output" "go.work exists in the checkout" "the refusal names the ignored file"
rm "$repo/go.work"

run_gate gate --ff-only --checks self-test
check "$status" "0" "the clean change passes its gate"
in_repo switch --quiet main
commit_file docs/other.md "other" "Merge another change first"
run_gate finish
check "$status" "1" "finish refuses when main moved instead of taking the gated change"
contains "$output" "hash check: failed: main is" "finish names the moved main"
check "$(lock_state)" "held" "a failed finish keeps the lock held"
run_gate release
check "$status" "0" "the owner can release the lock after a failed finish"

run_gate gate --squash --change fix/dirty --checks self-test
check "$status" "1" "a squash gate with an empty index is refused"
contains "$output" "nothing is staged" "the refusal names the empty index"
in_repo merge --quiet --squash fix/dirty >/dev/null 2>&1
run_gate gate --squash --checks self-test
check "$status" "1" "a squash gate without the change's branch is refused"
contains "$output" "needs --change" "the refusal names the missing branch"
run_gate gate --squash --change fix/dirty --checks self-test
check "$status" "0" "a staged squash gate passes"
contains "$output" "tested tree: $(in_repo write-tree)" "the squash gate records the staged tree"
in_repo commit --quiet -m "Read the guide"
record_lock_as_full_run
run_gate finish
check "$status" "0" "finish passes after the squash commit of a full gate"

start_change fix/extra
commit_file tools/extra.sh "echo extra" "Add an extra script"
in_repo switch --quiet main
commit_file docs/moved.md "moved" "Move main"
in_repo merge --quiet --squash fix/extra >/dev/null 2>&1
printf 'not part of the branch\n' >"$repo/docs/moved.md"
in_repo add -- docs/moved.md
run_gate gate --squash --change fix/extra --checks self-test
check "$status" "1" "a staged tree that is not the branch merged into main is refused"
contains "$output" "is not the merge of fix/extra" "the refusal names the branch"
in_repo commit --quiet -m "Squash the extra script with another edit"

start_change fix/late
commit_file tools/late.sh "echo late" "Add a late script"
in_repo switch --quiet fix/late
run_gate gate --squash --change fix/late --checks self-test
check "$status" "1" "a squash gate off main is refused"
contains "$output" "run a squash gate on main" "the refusal names main"
in_repo switch --quiet main
commit_file docs/moved-again.md "moved again" "Move main again"
in_repo merge --quiet --squash fix/late >/dev/null 2>&1
run_gate gate --squash --change fix/late --checks self-test
check "$status" "0" "the squash of the late change passes its gate"
printf 'edited after the gate\n' >"$repo/tools/late.sh"
in_repo add -- tools/late.sh
in_repo commit --quiet -m "Squash with an edit after the gate"
record_lock_as_full_run
run_gate finish
check "$status" "1" "finish refuses a main tree that differs from the tested tree"
contains "$output" "differs from the tested tree" "finish names the differing tree"
run_gate release

start_change fix/changing
commit_file gate-self-test.sh "printf 'changed\\n' >docs/guide.md" "Change the tree during the gate"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a tree that changes during the checks fails the gate"
contains "$output" "result: failed: the checkout changed during the gate" "the report names the change during the gate"
contains "$output" "merge gate report" "the report is printed when the tree changes"
check "$(lock_state)" "released" "a failed gate releases its lock"
in_repo commit --quiet -a -m "Keep the changed guide"

start_change fix/failing
commit_file gate-self-test.sh "exit 1" "Fail the self-test check"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a failing check fails the gate"
contains "$output" "result: failed: a check failed" "the report names the failed check"
contains "$output" "earlier attempts for this tree: none" "the first attempt has no earlier attempts"
run_gate gate --ff-only --checks self-test
contains "$output" "result=failed" "a rerun lists the earlier failed attempt for the same tree"
check "$(file_mode "$log")" "600" "the attempt log stays readable only by the owner"

start_change docs/mode
chmod +x "$repo/docs/guide.md"
in_repo commit --quiet -a -m "Make the guide executable"
run_gate gate --ff-only --checks self-test
check "$status" "0" "a mode change passes the self-test gate"
contains "$output" "scope: full" "a mode change of a Markdown file is gated with the full run"
run_gate release

start_change docs/gitlink
mkdir "$repo/docs/sub.md"
in_repo update-index --add --cacheinfo "160000,$(in_repo rev-parse main),docs/sub.md"
in_repo commit --quiet -m "Add a submodule named like Markdown"
run_gate gate --ff-only --checks self-test
check "$status" "0" "a submodule passes the self-test gate"
contains "$output" "scope: full" "a submodule named like Markdown is gated with the full run"
run_gate release
rmdir "$repo/docs/sub.md"

start_change fix/merged
commit_file tools/merged.sh "echo merged" "Add a merged script"
in_repo switch --quiet main
commit_file docs/before-merge.md "before" "Move main before the merge"
in_repo merge --quiet --no-ff --no-commit fix/merged >/dev/null 2>&1
run_gate gate --squash --change fix/merged --checks self-test
check "$status" "0" "a staged merge of the branch passes the squash gate"
in_repo commit --quiet -m "Merge instead of squashing"
record_lock_as_full_run
run_gate finish
check "$status" "1" "finish refuses a merge commit after a squash gate"
contains "$output" "is a merge commit, not the squash commit" "finish names the merge commit"
run_gate release

start_change fix/log
commit_file docs/log.md "log" "Add a document for the log checks"
mv "$log" "$work/kept.log"
ln -s "$work/kept.log" "$log"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a gate whose log is a link fails"
contains "$output" "is a link, not the merge gate log" "the refusal names the linked log"
check "$(lock_state)" "released" "a gate whose log is a link leaves no lock"
rm "$log"
mkdir "$log"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a gate whose log is not a regular file fails"
check "$(lock_state)" "released" "a gate whose log is not a regular file leaves no lock"
rmdir "$log"
mv "$work/kept.log" "$log"

start_change fix/stale
commit_file docs/stale.md "stale" "Add a stale document"
bash -c 'exit 0' &
dead_pid=$!
wait "$dead_pid"
printf 'pid=%s\nstarted_at=2026-01-01T00:00:00Z\ncheckout=%s\ncommon_dir=%s\nstate=running\nmode=ff-only\nchange=old\nbase=x\nhead=x\ntree=x\nchecks=full\nscope=full\n' \
  "$dead_pid" "$repo" "$repo/.git" >"$lock"
chmod 600 "$lock"
run_gate gate --ff-only --checks self-test
check "$status" "0" "a lock left by a gate that is no longer running is reclaimed"
contains "$output" "reclaimed $lock" "the reclaim is reported"
check "$(find "$lock_dir" -name '.merge-gate.*' | wc -l | tr -d ' ')" "0" "the reclaim leaves no moved lock behind"
run_gate release

ln -s "$work/elsewhere" "$lock"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a lock that is a link is refused"
rm "$lock"
mkdir "$lock"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a lock that is not a regular file is refused"
rmdir "$lock"

expected_full_list="$(
  cat <<'LIST'
pre|.|tools/check-tool-binaries.sh
C|.|pnpm --dir tools/docs install --frozen-lockfile
C|.|tools/docs/check-docs.sh
B|.|bin/gitleaks git --redact --no-banner --config .gitleaks.toml .
B|.|bin/gitleaks git --staged --redact --no-banner --config .gitleaks.toml .
B|.|bin/osv-scanner scan source --no-call-analysis=go --lockfile=tools/go.mod
B|.|bin/osv-scanner scan source --no-call-analysis=go --lockfile=backend/go.mod
B|.|bin/osv-scanner scan source --lockfile=frontend/pnpm-lock.yaml
B|.|bin/osv-scanner scan source --lockfile=tools/docs/pnpm-lock.yaml
B|backend|go mod tidy -diff
B|backend|unformatted="$(gofmt -l .)" && test -z "$unformatted"
B|backend|go vet ./...
B|backend|../bin/golangci-lint run ./...
B|backend|../bin/govulncheck ./...
A|backend|../tools/dev-env.sh go test -race -count=1 ./...
B|backend|../bin/sqlc -f sqlc.yaml diff
C|frontend|pnpm install --frozen-lockfile
C|frontend|pnpm typecheck
C|frontend|pnpm lint
C|frontend|pnpm format:check
C|frontend|pnpm test
C|frontend|pnpm build
C|frontend|pnpm check:classes
C|frontend|pnpm check:bundle
C|frontend|pnpm api:check
C|frontend|pnpm audit --audit-level high
e2e|frontend|../tools/dev-env.sh pnpm e2e
C|.|python3 .claude/hooks/check_guard_secrets.py
C|.|tools/check-dev-env.sh
B|.|tools/check-merge-gate.sh
LIST
)"
tagged_full_list="$(awk '/^full_checks=\(/ { inside = 1; next } inside && /^\)/ { exit } inside' "$source_gate" |
  sed -e 's/^  //' -e 's/^"\(.*\)"$/\1/' -e "s/^'\\(.*\\)'\$/\\1/")"
check "$tagged_full_list" "$expected_full_list" "the tagged list holds every command of the full run in order, each with its slot"
check "$(printf '%s\n' "$tagged_full_list" | cut -d'|' -f2- | LC_ALL=C sort | uniq -d)" "" "no command of the full run appears twice"
check "$(printf '%s\n' "$tagged_full_list" | cut -d'|' -f1 | LC_ALL=C sort | uniq -c | awk '{ printf "%s=%s ", $2, $1 }')" \
  "A=1 B=13 C=14 e2e=1 pre=1 " "the slots pre, A, B, C and e2e together hold the 30 commands of the full run"
check "$(printf '%s\n' "$tagged_full_list" | head -n 1)" "pre|.|tools/check-tool-binaries.sh" "the tool-binary check runs alone before every lane"

validate_records() {
  (
    source "$gate_script"
    checks=("${lane_self_test_checks[@]}")
    check_list=self-test-lanes
    split_checks
    problems=""
    failed_check=no
    phase_records="$1"
    record_lines="$1"
    check_lane_records A B C
    printf 'failed=%s\n%s' "$failed_check" "$problems"
  )
}

valid_records='A|1|0|0.1
B|2|0|0.1
C|3|0|0.1
A|4|0|0.1
B|5|0|0.1
C|7|0|0.1
A|done|0.3
B|done|0.3
C|done|0.3
'
without_line() {
  printf '%s' "$valid_records" | awk -v drop="$1" '$0 != drop'
}
check "$(validate_records "$valid_records")" "failed=no" "complete passing records of lanes A, B and C pass the record validation"
contains "$(validate_records "$(without_line 'A|4|0|0.1')")" "entry 4 of lane A has no record" "the record validation rejects a missing record"
contains "$(validate_records "${valid_records}A|1|0|0.1")" "entry 1 of lane A has 2 records" "the record validation rejects a duplicate record"
contains "$(validate_records "A|1|zero|0.1
$(without_line 'A|1|0|0.1')")" "line 1 of the lane records of A B C is malformed" "the record validation rejects a non-numeric record"
contains "$(validate_records "${valid_records}A|3|0|0.1")" "lane A reported a record for entry 3, which is not in lane A" \
  "the record validation rejects a record for another lane's entry"
contains "$(validate_records "$(without_line 'A|done|0.3')")" "lane A reported its end 0 times" "the record validation rejects a lane without its end record"
contains "$(validate_records "${valid_records}e2e|6|0|0.1")" "lane e2e reported a record for entry 6, which is not in lane e2e" \
  "the record validation rejects a record of another phase"
contains "$(validate_records "${valid_records}e2e|done|0.1")" "lane e2e reported its end outside its phase" \
  "the record validation rejects an end record of another phase"
check "$(validate_records "A|1|1|0.1
$(without_line 'A|1|0|0.1' | awk '$0 != "A|4|0|0.1"')")" "failed=yes" \
  "the record validation marks a failed check and accepts the lane's later entries as not run"

slot_order() {
  printf '%s\n' "$output" | awk -F': ' '/^(exit [0-9]+|not run|no record|[0-9]+ records): / { printf "%s ", $2 }'
}

count_matching() {
  printf '%s\n' "$output" | awk -v pattern="$1" '$0 ~ pattern { found++ } END { print found + 0 }'
}

result_line() {
  count_matching "^$1: $2: .*steps/$3[.]sh"
}

placement() {
  if [[ "$1" == $2 ]]; then
    echo inside
  else
    echo outside
  fi
}

upper() {
  printf '%s' "$1" | tr '[:lower:]' '[:upper:]'
}

step() {
  mkdir -p "$repo/steps"
  printf '%s\n' "$2" >"$repo/steps/$1.sh"
  in_repo add -- "steps/$1.sh"
}

lane_change() {
  start_change "$1"
  shift
  while [[ $# -ge 2 ]]; do
    step "$1" "$2"
    shift 2
  done
  in_repo commit --quiet --allow-empty -m "Add lane steps"
}

run_lanes() {
  run_gate gate --ff-only --checks self-test-lanes
}

lock_field() {
  sed -n "s/^$1=//p" "$lock"
}

live_groups() {
  local group alive=""
  for group in ${1//,/ }; do
    if kill -0 -- "-$group" 2>/dev/null; then
      alive="$alive $group"
    fi
  done
  printf '%s' "$alive"
}

mkdir -p "$lock_dir/logs.expired" "$lock_dir/logs.recent"
touch -t 202001010000 "$lock_dir/logs.expired"
lane_change fix/lanes-pass a1 "printf 'output of a1\\n'"
run_lanes
check "$status" "0" "a gate whose lanes all pass passes"
check "$(slot_order)" "pre A B C A B e2e C " "the report lists every command in list order with its slot"
contains "$output" "exit 0: A: " "the report gives each command its exit status"
check "$(count_matching '^exit 0: (pre|A|B|C|e2e): [0-9]+[.][0-9] s: ')" "8" "the report gives each command its seconds"
for slot in pre A B C e2e; do
  check "$(count_matching "^lane $slot wall time: [0-9]+[.][0-9] s$")" "1" "the report gives the wall time of lane $slot"
  check "$(count_matching "^lane $slot command seconds: [0-9]+[.][0-9]$")" "1" "the report gives the command seconds of lane $slot"
done
for phase in pre "A B C" e2e; do
  check "$(count_matching "^phase $phase wall time with the wait for its processes: [0-9]+[.][0-9] s$")" "1" \
    "the report gives the wall time of the $phase phase including the wait for its processes"
done
check "$(count_matching '^summed command seconds: [0-9]+[.][0-9]$')" "1" "the report gives the summed command seconds"
check "$(count_matching '^gate wall time: [0-9]+[.][0-9] s$')" "1" "the report gives the gate's wall time"
check "$(count_matching '^cores: [1-9][0-9]*$')" "1" "the report gives the machine's core count"
logs="$(printf '%s\n' "$output" | sed -n 's/^command logs: //p')"
check "${logs%/logs.*}" "$lock_dir" "the command logs lie in a folder inside the lock folder"
check "$(placement "$logs" "$repo/*")" "outside" "the command logs lie outside the repository"
check "$(file_mode "$logs")" "700" "the command log folder is readable only by the owner"
check "$(find "$logs" -type f -name '*.log' | wc -l | tr -d ' ')" "8" "every command has its own log"
check "$(find "$logs" ! -type d ! -name '*.log' | wc -l | tr -d ' ')" "0" "the log folder holds no record file and no pipe"
check "$(find "$logs" -type f ! -perm 600 | wc -l | tr -d ' ')" "0" "every command log is readable only by the owner"
contains "$(cat "$logs/1.log")" "output of a1" "a command's output goes to its own log"
check "$(test -d "$lock_dir/logs.expired" && echo kept || echo removed)" "removed" "log folders older than seven days are removed"
check "$(test -d "$lock_dir/logs.recent" && echo kept || echo removed)" "kept" "recent log folders are kept"
check "$(lock_field lanes)" "" "a passed gate's lock records no lane groups"
check "$(lock_field checks)" "self-test-lanes" "the lock records the lane self-test list"
run_gate finish
contains "$output" "check list check: failed" "finish refuses a gate that ran the lane self-test list"
run_gate release

for lane in a b c; do
  slot="$(upper "$lane")"
  lane_change "fix/lane-$lane-fails" "${lane}1" "printf '%s\\n' '${lane}1 failed on purpose'; exit 1"
  run_lanes
  check "$status" "1" "a failing first command in lane $slot fails the gate"
  check "$(result_line "exit 1" "$slot" "${lane}1")" "1" "the report names the failed command of lane $slot"
  check "$(result_line "not run" "$slot" "${lane}2")" "1" "lane $slot stops at its first failure"
  check "$(result_line "not run" e2e e2e)" "1" "the end-to-end suites do not run after lane $slot failed"
  contains "$output" "${lane}1 failed on purpose" "the report prints the end of the failed command's log in lane $slot"
  contains "$output" "result: failed: a check failed" "the result names the failed check of lane $slot"
  check "$(lock_state)" "released" "the gate releases its lock after lane $slot failed"
done

mkfifo "$work/fifo-A" "$work/fifo-B" "$work/fifo-C"
exec 3<>"$work/fifo-A" 4<>"$work/fifo-B" 5<>"$work/fifo-C"
handshake() {
  local own="$1" first="$2" second="$3"
  printf "printf '%s\\\\n' >'%s'; printf '%s\\\\n' >'%s'; read -r -t 30 one <'%s' && read -r -t 30 two <'%s'" \
    "$own" "$work/fifo-$first" "$own" "$work/fifo-$second" "$work/fifo-$own" "$work/fifo-$own"
}
lane_change fix/lanes-concurrent \
  a1 "$(handshake A B C)" \
  b1 "$(handshake B A C); exit 1" \
  c1 "$(handshake C A B)"
run_lanes
exec 3<&- 4<&- 5<&-
check "$status" "1" "a failure in lane B fails the gate"
check "$(result_line "exit 0" A a1)|$(result_line "exit 0" C c1)" "1|1" "lanes A, B and C run at the same time"
check "$(result_line "exit 1" B b1)" "1" "the failure in lane B is reported"
check "$(result_line "exit 0" A a2)|$(result_line "exit 0" C c2)" "1|1" "a failure in one lane lets the other lanes finish and be reported"
check "$(result_line "not run" e2e e2e)" "1" "the end-to-end suites do not run after a lane failed"

lane_change fix/lane-killed a1 "kill -KILL \"\$(ps -o pgid= -p \$\$ | tr -d ' ')\""
run_lanes
check "$status" "1" "a lane killed before its record is written fails the gate"
contains "$output" "result: failed: lane A ended with status 137" "the result names the killed lane"
contains "$output" "entry 1 of lane A has no record" "the report names the missing record"
check "$(result_line "no record" A a1)" "1" "the report marks the command of the killed lane as without a record"
check "$(result_line "not run" e2e e2e)" "1" "the end-to-end suites do not run after a lane was killed"

lane_change fix/lane-e2e-fails e2e "exit 1"
run_lanes
check "$status" "1" "a failing end-to-end entry after every lane passed fails the gate"
check "$(result_line "exit 0" C c2)|$(result_line "exit 1" e2e e2e)" "1|1" "the end-to-end entry runs only after the lanes passed and is reported"
contains "$output" "result: failed: a check failed" "the result names the failed end-to-end entry"

lane_change fix/lane-changes-tree c1 "printf 'changed by a lane\\n' >docs/guide.md"
run_lanes
check "$status" "1" "a tree changed by a lane fails the gate before the end-to-end entry"
contains "$output" "result: failed: the checkout changed during the gate" "the result names the changed checkout"
check "$(result_line "not run" e2e e2e)" "1" "the end-to-end entry does not run on a changed checkout"
in_repo commit --quiet -a -m "Keep the guide a lane changed"

mkfifo "$work/fifo-ready" "$work/fifo-trapped" "$work/fifo-release" "$work/fifo-never" "$work/fifo-gate-errors"
exec 3<>"$work/fifo-ready" 4<>"$work/fifo-trapped" 5<>"$work/fifo-release" 6<>"$work/fifo-never"

read_until() {
  local wanted="$1" line
  while read -r -t 30 line <&7; do
    if [[ "$line" == *"$wanted"* ]]; then
      echo seen
      return 0
    fi
  done
  echo "not seen within 30 seconds"
}

start_background_gate() {
  exec 7<>"$work/fifo-gate-errors"
  bash "$gate_script" gate --ff-only --checks self-test-lanes "$@" --repo "$repo" --lock-dir "$lock_dir" \
    >"$work/background.out" 2>"$work/fifo-gate-errors" &
  gate_pid=$!
}

finish_background_gate() {
  gate_status=0
  wait "$gate_pid" || gate_status=$?
  exec 7<&-
  output="$(cat "$work/background.out")"
}

fd9_probe() {
  printf 'if { printf "%s\\\\n" >&9; } 2>/dev/null; then echo open; else echo closed; fi >"%s"; for found in "%s"/logs.*/*; do printf "%s\\\\n" >>"$found"; done' \
    "$1" "$2" "$lock_dir" "$1"
}

lane_change fix/forge-from-lane-c a1 "exit 1" c1 "$(fd9_probe "A|1|0|0.1" "$work/c1-fd9")"
run_lanes
check "$status" "1" "lane C cannot turn lane A's failed record into a pass"
check "$(cat "$work/c1-fd9")" "closed" "a command runs without the lane record descriptor"
check "$(result_line "exit 1" A a1)|$(result_line "exit 0" C c1)" "1|1" "the report keeps lane A's failure and lane C's forging command"
contains "$output" "result: failed: a check failed" "the result names the failed check despite the forging attempt"
check "$(find "$lock_dir"/logs.* -name '*.records' | wc -l | tr -d ' ')" "0" "no record file exists that a command could rewrite"

lane_change fix/forge-from-e2e-child e2e "( read -r -t 30 go <\"$work/fifo-release\"; $(fd9_probe "e2e|6|0|0.1" "$work/e2e-fd9") ) & exit 1"
start_background_gate
check "$(read_until "remaining processes of lane:process-group e2e:")" "seen" "the gate waits for the end-to-end entry's background child"
printf 'go\n' >&5
finish_background_gate
check "$gate_status" "1" "a background child of the end-to-end entry cannot rewrite its record within the grace"
check "$(cat "$work/e2e-fd9")" "closed" "the end-to-end entry's child runs without the lane record descriptor"
check "$(result_line "exit 1" e2e e2e)" "1" "the report keeps the end-to-end entry's failure"
contains "$output" "result: failed: a check failed" "the result names the failed end-to-end entry despite the forging child"

lane_change fix/lane-child-waited \
  a1 "( read -r -t 30 go <\"$work/fifo-release\"; printf 'finished\\n' >\"$work/child-done\" ) &" \
  e2e "if test -f \"$work/child-done\"; then printf 'after\\n'; else printf 'before\\n'; fi >\"$work/e2e-saw\""
start_background_gate
check "$(read_until "remaining processes of lane:process-group A:")" "seen" "the gate waits for a lane's child after the lane's records"
printf 'go\n' >&5
finish_background_gate
check "$gate_status" "0" "a gate whose lane child ends within the grace passes"
check "$(cat "$work/e2e-saw")" "after" "the end-to-end entry starts only after every lane process ended"
run_gate release

lane_change fix/lane-child-stays a1 "cat <\"$work/fifo-never\" >/dev/null &"
run_gate gate --ff-only --checks self-test-lanes --lane-grace 1
check "$status" "1" "a lane child that never ends fails the gate"
contains "$output" "processes of lane A (process group" "the report names the lane whose processes stayed"
contains "$output" "were still running 1 seconds after it ended" "the report names the grace the processes outlived"
check "$(result_line "not run" e2e e2e)" "1" "the end-to-end entry does not run while a lane process stays"
check "$(lock_state)" "released" "the gate releases its lock after stopping the lane that stayed"

run_unchecked bash "$gate_script" gate --ff-only --lane-grace 1 --repo "$repo" --lock-dir "$lock_dir"
check "$status" "1" "--lane-grace is refused for the full run"
contains "$output" "--lane-grace is accepted only by a gate with --checks self-test or self-test-lanes" "the refusal names the self-test lists"

lane_change fix/lane-ignores-term a1 "$(
  printf '%s\n' \
    "trap '' TERM" \
    "printf '%s\\n' \"\$\$\" >\"$work/stubborn-pid\"" \
    "printf 'ready\\n' >\"$work/fifo-ready\"" \
    "read -r -t 60 never <\"$work/fifo-never\""
)"
start_background_gate --lane-grace 1
read -r -t 30 ready_line <&3 || ready_line="no line within 30 seconds"
check "$ready_line" "ready" "the lane that ignores TERM started"
kill -TERM "$gate_pid"
check "$(read_until "stopped with status 130 before a result; command logs: $lock_dir/logs.")" "seen" \
  "an interrupted gate names its command log folder"
finish_background_gate
check "$gate_status" "130" "a gate interrupted while a lane ignores TERM exits with the interruption status"
check "$(kill -0 "$(cat "$work/stubborn-pid")" 2>/dev/null && echo alive || echo dead)" "dead" \
  "a lane process that ignores TERM is killed before the interrupted gate exits"
check "$(lock_state)" "released" "the interrupted gate releases its lock after KILL ended the lane"
contains "$(tail -n 1 "$log")" "result=interrupted" "an interrupted gate appends its attempt to the attempt log"

lane_change fix/lane-interrupted a1 "$(
  printf '%s\n' \
    "trap 'printf \"trapped\\\\n\" >\"$work/fifo-trapped\"; read -r -t 30 go <\"$work/fifo-release\"; if test -e \"$lock\"; then echo held; else echo released; fi >\"$work/observed\"; exit 0' TERM" \
    "printf '%s\\n' \"\$\$\" >\"$work/step-pid\"" \
    "printf 'ready\\n' >\"$work/fifo-ready\"" \
    "cat <\"$work/fifo-never\" &" \
    "wait \"\$!\""
)"
start_background_gate
read -r -t 30 ready_line <&3 || ready_line="no line within 30 seconds"
check "$ready_line" "ready" "the lane of the interrupted gate started"
step_group="$(ps -o pgid= -p "$(cat "$work/step-pid")" | tr -d ' ')"
recorded_groups="$(lock_field lanes)"
check "$(placement ",$recorded_groups," "*,$step_group,*")" "inside" "the lock records the lane process groups"
kill -TERM "$gate_pid"
read -r -t 30 trapped_line <&4 || trapped_line="no line within 30 seconds"
check "$trapped_line" "trapped" "TERM sent to the gate reaches the lane's processes"
check "$(read_until "seconds for the processes of lane:process-group A:$step_group")" "seen" \
  "the interrupted gate has reaped its lane processes and waits for the rest of the lane's process group"
check "$(lock_state)" "held" "the lock stays held while a lane process still runs after TERM"
printf 'go\n' >&5
finish_background_gate
exec 3<&- 4<&- 5<&- 6<&-
check "$gate_status" "130" "an interrupted gate exits with the interruption status"
check "$(cat "$work/observed")" "held" "the lane's last process saw the lock still held"
check "$(lock_state)" "released" "the interrupted gate releases its lock once its lanes ended"
check "$(live_groups "$recorded_groups")" "" "no lane process of the interrupted gate is left alive"
in_repo status --porcelain --untracked-files=all >"$work/status-after-interrupt"
check "$(cat "$work/status-after-interrupt")" "" "the interrupted gate leaves the checkout clean"

start_change fix/stale-lanes
commit_file docs/stale-lanes.md "stale lanes" "Add a document for the stale lane lock"
mkfifo "$work/fifo-hold"
exec 6<>"$work/fifo-hold"
set -m
cat <"$work/fifo-hold" &
lane_holder=$!
set +m
bash -c 'exit 0' &
dead_pid=$!
wait "$dead_pid"
printf 'pid=%s\nstarted_at=2026-01-01T00:00:00Z\ncheckout=%s\ncommon_dir=%s\nstate=running\nmode=ff-only\nchange=old\nbase=x\nhead=x\ntree=x\nchecks=full\nscope=full\nlanes=%s\n' \
  "$dead_pid" "$repo" "$repo/.git" "$lane_holder" >"$lock"
chmod 600 "$lock"
held="$(cat "$lock")"
run_gate gate --ff-only --checks self-test
check "$status" "1" "a stale lock whose lane process group is alive is not reclaimed"
contains "$output" "lane process groups $lane_holder" "the refusal names the live lane process group"
check "$(cat "$lock")" "$held" "the refused gate leaves the lock with the live lane group alone"
run_gate release
check "$status" "1" "release refuses a lock whose lane process group is alive"
kill -TERM "$lane_holder"
holder_status=0
wait "$lane_holder" 2>/dev/null || holder_status=$?
exec 6<&-
check "$holder_status" "143" "the lane process group of the stale lock ended"
run_gate gate --ff-only --checks self-test
check "$status" "0" "a stale lock whose lane process groups ended is reclaimed"
run_gate release

start_change docs/sequential
commit_file docs/sequential.md "sequential" "Add a document for the sequential list"
run_gate gate --ff-only --checks self-test
check "$status" "0" "the sequential self-test list still passes"
check "$(cut -d= -f1 "$lock" | tr '\n' ' ')" "pid started_at checkout common_dir state mode change base head tree checks scope " \
  "a sequential list writes the same lock fields as before"
check "$(count_matching '^(lane |summed command seconds|gate wall time|cores|command logs)')" "0" \
  "a sequential list prints the same report fields as before"
contains "$output" "exit 0: .: git rev-parse --verify HEAD" "a sequential list reports each command as before"
run_gate release

run_unchecked bash "$gate_script" gate --ff-only --checks self-test --repo "$repo" --lock-dir "$repo/lock-inside"
check "$status" "1" "a lock folder inside the checkout is refused"
contains "$output" "lies inside the checkout" "the refusal names the checkout"
check "$(test -e "$repo/lock-inside" && echo created || echo absent)" "absent" "the refused lock folder is not created"

case_variant="${repo%/repo}/REPO"
if [[ "$case_variant" -ef "$repo" ]]; then
  run_unchecked bash "$gate_script" gate --ff-only --checks self-test --repo "$repo" --lock-dir "$case_variant/lock-inside"
  check "$status" "1" "a lock folder under a case variant of the checkout path is refused"
  contains "$output" "lies inside the checkout" "the refusal of the case variant names the checkout"
  check "$(test -e "$repo/lock-inside" && echo created || echo absent)" "absent" "the refused case-variant lock folder is not created"
else
  check "$(test -e "$case_variant" && echo exists || echo absent)" "absent" \
    "on a case-sensitive file system a case variant of the checkout path names no folder of the checkout"
fi
