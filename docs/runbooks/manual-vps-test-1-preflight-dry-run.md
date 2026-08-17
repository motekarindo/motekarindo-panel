# Manual VPS Test 1: Preflight and Dry Run

This test verifies read-only server detection and installer planning on a disposable Ubuntu 24.04 VPS.

Do not use an important server. Use a fresh VPS that can be rebuilt.

## Scope

This test is allowed to:

- Read OS release, CPU, RAM, swap, disk, root status, systemd status, and port availability.
- Print an installer dry-run plan.

This test must not:

- Install packages.
- Create Linux users.
- Write panel config.
- Modify Nginx, Apache, PostgreSQL, firewall, DNS, or systemd.
- Start or stop services.

## VPS Requirement

- Ubuntu 24.04 LTS.
- Root or sudo shell.
- Fresh server.
- Rebuildable after test.
- At least 15 GB free disk space for `single-user` or 20 GB for `shared-hosting`.

Use `single-user` for a 1 GB RAM personal VPS. Use `shared-hosting` for a real shared-hosting target.

## Commands With Installer Script

When a release binary is available, the end-user flow should use only the installer script:

```bash
curl -fsSLO https://github.com/motekarindo/motekarindo-panel/releases/latest/download/install-ubuntu-24.04-amd64.sh
chmod +x install-ubuntu-24.04-amd64.sh
```

Run dry-run for shared hosting:

```bash
./install-ubuntu-24.04-amd64.sh --dry-run \
  --profile shared-hosting \
  --web-server nginx \
  --postgresql install
```

The script downloads `motekarctl-linux-amd64` from GitHub Releases and verifies its `.sha256` checksum by default.

Run dry-run for a 1 GB personal VPS:

```bash
./install-ubuntu-24.04-amd64.sh --dry-run \
  --profile single-user \
  --web-server nginx \
  --postgresql install
```

For development before a release binary exists, build or copy `motekarctl` to the VPS and pass it explicitly:

```bash
./install-ubuntu-24.04-amd64.sh --dry-run \
  --local-binary ./motekarctl \
  --profile shared-hosting \
  --web-server nginx \
  --postgresql install
```

## Commands From Repository Checkout

Repository checkout is only needed for development testing before release artifacts exist.

Clone and enter the repository:

```bash
git clone https://github.com/motekarindo/motekarindo-panel.git
cd motekarindo-panel
```

Run read-only preflight:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl preflight \
  --profile shared-hosting \
  --postgresql install
```

## Expected Result

- Ubuntu 24.04 should pass OS detection.
- A nominal 1 GB VM reporting at least 960 MB RAM should warn but pass in `single-user`.
- 1 GB RAM should fail in `shared-hosting`.
- 1 GB swap should warn but pass in `single-user`; less than 1 GB should fail.
- 15 GB free disk space should pass in `single-user`.
- Less than 20 GB free disk space should fail in `shared-hosting`.
- Dry-run output must include `No changes were made.`
- Any `WOULD_CHANGE` actions are informational only and must not be executed by this command.

## Verification Record

Re-verified on 2026-08-17 on a disposable Ubuntu 24.04 VPS (2 vCPU, 2048 MB RAM, 24 GB disk).

- `single-user` dry-run passed: OS/cpu/disk/root/systemd/postgresql/ports all PASS; memory (1967 MB) and swap (2047 MB) reported as WARN with the expected recommendation; plan printed `WOULD_CHANGE` actions and completed without changes.
- `shared-hosting` dry-run correctly blocked at preflight with `FAIL memory` because 2048 MB is the minimum and the VPS reported 1967 MB; `motekarctl: preflight has blocking failures` prevented the plan from running. This matches the requirement that shared-hosting still blocks a 1 GB VM.
- `--apply` was refused with `error: --apply is not available yet; this bootstrapper is dry-run only`.
- The release script downloaded `motekarctl-linux-amd64` from GitHub Releases and verified its checksum (`motekarctl: OK`) before running preflight.
