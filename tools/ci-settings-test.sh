#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/tools/ci-settings.sh"
work="$(mktemp -d "${TMPDIR:-/tmp}/ci-settings-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
example_files=("$root/.env.example" "$root/backend/.env.example")
optional_empty=" OVC_DB_POOL_SIZE OVC_FAKE_ROOM_ORIGIN OVC_SMTP_HOST OVC_SMTP_USERNAME OVC_SMTP_PASSWORD OVC_MAIL_FROM OVC_TRUSTED_PROXY_CIDRS "
expected_generated="OVC_DATABASE_URL OVC_DEV_POSTGRES_PASSWORD OVC_DEV_STORAGE_ACCESS_KEY OVC_DEV_STORAGE_SECRET_KEY OVC_FIELD_ENCRYPTION_KEY OVC_JWT_SECRET OVC_STORAGE_ACCESS_KEY OVC_STORAGE_SECRET_KEY OVC_TEST_DATABASE_ADMIN_URL OVC_UNSUBSCRIBE_SIGNING_SECRET"
failures=0
cases=0

check() {
  cases=$((cases + 1))
  if [[ "$1" != "$2" ]]; then
    echo "FAIL: $3: got [$1], want [$2]" >&2
    failures=$((failures + 1))
    return 0
  fi
  echo "ok: $3"
}

run_settings() {
  local environment_file="$1" debug="$2" candidate="${3:-$root}"
  : >"$environment_file"
  status=0
  env -u RUNNER_DEBUG GITHUB_ENV="$environment_file" ${debug:+RUNNER_DEBUG="$debug"} bash "$script" "$candidate" \
    >"$work/stdout" 2>"$work/stderr" || status=$?
}

new_candidate() {
  local folder="$work/$1"
  mkdir -p "$folder/backend"
  cp "${example_files[0]}" "$folder/.env.example"
  cp "${example_files[1]}" "$folder/backend/.env.example"
  printf '%s' "$folder"
}

replace_example_line() {
  local file="$1" old="$2" new="$3"
  awk -v old="$old" -v new="$new" '
    !done && $0 == old { print new; done = 1; next }
    { print }
    END { if (!done) exit 3 }
  ' "$file" >"$file.next"
  mv "$file.next" "$file"
}

generated_names_of() {
  setting_value "$1" CI_GENERATED_SETTING_NAMES | tr ' ' '\n' | LC_ALL=C sort | paste -sd ' ' -
}

written_names() {
  sed -n 's/^\(OVC_[A-Z0-9_]*\)=.*$/\1/p' "$1" | LC_ALL=C sort | paste -sd ' ' -
}

example_names() {
  sed -n 's/^\(OVC_[A-Z0-9_]*\)=.*$/\1/p' "${example_files[@]}" | LC_ALL=C sort | paste -sd ' ' -
}

setting_value() {
  sed -n "s/^$2=//p" "$1"
}

empty_required_settings() {
  local name value
  while IFS='=' read -r name value; do
    [[ "$name" == OVC_* ]] || continue
    if [[ -z "$value" && "$optional_empty" != *" $name "* ]]; then
      printf '%s ' "$name"
    fi
  done <"$1"
}

placeholder_settings() {
  local name value
  while IFS='=' read -r name value; do
    if [[ "$value" == *"<"* || "$value" == *">"* ]]; then
      printf '%s ' "$name"
    fi
  done <"$1"
}

unmasked_values() {
  local environment_file="$1" name value
  for name in $(setting_value "$environment_file" CI_GENERATED_SETTING_NAMES); do
    value="$(setting_value "$environment_file" "$name")"
    if ! grep -qxF "::add-mask::$value" "$work/stdout"; then
      printf '%s ' "$name"
    fi
  done
}

value_lines_before_masks() {
  awk '
    /^::add-mask::/ { if (seen_other) late++; next }
    { seen_other = 1 }
    END { print late + 0 }
  ' "$work/stdout"
}

printed_values() {
  local environment_file="$1" name value
  for name in $(setting_value "$environment_file" CI_GENERATED_SETTING_NAMES); do
    value="$(setting_value "$environment_file" "$name")"
    if grep -v '^::add-mask::' "$work/stdout" "$work/stderr" | grep -qF -- "$value"; then
      printf '%s ' "$name"
    fi
  done
}

test_writes_every_example_setting() {
  local environment_file="$work/written-env"

  run_settings "$environment_file" ""

  check "$status" "0" "ci-settings exits 0 for the repository's example files"
  check "$(written_names "$environment_file")" "$(example_names)" "ci-settings writes every OVC_ name of the example files exactly once"
  check "$(empty_required_settings "$environment_file")" "" "ci-settings leaves no setting empty except the optional ones"
  check "$(placeholder_settings "$environment_file")" "" "ci-settings leaves no placeholder in any value"
  check "$(setting_value "$environment_file" CI_GENERATED_SETTING_NAMES | tr ' ' '\n' | LC_ALL=C sort | paste -sd ' ' -)" \
    "$expected_generated" "ci-settings exports the names of the generated settings for the evidence redaction"
}

