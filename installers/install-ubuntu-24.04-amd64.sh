#!/usr/bin/env bash
set -euo pipefail

readonly INSTALLER_NAME="install-ubuntu-24.04-amd64.sh"
readonly DEFAULT_PROFILE="shared-hosting"
readonly DEFAULT_WEB_SERVER="nginx"
readonly DEFAULT_POSTGRESQL="install"
readonly DEFAULT_BINARY_NAME="motekarctl-linux-amd64"
readonly DEFAULT_RELEASE_BASE_URL="https://github.com/motekarindo/motekarindo-panel/releases/latest/download"

profile="${DEFAULT_PROFILE}"
web_server="${DEFAULT_WEB_SERVER}"
postgresql="${DEFAULT_POSTGRESQL}"
mode="dry-run"
local_binary=""
download_url=""
checksum_url=""
verify_checksum="1"
skip_os_check="0"
skip_root_check="0"

usage() {
  cat <<USAGE
Motekar Panel Ubuntu 24.04 amd64 installer bootstrapper.

Usage:
  ${INSTALLER_NAME} --dry-run [options]

Options:
  --dry-run                  Print preflight and install plan only. This is the only supported mode for now.
  --apply                    Reserved for future actual installs. Currently refused.
  --profile VALUE            Install profile: shared-hosting or single-user. Default: ${DEFAULT_PROFILE}
  --web-server VALUE         Web server: nginx or apache. Default: ${DEFAULT_WEB_SERVER}
  --postgresql VALUE         PostgreSQL plan: install or external. Default: ${DEFAULT_POSTGRESQL}
  --local-binary PATH        Use an existing motekarctl binary instead of downloading one.
  --download-url URL         Download motekarctl from this URL.
  --checksum-url URL         sha256 checksum URL for downloaded motekarctl.
  --no-checksum              Skip checksum verification. Intended only for development/custom mirrors.
  --skip-os-check            Skip OS/architecture validation. Intended only for automated tests.
  --skip-root-check          Skip root validation. Intended only for automated tests.
  -h, --help                 Show this help.

Examples:
  ${INSTALLER_NAME} --dry-run --profile single-user --web-server nginx
  ${INSTALLER_NAME} --dry-run --local-binary ./motekarctl --profile shared-hosting --web-server nginx
USAGE
}

log() {
  printf '[motekar-installer] %s\n' "$*"
}

fail() {
  printf '[motekar-installer] error: %s\n' "$*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dry-run)
      mode="dry-run"
      shift
      ;;
    --apply)
      mode="apply"
      shift
      ;;
    --profile)
      [ "$#" -ge 2 ] || fail "--profile requires a value"
      profile="$2"
      shift 2
      ;;
    --web-server)
      [ "$#" -ge 2 ] || fail "--web-server requires a value"
      web_server="$2"
      shift 2
      ;;
    --postgresql)
      [ "$#" -ge 2 ] || fail "--postgresql requires a value"
      postgresql="$2"
      shift 2
      ;;
    --local-binary)
      [ "$#" -ge 2 ] || fail "--local-binary requires a value"
      local_binary="$2"
      shift 2
      ;;
    --download-url)
      [ "$#" -ge 2 ] || fail "--download-url requires a value"
      download_url="$2"
      shift 2
      ;;
    --checksum-url)
      [ "$#" -ge 2 ] || fail "--checksum-url requires a value"
      checksum_url="$2"
      shift 2
      ;;
    --no-checksum)
      verify_checksum="0"
      shift
      ;;
    --skip-os-check)
      skip_os_check="1"
      shift
      ;;
    --skip-root-check)
      skip_root_check="1"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[ "$mode" = "dry-run" ] || fail "--apply is not available yet; this bootstrapper is dry-run only"

case "$profile" in
  shared-hosting|single-user) ;;
  *) fail "unsupported profile: ${profile}" ;;
esac

case "$web_server" in
  nginx|apache) ;;
  *) fail "unsupported web server: ${web_server}" ;;
esac

case "$postgresql" in
  install|external) ;;
  *) fail "unsupported PostgreSQL plan: ${postgresql}" ;;
esac

if [ "$skip_root_check" != "1" ] && [ "${EUID}" -ne 0 ]; then
  fail "run as root so preflight can validate the real installer environment"
fi

os_release_path="${MOTEKAR_INSTALLER_OS_RELEASE:-/etc/os-release}"
machine="${MOTEKAR_INSTALLER_UNAME_M:-$(uname -m)}"

if [ "$skip_os_check" != "1" ]; then
  [ -r "$os_release_path" ] || fail "cannot read ${os_release_path}"
  # shellcheck disable=SC1090
  . "$os_release_path"
  [ "${ID:-}" = "ubuntu" ] || fail "unsupported OS ID: ${ID:-unknown}; this installer targets Ubuntu 24.04"
  [ "${VERSION_ID:-}" = "24.04" ] || fail "unsupported Ubuntu version: ${VERSION_ID:-unknown}; this installer targets Ubuntu 24.04"
  [ "$machine" = "x86_64" ] || [ "$machine" = "amd64" ] || fail "unsupported architecture: ${machine}; this installer targets amd64"
fi

tmp_dir=""
cleanup() {
  if [ -n "$tmp_dir" ] && [ -d "$tmp_dir" ]; then
    rm -rf "$tmp_dir"
  fi
}
trap cleanup EXIT

motekarctl_path=""
if [ -n "$local_binary" ]; then
  [ -x "$local_binary" ] || fail "--local-binary must point to an executable motekarctl binary"
  motekarctl_path="$local_binary"
else
  command -v curl >/dev/null 2>&1 || fail "curl is required to download motekarctl; install curl or pass --local-binary"
  tmp_dir="$(mktemp -d)"
  motekarctl_path="${tmp_dir}/motekarctl"
  if [ -z "$download_url" ]; then
    download_url="${DEFAULT_RELEASE_BASE_URL}/${DEFAULT_BINARY_NAME}"
  fi
  if [ -z "$checksum_url" ]; then
    checksum_url="${download_url}.sha256"
  fi
  log "downloading motekarctl from ${download_url}"
  curl -fsSL "$download_url" -o "$motekarctl_path"
  chmod 0755 "$motekarctl_path"

  if [ "$verify_checksum" = "1" ]; then
    command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required for checksum verification"
    log "verifying motekarctl checksum from ${checksum_url}"
    curl -fsSL "$checksum_url" -o "${tmp_dir}/motekarctl.sha256"
    expected_checksum="$(awk '{print $1}' "${tmp_dir}/motekarctl.sha256")"
    [ -n "$expected_checksum" ] || fail "checksum file is empty"
    (
      cd "$tmp_dir"
      printf '%s  motekarctl\n' "$expected_checksum" | sha256sum -c -
    )
  else
    log "checksum verification skipped by explicit --no-checksum"
  fi
fi

log "running read-only preflight"
"$motekarctl_path" preflight \
  --profile "$profile" \
  --postgresql "$postgresql"

log "running installer dry-run plan"
"$motekarctl_path" install plan \
  --profile "$profile" \
  --web-server "$web_server" \
  --postgresql "$postgresql"

log "dry-run completed; no changes were made"
