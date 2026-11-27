#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/tools/check-github-settings.sh"
fixtures="$root/tools/testdata/github/match"
work="$(mktemp -d "${TMPDIR:-/tmp}/github-settings-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
failures=0
cases=0

prepare_answers() {
  local folder="$1"
  rm -rf "$folder"
  cp -R "$fixtures" "$folder"
  jq --slurpfile settings "$root/tools/github-settings.json" '.patterns_allowed = ($settings[0].selected_actions.patterns_allowed | reverse)' \
    "$folder/selected-actions.json" >"$folder/selected-actions.next"
  mv "$folder/selected-actions.next" "$folder/selected-actions.json"
}

mutate() {
  local folder="$1" file="$2" program="$3"
  jq "$program" "$folder/$file" >"$folder/$file.next"
  mv "$folder/$file.next" "$folder/$file"
}

record_failure() {
  echo "FAIL: $1" >&2
  failures=$((failures + 1))
}

expect_match() {
  local folder="$work/match" output status
  prepare_answers "$folder"

  status=0
  output="$(bash "$script" --compare "$folder" 2>&1)" || status=$?

  cases=$((cases + 1))
  if [[ "$status" -ne 0 || "$output" != "settings match tools/github-settings.json" ]]; then
    record_failure "matching answers: exit $status, output: $output"
  fi
}

expect_mismatch() {
  local group="$1" file="$2" program="$3"
  local folder="$work/mismatch" output status
  prepare_answers "$folder"
  mutate "$folder" "$file" "$program"

  status=0
  output="$(bash "$script" --compare "$folder" 2>&1)" || status=$?

  cases=$((cases + 1))
  if [[ "$status" -ne 1 ]] || ! grep -q "^mismatch: $group: " <<<"$output"; then
    record_failure "$group mismatch through $file ($program): exit $status, output: $output"
  fi
}

expect_missing_answer() {
  local folder="$work/missing" output status
  prepare_answers "$folder"
  rm "$folder/fork-approval.json"

  status=0
  output="$(bash "$script" --compare "$folder" 2>&1)" || status=$?

  cases=$((cases + 1))
  if [[ "$status" -ne 1 ]] || ! grep -q "^mismatch: answers: fork-approval.json is missing" <<<"$output"; then
    record_failure "missing answer: exit $status, output: $output"
  fi
}

expect_refused_variable() {
  local name="$1" output status

  status=0
  output="$(env "$name=x" bash "$script" 2>&1)" || status=$?

  cases=$((cases + 1))
  if [[ "$status" -ne 2 ]] || ! grep -q "^refused: $name is set" <<<"$output"; then
    record_failure "$name refusal: exit $status, output: $output"
  fi
}

expect_match
expect_mismatch repository repo.json '.has_wiki = true'
expect_mismatch repository repo.json '.has_discussions = true'
expect_mismatch repository repo.json '.has_projects = true'
expect_mismatch repository repo.json '.has_pages = true'
expect_mismatch repository repo.json '.visibility = "private"'
expect_mismatch repository repo.json '.full_name = "someone/ovc"'
expect_mismatch secret-scanning repo.json '.security_and_analysis.secret_scanning.status = "disabled"'
expect_mismatch secret-scanning repo.json '.security_and_analysis.secret_scanning_push_protection.status = "disabled"'
expect_mismatch actions-permissions actions-permissions.json '.sha_pinning_required = false'
expect_mismatch actions-permissions actions-permissions.json '.allowed_actions = "all"'
expect_mismatch selected-actions selected-actions.json '.patterns_allowed += ["actions/cache@0000000000000000000000000000000000000000"]'
expect_mismatch selected-actions selected-actions.json '.github_owned_allowed = true'
expect_mismatch workflow-permissions workflow-permissions.json '.default_workflow_permissions = "write"'
expect_mismatch workflow-permissions workflow-permissions.json '.can_approve_pull_request_reviews = true'
expect_mismatch fork-approval fork-approval.json '.approval_policy = "first_time_contributors"'
expect_mismatch retention retention.json '.days = 90'
expect_mismatch vulnerability-reporting vulnerability-reporting.json '.enabled = false'
expect_mismatch rulesets ruleset-4000001.json '.bypass_actors = [{"actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always"}]'
expect_mismatch rulesets ruleset-4000001.json '.rules |= map(select(.type != "non_fast_forward"))'
expect_mismatch rulesets ruleset-4000001.json '.enforcement = "evaluate"'
expect_mismatch rulesets ruleset-4000002.json '.rules |= map(select(.type != "creation"))'
expect_mismatch rulesets ruleset-4000002.json '.conditions.ref_name.exclude = ["refs/tags/v*"]'
expect_mismatch labels labels.json 'map(select(.name != "needs-owner"))'
expect_mismatch write-access collaborators.json '. + [{"login": "another-account", "id": 2000002, "role_name": "write"}]'
expect_mismatch write-access keys.json '.[0].read_only = false'
expect_mismatch write-access hooks.json '. + [{"id": 5000001, "active": true}]'
expect_mismatch stored-settings secrets.json '.total_count = 1'
expect_mismatch stored-settings variables.json '.total_count = 1'
expect_mismatch stored-settings environments.json '.total_count = 1'
expect_missing_answer
expect_refused_variable GH_TOKEN
expect_refused_variable GH_HOST
expect_refused_variable GH_REPO
expect_refused_variable GH_CONFIG_DIR
expect_refused_variable GH_ENTERPRISE_TOKEN
expect_refused_variable GITHUB_TOKEN

if [[ "$failures" -ne 0 ]]; then
  echo "check-github-settings self-test: $failures of $cases cases failed" >&2
  exit 1
fi
echo "check-github-settings self-test: $cases cases passed"
