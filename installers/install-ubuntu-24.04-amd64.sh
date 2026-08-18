#!/usr/bin/env bash
set -euo pipefail

readonly INSTALLER_NAME="install-ubuntu-24.04-amd64.sh"
readonly DEFAULT_PROFILE="single-user"
readonly DEFAULT_WEB_SERVER="nginx"
readonly DEFAULT_POSTGRESQL="install"
readonly DEFAULT_BIN_DIR="/usr/local/bin"
readonly RELEASE_ARTIFACTS=("motekarctl-linux-amd64" "motekar-panel-linux-amd64" "motekar-agent-linux-amd64")
readonly DEFAULT_RELEASE_BASE_URL="https://github.com/motekarindo/motekarindo-panel/releases/latest/download"

profile="${DEFAULT_PROFILE}"
web_server="${DEFAULT_WEB_SERVER}"
postgresql="${DEFAULT_POSTGRESQL}"
mode="dry-run"
bin_dir="${DEFAULT_BIN_DIR}"
local_binary_dir=""
local_binary=""
download_base_url="${DEFAULT_RELEASE_BASE_URL}"
verify_checksum="1"
skip_os_check="0"
skip_root_check="0"
admin_email=""
admin_display_name=""
admin_password=""

usage() {
  cat <<USAGE
Motekar Panel Ubuntu 24.04 amd64 installer bootstrapper.

Usage:
  ${INSTALLER_NAME} --apply [options]
  ${INSTALLER_NAME} --dry-run [options]

Options:
  --apply                    Install Motekar Panel (PostgreSQL, web server, panel, agent, systemd services, first admin).
  --dry-run                  Print preflight and install plan only. No changes are made.
  --profile VALUE            Install profile: shared-hosting or single-user. Default: ${DEFAULT_PROFILE}
  --web-server VALUE         Web server: nginx or apache. Default: ${DEFAULT_WEB_SERVER}
  --postgresql VALUE         PostgreSQL plan: install or external. Default: ${DEFAULT_POSTGRESQL}
  --admin-email EMAIL        First admin email. Prompted interactively when applying and not provided.
  --admin-display-name NAME  First admin display name. Prompted interactively when applying and not provided.
  --admin-password VALUE     First admin password. Prompted interactively when applying and not provided.
  --bin-dir DIR              Install binaries to DIR. Default: ${DEFAULT_BIN_DIR}
  --local-binary PATH        Use an existing motekarctl binary instead of downloading one (dry-run only).
  --local-binary-dir DIR     Use existing motekarctl, motekar-panel, and motekar-agent binaries from DIR.
  --download-url URL         Download binaries from this base URL.
  --no-checksum              Skip checksum verification. Intended only for development/custom mirrors.
  --skip-os-check            Skip OS/architecture validation. Intended only for automated tests.
  --skip-root-check          Skip root validation. Intended only for automated tests.
  -h, --help                 Show this help.

Examples:
  ${INSTALLER_NAME} --dry-run --profile single-user --web-server nginx
  ${INSTALLER_NAME} --apply --profile single-user --web-server nginx
  ${INSTALLER_NAME} --apply --local-binary-dir ./dist --admin-email owner@example.com
USAGE
}

log() {
  printf '[motekar-installer] %s\n' "$*"
}

fail() {
  printf '[motekar-installer] error: %s\n' "$*" >&2
  exit 1
}

prompt_secret() {
  local label="$1"
  local value=""
  local confirm=""
  while true; do
    read -r -s -p "[motekar-installer] ${label}: " value
    printf '\n' >&2
    [ -n "$value" ] || {
      printf '[motekar-installer] %s cannot be empty\n' "$label" >&2
      continue
    }
    read -r -s -p "[motekar-installer] ${label} (again): " confirm
    printf '\n' >&2
    [ "$value" = "$confirm" ] || {
      printf '[motekar-installer] %s values do not match\n' "$label" >&2
      continue
    }
    printf '%s' "$value"
    return 0
  done
}

prompt_value() {
  local label="$1"
  local value=""
  read -r -p "[motekar-installer] ${label}: " value
  printf '%s' "$value"
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
    --admin-email)
      [ "$#" -ge 2 ] || fail "--admin-email requires a value"
      admin_email="$2"
      shift 2
      ;;
    --admin-display-name)
      [ "$#" -ge 2 ] || fail "--admin-display-name requires a value"
      admin_display_name="$2"
      shift 2
      ;;
    --admin-password)
      [ "$#" -ge 2 ] || fail "--admin-password requires a value"
      admin_password="$2"
      shift 2
      ;;
    --bin-dir)
      [ "$#" -ge 2 ] || fail "--bin-dir requires a value"
      bin_dir="$2"
      shift 2
      ;;
    --local-binary)
      [ "$#" -ge 2 ] || fail "--local-binary requires a value"
      local_binary="$2"
      shift 2
      ;;
    --local-binary-dir)
      [ "$#" -ge 2 ] || fail "--local-binary-dir requires a value"
      local_binary_dir="$2"
      shift 2
      ;;
    --download-url)
      [ "$#" -ge 2 ] || fail "--download-url requires a value"
      download_base_url="$2"
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

