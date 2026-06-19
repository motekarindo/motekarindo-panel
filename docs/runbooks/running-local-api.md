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
| `MOTEKAR_ENV` | `development` | Runtime environment label |
| `MOTEKAR_LOG_LEVEL` | `info` | Structured log level |

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

Example:

```bash
curl http://127.0.0.1:9090/capabilities
```

Expected response:

```json
{"actions":["agent.health","agent.capabilities"]}
```

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

