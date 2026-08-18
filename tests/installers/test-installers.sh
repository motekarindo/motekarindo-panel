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

mkdir -p "${tmp_dir}/bin"
cp "$fake_motekarctl" "${tmp_dir}/bin/motekarctl-linux-amd64"
cp "$fake_motekarctl" "${tmp_dir}/bin/motekar-panel-linux-amd64"
cp "$fake_motekarctl" "${tmp_dir}/bin/motekar-agent-linux-amd64"

apply_output="${tmp_dir}/apply.out"
apply_err="${tmp_dir}/apply.err"
"$installer" \
  --apply \
  --skip-os-check \
  --skip-root-check \
  --local-binary-dir "${tmp_dir}/bin" \
  --bin-dir "${tmp_dir}/deploy" \
  --profile single-user \
  --web-server nginx \
  --postgresql install \
  --admin-email owner@example.com \
  --admin-display-name "Owner" \
  --admin-password "correct-horse-battery" \
  >"$apply_output" 2>"$apply_err"

grep -q 'fake-motekarctl install apply --profile single-user --web-server nginx --postgresql install --bin-dir' "$apply_output"
grep -q -- '--admin-email owner@example.com' "$apply_output"
grep -q -- '--admin-password-stdin' "$apply_output"
grep -q 'Motekar Panel installed' "$apply_output"
test -x "${tmp_dir}/deploy/motekarctl"
test -x "${tmp_dir}/deploy/motekar-panel"
test -x "${tmp_dir}/deploy/motekar-agent"

external_output="${tmp_dir}/external.out"
if "$installer" \
  --apply \
  --skip-os-check \
  --skip-root-check \
  --local-binary-dir "${tmp_dir}/bin" \
  --bin-dir "${tmp_dir}/deploy" \
  --profile single-user \
  --web-server nginx \
  --postgresql external \
  --admin-email owner@example.com \
  --admin-display-name "Owner" \
  --admin-password "correct-horse-battery" \
  >"$external_output" 2>&1; then
  printf 'expected external PostgreSQL without MOTEKAR_DATABASE_URL to fail\n' >&2
  exit 1
fi
grep -q 'requires MOTEKAR_DATABASE_URL' "$external_output"

download_www="${tmp_dir}/www"
download_deploy="${tmp_dir}/deploy2"
mkdir -p "$download_www"
for name in motekarctl-linux-amd64 motekar-panel-linux-amd64 motekar-agent-linux-amd64; do
  cp "$fake_motekarctl" "${download_www}/${name}"
  chmod +x "${download_www}/${name}"
  (
    cd "$download_www"
    if command -v sha256sum >/dev/null 2>&1; then
      checksum_cmd="sha256sum"
    else
      checksum_cmd="shasum -a 256"
    fi
    $checksum_cmd "$name" > "$name.sha256"
  )
done
python3 -m http.server 18923 --directory "$download_www" >/dev/null 2>&1 &
server_pid=$!
trap 'rm -rf "$tmp_dir"; kill "$server_pid" 2>/dev/null' EXIT
sleep 1

download_output="${tmp_dir}/download.out"
"$installer" \
  --apply \
  --skip-os-check \
  --skip-root-check \
  --download-url "http://127.0.0.1:18923" \
  --bin-dir "$download_deploy" \
  --profile single-user \
  --web-server nginx \
  --postgresql install \
  --admin-email owner@example.com \
  --admin-display-name "Owner" \
  --admin-password "correct-horse-battery" \
  >"$download_output" 2>&1

grep -q 'Motekar Panel installed' "$download_output"
test -x "${download_deploy}/motekarctl"
test -x "${download_deploy}/motekar-panel"
test -x "${download_deploy}/motekar-agent"

printf 'all installer tests passed\n'
