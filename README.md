# Motekar Panel

Motekar Panel is a shared-hosting server control panel owned by Motekar Teknologi Indonesia.

Current implementation status: early Go foundation with panel API, agent API, configuration loading, structured logging, and health/version endpoints.

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

Run the panel API:

```bash
make dev
```

The panel API listens on `:8080` by default.

Check it:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/version
```

For detailed local API and agent instructions, see [docs/runbooks/running-local-api.md](docs/runbooks/running-local-api.md).

