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
run-name: gate ${{ inputs.scope }} ${{ inputs.sha }}

on:
  workflow_dispatch:
    inputs:
      sha:
        required: true
        type: string
      scope:
        required: true
        type: choice
        options:
          - full
          - docs
          - change
          - evidence
      jobs:
        type: string
      kind:
        type: choice
        options:
          - none
          - go
          - playwright
          - check
      package:
        type: string
      test:
        type: string
      project:
        type: string
      title:
        type: string
      entry:
        type: string
      series:
        type: choice
        options:
          - none
          - before-first
          - before-rest
          - after

permissions: {}

jobs:
  validate:
    runs-on: ubuntu-24.04
    permissions:
      contents: read
    steps:
      - name: Validate the inputs
        env:
          TEST: ${{ inputs.test }}
          TITLE: ${{ inputs.title }}
        run: |
          [[ "$TEST" =~ ^Test[A-Za-z0-9_/]+$ ]]
          python3 -I -c 'import os, sys; title_limit = 200; sys.exit(len(os.environ["TITLE"]) > title_limit)'

  prepare:
    needs: validate
    runs-on: ubuntu-24.04
    permissions:
      contents: read
    outputs:
      scope: ${{ steps.matrix.outputs.scope }}
      jobs: ${{ steps.matrix.outputs.jobs }}
      e2e-shards: ${{ steps.matrix.outputs.e2e-shards }}
    steps:
      - name: Choose the jobs and shards
        id: matrix
        run: |
          harness/tools/merge-gate.sh ci-matrix --scope full

  backend-checks:
    needs: prepare
    if: contains(fromJSON(needs.prepare.outputs.jobs), 'backend-checks')
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
    needs: [validate, prepare, backend-checks]
    if: always()
    runs-on: ubuntu-24.04
    permissions:
      statuses: write
    steps:
      - name: Require every result to be success
        env:
          NEEDS: ${{ toJSON(needs) }}
          SCOPE: ${{ needs.prepare.outputs.scope }}
          CANDIDATE: ${{ inputs.sha }}
        run: |
          jq -e 'all(.[]; .result == "success")' <<<"$NEEDS"
          if [[ "$SCOPE" == full || "$SCOPE" == docs ]]; then
            gh api --method POST "repos/owner/repository/statuses/$CANDIDATE" -f state=success -f context=ovc-gate -f description="gate $SCOPE"
          fi
YAML

IFS= read -r -d '' e2e_job <<'YAML' || true
  e2e:
    needs: prepare
    if: needs.prepare.outputs.e2e-shards != '' && needs.prepare.outputs.e2e-shards != '[]'
    runs-on: ubuntu-24.04
    permissions:
      contents: read
    strategy:
      fail-fast: false
      matrix:
        shard: ${{ fromJSON(needs.prepare.outputs.e2e-shards) }}
    steps:
      - name: Run the shard
        env:
          SHARD: ${{ matrix.shard }}
        run: |
          harness/tools/merge-gate.sh run-job e2e --shard "$SHARD/12" --repo candidate

YAML

IFS= read -r -d '' evidence_job <<'YAML' || true
  evidence:
    needs: prepare
    if: needs.prepare.outputs.scope == 'evidence'
    runs-on: ubuntu-24.04
    permissions:
      contents: read
    steps:
      - name: Run the series
        env:
          TITLE: ${{ inputs.title }}
        run: |
          pnpm e2e --retries=0 -g "$TITLE" --reporter=list

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

add_job() {
  local folder="$1" name="$2" content="$3" file
  file="$(workflow_of "$folder")"
  printf '%s' "$content" >"$folder/job.yml"
  awk -v job_file="$folder/job.yml" '
    !done && $0 == "  gate:" {
      while ((getline line < job_file) > 0) print line
      done = 1
    }
    { print }
    END { if (!done) exit 3 }
  ' "$file" >"$file.next"
  mv "$file.next" "$file"
  rm -f "$folder/job.yml"
  replace_line "$file" "    needs: [validate, prepare, backend-checks]" "    needs: [validate, prepare, backend-checks, $name]"
}

