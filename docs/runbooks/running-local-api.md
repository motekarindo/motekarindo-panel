# Running the Local API

This document explains how to run the current Motekar Panel API and agent during development.

## Prerequisites

- Go 1.24+
- A shell environment that can bind to localhost ports

PostgreSQL is part of the product architecture, but the current API skeleton does not connect to PostgreSQL yet. Database setup will be added with the migration layer.

## Development Commands

Run all tests:

```bash
make test
```

Build all binaries:

```bash
make build
```

Format Go files:

```bash
make fmt
```

Run the panel API:

```bash
make dev
```

`make dev` runs:

```bash
go run ./cmd/motekar-panel serve
```

The `Makefile` sets `GOCACHE` to `.cache/go-build` inside the workspace so sandboxed development environments do not try to write to the user-level Go cache.

## Panel API

Default address:

```text
:8080
```

Use `MOTEKAR_PANEL_ADDR` to override it:

```bash
MOTEKAR_PANEL_ADDR=127.0.0.1:18080 make dev
```

Available endpoints:

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Minimal HTML landing page |
| `GET` | `/healthz` | Liveness check |
| `GET` | `/readyz` | Readiness check |
| `GET` | `/version` | Build/version metadata |

Example:

```bash
curl http://127.0.0.1:8080/healthz
```

Expected response:

```json
{"status":"ok"}
```

Readiness check:

```bash
curl http://127.0.0.1:8080/readyz
```

Expected response:

```json
{"status":"ready"}
```

Version check:

```bash
curl http://127.0.0.1:8080/version
```

Expected response shape:

```json
{
  "name": "Motekar Panel",
  "version": "dev",
  "commit": "unknown",
  "date": "unknown"
}
```

## Panel Environment Variables

| Variable | Default | Description |
|---|---:|---|
| `MOTEKAR_PANEL_ADDR` | `:8080` | Panel API listen address |
| `MOTEKAR_DATABASE_URL` | empty | Future PostgreSQL connection string |
| `MOTEKAR_MIGRATIONS_DIR` | `services/migrations` | SQL migration directory |
| `MOTEKAR_ENV` | `development` | Runtime environment label |
| `MOTEKAR_LOG_LEVEL` | `info` | Structured log level |

## Database Migrations

The panel now includes a PostgreSQL migration runner and an initial core schema.

Run migrations with:

```bash
MOTEKAR_DATABASE_URL="postgres://user:password@127.0.0.1:5432/motekar_panel?sslmode=disable" \
  GOCACHE="$(pwd)/.cache/go-build" \
  go run ./cmd/motekar-panel migrate up
```

Or with a custom migrations directory:

```bash
MOTEKAR_DATABASE_URL="postgres://user:password@127.0.0.1:5432/motekar_panel?sslmode=disable" \
  MOTEKAR_MIGRATIONS_DIR="services/migrations" \
  GOCACHE="$(pwd)/.cache/go-build" \
  go run ./cmd/motekar-panel migrate up
```

Current migration files:

- `services/migrations/000001_initial_core.sql`
- `services/migrations/000002_seed_rbac.sql`

The migration runner records applied versions in `schema_migrations`.

Do not run database migrations against a production or important database until backup and restore procedures exist. For now, run them only against a disposable local database or a test VPS database.

### PostgreSQL Integration Test

Run the repeatable migration and core-schema smoke test against a disposable database:

```bash
MOTEKAR_TEST_ALLOW_DATABASE=1 \
  MOTEKAR_TEST_DATABASE_URL="postgres://motekar:motekar-test@127.0.0.1:5432/motekar_panel_test?sslmode=disable" \
  make test-integration-postgres
```

The test verifies migration idempotency, RBAC seeds, web-server settings, and core record insert/read behavior. It requires an explicit safety marker, refuses databases whose name does not end with `_test`, and creates an isolated temporary schema for each run; never point it at production or important data. CI runs this target with a disposable PostgreSQL 16 service.

## First Admin Bootstrap

After migrations have been applied to a disposable PostgreSQL database, create the first admin with:

```bash
printf '%s\n' 'change-this-long-password' | \
  MOTEKAR_DATABASE_URL="postgres://user:password@127.0.0.1:5432/motekar_panel?sslmode=disable" \
  GOCACHE="$(pwd)/.cache/go-build" \
  go run ./cmd/motekar-panel bootstrap admin \
    --email owner@example.com \
    --display-name "Owner" \
    --password-stdin
```

The password is read from stdin so it does not need to be passed as a command argument. The bootstrap command assigns the first admin to the seeded `owner` role and records `auth.bootstrap_admin.created` in the same database transaction. Do not run this against a production database until backup and restore procedures exist.

