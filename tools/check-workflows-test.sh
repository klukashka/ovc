#!/usr/bin/env bash
set -euo pipefail

tools="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository="$(cd "$tools/.." && pwd)"
checker="$tools/check-workflows.sh"
work="$(mktemp -d "${TMPDIR:-/tmp}/check-workflows-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
pin="$(printf 'a%.0s' $(seq 1 40))"
failures=0
cases=0

read -r -d '' base_workflow <<'YAML' || true
name: gate
run-name: gate ${{ inputs.sha }}

on:
  workflow_dispatch:
    inputs:
      sha:
        required: true
        type: string

permissions: {}

jobs:
  backend-checks:
    runs-on: ubuntu-24.04
    permissions:
      contents: read
    steps:
      - name: Check out the harness
        uses: actions/checkout@PIN
        with:
          path: harness
          persist-credentials: false
      - name: Check out the candidate
        uses: actions/checkout@PIN
        with:
          path: candidate
          fetch-depth: 0
          persist-credentials: false
      - name: Set up Go
        uses: actions/setup-go@PIN
        with:
          go-version-file: candidate/tools/go.mod
          cache: false
      - name: Set up Node
        uses: actions/setup-node@PIN
        with:
          node-version-file: candidate/frontend/.node-version
          package-manager-cache: false
      - name: Run the job
        env:
          CANDIDATE: ${{ inputs.sha }}
        run: |
          harness/tools/merge-gate.sh run-job backend-checks --repo candidate --base "$CANDIDATE"

  gate:
    needs: [backend-checks]
    if: always()
    runs-on: ubuntu-24.04
    permissions:
      statuses: write
    steps:
      - name: Require every result to be success
        env:
          NEEDS: ${{ toJSON(needs) }}
        run: |
          jq -e 'all(.[]; .result == "success")' <<<"$NEEDS"
YAML

new_fixture() {
  local folder="$work/$1"
  mkdir -p "$folder/.github/workflows" "$folder/.github/ISSUE_TEMPLATE"
  printf 'blank_issues_enabled: false\n' >"$folder/.github/ISSUE_TEMPLATE/config.yml"
  printf '## Change\n\nDescribe the change.\n' >"$folder/.github/pull_request_template.md"
  printf '%s\n' "${base_workflow//PIN/$pin}" >"$folder/.github/workflows/gate.yml"
  printf '%s' "$folder"
}

replace_line() {
  local file="$1" old="$2" new="$3"
  awk -v old="$old" -v new="$new" '
    !done && $0 == old { print new; done = 1; next }
    { print }
    END { if (!done) exit 3 }
  ' "$file" >"$file.next"
  mv "$file.next" "$file"
}

workflow_of() {
  printf '%s/.github/workflows/gate.yml' "$1"
}

run_checker() {
  bash "$checker" --skip-self-tests --root "$1" 2>&1
}

expect_failure() {
  local name="$1" folder="$2" expected="$3" output status=0
  cases=$((cases + 1))

  output="$(run_checker "$folder")" || status=$?

  if [[ "$status" -eq 0 ]]; then
    echo "FAIL: $name: the check passed" >&2
    failures=$((failures + 1))
  elif [[ "$output" != *"$expected"* ]]; then
    printf 'FAIL: %s: the output does not name [%s]:\n%s\n' "$name" "$expected" "$output" >&2
    failures=$((failures + 1))
  fi
}

expect_pass() {
  local name="$1" folder="$2" output status=0
  cases=$((cases + 1))

  output="$(run_checker "$folder")" || status=$?

  if [[ "$status" -ne 0 ]]; then
    printf 'FAIL: %s: the check failed:\n%s\n' "$name" "$output" >&2
    failures=$((failures + 1))
  fi
}

test_base_fixture_passes() {
  local folder
  folder="$(new_fixture base)"

  expect_pass "base fixture" "$folder"
}

test_committed_workflow_passes() {
  expect_pass "committed workflow" "$repository"
}

test_other_workflow_file_fails() {
  local folder
  folder="$(new_fixture other-workflow)"
  printf 'name: other\n' >"$folder/.github/workflows/other.yml"

  expect_failure "other workflow file" "$folder" "only .github/workflows/gate.yml is allowed"
}

