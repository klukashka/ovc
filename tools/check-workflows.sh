#!/usr/bin/env bash
set -euo pipefail

tools="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$tools/.." && pwd)"
self_tests=yes

usage() {
  echo "usage: tools/check-workflows.sh [--root FOLDER] [--skip-self-tests]" >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root)
      [[ $# -ge 2 && -n "$2" ]] || usage
      root="$2"
      shift 2
      ;;
    --skip-self-tests)
      self_tests=no
      shift
      ;;
    *) usage ;;
  esac
done

if [[ "$self_tests" == yes ]]; then
  bash "$tools/check-workflows-test.sh"
  if [[ -f "$tools/check-github-settings.sh" ]]; then
    bash "$tools/check-github-settings-test.sh"
  fi
  bash "$tools/ci-settings-test.sh"
fi

read -r -d '' comment_program <<'AWK' || true
function opens_quote(character) {
  return index(" \t:=([{,-", character) > 0
}
kind == "markdown" {
  if (index($0, "<!--")) print FNR
  next
}
FNR == 1 && substr($0, 1, 2) == "#!" {
  next
}
{
  quote = ""
  escaped = 0
  previous = " "
  count = length($0)
  for (position = 1; position <= count; position++) {
    character = substr($0, position, 1)
    if (escaped) {
      escaped = 0
    } else if (quote == "\"" && character == "\\") {
      escaped = 1
    } else if (quote != "") {
      if (character == quote) quote = ""
    } else if ((character == "\"" || character == "'") && opens_quote(previous)) {
      quote = character
    } else if (character == "#" && (previous == " " || previous == "\t")) {
      print FNR
      break
    }
    previous = character
  }
}
AWK

