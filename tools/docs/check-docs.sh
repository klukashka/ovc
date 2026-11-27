#!/usr/bin/env bash
set -euo pipefail

repository="$(cd "$(dirname "$0")/../.." && pwd)"
expected_lychee="lychee 0.24.2"
tools="$repository/tools/docs"
cd "$repository"

documents="$(mktemp)"
work="$(mktemp -d)"
trap 'rm -rf "$documents" "$work"' EXIT

git ls-files --cached --others --exclude-standard -z -- '*.md' > "$documents"
test -s "$documents"

xargs -0 "$tools/node_modules/.bin/markdownlint" -- < "$documents"
installed_lychee="$(lychee --version)"
if [ "$installed_lychee" != "$expected_lychee" ]; then
  echo "expected $expected_lychee, found $installed_lychee; install the recorded version" >&2
  exit 1
fi
xargs -0 lychee --config lychee.toml -- < "$documents"

chromium="$(cd "$tools" && node --input-type=module -e "import { chromium } from 'playwright-core'; process.stdout.write(chromium.executablePath());")"
test -x "$chromium"
chromium_arguments='[]'
if [ "$(uname -s)" = Linux ]; then
  chromium_arguments='["--no-sandbox"]'
fi
printf '{"executablePath": "%s", "headless": "shell", "args": %s}\n' "$chromium" "$chromium_arguments" > "$work/puppeteer.json"

parsed=0
while IFS= read -r -d '' file; do
  grep -q '^```mermaid' "$file" || continue
  echo "parsing $file"
  "$tools/node_modules/.bin/mmdc" --quiet --puppeteerConfigFile "$work/puppeteer.json" \
    --input "$file" --output "$work/${file//\//_}" --artefacts "$work"
  parsed=$((parsed + 1))
done < "$documents"
test "$parsed" -gt 0
