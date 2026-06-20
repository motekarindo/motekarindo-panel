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

Use `single-user` for a 1 GB RAM personal VPS. Use `shared-hosting` for a real shared-hosting target.

## Commands

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

For a 1 GB personal VPS:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl preflight \
  --profile single-user \
  --postgresql install
```

Run installer dry-run:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl install plan \
  --profile shared-hosting \
  --web-server nginx \
  --postgresql install
```

For a 1 GB personal VPS:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl install plan \
  --profile single-user \
  --web-server nginx \
  --postgresql install
```

## Expected Result

- Ubuntu 24.04 should pass OS detection.
- 1 GB RAM should warn in `single-user`.
- 1 GB RAM should fail in `shared-hosting`.
- Dry-run output must end with `No changes were made.`
- Any `WOULD_CHANGE` actions are informational only and must not be executed by this command.