test_yaml_extension_fails() {
  local folder
  folder="$(new_fixture yaml-extension)"
  printf 'name: form\n' >"$folder/.github/ISSUE_TEMPLATE/form.yaml"

  expect_failure "yaml extension" "$folder" "the .yaml extension is not allowed"
}

test_unpinned_action_fails() {
  local folder
  folder="$(new_fixture unpinned)"
  replace_line "$(workflow_of "$folder")" "        uses: actions/setup-go@$pin" "        uses: actions/setup-go@v7"

  expect_failure "unpinned action" "$folder" "not pinned to a full commit SHA"
}

test_action_outside_allowlist_fails() {
  local folder
  folder="$(new_fixture outside-allowlist)"
  replace_line "$(workflow_of "$folder")" "        uses: actions/checkout@$pin" "        uses: someone/thing@$pin"

  expect_failure "action outside the allowlist" "$folder" "outside the allowlist"
}

test_reusable_workflow_fails() {
  local folder
  folder="$(new_fixture reusable)"
  replace_line "$(workflow_of "$folder")" "  gate:" "  reuse:\n    uses: someone/workflows/.github/workflows/check.yml@$pin\n  gate:"

  expect_failure "reusable workflow" "$folder" "calls a reusable workflow"
}

test_docker_action_fails() {
  local folder
  folder="$(new_fixture docker)"
  replace_line "$(workflow_of "$folder")" "        uses: actions/checkout@$pin" "        uses: docker://alpine"

  expect_failure "docker action" "$folder" "docker:// actions are not allowed"
}

test_other_trigger_fails() {
  local folder
  folder="$(new_fixture push-trigger)"
  replace_line "$(workflow_of "$folder")" "  workflow_dispatch:" "  push:\n  workflow_dispatch:"

  expect_failure "push trigger" "$folder" "the trigger push is not allowed"
}

test_missing_top_level_permissions_fails() {
  local folder
  folder="$(new_fixture no-permissions)"
  replace_line "$(workflow_of "$folder")" "permissions: {}" ""

  expect_failure "missing top-level permissions" "$folder" "no top-level permissions"
}

test_write_permission_outside_gate_fails() {
  local folder
  folder="$(new_fixture job-write)"
  replace_line "$(workflow_of "$folder")" "      contents: read" "      contents: write"

  expect_failure "write permission outside the gate" "$folder" "has the write permission contents"
}

test_id_token_permission_fails() {
  local folder
  folder="$(new_fixture id-token)"
  replace_line "$(workflow_of "$folder")" "      statuses: write" "      statuses: write\n      id-token: write"

  expect_failure "id-token permission" "$folder" "the id-token permission is not allowed"
}

test_other_runner_fails() {
  local folder
  folder="$(new_fixture runner)"
  replace_line "$(workflow_of "$folder")" "    runs-on: ubuntu-24.04" "    runs-on: ubuntu-latest"

  expect_failure "other runner" "$folder" "runs on ubuntu-latest"
}

test_container_fails() {
  local folder
  folder="$(new_fixture container)"
  replace_line "$(workflow_of "$folder")" "    runs-on: ubuntu-24.04" "    runs-on: ubuntu-24.04\n    container: ubuntu"

  expect_failure "container" "$folder" "sets container:"
}

test_services_fails() {
  local folder
  folder="$(new_fixture services)"
  replace_line "$(workflow_of "$folder")" "    runs-on: ubuntu-24.04" "    runs-on: ubuntu-24.04\n    services:\n      database:\n        image: postgres"

  expect_failure "services" "$folder" "sets services:"
}

test_continue_on_error_fails() {
  local folder
  folder="$(new_fixture continue-on-error)"
  replace_line "$(workflow_of "$folder")" "      - name: Run the job" "      - name: Run the job\n        continue-on-error: true"

  expect_failure "continue-on-error" "$folder" "continue-on-error is not allowed"
}

test_always_outside_gate_fails() {
  local folder
  folder="$(new_fixture always-outside-gate)"
  replace_line "$(workflow_of "$folder")" "      - name: Run the job" "      - name: Run the job\n        if: always()"

  expect_failure "always() outside the gate" "$folder" "allowed only in the gate job"
}