## Agent API

Run the agent:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekar-agent serve
```

Default address:

```text
127.0.0.1:9090
```

Use `MOTEKAR_AGENT_ADDR` to override it:

```bash
GOCACHE="$(pwd)/.cache/go-build" MOTEKAR_AGENT_ADDR=127.0.0.1:19090 go run ./cmd/motekar-agent serve
```

Available endpoints:

| Method | Path | Description |
|---|---|---|
| `GET` | `/healthz` | Agent liveness check |
| `GET` | `/capabilities` | Current allowlisted agent capabilities |
| `GET` | `/version` | Build/version metadata |
| `POST` | `/actions/{name}` | Execute an allowlisted agent action |

Example:

```bash
curl http://127.0.0.1:9090/capabilities
```

Expected response:

```json
{"actions":["agent.health","agent.capabilities"]}
```

Execute an allowlisted health action:

```bash
curl -X POST http://127.0.0.1:9090/actions/agent.health \
  -H "Content-Type: application/json" \
  -d '{"payload":{}}'
```

Expected response:

```json
{
  "action": "agent.health",
  "status": "ok",
  "data": {
    "status": "ok"
  }
}
```

Unknown actions return `404` with `UNKNOWN_ACTION`. This is intentional: future privileged server operations must be explicitly registered in the agent action registry.

## Agent Environment Variables

| Variable | Default | Description |
|---|---:|---|
| `MOTEKAR_AGENT_ADDR` | `127.0.0.1:9090` | Agent API listen address |
| `MOTEKAR_ENV` | `development` | Runtime environment label |
| `MOTEKAR_LOG_LEVEL` | `info` | Structured log level |

## CLI

Show CLI version:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl version
```

Show panel version:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekar-panel version
```

Show agent version:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekar-agent version
```

Show agent capabilities without running the agent HTTP server:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekar-agent capabilities
```

Run a host-safe sample preflight report:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl preflight sample
```

This command uses hardcoded sample facts for Ubuntu 24.04 LTS. It does not inspect or modify the host machine.

Run the real read-only preflight collector:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl preflight --profile shared-hosting --postgresql install
```

For a 1 GB personal VPS, use the single-user profile:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl preflight --profile single-user --postgresql install
```

The real collector reads OS release, CPU, RAM, swap, disk, root status, systemd status, and port availability. It does not install packages, edit files, create users, or start/stop services.

Run an installer dry-run plan with sample facts:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl install plan \
  --sample \
  --profile shared-hosting \
  --web-server nginx \
  --postgresql install
```

Run an installer dry-run plan with real read-only preflight facts:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekarctl install plan \
  --profile shared-hosting \
  --web-server nginx \
  --postgresql install
```

The install plan command is dry-run only. It prints actions that would change the host later, but it does not execute them.

## Installer Bootstrapper

The end-user installer should not require cloning the full repository. The first bootstrapper is:

```text
installers/install-ubuntu-24.04-amd64.sh
```

Current behavior:

- Dry-run only.
- Validates Ubuntu 24.04 amd64 unless `--skip-os-check` is used for automated tests.
- Downloads `motekarctl` from GitHub Releases by default, or uses `--local-binary`.
- Verifies the downloaded `motekarctl` checksum by default.
- Runs `motekarctl preflight`.
- Runs `motekarctl install plan`.
- Refuses `--apply` until actual install support exists.

Development test with a local binary on a disposable Ubuntu 24.04 VPS:

```bash
GOCACHE="$(pwd)/.cache/go-build" go build -o .cache/motekarctl ./cmd/motekarctl
installers/install-ubuntu-24.04-amd64.sh --dry-run \
  --skip-os-check \
  --skip-root-check \
  --local-binary .cache/motekarctl \
  --profile shared-hosting \
  --web-server nginx \
  --postgresql install
```

Installer script checks:

```bash
make test-installers
```

## Troubleshooting

### `operation not permitted` when running Go commands

If Go tries to write to the user-level build cache, run commands with a workspace-local cache:

```bash
GOCACHE="$(pwd)/.cache/go-build" go test ./...
```

The provided `Makefile` already does this.

### Port already in use

Use a different address:

```bash
MOTEKAR_PANEL_ADDR=127.0.0.1:18080 make dev
```

For the agent:

```bash
GOCACHE="$(pwd)/.cache/go-build" MOTEKAR_AGENT_ADDR=127.0.0.1:19090 go run ./cmd/motekar-agent serve
```
