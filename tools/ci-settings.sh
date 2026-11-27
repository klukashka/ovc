#!/usr/bin/env bash
set +o xtrace +o verbose
set -euo pipefail

usage() {
  echo "usage: tools/ci-settings.sh <candidate checkout>" >&2
  exit 2
}

fail() {
  echo "ci-settings: $*" >&2
  exit 1
}

[[ $# -eq 1 ]] || usage
repository="$1"
[[ -d "$repository" ]] || fail "$repository is not a folder"
[[ -n "${GITHUB_ENV:-}" && -f "$GITHUB_ENV" ]] || fail "GITHUB_ENV must name the job's environment file"
[[ "${RUNNER_DEBUG:-}" != 1 ]] || fail "step debugging is on and could print the generated settings; run the job without it"
command -v openssl >/dev/null 2>&1 || fail "openssl is required to generate the settings"

example_files=(".env.example" "backend/.env.example")
optional_empty_settings=" OVC_DB_POOL_SIZE OVC_FAKE_ROOM_ORIGIN OVC_SMTP_HOST OVC_SMTP_USERNAME OVC_SMTP_PASSWORD OVC_MAIL_FROM OVC_TRUSTED_PROXY_CIDRS "
database_host=127.0.0.1
database_name=ovc_dev
operator_name="CI operator"
operator_email=operator@example.test
generated_settings=" OVC_DEV_POSTGRES_PASSWORD OVC_DEV_STORAGE_ACCESS_KEY OVC_STORAGE_ACCESS_KEY OVC_DEV_STORAGE_SECRET_KEY OVC_STORAGE_SECRET_KEY OVC_DATABASE_URL OVC_TEST_DATABASE_ADMIN_URL OVC_JWT_SECRET OVC_FIELD_ENCRYPTION_KEY OVC_UNSUBSCRIBE_SIGNING_SECRET "

setting_names=()
setting_values=()

example_index() {
  local index=0
  while [[ "$index" -lt "${#setting_names[@]}" ]]; do
    if [[ "${setting_names[index]}" == "$1" ]]; then
      printf '%s' "$index"
      return 0
    fi
    index=$((index + 1))
  done
  return 1
}

read_example() {
  local relative="$1" file="$repository/$1" line_number=0 line name value
  [[ -f "$file" && ! -L "$file" ]] || fail "$relative is missing from the candidate"
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    [[ -z "${line//[[:space:]]/}" ]] && continue
    [[ "$line" =~ ^(OVC_[A-Z0-9_]+)=([^[:cntrl:]]*)$ ]] || fail "line $line_number of $relative is not OVC_KEY=VALUE"
    name="${BASH_REMATCH[1]}"
    value="${BASH_REMATCH[2]}"
    ! example_index "$name" >/dev/null || fail "line $line_number of $relative repeats $name"
    setting_names+=("$name")
    setting_values+=("$value")
  done <"$file"
}

example_value() {
  local index
  index="$(example_index "$1")" || fail "$1 is missing from the example files"
  printf '%s' "${setting_values[index]}"
}

for relative in "${example_files[@]}"; do
  read_example "$relative"
done

postgres_user="$(example_value OVC_DEV_POSTGRES_USER)"
postgres_port="$(example_value OVC_DEV_POSTGRES_PORT)"
[[ "$postgres_user" =~ ^[a-z_][a-z0-9_]{0,62}$ ]] || fail "OVC_DEV_POSTGRES_USER in the example file is not a plain role name"
[[ "$postgres_port" =~ ^[1-9][0-9]{0,4}$ ]] || fail "OVC_DEV_POSTGRES_PORT in the example file is not a port number"

postgres_password="$(openssl rand -hex 24)"
storage_access_key="$(openssl rand -hex 16)"
storage_secret_key="$(openssl rand -hex 32)"
jwt_secret="$(openssl rand -hex 32)"
field_key="$(openssl rand -base64 32)"
unsubscribe_key="$(openssl rand -base64 32)"
database_url="postgres://$postgres_user:$postgres_password@$database_host:$postgres_port/$database_name"

generated_values=(
  "$postgres_password"
  "$storage_access_key"
  "$storage_secret_key"
  "$jwt_secret"
  "$field_key"
  "$unsubscribe_key"
  "$postgres_user:$postgres_password"
  "$database_url"
)
for value in "${generated_values[@]}"; do
  [[ -n "$value" && "$value" != *[[:cntrl:]]* ]] || fail "openssl produced an unusable value"
  printf '::add-mask::%s\n' "$value"
done

value_for() {
  local name="$1" value
  case "$name" in
    OVC_DEV_POSTGRES_PASSWORD) printf '%s' "$postgres_password" ;;
    OVC_DEV_STORAGE_ACCESS_KEY | OVC_STORAGE_ACCESS_KEY) printf '%s' "$storage_access_key" ;;
    OVC_DEV_STORAGE_SECRET_KEY | OVC_STORAGE_SECRET_KEY) printf '%s' "$storage_secret_key" ;;
    OVC_DATABASE_URL | OVC_TEST_DATABASE_ADMIN_URL) printf '%s' "$database_url" ;;
    OVC_JWT_SECRET) printf '%s' "$jwt_secret" ;;
    OVC_FIELD_ENCRYPTION_KEY) printf '%s' "$field_key" ;;
    OVC_UNSUBSCRIBE_SIGNING_SECRET) printf '%s' "$unsubscribe_key" ;;
    OVC_OPERATOR_NAME) printf '%s' "$operator_name" ;;
    OVC_OPERATOR_EMAIL) printf '%s' "$operator_email" ;;
    *)
      value="$(example_value "$name")" || exit 1
      [[ "$value" != *"<"* && "$value" != *">"* ]] ||
        fail "$name holds a placeholder in the example file; teach tools/ci-settings.sh to generate it"
      [[ -n "$value" || "$optional_empty_settings" == *" $name "* ]] ||
        fail "$name is empty in the example file; teach tools/ci-settings.sh to generate it"
      printf '%s' "$value"
      ;;
  esac
}

lines=()
written_generated=()
for name in "${setting_names[@]}"; do
  value="$(value_for "$name")"
  lines+=("$name=$value")
  if [[ "$generated_settings" == *" $name "* ]]; then
    written_generated+=("$name")
  fi
done
[[ ${#written_generated[@]} -gt 0 ]] || fail "the example files name none of the generated settings"
lines+=("CI_GENERATED_SETTING_NAMES=${written_generated[*]}")
printf '%s\n' "${lines[@]}" >>"$GITHUB_ENV"
echo "ci-settings: wrote ${#setting_names[@]} settings to the job environment: ${setting_names[*]}"
echo "ci-settings: evidence redaction reads the generated settings from CI_GENERATED_SETTING_NAMES: ${written_generated[*]}"
