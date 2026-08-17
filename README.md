# Motekar Panel

Motekar Panel is a shared-hosting server control panel owned by Motekar Teknologi Indonesia.

Current implementation status: early Go foundation with panel API, Unix-socket agent API, configuration loading, structured logging, and health/version endpoints.

## Quick Start

Prerequisites:

- Go 1.24+

Run tests:

```bash
make test
```

Build binaries:

```bash
make build
```

Run the local agent in one terminal:

```bash
GOCACHE="$(pwd)/.cache/go-build" go run ./cmd/motekar-agent serve
```

Run the panel API in another terminal:

```bash
make dev
```

The panel API listens on `:8080` and connects to the agent through `.cache/motekar-agent.sock` by default.

Check it:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/version
```

For detailed local API and agent instructions, see [docs/runbooks/running-local-api.md](docs/runbooks/running-local-api.md).

## Database Migrations

The current migration runner targets PostgreSQL. Use a disposable development database:

```bash
MOTEKAR_DATABASE_URL="postgres://user:password@127.0.0.1:5432/motekar_panel?sslmode=disable" \
  GOCACHE="$(pwd)/.cache/go-build" \
  go run ./cmd/motekar-panel migrate up
```