[ "$mode" = "apply" ] || [ "$mode" = "dry-run" ] || fail "invalid mode: ${mode}"

if [ "$skip_root_check" != "1" ] && [ "${EUID}" -ne 0 ]; then
  fail "run as root so the installer can install Motekar Panel"
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

find_binary() {
  local name="$1"
  if [ -n "$local_binary_dir" ]; then
    local candidate="${local_binary_dir}/${name}"
    [ -x "$candidate" ] || fail "--local-binary-dir must contain an executable ${name}"
    printf '%s' "$candidate"
    return 0
  fi
  command -v curl >/dev/null 2>&1 || fail "curl is required to download binaries; install curl or pass --local-binary-dir"
  if [ -z "$tmp_dir" ]; then
    tmp_dir="$(mktemp -d)"
  fi
  local target="${tmp_dir}/${name}"
  log "downloading ${name} from ${download_base_url}/${name}" >&2
  curl -fsSL "${download_base_url}/${name}" -o "$target"
  chmod 0755 "$target"
  if [ "$verify_checksum" = "1" ]; then
    command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required for checksum verification"
    log "verifying ${name} checksum" >&2
    curl -fsSL "${download_base_url}/${name}.sha256" -o "${tmp_dir}/${name}.sha256"
    expected_checksum="$(awk '{print $1}' "${tmp_dir}/${name}.sha256")"
    [ -n "$expected_checksum" ] || fail "checksum file for ${name} is empty"
    (
      cd "$tmp_dir"
      printf '%s  %s\n' "$expected_checksum" "$name" | sha256sum -c - >&2
    )
  fi
  printf '%s' "$target"
  return 0
}

run_dry_run() {
  local motekarctl_path
  if [ -n "$local_binary" ]; then
    [ -x "$local_binary" ] || fail "--local-binary must point to an executable motekarctl binary"
    motekarctl_path="$local_binary"
  else
    motekarctl_path="$(find_binary "motekarctl-linux-amd64")"
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
}

run_apply() {
  [ -n "$local_binary" ] && fail "--local-binary is only supported with --dry-run"

  if [ "$postgresql" = "external" ] && [ -z "${MOTEKAR_DATABASE_URL:-}" ]; then
    fail "the external PostgreSQL plan requires MOTEKAR_DATABASE_URL to be set"
  fi

  local motekarctl_path
  motekarctl_path="$(find_binary "motekarctl-linux-amd64")"
  local panel_binary
  panel_binary="$(find_binary "motekar-panel-linux-amd64")"
  local agent_binary
  agent_binary="$(find_binary "motekar-agent-linux-amd64")"

  log "installing binaries to ${bin_dir}"
  mkdir -p "$bin_dir"
  install -m 0755 "$motekarctl_path" "${bin_dir}/motekarctl"
  install -m 0755 "$panel_binary" "${bin_dir}/motekar-panel"
  install -m 0755 "$agent_binary" "${bin_dir}/motekar-agent"

  if [ -z "$admin_email" ]; then
    admin_email="$(prompt_value "First admin email")"
  fi
  if [ -z "$admin_display_name" ]; then
    admin_display_name="$(prompt_value "First admin display name")"
  fi
  if [ -z "$admin_password" ]; then
    admin_password="$(prompt_secret "First admin password (min 12 characters)")"
  fi
  [ -n "$admin_email" ] || fail "admin email is required"
  [ -n "$admin_display_name" ] || fail "admin display name is required"
  [ "${#admin_password}" -ge 12 ] || fail "admin password must be at least 12 characters"

  log "running read-only preflight"
  "${bin_dir}/motekarctl" preflight \
    --profile "$profile" \
    --postgresql "$postgresql"

  log "installing Motekar Panel (this can take several minutes)"
  printf '%s\n' "$admin_password" | "${bin_dir}/motekarctl" install apply \
    --profile "$profile" \
    --web-server "$web_server" \
    --postgresql "$postgresql" \
    --bin-dir "$bin_dir" \
    --agent-socket "/run/motekar-panel/agent.sock" \
    --admin-email "$admin_email" \
    --admin-display-name "$admin_display_name" \
    --admin-password-stdin

  log "install completed"
  printf '\nMotekar Panel is installed.\n'
  printf '  Panel: http://<server-ip>:8080\n'
  printf '  Admin: %s\n' "$admin_email"
  printf '\nManage it with: systemctl status motekar-panel motekar-agent\n'
}

if [ "$mode" = "dry-run" ]; then
  run_dry_run
else
  run_apply
fi
