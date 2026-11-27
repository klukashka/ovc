#!/usr/bin/env bash
set -euo pipefail

readonly expected_repository="klukashka/ovc"
root="$(cd "$(dirname "$0")/.." && pwd)"
readonly settings="$root/tools/github-settings.json"
work="$(mktemp -d "${TMPDIR:-/tmp}/github-settings.XXXXXX")"
trap 'rm -rf "$work"' EXIT

usage() {
  echo "usage: tools/check-github-settings.sh [--compare FOLDER]" >&2
  exit 2
}

refuse_github_environment() {
  local name refused=0
  while IFS= read -r name; do
    case "$name" in
      GH_* | GITHUB_TOKEN)
        echo "refused: $name is set; unset it so that gh uses only the owner's own login for github.com" >&2
        refused=1
        ;;
    esac
  done < <(compgen -e)
  if [[ "$refused" -ne 0 ]]; then
    exit 2
  fi
}

read_repository() {
  local repository
  repository="$(jq -er '.repository' "$settings")"
  if [[ "$repository" != "$expected_repository" ]]; then
    echo "refused: $settings names repository $repository instead of $expected_repository" >&2
    exit 2
  fi
  printf '%s\n' "$repository"
}

optional_answers=" collaborators keys hooks secrets variables environments "

fetch() {
  local repository="$1" folder="$2" name="$3" path="$4"
  local endpoint="repos/$repository"
  if [[ -n "$path" ]]; then
    endpoint="$endpoint/$path"
  fi
  if gh api --hostname github.com --method GET "$endpoint" >"$folder/$name.json" 2>"$folder/$name.error"; then
    rm -f "$folder/$name.error"
    return 0
  fi
  if [[ "$optional_answers" == *" $name "* ]] && grep -q 'HTTP 403' "$folder/$name.error"; then
    rm -f "$folder/$name.json" "$folder/$name.error"
    : >"$folder/$name.unverified"
    return 0
  fi
  echo "could not read $endpoint" >&2
  exit 1
}

neutral_answer() {
  case "$1" in
    collaborators | keys | hooks) printf '[]' ;;
    secrets) jq '{total_count: .stored_settings.secrets}' "$settings" ;;
    variables) jq '{total_count: .stored_settings.variables}' "$settings" ;;
    environments) jq '{total_count: .stored_settings.environments}' "$settings" ;;
  esac
}

fetch_all() {
  local repository="$1" folder="$2" ruleset_id
  fetch "$repository" "$folder" repo ""
  fetch "$repository" "$folder" actions-permissions "actions/permissions"
  fetch "$repository" "$folder" selected-actions "actions/permissions/selected-actions"
  fetch "$repository" "$folder" workflow-permissions "actions/permissions/workflow"
  fetch "$repository" "$folder" fork-approval "actions/permissions/fork-pr-contributor-approval"
  fetch "$repository" "$folder" retention "actions/permissions/artifact-and-log-retention"
  fetch "$repository" "$folder" vulnerability-reporting "private-vulnerability-reporting"
  fetch "$repository" "$folder" labels "labels?per_page=100"
  fetch "$repository" "$folder" collaborators "collaborators?per_page=100"
  fetch "$repository" "$folder" keys "keys?per_page=100"
  fetch "$repository" "$folder" hooks "hooks?per_page=100"
  fetch "$repository" "$folder" secrets "actions/secrets"
  fetch "$repository" "$folder" variables "actions/variables"
  fetch "$repository" "$folder" environments "environments"
  fetch "$repository" "$folder" rulesets "rulesets?includes_parents=false&per_page=100"
  while IFS= read -r ruleset_id; do
    if [[ ! "$ruleset_id" =~ ^[0-9]+$ ]]; then
      echo "refused: ruleset identifier $ruleset_id is not a number" >&2
      exit 1
    fi
    fetch "$repository" "$folder" "ruleset-$ruleset_id" "rulesets/$ruleset_id"
  done < <(jq -r '.[].id' "$folder/rulesets.json")
}

required_answers=(repo actions-permissions selected-actions workflow-permissions fork-approval retention vulnerability-reporting labels collaborators keys hooks secrets variables environments rulesets)

