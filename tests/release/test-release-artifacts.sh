#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist_dir="${repo_root}/dist"

VERSION="test-release" COMMIT="testcommit" DATE="2026-06-20T00:00:00Z" \
  "${repo_root}/scripts/build-release.sh"

for artifact in \
  motekarctl-linux-amd64 \
  motekar-panel-linux-amd64 \
  motekar-agent-linux-amd64 \
  install-ubuntu-24.04-amd64.sh
do
  test -s "${dist_dir}/${artifact}"
  test -s "${dist_dir}/${artifact}.sha256"
done

host_os="$(uname -s)"
host_arch="$(uname -m)"
if { [ "$host_os" = "Linux" ] && [ "$host_arch" = "x86_64" ]; } || { [ "$host_os" = "Linux" ] && [ "$host_arch" = "amd64" ]; }; then
  "${dist_dir}/motekarctl-linux-amd64" version | grep -q 'test-release'
fi

(
  cd "$dist_dir"
  for checksum in *.sha256; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum -c "$checksum"
    else
      shasum -a 256 -c "$checksum"
    fi
  done
)