test_failure_outside_gate_fails() {
  local folder
  folder="$(new_fixture failure-outside-gate)"
  replace_line "$(workflow_of "$folder")" "    runs-on: ubuntu-24.04" "    if: failure()\n    runs-on: ubuntu-24.04"

  expect_failure "failure() outside the gate" "$folder" "allowed only in the gate job"
}

test_gate_without_success_check_fails() {
  local folder
  folder="$(new_fixture gate-no-success)"
  replace_line "$(workflow_of "$folder")" "          NEEDS: \${{ toJSON(needs) }}" "          NEEDS: none"

  expect_failure "gate without a success check" "$folder" "check every needs result for success"
}

test_gate_missing_a_need_fails() {
  local folder
  folder="$(new_fixture gate-missing-need)"
  replace_line "$(workflow_of "$folder")" "    needs: [backend-checks]" "    needs: []"

  expect_failure "gate missing a need" "$folder" "does not need backend-checks"
}

test_gate_without_always_fails() {
  local folder
  folder="$(new_fixture gate-no-always)"
  replace_line "$(workflow_of "$folder")" "    if: always()" "    if: success()"

  expect_failure "gate without always()" "$folder" "must run with if: always()"
}

test_inputs_inside_run_fails() {
  local folder
  folder="$(new_fixture inputs-in-run)"
  replace_line "$(workflow_of "$folder")" "          harness/tools/merge-gate.sh run-job backend-checks --repo candidate --base \"\$CANDIDATE\"" \
    "          harness/tools/merge-gate.sh run-job backend-checks --repo candidate --base \${{ inputs.sha }}"

  expect_failure "inputs inside run" "$folder" "inside run:"
}

test_event_inside_inline_run_fails() {
  local folder
  folder="$(new_fixture event-in-run)"
  replace_line "$(workflow_of "$folder")" "        run: |" "        run: echo \${{ github.event.inputs.sha }}"

  expect_failure "github.event inside an inline run" "$folder" "inside run:"
}

test_secrets_fail() {
  local folder
  folder="$(new_fixture secrets)"
  replace_line "$(workflow_of "$folder")" "          CANDIDATE: \${{ inputs.sha }}" "          CANDIDATE: \${{ secrets.TOKEN }}"

  expect_failure "secrets" "$folder" "secrets. is not allowed"
}

test_pull_request_target_fails() {
  local folder
  folder="$(new_fixture pull-request-target)"
  replace_line "$(workflow_of "$folder")" "  workflow_dispatch:" "  pull_request_target:\n  workflow_dispatch:"

  expect_failure "pull_request_target" "$folder" "pull_request_target is not allowed"
}

test_cache_action_fails() {
  local folder
  folder="$(new_fixture cache-action)"
  replace_line "$(workflow_of "$folder")" "      - name: Run the job" "      - name: Restore a cache\n        uses: actions/cache@$pin\n      - name: Run the job"

  expect_failure "cache action" "$folder" "actions/cache is not allowed"
}

test_setup_go_cache_on_fails() {
  local folder
  folder="$(new_fixture setup-go-cache)"
  replace_line "$(workflow_of "$folder")" "          cache: false" "          cache: true"

  expect_failure "setup-go cache on" "$folder" "actions/setup-go must set cache: false"
}

test_setup_go_cache_default_fails() {
  local folder
  folder="$(new_fixture setup-go-cache-default)"
  replace_line "$(workflow_of "$folder")" "          cache: false" ""

  expect_failure "setup-go cache left at its default" "$folder" "actions/setup-go must set cache: false"
}

test_setup_node_cache_input_fails() {
  local folder
  folder="$(new_fixture setup-node-cache)"
  replace_line "$(workflow_of "$folder")" "          package-manager-cache: false" "          package-manager-cache: false\n          cache: pnpm"

  expect_failure "setup-node cache input" "$folder" "actions/setup-node must not set cache:"
}

test_setup_node_package_manager_cache_on_fails() {
  local folder
  folder="$(new_fixture setup-node-package-manager-cache)"
  replace_line "$(workflow_of "$folder")" "          package-manager-cache: false" "          package-manager-cache: true"

  expect_failure "setup-node package-manager cache on" "$folder" "must set package-manager-cache: false"
}

