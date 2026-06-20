#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="${repo_root}/dist"

version="${VERSION:-}"
commit="${COMMIT:-}"
date="${DATE:-}"

if [ -z "$version" ]; then
  version="$(git -C "$repo_root" describe --tags --always --dirty 2>/dev/null || printf 'dev')"
fi
if [ -z "$commit" ]; then
  commit="$(git -C "$repo_root" rev-parse --short HEAD 2>/dev/null || printf 'unknown')"
fi
if [ -z "$date" ]; then
  date="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
fi

rm -rf "$dist_dir"
mkdir -p "$dist_dir"

ldflags="-s -w -X github.com/motekar/motekar-panel/internal/buildinfo.Version=${version} -X github.com/motekar/motekar-panel/internal/buildinfo.Commit=${commit} -X github.com/motekar/motekar-panel/internal/buildinfo.Date=${date}"

build_binary() {
  local package="$1"
  local output="$2"

  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOCACHE="${GOCACHE:-${repo_root}/.cache/go-build}" \
    go build -trimpath -ldflags "$ldflags" -o "${dist_dir}/${output}" "$package"
}

build_binary ./cmd/motekarctl motekarctl-linux-amd64
build_binary ./cmd/motekar-panel motekar-panel-linux-amd64
build_binary ./cmd/motekar-agent motekar-agent-linux-amd64

cp "${repo_root}/installers/install-ubuntu-24.04-amd64.sh" "${dist_dir}/install-ubuntu-24.04-amd64.sh"
chmod 0755 "${dist_dir}/install-ubuntu-24.04-amd64.sh"

(
  cd "$dist_dir"
  for artifact in motekarctl-linux-amd64 motekar-panel-linux-amd64 motekar-agent-linux-amd64 install-ubuntu-24.04-amd64.sh; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$artifact" > "${artifact}.sha256"
    else
      shasum -a 256 "$artifact" > "${artifact}.sha256"
    fi
  done
)

printf 'release artifacts written to %s\n' "$dist_dir"