job_keys_of() {
  awk '
    /^jobs:/ { in_jobs = 1; next }
    in_jobs && /^[^ ]/ { in_jobs = 0 }
    in_jobs && /^  [A-Za-z0-9_-]+:$/ { name = $0; sub(/^  /, "", name); sub(/:$/, "", name); print name }
  ' "$1" | LC_ALL=C sort | paste -sd ' ' -
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

test_committed_jobs_pattern_accepts_digits_and_shards() {
  local pattern value verdicts=""
  pattern="$(sed -n 's/^.*"[$]JOBS" =~ \(.\{1,\}\) ]] ||$/\1/p' "$repository/.github/workflows/gate.yml")"
  cases=$((cases + 1))

  for value in e2e:3 backend-tests:2,e2e:11 backend-checks E2E:3 'e2e;x' e2e:3,; do
    if [[ -n "$pattern" && "$value" =~ $pattern ]]; then
      verdicts="$verdicts $value=accepted"
    else
      verdicts="$verdicts $value=refused"
    fi
  done

  if [[ "$verdicts" != " e2e:3=accepted backend-tests:2,e2e:11=accepted backend-checks=accepted E2E:3=refused e2e;x=refused e2e:3,=refused" ]]; then
    printf "FAIL: jobs pattern: gate.yml's validate pattern [%s] gives%s\n" "$pattern" "$verdicts" >&2
    failures=$((failures + 1))
  fi
}

test_corepack_without_global_install_fails() {
  local folder
  folder="$(new_fixture corepack-without-install)"
  replace_line "$(workflow_of "$folder")" '          harness/tools/merge-gate.sh run-job backend-checks --repo candidate --base "$CANDIDATE"' \
    '          corepack enable\n          harness/tools/merge-gate.sh run-job backend-checks --repo candidate --base "$CANDIDATE"'

  expect_failure "corepack without the global install" "$folder" "runs corepack enable without corepack install --global"
}

test_corepack_with_global_install_passes() {
  local folder
  folder="$(new_fixture corepack-with-install)"
  replace_line "$(workflow_of "$folder")" '          harness/tools/merge-gate.sh run-job backend-checks --repo candidate --base "$CANDIDATE"' \
    '          corepack enable\n          corepack install --global "$package_manager"\n          harness/tools/merge-gate.sh run-job backend-checks --repo candidate --base "$CANDIDATE"'

  expect_pass "corepack with the global install" "$folder"
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
  replace_line "$(workflow_of "$folder")" "    needs: [validate, prepare, backend-checks]" "    needs: [validate, prepare]"

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

test_substring_selection_fails() {
  local folder
  folder="$(new_fixture substring-selection)"
  replace_line "$(workflow_of "$folder")" "    if: contains(fromJSON(needs.prepare.outputs.jobs), 'backend-checks')" \
    "    if: contains(needs.prepare.outputs.jobs, 'backend-checks')"

  expect_failure "selection by substring" "$folder" "must select itself by exact membership"
}

test_selection_by_other_name_fails() {
  local folder
  folder="$(new_fixture other-name-selection)"
  replace_line "$(workflow_of "$folder")" "    if: contains(fromJSON(needs.prepare.outputs.jobs), 'backend-checks')" \
    "    if: contains(fromJSON(needs.prepare.outputs.jobs), 'backend')"

  expect_failure "selection by another name" "$folder" "must select itself by exact membership"
}

test_status_from_change_fails() {
  local folder
  folder="$(new_fixture status-from-change)"
  replace_line "$(workflow_of "$folder")" '          if [[ "$SCOPE" == full || "$SCOPE" == docs ]]; then' \
    '          if [[ "$SCOPE" == full || "$SCOPE" == docs || "$SCOPE" == change ]]; then'

  expect_failure "status from a change run" "$folder" "only under a guard naming exactly the full and docs scopes"
}

test_unguarded_status_fails() {
  local folder
  folder="$(new_fixture unguarded-status)"
  replace_line "$(workflow_of "$folder")" '          if [[ "$SCOPE" == full || "$SCOPE" == docs ]]; then' '          if true; then'

  expect_failure "status without a scope guard" "$folder" "only under a guard naming exactly the full and docs scopes"
}

test_scoped_status_context_fails() {
  local folder
  folder="$(new_fixture scoped-context)"
  replace_line "$(workflow_of "$folder")" \
    '            gh api --method POST "repos/owner/repository/statuses/$CANDIDATE" -f state=success -f context=ovc-gate -f description="gate $SCOPE"' \
    '            gh api --method POST "repos/owner/repository/statuses/$CANDIDATE" -f state=success -f context=ovc-gate/full -f description="gate $SCOPE"'

  expect_failure "status context with a scope suffix" "$folder" "the single context ovc-gate"
}

test_status_outside_gate_fails() {
  local folder
  folder="$(new_fixture status-outside-gate)"
  replace_line "$(workflow_of "$folder")" "          harness/tools/merge-gate.sh run-job backend-checks --repo candidate --base \"\$CANDIDATE\"" \
    "          gh api --method POST \"repos/owner/repository/statuses/\$CANDIDATE\" -f state=success -f context=ovc-gate"

  expect_failure "status outside the gate job" "$folder" "only the gate job posts a commit status"
}

test_title_in_run_name_fails() {
  local folder
  folder="$(new_fixture title-in-run-name)"
  replace_line "$(workflow_of "$folder")" 'run-name: gate ${{ inputs.scope }} ${{ inputs.sha }}' \
    'run-name: gate ${{ inputs.scope }} ${{ inputs.sha }} ${{ inputs.title }}'

  expect_failure "title in run-name" "$folder" "run-name names inputs.title"
}

test_title_outside_env_fails() {
  local folder
  folder="$(new_fixture title-outside-env)"
  replace_line "$(workflow_of "$folder")" "      - name: Validate the inputs" "      - name: Show the title\n        if: inputs.title != ''\n        run: echo title\n      - name: Validate the inputs"

  expect_failure "title outside an env binding" "$folder" "inputs.title may appear only as an env: binding"
}

test_title_in_other_job_fails() {
  local folder
  folder="$(new_fixture title-other-job)"
  replace_line "$(workflow_of "$folder")" "          CANDIDATE: \${{ inputs.sha }}" "          CANDIDATE: \${{ inputs.sha }}\n          TITLE: \${{ inputs.title }}"

  expect_failure "title read by another job" "$folder" "only the validate and evidence jobs may read inputs.title"
}

test_title_limit_missing_fails() {
  local folder
  folder="$(new_fixture title-limit)"
  replace_line "$(workflow_of "$folder")" \
    "          python3 -I -c 'import os, sys; title_limit = 200; sys.exit(len(os.environ[\"TITLE\"]) > title_limit)'" \
    "          python3 -I -c 'import os, sys; sys.exit(len(os.environ[\"TITLE\"]) > 500)'"

  expect_failure "title without its 200-character limit" "$folder" "limit the title input to 200 characters"
}

test_test_pattern_missing_fails() {
  local folder
  folder="$(new_fixture test-pattern)"
  replace_line "$(workflow_of "$folder")" '          [[ "$TEST" =~ ^Test[A-Za-z0-9_/]+$ ]]' '          [[ "$TEST" =~ ^Test.+$ ]]'

  expect_failure "test input without its pattern" "$folder" "against ^Test[A-Za-z0-9_/]+\$"
}

test_unknown_input_fails() {
  local folder
  folder="$(new_fixture unknown-input)"
  replace_line "$(workflow_of "$folder")" "      entry:" "      command:\n        type: string\n      entry:"

  expect_failure "unknown input" "$folder" "the input command is not one of the gate's inputs"
}

test_scope_option_missing_fails() {
  local folder
  folder="$(new_fixture scope-options)"
  replace_line "$(workflow_of "$folder")" "          - evidence" ""

  expect_failure "scope without the evidence option" "$folder" "the input scope must be a choice"
}

test_e2e_job_passes() {
  local folder
  folder="$(new_fixture e2e-job)"
  add_job "$folder" e2e "$e2e_job"

  expect_pass "e2e matrix job" "$folder"
}

test_e2e_other_shard_total_fails() {
  local folder
  folder="$(new_fixture e2e-shard-total)"
  add_job "$folder" e2e "$e2e_job"
  replace_line "$(workflow_of "$folder")" '          harness/tools/merge-gate.sh run-job e2e --shard "$SHARD/12" --repo candidate' \
    '          harness/tools/merge-gate.sh run-job e2e --shard "$SHARD/4" --repo candidate'

  expect_failure "e2e shard total other than 12" "$folder" "must run run-job e2e --shard \"\$SHARD/12\""
}

test_e2e_project_choice_fails() {
  local folder
  folder="$(new_fixture e2e-project)"
  add_job "$folder" e2e "$e2e_job"
  replace_line "$(workflow_of "$folder")" '          harness/tools/merge-gate.sh run-job e2e --shard "$SHARD/12" --repo candidate' \
    '          harness/tools/merge-gate.sh run-job e2e --shard "$SHARD/12" --repo candidate --project chromium'

  expect_failure "e2e job choosing a project" "$folder" "must not choose Playwright projects"
}

test_literal_matrix_fails() {
  local folder
  folder="$(new_fixture literal-matrix)"
  add_job "$folder" e2e "$e2e_job"
  replace_line "$(workflow_of "$folder")" '        shard: ${{ fromJSON(needs.prepare.outputs.e2e-shards) }}' '        shard: [1, 2, 3]'

  expect_failure "literal matrix" "$folder" "the matrix of e2e must be \${{ fromJSON(needs.prepare.outputs.<list>) }}"
}

test_matrix_without_guard_fails() {
  local folder
  folder="$(new_fixture matrix-guard)"
  add_job "$folder" e2e "$e2e_job"
  replace_line "$(workflow_of "$folder")" "    if: needs.prepare.outputs.e2e-shards != '' && needs.prepare.outputs.e2e-shards != '[]'" \
    "    if: needs.prepare.outputs.scope == 'full'"

  expect_failure "matrix job without the empty-list guard" "$folder" "must have the job-level if: needs.prepare.outputs.e2e-shards != ''"
}

test_evidence_job_passes() {
  local folder
  folder="$(new_fixture evidence-job)"
  add_job "$folder" evidence "$evidence_job"

  expect_pass "evidence job" "$folder"
}

test_evidence_upload_fails() {
  local folder
  folder="$(new_fixture evidence-upload)"
  add_job "$folder" evidence "$evidence_job"
  replace_line "$(workflow_of "$folder")" "      - name: Run the series" "      - name: Upload traces\n        uses: actions/upload-artifact@$pin\n      - name: Run the series"

  expect_failure "evidence job uploading artifacts" "$folder" "the evidence job uploads no traces or artifacts"
}

test_evidence_selection_fails() {
  local folder
  folder="$(new_fixture evidence-selection)"
  add_job "$folder" evidence "$evidence_job"
  replace_line "$(workflow_of "$folder")" "    if: needs.prepare.outputs.scope == 'evidence'" "    if: needs.prepare.outputs.scope != 'docs'"

  expect_failure "evidence job selected by another scope" "$folder" "the evidence job must have the job-level if"
}

test_job_keys_match_merge_gate() {
  local full docs expected actual status=0
  cases=$((cases + 1))

  full="$(bash "$tools/merge-gate.sh" ci-matrix --scope full 2>&1)" || status=$?
  docs="$(bash "$tools/merge-gate.sh" ci-matrix --scope docs 2>&1)" || status=$?
  if [[ "$status" -eq 0 ]]; then
    expected="$(python3 -I -c '
import json
import sys

names = {"validate", "prepare", "evidence", "gate"}
for text in sys.argv[1:]:
    selection = json.loads(text)
    for key in ("backend-tests", "e2e"):
        if selection[key]:
            names.add(key)
    names.update(selection["jobs"])
print(" ".join(sorted(names)))
full = json.loads(sys.argv[1])
print(len(full["backend-tests"]), len(full["e2e"]))
' "$full" "$docs" 2>&1)" || status=$?
  fi
  actual="$(job_keys_of "$repository/.github/workflows/gate.yml")"

  if [[ "$status" -ne 0 ]]; then
    printf 'FAIL: job keys: merge-gate.sh ci-matrix gave no usable selection:\n%s\n%s\n%s\n' "$full" "${docs:-}" "${expected:-}" >&2
    failures=$((failures + 1))
  elif [[ "$(sed -n 1p <<<"$expected")" != "$actual" ]]; then
    printf "FAIL: job keys: gate.yml's job keys [%s] differ from merge-gate.sh's expected sets [%s]\n" "$actual" "$(sed -n 1p <<<"$expected")" >&2
    failures=$((failures + 1))
  elif ! grep -qF "run-job backend-tests --shard \"\$SHARD/$(sed -n 2p <<<"$expected" | cut -d ' ' -f 1)\"" "$repository/.github/workflows/gate.yml" ||
    ! grep -qF "run-job e2e --shard \"\$SHARD/$(sed -n 2p <<<"$expected" | cut -d ' ' -f 2)\"" "$repository/.github/workflows/gate.yml"; then
    printf "FAIL: job keys: gate.yml's shard totals differ from merge-gate.sh ci-matrix's full lists (%s)\n" "$(sed -n 2p <<<"$expected")" >&2
    failures=$((failures + 1))
  fi
}

test_base_fixture_passes
test_committed_workflow_passes
test_committed_jobs_pattern_accepts_digits_and_shards
test_corepack_without_global_install_fails
test_corepack_with_global_install_passes
test_job_keys_match_merge_gate
test_substring_selection_fails
test_selection_by_other_name_fails
test_status_from_change_fails
test_unguarded_status_fails
test_scoped_status_context_fails
test_status_outside_gate_fails
test_title_in_run_name_fails
test_title_outside_env_fails
test_title_in_other_job_fails
test_title_limit_missing_fails
test_test_pattern_missing_fails
test_unknown_input_fails
test_scope_option_missing_fails
test_e2e_job_passes
test_e2e_other_shard_total_fails
test_e2e_project_choice_fails
test_literal_matrix_fails
test_matrix_without_guard_fails
test_evidence_job_passes
test_evidence_upload_fails
test_evidence_selection_fails
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
