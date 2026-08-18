#!/usr/bin/env bash
set -euo pipefail

printf 'fake-motekarctl %s\n' "$*"

case "$1" in
  preflight)
    printf 'PASS\tos\tUbuntu 24.04 LTS is supported\n'
    ;;
  install)
    if [ "${2:-}" != "plan" ] && [ "${2:-}" != "apply" ]; then
      printf 'unexpected install subcommand\n' >&2
      exit 2
    fi
    printf 'Motekar Panel install %s\n' "$2"
    if [ "$2" = "plan" ]; then
      printf 'mode: dry-run\n'
      printf 'No changes were made.\n'
    else
      printf 'web_server: nginx\n'
      printf 'Motekar Panel installed.\n'
    fi
    ;;
  *)
    printf 'unexpected command: %s\n' "$1" >&2
    exit 2
    ;;
esac
