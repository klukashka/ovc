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
