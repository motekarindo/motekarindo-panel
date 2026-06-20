#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
installer="${repo_root}/installers/install-ubuntu-24.04-amd64.sh"
fake_motekarctl="${repo_root}/tests/fixtures/fake-motekarctl.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

bash -n "$installer"

output="$(
  "$installer" \
    --dry-run \
    --skip-os-check \
    --skip-root-check \
    --local-binary "$fake_motekarctl" \
    --profile single-user \
    --web-server nginx \
    --postgresql install
)"

printf '%s\n' "$output" | grep -q 'fake-motekarctl preflight --profile single-user --postgresql install'
printf '%s\n' "$output" | grep -q 'fake-motekarctl install plan --profile single-user --web-server nginx --postgresql install'
printf '%s\n' "$output" | grep -q 'dry-run completed; no changes were made'

apply_output="${tmp_dir}/apply.out"
if "$installer" --apply --skip-os-check --skip-root-check --local-binary "$fake_motekarctl" >"$apply_output" 2>&1; then
  printf 'expected --apply to fail\n' >&2
  exit 1
fi

grep -q -- '--apply is not available yet' "$apply_output"