read -r -d '' workflow_program <<'AWK' || true
function trim(text) {
  sub(/^[ \t]+/, "", text)
  sub(/[ \t]+$/, "", text)
  return text
}
function unquote(text,   first, last) {
  text = trim(text)
  first = substr(text, 1, 1)
  last = substr(text, length(text), 1)
  if (length(text) >= 2 && ((first == "\"" && last == "\"") || (first == "'" && last == "'"))) {
    text = substr(text, 2, length(text) - 2)
  }
  return text
}
function report(line, message) {
  printf "%s:%d: %s\n", file, line, message
  failed = 1
}
function is_on_key(key) {
  return key == "on" || key == "true"
}
function is_key(text,   rest) {
  if (!match(text, /^("[^"]*"|'[^']*'|[A-Za-z0-9_][A-Za-z0-9_.-]*):/)) return 0
  rest = substr(text, RLENGTH + 1)
  if (rest != "" && substr(rest, 1, 1) != " ") return 0
  parsed_key = unquote(substr(text, 1, RLENGTH - 1))
  parsed_value = trim(rest)
  return 1
}
function set_context() {
  job = (depth >= 2 && keys[1] == "jobs") ? keys[2] : ""
  in_step = (job != "" && depth >= 4 && keys[3] == "steps")
}
function pop_from(level) {
  while (depth > 0 && indents[depth] >= level) depth--
}
function pop_above(level) {
  while (depth > 0 && indents[depth] > level) depth--
}
function check_if(value, line) {
  if (job != "gate" && (index(value, "always()") || index(value, "failure()"))) {
    report(line, "if: with always() or failure() is allowed only in the gate job")
  }
}
function check_run(text, line) {
  if (text ~ /[$][{][{][^}]*(inputs|github[.]event)[.]/) {
    report(line, "${{ inputs.* }} or ${{ github.event.* }} inside run:; pass the value through env:")
  }
  if (in_step) step_run[step] = step_run[step] "\n" text
}
function check_triggers(list, line,   count, parts, i, name) {
  gsub(/\[/, "", list)
  gsub(/\]/, "", list)
  count = split(list, parts, ",")
  for (i = 1; i <= count; i++) {
    name = unquote(parts[i])
    if (name != "workflow_dispatch") report(line, "the trigger " name " is not allowed; the only trigger is workflow_dispatch")
  }
}
function add_needs(list,   count, parts, i, name) {
  gsub(/\[/, "", list)
  gsub(/\]/, "", list)
  count = split(list, parts, ",")
  for (i = 1; i <= count; i++) {
    name = unquote(parts[i])
    if (name != "") needs[job, name] = 1
  }
}
function scalar_item(value, line) {
  if (depth == 1 && is_on_key(keys[1])) check_triggers(value, line)
  if (job != "" && depth == 3 && keys[3] == "needs") needs[job, value] = 1
}
function handle(key, value, line,   first) {
  first = substr(value, 1, 1)
  if (first == "&" || first == "*" || key == "<<") report(line, "YAML anchors, aliases and merge keys are not allowed")
  if (first == "{" && value != "{}") report(line, "flow mappings are not allowed; use block style")
  if (depth == 1) {
    if (is_on_key(key)) {
      seen_on = 1
      if (value != "") check_triggers(value, line)
    }
    if (key == "permissions") {
      seen_permissions = 1
      if (index(value, "write")) report(line, "the top-level permissions grant a write permission")
    }
    return
  }
  if (depth == 2 && is_on_key(keys[1])) {
    if (key != "workflow_dispatch") report(line, "the trigger " key " is not allowed; the only trigger is workflow_dispatch")
    return
  }
  if (keys[1] == "permissions") {
    if (key == "id-token") report(line, "the id-token permission is not allowed")
    if (unquote(value) == "write") report(line, "the top-level permissions grant a write permission")
    return
  }
  if (keys[1] != "jobs") return
  if (depth == 2) {
    jobs_count++
    job_names[jobs_count] = key
    job_line[key] = line
    return
  }
  if (depth == 3) {
    if (key == "runs-on") runs_on[job] = unquote(value)
    else if (key == "uses") report(line, "the job " job " calls a reusable workflow; job-level uses: is not allowed")
    else if (key == "container" || key == "services") report(line, "the job " job " sets " key ":, which is not allowed")
    else if (key == "permissions" && value != "" && value != "{}") report(line, "the job " job " sets its permissions as a scalar; list each permission")
    else if (key == "needs" && value != "") add_needs(value)
    else if (key == "if") {
      check_if(value, line)
      job_if[job] = value
    }
    return
  }
  if (depth == 4 && keys[3] == "permissions") {
    if (key == "id-token") report(line, "the id-token permission is not allowed")
    if (unquote(value) == "write" && !(job == "gate" && key == "statuses")) {
      report(line, "the job " job " has the write permission " key "; only the gate job may write, and only statuses")
    }
    return
  }
  if (in_step && depth == 4) {
    if (key == "uses") {
      step_uses[step] = unquote(value)
      step_uses_line[step] = line
    } else if (key == "run") {
      if (value !~ /^[|>]/) check_run(value, line)
    } else if (key == "working-directory") step_dir[step] = unquote(value)
    else if (key == "if") check_if(value, line)
    return
  }
  if (in_step && depth == 5 && keys[4] == "with") {
    step_with[step, key] = unquote(value)
    step_has[step, key] = 1
  }
}
function check_uses(s, name, uses,   line, at, action, ref) {
  line = step_uses_line[s]
  if (substr(uses, 1, 9) == "docker://") {
    report(line, "docker:// actions are not allowed")
    return
  }
  at = index(uses, "@")
  action = at ? substr(uses, 1, at - 1) : uses
  ref = at ? substr(uses, at + 1) : ""
  if (!(action in allowed)) report(line, "the action " action " is outside the allowlist")
  if (length(ref) != 40 || ref !~ /^[0-9a-f]+$/) report(line, "the action " action " is not pinned to a full commit SHA")
  if (name == "gate" && action == "actions/checkout") report(line, "the gate job checks nothing out")
  if (action == "actions/setup-go" && step_with[s, "cache"] != "false") report(line, "actions/setup-go must set cache: false")
  if (action == "actions/setup-node") {
    if (step_has[s, "cache"]) report(line, "actions/setup-node must not set cache:")
    if (step_with[s, "package-manager-cache"] != "false") report(line, "actions/setup-node must set package-manager-cache: false")
  }
  if (action ~ /\/setup-/ && action != "actions/setup-go" && action != "actions/setup-node") {
    report(line, "the setup action " action " has no rule that turns its cache off")
  }
}
BEGIN {
  allowed["actions/checkout"] = 1
  allowed["actions/setup-go"] = 1
  allowed["actions/setup-node"] = 1
  allowed["actions/upload-artifact"] = 1
  depth = 0
  steps = 0
  jobs_count = 0
}
{
  raw = $0
  if (raw ~ /\r$/) {
    report(NR, "carriage returns are not allowed")
    sub(/\r$/, "", raw)
  }
  if (index(raw, "secrets.")) report(NR, "secrets. is not allowed; the workflow uses no stored secrets")
  if (index(raw, "pull_request_target")) report(NR, "pull_request_target is not allowed")
  if (index(raw, "continue-on-error")) report(NR, "continue-on-error is not allowed")
  if (index(raw, "actions/cache")) report(NR, "actions/cache is not allowed; nothing is cached")
  if (in_block) {
    if (raw ~ /^[ ]*$/) next
    match(raw, /^ */)
    if (RLENGTH > block_indent) {
      if (block_key == "run") check_run(raw, NR)
      if (job != "") job_text[job] = job_text[job] "\n" raw
      next
    }
    in_block = 0
  }
  if (raw ~ /^[ ]*$/) next
  if (raw ~ /^[ ]*\t/) {
    report(NR, "tabs are not allowed in indentation")
    next
  }
  match(raw, /^ */)
  indent = RLENGTH
  content = substr(raw, indent + 1)
  if (content ~ /^#/) next
  if (indent == 0 && (content ~ /^---/ || content ~ /^[.][.][.]/)) {
    report(NR, "YAML document markers are not allowed")
    next
  }
  if (content == "-" || substr(content, 1, 2) == "- ") {
    pop_above(indent)
    set_context()
    if (depth == 3 && keys[1] == "jobs" && keys[3] == "steps") {
      steps++
      step = steps
      step_job[step] = keys[2]
      step_line[step] = NR
    }
    content = trim(substr(content, 2))
    indent = indent + 2
    if (content == "") next
    if (!is_key(content)) {
      scalar_item(unquote(content), NR)
      next
    }
  }
  if (!is_key(content)) {
    report(NR, "the line is neither a key nor a list item; use block style")
    next
  }
  pop_from(indent)
  depth++
  indents[depth] = indent
  keys[depth] = parsed_key
  set_context()
  if (job != "") job_text[job] = job_text[job] "\n" raw
  handle(parsed_key, parsed_value, NR)
  if (parsed_value ~ /^[|>][-+0-9]*$/) {
    in_block = 1
    block_indent = indent
    block_key = parsed_key
  }
}
END {
  if (!seen_on) report(0, "the workflow has no on: trigger")
  if (!seen_permissions) report(0, "the workflow has no top-level permissions")
  has_gate = 0
  for (i = 1; i <= jobs_count; i++) {
    name = job_names[i]
    if (name == "gate") has_gate = 1
    if (runs_on[name] != "ubuntu-24.04") {
      report(job_line[name], "the job " name " runs on " (runs_on[name] == "" ? "nothing" : runs_on[name]) "; every job runs on ubuntu-24.04")
    }
    gitleaks[name] = (name == "backend-checks" || name == "docs-checks" || index(job_text[name], "bin/gitleaks")) ? 1 : 0
  }
  for (s = 1; s <= steps; s++) {
    name = step_job[s]
    uses = step_uses[s]
    if (uses != "") check_uses(s, name, uses)
    if (step_run[s] ~ /candidate\/tools\/[^ \t\n"';|&)]*[.](sh|bash|py|js|mjs|cjs|ts)/) {
      report(step_line[s], "the step runs a script from candidate/tools/; harness scripts run from harness/")
    }
    if (step_dir[s] ~ /^(\.\/)?candidate(\/|$)/ && step_run[s] ~ /(^|[ \t\n;&|(])(\.\/)?tools\/[^ \t\n"';|&)]*[.](sh|bash|py|js|mjs|cjs|ts)/) {
      report(step_line[s], "the step runs a script from candidate/tools/; harness scripts run from harness/")
    }
    if (gitleaks[name] && uses ~ /^actions\/checkout@/ && step_with[s, "path"] == "candidate" && step_with[s, "fetch-depth"] == "0") {
      deep[name] = 1
    }
  }
  for (i = 1; i <= jobs_count; i++) {
    name = job_names[i]
    if (gitleaks[name] && !deep[name]) report(job_line[name], "the job " name " runs Gitleaks but checks out the candidate without fetch-depth: 0")
  }
  if (!has_gate) {
    report(0, "the workflow has no gate job")
  } else {
    if (!index(job_if["gate"], "always()")) report(job_line["gate"], "the gate job must run with if: always() so that it sees every result")
    for (i = 1; i <= jobs_count; i++) {
      name = job_names[i]
      if (name != "gate" && !needs["gate", name]) report(job_line["gate"], "the gate job does not need " name "; it must require the result of every job")
    }
    if (!index(job_text["gate"], "toJSON(needs)") || !index(job_text["gate"], "success")) {
      report(job_line["gate"], "the gate job must check every needs result for success through toJSON(needs)")
    }
  }
  exit failed
}
AWK

github="$root/.github"
workflow="$github/workflows/gate.yml"
problems=()

if [[ ! -d "$github" || -L "$github" ]]; then
  echo "check-workflows: $github is missing or is a link" >&2
  exit 1
fi

while IFS= read -r -d '' path; do
  relative=".github/${path#"$github"/}"
  if [[ -L "$path" ]]; then
    problems+=("$relative: links are not allowed under .github")
    continue
  fi
  [[ -f "$path" ]] || continue
  case "$relative" in
    .github/workflows/gate.yml) ;;
    .github/workflows/*) problems+=("$relative: only .github/workflows/gate.yml is allowed as a workflow") ;;
  esac
  case "$relative" in
    *.yaml) problems+=("$relative: the .yaml extension is not allowed; use .yml") ;;
  esac
  case "$relative" in
    .github/dependabot.yml | .github/dependabot.yaml) problems+=("$relative: Dependabot is not allowed") ;;
  esac
  kind=text
  if [[ "$relative" == *.md ]]; then
    kind=markdown
  fi
  while IFS= read -r line_number; do
    problems+=("$relative:$line_number: comments are not allowed under .github")
  done < <(awk -v kind="$kind" "$comment_program" "$path")
done < <(find "$github" -mindepth 1 -print0)

if [[ -f "$workflow" && ! -L "$workflow" ]]; then
  while IFS= read -r problem; do
    problems+=("$problem")
  done < <(awk -v file=".github/workflows/gate.yml" "$workflow_program" "$workflow" || true)
else
  problems+=(".github/workflows/gate.yml is missing")
fi

if [[ "${#problems[@]}" -gt 0 ]]; then
  printf 'check-workflows: %s\n' "${problems[@]}" >&2
  exit 1
fi
echo "check-workflows: .github passes every workflow rule"