compare() {
  local folder="$1" name details unverified=""
  for name in "${required_answers[@]}"; do
    if [[ ! -f "$folder/$name.json" && -f "$folder/$name.unverified" && "$optional_answers" == *" $name "* ]]; then
      neutral_answer "$name" >"$work/neutral-$name.json"
      unverified="$unverified $name"
      continue
    fi
    if [[ ! -f "$folder/$name.json" ]]; then
      echo "mismatch: answers: $name.json is missing from $folder"
      return 1
    fi
  done
  details="$work/rulesets-combined.json"
  local ruleset_file
  printf '[]' >"$details"
  shopt -s nullglob
  for ruleset_file in "$folder"/ruleset-*.json; do
    jq --slurpfile next "$ruleset_file" '. + $next' "$details" >"$details.next"
    mv "$details.next" "$details"
  done
  shopt -u nullglob
  local problems
  problems="$(jq -nr \
    --slurpfile want "$settings" \
    --slurpfile repo "$folder/repo.json" \
    --slurpfile actions "$folder/actions-permissions.json" \
    --slurpfile selected "$folder/selected-actions.json" \
    --slurpfile workflow "$folder/workflow-permissions.json" \
    --slurpfile fork "$folder/fork-approval.json" \
    --slurpfile retention "$folder/retention.json" \
    --slurpfile reporting "$folder/vulnerability-reporting.json" \
    --slurpfile labels "$folder/labels.json" \
    --slurpfile collaborators "$(answer_file "$folder" collaborators)" \
    --slurpfile keys "$(answer_file "$folder" keys)" \
    --slurpfile hooks "$(answer_file "$folder" hooks)" \
    --slurpfile secrets "$(answer_file "$folder" secrets)" \
    --slurpfile variables "$(answer_file "$folder" variables)" \
    --slurpfile environments "$(answer_file "$folder" environments)" \
    --slurpfile rulesets "$details" '
    def fields($group; $want; $have):
      $want | to_entries[] | select($have[.key] != .value)
      | "\($group): \(.key) is \($have[.key] | tojson), expected \(.value | tojson)";
    def ruleset_matches($want):
      .target == $want.target
      and .enforcement == $want.enforcement
      and (((.conditions.ref_name.include // []) | sort) as $included | ($want.include_any // [$want.include]) | any(.[]; sort == $included))
      and ((.conditions.ref_name.exclude // []) == [])
      and (has("bypass_actors") and .bypass_actors == [])
      and ([.rules[]?.type] as $types | all($want.rules[]; . as $rule | $types | index($rule) != null));
    $want[0] as $w
    | $repo[0] as $r
    | (
        (if $r.full_name != $w.repository then "repository: full_name is \($r.full_name | tojson), expected \($w.repository | tojson)" else empty end),
        fields("repository"; $w.repository_settings; $r),
        (if ($r.security_and_analysis.secret_scanning.status // "missing") != $w.secret_scanning.secret_scanning
          then "secret-scanning: secret_scanning is \($r.security_and_analysis.secret_scanning.status // "missing" | tojson), expected \($w.secret_scanning.secret_scanning | tojson)" else empty end),
        (if ($r.security_and_analysis.secret_scanning_push_protection.status // "missing") != $w.secret_scanning.secret_scanning_push_protection
          then "secret-scanning: secret_scanning_push_protection is \($r.security_and_analysis.secret_scanning_push_protection.status // "missing" | tojson), expected \($w.secret_scanning.secret_scanning_push_protection | tojson)" else empty end),
        fields("actions-permissions"; $w.actions_permissions; $actions[0]),
        fields("selected-actions"; $w.selected_actions | del(.patterns_allowed); $selected[0]),
        (if (($selected[0].patterns_allowed // []) | sort) != ($w.selected_actions.patterns_allowed | sort)
          then "selected-actions: patterns_allowed is \($selected[0].patterns_allowed // [] | sort | tojson), expected \($w.selected_actions.patterns_allowed | sort | tojson)" else empty end),
        fields("workflow-permissions"; $w.workflow_permissions; $workflow[0]),
        fields("fork-approval"; $w.fork_pr_approval; $fork[0]),
        fields("retention"; $w.retention; $retention[0]),
        fields("vulnerability-reporting"; $w.private_vulnerability_reporting; $reporting[0]),
        ($w.rulesets[] as $ws
          | if any($rulesets[0][]; ruleset_matches($ws)) then empty
            else "rulesets: no active ruleset \($ws.name | tojson) on \($ws.target) \(($ws.include_any // [$ws.include]) | tojson) with rules \($ws.rules | tojson), no exclusions and no bypass" end),
        ([$labels[0][].name] as $names
          | $w.labels[] | select(.name as $n | $names | index($n) | not)
          | "labels: \(.name | tojson) is missing"),
        ($collaborators[0][] | select(.login as $l | $w.write_access.collaborators | index($l) | not)
          | "write-access: collaborator \(.login | tojson) is not allowed"),
        ($keys[0][] | select(.read_only != true)
          | "write-access: deploy key \(.title | tojson) has write access"),
        (if ($hooks[0] | length) != 0 then "write-access: \($hooks[0] | length) webhooks exist, expected none" else empty end),
        (if $secrets[0].total_count != $w.stored_settings.secrets then "stored-settings: \($secrets[0].total_count) repository secrets exist, expected \($w.stored_settings.secrets)" else empty end),
        (if $variables[0].total_count != $w.stored_settings.variables then "stored-settings: \($variables[0].total_count) repository variables exist, expected \($w.stored_settings.variables)" else empty end),
        (if $environments[0].total_count != $w.stored_settings.environments then "stored-settings: \($environments[0].total_count) environments exist, expected \($w.stored_settings.environments)" else empty end)
      )
  ')"
  if [[ -n "$problems" ]]; then
    while IFS= read -r name; do
      printf 'mismatch: %s\n' "$name"
    done <<<"$problems"
    if [[ -n "$unverified" ]]; then
      printf 'unverified (no read permission):%s\n' "$unverified"
    fi
    return 1
  fi
  if [[ -n "$unverified" ]]; then
    printf 'settings match where readable; unverified (no read permission):%s\n' "$unverified"
    return 0
  fi
  echo "settings match tools/github-settings.json"
}

answer_file() {
  if [[ -f "$1/$2.json" ]]; then
    printf '%s' "$1/$2.json"
  else
    printf '%s' "$work/neutral-$2.json"
  fi
}

compare_folder=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --compare)
      [[ $# -ge 2 ]] || usage
      compare_folder="$2"
      shift 2
      ;;
    *)
      usage
      ;;
  esac
done

if [[ -n "$compare_folder" ]]; then
  read_repository >/dev/null
  compare "$compare_folder"
  exit $?
fi

refuse_github_environment
repository="$(read_repository)"
mkdir "$work/answers"
fetch_all "$repository" "$work/answers"
compare "$work/answers"