test_masks_before_printing() {
  local environment_file="$work/masked-env" user password

  run_settings "$environment_file" ""

  user="$(setting_value "$environment_file" OVC_DEV_POSTGRES_USER)"
  password="$(setting_value "$environment_file" OVC_DEV_POSTGRES_PASSWORD)"
  check "$(unmasked_values "$environment_file")" "" "ci-settings masks every generated value"
  check "$(grep -cxF "::add-mask::$user:$password" "$work/stdout")" "1" "ci-settings masks the database user information"
  check "$(value_lines_before_masks)" "0" "ci-settings prints every mask line before any other output"
  check "$(printed_values "$environment_file")" "" "ci-settings prints no generated value outside its mask lines"
}

test_refuses_step_debugging() {
  local environment_file="$work/debug-env"

  run_settings "$environment_file" 1

  check "$status" "1" "ci-settings refuses to run with RUNNER_DEBUG=1"
  check "$(wc -c <"$environment_file" | tr -d ' ')" "0" "ci-settings writes nothing to the job environment under step debugging"
  check "$(grep -c '^::add-mask::' "$work/stdout" || true)" "0" "ci-settings prints no mask line under step debugging"
}

test_generates_a_new_placeholder_setting() {
  local environment_file="$work/placeholder-env" candidate key
  candidate="$(new_candidate placeholder)"
  printf 'OVC_NEW_SIGNING_KEY=<signing-key>\n' >>"$candidate/backend/.env.example"

  run_settings "$environment_file" "" "$candidate"

  key="$(setting_value "$environment_file" OVC_NEW_SIGNING_KEY)"
  check "$status" "0" "ci-settings exits 0 for a candidate that adds a placeholder setting"
  check "$([[ "$key" =~ ^[0-9a-f]{64}$ ]] && echo hex || echo "$key")" "hex" "ci-settings generates 32 random bytes in hex for a placeholder of an unknown format"
  check "$(generated_names_of "$environment_file")" "$(printf '%s\n' $expected_generated OVC_NEW_SIGNING_KEY | LC_ALL=C sort | paste -sd ' ' -)" \
    "ci-settings lists the new generated setting for the evidence redaction"
  check "$(unmasked_values "$environment_file")" "" "ci-settings masks the new generated setting"
  check "$(value_lines_before_masks)" "0" "ci-settings prints the new setting's mask before any other output"
  check "$(printed_values "$environment_file")" "" "ci-settings prints the new generated setting only in its mask line"
}

test_fills_the_password_placeholder_of_a_new_url() {
  local environment_file="$work/password-env" candidate
  candidate="$(new_candidate password)"
  printf 'OVC_REPLICA_DATABASE_URL=postgres://ovc:<password>@127.0.0.1:55432/ovc_dev\n' >>"$candidate/backend/.env.example"

  run_settings "$environment_file" "" "$candidate"

  check "$status" "0" "ci-settings exits 0 for a candidate that adds a URL with the password placeholder"
  check "$(setting_value "$environment_file" OVC_REPLICA_DATABASE_URL)" "$(setting_value "$environment_file" OVC_DATABASE_URL)" \
    "ci-settings fills the password placeholder with the generated database password"
  check "$(unmasked_values "$environment_file")" "" "ci-settings masks the derived URL"
}

test_refuses_a_reserved_harness_name() {
  local environment_file="$work/reserved-env" candidate
  candidate="$(new_candidate reserved)"
  printf 'E2E_HARNESS_RUN_OUTPUT=run-1\n' >>"$candidate/backend/.env.example"

  run_settings "$environment_file" "" "$candidate"

  check "$status" "1" "ci-settings refuses an example file that sets a name the harness reserves"
  check "$(grep -c 'E2E_HARNESS_RUN_OUTPUT, which the CI harness reserves' "$work/stderr" || true)" "1" "ci-settings names the reserved setting"
  check "$(wc -c <"$environment_file" | tr -d ' ')" "0" "ci-settings writes nothing for a reserved name"
  check "$(grep -c '^::add-mask::' "$work/stdout" || true)" "0" "ci-settings prints no mask line for a reserved name"
}

test_refuses_a_changed_test_control_setting() {
  local environment_file="$work/test-control-env" candidate
  candidate="$(new_candidate test-control)"
  replace_example_line "$candidate/backend/.env.example" "OVC_TEST_CONTROL=1" "OVC_TEST_CONTROL=0"

  run_settings "$environment_file" "" "$candidate"

  check "$status" "1" "ci-settings refuses an example file that changes OVC_TEST_CONTROL"
  check "$(wc -c <"$environment_file" | tr -d ' ')" "0" "ci-settings writes nothing for a changed OVC_TEST_CONTROL"
}

test_refuses_an_empty_new_setting() {
  local environment_file="$work/empty-env" candidate
  candidate="$(new_candidate empty)"
  printf 'OVC_NEW_REQUIRED_SETTING=\n' >>"$candidate/backend/.env.example"

  run_settings "$environment_file" "" "$candidate"

  check "$status" "1" "ci-settings refuses an empty new setting that is not optional"
  check "$(grep -c 'write its value as a placeholder' "$work/stderr" || true)" "1" "ci-settings asks for a placeholder instead"
}

test_writes_every_example_setting
test_masks_before_printing
test_refuses_step_debugging
test_generates_a_new_placeholder_setting
test_fills_the_password_placeholder_of_a_new_url
test_refuses_a_reserved_harness_name
test_refuses_a_changed_test_control_setting
test_refuses_an_empty_new_setting

echo "$((cases - failures)) passed, $failures failed"
[[ "$failures" -eq 0 ]]