test_gitleaks_job_without_full_history_fails() {
  local folder
  folder="$(new_fixture gitleaks-depth)"
  replace_line "$(workflow_of "$folder")" "          fetch-depth: 0" "          fetch-depth: 1"

  expect_failure "Gitleaks job without full history" "$folder" "without fetch-depth: 0"
}

test_candidate_tools_script_fails() {
  local folder
  folder="$(new_fixture candidate-tools)"
  replace_line "$(workflow_of "$folder")" "          harness/tools/merge-gate.sh run-job backend-checks --repo candidate --base \"\$CANDIDATE\"" \
    "          candidate/tools/merge-gate.sh run-job backend-checks --repo candidate --base \"\$CANDIDATE\""

  expect_failure "script from candidate/tools" "$folder" "runs a script from candidate/tools/"
}

test_gate_checkout_fails() {
  local folder
  folder="$(new_fixture gate-checkout)"
  replace_line "$(workflow_of "$folder")" "      - name: Require every result to be success" "      - name: Check out\n        uses: actions/checkout@$pin\n      - name: Require every result to be success"

  expect_failure "gate checkout" "$folder" "the gate job checks nothing out"
}

test_trailing_comment_in_workflow_fails() {
  local folder
  folder="$(new_fixture workflow-trailing-comment)"
  replace_line "$(workflow_of "$folder")" "permissions: {}" "permissions: {} #note"

  expect_failure "trailing comment in the workflow" "$folder" "comments are not allowed under .github"
}

test_comment_line_in_workflow_fails() {
  local folder
  folder="$(new_fixture workflow-comment-line)"
  replace_line "$(workflow_of "$folder")" "jobs:" "  #note\njobs:"

  expect_failure "comment line in the workflow" "$folder" "comments are not allowed under .github"
}

test_comment_in_issue_config_fails() {
  local folder
  folder="$(new_fixture issue-config-comment)"
  printf 'contact_links: [] #note\n' >>"$folder/.github/ISSUE_TEMPLATE/config.yml"

  expect_failure "comment in the issue form configuration" "$folder" "comments are not allowed under .github"
}

test_comment_in_markdown_fails() {
  local folder
  folder="$(new_fixture markdown-comment)"
  printf '<!-- note -->\n' >>"$folder/.github/pull_request_template.md"

  expect_failure "comment in the pull request template" "$folder" "comments are not allowed under .github"
}

test_dependabot_fails() {
  local folder
  folder="$(new_fixture dependabot)"
  printf 'version: 2\n' >"$folder/.github/dependabot.yml"

  expect_failure "Dependabot configuration" "$folder" "Dependabot is not allowed"
}

test_base_fixture_passes
test_committed_workflow_passes
test_other_workflow_file_fails
test_yaml_extension_fails
test_unpinned_action_fails
test_action_outside_allowlist_fails
test_reusable_workflow_fails
test_docker_action_fails
test_other_trigger_fails
test_missing_top_level_permissions_fails
test_write_permission_outside_gate_fails
test_id_token_permission_fails
test_other_runner_fails
test_container_fails
test_services_fails
test_continue_on_error_fails
test_always_outside_gate_fails
test_failure_outside_gate_fails
test_gate_without_success_check_fails
test_gate_missing_a_need_fails
test_gate_without_always_fails
test_inputs_inside_run_fails
test_event_inside_inline_run_fails
test_secrets_fail
test_pull_request_target_fails
test_cache_action_fails
test_setup_go_cache_on_fails
test_setup_go_cache_default_fails
test_setup_node_cache_input_fails
test_setup_node_package_manager_cache_on_fails
test_gitleaks_job_without_full_history_fails
test_candidate_tools_script_fails
test_gate_checkout_fails
test_trailing_comment_in_workflow_fails
test_comment_line_in_workflow_fails
test_comment_in_issue_config_fails
test_comment_in_markdown_fails
test_dependabot_fails

if [[ "$failures" -gt 0 ]]; then
  echo "check-workflows-test: $failures of $cases cases failed" >&2
  exit 1
fi
echo "check-workflows-test: all $cases cases passed"
